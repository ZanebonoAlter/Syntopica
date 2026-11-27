#!/usr/bin/env bash
# 按 change 的 stage.md 输出应派发的 agent 名。
# 用法: stage-agent.sh <change> [--cheap] [--explain]
#   --cheap    输出便宜档建议（gpt-6.1-terra 贵，探索类任务可用它降本）
#   --explain  附一行人类可读理由
# fail-open：任何读取失败都输出 general-purpose 并 exit 0，不让路由问题挡住开发。
set -uo pipefail

CHANGE=""; CHEAP=0; EXPLAIN=0
for a in "$@"; do
  case "$a" in
    --cheap) CHEAP=1 ;;
    --explain) EXPLAIN=1 ;;
    -*) echo "未知参数: $a" >&2 ;;
    *) [ -z "$CHANGE" ] && CHANGE="$a" ;;
  esac
done

fallback() {
  [ "$EXPLAIN" = 1 ] && echo "general-purpose  # ${1}"
  [ "$EXPLAIN" = 0 ] && echo "general-purpose"
  exit 0
}

[ -z "$CHANGE" ] && fallback "未给 change 名"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
STAGE_FILE="$ROOT/openspec/changes/$CHANGE/stage.md"
[ -f "$STAGE_FILE" ] || fallback "无 stage.md（legacy change）"

# marker 判定与 constraint-injection 的 parseStageMarker 对齐（review R-5）：
# 全文检索；允许 `<!--stage:x-->` 无空格、允许行尾多余空白；出现多条即判非法。
# （旧实现只看前 20 行 + 取首条，与权威解析器五类输入分叉。）
MARKERS="$(grep -oE '<!--[[:space:]]*stage[[:space:]]*:[[:space:]]*[A-Za-z]+[[:space:]]*-->' "$STAGE_FILE" 2>/dev/null)"
COUNT="$(printf '%s' "$MARKERS" | grep -c . 2>/dev/null || echo 0)"
[ "$COUNT" = 0 ] && fallback "stage marker 缺失"
[ "$COUNT" -gt 1 ] && fallback "stage marker 出现 $COUNT 条（应为 1 条）"

STAGE="$(printf '%s' "$MARKERS" | sed -E 's/.*stage[[:space:]]*:[[:space:]]*//; s/[[:space:]]*-->$//' | tr 'A-Z' 'a-z')"

case "$STAGE" in
  explore|propose|design|implement|verify|archive) ;;
  *) fallback "stage 取值非法: $STAGE" ;;
esac

AGENT="stage-$STAGE"; WHY="stage.md 声明 $STAGE → stage-$STAGE"

# --cheap 只给建议，不改路由：把贵档降级到权限最宽的 general-purpose 会让「降本」
# 等于「放开写权限」，正好抵消阶段档存在的意义（review R-3）。
if [ "$CHEAP" = 1 ]; then
  case "$STAGE" in
    explore|propose|verify)
      WHY="$WHY（降本建议：该档配 gpt-6.1-terra，贵的价档；要降本请改 .pi/agents/stage-$STAGE.md 的 model 一行——别改用 general-purpose，它 write/edit/bash 全开）" ;;
    *) WHY="$WHY（该档已是中档，无需降本）" ;;
  esac
fi

[ "$EXPLAIN" = 1 ] && printf '%s  # %s\n' "$AGENT" "$WHY"
[ "$EXPLAIN" = 0 ] && printf '%s\n' "$AGENT"
exit 0
