#!/usr/bin/env bash
# constraint-digest.smoke.sh — 取数脚本 + 红线下沉的冒烟自测（add-dispatch-constraint-digest 任务组 1-2）
#
# case 清单：
#   A1 --domains daily-report：命中 + 输出含源路径 + 红线行 ≤8
#   A2 --paths 反查：目标路径命中 daily-report 域
#   B1 三态①：路径无命中 → 「未命中任何 doc-impact-applies 标签」+ constraints-index 回退指引
#   B2 三态②：路径命中非业务域文档 → 「命中 <doc>，该文档不属业务域」+ 可用域名
#   C1 未知域名 → 非零退出且输出可用域名
#   D1 红线逐字节对账：10 篇 flow 文档，脚本输出 vs 直接 bundle 调用 extractRedlines
#      （含「红线层字节低于阈值 → 无结构化红线」回退形态判定一致）
#   E1 路径命中矩阵对账：脚本 --paths 命中集 vs extension scanAppliesTags（JIT 单一真相源）
#   F1 --brief 端到端契约对账：脚本 --brief 输出 → **真实** parseConstraintBrief 解析
#      → digest=present + 域集合（单域 / 多域 / --paths 反查三种）
#   F2 design D3 边界 fixture（临时目录）：标签写在第 16 行 → 不命中；
#      同一文档头部两条标签 → 只取首条（首匹配即止）
#
# 用法：bash scripts/harness/constraint-digest.smoke.sh   退出码：0 全过；1 有失败
set -uo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$HERE/../.." && pwd)"
DIGEST="$HERE/constraint-digest.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

pass=0
failn=0
ok() { echo "✅ $1"; pass=$((pass+1)); }
bad() { echo "❌ $1"; failn=$((failn+1)); }
check() { # check <名> <条件 0/1> [说明]
	if [ "$2" -eq 0 ]; then ok "$1"; else bad "$1${3:+ — $3}"; fi
}

cd "$REPO_ROOT"

# esbuild 解析（离线优先，review R-13）：本仓已装 → npx 缓存 → 最后才 npx -y（需联网）
ESBUILD_ARGS=()
resolve_esbuild() {
	if [ "${#ESBUILD_ARGS[@]}" -gt 0 ]; then return 0; fi
	if command -v esbuild >/dev/null 2>&1; then
		ESBUILD_ARGS=(esbuild); return 0
	fi
	if [ -x "node_modules/.bin/esbuild" ]; then
		ESBUILD_ARGS=("$REPO_ROOT/node_modules/.bin/esbuild"); return 0
	fi
	if npx --no-install esbuild --version >/dev/null 2>&1; then
		ESBUILD_ARGS=(npx --no-install esbuild); return 0
	fi
	ESBUILD_ARGS=(npx -y esbuild)
}
resolve_esbuild

# --- D1 对账器：脚本输出块 vs 直接 bundle 调用 extractRedlines ---
cat >"$TMP/compare-redlines.cjs" <<'CJS'
const fs = require("node:fs");
const { extractRedlines } = require(process.argv[2]);
const scriptOut = fs.readFileSync(process.argv[3], "utf8");
const ROOT = process.argv[4] || process.cwd();
const flowDir = ROOT + "/docs/reference/flow";
let threshold = 512;
try {
	const cfg = JSON.parse(fs.readFileSync(ROOT + "/.pi/constraint-injection.json", "utf8"));
	if (typeof cfg.minSectionBytes === "number" && isFinite(cfg.minSectionBytes)) threshold = cfg.minSectionBytes;
} catch (e) {}

