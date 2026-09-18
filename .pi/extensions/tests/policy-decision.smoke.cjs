// policy-decision smoke（harden-harness-policy-and-spill）：helper 有界收敛（normalizeDecision
// 白盒）+ logPolicyDecision 集成（change 归属三分支/写入失败旁路）+ quota-gate 行为（block/
// fail-open/fuzzy-warn/健康放行零记录/target 安全）+ test-scope-guard 行为（soft warn/hard
// block/off 与未命中/归档语境零记录）+ quality-gate/entry-gate 与 policy.decision 隔离复核。
// 由 run-harness-smoke.sh 调用（先 esbuild 产出 .pdec.cjs / .qgate.cjs / .tsg.cjs）。断言失败 exit 1。
const fs = require('fs');
const os = require('os');
const path = require('path');
const { execSync } = require('child_process');

// node:sqlite ExperimentalWarning 抑制（与 lib/harness-log 同策略，保输出干净）
const priorWarn = process.listeners('warning');
process.removeAllListeners('warning');
process.on('warning', (w) => {
	if (!(w.name === 'ExperimentalWarning' && /sqlite/i.test(w.message))) {
		for (const l of priorWarn) l(w);
	}
});
const { DatabaseSync } = require('node:sqlite');

const checks = [];
const check = (name, ok) => checks.push([name, ok]);
const mktmp = () => fs.mkdtempSync(path.join(os.tmpdir(), 'pdec-smoke-'));
const dbOf = (cwd) => path.join(cwd, '.pi', 'harness', 'events.db');
const policyRows = (cwd) => {
	if (!fs.existsSync(dbOf(cwd))) return [];
	const db = new DatabaseSync(dbOf(cwd));
	const rows = db.prepare("SELECT * FROM events WHERE kind='policy.decision' ORDER BY id").all();
	db.close();
	return rows.map((r) => ({ ...r, payload: JSON.parse(r.payload) }));
};
/** 重载 bundle（quota-gate 有模块级 cache/inflight，每场景需干净实例） */
const loadFresh = (p) => {
	delete require.cache[require.resolve(p)];
	return require(p);
};
function makePi() {
	const handlers = {};
	return { pi: { on: (name, fn) => (handlers[name] = fn) }, handlers };
}

