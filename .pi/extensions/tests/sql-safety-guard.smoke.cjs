// sql-safety-guard smoke（change: add-sql-safety-guard）：事故金样例 block 回归（2026-09-22
// psql -c 裸跑 TRUNCATE…CASCADE 事故原文 fixture，语义不可改）+ 双键粗筛放行反例 + 逃生注释/
// BEGIN+ROLLBACK/一次性容器放行 + psql -f 文件扫描（可读无词/含词/不可读保守拦/解析不出）+
// 三通道（bash / ctx_execute shell / ctx_batch_execute）× 三模式（hard 默认 block / soft 提醒 /
// off 零评估，非法值回退 hard）+ policy.decision 记账参数（block/warn/bypass；off/未命中/
// 事务与容器放行零记录）+ 会话语境不构成豁免 + 1MB 超长命令计时断言。
// 白盒分支对齐 test-cases.md B1–B10。自建 bundle（esbuild ../sql-safety-guard.ts → ./.ssql.cjs，
// 用完清理），可独立运行：node .pi/extensions/tests/sql-safety-guard.smoke.cjs → 退出码 0。
// 所有命令均为字符串 fixture，绝不真执行（纯 mock 驱动：无 DB、无 psql、无 docker）。断言失败 exit 1。
const fs = require('fs');
const os = require('os');
const path = require('path');
const { execSync } = require('child_process');

// node:sqlite ExperimentalWarning 抑制（与 spec-gate.smoke / policy-decision.smoke 同策略）
const priorWarn = process.listeners('warning');
process.removeAllListeners('warning');
process.on('warning', (w) => {
	if (!(w.name === 'ExperimentalWarning' && /sqlite/i.test(w.message))) {
		for (const l of priorWarn) l(w);
	}
});
const { DatabaseSync } = require('node:sqlite');

const HERE = __dirname;
const BUNDLE = path.join(HERE, '.ssql.cjs');
const checks = [];
const check = (name, ok) => checks.push([name, ok]);
const mktmp = () => fs.mkdtempSync(path.join(os.tmpdir(), 'ssql-smoke-'));
const policyRows = (cwd) => {
	const dbFile = path.join(cwd, '.pi', 'harness', 'events.db');
	if (!fs.existsSync(dbFile)) return [];
	const db = new DatabaseSync(dbFile);
	const rows = db.prepare("SELECT * FROM events WHERE kind='policy.decision' ORDER BY id").all();
	db.close();
	return rows.map((r) => ({ ...r, payload: JSON.parse(r.payload) }));
};

/** 自建 bundle（独立可跑；run-harness-smoke.sh 调用本文件时同样自建） */
function build() {
	execSync(
		`npx -y esbuild ${JSON.stringify(path.join(HERE, '..', 'sql-safety-guard.ts'))} --bundle --platform=node --format=cjs --outfile=${JSON.stringify(BUNDLE)}`,
		{ stdio: ['ignore', 'pipe', 'pipe'], cwd: HERE },
	);
}

/** 设 env → 清 require.cache → 重新 require（MODE 是模块加载时常量，逐模式重载） */
function loadMod(mode) {
	if (mode === undefined) delete process.env.SQL_SAFETY_GUARD;
	else process.env.SQL_SAFETY_GUARD = mode;
	delete require.cache[require.resolve(BUNDLE)];
	return require(BUNDLE);
}

/** mock pi（仿 spec-gate.smoke：makePi 收集 pi.on handler，直接调 handlers.tool_call） */
function makePi() {
	const handlers = {};
	return { pi: { on: (name, fn) => (handlers[name] = fn) }, handlers };
}

/** 驱动一次 tool_call；返回 { result, notices }——undefined=放行 / {block:true}=拦 */
async function run({ mode, toolName = 'bash', input, cwd, entries = [], ui } = {}) {
	const mod = loadMod(mode);
	const { pi, handlers } = makePi();
	mod.default(pi);
	const notices = [];
	const result = await handlers['tool_call'](
		{ toolName, input, toolCallId: 'ss1' },
		{
			ui: ui || { notify: (m) => notices.push(m) },
			sessionManager: {
				getSessionId: () => 'ssql1',
				getEntries: () => entries,
				buildContextEntries: () => entries,
			},
			cwd,
		},
	);
	return { result, notices, mod };
}

