#!/bin/bash

# afk-ralph.sh
# AFK（离开键盘）模式 - 在循环中运行，无人值守
# 适用于批量工作、低风险任务
#
# 用法: ./afk-ralph.sh [迭代次数]
# 默认: 30 次迭代

set -e

ITERATIONS="${1:-30}"

echo "========================================="
echo "  Ralph AFK 模式"
echo "========================================="
echo "将运行 $ITERATIONS 次迭代，无人值守。"
echo "请确保:"
echo "  1. 你的提示已经过 HITL 模式验证"
echo "  2. 反馈循环 (类型/测试/lint) 已设置好"
echo "  3. 使用 Docker 沙箱进行安全隔离"
echo "  4. 任务范围明确，停止条件清晰"
echo "========================================="
echo ""

read -p "确认开始 AFK 模式? (y/N) " confirm
if [[ "$confirm" != "y" && "$confirm" != "Y" ]]; then
  echo "已取消。"
  exit 0
fi

# 运行主循环
./ralph.sh "$ITERATIONS"

# 完成后可以集成通知 (如 WhatsApp/邮件等)
echo ""
echo "AFK Ralph 已完成。请审查 git log 和 $PROGRESS_FILE。"
