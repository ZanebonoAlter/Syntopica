#!/usr/bin/env bash
# concurrency-status.sh — 并发 change 态势拉取（coordinate-concurrent-changes）
#
# 单一入口、严格只读（不写事实库、不写 git、mode=ro SQLite 连接）。
# 拉模式设计：时变数据不进 system prompt（前缀缓存保护，design D1），
# 仅在 Agent 主动查询 / spec-gate 检查⑤' / 编排六步派发前以命令输出形态出现。
#
# 用法：
#   bash scripts/concurrency-status.sh [change]           人读三段输出
#     ① 活跃 change 清单（名称、绑定 session、最近活动、归属文件数）
#     ② git 脏文件 × 归属地图对照（归属本 change / 归属其他 active change /
#        无归属三类分列，冲突文件带 ⚠ 标记）
#     ③ 近 6h gate.check 验证流水摘要（最多 20 条）
#   bash scripts/concurrency-status.sh --check <change>    机器可读（spec-gate 检查⑤'数据源）
#     exit 0 = 干净（无脏文件 / 脏文件全归属本 change）
#     exit 2 = 存在归属其他 active change 的未 commit 脏文件（stdout 一行 JSON）
#     exit 3 = 冷启动跳过（全部脏文件无归属 / sqlite3 或库不可用 / 参数缺失）
set -uo pipefail

# ---------------------------------------------------------------------------
# 基础收集（每项只算一次，全段复用——DrvFS git 扫描秒级，见 doc-impact.sh 同款注释）
# ---------------------------------------------------------------------------
ROOT="$(git rev-parse --show-toplevel 2>/dev/null || true)"
if [ -z "$ROOT" ]; then
	echo "concurrency-status: 非 git 仓库，无法判定" >&2
	exit 3
fi
DB="$ROOT/.pi/harness/events.db"

# git 脏文件（tracked diff + staged + untracked；中文路径原样）
dirty_files() {
	git -c core.quotepath=false diff --name-only HEAD 2>/dev/null
	git -c core.quotepath=false diff --cached --name-only 2>/dev/null
	git -c core.quotepath=false ls-files --others --exclude-standard 2>/dev/null
}
DIRTY="$(dirty_files | sort -u)"

# 活跃 change（openspec/changes/ 下非 archive 目录）
active_changes() {
	local dir="$ROOT/openspec/changes"
	[ -d "$dir" ] || return 0
	find "$dir" -mindepth 1 -maxdepth 1 -type d ! -name archive -printf '%f\n' 2>/dev/null | sort
}
ACTIVE="$(active_changes)"

# 事实库可用性：sqlite3 + 库文件均在才可查归属
DB_OK=0
if command -v sqlite3 >/dev/null 2>&1 && [ -f "$DB" ]; then
	DB_OK=1
fi

# 每个活跃 change 的最新 edit.map 快照行化（change \t path）；
# json_each 直接在 SQL 里拆 paths 数组，bash 侧零 JSON 解析依赖。
# 归属其他但已 archive 的 change 天然不在此集合（只查 active）。
ownership_rows() {
	# shellcheck disable=SC2016
	sqlite3 "$DB" -readonly -separator '	' \
		"SELECT e.change, json_each.value
		 FROM events AS e, json_each(e.payload, '\$.paths')
		 WHERE e.kind = 'edit.map'
		   AND e.id IN (SELECT MAX(id) FROM events WHERE kind = 'edit.map' GROUP BY change)
		 ORDER BY e.change, json_each.value" 2>/dev/null
}

