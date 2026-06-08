package agent

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yahao333/ralph/internal/executor"
	"github.com/yahao333/ralph/internal/memory"
)

// mockVerifier is a programmable verifier for testing.
type mockVerifier struct {
	results   []bool
	errors    []error
	calls     int
	outputs   []string
	exitCodes []int
}

func (m *mockVerifier) Run(ctx context.Context) (bool, string, int, error) {
	if m.calls >= len(m.results) {
		m.calls++
		return false, "no more results", 1, nil
	}
	idx := m.calls
	m.calls++
	out := ""
	if idx < len(m.outputs) {
		out = m.outputs[idx]
	}
	code := 0
	if idx < len(m.exitCodes) {
		code = m.exitCodes[idx]
	}
	err := error(nil)
	if idx < len(m.errors) && m.errors[idx] != nil {
		err = m.errors[idx]
	}
	return m.results[idx], out, code, err
}

// makeStateBlock creates a standalone scratchpad with just the state block.
func makeStateBlock(status memory.IterationStatus, summary string) string {
	return "<!-- ralph:state -->\n```json\n{\n  \"iteration_status\": \"" +
		string(status) + "\",\n  \"summary\": \"" + summary + "\"\n}\n```\n<!-- /ralph:state -->"
}

// loopTestHarness sets up a minimal Loop with mock executor and verifier.
type loopTestHarness struct {
	runDir string
	runID  string
	exec   *executor.MockExecutor
	verif  *mockVerifier
	loop   *Loop
}

func newLoopHarness(t *testing.T, cfg Config) *loopTestHarness {
	runDir := t.TempDir()
	runID := "test-run-001"

	require.NoError(t, memory.InitScratchpad(runDir, runID, "test goal"))

	exec := executor.NewMock()
	verif := &mockVerifier{}
	sink := executor.NopSink

	loop := New(cfg, runDir, runID, runDir, exec, verif, sink)
	h := &loopTestHarness{runDir: runDir, runID: runID, exec: exec, verif: verif, loop: loop}
	// By default scratchpad is already set to in_progress by InitScratchpad.
	// Tests that need a different initial state should use h.writeScratchpad(t, ...) or
	// h.updateScratchpadState(t, ...) BEFORE calling h.loop.Run().
	return h
}

// writeScratchpad replaces the entire scratchpad file.
func (h *loopTestHarness) writeScratchpad(t *testing.T, content string) {
	require.NoError(t, os.WriteFile(filepath.Join(h.runDir, "scratchpad.md"), []byte(content), 0644))
}

// updateScratchpadState replaces only the state block within the existing scratchpad.
// This preserves the surrounding template content.
func (h *loopTestHarness) updateScratchpadState(t *testing.T, status memory.IterationStatus, summary string) {
	existing, err := os.ReadFile(filepath.Join(h.runDir, "scratchpad.md"))
	require.NoError(t, err)

	prefix := []byte("<!-- ralph:state -->")
	suffix := []byte("<!-- /ralph:state -->")
	before, rest, found := bytes.Cut(existing, prefix)
	_, after, found2 := bytes.Cut(rest, suffix)
	require.True(t, found && found2, "state block markers not found in scratchpad")

	newBlock := "<!-- ralph:state -->\n```json\n{\n  \"iteration_status\": \"" +
		string(status) + "\",\n  \"summary\": \"" + summary + "\"\n}\n```\n<!-- /ralph:state -->"

	newContent := append(append(before, []byte(newBlock)...), after...)
	require.NoError(t, os.WriteFile(filepath.Join(h.runDir, "scratchpad.md"), newContent, 0644))
}

func mockResponse() *executor.Response {
	return &executor.Response{CostUSD: 0.1}
}

func mockErrorResponse() *executor.Response {
	return &executor.Response{CostUSD: 0, IsError: true}
}

