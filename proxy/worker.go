package proxy

import (
	"context"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
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
	log.WithFields(log.Fields{
		"enabled":           config.Queue.Enabled,
		"workers":           config.Queue.Workers,
		"handle_per_second": config.Queue.HandlePerSecond,
		"max_retries":       config.Queue.MaxRetries,
	}).Info("Initializing worker")

	if !config.Queue.Enabled {
		log.Warn("(!) Queueing disabled")
		return nil
	}

	queue, err := NewPgQueue(
		config.Db.ConnectionString,
		config.Db.MaxConnections,
	)
	if err != nil {
		log.Fatal(err)
	}

	if config.Queue.Workers < 1 {
		log.Fatal("workers must be >= 1")
	}

	if config.Queue.HandlePerSecond < 1 {
		log.Fatal("max rps must be >= 1")
	}

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
	log.Info("Stopping workers...")

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
		log.WithError(err).Warn("dequeue error")
		return
	}

	// Try handling the request once again
	if err := w.doRequest(ctx, request); err != nil {
		if attempt > w.maxRetries {
			log.WithFields(log.Fields{
				"method":  request.Method,
				"url":     request.OriginURL,
				"retries": attempt,
			}).Warn("max attempts exceded")
			return
		}

		if err = w.queue.EnqueueRequest(request, attempt+1); err != nil {
			log.WithFields(log.Fields{
				"request": request.String(),
				"error":   err,
			}).Warn("couldn't retry request")
		}
	}
}
