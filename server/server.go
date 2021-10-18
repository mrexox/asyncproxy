package server

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	cfg "github.com/evilmartians/asyncproxy/config"
	proxy "github.com/evilmartians/asyncproxy/proxy"
)

var (
	// Concurrency limiter. Limits the number of http-created goroutines
	asyncLimiter chan struct{}

	// Graceful workers shutdown helper
	stopWorker context.CancelFunc

	// Main worker object to work with proxy requests
	worker *proxy.Worker

	// Main proxy object to handle the requests
	client *proxy.Proxy

	// Metrics for outgoing requests
	proxyRequestsDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_proxy_response_time_seconds",
		Help:    "Proxy request response time.",
		Buckets: []float64{.001, .005, .01, .025, .05, .1, .5, 1, 2.5, 5, 10, 30},
	}, []string{"path", "status"})
)

// Init everything related to asynchronous proxying
func Init(config *cfg.Config) {
	log.Printf("Server -- concurrency: %d", config.Server.Concurrency)

	asyncLimiter = make(chan struct{}, config.Server.Concurrency)
	client = proxy.NewProxy(config)
	worker = proxy.NewWorker(config, SendProxyRequest)
}

// Start workers proxying the requests
func Start() {
	ctx, cancel := context.WithCancel(context.Background())
	if worker != nil {
		worker.Run(ctx)
	}

	stopWorker = cancel
}

// Stop proxying the requests gracefully
func Stop(ctx context.Context) error {
	log.Printf("Stopping proxying...")

	err := func() error {
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				if len(asyncLimiter) == 0 {
					return nil
				}
			}
		}
	}()
	if err != nil {
		return err
	}

	stopWorker()
	if err = shutdownWorker(ctx); err != nil {
		return err
	}

	return client.Shutdown(ctx)
}

// Handle http request: convert it into the proxy request
// Store it into the queue or just send it
func HandleRequest(r *http.Request) error {
	proxyRequest, err := proxy.NewProxyRequest(r)
	if err != nil {
		return err
	}

	// Limit the amount of goroutines that can be created
	asyncLimiter <- struct{}{}

	go WorkProxyRequest(proxyRequest)

	return nil
}

// Put the proxy request into the queue or send it if queue is disabled
func WorkProxyRequest(r *proxy.ProxyRequest) {
	defer func() { <-asyncLimiter }()

	var err error
	if worker != nil {
		if err = worker.Enqueue(r); err == nil {
			return
		}

		log.Printf("enqueueing error: %v", err)
	}

	if err = SendProxyRequest(r); err != nil {
		log.Printf("error: %s", err)
	}
}

// Process the ProxyRequest
func SendProxyRequest(r *proxy.ProxyRequest) error {
	var err error
	res := "OK"

	start := time.Now()

	if err = client.Do(r); err != nil {
		res = err.Error()
	}

	trackProxyRequestDuration(start, r, res)

	return err
}

func shutdownWorker(ctx context.Context) error {
	if worker == nil {
		return nil
	}

	return worker.Shutdown(ctx)
}

func trackProxyRequestDuration(start time.Time, r *proxy.ProxyRequest, res string) {
	proxyRequestsDuration.
		WithLabelValues(r.OriginURL, res).
		Observe(time.Since(start).Seconds())
}
