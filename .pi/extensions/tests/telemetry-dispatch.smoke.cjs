#!/usr/bin/env node
/* telemetry-dispatch.smoke — subagent.dispatch change 归属记账白盒（add-review-trace-gate 2.2）
 *
 * 设计 D2：dispatch 落账补 change 列（与 session.rollup 快照同源 detectActiveChange，
 * 一次判定两用 agentChangeBindings + 事件 change 列）；既有行不可变（追加不改写）。
 *
 * 集成（仿 telemetry-archive.smoke.cjs）：mock pi 挂 tool_call/tool_result 钩子 →
 * 临时 cwd 落真库（events.db）→ 断言 change 列与行序：
 *   1. 有活跃 change 目录（openspec/changes/<name>）→ dispatch 行 change=目录名
 *   2. 无非 archive 目录（只有 archive/）→ change=null，事件照常落库不阻断
 *   3. 既有行不可变：先无归属记一条（null）→ 造目录再记一条（带值）→ 旧行仍 null、
 *      行数追加为 2（不改写）
 * 追加（add-dispatch-constraint-digest 3.3）：派发约束段解析纯函数 parseConstraintBrief
 *   + payload 追加 constraintDigest/constraintDomains（append-only 派生值，不落任务正文）：
 *   4. 带约束段 → present + ["daily-report"]；无约束段 → absent + []
 *   5. 段在但无 flow/ 源路径 → present + []（不猜域）
 *   6. 多域去重 + 顺序稳定 + 段边界（下一个 `## ` 之后的源路径不计入）
 *   7. brief 形态（constraint-digest.sh --brief 真实输出）→ present + 多域
 *   8. 源路径缺 docs/reference/ 前缀 → 不收域（present + []，review R-3 收紧口径）
 * harness-telemetry.ts 直接 require（相对导入无扩展名）会失败 → 现场 esbuild bundle
 * 再加载（与 run-harness-smoke.sh 的 .tel.cjs 同产物，此处独立生成保证单跑自足）。
 * 跑法：node .pi/extensions/tests/telemetry-dispatch.smoke.cjs */
"use strict";
const assert = require("node:assert");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { execSync } = require("node:child_process");

/* node:sqlite ExperimentalWarning 抑制（与 lib/harness-log 同策略） */
const priorWarn = process.listeners("warning");
process.removeAllListeners("warning");
process.on("warning", (w) => {
	if (!(w.name === "ExperimentalWarning" && /sqlite/i.test(w.message))) {
		for (const l of priorWarn) l(w);
	}
});

let pass = 0;
const ok = (name, fn) => {
	fn();
	pass++;
	console.log(`  ✓ ${name}`);
};

/* 现场 bundle（相对导入无扩展名，直接 require 会失败）
 * esbuild 解析离线优先（review R-13）：本仓已装 → npx 缓存 → 最后才 npx -y（需联网） */
const REPO_ROOT = path.resolve(__dirname, "../../..");
const cmdWorks = (c) => {
	try {
		execSync(c, { stdio: "ignore" });
		return true;
	} catch {
		return false;
	}
};
function esbuildCmd() {
	const tries = [
		{ cmd: "esbuild", probe: () => cmdWorks("esbuild --version") },
		{
			cmd: JSON.stringify(path.join(REPO_ROOT, "node_modules/.bin/esbuild")),
			probe: () => fs.existsSync(path.join(REPO_ROOT, "node_modules/.bin/esbuild")),
		},
		{ cmd: "npx --no-install esbuild", probe: () => cmdWorks("npx --no-install esbuild --version") },
	];
	for (const t of tries) if (t.probe()) return t.cmd;
	return "npx -y esbuild";
}

const TMP_BUNDLE = path.join(fs.mkdtempSync(path.join(os.tmpdir(), "tdisp-")), "tel-bundle.cjs");
execSync(
	`${esbuildCmd()} ${JSON.stringify(path.resolve(__dirname, "../harness-telemetry.ts"))} --bundle --platform=node --format=cjs --outfile=${JSON.stringify(TMP_BUNDLE)}`,
	{ cwd: __dirname, stdio: ["ignore", "ignore", "ignore"] },
);
const telemetry = require(TMP_BUNDLE);

/** dispatch 一次 Agent 子线程（tool_call 暂存起点 + tool_result 落账），返回触发的 handlers
 *  prompt 可选（add-dispatch-constraint-digest：约束段记账用例需要真实任务文本） */
