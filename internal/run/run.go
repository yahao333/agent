// Package run manages .ralph/runs/<run_id>/ directory layout.
package run

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// Dirs holds the canonical paths for a single Run.
type Dirs struct {
	Root       string // .ralph/runs/<run_id>
	Iterations string // .ralph/runs/<run_id>/iterations
	Verify     string // .ralph/runs/<run_id>/verify
}

// New creates a fresh Run directory.
// run_id format: <timestamp>-<short-uuid>, e.g. 20250115T103000Z-a1b2c3d4
// session_id: full UUID (separate, used for claude --session-id)
func New(workDir string) (runID, sessionID string, dirs Dirs, err error) {
	sessionID = uuid.New().String()
	runID = fmt.Sprintf("%s-%s",
		time.Now().UTC().Format("20060102T150405Z"),
		sessionID[:8],
	)

	dirs = Dirs{
		Root:       filepath.Join(workDir, ".ralph", "runs", runID),
		Iterations: filepath.Join(workDir, ".ralph", "runs", runID, "iterations"),
		Verify:     filepath.Join(workDir, ".ralph", "runs", runID, "verify"),
	}

	for _, d := range []string{dirs.Root, dirs.Iterations, dirs.Verify} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return "", "", Dirs{}, fmt.Errorf("mkdir %s: %w", d, err)
		}
	}
	return
}

// IterDir returns the per-iteration directory: <root>/iterations/001/
func (d Dirs) IterDir(n int) string {
	return filepath.Join(d.Iterations, fmt.Sprintf("%03d", n))
}
