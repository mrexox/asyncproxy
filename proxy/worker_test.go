package proxy

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/time/rate"
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
		limiter:    rate.NewLimiter(rate.Limit(15), 15),
	}

	sendRequest := func(_ context.Context, r *ProxyRequest) error {
		sendCnt += 1
		return nil
	}

	ctx := context.Background()
	stopped := make(chan struct{}, 1)
	worker.Work(ctx, stopped, sendRequest)

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

	sendRequest = func(_ context.Context, r *ProxyRequest) error {
		return errors.New("any kind of error")
	}

	worker.Work(ctx, stopped, sendRequest)

	if q.dequeued != 1 {
		t.Errorf("should have enqueued the request")
	}
	if q.enqueued != 1 {
		t.Errorf("should have enqueued the request again")
	}

	worker.maxRetries = 1
	q.dequeued = 0
	q.enqueued = 0

	worker.Work(ctx, stopped, sendRequest)
	sendCnt = 0

	if sendCnt != 0 {
		t.Errorf("shouldn't have sent the request with max attempts reached")
	}
}
