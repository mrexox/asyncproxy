package main

import (
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"

	p "github.com/evilmartians/asyncproxy/proxy"
)

var (
	metricsServer     *http.Server
	prometheusHandler http.Handler
	prometheusPath    string

	requestsCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Number of requests.",
	}, []string{"path"})

	requestDurationBuckets = []float64{
		0.001, .005, 0.01, .025, 0.05, 0.1, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300,
	}

	requestsDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_response_time_seconds",
		Help:    "Response time.",
		Buckets: requestDurationBuckets,
	}, []string{"path"})

	proxyRequestsDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_proxy_response_time_seconds",
		Help:    "Proxy request response time.",
		Buckets: requestDurationBuckets,
	}, []string{"path", "status"})
)

type metricsHandler struct{}

func InitMetrics(v *viper.Viper) {
	prometheusHandler = promhttp.Handler()
	prometheusPath = v.GetString("metrics.path")

	metricsServer = &http.Server{
		Addr:         v.GetString("metrics.bind"),
		Handler:      metricsHandler{},
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	queue, err := NewPgQueue(
		v.GetString("db.connection_string"),
		v.GetInt("db.max_connections"),
	)
	if err != nil {
		log.Fatal(err)
	}

	promauto.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "queue_unprocessed_size",
		Help: "Number of unprocessed requests in the queue.",
	}, func() float64 {
		return float64(queue.GetUnprocessed())
	})

	promauto.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "queue_total_size",
		Help: "Number of all requests in the queue.",
	}, func() float64 {
		return float64(queue.GetTotal())
	})
}

func GetMetricsServer() *http.Server {
	return metricsServer
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

func trackProxyRequestDuration(start time.Time, r *p.ProxyRequest, res string) {
	proxyRequestsDuration.
		WithLabelValues(r.OriginURL, res).
		Observe(time.Since(start).Seconds())
}

func (m metricsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("<- %s %s (%s)", r.Method, r.RequestURI, r.RemoteAddr)

	if r.URL.Path == prometheusPath {
		handleMetrics(w, r)
		return
	}
}
