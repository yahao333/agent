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
	var configPath string
	cmd := &cobra.Command{
		Use:   "run [goal]",
		Short: "Run Ralph on a goal until completion or limit",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			goal := args[0]
			if err := validateGoal(goal); err != nil {
				return fmt.Errorf("invalid goal: %w", err)
			}
						workDir, _ := os.Getwd()
			cfg, err := config.Load(filepath.Join(workDir, ".ralph", "config.yaml"))
			if err != nil {
				return err
			}
			runID, sessionID, dirs, err := run.New(workDir)
			if err != nil {
				return err
			}
			fmt.Printf("▶ Run %s started (session %s)\n", runID, sessionID)
			fmt.Printf("  goal: %s\n", goal)
			fmt.Printf("  dir:  %s\n", dirs.Root)
			verifier, err := verify.Resolve(cfg.VerifyCmd, workDir)
			if err != nil {
				return err
			}
			exec := executor.New()
			sink := &consoleSink{}
			loop := agent.New(
				agent.Config{
					MaxIterations:       cfg.MaxIterations,
					MaxCostUSD:          cfg.MaxCostUSD,
					MaxConsecutiveFails: cfg.MaxConsecutiveFails,
					MaxWallClockSec:     cfg.MaxWallClockSec,
					PermissionMode:      cfg.PermissionMode,
					Model:               cfg.Model,
				},
				dirs.Root, runID, exec, verifier, sink,
			)
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()
			// Graceful shutdown on Ctrl+C
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				<-sigCh
				fmt.Fprintln(os.Stderr, "\n⏹ interrupt received, shutting down...")
				cancel()
			}()
			return loop.Run(ctx, goal, sessionID)
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "", "path to config.yaml (default: .ralph/config.yaml)")
	return cmd
}

// consoleSink prints assistant text to stdout in real-time.
type consoleSink struct{}
func (c *consoleSink) OnAssistantText(text string)     { fmt.Print(text) }
func (c *consoleSink) OnAssistantThinking(text string) {} // suppressed by default
func (c *consoleSink) OnToolUse(name, input string)    { fmt.Fprintf(os.Stderr, "🔧 %s\n", name) }
func (c *consoleSink) OnSystemInit(model, cwd string, tools []string) {
	fmt.Fprintf(os.Stderr, "✓ model=%s cwd=%s tools=%d\n", model, cwd, len(tools))
}
