package config

import (
	"fmt"
	"os"
	"time"
)

// StreamKeyMapping defines how logical track kinds map to stream keys.
type StreamKeyMapping struct {
	Video   string
	Desktop string
	Audio   string
}

// Config aggregates runtime configuration for the publisher client.
type Config struct {
	ClientID     string
	BackendURL   string
	PollInterval time.Duration
	WhipBaseURL  string
	WhipSecret   string
	StreamKeys   StreamKeyMapping
	HTTPTimeout  time.Duration
}

// Load returns configuration populated from environment variables, applying
// defaults when necessary.
func Load() (Config, error) {
	cfg := Config{
		ClientID:     getEnvDefault("CLIENT_ID", "my-golang-publisher-1"),
		BackendURL:   getEnvDefault("BACKEND_URL", "http://localhost:8080"),
		PollInterval: mustParseDuration(getEnvDefault("POLL_INTERVAL", "5s")),
		WhipBaseURL:  getEnvDefault("WHIP_BASE_URL", "http://192.168.123.21:2022/rtc/v1/whip/?app=live"),
		WhipSecret:   getEnvDefault("WHIP_SECRET", "6a9df1a76bc242f8adb8a309fa78fe92"),
		StreamKeys: StreamKeyMapping{
			Video:   getEnvDefault("STREAM_KEY_VIDEO", "cama"),
			Desktop: getEnvDefault("STREAM_KEY_DESKTOP", "desktop"),
			Audio:   getEnvDefault("STREAM_KEY_AUDIO", "audio"),
		},
		HTTPTimeout: mustParseDuration(getEnvDefault("HTTP_TIMEOUT", "10s")),
	}

	if cfg.ClientID == "" {
		return Config{}, fmt.Errorf("CLIENT_ID cannot be empty")
	}

	if cfg.BackendURL == "" {
		return Config{}, fmt.Errorf("BACKEND_URL cannot be empty")
	}

	if cfg.WhipBaseURL == "" {
		return Config{}, fmt.Errorf("WHIP_BASE_URL cannot be empty")
	}

	if cfg.WhipSecret == "" {
		return Config{}, fmt.Errorf("WHIP_SECRET cannot be empty")
	}

	return cfg, nil
}

func mustParseDuration(value string) time.Duration {
	d, err := time.ParseDuration(value)
	if err != nil {
		panic(fmt.Sprintf("invalid duration %q: %v", value, err))
	}
	return d
}

func getEnvDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
