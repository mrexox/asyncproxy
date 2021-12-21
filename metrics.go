package main

import (
	"context"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"

	cfg "github.com/evilmartians/asyncproxy/config"
	proxy "github.com/evilmartians/asyncproxy/proxy"
)

var (
	prometheusHandler http.Handler
	prometheusPath    string

	requestsCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Number of requests.",
	}, []string{"path"})

	requestsDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_response_time_seconds",
		Help:    "Response time.",
		Buckets: []float64{.5, 1, 2.5, 5},
	}, []string{"path"})
)

type Metrics struct {
	server *http.Server
	queue  proxy.Queue
}

type httpHandler struct{}

func NewMetrics(config *cfg.Config) *Metrics {
	prometheusHandler = promhttp.Handler()
	prometheusPath = config.Metrics.Path

	queue, err := proxy.NewPgQueue(config)
	if err != nil {
		log.Fatal(err)
	}

	promauto.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "queue_total_size",
		Help: "Number of all requests in the queue.",
	}, func() float64 {
		return float64(queue.Total())
	})

	return &Metrics{
		server: &http.Server{
			Addr:         config.Metrics.Bind,
			Handler:      httpHandler{},
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
		},
		queue: queue,
	}
}

func (m *Metrics) ListenAndServe() error {
	return m.server.ListenAndServe()
}

func (m *Metrics) Shutdown(ctx context.Context) error {
	err1 := m.server.Shutdown(ctx)
	err2 := m.queue.Shutdown()

	if err1 != nil {
		return err1
	}

	return err2
}

func (h httpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.WithFields(log.Fields{
		"ip":  r.RemoteAddr,
		"uri": r.RequestURI,
	}).Info("metrics check")

	if r.URL.Path != prometheusPath {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	handleMetrics(w, r)
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
