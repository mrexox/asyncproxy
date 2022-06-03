package server

import (
	"context"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"

	"github.com/evilmartians/asyncproxy/config"
	"github.com/evilmartians/asyncproxy/internal/worker"
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
	queue  worker.Queue
}

type httpHandler struct{}

func NewMetrics(cfg *config.Config) *Metrics {
	prometheusHandler = promhttp.Handler()
	prometheusPath = cfg.Metrics.Path

	queue, err := worker.NewPgQueue(cfg)
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
			Addr:         cfg.Metrics.Bind,
			Handler:      httpHandler{},
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
		},
		queue: queue,
	}
}

func (m *Metrics) Start() {
	go func() {
		if err := m.server.ListenAndServe(); err != http.ErrServerClosed {
			log.WithError(err).Warn("metrics error")
		}
	}()
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

func (m *Metrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trackRequest(r)

		start := time.Now()
		next.ServeHTTP(w, r)
		trackRequestDuration(start, r)
	})
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
