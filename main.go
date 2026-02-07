package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	cfg := loadConfig()

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	})))

	upstream, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", cfg.UpstreamPort))
	proxy := newReverseProxy(upstream, cfg.ListenAddr)

	auth := NewJWTAuth(cfg.JWTSecret)

	// Log a token so operators don't have to hit /token just to test.
	tok, _ := auth.IssueToken("dev-user", 24*time.Hour)
	slog.Info("dev token (24h)", "token", tok)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /token", auth.HandleToken)
	mux.Handle("/", auth.Middleware(proxy))

	srv := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      withLogging(mux),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("listening", "addr", cfg.ListenAddr, "upstream", upstream.String())
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("serve", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(shutdownCtx)
}