const dispatchAgent = (handlers, ctx, id, prompt = "x".repeat(10)) => {
	const input = { subagent_type: "explore", description: `派发 ${id}`, prompt };
	handlers["tool_call"](
		{ toolName: "Agent", toolCallId: `tc-${id}`, input },
		ctx,
	);
	handlers["tool_result"](
		{
			toolName: "Agent",
			toolCallId: `tc-${id}`,
			isError: false,
			input,
			details: { tokens: "12.3k", durationMs: 4567, status: "completed", agentId: `ab12cd3-${id}` },
		},
		ctx,
	);
};

const makeTelemetry = (cwd) => {
	const handlers = {};
	const pi = { on: (name, fn) => (handlers[name] = fn) };
	telemetry.default(pi);
	const ctx = { cwd, hasUI: false, sessionManager: { getSessionId: () => "s-disp-test" } };
	return { handlers, ctx };
};

const dispatchRows = (cwd) => {
	const dbFile = path.join(cwd, ".pi", "harness", "events.db");
	assert.ok(fs.existsSync(dbFile), "events.db 应已落库");
	const { DatabaseSync } = require("node:sqlite");
	const db = new DatabaseSync(dbFile);
	const rows = db.prepare("SELECT id, kind, change, payload FROM events WHERE kind='subagent.dispatch' ORDER BY id").all();
	db.close();
	return rows;
};

ok("有活跃 change 目录 → dispatch 行 change=目录名（payload 契约不变）", () => {
	const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "tdisp-a-"));
	try {
		fs.mkdirSync(path.join(tmp, "openspec", "changes", "demo-active"), { recursive: true });
		fs.writeFileSync(path.join(tmp, "openspec", "changes", "demo-active", "proposal.md"), "<!-- ui-impact: none -->\n");
		const { handlers, ctx } = makeTelemetry(tmp);
		dispatchAgent(handlers, ctx, 1);
		const rows = dispatchRows(tmp);
		assert.equal(rows.length, 1, `应恰一条 dispatch（实际 ${rows.length}）`);
		assert.equal(rows[0].change, "demo-active", `change 列应为目录名（实际 ${rows[0].change}）`);
		const payload = JSON.parse(rows[0].payload);
		assert.equal(payload.type, "explore");
		assert.equal(payload.agentId, "ab12cd3-1");
		assert.equal(payload.isError, false);
	} finally {
		fs.rmSync(tmp, { recursive: true, force: true });
	}
});

ok("无非 archive 目录 → change=null，事件照常落库不阻断", () => {
	const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "tdisp-b-"));
	try {
		fs.mkdirSync(path.join(tmp, "openspec", "changes", "archive"), { recursive: true }); // 只有 archive/
		const { handlers, ctx } = makeTelemetry(tmp);
		dispatchAgent(handlers, ctx, 2);
		const rows = dispatchRows(tmp);
		assert.equal(rows.length, 1, `应恰一条 dispatch（实际 ${rows.length}）`);
		assert.equal(rows[0].change, null, `change 列应为 null（实际 ${rows[0].change}）`);
	} finally {
		fs.rmSync(tmp, { recursive: true, force: true });
	}
});

ok("既有行不可变：先无归属（null）→ 后带值，追加不改写（旧行仍 null）", () => {
	const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "tdisp-c-"));
	try {
		// 第一步：无 change 目录环境派发 → 落 change=null
		fs.mkdirSync(path.join(tmp, "openspec", "changes", "archive"), { recursive: true });
		const t1 = makeTelemetry(tmp);
		dispatchAgent(t1.handlers, t1.ctx, 3);
		let rows = dispatchRows(tmp);
		assert.equal(rows.length, 1);
		assert.equal(rows[0].change, null, "首条应无归属");

		// 第二步：同库再造活跃 change 目录派发 → 追加带值行，旧行不改写
		fs.mkdirSync(path.join(tmp, "openspec", "changes", "later-change"), { recursive: true });
		fs.writeFileSync(path.join(tmp, "openspec", "changes", "later-change", "proposal.md"), "<!-- ui-impact: none -->\n");
		const t2 = makeTelemetry(tmp);
		dispatchAgent(t2.handlers, t2.ctx, 4);
		rows = dispatchRows(tmp);
		assert.equal(rows.length, 2, `应追加为两条（实际 ${rows.length}）`);
		assert.equal(rows[0].change, null, "既有行不被改写（仍 null）");
		assert.equal(rows[1].change, "later-change", "新行带归属");
	} finally {
		fs.rmSync(tmp, { recursive: true, force: true });
	}
});

