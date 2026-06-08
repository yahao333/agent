#!/bin/bash

# ralph-once.sh
# HITL（人在回路）模式 - 仅运行单次迭代
# 用于学习 Ralph、优化提示，或处理高风险任务
#
# 用法: ./ralph-once.sh

set -e

PLAN_FILE="${PLAN_FILE:-prd.json}"
PROGRESS_FILE="${PROGRESS_FILE:-progress.txt}"

echo "========================================="
echo "  Ralph HITL 模式 - 单次迭代"
echo "========================================="
echo "你将观察每一步，在需要时干预。"
echo ""

# 调用主脚本，只运行一次
./ralph.sh 1
