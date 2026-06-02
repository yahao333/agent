// Package executor abstracts the LLM-backend invocation.
// v1 only ships ClaudeCodeExecutor; the interface exists to keep the door open.
package executor

import "context"

// Request is one LLM turn from Ralph's perspective.
type Request struct {
	// SessionID is the UUID Ralph generated (or reused) for this Run.
	// First turn passes it via --session-id; later turns via --resume.
	SessionID string

	// IsFirstTurn distinguishes --session-id vs --resume.
	IsFirstTurn bool

	// Prompt is the user message for this turn (passed via -p).
	Prompt string

	// SystemPromptSuffix is appended to Claude Code's default system prompt
	// via --append-system-prompt. Carries the Ralph Protocol (see
	// docs/design/claude-code-integration.md §3).
	SystemPromptSuffix string

	// MaxBudgetUSD is the remaining budget for this Run (forwarded to
	// --max-budget-usd). Executor MUST NOT exceed this.
	MaxBudgetUSD float64

	// PermissionMode: "bypassPermissions" | "acceptEdits" | ...
	PermissionMode string

	// Model: optional, e.g. "sonnet". Empty = Claude Code's default.
	Model string

	// WorkDir is where claude is invoked (typically the user's repo root).
	WorkDir string
}

// Response is the structured result of one LLM turn.
// Populated incrementally as stream-json events arrive; finalized when the
// result event is seen.
type Response struct {
	SessionID         string // confirmed by the init event
	FinalText         string // result.result
	IsError           bool   // result.is_error
	TerminalReason    string // result.terminal_reason (e.g. "completed")
	DurationMS        int64
	CostUSD           float64
	InputTokens       int64
	OutputTokens      int64
	NumTurns          int // Claude Code's internal turn count
	PermissionDenials []string
	// RawEventsPath: where the full stream-json was dumped for forensics.
	RawEventsPath string
}

// Executor runs one LLM turn end-to-end.
//
// Contract:
//   - Streams events to EventSink as they arrive (for live UI).
//   - Persists raw stream-json to <iterDir>/events.jsonl.
//   - Returns when the `result` event is seen OR the process exits.
//   - Honors ctx cancellation: propagates SIGINT to child, waits 10s,
//     then SIGKILL.
//   - Does NOT retry on its own; retry policy lives in agent.Loop.GUARD.
type Executor interface {
	Run(ctx context.Context, req Request, sink EventSink, iterDir string) (*Response, error)
}

// EventSink receives parsed stream-json events for live observation.
// Implementations MUST be non-blocking (drop or buffer).
type EventSink interface {
	OnAssistantText(text string)     // user-visible assistant output
	OnAssistantThinking(text string) // optional, for verbose mode
	OnToolUse(name, input string)    // tool calls (for status line)
	OnSystemInit(model, cwd string, tools []string)
}
