#!/usr/bin/env bash
# constraint-digest.sh — 派发前取「业务域红线」摘要（add-dispatch-constraint-digest D1/D2/D3/D4）
#
# 用途：主线程派发子线程前，把目标路径/域对应的业务红线抽成一份短清单，粘进任务文本
#       「## 约束（红线，源 docs/reference/flow/<域>.md）」段（见 design D6）。
#
# 三条输入：
#   --domains d1,d2   业务域（逗号分隔）
#   --paths p1,p2     目标路径（逗号分隔）→ 按文档头部 doc-impact-applies 标签反查域
#   --brief           输出 brief 形态（派发任务文本「## 约束」段的契约形态，见下方用法）
#
# 只读边界：本脚本只读 docs/reference/flow/<域>.md 与文档头部的 doc-impact-applies 标签；
#   **不读** proposal.md 的 constraint-domains 注释，也**不读** tasks.md 的 doc-impact 注释
#   （两套注释语义不同：前者是运行时注入域声明，后者是归档门禁文档域声明，design R9）。
#
# 红线提取：唯一实现 = .pi/extensions/lib/constraint-redlines.ts（extension 与本脚本双引，
#   esbuild bundle 后 node 直调）。bundle 失败 → 该域块降级为「该域红线不可用」+ 细节层
#   read 指引，末尾非零退出（1），**禁止**回退脚本自带正则（避免两套口径漂移，design D2 / R1）。
#
# 域唯一真相源 = docs/reference/flow/*.md basename（与 constraint-injection.ts 的
#   listFlowDomains 同源语义）。scripts/harness/doc-impact.sh menu 的 8 个是**文档目录轴**
#   （哪类文档要更新），不是业务域轴，禁止混用（design D4 / R2）。
#
# 用法：bash scripts/harness/constraint-digest.sh --paths backend-go/internal/admin/scheduler/
#       bash scripts/harness/constraint-digest.sh --brief --paths <目标>   # brief 形态（可原样粘进任务文本）
#
# --brief：输出「派发任务文本约束段」的契约形态（design D6 模板；telemetry 解析依赖此形态）：
#   ## 约束（红线，源 <源路径1>, <源路径2>）
#   ### <域>（源 docs/reference/flow/<域>.md）
#   <红线行>
#   细节层：read `docs/reference/flow/<域>.md`「业务约束与不变量」
#   > 目标路径与本清单冲突时以 `--paths <你的目标>` 输出为准
#   不带 --brief 时输出人读形态（每域一个「### <域>（源 …）」块 + 命中来源 / 未命中说明）。
#
# 退出码：0 成功；2 参数错误 / 未知域名 / 无输入；1 取数失败（bundle 失败等）
set -u

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

FLOW_DIR="docs/reference/flow"
STD_DIR="docs/reference/standard"
# 测试夹具接缝（仅 smoke 用）：把两个文档根指向临时目录，便于在仓库外验证 design D3 的
# 两个边界（标签在 15 行外不命中 / 一文档仅首条标签生效）。默认走仓库真实路径。
if [ -n "${CONSTRAINT_DIGEST_DOC_ROOT:-}" ]; then
	FLOW_DIR="$CONSTRAINT_DIGEST_DOC_ROOT/flow"
	STD_DIR="$CONSTRAINT_DIGEST_DOC_ROOT/standard"
fi
SECTION_NAME="业务约束与不变量"
MAX_REDLINES=8          # 单域红线最多输出行数（超出截断 + 附总条数）
TAG_HEAD_LINES=15       # doc-impact-applies 标签只读文档头部 15 行

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

