package main

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"

	p "github.com/evilmartians/asyncproxy/proxy"
)

type asyncProxyHandler struct{}

var (
	proxy           *p.Proxy
	status          int // e.g. 200
	shutdownTimeout time.Duration

	queue        Queue
	queueEnabled bool
	queueWorkers int

	// Metrics
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

func init() {
	log.Println("Reading config...")

	path, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	viper.AddConfigPath(path)
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		log.Fatal(err)
	}

	status = viper.GetInt("server.response_status")
	shutdownTimeout = time.Duration(viper.GetInt("server.shutdown_timeout")) * time.Second

	prometheusHandler = promhttp.Handler()
	prometheusPath = viper.GetString("metrics.path")

	remoteURL, err := url.Parse(viper.GetString("proxy.remote_url"))
	if err != nil {
		log.Fatal(err)
	}

	proxy, err = p.NewProxy(
		&p.ProxyConfig{
			RemoteHost:     remoteURL.Host,
			RemoteScheme:   remoteURL.Scheme,
			NumClients:     viper.GetInt("proxy.num_clients"),
			RequestTimeout: time.Duration(viper.GetInt("proxy.request_timeout")),
		},
	)
	if err != nil {
		log.Fatal(err)
	}

	queueEnabled = viper.GetBool("queue.enabled")
	if queueEnabled {
		queueType := viper.GetString("queue.type")
		log.Printf("Queueing enabled: %s", queueType)

		queue, err = NewQueue(&QueueOptions{
			RedisKey:      viper.GetString("redis.key"),
			RedisURL:      viper.GetString("redis.url"),
			RedisPoolSize: viper.GetInt("redis.pool_size"),
			DbName:        viper.GetString("db.name"),
			QueueType:     queueType,
		})
		if err != nil {
			log.Fatal(err)
		}
		queueWorkers = viper.GetInt("queue.workers")
		if queueWorkers < 1 {
			log.Fatal("workers count cannot be less than 1")
		}
	}
}

func main() {
	log.Println("Starting server...")

	srv := &http.Server{
		Addr:         viper.GetString("server.bind"),
		Handler:      asyncProxyHandler{},
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	srv.SetKeepAlivesEnabled(false)

	var worker *Worker
	if queueEnabled {
		worker = NewWorker(queueWorkers, queue, sendRequestToRemote)
		worker.Run()
	}

	// Run server
	go func() {
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("server error: %v", err)
		}
	}()

	// Handle shutdowns gracefully
	signalChan := make(chan os.Signal, 1)
	signal.Notify(
		signalChan,
		syscall.SIGINT,
		syscall.SIGTERM,
	)

	<-signalChan
	log.Printf("Shutting down gracefully...")

	gracefulCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(gracefulCtx); err != nil {
		log.Fatal(err)
	} else {
		log.Printf("Gracefully stopped server")
	}

	if worker != nil {
		worker.Shutdown()
	}

	if err := proxy.Shutdown(gracefulCtx); err != nil {
		log.Fatal(err)
	} else {
		log.Printf("Gracefully stopped proxy")
	}

	if queue == nil {
		return
	}

	if err := queue.Shutdown(); err != nil {
		log.Fatal(err)
	} else {
		log.Printf("Gracefully closed queue")
	}
}

func (h asyncProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("<- %s %s (%s)", r.Method, r.RequestURI, r.RemoteAddr)

	if r.URL.Path == prometheusPath {
		prometheusHandler.ServeHTTP(w, r)
		return
	}

	timer := prometheus.NewTimer(requestsDuration.WithLabelValues(r.URL.Path))

	pRequest, err := p.NewProxyRequest(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Println(err)
		return
	}

	go proxyRequest(*pRequest)

	w.WriteHeader(status)
	timer.ObserveDuration()
}

func proxyRequest(r p.ProxyRequest) {
	if queueEnabled {
		err := queue.EnqueueRequest(&r)
		if err == nil {
			return
		} else {
			log.Printf("enqueueing error: %v", err)
		}
	}

	sendRequestToRemote(&r)
}

func sendRequestToRemote(r *p.ProxyRequest) {
	var res string

	begin := time.Now()

	if err := proxy.Do(r); err == nil {
		res = "OK"
	} else {
		res = err.Error()
		log.Println(res)
	}

	proxyRequestsDuration.
		WithLabelValues(r.OriginURL, res).
		Observe(time.Since(begin).Seconds())
}
