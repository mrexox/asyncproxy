package main

import (
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
	Method         string
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

func (p *Proxy) HandleRequest(r *http.Request) error {
	// Without balancing the process will eat all available file descriptors
	p.balancer <- struct{}{}
	defer func() { <-p.balancer }()

	if err := p.handle(r); err != nil {
		return err
	}

	return nil
}

func (p *Proxy) handle(r *http.Request) error {
	newReq, err := p.transform(r)
	if err != nil {
		return fmt.Errorf("request error: %s", err)
	}

	reqUrl := newReq.URL.String()
	log.Printf("-> %s %s", newReq.Method, reqUrl)

	resp, err := p.client.Do(newReq)
	if err != nil {
		return fmt.Errorf("response error: %s", err)
	}

	log.Printf("   %s %s %s", newReq.Method, reqUrl, resp.Status)

	if resp.StatusCode > 299 {
		return fmt.Errorf(resp.Status)
	}

	return nil
}

func (p *Proxy) transform(r *http.Request) (*http.Request, error) {
	newUrl := *r.URL

	newUrl.Host = p.RemoteHost
	newUrl.Scheme = p.RemoteScheme

	newReq, err := http.NewRequest(p.Method, newUrl.String(), r.Body)
	if err != nil {
		return nil, err
	}

	newReq.Header = r.Header.Clone()

	return newReq, nil
}