// spec-gate smoke：检查⑤验收措辞扫描纯函数（⑤b 纯函数×SQLite 分层错配；⑤a 复杂档白盒
// 缺件提醒已退役——复杂档/算法关键词不再催补 test-cases.md）+ 归档门禁 policy.decision 记账
// （harden-harness-policy-and-spill）：block / bypass / 检查⑤ warn / 正常放行零记录，
// 且原 block/放行/custom_message 行为不变（mock pi.exec 驱动完整 tool_call）。
// + 归档侧约束文档读取对账（stage-driven-constraint-injection D6）：对账报红 → block 且 reason
// 含未读域清单与取数指引；脚本缺席（127）→ fail-open 不阻断。
// + 复杂档 + 有 delta Scenario 但验证节无映射表 → 归档门禁放行（④ 场景映射对账已退役）。
// 由 run-harness-smoke.sh 调用（先 esbuild 产出 .sgate.cjs）。断言失败 exit 1。
const fs = require('fs');
const os = require('os');
const path = require('path');
const { execSync } = require('child_process');
const { scanAcceptanceWording } = require('./.sgate.cjs');

const priorWarn = process.listeners('warning');
process.removeAllListeners('warning');
process.on('warning', (w) => {
	if (!(w.name === 'ExperimentalWarning' && /sqlite/i.test(w.message))) {
		for (const l of priorWarn) l(w);
	}
});
const { DatabaseSync } = require('node:sqlite');

const mktmp = () => fs.mkdtempSync(path.join(os.tmpdir(), 'sgate-smoke-'));
const policyRows = (cwd) => {
	const dbFile = path.join(cwd, '.pi', 'harness', 'events.db');
	if (!fs.existsSync(dbFile)) return [];
	const db = new DatabaseSync(dbFile);
	const rows = db.prepare("SELECT * FROM events WHERE kind='policy.decision' ORDER BY id").all();
	db.close();
	return rows.map((r) => ({ ...r, payload: JSON.parse(r.payload) }));
};
/** mock pi：exec 按 (cmd,args) 分发；sendMessage 收集 custom_message */
function makeGatePi(execImpl) {
	const handlers = {};
	const messages = [];
	const pi = {
		on: (name, fn) => (handlers[name] = fn),
		exec: execImpl,
		sendMessage: (m) => messages.push(m),
	};
	return { pi, handlers, messages };
}
const OK_TASKS = ['## 1. 任务', '- [ ] 1.1 干活', '', '## 2. 测试', '## 3. 文档', '<!-- doc-impact: 无 -->', '', '## 4. 验证'].join('\n');
/** 默认 exec：归档检查全过（bash→code 0；cat→合法 tasks；ls→文件列表） */
const passExec = async (cmd, args) => {
	const a = args.join(' ');
	if (cmd === 'bash') return { code: 0, stdout: '', stderr: '' };
	if (cmd === 'cat' && a.includes('tasks.md')) return { code: 0, stdout: OK_TASKS, stderr: '' };
	if (cmd === 'cat' && a.includes('proposal.md')) return { code: 0, stdout: '<!-- complexity: simple -->', stderr: '' };
	if (cmd === 'ls') return { code: 0, stdout: 'proposal.md\ndesign.md\ntasks.md\ntest-cases.md', stderr: '' };
	return { code: 0, stdout: '', stderr: '' };
};
/** 驱动一次归档 tool_call；sessionId 可指定（默认 'sgs1'）以测会话隔离/新会话首见 */
const runGate = (pi, command, cwd, sessionId) => pi.handlers['tool_call'](
	{ toolName: 'bash', input: { command }, toolCallId: 'sg1' },
	{ signal: undefined, cwd, sessionManager: sessionId === null ? {} : { getSessionId: () => sessionId ?? 'sgs1' } },
);

const checks = [];
const check = (name, ok) => checks.push([name, ok]);

