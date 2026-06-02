package executor

import (
	"context"
	"fmt"
	"os/exec"
)

// ClaudeCodeExecutor invokes the `claude` CLI in non-interactive mode.
type ClaudeCodeExecutor struct {
	BinaryPath string // default "claude", overridable for testing
}

func New() *ClaudeCodeExecutor {
	return &ClaudeCodeExecutor{BinaryPath: "claude"}
}

func (e *ClaudeCodeExecutor) Run(
	ctx context.Context,
	req Request,
	sink EventSink,
	iterDir string,
) (*Response, error) {
	_ = e.buildArgs(req)
	_ = ctx
	_ = sink
	_ = iterDir

	// TODO(claude-code):
	// 1. Write req.SystemPromptSuffix to <iterDir>/system-suffix.md and pass
	//    --append-system-prompt @<path>.
	// 2. exec.CommandContext(ctx, e.BinaryPath, args...) with stdin=prompt.
	// 3. Tee stdout to <iterDir>/events.jsonl AND to parseStreamJSON.
	// 4. Tee stderr to <iterDir>/stderr.log.
	// 5. On ctx.Done(): send SIGINT, wait 10s, then SIGKILL.
	// 6. parseStreamJSON returns *Response on `result` event.
	// 7. Wait for child to exit; if no result event seen, return error.
	_ = exec.CommandContext // placeholder
	return nil, fmt.Errorf("not implemented")
}

func (e *ClaudeCodeExecutor) buildArgs(req Request) []string {
	args := []string{
		"--output-format", "stream-json",
		"--verbose",
		"--permission-mode", req.PermissionMode,
		"-p", req.Prompt,
	}

	if req.IsFirstTurn {
		args = append(args, "--session-id", req.SessionID)
	} else {
		args = append(args, "--resume", req.SessionID)
	}

	if req.MaxBudgetUSD > 0 {
		args = append(args, "--max-budget-usd", fmt.Sprintf("%.4f", req.MaxBudgetUSD))
	}
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}

	// --append-system-prompt is added in Run() after writing to disk.

	return args
}
