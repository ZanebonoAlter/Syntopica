// ui-design-gate smoke（make-ui-design-first-class）：
// §A 合同解析分支表（test-cases.md 4.1 全行）—— lib 纯函数直测（声明缺失/非法/重复、
//    none/minor/major 三档合同、原型路径边界、mismatch 反向质询、legacy、marker 健壮性）；
// §B 工具动作分支表（4.2）—— decideUiToolGate 纯函数（requirements 静默/pass 放行/block 分流/
//    change 内修复放行/legacy warn）；
// §C extension 集成 —— 真 tmp 仓库 + 真 events.db mode.set（major pending 阻断 Agent/mutation、
//    approved/none 放行、requirements 静默、legacy 每 session/change 一次、断链 symlink、
//    bypass 留痕、fail-open 记账）；
// §D telemetry —— block/warn/bypass/fail-open 四类 policy=ui-design-gate 事件、健康放行零记录、
//    reasonCode 白名单对齐 spec。
// 由 run-harness-smoke.sh 调用（先 esbuild 产出 .uig.cjs / .uige.cjs）。断言失败 exit 1。
const fs = require('fs');
const os = require('os');
const path = require('path');
const { execSync } = require('child_process');

const priorWarn = process.listeners('warning');
process.removeAllListeners('warning');
process.on('warning', (w) => {
	if (!(w.name === 'ExperimentalWarning' && /sqlite/i.test(w.message))) {
		for (const l of priorWarn) l(w);
	}
});
const { DatabaseSync } = require('node:sqlite');

const {
	UI_SCHEMA,
	UI_REASON_CODES,
	evaluateUiContract,
	decideUiToolGate,
	parseUiImpactDeclaration,
	validatePrototypePath,
	missingArchiveUiEvidence,
} = require('./.uig.cjs');

const checks = [];
const check = (name, ok) => checks.push([name, ok]);
const mktmp = () => fs.mkdtempSync(path.join(os.tmpdir(), 'uig-smoke-'));
const loadFresh = (p) => {
	delete require.cache[require.resolve(p)];
	return require(p);
};

// ---------- fixtures ----------

const PROP_NONE = '<!-- complexity: simple -->\n<!-- ui-impact: none -->\n## Why\nx';
const PROP_MINOR = '<!-- ui-impact: minor -->\n## Why\nx';
const PROP_MAJOR = '<!-- ui-impact: major -->\n## Why\nx';
const TASKS_CLEAN = '## 1. 任务\n- [ ] 1.1 干活';

const UI_NONE_OK = [
	'<!-- ui-impact: none -->',
	'<!-- ui-approval: not-required -->',
	'<!-- ui-prototype: none -->',
	'',
	'## N/A Reason',
	'纯后端处理，无用户可见界面变化。',
].join('\n');
const UI_MINOR_OK = [
	'<!-- ui-impact: minor -->',
	'<!-- ui-approval: not-required -->',
	'<!-- ui-prototype: none -->',
	'',
	'## 入口与变更',
	'设置页 AI 健康区新增一行文案。',
	'## 受影响状态',
	'loading/empty/error 沿用既有组件；success 新增一行。',
	'## 复用组件与布局模式',
	'AppSectionHeader + contained shell。',
	'## 验收映射',
	'组件测试 + opencli 各一。',
].join('\n');
const MAJOR_SECTIONS = [
	'## User Journey', '## Information Architecture', '## Interaction Contract',
	'## State Matrix', '## Layout Contract', '## Component Reuse',
	'## Prototype', '## Acceptance',
].join('\n内容\n\n');
const UI_MAJOR_FULL = (approval, proto) => [
	`<!-- ui-impact: major -->`,
	`<!-- ui-approval: ${approval} -->`,
	`<!-- ui-prototype: ${proto} -->`,
	'',
	MAJOR_SECTIONS,
	'内容（Acceptance 节含实现与原型差异说明）',
].join('\n');
const FILES_WITH_PROTO = ['proposal.md', 'ui-design.md', 'ui-prototype/index.html'];

