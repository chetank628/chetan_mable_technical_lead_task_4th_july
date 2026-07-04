// Package config loads the API's runtime configuration from the environment.
// It fails fast on misconfiguration that would be unsafe in production (e.g. a
// missing JWT secret) so the service never boots into an insecure state.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the fully-resolved runtime configuration for the API service.
type Config struct {
	// Env is the deployment environment: "dev", "test", or "prod". It gates
	// cookie Secure/SameSite behaviour and whether a JWT secret is mandatory.
	Env string
	// Addr is the listen address for the HTTP server, e.g. ":8080".
	Addr string
	// DBPath is the SQLite database file path. ":memory:" is allowed for tests.
	DBPath string
	// JWTSecret signs and verifies auth tokens. Required outside dev/test.
	JWTSecret []byte
	// JWTTTL is the lifetime of an issued auth token.
	JWTTTL time.Duration
	// CORSOrigin is the exact SPA origin permitted to send credentialed
	// requests. "*" is rejected when credentials are allowed.
	CORSOrigin string
	// MaxBodyBytes caps the ingest request body to guard against oversized
	// payloads (413).
	MaxBodyBytes int64

	// --- Ingest / windowed-pipeline tuning -------------------------------

	// IngestBufferDepth is the capacity of the bounded ingest channel. When
	// full, submissions are dropped and counted (freshness over completeness).
	IngestBufferDepth int
	// WindowDuration bounds how long a single windowed pipeline run waits
	// before flushing (and thereby flushing per-stage metrics).
	WindowDuration time.Duration
	// WindowMaxEvents bounds how many events a single window collects before
	// flushing, whichever comes first with WindowDuration.
	WindowMaxEvents int
	// PipelineBatchSize maps to pipeline.WithBatchSize.
	PipelineBatchSize int
	// PipelineWorkers maps to pipeline.WithWorkerCount (0 => NumCPU default).
	PipelineWorkers int
	// PipelineChannelBuffer maps to pipeline.WithChannelBufferDepth.
	PipelineChannelBuffer int
	// PipelineBatchTimeout maps to pipeline.WithBatchTimeout.
	PipelineBatchTimeout time.Duration
	// DedupCapacity bounds the dedupe LRU seen-set.
	DedupCapacity int
}

// Load reads configuration from the environment, applies defaults, and
// validates it. It returns an error rather than panicking so the caller
// controls the exit path.
func Load() (Config, error) {
	c := Config{
		Env:                   getEnv("APP_ENV", "dev"),
		Addr:                  getEnv("ADDR", ":8080"),
		DBPath:                getEnv("DB_PATH", "mable.db"),
		CORSOrigin:            getEnv("CORS_ORIGIN", "http://localhost:5173"),
		JWTTTL:                getDuration("JWT_TTL", 24*time.Hour),
		MaxBodyBytes:          getInt64("MAX_BODY_BYTES", 1<<20), // 1 MiB
		IngestBufferDepth:     getInt("INGEST_BUFFER_DEPTH", 8192),
		WindowDuration:        getDuration("WINDOW_DURATION", 500*time.Millisecond),
		WindowMaxEvents:       getInt("WINDOW_MAX_EVENTS", 2048),
		PipelineBatchSize:     getInt("PIPELINE_BATCH_SIZE", 256),
		PipelineWorkers:       getInt("PIPELINE_WORKERS", 0),
		PipelineChannelBuffer: getInt("PIPELINE_CHANNEL_BUFFER", 8),
		PipelineBatchTimeout:  getDuration("PIPELINE_BATCH_TIMEOUT", 5*time.Millisecond),
		DedupCapacity:         getInt("DEDUP_CAPACITY", 200_000),
	}

	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		if c.Env == "prod" {
			return Config{}, errors.New("JWT_SECRET must be set in prod")
		}
		// Dev/test convenience: a fixed, clearly non-production secret.
		secret = "dev-insecure-secret-change-me"
	}
	c.JWTSecret = []byte(secret)

	if c.IsProd() && c.CORSOrigin == "*" {
		return Config{}, errors.New("CORS_ORIGIN cannot be '*' with credentialed cookies in prod")
	}
	if c.WindowMaxEvents < 1 {
		return Config{}, fmt.Errorf("WINDOW_MAX_EVENTS must be >= 1, got %d", c.WindowMaxEvents)
	}
	return c, nil
}

// IsProd reports whether the service is running in the production environment.
func (c Config) IsProd() bool { return strings.EqualFold(c.Env, "prod") }

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getInt64(key string, def int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}

func getDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
