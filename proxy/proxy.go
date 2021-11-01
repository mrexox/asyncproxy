package proxy

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	log "github.com/sirupsen/logrus"

	cfg "github.com/evilmartians/asyncproxy/config"
)

// Proxy handles proxy requests
// It controls the number of parallel requests made
type Proxy struct {
	client    *http.Client
	fdLimiter chan struct{}

	remoteHost, remoteScheme string
}

func NewProxy(config *cfg.Config) *Proxy {
	remoteURL, err := url.Parse(config.Proxy.RemoteUrl)
	if err != nil {
		log.Fatal(err)
	}

	log.WithFields(log.Fields{
		"redirect_url":    fmt.Sprintf("%s://%s", remoteURL.Scheme, remoteURL.Host),
		"max_open_fd":     config.Proxy.NumClients,
		"request_timeout": config.Proxy.RequestTimeout,
	}).Info("Initializing proxy")

	if config.Proxy.NumClients < 1 {
		log.Fatal("number of clients must be >= 1")
	}

	return &Proxy{
		client: &http.Client{
			Timeout: config.Proxy.RequestTimeout,
		},
		fdLimiter:    make(chan struct{}, config.Proxy.NumClients),
		remoteHost:   remoteURL.Host,
		remoteScheme: remoteURL.Scheme,
	}
}

// Shutdown gracefully waits for running requests to finish
// or returns an error if context was cancelled.
func (p *Proxy) Shutdown(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if len(p.fdLimiter) == 0 {
				return nil
			}
		}
	}
}

// Sends the ProxyRequest limiting the number of parallel requests
func (p *Proxy) Do(ctx context.Context, r *ProxyRequest) error {
	httpReq, err := r.ToHTTPRequest(ctx, p)
	if err != nil {
		return fmt.Errorf("request error: %s", err)
	}

	// Without file descriptor limiting goroutines might eat all file descriptors
	p.fdLimiter <- struct{}{}
	defer func() { <-p.fdLimiter }()

	if err = p.do(httpReq); err != nil {
		return err
	}

	return nil
}

// Performs the HTTP requests.
func (p *Proxy) do(r *http.Request) error {
	reqURL := r.URL.String()
	log.WithFields(log.Fields{
		"method": r.Method,
		"url":    reqURL,
	}).Info("proxying...")

	resp, err := p.client.Do(r)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return fmt.Errorf("response error: %s", err)
	}

	log.WithFields(log.Fields{
		"method": r.Method,
		"url":    reqURL,
		"status": resp.StatusCode,
	}).Info("...done")

	if resp.StatusCode > 299 {
		return fmt.Errorf(resp.Status)
	}

	return nil
}
