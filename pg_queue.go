package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"log"
	"sync"

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

	countUnprocessedSQL = `
    SELECT COUNT(*) FROM proxy_requests WHERE processed = 'f';
  `
	countTotalSQL = `
    SELECT COUNT(*) FROM proxy_requests;
  `
)

var (
	EmptyQueueError = errors.New("queue is empty")
)

type PgQueue struct {
	sync.RWMutex
	db *sql.DB
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

	queue := &PgQueue{db: db}

	return queue, nil
}

func (q *PgQueue) DeleteStale() {
	_, err := q.db.Exec(deleteStaleSQL)
	if err != nil {
		log.Println(err)
	}
}

func (q *PgQueue) Shutdown() error {
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
	q.Lock()
	defer q.Unlock()

	tx, err := q.db.Begin()
	if err != nil {
		return nil, err
	}

	proxyRequest, id, err := q.selectFirstUnprocessed(tx)
	if err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return nil, fmt.Errorf("rollback error: %v: %v", rollbackErr, err)
		}

		return nil, err
	}

	_, err = tx.Exec(setStaleSQL, id)
	if err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return nil, fmt.Errorf("rollback error: %v: %v", rollbackErr, err)
		}
		return nil, err
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("commit error: %v", err)
	}

	return proxyRequest, nil
}

func (q *PgQueue) selectFirstUnprocessed(tx *sql.Tx) (*p.ProxyRequest, string, error) {
	var (
		id           string
		headers      []byte
		proxyRequest p.ProxyRequest
		err          error
	)

	rows, err := tx.Query(selectSQL)
	if err != nil {
		return nil, id, err
	}
	defer rows.Close()

	if rows.Next() {
		err = rows.Scan(
			&id,
			&proxyRequest.Method,
			&headers,
			&proxyRequest.Body,
			&proxyRequest.OriginURL,
		)
		if err != nil {
			return nil, id, err
		}
	} else {
		return nil, id, EmptyQueueError
	}

	err = json.Unmarshal(headers, &proxyRequest.Header)
	if err != nil {
		return nil, id, err
	}

	return &proxyRequest, id, nil
}

func (q *PgQueue) GetUnprocessed() (cnt uint64) {
	_ = q.db.QueryRow(countUnprocessedSQL).Scan(&cnt)
	return
}

func (q *PgQueue) GetTotal() (cnt uint64) {
	_ = q.db.QueryRow(countTotalSQL).Scan(&cnt)
	return
}
