package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/yahao333/ralph/internal/executor"
	"github.com/yahao333/ralph/internal/memory"
	"github.com/yahao333/ralph/internal/verify"
)

// Config is the user-facing knobs (from config.yaml + flags).
type Config struct {
	MaxIterations       int     // hard cap
	MaxCostUSD          float64 // forwarded to claude --max-budget-usd
	MaxConsecutiveFails int     // GUARD aborts after this many in a row
	MaxWallClockSec     int     // GUARD aborts after this many seconds
	PermissionMode      string  // default "bypassPermissions"
	Model               string  // optional
	VerifyCmd           string  // user-set OR resolved via B2 fallback
}

// Loop is the orchestrator.
type Loop struct {
	cfg      Config
	runDir   string
	runID    string
	workDir  string // project root where claude operates
	store    *StateStore
	exec     executor.Executor
	verifier verify.Verifier
	sink     executor.EventSink
}

func New(cfg Config, runDir, runID, workDir string, exec executor.Executor, verifier verify.Verifier, sink executor.EventSink) *Loop {
	return &Loop{
		cfg:      cfg,
		runDir:   runDir,
		runID:    runID,
		workDir:  workDir,
		store:    NewStateStore(runDir),
		exec:     exec,
		verifier: verifier,
		sink:     sink,
	}
}

// Run executes the full state machine until a terminal state.
// Returns nil on SUCCESS, error on FAILURE/ABORTED.
//
//go:generate go run golang.org/x/tools/cmd/stringer@latest -type=FSMState
//nolint:gocyclo // scaffold phase, complexity ~21, will refactor when loop is complete
func (l *Loop) Run(ctx context.Context, goal string, sessionID string) error {
	// === INIT ===
	if err := l.store.Init(l.runID, sessionID, goal); err != nil {
		return fmt.Errorf("init state: %w", err)
	}
	if err := memory.InitScratchpad(l.runDir, l.runID, goal); err != nil {
		return fmt.Errorf("init scratchpad: %w", err)
	}

	// === MAIN LOOP ===
	for {
		select {
		case <-ctx.Done():
			return l.transitionTo("ABORTED", "context canceled")
		default:
		}

		st := l.store.Get()
		// Terminal states: loop should not re-enter.
		if st.FSMState == "SUCCESS" || st.FSMState == "FAILURE" || st.FSMState == "ABORTED" {
			return nil
		}

		// --- THINK ---
		if err := l.transitionTo("THINK", ""); err != nil {
			return err
		}
		resp, err := l.think(ctx, st)
		if err != nil {
			return l.transitionTo("FAILURE", fmt.Sprintf("think failed: %v", err))
		}

		// --- EXTRACT ---
		if err := l.transitionTo("EXTRACT", ""); err != nil {
			return err
		}
		block, extractErr := l.extract()
		if extractErr != nil {
			if err := l.store.Update(func(s *RunState) {
				s.LastExtractError = extractErr.Error()
			}); err != nil {
				return fmt.Errorf("update state after extract error: %w", err)
			}
		}

		if err := l.store.Update(func(s *RunState) {
			s.TotalCostUSD += resp.CostUSD
			s.TotalDurMS += resp.DurationMS
			s.IterationN++
			if resp.IsError {
				s.ConsecutiveFails++
			} else {
				s.ConsecutiveFails = 0
			}
		}); err != nil {
			return fmt.Errorf("update state after think: %w", err)
		}

		// --- Branch on status ---
		switch block.IterationStatus {
		case memory.StatusDone:
			slog.Info("EXTRACT → done, entering VERIFY", "iterN", st.IterationN)
			// --- VERIFY ---
			if err := l.transitionTo("VERIFY", ""); err != nil {
				return err
			}
			ok, output, exitCode, err := l.verifier.Run(ctx)
			slog.Info("VERIFY completed", "ok", ok, "exitCode", exitCode, "err", err)
			if err != nil {
				return l.transitionTo("FAILURE", fmt.Sprintf("verify error: %v", err))
			}
			if ok {
				return l.transitionTo("SUCCESS", "")
			}
			// Inject feedback for next iteration.
			if err := l.store.Update(func(s *RunState) {
				s.LastVerifyOutput = output
				s.LastVerifyExit = exitCode
			}); err != nil {
				return fmt.Errorf("update state after verify: %w", err)
			}
			// fall through to GUARD then back to THINK

		case memory.StatusNeedsHuman:
			return l.transitionTo("FAILURE", "LLM requested human intervention: "+block.Summary)

		case memory.StatusBlocked, memory.StatusInProgress:
			// keep going
		}

		// --- GUARD ---
		if err := l.transitionTo("GUARD", ""); err != nil {
			return err
		}
		if reason := l.guardCheck(); reason != "" {
			return l.transitionTo("FAILURE", reason)
		}
	}
}

