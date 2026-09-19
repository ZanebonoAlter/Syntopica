#!/usr/bin/env node
/* session-rollup.smoke — 纯函数直测（harness-effectiveness-metrics test-cases 白盒附加）
 * 覆盖：B1~B10 分支表 + 节流边界 + 回填判定 + 增量幂等/残行重读 + prev 文件名 UUID 提取。
 * 跑法：node .pi/extensions/tests/session-rollup.smoke.cjs（由 run-harness-smoke.sh esbuild 后调）。
 * 输入形态来自实测 jsonl（2026-09-17 考古）：{type:'message',timestamp,message:{role,usage}}。 */
"use strict";
const assert = require("node:assert");

/* ---- stub 注入（run-harness-smoke.sh bundle 后 globalThis 上无模块系统时用直载） ---- */
let mod;
try {
	mod = require("../lib/session-rollup.ts");
} catch {
	mod = require("./.srollup.cjs");
}
const {
	splitCompleteLines,
	parseSessionText,
	consumeLines,
	newAccumulator,
	accumulateFromFile,
	shouldWriteSnapshot,
	shouldBackfillPrev,
	readParentSessionId,
	findPrevSessionFile,
	sessionFileIdOf,
} = mod;

/* ---- fixture：贴近真实 jsonl 行 ---- */
const L = {
	session: `{"type":"session","version":1,"id":"aaa","timestamp":"2026-09-17T03:07:24.875Z","cwd":"/x"}`,
	user: (ts, text) =>
		`{"type":"message","id":"u1","timestamp":"${ts}","message":{"role":"user","content":[{"type":"text","text":"${text ?? "hi"}"}]}}`,
	assistant: (ts, u, model) =>
		`{"type":"message","id":"a1","timestamp":"${ts}","message":{"role":"assistant","content":[{"type":"text","text":"ok"}]${
			model ? `,"model":"${model}"` : ""
		}${u ? `,"usage":${JSON.stringify(u)}` : ""}}}`,
	toolResult: (ts) =>
		`{"type":"message","id":"t1","timestamp":"${ts}","message":{"role":"toolResult","content":[{"type":"text","text":"done"}]}}`,
	modelChange: (ts, m) =>
		`{"type":"model_change","id":"m1","timestamp":"${ts}","provider":"zai","modelId":"${m}"}`,
	custom: `{"type":"custom_message","id":"c1","timestamp":"2026-09-17T03:07:25.000Z"}`,
};
const usage = (i, o, cr, t, cost) => ({
	input: i,
	output: o,
	cacheRead: cr,
	cacheWrite: 0,
	totalTokens: t,
	...(cost ? { cost: { total: cost } } : {}),
});

let pass = 0;
const ok = (name, fn) => {
	fn();
	pass++;
	console.log(`  ✓ ${name}`);
};

/* ---- B1~B10：consumeLines/parseSessionText 分支表 ---- */
ok("B1 坏行跳过计数（空串/截断 JSON/全角空白）", () => {
	const s = parseSessionText(
		["", "{truncated", "　", L.user("2026-09-17T03:00:00Z")].join("\n") + "\n",
	);
	assert.equal(s.skippedLines, 1); // 空行与全角空白不计坏行、不计数；仅截断 JSON 计 1
	assert.equal(s.turns, 1);
});
ok("B2 非 message 行忽略", () => {
	const s = parseSessionText([L.session, L.custom, L.modelChange("2026-09-17T03:00:00Z", "glm-5.3")].join("\n") + "\n");
	assert.equal(s.turns + s.steps + s.toolCalls, 0);
	assert.equal(s.model, "glm-5.3"); // B10：model_change 取最后一条
});
ok("B3/B4/B5/B6 计数与 token 累计", () => {
	const s = parseSessionText(
		[
			L.user("2026-09-17T03:00:00Z"),
			L.assistant("2026-09-17T03:00:10Z", usage(100, 50, 1000, 1150, 0.01)),
			L.assistant("2026-09-17T03:00:20Z"), // 无 usage：steps+1、tokens+0（B5）
			L.toolResult("2026-09-17T03:00:30Z"),
		].join("\n") + "\n",
	);
	assert.equal(s.turns, 1);
	assert.equal(s.steps, 2);
	assert.equal(s.toolCalls, 1);
	assert.equal(s.tokens.total, 1150); // total 以 totalTokens 为准（B7）
	assert.equal(s.tokens.input, 100);
	assert.equal(s.cost, 0.01); // B8：有值累计
});
ok("B7/B8 缺 cost 与 totalTokens 口径", () => {
	const s = parseSessionText([L.assistant("2026-09-17T03:00:00Z", { input: 5 })].join("\n") + "\n");
	assert.equal(s.cost, null);
	assert.equal(s.tokens.total, 0); // totalTokens 缺 → 0
});
ok("B9 单消息 durationSec=0；首末差计算", () => {
	const s0 = parseSessionText([L.user("2026-09-17T03:00:00Z")].join("\n") + "\n");
	assert.equal(s0.durationSec, 0);
	const s1 = parseSessionText(
		[L.user("2026-09-17T03:00:00Z"), L.user("2026-09-17T03:01:30Z")].join("\n") + "\n",
	);
	assert.equal(s1.durationSec, 90);
});
ok("空文本零值（空集变体）", () => {
	const s = parseSessionText("");
	assert.deepEqual(
		{ ...s, tokens: undefined },
		{ turns: 0, steps: 0, toolCalls: 0, cost: null, durationSec: 0, model: null, models: {}, skippedLines: 0, tokens: undefined },
	);
});

