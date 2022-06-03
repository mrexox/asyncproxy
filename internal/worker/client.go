package worker

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync"

	log "github.com/sirupsen/logrus"

	cfg "github.com/evilmartians/asyncproxy/config"
)

// Client performs the requests
// It controls the number of parallel requests made
type Client struct {
	client *http.Client

	openRequests sync.WaitGroup

	remoteHost, remoteScheme string
}

func NewClient(config *cfg.Config) *Client {
	remoteURL, err := url.Parse(config.Proxy.RemoteUrl)
	if err != nil {
		log.Fatal(err)
	}

	log.WithFields(log.Fields{
		"redirect_url":    fmt.Sprintf("%s://%s", remoteURL.Scheme, remoteURL.Host),
		"max_open_fd":     config.Proxy.NumClients,
		"request_timeout": config.Proxy.RequestTimeout,
	}).Info("Initializing proxy")

	if config.Proxy.NumClients < 1 {
		log.Fatal("number of clients must be >= 1")
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()

	// Limit open file descriptors
	transport.MaxConnsPerHost = config.Proxy.NumClients
	transport.MaxIdleConnsPerHost = config.Proxy.NumClients

	return &Client{
		client: &http.Client{
			Timeout:   config.Proxy.RequestTimeout,
			Transport: transport,
		},
		remoteHost:   remoteURL.Host,
		remoteScheme: remoteURL.Scheme,
	}
}

// Shutdown gracefully waits for running requests to finish
// or returns an error if context was cancelled.
func (c *Client) Shutdown(ctx context.Context) error {
	allRequestsClosed := make(chan struct{})
	go func() {
		c.openRequests.Wait()
		close(allRequestsClosed)
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-allRequestsClosed:
			return nil
		}
	}
}

// Sends the Request limiting the number of parallel requests
func (c *Client) Do(ctx context.Context, r *Request) error {
	c.openRequests.Add(1)
	defer c.openRequests.Done()

	httpReq, err := r.ToHTTPRequest(ctx, c.remoteHost, c.remoteScheme)
	if err != nil {
		return fmt.Errorf("creating request: %s", err)
	}

	return c.do(httpReq)
}

// Performs the HTTP requests.
func (c *Client) do(r *http.Request) error {
	reqURL := r.URL.String()
	log.WithFields(log.Fields{
		"method": r.Method,
		"url":    reqURL,
	}).Info("proxying...")

	resp, err := c.client.Do(r)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return fmt.Errorf("request error")
	}

	log.WithFields(log.Fields{
		"method": r.Method,
		"url":    reqURL,
		"status": resp.StatusCode,
	}).Info("...done")

	if resp.StatusCode > 299 {
		return fmt.Errorf("response %d", resp.StatusCode)
	}

	return nil
}
