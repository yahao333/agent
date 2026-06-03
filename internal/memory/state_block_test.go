package memory

import (
	"testing"
)

func TestExtractStateBlock_Normal(t *testing.T) {
	content := "Some text before.\n<!-- ralph:state -->\n" +
		"```json\n" +
		"{\n" +
		`  "iteration_status": "done",` + "\n" +
		`  "summary": "All tasks completed",` + "\n" +
		`  "next_action": "Verify the changes",` + "\n" +
		`  "blockers": []` + "\n" +
		"}\n" +
		"```\n" +
		"<!-- /ralph:state -->\n" +
		"Some text after."

	block, err := ExtractStateBlock(content)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if block.IterationStatus != StatusDone {
		t.Errorf("expected status done, got: %v", block.IterationStatus)
	}
	if block.Summary != "All tasks completed" {
		t.Errorf("expected summary 'All tasks completed', got: %q", block.Summary)
	}
	if block.NextAction != "Verify the changes" {
		t.Errorf("expected next_action 'Verify the changes', got: %q", block.NextAction)
	}
	if len(block.Blockers) != 0 {
		t.Errorf("expected no blockers, got: %v", block.Blockers)
	}
}

func TestExtractStateBlock_MissingBlock(t *testing.T) {
	content := "# Scratchpad\nNo state block here."

	block, err := ExtractStateBlock(content)

	if err == nil {
		t.Fatal("expected an error for missing block")
	}
	extractErr, ok := err.(*ExtractError)
	if !ok {
		t.Fatalf("expected *ExtractError, got: %T", err)
	}
	if extractErr.Reason != "missing" {
		t.Errorf("expected reason 'missing', got: %q", extractErr.Reason)
	}
	if block.IterationStatus != StatusInProgress {
		t.Errorf("expected fallback status in_progress, got: %v", block.IterationStatus)
	}
	if block.Summary != "<missing state block>" {
		t.Errorf("expected fallback summary '<missing state block>', got: %q", block.Summary)
	}
}

func TestExtractStateBlock_DuplicateBlock(t *testing.T) {
	content := "First block.\n<!-- ralph:state -->\n" +
		"```json\n" +
		"{\n" +
		`  "iteration_status": "in_progress",` + "\n" +
		`  "summary": "First"` + "\n" +
		"}\n" +
		"```\n" +
		"<!-- /ralph:state -->\n" +
		"Middle text.\n<!-- ralph:state -->\n" +
		"```json\n" +
		"{\n" +
		`  "iteration_status": "done",` + "\n" +
		`  "summary": "Second",` + "\n" +
		`  "next_action": "Done"` + "\n" +
		"}\n" +
		"```\n" +
		"<!-- /ralph:state -->\n" +
		"Last block."

	block, err := ExtractStateBlock(content)

	if err == nil {
		t.Fatal("expected an error for duplicate block")
	}
	extractErr, ok := err.(*ExtractError)
	if !ok {
		t.Fatalf("expected *ExtractError, got: %T", err)
	}
	if extractErr.Reason != "duplicate" {
		t.Errorf("expected reason 'duplicate', got: %q", extractErr.Reason)
	}
	// Should use the last block.
	if block.Summary != "Second" {
		t.Errorf("expected last block's summary 'Second', got: %q", block.Summary)
	}
	if block.IterationStatus != StatusDone {
		t.Errorf("expected last block's status done, got: %v", block.IterationStatus)
	}
}

func TestExtractStateBlock_MalformedJSON(t *testing.T) {
	content := "<!-- ralph:state -->\n" +
		"```json\n" +
		"{\n" +
		`  "iteration_status": "done",` + "\n" +
		`  "summary": "Missing closing brace` + "\n" +
		"}\n" +
		"```\n" +
		"<!-- /ralph:state -->"

	block, err := ExtractStateBlock(content)

	if err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
	extractErr, ok := err.(*ExtractError)
	if !ok {
		t.Fatalf("expected *ExtractError, got: %T", err)
	}
	if extractErr.Reason != "malformed_json" {
		t.Errorf("expected reason 'malformed_json', got: %q", extractErr.Reason)
	}
	if block.IterationStatus != StatusInProgress {
		t.Errorf("expected fallback status in_progress, got: %v", block.IterationStatus)
	}
	if block.Summary != "<malformed state>" {
		t.Errorf("expected fallback summary '<malformed state>', got: %q", block.Summary)
	}
}

func TestExtractStateBlock_UnknownStatus(t *testing.T) {
	content := "<!-- ralph:state -->\n" +
		"```json\n" +
		"{\n" +
		`  "iteration_status": "unknown_status_value",` + "\n" +
		`  "summary": "Some summary"` + "\n" +
		"}\n" +
		"```\n" +
		"<!-- /ralph:state -->"

	block, err := ExtractStateBlock(content)

	if err == nil {
		t.Fatal("expected an error for unknown status")
	}
	extractErr, ok := err.(*ExtractError)
	if !ok {
		t.Fatalf("expected *ExtractError, got: %T", err)
	}
	if extractErr.Reason != "unknown_status" {
		t.Errorf("expected reason 'unknown_status', got: %q", extractErr.Reason)
	}
	if block.IterationStatus != StatusInProgress {
		t.Errorf("expected fallback status in_progress, got: %v", block.IterationStatus)
	}
}
func TestExtractStateBlock_CompletedAlias(t *testing.T) {
	// "completed" should be treated as an alias for "done".
	content := "<!-- ralph:state -->\n" +
		"```json\n" +
		"{\n" +
		`  "iteration_status": "completed",` + "\n" +
		`  "summary": "Task done",` + "\n" +
		`  "blockers": []` + "\n" +
		"}\n" +
		"```\n" +
		"<!-- /ralph:state -->"

	block, err := ExtractStateBlock(content)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if block.IterationStatus != StatusDone {
		t.Errorf("expected status 'done', got: %v", block.IterationStatus)
	}
	if block.Summary != "Task done" {
		t.Errorf("expected summary 'Task done', got: %q", block.Summary)
	}
}
