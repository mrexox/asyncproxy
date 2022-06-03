package worker

import (
	"context"
	"net/http"
	"testing"
)

type MockedRoundTripper struct {
	f func(r *http.Request)
}

func (m MockedRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	m.f(r)
	return &http.Response{Status: "200 OK"}, nil
}

func TestDo(t *testing.T) {
	var (
		checkHost   string
		checkScheme string
		checkPath   string
		checkBody   []byte = make([]byte, 4)
	)

	transport := MockedRoundTripper{
		func(r *http.Request) {
			checkHost = r.URL.Host
			checkScheme = r.URL.Scheme
			checkPath = r.URL.Path
			_, _ = r.Body.Read(checkBody)
		},
	}

	client := &Client{
		client: &http.Client{
			Transport: transport,
		},

		remoteHost:   "remote",
		remoteScheme: "http",
	}

	// POST request successfully forwarded

	err := client.Do(context.Background(), &Request{
		Header:    map[string][]string{},
		Method:    "POST",
		Body:      []byte("Body"),
		OriginURL: "https://nevergone.com/endpoint",
	})
	if err != nil {
		t.Errorf("request should complete without errors: %s", err)
	}
	if string(checkBody) != "Body" {
		t.Errorf("expected not to change request body: %s != Body", checkBody)
	}
	if checkHost != "remote" {
		t.Errorf("expected to change request host: %s != remote", checkHost)
	}
	if checkScheme != "http" {
		t.Errorf("expected to change request scheme: %s != http", checkScheme)
	}
	if checkPath != "/endpoint" {
		t.Errorf("expected to change request endpoint: %s != /endpoint", checkPath)
	}
}