ok("后台派发 agentChangeBindings 复用同一次判定（complete 回填 change 与 dispatch 同源）", () => {
	const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "tdisp-d-"));
	try {
		fs.mkdirSync(path.join(tmp, "openspec", "changes", "bind-change"), { recursive: true });
		fs.writeFileSync(path.join(tmp, "openspec", "changes", "bind-change", "proposal.md"), "<!-- ui-impact: none -->\n");
		const { handlers, ctx } = makeTelemetry(tmp);
		// 后台派发（run_in_background=true + agentId）→ dispatch 带 change；complete 回填同值
		handlers["tool_call"](
			{ toolName: "Agent", toolCallId: "tc-bg", input: { subagent_type: "general-purpose", description: "后台任务", prompt: "y", run_in_background: true } },
			ctx,
		);
		handlers["tool_result"](
			{
				toolName: "Agent",
				toolCallId: "tc-bg",
				isError: false,
				input: { subagent_type: "general-purpose", description: "后台任务", prompt: "y", run_in_background: true },
				details: { durationMs: 10, status: "background", agentId: "ab12cd34-ef01" },
			},
			ctx,
		);
		handlers["tool_result"](
			{
				toolName: "get_subagent_result",
				toolCallId: "tc-get",
				isError: false,
				content: [{ type: "text", text: "Agent: ab12cd34-ef01\nType: Agent | Status: completed | Tool uses: 3 | 1.2M token | Context: 40% | Duration: 12.0s" }],
			},
			ctx,
		);
		const dbFile = path.join(tmp, ".pi", "harness", "events.db");
		const { DatabaseSync } = require("node:sqlite");
		const db = new DatabaseSync(dbFile);
		const disp = db.prepare("SELECT change FROM events WHERE kind='subagent.dispatch'").get();
		const comp = db.prepare("SELECT change FROM events WHERE kind='subagent.complete'").get();
		db.close();
		assert.equal(disp.change, "bind-change", "dispatch 行带 change");
		assert.equal(comp.change, "bind-change", "complete 回填与 dispatch 同源同值");
	} finally {
		fs.rmSync(tmp, { recursive: true, force: true });
	}
});

/* ---- add-dispatch-constraint-digest D5：约束段解析纯函数 + dispatch 记账 ---- */

ok("parseConstraintBrief：带约束段 → present + 域名数组", () => {
	const brief = telemetry.parseConstraintBrief(
		[
			"## 任务",
			"做点事",
			"## 约束（红线，源 docs/reference/flow/daily-report.md）",
			"- 日报生成必须幂等：同日重复触发不得产生重复记录",
			"- 生成失败必须落失败态并保留原因，禁止静默吞错",
		].join("\n"),
	);
	assert.equal(brief.digest, "present");
	assert.deepEqual(brief.domains, ["daily-report"]);
});

ok("parseConstraintBrief：无约束段 → absent + []", () => {
	const brief = telemetry.parseConstraintBrief("## 任务\n只做事，无约束段\n");
	assert.equal(brief.digest, "absent");
	assert.deepEqual(brief.domains, []);
});

ok("parseConstraintBrief：段在但无 flow/ 源路径 → present + []（不猜域）", () => {
	const brief = telemetry.parseConstraintBrief(
		"## 约束\n- 凭经验办，别问\n- 不指定源文档\n",
	);
	assert.equal(brief.digest, "present");
	assert.deepEqual(brief.domains, []);
});

ok("parseConstraintBrief：多域去重 + 顺序稳定 + 段边界（下一个 ## 之后不算）", () => {
	const text = [
		"## 约束（红线，源 docs/reference/flow/daily-report.md）",
		"- 日报生成必须幂等",
		"- 另见 docs/reference/flow/ai-summary.md",
		"- 重复提到 docs/reference/flow/daily-report.md",
		"## 任务",
		"- 下面这段不在约束段内：docs/reference/flow/topic-graph.md",
	].join("\n");
	const brief = telemetry.parseConstraintBrief(text);
	assert.equal(brief.digest, "present");
	assert.deepEqual(
		brief.domains,
		["ai-summary", "daily-report"],
		"去重 + 字典序稳定 + 不越过段边界",
	);
	// 顺序稳定：同输入重复解析结果一致
	assert.deepEqual(telemetry.parseConstraintBrief(text), brief);
});

