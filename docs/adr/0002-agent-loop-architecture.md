# ADR 0002: Agent Loop Architecture

- **Status**: Accepted
- **Date**: 2025-01-XX
- **Deciders**: 项目维护者
- **Supersedes**: -

## Context

Ralph 需要一个核心循环把"LLM 调用 → 状态判断 → 验证 → 决策"组织起来。
我们权衡过多种范式（详见 docs/design/agent-loop.md）。

## Decision

采用**状态机驱动的循环**，包含 7 个状态（INIT/THINK/EXTRACT/GUARD/VERIFY/
SUCCESS/FAILURE/ABORTED），所有状态转移**强制刷盘** `state.json`。

完成判定采用 **LLM 自报 + 外部验证** 双轨制：
- LLM 在 scratchpad 的 `<!-- ralph:state -->` 区块声明完成
- Ralph 跑用户配置的 `verify_cmd`，退出码 0 才算真完成
- 验证失败时把输出注入下一轮 prompt

记忆采用**双轨制**：
- `scratchpad.md`（LLM 主权，Markdown，长期记忆）
- `state.json`（Ralph 主权，结构化，迭代计数/成本/session_id）

## Consequences

### 正面

- 任意时刻 kill -9 可重建（v2 实现 resume 命令）
- 状态机清晰，易测试（可对每个状态写单元测试）
- 验证器外部化，零成本支持任何项目（用户配 `verify_cmd` 即可）
- 双轨记忆避免了"LLM 修改自己状态导致幻觉"的问题

### 负面

- 实现复杂度高于"无脑 while 循环"
- 每轮多一次 scratchpad 解析
- state.json 频繁写盘（每状态转移）—— 但这是 KB 级文件，可忽略

### 中性

- 当 LLM 拒绝按格式输出 state 区块时，Ralph 退化为"宽容默认"，可能多跑几轮才发现卡住

## Alternatives Considered

### A. 纯计数循环（"跑 N 轮就停"）
拒绝：完全不考虑任务进度，浪费成本

### B. 仅依赖 LLM 自报
拒绝：LLM 幻觉会导致虚假完成

### C. 仅依赖外部验证
拒绝：没有 LLM 的"我认为完成"信号，每轮都要跑验证器，慢且贵

### D. 让 LLM 通过 `--json-schema` 直接返回结构化状态
拒绝：会破坏 Claude Code 的工具调用能力（详见 claude-code-integration.md）
