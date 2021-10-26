package proxy

import (
	"context"
	"log"
	"sync"
	"time"

	"go.uber.org/ratelimit"

	cfg "github.com/evilmartians/asyncproxy/config"
)

type sendProxyRequestFunc func(context.Context, *ProxyRequest) error

type Worker struct {
	numWorkers int
	maxRetries int
	queue      Queue
	doRequest  sendProxyRequestFunc
	limiter    ratelimit.Limiter

	works sync.WaitGroup
}

func NewWorker(config *cfg.Config, sendFunc sendProxyRequestFunc) *Worker {
	if !config.Queue.Enabled {
		return nil
	}

	log.Printf("Queueing enabled")

	queue, err := NewPgQueue(
		config.Db.ConnectionString,
		config.Db.MaxConnections,
	)
	if err != nil {
		log.Fatal(err)
	}

	if config.Queue.Workers < 1 {
		log.Fatal("workers count cannot be less than 1")
	}

	if config.Queue.HandlePerSecond < 1 {
		log.Fatal("max rps must be >= 1")
	}

	log.Printf("Worker -- rate limit: %d rps", config.Queue.HandlePerSecond)
	log.Printf("Worker -- parallelism: %d", config.Queue.Workers)

	return &Worker{
		numWorkers: config.Queue.Workers,
		maxRetries: config.Queue.MaxRetries,
		queue:      queue,
		doRequest:  sendFunc,
		limiter:    ratelimit.New(config.Queue.HandlePerSecond),
	}
}

// Gracefully stops all goroutines
func (w *Worker) Shutdown(ctx context.Context) error {
	log.Printf("Stopping workers...")

	waitChan := make(chan struct{})
	go func() {
		w.works.Wait()
		waitChan <- struct{}{}
	}()

	err := func() error {
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-waitChan:
				return nil
			}
		}
	}()
	if err != nil {
		return err
	}

	return w.queue.Shutdown()
}

func (w *Worker) Run(gracefulCtx, forceCtx context.Context) {
	for i := 0; i < w.numWorkers; i++ {
		go w.concurrentRun(gracefulCtx, forceCtx)
	}
}

func (w *Worker) concurrentRun(gracefulCtx, forceCtx context.Context) {
	for {
		select {
		case <-gracefulCtx.Done():
			return
		default:
			w.Work(forceCtx)
		}
	}
}

func (w *Worker) Enqueue(r *ProxyRequest) error {
	return w.queue.EnqueueRequest(r, 1)
}

// Dequeues request and sends it to the destination
// Uses a limiter to balance the outgoing load
func (w *Worker) Work(ctx context.Context) {
	w.works.Add(1)
	defer w.works.Done()

	_ = w.limiter.Take() // limit outgoing load

	request, attempt, err := w.queue.DequeueRequest(ctx)
	if err == EmptyQueueError {
		time.Sleep(5 * time.Second) // small delay before the next try
		return
	}
	if err != nil {
		log.Printf("dequeue error: %s", err)
		return
	}

	// Try handling the request once again
	if err := w.doRequest(ctx, request); err != nil {
		if attempt > w.maxRetries {
			log.Printf("max attempts exceded: %s", request.String())
			return
		}

		if err = w.queue.EnqueueRequest(request, attempt+1); err != nil {
			log.Printf("couldn't retry request: %s", err)
		}
	}
}
