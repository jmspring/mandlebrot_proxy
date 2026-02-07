package main

import (
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

func newReverseProxy(upstream *url.URL, listenAddr string) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(upstream)

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		slog.Error("proxy", "path", r.URL.Path, "err", err)
		jsonError(w, http.StatusBadGateway, "upstream unavailable")
	}

	// Figure out the public-facing origin for Location rewrites.
	// listenAddr is typically ":9090", so we need "http://localhost:9090".
	proxyHost := listenAddr
	if strings.HasPrefix(proxyHost, ":") {
		proxyHost = "localhost" + proxyHost
	}
	proxyOrigin := "http://" + proxyHost

	proxy.ModifyResponse = func(resp *http.Response) error {
		loc := resp.Header.Get("Location")
		if loc == "" {
			return nil
		}

		// Only rewrite if the Location points at our upstream.
		upstreamOrigin := upstream.Scheme + "://" + upstream.Host
		if strings.HasPrefix(loc, upstreamOrigin) {
			rewritten := proxyOrigin + loc[len(upstreamOrigin):]
			resp.Header.Set("Location", rewritten)
			slog.Debug("rewrote redirect", "from", loc, "to", rewritten)
		}
		return nil
	}

	return proxy
}
