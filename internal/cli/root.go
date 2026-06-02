package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewRootCmd 构造根命令。
// 注意：用 New 函数而非全局变量，方便测试。
func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ralph",
		Short: "An AI agent that orchestrates Claude Code / other LLM CLIs",
		Long: `Ralph is a CLI agent inspired by the "Ralph Wiggum technique" —
a simple loop that drives Claude Code (or other LLM CLIs) toward a goal.`,
		SilenceUsage:  true, // 出错时不打印 usage（更干净）
		SilenceErrors: true, // 错误由 main 统一处理，避免重复输出
	}

	cmd.AddCommand(newVersionCmd())
	cmd.AddCommand(newRunCmd())

	return cmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "ralph v0.0.1 (dev)")
			return err
		},
	}
}

func newRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run [goal]",
		Short: "Run the agent loop toward a goal",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			goal := args[0]
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "TODO: run agent with goal=%q\n", goal)
			return err
		},
	}
}
