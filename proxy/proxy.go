package proxy

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync"

	log "github.com/sirupsen/logrus"

	cfg "github.com/evilmartians/asyncproxy/config"
)

// Proxy handles proxy requests
// It controls the number of parallel requests made
type Proxy struct {
	client *http.Client

	openRequests sync.WaitGroup

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

	transport := http.DefaultTransport.(*http.Transport).Clone()

	// Limit open file descriptors
	transport.MaxConnsPerHost = config.Proxy.NumClients
	transport.MaxIdleConnsPerHost = config.Proxy.NumClients

	return &Proxy{
		client: &http.Client{
			Timeout:   config.Proxy.RequestTimeout,
			Transport: transport,
		},
		remoteHost:   remoteURL.Host,
		remoteScheme: remoteURL.Scheme,
	}
}

// Shutdown gracefully waits for running requests to finish
// or returns an error if context was cancelled.
func (p *Proxy) Shutdown(ctx context.Context) error {
	allRequestsClosed := make(chan struct{})
	go func() {
		p.openRequests.Wait()
		close(allRequestsClosed)
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-allRequestsClosed:
			return nil
		}
	}
}

// Sends the ProxyRequest limiting the number of parallel requests
func (p *Proxy) Do(ctx context.Context, r *ProxyRequest) error {
	p.openRequests.Add(1)
	defer p.openRequests.Done()

	httpReq, err := r.ToHTTPRequest(ctx, p)
	if err != nil {
		return fmt.Errorf("creating request: %s", err)
	}

	return p.do(httpReq)
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
		return fmt.Errorf("request error")
	}

	log.WithFields(log.Fields{
		"method": r.Method,
		"url":    reqURL,
		"status": resp.StatusCode,
	}).Info("...done")

	if resp.StatusCode > 299 {
		return fmt.Errorf("response %d", resp.StatusCode)
	}

	return nil
}
