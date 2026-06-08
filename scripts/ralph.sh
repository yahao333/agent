#!/usr/bin/env bash
#
# ralph.sh — Ralph Wiggum 风格 AI 编程循环
# 让 Claude Code 在循环中自主处理任务，直到 PRD 完成或达到迭代上限。
#
# 用法:
#   ./ralph.sh <iterations>            # 跑 N 次迭代
#   ./ralph.sh <iterations> --dry-run  # 只打印要执行的命令，不实际调用
#   ./ralph.sh -h | --help             # 帮助
#
# 环境变量:
#   PLAN_FILE      计划文件路径       (默认: prd.json)
#   PROGRESS_FILE  进度文件路径       (默认: progress.txt)
#   MAX_BUDGET_USD 每次迭代预算上限    (默认: 不设，传给 --max-budget-usd)
#   RALPH_PROMPT   自定义 prompt 文件 (默认: scripts/prompts/ralph.md)
#   RALPH_LOG_DIR  日志目录           (默认: .ralph/logs)
#
# 退出码:
#   0  PRD 完成 (找到 <promise>COMPLETE</promise>)
#   2  使用错误 (参数错、依赖缺失、文件不存在)
#   3  claude 调用失败
#   130 SIGINT / 143 SIGTERM

set -euo pipefail

# ===== 颜色（检测到 TTY 时启用） =====
if [[ -t 1 ]]; then
  C_BOLD=$'\033[1m'
  C_DIM=$'\033[2m'
  C_GREEN=$'\033[32m'
  C_YELLOW=$'\033[33m'
  C_RED=$'\033[31m'
  C_RESET=$'\033[0m'
else
  C_BOLD=""; C_DIM=""; C_GREEN=""; C_YELLOW=""; C_RED=""; C_RESET=""
fi

# ===== 帮助 =====
usage() {
  cat <<EOF
${C_BOLD}ralph.sh${C_RESET} — Ralph Wiggum 风格 AI 编程循环

${C_BOLD}用法:${C_RESET}
  $0 <iterations> [options]

${C_BOLD}参数:${C_RESET}
  iterations               迭代次数上限 (正整数)

${C_BOLD}选项:${C_RESET}
  --dry-run                只打印命令，不实际执行
  -h, --help               显示此帮助

${C_BOLD}环境变量:${C_RESET}
  PLAN_FILE      计划文件     (默认: prd.json)
  PROGRESS_FILE  进度文件     (默认: progress.txt)
  MAX_BUDGET_USD 单次预算上限  (默认: 不限制)
  RALPH_PROMPT   prompt 文件  (默认: scripts/prompts/ralph.md)
  RALPH_LOG_DIR  日志目录     (默认: .ralph/logs)

${C_BOLD}示例:${C_RESET}
  $0 5
  $0 30 --dry-run
  PLAN_FILE=my.json PROGRESS_FILE=notes.txt $0 10
EOF
}

# ===== 参数解析 =====
if [[ $# -lt 1 ]]; then
  usage
  exit 2
fi

ITERATIONS=""
DRY_RUN=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      usage
      exit 0
      ;;
    --dry-run)
      DRY_RUN=1
      shift
      ;;
    -[0-9]*)
      # 负数走数值校验分支，给出更准确的错误
      if [[ -n "$ITERATIONS" ]]; then
        echo "${C_RED}错误:${C_RESET} 多次指定了迭代次数" >&2
        usage >&2
        exit 2
      fi
      ITERATIONS="$1"
      shift
      ;;
    -*)
      echo "${C_RED}错误:${C_RESET} 未知选项 '$1'" >&2
      usage >&2
      exit 2
      ;;
    *)
      if [[ -n "$ITERATIONS" ]]; then
        echo "${C_RED}错误:${C_RESET} 多次指定了迭代次数" >&2
        usage >&2
        exit 2
      fi
      ITERATIONS="$1"
      shift
      ;;
  esac
done

# ===== 校验迭代次数 =====
if ! [[ "$ITERATIONS" =~ ^[0-9]+$ ]] || [[ "$ITERATIONS" -lt 1 ]]; then
  echo "${C_RED}错误:${C_RESET} 迭代次数必须是正整数，得到: '$ITERATIONS'" >&2
  exit 2
fi

# ===== 配置 =====
PLAN_FILE="${PLAN_FILE:-prd.json}"
PROGRESS_FILE="${PROGRESS_FILE:-progress.txt}"
MAX_BUDGET_USD="${MAX_BUDGET_USD:-}"
# 解析 RALPH_PROMPT 相对路径时基于脚本所在目录
RALPH_PROMPT="${RALPH_PROMPT:-$(cd "$(dirname "$0")" && pwd)/prompts/ralph.md}"
RALPH_LOG_DIR="${RALPH_LOG_DIR:-.ralph/logs}"

# ===== 预检 =====
if ! command -v claude >/dev/null 2>&1; then
  echo "${C_RED}错误:${C_RESET} 未找到 'claude' 命令。请先安装 Claude Code CLI。" >&2
  exit 2
fi

if [[ ! -f "$RALPH_PROMPT" ]]; then
  echo "${C_RED}错误:${C_RESET} 找不到 prompt 文件: $RALPH_PROMPT" >&2
  exit 2
fi

if [[ ! -f "$PLAN_FILE" ]]; then
  echo "${C_RED}错误:${C_RESET} 找不到计划文件: $PLAN_FILE" >&2
  echo "请先创建 PRD 文件，或通过 PLAN_FILE 环境变量指定。" >&2
  exit 2
fi

