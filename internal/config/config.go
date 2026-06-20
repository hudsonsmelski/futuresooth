// Package config loads service configuration from the environment, optionally
// seeded from two local files: .config (non-secret settings, committed) and .env
// (secrets, gitignored). No third-party dependencies.
package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime configuration for the service.
type Config struct {
	// BLSAPIKey is the BLS API v2 registration key. Empty is allowed: the
	// service falls back to the public (keyless) rate limits.
	BLSAPIKey string

	// CensusAPIKey is the U.S. Census Bureau API key. Required for Census data
	// (the API rejects keyless requests); empty disables the Census source.
	CensusAPIKey string

	// HTTPAddr is the listen address for the HTTP server, e.g. ":8080".
	HTTPAddr string

	// DataDir is where the on-disk cache is persisted.
	DataDir string

	// RefreshInterval is how often the background refresher re-pulls series.
	RefreshInterval time.Duration

	// StartYear is the first year of history to request from BLS. The end year is
	// always the current year. Defaults to 1948 (start of the headline series).
	StartYear int

	// AdminToken, if non-empty, guards POST /admin/refresh via X-Admin-Token.
	AdminToken string

	// RequestTimeout bounds a single outbound HTTP request (BLS for the API
	// service, the API service for the web service).
	RequestTimeout time.Duration

	// --- web service (cmd/web) ---

	// FrontendAddr is the listen address for the web UI, e.g. ":3000".
	FrontendAddr string

	// BackendURL is the base URL of the API service the web UI fetches from.
	BackendURL string

	// CacheTTL is how long the web UI caches fetched API responses in memory.
	CacheTTL time.Duration
}

// HasAPIKey reports whether a BLS registration key was provided.
func (c Config) HasAPIKey() bool { return c.BLSAPIKey != "" }

// Load builds a Config from the environment. Non-secret settings in .config and
// secrets in .env (if those files exist in the working directory) are applied
// first, without overriding variables already set in the real environment.
func Load() (Config, error) {
	// .config holds non-secret settings; .env holds secrets. Neither overrides a
	// variable already present in the real environment.
	for _, path := range []string{".config", ".env"} {
		if err := loadEnvFile(path); err != nil {
			return Config{}, fmt.Errorf("loading %s: %w", path, err)
		}
	}

	cfg := Config{
		BLSAPIKey:       getenv("BLS_API_KEY", ""),
		CensusAPIKey:    getenv("CENSUS_API_KEY", ""),
		HTTPAddr:        getenv("HTTP_ADDR", ":8080"),
		DataDir:         getenv("DATA_DIR", "./data"),
		AdminToken:      getenv("ADMIN_TOKEN", ""),
		RequestTimeout:  30 * time.Second,
		RefreshInterval: 360 * time.Hour, // ~twice a month; BLS data is monthly
		StartYear:       1948,
		FrontendAddr:    getenv("FRONTEND_ADDR", ":3000"),
		BackendURL:      strings.TrimRight(getenv("BACKEND_URL", "http://localhost:8080"), "/"),
		CacheTTL:        5 * time.Minute,
	}

	if v := os.Getenv("REFRESH_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid REFRESH_INTERVAL %q: %w", v, err)
		}
		cfg.RefreshInterval = d
	}

	if v := os.Getenv("START_YEAR"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1900 {
			return Config{}, fmt.Errorf("invalid START_YEAR %q", v)
		}
		cfg.StartYear = n
	}

	return cfg, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// loadEnvFile reads a simple KEY=VALUE file and sets any variables not already
// present in the environment. A missing file is not an error. Lines starting
// with '#' and blank lines are ignored; surrounding quotes on values are
// stripped. This intentionally avoids a third-party dependency.
func loadEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		val = strings.Trim(val, `"'`)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); !exists {
			if err := os.Setenv(key, val); err != nil {
				return err
			}
		}
	}
	return sc.Err()
}
