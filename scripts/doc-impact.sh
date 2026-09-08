#!/usr/bin/env bash
# doc-impact.sh — 选项式文档影响门禁（docs-harness-consolidation 引入）
#
# 把「文档要更新哪些」从回忆题变成选择题。四个子命令：
#   menu                      打印 8 个固定文档域选项菜单
#   suggest [--base <ref>]    按 git diff 启发式预勾选 + 命中理由 + 声明注释模板
#   verify <change-dir> [--base <ref>]  归档前对账（声明↔git diff + 反向启发式 + 文件存在性）
#     反向启发式（疑似遗漏/声明 none 但命中）双轨输入（coordinate-concurrent-changes）：
#     事实库存在该 change 的 edit.map 归属集合且非空 → 只扫归属集合（其他 active
#     change 的脏文件不再干扰）；无记录/空/库不可用 → 回退全树 git diff（现状行为）。
#     「声明了未更新」对账始终用全树（文档文件以 git 为准）。
#     doc-impact-excuse 已退役（输入收窄后跨 change 误报源消失）；既有注释保留解析兼容不判 FAIL
#
# 详见 openspec/changes/docs-harness-consolidation/design.md §1。
# WSL bash 可跑。退出码：0 过；1 有失败（仅 verify 会非零）。

set -u

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

# backend domain 白名单（与 check-standards.sh B 段一致，权威：standard/backend/package-layout.md）
DOMAIN_WHITELIST="admin dataenrichment reader tagmanagement topicgraph"

# ---------------------------------------------------------------------------
# 公共：取本 change 的改动文件集合（已跟踪改动 + 未跟踪新文件）
#   $1 = base ref（默认 HEAD）。工作区/暂存/未跟踪都算「本次改动」。
# ---------------------------------------------------------------------------
changed_files() {
	local base="${1:-HEAD}"
	# 已跟踪：git diff（含未 stage 的工作区改动 + 已 stage）
	# core.quotepath=false 让中文路径原样输出（否则 git 八进制转义，verify 比对失败）
	# core.checkStat=minimal：WSL DrvFS(/mnt/*) 上完整 stat 极慢（29s→2s），
	# 对启发式门禁而言 mtime+size 粒度足够
	git -c core.checkStat=minimal -c core.quotepath=false diff --name-only "$base" 2>/dev/null
	git -c core.quotepath=false diff --cached --name-only 2>/dev/null
	# 未跟踪新文件
	git -c core.quotepath=false ls-files --others --exclude-standard 2>/dev/null
}

# ---------------------------------------------------------------------------
# 归属轨取数（coordinate-concurrent-changes）：事实库该 change 最新一条 edit.map
#   的 paths（一行一路径）。只读连接（mode=ro）；sqlite3 缺失/库不存在/查询失败/
#   无记录 → 空串（调用方回退全树，fail-open 语义）。
#   注：只认最新一条快照（quality-gate 落库侧并集，取最新 = 完整集合）。
# ---------------------------------------------------------------------------
ownership_paths() {
	local change_name="$1" root db
	root="$(git rev-parse --show-toplevel 2>/dev/null)" || return 0
	db="$root/.pi/harness/events.db"
	command -v sqlite3 >/dev/null 2>&1 || return 0
	[ -f "$db" ] || return 0
	sqlite3 "$db" -readonly "SELECT DISTINCT json_each.value
		FROM events AS e, json_each(e.payload, '$.paths')
		WHERE e.kind = 'edit.map' AND e.change = '${change_name//\'/\'\'}'
		  AND e.id = (SELECT MAX(id) FROM events
		              WHERE kind = 'edit.map' AND change = '${change_name//\'/\'\'}')" 2>/dev/null | head -2000 || true
}

# ---------------------------------------------------------------------------
# 黑名单（fix-doc-impact-misattribution）：工具自管文件 + 仓库级共享文档。
# 启发式输入（verify 两轨 / suggest 全树）统一过滤——被剔除路径不触发“疑似遗漏”
# 与“声明 none 但命中”；checkbox 显式声明的对账不受影响（以 git 全树为准）。
# 与 lib/edit-map.ts 采集侧 isToolManagedPath 镜像（共享文档仅在消费侧剔除，
# 保留在归属地图供并发冲突感知），smoke 同 fixture 断言两侧一致（防漂移）。
# ---------------------------------------------------------------------------
filter_blacklist() {
	grep -vE \
		-e '^openspec/changes/archive/' \
		-e '^openspec/changes/[^/]+/\.openspec\.yaml$' \
		-e '^(AGENTS\.md|front/AGENTS\.md|backend-go/AGENTS\.md|docs/reference/constraints-index\.md)$' \
		|| true
}

