// Package verify runs the user's verify_cmd to confirm task completion.
package verify

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Verifier interface {
	// Run executes the verifier.
	// Returns: (pass, output, exitCode, error).
	// `error` is only for internal failures (cmd not found etc), NOT for
	// the verifier reporting failure — that's `pass=false`.
	Run(ctx context.Context) (bool, string, int, error)
}

type ShellVerifier struct {
	Cmd     string // e.g. "make test"
	WorkDir string
}

func (v *ShellVerifier) Run(ctx context.Context) (bool, string, int, error) {
	//lint:ignore G204 // v.Cmd is validated at config load time; shell injection risk is acceptable for local dev tool
	cmd := exec.CommandContext(ctx, "sh", "-c", v.Cmd)
	cmd.Dir = v.WorkDir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()
	output := buf.String()

	if err == nil {
		return true, output, 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, output, exitErr.ExitCode(), nil
	}
	// Real error (e.g. shell not found)
	return false, output, -1, err
}

// Resolve implements the B2 fallback ladder:
//  1. If userCmd != "": use it
//  2. Else if Makefile exists with a `test:` target: use "make test"
//  3. Else: return error (user must configure)
func Resolve(userCmd, workDir string) (Verifier, error) {
	if userCmd != "" {
		return &ShellVerifier{Cmd: userCmd, WorkDir: workDir}, nil
	}

	mfPath := filepath.Join(workDir, "Makefile")
	if hasTestTarget(mfPath) {
		return &ShellVerifier{Cmd: "make test", WorkDir: workDir}, nil
	}

	return nil, fmt.Errorf(
		"no verify_cmd configured and no Makefile with `test:` target found.\n" +
			"Please add to .ralph/config.yaml:\n  verify_cmd: \"<your command>\"")
}

func hasTestTarget(makefilePath string) bool {
	b, err := os.ReadFile(makefilePath)
	if err != nil {
		return false
	}
	// Naive but sufficient: look for a line starting with "test:" or "test :"
	for _, line := range strings.Split(string(b), "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, "test:") || strings.HasPrefix(trimmed, "test ") {
			return true
		}
	}
	return false
}
