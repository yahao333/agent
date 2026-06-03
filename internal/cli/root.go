package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"unicode"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/yahao333/ralph/internal/agent"
	"github.com/yahao333/ralph/internal/config"
	"github.com/yahao333/ralph/internal/executor"
	"github.com/yahao333/ralph/internal/run"
	"github.com/yahao333/ralph/internal/verify"
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
	cmd.AddCommand(newInitCmd())

	return cmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "Ralph v0.0.1 (dev)\n")
			// 显示 claude 版本（如果可用）
			if out, err := exec.Command("claude", "--version").Output(); err == nil {
				lines := strings.Split(strings.TrimSpace(string(out)), "\n")
				if len(lines) > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "Claude Code %s\n", strings.TrimSpace(lines[0]))
				}
			}
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
				dirs.Root, runID, workDir, exec, verifier, sink,
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

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create .ralph/ with a starter config",
		RunE: func(cmd *cobra.Command, _ []string) error {
			workDir, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}
			ralphDir := filepath.Join(workDir, ".ralph")
			configPath := filepath.Join(ralphDir, "config.yaml")

			// 如果已经存在，报错而不是覆盖
			if _, err := os.Stat(configPath); err == nil {
				return fmt.Errorf(".ralph/config.yaml already exists, skipping")
			}

			// 创建 .ralph 目录
			if err := os.MkdirAll(ralphDir, 0755); err != nil {
				return fmt.Errorf("create .ralph dir: %w", err)
			}

			// 写入默认配置文件
			if err := os.WriteFile(configPath, []byte(defaultConfigYAML), 0644); err != nil {
				return fmt.Errorf("write config: %w", err)
			}

			color.New(color.FgGreen).Fprintf(cmd.OutOrStdout(), "✓ created .ralph/config.yaml — edit verify_cmd and run `ralph run <goal>\n")
			return nil
		},
	}
}

// defaultConfigYAML 是 ralph init 生成的默认配置内容
const defaultConfigYAML = `# Ralph configuration
# ===================
# All fields are optional — Ralph ships with sane defaults.
#
# Docs: https://github.com/yahao333/ralph/blob/main/README.md

# ─────────────────────────────────────────────────────────────
# 1. Verification — how Ralph knows the task is "done"
# ─────────────────────────────────────────────────────────────

# The command Ralph runs after the LLM reports ` + "`" + `iteration_status: done` + "`" + `.
# Exit code 0 = task complete (Ralph stops with SUCCESS).
# Non-zero = failure (output is fed back to the LLM as [RALPH FEEDBACK]).
#
# ⚠️  Keep it fast and narrow. Ralph runs this after EVERY iteration,
# and a slow verify_cmd is the #1 cause of slow收敛.
#
# ✅ GOOD: go test ./pkg/foo/... -run TestBar
# ✅ GOOD: pytest tests/unit/test_parser.py
# ✅ GOOD: ruff check . && mypy src/
# ❌ BAD: make ci          # 10 minutes full CI, only 6 rounds per hour
# ❌ BAD: go test ./...    # too slow
#
# Tip: If you only have slow full-suite tests, use the narrowest subset
# to get Ralph working first, then run full CI in final human review.
verify_cmd: "make test"

# ─────────────────────────────────────────────────────────────
# 2. Safety limits
# ─────────────────────────────────────────────────────────────

# Hard cap on iterations (one iteration = one ` + "`" + `claude -p` + "`" + ` call).
max_iterations: 50

# Hard cap on API spend (USD). Forwarded to ` + "`" + `claude --max-budget-usd` + "`" + `.
max_cost_usd: 5.00

# Abort after this many consecutive LLM errors.
max_consecutive_fails: 3

# Hard cap on wall-clock time (seconds).
max_wall_clock_sec: 3600

# ─────────────────────────────────────────────────────────────
# 3. Claude Code settings
# ─────────────────────────────────────────────────────────────

# Which model to use (empty = Claude Code default).
# model: "sonnet"

# Permission mode. Options:
#   bypassPermissions  — DEFAULT. No prompts; Claude can edit/run anything.
#   acceptEdits        — Auto-accept edits but prompt on shell commands.
#   default            — Prompt on everything (⚠️ will hang Ralph).
permission_mode: "bypassPermissions"
`

// consoleSink prints real-time events with ANSI color and progress bar.
type consoleSink struct{}

var stateColor = map[string]func(a ...interface{}) string{
	"INIT":    color.New(color.FgWhite).SprintFunc(),
	"THINK":   color.New(color.FgBlue).SprintFunc(),
	"EXTRACT": color.New(color.FgYellow).SprintFunc(),
	"VERIFY":  color.New(color.FgCyan).SprintFunc(),
	"GUARD":   color.New(color.FgMagenta).SprintFunc(),
	"SUCCESS": color.New(color.FgGreen, color.Bold).SprintFunc(),
	"FAILURE": color.New(color.FgRed, color.Bold).SprintFunc(),
	"ABORTED": color.New(color.FgRed).SprintFunc(),
}

func (c *consoleSink) OnAssistantText(text string) {
	fmt.Print(text)
}

func (c *consoleSink) OnAssistantThinking(text string) {} // suppressed by default

func (c *consoleSink) OnToolUse(name, input string) {
	color.New(color.FgYellow).Fprintf(os.Stderr, "🔧 %s\n", name)
}

func (c *consoleSink) OnSystemInit(model, cwd string, tools []string) {
	color.New(color.FgGreen).Fprintf(os.Stderr, "✓ model=%s cwd=%s tools=%d\n", model, cwd, len(tools))
}

func (c *consoleSink) OnStateTransition(prevState, nextState string, iterationN int, reason string) {
	col := stateColor[nextState]
	if col == nil {
		col = color.New(color.FgWhite).SprintFunc()
	}
	var extra string
	if nextState == "FAILURE" || nextState == "ABORTED" {
		extra = fmt.Sprintf(" — %s", reason)
	}
	var iterPrefix string
	if nextState == "THINK" && iterationN > 0 {
		iterPrefix = fmt.Sprintf("[Iter %d] ", iterationN)
	}
	fmt.Fprintf(os.Stderr, "\n%s→ %s%s\n", iterPrefix, col(nextState), extra)
}