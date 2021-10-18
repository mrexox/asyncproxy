package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	cfg "github.com/evilmartians/asyncproxy/config"
	proxy "github.com/evilmartians/asyncproxy/proxy"
)

var (
	metricsServer     *http.Server
	prometheusHandler http.Handler
	prometheusPath    string
	queue             proxy.Queue

	requestsCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Number of requests.",
	}, []string{"path"})

	requestsDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_response_time_seconds",
		Help:    "Response time.",
		Buckets: []float64{.001, .005, 0.01, .025, .05, .1, .5, 1, 2.5, 5, 10, 30},
	}, []string{"path"})
)

type metricsHandler struct{}

func InitMetrics(config *cfg.Config) {
	prometheusHandler = promhttp.Handler()
	prometheusPath = config.Metrics.Path

	metricsServer = &http.Server{
		Addr:         config.Metrics.Bind,
		Handler:      metricsHandler{},
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	var err error
	queue, err = proxy.NewPgQueue(
		config.Db.ConnectionString,
		config.Db.MaxConnections,
	)
	if err != nil {
		log.Fatal(err)
	}

	promauto.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "queue_total_size",
		Help: "Number of all requests in the queue.",
	}, func() float64 {
		return float64(queue.Total())
	})
}

func RunMetricsServer() {
	if err := metricsServer.ListenAndServe(); err != http.ErrServerClosed {
		log.Printf("server error: %v", err)
	}
}

func ShutdownMetricsServer(ctx context.Context) error {
	err1 := metricsServer.Shutdown(ctx)
	err2 := queue.Shutdown()

	if err1 != nil {
		return err1
	}

	return err2
}

func handleMetrics(w http.ResponseWriter, r *http.Request) {
	prometheusHandler.ServeHTTP(w, r)
}

func trackRequest(r *http.Request) {
	requestsCounter.WithLabelValues(r.URL.Path).Inc()
}

func trackRequestDuration(start time.Time, r *http.Request) {
	requestsDuration.
		WithLabelValues(r.URL.Path).
		Observe(time.Since(start).Seconds())
}

func (m metricsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("<- %s %s (%s)", r.Method, r.RequestURI, r.RemoteAddr)

	if r.URL.Path == prometheusPath {
		handleMetrics(w, r)
		return
	}
}
