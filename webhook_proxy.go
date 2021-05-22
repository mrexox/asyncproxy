package main

import (
	"fmt"
	"log"
	"net/http"
	"time"
)

type WebhookProxy struct {
	balancer chan struct{}
	client   *http.Client
	*WebhookProxyConfig
}

type WebhookProxyConfig struct {
	Method         string
	RemoteHost     string
	RemoteScheme   string
	ContentType    string
	NumClients     int
	RequestTimeout time.Duration
}

func NewWebhookProxy(cfg *WebhookProxyConfig) (*WebhookProxy, error) {
	if cfg.NumClients < 1 {
		return nil, fmt.Errorf("numClients must be >= 1")
	}

	log.Printf("Redirecting to: %s://%s", cfg.RemoteScheme, cfg.RemoteHost)
	log.Printf("Number of concurrent clients: %d", cfg.NumClients)

	return &WebhookProxy{
		make(chan struct{}, cfg.NumClients),
		&http.Client{
			Timeout: cfg.RequestTimeout * time.Second,
		},
		cfg,
	}, nil
}

func (wp *WebhookProxy) HandleRequest(r *http.Request) {
	// Without balancing the process will eat all available file descriptors
	wp.balancer <- struct{}{}
	defer func() { <-wp.balancer }()

	if err := wp.handle(r); err != nil {
		log.Println(err)
	}
}

func (wp *WebhookProxy) handle(r *http.Request) error {
	requestId := r.Context().Value("reqid")

	newReq, err := wp.transform(r)
	if err != nil {
		return fmt.Errorf("reqid=%d, request error: %s", requestId, err)
	}

	log.Printf("reqid=%d, -> %s %s", requestId, newReq.Method, newReq.URL.String())

	resp, err := wp.client.Do(newReq)
	if err != nil {
		return fmt.Errorf("reqid=%d, response error: %s", requestId, err)
	}

	log.Printf("reqid=%d, %s", requestId, resp.Status)

	return nil
}

func (wp *WebhookProxy) transform(r *http.Request) (*http.Request, error) {
	newUrl := *r.URL

	newUrl.Host = wp.RemoteHost
	newUrl.Scheme = wp.RemoteScheme

	newReq, err := http.NewRequest(wp.Method, newUrl.String(), r.Body)
	if err != nil {
		return nil, err
	}

	newReq.Header.Set("Content-Type", wp.ContentType)

	return newReq, nil
}
