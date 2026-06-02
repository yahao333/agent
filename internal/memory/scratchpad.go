package memory

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
	"time"
)

//go:embed scratchpad_template.md
var scratchpadTemplate string

// InitScratchpad creates the initial scratchpad.md for a Run.
// Called once during INIT state.
func InitScratchpad(runDir, runID, goal string) error {
	tmpl, err := template.New("sp").Parse(scratchpadTemplate)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	path := filepath.Join(runDir, "scratchpad.md")
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create scratchpad: %w", err)
	}
	defer f.Close()

	return tmpl.Execute(f, map[string]string{
		"RunID":   runID,
		"Goal":    goal,
		"Started": time.Now().UTC().Format(time.RFC3339),
	})
}

// ReadScratchpad reads the full scratchpad content. Pair with ExtractStateBlock.
func ReadScratchpad(runDir string) (string, error) {
	path := filepath.Join(runDir, "scratchpad.md")
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read scratchpad: %w", err)
	}
	return string(b), nil
}
