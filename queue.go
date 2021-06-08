package main

import (
	"fmt"

	p "github.com/evilmartians/asyncproxy/proxy"
	q "github.com/evilmartians/asyncproxy/queues"
)

const (
	sqliteQueueType  = "sqlite"
	leveldbQueueType = "leveldb"
)

type Queue interface {
	Shutdown() error
	EnqueueRequest(r *p.ProxyRequest) error
	DequeueRequest() (*p.ProxyRequest, error)
}

type QueueOptions struct {
	DBName    string
	QueueType string
}

func NewQueue(opts *QueueOptions) (Queue, error) {
	switch opts.QueueType {
	case leveldbQueueType:
		return q.NewLevelDBQueue(opts.DBName)
	case sqliteQueueType:
		return q.NewSQLiteQueue(opts.DBName)

	default:
		return nil, fmt.Errorf("Unknown queue type: %s", opts.QueueType)
	}
}