// 解析脚本输出：域 → { lines, state, marker }
const blocks = new Map();
let cur = null;
for (const line of scriptOut.split(/\r?\n/)) {
	const h = line.match(/^### (.+?)（源 (.+?)）$/);
	if (h) {
		cur = { domain: h[1], src: h[2], lines: [], state: null, marker: null };
		blocks.set(h[1], cur);
		continue;
	}
	if (!cur) continue;
	if (line.startsWith("细节层：")) { cur = null; continue; }
	if (/^(?:\d+\.|-) \*\*/.test(line)) { cur.lines.push(line); continue; }
	if (line.includes("该域无结构化红线")) { cur.state = line; continue; }
	if (line.startsWith("…（共")) { cur.marker = line; continue; }
}

const files = fs.readdirSync(flowDir).filter((f) => f.endsWith(".md") && f !== "README.md").sort();
for (const f of files) {
	const domain = f.replace(/\.md$/, "");
	const src = fs.readFileSync(flowDir + "/" + f, "utf8").split(/\r?\n/);
	const i = src.findIndex((l) => l.trimEnd() === "## 业务约束与不变量");
	let sec = null;
	if (i >= 0) {
		const o = [];
		for (let k = i + 1; k < src.length; k++) {
			if (/^##\s+/.test(src[k])) break;
			o.push(src[k]);
		}
		sec = o.join("\n").trim();
	}
	const red = sec === null ? null : extractRedlines(sec);
	const expectNone = !red || red.bytes < threshold;
	const blk = blocks.get(domain);
	if (!blk) {
		console.log(`FAIL: D1 ${domain}：脚本输出缺该域块`);
		continue;
	}
	if (expectNone) {
		if (!blk.state) {
			console.log(`FAIL: D1 ${domain}：期望「该域无结构化红线」回退形态，实际输出 ${blk.lines.length} 行红线`);
			continue;
		}
		if (blk.lines.length) {
			console.log(`FAIL: D1 ${domain}：回退形态下仍有 ${blk.lines.length} 行红线`);
			continue;
		}
		console.log(`PASS: D1 ${domain}：无结构化红线回退形态一致（红线层字节 ${red ? red.bytes : 0} < 阈值 ${threshold}）`);
		continue;
	}
	if (blk.state) {
		console.log(`FAIL: D1 ${domain}：期望红线层，实际输出「${blk.state}」`);
		continue;
	}
	const expect = red.lines.slice(0, 8);
	const got = blk.lines;
	if (got.length !== expect.length || got.some((l, idx) => l !== expect[idx])) {
		console.log(`FAIL: D1 ${domain}：红线逐字节不一致（期望 ${expect.length} 行 / 实际 ${got.length} 行）`);
		console.log(`  期望首行：${expect[0]}`);
		console.log(`  实际首行：${got[0]}`);
		continue;
	}
	if (red.lines.length > 8 && (!blk.marker || !blk.marker.includes(`共 ${red.lines.length} 条`))) {
		console.log(`FAIL: D1 ${domain}：缺截断标记「…（共 ${red.lines.length} 条，见细节层）」`);
		continue;
	}
	console.log(`PASS: D1 ${domain}：红线逐字节一致（输出 ${expect.length} / 共 ${red.lines.length} 条）`);
}
CJS

# --- E1 对账器：脚本 --paths 命中集 vs extension scanAppliesTags ---
cat >"$TMP/compare-paths.cjs" <<'CJS'
const fs = require("node:fs");
const ext = require(process.argv[2]);
const scriptOut = fs.readFileSync(process.argv[3], "utf8");
const ROOT = process.argv[4];
const paths = process.argv.slice(5);

const tags = ext.scanAppliesTags(ROOT);
const expected = new Map();
for (const p of paths) {
	const hits = [];
	for (const [doc, tag] of tags) {
		const sig = tag.signals.find((s) => p.includes(s));
		if (sig) hits.push(doc + "\t" + sig);
	}
	expected.set(p, hits);
}
const got = new Map();
for (const line of scriptOut.split(/\r?\n/)) {
	const m = line.match(/^- (.+?) → (.+?)（信号：(.*)）$/);
	if (!m) continue;
	if (!got.has(m[1])) got.set(m[1], []);
	got.get(m[1]).push(m[2] + "\t" + m[3]);
}
for (const p of paths) {
	const e = (expected.get(p) || []).slice().sort();
	const g = (got.get(p) || []).slice().sort();
	if (e.length !== g.length || e.some((v, i) => v !== g[i])) {
		console.log(`FAIL: E1 路径命中矩阵不一致 ${p}：期望 ${JSON.stringify(e)}，实际 ${JSON.stringify(g)}`);
	} else {
		console.log(`PASS: E1 路径命中矩阵一致 ${p}（${e.length} 命中）`);
	}
}
CJS

# ---------------------------------------------------------------------------
# A1 按域取红线：命中 + 源路径 + 红线行 ≤8
# ---------------------------------------------------------------------------
out="$(bash "$DIGEST" --domains daily-report 2>"$TMP/a1.err")"
rc=$?
check "A1 --domains daily-report 退出码 0" "$([ "$rc" -eq 0 ] && echo 0 || echo 1)" "实际 exit=$rc"
check "A1 输出含域标题与源路径" \
	"$(printf '%s' "$out" | grep -q '^### daily-report（源 docs/reference/flow/daily-report.md）$' && echo 0 || echo 1)"
check "A1 输出含细节层 read 指引" \
	"$(printf '%s' "$out" | grep -q '^细节层：read `docs/reference/flow/daily-report.md`「业务约束与不变量」$' && echo 0 || echo 1)"
redline_cnt="$(printf '%s\n' "$out" | grep -cE '^([0-9]+\.|-) \*\*')"
check "A1 红线行 ≤8" "$([ "$redline_cnt" -le 8 ] && echo 0 || echo 1)" "实际 $redline_cnt 行"
check "A1 红线行为非空" "$([ "$redline_cnt" -ge 1 ] && echo 0 || echo 1)" "实际 $redline_cnt 行"

# ---------------------------------------------------------------------------
# A2 按路径反查：命中 daily-report
#   信号为纯子串匹配（backend-go/internal/admin/scheduler/ 是 daily-report.md 的辖区信号）
# ---------------------------------------------------------------------------
HIT_PATH="backend-go/internal/admin/scheduler/job_daily_report.go"
out="$(bash "$DIGEST" --paths "$HIT_PATH" 2>"$TMP/a2.err")"
rc=$?
check "A2 --paths 退出码 0" "$([ "$rc" -eq 0 ] && echo 0 || echo 1)" "实际 exit=$rc"
check "A2 --paths 命中 daily-report 域" \
	"$(printf '%s' "$out" | grep -q '^### daily-report（源 docs/reference/flow/daily-report.md）$' && echo 0 || echo 1)"
check "A2 输出附命中信号原文" \
	"$(printf '%s' "$out" | grep -q '（信号：backend-go/internal/admin/scheduler/）' && echo 0 || echo 1)"

# ---------------------------------------------------------------------------
# B1 三态①：路径不匹配任何标签
# ---------------------------------------------------------------------------
out="$(bash "$DIGEST" --paths front/app/stores/api.ts 2>"$TMP/b1.err")"
rc=$?
check "B1 无命中退出码 0" "$([ "$rc" -eq 0 ] && echo 0 || echo 1)" "实际 exit=$rc"
check "B1 输出「未命中任何 doc-impact-applies 标签」" \
	"$(printf '%s' "$out" | grep -q '未命中任何 doc-impact-applies 标签' && echo 0 || echo 1)"
check "B1 输出 constraints-index.md 回退指引" \
	"$(printf '%s' "$out" | grep -q 'docs/reference/constraints-index.md' && echo 0 || echo 1)"
check "B1 未命中时不输出空红线块" \
	"$(printf '%s' "$out" | grep -q '^### ' && echo 1 || echo 0)"

# ---------------------------------------------------------------------------
# B2 三态②：命中文档但非业务域
# ---------------------------------------------------------------------------
out="$(bash "$DIGEST" --paths front/app/app.vue 2>"$TMP/b2.err")"
rc=$?
check "B2 非业务域退出码 0" "$([ "$rc" -eq 0 ] && echo 0 || echo 1)" "实际 exit=$rc"
check "B2 输出「命中 <doc>，该文档不属业务域」" \
	"$(printf '%s' "$out" | grep -q '命中 docs/reference/standard/frontend/loading-experience.md，该文档不属业务域' && echo 0 || echo 1)"
check "B2 输出可用业务域清单" \
	"$(printf '%s' "$out" | grep -q '可用业务域：.*daily-report' && echo 0 || echo 1)"
check "B2 非业务域时不输出红线块" \
	"$(printf '%s' "$out" | grep -q '^### ' && echo 1 || echo 0)"

# ---------------------------------------------------------------------------
# C1 未知域名拒绝
# ---------------------------------------------------------------------------
out="$(bash "$DIGEST" --domains not-a-domain 2>"$TMP/c1.err")"
rc=$?
check "C1 未知域名非零退出" "$([ "$rc" -ne 0 ] && echo 0 || echo 1)" "实际 exit=$rc"
check "C1 输出未知域名提示" \
	"$(grep -q '未知业务域：not-a-domain' "$TMP/c1.err" && echo 0 || echo 1)"
check "C1 输出可用业务域清单" \
	"$(grep -q '可用业务域.*daily-report' "$TMP/c1.err" && echo 0 || echo 1)"

# ---------------------------------------------------------------------------
# D1 红线逐字节对账（10 篇 flow 文档）
# ---------------------------------------------------------------------------
"${ESBUILD_ARGS[@]}" .pi/extensions/lib/constraint-redlines.ts --bundle --platform=node \
	--format=cjs --outfile="$TMP/redlines.cjs" >"$TMP/d1.esbuild.log" 2>&1
if [ ! -f "$TMP/redlines.cjs" ]; then
	bad "D1 红线提取实现 bundle 失败（无法对账）"
else
	ALL_DOMAINS="$(ls docs/reference/flow/*.md | grep -v '/README\.md$' | sed 's#.*/##; s#\.md$##' | LC_ALL=C sort | tr '\n' ',' | sed 's/,$//')"
	bash "$DIGEST" --domains "$ALL_DOMAINS" >"$TMP/d1.out" 2>"$TMP/d1.err"
	rc=$?
	check "D1 全域取数退出码 0" "$([ "$rc" -eq 0 ] && echo 0 || echo 1)" "实际 exit=$rc"
	node "$TMP/compare-redlines.cjs" "$TMP/redlines.cjs" "$TMP/d1.out" "$REPO_ROOT" >"$TMP/d1.report" 2>&1
	while IFS= read -r line; do
		case "$line" in
			"PASS: "*) ok "${line#PASS: }" ;;
			"FAIL: "*) bad "${line#FAIL: }" ;;
		esac
	done <"$TMP/d1.report"
