package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	cfg "github.com/evilmartians/asyncproxy/config"
	proxyServer "github.com/evilmartians/asyncproxy/server"
)

type asyncProxyHandler struct{}

var config *cfg.Config

func init() {
	log.Println("Reading config...")

	path, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	config, err = cfg.LoadConfig(path)
	if err != nil {
		log.Fatal(err)
	}

	InitMetrics(config)
	proxyServer.Init(config)
}

func main() {
	log.Println("Starting server...")

	srv := &http.Server{
		Addr:         config.Server.Bind,
		Handler:      asyncProxyHandler{},
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	srv.SetKeepAlivesEnabled(false)

	proxyServer.Start()

	// Run metrics server
	go RunMetricsServer()

	// Run http server
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

	gracefulCtx, cancel := context.WithTimeout(
		context.Background(), config.Server.ShutdownTimeout,
	)
	defer cancel()

	if err := srv.Shutdown(gracefulCtx); err != nil {
		log.Fatal(err)
	} else {
		log.Printf("Gracefully stopped server")
	}

	if err := proxyServer.Stop(gracefulCtx); err != nil {
		log.Fatal(err)
	} else {
		log.Printf("Gracefully stopped proxy")
	}

	if err := ShutdownMetricsServer(gracefulCtx); err != nil {
		log.Fatal(err)
	} else {
		log.Printf("Gracefully stopped metrics")
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

	err := proxyServer.HandleRequest(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Println(err)
		return
	}

	w.WriteHeader(config.Server.ResponseStatus)
	trackRequestDuration(start, r)
}