/** 放行类断言统一口径：undefined + 零 UI 提醒 + 零 policy.decision */
const checkPass = (name, r, cwd) =>
	check(name, r.result === undefined && r.notices.length === 0 && policyRows(cwd).length === 0);

// 事故金样例（2026-09-22 事故原文 fixture：psql 入口 + TRUNCATE TABLE categories，含 BEGIN 无 ROLLBACK，
// 无逃生注释——语义不可改，任务 6.4 回归锚点）
const GOLD =
	'docker exec syntopica-postgres psql -c "DO $$ BEGIN EXECUTE \'TRUNCATE TABLE categories CASCADE\'; END $$;"';

(async () => {
	const tmps = [];
	try {
		build();

		/* ---------- 1. 判定纯函数白盒（tasks 1.1「smoke 断言纯函数行为」） ---------- */
		const mod = loadMod(undefined);
		const vGold = await mod.classifySqlCommand(GOLD, '');
		check('classify 白盒：金样例 → hit(command)', vGold.kind === 'hit' && vGold.source === 'command');
		const vEsc = await mod.classifySqlCommand(GOLD + ' # allow-truncate-drop', '');
		check('classify 白盒：逃生注释 → bypass', vEsc.kind === 'bypass');
		const vGrep = await mod.classifySqlCommand('grep -rn TRUNCATE docs/', '');
		check('classify 白盒：无 psql 入口 → pass', vGrep.kind === 'pass');
		const vRo = await mod.classifySqlCommand('psql -c "select 1"', '');
		check('classify 白盒：只读 psql → pass', vRo.kind === 'pass');
		const tW = mktmp(); tmps.push(tW);
		const vUnread = await mod.classifySqlCommand('psql -f nope-xyz.sql', tW);
		check('classify 白盒：-f 不可读 → hit(file-unverified)', vUnread.kind === 'hit' && vUnread.source === 'file-unverified');

		/* ---------- 2. 事故金样例（Scenario：事故金样例被拦截；B8） ---------- */
		const t1 = mktmp(); tmps.push(t1);
		const g1 = await run({ cwd: t1, input: { command: GOLD } });
		check('金样例 → block（命令未执行由返回值语义保证）', !!(g1.result && g1.result.block === true));
		const reason = (g1.result && g1.result.reason) || '';
		check('金样例 reason 含 [sql-safety-guard] 前缀', reason.startsWith('[sql-safety-guard]'));
		check(
			'金样例 reason 含三条出路（BEGIN/…/ROLLBACK 事务、# allow-truncate-drop、SQL_SAFETY_GUARD=off/soft）',
			reason.includes('BEGIN') && reason.includes('ROLLBACK') &&
			reason.includes('# allow-truncate-drop') && reason.includes('SQL_SAFETY_GUARD=off') && reason.includes('soft'),
		);
		check('金样例 reason 注明命令原文来源', reason.includes('命令原文'));
		const rows1 = policyRows(t1);
		check(
			'金样例记账 policy.decision(block, sql-safety) 恰一条',
			rows1.length === 1 && rows1[0].payload.policy === 'sql-safety-guard' &&
			rows1[0].payload.action === 'block' && rows1[0].payload.reasonCode === 'sql-safety',
		);

		/* ---------- 3. block 变体（Scenario：DROP 命中 / 大小写 / heredoc / 嵌套 / --command / 特殊字符；B8） ---------- */
		const blockCases = [
			['DROP 命中（Scenario）', 'psql -c "DROP TABLE tmp_x"'],
			['小写 truncate', 'psql -c "truncate table tmp_x"'],
			['首字母大写 Truncate', 'psql -c "Truncate table tmp_x"'],
			['小写 drop', 'psql -c "drop table tmp_x"'],
			['heredoc 正文含两词', "psql <<'EOF'\nTRUNCATE TABLE a;\nDROP TABLE b;\nEOF"],
			['嵌套 bash -c 原文命中（Scenario）', 'bash -c \'docker exec syntopica-postgres psql -c "DROP TABLE t"\''],
			['--command 长形态（test-cases 边界值）', 'psql --command="DROP TABLE t"'],
			['词边界 + 分号 + 中文注释包围', 'psql -c "TRUNCATE;" # 清理说明'],
			['词边界 + 换行（DROP INDEX\\n）', 'psql -c "DROP INDEX\nidx_name"'],
			['只有 BEGIN 无 ROLLBACK 不放行（Scenario）', 'psql -c "DO $$ BEGIN EXECUTE \'TRUNCATE TABLE tmp_x\'; END $$;"'],
		];
		for (const [name, cmd] of blockCases) {
			const cwd = mktmp(); tmps.push(cwd);
			const { result } = await run({ cwd, input: { command: cmd } });
			check(`block：${name}`, !!(result && result.block === true));
		}

		/* ---------- 4. 通道覆盖（Scenario：ctx_batch_execute 逐条生效；ctx_execute shell；B9 通道矩阵） ---------- */
		{
			const cwd = mktmp(); tmps.push(cwd);
			const { result } = await run({
				cwd, toolName: 'ctx_execute',
				input: { language: 'shell', code: 'docker exec syntopica-postgres psql -c "DROP TABLE t"' },
			});
			check('block：ctx_execute(language=shell) 通道', !!(result && result.block === true));
		}
		{
			const cwd = mktmp(); tmps.push(cwd);
			const { result } = await run({
				cwd, toolName: 'ctx_batch_execute',
				input: {
					commands: [
						{ label: 'a', command: 'psql -c "select 1"' },
						{ label: 'b', command: GOLD },
					],
				},
			});
			check('block：ctx_batch_execute 某条命中 → 整次调用 block（Scenario）', !!(result && result.block === true));
		}
		{
			const cwd = mktmp(); tmps.push(cwd);
			const { result, notices } = await run({
				cwd, toolName: 'ctx_execute',
				input: { language: 'javascript', code: 'execSync(\'psql -c "TRUNCATE TABLE t"\')' },
			});
			check('ctx_execute(language=javascript) 不评估：放行零提醒零记账',
				result === undefined && notices.length === 0 && policyRows(cwd).length === 0);
		}

		/* ---------- 5. psql -f 文件扫描（Scenario：只读放行 / 含 DROP 拦 / 不可读保守拦；B5–B7） ---------- */
		{
			const cwd = mktmp(); tmps.push(cwd);
			fs.writeFileSync(path.join(cwd, 'migration-drop.sql'), 'SELECT 1;\nDROP INDEX idx_x;\n');
			const { result } = await run({ cwd, input: { command: 'psql -f migration-drop.sql' } });
			check('block：-f 文件含 DROP INDEX → block（Scenario）', !!(result && result.block === true));
			check('block：-f 文件来源 reason 注明「文件来源」',
				!!(result && result.reason.includes('文件来源') && result.reason.includes('[sql-safety-guard]')));
			const rows = policyRows(cwd);
			check('block：-f 文件来源记账 block 恰一条',
				rows.length === 1 && rows[0].payload.action === 'block' && rows[0].payload.reasonCode === 'sql-safety');
		}
		{
			const cwd = mktmp(); tmps.push(cwd);
			fs.writeFileSync(path.join(cwd, 'wipe.sql'), 'TRUNCATE TABLE t;');
			const { result } = await run({ cwd, input: { command: 'psql --file=wipe.sql' } });
			check('block：--file= 连写形态 + 文件含 TRUNCATE → block（test-cases 边界值）',
				!!(result && result.block === true));
		}
		{
			const cwd = mktmp(); tmps.push(cwd);
			const { result } = await run({ cwd, input: { command: 'psql -f /not/exist.sql' } });
			check('block：-f 文件不可读 → 保守 block（Scenario，reason 注明「文件无法验证」）',
				!!(result && result.block === true && result.reason.includes('文件无法验证')));
		}
		{
			const cwd = mktmp(); tmps.push(cwd);
			const { result } = await run({ cwd, input: { command: 'psql --file' } });
			check('block：--file 参数解析不出 → 保守 block（spec：无法证明无害即拦）',
				!!(result && result.block === true && result.reason.includes('文件无法验证')));
		}
		{
			const cwd = mktmp(); tmps.push(cwd);
			fs.mkdirSync(path.join(cwd, 'scripts/db'), { recursive: true });
			fs.writeFileSync(path.join(cwd, 'scripts/db/readonly-report.sql'), 'SELECT count(*) FROM articles;\n');
			checkPass('只读 -f 文件（分写形态）放行零记账（Scenario）',
				await run({ cwd, input: { command: 'psql -f scripts/db/readonly-report.sql' } }), cwd);
			checkPass('只读 --file= 连写形态放行零记账（test-cases 边界值）',
				await run({ cwd, input: { command: 'psql --file=scripts/db/readonly-report.sql' } }), cwd);
		}

		/* ---------- 6. 放行反例（Scenario：只有动词无 psql / 只有 psql 无动词 / DELETE / BEGIN+ROLLBACK；B1/B3） ---------- */
		const passCases = [
			['只有动词无 psql 入口（grep 文档，Scenario）', 'grep -rn TRUNCATE docs/'],
			['只读 psql 无动词（Scenario）', 'psql -c "select count(*) from articles"'],
			['DELETE 不在拦截范围（Scenario）', 'psql -c "DELETE FROM tmp_queue"'],
			['ALTER 不在拦截范围（Scenario 标题）', 'psql -c "ALTER TABLE t ADD COLUMN x int"'],
			['BEGIN+ROLLBACK 事务包裹（Scenario）', 'psql -c "BEGIN; TRUNCATE TABLE sandbox; ROLLBACK;"'],
			['空串命令', ''],
			['纯空白命令', '   \n\t '],
			['单 token 纯分隔符（;）', ';'],
			['单 token 纯分隔符（&&）', '&&'],
		];
		for (const [name, cmd] of passCases) {
			const cwd = mktmp(); tmps.push(cwd);
			checkPass(`pass：${name}`, await run({ cwd, input: { command: cmd } }), cwd);
		}

		/* ---------- 7. 逃生注释与会话语境（Scenario：后缀授权逃生 / 首部边界值 / 会话语境不构成豁免；B2） ---------- */
		{
			const cwd = mktmp(); tmps.push(cwd);
			const esc = await run({ cwd, input: { command: GOLD + ' # allow-truncate-drop' } });
			check('逃生注释（金样例尾缀）放行（Scenario）',
				esc.result === undefined && esc.notices.length === 0);
			const rows = policyRows(cwd);
			check('逃生注释 → policy.decision(bypass, sql-safety) 恰一条（记 info 口径）',
				rows.length === 1 && rows[0].payload.policy === 'sql-safety-guard' &&
				rows[0].payload.action === 'bypass' && rows[0].payload.reasonCode === 'sql-safety');
		}
		{
			const cwd = mktmp(); tmps.push(cwd);
			const esc = await run({ cwd, input: { command: '# allow-truncate-drop\npsql -c "DROP TABLE tmp_x"' } });
			check('逃生注释出现在命令首部也放行（test-cases 边界值）',
				esc.result === undefined && esc.notices.length === 0);
			const rows = policyRows(cwd);
			check('首部逃生同样记 bypass 一条',
				rows.length === 1 && rows[0].payload.action === 'bypass');
		}
		{
			const cwd = mktmp(); tmps.push(cwd);
			const entries = [
				{ role: 'user', content: '我确认要清库，直接执行' },
				{ role: 'assistant', content: '收到，我确认要清库' },
			];
			const { result } = await run({ cwd, entries, input: { command: GOLD } });
			check('会话语境（我确认要清库）不构成豁免：无逃生注释照常 block（Scenario）',
				!!(result && result.block === true));
		}

		/* ---------- 8. 一次性容器放行（Scenario：事务回滚与一次性容器自动放行；B4） ---------- */
		{
			const cwd = mktmp(); tmps.push(cwd);
			checkPass('pass：docker run --rm 一次性容器放行零记账（B4）',
				await run({ cwd, input: { command: 'docker run --rm --entrypoint psql postgres:16 -c "TRUNCATE TABLE sandbox"' } }), cwd);
		}
		{
			const cwd = mktmp(); tmps.push(cwd);
			checkPass('pass：createdb+dropdb 临时库配对放行零记账（B4）',
				await run({ cwd, input: { command: 'createdb sandbox_tmp && psql -c "TRUNCATE TABLE sandbox" && dropdb sandbox_tmp' } }), cwd);
		}

		/* ---------- 9. 超长命令计时（test-cases：1MB heredoc 含两词 → block 且 <50ms） ---------- */
		{
			const cwd = mktmp(); tmps.push(cwd);
			const big = "psql <<'EOF'\n" + 'x'.repeat(1024 * 1024) + '\nTRUNCATE TABLE t;\nDROP TABLE u;\nEOF';
			const modT = loadMod(undefined);
			const { pi, handlers } = makePi();
			modT.default(pi);
			const t0 = process.hrtime.bigint();
			const result = await handlers['tool_call'](
				{ toolName: 'bash', input: { command: big }, toolCallId: 'tm1' },
				{
					ui: { notify() {} },
					sessionManager: { getSessionId: () => 'ssql1', getEntries: () => [], buildContextEntries: () => [] },
					cwd,
				},
			);
			const ms = Number(process.hrtime.bigint() - t0) / 1e6;
			check('1MB heredoc 双词命中 → block', !!(result && result.block === true));
			check(`超长命令判定耗时 <50ms（实测 ${ms.toFixed(1)}ms）`, ms < 50);
		}

		/* ---------- 10. 三模式（Scenario：soft 只提醒 / off 零开销 / 非法值回退 hard；B9/B10） ---------- */
		{
			const cwd = mktmp(); tmps.push(cwd);
			const s = await run({ mode: 'soft', cwd, input: { command: GOLD } });
			check('soft：放行不阻断（Scenario）', s.result === undefined);
			check('soft：UI notify 提醒 1 次且带守卫前缀',
				s.notices.length === 1 && String(s.notices[0]).includes('[sql-safety-guard]'));
			const rows = policyRows(cwd);
			check('soft：policy.decision(warn, sql-safety) 恰一条',
				rows.length === 1 && rows[0].payload.policy === 'sql-safety-guard' &&
				rows[0].payload.action === 'warn' && rows[0].payload.reasonCode === 'sql-safety');
		}
		{
			const cwd = mktmp(); tmps.push(cwd);
			const s = await run({ mode: 'soft', cwd, input: { command: GOLD }, ui: { notify() { throw new Error('ui down'); } } });
			check('soft + UI 不可用：try/catch 静默降级仍放行仍记 warn',
				s.result === undefined && policyRows(cwd).length === 1 &&
				policyRows(cwd)[0].payload.action === 'warn');
		}
		{
			const cwd = mktmp(); tmps.push(cwd);
			const o = await run({ mode: 'off', cwd, input: { command: GOLD } });
			check('off：放行 + 零提醒 + 零 policy.decision（Scenario）',
				o.result === undefined && o.notices.length === 0 && policyRows(cwd).length === 0);
			const o2 = await run({ mode: 'off', cwd, input: { command: 'psql -f /not/exist.sql' } });
			check('off：粗筛前直接返回（-f 不可读也不评估不记账，B10）',
				o2.result === undefined && policyRows(cwd).length === 0);
		}
		{
			const cwd = mktmp(); tmps.push(cwd);
			const i = await run({ mode: 'yes', cwd, input: { command: GOLD } });
			check('非法 SQL_SAFETY_GUARD=yes → 按 hard：block（Scenario）', !!(i.result && i.result.block === true));
			const rows = policyRows(cwd);
			check('非法值 → hard 记账 block 恰一条',
				rows.length === 1 && rows[0].payload.action === 'block' && rows[0].payload.reasonCode === 'sql-safety');
		}

		let fail = 0;
		for (const [name, ok] of checks) {
			console.log(`${ok ? '✅' : '❌'} ${name}`);
			if (!ok) fail++;
		}
		if (fail) {
			console.log(`\n${fail}/${checks.length} 项失败`);
			process.exitCode = 1;
		} else {
			console.log(`\nSMOKE OK：${checks.length} 项断言全部通过`);
		}
	} catch (e) {
		console.error('FAIL', e);
		process.exitCode = 1;
	} finally {
		try { fs.rmSync(BUNDLE, { force: true }); } catch { /* 清理失败忽略 */ }
		for (const t of tmps) fs.rmSync(t, { recursive: true, force: true });
	}
})();
