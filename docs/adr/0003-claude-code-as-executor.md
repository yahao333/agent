# ADR 0003: Claude Code as the Sole Executor (v1)

- **Status**: Accepted
- **Date**: 2025-01-XX

## Context

LLM 调用层有多种实现路径：
1. 直接调 Anthropic/OpenAI API
2. 通过 Claude Code CLI
3. 通过 LangChain/AutoGen 等框架
4. 自研工具调用框架

## Decision

v1 **只支持 Claude Code CLI** 作为 Executor。通过 `--session-id`、
`--resume`、`--output-format stream-json`、`--max-budget-usd`、
`--permission-mode bypassPermissions` 等标志集成。

抽象出 `executor.Executor` 接口，未来可扩展其他 backend。

## Rationale

### 为什么选 Claude Code

1. **工具生态完整**：Read/Write/Edit/Bash/Grep/Glob/WebFetch 等开箱即用
2. **会话恢复内置**：`--resume <id>` 免去自研 context 管理
3. **成本控制内置**：`--max-budget-usd` 替代自研 token 计数
4. **权限模型内置**：`--permission-mode` 提供安全边界
5. **结构化输出**：`stream-json` 给 Ralph 完美的解析接口

### 为什么不直接调 API

会需要自研：
- 工具调用循环
- 多模型支持
- 成本/token 跟踪
- 文件系统访问控制

这些是 Claude Code 已经做好的事，重复造轮子违背 Ralph 的定位（**编排器，不是 LLM 框架**）。

### 为什么不上 LangChain/AutoGen

抽象层级太深，调试困难，且与 Claude Code 重叠。

## Consequences

### 正面

- 实现简单：Ralph 是 ~500 行 Go 编排代码 + Claude Code 做苦活
- 升级 Claude Code 即升级 Ralph 的"大脑"
- 用户已有的 Claude Code 配置（CLAUDE.md、settings.json）自动生效

### 负面

- **强耦合 Claude Code 接口稳定性**：如果 Anthropic 改 CLI，Ralph 要跟
- 无法在没装 Claude Code 的环境使用
- 受 Claude Code 模型可用性限制

### 缓解

- `executor.Executor` 接口预留，未来可加 `OpenAIExecutor`、`OllamaExecutor` 等
- 在 `docs/design/claude-code-integration.md` 标注实测的 CLI 版本（2.1.157）

## Future Work

- v2: 抽象 `MemoryBackend`，让 scratchpad 可以是文件/S3/数据库
- v3: 多 Executor 支持
