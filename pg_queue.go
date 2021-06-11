package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/google/uuid"
	"sync"
	"time"

	_ "github.com/lib/pq"

	p "github.com/evilmartians/asyncproxy/proxy"
)

const (
	insertSQL = `
    INSERT INTO proxy_requests (
      timestamp, processed, id, method, header, body, origin_url
    ) VALUES (now(), false, $1, $2, $3, $4, $5);
  `

	selectSQL = `
    SELECT id, method, header, body, origin_url
    FROM proxy_requests
    WHERE processed = false
    ORDER BY timestamp ASC
    LIMIT 1
    FOR UPDATE
    SKIP LOCKED;
  `

	deleteStaleSQL = `
    DELETE FROM proxy_requests WHERE processed = 't';
  `

	setStaleSQL = `
    UPDATE proxy_requests SET processed = true WHERE id = $1;
  `
)

type PgQueue struct {
	sync.RWMutex
	db       *sql.DB
	shutdown context.CancelFunc
}

func NewPgQueue(connString string, maxConns int) (*PgQueue, error) {
	db, err := sql.Open("postgres", connString)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(maxConns)

	err = db.Ping()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	queue := &PgQueue{db: db, shutdown: cancel}

	go queue.deleteStale(ctx)

	return queue, nil
}

func (q *PgQueue) deleteStale(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			time.Sleep(10 * time.Second)
			q.db.Exec(deleteStaleSQL)
		}
	}
}

func (q *PgQueue) Shutdown() error {
	q.shutdown()
	q.db.Close()

	return nil
}

func (q *PgQueue) EnqueueRequest(r *p.ProxyRequest) error {
	statement, err := q.db.Prepare(insertSQL)
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

func (q *PgQueue) DequeueRequest() (*p.ProxyRequest, error) {
	var (
		anything     bool
		id           string
		proxyRequest p.ProxyRequest
		headers      []byte
	)

	q.Lock()
	defer q.Unlock()

	tx, err := q.db.Begin()
	if err != nil {
		return nil, err
	}

	for !anything {
		rows, err := tx.Query(selectSQL)
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

	err = json.Unmarshal(headers, &proxyRequest.Header)
	if err != nil {
		return nil, err
	}

	statement, err := tx.Prepare(setStaleSQL)
	if err != nil {
		return nil, err
	}
	defer statement.Close()

	_, err = statement.Exec(id)
	if err != nil {
		return nil, err
	}

	err = tx.Commit()
	if err != nil {
		return nil, err
	}

	return &proxyRequest, nil
}
