package proxy

type Queue interface {
	Total() uint64
	Shutdown() error
	EnqueueRequest(r *ProxyRequest, attempt int) error
	DequeueRequest() (r *ProxyRequest, attempt int, err error)
}
