#!/usr/bin/env node
/* telemetry-archive.smoke — change.archive 归档成功记账白盒（task-cost-metrics 2.3/2.4）
 * 纯函数：isArchiveCommand / extractArchiveChangeName / decideArchiveEvent 表驱动
 *   （成功形态、带 flag、交互式无名字、非法字符、isError 零记录——spec 四 Scenario 对应）。
 * 集成：mock pi 挂 tool_call/tool_result 钩子 → 临时 cwd 落真库（events.db）→ 断言行数与内容；
 *   harness-telemetry.ts 直接 require（相对导入无扩展名）会失败 → 现场 esbuild bundle 再加载
 *   （与 run-harness-smoke.sh 的 .tel.cjs 同产物，此处独立生成保证单跑自足）。
 * 跑法：node .pi/extensions/tests/telemetry-archive.smoke.cjs */
"use strict";
const assert = require("node:assert");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { execSync } = require("node:child_process");

/* ---- 纯函数直载（lib/archive-accounting.ts 无相对依赖，node type stripping 直跑） ---- */
const {
	isArchiveCommand,
	extractArchiveChangeName,
	decideArchiveEvent,
} = require("../lib/archive-accounting.ts");

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

/* ---- 纯函数表驱动 ---- */
ok("isArchiveCommand：命中/不命中/多空白形态", () => {
	assert.equal(isArchiveCommand("openspec archive foo"), true);
	assert.equal(isArchiveCommand("openspec  archive foo"), true); // 多空白
	assert.equal(isArchiveCommand("cd x && openspec archive foo"), true); // 命令链中
	assert.equal(isArchiveCommand("openspec list --json"), false);
	assert.equal(isArchiveCommand("openspec archive"), true); // 无名字也命中（交互式候选）
	assert.equal(isArchiveCommand(""), false);
});
ok("extractArchiveChangeName：成功形态（裸名 / 带 flag / 命令链）", () => {
	assert.equal(extractArchiveChangeName("openspec archive task-cost-metrics"), "task-cost-metrics");
	assert.equal(extractArchiveChangeName("openspec archive --yes task-cost-metrics"), "task-cost-metrics"); // 带 flag：首个非 flag 词
	assert.equal(extractArchiveChangeName("openspec archive -y --force my.change_v2"), "my.change_v2"); // 合法字符集（._-）
	assert.equal(extractArchiveChangeName("bash -c 'openspec archive foo'"), ""); // 引号尾随使名字非法 → 零记录（fail-open，同 spec-gate 语义）；无引号裸名才记账
});
ok("extractArchiveChangeName：交互式无名字 → 空串", () => {
	assert.equal(extractArchiveChangeName("openspec archive"), "");
	assert.equal(extractArchiveChangeName("openspec archive --yes"), ""); // 只有 flag
	assert.equal(extractArchiveChangeName("openspec archive --interactive"), "");
});
ok("extractArchiveChangeName：非法字符 → 空串（防路径逃逸）", () => {
	assert.equal(extractArchiveChangeName("openspec archive ../evil"), "");
	assert.equal(extractArchiveChangeName("openspec archive /etc/passwd"), "");
	assert.equal(extractArchiveChangeName("openspec archive foo;rm -rf"), "");
	assert.equal(extractArchiveChangeName("openspec archive 'quoted'"), "");
	assert.equal(extractArchiveChangeName("openspec list"), ""); // 非归档命令
});
ok("decideArchiveEvent：is error / 未配对 / 非归档命令 → null 零记录", () => {
	assert.equal(decideArchiveEvent("openspec archive foo", false), "foo");
	assert.equal(decideArchiveEvent("openspec archive foo", true), null); // isError：被 block/CLI 失败
	assert.equal(decideArchiveEvent(undefined, false), null); // tool_call 未暂存（未配对）
	assert.equal(decideArchiveEvent("go test ./...", false), null); // 非归档命令
	assert.equal(decideArchiveEvent("openspec archive", false), null); // 提取不到合法名
});