# ---------------------------------------------------------------------------
# 归属对照：脏文件 → 归属 change 集合
# 输出三个全局变量（字符串，换行分隔）+ 冲突计数
# ---------------------------------------------------------------------------
OWN_MINE=""    # 归属本 change（含仅归属本 change 的文件）
OWN_FOREIGN="" # 归属其他 active change（格式：path → change1, change2）
OWN_NONE=""    # 无任何归属
CONFLICT=0
build_classification() {
	local target="$1" row path ch owners
	local map="" # path\x09ch1,ch2 累积表（仅 active change 的归属；已 archive 的视同无归属，explore-findings §8）
	if [ "$DB_OK" -eq 1 ]; then
		while IFS=$'\t' read -r ch path; do
			[ -z "$ch" ] && continue
			printf '%s\n' "$ACTIVE" | grep -qx "$ch" || continue # 归属 change 不在 active 列表 → 忽略
			map+="$path"$'\t'"$ch"$'\n'
		done <<<"$(ownership_rows)"
	fi
	while IFS= read -r path; do
		[ -z "$path" ] && continue
		owners="$(printf '%s' "$map" | awk -F'\t' -v p="$path" '$1==p {print $2}' | sort -u | paste -sd, -)"
		if [ -z "$owners" ]; then
			OWN_NONE+="$path"$'\n'
		elif [ "$owners" = "$target" ]; then
			OWN_MINE+="$path"$'\n'
		else
			local n_owners
			n_owners="$(printf '%s' "$owners" | tr ',' '\n' | grep -c . || true)"
			[ "$n_owners" -gt 1 ] && CONFLICT=$((CONFLICT + 1))
			OWN_FOREIGN+="$path → $owners"$'\n'
		fi
	done <<<"$(printf '%s\n' "$DIRTY")"
}

