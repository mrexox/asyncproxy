package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"

	cfg "github.com/evilmartians/asyncproxy/config"
	server "github.com/evilmartians/asyncproxy/server"
)

type asyncProxyHandler struct{}

var config *cfg.Config
var proxyServer *server.Server

func init() {
	// Disable colors with LOG_COLOR=false
	log.SetFormatter(&log.TextFormatter{
		DisableColors: os.Getenv("LOG_COLOR") == "false",
	})

	log.Info("Initializing...")

	path, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	config, err = cfg.LoadConfig(path)
	if err != nil {
		log.Fatal(err)
	}

	proxyServer = server.NewServer(config)

	log.Info("Initialization done!")
}

func main() {
	log.Info("Server starting...")

	ctx, forceShutdown := context.WithCancel(context.Background())

	srv := &http.Server{
		Addr:         config.Server.Bind,
		Handler:      asyncProxyHandler{},
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		BaseContext:  func(_ net.Listener) context.Context { return ctx },
	}

	srv.SetKeepAlivesEnabled(false)

	// Run metrics server
	metricsServer := NewMetrics(config)
	go func() {
		if err := metricsServer.ListenAndServe(); err != http.ErrServerClosed {
			log.WithError(err).Warn("metrics server error")
		}
	}()

	proxyServer.Start(ctx)

	// Run http server
	go func() {
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.WithError(err).Warn("server error")
		}
	}()

	log.Info("Server started!")

	// Handle shutdowns gracefully
	signalChan := make(chan os.Signal, 1)
	signal.Notify(
		signalChan,
		syscall.SIGINT,
		syscall.SIGTERM,
	)

	<-signalChan
	log.Info("Shutting down gracefully...")
	go func() {
		<-signalChan
		forceShutdown()
	}()

	gracefulCtx, cancel := context.WithTimeout(ctx, config.Server.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(gracefulCtx); err != nil {
		log.Fatal(err)
	} else {
		log.Info("Gracefully stopped server!")
	}

	if err := proxyServer.Stop(gracefulCtx); err != nil {
		log.Fatal(err)
	} else {
		log.Info("Gracefully stopped proxy!")
	}

	if err := metricsServer.Shutdown(gracefulCtx); err != nil {
		log.Fatal(err)
	} else {
		log.Info("Gracefully stopped metrics!")
	}
}

func (h asyncProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	trackRequest(r)

	start := time.Now()

	log.WithFields(log.Fields{
		"method": r.Method,
		"uri":    r.RequestURI,
		"ip":     r.RemoteAddr,
	}).Info("received")

	err := proxyServer.HandleRequest(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.WithError(err).Warn("proxying error")
		return
	}

	w.WriteHeader(config.Server.ResponseStatus)
	trackRequestDuration(start, r)
}
