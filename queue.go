package main

import (
	"encoding/json"
	"log"
	"net/url"
	"strconv"

	"github.com/go-redis/redis"
)

type Queue struct {
	client *redis.Client
	key    string
}

func NewQueue(key, urlStr string) (*Queue, error) {
	redisUrl, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	db, err := strconv.Atoi(redisUrl.Path[1:])
	if err != nil {
		return nil, err
	}

	return &Queue{
		client: redis.NewClient(&redis.Options{
			Addr:     redisUrl.Host,
			Password: "",
			DB:       db,
		}),
		key: key,
	}, nil
}

func (q *Queue) EnqueueRequest(r *ProxyRequest) {
	marshalledRequest, err := json.Marshal(*r)
	if err != nil {
		log.Printf("request marshall error: %s", err)
		return
	}

	if err := q.client.RPush(q.key, marshalledRequest).Err(); err != nil {
		log.Printf("redis rpush error: %s", err)
	}
}

func (q *Queue) DequeueRequest() (*ProxyRequest, error) {
	result, err := q.client.BLPop(0, q.key).Result()
	if err != nil {
		return nil, err
	}

	// result[0] == key
	var proxyRequest ProxyRequest
	if err = json.Unmarshal([]byte(result[1]), &proxyRequest); err != nil {
		return nil, err
	}

	return &proxyRequest, nil
}
