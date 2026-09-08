#!/usr/bin/env bash
# check-standards.sh — L1 代码规范结构验收（归档前随《开发执行规范》§11.4 运行）
#
# 纯静态校验，不触发编译。WSL bash 可跑。检查维度：
#   A. 文档完整性（standard/ flow/ architecture/map.md 关键文件存在）
#   B. 后端结构（golangci 配置、domain 白名单、三层包结构）
#   C. 前端结构（ESLint 配置、Token 三层、双主题）
#   D. 防孤立引用（每个 standard/*.md 被至少一处 AGENTS.md / README 引用）
#   E. flow 变更溯源链接（archive change 被某 flow 文档「变更溯源」表引用，归档后校验）
#   H. model tag 守门（Top3 密集文件禁止 GORM tag 里的 not null，约束由显式迁移兜底）
#
# 用法： bash scripts/check-standards.sh [--change <changeName>]
# 退出码：0 全过；1 有失败或参数非法。

set -u

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

# --change 仅收窄 F 段（active change 的 doc-impact 对账）；其余 A-E/G-H 仍全仓运行。
# 无参保留人工全仓巡检语义。拒绝路径、未知参数及缺参，不能静默退回全仓。
CHANGE_NAME=""
if [ "$#" -eq 0 ]; then
	:
elif [ "$#" -eq 2 ] && [ "$1" = "--change" ]; then
	CHANGE_NAME="$2"
	if [ -z "$CHANGE_NAME" ]; then
		echo "错误：--change 缺少 change 名称。用法：bash scripts/check-standards.sh [--change <changeName>]" >&2
		exit 1
	fi
	case "$CHANGE_NAME" in
	*/* | *\\*)
		echo "错误：--change 只接受 change 目录名，不能包含路径分隔符。" >&2
		exit 1
		;;
	esac
	if [[ ! "$CHANGE_NAME" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]]; then
		echo "错误：--change 的 change 名称非法。" >&2
		exit 1
	fi
	if [ ! -d "openspec/changes/$CHANGE_NAME" ]; then
		echo "错误：目标 change 不存在：$CHANGE_NAME" >&2
		exit 1
	fi
else
	if [ "${1:-}" = "--change" ]; then
		echo "错误：--change 必须且只能携带一个 change 名称。用法：bash scripts/check-standards.sh [--change <changeName>]" >&2
	else
		echo "错误：未知参数：${1:-}。用法：bash scripts/check-standards.sh [--change <changeName>]" >&2
	fi
	exit 1
fi

PASS=0
FAIL=0

ok() {
	echo "  [OK]   $1"
	PASS=$((PASS + 1))
}
fail() {
	echo "  [FAIL] $1"
	FAIL=$((FAIL + 1))
}

check_file() { [ -f "$1" ] && ok "存在 $1" || fail "缺失 $1"; }

echo "== A. 文档完整性 =="
# standard/ 权威层
for f in \
	docs/reference/standard/README.md \
	docs/reference/standard/frontend/theming.md \
	docs/reference/standard/frontend/code-style.md \
	docs/reference/standard/frontend/lint.md \
	docs/reference/standard/frontend/testing.md \
	docs/reference/standard/backend/package-layout.md \
	docs/reference/standard/backend/code-style.md \
	docs/reference/standard/backend/lint.md \
	docs/reference/standard/backend/testing.md \
	docs/reference/standard/shared/commit-pr.md \
	docs/reference/flow/README.md \
	docs/reference/flow/reading.md \
	docs/reference/flow/content-enrichment.md \
	docs/reference/flow/ai-summary.md \
	docs/reference/flow/daily-report.md \
	docs/reference/flow/topic-graph.md \
	docs/reference/flow/semantic-board.md \
	docs/reference/flow/scheduler.md \
	docs/reference/flow/data-enrichment.md \
	docs/reference/architecture/map.md; do
	check_file "$f"
done

# flow 文档五段式结构校验（docs-harness-consolidation 引入）
for f in docs/reference/flow/*.md; do
	[ -f "$f" ] || continue
	[ "$(basename "$f")" = "README.md" ] && continue
	missing=""
	for sec in 需求说明 链路设计 业务约束与不变量 代码入口 变更溯源; do
		grep -q "^## $sec" "$f" || missing="$missing $sec"
	done
	if [ -z "$missing" ]; then
		ok "$(basename "$f") 五段式齐全"
	else
		fail "$(basename "$f") 缺五段式:$missing"
	fi
done

echo ""
echo "== B. 后端结构 =="
# golangci 配置
check_file backend-go/.golangci.yml

# domain 白名单（权威：standard/backend/package-layout.md）
WHITELIST="admin dataenrichment reader tagmanagement topicgraph"

# 1) 每个"业务 domain 签名"（同时有 routes.go + handler/）必须在白名单内
for d in backend-go/internal/*/; do
	name="$(basename "$d")"
	if [ -f "$d/routes.go" ] && [ -d "$d/handler" ]; then
		# 是 domain 签名
		case " $WHITELIST " in
		*" $name "*) ok "domain $name 在白名单内且有三层结构" ;;
		*) fail "domain $name 有 routes.go+handler/ 但不在白名单（先登记到 standard/backend/package-layout.md）" ;;
		esac
	fi
