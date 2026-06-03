## 阶段 1：基础（无大模型调用）
1. ☐ 为上述所有接口搭建空壳包。确保 `go build` 通过。
2. ☐ 为 `memory.ExtractStateBlock` 编写测试：
     - 正常路径
     - 缺少代码块
     - 重复代码块
     - 格式错误的 JSON
     - 未知状态
3. ☐ 为 `verify.Resolve` 编写测试：
     - 用户设置了命令
     - 未设置命令，但 Makefile 中包含 `test:`
     - 未设置命令，也没有 Makefile → 报错
     - 未设置命令，Makefile 中没有 `test:` → 报错
4. ☐ 为 `agent.StateStore` 编写测试：
     - Init + Get
     - Update 操作是原子的（并发 Update）
     - Load 往返一致性
5. ☐ 为 `config.Load` 编写测试：
     - 缺少文件 → 使用默认值
     - 部分 YAML → 与默认值合并
     - YAML 无效 → 报错

## 阶段 2：基于 Mock 的循环
6. ☐ 创建 `internal/executor/mock.go`，实现可编程的 MockExecutor。
7. ☐ 使用 MockExecutor 端到端串联 `agent.Loop`。
8. ☐ 编写集成测试：mock 执行器返回 "done" → 循环运行验证
     → 验证（mock 版）通过 → 成功。
9. ☐ 测试：验证失败两次，第三次通过。确认反馈注入正常工作。
10. ☐ 测试：当达到 max_iterations 时触发 GUARD。

## 阶段 3：真实 Claude Code
11. ☐ 实现 `claudecode.ClaudeCodeExecutor.Run`：
      - exec.CommandContext
      - 将标准输出同时写入文件和解析器
      - SIGINT 信号传播
12. ☐ 按照 events.go 规范实现 `parseStreamJSON`。
13. 针对真实 `claude` 运行冒烟测试，目标为简单任务（如“创建 hello.txt”）。
14. ☐ 打标签的集成测试（`//go:build integration`）。

## 阶段 4：完善
15. ☐ result.json 写入器
16. ☐ 美观的控制台输出（彩色、进度显示）
17. ☐ `ralph version` 同时显示 claude 版本
18. ☐ 包含快速上手指南的 README
