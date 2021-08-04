package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/viper"

	p "github.com/evilmartians/asyncproxy/proxy"
)

type asyncProxyHandler struct{}

var (
	proxy           *p.Proxy
	status          int // e.g. 200
	shutdownTimeout time.Duration
	unwantedEvents  []string

	worker *Worker
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
	unwantedEvents = viper.GetStringSlice("ignore_events")
	shutdownTimeout = time.Duration(viper.GetInt("server.shutdown_timeout")) * time.Second

	proxy = p.InitProxy(viper.GetViper())
	worker = InitWorker(viper.GetViper())

	InitMetrics(viper.GetViper())
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

	ctx, stopWorker := context.WithCancel(context.Background())
	if worker != nil {
		worker.Run(ctx)
	}

	// Run metrics server
	go RunMetricsServer()

	// Run proxying server
	go func() {
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("metrics server error: %v", err)
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

	if err := ShutdownMetricsServer(gracefulCtx); err != nil {
		log.Fatal(err)
	} else {
		log.Printf("Gracefully stopped metrics")
	}

	stopWorker()
	if worker != nil {
		if err := worker.Shutdown(gracefulCtx); err != nil {
			log.Fatal(err)
		} else {
			log.Printf("Gracefully stopped worker")
		}
	}

	if err := proxy.Shutdown(gracefulCtx); err != nil {
		log.Fatal(err)
	} else {
		log.Printf("Gracefully stopped proxy")
	}
}

func (h asyncProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	trackRequest(r)

	start := time.Now()

	log.Printf("<- %s %s (%s)", r.Method, r.RequestURI, r.RemoteAddr)

	if r.URL.Path == prometheusPath {
		handleMetrics(w, r)
		return
	}

	pRequest, err := p.NewProxyRequest(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Println(err)
		return
	}

	go proxyRequest(*pRequest)

	w.WriteHeader(status)
	trackRequestDuration(start, r)
}

func proxyRequest(r p.ProxyRequest) {
	for _, unwantedEvent := range unwantedEvents {
		if r.MatchEvent(unwantedEvent) {
			log.Printf("ignoring unwanted event %s", unwantedEvent)
			return
		}
	}

	if worker != nil {
		err := worker.Enqueue(&r)
		if err == nil {
			return
		} else {
			log.Printf("enqueueing error: %v", err)
		}
	}

	if err := sendRequestToRemote(&r); err != nil {
		log.Printf("error: %s", err)
	}
}

func sendRequestToRemote(r *p.ProxyRequest) error {
	var (
		res string
		err error
	)

	start := time.Now()

	if err = proxy.Do(r); err == nil {
		res = "OK"
	} else {
		res = err.Error()
	}

	trackProxyRequestDuration(start, r, res)

	return err
}
