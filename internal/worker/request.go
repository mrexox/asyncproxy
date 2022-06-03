package worker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
)

// Need to store HTTP request properties to allow goroutines handle
// them asynchronously and thread-safe.
type Request struct {
	Header    http.Header
	Method    string
	Body      []byte
	OriginURL string
}

func NewRequest(r *http.Request) (*Request, error) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	return &Request{
		Header:    r.Header.Clone(),
		Method:    r.Method,
		Body:      body,
		OriginURL: r.URL.String(),
	}, nil
}

func (r *Request) URL() (*url.URL, error) {
	res, err := url.Parse(r.OriginURL)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (r *Request) ToHTTPRequest(ctx context.Context, host, scheme string) (*http.Request, error) {
	var bodyReader io.Reader = bytes.NewReader(r.Body)

	reqURL, err := r.URL()
	if err != nil {
		return nil, err
	}

	reqURL.Host = host
	reqURL.Scheme = scheme

	httpReq, err := http.NewRequestWithContext(
		ctx, r.Method, reqURL.String(), bodyReader,
	)
	if err != nil {
		return nil, err
	}

	httpReq.Header = r.Header
	httpReq.Close = true

	return httpReq, nil
}

func (r *Request) String() string {
	return fmt.Sprintf("%s %s", r.Method, r.OriginURL)
}
