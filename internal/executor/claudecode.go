package executor

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ClaudeCodeExecutor invokes the `claude` CLI in non-interactive mode.
type ClaudeCodeExecutor struct {
	BinaryPath string // default "claude", overridable for testing
	Logger     *slog.Logger
}

func New() *ClaudeCodeExecutor {
	return &ClaudeCodeExecutor{BinaryPath: "claude", Logger: slog.Default()}
}

// parseState is shared between Run() and parseStreamJSON goroutine.
type parseState struct {
	mu        sync.Mutex
	resp      *Response
	done      bool
	resultErr error
}

func (s *parseState) setResult(resp *Response) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resp = resp
	s.done = true
}

func (s *parseState) setError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resultErr = err
	s.done = true
}

func (s *parseState) getResponse() *Response {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.resp
}

func (s *parseState) getError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.resultErr
}

func (e *ClaudeCodeExecutor) Run(
	ctx context.Context,
	req Request,
	sink EventSink,
	iterDir string,
) (*Response, error) {
	logger := e.Logger

	// 1. Write system-suffix.md to iterDir.
	var suffixArg string
	if req.SystemPromptSuffix != "" {
		suffixPath := iterDir + "/system-suffix.md"
		if err := os.WriteFile(suffixPath, []byte(req.SystemPromptSuffix), 0644); err != nil {
			return nil, fmt.Errorf("write system-suffix.md: %w", err)
		}
		suffixArg = "--append-system-prompt @" + suffixPath
	}

	// 2. Build args.
	args := e.buildArgs(req)
	if suffixArg != "" {
		args = append(args, suffixArg)
	}

	// 3. Open output files.
	eventsPath := iterDir + "/events.jsonl"
	eventsFile, err := os.Create(eventsPath)
	if err != nil {
		return nil, fmt.Errorf("create events.jsonl: %w", err)
	}
	defer eventsFile.Close()

	stderrPath := iterDir + "/stderr.log"
	stderrFile, err := os.Create(stderrPath)
	if err != nil {
		return nil, fmt.Errorf("create stderr.log: %w", err)
	}
	defer stderrFile.Close()

	// 4. Set up command.
	cmd := exec.CommandContext(ctx, e.BinaryPath, args...)
	cmd.Dir = req.WorkDir

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}

	stdoutR, stdoutW := io.Pipe()
	cmd.Stdout = stdoutW

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	// 5. Start process.
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start claude: %w", err)
	}

	// Goroutine: SIGINT on ctx cancel, then SIGKILL after 10s.
	var procWg sync.WaitGroup
	procWg.Add(1)
	go func() {
		defer procWg.Done()
		<-ctx.Done()
		logger.LogAttrs(ctx, slog.LevelWarn, "context cancelled, sending SIGINT")
		_ = cmd.Process.Signal(syscall.SIGINT)
		select {
		case <-ctx.Done():
		case <-time.After(10 * time.Second):
			logger.LogAttrs(ctx, slog.LevelWarn, "graceful exit timeout, sending SIGKILL")
			_ = cmd.Process.Kill()
		}
	}()

	// Shared parse state.
	state := &parseState{}

	// Goroutine: tee stdout to events.jsonl AND feed parseStreamJSON.
	var parseWg sync.WaitGroup
	parseWg.Add(1)
	go func() {
		defer parseWg.Done()
		parseStreamJSON(stdoutR, sink, eventsFile, eventsPath, state, logger)
	}()

	// Goroutine: drain stderr to stderr.log.
	parseWg.Add(1)
	go func() {
		defer parseWg.Done()
		io.Copy(stderrFile, stderr)
	}()

	// Write prompt to stdin, then close it.
	go func() {
		defer stdin.Close()
		fmt.Fprint(stdin, req.Prompt)
	}()

	// Wait for parse to finish (signals result event or stream close).
	parseWg.Wait()

	// Wait for process to fully exit.
	waitErr := cmd.Wait()
	procWg.Wait()

	if err := state.getError(); err != nil {
		logger.LogAttrs(ctx, slog.LevelError, "parseStreamJSON error", slog.String("err", err.Error()))
		return nil, err
	}

	resp := state.getResponse()
	if resp == nil {
		// Stream ended without result event.
		if waitErr != nil {
			return nil, fmt.Errorf("claude exited with error: %w", waitErr)
		}
		return nil, fmt.Errorf("stream ended without result event")
	}

	// Attach raw events path.
	resp.RawEventsPath = eventsPath

	return resp, nil
}

