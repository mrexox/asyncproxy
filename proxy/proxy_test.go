package proxy

import (
	"net/http"
	"testing"
)

func TestNewProxy(t *testing.T) {
	var err error

	// Success case
	_, err = New(&config{
		numClients:     2,
		requestTimeout: 10,
	})
	if err != nil {
		t.Errorf("wanted: nil, got: %s", err)
	}

	// Bad NumClients
	_, err = New(&config{
		numClients:     0,
		requestTimeout: 10,
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

	wp := &Proxy{
		fdLimiter: make(chan struct{}, 1),
		client: &http.Client{
			Transport: transport,
		},

		remoteHost:   "remote",
		remoteScheme: "http",
	}

	// POST request successfully forwarded

	err := wp.Do(&ProxyRequest{
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

func TestProxyRequestMatchEvent(t *testing.T) {
	request := &ProxyRequest{
		Header:    map[string][]string{},
		Method:    "POST",
		Body:      []byte(`<NotificationEventName> Value </NotificationEventName>`),
		OriginURL: "https://nevergone.com/endpoint",
	}

	if request.MatchEvent("Value") != true {
		t.Errorf("expected to match Value notification event")
	}

	if request.MatchEvent("Val(ue") != false {
		t.Errorf("expected to return false if given event is bad for regexp")
	}
}
