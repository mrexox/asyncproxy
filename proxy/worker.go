package proxy

import (
	"context"
	"sync"
	"time"

	"github.com/jpillora/backoff"
	log "github.com/sirupsen/logrus"
	"go.uber.org/ratelimit"

	cfg "github.com/evilmartians/asyncproxy/config"
)

type sendProxyRequestFunc func(context.Context, *ProxyRequest) error

type Worker struct {
	numWorkers int
	maxRetries int
	queue      Queue
	limiter    ratelimit.Limiter
	backoff    backoff.Backoff

	works sync.WaitGroup
}

func NewWorker(config *cfg.Config) *Worker {
	log.WithFields(log.Fields{
		"enabled":           config.Queue.Enabled,
		"workers":           config.Queue.Workers,
		"handle_per_second": config.Queue.HandlePerSecond,
		"max_retries":       config.Queue.MaxRetries,
	}).Info("Initializing worker")

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
		limiter:    ratelimit.New(config.Queue.HandlePerSecond),
		backoff: backoff.Backoff{
			Min:    10 * time.Millisecond,
			Max:    5 * time.Second,
			Factor: 2,
			Jitter: true,
		},
	}
}

// Gracefully stops all goroutines
func (w *Worker) Shutdown(ctx context.Context) error {
	log.Info("Stopping workers...")

	waitChan := make(chan struct{})
	go func() {
		w.works.Wait()
		close(waitChan)
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

func (w *Worker) Run(ctx context.Context, stopped <-chan struct{}, fn sendProxyRequestFunc) {
	for i := 0; i < w.numWorkers; i++ {
		go func() {
			for {
				select {
				case <-stopped:
					return
				default:
					w.Work(ctx, stopped, fn)
				}
			}
		}()
	}
}

func (w *Worker) Enqueue(r *ProxyRequest) error {
	return w.queue.EnqueueRequest(r, 1)
}

// Dequeues request and sends it to the destination
// Uses a limiter to balance the outgoing load
func (w *Worker) Work(ctx context.Context, stopped <-chan struct{}, fn sendProxyRequestFunc) {
	w.works.Add(1)
	defer w.works.Done()

	_ = w.limiter.Take() // limit outgoing load

	var (
		request *ProxyRequest
		attempt int
	)
	for {
		var err error
		request, attempt, err = w.queue.DequeueRequest(ctx)
		if err == nil {
			break
		}

		if err != EmptyQueueError {
			log.WithError(err).Error("dequeue error")
		}

		select {
		case <-time.After(w.backoff.Duration()):
		case <-stopped:
			return
		case <-ctx.Done():
			log.WithError(ctx.Err()).Warn("worker context is done")
			return
		}
	}

	w.backoff.Reset()

	// Try handling the request once again
	if err := fn(ctx, request); err != nil {
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
