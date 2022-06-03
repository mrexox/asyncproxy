package worker

import (
	"context"
)

type Queue interface {
	Total() uint64
	Shutdown() error
	EnqueueRequest(r *Request, attempt int) error
	DequeueRequest(ctx context.Context) (r *Request, attempt int, err error)
}