usage() {
	cat <<'EOF'
constraint-digest.sh — 派发前取业务域红线摘要（add-dispatch-constraint-digest）

用法：
  bash scripts/harness/constraint-digest.sh --domains <域1,域2>
  bash scripts/harness/constraint-digest.sh --paths <路径1,路径2>
  bash scripts/harness/constraint-digest.sh --brief --paths <路径1,路径2>
  bash scripts/harness/constraint-digest.sh --help

三条输入：
  --domains  业务域清单（逗号分隔）。域名 = docs/reference/flow/*.md 的 basename，
             业务域**唯一真相源**（与 constraint-injection.ts 的 listFlowDomains 同源）。
             未知域名 → 非零退出并列出可用域名。
  --paths    目标路径清单（逗号分隔）。按文档头部 doc-impact-applies 标签反查：
             只读文档头部 15 行的标签（单文档首个匹配行即止），信号为**纯子串**匹配
             （path.includes(signal)，非路径前缀 / glob），命中 flow 文档即导出该域。
  --brief    输出 brief 形态（派发任务文本「## 约束」段的契约形态，见 design D6）：
             单个「## 约束（红线，源 <源路径1>, <源路径2>）」标题行（多域源路径逗号分隔，
             顺序与域输出顺序一致）+ 每域一个「### <域>（源 docs/reference/flow/<域>.md）」
             子标题与该域红线行 + 段末固定一行
             「> 目标路径与本清单冲突时以 `--paths <你的目标>` 输出为准」。
             可与 --domains / --paths 组合；不带 --brief 时输出人读形态（保持不变）。

输出契约（每域）：
  ### <域>（源 docs/reference/flow/<域>.md）
  1. **红线句**
  ...
  细节层：read `docs/reference/flow/<域>.md`「业务约束与不变量」
  单域红线最多 8 行（超出截断并附「…（共 N 条，见细节层）」）。

未命中三态（显式输出，禁止静默空）：
  ① 路径不匹配任何标签 → 「未命中任何 doc-impact-applies 标签」+ 回退指引
     （按任务词查 docs/reference/constraints-index.md 域表后改用 --domains）
  ② 命中文档但非业务域 → 「命中 <doc>，该文档不属业务域」+ 列出可用域
  ③ 命中 flow 文档但红线层为空 / 低于阈值 → 「该域无结构化红线（…）」+ 细节层 read 指引

只读声明：
  本脚本**只读文档头部 doc-impact-applies** 标签；
  **不读** proposal.md 的 `constraint-domains` 注释，也**不读** tasks.md 的 `doc-impact` 注释
  （那两套注释分别是运行时注入域声明 / 归档门禁文档域声明，与本脚本无关）。

红线提取：唯一实现 = .pi/extensions/lib/constraint-redlines.ts（extension 与本脚本双引）。
  esbuild bundle 失败即非零退出（提示「无法加载红线提取实现」），**不回退**脚本自带正则。
  bundle **惰性构建**：--paths 的标签扫描与未命中三态判定不依赖 esbuild；只有确定要输出
  域红线块时才 bundle，失败则该域块降级为「该域红线不可用（bundle 失败：…）」+ 细节层
  read 指引，末尾非零退出（1），未命中说明照常打印。

域清单口径：业务域唯一真相源 = docs/reference/flow/*.md basename（数量随仓库演进，
  实时清单见 --domains 报错输出）；
  scripts/harness/doc-impact.sh menu 的 8 个是**文档目录轴**（哪类文档要更新），
  不是业务域轴，禁止混用。

退出码：0 成功；2 参数错误 / 未知域名 / 无输入；1 取数失败（bundle 失败等）
EOF
}

# ---------------------------------------------------------------------------
# 业务域清单（唯一真相源 = docs/reference/flow/*.md basename，排除 README）
# ---------------------------------------------------------------------------
flow_domains() {
	local f b
	for f in "$FLOW_DIR"/*.md; do
		[ -e "$f" ] || continue
		b="${f##*/}"
		[ "$b" = "README.md" ] && continue
		printf '%s\n' "${b%.md}"
	done | LC_ALL=C sort
}

