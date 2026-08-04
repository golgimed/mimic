package config

import (
	"log/slog"
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != 3000 {
		t.Errorf("Port = %d, want 3000", cfg.Port)
	}
	if cfg.LogLevel != slog.LevelInfo {
		t.Errorf("LogLevel = %v, want info", cfg.LogLevel)
	}
	if cfg.DBPath != "db/simulator.sqlite" {
		t.Errorf("DBPath = %q, want default", cfg.DBPath)
	}
	if cfg.OpenAPIPersist {
		t.Errorf("OpenAPIPersist = true, want false by default")
	}
	if cfg.EnabledProviders != nil {
		t.Errorf("EnabledProviders = %v, want nil", cfg.EnabledProviders)
	}
}

func TestLoadInvalidPort(t *testing.T) {
	t.Setenv("PORT", "abc")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for invalid PORT")
	}
}

func TestLoadInvalidLogLevel(t *testing.T) {
	t.Setenv("LOG_LEVEL", "verbose")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for invalid LOG_LEVEL")
	}
}

func TestLoadInvalidSchedulerInterval(t *testing.T) {
	t.Setenv("SCHEDULER_INTERVAL_MS", "not-a-number")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for invalid SCHEDULER_INTERVAL_MS")
	}
}

func TestLoadInvalidBool(t *testing.T) {
	t.Setenv("MIMIC_OPENAPI_PERSIST", "maybe")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for invalid MIMIC_OPENAPI_PERSIST")
	}
}

func TestLoadEnabledProviders(t *testing.T) {
	t.Setenv("MIMIC_PROVIDERS", " zenvia, integraicp ,,")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"zenvia", "integraicp"}
	if len(cfg.EnabledProviders) != len(want) {
		t.Fatalf("EnabledProviders = %v, want %v", cfg.EnabledProviders, want)
	}
	for i, v := range want {
		if cfg.EnabledProviders[i] != v {
			t.Errorf("EnabledProviders[%d] = %q, want %q", i, cfg.EnabledProviders[i], v)
		}
	}
}

func TestLoadDotEnvMissingFileIsNotError(t *testing.T) {
	if err := LoadDotEnv("does-not-exist.env"); err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
}

func TestLoadDotEnvDoesNotOverrideExistingEnv(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/.env"
	if err := os.WriteFile(path, []byte("PORT=9999\n"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	t.Setenv("PORT", "1234")
	if err := LoadDotEnv(path); err != nil {
		t.Fatalf("LoadDotEnv: %v", err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != 1234 {
		t.Errorf("Port = %d, want 1234 (existing env should win over .env)", cfg.Port)
	}
}
