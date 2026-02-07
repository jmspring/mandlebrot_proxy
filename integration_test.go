package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func newTestStack(t *testing.T) (*httptest.Server, *JWTAuth) {
	t.Helper()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/generate" && r.Method == "POST":
			w.Header().Set("Content-Type", "image/png")
			w.Write([]byte("FAKEPNG"))
		case r.URL.Path == "/":
			w.Write([]byte("ok"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(backend.Close)

	auth := NewJWTAuth(testSecret)
	u, _ := url.Parse(backend.URL)
	proxy := newReverseProxy(u, ":0") // port irrelevant for these tests

	mux := http.NewServeMux()
	mux.HandleFunc("POST /token", auth.HandleToken)
	mux.Handle("/", auth.Middleware(proxy))

	srv := httptest.NewServer(withLogging(mux))
	t.Cleanup(srv.Close)
	return srv, auth
}

func TestE2E_NoAuth(t *testing.T) {
	srv, _ := newTestStack(t)
	resp, err := srv.Client().Post(srv.URL+"/generate", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestE2E_TokenThenGenerate(t *testing.T) {
	srv, _ := newTestStack(t)
	client := srv.Client()

	// get a token
	resp, err := client.Post(srv.URL+"/token", "application/json",
		strings.NewReader(`{"subject":"e2e","duration":"1h"}`))
	if err != nil {
		t.Fatal(err)
	}
	var tr map[string]string
	json.NewDecoder(resp.Body).Decode(&tr)
	resp.Body.Close()

	if tr["token"] == "" {
		t.Fatal("empty token")
	}

	// use it
	body := `{"width":640,"height":480,"iterations":100,"re_min":-2,"re_max":1,"im_min":-1,"im_max":1,"kind":"png"}`
	req, _ := http.NewRequest("POST", srv.URL+"/generate", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tr["token"])
	req.Header.Set("Content-Type", "application/json")

	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d: %s", resp.StatusCode, b)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "image/png" {
		t.Errorf("content-type = %q", ct)
	}
}

func TestE2E_BadTokens(t *testing.T) {
	srv, _ := newTestStack(t)
	client := srv.Client()

	expired, _ := NewJWTAuth(testSecret).IssueToken("x", -time.Hour)
	wrongKey, _ := NewJWTAuth("wrong").IssueToken("x", time.Hour)

	for _, tok := range []string{expired, wrongKey, "garbage"} {
		req, _ := http.NewRequest("POST", srv.URL+"/generate", strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 401 {
			t.Errorf("token %q: status = %d", tok[:min(len(tok), 20)], resp.StatusCode)
		}
	}
}
