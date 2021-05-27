package main

import (
	"fmt"
)

const (
	redisQueueType = "redis"
	dbQueueType    = "db"
)

type Queue interface {
	Shutdown() error
	EnqueueRequest(r *ProxyRequest) error
	DequeueRequest() (*ProxyRequest, error)
}

type QueueOptions struct {
	RedisKey, RedisUrl, DbName string
	QueueType                  string
}

func NewQueue(opts *QueueOptions) (Queue, error) {
	switch opts.QueueType {
	case redisQueueType:
		return NewRedisQueue(opts.RedisKey, opts.RedisUrl)
	case dbQueueType:
		return NewDbQueue(opts.DbName)
	default:
		return nil, fmt.Errorf("Unknown queue type: %s", opts.QueueType)
	}
}