# ---------------------------------------------------------------------------
# 启发式：某文档域是否被改动文件集合命中
#   $1 = 域 key；$2 = 预先算好的改动文件列表（换行分隔）。命中则 echo 命中的
#   文件路径（可能多个），否则空。
#   ⚠ 调用方必须预计算文件列表传入——曾在函数内每次重跑 changed_files，
#   verify/suggest 的 7 域循环导致 7+ 次全量 git 扫描（DrvFS 上每次 ~30s）。
# ---------------------------------------------------------------------------
heuristic_hit() {
	local domain="$1" files="$2"
	case "$domain" in
	flow)
		echo "$files" | grep -E 'backend-go/internal/(admin|reader|tagmanagement|topicgraph|dataenrichment)/service/' ||
			echo "$files" | grep -E '^front/app/features/'
		;;
	api)
		echo "$files" | grep -E 'backend-go/internal/[^/]+/handler/' ||
			echo "$files" | grep -E 'backend-go/internal/app/router\.go'
		;;
	database)
		echo "$files" | grep -E 'backend-go/internal/models/' ||
			{ echo "$files" | grep -E '^backend-go/.*\.go$' | xargs -r grep -lE 'AutoMigrate|CREATE TABLE|ALTER TABLE' 2>/dev/null; }
		;;
	architecture)
		echo "$files" | grep -E 'backend-go/internal/app/' ||
			echo "$files" | grep -E 'backend-go/internal/platform/tracing/'
		;;
	standard)
		echo "$files" | grep -E '(^|/)\.golangci\.yml|eslint\.config\.js' ||
			echo "$files" | grep -E '^docs/reference/standard/'
		;;
	configuration)
		# openspec/ 前缀排除（fix-doc-impact-misattribution D3）：openspec CLI 自管
		# 的 yaml 元数据（如 configure-*/.openspec.yaml 路径含 “configure” 字样）
		# 不属于配置变更；真实配置文件命中面不变。
		echo "$files" | grep -vE '^openspec/' | grep -E 'config.*\.ya?ml' ||
			echo "$files" | grep -E 'backend-go/internal/platform/config/'
		;;
	deployment)
		echo "$files" | grep -E 'Dockerfile|docker-compose' ||
			echo "$files" | grep -E '(^|/)(init\.ps1|init\.sh)$'
		;;
	none) ;;
	esac
}

# ---------------------------------------------------------------------------
# menu：打印 8 域选项
# ---------------------------------------------------------------------------
cmd_menu() {
	cat <<'EOF'
文档影响域（固定 8 选项，apply 启动时按 git diff 启发式预勾选）：
  flow          业务链路文档 docs/reference/flow/
  api           API 参考 docs/reference/api/
  database      数据库 docs/reference/database/
  architecture  架构 docs/reference/architecture/
  standard      代码规约 docs/reference/standard/
  configuration 配置 docs/reference/configuration.md
  deployment    部署 docs/reference/deployment.md
  none          纯内部重构（声明须附理由）
EOF
}

# ---------------------------------------------------------------------------
# suggest：预勾选 + 命中理由 + 声明模板
# ---------------------------------------------------------------------------
cmd_suggest() {
	local base="HEAD"
	[ "${1:-}" = "--base" ] && base="${2:-HEAD}"
	echo "文档域预勾选（--base $base）："
	local declared="" files
	files="$(changed_files "$base" | sort -u | filter_blacklist)"
	for domain in flow api database architecture standard configuration deployment; do
		local hit
		hit="$(heuristic_hit "$domain" "$files")"
		if [ -n "$hit" ]; then
			local first
			first="$(echo "$hit" | head -1)"
			printf '  [x] %-14s — 命中: %s\n' "$domain" "$first"
			declared="$declared $domain"
		else
			printf '  [ ] %-14s\n' "$domain"
		fi
	done
	if [ -z "${declared// /}" ]; then
		echo '  [x] none         — 未命中任何启发式（若是纯内部重构，声明 none 并附理由）'
		echo
		echo '请确认后写入 <change>/tasks.md「文档」节第一行：'
		echo '  <!-- doc-impact: none(纯内部重构，无对外行为变化) -->'
	else
		echo
		echo '请确认后写入 <change>/tasks.md「文档」节第一行：'
		echo "  <!-- doc-impact:${declared} -->"
	fi
}