const contract = (over = {}) => ({
	schema: UI_SCHEMA,
	proposalText: PROP_MAJOR,
	tasksMd: TASKS_CLEAN,
	uiDesignText: UI_MAJOR_FULL('approved', 'ui-prototype/index.html'),
	changeDirFiles: FILES_WITH_PROTO,
	...over,
});
const isBlock = (v, code) => v && v.kind === 'block' && v.reasonCode === code;

// ---------- §A 合同解析分支表（4.1 全行） ----------

check('A1 proposal 声明缺失 → block ui-impact-missing', isBlock(evaluateUiContract(contract({ proposalText: '<!-- complexity: complex -->\n## Why\nx' })), 'ui-impact-missing'));
check('A1 proposal 纯空白 → block ui-impact-missing', isBlock(evaluateUiContract(contract({ proposalText: '   \n\n' })), 'ui-impact-missing'));
check('A1 值为空（<!-- ui-impact: -->）→ block ui-impact-missing', isBlock(evaluateUiContract(contract({ proposalText: '<!-- ui-impact: -->\n## Why\nx' })), 'ui-impact-missing'));
check('A1 值 Major（大写）→ 非法阻断', parseUiImpactDeclaration('<!-- ui-impact: Major -->') === null && isBlock(evaluateUiContract(contract({ proposalText: '<!-- ui-impact: Major -->' })), 'ui-impact-missing'));
check('A1 值 large（词表外）→ 非法阻断', parseUiImpactDeclaration('<!-- ui-impact: large -->') === null);
check('A1 重复冲突 marker → 歧义阻断', parseUiImpactDeclaration('<!-- ui-impact: none -->\n<!-- ui-impact: major -->') === null);
check('A1 重复同值 marker → 仍歧义阻断', parseUiImpactDeclaration('<!-- ui-impact: none -->\n<!-- ui-impact: none -->') === null);
check('A1 marker 内空白容忍且值规范化', parseUiImpactDeclaration('前文\n<!--  ui-impact:   minor  -->\n后文') === 'minor');
check('A2 mismatch：声明 none + tasks 命中「新增面板」→ block ui-impact-mismatch',
	isBlock(evaluateUiContract(contract({ proposalText: PROP_NONE, uiDesignText: UI_NONE_OK, tasksMd: '- [ ] 1.1 新增面板入口' })), 'ui-impact-mismatch'));
check('A2 mismatch：声明 minor + proposal 命中「多步流程」→ block ui-impact-mismatch',
	isBlock(evaluateUiContract(contract({ proposalText: '<!-- ui-impact: minor -->\n## Why\n引入多步流程', uiDesignText: UI_MINOR_OK })), 'ui-impact-mismatch'));
check('A2 mismatch：声明 none + tasks 出现前端路径 → block ui-impact-mismatch',
	isBlock(evaluateUiContract(contract({ proposalText: PROP_NONE, uiDesignText: UI_NONE_OK, tasksMd: '- [ ] 1.1 改 front/app/pages/tags.vue' })), 'ui-impact-mismatch'));
check('A2 none 档前端路径 + ui-design 豁免注释 → pass（提示语义兑现）', evaluateUiContract(contract({ proposalText: PROP_NONE, uiDesignText: UI_NONE_OK + '\n<!-- ui-front-path-excuse: 他人脏文件路径示例，与本 change 无关 -->', tasksMd: '- [ ] 1.1 改 front/app/pages/tags.vue' })).kind === 'pass');
check('A2 major 声明不反向质询（无 mismatch）', evaluateUiContract(contract()).kind === 'pass');

check('A3 none 档 ui-design 缺失 → block ui-design-missing', isBlock(evaluateUiContract(contract({ proposalText: PROP_NONE, uiDesignText: null })), 'ui-design-missing'));
check('A3 none 档缺 N/A 节 → block ui-design-missing', isBlock(evaluateUiContract(contract({ proposalText: PROP_NONE, uiDesignText: '<!-- ui-impact: none -->\n<!-- ui-approval: not-required -->\n<!-- ui-prototype: none -->\n\n## 随便' })), 'ui-design-missing'));
check('A3 none 档 approval=pending（档位矛盾）→ block ui-design-missing', isBlock(evaluateUiContract(contract({ proposalText: PROP_NONE, uiDesignText: '<!-- ui-impact: none -->\n<!-- ui-approval: pending -->\n<!-- ui-prototype: none -->\n## N/A Reason\nx' })), 'ui-design-missing'));
check('A4 none 档 N/A 完整 → pass', evaluateUiContract(contract({ proposalText: PROP_NONE, uiDesignText: UI_NONE_OK })).kind === 'pass');

