package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"
)

type Proxy struct {
	balancer chan struct{}
	client   *http.Client
	*ProxyConfig
}

type ProxyConfig struct {
	RemoteHost     string
	RemoteScheme   string
	NumClients     int
	RequestTimeout time.Duration
}

func NewProxy(cfg *ProxyConfig) (*Proxy, error) {
	if cfg.NumClients < 1 {
		return nil, fmt.Errorf("number of clients must be >= 1")
	}

	log.Printf("Redirecting to: %s://%s", cfg.RemoteScheme, cfg.RemoteHost)
	log.Printf("Number of concurrent clients: %d", cfg.NumClients)

	return &Proxy{
		make(chan struct{}, cfg.NumClients),
		&http.Client{
			Timeout: cfg.RequestTimeout * time.Second,
		},
		cfg,
	}, nil
}

func (p *Proxy) Do(r *ProxyRequest) error {
	// Without balancing the process will eat all available file descriptors
	p.balancer <- struct{}{}
	defer func() { <-p.balancer }()

	httpReq, err := r.ToHTTPRequest(p)
	if err != nil {
		return fmt.Errorf("request error: %s", err)
	}

	if err := p.sendRequest(httpReq); err != nil {
		return err
	}

	return nil
}

func (p *Proxy) Shutdown(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if len(p.balancer) == 0 {
				return nil
			}
		}
	}
}

func (p *Proxy) sendRequest(r *http.Request) error {
	reqUrl := r.URL.String()
	log.Printf("-> %s %s", r.Method, reqUrl)

	resp, err := p.client.Do(r)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return fmt.Errorf("response error: %s", err)
	}

	log.Printf("   %s %s %s", r.Method, reqUrl, resp.Status)

	if resp.StatusCode > 299 {
		return fmt.Errorf(resp.Status)
	}

	return nil
}