(async () => {
	const tmps = [];
	try {
		/* ---------- 1. normalizeDecision 白盒：action 白名单 / reasonCode 有界 / target / durationMs ---------- */
		const { normalizeDecision, logPolicyDecision, UNKNOWN_REASON_CODE } = loadFresh('./.pdec.cjs');
		for (const a of ['block', 'warn', 'bypass', 'fail-open']) {
			const r = normalizeDecision({ policy: 'spec-gate', action: a, reasonCode: 'full-go-test' });
			check(`四种合法 action 入白名单（${a}）`, r.ok === true && r.payload.action === a);
		}
		check('action 白名单外（"allow"）拒绝 → ok:false', normalizeDecision({ policy: 'x', action: 'allow', reasonCode: 'r' }).ok === false);
		check('action 空串拒绝', normalizeDecision({ policy: 'x', action: '', reasonCode: 'r' }).ok === false);
		check('policy 非字符串拒绝', normalizeDecision({ policy: 42, action: 'warn', reasonCode: 'r' }).ok === false);
		// reasonCode：合法 kebab-case 保留；空串/大写/空格/特殊字符/超长一律归一 unknown（不写自由文本）
		const kebab = normalizeDecision({ policy: 'quota-gate', action: 'block', reasonCode: 'quota-exhausted' });
		check('合法 kebab-case reasonCode 原样保留', kebab.ok && kebab.payload.reasonCode === 'quota-exhausted');
		for (const bad of ['', 'BadCode', 'has space', 'a_b', '中文代码', 'x'.repeat(65)]) {
			const r = normalizeDecision({ policy: 'x', action: 'warn', reasonCode: bad });
			const label = bad === '' ? '空串' : bad === 'x'.repeat(65) ? '超长65' : bad;
			check(`非法 reasonCode 归一 unknown（${label}）`, r.ok && r.payload.reasonCode === UNKNOWN_REASON_CODE);
		}
		// target：null/省略 → 无字段；超长截断有界
		const noTarget = normalizeDecision({ policy: 'x', action: 'warn', reasonCode: 'r', target: null });
		check('target=null → payload 省略 target 字段', noTarget.ok && !('target' in noTarget.payload));
		const longTarget = normalizeDecision({ policy: 'x', action: 'warn', reasonCode: 'r', target: 'z'.repeat(500) });
		check('target 超长截断至 120+省略号', longTarget.ok && longTarget.payload.target.length === 121 && String(longTarget.payload.target).endsWith('…'));
		// durationMs：负数/NaN 省略；合法取整
		const noDur = normalizeDecision({ policy: 'x', action: 'warn', reasonCode: 'r', durationMs: -1 });
		check('durationMs 负数省略', noDur.ok && !('durationMs' in noDur.payload));
		const dur = normalizeDecision({ policy: 'x', action: 'warn', reasonCode: 'r', durationMs: 12.6 });
		check('durationMs 合法值取整保留', dur.ok && dur.payload.durationMs === 13);

		/* ---------- 2. logPolicyDecision 集成：change 归属三分支 + 写入失败旁路 ---------- */
		const t2 = mktmp(); tmps.push(t2);
		fs.mkdirSync(path.join(t2, 'openspec/changes/fake-active-change'), { recursive: true }); // 自动检测锚点
		logPolicyDecision(t2, { sessionId: 's1', policy: 'spec-gate', action: 'block', reasonCode: 'archive-check-failed', change: 'explicit-change' });
		logPolicyDecision(t2, { sessionId: 's1', policy: 'quota-gate', action: 'warn', reasonCode: 'fuzzy-model-resolve' }); // undefined → 自动检测
		logPolicyDecision(t2, { sessionId: 's1', policy: 'test-scope-guard', action: 'block', reasonCode: 'full-go-test', change: null }); // null → 明确不绑定
		const rows2 = policyRows(t2);
		check('显式 change 优先绑定', rows2.length >= 1 && rows2[0].change === 'explicit-change');
		check('change=undefined 自动检测活跃 change', rows2.length >= 2 && rows2[1].change === 'fake-active-change');
		check('change=null 明确不绑定', rows2.length === 3 && rows2[2].change === null);
		// 写入失败旁路（D3）：.pi/harness 被文件占位 → openDb 失败 → 返回 false 不抛异常
		const t2b = mktmp(); tmps.push(t2b);
		fs.mkdirSync(path.join(t2b, '.pi'), { recursive: true });
		fs.writeFileSync(path.join(t2b, '.pi/harness'), '占位文件');
		let sideEffectOnly = true;
		try {
			sideEffectOnly = logPolicyDecision(t2b, { sessionId: 's1', policy: 'x', action: 'block', reasonCode: 'r' }) === false;
		} catch {
			sideEffectOnly = false;
		}
		check('记账故障只返回 false（不抛异常、不改调用方控制流）', sideEffectOnly);

		/* ---------- 3. quota-gate 行为：block / fail-open / fuzzy-warn / 健康放行零记录 ---------- */
		// mock fetch：GLM 适配器响应形态；scenario 控制剩余百分比或抛错
		const quotaScenario = async (cwd, fetchImpl, input) => {
			const realFetch = globalThis.fetch;
			globalThis.fetch = fetchImpl;
			try {
				const mod = loadFresh('./.qgate.cjs');
				const { pi, handlers } = makePi();
				mod.default(pi);
				return await handlers['tool_call'](
					{ toolName: 'Agent', input, toolCallId: 'qc1' },
					{
						model: { provider: 'zai-coding-cn' },
						modelRegistry: {
							getAvailable: () => [
								{ id: 'glm-x', provider: 'opencode-go' },
								{ id: 'glm-x', provider: 'zai-coding-cn' },
							],
							getProviderAuth: async () => ({ auth: { apiKey: 'sk-test-secret' } }),
						},
						ui: { notify() {} },
						sessionManager: { getSessionId: () => 'pq1' },
						cwd,
					},
				);
			} finally {
				globalThis.fetch = realFetch;
			}
		};
		const glmResp = (pct) => async () => ({
			ok: true,
			text: async () => JSON.stringify({ data: { limits: [{ type: 'TOKENS_LIMIT', percentage: pct, nextResetTime: Date.now() + 3600e3 }] } }),
		});

		// 3a. 健康放行（剩 50%）：undefined + 零 policy.decision（低噪声）
		const t3a = mktmp(); tmps.push(t3a);
		const r3a = await quotaScenario(t3a, glmResp(50), { subagent_type: 'Explore' });
		check('quota 健康 → 放行 undefined', r3a === undefined);
		check('quota 健康放行零 policy.decision（低噪声）', policyRows(t3a).length === 0);

		// 3b. low 阻断（剩 5%）：block + block 事件（target 仅 provider，无 key/响应正文）
		const t3b = mktmp(); tmps.push(t3b);
		const r3b = await quotaScenario(t3b, glmResp(95), { subagent_type: 'Explore' });
		const rows3b = policyRows(t3b);
		check('quota low → block true + reason 含额度', r3b && r3b.block === true && /额度不足/.test(r3b.reason));
		check('quota low → policy.decision(block, quota-low) 恰一条', rows3b.length === 1 && rows3b[0].payload.action === 'block' && rows3b[0].payload.reasonCode === 'quota-low');
		check('quota target 仅含 provider 摘要', rows3b[0].payload.target === 'zai-coding-cn');
		const raw3b = JSON.stringify(rows3b[0].payload);
		check('quota payload 不含 API key 与响应正文', !raw3b.includes('sk-test-secret') && !raw3b.includes('TOKENS_LIMIT'));

		// 3c. 查询异常 fail-open：放行 + fail-open 事件
		const t3c = mktmp(); tmps.push(t3c);
		const r3c = await quotaScenario(t3c, async () => { throw new Error('network down'); }, { subagent_type: 'Explore' });
		const rows3c = policyRows(t3c);
		check('quota 查询失败 → fail-open 放行 undefined', r3c === undefined);
		check('quota 查询失败 → policy.decision(fail-open, quota-query-failed)', rows3c.length === 1 && rows3c[0].payload.action === 'fail-open' && rows3c[0].payload.reasonCode === 'quota-query-failed');

		// 3d. fuzzy 裸模型名 warn：解析到非默认 provider → warn 事件（此时无适配器/额度健康）
		const t3d = mktmp(); tmps.push(t3d);
		const r3d = await quotaScenario(t3d, glmResp(50), { model: 'glm-x', subagent_type: 'Explore' });
		const rows3d = policyRows(t3d);
		check('fuzzy 解析 + 无适配器 → 仍放行', r3d === undefined);
		check('fuzzy 裸模型名 → policy.decision(warn, fuzzy-model-resolve)', rows3d.length === 1 && rows3d[0].payload.action === 'warn' && rows3d[0].payload.reasonCode === 'fuzzy-model-resolve' && rows3d[0].payload.target === 'opencode-go');

		/* ---------- 4. test-scope-guard 行为：soft warn / 未命中与归档语境零记录（主进程默认 soft） ---------- */
		const tsgScenario = (cwd, command) => {
			const mod = loadFresh('./.tsg.cjs');
			const { pi, handlers } = makePi();
			const notices = [];
			mod.default(pi);
			const result = handlers['tool_call'](
				{ toolName: 'bash', input: { command }, toolCallId: 'tc1' },
				{
					ui: { notify: (m) => notices.push(m) },
					sessionManager: { getSessionId: () => 'ts1', getEntries: () => [], buildContextEntries: () => [] },
					cwd,
				},
			);
			return { result, notices };
		};
		const t4 = mktmp(); tmps.push(t4);
		const s4 = tsgScenario(t4, 'go test ./...');
		check('test-scope soft → 不阻断（undefined）+ UI 软提醒', (await s4.result) === undefined && s4.notices.length === 1);
		check(
			'test-scope soft → policy.decision(warn, full-go-test)',
			policyRows(t4).length === 1 && policyRows(t4)[0].payload.action === 'warn' && policyRows(t4)[0].payload.reasonCode === 'full-go-test' && policyRows(t4)[0].payload.policy === 'test-scope-guard',
		);
		const t4b = mktmp(); tmps.push(t4b);
		const s4b = await tsgScenario(t4b, 'go test ./internal/... # archive-gate');
		check('归档语境（显式注释）→ 零 policy.decision', policyRows(t4b).length === 0);
		check(
			'归档语境放行 → 附记账指引提示（test-debt-patrol）',
			s4b.notices.length === 1 && String(s4b.notices[0]).includes('scripts/harness/test-patrol.sh --register'),
		);
		const t4c = mktmp(); tmps.push(t4c);
		await tsgScenario(t4c, 'go test ./internal/domain/x');
		check('非全量测试命令未命中 → 零 policy.decision', policyRows(t4c).length === 0);

		// hard / off 是模块加载时常量（TEST_SCOPE_GUARD），用子进程逐模式验证
		const runTsgChild = (mode, command) => {
			const cwd = mktmp(); tmps.push(cwd);
			const script = [
				`process.env.TEST_SCOPE_GUARD=${JSON.stringify(mode)};`,
				`const mod=require(${JSON.stringify(path.resolve('.tsg.cjs'))});`,
				`const handlers={};mod.default({on:(n,f)=>handlers[n]=f});`,
				`handlers['tool_call']({toolName:'bash',input:{command:${JSON.stringify(command)}},toolCallId:'t'},`,
				`  {ui:{notify(){}},sessionManager:{getSessionId:()=>'ts2',getEntries:()=>[],buildContextEntries:()=>[]},cwd:${JSON.stringify(cwd)}})`,
				`.then((r)=>console.log(JSON.stringify({blocked:!!(r&&r.block)})));`,
			].join('');
			const out = execSync(`node -e ${JSON.stringify(script)}`, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
			return { cwd, out: JSON.parse(out) };
		};
		const hard = runTsgChild('hard', 'go test ./...');
		check('test-scope hard → block true', hard.out.blocked === true);
		const hardRows = policyRows(hard.cwd);
		check('test-scope hard → policy.decision(block, full-go-test)', hardRows.length === 1 && hardRows[0].payload.action === 'block' && hardRows[0].payload.reasonCode === 'full-go-test');
		const off = runTsgChild('off', 'go test ./...');
		check('test-scope off → 不阻断且零 policy.decision', off.out.blocked === false && policyRows(off.cwd).length === 0);

		/* ---------- 5. 隔离复核（2.4，harden-gate-interop-health 修订）：quality-gate /
		     entry-gate 只写 gate.check；quality-gate 唯一例外 = interop 探测短路
		     （spec harness-fact-log「策略显著裁决统一记账」：policy=quality-gate、
		     fail-open、interop-down，且被短路跳过的命令零 gate.check 双写） ---------- */
		const qg = fs.readFileSync(path.resolve('../quality-gate.ts'), 'utf8');
		const eg = fs.readFileSync(path.resolve('../entry-gate.ts'), 'utf8');
		check('quality-gate 源码 logPolicyDecision 仅 interop 短路一处（唯一例外场景）',
			(qg.match(/logPolicyDecision\(/g) ?? []).length === 1 &&
			qg.includes('reasonCode: "interop-down"') &&
			qg.includes('"quality-gate"'));
		check('entry-gate 源码无 policy.decision 引用（同裁决无双写）', !eg.includes('policy.decision') && !eg.includes('logPolicyDecision'));

		let fail = 0;
		for (const [name, ok] of checks) {
			console.log(`${ok ? '✅' : '❌'} ${name}`);
			if (!ok) fail++;
		}
		console.log(fail ? `\n${fail} 项失败` : '\nSMOKE OK');
		if (fail) process.exitCode = 1;
	} catch (e) {
		console.error('FAIL', e);
		process.exitCode = 1;
	} finally {
		for (const t of tmps) fs.rmSync(t, { recursive: true, force: true });
	}
})();
