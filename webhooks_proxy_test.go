package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewWebhookProxy(t *testing.T) {
	var err error

	// Success case
	_, err = NewWebhookProxy(&WebhookProxyConfig{
		url:            "string",
		numClients:     400,
		requestTimeout: 10,
	})

	if err != nil {
		t.Errorf("wanted: nil, got: %s", err)
	}

	// Wring numClients
	_, err = NewWebhookProxy(&WebhookProxyConfig{
		url:            "string",
		numClients:     0,
		requestTimeout: 10,
	})

	if err == nil {
		t.Errorf("must fail if numClients < 1")
	}
}

type MockedRoundTripper struct {
	f func()
}

func (m MockedRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	m.f()
	return &http.Response{Status: "200 OK"}, nil
}

func TestHandleRequest(t *testing.T) {
	var (
		numCalls int
		req      *http.Request
	)

	transport := MockedRoundTripper{
		f: func() { numCalls += 1 },
	}

	wp := &WebhookProxy{
		url:      "string",
		balancer: make(chan struct{}, 1),
		client: &http.Client{
			Transport: transport,
		},
	}

	// POST request successfully forwarded
	req = httptest.NewRequest("POST", "http://localhost", nil)
	wp.HandleRequest(req)

	if numCalls != 1 {
		t.Errorf("expected to make a POST request %d", numCalls)
	}

	// GET request ignored
	req = httptest.NewRequest("GET", "http://localhost", nil)
	wp.HandleRequest(req)

	if numCalls != 1 {
		t.Errorf("expected to ignore a GET request")
	}
}
