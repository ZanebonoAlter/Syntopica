// failure-fingerprint smoke 断言（add-foreign-gate-steer-attribution test-cases 故事 S1 +
// 白盒分支表 + 任务 1.2 确定性；design D1 五规则）。
// 运行方式：node --experimental-strip-types failure-fingerprint.smoke.ts（直跑 import，
// 无 esbuild——failure-classify 是零依赖纯函数库）。由 run-harness-smoke.sh 调用。
// 断言失败 exit 1。
//
// 故事 S1（test-cases.md 主链路，步 2/步 3 落点 normalizeDiagFingerprint）：
// 2026-09-28 session 01a0e7f1-21eb-76de-825c-5279cb5c5ea6 的实账 go test 失败 diag
// （sqlite3 逐条导出，非构造样本）：`:260 ai_vendors` 系列 21 轮 + `:86 ai_route_models`
// 系列 17 轮 + 首轮 FAIL 行——时间戳逐轮必变是 23 轮近乎逐字重复 steer 轰炸的根因
// （旧 `prev.diag === diag` 精确相等比较永假）。修复后：系列内逐条归一化互相同
// （S1-1/S1-2），错误主体变化（缺表名不同）指纹不同（S1-3，spec scenario
// 「运行时动态值不构成新指纹」+ test-cases 反例行）。
import { normalizeDiagFingerprint } from "../lib/failure-classify.ts";

const checks: Array<[string, boolean]> = [];
const check = (name: string, ok: boolean): void => {
	checks.push([name, ok]);
};
const nf = normalizeDiagFingerprint;

/* ---------- 段一：2026-09-28 实账样本（故事 S1） ---------- */
{
	// 实账逐条导出的时间戳（events.db gate.check ok=false，cmd=go test -short ./internal/reader/...）
	const TS_260 = [
		"22:33:46", "22:35:39", "22:36:29", "22:37:03", "22:37:59", "22:38:39", "22:39:08",
		"22:40:49", "22:41:38", "22:42:08", "22:42:32", "22:42:54", "22:43:26", "22:43:49",
		"22:44:14", "22:44:35", "22:45:00", "22:45:22", "22:45:51", "22:46:17", "22:46:39",
	];
	const TS_86 = [
		"22:47:10", "22:47:37", "22:48:03", "22:48:27", "22:48:53", "22:49:21", "22:49:53",
		"22:50:22", "22:50:42", "22:51:06", "22:51:28", "22:51:51", "22:52:12", "22:52:34",
		"22:56:07", "22:58:16", "22:58:57",
	];
	const diagAt = (ts: string, line: string, table: string): string =>
		`2026/09/28 ${ts} /home/zanebono/software/Syntopica/backend-go/internal/platform/airouter/store_vendor.go:${line} SQL logic error: no such table: ${table} (1)`;
	const seriesA = TS_260.map((ts) => diagAt(ts, "260", "ai_vendors"));
	const seriesB = TS_86.map((ts) => diagAt(ts, "86", "ai_route_models"));

	const fpsA = seriesA.map(nf);
	check(
		"S1-1 实账 `:260 ai_vendors` 系列 21 轮逐条归一化互相同（时间戳免疫）",
		seriesA.length === 21 && fpsA.length === 21 && fpsA.every((f) => f === fpsA[0]),
	);
	const fpsB = seriesB.map(nf);
	check(
		"S1-2 实账 `:86 ai_route_models` 系列 17 轮逐条归一化互相同",
		seriesB.length === 17 && fpsB.length === 17 && fpsB.every((f) => f === fpsB[0]),
	);
	check(
		"S1-3 错误文本主体不同（缺表名 ai_vendors vs ai_route_models）→ 指纹不同（重新全文注入）",
		fpsA[0] !== fpsB[0],
	);
	check(
		"S1-4 首轮 FAIL 行（无动态值）归一化后保持原文形态",
		nf("FAIL syntopica-backend/internal/reader [build failed]") ===
			"FAIL syntopica-backend/internal/reader [build failed]",
	);
	// 变体走查（test-cases.md）：时间窗口——跨分钟/跨日时间戳仍同指纹（规则 1 + 数字序列压平覆盖）
	check(
		"S1-5 时间窗口：跨日时间戳（22:59 → 次日 23:00）仍同指纹",
		nf("2026/09/28 22:59:59 doing x") === nf("2026/09/29 23:00:01 doing x"),
	);
	// 变体走查：空 diag / 纯时间戳行 → 空串族（同失败静默，保守多静默优于逐轮轰炸）
	check(
		"S1-6 纯时间戳行归一化为空串（与空 diag 同指纹族）",
		nf("2026/09/28 22:33:46") === "" && nf("") === "" && nf("2026/09/28 22:35:39") === nf(""),
	);
}