// --- Item 8: mock done → verify (mock) pass → SUCCESS ---
func TestLoop_MockDoneVerifyPass(t *testing.T) {
	h := newLoopHarness(t, Config{MaxIterations: 5})
	h.writeScratchpad(t, makeStateBlock(memory.StatusDone, "All done"))
	h.verif.results = []bool{true}
	h.verif.exitCodes = []int{0}
	h.verif.outputs = []string{""}
	h.exec.AppendResponse(mockResponse())

	// The loop must read "done" in scratchpad. Since AfterEach runs AFTER the
	// executor returns, we must write "done" here (simulating LLM's final state
	// before returning from exec.Run). This way extract() sees "done" on first pass.
	h.exec.AfterEach = func(req executor.Request, resp *executor.Response, err error) {
		h.updateScratchpadState(t, memory.StatusDone, "All done")
	}

	_ = h.loop.Run(context.Background(), "test goal", "sess-001")
	st := h.loop.store.Get()
	t.Logf("fsmState=%q  iterN=%d  execCalls=%d  verifCalls=%d",
		st.FSMState, st.IterationN, len(h.exec.Calls), h.verif.calls)

	assert.Equal(t, "SUCCESS", st.FSMState)
	assert.Equal(t, 1, st.IterationN)
}

// --- Item 9: verify fails twice, third time passes → feedback injection ---
func TestLoop_VerifyFailsTwiceThenPasses(t *testing.T) {
	h := newLoopHarness(t, Config{MaxIterations: 10})
	h.verif.results = []bool{false, false, true}
	h.verif.exitCodes = []int{1, 1, 0}
	h.verif.outputs = []string{"FAIL: test failed", "FAIL: still broken", ""}

	h.exec.AppendResponse(mockResponse())
	h.exec.AppendResponse(mockResponse())
	h.exec.AppendResponse(mockResponse())

	// Before each executor call, update scratchpad to "done".
	// On iter 1: already "done" in scratchpad → VERIFY fails → FEEDBACK stored
	// On iter 2: scratchpad still "done" → VERIFY fails → FEEDBACK stored (overwrites)
	// On iter 3: scratchpad still "done" → VERIFY passes → SUCCESS
	beforeCall := 0
	h.exec.AfterEach = func(req executor.Request, resp *executor.Response, err error) {
		beforeCall++
		// Update scratchpad to "done" before the NEXT extract runs.
		// AfterCall #1 (after iter1 exec) → scratchpad "done" for iter2 extract
		// AfterCall #2 (after iter2 exec) → scratchpad "done" for iter3 extract
		if beforeCall <= 3 {
			h.updateScratchpadState(t, memory.StatusDone, "Done")
		}
	}

	h.writeScratchpad(t, makeStateBlock(memory.StatusDone, "Done iter 1"))

	_ = h.loop.Run(context.Background(), "test goal", "sess-001")
	st := h.loop.store.Get()
	t.Logf("fsmState=%q  iterN=%d  execCalls=%d  verifCalls=%d",
		st.FSMState, st.IterationN, len(h.exec.Calls), h.verif.calls)

	assert.Equal(t, "SUCCESS", st.FSMState)
	assert.Equal(t, 3, st.IterationN)

	require.GreaterOrEqual(t, len(h.exec.Calls), 2)
	assert.Contains(t, h.exec.Calls[1].Prompt, "RALPH FEEDBACK")
	assert.Contains(t, h.exec.Calls[1].Prompt, "FAIL: test failed")
}

// --- Item 10: max_iterations triggers GUARD → FAILURE ---
func TestLoop_MaxIterationsTriggersGuard(t *testing.T) {
	h := newLoopHarness(t, Config{MaxIterations: 3, MaxConsecutiveFails: 0})
	h.verif.results = []bool{false}
	h.verif.exitCodes = []int{1}
	h.verif.outputs = []string{"fail"}

	// 5 in_progress responses, but max is 3.
	for range 5 {
		h.exec.AppendResponse(mockResponse())
	}

	beforeCall := 0
	h.exec.AfterEach = func(req executor.Request, resp *executor.Response, err error) {
		beforeCall++
		// Keep scratchpad as in_progress for each iteration.
		if beforeCall <= 5 {
			h.updateScratchpadState(t, memory.StatusInProgress, "Still working")
		}
	}

	h.writeScratchpad(t, makeStateBlock(memory.StatusInProgress, "Still working"))

	_ = h.loop.Run(context.Background(), "test goal", "sess-001")
	st := h.loop.store.Get()
	t.Logf("fsmState=%q  iterN=%d  execCalls=%d  verifCalls=%d  reason=%q",
		st.FSMState, st.IterationN, len(h.exec.Calls), h.verif.calls, st.FailureReason)

	assert.Equal(t, "FAILURE", st.FSMState)
	assert.Contains(t, st.FailureReason, "max iterations reached")
	assert.Equal(t, 3, st.IterationN)
}

