#!/usr/bin/env bash
# gate-status.sh — 门禁状态只读查询入口（aggregate-concurrent-gate-warns / gate-status-query）
#
# 单一职责：读 facts 库 gate.check 记账，展示各门禁命令的最新红绿状态、失败特征摘要、
# 记账时间与会话。严格只读：sqlite3 file:...?mode=ro 连接，不写账本、不写任何文件、
# 不执行任何门禁命令（与 concurrency-status.sh 的只读安全口径一致）。
#
# 用法：bash scripts/harness/gate-status.sh
#
# 定位：quality-gate 边沿触发注入收敛后的「红态知情三路兜底」之一（design D5）——
#   会话内首次全文（已见过）→ 本入口（随时主动查）→ spec-gate 归档前全绿硬门禁。
# 正确性依据：门禁记账协议转红全量、转绿必记翻转锚点 → 每 cmd 最新一条忠实反映当前态
# （skill harness-facts 记账协议）。
#
# 退出码：0 = 查询成功（展示红态也算成功）；1 = 账本库缺失/损坏/sqlite3 不可用（stderr 说明）。
set -uo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || true)"
if [ -z "$ROOT" ]; then
	echo "gate-status: 非 git 仓库，无法定位账本库" >&2
	exit 1
fi
DB="$ROOT/.pi/harness/events.db"

if ! command -v sqlite3 >/dev/null 2>&1; then
	echo "gate-status: sqlite3 CLI 不可用，无法查询账本库" >&2
	exit 1
fi
if [ ! -f "$DB" ]; then
	echo "gate-status: 账本库不存在（$DB），无门禁记账可查询" >&2
	exit 1
fi

# 每 cmd 最新一条 gate.check（窗口函数，id 最大 = 最新）；红态是查询结果不是查询失败。
# 注意：diag 空串必须在 SQL 侧填占位——IFS=tab 的 read 会把相邻制表符视作单一
# 分隔符吞掉空字段（实测踩坑），列非空才能正确对位。
SQL="
SELECT json_extract(payload, '\$.cmd'),
       COALESCE(json_extract(payload, '\$.ok'), 0),
       CASE
         WHEN COALESCE(json_extract(payload, '\$.ok'), 0) = 1 THEN '-'
         ELSE COALESCE(NULLIF(json_extract(payload, '\$.diag'), ''), '(无失败摘要)')
       END,
       ts,
       COALESCE(NULLIF(session_id, ''), '-')
FROM (
  SELECT id, ts, session_id, payload,
         ROW_NUMBER() OVER (PARTITION BY json_extract(payload, '\$.cmd') ORDER BY id DESC) AS rn
  FROM events
  WHERE kind = 'gate.check' AND json_valid(payload)
)
WHERE rn = 1 AND json_extract(payload, '\$.cmd') IS NOT NULL
ORDER BY json_extract(payload, '\$.cmd')"

if ! ROWS="$(sqlite3 "file:$DB?mode=ro" -separator "$(printf '\t')" "$SQL" 2>&1)"; then
	echo "gate-status: 账本库打开/查询失败（$DB）：$ROWS" >&2
	exit 1
fi

echo "== 门禁状态（只读） =="
echo "时刻：$(date -u +%Y-%m-%dT%H:%M:%SZ)"
echo "库：$DB"
echo
if [ -z "$ROWS" ]; then
	echo "（暂无门禁记账——本会话尚未有门禁命令落过账，或账本刚初始化）"
	exit 0
fi

red=0
green=0
while IFS="$(printf '\t')" read -r cmd ok diag ts sid; do
	[ -z "${cmd:-}" ] && continue
	if [ "$ok" = "1" ]; then
		state="绿"
		green=$((green + 1))
	else
		state="红"
		red=$((red + 1))
	fi
	# diag 已是单行 ≤512B（truncateDiagGate 同源，SQL 侧占位非空）；展示再截 160 列防刷屏
	diag="$(printf '%s' "$diag" | cut -c1-160)"
	printf '[%s] %-30s %s  %s  session %s\n' "$state" "$cmd" "$diag" "$ts" "$sid"
done <<<"$ROWS"

echo
echo "合计：红 $red / 绿 $green（红态不改退出码；归档前全绿硬门禁由 spec-gate 检查①把守）"
exit 0
