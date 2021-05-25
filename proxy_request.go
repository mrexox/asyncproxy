package main

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
)

type ProxyRequest struct {
	Header http.Header
	Method string
	Body   []byte
	Url    string
}

func NewProxyRequest(r *http.Request) (*ProxyRequest, error) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	return &ProxyRequest{
		Header: r.Header.Clone(),
		Method: r.Method,
		Body:   body,
		Url:    r.URL.String(),
	}, nil
}

func (pr *ProxyRequest) URL() (*url.URL, error) {
	res, err := url.Parse(pr.Url)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (pr *ProxyRequest) ToHTTPRequest(p *Proxy) (*http.Request, error) {
	var bodyReader io.Reader
	bodyReader = bytes.NewReader(pr.Body)

	reqURL, err := pr.URL()
	if err != nil {
		return nil, err
	}

	reqURL.Host = p.RemoteHost
	reqURL.Scheme = p.RemoteScheme

	httpReq, err := http.NewRequest(pr.Method, reqURL.String(), bodyReader)
	if err != nil {
		return nil, err
	}

	httpReq.Header = pr.Header
	httpReq.Close = true

	return httpReq, nil
}