domains_csv() {
	local first=1 d
	while IFS= read -r d; do
		[ -n "$d" ] || continue
		if [ "$first" -eq 1 ]; then first=0; else printf ','; fi
		printf '%s' "$d"
	done < <(flow_domains)
	printf '\n'
}

# ---------------------------------------------------------------------------
# doc-impact-applies 标签扫描（与 constraint-injection.ts scanAppliesTags 同语义）
#   只读文档头部 15 行；单文档首个匹配行即止；信号逗号分隔；
#   `| section=<节名>` 可选；信号匹配为**纯子串**（path.includes(signal)）。
# ---------------------------------------------------------------------------
# 标签行 → 标签值（剥离尾部可选的 -->）；非标签行返回 1
tag_value_of_line() {
	local line="$1" s="" t=""
	s="${line#"${line%%[![:space:]]*}"}"                       # ltrim
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
	s="${s#"${s%%[![:space:]]*}"}"                           # ltrim 值
	s="${s%"${s##*[![:space:]]}"}"                           # rtrim 值
	[ -n "$s" ] || return 1                                  # 值不得为空
	case "$s" in
		*"-->")
			t="${s%-->}"
			t="${t%"${t##*[![:space:]]}"}"
			[ -n "$t" ] && s="$t"
			;;
	esac
	printf '%s' "$s"
}

# 标签值 → 信号（逐行输出）
#   `| section=节名` 尾段**本脚本不消费**（section 未实现：脚本只按信号反查域，红线节固定为
#   「业务约束与不变量」；标签里的 section= 由 constraint-injection.ts 消费），此处把 `|`
#   之后的尾段整段丢弃，避免把它误当成信号。
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

# 参与扫描的文档（flow 深度 1、standard 深度 2 —— 与 walkDocs 同语义；
# walkDocs 的 depth 是「目录可再下钻层数」，故文件实际最深到 flow/x/y.md、
# standard/x/y/z.md，与 find -maxdepth 一一对应）
doc_files() {
	find "$FLOW_DIR" -maxdepth 2 -name '*.md' ! -name 'README.md' 2>/dev/null | LC_ALL=C sort
	find "$STD_DIR" -maxdepth 3 -name '*.md' ! -name 'README.md' 2>/dev/null | LC_ALL=C sort
}

# 每行输出：<rel>\t<信号>（每个信号一行；同一文档的信号连续出现）
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
			break                       # 单文档首个匹配行即止
		done <<<"$head"
	done < <(doc_files)
}

