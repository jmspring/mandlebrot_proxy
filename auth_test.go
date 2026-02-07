package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-secret"

func TestJWT_RoundTrip(t *testing.T) {
	auth := NewJWTAuth(testSecret)

	tok, err := auth.IssueToken("alice", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := auth.Validate(tok)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != "alice" {
		t.Errorf("subject = %q", claims.Subject)
	}
}

func TestJWT_Rejects(t *testing.T) {
	auth := NewJWTAuth(testSecret)
	now := time.Now()

	expired, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject: "x", ExpiresAt: jwt.NewNumericDate(now.Add(-time.Hour)),
	}).SignedString([]byte(testSecret))

	noneTok, _ := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.RegisteredClaims{
		Subject: "x", ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
	}).SignedString(jwt.UnsafeAllowNoneSignatureType)

	wrongKey, _ := NewJWTAuth("other").IssueToken("x", time.Hour)

	for name, tok := range map[string]string{
		"expired":   expired,
		"wrong key": wrongKey,
		"alg none":  noneTok,
		"garbage":   "not.a.jwt",
		"empty":     "",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := auth.Validate(tok); err == nil {
				t.Error("expected rejection")
			}
		})
	}
}

func TestMiddleware(t *testing.T) {
	auth := NewJWTAuth(testSecret)
	valid, _ := auth.IssueToken("alice", time.Hour)
	expired, _ := auth.IssueToken("bob", -time.Hour)
	wrongKey, _ := NewJWTAuth("nope").IssueToken("eve", time.Hour)

	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	handler := auth.Middleware(ok)

	cases := []struct {
		name   string
		header string
		want   int
	}{
		{"valid", "Bearer " + valid, 200},
		{"no header", "", 401},
		{"basic auth", "Basic dXNlcjpwYXNz", 401},
		{"bare token", valid, 401},
		{"empty bearer", "Bearer ", 401},
		{"expired", "Bearer " + expired, 401},
		{"wrong key", "Bearer " + wrongKey, 401},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/generate", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Errorf("got %d, want %d", rec.Code, tc.want)
			}
		})
	}
}

func TestHandleToken(t *testing.T) {
	auth := NewJWTAuth(testSecret)

	t.Run("basic", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"subject": "alice", "duration": "1h"})
		rec := httptest.NewRecorder()
		auth.HandleToken(rec, httptest.NewRequest("POST", "/token", bytes.NewReader(body)))

		if rec.Code != 200 {
			t.Fatalf("status = %d", rec.Code)
		}
		var resp map[string]string
		json.NewDecoder(rec.Body).Decode(&resp)
		if resp["token"] == "" {
			t.Fatal("empty token")
		}
		// round-trip: the token we got should validate
		if _, err := auth.Validate(resp["token"]); err != nil {
			t.Errorf("token invalid: %v", err)
		}
	})

	t.Run("defaults to anonymous", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{})
		rec := httptest.NewRecorder()
		auth.HandleToken(rec, httptest.NewRequest("POST", "/token", bytes.NewReader(body)))
		var resp map[string]string
		json.NewDecoder(rec.Body).Decode(&resp)
		if resp["subject"] != "anonymous" {
			t.Errorf("subject = %q", resp["subject"])
		}
	})

	t.Run("bad duration", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"duration": "nope"})
		rec := httptest.NewRecorder()
		auth.HandleToken(rec, httptest.NewRequest("POST", "/token", bytes.NewReader(body)))
		if rec.Code != 400 {
			t.Errorf("status = %d", rec.Code)
		}
	})

	t.Run("caps at 72h", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"subject": "x", "duration": "200h"})
		rec := httptest.NewRecorder()
		auth.HandleToken(rec, httptest.NewRequest("POST", "/token", bytes.NewReader(body)))
		var resp map[string]string
		json.NewDecoder(rec.Body).Decode(&resp)
		if resp["expires_in"] != "72h0m0s" {
			t.Errorf("expires_in = %q", resp["expires_in"])
		}
	})

	t.Run("bad json", func(t *testing.T) {
		rec := httptest.NewRecorder()
		auth.HandleToken(rec, httptest.NewRequest("POST", "/token", bytes.NewReader([]byte("{bad"))))
		if rec.Code != 400 {
			t.Errorf("status = %d", rec.Code)
		}
	})
}
