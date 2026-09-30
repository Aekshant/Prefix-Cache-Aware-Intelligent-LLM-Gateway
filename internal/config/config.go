// Package config loads gateway settings from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Worker describes one downstream LLM backend (e.g. an Ollama instance).
type Worker struct {
	ID  string
	URL string
}

// Config holds all runtime settings for the gateway.
type Config struct {
	ListenAddr      string        // LISTEN_ADDR
	Workers         []Worker      // WORKERS
	CacheTTL        time.Duration // CACHE_TTL
	JanitorInterval time.Duration // JANITOR_INTERVAL
	MaxActiveConns  int64         // MAX_ACTIVE_CONNS
	PrefixBytes     int           // PREFIX_BYTES
}

const defaultWorkers = "worker-1=http://127.0.0.1:11434,worker-2=http://127.0.0.1:11435"

// Load reads the configuration from the environment, falling back to defaults.
func Load() (*Config, error) {
	cfg := &Config{
		ListenAddr: getEnv("LISTEN_ADDR", ":8080"),
	}

	var err error
	if cfg.Workers, err = ParseWorkers(getEnv("WORKERS", defaultWorkers)); err != nil {
		return nil, err
	}
	if cfg.CacheTTL, err = getDuration("CACHE_TTL", 10*time.Minute); err != nil {
		return nil, err
	}
	if cfg.JanitorInterval, err = getDuration("JANITOR_INTERVAL", 30*time.Second); err != nil {
		return nil, err
	}
	if cfg.MaxActiveConns, err = getInt64("MAX_ACTIVE_CONNS", 5); err != nil {
		return nil, err
	}
	prefix, err := getInt64("PREFIX_BYTES", 120)
	if err != nil {
		return nil, err
	}
	cfg.PrefixBytes = int(prefix)

	return cfg, nil
}

// ParseWorkers parses a comma-separated list of "id=url" pairs.
func ParseWorkers(raw string) ([]Worker, error) {
	var workers []Worker
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, url, ok := strings.Cut(part, "=")
		if !ok || id == "" || url == "" {
			return nil, fmt.Errorf("invalid worker %q: expected id=url", part)
		}
		workers = append(workers, Worker{ID: strings.TrimSpace(id), URL: strings.TrimRight(strings.TrimSpace(url), "/")})
	}
	if len(workers) == 0 {
		return nil, fmt.Errorf("WORKERS must contain at least one worker")
	}
	return workers, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getDuration(key string, fallback time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	return d, nil
}

func getInt64(key string, fallback int64) (int64, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid %s: must be a positive integer", key)
	}
	return n, nil
}
