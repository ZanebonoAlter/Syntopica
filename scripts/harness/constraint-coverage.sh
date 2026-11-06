#!/usr/bin/env bash
# constraint-coverage.sh — 业务路径 → 业务域标签覆盖盘点（enable-unbound-jit-injection D3）
#
# 用途：盘点「某条业务路径被哪些业务域的 doc-impact-applies 标签覆盖」，输出矩阵 + 覆盖率。
#       标签补全前跑一次（基线）、补全后跑一次（对账），两次数字贴 review.md。
#
# 只读边界：只读 docs/reference/flow/*.md 与 docs/reference/standard/**/*.md 头部 15 行的
#   `doc-impact-applies` 标签（与 constraint-injection.ts scanAppliesTags 同语义：
#   单文档首个匹配行即止、信号纯子串匹配 path.includes(signal)、`| section=` 尾段丢弃）。
#   **不写任何文件**。
#
# 样本清单：**固定在脚本内**（不靠外部输入）——分母 = 样本清单长度，保证两次盘点可重复对账。
#   清单只收「业务路径」：后端业务包 + 前端业务功能目录/页面。
#   基础设施目录（cmd/ scripts/ stores/ utils/ types/ middleware/ app 装配层等）**不收**
#   ——它们没有域红线可注入，硬塞只会制造噪音（design D3 原则 2）。
#
# 期望列：每个样本的期望业务域集合（逗号分隔，排序后比对）。多域重叠是**合法结果**
#   （如 topicgraph 同时属 daily-report / semantic-board / topic-graph），不算误匹配；
#   脚本单独列出重叠条目，避免把重叠误当误匹配（design D3 原则 3）。
#
# 用法：
#   bash scripts/harness/constraint-coverage.sh            # 矩阵 + 覆盖率（默认）
#   bash scripts/harness/constraint-coverage.sh --quiet     # 只出覆盖率汇总行
#   bash scripts/harness/constraint-coverage.sh --json      # 机器可读（每行一个 JSON）
#
# 环境覆盖（仅 smoke 用；生产走仓库真实路径）：
#   CONSTRAINT_COVERAGE_DOC_ROOT=<dir>   文档根（其下 flow/ 与 standard/）
#   CONSTRAINT_COVERAGE_SAMPLES=<file>   样本清单文件（覆盖脚本内清单，格式同内嵌清单）
#
# 退出码：0 全部样本命中且期望集合一致；1 有未覆盖或期望不符
set -u

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

FLOW_DIR="docs/reference/flow"
STD_DIR="docs/reference/standard"
if [ -n "${CONSTRAINT_COVERAGE_DOC_ROOT:-}" ]; then
	FLOW_DIR="$CONSTRAINT_COVERAGE_DOC_ROOT/flow"
	STD_DIR="$CONSTRAINT_COVERAGE_DOC_ROOT/standard"
fi
TAG_HEAD_LINES=15

# ---------------------------------------------------------------------------
# 样本清单（固定在脚本内）：<样本路径>|<期望业务域，逗号分隔>
#   期望域写全（含合法重叠）；空期望 = 期望「未覆盖」（当前无此条目，留作回归护栏）。
#   匹配语义与 extension / constraint-digest.sh **完全一致**（纯子串 path.includes(signal)）：
#   目录样本只反映**目录级信号**的覆盖——文件级信号（带文件名）对目录样本不可见
#   （信号含文件名，目录路径不含它）；需要精确到文件时用文件样本。
# ---------------------------------------------------------------------------
SAMPLES=(
	# —— 后端业务包（目录级信号 → 目录样本）——
	"backend-go/internal/dataenrichment/|data-enrichment"
	"backend-go/internal/datasources/|research-data-sources"
	"backend-go/internal/discovery/|discovery"
	"backend-go/internal/reader/|content-enrichment,reading"
	"backend-go/internal/tagmanagement/|semantic-board,topic-graph"
	"backend-go/internal/topicgraph/|daily-report,semantic-board,topic-graph"
	"backend-go/internal/platform/airouter/|ai-summary"
	"backend-go/internal/platform/aihealth/|ai-summary,scheduler"
	"backend-go/internal/platform/aisettings/|ai-summary"
	"backend-go/internal/platform/analysispause/|scheduler"
	"backend-go/internal/platform/notification/|daily-report"
	"backend-go/internal/platform/scheduler/|scheduler"
	"backend-go/internal/platform/ws/|content-enrichment,daily-report"
	# —— 后端文件样本（文件级信号 → 文件样本）——
	"backend-go/internal/admin/handler/scheduler_handler.go|daily-report,scheduler"
	"backend-go/internal/admin/handler/ai_handler.go|ai-summary"
	"backend-go/internal/admin/handler/preferences_handler.go|reading"
	"backend-go/internal/admin/repository/repository.go|ai-summary,reading,scheduler"
	"backend-go/internal/admin/scheduler/job_daily_report.go|daily-report,scheduler"
	"backend-go/internal/admin/scheduler/job_discovery_v2.go|daily-report,discovery,scheduler"
	# —— 前端功能目录（目录级信号 → 目录样本）——
	"front/app/features/ai/|ai-summary"
	"front/app/features/articles/|reading"
	"front/app/features/discovery/|discovery"
	"front/app/features/feeds/|reading"
	"front/app/features/preferences/|reading"
	"front/app/features/search/|reading"
	"front/app/features/shell/|reading"
	"front/app/features/tags/|semantic-board"
	# —— 前端文件样本 ——
	"front/app/features/articles/composables/useContentCompletion.ts|content-enrichment,reading"
	"front/app/features/shell/components/AppHeaderView.vue|reading,scheduler"
	"front/app/features/settings/components/SettingsSectionFeeds.vue|reading"
	"front/app/features/settings/components/SettingsSectionSchedulers.vue|scheduler"
	"front/app/features/settings/components/SettingsSectionAiHealth.vue|ai-summary"
	"front/app/features/settings/components/SettingsSectionPreferences.vue|discovery"
	"front/app/features/settings/components/SettingsSectionDatasources.vue|research-data-sources"
	"front/app/features/tags/components/daily-report/DailyReportDeskShell.vue|daily-report,semantic-board,topic-graph"
	"front/app/features/tags/components/BoardEnrichmentPanel.vue|data-enrichment,semantic-board"
	"front/app/features/tags/components/console/BoardRadarView.vue|semantic-board"
	"front/app/features/tags/components/TopicGraphMergeDialog.vue|semantic-board,topic-graph"
	"front/app/pages/discovery.vue|discovery"
	"front/app/pages/index.vue|reading"
	"front/app/pages/search.vue|reading"
	"front/app/pages/settings.vue|scheduler"
	"front/app/pages/tags/index.vue|semantic-board"
	"front/app/api/semanticBoards.ts|semantic-board"
	"front/app/api/discovery.ts|discovery"
	"front/app/api/scheduler.ts|scheduler"
	"front/app/api/dailyReports.ts|daily-report"
	"front/app/api/datasources.ts|research-data-sources"
	"front/app/api/aiAdmin.ts|ai-summary"
)