done

# 2) 白名单内每个 domain 都真实存在且有 handler/
for name in $WHITELIST; do
	d="backend-go/internal/$name"
	if [ ! -d "$d" ]; then
		fail "白名单 domain $name 目录不存在"
		continue
	fi
	[ -d "$d/handler" ] && ok "$name/handler/ 存在" || fail "$name 缺 handler/ 子包"
done

echo ""
echo "== C. 前端结构 =="
check_file front/eslint.config.js

CSS="front/app/assets/css/main.css"
check_file "$CSS"
if [ -f "$CSS" ]; then
	grep -q -- "--raw-" "$CSS" && ok "Token Layer1 (Primitive --raw-*)" || fail "缺 Primitive token (--raw-*)"
	grep -q -- "--color-" "$CSS" && ok "Token Layer2 (Semantic --color-*)" || fail "缺 Semantic token (--color-*)"
	grep -q "data-theme" "$CSS" && ok "主题切换机制 (data-theme)" || fail "缺 data-theme 主题切换"
	if grep -q "editorial" "$CSS" && grep -q "dark" "$CSS"; then
		ok "双主题定义 (editorial + dark)"
	else
		fail "缺双主题 (editorial + dark)"
	fi
fi

echo ""
echo "== D. 防孤立引用 =="
# 每个 standard/*.md（README 除外）必须被至少一处 AGENTS.md / README 引用
REF_FILES="front/AGENTS.md backend-go/AGENTS.md AGENTS.md docs/reference/standard/README.md docs/reference/开发执行规范.md"
for f in docs/reference/standard/frontend/*.md docs/reference/standard/backend/*.md docs/reference/standard/shared/*.md; do
	base="$(basename "$f")"
	if grep -rq "$base" $REF_FILES 2>/dev/null; then
		ok "被引用 $base"
	else
		fail "孤立文档 $base 未被任何 AGENTS.md / README 引用"
	fi
done

echo ""
echo "== E. flow 变更溯源链接（归档后校验，见《开发执行规范》§12.2）=="
# 新流程生效日（2026-06-29）之后的 archive change 必须被 flow 文档的变更溯源表引用；
# 历史存量免校验，避免一次性爆 FAIL。
CUTOFF="2026-06-29"
# 溯源宽限期（fix-doc-impact-misattribution D5）：§12 流程为“归档后补溯源”，宽限窗内
# 的债不 block 其他 change 的归档；超期未溯源仍 FAIL（债由归档门禁催收）。
# date 不可用（非 GNU date）时宽限检查 fail-open 跳过，保持现状催收语义。
GRACE_DAYS=3
GRACE_CUTOFF="$(date -d "-${GRACE_DAYS} days" +%F 2>/dev/null || true)"
FLOW_DIR="docs/reference/flow"
if [ -d "openspec/changes/archive" ]; then
	for d in openspec/changes/archive/*/; do
		[ -d "$d" ] || continue
		name="$(basename "$d")"
		arch_date="${name:0:10}"
		# 只校验生效日及之后的 archive（YYYY-MM-DD 字典序比较 = 日期比较）
		if [[ "$arch_date" < "$CUTOFF" ]]; then
			continue
		fi
		# §12.2 豁免：tasks.md「文档」节声明「无 flow 影响」的 change 不触及任何业务 flow，免溯源校验
		if [ -f "$d/tasks.md" ] && grep -q "无 flow 影响" "$d/tasks.md" 2>/dev/null; then
			ok "豁免溯源 $name（tasks.md 声明无 flow 影响）"
			continue
		fi
		# 溯源宽限期：归档日期在 GRACE_DAYS 天内的免检（字典序比较，同 CUTOFF 手法）
		if [ -n "$GRACE_CUTOFF" ] && [[ "$arch_date" > "$GRACE_CUTOFF" ]]; then
			ok "宽限期内免检 $name（归档未满 ${GRACE_DAYS} 天，溯源按 §12 归档后流程补）"
			continue
		fi
		if grep -rq "$name" "$FLOW_DIR"/*.md 2>/dev/null; then
			ok "已溯源 $name"
		else
			fail "未溯源 $name（在 docs/reference/flow/*.md 变更溯源表补一行链接回该 archive）"
		fi
	done
fi

echo ""
echo "== F. doc-impact 声明对账（见《开发执行规范》§11.4）=="
# 只校验已声明 doc-impact 的 change（本 capability 首次引入于 docs-harness-consolidation，
# 此前的 active change 无声明属正常，跳过；新 change 声明了才对账）。
if [ -f scripts/doc-impact.sh ]; then
	if [ -n "$CHANGE_NAME" ]; then
		change_dirs=("openspec/changes/$CHANGE_NAME/")
	else
		change_dirs=(openspec/changes/*/)
	fi
	for d in "${change_dirs[@]}"; do
		[ -d "$d" ] || continue
		name="$(basename "$d")"
		[ "$name" = "archive" ] && continue
		# 过渡期：未声明 doc-impact 的旧 change 跳过
		if [ -f "$d/tasks.md" ] && grep -q '<!-- doc-impact:' "$d/tasks.md" 2>/dev/null; then
			if bash scripts/doc-impact.sh verify "$d" >/dev/null 2>&1; then
				ok "doc-impact 通过 $name"
			else
				fail "doc-impact 失败 $name（跑 bash scripts/doc-impact.sh verify $d 看详情）"
			fi
		else
			ok "跳过 $name（未声明 doc-impact）"
		fi
	done