/* ---------- 段二：白盒分支表（test-cases.md normalizeDiagFingerprint 分支 W1-W8） ---------- */
{
	check(
		"W1 日志时间戳剥除：仅时间戳不同 → 相等",
		nf("2026/09/28 22:33:46 INFO icon...") === nf("2026/09/28 22:35:39 INFO icon..."),
	);
	check(
		"W1b RFC3339 形态（T 分隔 + 毫秒）剥除",
		nf("2026-09-28T22:33:46.123 INFO x") === nf("2026-09-28T22:35:39.456 INFO x"),
	);
	check(
		"W2 IP:port 剥除：仅端口不同 → 相等",
		nf("http://127.0.0.1:34505/favicon.ico") === nf("http://127.0.0.1:38117/favicon.ico"),
	);
	check(
		"W2b URL host:port 端口段剥除（非 IP host）",
		nf("GET http://localhost:3000/api/x failed") === nf("GET http://localhost:3001/api/x failed"),
	);
	check(
		"W3 耗时括号/毫秒剥除：(0.02s) vs (0.03s)、[0.029ms] vs [0.049ms]",
		nf("FAIL pkg (0.02s) [0.029ms]") === nf("FAIL pkg (0.03s) [0.049ms]"),
	);
	check(
		"W4 数字序列压平：裸耗时 10.793s vs 9.283s",
		nf("FAIL pkg 10.793s") === nf("FAIL pkg 9.283s"),
	);
	check(
		"W4b 字母邻接数字保留：e1 vs e7 → 不相等（错误文本主体，宁可多报）",
		nf("internal/mine/a.go:1:1: e1") !== nf("internal/mine/a.go:7:7: e7"),
	);
	check(
		"W4c 字母邻接数字保留：models2 vs models3 / go1.27 vs go1.30 → 不相等",
		nf("a.go:1:1: undefined: models2") !== nf("a.go:1:1: undefined: models3")
			&& nf("requires go1.27") !== nf("requires go1.30"),
	);
	check(
		"W5 仓库根前缀剥除：绝对路径 vs 仓库相对路径 → 相等",
		nf("/home/x/Syntopica/backend-go/a.go:1:1: e") === nf("backend-go/a.go:1:1: e"),
	);
	check(
		"W6 错误主体保留：undefined: modelsA vs modelsB → 不相等",
		nf("a.go:1:1: undefined: modelsA") !== nf("a.go:1:1: undefined: modelsB"),
	);
	check("W7 空输入 → 空串", nf("") === "");
	check(
		"W8 组合风暴：时间戳 + 端口 + 耗时 + 行号全变 → 相等",
		nf("2026/09/28 22:33:46 GET http://127.0.0.1:34505/x (0.02s) store.go:260: boom") ===
			nf("2026/09/28 22:44:35 GET http://127.0.0.1:38117/x (0.03s) store.go:86: boom"),
	);
	// 非仓库根的绝对路径前缀保留（规则 5 仅剥 /…/Syntopica/ 仓库根，不误伤其他绝对路径）
	check(
		"W5b 非本仓库绝对路径不被剥除（仅数字压平参与）",
		nf("/usr/local/go/src/x.go:1:1: e") === "/usr/local/go/src/x.go:<N>:<N>: e",
	);
}

/* ---------- 段三：确定性断言（任务 1.2：相同输入字节两次调用输出恒等 / 规则幂等稳定） ---------- */
{
	const sample =
		"2026/09/28 22:33:46 /home/zanebono/software/Syntopica/backend-go/internal/platform/airouter/store_vendor.go:260 SQL logic error: no such table: ai_vendors (1)";
	const once = nf(sample);
	check("D1 相同输入字节两次调用输出恒等（无隐藏状态）", once === nf(sample) && once === nf(sample));
	check("D2 规则幂等：归一化输出再归一化恒等（输出不含可再剥形态）", nf(once) === once);
	// 实账全样本双轮调用一致（39 条 = 21 + 17 + 1 FAIL 行）
	const all = [
		...[...Array(2)].map(() => sample),
		"FAIL syntopica-backend/internal/reader [build failed]",
	];
	const round1 = all.map(nf);
	const round2 = all.map(nf);
	check("D3 实账样本两轮全量调用逐一一致", round1.every((f, i) => f === round2[i]));
	// null/undefined 非字符串输入安全返回空串（防 defensive，对齐 isInteropFailure I8 风格）
	check(
		"D4 null/undefined 输入安全返回空串",
		nf(null as unknown as string) === "" && nf(undefined as unknown as string) === "",
	);
}

let fail = 0;
for (const [name, ok] of checks) {
	console.log(`${ok ? "✅" : "❌"} ${name}`);
	if (!ok) fail++;
}
console.log(fail ? `\n${fail} 项失败` : "\nSMOKE OK");
if (fail) process.exitCode = 1;
