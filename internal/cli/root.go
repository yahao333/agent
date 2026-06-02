package cli

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/spf13/cobra"
)

const maxGoalLen = 10000

// ErrGoalEmpty is returned when the goal string is empty or whitespace-only.
var ErrGoalEmpty = errors.New("goal must not be empty")

// ErrGoalTooLong is returned when the goal exceeds maxGoalLen characters.
var ErrGoalTooLong = fmt.Errorf("goal must not exceed %d characters", maxGoalLen)

// ErrGoalInvalidChars is returned when the goal contains null bytes or control characters.
var ErrGoalInvalidChars = errors.New("goal contains invalid characters (null bytes or control characters)")

// validateGoal checks that user-supplied goal text is safe to process.
func validateGoal(goal string) error {
	if strings.TrimSpace(goal) == "" {
		return ErrGoalEmpty
	}
	if len(goal) > maxGoalLen {
		return ErrGoalTooLong
	}
	for _, r := range goal {
		if r == 0 || (unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t') {
			return ErrGoalInvalidChars
		}
	}
	return nil
}

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
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintln(cmd.OutOrStdout(), "ralph v0.0.1 (dev)")
		},
	}
}

func newRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run [goal]",
		Short: "Run the agent loop toward a goal",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			goal := args[0]
			if err := validateGoal(goal); err != nil {
				return fmt.Errorf("invalid goal: %w", err)
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "TODO: run agent with goal=%q\n", goal)
			return err
		},
	}
	return cmd
}