check('A5 minor 档必填节缺失 → block ui-design-missing', isBlock(evaluateUiContract(contract({ proposalText: PROP_MINOR, uiDesignText: '<!-- ui-impact: minor -->\n<!-- ui-approval: not-required -->\n<!-- ui-prototype: none -->\n\n## 入口\nx' })), 'ui-design-missing'));
check('A5 minor 档 approval 非法值 → block ui-design-missing', isBlock(evaluateUiContract(contract({ proposalText: PROP_MINOR, uiDesignText: UI_MINOR_OK.replace('not-required', 'later') })), 'ui-design-missing'));
check('A6 minor 档完整无原型 → pass', evaluateUiContract(contract({ proposalText: PROP_MINOR, uiDesignText: UI_MINOR_OK })).kind === 'pass');
check('A6b minor 档提供原型且存在 → pass', evaluateUiContract(contract({ proposalText: PROP_MINOR, uiDesignText: UI_MINOR_OK.replace('ui-prototype: none', 'ui-prototype: ui-prototype/index.html') })).kind === 'pass');

check('A7 major 档 ui-design 缺失 → block ui-design-missing', isBlock(evaluateUiContract(contract({ uiDesignText: null })), 'ui-design-missing'));
check('A7 major 档节不全 → block ui-design-missing', isBlock(evaluateUiContract(contract({ uiDesignText: '<!-- ui-impact: major -->\n<!-- ui-approval: approved -->\n<!-- ui-prototype: ui-prototype/index.html -->\n\n## User Journey\nx\n## Acceptance\ny' })), 'ui-design-missing'));
check('A7 major 档 marker 与 proposal 声明不一致（ui-design=minor vs proposal=major）→ block ui-design-missing',
	isBlock(evaluateUiContract(contract({ proposalText: PROP_MAJOR, uiDesignText: '<!-- ui-impact: minor -->\n<!-- ui-approval: pending -->\n<!-- ui-prototype: ui-prototype/index.html -->\n' + MAJOR_SECTIONS })), 'ui-design-missing'));
check('A7 major 档 approval=not-required → block ui-design-missing', isBlock(evaluateUiContract(contract({ uiDesignText: UI_MAJOR_FULL('not-required', 'ui-prototype/index.html') })), 'ui-design-missing'));
check('A8 major 原型缺失（marker none）→ block ui-prototype-missing', isBlock(evaluateUiContract(contract({ uiDesignText: UI_MAJOR_FULL('approved', 'none') })), 'ui-prototype-missing'));
check('A8 major 原型空值 → block ui-prototype-missing', isBlock(evaluateUiContract(contract({ uiDesignText: UI_MAJOR_FULL('approved', '') })), 'ui-prototype-missing'));
check('A8 major 原型绝对路径 → block ui-prototype-missing', validatePrototypePath('/etc/passwd', FILES_WITH_PROTO) !== null && isBlock(evaluateUiContract(contract({ uiDesignText: UI_MAJOR_FULL('approved', '/abs/index.html') })), 'ui-prototype-missing'));
check('A8 major 原型 ../ 越界 → block ui-prototype-missing', validatePrototypePath('../ui-prototype/index.html', ['../ui-prototype/index.html']) !== null && isBlock(evaluateUiContract(contract({ uiDesignText: UI_MAJOR_FULL('approved', '../escape.html') })), 'ui-prototype-missing'));
check('A8 major 原型目录（尾斜杠）→ block ui-prototype-missing', validatePrototypePath('ui-prototype/', ['ui-prototype/']) !== null);
check('A8 major 原型不在清单（断链 symlink/不存在）→ block ui-prototype-missing', isBlock(evaluateUiContract(contract({ uiDesignText: UI_MAJOR_FULL('approved', 'ui-prototype/broken.html') })), 'ui-prototype-missing'));
check('A8 major 原型越出 ui-prototype/ 前缀 → block ui-prototype-missing', isBlock(evaluateUiContract(contract({ uiDesignText: UI_MAJOR_FULL('approved', 'specs/spec.md'), changeDirFiles: ['specs/spec.md'] })), 'ui-prototype-missing'));
check('A9 major 完整 + 原型存在 + pending → block ui-approval-pending', isBlock(evaluateUiContract(contract({ uiDesignText: UI_MAJOR_FULL('pending', 'ui-prototype/index.html') })), 'ui-approval-pending'));
check('A9 pending 优先于 approval 但原型缺失仍先报 prototype（表序：原型先于审批）', isBlock(evaluateUiContract(contract({ uiDesignText: UI_MAJOR_FULL('pending', 'none') })), 'ui-prototype-missing'));
check('A10 major 完整 + 原型存在 + approved → pass', evaluateUiContract(contract()).kind === 'pass');
check('A11 legacy schema（spec-driven/null）→ legacy（不硬阻断）', evaluateUiContract(contract({ schema: 'spec-driven' })).kind === 'legacy' && evaluateUiContract(contract({ schema: null })).kind === 'legacy');
check('A12 marker 前后空白容忍（ui-design 侧）', evaluateUiContract(contract({ uiDesignText: UI_MAJOR_FULL('approved', 'ui-prototype/index.html').replace('<!-- ui-impact: major -->', '<!--  ui-impact:  major  -->') })).kind === 'pass');
check('A13 超长自由文本输入不抛异常', (() => { try { const big = 'x'.repeat(50000); evaluateUiContract(contract({ proposalText: big, tasksMd: big, uiDesignText: big })); return true; } catch { return false; } })());