/* ---- 集成：mock pi → tool_call/tool_result → events.db 断言 ---- */
ok("集成：成功归档记账一条（kind=change.archive，change 列与 payload.name 同名）", () => {
	const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "tarch-"));
	try {
		// 现场 bundle（相对导入无扩展名，直接 require 会失败）
		const bundle = path.join(tmp, "tel-bundle.cjs");
		execSync(
			`npx -y esbuild ${JSON.stringify(path.resolve(__dirname, "../harness-telemetry.ts"))} --bundle --platform=node --format=cjs --outfile=${JSON.stringify(bundle)}`,
			{ cwd: __dirname, stdio: ["ignore", "ignore", "ignore"] },
		);
		const telemetry = require(bundle);
		const handlers = {};
		const pi = { on: (name, fn) => (handlers[name] = fn) };
		const ctx = () => ({
			cwd: tmp,
			hasUI: false,
			sessionManager: { getSessionId: () => "s-arch-test" },
		});
		telemetry.default(pi);
		assert.equal(typeof handlers["tool_call"], "function");
		assert.equal(typeof handlers["tool_result"], "function");

		// 成功归档：暂存 → 配对成功 → 记账一条
		handlers["tool_call"](
			{ toolName: "bash", toolCallId: "t1", input: { command: "openspec archive demo-change" } },
			ctx(),
		);
		handlers["tool_result"]({ toolName: "bash", toolCallId: "t1", isError: false }, ctx());
		// isError（spec-gate block / CLI 失败）：零记录
		handlers["tool_call"](
			{ toolName: "bash", toolCallId: "t2", input: { command: "openspec archive demo-change" } },
			ctx(),
		);
		handlers["tool_result"]({ toolName: "bash", toolCallId: "t2", isError: true }, ctx());
		// 交互式无名字：零记录
		handlers["tool_call"](
			{ toolName: "bash", toolCallId: "t3", input: { command: "openspec archive" } },
			ctx(),
		);
		handlers["tool_result"]({ toolName: "bash", toolCallId: "t3", isError: false }, ctx());
		// 未暂存的 bash 结果（直接 tool_result，无 tool_call 配对）：零记录
		handlers["tool_result"]({ toolName: "bash", toolCallId: "t9", isError: false }, ctx());
		// 非归档命令暂存不发生
		handlers["tool_call"](
			{ toolName: "bash", toolCallId: "t4", input: { command: "go test ./..." } },
			ctx(),
		);
		handlers["tool_result"]({ toolName: "bash", toolCallId: "t4", isError: false }, ctx());

		const { DatabaseSync } = require("node:sqlite");
		const db = new DatabaseSync(path.join(tmp, ".pi", "harness", "events.db"));
		const rows = db
			.prepare("SELECT kind, change, payload FROM events WHERE kind = 'change.archive'")
			.all();
		db.close();
		assert.equal(rows.length, 1, `期望恰好 1 条 change.archive，实际 ${rows.length}`);
		assert.equal(rows[0].change, "demo-change");
		assert.equal(JSON.parse(rows[0].payload).name, "demo-change");
	} finally {
		fs.rmSync(tmp, { recursive: true, force: true });
	}
});
ok("集成：同一 change 重跑两次成功归档 → 两条（append-only 不去重，消费侧按名聚合）", () => {
	const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "tarch2-"));
	try {
		const bundle = path.join(tmp, "tel-bundle.cjs");
		execSync(
			`npx -y esbuild ${JSON.stringify(path.resolve(__dirname, "../harness-telemetry.ts"))} --bundle --platform=node --format=cjs --outfile=${JSON.stringify(bundle)}`,
			{ cwd: __dirname, stdio: ["ignore", "ignore", "ignore"] },
		);
		const telemetry = require(bundle);
		const handlers = {};
		const pi = { on: (name, fn) => (handlers[name] = fn) };
		const ctx = () => ({
			cwd: tmp,
			hasUI: false,
			sessionManager: { getSessionId: () => "s-arch-test2" },
		});
		telemetry.default(pi);
		for (const id of ["t1", "t2"]) {
			handlers["tool_call"](
				{ toolName: "bash", toolCallId: id, input: { command: "openspec archive retry-change" } },
				ctx(),
			);
			handlers["tool_result"]({ toolName: "bash", toolCallId: id, isError: false }, ctx());
		}
		const { DatabaseSync } = require("node:sqlite");
		const db = new DatabaseSync(path.join(tmp, ".pi", "harness", "events.db"));
		const n = db
			.prepare("SELECT COUNT(*) AS n FROM events WHERE kind = 'change.archive'")
			.get().n;
		db.close();
		assert.equal(n, 2, `重跑应记 2 条，实际 ${n}`);
	} finally {
		fs.rmSync(tmp, { recursive: true, force: true });
	}
});

console.log(`telemetry-archive smoke: ${pass} 组断言全绿`);