(async () => {
try {
	// ---- 1. 无违例输入（干净 tasks.md）→ 空数组 ----
	const cleanTasks = [
		'## 1. 任务',
		'- [ ] 1.1 写 handler 测试：验收 `go test ./internal/x -run TestY → PASS`',
		'- [x] 1.2 补文档：新增 docs/foo.md',
		'非任务行含解析算法协议状态机——不锚定任务行，不应命中',
	].join('\n');
	const none = scanAcceptanceWording(cleanTasks);
	check('干净 tasks.md → 空数组', Array.isArray(none) && none.length === 0);

	// ---- 2. ⑤a 退役（streamline-business-test-verification）：复杂档/关键词不再催补白盒文档 ----
	const a1 = scanAcceptanceWording('- [ ] 2.1 实现正文解析逻辑 extractContent');
	check('⑤a 退役：任务行含「解析」→ 0 条（不再催补白盒文档）', a1.length === 0);
	const a2 = scanAcceptanceWording(
		['- [ ] 1.1 实现解析逻辑', '- [x] 1.2 再写一层解析', '- [ ] 1.3 状态机迁移'].join('\n'),
	);
	check('⑤a 退役：多关键词多行 → 0 条', a2.length === 0);

	// ---- 3. ⑤b 正样本：同一任务行同含「纯函数」+「SQLite」→ 1 条 ----
	const b1 = scanAcceptanceWording('- [ ] 2.1 给排序写纯函数单测：SQLite 内存库建表 → PASS');
	check('⑤b 正样本：纯函数+SQLite 同行 → 1 条', b1.length === 1);
	check('⑤b 文案含分层指引 *_unit_test.go 无 DB', b1[0].includes('*_unit_test.go 无 DB'));

	// ⑤b 每行只一条：同一行纯函数/SQLite 出现多次仍 1 条；两行则 2 条
	const b2 = scanAcceptanceWording('- [x] 3.1 纯函数纯函数逻辑用 SQLite SQLite 验证');
	check('⑤b 每行一条：同词重复仍 1 条', b2.length === 1);
	const b3 = scanAcceptanceWording(
		['- [ ] 4.1 纯函数走 SQLite', '- [ ] 4.2 另一任务纯函数配 SQLite'].join('\n'),
	);
	check('⑤b 多行各一条：2 行 → 2 条', b3.length === 2);
	// ⑤b 半命中（只含其一）不告警
	const b4 = scanAcceptanceWording('- [ ] 5.1 纯函数单测（无 DB）\n- [ ] 5.2 repository 用 SQLite 集成测试');
	check('⑤b 半命中（纯函数/SQLite 各自单独出现）→ 0 条', b4.length === 0);

	// ---- 4. 健壮性：空串 / 超长行(>10KB) / 纯标签行 / 畸形输入 → 不抛异常返回数组 ----
	let robust = true;
	try {
		const empty = scanAcceptanceWording('');
		robust = robust && Array.isArray(empty) && empty.length === 0;
		const longLine = scanAcceptanceWording('- [ ] ' + '解析'.repeat(6000)); // ~18KB 行
		robust = robust && Array.isArray(longLine) && longLine.length === 0;
		const bareLabels = scanAcceptanceWording(['- [ ]', '- [x]', '  - [x] ', '- []', '[]', '- [q] bogus'].join('\n'));
		robust = robust && Array.isArray(bareLabels) && bareLabels.length === 0;
		const weird = scanAcceptanceWording('- [ ] 1.1 纯函数 SQLite');
		robust = robust && Array.isArray(weird) && weird.length === 1; // 含换行的畸形行只要求不抛异常返回数组
	} catch (e) {
		robust = false;
		console.error('健壮性用例抛异常：', e);
	}
	check('健壮性：空串/超长行/纯标签行/畸形行 → 不抛异常返回合理结果', robust);

	// ---- 5. ⑤a 退役锚点：复杂档 + 关键词任务行 → 0 条（不催补白盒文档）----
	check(
		'⑤a 退役：复杂档关键词任务行（解析/算法/状态机/协议）→ 0 条',
		scanAcceptanceWording('- [ ] 6.1 实现状态机迁移\n- [ ] 6.2 协议字段\n- [ ] 6.3 解析算法').length === 0,
	);

	// ---- 6. ⑤b 保留锚点：分层错配提示与复杂度无关，仍照常产出 ----
	const s3 = scanAcceptanceWording('- [ ] 7.1 给排序写纯函数单测：SQLite 内存库建表 → PASS');
	check('⑤b 保留：分层错配提示照常产出（与复杂度无关）', s3.length === 1 && s3[0].includes('*_unit_test.go 无 DB'));

	// ---- 7. 归档门禁 policy.decision 记账（harden-harness-policy-and-spill，驱动完整 tool_call）----
	const tmps = [];
	try {
		// 7a. 归档检查失败 → block + policy.decision(block, archive-check-failed)，绑定被归档 change
		const tA = mktmp(); tmps.push(tA);
		const gA = makeGatePi(async () => ({ code: 1, stdout: '', stderr: 'boom' }));
		require('./.sgate.cjs').default(gA.pi);
		const rA = await runGate(gA, 'openspec archive fake-change', tA);
		const rowsA = policyRows(tA);
		check('归档检查失败 → 原 block 行为不变（block:true + reason 含失败项）', rA && rA.block === true && /归档门禁未通过/.test(rA.reason));
		check('block reason 含台账登记指引（撞见域外红 → test-patrol.sh --register）', rA && /test-patrol\.sh --register/.test(rA.reason));
		check('block reason 含归档就绪自查指引（archive-readiness.sh + 勿逐项试探）', rA && /archive-readiness\.sh/.test(rA.reason) && /勿补一项就试一次/.test(rA.reason));
		check('block → 恰两条记账：ui-design-gate block ui-verification-missing + spec-gate block archive-check-failed',
			rowsA.length === 2
				&& rowsA[0].payload.policy === 'ui-design-gate' && rowsA[0].payload.action === 'block' && rowsA[0].payload.reasonCode === 'ui-verification-missing'
				&& rowsA[1].payload.policy === 'spec-gate' && rowsA[1].payload.action === 'block' && rowsA[1].payload.reasonCode === 'archive-check-failed');
		check('block 事件 change 绑定被归档 change（fake-change）', rowsA.every((r) => r.change === 'fake-change'));
		check('block 事件 target 只含失败检查名（无输出正文；④ 场景映射对账已退役）', rowsA[1].payload.target === 'doc-impact,standards,tasks,review,ui-evidence,constraint-read' && !JSON.stringify(rowsA[1].payload).includes('boom'));
		check('block 路径不发 custom_message', gA.messages.length === 0);

		// 7b. 显式 bypass（--force）→ 放行 + policy.decision(bypass, explicit-bypass)
		const tB = mktmp(); tmps.push(tB);
		const gB = makeGatePi(passExec);
		require('./.sgate.cjs').default(gB.pi);
		const rB = await runGate(gB, 'openspec archive fake-change --force', tB);
		const rowsB = policyRows(tB);
		check('--force → 原放行行为不变（undefined）+ custom_message 留痕', rB === undefined && gB.messages.length === 1 && gB.messages[0].customType === 'spec-gate-warning');
		check('bypass → policy.decision(bypass, explicit-bypass)，change 绑定命令中的 change',
			rowsB.length === 1 && rowsB[0].payload.action === 'bypass' && rowsB[0].payload.reasonCode === 'explicit-bypass' && rowsB[0].change === 'fake-change');

		// 7c. SPEC_GATE_BYPASS=1 环境豁免 → 同样 bypass 记账（子进程验证环境变量，主进程不污染）
		const tC = mktmp(); tmps.push(tC);
		const outC = JSON.parse(execSync(`node -e ${JSON.stringify([
			`process.env.SPEC_GATE_BYPASS='1';`,
			`const mod=require(${JSON.stringify(path.resolve(__dirname, '.sgate.cjs'))});`,
			`const h={};const msgs=[];const pi={on:(n,f)=>(h[n]=f),exec:async()=>({code:0,stdout:'',stderr:''}),sendMessage:(m)=>msgs.push(m)};`,
			`mod.default(pi);`,
			`h['tool_call']({toolName:'bash',input:{command:'openspec archive envbypass-change'},toolCallId:'t'},{cwd:${JSON.stringify(tC)},sessionManager:{getSessionId:()=>'sgs2'}})`,
			`.then((r)=>console.log(JSON.stringify({pass:r===undefined,msgs:msgs.length})));`,
		].join(''))}`, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim());
		const rowsC = policyRows(tC);
		check('SPEC_GATE_BYPASS=1 → 放行 + warning 留痕 + bypass 记账', outC.pass === true && outC.msgs === 1 && rowsC.length === 1 && rowsC[0].payload.action === 'bypass' && rowsC[0].change === 'envbypass-change');

		// 7d. 检查⑤ warning → policy.decision(warn, acceptance-wording)；归档检查仍放行（行为不变）
		const tD = mktmp(); tmps.push(tD);
		const gD = makeGatePi(async (cmd, args) => {
			const r = await passExec(cmd, args);
			if (cmd === 'cat' && args.join(' ').includes('tasks.md')) return { code: 0, stdout: ['## 1. 任务', '- [ ] 1.1 给排序写纯函数单测：SQLite 内存库建表 → PASS', '', '## 2. 测试', '## 3. 文档', '<!-- doc-impact: 无 -->', '', '## 4. 验证'].join('\n'), stderr: '' };
			return r;
		});
		require('./.sgate.cjs').default(gD.pi);
		const rD = await runGate(gD, 'openspec archive warn-change', tD);
		const rowsD = policyRows(tD);
		check('检查⑤ warn → 归档检查仍放行（undefined）+ warning custom_message', rD === undefined && gD.messages.some((m) => m.customType === 'spec-gate-warning' && /检查⑤/.test(m.content)));
		check('检查⑤ warn → policy.decision(warn, acceptance-wording) 绑定 warn-change',
			rowsD.length === 1 && rowsD[0].payload.action === 'warn' && rowsD[0].payload.reasonCode === 'acceptance-wording' && rowsD[0].change === 'warn-change');

		// 7e. 正常放行（归档检查全过 + ⑤无违例）→ 零 policy.decision（低噪声）
		const tE = mktmp(); tmps.push(tE);
		const gE = makeGatePi(passExec);
		require('./.sgate.cjs').default(gE.pi);
		const rE = await runGate(gE, 'openspec archive ok-change', tE);
		check('归档检查全过 + ⑤无违例 → 放行 undefined + 零 policy.decision', rE === undefined && policyRows(tE).length === 0);

		// 7e'. 检查⑤'归档并发（coordinate-concurrent-changes）：exit 2 → warn 不 block + 记账
		const tEm = mktmp(); tmps.push(tEm);
		const gEm = makeGatePi(async (cmd, args) => {
			const r = await passExec(cmd, args);
			if (cmd === 'bash' && args[0] === 'scripts/harness/concurrency-status.sh') {
				return { code: 2, stdout: '{"foreignFiles":["b.go"],"changes":{"b.go":"bar"}}', stderr: '' };
			}
			return r;
		});
		require('./.sgate.cjs').default(gEm.pi);
		const rEm = await runGate(gEm, 'openspec archive conc-change', tEm);
		const rowsEm = policyRows(tEm);
		check("检查⑤' exit 2 → warn 不 block（放行）+ custom_message 含归属文件与指引",
			rEm === undefined && gEm.messages.some((m) => m.customType === 'spec-gate-warning' && /检查⑤'/.test(m.content) && m.content.includes('b.go') && m.content.includes('concurrency-status')));
		check("检查⑤' exit 2 → policy.decision(warn, concurrent-dirty-tree) 绑定 conc-change",
			rowsEm.length === 1 && rowsEm[0].payload.action === 'warn' && rowsEm[0].payload.reasonCode === 'concurrent-dirty-tree' && rowsEm[0].change === 'conc-change');

		// exit 3 冷启动跳过 → 零输出零记账；exit 0 同形（passExec 默认）
		const tEm3 = mktmp(); tmps.push(tEm3);
		const gEm3 = makeGatePi(async (cmd, args) => {
			const r = await passExec(cmd, args);
			if (cmd === 'bash' && args[0] === 'scripts/harness/concurrency-status.sh') return { code: 3, stdout: '', stderr: '' };
			return r;
		});
		require('./.sgate.cjs').default(gEm3.pi);
		const rEm3 = await runGate(gEm3, 'openspec archive cold-change', tEm3);
		check("检查⑤' exit 3 冷启动 → 放行零输出零记账",
			rEm3 === undefined && !gEm3.messages.some((m) => /检查⑤'/.test(String(m.content))) && policyRows(tEm3).length === 0);

		// 7f. 非 bypass 归档：standards 检查须严格携带已解析的目标 change（不再扫全仓 F 段）
		const tF = mktmp(); tmps.push(tF);
		const scopeCalls = [];
		const gF = makeGatePi(async (cmd, args) => {
			if (cmd === 'bash') scopeCalls.push(args);
			return passExec(cmd, args);
		});
		require('./.sgate.cjs').default(gF.pi);
		const rF = await runGate(gF, 'openspec archive scoped-change', tF);
		check('非 bypass 归档 → check-standards 携带 --change scoped-change',
			rF === undefined && scopeCalls.some((args) => args.join('\u0000') === 'scripts/harness/check-standards.sh\u0000--change\u0000scoped-change'));

		// 7g. 目标范围 standards 自身失败仍须阻断归档。
		const tG = mktmp(); tmps.push(tG);
		const gG = makeGatePi(async (cmd, args) => {
			if (cmd === 'bash' && args[0] === 'scripts/harness/check-standards.sh') return { code: 1, stdout: '', stderr: 'target standards failure' };
			return passExec(cmd, args);
		});
		require('./.sgate.cjs').default(gG.pi);
		const rG = await runGate(gG, 'openspec archive target-fail-change', tG);
		check('目标范围 standards 失败 → 原 block 行为不变', rG && rG.block === true && /check-standards/.test(rG.reason));

		// 7h. 非归档命令 → 零介入零记账
		const tH = mktmp(); tmps.push(tH);
		const gH = makeGatePi(async () => { throw new Error('不应执行任何检查'); });
		require('./.sgate.cjs').default(gH.pi);
		const rH = await runGate(gH, 'go build ./...', tH);
		check('非归档命令 → 零开销放行（不 exec）+ 零 policy.decision', rH === undefined && policyRows(tH).length === 0);

		// ---- 8. 检查⑥' UI 验收（make-ui-design-first-class 规划制品 + add-ui-verification-trace-gate
	//     验收实物对账）：真跑真脚本 ui-verification-trace.sh + 真 fixture；lib 规划检查读真文件 ----
	const { spawnSync: spawnSyncUvt } = require('child_process');
	const REAL_UVT = path.resolve(__dirname, '../../../scripts/harness/ui-verification-trace.sh');
	/** ui-verification-trace.sh 真跑（绝对路径透传，exit code 直通），其余走 passExec */
	const uvtRealExec = (t) => async (cmd, args) => {
		if (cmd === 'bash' && args[0] === 'scripts/harness/ui-verification-trace.sh') {
			const r = spawnSyncUvt('bash', [REAL_UVT, path.resolve(t, args[1])], { encoding: 'utf8' });
			return { code: r.status, stdout: r.stdout || '', stderr: r.stderr || '' };
		}
		return passExec(cmd, args);
	};
	const writeUiChange = (cwd, name, files) => {
		const dir = path.join(cwd, 'openspec/changes', name);
		for (const [rel, content] of Object.entries(files)) {
			const p = path.join(dir, rel);
			fs.mkdirSync(path.dirname(p), { recursive: true });
			fs.writeFileSync(p, content);
		}
	};
	const MAJOR_SECTIONS = ['## User Journey', '## Information Architecture', '## Interaction Contract', '## State Matrix', '## Layout Contract', '## Component Reuse', '## Prototype', '## Acceptance'].join('\n内容\n\n');
	const majorUi = (approval) => [`<!-- ui-impact: major -->`, `<!-- ui-approval: ${approval} -->`, '<!-- ui-prototype: ui-prototype/index.html -->', '', MAJOR_SECTIONS, '与原型的差异说明：无阻断项。'].join('\n');
	const minorUi = (acc) => ['<!-- ui-impact: minor -->', '<!-- ui-approval: not-required -->', '<!-- ui-prototype: none -->', '', '## 入口', 'x', '## 状态', 'y', '## 复用', 'z', '## Acceptance', acc].join('\n');
	/** major 验收实物：report.md 一条标题式合法条目 + 点名截图在盘 */
const majorEvidence = (dir) => {
		const vd = path.join(dir, 'ui-verification');
		fs.mkdirSync(vd, { recursive: true });
		fs.writeFileSync(path.join(vd, 'ke-shot-1440.png'), 'x');
		fs.writeFileSync(path.join(vd, 'report.md'), ['### A-1 主链路【pass】', '', '打开→点击→断言到达（证据 `ke-shot-1440.png`）', ''].join('\n'));
	};
	const YAML_UI = 'schema: syntopica-ui\ncreated: 2026-09-03\n';
	// tasks 验证节两种形态：字样齐备（opencli/1440/1920）vs 零字样——新契约下都不再构成判定输入
	const TASKS_FULL_EVIDENCE = ['## 7. 测试', '## 8. 文档', '<!-- doc-impact: none -->', '', '## 9. 验证', '- opencli 主链路断言 PASS', '- 1440×900 视觉检查通过', '- 1920×1080 视觉检查通过'].join('\n');
	const TASKS_NO_EVIDENCE = ['## 7. 测试', '## 8. 文档', '<!-- doc-impact: none -->', '', '## 9. 验证', '- pnpm test:unit 全绿'].join('\n');

	// 8a. major 规划制品齐 + 验收实物齐（tasks 零字样）→ 放行零 UI 事件
	//     （字样不再是必要条件：实物在，无 opencli/1440/1920 字样也过）
	const t8a = mktmp(); tmps.push(t8a);
	writeUiChange(t8a, 'ui-major-ok', {
		'.openspec.yaml': YAML_UI,
		'proposal.md': '<!-- ui-impact: major -->\n## Why\nx',
		'ui-design.md': majorUi('approved'),
		'ui-prototype/index.html': '<html>proto</html>',
		'tasks.md': TASKS_NO_EVIDENCE,
	});
	majorEvidence(path.join(t8a, 'openspec/changes/ui-major-ok'));
	const g8a = makeGatePi(uvtRealExec(t8a));
	require('./.sgate.cjs').default(g8a.pi);
	const r8a = await runGate(g8a, 'openspec archive ui-major-ok', t8a);
	check('8a major 实物齐（tasks 零字样）→ 放行 + 零 policy.decision', r8a === undefined && policyRows(t8a).length === 0);

	// 8b. major 字样齐备（opencli/1440/1920 全写上）但无实物 → 阻断（脚本结论，字样不再放行）
	const t8b = mktmp(); tmps.push(t8b);
	writeUiChange(t8b, 'ui-major-gap', {
		'.openspec.yaml': YAML_UI,
		'proposal.md': '<!-- ui-impact: major -->\n## Why\nx',
		'ui-design.md': majorUi('approved'),
		'ui-prototype/index.html': '<html>proto</html>',
		'tasks.md': TASKS_FULL_EVIDENCE,
	});
	const g8b = makeGatePi(uvtRealExec(t8b));
	require('./.sgate.cjs').default(g8b.pi);
	const r8b = await runGate(g8b, 'openspec archive ui-major-gap', t8b);
	const rows8b = policyRows(t8b);
	check('8b 字样齐备无实物 → block + reason 透传脚本零痕迹结论（不再因字样放行）',
		r8b && r8b.block === true && /UI 验收/.test(r8b.reason) && /零痕迹/.test(r8b.reason) && /显式降档/.test(r8b.reason));
	check('8b 缺实物 → policy=ui-design-gate block ui-verification-missing + 聚合 spec-gate block 各一条',
		rows8b.length === 2 && rows8b.some((r) => r.payload.policy === 'ui-design-gate' && r.payload.action === 'block' && r.payload.reasonCode === 'ui-verification-missing') && rows8b.some((r) => r.payload.policy === 'spec-gate' && r.payload.reasonCode === 'archive-check-failed'));

	// 8c. major 审批仍 pending（实物齐）→ 阻断（lib 规划制品检查保留，不相信孤立证据）
	const t8c = mktmp(); tmps.push(t8c);
	writeUiChange(t8c, 'ui-major-pending', {
		'.openspec.yaml': YAML_UI,
		'proposal.md': '<!-- ui-impact: major -->\n## Why\nx',
		'ui-design.md': majorUi('pending'),
		'ui-prototype/index.html': '<html>proto</html>',
		'tasks.md': TASKS_NO_EVIDENCE,
	});
	majorEvidence(path.join(t8c, 'openspec/changes/ui-major-pending'));
	const g8c = makeGatePi(uvtRealExec(t8c));
	require('./.sgate.cjs').default(g8c.pi);
	const r8c = await runGate(g8c, 'openspec archive ui-major-pending', t8c);
	check('8c major pending → block 且 reason 含 approved 要求（脚本绿、lib 红）', r8c && r8c.block === true && /approved/.test(r8c.reason) && policyRows(t8c).some((r) => r.payload.reasonCode === 'ui-verification-missing'));

	// 8d. minor 指名道姓（agent-browser/人工 → report.md 建账）→ 放行
	const t8d = mktmp(); tmps.push(t8d);
	writeUiChange(t8d, 'ui-minor-ok', {
		'.openspec.yaml': YAML_UI,
		'proposal.md': '<!-- ui-impact: minor -->\n## Why\nx',
		'ui-design.md': minorUi('agent-browser 走主链路 + 人工双主题目检。'),
		'tasks.md': TASKS_NO_EVIDENCE,
	});
	writeUiChange(t8d, 'ui-minor-ok', { 'ui-verification/report.md': '| A1 弹窗开合 | **pass** | agent-browser 断言开合与焦点 |' });
	const g8d = makeGatePi(uvtRealExec(t8d));
	require('./.sgate.cjs').default(g8d.pi);
	const r8d = await runGate(g8d, 'openspec archive ui-minor-ok', t8d);
	check('8d minor agent-browser 建账在案 → 放行零 UI 事件', r8d === undefined && policyRows(t8d).length === 0);

	// 8d2. minor 组件测试映射但路径不存在 → 阻断（指名道姓的牙齿：路径必须真实在盘）
	const t8d2 = mktmp(); tmps.push(t8d2);
	writeUiChange(t8d2, 'ui-minor-badpath', {
		'.openspec.yaml': YAML_UI,
		'proposal.md': '<!-- ui-impact: minor -->\n## Why\nx',
		'ui-design.md': minorUi('组件测试 front/app/components/NoSuchTest.test.ts 覆盖。'),
		'tasks.md': TASKS_NO_EVIDENCE,
	});
	const g8d2 = makeGatePi(uvtRealExec(t8d2));
	require('./.sgate.cjs').default(g8d2.pi);
	const r8d2 = await runGate(g8d2, 'openspec archive ui-minor-badpath', t8d2);
	check('8d2 minor 测试路径不存在 → block + reason 含脚本结论（ui-verification-missing）',
		r8d2 && r8d2.block === true && /NoSuchTest\.test\.ts/.test(r8d2.reason) && policyRows(t8d2).some((r) => r.payload.policy === 'ui-design-gate' && r.payload.reasonCode === 'ui-verification-missing'));

	// 8e. minor 无验收映射（裸文本）→ 阻断
	const t8e = mktmp(); tmps.push(t8e);
	writeUiChange(t8e, 'ui-minor-gap', {
		'.openspec.yaml': YAML_UI,
		'proposal.md': '<!-- ui-impact: minor -->\n## Why\nx',
		'ui-design.md': minorUi('待定。'),
		'tasks.md': TASKS_NO_EVIDENCE,
	});
	const g8e = makeGatePi(uvtRealExec(t8e));
	require('./.sgate.cjs').default(g8e.pi);
	const r8e = await runGate(g8e, 'openspec archive ui-minor-gap', t8e);
	check('8e minor 无验收映射 → block + ui-verification-missing', r8e && r8e.block === true && /指名道姓|验收映射/.test(r8e.reason) && policyRows(t8e).some((r) => r.payload.policy === 'ui-design-gate' && r.payload.reasonCode === 'ui-verification-missing'));

	// 8f. none N/A 一致 → 放行；声明与制品不一致 → 阻断
	const t8f = mktmp(); tmps.push(t8f);
	writeUiChange(t8f, 'ui-none-ok', {
		'.openspec.yaml': YAML_UI,
		'proposal.md': '<!-- ui-impact: none -->\n## Why\nx',
		'ui-design.md': '<!-- ui-impact: none -->\n<!-- ui-approval: not-required -->\n<!-- ui-prototype: none -->\n\n## N/A Reason\n纯后端。',
		'tasks.md': TASKS_NO_EVIDENCE,
	});
	const g8f = makeGatePi(uvtRealExec(t8f));
	require('./.sgate.cjs').default(g8f.pi);
	const r8f = await runGate(g8f, 'openspec archive ui-none-ok', t8f);
	check('8f none N/A 一致 → 放行零 UI 事件', r8f === undefined && policyRows(t8f).length === 0);

	// 8g. legacy schema → UI 检查零介入（脚本 fail-open + lib legacy 空，不硬阻断）
	const t8g = mktmp(); tmps.push(t8g);
	writeUiChange(t8g, 'legacy-chg', { '.openspec.yaml': 'schema: spec-driven\ncreated: 2026-01-01\n', 'proposal.md': '## Why\nx' });
	const g8g = makeGatePi(uvtRealExec(t8g));
	require('./.sgate.cjs').default(g8g.pi);
	const r8g = await runGate(g8g, 'openspec archive legacy-chg', t8g);
	check('8g legacy schema → UI 证据零检查零事件（旧 change 不卡死）', r8g === undefined && policyRows(t8g).length === 0);

	// 8g2. 归档侧放行锚点（add-change-lifecycle-retro D2）：先 block(ui-verification-missing)，
	//      补齐实物（report.md 建账+截图）后归档检查全过 → 放行时刻记 pass(ui-design-satisfied)
	//      恰一条；再归档一次（已解除）零新增。首次归档零记录（8a 已覆盖）。
	const t8g2 = mktmp(); tmps.push(t8g2);
	writeUiChange(t8g2, 'ui-evidence-retro', {
		'.openspec.yaml': YAML_UI,
		'proposal.md': '<!-- ui-impact: major -->\n## Why\nx',
		'ui-design.md': majorUi('approved'),
		'ui-prototype/index.html': '<html>proto</html>',
		'tasks.md': TASKS_NO_EVIDENCE,
	});
	const g8g2a = makeGatePi(uvtRealExec(t8g2));
	require('./.sgate.cjs').default(g8g2a.pi);
	const r8g2a = await runGate(g8g2a, 'openspec archive ui-evidence-retro', t8g2);
	check('8g2a 实物缺失 → block（ui-verification-missing）', r8g2a && r8g2a.block === true && policyRows(t8g2).some((r) => r.payload.policy === 'ui-design-gate' && r.payload.reasonCode === 'ui-verification-missing'));
	majorEvidence(path.join(t8g2, 'openspec/changes/ui-evidence-retro'));
	const g8g2b = makeGatePi(uvtRealExec(t8g2));
	require('./.sgate.cjs').default(g8g2b.pi);
	const r8g2b = await runGate(g8g2b, 'openspec archive ui-evidence-retro', t8g2);
	check('8g2b 补齐实物后归档放行 → pass ui-design-satisfied 恰一条（绑定 change，不与 block 双写）', (() => {
		const rows = policyRows(t8g2).filter((r) => r.payload.policy === 'ui-design-gate');
		return r8g2b === undefined && rows.length === 2 && rows[1].payload.action === 'pass' && rows[1].payload.reasonCode === 'ui-design-satisfied' && rows[1].change === 'ui-evidence-retro';
	})());
	const g8g2c = makeGatePi(uvtRealExec(t8g2));
	require('./.sgate.cjs').default(g8g2c.pi);
	const r8g2c = await runGate(g8g2c, 'openspec archive ui-evidence-retro', t8g2);
	check('8g2c 已解除后再归档零新增 pass', r8g2c === undefined && policyRows(t8g2).filter((r) => r.payload.action === 'pass').length === 1);

	// 8h. --force 仍豁免 UI 检查（逃生口留痕不变）
	const t8h = mktmp(); tmps.push(t8h);
	writeUiChange(t8h, 'ui-major-force', {
		'.openspec.yaml': YAML_UI,
		'proposal.md': '<!-- ui-impact: major -->\n## Why\nx',
		'ui-design.md': majorUi('approved'),
		'ui-prototype/index.html': '<html>proto</html>',
		'tasks.md': TASKS_NO_EVIDENCE,
	});
	const g8h = makeGatePi(uvtRealExec(t8h));
	require('./.sgate.cjs').default(g8h.pi);
	const r8h = await runGate(g8h, 'openspec archive ui-major-force --force', t8h);
	check('8h --force → UI 检查同被豁免 + warning 留痕含 UI 项', r8h === undefined && g8h.messages.some((m) => /UI 验收/.test(m.content)));

	// 8i. spawn 失败（脚本不可执行）→ 检查⑥' fail-open 放行该项，不记 ui-verification-missing
	const t8i = mktmp(); tmps.push(t8i);
	writeUiChange(t8i, 'ui-spawn-fail', {
		'.openspec.yaml': YAML_UI,
		'proposal.md': '<!-- ui-impact: major -->\n## Why\nx',
		'ui-design.md': majorUi('approved'),
		'ui-prototype/index.html': '<html>proto</html>',
		'tasks.md': TASKS_NO_EVIDENCE,
	});
	const g8i = makeGatePi(async (cmd, args) => {
		if (cmd === 'bash' && args[0] === 'scripts/harness/ui-verification-trace.sh') {
			throw new Error('bash 不可用（模拟 spawn 失败）');
		}
		return passExec(cmd, args);
	});
	require('./.sgate.cjs').default(g8i.pi);
	const r8i = await runGate(g8i, 'openspec archive ui-spawn-fail', t8i);
	check("8i 检查⑥' spawn 失败 → fail-open 放行 + 零 ui-verification-missing 记账（不误伤）",
		r8i === undefined && !policyRows(t8i).some((r) => r.payload.reasonCode === 'ui-verification-missing'));

		// ---- 7i/7j. 检查⑤ review-trace 痕迹对账（add-review-trace-gate）：真跑真脚本 + 真 fixture ----
		const { spawnSync } = require('child_process');
		const REAL_RT = path.resolve(__dirname, '../../../scripts/harness/review-trace.sh');
		const rtChangeDir = (t, name) => {
			const dir = path.join(t, 'openspec', 'changes', name);
			fs.mkdirSync(dir, { recursive: true });
			return dir;
		};
		/** review-trace.sh 真跑（绝对路径透传），其余走 passExec */
		const rtRealExec = (t) => async (cmd, args) => {
			if (cmd === 'bash' && args[0] === 'scripts/harness/review-trace.sh') {
				const r = spawnSync('bash', [REAL_RT, path.resolve(t, args[1])], { encoding: 'utf8' });
				return { code: r.status, stdout: r.stdout || '', stderr: r.stderr || '' };
			}
			return passExec(cmd, args);
		};

		// 7i. 零痕迹 fixture（无 review.md 无 waived）→ block，reason 含两条修复路径
		const tI = mktmp(); tmps.push(tI);
		const dirI = rtChangeDir(tI, 'rt-zero-chg');
		fs.writeFileSync(path.join(dirI, 'proposal.md'), '## Why\nx\n');
		const gI = makeGatePi(rtRealExec(tI));
		require('./.sgate.cjs').default(gI.pi);
		const rI = await runGate(gI, 'openspec archive rt-zero-chg', tI);
		const rowsI = policyRows(tI);
		check('7i review-trace 零痕迹 → block + reason 含零痕迹与两条修复路径（真跑真脚本）',
			rI && rI.block === true && /\[review-trace\]/.test(rI.reason) && /零痕迹/.test(rI.reason) && /修复路径（二选一）/.test(rI.reason));
		check('7i review-trace 失败 → target 只含 review（其余检查绿）',
			rowsI.length === 1 && rowsI[0].payload.target === 'review' && rowsI[0].payload.reasonCode === 'archive-check-failed');

		// 7j. waived fixture（proposal 头部声明）→ 真跑通过 → 放行零记账
		const tJ = mktmp(); tmps.push(tJ);
		const dirJ = rtChangeDir(tJ, 'rt-waived-chg');
		fs.writeFileSync(path.join(dirJ, 'proposal.md'), '<!-- review: waived -->\n\n## Why\nx\n');
		const gJ = makeGatePi(rtRealExec(tJ));
		require('./.sgate.cjs').default(gJ.pi);
		const rJ = await runGate(gJ, 'openspec archive rt-waived-chg', tJ);
		check('7j review-trace waived 声明 → 归档检查全过放行 + 零 policy.decision（真跑真脚本）',
			rJ === undefined && policyRows(tJ).length === 0);

		// 7k. 归档侧约束文档读取对账（stage-driven-constraint-injection D6 / spec
		// 「约束文档读取记账与归档对账」）：对账报红 → 归档被 block，reason 含未读域清单与取数指引
		const tK = mktmp(); tmps.push(tK);
		const crtRed = [
			'✗ 未通过：1/1 个声明域的约束文档未在该 change 生命周期窗内被读过：',
			'  - 域 scheduler：未读到 docs/reference/flow/scheduler.md',
			'    取数指引：bash scripts/harness/constraint-digest.sh --brief --domains scheduler',
			'  未读域清单：scheduler',
		].join('\n');
		const gK = makeGatePi(async (cmd, args) => {
			const r = await passExec(cmd, args);
			if (cmd === 'bash' && args[0] === 'scripts/harness/constraint-read-trace.sh') {
				return { code: 1, stdout: crtRed, stderr: '' };
			}
			return r;
		});
		require('./.sgate.cjs').default(gK.pi);
		const rK = await runGate(gK, 'openspec archive crt-red-chg', tK);
		const rowsK = policyRows(tK);
		check('7k 约束读取对账红 → block + reason 含未读域清单与取数指引',
			rK && rK.block === true
				&& /\[constraint-read-trace\]/.test(rK.reason)
				&& /scheduler/.test(rK.reason)
				&& /constraint-digest\.sh --brief --domains scheduler/.test(rK.reason));
		check('7k 约束读取对账红 → policy.decision target 只含 constraint-read',
			rowsK.length === 1 && rowsK[0].payload.target === 'constraint-read'
				&& rowsK[0].payload.reasonCode === 'archive-check-failed');

		// 7k'. 对账脚本缺席/不可执行（127）→ fail-open 放行该项（不阻断归档，脚本不在不误伤）
		const tK2 = mktmp(); tmps.push(tK2);
		const gK2 = makeGatePi(async (cmd, args) => {
			const r = await passExec(cmd, args);
			if (cmd === 'bash' && args[0] === 'scripts/harness/constraint-read-trace.sh') {
				return { code: 127, stdout: '', stderr: 'bash: scripts/harness/constraint-read-trace.sh: No such file or directory' };
			}
			return r;
		});
		require('./.sgate.cjs').default(gK2.pi);
		const rK2 = await runGate(gK2, 'openspec archive crt-missing-chg', tK2);
		check('7k\' 对账脚本缺席（exit 127）→ fail-open 放行该项（不 block + 零记账）',
			rK2 === undefined && policyRows(tK2).length === 0);

		// 7L. 复杂档 + 有 delta Scenario 但验证节无映射表 → 归档门禁放行
		//     （streamline-business-test-verification：④ 场景映射对账退役 + ⑤a 白盒缺件提醒退役）
		const tL = mktmp(); tmps.push(tL);
		const dirL = path.join(tL, 'openspec', 'changes', 'complex-no-map');
		fs.mkdirSync(path.join(dirL, 'specs', 'demo'), { recursive: true });
		fs.writeFileSync(path.join(dirL, 'proposal.md'), '<!-- complexity: complex -->\n\n## Why\nx\n');
		fs.writeFileSync(path.join(dirL, 'specs', 'demo', 'spec.md'), [
			'## ADDED Requirements',
			'',
			'### Requirement: 某能力',
			'',
			'#### Scenario: 主路径可用',
			'',
			'- **WHEN** 用户走主路径',
			'- **THEN** 结果正确',
			'',
		].join('\n'));
		const tasksL = [
			'## 1. 任务',
			'- [ ] 1.1 实现状态机迁移与协议解析（复杂档，关键词齐全）',
			'',
			'## 2. 测试',
			'- go test ./internal/demo → PASS',
			'',
			'## 3. 文档',
			'<!-- doc-impact: none -->',
			'',
			'## 4. 验证',
			'- go test ./internal/demo -run TestMain → PASS（主路径实测）',
		].join('\n');
		fs.writeFileSync(path.join(dirL, 'tasks.md'), tasksL);
		const bashCallsL = [];
		const gL = makeGatePi(async (cmd, args) => {
			if (cmd === 'bash') bashCallsL.push(args.join(' '));
			// 场景映射对账若仍被调用则让脚本红——退役后不应触发
			if (cmd === 'bash' && args[0] === 'scripts/harness/scenario-trace.sh') {
				return { code: 1, stdout: '✗ 未通过：验证节内未找到映射表', stderr: '' };
			}
			if (cmd === 'cat' && args.join(' ').includes('tasks.md')) return { code: 0, stdout: tasksL, stderr: '' };
			return passExec(cmd, args);
		});
		require('./.sgate.cjs').default(gL.pi);
		const rL = await runGate(gL, 'openspec archive complex-no-map', tL);
		check('7L 复杂档 + 验证节无映射表 → 归档放行（不因缺映射表阻断）', rL === undefined);
		check('7L 归档门禁不再调用 scenario-trace.sh（④ 场景映射对账退役）',
			!bashCallsL.some((c) => c.includes('scenario-trace.sh')));
		check('7L 复杂档缺 test-cases.md 不再催补白盒文档（⑤a 退役，零 warning）',
			!gL.messages.some((m) => /白盒|test-cases|⑤a/.test(String(m.content))));
		check('7L 放行零 policy.decision（低噪声）', policyRows(tL).length === 0);

		// ---- 9. 归档 warn 边沿触发（edge-trigger-archive-gate-warns）：同会话多次 tool_call，
		//     指纹态 per-sessionId（模块级 Map，跨用例存活——新用例 change 名已避让既有用例）----
		// 9-Ⅰ（A1→A2→A3）：首见投递 → 同指纹静默 → 清单变化重投
		const t9a = mktmp(); tmps.push(t9a);
		let concOut9a = { code: 2, stdout: 'backend-go/b.go', stderr: '' };
		const g9a = makeGatePi(async (cmd, args) => {
			const r = await passExec(cmd, args);
			if (cmd === 'bash' && args[0] === 'scripts/harness/concurrency-status.sh') return concOut9a;
			return r;
		});
		require('./.sgate.cjs').default(g9a.pi);
		await runGate(g9a, 'openspec archive edge-conc', t9a, 'esgs-a');
		const m9a1 = g9a.messages.length, r9a1 = policyRows(t9a).length;
		const r9a2 = await runGate(g9a, 'openspec archive edge-conc', t9a, 'esgs-a');
		const rows9a = policyRows(t9a);
		check("9-Ⅰ⑤' 首见尝试 → 投递 warn + 记账 concurrent-dirty-tree 恰 1 条（锚点）",
			m9a1 === 1 && r9a1 === 1 && rows9a[0].payload.reasonCode === 'concurrent-dirty-tree');
		check("9-Ⅰ⑤' 同指纹第二次尝试 → 零 sendMessage 零 warn 记账（现行全量重发 → 红）",
			r9a2 === undefined && g9a.messages.length === m9a1 && rows9a.length === r9a1);

		// 9-Ⅰb（A3）：清单变化第二次重新投递并再记账（防回归锚点：现行绿，与同指纹用例解耦）
		const t9a2 = mktmp(); tmps.push(t9a2);
		let concOut9a2 = { code: 2, stdout: 'backend-go/b.go', stderr: '' };
		const g9a2 = makeGatePi(async (cmd, args) => {
			const r = await passExec(cmd, args);
			if (cmd === 'bash' && args[0] === 'scripts/harness/concurrency-status.sh') return concOut9a2;
			return r;
		});
		require('./.sgate.cjs').default(g9a2.pi);
		await runGate(g9a2, 'openspec archive edge-conc', t9a2, 'esgs-a2');
		const m9a2 = g9a2.messages.length, r9a2b = policyRows(t9a2).length;
		concOut9a2 = { code: 2, stdout: 'backend-go/b.go\nbackend-go/c.go', stderr: '' };
		const r9a2c = await runGate(g9a2, 'openspec archive edge-conc', t9a2, 'esgs-a2');
		check("9-Ⅰb⑤' 清单变化第二次尝试 → 重新投递完整 warn 并再记账（防回归锚点）",
			r9a2c === undefined && g9a2.messages.length === m9a2 + 1 && policyRows(t9a2).length === r9a2b + 1);

		// 9-Ⅱ（A4→A5）：转净收尾一行 → 再犯回首见
		const t9b = mktmp(); tmps.push(t9b);
		let concOut9b = { code: 2, stdout: 'backend-go/b.go', stderr: '' };
		const g9b = makeGatePi(async (cmd, args) => {
			const r = await passExec(cmd, args);
			if (cmd === 'bash' && args[0] === 'scripts/harness/concurrency-status.sh') return concOut9b;
			return r;
		});
		require('./.sgate.cjs').default(g9b.pi);
		await runGate(g9b, 'openspec archive edge-conc', t9b, 'esgs-b');
		concOut9b = { code: 0, stdout: '', stderr: '' };
		const m9b1 = g9b.messages.length, r9b1 = policyRows(t9b).length;
		const r9b2 = await runGate(g9b, 'openspec archive edge-conc', t9b, 'esgs-b');
		check("9-Ⅱ⑤' 转净（exit 0）→ 单行「已转净」收尾投递 + 零记账（现行无收尾 → 红）",
			r9b2 === undefined && g9b.messages.length === m9b1 + 1 && /已转净/.test(g9b.messages[m9b1].content) && policyRows(t9b).length === r9b1);
		concOut9b = { code: 2, stdout: 'backend-go/b.go', stderr: '' };
		const r9b3 = await runGate(g9b, 'openspec archive edge-conc', t9b, 'esgs-b');
		check("9-Ⅱ⑤' 转净后再犯 → 条目已删回首见：投递 + 记账",
			r9b3 === undefined && g9b.messages.length === m9b1 + 2 && policyRows(t9b).length === r9b1 + 1);

		// 9-Ⅲ：exit 3 冷启动两次 → 零输出零记账不触碰状态（锚点：现行绿）
		const t9c = mktmp(); tmps.push(t9c);
		const g9c = makeGatePi(async (cmd, args) => {
			const r = await passExec(cmd, args);
			if (cmd === 'bash' && args[0] === 'scripts/harness/concurrency-status.sh') return { code: 3, stdout: '', stderr: '' };
			return r;
		});
		require('./.sgate.cjs').default(g9c.pi);
		await runGate(g9c, 'openspec archive edge-cold', t9c, 'esgs-c');
		await runGate(g9c, 'openspec archive edge-cold', t9c, 'esgs-c');
		check("9-Ⅲ⑤' exit 3 冷启动重复尝试 → 零投递零记账（锚点）",
			g9c.messages.length === 0 && policyRows(t9c).length === 0);

		// 9-Ⅳ（A6）：新 sessionId 首见重投（锚点：现行绿——每尝试都投）
		const t9d = mktmp(); tmps.push(t9d);
		const g9d = makeGatePi(async (cmd, args) => {
			const r = await passExec(cmd, args);
			if (cmd === 'bash' && args[0] === 'scripts/harness/concurrency-status.sh') return { code: 2, stdout: 'backend-go/b.go', stderr: '' };
			return r;
		});
		require('./.sgate.cjs').default(g9d.pi);
		await runGate(g9d, 'openspec archive edge-conc', t9d, 'esgs-d1');
		const m9d1 = g9d.messages.length, r9d1 = policyRows(t9d).length;
		const r9d2 = await runGate(g9d, 'openspec archive edge-conc', t9d, 'esgs-d2');
		check("9-Ⅳ⑤' 新 sessionId 首见 → 重新投递 + 记账（会话边界清零，锚点）",
			r9d2 === undefined && g9d.messages.length === m9d1 + 1 && policyRows(t9d).length === r9d1 + 1);

		// 9-Ⅴ（A7）：同会话同指纹静默 → session_compact 清条目 → 同指纹重投一次
		const t9e = mktmp(); tmps.push(t9e);
		const g9e = makeGatePi(async (cmd, args) => {
			const r = await passExec(cmd, args);
			if (cmd === 'bash' && args[0] === 'scripts/harness/concurrency-status.sh') return { code: 2, stdout: 'backend-go/b.go', stderr: '' };
			return r;
		});
		require('./.sgate.cjs').default(g9e.pi);
		const compact9e = g9e.handlers['session_compact'];
		await runGate(g9e, 'openspec archive edge-conc', t9e, 'esgs-e');
		const m9e1 = g9e.messages.length;
		await runGate(g9e, 'openspec archive edge-conc', t9e, 'esgs-e'); // 同指纹静默
		const silentCount = g9e.messages.length;
		if (typeof compact9e === 'function') await compact9e({}, { cwd: t9e, sessionManager: { getSessionId: () => 'esgs-e' } });
		const r9e3 = await runGate(g9e, 'openspec archive edge-conc', t9e, 'esgs-e');
		check("9-Ⅴ⑤' 同指纹静默（前置）→ compact 后同指纹重投一次（现行无 compact handler → 红）",
			typeof compact9e === 'function' && silentCount === m9e1 && r9e3 === undefined && g9e.messages.length === m9e1 + 1);

		// 9-Ⅵ：空 sessionId 兑底槽：无 getSessionId → 首见投递，同指纹再试静默
		const t9f = mktmp(); tmps.push(t9f);
		const g9f = makeGatePi(async (cmd, args) => {
			const r = await passExec(cmd, args);
			if (cmd === 'bash' && args[0] === 'scripts/harness/concurrency-status.sh') return { code: 2, stdout: 'backend-go/b.go', stderr: '' };
			return r;
		});
		require('./.sgate.cjs').default(g9f.pi);
		await runGate(g9f, 'openspec archive edge-conc', t9f, null);
		const m9f1 = g9f.messages.length;
		await runGate(g9f, 'openspec archive edge-conc', t9f, null);
		check('9-Ⅵ 空 sessionId → 兑底槽首见投递 + 同指纹静默（投递层面，记账本就不记）',
			m9f1 === 1 && g9f.messages.length === m9f1);

		// 9-Ⅶ：同会话跨 change 键隔离：edge-concX warn 后，同指纹输入对 edge-concY 仍首见
		const t9g = mktmp(); tmps.push(t9g);
		const g9g = makeGatePi(async (cmd, args) => {
			const r = await passExec(cmd, args);
			if (cmd === 'bash' && args[0] === 'scripts/harness/concurrency-status.sh') return { code: 2, stdout: 'backend-go/b.go', stderr: '' };
			return r;
		});
		require('./.sgate.cjs').default(g9g.pi);
		await runGate(g9g, 'openspec archive edge-conc-x', t9g, 'esgs-g');
		const m9g1 = g9g.messages.length;
		const r9g2 = await runGate(g9g, 'openspec archive edge-conc-y', t9g, 'esgs-g');
		check('9-Ⅶ 同会话跨 change 键隔离 → 换 change 仍首见投递',
			r9g2 === undefined && g9g.messages.length === m9g1 + 1);

		// 9-Ⅷ（W1→W2）：⑤ 首见投递+记账 → 同违例集合第二次零投递零记账
		const t9h = mktmp(); tmps.push(t9h);
		const g9h = makeGatePi(async (cmd, args) => {
			const r = await passExec(cmd, args);
			if (cmd === 'cat' && args.join(' ').includes('tasks.md')) return { code: 0, stdout: ['## 1. 任务', '- [ ] 1.1 给排序写纯函数单测：SQLite 内存库建表 → PASS', '', '## 2. 测试', '## 3. 文档', '<!-- doc-impact: 无 -->', '', '## 4. 验证'].join('\n'), stderr: '' };
			return r;
		});
		require('./.sgate.cjs').default(g9h.pi);
		await runGate(g9h, 'openspec archive edge-word', t9h, 'esgs-h');
		const m9h1 = g9h.messages.length, r9h1 = policyRows(t9h).length;
		const r9h2 = await runGate(g9h, 'openspec archive edge-word', t9h, 'esgs-h');
		const rows9h = policyRows(t9h);
		check('9-Ⅷ⑤ 首见违例 → 投递 + acceptance-wording 记账恰 1 条（锚点）',
			m9h1 === 1 && r9h1 === 1 && rows9h[0].payload.reasonCode === 'acceptance-wording');
		check('9-Ⅷ⑤ 同指纹重试 → 零重复 warning 零记账（现行全量重发 → 红）',
			r9h2 === undefined && g9h.messages.length === m9h1 && rows9h.length === r9h1);

		// 9-Ⅸ（W3）：违例集合变化（tasks 再加一行分层错配）→ 重新投递再记账（锚点）
		const t9i = mktmp(); tmps.push(t9i);
		let tasks9i = ['## 1. 任务', '- [ ] 1.1 给排序写纯函数单测：SQLite 内存库建表 → PASS', '', '## 2. 测试', '## 3. 文档', '<!-- doc-impact: 无 -->', '', '## 4. 验证'].join('\n');
		const g9i = makeGatePi(async (cmd, args) => {
			const r = await passExec(cmd, args);
			if (cmd === 'cat' && args.join(' ').includes('tasks.md')) return { code: 0, stdout: tasks9i, stderr: '' };
			return r;
		});
		require('./.sgate.cjs').default(g9i.pi);
		await runGate(g9i, 'openspec archive edge-word', t9i, 'esgs-i');
		const m9i1 = g9i.messages.length;
		tasks9i = ['## 1. 任务', '- [ ] 1.1 纯函数走 SQLite', '', '## 2. 测试', '## 3. 文档', '<!-- doc-impact: 无 -->', '', '## 4. 验证'].join('\n');
		const r9i2 = await runGate(g9i, 'openspec archive edge-word', t9i, 'esgs-i');
		check('9-Ⅸ⑤ 违例集合变化 → 重新投递 + 再记账（锚点）',
			r9i2 === undefined && g9i.messages.length === m9i1 + 1 && policyRows(t9i).length === 2);

		// 9-Ⅹ（W4）：违例清零（措辞已修）→ 单行「已清零」收尾 + 零记账
		const t9j = mktmp(); tmps.push(t9j);
		let tasks9j = ['## 1. 任务', '- [ ] 1.1 给排序写纯函数单测：SQLite 内存库建表 → PASS', '', '## 2. 测试', '## 3. 文档', '<!-- doc-impact: 无 -->', '', '## 4. 验证'].join('\n');
		const g9j = makeGatePi(async (cmd, args) => {
			const r = await passExec(cmd, args);
			if (cmd === 'cat' && args.join(' ').includes('tasks.md')) return { code: 0, stdout: tasks9j, stderr: '' };
			return r;
		});
		require('./.sgate.cjs').default(g9j.pi);
		await runGate(g9j, 'openspec archive edge-word', t9j, 'esgs-j');
		const m9j1 = g9j.messages.length;
		tasks9j = ['## 1. 任务', '- [ ] 1.1 给排序写纯函数单测（无 DB）', '', '## 2. 测试', '## 3. 文档', '<!-- doc-impact: 无 -->', '', '## 4. 验证'].join('\n'); // 措辞已修 → ⑤b 消失 → 清零
		const r9j2 = await runGate(g9j, 'openspec archive edge-word', t9j, 'esgs-j');
		check('9-Ⅹ⑤ 违例清零 → 单行「已清零」收尾投递 + 零记账（现行无收尾 → 红）',
			r9j2 === undefined && g9j.messages.length === m9j1 + 1 && /已清零/.test(g9j.messages[m9j1].content) && policyRows(t9j).length === 1);

		// ---- 10. 边沿判定纯函数 decideWarnEdge 三态矩阵（1.4，无 DB 依赖；现行无此函数 → 红）----
		const sg = require('./.sgate.cjs');
		const hasEdge = typeof sg.decideWarnEdge === 'function';
		const edge = (p, c) => (hasEdge ? sg.decideWarnEdge(p, c) : undefined);
		check('10 矩阵：首见（prev=null,curr=fp）→ deliver', hasEdge && edge(null, 'fp1') === 'deliver');
		check('10 矩阵：同指纹（fp1,fp1）→ silent', hasEdge && edge('fp1', 'fp1') === 'silent');
		check('10 矩阵：指纹变化（fp1,fp2）→ deliver', hasEdge && edge('fp1', 'fp2') === 'deliver');
		check('10 矩阵：转绿（fp1,null）→ close', hasEdge && edge('fp1', null) === 'close');
		check('10 矩阵：防御（null,null）→ silent（不冒空收尾）', hasEdge && edge(null, null) === 'silent');
		check('10 再犯闭环：close 后 prev 已删（null）→ 再犯回到 deliver', hasEdge && edge(null, 'fp1') === 'deliver');

		// ---- 11. 指纹取值稳定性（design D2；现行无此函数 → 红）----
		const hasFp = typeof sg.concurrencyFingerprint === 'function' && typeof sg.wordingFingerprint === 'function';
		const cfp = (s) => (typeof sg.concurrencyFingerprint === 'function' ? sg.concurrencyFingerprint(s) : undefined);
		check('11 空/纯空白/空行混入清单 → 与空串同指纹，不抛异常',
			hasFp && cfp('') === cfp('') && cfp('') === cfp('\n  \n\n') && typeof cfp('') === 'string');
		check('11 重复行/顺序颠倒/行首尾空白 → 同指纹（排序去重+trim）',
			hasFp && cfp('b.go') === cfp('b.go\nb.go') && cfp('a.go\nb.go') === cfp('b.go\na.go') && cfp(' b.go ') === cfp('b.go'));
		check('11 不同清单 → 不同指纹；超长清单不抛异常且仍 32 hex',
			hasFp && cfp('a.go') !== cfp('a.go\nc.go') && /^[0-9a-f]{32}$/.test(cfp(Array.from({ length: 1000 }, (_, i) => `f${i}.go`).join('\n'))));
		check('11 ⑤ 文案列表 join 不排序：顺序不同 → 指纹不同（保留新增关键词排前语义，design D2）',
			hasFp && sg.wordingFingerprint(['w1', 'w2']) !== sg.wordingFingerprint(['w2', 'w1']) && sg.wordingFingerprint(['w1']) !== sg.wordingFingerprint(['w1', 'w2']));
	} finally {
		for (const t of tmps) fs.rmSync(t, { recursive: true, force: true });
	}

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
}
})();
