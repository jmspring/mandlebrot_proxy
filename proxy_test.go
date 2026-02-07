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
	proxy := httputil.NewSingleHostReverseProxy(u)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		jsonError(w, http.StatusBadGateway, "upstream unavailable")
	}
	return withLogging(proxy)
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
	// point at nothing
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