else
	fail "scripts/doc-impact.sh 不存在，F 段无法运行"
fi

echo ""
echo "== G. 导航层文档死链检查 =="
# 只查导航层：docs/README.md、docs/reference/*.md（一级）、flow/README.md、architecture/map.md
check_md_links() {
	local file="$1" dir link target
	dir="$(dirname "$file")"
	while IFS= read -r link; do
		case "$link" in http* | mailto* | \#*) continue ;; esac
		target="${link%%#*}"
		[ -e "$dir/$target" ] || fail "死链 $file → $link"
	done < <(grep -oE '\]\([^)]+\.md\)' "$file" 2>/dev/null | sed -E 's/^\]\(//; s/\)$//')
}
NAV_FILES="docs/README.md docs/reference/flow/README.md docs/reference/architecture/map.md"
for f in docs/reference/*.md; do [ -f "$f" ] && NAV_FILES="$NAV_FILES $f"; done
for f in $NAV_FILES; do
	[ -f "$f" ] || continue
	check_md_links "$f"
done

echo ""
echo "== H. model tag 守门（Top3 密集文件禁止 not null）=="
# 规范（standard/backend/code-style.md「GORM model tag 与迁移」）：显式迁移管的表，
# model tag 只写字段名/类型/json，不写 not null——让显式迁移唯一管 DB 约束。
# 这 3 个文件已由迁移 20260723_0001 把约束收敛到 DB 层，tag 里不得再出现 not null。
# 注：serializer:json 字段的 default:'xxx' 是必要例外（影响 GORM 零值省略），本段不禁 default。
MODEL_TAG_FILES="backend-go/internal/models/ai_models.go backend-go/internal/models/topic_graph.go backend-go/internal/models/semantic_label.go"
for f in $MODEL_TAG_FILES; do
	if [ ! -f "$f" ]; then
		fail "缺失 $f（H 段无法扫描）"
		continue
	fi
	# 只匹配 GORM tag 里的 not null（排除 Go 代码注释/字符串）。gorm tag 形如 `gorm:"...;not null;..."`
	if grep -qE 'gorm:"[^"]*\bnot null\b' "$f"; then
		count=$(grep -cE 'gorm:"[^"]*\bnot null\b' "$f")
		fail "$(basename "$f") 仍有 $count 处 not null（应由迁移 20260723_0001 兜底，tag 里禁止）"
	else
		ok "$(basename "$f") 无 not null（约束已收敛到显式迁移）"
	fi
done

echo ""
echo "=============================="
echo "  通过 $PASS / 失败 $FAIL"
echo "=============================="
[ "$FAIL" -eq 0 ] && exit 0 || exit 1
