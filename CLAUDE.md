# CLAUDE.md

> 本文件是 Claude Code（以及其他 AI 编程助手）在本项目工作时的**必读规范**。
> 维护原则：随项目演化持续更新；每次重大决策变更后立即同步。

---

## 1. 项目背景

**Ralph** 是一个 CLI 工具，本质是一个 **AI Agent 智能体**：通过循环调用 Claude Code（或其他 LLM CLI）逐步推进一个目标。设计灵感来自 "Ralph Wiggum technique"，类似项目：`aider`、`claude-code` 本身、`opencode`。

**核心机制（极简版）**：
1. 用户提供一个目标（goal）
2. Ralph 维护一份 "scratchpad"（agent 的工作记忆，存为本地 markdown）
3. 每轮迭代：组装 prompt → 调用 LLM CLI 子进程 → 解析输出 → 更新 scratchpad → 判断是否完成
4. 失败/超时/达到迭代上限则停止，输出可读的运行报告

---

## 2. 技术栈（权威）

| 类别 | 选型 | 版本约束 |
|---|---|---|
| 语言 | Go | 1.24+ |
| CLI 框架 | `spf13/cobra` | 最新稳定版 |
| 日志 | 标准库 `log/slog` | — |
| 测试 | 标准库 `testing` + `stretchr/testify` | — |
| 配置 | 自写 `internal/config` 包 | — |
| 子进程编排 | 标准库 `os/exec` + `context` | — |
| Lint | `golangci-lint` | 配置见 `.golangci.yml` |

**未引入但已规划**（未来需要时再加，**不要提前引入**）：
- `gin`：未来加 Web Dashboard 时引入
- `gorm`：未来需要持久化运行历史时引入
- TUI 库（`bubbletea`/`lipgloss`）：用户体验阶段再考虑

---

## 3. 目录结构与职责
cmd/ralph/           只做依赖装配，禁止写业务逻辑
internal/
agent/             Agent 核心循环（loop / 终止条件 / 重试）
executor/          os/exec 封装：子进程启停、stdout/stderr 捕获、超时控制
prompt/            Prompt 模板加载、变量替换、token 估算
memory/            Scratchpad 读写（文件系统层）
config/            配置加载（env / yaml / 默认值）
cli/               cobra 命令定义（root / run / version 等）
pkg/                 （目前为空）只放真正想给外部用的代码
testdata/            测试 fixture
docs/adr/            架构决策记录


**包依赖方向（强约束）**：

cli  ──→  agent  ──→  executor
│
├──→  prompt
├──→  memory
└──→  config

- ❌ 禁止反向依赖（如 `executor` 引用 `agent`）
- ❌ 禁止跨层依赖（如 `cli` 直接引用 `executor`，应通过 `agent`）
- 如需打破，**必须先写 ADR**

---

## 4. 编码规范

### 4.1 命名

- **包名**：小写单词，不用下划线，不用复数。`agent` ✅；`agents` ❌；`agent_loop` ❌
- **接口**：单方法接口以 `-er` 结尾（`Runner`、`Executor`）；多方法接口用功能名（`Agent`、`Memory`）
- **错误变量**：`ErrXxx`（如 `ErrTimeout`）；错误类型：`XxxError`
- **测试**：`TestXxx_Scenario`（如 `TestRunCmd_RequiresGoal`）；表驱动用 `tests := []struct{...}`

### 4.2 错误处理

- **永远不要丢弃错误**：`_ = doSomething()` 必须有注释解释为什么可以忽略
- **错误包装**：用 `fmt.Errorf("...: %w", err)`，保留 `%w` 以便 `errors.Is/As`
- **哨兵错误 vs 错误类型**：
  - 简单标识用哨兵：`var ErrTimeout = errors.New("timeout")`
  - 需要携带上下文用类型：`type ExecError struct { Cmd string; ExitCode int }`
- **不要 panic**：除非是 init 期的不可恢复错误。库代码永远不 panic。
- **错误日志**：错误**要么处理，要么返回，不要既 log 又 return**（除非在最顶层）

### 4.3 日志（slog）

