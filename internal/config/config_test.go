package config

import (
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// A4: Use t.Setenv (auto-restores after test)
	t.Setenv("SERVICENOW_INSTANCE", "test.service-now.com")
	t.Setenv("SERVICENOW_API_USER", "test_user")
	t.Setenv("SERVICENOW_API_PASSWORD", "test_password")

	cfg, err := Load()

	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.ServiceNow.Instance != "test.service-now.com" {
		t.Errorf("Expected instance test.service-now.com, got %s", cfg.ServiceNow.Instance)
	}

	if cfg.ServiceNow.Username != "test_user" {
		t.Errorf("Expected username test_user, got %s", cfg.ServiceNow.Username)
	}
}

func TestLoadConfigMissingRequired(t *testing.T) {
	// Ensure required env vars are not set (t.Setenv not needed; just don't set them)
	_, err := Load()

	if err == nil {
		t.Error("Expected error for missing required config, got nil")
	}
}

func TestDefaultValues(t *testing.T) {
	t.Setenv("SERVICENOW_INSTANCE", "test.service-now.com")
	t.Setenv("SERVICENOW_API_USER", "test_user")
	t.Setenv("SERVICENOW_API_PASSWORD", "test_password")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.Server.Port != 3000 {
		t.Errorf("Expected default port 3000, got %d", cfg.Server.Port)
	}

	if cfg.Session.TimeoutMinutes != 30 {
		t.Errorf("Expected default timeout 30, got %d", cfg.Session.TimeoutMinutes)
	}

	if cfg.Session.MaxPerUser != 3 {
		t.Errorf("Expected default max per user 3, got %d", cfg.Session.MaxPerUser)
	}

	if cfg.Workspace.Type != "isolated" {
		t.Errorf("Expected default workspace type isolated, got %s", cfg.Workspace.Type)
	}
}

func TestCustomValues(t *testing.T) {
	t.Setenv("SERVICENOW_INSTANCE", "custom.service-now.com")
	t.Setenv("SERVICENOW_API_USER", "custom_user")
	t.Setenv("SERVICENOW_API_PASSWORD", "custom_password")
	t.Setenv("NODE_SERVICE_PORT", "8080")
	t.Setenv("SESSION_TIMEOUT_MINUTES", "60")
	t.Setenv("MAX_SESSIONS_PER_USER", "5")
	t.Setenv("WORKSPACE_TYPE", "persistent")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("Expected port 8080, got %d", cfg.Server.Port)
	}

	if cfg.Session.TimeoutMinutes != 60 {
		t.Errorf("Expected timeout 60, got %d", cfg.Session.TimeoutMinutes)
	}

	if cfg.Session.MaxPerUser != 5 {
		t.Errorf("Expected max per user 5, got %d", cfg.Session.MaxPerUser)
	}

	if cfg.Workspace.Type != "persistent" {
		t.Errorf("Expected workspace type persistent, got %s", cfg.Workspace.Type)
	}
}

func BenchmarkLoadConfig(b *testing.B) {
	b.Setenv("SERVICENOW_INSTANCE", "bench.service-now.com")
	b.Setenv("SERVICENOW_API_USER", "bench_user")
	b.Setenv("SERVICENOW_API_PASSWORD", "bench_password")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Load()
	}
}
