package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	log "github.com/sirupsen/logrus"

	"github.com/evilmartians/asyncproxy/config"
	"github.com/evilmartians/asyncproxy/internal/proxy"
	"github.com/evilmartians/asyncproxy/server"
)

func main() {
	// Disable colors with LOG_COLOR=false
	log.SetFormatter(&log.TextFormatter{
		DisableColors: os.Getenv("LOG_COLOR") == "false",
	})

	log.Info("Initializing...")

	path, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	cfg, err := config.LoadConfig(path)
	if err != nil {
		log.Fatal(err)
	}

	log.Info("Initialization done!")
	log.Info("Server starting...")

	ctx, forceShutdown := context.WithCancel(context.Background())
	defer forceShutdown()

	asyncProxy := proxy.NewProxy(cfg)

	srv := server.NewServer(cfg, ctx)
	srv.Mux.Handle("/", srv.MetricsMiddleware(asyncProxy))

	asyncProxy.Start(ctx)
	srv.Start()

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

	gracefulCtx, cancel := context.WithTimeout(ctx, cfg.Server.ShutdownTimeout)
	defer cancel()

	srv.Stop(gracefulCtx)
	asyncProxy.Stop(gracefulCtx)
}