fi

# ---------------------------------------------------------------------------
# E1 路径命中矩阵对账（脚本 vs extension scanAppliesTags）
# ---------------------------------------------------------------------------
cat >"$TMP/typebox-stub.cjs" <<'STUB'
const Type = new Proxy({}, { get: () => () => ({}) });
module.exports = { Type };
STUB
"${ESBUILD_ARGS[@]}" .pi/extensions/constraint-injection.ts --bundle --platform=node --format=cjs \
	--alias:typebox="$TMP/typebox-stub.cjs" --outfile="$TMP/ci.cjs" >"$TMP/e1.esbuild.log" 2>&1
if [ ! -f "$TMP/ci.cjs" ]; then
	bad "E1 extension bundle 失败（无法对账）"
else
	bash "$DIGEST" --paths \
		"backend-go/internal/admin/scheduler/job_daily_report.go,front/app/app.vue,front/app/stores/api.ts,backend-go/internal/reader/handler/reader_handler.go,backend-go/internal/app/router.go,docs/reference/flow/daily-report.md" \
		>"$TMP/e1.out" 2>"$TMP/e1.err"
	rc=$?
	check "E1 矩阵取数退出码 0" "$([ "$rc" -eq 0 ] && echo 0 || echo 1)" "实际 exit=$rc"
	node "$TMP/compare-paths.cjs" "$TMP/ci.cjs" "$TMP/e1.out" "$REPO_ROOT" \
		"backend-go/internal/admin/scheduler/job_daily_report.go" \
		"front/app/app.vue" \
		"front/app/stores/api.ts" \
		"backend-go/internal/reader/handler/reader_handler.go" \
		"backend-go/internal/app/router.go" \
		"docs/reference/flow/daily-report.md" >"$TMP/e1.report" 2>&1
	while IFS= read -r line; do
		case "$line" in
			"PASS: "*) ok "${line#PASS: }" ;;
			"FAIL: "*) bad "${line#FAIL: }" ;;
		esac
	done <"$TMP/e1.report"
