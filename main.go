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

	slog.Info("starting", "addr", cfg.ListenAddr, "image", cfg.Image, "port", cfg.ContainerPort)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// --- container lifecycle ---

	dm, err := NewDockerManager(cfg.Image, cfg.ContainerPort)
	if err != nil {
		fatal("docker client", err)
	}

	cid, err := dm.Start(ctx)
	if err != nil {
		fatal("start container", err)
	}
	slog.Info("container started", "id", cid[:12])

	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := dm.Stop(cleanupCtx, cid); err != nil {
			slog.Error("cleanup failed", "err", err)
		}
	}()

	if err := dm.WaitReady(ctx, 30*time.Second); err != nil {
		fatal("health check", err)
	}
	slog.Info("container ready")

	// --- auth + proxy ---

	auth := NewJWTAuth(cfg.JWTSecret)

	tok, _ := auth.IssueToken("dev-user", 24*time.Hour)
	slog.Info("dev token (24h)", "token", tok)

	upstream, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", cfg.ContainerPort))
	proxy := newReverseProxy(upstream)

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

	go func() {
		slog.Info("listening", "addr", cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fatal("serve", err)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(shutdownCtx)
}

func fatal(msg string, err error) {
	slog.Error(msg, "err", err)
	os.Exit(1)
}