/* ---- task-cost-metrics：models 逐模型聚合 ---- */
ok("models 多模型归属：各桶之和=总 cost，tokens 各自分项累计", () => {
	const s = parseSessionText(
		[
			L.user("2026-09-19T03:00:00Z"),
			L.assistant("2026-09-19T03:00:10Z", usage(100, 50, 1000, 1150, 0.3), "glm-5.3"),
			L.assistant("2026-09-19T03:00:20Z", usage(100, 50, 1150, 1250, 0.3), "glm-5.3"),
			L.assistant("2026-09-19T03:00:30Z", usage(200, 80, 1500, 1780, 0.1), "kimi-k2"),
		].join("\n") + "\n",
	);
	assert.deepEqual(Object.keys(s.models).sort(), ["glm-5.3", "kimi-k2"]);
	assert.equal(s.models["glm-5.3"].cost, 0.6);
	assert.equal(s.models["glm-5.3"].tokens.total, 2400); // 1150+1250
	assert.equal(s.models["glm-5.3"].tokens.input, 200);
	assert.equal(s.models["kimi-k2"].cost, 0.1);
	assert.equal(s.models["kimi-k2"].tokens.output, 80);
	const sum = Object.values(s.models).reduce((a, m) => a + (m.cost ?? 0), 0);
	assert.ok(Math.abs(sum - 0.7) < 1e-9, `桶和 ${sum} ≠ 总 ${s.cost}`); // 对账：各桶之和=会话总 cost
	assert.ok(Math.abs(s.cost - 0.7) < 1e-9);
});
ok("models 无 model 字段的消息计 unknown 桶（不丢不并桶）", () => {
	const s = parseSessionText(
		[
			L.assistant("2026-09-19T03:00:00Z", usage(10, 5, 0, 15, 0.02), "glm-5.3"),
			L.assistant("2026-09-19T03:00:10Z", usage(20, 8, 0, 28, 0.03)), // 无 model → unknown
			L.assistant("2026-09-19T03:00:20Z"), // 无 usage：不入任何桶
		].join("\n") + "\n",
	);
	assert.deepEqual(Object.keys(s.models).sort(), ["glm-5.3", "unknown"]);
	assert.equal(s.models.unknown.cost, 0.03);
	assert.equal(s.models.unknown.tokens.total, 28);
	assert.equal(s.models["glm-5.3"].cost, 0.02);
	assert.ok(Math.abs((s.models.unknown.cost ?? 0) + (s.models["glm-5.3"].cost ?? 0) - 0.05) < 1e-9);
});
ok("models 全缺 cost 时桶 cost 保持 null（不伪造成 0）", () => {
	const s = parseSessionText([L.assistant("2026-09-19T03:00:00Z", { input: 5 }, "glm-5.3")].join("\n") + "\n");
	assert.equal(s.models["glm-5.3"].cost, null);
	assert.equal(s.models["glm-5.3"].tokens.input, 5);
});

/* ---- splitCompleteLines：残行语义 ---- */
ok("残行留存：无 \\n 全是 remainder", () => {
	const r = splitCompleteLines("abc", "");
	assert.deepEqual(r, { lines: [], remainder: "abc" });
});
ok("残行拼接：prevRemainder + chunk 按最后 \\n 切", () => {
	const r = splitCompleteLines('{"partial":1}\n{"next', '{"type":"mess');
	assert.equal(r.lines.length, 1);
	assert.equal(r.lines[0], '{"type":"mess{"partial":1}'); // 残行与新块首行拼接
	assert.equal(r.remainder, '{"next');
});