fi

# ---------------------------------------------------------------------------
# F1 --brief 端到端契约对账：脚本 --brief 输出 → **真实** parseConstraintBrief 解析
#   契约失真是静默的（输出形态不匹配 → 记 absent / 收错域 → 派发带域比例系统性失真），
#   故必须调扩展 bundle 出来的真实实现，**禁止**在 smoke 里手写一份正则冒充。
# ---------------------------------------------------------------------------
cat >"$TMP/parse-brief.cjs" <<'CJS'
const fs = require("node:fs");
const { parseConstraintBrief } = require(process.argv[2]);
const brief = parseConstraintBrief(fs.readFileSync(process.argv[3], "utf8"));
console.log("DIGEST " + brief.digest);
for (const d of brief.domains) console.log("DOMAIN " + d);
CJS

"${ESBUILD_ARGS[@]}" .pi/extensions/harness-telemetry.ts --bundle --platform=node --format=cjs \
	--outfile="$TMP/tel.cjs" >"$TMP/f1.esbuild.log" 2>&1
if [ ! -f "$TMP/tel.cjs" ]; then
	bad "F1 telemetry bundle 失败（无法做端到端契约对账）"
else
	# F1a 单域：--brief --domains daily-report
	bash "$DIGEST" --brief --domains daily-report >"$TMP/f1a.out" 2>"$TMP/f1a.err"
	rc=$?
	check "F1a --brief --domains daily-report 退出码 0" "$([ "$rc" -eq 0 ] && echo 0 || echo 1)" "实际 exit=$rc"
	check "F1a 首行是「## 约束（红线，源 …）」标题" \
		"$(head -n 1 "$TMP/f1a.out" | grep -qx '## 约束（红线，源 docs/reference/flow/daily-report.md）' && echo 0 || echo 1)" \
		"实际首行：$(head -n 1 "$TMP/f1a.out")"
	check "F1a 段末固定兜底行" \
		"$(grep -qx '> 目标路径与本清单冲突时以 `--paths <你的目标>` 输出为准' "$TMP/f1a.out" && echo 0 || echo 1)"
	node "$TMP/parse-brief.cjs" "$TMP/tel.cjs" "$TMP/f1a.out" >"$TMP/f1a.parse" 2>&1
	check "F1a 真实 parseConstraintBrief 判 present" \
		"$(grep -qx 'DIGEST present' "$TMP/f1a.parse" && echo 0 || echo 1)" "解析结果：$(tr '\n' ' ' <"$TMP/f1a.parse")"
	check "F1a 真实解析收域 daily-report" \
		"$(grep -qx 'DOMAIN daily-report' "$TMP/f1a.parse" && echo 0 || echo 1)" "解析结果：$(tr '\n' ' ' <"$TMP/f1a.parse")"

	# F1b 多域：--brief --domains daily-report,scheduler（标题行源路径逗号分隔）
	bash "$DIGEST" --brief --domains daily-report,scheduler >"$TMP/f1b.out" 2>"$TMP/f1b.err"
	rc=$?
	check "F1b --brief 多域退出码 0" "$([ "$rc" -eq 0 ] && echo 0 || echo 1)" "实际 exit=$rc"
	check "F1b 只有单个「## 约束」标题行" \
		"$([ "$(grep -c '^## 约束（红线，源 ' "$TMP/f1b.out")" -eq 1 ] && echo 0 || echo 1)" \
		"实际 $(grep -c '^## 约束（红线，源 ' "$TMP/f1b.out") 个"
	check "F1b 标题行含两域源路径（逗号分隔）" \
		"$(head -n 1 "$TMP/f1b.out" | grep -qx '## 约束（红线，源 docs/reference/flow/daily-report.md, docs/reference/flow/scheduler.md）' && echo 0 || echo 1)" \
		"实际首行：$(head -n 1 "$TMP/f1b.out")"
	node "$TMP/parse-brief.cjs" "$TMP/tel.cjs" "$TMP/f1b.out" >"$TMP/f1b.parse" 2>&1
	check "F1b 真实解析判 present" \
		"$(grep -qx 'DIGEST present' "$TMP/f1b.parse" && echo 0 || echo 1)" "解析结果：$(tr '\n' ' ' <"$TMP/f1b.parse")"
	check "F1b 真实解析收域含 daily-report" \
		"$(grep -qx 'DOMAIN daily-report' "$TMP/f1b.parse" && echo 0 || echo 1)" "解析结果：$(tr '\n' ' ' <"$TMP/f1b.parse")"
	check "F1b 真实解析收域含 scheduler" \
		"$(grep -qx 'DOMAIN scheduler' "$TMP/f1b.parse" && echo 0 || echo 1)" "解析结果：$(tr '\n' ' ' <"$TMP/f1b.parse")"

	# F1c 路径反查 + brief：--brief --paths（命中 daily-report + scheduler 两域）
	bash "$DIGEST" --brief --paths "backend-go/internal/admin/scheduler/job_daily_report.go" \
		>"$TMP/f1c.out" 2>"$TMP/f1c.err"
	rc=$?
	check "F1c --brief --paths 退出码 0" "$([ "$rc" -eq 0 ] && echo 0 || echo 1)" "实际 exit=$rc"
	node "$TMP/parse-brief.cjs" "$TMP/tel.cjs" "$TMP/f1c.out" >"$TMP/f1c.parse" 2>&1
	check "F1c 真实解析判 present" \
		"$(grep -qx 'DIGEST present' "$TMP/f1c.parse" && echo 0 || echo 1)" "解析结果：$(tr '\n' ' ' <"$TMP/f1c.parse")"
	check "F1c 真实解析收域含 daily-report" \
		"$(grep -qx 'DOMAIN daily-report' "$TMP/f1c.parse" && echo 0 || echo 1)" "解析结果：$(tr '\n' ' ' <"$TMP/f1c.parse")"
	check "F1c 真实解析收域含 scheduler" \
		"$(grep -qx 'DOMAIN scheduler' "$TMP/f1c.parse" && echo 0 || echo 1)" "解析结果：$(tr '\n' ' ' <"$TMP/f1c.parse")"
