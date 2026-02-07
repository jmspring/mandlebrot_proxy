package main

import (
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// builds a reverse proxy that rewrites Location headers
// on 3xx responses so redirects route back through the proxy rather
// than leaking the upstream address to the client.
func newReverseProxy(upstream *url.URL, listenAddr string) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(upstream)

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		slog.Error("proxy", "path", r.URL.Path, "err", err)
		jsonError(w, http.StatusBadGateway, "upstream unavailable")
	}

	// public facing or local?
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
