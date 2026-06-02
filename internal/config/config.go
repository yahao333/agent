// Package config loads .ralph/config.yaml.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	// Executor settings
	Model          string `yaml:"model,omitempty"`           // e.g. "sonnet"
	PermissionMode string `yaml:"permission_mode,omitempty"` // default: "bypassPermissions"

	// Safety limits
	MaxIterations       int     `yaml:"max_iterations,omitempty"`        // default 50
	MaxCostUSD          float64 `yaml:"max_cost_usd,omitempty"`          // default 5.0
	MaxConsecutiveFails int     `yaml:"max_consecutive_fails,omitempty"` // default 3
	MaxWallClockSec     int     `yaml:"max_wall_clock_sec,omitempty"`    // default 3600

	// Verification
	VerifyCmd string `yaml:"verify_cmd,omitempty"` // empty → B2 fallback

	// Workspace
	AddDirs []string `yaml:"add_dirs,omitempty"` // passed to --add-dir
}

func Default() Config {
	return Config{
		PermissionMode:      "bypassPermissions",
		MaxIterations:       50,
		MaxCostUSD:          5.0,
		MaxConsecutiveFails: 3,
		MaxWallClockSec:     3600,
	}
}

// Load reads .ralph/config.yaml (if exists) merged on top of defaults.
func Load(path string) (Config, error) {
	cfg := Default()

	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil // no config = pure defaults
	}
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}

	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}