fi

# ---------------------------------------------------------------------------
# F2 design D3 边界 fixture（临时目录；仓内所有标签都在第 3–6 行且无一文档两条标签，
#   这两个语义只有这两个 fixture 覆盖）：
#   F2a「标签写在文档头部 15 行窗口之外」→ 必须不命中（只读前 15 行）
#   F2b「同一文档头部两条 doc-impact-applies」→ 只取首条（首匹配即止）
#   对账基准 = 临时 fixture 自身（extension 侧 scanAppliesTags 扫整仓目录，在临时目录上
#   对账成本高于收益）；此处断言脚本侧行为符合 design D3 的两条语义。
# ---------------------------------------------------------------------------
FIX_ROOT="$TMP/fixture-docs"
mkdir -p "$FIX_ROOT/flow" "$FIX_ROOT/standard"
{
	echo "# fixture-late"
	echo
	for i in $(seq 3 15); do echo "<!-- filler line $i -->"; done
	echo "<!-- doc-impact-applies: zz-fixture-late/ -->"
} >"$FIX_ROOT/standard/fixture-late.md"
cat >"$FIX_ROOT/standard/fixture-multi.md" <<'MD'
# fixture-multi

<!-- doc-impact-applies: zz-fixture-multi/first -->
<!-- doc-impact-applies: zz-fixture-multi/second -->
MD
# 夹具自检：标签确实落在第 16 行（防构造时行数漂移）
check "F2 fixture-late 标签落在第 16 行（15 行窗口外）" \
	"$(awk 'NR==16' "$FIX_ROOT/standard/fixture-late.md" | grep -q 'doc-impact-applies: zz-fixture-late/' && echo 0 || echo 1)"

