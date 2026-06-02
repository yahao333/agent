// Package agent contains the Ralph main loop and its state machine.
package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// RunState is Ralph-owned, persisted to state.json after EVERY state transition.
// See docs/design/agent-loop.md §3.2.
type RunState struct {
	RunID            string    `json:"run_id"`
	SessionID        string    `json:"session_id"`
	Goal             string    `json:"goal"`
	StartedAt        time.Time `json:"started_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	FSMState         string    `json:"fsm_state"` // INIT|THINK|EXTRACT|GUARD|VERIFY|SUCCESS|FAILURE|ABORTED
	IterationN       int       `json:"iteration_n"`
	TotalCostUSD     float64   `json:"total_cost_usd"`
	TotalDurMS       int64     `json:"total_duration_ms"`
	ConsecutiveFails int       `json:"consecutive_fails"`

	// Last verify output, injected into next THINK as [RALPH FEEDBACK].
	LastVerifyOutput string `json:"last_verify_output,omitempty"`
	LastVerifyExit   int    `json:"last_verify_exit,omitempty"`

	// Last extract error, for debugging.
	LastExtractError string `json:"last_extract_error,omitempty"`

	// Terminal state info.
	FailureReason string `json:"failure_reason,omitempty"`
}

// StateStore is goroutine-safe state.json reader/writer.
type StateStore struct {
	mu   sync.Mutex
	path string
	st   *RunState
}

func NewStateStore(runDir string) *StateStore {
	return &StateStore{path: filepath.Join(runDir, "state.json")}
}

// Init creates state.json with the initial state.
func (s *StateStore) Init(runID, sessionID, goal string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.st = &RunState{
		RunID:     runID,
		SessionID: sessionID,
		Goal:      goal,
		StartedAt: time.Now().UTC(),
		FSMState:  "INIT",
	}
	return s.flushLocked()
}

// Get returns a snapshot.
func (s *StateStore) Get() RunState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return *s.st
}

// Update applies a mutation under the lock and flushes.
func (s *StateStore) Update(fn func(*RunState)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(s.st)
	s.st.UpdatedAt = time.Now().UTC()
	return s.flushLocked()
}

func (s *StateStore) flushLocked() error {
	tmp := s.path + ".tmp"
	b, err := json.MarshalIndent(s.st, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	// Atomic rename — crash-safe.
	return os.Rename(tmp, s.path)
}

// Load reads state.json (for `ralph resume`, v2).
func (s *StateStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(s.path)
	if err != nil {
		return fmt.Errorf("read state: %w", err)
	}
	s.st = &RunState{}
	return json.Unmarshal(b, s.st)
}
