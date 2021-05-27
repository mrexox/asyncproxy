package main

import (
	"context"
	"log"

	p "github.com/evilmartians/asyncproxy/proxy"
)

type handleFunc func(*p.ProxyRequest)

type Worker struct {
	Shutdown context.CancelFunc

	ctx        context.Context
	numWorkers int
	queue      Queue
	handle     handleFunc
}

func NewWorker(numWorkers int, queue Queue, handle handleFunc) *Worker {
	ctx, ctxCancel := context.WithCancel(context.Background())

	return &Worker{
		Shutdown:   ctxCancel,
		numWorkers: numWorkers,
		ctx:        ctx,
		queue:      queue,
		handle:     handle,
	}
}

func (w *Worker) Run() {
	for i := 0; i < w.numWorkers; i++ {
		go w.run()
	}
}

func (w *Worker) run() {
	for {
		select {
		case <-w.ctx.Done():
			return
		default:
			request, err := w.queue.DequeueRequest()
			if err != nil {
				log.Printf("queue error: %s", err)
				continue
			}

			w.handle(request)
		}
	}
}