out="$(CONSTRAINT_DIGEST_DOC_ROOT="$FIX_ROOT" bash "$DIGEST" --paths "zz-fixture-late/src/x.go" 2>"$TMP/f2a.err")"
rc=$?
check "F2a 15 行外标签 → 退出码 0" "$([ "$rc" -eq 0 ] && echo 0 || echo 1)" "实际 exit=$rc"
check "F2a 15 行外标签 → 未命中说明" \
	"$(printf '%s' "$out" | grep -q '未命中任何 doc-impact-applies 标签' && echo 0 || echo 1)"
check "F2a 15 行外标签 → 不产生命中信号" \
	"$(printf '%s' "$out" | grep -q '信号：zz-fixture-late' && echo 1 || echo 0)"

out="$(CONSTRAINT_DIGEST_DOC_ROOT="$FIX_ROOT" bash "$DIGEST" --paths "zz-fixture-multi/first/src/x.go" 2>"$TMP/f2b.err")"
rc=$?
check "F2b 首条标签 → 退出码 0" "$([ "$rc" -eq 0 ] && echo 0 || echo 1)" "实际 exit=$rc"
check "F2b 首条标签 → 命中 fixture-multi.md" \
	"$(printf '%s' "$out" | grep -q 'fixture-multi.md' && echo 0 || echo 1)"
