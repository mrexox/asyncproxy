package main

import (
	"fmt"
	"log"
	"net/http"
	"time"
)

type WebhookProxy struct {
	url      string
	balancer chan struct{}
	client   *http.Client
}

type WebhookProxyConfig struct {
	url            string
	numClients     int
	requestTimeout time.Duration
}

func NewWebhookProxy(cfg *WebhookProxyConfig) (*WebhookProxy, error) {
	log.Printf("Redirection URL: %s", cfg.url)

	if cfg.numClients < 1 {
		return nil, fmt.Errorf("numClients must be >= 1")
	}

	log.Printf("Number of concurrent clients: %d", cfg.numClients)

	return &WebhookProxy{
		url:      cfg.url,
		balancer: make(chan struct{}, cfg.numClients),
		client: &http.Client{
			Timeout: cfg.requestTimeout * time.Second,
		},
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

	if r.Method != "POST" {
		return fmt.Errorf("reqid=%d, skip (invalid method)", requestId)
	}

	resp, err := wp.client.Post(wp.url, "application/xml", r.Body)
	if err != nil {
		return fmt.Errorf("reqid=%d, error: %s", requestId, err)
	}

	log.Printf("reqid=%d, %s", requestId, resp.Status)

	return nil
}
