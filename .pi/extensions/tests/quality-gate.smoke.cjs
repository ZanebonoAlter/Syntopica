// quality-gate smoke（harness-observability-fixes D1）：computeTriggerSet 纯函数
// + 增量路由判定（残留不触发 / 编辑触发 / 删除不触发 / 粘性路由）。
// 由 run-harness-smoke.sh 调用（先 esbuild 产出 .tgset.cjs）。断言失败 exit 1。
const { computeTriggerSet } = require('./.tgset.cjs');
const { initGateOkState, stepGateOk, isCompileFailure, GATE_OK_SAMPLE_EVERY } = require('./.gsamp.cjs');
const { sessionIdFromSessionFile, parentSessionIdFromForkFile, parentSessionId, isChildSession } = require('./.childsess.cjs');

const checks = [];
const check = (name, ok) => checks.push([name, ok]);

/* ---------- computeTriggerSet 纯函数 ---------- */
{
	// 残留基线：prev === curr → 空触发集（会话前脏文件不触发）
	const base = new Map([
		['front/app/foo.vue', { mtimeMs: 1000, size: 10 }],
		['backend-go/internal/x/a.go', { mtimeMs: 1000, size: 20 }],
	]);
	check('残留基线不触发（prev=curr → 空）', computeTriggerSet(base, new Map(base)).size === 0);

	// 编辑：mtime 变化 → 触发该路径
	const edited = new Map(base);
	edited.set('backend-go/internal/x/a.go', { mtimeMs: 2000, size: 20 });
	const t1 = computeTriggerSet(base, edited);
	check('编辑（mtime 变）触发该路径', t1.size === 1 && t1.has('backend-go/internal/x/a.go'));

	// size 变化（mtime 相同的极端场景）也触发
	const resized = new Map(base);
	resized.set('front/app/foo.vue', { mtimeMs: 1000, size: 99 });
	check('size 变（mtime 同）触发', computeTriggerSet(base, resized).has('front/app/foo.vue'));

	// 新增路径触发
	const added = new Map(base);
	added.set('backend-go/internal/y/b.go', { mtimeMs: 3000, size: 5 });
	check('新增 untracked 路径触发', computeTriggerSet(base, added).has('backend-go/internal/y/b.go'));

	// 纯删除不触发（删除文件由归档门禁/agent 手动负责，无验证命令可跑）
	const removed = new Map(base);
	removed.delete('front/app/foo.vue');
	const t2 = computeTriggerSet(base, removed);
	check('纯删除不触发', t2.size === 0);

	// 空快照 → 空触发集
	check('空触发集为空集', computeTriggerSet(new Map(), new Map()).size === 0);

	// 确定性
	const d1 = computeTriggerSet(base, added);
	const d2 = computeTriggerSet(base, new Map(added));
	check('确定性（同输入同输出）', d1.size === d2.size && [...d1].every((p) => d2.has(p)));
}