ok("parseConstraintBrief：空/非字符串输入 → absent + []（不抛异常）", () => {
	assert.deepEqual(telemetry.parseConstraintBrief(""), { digest: "absent", domains: [] });
	assert.deepEqual(telemetry.parseConstraintBrief(undefined), { digest: "absent", domains: [] });
	assert.deepEqual(telemetry.parseConstraintBrief(null), { digest: "absent", domains: [] });
});

ok("parseConstraintBrief：brief 形态（constraint-digest.sh --brief 真实输出）→ present + 多域", () => {
	// 与 constraint-digest.sh --brief 的真实输出形态逐行对齐（title 源路径逗号分隔 +
	// 每域一个 ### 子标题 + 细节层 + 段末兜底行）
	const brief = telemetry.parseConstraintBrief(
		[
			"## 约束（红线，源 docs/reference/flow/daily-report.md, docs/reference/flow/scheduler.md）",
			"### daily-report（源 docs/reference/flow/daily-report.md）",
			"1. **日报生成必须幂等**",
			"细节层：read `docs/reference/flow/daily-report.md`「业务约束与不变量」",
			"### scheduler（源 docs/reference/flow/scheduler.md）",
			"1. **定时任务不得重入**",
			"细节层：read `docs/reference/flow/scheduler.md`「业务约束与不变量」",
			"> 目标路径与本清单冲突时以 `--paths <你的目标>` 输出为准",
		].join("\n"),
	);
	assert.equal(brief.digest, "present");
	assert.deepEqual(brief.domains, ["daily-report", "scheduler"]);
});

ok("parseConstraintBrief：源路径缺 docs/reference/ 前缀 → 不收域（present + []）", () => {
	// review R-3：只认 docs/reference/flow/ 前缀，路径里任意出现的 flow/<x>.md 不算业务域
	const brief = telemetry.parseConstraintBrief(
		"## 约束（红线，源 flow/daily-report.md）\n- 凭经验办\n",
	);
	assert.equal(brief.digest, "present");
	assert.deepEqual(brief.domains, [], "缺目录前缀的源路径不收域");
});

ok("dispatch 落 constraintDigest/constraintDomains（append-only 派生值）", () => {
	const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "tdisp-e-"));
	try {
		fs.mkdirSync(path.join(tmp, "openspec", "changes", "digest-change"), { recursive: true });
		fs.writeFileSync(path.join(tmp, "openspec", "changes", "digest-change", "proposal.md"), "<!-- ui-impact: none -->\n");
		const { handlers, ctx } = makeTelemetry(tmp);
		// 带约束段 → present + 域名
		const withBrief = [
			"## 任务",
			"实现日记",
			"## 约束（红线，源 docs/reference/flow/daily-report.md）",
			"- 日报生成必须幂等",
		].join("\n");
		dispatchAgent(handlers, ctx, 5, withBrief);
		// 无约束段 → absent + []
		const withoutBrief = "## 任务\n没有约束段\n";
		dispatchAgent(handlers, ctx, 6, withoutBrief);

		const rows = dispatchRows(tmp);
		assert.equal(rows.length, 2, `应两条 dispatch（实际 ${rows.length}）`);
		const p1 = JSON.parse(rows[0].payload);
		assert.equal(p1.constraintDigest, "present");
		assert.deepEqual(p1.constraintDomains, ["daily-report"]);
		const p2 = JSON.parse(rows[1].payload);
		assert.equal(p2.constraintDigest, "absent");
		assert.deepEqual(p2.constraintDomains, []);
		// 既有字段语义不变 + 不落任务正文
		assert.equal(p1.promptChars, withBrief.length, "promptChars 仍是原 prompt 长度");
		assert.equal(p1.isError, false);
		assert.ok(!("prompt" in p1), "payload 不落任务正文");
		assert.ok(!JSON.stringify(p1).includes("日报生成必须幂等"), "payload 不含 prompt 原文");
	} finally {
		fs.rmSync(tmp, { recursive: true, force: true });
	}
});

fs.rmSync(path.dirname(TMP_BUNDLE), { recursive: true, force: true });
console.log(`✓ telemetry-dispatch.smoke 全过（${pass} case）`);
