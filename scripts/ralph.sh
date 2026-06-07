#!/bin/bash

# ralph.sh
# 使用 Ralph Wiggum 进行 AI 编程的循环脚本
# 让 AI 编程 CLI 在循环中自主处理任务，直到完成
#
# 用法: ./ralph.sh <迭代次数>
# 示例: ./ralph.sh 10

set -e

# ===== 参数检查 =====
if [ -z "$1" ]; then
  echo "Usage: $0 <iterations>"
  echo ""
  echo "示例:"
  echo "  $0 5    # 小型任务，运行 5 次迭代"
  echo "  $0 30   # 大型任务，运行 30 次迭代"
  echo ""
  echo "提示: AFK 模式下始终限制迭代次数，避免无限循环。"
  exit 1
fi

ITERATIONS=$1
PLAN_FILE="${PLAN_FILE:-prd.json}"
PROGRESS_FILE="${PROGRESS_FILE:-progress.txt}"

# ===== 启动信息 =====
echo "========================================="
echo "   Ralph Wiggum AI 编程循环启动"
echo "========================================="
echo "计划文件: $PLAN_FILE"
echo "进度文件: $PROGRESS_FILE"
echo "最大迭代次数: $ITERATIONS"
echo "========================================="
echo ""

# ===== 确保进度文件存在 =====
if [ ! -f "$PROGRESS_FILE" ]; then
  touch "$PROGRESS_FILE"
  echo "已创建进度文件: $PROGRESS_FILE"
fi

# ===== 检查计划文件 =====
if [ ! -f "$PLAN_FILE" ]; then
  echo "警告: 找不到计划文件 $PLAN_FILE"
  echo "请创建一个 PRD 文件来定义任务范围。"
  exit 1
fi

# ===== Ralph 循环 =====
# 每次迭代，用以下提示运行 Claude Code。
# 提示包含了反馈循环、进度跟踪、小步快跑等关键技巧。

for ((i = 1; i <= $ITERATIONS; i++)); do
  echo ""
  echo "========================================="
  echo "  迭代 $i / $ITERATIONS"
  echo "========================================="

  # 使用 Docker 沙箱隔离 AFK Ralph，确保安全
  # 沙箱只挂载当前目录，无法触碰主目录、SSH 密钥或系统文件
  result=$(docker sandbox run claude -p \
    "@${PLAN_FILE} @${PROGRESS_FILE} \
    \
    你是 Ralph，一个长时间运行的自主 AI 编程代理。 \
    \
    === 工作流程 === \
    1. 决定下一个要处理的任务。 \
       应该是 YOU 决定优先级最高的那个， \
       不一定是列表中的第一个。 \
       按以下顺序优先处理: \
         a. 架构决策和核心抽象 \
         b. 模块之间的集成点 \
         c. 未知未知和技术探针工作 \
         d. 标准功能和实现 \
         e. 打磨、清理和快速获胜 \
    \
    2. 探索代码库，理解当前状态。 \
    \
    3. 实现该功能。保持更改小而聚焦: \
       - 每次提交一个逻辑更改 \
       - 如果任务感觉太大，将其分解为子任务 \
       - 优先多次小提交，而不是一次大提交 \
       - 质量优先于速度 \
    \
    4. 检查所有反馈循环 (必须全部通过): \
       - TypeScript: npm run typecheck \
       - 测试: npm run test \
       - Lint: npm run lint \
       如果任何反馈循环失败，不要提交。先修复问题。 \
    \
    5. 将你的进度追加到 ${PROGRESS_FILE}: \
       - 完成的任务和 PRD 项引用 \
       - 做出的关键决策和推理 \
       - 更改的文件 \
       - 任何阻碍或给下一次迭代的备注 \
       保持条目简洁。 \
    \
    6. 对该功能进行 git 提交。 \
    \
    === 质量标准 === \
    这个代码库会比你活得更久。你走的每一个捷径都会成为 \
    别人的负担。对抗熵。让代码库比你发现它时更好。 \
    \
    === 范围控制 === \
    只处理一个功能。不要试图一次做完所有事。 \
    \
    === 完成标准 === \
    如果在实现功能时，你注意到 ${PLAN_FILE} 中所有工作 \
    都已完成 (所有 passes 字段为 true)，输出 \
    <promise>COMPLETE</promise>。 \
    ")

  echo "$result"

  # ===== 检查完成标记 =====
  if [[ "$result" == *"<promise>COMPLETE</promise>"* ]]; then
    echo ""
    echo "========================================="
    echo "  ✓ PRD 完成，退出。"
    echo "  共执行了 $i 次迭代。"
    echo "========================================="
    exit 0
  fi

  echo ""
  echo "迭代 $i 完成。继续下一次迭代..."
done

echo ""
echo "========================================="
echo "  已达到最大迭代次数 ($ITERATIONS)"
echo "  请审查 $PROGRESS_FILE 查看进度"
echo "  必要时重新运行此脚本继续工作"
echo "========================================="