/* ---------- gate-sample 状态机（harness-quick-wins D3/D4，白盒分支表见 test-cases.md） ---------- */
{
	// 会话锚点：初始态首条成功必记 flip，进入 everGreen
	let st = initGateOkState();
	let d = stepGateOk(st, true);
	check('会话首条成功=锚点必记 flip', d.log === true && d.flip === true && d.next.everGreen === true && d.next.consecOk === 1);

	// 连续成功 2~4 不落库，第 5 次落 sampled
	st = d.next; d = stepGateOk(st, true);
	check('连续成功第 2 次不记', d.log === false && d.next.consecOk === 2);
	st = d.next; d = stepGateOk(st, true);
	check('连续成功第 3 次不记', d.log === false);
	st = d.next; d = stepGateOk(st, true);
	check('连续成功第 4 次不记', d.log === false && d.next.consecOk === 4);
	st = d.next; d = stepGateOk(st, true);
	check(`连续成功第 ${GATE_OK_SAMPLE_EVERY} 次记 sampled`, d.log === true && d.sampled === true && d.n === GATE_OK_SAMPLE_EVERY && d.next.consecOk === 0);

	// 周期回绕：第 6~9 不记，第 10 再记
	st = d.next;
	let logged = 0;
	for (let i = 0; i < 4; i++) { st = stepGateOk(st, true).next; }
	check('采样后第 6~9 次不记（consecOk 回到 4）', st.consecOk === 4);
	d = stepGateOk(st, true);
	check('第 10 次再记 sampled', d.log === true && d.sampled === true);

	// 失败：全量记 + 前缀分级 + 计数清零；曾绿 → [回归]
	d = stepGateOk(d.next, false);
	check('曾绿失败全记且前缀[回归]', d.log === true && d.failPrefix === '[回归]' && d.next.consecOk === 0 && d.next.lastWasFail === true && d.next.everGreen === true);

	// 转绿锚点：失败后首个成功必记 flip
	d = stepGateOk(d.next, true);
	check('转绿首个成功=锚点必记 flip', d.log === true && d.flip === true && d.next.consecOk === 1);

	// 从未绿失败：前缀[中间态]
	const fresh = initGateOkState();
	d = stepGateOk(fresh, false);
	check('从未绿失败前缀[中间态]', d.log === true && d.failPrefix === '[中间态]' && d.next.everGreen === false);

	// 失败/成功交替（最坏记账量）：失败后的每个成功都是锚点必记
	let stAlt = stepGateOk(d.next, false).next; // 从失败态起算（交替模式）
	let flips = 0;
	for (let i = 0; i < 3; i++) {
		const okD = stepGateOk(stAlt, true);
		if (okD.flip) flips++;
		stAlt = stepGateOk(okD.next, false).next;
	}
	check('失败/成功交替时每次成功都是锚点（3/3）', flips === 3);

	// 确定性
	const a1 = stepGateOk(initGateOkState(), false);
	const a2 = stepGateOk(initGateOkState(), false);
	check('确定性（同输入同输出）', JSON.stringify(a1) === JSON.stringify(a2));
}

/* ---------- isCompileFailure 短路哨兵（D2） ---------- */
{
	check('typechecking error 命中短路', isCompileFailure('level=error msg="[linters_context] typechecking error: : # pkg\nmain.go:1:1: expected declaration"') === true);
	check('[build failed] 命中短路', isCompileFailure('FAIL syntopica-backend/internal/x [build failed]') === true);
	check('unused 等 lint 规则错误不短路', isCompileFailure('internal\\service\\evidence.go:43:6: func sanitizeEvidenceChain is unused (unused)') === false);
	check('gofmt 错误不短路', isCompileFailure('internal\\dataenrichment\\repository\\models.go:194:1: File is not properly formatted (gofmt)') === false);
	check('空输出不短路', isCompileFailure('') === false);
	check('0 issues 不短路', isCompileFailure('0 issues.') === false);
}

