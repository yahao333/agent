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

func (e *ClaudeCodeExecutor) Run(
	ctx context.Context,
	req Request,
	sink EventSink,
	iterDir string,
) (*Response, error) {
	logger := e.Logger
	logger.LogAttrs(ctx, slog.LevelDebug, "claude executor started")

	// 1. Write system-suffix.md to iterDir.
	var suffixArg string
	if req.SystemPromptSuffix != "" {
		suffixPath := iterDir + "/system-suffix.md"
		if err := os.WriteFile(suffixPath, []byte(req.SystemPromptSuffix), 0644); err != nil {
			return nil, fmt.Errorf("write system-suffix.md: %w", err)
		}
		suffixArg = "--append-system-prompt @" + suffixPath
	}

	// 2. Write prompt to a temp file (avoids CLI arg parsing issues with newlines).
	promptPath := iterDir + "/prompt.txt"
	if err := os.WriteFile(promptPath, []byte(req.Prompt), 0644); err != nil {
		return nil, fmt.Errorf("write prompt.txt: %w", err)
	}

	// 3. Build args. Use @promptPath for -p to avoid CLI arg parsing issues.
	args := e.buildArgs(req, promptPath, suffixArg)

	// 4. Open output files.
	eventsPath := iterDir + "/events.jsonl"
	stderrPath := iterDir + "/stderr.log"
	parseErrPath := strings.TrimSuffix(eventsPath, ".jsonl") + "-parse-errors.log"

	// 5. Set up command.
	cmd := exec.CommandContext(ctx, e.BinaryPath, args...)
	cmd.Dir = req.WorkDir

	stderrFile, err := os.Create(stderrPath)
	if err != nil {
		return nil, fmt.Errorf("create stderr.log: %w", err)
	}
	cmd.Stderr = stderrFile

	// Use stdout file directly.
	stdoutPath := iterDir + "/stdout.txt"
	stdoutFile, err := os.Create(stdoutPath)
	if err != nil {
		return nil, fmt.Errorf("create stdout file: %w", err)
	}
	cmd.Stdout = stdoutFile

	// 6. Start process.
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start claude: %w", err)
	}
	stderrFile.Close()
	stdoutFile.Close()

	// 7. Poll the stdout file for result events.
	var resultResp *Response
	pollTicker := time.NewTicker(200 * time.Millisecond)
	defer pollTicker.Stop()
	var lastPos int64 = 0

	ctxDone := ctx.Done()

EventsLoop:
	for {
		select {
		case <-ctxDone:
			_ = cmd.Process.Kill()
			break EventsLoop

		case <-pollTicker.C:
			f, err := os.Open(stdoutPath)
			if err != nil {
				continue
			}
			fi, _ := f.Stat()
			size := fi.Size()
			if size > lastPos {
				_, err = f.Seek(lastPos, io.SeekStart)
				if err != nil {
					f.Close()
					continue
				}
				br := bufio.NewReader(f)
				for {
					line, err := br.ReadBytes('\n')
					if len(line) > 0 {
						line = line[:len(line)-1]
						if resp := tryParseResult(line, sink); resp != nil {
							resultResp = resp
							f.Close()
							_ = cmd.Process.Signal(syscall.SIGINT)
							time.Sleep(5 * time.Second)
							_ = cmd.Process.Kill()
							break EventsLoop
						}
					}
					if err != nil {
						break
					}
				}
				lastPos = size
			}
			f.Close()
		}
	}

	// 8. Write events.jsonl from the full stdout file.
	eventsFile, err := os.Create(eventsPath)
	if err != nil {
		return nil, fmt.Errorf("create events.jsonl: %w", err)
	}
	defer eventsFile.Close()

	parseErrFile, err := os.Create(parseErrPath)
	if err != nil {
		return nil, fmt.Errorf("create parse-errors.log: %w", err)
	}
	defer parseErrFile.Close()

	f, err := os.Open(stdoutPath)
	if err != nil {
		return nil, fmt.Errorf("open stdout: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	const maxScanTokenSize = 1024 * 1024
	buf := make([]byte, maxScanTokenSize)
	scanner.Buffer(buf, maxScanTokenSize)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		eventsFile.Write(line)
		eventsFile.Write([]byte("\n"))

		var raw rawEvent
		if err := json.Unmarshal(line, &raw); err != nil {
			parseErrFile.Write(line)
			parseErrFile.Write([]byte("\n"))
			continue
		}

		if raw.Type == "system" && raw.Subtype == "init" {
			var ev initEvent
			if json.Unmarshal(line, &ev) == nil {
				sink.OnSystemInit(ev.Model, ev.CWD, ev.Tools)
			}
		}
	}

	if resultResp != nil {
		resultResp.RawEventsPath = eventsPath
		logger.LogAttrs(ctx, slog.LevelDebug, "returning result", slog.Float64("costUSD", resultResp.CostUSD))
		return resultResp, nil
	}

	// No result found.
	exiterr, _ := cmd.Process.Wait()
	if exiterr != nil && !exiterr.Success() {
		return nil, fmt.Errorf("claude exited: %s", exiterr.String())
	}
	return nil, fmt.Errorf("no result event found")
}

// tryParseResult attempts to parse a JSON line as a result event.
func tryParseResult(line []byte, sink EventSink) *Response {
	var raw rawEvent
	if err := json.Unmarshal(line, &raw); err != nil {
		return nil
	}
	if raw.Type == "result" {
		var ev resultEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			return nil
		}
		return ev.ToResponse()
	}
	return nil
}

// buildArgs assembles the CLI flags.
// promptPath: file containing the prompt (passed as @path to -p).
// suffixArg: --append-system-prompt @path (already built by caller).
func (e *ClaudeCodeExecutor) buildArgs(req Request, promptPath, suffixArg string) []string {
	args := []string{
		"--output-format", "stream-json",
		"--verbose",
		"--permission-mode", req.PermissionMode,
		"-p", "@" + promptPath, // pass prompt via file to avoid CLI arg parsing issues
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
	if suffixArg != "" {
		args = append(args, suffixArg)
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