if [ -n "${CONSTRAINT_COVERAGE_SAMPLES:-}" ]; then
	SAMPLES=()
	while IFS= read -r line; do
		[ -n "$line" ] || continue
		SAMPLES+=("$line")
	done <"$CONSTRAINT_COVERAGE_SAMPLES"
fi

QUIET=0
JSON=0
for a in "$@"; do
	case "$a" in
		--quiet) QUIET=1 ;;
		--json) JSON=1 ;;
		-h | --help)
			sed -n '2,40p' "$0"
			exit 0
			;;
		*)
			echo "未知参数：$a" >&2
			exit 2
			;;
	esac
done

# ---------------------------------------------------------------------------
# 标签扫描（与 constraint-injection.ts scanAppliesTags / constraint-digest.sh 同语义）
#   只读头部 15 行；单文档首个匹配行即止；信号逗号分隔；`| section=` 尾段丢弃。
# ---------------------------------------------------------------------------
tag_value_of_line() {
	local line="$1" s="" t=""
	s="${line#"${line%%[![:space:]]*}"}"
	case "$s" in
		"<!--"*)
			s="${s#<!--}"
			s="${s#"${s%%[![:space:]]*}"}"
			;;
	esac
	case "$s" in
		doc-impact-applies:*) s="${s#doc-impact-applies:}" ;;
		*) return 1 ;;
	esac
	s="${s#"${s%%[![:space:]]*}"}"
	s="${s%"${s##*[![:space:]]}"}"
	[ -n "$s" ] || return 1
	case "$s" in
		*"-->")
			t="${s%-->}"
			t="${t%"${t##*[![:space:]]}"}"
			[ -n "$t" ] && s="$t"
			;;
	esac
	printf '%s' "$s"
}