```go
// ✅ 推荐：结构化 key-value
slog.Info("agent iteration completed",
    "iter", i,
    "goal", goal,
    "tokens_used", tokens)

// ❌ 禁止：字符串拼接
slog.Info(fmt.Sprintf("iter %d done", i))

// ✅ 错误日志
slog.Error("executor failed", "err", err, "cmd", cmd)

级别使用：
  - Debug：开发调试，详细执行轨迹
  - Info：正常状态变更（迭代开始/完成）
  - Warn：可恢复异常（重试中）
  - Error：不可恢复，即将退出/返回错误

禁止在库代码（internal/*）中 SetDefault，只在 main.go 配置一次

4.4 Context
- 所有可能阻塞或 IO 的函数第一个参数必须是 ctx context.Context
- 不要把 ctx 存到 struct 字段（除非是长期运行的 server）
- 不要传 context.Background()，向上游接受
- 子进程：exec.CommandContext(ctx, ...)，禁止用 exec.Command

4.5 函数与文件大小
- 函数：≤ 80 行（lint 强制）
- 文件：目标 ≤ 300 行，超过考虑拆分
- 圈复杂度：≤ 15（lint 强制）

4.6 测试
- 新增功能必须有测试（核心包覆盖率目标 70%+，CLI/main 不强求）
- 表驱动优先：

```
tests := []struct {
    name    string
    input   string
    want    string
    wantErr bool
}{
    {"empty", "", "", true},
    {"normal", "hello", "HELLO", false},
}
for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        got, err := Upper(tt.input)
        if tt.wantErr {
            require.Error(t, err)
            return
        }
        require.NoError(t, err)
        assert.Equal(t, tt.want, got)
    })
}
```

- 断言用 testify：require 用于"失败则不能继续"，assert 用于"失败但继续"
- 不要测试私有函数（通过公共 API 间接测）
- mock：先尝试用接口 + 手写 stub；除非真的复杂，不要急着引入 testify/mock

4.7 并发
- 优先用 channel + goroutine，慎用 sync.Mutex
- 每个 goroutine 必须有明确的退出机制（ctx 取消、close channel）
- 不要泄漏 goroutine：启动 goroutine 时同时考虑"它怎么停"
- 用 go test -race 检测竞态（Makefile 已默认开启）

5. AI 协作规范（重要）

5.1 修改代码前必须做的事
1. 阅读相关文件：不要只看一个函数就动手，先理解所在包的 doc.go / 主要类型
2. 检查依赖方向：参考第 3 节的依赖图，不要引入反向依赖
3. 跑测试确认基线：make test 必须先通过
4. 改动前简述计划：对于跨多文件的修改，先在对话中输出"我打算改 A、B、C，理由是 X"，等用户确认

5.2 代码生成约束
- 优先使用标准库：不要为了"显得专业"引入新依赖
- 新增依赖必须征询：go get 新包前，必须在对话中说明：
  * 为什么标准库不够
  * 候选包对比（至少 2 个）
  * 该包的维护状态、star 数、最近更新
- 不要预先设计：YAGNI 原则。"以后可能用到"不是引入抽象的理由。
- 不要写注释解释明显的代码：注释解释"为什么"，不解释"是什么"

5.3 输出风格
- 每次修改后总结：改了哪些文件、加/减了多少行、为什么
- 失败要诚实：测试没跑通就说没跑通，禁止编造"应该可以工作"
- 不确定就问：遇到歧义优先问用户，不要赌

5.4 禁止行为（红线）
- ❌ 直接修改 go.mod / go.sum（必须通过 go get / go mod tidy）
- ❌ 删除测试文件，或为了让测试通过而修改测试断言（除非用户明确要求）
- ❌ 在 internal/ 包之间引入循环依赖
- ❌ 在没有 ADR 的情况下做架构级变更（如新增顶层包、改包职责划分）
- ❌ 提交（commit）前未运行 make lint && make test
- ❌ 把密钥、API key、token 写进代码或测试 fixture

6. 工作流

6.1 日常开发循环
```
make fmt       # 格式化
make lint      # 静态检查
make test      # 跑测试
make build     # 编译
make run       # 运行
```

6.2 提交前 checklist
- make lint 无 error（warning 视情况）
- make test 全部通过
- 新功能有对应测试
- 如果改了公共 API（包导出符号），更新对应的 doc comment
- 如果做了架构决策，新增 ADR

6.3 Commit 信息规范（约定式提交）
```
<type>(<scope>): <subject>

[optional body]

[optional footer]
```

- type：feat / fix / refactor / test / docs / chore / perf
- scope：包名，如 agent / executor / cli
- subject：50 字以内，祈使句，首字母小写，结尾无句号

例：
```
feat(agent): add retry logic for transient executor errors

When the underlying LLM CLI exits with code 1 within the first 5s,
treat it as transient and retry up to 3 times with exponential backoff.

Refs: docs/adr/0003-retry-strategy.md
```

7. 当前阶段的特别说明

> 项目刚起步，以下规则会随阶段演化调整。

- 优先把核心循环跑通：internal/agent + internal/executor 是当下重点
- 不要在 CLI 上花太多时间：cobra 命令保持最小，先解决"agent 能不能跑"
- 不要急着抽象：第一个版本允许"丑陋但能跑"，重构等有 3 个真实使用场景后再做
- scratchpad 先用文件：不要急着设计复杂的 memory 层，markdown 文件足够

8. 参考资料

Go 官方：Effective Go
项目布局：golang-standards/project-layout（仅参考，不要照搬）
slog 教程：Go Blog: Structured Logging
Ralph Wiggum technique：（待补充原始链接）


9. 本文件的维护
- 每次架构变更后必须更新对应章节
- 新增依赖后必须更新第 2 节
- 新增/移除包后必须更新第 3 节
- 如果某条规范被频繁违反，要么改规范，要么加 lint 规则强制
- 不要让本文件超过 500 行：超过说明该拆成多个文件（如 docs/coding-style.md）

```

---

## 启动操作清单（一次性）

按顺序执行，**今晚就能让 Claude Code 写第一个真实功能**：

```bash
# 1. 初始化仓库
mkdir ralph && cd ralph
git init
go mod init github.com/yourname/ralph

# 2. 创建上述目录结构（手动 mkdir 或写个脚本）
mkdir -p cmd/ralph internal/{agent,executor,prompt,memory,config,cli} pkg testdata docs/adr

# 3. 复制上面所有文件内容到对应位置

# 4. 拉依赖
go mod tidy

# 5. 安装开发工具
make install-tools

# 6. 验证一切正常
make fmt
make lint
make test
make run    # 应该打印 "TODO: run agent with goal=..." 或 cobra usage

# 7. 首次提交
git add .
git commit -m "chore: initial project scaffold with CLAUDE.md"
```



