# Ralph Agent Loop Design

> 本文档描述 Ralph 主循环的状态机、时序与各组件职责。
> 状态：Draft v1 · 关联 ADR：0002, 0003

## 1. 设计目标

Ralph 的核心是一个**有状态、可恢复、可观测**的 LLM 任务循环。给定一个目标（goal），
Ralph 反复调用 Claude Code 执行小步动作，直到：

- LLM 自报完成 **且** 外部验证器通过 → 成功
- 触发任一保护阈值（迭代数、成本、连续失败、wall-clock）→ 失败
- 用户 Ctrl+C → 优雅中止

## 2. 核心概念

| 概念 | 定义 |
|---|---|
| **Goal** | 用户输入的任务目标（一句话或一段描述） |
| **Run** | 一次完整的"目标→结果"过程，对应一个 `session_id` 与 `.ralph/runs/<run_id>/` 目录 |
| **Iteration** | Run 内的一次 LLM 调用（一次 `claude -p`） |
| **Scratchpad** | LLM 主权的 Markdown 文件，长期记忆 |
| **State** | Ralph 主权的 JSON 文件，迭代计数、成本、session_id 等 |
| **Verifier** | 用户配置的外部验证命令（如 `make test`） |

## 3. 状态机

```
                  ┌─────────┐
                  │  INIT   │
                  └────┬────┘
                       │ create run dir, gen session_id, write goal
                       ▼
                  ┌─────────┐
        ┌────────►│ THINK   │ (调用 Claude Code 一轮)
        │         └────┬────┘
        │              │ parse stream-json, write logs
        │              ▼
        │         ┌─────────┐
        │         │ EXTRACT │ (从 scratchpad 提取 ralph:state 区块)
        │         └────┬────┘
        │              │
        │      ┌───────┴────────┐
        │      │                │
        │  in_progress       done / blocked / needs_human
        │      │                │
        │      ▼                ▼
        │  ┌────────┐      ┌─────────┐
        │  │ GUARD  │      │ VERIFY  │ (仅当 done)
        │  └───┬────┘      └────┬────┘
        │      │                │
        │  通过 │                │ pass → SUCCESS
        │      │                │ fail → 注入失败反馈，回 THINK
        │      ▼                │
        └──────┘                ▼
       超阈值                ┌─────────┐
          │                  │ SUCCESS │ / FAILURE / ABORTED
          ▼                  └─────────┘
       ┌─────────┐
       │ FAILURE │
       └─────────┘
```

### 3.1 状态详解

| 状态 | 入口 | 出口 | 关键动作 |
|---|---|---|---|
| **INIT** | 用户启动 `ralph run "<goal>"` | → THINK | 创建 `runs/<run_id>/`，生成 UUID 作为 session_id，写 `goal.txt`、初始化 `state.json`、初始化 `scratchpad.md` |
| **THINK** | 来自 INIT 或 GUARD/VERIFY 通过 | → EXTRACT | 调用 `executor.RunIteration()`，落盘 stream-json 到 `iterations/NNN.jsonl` |
| **EXTRACT** | THINK 完成 | → GUARD（继续） / → VERIFY（done） | 从 scratchpad 解析 `<!-- ralph:state -->` 区块；更新 state.json |
| **GUARD** | EXTRACT 状态为 `in_progress`/`blocked` | → THINK（继续） / → FAILURE（超限） | 检查 5 重保护阈值 |
| **VERIFY** | EXTRACT 状态为 `done` | → SUCCESS / → THINK（注入失败反馈） | 执行 `verify_cmd`，落盘输出到 `verify/NNN.log` |
| **SUCCESS** | VERIFY 通过 | 终态 | 写 `result.json`，打印总结 |
| **FAILURE** | GUARD 阈值超限 / 不可恢复错误 | 终态 | 写 `result.json`，打印诊断 |
| **ABORTED** | 收到 SIGINT/SIGTERM | 终态 | 写 `result.json`，标记中止原因 |

### 3.2 状态转移的不变量

1. **每个状态转移都必须刷盘** `state.json`，确保任意时刻 kill -9 后能从 disk 重建
2. **session_id 在 Run 生命周期内不变**（除非未来支持 fork）
3. **iteration 号单调递增**，永不回退
4. **失败反馈注入是有状态的**：VERIFY 失败时，把验证器输出存入 `state.json.last_verify_output`，下一轮 THINK 时附加到 prompt

## 4. 时序图（典型成功流）

```
User       Ralph                Executor          ClaudeCode        Verifier      FS
 │           │                     │                  │                │           │
 │─run "X"──>│                     │                  │                │           │
 │           │─mkdir runs/<id>─────────────────────────────────────────────────────>│
 │           │─write goal.txt──────────────────────────────────────────────────────>│
 │           │─write state.json────────────────────────────────────────────────────>│
 │           │                     │                  │                │           │
 │           │─Iter 1 prompt──────>│                  │                │           │
 │           │                     │─claude --sid X ─>│                │           │
 │           │                     │<─stream-json ───┤                │           │
 │           │<──events stream─────│                  │                │           │
 │           │─append iterations/001.jsonl────────────────────────────────────────>│
 │           │─parse scratchpad────────────────────────────────────────────────────>│
 │           │  state=in_progress                                                   │
 │           │─update state.json───────────────────────────────────────────────────>│
 │           │                     │                  │                │           │
 │           │─Iter 2 prompt──────>│                  │                │           │
 │           │                     │─claude --resume X>│                │           │
 │           │                     │<─stream-json ───┤                │           │
 │           │<──events stream─────│                  │                │           │
 │           │  state=done                                                          │
 │           │                     │                  │                │           │
 │           │─run verify_cmd─────────────────────────────────────────>│           │
 │           │<─exit 0 ────────────────────────────────────────────────│           │
 │           │─write verify/002.log────────────────────────────────────────────────>│
 │           │─write result.json───────────────────────────────────────────────────>│
 │<─SUCCESS──│                     │                  │                │           │
```

## 5. 失败注入机制（GUARD/VERIFY 失败时回到 THINK）

当 VERIFY 失败时，Ralph 在下一轮 THINK 的 prompt 里**追加**：

```
[RALPH FEEDBACK · Iter N]
你上一轮报告任务完成，但外部验证失败：
$ make test
<verify output, 最多 2000 字符尾部>

请分析失败原因并继续修复。
```

这就是"LLM 自报 + 外部验证"协议的闭环。

## 6. 中断恢复（未来）

v1 不支持恢复中断的 Run。结构已为其准备：
- `state.json` 包含完整恢复所需的所有字段
- session_id 持久化，可以 `claude --resume` 续接

v2 将提供 `ralph resume <run_id>` 命令。

## 7. 不做的事（明确边界）

- ❌ 不做并发/多 agent 协调（v1 单 Run 单进程）
- ❌ 不做 prompt 自动重写/反思（让 Claude Code 自己处理）
- ❌ 不做工具调用（Ralph 不直接调 LLM API，全部委托给 Claude Code）
- ❌ 不做 token 计数（用 Claude Code 的 `--max-budget-usd` 兜底）
