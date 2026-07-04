package config

import (
	"testing"
	"time"
)

func TestMaskDBURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"standard", "postgres://user:pass@localhost:5434/costdb", "postgres://****@localhost:5434/costdb"},
		{"no credentials", "postgres://localhost:5434/costdb", "postgres://localhost:5434/costdb"},
		{"empty", "", ""},
		// Note: maskDBURL finds the first @ — passwords with @ in them
		// will mask incorrectly. Accepted for PoC (URL-encoded passwords
		// like %40 don't have this issue).
		{"password with @", "postgres://admin:p@ss@db.example.com:5432/prod", "postgres://****@ss@db.example.com:5432/prod"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := maskDBURL(tc.url)
			if got != tc.want {
				t.Errorf("maskDBURL(%q) = %q, want %q", tc.url, got, tc.want)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{"valid", Config{OSACBaseURL: "http://localhost", InventoryDBURL: "postgres://..."}, false},
		{"missing OSAC URL", Config{OSACBaseURL: "", InventoryDBURL: "postgres://..."}, true},
		{"missing DB URL", Config{OSACBaseURL: "http://localhost", InventoryDBURL: ""}, true},
		{"both missing", Config{}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestEnvOrDefault(t *testing.T) {
	got := envOrDefault("THIS_ENV_SHOULD_NOT_EXIST_12345", "fallback")
	if got != "fallback" {
		t.Errorf("expected fallback, got %q", got)
	}

	t.Setenv("TEST_CONFIG_ENV", "override")
	got = envOrDefault("TEST_CONFIG_ENV", "fallback")
	if got != "override" {
		t.Errorf("expected override, got %q", got)
	}
}

func TestDurationOrDefault(t *testing.T) {
	got := durationOrDefault("THIS_ENV_SHOULD_NOT_EXIST_12345", 5*time.Minute)
	if got != 5*time.Minute {
		t.Errorf("expected 5m, got %v", got)
	}

	t.Setenv("TEST_DURATION_ENV", "30s")
	got = durationOrDefault("TEST_DURATION_ENV", 5*time.Minute)
	if got != 30*time.Second {
		t.Errorf("expected 30s, got %v", got)
	}

	t.Setenv("TEST_DURATION_BAD", "notaduration")
	got = durationOrDefault("TEST_DURATION_BAD", 10*time.Second)
	if got != 10*time.Second {
		t.Errorf("expected 10s fallback for bad duration, got %v", got)
	}
}

func TestDiagnostics_MasksCredentials(t *testing.T) {
	cfg := Config{
		OSACBaseURL:    "http://osac",
		OSACToken:      "secret-token",
		InventoryDBURL: "postgres://admin:secretpass@db:5432/costdb",
		LogLevel:       "info",
	}
	diag := cfg.Diagnostics()

	if diag.InventoryDBHost == cfg.InventoryDBURL {
		t.Error("diagnostics should mask DB URL credentials")
	}
	if !diag.OSACTokenSet {
		t.Error("OSACTokenSet should be true when token is set")
	}
}

func TestDiagnostics_NoToken(t *testing.T) {
	cfg := Config{OSACBaseURL: "http://osac", InventoryDBURL: "postgres://localhost/db"}
	diag := cfg.Diagnostics()

	if diag.OSACTokenSet {
		t.Error("OSACTokenSet should be false when token is empty")
	}
	if diag.OSACCACertSet {
		t.Error("OSACCACertSet should be false when cert is empty")
	}
}

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("OSAC_BASE_URL", "")
	t.Setenv("INVENTORY_DB_URL", "")
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("LOG_FORMAT", "")
	t.Setenv("METRICS_PORT", "")
	t.Setenv("DEBUG_DASHBOARD", "")

	cfg := Load()

	if cfg.OSACBaseURL != "http://localhost:8011" {
		t.Errorf("default OSAC URL: got %q", cfg.OSACBaseURL)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("default log level: got %q", cfg.LogLevel)
	}
	if cfg.LogFormat != "text" {
		t.Errorf("default log format: got %q", cfg.LogFormat)
	}
	if cfg.MetricsPort != "9000" {
		t.Errorf("default metrics port: got %q", cfg.MetricsPort)
	}
	if !cfg.DebugDashboard {
		t.Error("debug dashboard should default to true")
	}
	if cfg.ReconcileInterval != 1*time.Hour {
		t.Errorf("default reconcile interval: got %v", cfg.ReconcileInterval)
	}
}

func TestLoad_DebugDashboardFalse(t *testing.T) {
	t.Setenv("DEBUG_DASHBOARD", "false")
	cfg := Load()
	if cfg.DebugDashboard {
		t.Error("DEBUG_DASHBOARD=false should disable dashboard")
	}
}