/* ---- 增量 accumulateFromFile：幂等 / 残行重读 / 文件缺失 ---- */
const os = require("node:os");
const fs = require("node:fs");
const path = require("node:path");
const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "srollup-"));
ok("增量消费 + 幂等（重复调零变化）+ 残行补齐", () => {
	const f = path.join(tmp, "a.jsonl");
	fs.writeFileSync(f, L.user("2026-09-17T03:00:00Z") + "\n" + L.assistant("2026-09-17T03:00:10Z", usage(1, 2, 3, 6, 0.001)));
	const acc = newAccumulator();
	const a1 = accumulateFromFile(acc, f);
	assert.equal(a1.summary.turns, 1); // 末尾无 \n → 整行是残行，暂不消费
	assert.equal(a1.summary.steps, 0);
	assert.equal(a1.offset, Buffer.byteLength(L.user("2026-09-17T03:00:00Z") + "\n")); // offset=已消费完整行末尾
	const a2 = accumulateFromFile(a1, f); // 幂等：无新增字节零变化
	assert.deepEqual(a2.summary, a1.summary);
	fs.appendFileSync(f, "\n"); // 补换行 → 残行转完整行
	const a3 = accumulateFromFile(a2, f);
	assert.equal(a3.summary.steps, 1);
	assert.equal(a3.summary.tokens.total, 6);
	assert.ok(a3.offset > 0);
});
ok("文件不存在 → null（零写入 fail-open）", () => {
	assert.equal(accumulateFromFile(newAccumulator(), path.join(tmp, "nope.jsonl")), null);
});
ok("多字节截断：残行整行重读后正确解码（offset 不动语义）", () => {
	const f = path.join(tmp, "b.jsonl");
	const text = L.user("2026-09-17T03:00:00Z") + "\n";
	fs.writeFileSync(f, Buffer.from(text, "utf8"));
	const acc = newAccumulator();
	accumulateFromFile(acc, f);
	assert.equal(acc.summary.turns, 1);
	// 追加残行（多字节字符结尾、无 \\n）→ offset 不动、不计数
	fs.appendFileSync(f, Buffer.from('{"type":"message","timestamp":"2026-09-17T03:00:05Z","message":{"role":"user","content":[{"type":"text","text":"中文未完', "utf8"));
	const a2 = accumulateFromFile(acc, f);
	assert.equal(a2.summary.turns, 1); // 残行未消费
	assert.ok(a2.remainder.length > 0);
	const offsetBefore = a2.offset;
	// 补完并加换行 → 残行整行重读成功
	fs.appendFileSync(f, Buffer.from('"}]}}\n', "utf8"));
	const a3 = accumulateFromFile(a2, f);
	assert.equal(a3.summary.turns, 2); // 重读后成功解析
	assert.ok(a3.offset > offsetBefore);
});

/* ---- 节流边界（白盒边界表） ---- */
ok("节流边界：turn%5 / >20% 严格 / bootstrap / turn0", () => {
	assert.equal(shouldWriteSnapshot(4, 1000, 1100), false); // 增量 10%、非 5 倍数
	assert.equal(shouldWriteSnapshot(5, 1000, 1001), true); // 5 倍数
	assert.equal(shouldWriteSnapshot(4, 1000, 1201), true); // >20% 严格
	assert.equal(shouldWriteSnapshot(4, 1000, 1200), false); // =20% 整不写
	assert.equal(shouldWriteSnapshot(1, null, 0), true); // bootstrap
	assert.equal(shouldWriteSnapshot(0, 500, 500), false); // turn0 不写
});

/* ---- 回填判定 ---- */
ok("回填判定：prev 在/无 final/turns>0", () => {
	const s = parseSessionText([L.user("2026-09-17T03:00:00Z")].join("\n") + "\n");
	assert.equal(shouldBackfillPrev("/tmp/x.jsonl", s, false), true);
	assert.equal(shouldBackfillPrev(null, s, false), false);
	assert.equal(shouldBackfillPrev("/tmp/x.jsonl", s, true), false); // 已有 final 幂等
	const empty = parseSessionText("");
	assert.equal(shouldBackfillPrev("/tmp/x.jsonl", empty, false), false); // 无效会话不回填
});

/* ---- telemetry 侧 prev 文件名 UUID 提取（bundle 后从 .tel.cjs 取） ---- */
ok("sessionIdFromSessionFile 形态", () => {
	let fn;
	try {
		const tel = require("../harness-telemetry.ts");
		fn = tel.sessionIdFromSessionFile;
	} catch {
		if (!fs.existsSync("./.tel.cjs")) {
			console.log("  (skip: telemetry bundle 未生成，run-harness-smoke.sh 内会覆盖)");
			return;
		}
		fn = require("./.tel.cjs").sessionIdFromSessionFile;
	}
	assert.equal(
		fn("/home/u/.pi/agent/sessions/dir/2026-09-17T03-07-24-184Z_01a0ad55-4e98-71fa-8fc9-b20801848209.jsonl"),
		"01a0ad55-4e98-71fa-8fc9-b20801848209",
	);
	assert.equal(fn("/tmp/nope.jsonl"), null);
});

