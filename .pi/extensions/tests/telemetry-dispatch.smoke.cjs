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

/* 现场 bundle（相对导入无扩展名，直接 require 会失败） */
const TMP_BUNDLE = path.join(fs.mkdtempSync(path.join(os.tmpdir(), "tdisp-")), "tel-bundle.cjs");
execSync(
	`npx -y esbuild ${JSON.stringify(path.resolve(__dirname, "../harness-telemetry.ts"))} --bundle --platform=node --format=cjs --outfile=${JSON.stringify(TMP_BUNDLE)}`,
	{ cwd: __dirname, stdio: ["ignore", "ignore", "ignore"] },
);
const telemetry = require(TMP_BUNDLE);

/** dispatch 一次 Agent 子线程（tool_call 暂存起点 + tool_result 落账），返回触发的 handlers */
const dispatchAgent = (handlers, ctx, id) => {
	handlers["tool_call"](
		{ toolName: "Agent", toolCallId: `tc-${id}`, input: { subagent_type: "explore", description: `派发 ${id}`, prompt: "x".repeat(10) } },
		ctx,
	);
	handlers["tool_result"](
		{
			toolName: "Agent",
			toolCallId: `tc-${id}`,
			isError: false,
			input: { subagent_type: "explore", description: `派发 ${id}`, prompt: "x".repeat(10) },
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

fs.rmSync(path.dirname(TMP_BUNDLE), { recursive: true, force: true });
console.log(`✓ telemetry-dispatch.smoke 全过（${pass} case）`);