// ---------- §B 工具动作分支表（4.2） ----------

const TOOL = { agent: { kind: 'agent-dispatch', targetPath: null }, read: { kind: 'other', targetPath: null } };
const mut = (p) => ({ kind: 'mutation', targetPath: p });
const gate = (over = {}) => ({
	schema: UI_SCHEMA, mode: 'implementation', boundChange: 'chg',
	proposalText: PROP_MAJOR, tasksMd: TASKS_CLEAN,
	uiDesignText: UI_MAJOR_FULL('pending', 'ui-prototype/index.html'),
	changeDirFiles: FILES_WITH_PROTO,
	tool: TOOL.agent,
	...over,
});

check('B1 requirements 档任意工具 → 静默放行', decideUiToolGate(gate({ mode: 'requirements' })).action === 'allow' && decideUiToolGate(gate({ mode: 'requirements', tool: mut('src/x.go') })).action === 'allow');
check('B2 无档位记录/未绑定 change → 静默放行', decideUiToolGate(gate({ mode: null })).action === 'allow' && decideUiToolGate(gate({ boundChange: null })).action === 'allow');
check('B3 合同 pass：Agent 派发与 mutation 均放行（零噪声）', decideUiToolGate(gate({ uiDesignText: UI_MAJOR_FULL('approved', 'ui-prototype/index.html') })).action === 'allow');
check('B4 合同 block + Agent 派发 → block ui-approval-pending', isBlock(decideUiToolGate(gate()), 'ui-approval-pending') || (decideUiToolGate(gate()).action === 'block' && decideUiToolGate(gate()).reasonCode === 'ui-approval-pending'));
check('B4 合同 block + 项目代码 mutation → block', decideUiToolGate(gate({ tool: mut('src/x.go') })).action === 'block' && decideUiToolGate(gate({ tool: mut('backend-go/main.go') })).action === 'block');
check('B5 合同 block + 当前 change 内 ui-design.md / ui-prototype/** 修复 → 放行', decideUiToolGate(gate({ tool: mut('openspec/changes/chg/ui-design.md') })).action === 'allow' && decideUiToolGate(gate({ tool: mut('openspec/changes/chg/ui-prototype/index.html') })).action === 'allow');
check('B6 合同 block + 当前 change 内其他规划制品 → 放行', decideUiToolGate(gate({ tool: mut('openspec/changes/chg/tasks.md') })).action === 'allow');
check('B6 其他 change 目录 mutation → 仍 block（只放行当前 change）', decideUiToolGate(gate({ tool: mut('openspec/changes/other/proposal.md') })).action === 'block');
check('B7 合同 block + 普通读取/验证/bash（other）→ 放行', decideUiToolGate(gate({ tool: TOOL.read })).action === 'allow');
check('B8 legacy + front mutation → warn-legacy（ui-design-missing）', (() => { const d = decideUiToolGate(gate({ schema: 'spec-driven', proposalText: '<!-- ui-impact: none -->', uiDesignText: null, tool: mut('front/app/pages/x.vue') })); return d.action === 'warn-legacy' && d.reasonCode === 'ui-design-missing'; })());
check('B8 legacy + 非 front mutation → 放行', decideUiToolGate(gate({ schema: 'spec-driven', uiDesignText: null, proposalText: '<!-- ui-impact: none -->', tool: mut('backend-go/x.go') })).action === 'allow');
check('B9 阻断文案三要素：change 名 + reasonCode + 修复路径', (() => { const d = decideUiToolGate(gate()); return d.action === 'block' && d.message.includes('chg') && d.message.includes('ui-approval-pending') && d.message.includes('openspec/changes/chg/ui-design.md'); })());

