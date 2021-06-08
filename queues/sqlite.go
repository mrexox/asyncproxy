package queues

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"sync"
	"time"

	p "github.com/evilmartians/asyncproxy/proxy"
)

const (
	sqlCreateDatabase = `
    CREATE TABLE IF NOT EXISTS proxy_requests (
      id TEXT,
      timestamp DATETIME,
      processed BOOLEAN,
      Method TEXT,
      Header TEXT,
      Body TEXT,
      OriginURL TEXT
    );
  `
	sqlInsert = `
    INSERT INTO proxy_requests (
      id,
      timestamp,
      processed,
      Method,
      Header,
      Body,
      OriginURL
    ) VALUES (?, CURRENT_TIMESTAMP, false, ?, ?, ?, ?);
  `
	sqlSelect = `
    SELECT id, Method, Header, Body, OriginURL
    FROM proxy_requests
    WHERE processed = false
    ORDER BY datetime(timestamp) ASC
    LIMIT 1;
  `
	sqlDeleteStale = `
    DELETE FROM proxy_requests WHERE processed = true;
  `
)

type SQLiteQueue struct {
	sync.RWMutex
	db       *sql.DB
	shutdown context.CancelFunc
}

func NewSQLiteQueue(dbName string) (*SQLiteQueue, error) {
	db, err := sql.Open("sqlite3", dbName)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1) // prevents locks on inserting

	// Creating DB
	_, err = db.Exec(sqlCreateDatabase)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	queue := &SQLiteQueue{db: db, shutdown: cancel}
	go queue.deleteStale(ctx)
	return queue, nil
}

func (q *SQLiteQueue) deleteStale(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			time.Sleep(10 * time.Second)
			q.db.Exec(sqlDeleteStale)
		}
	}
}

func (q *SQLiteQueue) Shutdown() error {
	q.shutdown()
	q.db.Close()

	return nil
}

func (q *SQLiteQueue) EnqueueRequest(r *p.ProxyRequest) error {
	statement, err := q.db.Prepare(sqlInsert)
	if err != nil {
		return err
	}
	defer statement.Close()

	headers, err := json.Marshal(r.Header)
	if err != nil {
		return err
	}

	_, err = statement.Exec(uuid.New(), r.Method, headers, r.Body, r.OriginURL)
	if err != nil {
		return err
	}

	return nil
}

func (q *SQLiteQueue) DequeueRequest() (*p.ProxyRequest, error) {
	var (
		anything     bool
		id           string
		proxyRequest p.ProxyRequest
		headers      []byte
	)

	q.Lock()
	defer q.Unlock()

	for !anything {
		rows, err := q.db.Query(sqlSelect)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		for rows.Next() {
			anything = true
			err = rows.Scan(
				&id,
				&proxyRequest.Method,
				&headers,
				&proxyRequest.Body,
				&proxyRequest.OriginURL,
			)
			if err != nil {
				return nil, err
			}
		}
		if !anything {
			time.Sleep(100 * time.Millisecond)
		}
	}

	err := json.Unmarshal(headers, &proxyRequest.Header)
	if err != nil {
		return nil, err
	}

	updateSQL := `
    UPDATE proxy_requests SET processed = true WHERE id = ?;
  `

	statement, err := q.db.Prepare(updateSQL)
	if err != nil {
		return nil, err
	}
	defer statement.Close()

	_, err = statement.Exec(id)
	if err != nil {
		return nil, err
	}

	return &proxyRequest, nil
}
