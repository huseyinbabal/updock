package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadSecretFile_Empty(t *testing.T) {
	if result := ReadSecretFile(""); result != "" {
		t.Errorf("expected empty, got %q", result)
	}
}

func TestReadSecretFile_PlainValue(t *testing.T) {
	if result := ReadSecretFile("mysecret"); result != "mysecret" {
		t.Errorf("expected 'mysecret', got %q", result)
	}
}

func TestReadSecretFile_FileReference(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.txt")
	_ = os.WriteFile(path, []byte("  file-secret  \n"), 0644)

	result := ReadSecretFile(path)
	if result != "file-secret" {
		t.Errorf("expected 'file-secret', got %q", result)
	}
}

func TestReadSecretFile_FileNotExists(t *testing.T) {
	result := ReadSecretFile("/nonexistent/path/secret.txt")
	if result != "/nonexistent/path/secret.txt" {
		t.Errorf("expected path returned as-is, got %q", result)
	}
}

func TestReadSecretFile_NonSlashPrefix(t *testing.T) {
	result := ReadSecretFile("relative/path")
	if result != "relative/path" {
		t.Errorf("expected value returned as-is, got %q", result)
	}
}

func TestLoad_Defaults(t *testing.T) {
	// Clear any env that might interfere
	for _, key := range []string{
		"UPDOCK_DOCKER_HOST", "UPDOCK_INTERVAL", "UPDOCK_MONITOR_ALL",
		"UPDOCK_HTTP_ADDR", "UPDOCK_LOG_LEVEL", "UPDOCK_POLICY_FILE",
	} {
		_ = os.Unsetenv(key)
	}

	cfg := Load()
	if cfg == nil {
		t.Fatal("Load returned nil")
	}
	if cfg.DockerHost != "unix:///var/run/docker.sock" {
		t.Errorf("unexpected docker host: %q", cfg.DockerHost)
	}
	if cfg.MonitorAll != true {
		t.Error("expected MonitorAll true by default")
	}
	if cfg.HTTPAddr != ":8080" {
		t.Errorf("unexpected http addr: %q", cfg.HTTPAddr)
	}
	if cfg.HTTPEnabled != true {
		t.Error("expected HTTPEnabled true by default")
	}
	if cfg.MetricsEnabled != true {
		t.Error("expected MetricsEnabled true by default")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("unexpected log level: %q", cfg.LogLevel)
	}
	if cfg.PolicyFile != "updock.yml" {
		t.Errorf("unexpected policy file: %q", cfg.PolicyFile)
	}
	if cfg.DryRun != false {
		t.Error("expected DryRun false by default")
	}
	if cfg.RunOnce != false {
		t.Error("expected RunOnce false by default")
	}
	if cfg.CleanupImages != false {
		t.Error("expected CleanupImages false by default")
	}
	if cfg.RemoveVolumes != false {
		t.Error("expected RemoveVolumes false by default")
	}
}

func TestLabelConstants(t *testing.T) {
	// Verify all labels use com.updock prefix
	labels := []string{
		LabelEnable, LabelDisable, LabelScope, LabelMonitorOnly,
		LabelNoPull, LabelStopSignal, LabelDependsOn,
		LabelPreCheck, LabelPreUpdate, LabelPostUpdate, LabelPostCheck,
		LabelPreUpdateTimeout, LabelPostUpdateTimeout,
	}
	prefix := "com.updock."
	for _, l := range labels {
		if len(l) < len(prefix) {
			t.Errorf("label too short: %q", l)
		}
		if l[:len(prefix)] != prefix {
			t.Errorf("label %q does not start with %q", l, prefix)
		}
	}
}
