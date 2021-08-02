package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/spf13/viper"
	"go.uber.org/ratelimit"

	p "github.com/evilmartians/asyncproxy/proxy"
)

type handleFunc func(*p.ProxyRequest) error

type Worker struct {
	numWorkers int
	maxRetries int
	queue      *PgQueue
	handle     handleFunc

	limiter  ratelimit.Limiter
	requests chan struct{}
}

type config struct {
	numWorkers int
	queue      *PgQueue
	handle     handleFunc
	perSecond  int
	maxRetries int
}

func InitWorker(v *viper.Viper) *Worker {
	var worker *Worker

	queueEnabled := v.GetBool("queue.enabled")
	if queueEnabled {
		log.Printf("Queueing enabled")

		queue, err := NewPgQueue(
			v.GetString("db.connection_string"),
			v.GetInt("db.max_connections"),
		)
		if err != nil {
			log.Fatal(err)
		}

		numWorkers := v.GetInt("queue.workers")
		if numWorkers < 1 {
			log.Fatal("workers count cannot be less than 1")
		}

		worker, err = NewWorker(&config{
			numWorkers: numWorkers,
			queue:      queue,
			handle:     sendRequestToRemote,
			perSecond:  v.GetInt("queue.handle_per_second"),
			maxRetries: v.GetInt("queue.max_retries"),
		})
		if err != nil {
			log.Fatal(err)
		}
	}

	return worker
}

func NewWorker(cfg *config) (*Worker, error) {
	if cfg.perSecond < 1 {
		return nil, fmt.Errorf("max rps must be >= 1")
	}

	log.Printf("Worker rate limit is: %d per second", cfg.perSecond)

	return &Worker{
		numWorkers: cfg.numWorkers,
		queue:      cfg.queue,
		handle:     cfg.handle,
		limiter:    ratelimit.New(cfg.perSecond),
		requests:   make(chan struct{}, cfg.perSecond),
		maxRetries: cfg.maxRetries,
	}, nil
}

func (w *Worker) Run(ctx context.Context) {
	for i := 0; i < w.numWorkers; i++ {
		go w.run(ctx)
	}

	// Clean queue asynchronously
	go w.cleanQueue(ctx)
}

// Shutdown gracefully stops workers
func (w *Worker) Shutdown(ctx context.Context) error {
	log.Printf("Stopping workers...")

	err := func() error {
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				if len(w.requests) == 0 {
					return nil
				}
			}
		}
	}()
	if err != nil {
		return err
	}

	return w.queue.Shutdown()
}

func (w *Worker) Enqueue(r *p.ProxyRequest) error {
	return w.queue.EnqueueRequest(r, 1)
}

func (w *Worker) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			func() {
				_ = w.limiter.Take()

				w.requests <- struct{}{}
				defer func() { <-w.requests }()

				request, attempt, err := w.queue.DequeueRequest()
				if err == EmptyQueueError {
					time.Sleep(10 * time.Second) // small delay before the next try
					return
				}
				if err != nil {
					log.Printf("queue error: %s", err)
					return
				}

				// Try handling the request once again
				if err := w.handle(request); err != nil {
					if attempt <= w.maxRetries {
						w.queue.EnqueueRequest(request, attempt+1)
					} else {
						log.Printf("max attempts exceded: %s", request.String())
					}
				}
			}()
		}
	}
}

func (w *Worker) cleanQueue(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			time.Sleep(10 * time.Second)
			w.queue.DeleteStale()
		}
	}
}
