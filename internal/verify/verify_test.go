package verify

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestResolve_UserSetCmd(t *testing.T) {
	workDir := t.TempDir()

	v, err := Resolve("make test", workDir)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if v == nil {
		t.Fatal("expected non-nil verifier")
	}

	// Create a Makefile that would conflict if we fell back to it.
	// With user cmd set, it should be used instead.
	if err := os.WriteFile(filepath.Join(workDir, "Makefile"), []byte("default:\n\ttest:\n\t\techo OK"), 0644); err != nil {
		t.Fatalf("setup error: %v", err)
	}

	// Still should use the user-set command.
	v2, err := Resolve("make test", workDir)
	if err != nil {
		t.Fatalf("expected no error when user cmd set, got: %v", err)
	}
	_ = v2 // just ensure no error
}

func TestResolve_NoUserCmd_MakefileWithTestTarget(t *testing.T) {
	workDir := t.TempDir()

	mf := `test:
	echo OK
`
	if err := os.WriteFile(filepath.Join(workDir, "Makefile"), []byte(mf), 0644); err != nil {
		t.Fatalf("setup error: %v", err)
	}

	v, err := Resolve("", workDir)
	if err != nil {
		t.Fatalf("expected no error when Makefile has test target, got: %v", err)
	}
	if v == nil {
		t.Fatal("expected non-nil verifier")
	}

	// Verify it actually runs.
	pass, output, code, err := v.Run(context.Background())
	if err != nil {
		t.Fatalf("verifier run error: %v", err)
	}
	if !pass {
		t.Errorf("expected pass, got pass=false, code=%d, output=%q", code, output)
	}
}

func TestResolve_NoUserCmd_NoMakefile(t *testing.T) {
	workDir := t.TempDir()

	_, err := Resolve("", workDir)
	if err == nil {
		t.Fatal("expected error when no verify_cmd and no Makefile")
	}
}

func TestResolve_NoUserCmd_MakefileWithoutTestTarget(t *testing.T) {
	workDir := t.TempDir()

	mf := `default:
	echo OK
all: default
`
	if err := os.WriteFile(filepath.Join(workDir, "Makefile"), []byte(mf), 0644); err != nil {
		t.Fatalf("setup error: %v", err)
	}

	_, err := Resolve("", workDir)
	if err == nil {
		t.Fatal("expected error when Makefile has no test target")
	}
}