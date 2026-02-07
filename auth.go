package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type JWTAuth struct {
	secret []byte
}

func NewJWTAuth(secret string) *JWTAuth {
	return &JWTAuth{secret: []byte(secret)}
}

func (j *JWTAuth) IssueToken(sub string, ttl time.Duration) (string, error) {
	now := time.Now()
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   sub,
		Issuer:    "mandelbrot-auth-proxy",
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
	})
	return t.SignedString(j.secret)
}

func (j *JWTAuth) Validate(raw string) (*jwt.RegisteredClaims, error) {
	t, err := jwt.ParseWithClaims(raw, &jwt.RegisteredClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected alg %v", t.Header["alg"])
		}
		return j.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := t.Claims.(*jwt.RegisteredClaims)
	if !ok || !t.Valid {
		return nil, fmt.Errorf("invalid claims")
	}
	return claims, nil
}

func (j *JWTAuth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hdr := r.Header.Get("Authorization")
		if hdr == "" {
			jsonError(w, http.StatusUnauthorized, "missing Authorization header")
			return
		}

		scheme, token, ok := strings.Cut(hdr, " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
			jsonError(w, http.StatusUnauthorized, "expected: Bearer <token>")
			return
		}

		claims, err := j.Validate(token)
		if err != nil {
			slog.Warn("auth", "err", err, "addr", r.RemoteAddr, "path", r.URL.Path)
			jsonError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}

		slog.Debug("authed", "sub", claims.Subject, "path", r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

// POST /token — open for demo use. In production you'd gate this or
// use an external IdP.
func (j *JWTAuth) HandleToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Subject  string `json:"subject"`
		Duration string `json:"duration"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Subject == "" {
		req.Subject = "anonymous"
	}

	ttl := 24 * time.Hour
	if req.Duration != "" {
		d, err := time.ParseDuration(req.Duration)
		if err != nil {
			jsonError(w, http.StatusBadRequest, "bad duration: "+req.Duration)
			return
		}
		if d > 72*time.Hour {
			d = 72 * time.Hour
		}
		ttl = d
	}

	tok, err := j.IssueToken(req.Subject, ttl)
	if err != nil {
		slog.Error("issue token", "err", err)
		jsonError(w, http.StatusInternalServerError, "token generation failed")
		return
	}

	slog.Info("issued token", "sub", req.Subject, "ttl", ttl)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"token":      tok,
		"expires_in": ttl.String(),
		"subject":    req.Subject,
	})
}