parse_tag_value() {
	local raw="$1" sig_part p
	sig_part="${raw%%|*}"
	local IFS=','
	local -a parts=()
	read -r -a parts <<<"$sig_part"
	for p in "${parts[@]}"; do
		p="${p#"${p%%[![:space:]]*}"}"
		p="${p%"${p##*[![:space:]]}"}"
		[ -n "$p" ] && printf '%s\n' "$p"
	done
}

doc_files() {
	find "$FLOW_DIR" -maxdepth 2 -name '*.md' ! -name 'README.md' 2>/dev/null | LC_ALL=C sort
	find "$STD_DIR" -maxdepth 3 -name '*.md' ! -name 'README.md' 2>/dev/null | LC_ALL=C sort
}

# 每行输出：<rel>\t<信号>（同一文档的信号连续出现）
scan_tag_docs() {
	local f rel head line val sig
	while IFS= read -r f; do
		[ -e "$f" ] || continue
		rel="${f#./}"
		head="$(head -n "$TAG_HEAD_LINES" "$f" 2>/dev/null)"
		while IFS= read -r line; do
			line="${line%$'\r'}"
			val="$(tag_value_of_line "$line")" || continue
			while IFS= read -r sig; do
				[ -n "$sig" ] || continue
				printf '%s\t%s\n' "$rel" "$sig"
			done < <(parse_tag_value "$val")
			break
		done <<<"$head"
	done < <(doc_files)
}

# 路径 → 命中的业务域集合（排序、逗号分隔）
domains_for_path() {
	local path="$1" doc sig
	local -a hits=()
	while IFS=$'\t' read -r doc sig; do
		[ -n "$doc" ] || continue
		case "$path" in
			*"$sig"*) ;;
			*) continue ;;
		esac
		case "$doc" in
			"$FLOW_DIR"/*.md)
				hits+=("$(basename "$doc" .md)")
				;;
		esac
	done < <(scan_tag_docs)
	if [ "${#hits[@]}" -eq 0 ]; then
		printf ''
		return 0
	fi
	printf '%s\n' "${hits[@]}" | LC_ALL=C sort -u | paste -sd, -
}

join_display() { # 逗号串 → 「、」分隔（人读；不用 tr——tr 按字节处理会切坏 UTF-8）
	printf '%s' "$1" | sed 's/,/、/g'
}

# ---------------------------------------------------------------------------
# 主流程
# ---------------------------------------------------------------------------
total=0
covered=0
expected_uncovered=0
uncovered=0
mismatch=0
overlap=0
declare -a UNCOVERED_LIST=()
declare -a MISMATCH_LIST=()
declare -a OVERLAP_LIST=()

if [ "$JSON" -eq 0 ] && [ "$QUIET" -eq 0 ]; then
	echo "## 路径标签覆盖矩阵（enable-unbound-jit-injection D3）"
	echo
	echo "| 样本路径 | 命中业务域 | 期望业务域 | 判定 |"
	echo "| --- | --- | --- | --- |"
fi

for entry in "${SAMPLES[@]}"; do
	[ -n "$entry" ] || continue
	sample="${entry%%|*}"
	expect="${entry#*|}"
	got="$(domains_for_path "$sample")"
	total=$((total + 1))
	verdict="✅"
	if [ -z "$got" ] && [ -z "$expect" ]; then
		# 预期未覆盖（空期望 = 负样本）：不算失败，但不入覆盖率分子
		verdict="➖ 预期未覆盖"
		expected_uncovered=$((expected_uncovered + 1))
	elif [ -z "$got" ]; then
		verdict="⚠️ 未覆盖"
		uncovered=$((uncovered + 1))
		UNCOVERED_LIST+=("$sample")
	elif [ "$got" != "$expect" ]; then
		verdict="❌ 期望不符"
		mismatch=$((mismatch + 1))
		MISMATCH_LIST+=("$sample｜命中=$(join_display "$got")｜期望=$(join_display "$expect")")
	else
		covered=$((covered + 1))
	fi
	# 多域重叠（合法结果，单独标注——不算误匹配）
	if [ -n "$got" ]; then
		cnt="$(printf '%s' "$got" | tr ',' '\n' | grep -c . || true)"
		if [ "${cnt:-0}" -gt 1 ]; then
			overlap=$((overlap + 1))
			OVERLAP_LIST+=("$sample → $(join_display "$got")")
		fi
	fi
	if [ "$JSON" -eq 1 ]; then
		printf '{"sample":"%s","hit":"%s","expect":"%s","verdict":"%s"}\n' \
			"$sample" "$got" "$expect" "$verdict"
	elif [ "$QUIET" -eq 0 ]; then
		printf '| `%s` | %s | %s | %s |\n' \
			"$sample" "$(join_display "$got" | sed 's/^$/（无）/')" \
			"$(join_display "$expect" | sed 's/^$/（无）/')" "$verdict"
	fi
done

if [ "$JSON" -eq 0 ]; then
	echo
	if [ "$QUIET" -eq 0 ]; then
		echo "### 合法多域重叠（${overlap} 条，非误匹配）"
		if [ "${#OVERLAP_LIST[@]}" -gt 0 ]; then
			for o in "${OVERLAP_LIST[@]}"; do echo "- $o"; done
		else
			echo "- （无）"
		fi
		echo
	fi
	pct="0"
	den=$((total - expected_uncovered))
	if [ "$den" -gt 0 ]; then
		pct="$(awk -v c="$covered" -v t="$den" 'BEGIN{printf "%.1f", (c/t)*100}')"
	fi
	echo "覆盖率：${covered}/${den}（${pct}%）；未覆盖 ${uncovered} 条；期望不符 ${mismatch} 条；预期未覆盖负样本 ${expected_uncovered} 条（样本总数 ${total}）"
	if [ "${#UNCOVERED_LIST[@]}" -gt 0 ]; then
		echo "未覆盖样本："
		for u in "${UNCOVERED_LIST[@]}"; do echo "- $u"; done
	fi
	if [ "${#MISMATCH_LIST[@]}" -gt 0 ]; then
		echo "期望不符样本："
		for m in "${MISMATCH_LIST[@]}"; do echo "- $m"; done
	fi
fi

[ "$uncovered" -eq 0 ] && [ "$mismatch" -eq 0 ] && exit 0
exit 1
