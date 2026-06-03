# Ralph

An autonomous AI agent that drives Claude Code toward a goal using a simple state-machine loop.

## Features

- **State machine orchestration**: INIT → THINK → EXTRACT → VERIFY → GUARD loop
- **Scratchpad memory**: Persists conversation context between iterations
- **External verification**: Runs `make test` or custom command to confirm task completion
- **Cost control**: Delegates budget enforcement to Claude Code's `--max-budget-usd`
- **Graceful shutdown**: Handles Ctrl+C signals properly
- **Colored console output**: Real-time progress with state transitions

## Quick Start

### 1. Initialize

```bash
ralph init
```

This creates `.ralph/config.yaml` with sensible defaults.

### 2. Configure (optional)

Edit `.ralph/config.yaml` and set your verification command:

```yaml
# The command that confirms your task is "done"
verify_cmd: "make test"  # or "go test ./...", "pytest", etc.
```

### 3. Run

```bash
ralph run "your task here"
```

### 4. Observe

Ralph prints colored state transitions in real-time:

```
▶ Run 20250603T120000Z-a1b2c3d4 started (session xxxxx)
  goal: create hello.txt
  dir:  /path/to/project/.ralph/runs/20250603T120000Z-a1b2c3d4

→ INIT
→ THINK
✓ model=claude-sonnet cwd=/path/to/project tools=12
🔧 Read
🔧 Write
→ EXTRACT
→ VERIFY
→ SUCCESS
```

## How It Works

```
┌─────────────────────────────────────────────────────┐
│                     FSM Loop                         │
│                                                      │
│  INIT → THINK → EXTRACT → VERIFY → GUARD             │
│    ↑                                     │          │
│    └─────────────────────────────────────┘          │
│                         ↓                             │
│               SUCCESS / FAILURE / ABORTED            │
└─────────────────────────────────────────────────────┘
```

1. **INIT**: Initialize state store and scratchpad
2. **THINK**: Run Claude Code with goal prompt
3. **EXTRACT**: Parse state block from scratchpad
4. **VERIFY**: Run external verification command
5. **GUARD**: Check iteration/cost/failure limits

## Project Structure

```
ralph/
├── cmd/ralph/main.go          # Entry point
├── internal/
│   ├── agent/                  # FSM loop and state
│   ├── cli/                    # Cobra commands
│   ├── config/                 # YAML config loader
│   ├── executor/               # Claude Code executor
│   ├── memory/                 # Scratchpad & state block
│   ├── run/                    # Run directory management
│   └── verify/                 # Verification command
├── docs/design/               # Architecture docs
├── docs/adr/                  # Architecture decision records
├── work_plan.md               # Development roadmap
└── README.md
```

## Commands

```bash
ralph init                    # Create .ralph/config.yaml with defaults
ralph run [goal]              # Run a goal until completion or limit
ralph version                 # Show Ralph and Claude Code versions
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `RALPH_LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |

## Run Artifacts

Each run creates `.ralph/runs/<run_id>/`:

| File | Description |
|------|-------------|
| `state.json` | Current FSM state (updated after each transition) |
| `scratchpad.md` | Claude Code's working memory |
| `result.json` | Final result (SUCCESS/FAILURE/ABORTED) |
| `iterations/NNN/` | Per-iteration artifacts |
| `verify/NNN.log` | Verification command output |

## Testing

```bash
make test           # Run all unit tests
make lint           # Run linters
go test -tags=integration ./...  # Run integration tests (requires Claude Code)
```

## License

MIT