/* ---------- isChildSession 判定矩阵（harden-subagent-constraint-channel T3/T8）----------
 * 判定证据与 constraint-injection 父子继承同源（lib/child-session 单一真相源）：
 * header parentSession × fork 路径 × 无 sessionId stub 槽位。quality-gate 降载短路
 * （behavior smoke 场景 J/K）与 constraint-injection 继承（19.x）共用本判定面。 */
{
	const PARENT = '/home/x/.pi/agent/sessions/dir/2026-09-18T00-00-00-000Z_parent-session.jsonl';
	const ctxOf = (sm) => ({ sessionManager: sm });
	// 基元解析：<ts>_<id>.jsonl → id（时间戳与 id 之间唯一一个下划线，id 自身含连字符）
	check('CS1 sessionIdFromSessionFile 解析 id（保留首个下划线后完整段）', sessionIdFromSessionFile(PARENT) === 'parent-session');
	check('CS2 sessionIdFromSessionFile 空/无下划线 → null', sessionIdFromSessionFile(undefined) === null
		&& sessionIdFromSessionFile('') === null && sessionIdFromSessionFile('no-underscore-here.jsonl') === null);
	check('CS3 parentSessionIdFromForkFile 从 forks 上级目录解父 id',
		parentSessionIdFromForkFile('/x/2026-09-18T00-00-00-000Z_dad/forks/2026-09-18T00-01-00-000Z_kid.jsonl') === 'dad');
	check('CS4 非 fork 路径/空 → null（不可证）', parentSessionIdFromForkFile('/x/2026-09-18T00-00-00-000Z_kid.jsonl') === null
		&& parentSessionIdFromForkFile(undefined) === null);
	// ctx 判定矩阵
	check('CS5 header 带 parentSession（pi-web/pi-subagents 子会话实测形态）→ 子线程',
		isChildSession(ctxOf({ getSessionId: () => 'kid', getHeader: () => ({ parentSession: PARENT }) })) === true);
	check('CS6 header 无 parentSession 且无会话文件 → 非子线程',
		isChildSession(ctxOf({ getSessionId: () => 'kid', getHeader: () => ({}) })) === false);
	check('CS7 header 返回 null → 非子线程（失败方向安全）',
		isChildSession(ctxOf({ getHeader: () => null })) === false);
	check('CS8 header 缺失时 fork 路径兑底 → 子线程',
		isChildSession(ctxOf({
			getSessionId: () => 'kid',
			getHeader: () => null,
			getSessionFile: () => '/x/2026-09-18T00-00-00-000Z_dad/forks/2026-09-18T00-01-00-000Z_kid.jsonl',
		})) === true);
	check('CS9 会话文件存在但非 forks/ 下 → 非子线程',
		isChildSession(ctxOf({ getSessionFile: () => '/x/2026-09-18T00-00-00-000Z_kid.jsonl' })) === false);
	check('CS10 header 与 fork 路径同证 → 取 header（证据①优先）',
		parentSessionId(ctxOf({
			getHeader: () => ({ parentSession: PARENT }),
			getSessionFile: () => '/x/2026-09-18T00-00-00-000Z_dad/forks/2026-09-18T00-01-00-000Z_kid.jsonl',
		})) === 'parent-session');
	check('CS11 无 sessionId stub 槽位（无/空 sessionManager）→ 非子线程',
		isChildSession({}) === false && isChildSession(undefined) === false && isChildSession(ctxOf({})) === false);
}

