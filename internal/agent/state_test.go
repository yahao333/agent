package agent

import (
	"path/filepath"
	"sync"
	"testing"
)

func TestStateStore_InitAndGet(t *testing.T) {
	runDir := t.TempDir()
	store := NewStateStore(runDir)

	runID := "test-run-001"
	sessionID := "session-abc"
	goal := "Test the state store"

	err := store.Init(runID, sessionID, goal)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	st := store.Get()
	if st.RunID != runID {
		t.Errorf("expected RunID %q, got %q", runID, st.RunID)
	}
	if st.SessionID != sessionID {
		t.Errorf("expected SessionID %q, got %q", sessionID, st.SessionID)
	}
	if st.Goal != goal {
		t.Errorf("expected Goal %q, got %q", goal, st.Goal)
	}
	if st.FSMState != "INIT" {
		t.Errorf("expected FSMState INIT, got %q", st.FSMState)
	}
	if st.IterationN != 0 {
		t.Errorf("expected IterationN 0, got %d", st.IterationN)
	}
}

func TestStateStore_Update(t *testing.T) {
	runDir := t.TempDir()
	store := NewStateStore(runDir)

	runID := "test-run-002"
	err := store.Init(runID, "sess", "goal")
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	err = store.Update(func(s *RunState) {
		s.IterationN = 5
		s.FSMState = "THINK"
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	st := store.Get()
	if st.IterationN != 5 {
		t.Errorf("expected IterationN 5, got %d", st.IterationN)
	}
	if st.FSMState != "THINK" {
		t.Errorf("expected FSMState THINK, got %q", st.FSMState)
	}
}

func TestStateStore_UpdateAtomic(t *testing.T) {
	runDir := t.TempDir()
	store := NewStateStore(runDir)

	err := store.Init("run", "sess", "goal")
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	const nGoroutines = 20
	var wg sync.WaitGroup
	wg.Add(nGoroutines)

	for i := 0; i < nGoroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			_ = store.Update(func(s *RunState) {
				s.IterationN = i // each goroutine writes its own index
			})
		}()
	}
	wg.Wait()

	// After concurrent updates, the final value should be one of [0..nGoroutines-1].
	// We just verify no data race (race detector passes) and the value is sane.
	st := store.Get()
	if st.IterationN < 0 || st.IterationN >= nGoroutines {
		t.Errorf("IterationN out of range: %d", st.IterationN)
	}
}

func TestStateStore_LoadRoundTrip(t *testing.T) {
	runDir := t.TempDir()
	store := NewStateStore(runDir)

	runID := "test-run-003"
	sessionID := "session-xyz"
	err := store.Init(runID, sessionID, "Test round-trip")
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Update some fields.
	err = store.Update(func(s *RunState) {
		s.IterationN = 3
		s.TotalCostUSD = 1.5
		s.ConsecutiveFails = 2
		s.FSMState = "VERIFY"
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Create a new store and Load from the same path.
	store2 := &StateStore{path: store.path}
	err = store2.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	st2 := store2.Get()
	if st2.RunID != runID {
		t.Errorf("expected RunID %q, got %q", runID, st2.RunID)
	}
	if st2.SessionID != sessionID {
		t.Errorf("expected SessionID %q, got %q", sessionID, st2.SessionID)
	}
	if st2.IterationN != 3 {
		t.Errorf("expected IterationN 3, got %d", st2.IterationN)
	}
	if st2.TotalCostUSD != 1.5 {
		t.Errorf("expected TotalCostUSD 1.5, got %f", st2.TotalCostUSD)
	}
	if st2.ConsecutiveFails != 2 {
		t.Errorf("expected ConsecutiveFails 2, got %d", st2.ConsecutiveFails)
	}
	if st2.FSMState != "VERIFY" {
		t.Errorf("expected FSMState VERIFY, got %q", st2.FSMState)
	}
}

func TestStateStore_LoadNonExistent(t *testing.T) {
	store := NewStateStore(filepath.Join(t.TempDir(), "nonexistent.json"))
	err := store.Load()
	if err == nil {
		t.Error("expected error loading nonexistent file")
	}
}