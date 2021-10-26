package proxy

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/ratelimit"
)

type testQueue struct {
	dequeued int
	enqueued int
}

func (t *testQueue) Total() uint64 {
	return 1
}

func (t *testQueue) Shutdown() error {
	return nil
}

func (t *testQueue) EnqueueRequest(r *ProxyRequest, attempt int) error {
	t.enqueued += 1

	return nil
}

func (t *testQueue) DequeueRequest(ctx context.Context) (r *ProxyRequest, attempt int, err error) {
	t.dequeued += 1

	r = &ProxyRequest{}
	attempt = 2

	return
}

func TestWork(t *testing.T) {
	var sendCnt int
	q := testQueue{}

	worker := &Worker{
		numWorkers: 1,
		maxRetries: 2,
		queue:      &q,
		doRequest: func(_ context.Context, r *ProxyRequest) error {
			sendCnt += 1
			return nil
		},
		limiter: ratelimit.New(1),
	}

	ctx := context.Background()
	worker.Work(ctx)

	if q.dequeued != 1 {
		t.Errorf("should have enqueued the request")
	}
	if q.enqueued != 0 {
		t.Errorf("should not enqueue anything")
	}
	if sendCnt != 1 {
		t.Errorf("should have sent the request")
	}

	q.dequeued = 0
	q.enqueued = 0

	worker.doRequest = func(_ context.Context, r *ProxyRequest) error {
		return errors.New("any kind of error")
	}

	worker.Work(ctx)

	if q.dequeued != 1 {
		t.Errorf("should have enqueued the request")
	}
	if q.enqueued != 1 {
		t.Errorf("should have enqueued the request again")
	}

	worker.maxRetries = 1
	q.dequeued = 0
	q.enqueued = 0

	worker.Work(ctx)
	sendCnt = 0

	if sendCnt != 0 {
		t.Errorf("shouldn't have sent the request with max attempts reached")
	}
}
