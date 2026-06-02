package executor

import "encoding/json"

// Below structs mirror the stream-json events emitted by Claude Code v2.1.157.
// See docs/design/claude-code-integration.md §2 for the authoritative list.
//
// These are placeholder types; the JSON parsing lives in claudecode.go.
//
//lint:ignore U1000 // placeholder types for future JSON parsing
type rawEvent struct {
	Type    string          `json:"type"`
	Subtype string          `json:"subtype,omitempty"`
	Raw     json.RawMessage `json:"-"`
}

//lint:ignore U1000 // placeholder types for future JSON parsing
type initEvent struct {
	SessionID      string   `json:"session_id"`
	CWD            string   `json:"cwd"`
	Model          string   `json:"model"`
	PermissionMode string   `json:"permissionMode"`
	Tools          []string `json:"tools"`
}

//lint:ignore U1000 // placeholder types for future JSON parsing
type assistantEvent struct {
	Message struct {
		Content []struct {
			Type     string          `json:"type"` // "thinking" | "text" | "tool_use"
			Text     string          `json:"text,omitempty"`
			Thinking string          `json:"thinking,omitempty"`
			Name     string          `json:"name,omitempty"` // tool_use
			Input    json.RawMessage `json:"input,omitempty"`
		} `json:"content"`
	} `json:"message"`
	SessionID string `json:"session_id"`
}

//lint:ignore U1000 // placeholder types for future JSON parsing
type resultEvent struct {
	Subtype           string            `json:"subtype"`
	IsError           bool              `json:"is_error"`
	Result            string            `json:"result"`
	SessionID         string            `json:"session_id"`
	DurationMS        int64             `json:"duration_ms"`
	NumTurns          int               `json:"num_turns"`
	TotalCostUSD      float64           `json:"total_cost_usd"`
	TerminalReason    string            `json:"terminal_reason"`
	PermissionDenials []json.RawMessage `json:"permission_denials"`
	Usage             struct {
		InputTokens  int64 `json:"input_tokens"`
		OutputTokens int64 `json:"output_tokens"`
	} `json:"usage"`
}

// TODO(claude-code): implement parseStreamJSON(reader io.Reader, sink EventSink) (*Response, error)
// in claudecode.go. Pseudocode:
//   for each line:
//     unmarshal to rawEvent
//     switch type:
//       case "system" && subtype=="init": parse initEvent, call sink.OnSystemInit
//       case "assistant": parse, call sink.OnAssistantText / OnAssistantThinking / OnToolUse
//       case "result": parse, populate Response and return
//       default: log+skip
