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
