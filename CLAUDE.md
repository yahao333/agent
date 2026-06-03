## Ralph — Claude Code 工作协议

> 每当你处理 Ralph 相关代码时，请先阅读本文。它记录了人类（Yang）与 Claude Code 之间的契约，避免我们每次会话都重复争论同样的决策。

## 1. Ralph 是什么

Ralph 是一个 Go CLI 工具，它在 Claude Code 之上运行一个**自主任务循环**。给定一个目标后，Ralph 会反复调用 `claude -p`，直到大模型自我报告完成 **并且** 外部验证器（例如 `make test`）通过 —— 或者触发了安全限制为止。

Ralph 是**编排器**，而不是大模型本身。所有实际的编码工作都由 Claude Code 在 Ralph 的监督下完成。

## 2. 编码前请阅读以下文档

按顺序阅读：

1. **`docs/design/agent-loop.md`** —— 状态机设计
2. **`docs/design/claude-code-integration.md`** —— Claude Code CLI 契约
3. **`docs/design/scratchpad-protocol.md`** —— 记忆格式
4. **`docs/adr/0001-language-choice.md`**、**`0002-agent-loop-architecture.md`**、**`0003-claude-code-as-executor.md`** —— 设计决策的缘由

如果某段代码与这些文档相矛盾，**以文档为准** —— 请先发起 PR 修改文档，再修改代码。

## 3. 架构简述

`cmd/ralph` → `internal/agent.Loop`（状态机）→ `internal/executor.Executor`（调用 `claude` CLI，解析流式 JSON）→ `internal/memory`（scratchpad + state.json）→ `internal/verify`（执行 `verify_cmd`）。**每次**状态转换后，状态都会持久化到 `.ralph/runs/<run_id>/` 目录下。session_id 由 Ralph 生成（UUID），并通过 `--session-id`（第一轮）/ `--resume`（后续轮次）传递给 Claude Code。

## 4. 硬性规则

### 4.1 Claude Code 接口是神圣的

我们使用**固定的**标志集调用 Claude Code（参见 `claude-code-integration.md` §1.3）。在不更新文档和 ADR 的情况下，不得添加、删除或更改这些标志。

流式 JSON 解析器必须能容忍**未知的事件类型** —— 记录日志并跳过，绝不能崩溃。未来 Claude Code 版本新增的事件类型不能导致我们程序出错。

### 4.2 状态块提取是宽容的

根据 `scratchpad-protocol.md` §4.4：缺失或格式错误的状态块**绝不能**中断运行。应回退到 `in_progress` 状态，并将错误记录到 `state.json.last_extract_error` 中。

### 4.3 转换前先持久化

每个 FSM 状态转换都必须在执行新状态的工作**之前**调用 `StateStore.Update()`。这为 `kill -9` 后的恢复能力打下基础（v2 特性）。

### 4.4 成本限制由 Claude Code 执行

我们传递 `--max-budget-usd` 参数，并信任 Claude Code 会执行它。不要尝试自己统计 token 数量。

### 4.5 验证器是外部的，不是 Ralph 的职责

Ralph 运行 `verify_cmd`（用户配置，或回退到 `make test`）。Ralph 不知道"测试通过"的具体含义。退出码 0 表示通过，其他任何值表示失败。输出会被原样捕获并反馈给大模型。

## 5. 如何添加功能

1. 如果涉及状态机：先更新 `agent-loop.md`
2. 如果涉及 Claude Code 标志：先更新 `claude-code-integration.md`
3. 如果改变 scratchpad 格式：先更新 `scratchpad-protocol.md`
4. 如果是重要的设计决策：编写 ADR
5. 最后写代码，测试紧随其后；提交前运行 `make lint test`
6. 使用常规提交格式：`feat:`、`fix:`、`docs:`、`refactor:`、`test:`、`chore:`

## 6. 如何添加对 Claude Code 功能的依赖

我们目前依赖以下 CLI 标志（根据集成规范）：
`--session-id`、`--resume`、`--output-format stream-json`、`--verbose`、`--permission-mode`、`--max-budget-usd`、`--append-system-prompt`、`-p`。

如果你想使用其他标志（例如 `--json-schema`、`--fork-session`、`--include-partial-messages`）：

1. 验证该标志在当前 `claude --help` 中确实存在
2. 运行冒烟测试，捕获输出，粘贴到 PR 中
3. 在 `claude-code-integration.md` §1.4（可选标志）中添加一行

## 7. Ralph 刻意不做的事情（v1）

- 多智能体 / 并发运行
- 恢复被中断的运行（`ralph resume <id>`）
- 直接调用 Anthropic/OpenAI API
- Token 计数（委托给 Claude Code 的预算机制）
- 提示词重写 / 自反思
- 沙箱隔离（请使用 git + `bypassPermissions` 警告）

如果某个功能请求落入了上述列表，请拒绝并引用本节内容。

## 8. 测试策略

- **单元测试**：状态机转换、状态块解析器（这里有很多边界情况）、配置加载器、验证器回退链
- **集成测试（较慢，按需启用）**：在真实环境中对一个小任务运行 `claude -p`。打上 `//go:build integration` 标签，这样 `make test` 默认会跳过它。运行方式：`go test -tags=integration ./...`
- **模拟执行器**：`internal/executor/mock.go`（待添加），用于快速循环测试

## 9. 日志与可观测性

- 每轮迭代：`.ralph/runs/<id>/iterations/NNN/events.jsonl`（原始流）、`stderr.log`、`thinking.log`、`tools.log`
- 每次验证：`.ralph/runs/<id>/verify/NNN.log`
- 全局状态：`.ralph/runs/<id>/state.json`
- 最终结果：`.ralph/runs/<id>/result.json`

调试某次运行时，**从 `state.json` 开始**，然后深入到出现问题的那一轮迭代。

## 10. 有疑问时

请提问。花 5 分钟澄清问题，胜过花 5 小时实现错误的东西。说"我不确定"不会有任何惩罚 —— 真正的惩罚来自于默默做出错误的假设。