// parseStreamJSON reads stream-json from r, writes each line to eventsFile,
// calls sink methods for each event, populates state.resp on result, and
// signals state.done when finished.
func parseStreamJSON(r io.Reader, sink EventSink, eventsFile io.Writer, eventsPath string, state *parseState, logger *slog.Logger) {
	scanner := bufio.NewScanner(r)
	parseErrPath := eventsPath[:len(eventsPath)-4] + "parse-errors.log"
	parseErrFile, err := os.Create(parseErrPath)
	if err == nil {
		defer parseErrFile.Close()
	}

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		// Write raw line to events.jsonl.
		eventsFile.Write(line)
		eventsFile.Write([]byte("\n"))

		var raw rawEvent
		if err := json.Unmarshal(line, &raw); err != nil {
			logger.LogAttrs(nil, slog.LevelWarn, "malformed JSON line", slog.String("err", err.Error()))
			if parseErrFile != nil {
				parseErrFile.Write(line)
				parseErrFile.Write([]byte("\n"))
			}
			continue
		}

		switch raw.Type {
		case "system":
			if raw.Subtype == "init" {
				var ev initEvent
				if err := json.Unmarshal(line, &ev); err != nil {
					continue
				}
				sink.OnSystemInit(ev.Model, ev.CWD, ev.Tools)
			}
			// All other system subtypes: skip.

		case "assistant":
			var ev assistantEvent
			if err := json.Unmarshal(line, &ev); err != nil {
				continue
			}
			for _, content := range ev.Message.Content {
				switch content.Type {
				case "thinking":
					sink.OnAssistantThinking(content.Thinking)
				case "text":
					sink.OnAssistantText(content.Text)
				case "tool_use":
					sink.OnToolUse(content.Name, string(content.Input))
				}
			}

		case "result":
			var ev resultEvent
			if err := json.Unmarshal(line, &ev); err != nil {
				state.setError(fmt.Errorf("parse result event: %w", err))
				return
			}
			state.setResult(ev.ToResponse())
			return

		default:
			// Unknown event types: log and skip (forward-compatibility).
			logger.LogAttrs(nil, slog.LevelDebug, "unknown event type", slog.String("type", raw.Type))
		}
	}

	if err := scanner.Err(); err != nil {
		state.setError(fmt.Errorf("scanner error: %w", err))
		return
	}

	// Stream ended without result event.
	state.setError(fmt.Errorf("stream ended without result event"))
}

// buildArgs assembles the CLI flags. --append-system-prompt is added by Run().
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

	return args
}

// ParseResultEvent extracts a resultEvent from a line of stream-json.
func ParseResultEvent(line []byte) (*resultEvent, error) {
	var ev resultEvent
	if err := json.Unmarshal(line, &ev); err != nil {
		return nil, err
	}
	return &ev, nil
}

// ToResponse converts a resultEvent to a Response.
func (ev *resultEvent) ToResponse() *Response {
	denials := make([]string, 0, len(ev.PermissionDenials))
	for _, d := range ev.PermissionDenials {
		var s string
		if json.Unmarshal(d, &s) == nil {
			denials = append(denials, s)
		}
	}
	return &Response{
		SessionID:         ev.SessionID,
		FinalText:         ev.Result,
		IsError:           ev.IsError,
		TerminalReason:    ev.TerminalReason,
		CostUSD:           ev.TotalCostUSD,
		DurationMS:        ev.DurationMS,
		InputTokens:       ev.Usage.InputTokens,
		OutputTokens:      ev.Usage.OutputTokens,
		NumTurns:          ev.NumTurns,
		PermissionDenials: denials,
	}
}

// Unquote unquotes a JSON-string value.
func Unquote(s string) string {
	if s == "" {
		return s
	}
	if strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`) {
		s = s[1 : len(s)-1]
	}
	return strings.ReplaceAll(s, `\"`, `"`)
}
