package proxy

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/spf13/viper"
)

// Proxy handles proxy requests.
// It controls the number of parallel requests made.
type Proxy struct {
	client    *http.Client
	fdLimiter chan struct{}

	remoteHost, remoteScheme string
}

// config configures a proxy.
type config struct {
	remoteHost     string
	remoteScheme   string
	numClients     int
	requestTimeout time.Duration
}

// InitProxy applies viper configuration to init a proxy instance.
func InitProxy(v *viper.Viper) *Proxy {
	remoteURL, err := url.Parse(v.GetString("proxy.remote_url"))
	if err != nil {
		log.Fatal(err)
	}

	proxy, err := New(
		&config{
			remoteHost:     remoteURL.Host,
			remoteScheme:   remoteURL.Scheme,
			numClients:     v.GetInt("proxy.num_clients"),
			requestTimeout: time.Duration(v.GetInt("proxy.request_timeout")),
		},
	)
	if err != nil {
		log.Fatal(err)
	}

	return proxy
}

func New(cfg *config) (*Proxy, error) {
	if cfg.numClients < 1 {
		return nil, fmt.Errorf("number of clients must be >= 1")
	}

	log.Printf("Redirecting to: %s://%s", cfg.remoteScheme, cfg.remoteHost)
	log.Printf("Number of concurrent clients: %d", cfg.numClients)

	return &Proxy{
		client: &http.Client{
			Timeout: cfg.requestTimeout * time.Second,
		},
		fdLimiter:    make(chan struct{}, cfg.numClients),
		remoteHost:   cfg.remoteHost,
		remoteScheme: cfg.remoteScheme,
	}, nil
}

// Do sends the ProxyRequest limiting the number of parallel requests
func (p *Proxy) Do(r *ProxyRequest) error {
	httpReq, err := r.ToHTTPRequest(p)
	if err != nil {
		return fmt.Errorf("request error: %s", err)
	}

	// Without file descriptor limiting goroutines might eat all file descriptors
	p.fdLimiter <- struct{}{}
	defer func() { <-p.fdLimiter }()

	if err = p.doRequest(httpReq); err != nil {
		return err
	}

	return nil
}

// Shutdown gracefully waits for running requests to finish.
// Or returns an error if context is down.
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

// doRequest actually performs the HTTP requests.
func (p *Proxy) doRequest(r *http.Request) error {
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
