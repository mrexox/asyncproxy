package proxy

// NOTE: Do not use prepared statements. This service is supposed to
// be used with pg_bouncer in Transaction pooling mode, which does not
// support prepared statements.
// See: https://www.pgbouncer.org/features.html

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	_ "github.com/lib/pq"
)

const (
	insertSQL = `
    INSERT INTO proxy_requests (
      timestamp, id, method, header, body, origin_url, attempt
    ) VALUES (now(), $1, $2, $3, $4, $5, $6);
  `

	selectSQL = `
    SELECT id, method, header, body, origin_url, attempt
    FROM proxy_requests
    ORDER BY date_trunc('minute', timestamp) ASC
    LIMIT 1
    FOR UPDATE
    SKIP LOCKED;
  `

	deleteSQL = `
    DELETE FROM proxy_requests WHERE id = $1;
  `

	countTotalSQL = `
    SELECT COUNT(*) FROM proxy_requests;
  `
)

var (
	EmptyQueueError = errors.New("queue is empty")
)

type PgQueue struct {
	db *sql.DB
}

type record struct {
	request *ProxyRequest
	id      string
	attempt int
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

func (q *PgQueue) Total() (cnt uint64) {
	_ = q.db.QueryRow(countTotalSQL).Scan(&cnt)
	return
}

func (q *PgQueue) Shutdown() error {
	q.db.Close()

	return nil
}

// Put request into the database
func (q *PgQueue) EnqueueRequest(r *ProxyRequest, attempt int) error {
	headers, err := json.Marshal(r.Header)
	if err != nil {
		return err
	}

	_, err = q.db.Exec(
		insertSQL, uuid.New(), r.Method, headers, r.Body, r.OriginURL, attempt,
	)
	if err != nil {
		return err
	}

	return nil
}

// Get request fron the database
func (q *PgQueue) DequeueRequest(ctx context.Context) (*ProxyRequest, int, error) {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()

	// Get the record
	record, err := q.selectOne(ctx, tx)
	if err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return nil, 0, fmt.Errorf("rollback error: %v: %v", rollbackErr, err)
		}

		return nil, 0, err
	}

	// Delete the record
	_, err = tx.ExecContext(ctx, deleteSQL, record.id)
	if err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return nil, 0, fmt.Errorf("rollback error: %v: %v", rollbackErr, err)
		}
		return nil, 0, err
	}

	err = tx.Commit()
	if err != nil {
		return nil, 0, fmt.Errorf("commit error: %v", err)
	}

	return record.request, record.attempt, nil
}

func (q *PgQueue) selectOne(ctx context.Context, tx *sql.Tx) (record, error) {
	var (
		id           string
		headers      []byte
		proxyRequest ProxyRequest
		err          error
		attempt      int
	)

	row := tx.QueryRowContext(ctx, selectSQL)

	err = row.Scan(
		&id,
		&proxyRequest.Method,
		&headers,
		&proxyRequest.Body,
		&proxyRequest.OriginURL,
		&attempt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return record{}, EmptyQueueError
		}

		return record{}, err
	}

	err = json.Unmarshal(headers, &proxyRequest.Header)
	if err != nil {
		return record{}, err
	}

	return record{&proxyRequest, id, attempt}, nil
}