// ---------- §C/§D extension 集成 + telemetry ----------

const EVENTS_DDL = [
	'CREATE TABLE IF NOT EXISTS events (id INTEGER PRIMARY KEY AUTOINCREMENT, ts TEXT NOT NULL, session_id TEXT NOT NULL, kind TEXT NOT NULL, change TEXT, payload TEXT NOT NULL)',
	'CREATE INDEX IF NOT EXISTS idx_ev_session ON events(session_id, id)',
	'CREATE INDEX IF NOT EXISTS idx_ev_change ON events(change, id)',
	'CREATE INDEX IF NOT EXISTS idx_ev_kind_ts ON events(kind, ts)',
];
function seedModeSet(cwd, sessionId, mode, boundChange) {
	const dbDir = path.join(cwd, '.pi', 'harness');
	fs.mkdirSync(dbDir, { recursive: true });
	const db = new DatabaseSync(path.join(dbDir, 'events.db'));
	// harness-log 归属守卫：登记应用魔数 + schema 版本，否则 openDb 视为他人库拒写
	db.exec(`PRAGMA application_id = ${0x53594e54}`);
	db.exec('PRAGMA user_version = 1');
	for (const ddl of EVENTS_DDL) db.exec(ddl);
	db.prepare('INSERT INTO events (ts, session_id, kind, change, payload) VALUES (?, ?, ?, ?, ?)')
		.run(new Date().toISOString(), sessionId, 'mode.set', boundChange, JSON.stringify({ mode, boundChange }));
	db.close();
}
const policyRows = (cwd, policy) => {
	const dbFile = path.join(cwd, '.pi', 'harness', 'events.db');
	if (!fs.existsSync(dbFile)) return [];
	const db = new DatabaseSync(dbFile);
	const rows = db.prepare("SELECT * FROM events WHERE kind='policy.decision' ORDER BY id").all();
	db.close();
	const out = rows.map((r) => ({ ...r, payload: JSON.parse(r.payload) }));
	return policy ? out.filter((r) => r.payload.policy === policy) : out;
};
function writeChange(cwd, name, files) {
	const dir = path.join(cwd, 'openspec/changes', name);
	for (const [rel, content] of Object.entries(files)) {
		const p = path.join(dir, rel);
		fs.mkdirSync(path.dirname(p), { recursive: true });
		fs.writeFileSync(p, content);
	}
	return dir;
}
const SCHEMA_UI = `schema: ${UI_SCHEMA}\ncreated: 2026-09-03\n`;
const SCHEMA_LEGACY = 'schema: spec-driven\ncreated: 2026-01-01\n';
const MAJOR_PENDING_FILES = {
	'.openspec.yaml': SCHEMA_UI,
	'proposal.md': PROP_MAJOR,
	'ui-design.md': UI_MAJOR_FULL('pending', 'ui-prototype/index.html'),
	'ui-prototype/index.html': '<html>proto</html>',
};
const MAJOR_APPROVED_FILES = { ...MAJOR_PENDING_FILES, 'ui-design.md': UI_MAJOR_FULL('approved', 'ui-prototype/index.html') };
const NONE_OK_FILES = { '.openspec.yaml': SCHEMA_UI, 'proposal.md': PROP_NONE, 'ui-design.md': UI_NONE_OK };

