# Ralph

一个自主 AI 智能体，通过简单的状态机循环驱动 Claude Code 达成目标。

## 特性

- **状态机编排**：INIT → THINK → EXTRACT → VERIFY → GUARD 循环
- **便笺记忆**：在迭代之间持久化对话上下文
- **外部验证**：运行 `make test` 或自定义命令来确认任务完成
- **成本控制**：将预算执行委托给 Claude Code 的 `--max-budget-usd`
- **优雅关闭**：正确处理 Ctrl+C 信号
- **彩色控制台输出**：实时显示状态转换进度

## 快速开始

### 1. 配置

在你的项目中创建 `.ralph/config.yaml`：

```yaml
# 验证命令（可选，默认为 "make test"）
verify_cmd: "make test"

# 硬性限制
max_iterations: 50
max_cost_usd: 5.0
max_consecutive_fails: 3
max_wall_clock_sec: 3600

# Claude Code 选项
permission_mode: "bypassPermissions"  # 或 "acceptEdits"
model: ""  # 空 = Claude Code 默认值
```

### 2. 运行

```bash
# 单个目标
ralph run "创建包含 hello 这个词的 hello.txt"

# 使用自定义配置
ralph run --config /path/to/config.yaml "你的任务描述"
```

### 3. 观察

Ralph 实时打印彩色状态转换：

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

## 工作原理

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

1. **INIT**：初始化状态存储和便笺
2. **THINK**：使用目标提示词运行 Claude Code
3. **EXTRACT**：从便笺中解析状态块
4. **VERIFY**：运行外部验证命令
5. **GUARD**：检查迭代/成本/失败限制

## 项目结构

```
ralph/
├── cmd/ralph/main.go          # 入口点
├── internal/
│   ├── agent/                  # FSM 循环与状态
│   ├── cli/                    # Cobra 命令
│   ├── config/                 # YAML 配置加载器
│   ├── executor/               # Claude Code 执行器
│   ├── memory/                 # 便笺与状态块
│   ├── run/                    # 运行目录管理
│   └── verify/                 # 验证命令
├── docs/design/               # 架构设计文档
├── docs/adr/                  # 架构决策记录
├── work_plan.md               # 开发路线图
└── README.md
```

## 命令

```bash
ralph run [goal]    # 运行目标直到完成或达到限制
ralph version       # 显示 Ralph 和 Claude Code 版本
```

## 环境变量

| 变量 | 默认值 | 描述 |
|------|--------|------|
| `RALPH_LOG_LEVEL` | `info` | 日志级别：`debug`、`info`、`warn`、`error` |

## 运行产物

每次运行会在 `.ralph/runs/<run_id>/` 下创建以下文件：

| 文件 | 描述 |
|------|------|
| `state.json` | 当前 FSM 状态（每次转换后更新） |
| `scratchpad.md` | Claude Code 的工作记忆 |
| `result.json` | 最终结果（SUCCESS/FAILURE/ABORTED） |
| `iterations/NNN/` | 每轮迭代的产物 |
| `verify/NNN.log` | 验证命令的输出 |

## 测试

```bash
make test           # 运行所有单元测试
make lint           # 运行 linter
go test -tags=integration ./...  # 运行集成测试（需要 Claude Code）
```

## 许可证

MIT