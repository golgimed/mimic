// Package config loads .env (if present) and typed settings from the
// environment, mirroring the knobs documented in .env.example.
package config

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// LoadDotEnv applies KEY=VALUE lines from path into the process environment.
// Existing environment variables are never overridden, matching Node's
// process.loadEnvFile behavior. Missing file is not an error.
func LoadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
	return scanner.Err()
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s=%q is not a valid integer", key, v)
	}
	return n, nil
}

func getEnvBool(key string, fallback bool) (bool, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s=%q is not a valid boolean", key, v)
	}
	return b, nil
}

func parseLevel(level string) (slog.Level, error) {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("LOG_LEVEL=%q is not one of debug|info|warn|error", level)
	}
}

type Config struct {
	Port              int
	LogLevel          slog.Level
	SchedulerInterval time.Duration
	DBPath            string
	DefaultDelay      time.Duration
	ZenviaStatusDelay time.Duration
	OpenAPIPersist    bool
	// EnabledProviders filters which registered providers are served, per
	// MIMIC_PROVIDERS (comma-separated). Empty means "all enabled."
	EnabledProviders []string
}

func Load() (Config, error) {
	port, err := getEnvInt("PORT", 3000)
	if err != nil {
		return Config{}, err
	}
	logLevel, err := parseLevel(getEnv("LOG_LEVEL", "info"))
	if err != nil {
		return Config{}, err
	}
	schedulerIntervalMS, err := getEnvInt("SCHEDULER_INTERVAL_MS", 1000)
	if err != nil {
		return Config{}, err
	}
	defaultDelayMS, err := getEnvInt("DEFAULT_DELAY_MS", 0)
	if err != nil {
		return Config{}, err
	}
	zenviaStatusDelayMS, err := getEnvInt("ZENVIA_STATUS_DELAY_MS", 2000)
	if err != nil {
		return Config{}, err
	}
	openAPIPersist, err := getEnvBool("MIMIC_OPENAPI_PERSIST", false)
	if err != nil {
		return Config{}, err
	}

	return Config{
		Port:              port,
		LogLevel:          logLevel,
		SchedulerInterval: time.Duration(schedulerIntervalMS) * time.Millisecond,
		DBPath:            getEnv("DB_PATH", "db/simulator.sqlite"),
		DefaultDelay:      time.Duration(defaultDelayMS) * time.Millisecond,
		ZenviaStatusDelay: time.Duration(zenviaStatusDelayMS) * time.Millisecond,
		OpenAPIPersist:    openAPIPersist,
		EnabledProviders:  getEnvList("MIMIC_PROVIDERS"),
	}, nil
}

// getEnvList parses a comma-separated env var into a trimmed, non-empty
// slice. Returns nil if unset/empty, meaning "no filter."
func getEnvList(key string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}
	var out []string
	for _, v := range strings.Split(raw, ",") {
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
