package main

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"
)

type handler struct{}

var (
	proxy             *Proxy
	status            int // e.g. 200
	method            string
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

func initialize() {
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
	method = viper.GetString("proxy.request.method")

	prometheusHandler = promhttp.Handler()
	prometheusPath = viper.GetString("metrics.path")

	remoteUrl, err := url.Parse(viper.GetString("proxy.remote_url"))
	if err != nil {
		log.Fatal(err)
	}

	proxy, err = NewProxy(
		&ProxyConfig{
			Method:         method,
			RemoteHost:     remoteUrl.Host,
			RemoteScheme:   remoteUrl.Scheme,
			NumClients:     viper.GetInt("proxy.num_clients"),
			RequestTimeout: time.Duration(viper.GetInt("proxy.request.timeout")),
		},
	)
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	initialize()

	log.Println("Starting server...")

	srv := &http.Server{
		Addr:         viper.GetString("server.bind"),
		Handler:      handler{},
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	srv.SetKeepAlivesEnabled(false)

	log.Fatal(srv.ListenAndServe())
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("<- %s %s (%s)", r.Method, r.RequestURI, r.RemoteAddr)

	if r.URL.Path == prometheusPath {
		prometheusHandler.ServeHTTP(w, r)
		return
	}

	timer := prometheus.NewTimer(requestsDuration.WithLabelValues(r.URL.Path))

	rCopy, body, err := copyRequest(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Println(err)
		return
	}

	go proxyRequest(*rCopy, body)

	w.WriteHeader(status)
	timer.ObserveDuration()
}

func copyRequest(r *http.Request) (*http.Request, []byte, error) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, nil, err
	}

	rCopy := r.Clone(context.Background())

	return rCopy, body, nil
}

func proxyRequest(r http.Request, body []byte) {
	var res string
	var bodyReader io.Reader

	begin := time.Now()

	bodyReader = bytes.NewReader(body)
	if err := proxy.HandleRequest(&r, &bodyReader); err == nil {
		res = "OK"
	} else {
		res = err.Error()
		log.Println(res)
	}

	proxyRequestsDuration.
		WithLabelValues(r.URL.Path, res).
		Observe(time.Since(begin).Seconds())
}