# ---------------------------------------------------------------------------
# verify：归档前对账（5 规则，任一 FAIL 则退出码 1）
#   两层解析：(a) 域注释 <!-- doc-impact: ... -->；(b) 文档节 checkbox 文件路径
# ---------------------------------------------------------------------------
cmd_verify() {
	local change_dir="${1:-}"
	local base="HEAD"
	shift || true
	while [ $# -gt 0 ]; do
		case "$1" in
		--base)
			base="$2"
			shift 2
			;;
		*) shift ;;
		esac
	done

	if [ -z "$change_dir" ] || [ ! -f "$change_dir/tasks.md" ]; then
		echo "用法: doc-impact.sh verify <change-dir> [--base <ref>]" >&2
		echo "FAIL: change 目录无效或无 tasks.md" >&2
		exit 1
	fi

	local tasks="$change_dir/tasks.md"
	local v_fail=0
	local fails=""

	add_fail() {
		fails="${fails}  - $1\n"
		v_fail=$((v_fail + 1))
	}

	# --- 解析 (a) 域注释 ---
	local decl_line
	decl_line="$(grep -m1 '<!-- doc-impact:' "$tasks" 2>/dev/null || true)"
	if [ -z "$decl_line" ]; then
		add_fail "未声明 doc-impact（apply 启动时跑 suggest 补声明）"
		# 无声明则后续规则无意义，直接出结果
		printf '%b' "$fails" >&2
		echo "verify: $change_dir — $v_fail FAIL" >&2
		exit 1
	fi
	# 提取域列表（去掉 none(理由) 的括号内容）
	local decl_raw
	decl_raw="$(echo "$decl_line" | sed -E 's/.*<!-- doc-impact: *//; s/ *-->.*//; s/none\([^)]*\)/none/')"
	local declared_domains=" $decl_raw "

	# --- 解析豁免声明 <!-- doc-impact-excuse: domain=理由; ... --> ---
	# 豁免的域不触发"疑似遗漏"（巬发式过严/多 change 脏工作树的误报兑底）
	local excuse_line excused_domains
	excuse_line="$(grep -m1 '<!-- doc-impact-excuse:' "$tasks" 2>/dev/null || true)"
	excused_domains="$(echo "$excuse_line" | sed -E 's/.*<!-- doc-impact-excuse: *//; s/ *-->.*//' | tr ';' '\n' | sed -E 's/=.*//' | tr -d ' ' | tr '\n' ' ')"
	excused_domains=" $excused_domains "

	# --- 解析 (b) 文档节 checkbox 文件路径 ---
	# 取「## N. 文档」节内 - [ ] 后的路径
	local declared_files
	declared_files="$(awk '/^## [0-9]+\. 文档/{f=1;next} /^## [0-9]+\./{f=0} f' "$tasks" |
		grep -E '^[[:space:]]*- \[[ x]\]' |
		grep -oE 'docs/[^ "`)]+\.md' | sort -u)"

	local changed
	changed="$(changed_files "$base" | sort -u)"

	# --- 双轨输入（coordinate-concurrent-changes）：反向启发式优先用 edit.map 归属集合 ---
	# 归属轨：事实库该 change 最新一条 edit.map 的 paths（一行一路径）；无记录/空集合/
	# sqlite3 或库不可用 → 回退全树 $changed（冷启动兼容，现状行为）。规则 2/5（声明了
	# 未更新/路径不存在）始终用全树 $changed（文档对账以 git 为准）。
	local heuristic_input heuristic_track="fallback"
	heuristic_input="$(ownership_paths "$(basename "$change_dir")")"
	if [ -n "$(printf '%s' "$heuristic_input" | grep .)" ]; then
		heuristic_track="ownership"
	else
		heuristic_input="$changed"
	fi
	# 黑名单统一过滤（两轨同源，fix-doc-impact-misattribution）：工具自管文件与仓库级
	# 共享文档不参与域启发式（归属轨集合可能含存量污染条目，消费侧同步兑底）。
	heuristic_input="$(printf '%s\n' "$heuristic_input" | filter_blacklist)"

	# 启发式命中按轨道分级（fix-doc-impact-misattribution D4）：归属轨=会话真实编辑
	# （证据强）→ FAIL；回退轨=无法归属的全树视角（证据弱，他人脏文件可能误入）→
	# 仅 stderr 提示，不判 FAIL 不改退出码。规则 1/2/5 的 FAIL 语义不变。
	report_heuristic_hit() {
		# $1=domain $2=失败文案
		if [ "$heuristic_track" = "ownership" ]; then
			add_fail "$2"
		else
			echo "[提示] 无法归属: $1（全树回退仅提示，归属轨无记录）" >&2
		fi
	}

	# --- 规则 4：声明 none 但启发式命中（同样吃豁免） ---
	if echo "$declared_domains" | grep -qw none; then
		for domain in flow api database architecture standard configuration deployment; do
			if [ -n "$(heuristic_hit "$domain" "$heuristic_input")" ] && ! echo "$excused_domains" | grep -qw "$domain"; then
				report_heuristic_hit "$domain" "声明 none 但启发式命中 $domain"
			fi
		done
	else
		# --- 规则 3：反向启发式命中未声明域 ---
		for domain in flow api database architecture standard configuration deployment; do
			if [ -n "$(heuristic_hit "$domain" "$heuristic_input")" ] && ! echo "$declared_domains" | grep -qw "$domain" && ! echo "$excused_domains" | grep -qw "$domain"; then
				report_heuristic_hit "$domain" "疑似遗漏: 改了 ${domain} 相关代码未声明 $domain"
			fi
		done
	fi

	# --- 规则 2 & 5：checkbox 声明的文件 ---
	if [ -n "$declared_files" ]; then
		while IFS= read -r f; do
			[ -z "$f" ] && continue
			# 规则 5：路径不存在
			if [ ! -e "$f" ]; then
				add_fail "声明的文档不存在: $f"
				continue
			fi
			# 规则 2：不在 changed 集合，且未在 git 历史提交
			# （事后补归档场景：change 改动已 commit 进主线，工作树 diff 为空；
			#   只要文件曾被提交即视为「已更新」，避免 base=HEAD 对已提交改动的误报。
			#   gitignored 文档（如 docs/research/）双重兑底全 miss：不在 git status 也无 git 历史
			#   → 只要工作树存在即视为已更新，否则永远误报「声明了未更新」）
			if ! echo "$changed" | grep -qxF "$f"; then
				if ! git check-ignore -q "$f" 2>/dev/null && \
					! git log --oneline -- "$f" 2>/dev/null | grep -q .; then
					add_fail "声明了未更新: $f"
				fi
			fi
		done <<<"$declared_files"
	fi

	# --- 输出 ---
	if [ "$v_fail" -gt 0 ]; then
		printf '%b' "$fails" >&2
		echo "verify: $change_dir — $v_fail FAIL" >&2
		exit 1
	fi
	echo "verify: $change_dir — 通过（声明:${declared_domains} 文件:$(printf '%s\n' "$declared_files" | grep -c .) 个 启发式:${heuristic_track}）"
	exit 0
}

# ---------------------------------------------------------------------------
# 入口
# ---------------------------------------------------------------------------
sub="${1:-}"
case "$sub" in
menu)
	shift
	cmd_menu "$@"
	;;
suggest)
	shift
	cmd_suggest "$@"
	;;
context)
	echo "已退役：context 子命令已由 constraint-injection extension 取代（harness 层每 turn 自动注入 system prompt）。"
	echo "suggest（文档域预勾选）与 verify（归档对账）不受影响。"
	exit 1
	;;
verify)
	shift
	cmd_verify "$@"
	;;
"" | -h | --help | help)
	sed -n '2,12p' "$0"
	echo
	echo "用法: bash scripts/doc-impact.sh <menu|suggest|verify> [...]（context 已由 constraint-injection extension 取代）"
	;;
*)
	echo "未知子命令: $sub" >&2
	exit 2
	;;
esac
