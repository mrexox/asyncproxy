package proxy

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"
	"golang.org/x/time/rate"

	"github.com/evilmartians/asyncproxy/config"
	"github.com/evilmartians/asyncproxy/internal/worker"
)

var (
	// Metrics for outgoing requests
	proxyRequestsDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_proxy_response_time_seconds",
		Help:    "Proxy request response time.",
		Buckets: []float64{.5, 1, 2.5, 5},
	}, []string{"path", "status"})
)

type Proxy struct {
	// Main worker object to work with proxy requests
	worker *worker.Worker

	// Main sender object to perform the requests
	client *worker.Client

	// Track goroutines for the graceful shutdown
	asyncRoutines sync.WaitGroup

	// stopWorker signals all workers to stop
	stopWorker context.CancelFunc

	// Rate limiter to deternime when to start using the database
	rateLimiter *rate.Limiter

	// If enqueueing is enabled
	// Can be turned off if database latency is too big
	enqueueEnabled bool

	// Default response status for reply
	responseStatus int
}

// Init everything related to asynchronous proxying
func NewProxy(cfg *config.Config) *Proxy {
	log.WithFields(log.Fields{
		"enqueue_enabled": cfg.Server.EnqueueEnabled,
		"enqueue_rate":    cfg.Server.EnqueueRate,
	}).Info("Initializing proxy")

	return &Proxy{
		client:         worker.NewClient(cfg),
		worker:         worker.NewWorker(cfg),
		enqueueEnabled: cfg.Server.EnqueueEnabled,
		rateLimiter:    rate.NewLimiter(rate.Limit(cfg.Server.EnqueueRate), cfg.Server.EnqueueRate),
		responseStatus: cfg.Server.ResponseStatus,
	}
}

// Start workers proxying the requests
func (p *Proxy) Start(ctx context.Context) {
	stopCtx, stop := context.WithCancel(ctx)
	p.stopWorker = stop

	p.worker.Run(ctx, stopCtx.Done(), p.SendRequest)
}

// Stop proxying the requests gracefully
func (p *Proxy) Stop(ctx context.Context) error {
	log.Info("Stopping proxying...")

	routinesFinished := make(chan struct{})
	go func() {
		p.asyncRoutines.Wait()
		close(routinesFinished)
	}()

	err := func() error {
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-routinesFinished:
				return nil
			}
		}
	}()
	if err != nil {
		return err
	}

	p.stopWorker()
	if err = p.worker.Shutdown(ctx); err != nil {
		return err
	}

	return p.client.Shutdown(ctx)
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.WithFields(log.Fields{
		"method": r.Method,
		"uri":    r.RequestURI,
		"ip":     r.RemoteAddr,
	}).Info("received")

	err := p.HandleRequest(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.WithError(err).Warn("proxying error")
		return
	}

	w.WriteHeader(p.responseStatus)
}

// Handle http request: convert it into the proxy request
// Store it into the queue or just send it
func (p *Proxy) HandleRequest(r *http.Request) error {
	request, err := worker.NewRequest(r)
	if err != nil {
		return err
	}

	return p.proxyRequest(r.Context(), request)
}

// Put the proxy request into the queue or send it if queue is disabled
func (p *Proxy) proxyRequest(ctx context.Context, r *worker.Request) error {
	p.asyncRoutines.Add(1)
	defer p.asyncRoutines.Done()

	if !p.enqueueEnabled || p.rateLimiter.Allow() {
		return p.SendRequest(ctx, r)
	}

	var err error
	if err = p.worker.Enqueue(r); err == nil {
		return nil
	}

	log.WithError(err).Warn("enqueueing error, proxying withoud enqueueing")

	return p.SendRequest(ctx, r)
}

func (p *Proxy) SendRequest(ctx context.Context, r *worker.Request) error {
	var err error
	res := "OK"

	start := time.Now()

	if err = p.client.Do(ctx, r); err != nil {
		log.WithError(err).Error("proxy error")
		res = err.Error()
	}

	trackProxyRequestDuration(start, r, res)

	return err
}

func trackProxyRequestDuration(start time.Time, r *worker.Request, res string) {
	proxyRequestsDuration.
		WithLabelValues(r.OriginURL, res).
		Observe(time.Since(start).Seconds())
}
