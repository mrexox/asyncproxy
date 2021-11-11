package server

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"
	"go.uber.org/ratelimit"

	cfg "github.com/evilmartians/asyncproxy/config"
	proxy "github.com/evilmartians/asyncproxy/proxy"
)

var (
	srv *Server

	// Metrics for outgoing requests
	proxyRequestsDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_proxy_response_time_seconds",
		Help:    "Proxy request response time.",
		Buckets: []float64{.001, .005, .01, .025, .05, .1, .5, 1, 2.5, 5, 10, 30},
	}, []string{"path", "status"})
)

type Server struct {
	// Concurrency limiter. Limits the number of http-created goroutines
	asyncLimiter  ratelimit.Limiter
	asyncRoutines sync.WaitGroup

	// stopWorker signals all workers to stop
	stopWorker context.CancelFunc

	// Main worker object to work with proxy requests
	worker *proxy.Worker

	// Main proxy object to handle the requests
	client *proxy.Proxy
}

func Init(config *cfg.Config) {
	srv = NewServer(config)
}

// Init everything related to asynchronous proxying
func NewServer(config *cfg.Config) *Server {
	log.WithFields(log.Fields{
		"bind":             config.Server.Bind,
		"concurrency":      config.Server.Concurrency,
		"shutdown_timeout": config.Server.ShutdownTimeout,
	}).Info("Initializing server")

	return &Server{
		asyncLimiter: ratelimit.New(config.Server.Concurrency),
		client:       proxy.NewProxy(config),
		worker:       proxy.NewWorker(config, SendProxyRequest),
	}
}

func Start(forceCtx context.Context) {
	srv.Start(forceCtx)
}

// Start workers proxying the requests
func (s *Server) Start(forceCtx context.Context) {
	gracefulCtx, cancel := context.WithCancel(context.Background())
	if s.worker != nil {
		s.worker.Run(gracefulCtx, forceCtx)
	}

	s.stopWorker = cancel
}

func Stop(ctx context.Context) error {
	return srv.Stop(ctx)
}

// Stop proxying the requests gracefully
func (s *Server) Stop(ctx context.Context) error {
	log.Info("Stopping proxying...")

	routinesFinished := make(chan struct{})
	go func() {
		s.asyncRoutines.Wait()
		routinesFinished <- struct{}{}
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
	if err = s.shutdownWorker(ctx); err != nil {
		return err
	}

	return s.client.Shutdown(ctx)
}

func HandleRequest(r *http.Request) error {
	return srv.HandleRequest(r)
}

// Handle http request: convert it into the proxy request
// Store it into the queue or just send it
func (s *Server) HandleRequest(r *http.Request) error {
	proxyRequest, err := proxy.NewProxyRequest(r)
	if err != nil {
		return err
	}

	// Limit the amount of goroutines that can be created per second
	_ = s.asyncLimiter.Take()

	go s.workProxyRequest(proxyRequest)

	return nil
}

// Put the proxy request into the queue or send it if queue is disabled
func (s *Server) workProxyRequest(r *proxy.ProxyRequest) {
	s.asyncRoutines.Add(1)
	defer s.asyncRoutines.Done()

	var err error
	if s.worker != nil {
		if err = s.worker.Enqueue(r); err == nil {
			return
		}

		log.WithError(err).Warn("enqueueing error")
	}

	if err = SendProxyRequest(context.Background(), r); err != nil {
		log.WithError(err).Warn("request error")
	}
}

func SendProxyRequest(ctx context.Context, r *proxy.ProxyRequest) error {
	return srv.SendProxyRequest(ctx, r)
}

// Process the ProxyRequest
func (s *Server) SendProxyRequest(ctx context.Context, r *proxy.ProxyRequest) error {
	var err error
	res := "OK"

	start := time.Now()

	if err = s.client.Do(ctx, r); err != nil {
		res = err.Error()
	}

	trackProxyRequestDuration(start, r, res)

	return err
}

func (s *Server) shutdownWorker(ctx context.Context) error {
	if s.worker == nil {
		return nil
	}

	return s.worker.Shutdown(ctx)
}

func trackProxyRequestDuration(start time.Time, r *proxy.ProxyRequest, res string) {
	proxyRequestsDuration.
		WithLabelValues(r.OriginURL, res).
		Observe(time.Since(start).Seconds())
}