// think runs one executor turn. See claude-code-integration.md.
func (l *Loop) think(ctx context.Context, st RunState) (*executor.Response, error) {
	st = l.store.Get() // fresh snapshot

	prompt, err := l.buildPrompt(st)
	if err != nil {
		return nil, fmt.Errorf("build prompt: %w", err)
	}

	iterDir := filepath.Join(l.runDir, "iterations", fmt.Sprintf("%03d", st.IterationN+1))
	if err := os.MkdirAll(iterDir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir iter dir: %w", err)
	}

	systemSuffix, _ := os.ReadFile(filepath.Join(l.runDir, "system-prompt-suffix.md"))

	req := executor.Request{
		SessionID:          st.SessionID,
		IsFirstTurn:        st.IterationN == 0,
		Prompt:             prompt,
		SystemPromptSuffix: string(systemSuffix),
		MaxBudgetUSD:       l.cfg.MaxCostUSD,
		PermissionMode:     l.cfg.PermissionMode,
		Model:              l.cfg.Model,
		WorkDir:            l.workDir,
	}

	return l.exec.Run(ctx, req, l.sink, iterDir)
}

// buildPrompt constructs the prompt for this iteration.
// See agent-loop.md §5.
func (l *Loop) buildPrompt(st RunState) (string, error) {
	if st.IterationN == 0 {
		return st.Goal + "\n\nRead .ralph/runs/" + st.RunID + "/scratchpad.md and begin.", nil
	}

	var sb strings.Builder
	if st.LastVerifyOutput != "" {
		sb.WriteString(fmt.Sprintf("[RALPH FEEDBACK · Iter %d]\n", st.IterationN))
		sb.WriteString("You reported task complete, but external verification failed:\n")
		out := st.LastVerifyOutput
		if len(out) > 2000 {
			out = out[len(out)-2000:]
		}
		sb.WriteString(out)
		sb.WriteString("\n\nPlease analyze the failure and continue fixing.\n")
		_ = l.store.Update(func(s *RunState) {
			s.LastVerifyOutput = ""
		})
	}

	sb.WriteString("Continue.")
	return sb.String(), nil
}

func (l *Loop) extract() (*memory.StateBlock, error) {
	content, err := memory.ReadScratchpad(l.runDir)
	if err != nil {
		return nil, err
	}
	return memory.ExtractStateBlock(content)
}

// guardCheck returns "" if OK, or a failure reason string.
func (l *Loop) guardCheck() string {
	st := l.store.Get()
	if l.cfg.MaxIterations > 0 && st.IterationN >= l.cfg.MaxIterations {
		return fmt.Sprintf("max iterations reached (%d)", l.cfg.MaxIterations)
	}
	if l.cfg.MaxCostUSD > 0 && st.TotalCostUSD >= l.cfg.MaxCostUSD {
		return fmt.Sprintf("max cost reached ($%.4f)", l.cfg.MaxCostUSD)
	}
	if l.cfg.MaxConsecutiveFails > 0 && st.ConsecutiveFails >= l.cfg.MaxConsecutiveFails {
		return fmt.Sprintf("too many consecutive failures (%d)", l.cfg.MaxConsecutiveFails)
	}
	// TODO: wall-clock check via StartedAt
	return ""
}

func (l *Loop) transitionTo(state, reason string) error {
	return l.store.Update(func(s *RunState) {
		s.FSMState = state
		if state == "FAILURE" || state == "ABORTED" {
			s.FailureReason = reason
		}
	})
}