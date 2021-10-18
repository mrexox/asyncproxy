package proxy

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"

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

	if config.Proxy.NumClients < 1 {
		log.Fatal("number of clients must be >= 1")
	}

	log.Printf("Proxy -- redirect url: %s://%s", remoteURL.Scheme, remoteURL.Host)
	log.Printf("Proxy -- max concurrency: %d", config.Proxy.NumClients)

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
func (p *Proxy) Do(r *ProxyRequest) error {
	httpReq, err := r.ToHTTPRequest(p)
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
	log.Printf("-> %s %s", r.Method, reqURL)

	resp, err := p.client.Do(r)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return fmt.Errorf("response error: %s", err)
	}

	log.Printf("   %s %s %s", r.Method, reqURL, resp.Status)

	if resp.StatusCode > 299 {
		return fmt.Errorf(resp.Status)
	}

	return nil
}
