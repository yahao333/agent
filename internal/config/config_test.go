package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_MissingFile(t *testing.T) {
	workDir := t.TempDir()
	cfg, err := Load(filepath.Join(workDir, "nonexistent.yaml"))
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	// Should return defaults.
	if cfg.MaxIterations != 50 {
		t.Errorf("expected default MaxIterations 50, got %d", cfg.MaxIterations)
	}
	if cfg.MaxCostUSD != 5.0 {
		t.Errorf("expected default MaxCostUSD 5.0, got %f", cfg.MaxCostUSD)
	}
	if cfg.MaxConsecutiveFails != 3 {
		t.Errorf("expected default MaxConsecutiveFails 3, got %d", cfg.MaxConsecutiveFails)
	}
	if cfg.PermissionMode != "bypassPermissions" {
		t.Errorf("expected default PermissionMode bypassPermissions, got %q", cfg.PermissionMode)
	}
}

func TestLoad_PartialYAML(t *testing.T) {
	workDir := t.TempDir()
	cfgPath := filepath.Join(workDir, "config.yaml")

	partial := `
max_iterations: 10
max_cost_usd: 2.5
model: sonnet
verify_cmd: "make verify"
`
	if err := os.WriteFile(cfgPath, []byte(partial), 0644); err != nil {
		t.Fatalf("setup error: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Specified values.
	if cfg.MaxIterations != 10 {
		t.Errorf("expected MaxIterations 10, got %d", cfg.MaxIterations)
	}
	if cfg.MaxCostUSD != 2.5 {
		t.Errorf("expected MaxCostUSD 2.5, got %f", cfg.MaxCostUSD)
	}
	if cfg.Model != "sonnet" {
		t.Errorf("expected Model sonnet, got %q", cfg.Model)
	}
	if cfg.VerifyCmd != "make verify" {
		t.Errorf("expected VerifyCmd 'make verify', got %q", cfg.VerifyCmd)
	}
	// Unspecified fields should retain defaults.
	if cfg.MaxConsecutiveFails != 3 {
		t.Errorf("expected default MaxConsecutiveFails 3, got %d", cfg.MaxConsecutiveFails)
	}
	if cfg.MaxWallClockSec != 3600 {
		t.Errorf("expected default MaxWallClockSec 3600, got %d", cfg.MaxWallClockSec)
	}
	if cfg.PermissionMode != "bypassPermissions" {
		t.Errorf("expected default PermissionMode bypassPermissions, got %q", cfg.PermissionMode)
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	workDir := t.TempDir()
	cfgPath := filepath.Join(workDir, "config.yaml")

	invalid := `
max_iterations: not-a-number
  badly_indented: true
`
	if err := os.WriteFile(cfgPath, []byte(invalid), 0644); err != nil {
		t.Fatalf("setup error: %v", err)
	}

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.MaxIterations != 50 {
		t.Errorf("expected 50, got %d", cfg.MaxIterations)
	}
	if cfg.MaxCostUSD != 5.0 {
		t.Errorf("expected 5.0, got %f", cfg.MaxCostUSD)
	}
	if cfg.MaxConsecutiveFails != 3 {
		t.Errorf("expected 3, got %d", cfg.MaxConsecutiveFails)
	}
	if cfg.MaxWallClockSec != 3600 {
		t.Errorf("expected 3600, got %d", cfg.MaxWallClockSec)
	}
	if cfg.PermissionMode != "bypassPermissions" {
		t.Errorf("expected bypassPermissions, got %q", cfg.PermissionMode)
	}
}