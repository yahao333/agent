package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVersionCmd(t *testing.T) {
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"version"})

	err := root.Execute()
	require.NoError(t, err)
	assert.Contains(t, out.String(), "ralph")
}

func TestRunCmd_RequiresGoal(t *testing.T) {
	root := NewRootCmd()
	root.SetArgs([]string{"run"})
	// 注意：不显式 SetOut，避免污染 stderr 输出
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})

	err := root.Execute()
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
