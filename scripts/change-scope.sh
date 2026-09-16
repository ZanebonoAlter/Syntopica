#!/usr/bin/env bash
# change-scope.sh — 改动范围→最小验证命令判定（add-change-scope 引入，思路源自 deepseek-harness）
#
# 回答「这次改动该跑哪些测试」：收集改动文件（committed/staged/unstaged/untracked），
# 按目录自动发现的三档映射输出建议命令。只输出命令文本，不代替执行。
#
# 用法：
#   bash scripts/change-scope.sh [--base <ref>] [--json]
#     --base   base ref，默认 HEAD（工作区视角，匹配 turn_end「本轮编辑未提交」场景）
#     --json   机器可读输出 {base, paths[], testTargets[{cmd,tier}], notices[]}（quality-gate 消费）
#
# 映射档位（design.md §决策3；domain 目录运行时发现，零白名单维护）：
#   domain    backend-go/internal/<domain>/**（internal/ 下除 app/models/platform 的目录）
#             → go test -short ./internal/<domain>/...（递归；-short 下 DB 集成测试自动
#               t.Skip，无需 Docker——摸底结论 2026-08-21 见 change tasks.md 1.1 备注）
#   platform  backend-go/internal/platform/<pkg>/**
#             → go vet ./...（测试面广，不自动 test；被谁 import 不可知，升级为全量 vet）
#   skeleton  backend-go/internal/{app,models}/** 或 backend-go/cmd/**
#             → go build ./... + go vet ./...（不自动 test）
#   依赖       backend-go/go.mod|go.sum → 提示全量 go test ./...（不自动执行）
#   frontend  front/**（非 .md）→ pnpm lint（POSIX bash 可跑）；typecheck/test:unit/build 按平台标注执行前置
#   非代码    docs/ *.md scripts/ .pi/ openspec/ → 无测试命令
#   未命中    其他路径 → 「无法判定，请手动选择」，绝不猜测（codegraph affected 误报教训）
#
# 与 doc-impact.sh 的关系：文件收集逻辑复制不共享（各自独立演进，~15 行重叠不值当抽库）。
# POSIX bash（Linux / macOS / WSL）可跑。退出码恒 0（判定器不评判改动好坏）。

set -u

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

# ---------------------------------------------------------------------------
# 参数解析
# ---------------------------------------------------------------------------
BASE="HEAD"
JSON_MODE=0
while [ $# -gt 0 ]; do
	case "$1" in
	--base)
		BASE="${2:?--base 需要一个 ref 参数}"
		shift 2
		;;
	--json)
		JSON_MODE=1
		shift
		;;
	*)
		echo "未知参数: $1（支持 --base <ref> / --json）" >&2
		exit 1
		;;
	esac
done

# ---------------------------------------------------------------------------
# 改动文件集合（复制自 doc-impact.sh changed_files，DrvFS/quotepath 处理同源）
#   core.checkStat=minimal：在慢速网络挂载（WSL DrvFS /mnt/*）上完整 stat 极慢（29s→2s）
#   core.quotepath=false：中文路径原样输出
# ---------------------------------------------------------------------------
collect_changed_files() {
	git -c core.checkStat=minimal -c core.quotepath=false diff --name-only "$BASE" 2>/dev/null
	git -c core.quotepath=false diff --cached --name-only 2>/dev/null
	git -c core.quotepath=false ls-files --others --exclude-standard 2>/dev/null
}

# 去重保序
mapfile -t PATHS < <(collect_changed_files | awk '!seen[$0]++')

# ---------------------------------------------------------------------------
# domain 目录自动发现：internal/ 下除 app/models/platform 外的目录
# （新增 domain 零维护；与 check-standards.sh B 段 DOMAIN_WHITELIST 结构约束互为兜底）
# ---------------------------------------------------------------------------
declare -a DOMAIN_DIRS=()
while IFS= read -r d; do
	DOMAIN_DIRS+=("$d")
done < <(ls backend-go/internal 2>/dev/null | grep -vE '^(app|models|platform)$')

# ---------------------------------------------------------------------------
# 逐文件分类 → 聚合 testTargets / notices
# ---------------------------------------------------------------------------
declare -a TG_CMDS=() TG_TIERS=() NOTICES=()
SEEN_PLATFORM=0
SEEN_SKELETON=0
SEEN_FRONTEND=0
SEEN_DEPMOD=0

add_target() { # $1=cmd $2=tier（同 cmd 去重）
	local i
	for i in "${!TG_CMDS[@]}"; do
		[ "${TG_CMDS[$i]}" = "$1" ] && return
	done
	TG_CMDS+=("$1")
	TG_TIERS+=("$2")
}

add_notice() {
	local n
	for n in "${NOTICES[@]}"; do [ "$n" = "$1" ] && return; done
	NOTICES+=("$1")
}

