# Claude Code Integration Spec

> 基于实测 Claude Code v2.1.157 的接口规范。
> 关联 ADR：0003

## 1. 调用形态

Ralph **永远**以非交互模式调用 Claude Code：

### 1.1 首轮调用（INIT → THINK）

```bash
claude \
  --session-id <ralph-generated-uuid> \
  --output-format stream-json \
  --verbose \
  --permission-mode bypassPermissions \
  --append-system-prompt @.ralph/runs/<run_id>/system-prompt-suffix.md \
  --max-budget-usd <config.max_cost_usd> \
  -p "<initial-prompt>"
```

### 1.2 后续调用（THINK 重入）

```bash
claude \
  --resume <session-id> \
  --output-format stream-json \
  --verbose \
  --permission-mode bypassPermissions \
  --max-budget-usd <剩余预算> \
  -p "<continuation-prompt>"
```

### 1.3 必传标志说明

| 标志 | 值 | 为什么必传 |
|---|---|---|
| `--session-id` 或 `--resume` | UUID | 状态恢复的命脉 |
| `--output-format stream-json` | 固定 | 解析能力的基石 |
| `--verbose` | 固定 | 流式 JSON 仅在 verbose 下完整 |
| `--permission-mode` | `bypassPermissions` | 非交互模式必须 |
| `--max-budget-usd` | 配置 | 由 Claude Code 兜底成本 |

### 1.4 可选标志

| 标志 | 何时用 |
|---|---|
| `--model` | 用户在 `config.yaml` 指定 |
| `--allowed-tools` / `--disallowed-tools` | 用户限制工具集 |
| `--add-dir` | 用户允许 Ralph 访问额外目录 |
| `--append-system-prompt` | 注入 Ralph 协议约定（详见 §3） |

## 2. stream-json 事件处理

### 2.1 事件分类

```go
type EventType string

const (
    EventSystemInit       EventType = "system.init"
    EventSystemHook       EventType = "system.hook_started|hook_response"
    EventAssistantMessage EventType = "assistant"
    EventResult           EventType = "result"
)
```

### 2.2 必处理事件

**`type: "system", subtype: "init"`**
- 提取并验证 `session_id` 与 Ralph 预生成的 UUID 一致
- 提取 `model`、`cwd`、`permissionMode` 写入 iteration 元数据
- 提取 `tools` 列表，记录可用工具
- ⚠️ 校验：若 `permissionMode != "bypassPermissions"` 且配置要求 bypass，告警

**`type: "assistant"`**
- 实时透传 `message.content[].text` 到 stdout（实现 8.6）
- `message.content[].thinking` 写入 `iterations/NNN.thinking.log`（不透传）
- `message.content[].tool_use` 写入 `iterations/NNN.tools.log` 供调试

**`type: "result"`**
- **本轮终结信号**。Ralph 在此事件后开始 EXTRACT 阶段
- 提取并累加：
  - `total_cost_usd` → `state.json.total_cost_usd`
  - `duration_ms` → `state.json.total_duration_ms`
  - `usage.input_tokens` / `output_tokens` → 累加
  - `num_turns` → 累加（Claude Code 内部 turn 数，可能 > 1）
- 检查：
  - `is_error == true` → 标记本轮失败，进入 GUARD 判断重试
  - `permission_denials` 非空 → 警告（bypassPermissions 下不应发生）
  - `terminal_reason != "completed"` → 异常退出，进入 GUARD

### 2.3 忽略事件

- `system.hook_started` / `system.hook_response`：用户自定义 hooks，与 Ralph 无关
- 任何 `type` 不识别的事件：**记录但不报错**，向前兼容

### 2.4 容错

- 行不是合法 JSON → 写入 `iterations/NNN.parse-errors.log`，继续
- 进程在收到 `result` 前退出 → 进入 GUARD 判失败
- stdout 流中途断开 → 视为本轮失败

## 3. Append System Prompt（协议注入）

Ralph 在 `runs/<run_id>/system-prompt-suffix.md` 写入以下内容并通过 `--append-system-prompt` 注入：

```markdown
# Ralph Protocol

You are running inside Ralph, an autonomous task loop.

## Your Workspace
- `.ralph/runs/<run_id>/scratchpad.md`: your long-term memory. Read it at the start
  of every turn. Update it before ending each turn.
- `.ralph/runs/<run_id>/goal.txt`: the user's goal. Do not modify.

## Required: State Block
Every turn, you MUST update or insert this block in scratchpad.md:

<!-- ralph:state -->
```json
{
  "iteration_status": "in_progress" | "done" | "blocked" | "needs_human",
  "summary": "what you did this turn",
  "next_action": "what you plan to do next (omit if done)",
  "blockers": ["list of blockers if any"]
}
```
<!-- /ralph:state -->

## Status Semantics
- `in_progress`: keep working; Ralph will call you again
- `done`: you believe the goal is achieved; Ralph will run external verification
- `blocked`: external dependency missing (e.g., need credentials)
- `needs_human`: ambiguity requires human decision

## When You Receive `[RALPH FEEDBACK]`
External verification failed. Read the feedback carefully and fix the root cause.
```

## 4. 调用进程管理

### 4.1 启动

```go
cmd := exec.CommandContext(ctx, "claude", args...)
cmd.Stdin = strings.NewReader(prompt)  // -p 后跟的 prompt 通过 stdin 传
cmd.Stdout = streamReader              // 我们的 stream-json 解析器
cmd.Stderr = stderrLog                 // 写入 iterations/NNN.stderr
```

### 4.2 中断处理

- 收到 SIGINT/SIGTERM → 传播给 claude 子进程
- 等待 claude 优雅退出，最多 10s
- 超时后 SIGKILL

### 4.3 重试策略（属于 GUARD 的一部分）

| 失败类型 | 重试 | 间隔 |
|---|---|---|
| 子进程 exit code != 0 且无 `result` 事件 | 最多 3 次 | 指数退避 1s/2s/4s |
| `result.is_error == true` 且 `subtype` 含 "api_error" | 最多 3 次 | 指数退避 |
| `result.is_error == true` 且 `subtype == "budget_exceeded"` | 不重试 | — |
| stdout 流断开 | 1 次重试 | 立即 |

## 5. 已知限制

- 当前 stream-json 在某些版本会把 `usage.iterations` 数组留空 — Ralph 不依赖此字段
- `--include-partial-messages` 暂不使用（v2 增强实时性时再加）
- 不支持 `--input-format stream-json` 向 Claude Code 注入消息（v2 增强）
