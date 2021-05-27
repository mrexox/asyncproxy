package main

import (
	"fmt"

	p "github.com/evilmartians/asyncproxy/proxy"
	q "github.com/evilmartians/asyncproxy/queues"
)

const (
	redisQueueType = "redis"
	dbQueueType    = "db"
)

type Queue interface {
	Shutdown() error
	EnqueueRequest(r *p.ProxyRequest) error
	DequeueRequest() (*p.ProxyRequest, error)
}

type QueueOptions struct {
	RedisKey, RedisUrl, DbName string
	QueueType                  string
}

func NewQueue(opts *QueueOptions) (Queue, error) {
	switch opts.QueueType {
	case redisQueueType:
		return q.NewRedisQueue(opts.RedisKey, opts.RedisUrl)
	case dbQueueType:
		return q.NewDbQueue(opts.DbName)
	default:
		return nil, fmt.Errorf("Unknown queue type: %s", opts.QueueType)
	}
}
