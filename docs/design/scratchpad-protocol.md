# Scratchpad Protocol

> 定义 Ralph 与 LLM 共享的 scratchpad.md 文件格式。
> 状态：Normative

## 1. 文件位置

`.ralph/runs/<run_id>/scratchpad.md`

## 2. 主权约定

- **LLM 主权**：LLM 可任意修改 scratchpad.md 的**正文**
- **Ralph 主权**：仅 `<!-- ralph:state -->` 区块由 Ralph 解析（但写入由 LLM 完成）
- **Ralph 只读**：除非紧急情况（参见 §6），Ralph 不修改 scratchpad.md

## 3. 初始模板

Ralph 在 INIT 阶段写入：

```markdown
# Ralph Scratchpad

**Run ID**: <run_id>
**Goal**: <goal>
**Started**: <ISO timestamp>

<!-- ralph:state -->
```json
{
  "iteration_status": "in_progress",
  "summary": "Run initialized. No work done yet.",
  "next_action": "Read goal, plan approach.",
  "blockers": []
}
```
<!-- /ralph:state -->

## Plan

_LLM: write your plan here_

## Progress Log

_LLM: append entries as you work_

## Notes

_LLM: scratch space for anything else_
```

## 4. State 区块规范

### 4.1 语法

- 必须以 `<!-- ralph:state -->` 单独一行开始
- 必须以 `<!-- /ralph:state -->` 单独一行结束
- 中间必须是合法的 JSON code fence（` ```json ... ``` `）
- 全文件**只能有一个**这样的区块

### 4.2 提取算法（Go）

```go
var stateBlockRe = regexp.MustCompile(
    `(?s)<!--\s*ralph:state\s*-->\s*` +
    "```json\\s*\\n(.*?)\\n```" +
    `\s*<!--\s*/ralph:state\s*-->`,
)
```

### 4.3 字段规范

```json
{
  "iteration_status": "in_progress" | "done" | "blocked" | "needs_human",
  "summary": "string, required, non-empty",
  "next_action": "string, optional",
  "blockers": ["string", ...],
  "confidence": 0.0-1.0  // optional, future
}
```

### 4.4 容错

| 异常 | 处理 |
|---|---|
| 找不到区块 | 警告并视为 `in_progress`，summary="<missing state block>" |
| 找到多个区块 | 取最后一个，警告 |
| JSON 解析失败 | 视为 `in_progress`，summary="<malformed state>"，原始内容存 `state.json.last_extract_error` |
| `iteration_status` 不在枚举内 | 视为 `in_progress`，警告 |

宽容是关键：**LLM 偶尔会犯格式错误，不要因此 abort 整个 Run**。

## 5. 演进策略

未来如增加字段：
- 新字段必须 optional
- 旧 Run 的 scratchpad 不修改
- 不删除字段，只标记 deprecated

## 6. 紧急情况：Ralph 写 scratchpad

Ralph 唯一会写 scratchpad 的场景：**VERIFY 失败注入反馈**。但此时 Ralph 写的是
**prompt 的 stdin**，不写 scratchpad 文件本身。Scratchpad 始终由 LLM 自己更新。