check "F2b 首条标签 → 信号取首条" \
	"$(printf '%s' "$out" | grep -q '信号：zz-fixture-multi/first' && echo 0 || echo 1)"
check "F2b 首条标签 → 不取第二条信号" \
	"$(printf '%s' "$out" | grep -q '信号：zz-fixture-multi/second' && echo 1 || echo 0)"

out="$(CONSTRAINT_DIGEST_DOC_ROOT="$FIX_ROOT" bash "$DIGEST" --paths "zz-fixture-multi/second/src/x.go" 2>"$TMP/f2c.err")"
rc=$?
check "F2c 只匹配第二条标签 → 退出码 0" "$([ "$rc" -eq 0 ] && echo 0 || echo 1)" "实际 exit=$rc"
check "F2c 只匹配第二条标签 → 未命中（首匹配即止）" \
	"$(printf '%s' "$out" | grep -q '未命中任何 doc-impact-applies 标签' && echo 0 || echo 1)"
check "F2c 只匹配第二条标签 → 不产生命中信号" \
	"$(printf '%s' "$out" | grep -q '信号：zz-fixture-multi' && echo 1 || echo 0)"

# ---------------------------------------------------------------------------
# 汇总
# ---------------------------------------------------------------------------
echo
echo "constraint-digest smoke：$pass 通过 / $failn 失败"
[ "$failn" -eq 0 ] || exit 1
exit 0