function makePi() {
	const handlers = {};
	const messages = [];
	const pi = {
		on: (n, f) => (handlers[n] = f),
		sendMessage: (m) => messages.push(m),
	};
	return { pi, handlers, messages };
}
async function runTool(mod, cwd, sessionId, toolName, input, mode = 'implementation', boundChange = 'major-chg', ctxOverride = null) {
	const { pi, handlers, messages } = makePi();
	mod.default(pi);
	const ctx = ctxOverride ?? { cwd, sessionManager: { getSessionId: () => sessionId } };
	const result = await handlers['tool_call'](
		{ toolName, input, toolCallId: 'tc1' },
		ctx,
	);
	return { result, messages };
}

(async () => {
	const tmps = [];
	try {
		// C1 major pending：Agent 派发被阻断 + block 事件（change 绑定）
		const t1 = mktmp(); tmps.push(t1);
		writeChange(t1, 'major-chg', MAJOR_PENDING_FILES);
		seedModeSet(t1, 's1', 'implementation', 'major-chg');
		const r1 = await runTool(loadFresh('./.uige.cjs'), t1, 's1', 'Agent', { prompt: 'x', subagent_type: 'Explore' });
		check('C1 major pending Agent 派发 → block:true + 文案含 reasonCode/修复路径', r1.result && r1.result.block === true && r1.result.reason.includes('ui-approval-pending') && r1.result.reason.includes('major-chg'));
		check('D1 阻断 → policy=ui-design-gate block ui-approval-pending 绑定 change', (() => { const rows = policyRows(t1, 'ui-design-gate'); return rows.length === 1 && rows[0].payload.action === 'block' && rows[0].payload.reasonCode === 'ui-approval-pending' && rows[0].change === 'major-chg'; })());

		// C2 major pending：项目代码 mutation 阻断；change 内修复放行；读取放行（同 cwd 复用）
		const r2a = await runTool(loadFresh('./.uige.cjs'), t1, 's1', 'edit', { path: path.join(t1, 'backend-go/main.go'), edits: [] });
		check('C2 major pending 项目代码 mutation → block', r2a.result && r2a.result.block === true);
		const r2b = await runTool(loadFresh('./.uige.cjs'), t1, 's1', 'write', { path: path.join(t1, 'openspec/changes/major-chg/ui-design.md'), content: 'x' });
		check('C2 major pending 修 ui-design.md → 放行', r2b.result === undefined);
		const r2c = await runTool(loadFresh('./.uige.cjs'), t1, 's1', 'bash', { command: 'cat openspec/changes/major-chg/ui-design.md' });
		check('C2 bash 读取 → 零介入放行', r2c.result === undefined);
		check('D2 健康修复/读取路径零额外 block 记账', policyRows(t1, 'ui-design-gate').length === 2); // 仅 C1 Agent + C2a edit

		// C3 用户批准后（磁盘改 approved）同 session 重试 → 放行且零新增事件（不沿用旧 pending 缓存）
		fs.writeFileSync(path.join(t1, 'openspec/changes/major-chg/ui-design.md'), UI_MAJOR_FULL('approved', 'ui-prototype/index.html'));
		const r3 = await runTool(loadFresh('./.uige.cjs'), t1, 's1', 'Agent', { prompt: 'x', subagent_type: 'Explore' });
		check('C3 批准落盘后即时放行（重新读磁盘）', r3.result === undefined);
		check('D3 健康放行零记录', policyRows(t1, 'ui-design-gate').length === 2);

		// C4 requirements 档静默：同 change 任意工具零介入零记账
		const t4 = mktmp(); tmps.push(t4);
		writeChange(t4, 'major-chg', MAJOR_PENDING_FILES);
		seedModeSet(t4, 's4', 'requirements', 'major-chg');
		const r4 = await runTool(loadFresh('./.uige.cjs'), t4, 's4', 'Agent', { prompt: 'x', subagent_type: 'Explore' });
		check('C4 requirements 档 → 静默放行零记账', r4.result === undefined && policyRows(t4).length === 0);

		// C5 none 档合同齐备：front mutation 放行（零噪声）
		const t5 = mktmp(); tmps.push(t5);
		writeChange(t5, 'none-chg', NONE_OK_FILES);
		seedModeSet(t5, 's5', 'implementation', 'none-chg');
		const r5 = await runTool(loadFresh('./.uige.cjs'), t5, 's5', 'edit', { path: path.join(t5, 'backend-go/x.go'), edits: [] });
		check('C5 none 合同齐备 → 放行零记账', r5.result === undefined && policyRows(t5).length === 0);

		// C6 legacy：front mutation → warn 一次（policy warn）；同 session 第二次零新增；非 front 零提醒
		const t6 = mktmp(); tmps.push(t6);
		writeChange(t6, 'legacy-chg', { '.openspec.yaml': SCHEMA_LEGACY, 'proposal.md': '## Why\nx' });
		seedModeSet(t6, 's6', 'implementation', 'legacy-chg');
		const m6 = loadFresh('./.uige.cjs');
		const r6a = await runTool(m6, t6, 's6', 'edit', { path: path.join(t6, 'front/app/pages/x.vue'), edits: [] });
		check('C6 legacy front mutation → 放行（不硬阻断）+ steer 提醒', r6a.result === undefined && r6a.messages.length === 1 && /ui-design/.test(r6a.messages[0].content));
		check('D4 legacy 提醒 → policy warn ui-design-missing 恰一条（绑定 change）', (() => { const rows = policyRows(t6, 'ui-design-gate'); return rows.length === 1 && rows[0].payload.action === 'warn' && rows[0].payload.reasonCode === 'ui-design-missing' && rows[0].change === 'legacy-chg'; })());
		const r6b = await runTool(m6, t6, 's6', 'edit', { path: path.join(t6, 'front/app/components/y.vue'), edits: [] });
		check('C6 同 session 二次 front mutation → 静默零新增', r6b.messages.length === 0 && policyRows(t6, 'ui-design-gate').length === 1);
		const m6b = loadFresh('./.uige.cjs'); // 新 session（新模块实例）→ 再提醒一次
		const r6c = await runTool(m6b, t6, 's6', 'edit', { path: path.join(t6, 'front/app/pages/z.vue'), edits: [] });
		check('C6 新 session 可再提醒一次（恢复上下文）', r6c.messages.length === 1 && policyRows(t6, 'ui-design-gate').length === 2);

		// C7 断链 symlink 原型：普通文件清单不含 → block ui-prototype-missing
		const t7 = mktmp(); tmps.push(t7);
		const d7 = writeChange(t7, 'sym-chg', { '.openspec.yaml': SCHEMA_UI, 'proposal.md': PROP_MAJOR, 'ui-design.md': UI_MAJOR_FULL('approved', 'ui-prototype/broken.html') });
		fs.mkdirSync(path.join(d7, 'ui-prototype'), { recursive: true });
		fs.symlinkSync('/nonexistent/target', path.join(d7, 'ui-prototype/broken.html'));
		seedModeSet(t7, 's7', 'implementation', 'sym-chg');
		const r7 = await runTool(loadFresh('./.uige.cjs'), t7, 's7', 'Agent', { prompt: 'x', subagent_type: 'Explore' });
		check('C7 断链 symlink 原型 → block ui-prototype-missing', r7.result && r7.result.block === true && r7.result.reason.includes('ui-prototype-missing'));

		// C8 内部异常 → fail-open 放行 + 显式告警 + fail-open 记账（不伪装成功）；
		//    模拟「check 内部 bug 但上下文可读」：getSessionId 首调抛错、auditPolicy 复调恢复
		const t8 = mktmp(); tmps.push(t8);
		seedModeSet(t8, 's8', 'implementation', 'major-chg');
		writeChange(t8, 'major-chg', MAJOR_PENDING_FILES);
		let ctxCalls = 0;
		const ctx8 = {
			cwd: t8,
			sessionManager: {
				getSessionId: () => {
					if (ctxCalls++ === 0) throw new TypeError('模拟内部 bug');
					return 's8';
				},
			},
		};
		const r8 = await runTool(loadFresh('./.uige.cjs'), t8, 's8', 'Agent', { prompt: 'x', subagent_type: 'Explore' }, undefined, undefined, ctx8);
		check('C8 检查异常 → fail-open 放行 + steer 告警', r8.result === undefined && r8.messages.length === 1 && /fail-open/.test(r8.messages[0].content));
		check('D5 fail-open → policy fail-open ui-gate-check-failed', (() => { const rows = policyRows(t8, 'ui-design-gate'); return rows.length === 1 && rows[0].payload.action === 'fail-open' && rows[0].payload.reasonCode === 'ui-gate-check-failed'; })());

		// C9 显式 bypass：UI_DESIGN_GATE_BYPASS=1 → 放行 + 告警留痕 + policy bypass（子进程，防主进程 env 污染）
		const t9 = mktmp(); tmps.push(t9);
		writeChange(t9, 'major-chg', MAJOR_PENDING_FILES);
		seedModeSet(t9, 's9', 'implementation', 'major-chg');
		const out9 = JSON.parse(execSync(`node -e ${JSON.stringify([
			`process.env.UI_DESIGN_GATE_BYPASS='1';`,
			`const mod=require(${JSON.stringify(path.resolve('.uige.cjs'))});`,
			`const h={};const msgs=[];const pi={on:(n,f)=>(h[n]=f),sendMessage:(m)=>msgs.push(m)};mod.default(pi);`,
			`h['tool_call']({toolName:'Agent',input:{prompt:'x'},toolCallId:'t'},{cwd:${JSON.stringify(t9)},sessionManager:{getSessionId:()=>'s9'}})`,
			`.then((r)=>console.log(JSON.stringify({pass:r===undefined,msgs:msgs.length})));`,
		].join(''))}`, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim());
		check('C9 bypass → 放行 + 显式告警不静默', out9.pass === true && out9.msgs === 1);
		check('D6 bypass → policy bypass explicit-bypass', (() => { const rows = policyRows(t9, 'ui-design-gate'); return rows.length === 1 && rows[0].payload.action === 'bypass' && rows[0].payload.reasonCode === 'explicit-bypass'; })());

		// C10 提取不到档位（无 mode.set）→ 完全静默
		const t10 = mktmp(); tmps.push(t10);
		writeChange(t10, 'major-chg', MAJOR_PENDING_FILES);
		fs.mkdirSync(path.join(t10, '.pi/harness'), { recursive: true });
		const r10 = await runTool(loadFresh('./.uige.cjs'), t10, 's10', 'Agent', { prompt: 'x', subagent_type: 'Explore' });
		check('C10 无 mode.set 记录 → 静默放行零记账', r10.result === undefined && policyRows(t10).length === 0);

		// D7 reasonCode 白名单与 spec 八值对齐
		const expected = ['ui-impact-missing', 'ui-impact-mismatch', 'ui-design-missing', 'ui-prototype-missing', 'ui-approval-pending', 'ui-verification-missing', 'explicit-bypass', 'ui-gate-check-failed'];
		check('D7 UI_REASON_CODES 与 spec 八值对齐', expected.every((c) => UI_REASON_CODES.includes(c)) && UI_REASON_CODES.length === 8);

		// D8 归档证据检查纯函数速测（详测在 spec-gate.smoke.cjs）
		check('D8 missingArchiveUiEvidence：legacy → 空（不硬阻断）', missingArchiveUiEvidence({ schema: 'spec-driven', proposalText: PROP_MAJOR, uiDesignText: null, tasksMd: '', changeDirFiles: [] }).length === 0);
		check('D8 missingArchiveUiEvidence：major 全齐 → 空', missingArchiveUiEvidence({ schema: UI_SCHEMA, proposalText: PROP_MAJOR, uiDesignText: UI_MAJOR_FULL('approved', 'ui-prototype/index.html') + '\n差异说明见上。', tasksMd: '## 7. 测试\n## 8. 文档\n## 9. 验证\n- opencli 主链路 PASS\n- 1440×900 与 1920×1080 视觉检查通过', changeDirFiles: FILES_WITH_PROTO }).length === 0);
	} catch (e) {
		console.error('FAIL', e);
		check('smoke 执行不抛异常', false);
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
})();
