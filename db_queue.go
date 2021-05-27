package main

import (
	"github.com/beeker1121/goque"
)

type DbQueue struct {
	queue *goque.Queue
}

func NewDbQueue(dbName string) (*DbQueue, error) {
	q, err := goque.OpenQueue(dbName)
	if err != nil {
		return nil, err
	}

	return &DbQueue{queue: q}, nil
}

func (fq *DbQueue) Shutdown() error {
	fq.queue.Close()

	return nil
}

func (fq *DbQueue) EnqueueRequest(r *ProxyRequest) error {
	_, err := fq.queue.EnqueueObject(*r)
	if err != nil {
		return err
	}

	return nil
}

func (fq *DbQueue) DequeueRequest() (*ProxyRequest, error) {
	var (
		item *goque.Item
		err  error
	)

	for {
		item, err = fq.queue.Dequeue()
		if err != goque.ErrEmpty {
			break
		}
	}

	if err != nil {
		return nil, err
	}

	var proxyRequest ProxyRequest

	if err = item.ToObject(&proxyRequest); err != nil {
		return nil, err
	}

	return &proxyRequest, nil
}
