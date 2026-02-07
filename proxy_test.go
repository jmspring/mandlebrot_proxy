package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
)

func testProxy(t *testing.T, backend *httptest.Server) http.Handler {
	t.Helper()
	u, _ := url.Parse(backend.URL)
	return withLogging(newReverseProxy(u, ":9090"))
}

func TestProxy_Forwards(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/generate" {
			t.Errorf("path = %q", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "width") {
			t.Error("body missing 'width'")
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("PNGDATA"))
	}))
	defer backend.Close()

	handler := testProxy(t, backend)
	req := httptest.NewRequest("POST", "/generate", strings.NewReader(`{"width":640}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "image/png" {
		t.Errorf("content-type = %q", rec.Header().Get("Content-Type"))
	}
	if rec.Body.String() != "PNGDATA" {
		t.Errorf("body = %q", rec.Body.String())
	}
}

func TestProxy_BackendDown(t *testing.T) {
	u, _ := url.Parse("http://127.0.0.1:19999")
	proxy := httputil.NewSingleHostReverseProxy(u)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		jsonError(w, http.StatusBadGateway, "upstream unavailable")
	}

	req := httptest.NewRequest("POST", "/generate", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Code != 502 {
		t.Errorf("status = %d, want 502", rec.Code)
	}
}

func TestProxy_PreservesHeaders(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom") != "yes" {
			t.Error("custom header not forwarded")
		}
		w.WriteHeader(200)
	}))
	defer backend.Close()

	handler := testProxy(t, backend)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Custom", "yes")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Errorf("status = %d", rec.Code)
	}
}

func TestProxy_307Redirect(t *testing.T) {
	var backendURL string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/old" {
			// Upstream redirects with its own absolute URL — this is what
			// we need to rewrite. Many frameworks do this.
			w.Header().Set("Location", backendURL+"/new")
			w.WriteHeader(http.StatusTemporaryRedirect)
			return
		}
		w.Write([]byte("landed"))
	}))
	defer backend.Close()
	backendURL = backend.URL

	u, _ := url.Parse(backend.URL)
	proxy := newReverseProxy(u, ":9090")

	req := httptest.NewRequest("GET", "/old", nil)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Code != 307 {
		t.Fatalf("status = %d, want 307", rec.Code)
	}

	loc := rec.Header().Get("Location")
	if strings.Contains(loc, backend.URL) {
		t.Errorf("Location still points at upstream: %q", loc)
	}
	if !strings.Contains(loc, "/new") {
		t.Errorf("Location missing path: %q", loc)
	}
	if !strings.HasPrefix(loc, "http://localhost:9090") {
		t.Errorf("Location not rewritten to proxy: %q", loc)
	}
}

func TestProxy_RedirectPreservesExternalLocations(t *testing.T) {
	// Redirects to a different host should be left alone.
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "https://example.com/somewhere")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer backend.Close()

	u, _ := url.Parse(backend.URL)
	proxy := newReverseProxy(u, ":9090")

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	loc := rec.Header().Get("Location")
	if loc != "https://example.com/somewhere" {
		t.Errorf("external Location was mangled: %q", loc)
	}
}

func TestProxy_NonRedirectNoRewrite(t *testing.T) {
	// 200 with a Location header (weird but valid) shouldn't break.
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "http://whatever")
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer backend.Close()

	handler := testProxy(t, backend)
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestProxy_RelativeRedirectPassesThrough(t *testing.T) {
	// Relative Location paths are already safe — the browser resolves
	// them against the proxy URL. Make sure we don't mangle them.
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/new", http.StatusTemporaryRedirect)
	}))
	defer backend.Close()

	u, _ := url.Parse(backend.URL)
	proxy := newReverseProxy(u, ":9090")

	req := httptest.NewRequest("GET", "/old", nil)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Code != 307 {
		t.Fatalf("status = %d, want 307", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if loc != "/new" {
		t.Errorf("relative Location mangled: %q", loc)
	}
}
