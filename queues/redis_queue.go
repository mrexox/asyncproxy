package queues

import (
	"encoding/json"

	p "github.com/evilmartians/asyncproxy/proxy"
	"github.com/go-redis/redis"
)

type RedisQueue struct {
	client *redis.Client
	key    string
}

func NewRedisQueue(key, urlStr string) (*RedisQueue, error) {
	options, err := redis.ParseURL(urlStr)
	if err != nil {
		return nil, err
	}

	return &RedisQueue{
		client: redis.NewClient(options),
		key:    key,
	}, nil
}

func (q *RedisQueue) Shutdown() error {
	return q.client.Close()
}

func (q *RedisQueue) EnqueueRequest(r *p.ProxyRequest) error {
	marshalledRequest, err := json.Marshal(*r)
	if err != nil {
		return err
	}

	if err := q.client.RPush(q.key, marshalledRequest).Err(); err != nil {
		return err
	}

	return nil
}

func (q *RedisQueue) DequeueRequest() (*p.ProxyRequest, error) {
	result, err := q.client.BLPop(0, q.key).Result()
	if err != nil {
		return nil, err
	}

	// result[0] == key
	var proxyRequest p.ProxyRequest
	if err = json.Unmarshal([]byte(result[1]), &proxyRequest); err != nil {
		return nil, err
	}

	return &proxyRequest, nil
}
