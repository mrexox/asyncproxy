package server

import (
	"context"
	"net"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/evilmartians/asyncproxy/config"
)

type Server struct {
	Mux *http.ServeMux

	http    *http.Server
	metrics *Metrics
}

func NewServer(cfg *config.Config, ctx context.Context) Server {
	log.WithFields(log.Fields{
		"bind":             cfg.Server.Bind,
		"shutdown_timeout": cfg.Server.ShutdownTimeout,
	}).Info("Initializing server")

	mux := http.NewServeMux()

	httpServer := &http.Server{
		Addr:         cfg.Server.Bind,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		BaseContext:  func(_ net.Listener) context.Context { return ctx },
	}

	httpServer.SetKeepAlivesEnabled(false)

	return Server{
		Mux:     mux,
		http:    httpServer,
		metrics: NewMetrics(cfg),
	}
}

func (s Server) Start() {
	s.metrics.Start()
	go func() {
		if err := s.http.ListenAndServe(); err != http.ErrServerClosed {
			log.WithError(err).Warn("server error")
		}
	}()
}

func (s Server) Stop(ctx context.Context) {
	if err := s.http.Shutdown(ctx); err != nil {
		log.Fatal(err)
	} else {
		log.Info("Gracefully stopped server!")
	}

	if err := s.metrics.Shutdown(ctx); err != nil {
		log.Fatal(err)
	} else {
		log.Info("Gracefully stopped metrics!")
	}
}

func (s Server) MetricsMiddleware(next http.Handler) http.Handler {
	return s.metrics.Middleware(next)
}