if [[ ! -f "$PROGRESS_FILE" ]]; then
  : > "$PROGRESS_FILE"
  echo "${C_DIM}已创建进度文件: $PROGRESS_FILE${C_RESET}" >&2
fi

# ===== 准备日志目录 =====
mkdir -p "$RALPH_LOG_DIR"

# ===== 信号处理 =====
INTERRUPTED=0
cleanup() {
  local sig=$1
  INTERRUPTED=1
  echo "" >&2
  echo "${C_YELLOW}收到 $sig 信号，退出循环${C_RESET}" >&2
  echo "  进度: $PROGRESS_FILE" >&2
  echo "  日志: $RALPH_LOG_DIR" >&2
  exit 130
}
trap 'cleanup SIGINT' INT
trap 'cleanup SIGTERM' TERM

# ===== 启动信息 =====
started_at=$(date +%s)
echo "${C_BOLD}=========================================${C_RESET}"
echo "${C_BOLD}  Ralph AI 编程循环启动${C_RESET}"
echo "${C_BOLD}=========================================${C_RESET}"
echo "  计划文件:   $PLAN_FILE"
echo "  进度文件:   $PROGRESS_FILE"
echo "  迭代上限:   $ITERATIONS"
echo "  日志目录:   $RALPH_LOG_DIR"
echo "  prompt:     $RALPH_PROMPT"
[[ -n "$MAX_BUDGET_USD" ]] && echo "  单次预算:   \$$MAX_BUDGET_USD"
[[ "$DRY_RUN" -eq 1 ]] && echo "  ${C_YELLOW}模式: DRY-RUN${C_RESET}"
echo "${C_BOLD}=========================================${C_RESET}"
echo ""

# ===== 加载 prompt 模板并替换占位符 =====
prompt_template=$(cat "$RALPH_PROMPT")
prompt="${prompt_template//\{PLAN_FILE\}/$PLAN_FILE}"
prompt="${prompt//\{PROGRESS_FILE\}/$PROGRESS_FILE}"

# ===== 构造 claude 命令 =====
build_claude_cmd() {
  local -a cmd
  cmd=(claude --dangerously-skip-permissions)
  if [[ -n "$MAX_BUDGET_USD" ]]; then
    cmd+=(--max-budget-usd "$MAX_BUDGET_USD")
  fi
  cmd+=(-p "@${PLAN_FILE} @${PROGRESS_FILE} ${prompt}")
  printf '%q ' "${cmd[@]}"
}

# ===== 去除 ANSI 转义 =====
strip_ansi() {
  sed 's/\x1b\[[0-9;?]*[a-zA-Z]//g'
}

# ===== 主循环 =====
for ((i = 1; i <= ITERATIONS; i++)); do
  iter_started=$(date +%s)
  iter_log="$RALPH_LOG_DIR/iter-$(printf '%03d' "$i").log"

  echo "${C_BOLD}=========================================${C_RESET}"
  echo "${C_BOLD}  迭代 $i / $ITERATIONS${C_RESET}"
  echo "${C_BOLD}=========================================${C_RESET}"
  echo "  日志: $iter_log"

  if [[ "$DRY_RUN" -eq 1 ]]; then
    echo "${C_DIM}[dry-run] 跳过实际执行${C_RESET}"
    continue
  fi

  # 实际调用
  set +e
  result=$(claude --dangerously-skip-permissions \
    $([[ -n "$MAX_BUDGET_USD" ]] && printf -- '--max-budget-usd %q' "$MAX_BUDGET_USD") \
    -p "@${PLAN_FILE} @${PROGRESS_FILE} ${prompt}" \
    2> >(tee -a "$iter_log.stderr" >&2))
  claude_rc=$?
  set -e

  # 把 stdout 落到日志（tee 不需要，因为 result 已经在内存）
  printf '%s\n' "$result" | tee "$iter_log"

  iter_elapsed=$(( $(date +%s) - iter_started ))
  echo ""
  echo "${C_DIM}迭代 $i 耗时: ${iter_elapsed}s · claude 退出码: $claude_rc${C_RESET}"

  # claude 调用失败
  if [[ $claude_rc -ne 0 ]]; then
    echo "${C_RED}错误:${C_RESET} claude 调用失败 (退出码 $claude_rc)。详见 $iter_log" >&2
    echo "你可以修复后重新运行此脚本继续。" >&2
    exit 3
  fi

  # 完成检测：去除 ANSI + 空白后匹配
  clean=$(printf '%s' "$result" | strip_ansi)
  if grep -qi '<promise>COMPLETE</promise>' <<<"$clean"; then
    total=$(( $(date +%s) - started_at ))
    echo ""
    echo "${C_GREEN}=========================================${C_RESET}"
    echo "${C_GREEN}  ✓ PRD 完成${C_RESET}"
    echo "${C_GREEN}  共执行 $i / $ITERATIONS 次迭代，耗时 ${total}s${C_RESET}"
    echo "${C_GREEN}=========================================${C_RESET}"
    exit 0
  fi

  echo ""
  echo "${C_DIM}迭代 $i 完成。继续下一次迭代...${C_RESET}"
done

total=$(( $(date +%s) - started_at ))
echo ""
echo "${C_YELLOW}=========================================${C_RESET}"
echo "${C_YELLOW}  已达到最大迭代次数 ($ITERATIONS)，共耗时 ${total}s${C_RESET}"
echo "${C_YELLOW}  审查: $PROGRESS_FILE${C_RESET}"
echo "${C_YELLOW}  日志: $RALPH_LOG_DIR${C_RESET}"
echo "${C_YELLOW}  重新运行此脚本继续工作${C_RESET}"
echo "${C_YELLOW}=========================================${C_RESET}"
exit 0