json_escape() { printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'; }

# ---------------------------------------------------------------------------
# --check 模式（机器可读）
# ---------------------------------------------------------------------------
cmd_check() {
	local target="$1"
	if ! printf '%s\n' "$ACTIVE" | grep -qx "$target"; then
		echo "concurrency-status --check: change '$target' 不在活跃列表（或 openspec/changes 不存在）" >&2
		exit 3
	fi
	build_classification "$target"

	if [ -z "$(printf '%s' "$DIRTY" | grep . || true)" ]; then
		exit 0 # 工作树干净
	fi
	if [ -n "$(printf '%s' "$OWN_FOREIGN" | grep . || true)" ]; then
		# JSON：foreignFiles 数组 + files→changes 映射
		local jfiles="" jmap="" line path owners
		while IFS= read -r line; do
			[ -z "$line" ] && continue
			path="${line%% → *}"
			owners="${line#* → }"
			[ -n "$jfiles" ] && jfiles+=","
			jfiles+="\"$(json_escape "$path")\""
			jmap+="\"$(json_escape "$path")\":\"$(json_escape "$owners")\","
		done <<<"$(printf '%s' "$OWN_FOREIGN")"
		jmap="${jmap%,}"
		printf '{"foreignFiles":[%s],"changes":{%s}}\n' "$jfiles" "$jmap"
		exit 2
	fi
	if [ -z "$(printf '%s%s' "$OWN_MINE" "$OWN_NONE" | grep . || true)" ] && [ "$DB_OK" -eq 0 ]; then
		exit 3 # 事实库不可用 → fail-open 跳过
	fi
	# 脏文件全部无归属 → 冷启动跳过；有归属本 change 的 → 干净
	if [ -n "$(printf '%s' "$OWN_MINE" | grep . || true)" ]; then
		exit 0
	fi
	exit 3
}

# ---------------------------------------------------------------------------
# 人读模式（三段）
# ---------------------------------------------------------------------------
cmd_human() {
	local target="${1:-}" t0 ts sid cmd ok n_active ch
	echo "== 并发态势${target:+（视角：$target）} =="
	t0="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
	echo "时刻：$t0"

	# ① 活跃 change 清单
	n_active="$(printf '%s\n' "$ACTIVE" | grep -c . || true)"
	echo
	echo "① 活跃 change（${n_active} 个）"
	if [ "$n_active" -eq 0 ]; then
		echo "  （无活跃 change）"
	fi
	for ch in $ACTIVE; do
		local info="(事实库不可用)"
		if [ "$DB_OK" -eq 1 ]; then
			local last_ts last_sid n_files
			last_ts="$(sqlite3 "$DB" -readonly "SELECT ts FROM events WHERE change='$ch' ORDER BY id DESC LIMIT 1" 2>/dev/null || true)"
			last_sid="$(sqlite3 "$DB" -readonly "SELECT session_id FROM events WHERE change='$ch' ORDER BY id DESC LIMIT 1" 2>/dev/null | cut -c1-8 || true)"
			n_files="$(sqlite3 "$DB" -readonly "SELECT COALESCE(json_array_length(payload,'\$.paths'),0) FROM events WHERE change='$ch' AND kind='edit.map' ORDER BY id DESC LIMIT 1" 2>/dev/null || true)"
			[ -z "$n_files" ] && n_files=0
			info="session ${last_sid:-?}，最近活动 ${last_ts:-无记录}，归属文件 ${n_files}"
		fi
		echo "  - $ch（$info）"
	done

	# ② 脏文件归属对照
	build_classification "${target:-__none__}"
	local n_dirty
	n_dirty="$(printf '%s\n' "$DIRTY" | grep -c . || true)"
	echo
	echo "② git 脏文件归属（${n_dirty} 个，冲突 ${CONFLICT} 个）"
	if [ "$n_dirty" -eq 0 ]; then
		echo "  （工作树干净）"
	fi
	local sec
	for sec in MINE FOREIGN NONE; do
		local label="" body=""
		case "$sec" in
		MINE)
			[ -z "$target" ] && continue
			label="归属本 change（$target）"
			body="$OWN_MINE"
			;;
		FOREIGN) label="归属其他 active change" ; body="$OWN_FOREIGN" ;;
		NONE) label="无归属（冷启动/纯 bash 会话/归档后无主）" ; body="$OWN_NONE" ;;
		esac
		if [ -n "$(printf '%s' "$body" | grep . || true)" ]; then
			echo "  $label："
			printf '%s' "$body" | while IFS= read -r line; do
				[ -n "$line" ] && echo "    - $line"
			done
		fi
	done
	[ "$CONFLICT" -gt 0 ] && echo "  ⚠ 冲突文件（多 change 触碰，commit 拆分需人工确认）${CONFLICT} 个"

	# ③ 近 6h 验证流水
	echo
	echo "③ 近 6h gate.check 流水（最多 20 条）"
	if [ "$DB_OK" -eq 1 ]; then
		local since rows
		since="$(date -u -d '6 hours ago' +%Y-%m-%dT%H:%M:%S 2>/dev/null || date -u -v-6H +%Y-%m-%dT%H:%M:%S 2>/dev/null || true)"
		if [ -n "$since" ]; then
			rows="$(sqlite3 "$DB" -readonly -separator '	' \
				"SELECT substr(ts,12,5), COALESCE(change,'-'), json_extract(payload,'\$.cmd'), json_extract(payload,'\$.ok')
				 FROM events WHERE kind='gate.check' AND ts >= '$since'
				 ORDER BY id DESC LIMIT 20" 2>/dev/null || true)"
			if [ -n "$rows" ]; then
				while IFS=$'\t' read -r ts ch cmd ok; do
					[ "$ok" = "1" ] && ok="ok" || ok="FAIL"
					echo "  - $ts [$ch] $cmd $ok"
				done <<<"$(printf '%s' "$rows")"
			else
				echo "  （近 6h 无 gate.check 记录）"
			fi
		fi
	else
		echo "  （事实库不可用）"
	fi
}

# ---------------------------------------------------------------------------
# 入口
# ---------------------------------------------------------------------------
case "${1:-}" in
--check)
	if [ $# -lt 2 ] || [ -z "${2:-}" ]; then
		echo "用法: bash scripts/concurrency-status.sh --check <change>" >&2
		exit 3
	fi
	cmd_check "$2"
	;;
"") cmd_human "" ;;
-*) echo "未知选项: $1（支持 --check <change>）" >&2; exit 3 ;;
*) cmd_human "$1" ;;
esac
