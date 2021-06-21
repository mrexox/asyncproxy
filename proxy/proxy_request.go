package proxy

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
)

// Need to store HTTP request properties to allow goroutines handle
// them asynchronously and thread-safe.
type ProxyRequest struct {
	Header    http.Header
	Method    string
	Body      []byte
	OriginURL string
}

func NewProxyRequest(r *http.Request) (*ProxyRequest, error) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	return &ProxyRequest{
		Header:    r.Header.Clone(),
		Method:    r.Method,
		Body:      body,
		OriginURL: r.URL.String(),
	}, nil
}

func (pr *ProxyRequest) URL() (*url.URL, error) {
	res, err := url.Parse(pr.OriginURL)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (pr *ProxyRequest) ToHTTPRequest(p *Proxy) (*http.Request, error) {
	var bodyReader io.Reader = bytes.NewReader(pr.Body)

	reqURL, err := pr.URL()
	if err != nil {
		return nil, err
	}

	reqURL.Host = p.remoteHost
	reqURL.Scheme = p.remoteScheme

	httpReq, err := http.NewRequest(pr.Method, reqURL.String(), bodyReader)
	if err != nil {
		return nil, err
	}

	httpReq.Header = pr.Header
	httpReq.Close = true

	return httpReq, nil
}
