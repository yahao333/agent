package agent

import (
	"context"
	"fmt"

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
	store    *StateStore
	exec     executor.Executor
	verifier verify.Verifier
	sink     executor.EventSink
}

func New(cfg Config, runDir, runID string, exec executor.Executor, verifier verify.Verifier, sink executor.EventSink) *Loop {
	return &Loop{
		cfg:      cfg,
		runDir:   runDir,
		runID:    runID,
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

		// --- THINK ---
		if err := l.transitionTo("THINK", ""); err != nil {
			return err
		}
		resp, err := l.think(ctx, st)
		if err != nil {
			// TODO(claude-code): implement retry per claude-code-integration.md §4.3
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
			// --- VERIFY ---
			if err := l.transitionTo("VERIFY", ""); err != nil {
				return err
			}
			ok, output, exitCode, err := l.verifier.Run(ctx)
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
//
//nolint:revive // _st is reserved for future prompt injection
func (l *Loop) think(_ context.Context, _ RunState) (*executor.Response, error) {
	// TODO(claude-code): implement prompt construction:
	// - If st.IterationN == 0: prompt = goal + "\n\nRead .ralph/runs/<id>/scratchpad.md and begin."
	// - Else: prompt = "Continue."
	// - If st.LastVerifyOutput != "": prepend "[RALPH FEEDBACK · Iter <N>]\n<output>"
	//   then CLEAR LastVerifyOutput after this turn.
	// See agent-loop.md §5.
	return nil, fmt.Errorf("not implemented")
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
		return fmt.Sprintf("max cost reached ($%.4f)", st.TotalCostUSD)
	}
	if l.cfg.MaxConsecutiveFails > 0 && st.ConsecutiveFails >= l.cfg.MaxConsecutiveFails {
		return fmt.Sprintf("too many consecutive failures (%d)", st.ConsecutiveFails)
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