/* ---------- edit-map 归属地图（coordinate-concurrent-changes）---------- */
{
	const fs = require('fs');
	const os = require('os');
	const path = require('path');
	const {
		parseModeSetPayload,
		mergePaths,
		buildEditMapPayload,
		syncEditMap,
		resetEditMapState,
	} = require('./.emap.cjs');
	const { logEvent, queryByChange, queryBySession } = require('./.hlog.cjs');

	const mktmp = () => fs.mkdtempSync(path.join(os.tmpdir(), 'emap-smoke-'));
	const lastPaths = (cwd, change) => {
		const rows = queryByChange(cwd, change, ['edit.map']);
		return rows.length ? JSON.parse(rows[rows.length - 1].payload).paths : null;
	};

	// ---- B 系纯函数 ----
	check('B1 parseModeSet 合法行', parseModeSetPayload({ payload: JSON.stringify({ mode: 'implementation', boundChange: 'foo' }) }) === 'foo');
	check('B2 parseModeSet payload 损坏 → null', parseModeSetPayload({ payload: '{broken' }) === null);
	check('B2b parseModeSet 行缺失 → null', parseModeSetPayload(null) === null && parseModeSetPayload(undefined) === null);
	check('B3 parseModeSet boundChange=null → null（无档会话）', parseModeSetPayload({ payload: JSON.stringify({ mode: 'requirements', boundChange: null }) }) === null);
	check('B4 mergePaths 并集排序去重', JSON.stringify(mergePaths(['b', 'a'], ['a', 'c'])) === JSON.stringify(['a', 'b', 'c']));
	check('B5 mergePaths 空集边界', mergePaths([], []).length === 0);
	check('B5b mergePaths 非串/空串成员被滤', JSON.stringify(mergePaths(['a', '', undefined], ['b'])) === JSON.stringify(['a', 'b']));
	const many = buildEditMapPayload(Array.from({ length: 500 }, (_, i) => `dir/f${i}.go`));
	check('B6 buildPayload 500+ 路径 n=paths.length', many.n === 500 && many.paths.length === 500);

	// ---- Q 系状态机（临时库 + 伪造 mode.set）----
	const tmps = [];
	try {
		// Q3 无 mode.set 的会话：零事件不计入
		const t1 = mktmp(); tmps.push(t1);
		check('Q3 无档会话 syncEditMap 返回 false 零事件', syncEditMap(t1, 's-none', ['x.go']) === false && lastPaths(t1, 'any') === null);

		// Q1/Q2 同 session 绑 foo：首条快照 + 累计并集
		logEvent(t1, { kind: 'mode.set', sessionId: 's1', change: 'foo', payload: { mode: 'implementation', boundChange: 'foo' } });
		check('Q1 首次落库快照 = base∅∪trigger', syncEditMap(t1, 's1', ['a.go']) === true && JSON.stringify(lastPaths(t1, 'foo')) === JSON.stringify(['a.go']));
		syncEditMap(t1, 's1', ['b.go']);
		check('Q2 连续回合累计快照（并集）', JSON.stringify(lastPaths(t1, 'foo')) === JSON.stringify(['a.go', 'b.go']));
		const rowsFoo = queryByChange(t1, 'foo', ['edit.map']);
		check('Q2b 每回合独立追加（两条事件，非改写）', rowsFoo.length === 2);

		// Q5 档位切换到 bar：base 重读 bar（空起步），foo 不再增长
		logEvent(t1, { kind: 'mode.set', sessionId: 's1', change: 'bar', payload: { mode: 'implementation', boundChange: 'bar' } });
		syncEditMap(t1, 's1', ['z.go']);
		check('Q5 档位切换：bar 空起步且 foo 不再增长', JSON.stringify(lastPaths(t1, 'bar')) === JSON.stringify(['z.go']) && JSON.stringify(lastPaths(t1, 'foo')) === JSON.stringify(['a.go', 'b.go']));

		// Q8 跨 session 续接：新 session s2 绑 foo，base 从库读旧快照起步
		logEvent(t1, { kind: 'mode.set', sessionId: 's2', change: 'foo', payload: { mode: 'implementation', boundChange: 'foo' } });
		syncEditMap(t1, 's2', ['c.go']);
		check('Q8 跨 session 并集不丢（s2 落 {a,b,c}）', JSON.stringify(lastPaths(t1, 'foo')) === JSON.stringify(['a.go', 'b.go', 'c.go']));

		// Q6 reset 后重新 sync：从库续接（会话边界重置语义）
		resetEditMapState();
		syncEditMap(t1, 's2', ['d.go']);
		check('Q6 reset 后 base 从库重新续接（{a,b,c,d}）', JSON.stringify(lastPaths(t1, 'foo')) === JSON.stringify(['a.go', 'b.go', 'c.go', 'd.go']));

		// Q4 纯对话回合：由调用方保证（trigger.size=0 不调）；验证空 trigger 也不破坏状态
		const before = lastPaths(t1, 'foo');
		syncEditMap(t1, 's2', []);
		check('Q4b 空 trigger 落库为原快照（幂等无害）', JSON.stringify(lastPaths(t1, 'foo')) === JSON.stringify(before));
	} finally {
		for (const t of tmps) fs.rmSync(t, { recursive: true, force: true });
	}
}

