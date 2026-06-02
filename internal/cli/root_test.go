package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// executeCmd is a test helper that creates a fresh RootCmd, runs it with the
// given args, and returns captured stdout + any error.  Stderr is silenced.
func executeCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()

	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(args)

	err := root.Execute()
	return out.String(), err
}

func TestVersionCmd(t *testing.T) {
	out, err := executeCmd(t, "version")
	require.NoError(t, err)
	assert.Contains(t, out, "ralph")
}

func TestRunCmd_RequiresGoal(t *testing.T) {
	_, err := executeCmd(t, "run")
	require.Error(t, err, "run 命令在缺少 goal 参数时应该报错")
}

func TestRunCmd_WithGoal(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"single word", []string{"run", "refactor"}, `TODO: run agent with goal="refactor"`},
		{"quoted phrase", []string{"run", "fix the bug in parser"}, `TODO: run agent with goal="fix the bug in parser"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := NewRootCmd()
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetArgs(tt.args)

			err := root.Execute()
			require.NoError(t, err)
			assert.Contains(t, out.String(), tt.want)
		})
	}
}

func TestRootCmd_NoSubcommand(t *testing.T) {
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{})

	err := root.Execute()
	require.NoError(t, err, "root command without subcommand should show help, not error")
	assert.Contains(t, out.String(), "ralph")
}

func TestRootCmd_UnknownSubcommand(t *testing.T) {
	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"nonexistent"})

	err := root.Execute()
	require.Error(t, err, "unknown subcommand should return an error")
}

func TestVersionCmd_OutputFormat(t *testing.T) {
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"version"})

	err := root.Execute()
	require.NoError(t, err)
	assert.Contains(t, out.String(), "ralph v")
	assert.Contains(t, out.String(), "\n")
}

func TestValidateGoal(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		{"valid simple", "fix the bug", nil},
		{"valid with newlines", "step 1\nstep 2\nstep 3", nil},
		{"valid with tabs", "goal\twith\ttabs", nil},
		{"empty", "", ErrGoalEmpty},
		{"whitespace only", "   \t\n  ", ErrGoalEmpty},
		{"too long", strings.Repeat("a", maxGoalLen+1), ErrGoalTooLong},
		{"exactly max length", strings.Repeat("a", maxGoalLen), nil},
		{"null byte", "goal\x00injected", ErrGoalInvalidChars},
		{"control char BEL", "goal\x07text", ErrGoalInvalidChars},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGoal(tt.input)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestRunCmd_RejectsInvalidGoal(t *testing.T) {
	root := NewRootCmd()
	root.SetArgs([]string{"run", "   "})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})

	err := root.Execute()
	require.Error(t, err, "run should reject whitespace-only goal")
}