// --- Additional: verify fail → injects feedback for next iteration ---
func TestLoop_VerifyFailInjectsFeedback(t *testing.T) {
	h := newLoopHarness(t, Config{MaxIterations: 5})
	h.verif.results = []bool{false, true}
	h.verif.exitCodes = []int{1, 0}
	h.verif.outputs = []string{"Assertion failed: expected 200, got 404", ""}

	h.exec.AppendResponse(mockResponse())
	h.exec.AppendResponse(mockResponse())

	beforeCall := 0
	h.exec.AfterEach = func(req executor.Request, resp *executor.Response, err error) {
		beforeCall++
		if beforeCall <= 2 {
			h.updateScratchpadState(t, memory.StatusDone, "Done")
		}
	}

	h.writeScratchpad(t, makeStateBlock(memory.StatusDone, "Done"))

	_ = h.loop.Run(context.Background(), "test goal", "sess-001")
	st := h.loop.store.Get()
	t.Logf("fsmState=%q  iterN=%d  execCalls=%d  verifCalls=%d",
		st.FSMState, st.IterationN, len(h.exec.Calls), h.verif.calls)

	assert.Equal(t, "SUCCESS", st.FSMState)
	require.GreaterOrEqual(t, len(h.exec.Calls), 2)
	assert.Contains(t, h.exec.Calls[1].Prompt, "RALPH FEEDBACK")
	assert.Contains(t, h.exec.Calls[1].Prompt, "Assertion failed")
	assert.Equal(t, "", st.LastVerifyOutput)
}

// --- Additional: needs_human → FAILURE ---
func TestLoop_NeedsHumanFails(t *testing.T) {
	h := newLoopHarness(t, Config{MaxIterations: 5})
	h.writeScratchpad(t, makeStateBlock(memory.StatusNeedsHuman, "Need credentials"))
	// Provide 5 responses; after the first iteration transitions to FAILURE,
	// the loop's terminal-state guard at the top of the for-loop exits.
	for range 5 {
		h.exec.AppendResponse(mockResponse())
	}
	// AfterEach: keep scratchpad at needs_human so the terminal-state check
	// catches it on re-entry (if any re-entry occurs).
	h.exec.AfterEach = func(req executor.Request, resp *executor.Response, err error) {
		h.updateScratchpadState(t, memory.StatusNeedsHuman, "Need credentials")
	}

	_ = h.loop.Run(context.Background(), "test goal", "sess-001")
	st := h.loop.store.Get()
	t.Logf("fsmState=%q  execCalls=%d  reason=%q",
		st.FSMState, len(h.exec.Calls), st.FailureReason)

	assert.Equal(t, "FAILURE", st.FSMState)
	assert.Contains(t, st.FailureReason, "human intervention")
}

// --- Additional: context cancel → ABORTED ---
func TestLoop_ContextCancelAborts(t *testing.T) {
	h := newLoopHarness(t, Config{MaxIterations: 5})
	h.writeScratchpad(t, makeStateBlock(memory.StatusInProgress, "working"))
	h.exec.AppendResponse(mockResponse())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_ = h.loop.Run(ctx, "test goal", "sess-001")
	st := h.loop.store.Get()
	t.Logf("fsmState=%q  iterN=%d", st.FSMState, st.IterationN)

	assert.Equal(t, "ABORTED", st.FSMState)
}