/* ---- task-cost-metrics：parentSessionId 提取 + prev 文件定位 ---- */
const UUID_A = "01a0b7c5-a04d-7151-93b8-dd17aca514a6";
const UUID_B = "01a0b7f5-6697-7151-93b8-dd1a41398de8";
ok("readParentSessionId 正常路径（子会话头指向父文件 → 父 UUID）", () => {
	const f = path.join(tmp, "child.jsonl");
	const parentFile = `/home/u/.pi/agent/sessions/dir/2026-09-19T03-46-17-293Z_${UUID_A}.jsonl`;
	fs.writeFileSync(
		f,
		`{"type":"session","parentSession":"${parentFile}","id":"${UUID_B}"}\n{"type":"message"}\n`,
	);
	assert.equal(readParentSessionId(f), UUID_A);
});
ok("readParentSessionId 主会话（parentSession null/缺字段）→ null", () => {
	const f1 = path.join(tmp, "main1.jsonl");
	fs.writeFileSync(f1, `{"type":"session","parentSession":null,"id":"${UUID_A}"}\n`);
	assert.equal(readParentSessionId(f1), null);
	const f2 = path.join(tmp, "main2.jsonl");
	fs.writeFileSync(f2, `{"type":"session","id":"${UUID_A}"}\n`);
	assert.equal(readParentSessionId(f2), null);
});
ok("readParentSessionId 坏头（非 JSON/空文件/不存在/坏路径）→ null fail-open", () => {
	const bad = path.join(tmp, "bad.jsonl");
	fs.writeFileSync(bad, "not json at all\n");
	assert.equal(readParentSessionId(bad), null);
	const empty = path.join(tmp, "empty.jsonl");
	fs.writeFileSync(empty, "");
	assert.equal(readParentSessionId(empty), null);
	assert.equal(readParentSessionId(path.join(tmp, "nope.jsonl")), null);
	const weird = path.join(tmp, "weird.jsonl");
	fs.writeFileSync(weird, `{"type":"session","parentSession":"/tmp/随便一个名字.jsonl"}\n`);
	assert.equal(readParentSessionId(weird), null); // basename 提不出 UUID → null
});
ok("findPrevSessionFile：mtime 最新胜出、排除当前会话与杂项、并列 mtime 取文件名字典序大者", () => {
	const mk = (iso, id, mtimeMs) => ({
		file: `/home/u/.pi/agent/sessions/dir/${iso}_${id}.jsonl`,
		mtimeMs,
	});
	const cur = UUID_B;
	const eOld = mk("2026-09-19T03-46-17-293Z", UUID_A, 1000);
	const eNew = mk("2026-09-19T05-22-59-670Z", "01a0b81e-29d6-7151-93b8-dd240cc1b930", 2000);
	const eCur = mk("2026-09-19T05-25-29-200Z", cur, 9999); // 当前会话（必须被排除）
	const eJunk = { file: "/home/u/.pi/agent/sessions/dir/readme.txt", mtimeMs: 99999 }; // 提不出 UUID
	assert.equal(findPrevSessionFile([eOld, eNew, eCur, eJunk], cur), eNew.file);
	// 并列 mtime：取文件名字典序大者（ISO 前缀新者胜，确定性）
	const eTieA = mk("2026-09-19T03-00-00-000Z", UUID_A, 1500);
	const eTieB = mk("2026-09-19T04-00-00-000Z", "01a0b78d-9fa7-7151-93b8-dd10c0056c61", 1500);
	assert.equal(findPrevSessionFile([eTieA, eTieB], cur), eTieB.file);
	assert.equal(findPrevSessionFile([eTieB, eTieA], cur), eTieB.file); // 顺序无关
});
ok("findPrevSessionFile：空目录/仅当前会话 → null", () => {
	assert.equal(findPrevSessionFile([], UUID_A), null);
	assert.equal(
		findPrevSessionFile(
			[{ file: `/x/2026-09-19T05-25-29-200Z_${UUID_A}.jsonl`, mtimeMs: 1 }],
			UUID_A,
		),
		null,
	);
});
ok("sessionFileIdOf 形态（readParentSessionId/findPrevSessionFile 复用）", () => {
	assert.equal(
		sessionFileIdOf(`/x/2026-09-19T05-29-11-493Z_${UUID_B}.jsonl`),
		UUID_B,
	);
	assert.equal(sessionFileIdOf("/tmp/nope.jsonl"), null);
	assert.equal(sessionFileIdOf("/x/only-uuid-less.jsonl"), null);
});

fs.rmSync(tmp, { recursive: true, force: true });
console.log(`session-rollup smoke: ${pass} 组断言全绿`);
