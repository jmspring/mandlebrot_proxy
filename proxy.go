package main

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
)

// net.http allows up to 10 redirects
const maxRedirects = 10

// redirectFollower is a custom RoundTripper that allows the following
// of redirects internally
type redirectFollower struct {
	next http.RoundTripper
}

func (rf *redirectFollower) RoundTrip(req *http.Request) (*http.Response, error) {
	req.RequestURI = ""

	// to do the redirect internally, we need to save the body such
	// that, it can be re-read for the next request
	if req.Body != nil && req.GetBody == nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(body)), nil
		}
	}

	c := &http.Client{
		Transport: rf.next,
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return errors.New("too many redirects")
			}
			slog.Debug("following upstream redirect",
				"hop", len(via),
				"to", r.URL.String(),
			)
			return nil
		},
	}
	return c.Do(req)
}

func newReverseProxy(upstream *url.URL) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(upstream)

	// set the Director req.Host to the upstreams host (so that
	// redirects do not try and connect to the proxy)
	defaultDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		defaultDirector(req)
		req.Host = upstream.Host
	}

	proxy.Transport = &redirectFollower{next: http.DefaultTransport}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		slog.Error("proxy", "path", r.URL.Path, "err", err)
		jsonError(w, http.StatusBadGateway, "upstream unavailable")
	}

	return proxy
}
