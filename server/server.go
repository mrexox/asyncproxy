package server

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"
	"golang.org/x/time/rate"

	cfg "github.com/evilmartians/asyncproxy/config"
	proxy "github.com/evilmartians/asyncproxy/proxy"
)

var (
	// Metrics for outgoing requests
	proxyRequestsDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_proxy_response_time_seconds",
		Help:    "Proxy request response time.",
		Buckets: []float64{.5, 1, 2.5, 5},
	}, []string{"path", "status"})
)

type Server struct {
	// Main worker object to work with proxy requests
	worker *proxy.Worker

	// Main proxy object to handle the requests
	client *proxy.Proxy

	// Track goroutines for the graceful shutdown
	asyncRoutines sync.WaitGroup

	// stopWorker signals all workers to stop
	stopWorker context.CancelFunc

	// Rate limiter to deternime when to start using the database
	rateLimiter *rate.Limiter

	// If enqueueing is enabled
	// Can be turned off if database latency is too big
	enqueueEnabled bool
}

// Init everything related to asynchronous proxying
func NewServer(config *cfg.Config) *Server {
	log.WithFields(log.Fields{
		"bind":             config.Server.Bind,
		"shutdown_timeout": config.Server.ShutdownTimeout,
		"enqueue_enabled":  config.Server.EnqueueEnabled,
		"enqueue_rate":     config.Server.EnqueueRate,
	}).Info("Initializing server")

	return &Server{
		client:         proxy.NewProxy(config),
		worker:         proxy.NewWorker(config),
		enqueueEnabled: config.Server.EnqueueEnabled,
		rateLimiter:    rate.NewLimiter(rate.Limit(config.Server.EnqueueRate), config.Server.EnqueueRate),
	}
}

// Start workers proxying the requests
func (s *Server) Start(ctx context.Context) {
	stopCtx, stop := context.WithCancel(ctx)
	s.stopWorker = stop

	s.worker.Run(ctx, stopCtx.Done(), s.SendProxyRequest)
}

// Stop proxying the requests gracefully
func (s *Server) Stop(ctx context.Context) error {
	log.Info("Stopping proxying...")

	routinesFinished := make(chan struct{})
	go func() {
		s.asyncRoutines.Wait()
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

	s.stopWorker()
	if err = s.worker.Shutdown(ctx); err != nil {
		return err
	}

	return s.client.Shutdown(ctx)
}

// Handle http request: convert it into the proxy request
// Store it into the queue or just send it
func (s *Server) HandleRequest(r *http.Request) error {
	proxyRequest, err := proxy.NewProxyRequest(r)
	if err != nil {
		return err
	}

	return s.workProxyRequest(r.Context(), proxyRequest)
}

// Put the proxy request into the queue or send it if queue is disabled
func (s *Server) workProxyRequest(ctx context.Context, r *proxy.ProxyRequest) error {
	s.asyncRoutines.Add(1)
	defer s.asyncRoutines.Done()

	if !s.enqueueEnabled || s.rateLimiter.Allow() {
		return s.SendProxyRequest(ctx, r)
	}

	var err error
	if err = s.worker.Enqueue(r); err == nil {
		return nil
	}

	log.WithError(err).Warn("enqueueing error, proxying withoud enqueueing")

	return s.SendProxyRequest(ctx, r)
}

// Process the ProxyRequest
func (s *Server) SendProxyRequest(ctx context.Context, r *proxy.ProxyRequest) error {
	var err error
	res := "OK"

	start := time.Now()

	if err = s.client.Do(ctx, r); err != nil {
		log.WithError(err).Error("proxy error")
		res = err.Error()
	}

	trackProxyRequestDuration(start, r, res)

	return err
}

func trackProxyRequestDuration(start time.Time, r *proxy.ProxyRequest, res string) {
	proxyRequestsDuration.
		WithLabelValues(r.OriginURL, res).
		Observe(time.Since(start).Seconds())
}
