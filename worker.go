package main

import (
	"context"
	"fmt"
	"log"

	"github.com/spf13/viper"
	"go.uber.org/ratelimit"

	p "github.com/evilmartians/asyncproxy/proxy"
)

type handleFunc func(*p.ProxyRequest)

type Worker struct {
	stop context.CancelFunc

	ctx        context.Context
	numWorkers int
	queue      *PgQueue
	handle     handleFunc
	limiter    ratelimit.Limiter
	requests   chan struct{}
}

type config struct {
	numWorkers int
	queue      *PgQueue
	handle     handleFunc
	perSecond  int
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

	ctx, ctxCancel := context.WithCancel(context.Background())

	log.Printf("Worker rate limit is: %d per second", cfg.perSecond)

	return &Worker{
		stop:       ctxCancel,
		numWorkers: cfg.numWorkers,
		ctx:        ctx,
		queue:      cfg.queue,
		handle:     cfg.handle,
		limiter:    ratelimit.New(cfg.perSecond),
		requests:   make(chan struct{}, cfg.perSecond),
	}, nil
}

func (w *Worker) Run() {
	for i := 0; i < w.numWorkers; i++ {
		go w.run()
	}
}

// Shutdown gracefully stops workers
func (w *Worker) Shutdown(ctx context.Context) error {
	log.Printf("Stopping workers...")

	w.stop()

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
	return w.queue.EnqueueRequest(r)
}

func (w *Worker) run() {
	for {
		select {
		case <-w.ctx.Done():
			return
		default:
			func() {
				_ = w.limiter.Take()

				w.requests <- struct{}{}
				defer func() { <-w.requests }()

				request, err := w.queue.DequeueRequest()
				if err != nil {
					log.Printf("queue error: %s", err)
					return
				}

				w.handle(request)
			}()
		}
	}
}
