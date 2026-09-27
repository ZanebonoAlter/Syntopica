// spec-gate smoke：检查⑤验收措辞扫描纯函数——无违例静默 / ⑤a 关键词命中与文档检出抑制 /
// ⑤a 按关键词去重 / ⑤b 纯函数×SQLite 分层错配 / 健壮性（空串/超长行/纯标签行/畸形输入）。
// + 归档门禁 policy.decision 记账（harden-harness-policy-and-spill）：block / bypass / 检查⑤ warn /
// 正常放行零记录，且原 block/放行/custom_message 行为不变（mock pi.exec 驱动完整 tool_call）。
// 由 run-harness-smoke.sh 调用（先 esbuild 产出 .sgate.cjs）。断言失败 exit 1。
const fs = require('fs');
const os = require('os');
const path = require('path');
const { execSync } = require('child_process');
const { scanAcceptanceWording, COMPLEXITY_KEYWORDS } = require('./.sgate.cjs');

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
/** 默认 exec：四项检查全过（bash→code 0；cat→合法 tasks/proposal；ls→文件列表） */
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
	// ---- 1. 无违例输入（干净 tasks.md + 任意文件列表）→ 空数组 ----
	const cleanTasks = [
		'## 1. 任务',
		'- [ ] 1.1 写 handler 测试：验收 `go test ./internal/x -run TestY → PASS`',
		'- [x] 1.2 补文档：新增 docs/foo.md',
		'非任务行含解析算法协议状态机——不锚定任务行，不应命中',
	].join('\n');
	const none = scanAcceptanceWording(cleanTasks, ['proposal.md', 'design.md', 'tasks.md']);
	check('干净 tasks.md + 无关文件列表 → 空数组', Array.isArray(none) && none.length === 0);

	// ---- 2. ⑤a 正样本：任务行含「解析」+ 文件列表无 test-cases* → 1 条且文案含「可忽略」 ----
	const a1 = scanAcceptanceWording('- [ ] 2.1 实现正文解析逻辑 extractContent', ['tasks.md', 'proposal.md']);
	check('⑤a 正样本：解析关键词 + 无 test-cases* → 1 条', a1.length === 1);
	check('⑤a 文案含「可忽略」修复指引', a1[0].includes('可忽略'));
	check('⑤a 文案含白盒用例义务提示', a1[0].includes('case-first-testing'));

	// ⑤a 按关键词去重：同关键词多行只一条，多关键词各一条（顺序循常量表）
	const a2 = scanAcceptanceWording(
		['- [ ] 1.1 实现解析逻辑', '- [x] 1.2 再写一层解析', '- [ ] 1.3 状态机迁移'].join('\n'),
		[],
	);
	check('⑤a 去重：解析×2 行 + 状态机×1 行 → 2 条（按关键词）', a2.length === 2);
	check('⑤a 两条文案均不重复同关键词', a2.filter((s) => s.includes('解析')).length === 1 && a2.filter((s) => s.includes('状态机')).length === 1);

	// ⑤a 文档检出抑制：test-cases*.md 前缀与 *-test-cases.md 后缀变体均放行
	const suppressed = scanAcceptanceWording('- [ ] 1.1 实现解析逻辑', ['test-cases.md']);
	const suppressed2 = scanAcceptanceWording('- [ ] 1.1 实现解析逻辑', ['test-cases-whitebox.md']);
	const suppressed3 = scanAcceptanceWording('- [ ] 1.1 实现解析逻辑', ['daily-report-test-cases.md']);
	const suppressed4 = scanAcceptanceWording('- [ ] 1.1 实现解析逻辑', ['test-cases.csv', 'xtest-cases.md']);
	check(
		'⑤a 检出抑制：test-cases.md / test-cases-whitebox.md / *-test-cases.md → 0 条；近似名不算',
		suppressed.length === 0 && suppressed2.length === 0 && suppressed3.length === 0 && suppressed4.length === 1,
	);

	// ---- 3. ⑤b 正样本：同一任务行同含「纯函数」+「SQLite」→ 1 条 ----
	const b1 = scanAcceptanceWording('- [ ] 2.1 给排序写纯函数单测：SQLite 内存库建表 → PASS', ['test-cases.md']);
	check('⑤b 正样本：纯函数+SQLite 同行（文档已检出抑制⑤a）→ 1 条', b1.length === 1);
	check('⑤b 文案含分层指引 *_unit_test.go 无 DB', b1[0].includes('*_unit_test.go 无 DB'));

	// ⑤b 每行只一条：同一行纯函数/SQLite 出现多次仍 1 条；两行则 2 条
	const b2 = scanAcceptanceWording('- [x] 3.1 纯函数纯函数逻辑用 SQLite SQLite 验证', ['test-cases.md']);
	check('⑤b 每行一条：同词重复仍 1 条', b2.length === 1);
	const b3 = scanAcceptanceWording(
		['- [ ] 4.1 纯函数走 SQLite', '- [ ] 4.2 另一任务纯函数配 SQLite'].join('\n'),
		['test-cases.md'],
	);
	check('⑤b 多行各一条：2 行 → 2 条', b3.length === 2);
	// ⑤b 半命中（只含其一）不告警
	const b4 = scanAcceptanceWording('- [ ] 5.1 纯函数单测（无 DB）\n- [ ] 5.2 repository 用 SQLite 集成测试', ['test-cases.md']);
	check('⑤b 半命中（纯函数/SQLite 各自单独出现）→ 0 条', b4.length === 0);

	// ---- 4. 健壮性：空串 / 超长行(>10KB) / 纯标签行 / 畸形输入 → 不抛异常返回数组 ----
	let robust = true;
	try {
		const empty = scanAcceptanceWording('', ['tasks.md']);
		robust = robust && Array.isArray(empty) && empty.length === 0;
		const longLine = scanAcceptanceWording('- [ ] ' + '解析'.repeat(6000), ['tasks.md']); // ~18KB 行
		robust = robust && Array.isArray(longLine) && longLine.length === 1;
		const bareLabels = scanAcceptanceWording(['- [ ]', '- [x]', '  - [x] ', '- []', '[]', '- [q] bogus'].join('\n'), []);
		robust = robust && Array.isArray(bareLabels) && bareLabels.length === 0;
		const weirdFiles = scanAcceptanceWording('- [ ] 1.1 解析任务', ['a.md', 'test-cases.md\n Injection.md']);
		robust = robust && Array.isArray(weirdFiles); // 含换行的畸形文件名（实际 ls 不可能产出）只要求不抛异常返回数组
		const nullFiles = scanAcceptanceWording('- [ ] 1.1 解析任务', null);
		robust = robust && Array.isArray(nullFiles) && nullFiles.length === 1; // null 视作空列表 → 保守提醒不算异常
	} catch (e) {
		robust = false;
		console.error('健壮性用例抛异常：', e);
	}
	check('健壮性：空串/超长行/纯标签行/畸形文件名 → 不抛异常返回合理结果', robust);

	// ---- 5. 常量表冒烟：与权威源四词对齐（漂移时此处红） ----
	check(
		'COMPLEXITY_KEYWORDS 含算法/状态机/解析/协议',
		['算法', '状态机', '解析', '协议'].every((k) => COMPLEXITY_KEYWORDS.includes(k)),
	);

	// ---- 6. ⑤a 声明优先（test-case-entry-gate）：归档时序三分支 ----
	const noDoc = ['proposal.md', 'tasks.md'];
	// 声明 complex + 缺文档 → 一条强违例
	const s1 = scanAcceptanceWording('- [ ] 1.1 干活', noDoc, '<!-- complexity: complex -->');
	check('声明 complex 缺文档 → 1 条强违例', s1.length === 1 && s1[0].includes('声明复杂档') && s1[0].includes('complexity'));
	// 声明 complex + 有文档 → 静默（文档检出抑制一切 ⑤a）
	const s2 = scanAcceptanceWording('- [ ] 1.1 干活', ['test-cases.md'], '<!-- complexity: complex -->');
	check('声明 complex 有文档 → 静默', s2.length === 0);
	// 声明 simple + 词表命中 → 一条反向质询（不再逐词）
	const s3 = scanAcceptanceWording('- [ ] 1.1 实现状态机迁移\n- [ ] 1.2 协议字段', noDoc, '<!-- complexity: simple -->');
	check('simple+命中 → 1 条质询含两词', s3.length === 1 && s3[0].includes('simple') && s3[0].includes('状态机') && s3[0].includes('协议'));
	// 声明 simple + 未命中 → 静默
	const s4 = scanAcceptanceWording('- [ ] 1.1 干活', noDoc, '<!-- complexity: simple -->');
	check('simple 未命中 → 静默', s4.length === 0);
	// 未声明（proposalText=null）+ 命中 → 沿旧逐词文案（兼容两参调用）
	const s5 = scanAcceptanceWording('- [ ] 1.1 实现解析逻辑', noDoc, null);
	check('未声明+命中 → 旧逐词文案', s5.length === 1 && s5[0].includes('可忽略'));
	const s5b = scanAcceptanceWording('- [ ] 1.1 实现解析逻辑', noDoc);
	check('省略第三参 = 同 null 行为', s5b.length === 1 && s5b[0] === s5[0]);
	// proposal 读不到但声明 complex 在（正常读不到 → null → 未声明）：验证 null 路径
	const s6 = scanAcceptanceWording('- [ ] 1.1 干活', noDoc, '');
	check('空 proposal 文本 → 未声明未命中静默', s6.length === 0);

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
		check('block → policy=spec-gate/action=block/reasonCode=archive-check-failed 恰一条',
			rowsA.length === 1 && rowsA[0].payload.policy === 'spec-gate' && rowsA[0].payload.action === 'block' && rowsA[0].payload.reasonCode === 'archive-check-failed');
		check('block 事件 change 绑定被归档 change（fake-change）', rowsA[0].change === 'fake-change');
		check('block 事件 target 只含失败检查名（无输出正文）', rowsA[0].payload.target === 'doc-impact,standards,tasks,trace' && !JSON.stringify(rowsA[0].payload).includes('boom'));
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
			`const mod=require(${JSON.stringify(path.resolve('.sgate.cjs'))});`,
			`const h={};const msgs=[];const pi={on:(n,f)=>(h[n]=f),exec:async()=>({code:0,stdout:'',stderr:''}),sendMessage:(m)=>msgs.push(m)};`,
			`mod.default(pi);`,
			`h['tool_call']({toolName:'bash',input:{command:'openspec archive envbypass-change'},toolCallId:'t'},{cwd:${JSON.stringify(tC)},sessionManager:{getSessionId:()=>'sgs2'}})`,
			`.then((r)=>console.log(JSON.stringify({pass:r===undefined,msgs:msgs.length})));`,
		].join(''))}`, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim());
		const rowsC = policyRows(tC);
		check('SPEC_GATE_BYPASS=1 → 放行 + warning 留痕 + bypass 记账', outC.pass === true && outC.msgs === 1 && rowsC.length === 1 && rowsC[0].payload.action === 'bypass' && rowsC[0].change === 'envbypass-change');

		// 7d. 检查⑤ warning → policy.decision(warn, acceptance-wording)；四项仍放行（行为不变）
		const tD = mktmp(); tmps.push(tD);
		const gD = makeGatePi(async (cmd, args) => {
			const r = await passExec(cmd, args);
			if (cmd === 'cat' && args.join(' ').includes('tasks.md')) return { code: 0, stdout: ['## 1. 任务', '- [ ] 1.1 实现解析逻辑', '', '## 2. 测试', '## 3. 文档', '<!-- doc-impact: 无 -->', '', '## 4. 验证'].join('\n'), stderr: '' };
			if (cmd === 'ls') return { code: 0, stdout: 'proposal.md\ntasks.md', stderr: '' }; // 无 test-cases*
			return r;
		});
		require('./.sgate.cjs').default(gD.pi);
		const rD = await runGate(gD, 'openspec archive warn-change', tD);
		const rowsD = policyRows(tD);
		check('检查⑤ warn → 四项检查仍放行（undefined）+ warning custom_message', rD === undefined && gD.messages.some((m) => m.customType === 'spec-gate-warning' && /检查⑤/.test(m.content)));
		check('检查⑤ warn → policy.decision(warn, acceptance-wording) 绑定 warn-change',
			rowsD.length === 1 && rowsD[0].payload.action === 'warn' && rowsD[0].payload.reasonCode === 'acceptance-wording' && rowsD[0].change === 'warn-change');

		// 7e. 正常放行（四项全过 + ⑤无违例）→ 零 policy.decision（低噪声）
		const tE = mktmp(); tmps.push(tE);
		const gE = makeGatePi(passExec);
		require('./.sgate.cjs').default(gE.pi);
		const rE = await runGate(gE, 'openspec archive ok-change', tE);
		check('四项全过 + ⑤无违例 → 放行 undefined + 零 policy.decision', rE === undefined && policyRows(tE).length === 0);

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

		// ---- 8. UI 验收证据检查④'（make-ui-design-first-class）：none/minor/major + 缺项阻断 + legacy 零检查 ----
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
		const TASKS_FULL_EVIDENCE = ['## 7. 测试', '## 8. 文档', '<!-- doc-impact: none -->', '', '## 9. 验证', '- opencli 主链路断言 PASS', '- 1440×900 视觉检查通过', '- 1920×1080 视觉检查通过'].join('\n');
		const TASKS_NO_EVIDENCE = ['## 7. 测试', '## 8. 文档', '<!-- doc-impact: none -->', '', '## 9. 验证', '- pnpm test:unit 全绿'].join('\n');

		// 8a. major 双层验收全齐 → 放行零 UI 事件（四项脚本检查走 passExec mock）
		const t8a = mktmp(); tmps.push(t8a);
		writeUiChange(t8a, 'ui-major-ok', {
			'.openspec.yaml': 'schema: syntopica-ui\ncreated: 2026-09-03\n',
			'proposal.md': '<!-- ui-impact: major -->\n## Why\nx',
			'ui-design.md': majorUi('approved'),
			'ui-prototype/index.html': '<html>proto</html>',
			'tasks.md': TASKS_FULL_EVIDENCE,
		});
		const g8a = makeGatePi(passExec);
		require('./.sgate.cjs').default(g8a.pi);
		const r8a = await runGate(g8a, 'openspec archive ui-major-ok', t8a);
		check('8a major 证据全齐 → 放行 + 零 policy.decision', r8a === undefined && policyRows(t8a).length === 0);

		// 8b. major 只有单测无交互/视觉证据 → 阻断 + ui-design-gate block ui-verification-missing
		const t8b = mktmp(); tmps.push(t8b);
		writeUiChange(t8b, 'ui-major-gap', {
			'.openspec.yaml': 'schema: syntopica-ui\ncreated: 2026-09-03\n',
			'proposal.md': '<!-- ui-impact: major -->\n## Why\nx',
			'ui-design.md': majorUi('approved'),
			'ui-prototype/index.html': '<html>proto</html>',
			'tasks.md': TASKS_NO_EVIDENCE,
		});
		const g8b = makeGatePi(passExec);
		require('./.sgate.cjs').default(g8b.pi);
		const r8b = await runGate(g8b, 'openspec archive ui-major-gap', t8b);
		const rows8b = policyRows(t8b);
		check('8b major 缺 opencli/双视口证据 → block + reason 列缺失项', r8b && r8b.block === true && /UI 验收证据/.test(r8b.reason) && /1440/.test(r8b.reason) && /opencli/.test(r8b.reason));
		check('8b 缺项 → policy=ui-design-gate block ui-verification-missing + 聚合 spec-gate block 各一条',
			rows8b.length === 2 && rows8b.some((r) => r.payload.policy === 'ui-design-gate' && r.payload.action === 'block' && r.payload.reasonCode === 'ui-verification-missing') && rows8b.some((r) => r.payload.policy === 'spec-gate' && r.payload.reasonCode === 'archive-check-failed'));

		// 8c. major 审批仍 pending → 阻断（ui-verification-missing，不相信孤立 marker）
		const t8c = mktmp(); tmps.push(t8c);
		writeUiChange(t8c, 'ui-major-pending', {
			'.openspec.yaml': 'schema: syntopica-ui\ncreated: 2026-09-03\n',
			'proposal.md': '<!-- ui-impact: major -->\n## Why\nx',
			'ui-design.md': majorUi('pending'),
			'ui-prototype/index.html': '<html>proto</html>',
			'tasks.md': TASKS_FULL_EVIDENCE,
		});
		const g8c = makeGatePi(passExec);
		require('./.sgate.cjs').default(g8c.pi);
		const r8c = await runGate(g8c, 'openspec archive ui-major-pending', t8c);
		check('8c major pending → block 且 reason 含 approved 要求', r8c && r8c.block === true && /approved/.test(r8c.reason) && policyRows(t8c).some((r) => r.payload.reasonCode === 'ui-verification-missing'));

		// 8d. minor 有验收映射 → 放行；无映射 → 阻断
		const t8d = mktmp(); tmps.push(t8d);
		const minorUi = (acc) => ['<!-- ui-impact: minor -->', '<!-- ui-approval: not-required -->', '<!-- ui-prototype: none -->', '', '## 入口', 'x', '## 状态', 'y', '## 复用', 'z', '## Acceptance', acc].join('\n');
		writeUiChange(t8d, 'ui-minor-ok', {
			'.openspec.yaml': 'schema: syntopica-ui\ncreated: 2026-09-03\n',
			'proposal.md': '<!-- ui-impact: minor -->\n## Why\nx',
			'ui-design.md': minorUi('组件测试 AppDialog.test.ts 覆盖。'),
			'tasks.md': TASKS_NO_EVIDENCE,
		});
		const g8d = makeGatePi(passExec);
		require('./.sgate.cjs').default(g8d.pi);
		const r8d = await runGate(g8d, 'openspec archive ui-minor-ok', t8d);
		check('8d minor 有组件测试映射 → 放行零 UI 事件', r8d === undefined && policyRows(t8d).length === 0);
		const t8e = mktmp(); tmps.push(t8e);
		writeUiChange(t8e, 'ui-minor-gap', {
			'.openspec.yaml': 'schema: syntopica-ui\ncreated: 2026-09-03\n',
			'proposal.md': '<!-- ui-impact: minor -->\n## Why\nx',
			'ui-design.md': minorUi('待定。'),
			'tasks.md': TASKS_NO_EVIDENCE,
		});
		const g8e = makeGatePi(passExec);
		require('./.sgate.cjs').default(g8e.pi);
		const r8e = await runGate(g8e, 'openspec archive ui-minor-gap', t8e);
		check('8e minor 无验收映射 → block + ui-verification-missing', r8e && r8e.block === true && /验收映射/.test(r8e.reason) && policyRows(t8e).some((r) => r.payload.policy === 'ui-design-gate' && r.payload.reasonCode === 'ui-verification-missing'));

		// 8f. none N/A 一致 → 放行；声明与制品不一致 → 阻断
		const t8f = mktmp(); tmps.push(t8f);
		writeUiChange(t8f, 'ui-none-ok', {
			'.openspec.yaml': 'schema: syntopica-ui\ncreated: 2026-09-03\n',
			'proposal.md': '<!-- ui-impact: none -->\n## Why\nx',
			'ui-design.md': '<!-- ui-impact: none -->\n<!-- ui-approval: not-required -->\n<!-- ui-prototype: none -->\n\n## N/A Reason\n纯后端。',
			'tasks.md': TASKS_NO_EVIDENCE,
		});
		const g8f = makeGatePi(passExec);
		require('./.sgate.cjs').default(g8f.pi);
		const r8f = await runGate(g8f, 'openspec archive ui-none-ok', t8f);
		check('8f none N/A 一致 → 放行零 UI 事件', r8f === undefined && policyRows(t8f).length === 0);

		// 8g. legacy schema → UI 检查零介入（不硬阻断）
		const t8g = mktmp(); tmps.push(t8g);
		writeUiChange(t8g, 'legacy-chg', { '.openspec.yaml': 'schema: spec-driven\ncreated: 2026-01-01\n', 'proposal.md': '## Why\nx' });
		const g8g = makeGatePi(passExec);
		require('./.sgate.cjs').default(g8g.pi);
		const r8g = await runGate(g8g, 'openspec archive legacy-chg', t8g);
		check('8g legacy schema → UI 证据零检查零事件（旧 change 不卡死）', r8g === undefined && policyRows(t8g).length === 0);

		// 8h. --force 仍豁免 UI 检查（逃生口留痕不变）
		const t8h = mktmp(); tmps.push(t8h);
		writeUiChange(t8h, 'ui-major-gap', {
			'.openspec.yaml': 'schema: syntopica-ui\ncreated: 2026-09-03\n',
			'proposal.md': '<!-- ui-impact: major -->\n## Why\nx',
			'ui-design.md': majorUi('approved'),
			'ui-prototype/index.html': '<html>proto</html>',
			'tasks.md': TASKS_NO_EVIDENCE,
		});
		const g8h = makeGatePi(passExec);
		require('./.sgate.cjs').default(g8h.pi);
		const r8h = await runGate(g8h, 'openspec archive ui-major-gap --force', t8h);
		check('8h --force → UI 检查同被豁免 + warning 留痕含 UI 项', r8h === undefined && g8h.messages.some((m) => /UI 验收证据/.test(m.content)));

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
		let lsOut9h = 'proposal.md\ntasks.md'; // 无 test-cases* → ⑤a 命中
		const g9h = makeGatePi(async (cmd, args) => {
			const r = await passExec(cmd, args);
			if (cmd === 'cat' && args.join(' ').includes('tasks.md')) return { code: 0, stdout: ['## 1. 任务', '- [ ] 1.1 实现解析逻辑', '', '## 2. 测试', '## 3. 文档', '<!-- doc-impact: 无 -->', '', '## 4. 验证'].join('\n'), stderr: '' };
			if (cmd === 'ls') return { code: 0, stdout: lsOut9h, stderr: '' };
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

		// 9-Ⅸ（W3）：违例集合变化（tasks 加一行关键词）→ 重新投递再记账（锚点：现行绿）
		const t9i = mktmp(); tmps.push(t9i);
		let tasks9i = ['## 1. 任务', '- [ ] 1.1 实现解析逻辑', '', '## 2. 测试', '## 3. 文档', '<!-- doc-impact: 无 -->', '', '## 4. 验证'].join('\n');
		const g9i = makeGatePi(async (cmd, args) => {
			const r = await passExec(cmd, args);
			if (cmd === 'cat' && args.join(' ').includes('tasks.md')) return { code: 0, stdout: tasks9i, stderr: '' };
			if (cmd === 'ls') return { code: 0, stdout: 'proposal.md\ntasks.md', stderr: '' };
			return r;
		});
		require('./.sgate.cjs').default(g9i.pi);
		await runGate(g9i, 'openspec archive edge-word', t9i, 'esgs-i');
		const m9i1 = g9i.messages.length;
		tasks9i = ['## 1. 任务', '- [ ] 1.1 实现解析逻辑', '- [ ] 1.2 状态机迁移', '', '## 2. 测试', '## 3. 文档', '<!-- doc-impact: 无 -->', '', '## 4. 验证'].join('\n');
		const r9i2 = await runGate(g9i, 'openspec archive edge-word', t9i, 'esgs-i');
		check('9-Ⅸ⑤ 违例集合变化 → 重新投递 + 再记账（锚点）',
			r9i2 === undefined && g9i.messages.length === m9i1 + 1 && policyRows(t9i).length === 2);

		// 9-Ⅹ（W4）：违例清零（已补 test-cases.md）→ 单行「已清零」收尾 + 零记账（现行无 → 红）
		const t9j = mktmp(); tmps.push(t9j);
		let lsOut9j = 'proposal.md\ntasks.md';
		const g9j = makeGatePi(async (cmd, args) => {
			const r = await passExec(cmd, args);
			if (cmd === 'cat' && args.join(' ').includes('tasks.md')) return { code: 0, stdout: ['## 1. 任务', '- [ ] 1.1 实现解析逻辑', '', '## 2. 测试', '## 3. 文档', '<!-- doc-impact: 无 -->', '', '## 4. 验证'].join('\n'), stderr: '' };
			if (cmd === 'ls') return { code: 0, stdout: lsOut9j, stderr: '' };
			return r;
		});
		require('./.sgate.cjs').default(g9j.pi);
		await runGate(g9j, 'openspec archive edge-word', t9j, 'esgs-j');
		const m9j1 = g9j.messages.length;
		lsOut9j = 'proposal.md\ntasks.md\ntest-cases.md'; // 补文档 → ⑤a 消失 → 清零
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
