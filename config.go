package main

import (
	"log/slog"
	"os"
	"strconv"
)

type Config struct {
	ListenAddr    string
	Image         string
	ContainerPort int
	JWTSecret     string
	LogLevel      slog.Level
}

func loadConfig() Config {
	return Config{
		ListenAddr:    env("LISTEN_ADDR", ":9090"),
		Image:         env("MANDELBROT_IMAGE", "lechgu/mandelbrot"),
		ContainerPort: envInt("CONTAINER_PORT", 8080),
		JWTSecret:     env("JWT_SECRET", "mandelbrot-dev-secret-do-not-use-in-prod"),
		LogLevel:      parseLogLevel(env("LOG_LEVEL", "info")),
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func parseLogLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
