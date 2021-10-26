package proxy

import (
	"context"
)

type Queue interface {
	Total() uint64
	Shutdown() error
	EnqueueRequest(r *ProxyRequest, attempt int) error
	DequeueRequest(ctx context.Context) (r *ProxyRequest, attempt int, err error)
}
