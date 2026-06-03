// Package memory handles Ralph's persistence: scratchpad.md (LLM-owned)
// and state.json (Ralph-owned). See docs/design/scratchpad-protocol.md.
package memory

import (
	"encoding/json"
	"fmt"
	"regexp"
)

// IterationStatus is what the LLM tells Ralph at the end of each turn.
type IterationStatus string

const (
	StatusInProgress IterationStatus = "in_progress"
	StatusDone       IterationStatus = "done"
	StatusBlocked    IterationStatus = "blocked"
	StatusNeedsHuman IterationStatus = "needs_human"
)

// StateBlock is the JSON payload inside <!-- ralph:state --> ... <!-- /ralph:state -->.
type StateBlock struct {
	IterationStatus IterationStatus `json:"iteration_status"`
	Summary         string          `json:"summary"`
	NextAction      string          `json:"next_action,omitempty"`
	Blockers        []string        `json:"blockers,omitempty"`
	Confidence      *float64        `json:"confidence,omitempty"`
}

// ExtractError is returned (alongside a best-effort StateBlock) when the
// scratchpad's state block is missing/malformed. Callers should LOG and
// continue with the returned fallback block — never abort the Run.
type ExtractError struct {
	Reason string
	Raw    string // the offending text, for debugging
}

func (e *ExtractError) Error() string { return e.Reason }

var stateBlockRe = regexp.MustCompile(
	`(?s)<!--\s*ralph:state\s*-->\s*` +
		"```json\\s*\\n(.*?)\\n```" +
		`\s*<!--\s*/ralph:state\s*-->`,
)

// ExtractStateBlock parses the scratchpad contents.
//
// Contract (see scratchpad-protocol.md §4.4):
//   - Missing block      → returns in_progress + ExtractError(reason=missing)
//   - Multiple blocks    → returns last one + ExtractError(reason=duplicate)
//   - Malformed JSON     → returns in_progress + ExtractError(reason=malformed_json)
//   - Unknown status     → coerces to in_progress + ExtractError(reason=unknown_status)
//   - Valid              → returns block, nil
func ExtractStateBlock(scratchpadContent string) (*StateBlock, error) {
	matches := stateBlockRe.FindAllStringSubmatch(scratchpadContent, -1)

	if len(matches) == 0 {
		return fallback("<missing state block>"), &ExtractError{Reason: "missing"}
	}

	// Use the last block if duplicates (and warn).
	var extractErr error
	if len(matches) > 1 {
		extractErr = &ExtractError{Reason: "duplicate", Raw: fmt.Sprintf("%d blocks found", len(matches))}
	}
	jsonStr := matches[len(matches)-1][1]

	var block StateBlock
	if err := json.Unmarshal([]byte(jsonStr), &block); err != nil {
		return fallback("<malformed state>"), &ExtractError{Reason: "malformed_json", Raw: jsonStr}
	}

	// Normalize "completed" to "done" (some LLMs use this alias).
	if block.IterationStatus == "completed" {
		block.IterationStatus = StatusDone
	}

	switch block.IterationStatus {
	case StatusInProgress, StatusDone, StatusBlocked, StatusNeedsHuman:
		// ok
	default:
		fb := fallback(block.Summary)
		return fb, &ExtractError{Reason: "unknown_status", Raw: string(block.IterationStatus)}
	}

	return &block, extractErr
}

func fallback(summary string) *StateBlock {
	return &StateBlock{
		IterationStatus: StatusInProgress,
		Summary:         summary,
	}
}
