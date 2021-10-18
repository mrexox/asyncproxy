package proxy

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	cfg "github.com/evilmartians/asyncproxy/config"
)

var (
	stopWorker context.CancelFunc

	// Main worker object to work with proxy requests
	worker *Worker

	// Main proxy object to handle the requests
	proxy *Proxy

	// Metrics for outgoing requests
	proxyRequestsDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_proxy_response_time_seconds",
		Help:    "Proxy request response time.",
		Buckets: []float64{.001, .005, .01, .025, .05, .1, .5, 1, 2.5, 5, 10, 30},
	}, []string{"path", "status"})
)

// Init everything related to proxying
func InitAsyncProxy(config *cfg.Config) {
	proxy = NewProxy(config)
	worker = NewWorker(config, SendProxyRequest)
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
	stopWorker()
	if err := shutdownWorker(ctx); err != nil {
		return err
	}

	return proxy.Shutdown(ctx)
}

// Handle http request: convert it into the proxy request
// Store it into the queue or just send it
func HandleRequest(r *http.Request) error {
	proxyRequest, err := NewProxyRequest(r)
	if err != nil {
		return err
	}

	go WorkProxyRequest(proxyRequest)

	return nil
}

// Put the proxy request into the queue or send it if queue is disabled
func WorkProxyRequest(r *ProxyRequest) {
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
func SendProxyRequest(r *ProxyRequest) error {
	var err error
	res := "OK"

	start := time.Now()

	if err = proxy.Do(r); err != nil {
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

func trackProxyRequestDuration(start time.Time, r *ProxyRequest, res string) {
	proxyRequestsDuration.
		WithLabelValues(r.OriginURL, res).
		Observe(time.Since(start).Seconds())
}