/* ---------- F 系：工具自管文件剔除（fix-doc-impact-misattribution D1 采集侧） ---------- */
{
	const fs = require('fs');
	const os = require('os');
	const path = require('path');
	const { isToolManagedPath, syncEditMap, resetEditMapState } = require('./.emap.cjs');
	const { logEvent, queryByChange } = require('./.hlog.cjs');

	const mktmp = () => fs.mkdtempSync(path.join(os.tmpdir(), 'emap-filter-'));
	const lastPaths = (cwd, change) => {
		const rows = queryByChange(cwd, change, ['edit.map']);
		return rows.length ? JSON.parse(rows[rows.length - 1].payload).paths : null;
	};

	// F1/F2 工具自管路径判定（openspec CLI 归档移动产物与元数据）
	check('F1 archive 前缀路径剔除', isToolManagedPath('openspec/changes/archive/2026-09-05-watch-x/.openspec.yaml') === true
		&& isToolManagedPath('openspec/changes/archive/2026-09-05-watch-x/tasks.md') === true);
	check('F2 active change 的 .openspec.yaml 剔除', isToolManagedPath('openspec/changes/configure-dsh-energy-research/.openspec.yaml') === true);
	// F3 共享文档不在采集侧剔除（并发冲突感知保留，design D1）
	check('F3 共享文档不剔除（AGENTS.md 保留在归属集合）', isToolManagedPath('AGENTS.md') === false
		&& isToolManagedPath('front/AGENTS.md') === false && isToolManagedPath('docs/reference/constraints-index.md') === false);
	check('F3b 正常业务路径不剔除', isToolManagedPath('backend-go/internal/reader/handler/a.go') === false
		&& isToolManagedPath('docs/reference/standard/frontend/layout.md') === false);
	// F3c 边界：active change 目录下的 tasks.md 等真实产物不剔除（只有 .openspec.yaml 是工具元数据）
	check('F3c active change 的 tasks.md 不剔除（非工具元数据）', isToolManagedPath('openspec/changes/foo/tasks.md') === false);

	const tmps = [];
	resetEditMapState(); // 隔离 Q 系残留的模块状态（同 bundle 实例跨块共享）
	try {
		// F4 全工具自管 trigger：零事件（不落库，不产生污染快照）
		const t1 = mktmp(); tmps.push(t1);
		logEvent(t1, { kind: 'mode.set', sessionId: 's1', change: 'foo', payload: { mode: 'implementation', boundChange: 'foo' } });
		syncEditMap(t1, 's1', ['openspec/changes/archive/2026-09-05-x/.openspec.yaml', 'openspec/changes/foo/.openspec.yaml']);
		check('F4 全工具自管 trigger 零事件', lastPaths(t1, 'foo') === null);

		// F5 混合 trigger：落库集合只含正常路径
		syncEditMap(t1, 's1', ['backend-go/a.go', 'openspec/changes/archive/2026-09-05-x/tasks.md']);
		check('F5 混合 trigger 落库仅正常路径', JSON.stringify(lastPaths(t1, 'foo')) === JSON.stringify(['backend-go/a.go']));

		// F6 存量污染 base：新快照不再携带（采集侧止增，旧事件不动）
		const t2 = mktmp(); tmps.push(t2);
		logEvent(t2, { kind: 'mode.set', sessionId: 's1', change: 'bar', payload: { mode: 'implementation', boundChange: 'bar' } });
		logEvent(t2, { kind: 'edit.map', sessionId: 's1', change: 'bar', payload: { paths: ['openspec/changes/archive/legacy/.openspec.yaml', 'old.go'], n: 2 } });
		resetEditMapState();
		syncEditMap(t2, 's1', ['new.go']);
		check('F6 存量污染 base 不扩散到新快照', JSON.stringify(lastPaths(t2, 'bar')) === JSON.stringify(['new.go', 'old.go']));
	} finally {
		for (const t of tmps) fs.rmSync(t, { recursive: true, force: true });
	}
}