# ---------------------------------------------------------------------------
# 节提取（与 constraint-injection.ts extractSection 同语义）
#   「## <节名>」标题行（仅去行尾空白）到下一个 /^##\s+/ 标题之间；整体 trim。
# ---------------------------------------------------------------------------
SECTION_FOUND=0
extract_section() { # $1=文件 $2=节名 → stdout 输出 trim 后的节文本
	local file="$1" name="$2" raw=""
	SECTION_FOUND=0
	if grep -qE "^## ${name}[[:space:]]*$" "$file" 2>/dev/null; then
		SECTION_FOUND=1
	fi
	raw="$(awk -v heading="## $name" '
		BEGIN { started = 0 }
		{
			line = $0
			sub(/[[:space:]]+$/, "", line)
			if (!started) {
				if (line == heading) started = 1
				next
			}
			if ($0 ~ /^##[[:space:]]+/) exit
			print $0
		}
	' "$file" 2>/dev/null)"
	raw="${raw#"${raw%%[![:space:]]*}"}"                       # ltrim（JS trim 同义）
	raw="${raw%"${raw##*[![:space:]]}"}"                       # rtrim
	printf '%s' "$raw"
}

# ---------------------------------------------------------------------------
# 红线提取：唯一实现 = lib/constraint-redlines.ts（esbuild bundle → node 直调）
# ---------------------------------------------------------------------------
# esbuild 解析（离线优先）：本仓已装的 esbuild → npx 缓存 → 最后才回落 npx -y（需联网）。
# 顺序与 design R-13 处置一致：先离线可用源，最后才联网拉包。
ESBUILD_ARGS=()
resolve_esbuild() {
	if [ "${#ESBUILD_ARGS[@]}" -gt 0 ]; then return 0; fi
	if command -v esbuild >/dev/null 2>&1; then
		ESBUILD_ARGS=(esbuild)
		return 0
	fi
	if [ -x "node_modules/.bin/esbuild" ]; then
		ESBUILD_ARGS=("$REPO_ROOT/node_modules/.bin/esbuild")
		return 0
	fi
	if npx --no-install esbuild --version >/dev/null 2>&1; then
		ESBUILD_ARGS=(npx --no-install esbuild)
		return 0
	fi
	ESBUILD_ARGS=(npx -y esbuild)
}

# bundle **惰性构建**：只在确定要输出域红线块时才调用——--paths 的标签扫描与未命中三态判定
# 不依赖 esbuild（bundle 不可用时「未命中 → 回退 --domains」这条离线兜底必须照常给出）。
# 失败不立即退出：置 DEGRADE_ERR → 域块降级输出 + 末尾非零退出（1）。
BUNDLE=""
DEGRADE_ERR=""
build_redlines_bundle() {
	if [ -n "$BUNDLE" ]; then return 0; fi
	if [ -n "$DEGRADE_ERR" ]; then return 1; fi
	resolve_esbuild
	if ! "${ESBUILD_ARGS[@]}" .pi/extensions/lib/constraint-redlines.ts --bundle --platform=node \
		--format=cjs --outfile="$TMP/redlines.cjs" >"$TMP/esbuild.log" 2>&1; then
		DEGRADE_ERR="bundle 失败：esbuild bundle .pi/extensions/lib/constraint-redlines.ts 失败（$(head -n 1 "$TMP/esbuild.log" 2>/dev/null | tr -d '\r\n')）"
		echo "无法加载红线提取实现（$DEGRADE_ERR）：" >&2
		sed -n '1,20p' "$TMP/esbuild.log" >&2
		echo "→ 禁止回退脚本自带正则（会造成两套口径漂移）；请先修复红线提取实现。" >&2
		return 1
	fi
	BUNDLE="$TMP/redlines.cjs"
	return 0
}

# 调共用实现：node -e 直接 require bundle 产物并调用 extractRedlines。
# 节文本经临时文件传入（单域节可达 40KB，不走 argv）；输出走行协议而非 JSON
# （bash 侧不引 JSON 解析器，红线行原样透传即可）。
run_extract_redlines() { # $1=节文本文件 → stdout：RED_BYTES <n> / LINE <红线行>
	node -e '
const fs = require("node:fs");
const { extractRedlines } = require(process.argv[1]);
const text = fs.readFileSync(process.argv[2], "utf8");
const red = extractRedlines(text);
process.stdout.write("RED_BYTES " + (red ? red.bytes : 0) + "\n");
if (red) for (const l of red.lines) process.stdout.write("LINE " + l + "\n");
' "$BUNDLE" "$1"
}

# 阈值读同一 config（.pi/constraint-injection.json 的 minSectionBytes，缺省 512）
read_min_section_bytes() {
	node -e '
const fs = require("node:fs");
let v;
try { v = JSON.parse(fs.readFileSync(".pi/constraint-injection.json", "utf8")).minSectionBytes; } catch (e) {}
process.stdout.write(String(typeof v === "number" && isFinite(v) ? v : 512));
'
}

MIN_SECTION_BYTES=""

# ---------------------------------------------------------------------------
# 输出：单域红线块
# ---------------------------------------------------------------------------
print_domain_block() { # $1=域名
	local domain="$1"
	local doc="$FLOW_DIR/$domain.md"
	local sec_file="$TMP/section.txt"
	local sec_bytes=0 red_bytes=0 line out=""
	local -a reds=()

	printf '### %s（源 %s）\n' "$domain" "$doc"

	# bundle 惰性 + 降级：bundle 失败只降级本块（给细节层 read 指引），
	# 末尾统一非零退出；未命中说明照常打印。
	if ! build_redlines_bundle; then
		printf '该域红线不可用（%s）\n' "$DEGRADE_ERR"
		printf '细节层：read `%s`「%s」（全节）\n' "$doc" "$SECTION_NAME"
		return 0
	fi

	extract_section "$doc" "$SECTION_NAME" >"$sec_file"
	sec_bytes="$(wc -c <"$sec_file" | tr -d ' ')"
	out="$(run_extract_redlines "$sec_file")" || {
		DEGRADE_ERR="红线提取失败：$domain"
		echo "红线提取失败：$domain" >&2
		printf '该域红线不可用（%s）\n' "$DEGRADE_ERR"
		printf '细节层：read `%s`「%s」（全节）\n' "$doc" "$SECTION_NAME"
		return 0
	}
	while IFS= read -r line; do
		case "$line" in
			"RED_BYTES "*) red_bytes="${line#RED_BYTES }" ;;
			"LINE "*) reds+=("${line#LINE }") ;;
		esac
	done <<<"$out"

	if [ "$SECTION_FOUND" -eq 0 ]; then
		printf '该域无结构化红线（未找到「%s」节）\n' "$SECTION_NAME"
		printf '细节层：read `%s`（全文）\n' "$doc"
		return 0
	fi
	if [ "${#reds[@]}" -gt 0 ] && [ "$red_bytes" -ge "$MIN_SECTION_BYTES" ]; then
		local i
		for ((i = 0; i < MAX_REDLINES && i < ${#reds[@]}; i++)); do
			printf '%s\n' "${reds[$i]}"
		done
		if [ "${#reds[@]}" -gt "$MAX_REDLINES" ]; then
			printf '…（共 %s 条，见细节层）\n' "${#reds[@]}"
		fi
		printf '细节层：read `%s`「%s」\n' "$doc" "$SECTION_NAME"
	else
		printf '该域无结构化红线（节字节 %s，红线层字节 %s < 阈值 %s）\n' \
			"$sec_bytes" "$red_bytes" "$MIN_SECTION_BYTES"
		printf '细节层：read `%s`「%s」（全节）\n' "$doc" "$SECTION_NAME"
	fi
}

# ---------------------------------------------------------------------------
# 路径反查：--paths（命中 flow 文档 → 域；命中标准文档 → 三态②；无命中 → 三态①）
# ---------------------------------------------------------------------------
HIT_LINES=()
NOTE_LINES=()
MATCHED_DOMAINS=()

run_paths() { # $1=逗号分隔路径
	local paths_csv="$1"
	local IFS=','
	local -a plist=()
	read -r -a plist <<<"$paths_csv"
	IFS=$' \t\n'

	local path doc sig prev_doc cur_doc cur_sig domain
	local -a hit_docs=() hit_sigs=()
	local i

	for path in "${plist[@]}"; do
		[ -n "$path" ] || continue
		hit_docs=()
		hit_sigs=()
		prev_doc=""
		cur_doc=""
		cur_sig=""
		while IFS=$'\t' read -r doc sig; do
			[ -n "$doc" ] || continue
			if [ "$doc" != "$prev_doc" ]; then
				# 上一文档收口（每个文档只记首个匹配信号）
				if [ -n "$cur_doc" ]; then
					hit_docs+=("$cur_doc")
					hit_sigs+=("$cur_sig")
				fi
				prev_doc="$doc"
				cur_doc=""
				cur_sig=""
			fi
			[ -n "$cur_doc" ] && continue
			case "$path" in
				*"$sig"*)
					cur_doc="$doc"
					cur_sig="$sig"
					;;
			esac
		done < <(scan_tag_docs)
		if [ -n "$cur_doc" ]; then
			hit_docs+=("$cur_doc")
			hit_sigs+=("$cur_sig")
		fi

		if [ "${#hit_docs[@]}" -eq 0 ]; then
			NOTE_LINES+=("- $path：未命中任何 doc-impact-applies 标签 → 回退指引：按任务词查 docs/reference/constraints-index.md 域表后改用 --domains")
			continue
		fi
		for ((i = 0; i < ${#hit_docs[@]}; i++)); do
			doc="${hit_docs[$i]}"
			sig="${hit_sigs[$i]}"
			HIT_LINES+=("- $path → $doc（信号：$sig）")
			case "$doc" in
				"$FLOW_DIR"/*.md)
					domain="${doc#"$FLOW_DIR"/}"
					domain="${domain%.md}"
					MATCHED_DOMAINS+=("$domain")
					;;
				*)
					# 三态②：命中文档但非业务域
					NOTE_LINES+=("- $path：命中 $doc，该文档不属业务域 → 可用业务域：$(domains_csv)")
					;;
			esac
		done
	done
}

# ---------------------------------------------------------------------------
# 主流程
# ---------------------------------------------------------------------------
DOMAINS_ARG=""
PATHS_ARG=""
HAVE_DOMAINS=0
HAVE_PATHS=0
BRIEF=0

while [ $# -gt 0 ]; do
	case "$1" in
		--brief)
			BRIEF=1; shift ;;
		--domains)
			[ $# -ge 2 ] || { echo "--domains 缺少取值" >&2; exit 2; }
			DOMAINS_ARG="$2"; HAVE_DOMAINS=1; shift 2 ;;
		--domains=*)
			DOMAINS_ARG="${1#--domains=}"; HAVE_DOMAINS=1; shift ;;
		--paths)
			[ $# -ge 2 ] || { echo "--paths 缺少取值" >&2; exit 2; }
			PATHS_ARG="$2"; HAVE_PATHS=1; shift 2 ;;
		--paths=*)
			PATHS_ARG="${1#--paths=}"; HAVE_PATHS=1; shift ;;
		-h | --help)
			usage; exit 0 ;;
		*)
			echo "未知参数：$1" >&2
			usage >&2
			exit 2 ;;
	esac
done

if [ "$HAVE_DOMAINS" -eq 0 ] && [ "$HAVE_PATHS" -eq 0 ]; then
	echo "缺少输入：--domains <域> 或 --paths <路径>（见 --help）" >&2
	exit 2
fi

command -v node >/dev/null 2>&1 || { echo "缺少 node（红线提取实现需 node 运行）" >&2; exit 1; }

# 域名白名单校验（业务域唯一真相源 = flow basename）
REQUESTED_DOMAINS=""
if [ "$HAVE_DOMAINS" -eq 1 ]; then
	AVAILABLE="$(domains_csv)"
	IFS=','
	read -r -a DREQ <<<"$DOMAINS_ARG"
	IFS=$' \t\n'
	for d in "${DREQ[@]}"; do
		[ -n "$d" ] || continue
		ok=0
		while IFS= read -r cand; do
			if [ "$cand" = "$d" ]; then
				ok=1
				break
			fi
		done < <(flow_domains)
		if [ "$ok" -ne 1 ]; then
			echo "未知业务域：$d" >&2
			echo "可用业务域（docs/reference/flow/*.md basename，唯一真相源）：$AVAILABLE" >&2
			exit 2
		fi
		REQUESTED_DOMAINS="${REQUESTED_DOMAINS:+$REQUESTED_DOMAINS,}$d"
	done
fi

MIN_SECTION_BYTES="$(read_min_section_bytes)"

# bundle 惰性构建（design R-13 / M1）：不依赖 esbuild 的部分先跑——
# --paths 的标签扫描与未命中三态判定先出结果，esbuild 不可用也不吞掉回退指引。
if [ "$HAVE_PATHS" -eq 1 ]; then
	run_paths "$PATHS_ARG"
fi

# 域集合去重后排序输出
OUT_DOMAINS=()
if [ "$HAVE_DOMAINS" -eq 1 ]; then
	IFS=','
	read -r -a DLIST <<<"$REQUESTED_DOMAINS"
	IFS=$' \t\n'
	for d in "${DLIST[@]}"; do
		[ -n "$d" ] || continue
		OUT_DOMAINS+=("$d")
	done
fi
for d in "${MATCHED_DOMAINS[@]}"; do
	[ -n "$d" ] || continue
	OUT_DOMAINS+=("$d")
done
UNIQ_DOMAINS=()
for d in "${OUT_DOMAINS[@]}"; do
	dup=0
	for u in "${UNIQ_DOMAINS[@]}"; do
		if [ "$u" = "$d" ]; then
			dup=1
			break
		fi
	done
	[ "$dup" -eq 0 ] && UNIQ_DOMAINS+=("$d")
done
SORTED_DOMAINS=$(printf '%s\n' "${UNIQ_DOMAINS[@]}" | LC_ALL=C sort | grep -v '^$')

# ---------------------------------------------------------------------------
# 输出：人读形态（默认）/ brief 形态（--brief）
# ---------------------------------------------------------------------------
print_hit_lines() {
	[ "${#HIT_LINES[@]}" -gt 0 ] || return 0
	printf '## 命中来源（路径 → doc-impact-applies 标签）\n'
	local l
	for l in "${HIT_LINES[@]}"; do
		printf '%s\n' "$l"
	done
	printf '\n'
}

print_note_lines() {
	[ "${#NOTE_LINES[@]}" -gt 0 ] || return 0
	printf '## 未命中说明\n'
	local l
	for l in "${NOTE_LINES[@]}"; do
		printf '%s\n' "$l"
	done
	printf '\n'
}

# brief 段（派发任务文本「## 约束」段的契约形态，design D6）：
#   单个「## 约束（红线，源 <源路径1>, <源路径2>）」标题行（源路径逗号分隔，顺序与域输出顺序一致）
#   + 每域一个「### <域>（源 docs/reference/flow/<域>.md）」子标题与该域红线行
#   + 段末固定一行兜底（子线程按自己真实目标路径补取）
print_brief_section() { # $1=空格分隔的域列表
	local domains="$1" d src_csv=""
	local -a srcs=()
	for d in $domains; do
		[ -n "$d" ] || continue
		srcs+=("$FLOW_DIR/$d.md")
	done
	[ "${#srcs[@]}" -gt 0 ] || return 0
	for d in "${srcs[@]}"; do
		if [ -n "$src_csv" ]; then src_csv="$src_csv, "; fi
		src_csv="$src_csv$d"
	done
	printf '## 约束（红线，源 %s）\n' "$src_csv"
	for d in $domains; do
		[ -n "$d" ] || continue
		print_domain_block "$d"
	done
	printf '> 目标路径与本清单冲突时以 `--paths <你的目标>` 输出为准\n'
}

if [ "$BRIEF" -eq 1 ]; then
	# brief 形态：旁注段（命中来源 / 未命中说明）在前，「## 约束」段在后收尾——
	# 主线程原样粘贴时约束段完整、且以固定兜底行结尾（telemetry 解析契约）。
	print_hit_lines
	print_note_lines
	print_brief_section "$SORTED_DOMAINS"
else
	for d in $SORTED_DOMAINS; do
		print_domain_block "$d"
		printf '\n'
	done
	print_hit_lines
	print_note_lines
fi

# 取数失败（bundle / 提取失败）→ 非零退出（1）；域块已降级输出，未命中说明照常打印
if [ -n "$DEGRADE_ERR" ]; then
	exit 1
fi
exit 0
