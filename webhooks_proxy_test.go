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
		NumClients:     2,
		RequestTimeout: 10,
	})

	if err != nil {
		t.Errorf("wanted: nil, got: %s", err)
	}

	// Wring numClients
	_, err = NewWebhookProxy(&WebhookProxyConfig{
		NumClients:     0,
		RequestTimeout: 10,
	})

	if err == nil {
		t.Errorf("must fail if numClients < 1")
	}
}

type MockedRoundTripper struct {
	f func(r *http.Request)
}

func (m MockedRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	m.f(r)
	return &http.Response{Status: "200 OK"}, nil
}

func TestHandleRequest(t *testing.T) {
	var (
		checkMethod string
		checkHost   string
		checkScheme string
		req         *http.Request
	)

	transport := MockedRoundTripper{
		func(r *http.Request) {
			checkMethod = r.Method
			checkHost = r.URL.Host
			checkScheme = r.URL.Scheme
		},
	}

	wp := &WebhookProxy{
		make(chan struct{}, 1),
		&http.Client{
			Transport: transport,
		},
		&WebhookProxyConfig{
			Method:         "POST",
			RemoteHost:     "localhost",
			RemoteScheme:   "http",
			ContentType:    "application/json",
			NumClients:     1,
			RequestTimeout: 10,
		},
	}

	// POST request successfully forwarded
	req = httptest.NewRequest("POST", "https://superhost/endpoint", nil)
	wp.HandleRequest(req)

	if checkMethod != "POST" {
		t.Errorf("expected to make a POST request")
	}

	if checkHost != "localhost" {
		t.Errorf("expected to change Host of the request")
	}

	if checkScheme != "http" {
		t.Errorf("expected to change request scheme")
	}
}