for p in "${PATHS[@]}"; do
	# --- 非代码：产物目录、二进制/日志后缀，不产生测试命令也不告警 ---
	# （「无法判定」是防代码路径漏判的，不拿给二进制产物刷屏）
	case "$p" in
	*.md | docs/* | scripts/* | .pi/* | openspec/* | .agents/* | .gitignore | LICENSE) continue ;;
	artifacts/* | tests/* | data/*) continue ;;	esac
	case "$p" in
	*.wav | *.mp3 | *.mp4 | *.png | *.jpg | *.gif | *.webp | *.ttf | *.otf | *.woff2? | *.log | *.zip | *.srt | *.ass) continue ;;
	esac

	# --- 后端 ---
	if [[ "$p" == backend-go/go.mod || "$p" == backend-go/go.sum ]]; then
		SEEN_DEPMOD=1
		continue
	fi
	matched=0
	for d in "${DOMAIN_DIRS[@]}"; do
		if [[ "$p" == backend-go/internal/$d/* ]]; then
			add_target "go test -short ./internal/$d/..." "domain"
			matched=1
			break
		fi
	done
	[ "$matched" = 1 ] && continue
	case "$p" in
	backend-go/internal/platform/*)
		SEEN_PLATFORM=1
		;;
	backend-go/internal/app/* | backend-go/internal/models/* | backend-go/cmd/*)
		SEEN_SKELETON=1
		;;
	backend-go/*)
		# internal/ 下新建目录（非已知 domain）也按 domain 档处理——零维护设计的自然延伸
		if [[ "$p" =~ ^backend-go/internal/([^/]+)/ ]]; then
			add_target "go test -short ./internal/${BASH_REMATCH[1]}/..." "domain"
		else
			add_notice "无法判定（后端非映射路径）: $p — 请手动选择验证命令"
		fi
		;;
	# --- 前端 ---
	front/*)
		if [[ "$p" != *.md ]]; then SEEN_FRONTEND=1; fi
		;;
	*)
		add_notice "无法判定（未知路径）: $p — 请手动选择验证命令"
		;;
	esac
done

# 档位聚合命令
[ "$SEEN_PLATFORM" = 1 ] && add_target "go vet ./..." "platform"
[ "$SEEN_SKELETON" = 1 ] && {
	add_target "go build ./..." "skeleton"
	add_target "go vet ./..." "skeleton"
}
[ "$SEEN_FRONTEND" = 1 ] && add_target "pnpm lint" "frontend"
[ "$SEEN_DEPMOD" = 1 ] && add_notice "依赖变更（go.mod/go.sum）：建议全量 go test ./...（不自动执行）"
[ "$SEEN_FRONTEND" = 1 ] && add_notice "前端 typecheck/test:unit/build 的执行前置按平台而异：Windows 经 cmd.exe，Linux/macOS 本机直跑（均不属本条门禁范围，留给人工与归档门禁）"

# ---------------------------------------------------------------------------
# 输出
# ---------------------------------------------------------------------------
json_escape() { # 简单转义 \ 与 "
	sed 's/\\/\\\\/g; s/"/\\"/g' <<<"$1"
}

if [ "$JSON_MODE" = 1 ]; then
	{
		printf '{"base":"%s","paths":[' "$(json_escape "$BASE")"
		for i in "${!PATHS[@]}"; do
			[ "$i" -gt 0 ] && printf ','
			printf '"%s"' "$(json_escape "${PATHS[$i]}")"
		done
		printf '],"testTargets":['
		for i in "${!TG_CMDS[@]}"; do
			[ "$i" -gt 0 ] && printf ','
			printf '{"cmd":"%s","tier":"%s"}' "$(json_escape "${TG_CMDS[$i]}")" "${TG_TIERS[$i]}"
		done
		printf '],"notices":['
		for i in "${!NOTICES[@]}"; do
			[ "$i" -gt 0 ] && printf ','
			printf '"%s"' "$(json_escape "${NOTICES[$i]}")"
		done
		printf ']}\n'
	} | tr -d '\n' # 单行输出，消费方 JSON.parse 友好
	echo
	exit 0
fi

# 人类可读输出
echo "改动范围（base=${BASE}，${#PATHS[@]} 个文件）："
if [ "${#PATHS[@]}" -eq 0 ]; then
	echo "  （无改动）"
fi
for p in "${PATHS[@]}"; do echo "  $p"; done
echo
echo "建议验证命令："
if [ "${#TG_CMDS[@]}" -eq 0 ]; then
	echo "  （无代码改动，无需测试命令）"
fi
for i in "${!TG_CMDS[@]}"; do
	printf '  [%-8s] cd %s && %s\n' "${TG_TIERS[$i]}" \
		"$([ "${TG_TIERS[$i]}" = frontend ] && echo front || echo backend-go)" \
		"${TG_CMDS[$i]}"
done
if [ "${#NOTICES[@]}" -gt 0 ]; then
	echo
	echo "提示："
	for n in "${NOTICES[@]}"; do echo "  - $n"; done
fi
