package queues

import (
	"github.com/beeker1121/goque"
	p "github.com/evilmartians/asyncproxy/proxy"
)

type LevelDBQueue struct {
	queue *goque.Queue
}

func NewLevelDBQueue(dbName string) (*LevelDBQueue, error) {
	q, err := goque.OpenQueue(dbName)
	if err != nil {
		return nil, err
	}

	return &LevelDBQueue{queue: q}, nil
}

func (q *LevelDBQueue) Shutdown() error {
	q.queue.Close()

	return nil
}

func (q *LevelDBQueue) EnqueueRequest(r *p.ProxyRequest) error {
	_, err := q.queue.EnqueueObject(*r)
	if err != nil {
		return err
	}

	return nil
}

func (q *LevelDBQueue) DequeueRequest() (*p.ProxyRequest, error) {
	var (
		item *goque.Item
		err  error
	)

	for {
		item, err = q.queue.Dequeue()
		if err != goque.ErrEmpty {
			break
		}
	}

	if err != nil {
		return nil, err
	}

	var proxyRequest p.ProxyRequest

	if err = item.ToObject(&proxyRequest); err != nil {
		return nil, err
	}

	return &proxyRequest, nil
}