// --- Additional: verify fails with iterations exhausted → FAILURE ---
func TestLoop_VerifyFailsExhaustedIterations(t *testing.T) {
	h := newLoopHarness(t, Config{MaxIterations: 3})
	h.verif.results = []bool{false, false, false}
	h.verif.exitCodes = []int{1, 1, 1}
	h.verif.outputs = []string{"fail", "fail", "fail"}

	h.exec.AppendResponse(mockResponse())
	h.exec.AppendResponse(mockResponse())
	h.exec.AppendResponse(mockResponse())

	beforeCall := 0
	h.exec.AfterEach = func(req executor.Request, resp *executor.Response, err error) {
		beforeCall++
		if beforeCall <= 3 {
			h.updateScratchpadState(t, memory.StatusDone, "Done")
		}
	}

	h.writeScratchpad(t, makeStateBlock(memory.StatusDone, "Done"))

	_ = h.loop.Run(context.Background(), "test goal", "sess-001")
	st := h.loop.store.Get()
	t.Logf("fsmState=%q  iterN=%d  reason=%q", st.FSMState, st.IterationN, st.FailureReason)

	assert.Equal(t, "FAILURE", st.FSMState)
	assert.Contains(t, st.FailureReason, "max iterations reached")
}

// --- Additional: max_consecutive_fails → FAILURE ---
// Note: ConsecutiveFails tracks executor errors (IsError=true), not verifier failures.
func TestLoop_MaxConsecutiveFails(t *testing.T) {
	h := newLoopHarness(t, Config{MaxIterations: 10, MaxConsecutiveFails: 2})
	h.verif.results = []bool{true, true, true} // verifier always passes
	h.verif.exitCodes = []int{0, 0, 0}
	h.verif.outputs = []string{"", "", ""}

	// 3 executor errors → after 2nd error (ConsecutiveFails=2) guard fires.
	h.exec.AppendResponse(mockErrorResponse()) // iter1: IsError → consecutiveFails=1
	h.exec.AppendResponse(mockErrorResponse()) // iter2: IsError → consecutiveFails=2 → GUARD fires → FAILURE

	beforeCall := 0
	h.exec.AfterEach = func(req executor.Request, resp *executor.Response, err error) {
		beforeCall++
		if beforeCall <= 2 {
			h.updateScratchpadState(t, memory.StatusInProgress, "Still working")
		}
	}

	h.writeScratchpad(t, makeStateBlock(memory.StatusInProgress, "working"))

	_ = h.loop.Run(context.Background(), "test goal", "sess-001")
	st := h.loop.store.Get()
	t.Logf("fsmState=%q  consecutiveFails=%d  reason=%q",
		st.FSMState, st.ConsecutiveFails, st.FailureReason)

	assert.Equal(t, "FAILURE", st.FSMState)
	assert.Equal(t, 2, st.ConsecutiveFails)
	assert.Contains(t, st.FailureReason, "too many consecutive failures")
}

// --- Additional: max_wall_clock_sec → FAILURE ---
func TestLoop_MaxWallClockSec(t *testing.T) {
	h := newLoopHarness(t, Config{MaxIterations: 10, MaxWallClockSec: 1})
	h.verif.results = []bool{false} // verifier fails (doesn't matter, wall-clock will trigger first)
	h.verif.exitCodes = []int{1}
	h.verif.outputs = []string{"n/a"}

	// Provide 3 responses. The wall-clock check triggers at GUARD after iter 1's EXTRACT.
	// We backdate StartedAt in the second AfterEach (after first think()), so when
	// GUARD runs for iter 1, the clock is already expired.
	h.exec.AppendResponse(mockResponse()) // iter 1
	h.exec.AppendResponse(mockResponse()) // iter 2 (never reached - wall-clock kills it first)
	h.exec.AppendResponse(mockResponse()) // iter 3 (never reached)

	iter := 0
	h.exec.AfterEach = func(req executor.Request, resp *executor.Response, err error) {
		iter++
		// Backdate after first think completes but before its GUARD check.
		if iter == 1 {
			h.loop.store.Update(func(s *RunState) {
				s.StartedAt = time.Now().Add(-2 * time.Second)
			})
		}
		h.updateScratchpadState(t, memory.StatusInProgress, "Still working")
	}

	h.writeScratchpad(t, makeStateBlock(memory.StatusInProgress, "working"))

	_ = h.loop.Run(context.Background(), "test goal", "sess-001")
	st := h.loop.store.Get()
	t.Logf("fsmState=%q  reason=%q", st.FSMState, st.FailureReason)

	assert.Equal(t, "FAILURE", st.FSMState)
	assert.Contains(t, st.FailureReason, "max wall-clock time reached")
}