/* ---------- B2 系：门禁命令形态（tune-quality-gate-concurrency D2：限核） ----------
 * 事件形状回放（behavior smoke 同模式的最小 mock）：native/windows 两轮 turn_end 捕获
 * pi.exec 调用，断言 GOMAXPROCS=2 前缀两形态 / lint --concurrency=2 / vet-build 串行 / eslint 不变。
 * B2-4 串行判定：vet 分支经 setImmediate 异步结算后置 vetSettled；build 分支若在 vet
 * 结算前发起（Promise.all 并发窗口）则记违规——同步 mock 分不出并发窗口，必须异步间隔。
 * 本段异步（await turn_end），包 IIFE；汇总段同步先跑，B2 断言异常/失败均置 exitCode=1。 */
(async () => {
	const fs = require('fs');
	const os = require('os');
	const path = require('path');
	const qualityGate = require('./.qgateb.cjs').default;

	const fakeWin = fs.mkdtempSync(path.join(os.tmpdir(), 'b2fakewin-'));
	fs.writeFileSync(path.join(fakeWin, 'cmd.exe'), '#!/bin/sh\nexit 0\n', { mode: 0o755 });
	const fakeNative = fs.mkdtempSync(path.join(os.tmpdir(), 'b2nat-'));
	for (const exe of ['go', 'golangci-lint', 'pnpm']) {
		fs.writeFileSync(path.join(fakeNative, exe), '#!/bin/sh\nexit 0\n', { mode: 0o755 });
	}
	const REAL_PATH = process.env.PATH ?? '';

	const mkrepo = () => {
		const repo = fs.mkdtempSync(path.join(os.tmpdir(), 'b2smoke-'));
		fs.mkdirSync(path.join(repo, 'backend-go'), { recursive: true });
		fs.mkdirSync(path.join(repo, 'front', 'app'), { recursive: true });
		fs.writeFileSync(path.join(repo, 'backend-go', 'a.go'), 'package x\n');
		fs.writeFileSync(path.join(repo, 'front', 'app', 'x.ts'), 'export {}\n');
		return repo;
	};
	const mkctx = (sid) => ({ signal: undefined, sessionManager: { getSessionId: () => sid } });
	const EDIT_TURN = { toolResults: [{ toolName: 'edit' }] };

	/** 跑一轮 turn_end（session_start 空基线 → files 触发 → EDIT_TURN），返回 execCalls */
	const runGateTurn = async (platform, triggerFiles, routerExtra) => {
		process.env.PATH =
			platform === 'windows' ? `${fakeWin}${path.delimiter}${REAL_PATH}` : fakeNative;
		const repo = mkrepo();
		const handlers = {};
		const execCalls = [];
		let files = ''; // 基线阶段空，turn_end 前才放触发文件（否则进基线不触发）
		qualityGate({
			on: (e, h) => { handlers[e] = h; },
			exec: async (cmd, args, opts) => {
				execCalls.push({ cmd, cl: `${cmd} ${args.join(' ')}`, args, opts });
				if (cmd === 'git') {
					if (args[0] === 'diff') return { code: 0, stdout: '', stderr: '' };
					if (args[0] === 'ls-files') return { code: 0, stdout: files, stderr: '' };
					if (args[0] === 'rev-parse') return { code: 0, stdout: repo, stderr: '' };
					throw new Error(`unexpected git: ${args.join(' ')}`);
				}
				return routerExtra(cmd, args);
			},
			sendMessage: () => {},
		});
		await handlers['session_start']({ reason: 'new' }, mkctx('b2-smoke'));
		files = triggerFiles;
		await handlers['turn_end'](EDIT_TURN, mkctx('b2-smoke'));
		return execCalls;
	};

	// ---- B2-1/B2-3/B2-4：native 命令形态与串行 ----
	{
		let vetSettled = false;
		let buildRacedVet = false;
		const calls = await runGateTurn('native', 'backend-go/a.go', (cmd, args) => {
			const cl = args.join(' ');
			if (cmd === 'bash' && cl.includes('change-scope')) return { code: 0, stdout: '{"testTargets":[]}', stderr: '' };
			if (cmd === 'bash') {
				if (/\bvet\b/.test(cl)) {
					return (async () => {
						await new Promise((r) => setImmediate(r)); // 异步结算窗口（见头部说明）
						vetSettled = true;
						return { code: 0, stdout: '', stderr: '' };
					})();
				}
				if (/\bbuild\b/.test(cl)) {
					if (!vetSettled) buildRacedVet = true;
					return { code: 0, stdout: '', stderr: '' };
				}
				return { code: 0, stdout: '0 issues.', stderr: '' }; // lint
			}
			throw new Error(`unexpected: ${cmd} ${cl}`);
		});
		const backendCls = calls.filter((c) => c.cmd === 'bash' && !c.cl.includes('change-scope')).map((c) => c.args[1]);
		const goCls = backendCls.filter((cl) => /golangci-lint|\bgo (vet|build)\b|\bgo test\b/.test(cl));
		check('B2-1 native 模式 go 类门禁命令统一 GOMAXPROCS=2 前缀（bash -c 语境）',
			goCls.length >= 3 && goCls.every((cl) => cl.startsWith('GOMAXPROCS=2 ')));
		check('B2-3 golangci-lint 同时含 --concurrency=2 与 --allow-parallel-runners',
			backendCls.some((cl) => cl.includes('golangci-lint') && cl.includes('--concurrency=2') && cl.includes('--allow-parallel-runners')));
		check('B2-4 vet 与 build 串行（build 发起不早于 vet 结算）', !buildRacedVet);
	}

	// ---- B2-2：windows cmd.exe 形态 ----
	{
		const calls = await runGateTurn('windows', 'backend-go/a.go', (cmd, args) => {
			const cl = args.join(' ');
			if (cmd === 'cmd.exe' && cl.includes('echo ok')) return { code: 0, stdout: 'ok', stderr: '' };
			if (cmd === 'cmd.exe') return { code: 0, stdout: '0 issues.', stderr: '' };
			if (cmd === 'bash') return { code: 0, stdout: '{"testTargets":[]}', stderr: '' };
			throw new Error(`unexpected: ${cmd} ${cl}`);
		});
		const gateCmds = calls.filter((c) => c.cmd === 'cmd.exe' && !c.cl.includes('echo ok'));
		check('B2-2 windows 模式 set GOMAXPROCS=2 && 形态且 cd /d 链路不破坏',
			gateCmds.length >= 3
				&& gateCmds.every((c) => /cd \/d \S+\/backend-go && set GOMAXPROCS=2 && /.test(c.args[1] ?? '')));
	}

	// ---- B2-5：前端 eslint 命令零改动 ----
	{
		const calls = await runGateTurn('native', 'front/app/x.ts', (cmd, args) => {
			const cl = args.join(' ');
			if (cmd === 'bash' && cl.includes('change-scope')) return { code: 0, stdout: '{"testTargets":[]}', stderr: '' };
			if (cmd === 'bash' && cl.includes('eslint')) return { code: 0, stdout: '', stderr: '' };
			throw new Error(`unexpected: ${cmd} ${cl}`);
		});
		const eslintCls = calls.filter((c) => c.cmd === 'bash' && c.cl.includes('eslint')).map((c) => c.args[1]);
		check('B2-5 前端 eslint 命令零改动（无 GOMAXPROCS，--cache 参数原样）',
			eslintCls.length === 1 && !eslintCls[0].includes('GOMAXPROCS')
				&& eslintCls[0].includes('--cache --cache-location node_modules/.cache/eslint/.eslintcache'));
	}

	process.env.PATH = REAL_PATH;

	/* ---------- 汇总（在 async 尾部：同步段与 B2 段的 checks 一起结账） ---------- */
	let fail = 0;
	for (const [name, ok] of checks) {
		console.log(`${ok ? '✅' : '❌'} ${name}`);
		if (!ok) fail++;
	}
	console.log(fail ? `\n${fail} 项失败` : '\nSMOKE OK');
	if (fail) process.exitCode = 1;
})().catch((e) => { console.error('B2 SMOKE FAIL', e); process.exitCode = 1; });
