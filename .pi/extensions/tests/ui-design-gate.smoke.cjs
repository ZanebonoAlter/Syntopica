// ui-design-gate smoke（make-ui-design-first-class）：
// §A 合同解析分支表（test-cases.md 4.1 全行）—— lib 纯函数直测（声明缺失/非法/重复、
//    none/minor/major 三档合同、原型路径边界、mismatch 反向质询、legacy、marker 健壮性）；
// §B 工具动作分支表（4.2）—— decideUiToolGate 纯函数（requirements 静默/pass 放行/block 分流/
//    change 内修复放行/legacy warn）；
// §C extension 集成 —— 真 tmp 仓库 + 真 events.db stage.set（major pending 阻断 Agent/mutation、
//    approved/none 放行、requirements 静默、legacy 每 session/change 一次、断链 symlink、
//    bypass 留痕、fail-open 记账）；
// §D telemetry —— block/warn/bypass/fail-open 四类 policy=ui-design-gate 事件、健康放行零记录、
//    reasonCode 白名单对齐 spec；
// §E 派发放行与写侧不放宽（add-ui-prototype-editor-agent）—— 只读档/原型编辑档派发在合同未就绪时
//    放行并留痕 ui-dispatch-allowed、写能力档仍 block、子线程越界写入仍 block、白名单漂移防护
//    （正向：白名单项均有同名 profile；反向：无写工具的 profile 必须在白名单内）、原型编辑档 profile
//    前提断言（load_extensions: true + tools 含 edit/write）。
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
	UI_PASS_REASON_CODES,
	UI_DISPATCH_ALLOWED_AGENTS,
	passReasonForRelease,
	evaluateUiContract,
	decideUiToolGate,
	parseUiImpactDeclaration,
	validatePrototypePath,
	missingArchiveUiEvidence,
	UI_STAGES,
	UI_STAGE_WARN_REASON_CODES,
	parseUiStage,
	changeNameFromPath,
	relInsideChange,
	checkStageScope,
	checkPrototypeChain,
	readPseudocodeSection,
	hasSkipMarker,
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
	'组件测试 + agent-browser 各一。',
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
	// §F 引入 stage 记账后，本 fixture 显式声明 interaction 阶段：
	// 让 §A–§E 的断言只测审批/派发通道，不被 stage warn 行污染
	// （stage 通道自带 §F 专用用例，含 l0/skeleton 越域与 legacy 缺失）。
	`<!-- ui-stage: interaction -->`,
	'',
	MAJOR_SECTIONS,
	'内容（Acceptance 节含实现与原型差异说明）',
].join('\n');
const FILES_WITH_PROTO = ['proposal.md', 'ui-design.md', 'ui-prototype/index.html', 'ui-prototype/skeleton/index.html'];

// ISS-02：major 批准的那一刻要求 design.md 有伪代码节——默认 fixture 带上，
// 单独的负例用 PSEUDO_MISSING 覆盖。
const DESIGN_PSEUDO = [
	'## Decision',
	'x',
	'',
	'## 核心逻辑（伪代码）',
	'```',
	'computeLaneHeat(lane, date) -> { heat, clues }',
	'  if lanes empty -> return 0',
	'```',
].join('\n');
const contract = (over = {}) => ({
	schema: UI_SCHEMA,
	proposalText: PROP_MAJOR,
	tasksMd: TASKS_CLEAN,
	uiDesignText: UI_MAJOR_FULL('approved', 'ui-prototype/index.html'),
	designMd: DESIGN_PSEUDO,
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

const TOOL = { agent: { kind: 'agent-dispatch', subagentType: null }, read: { kind: 'other', targetPath: null } };
const mut = (p) => ({ kind: 'mutation', targetPath: p });
const dispatch = (t) => ({ kind: 'agent-dispatch', subagentType: t });
const gate = (over = {}) => ({
	schema: UI_SCHEMA, stageFamily: 'implementation', boundChange: 'chg',
	proposalText: PROP_MAJOR, tasksMd: TASKS_CLEAN,
	uiDesignText: UI_MAJOR_FULL('pending', 'ui-prototype/index.html'),
	designMd: DESIGN_PSEUDO,
	changeDirFiles: FILES_WITH_PROTO,
	tool: TOOL.agent,
	...over,
});

check('B1 planning 族任意工具 → 静默放行', decideUiToolGate(gate({ stageFamily: 'planning' })).action === 'allow' && decideUiToolGate(gate({ stageFamily: 'planning', tool: mut('src/x.go') })).action === 'allow');
check('B2 未绑定阶段/未绑定 change → 静默放行', decideUiToolGate(gate({ stageFamily: null })).action === 'allow' && decideUiToolGate(gate({ boundChange: null })).action === 'allow');
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
// B10~B14 派发放行（add-ui-prototype-editor-agent design D2/D3）：只读档与原型编辑档在合同
// block 态放行（dispatch-allowed + ui-dispatch-allowed）；写能力档/未指定档位维持 block
check('B10 合同 block + 只读档（explore/plan/change-review）派发 → dispatch-allowed ui-dispatch-allowed', ['explore', 'plan', 'change-review'].every((t) => { const d = decideUiToolGate(gate({ tool: dispatch(t) })); return d.action === 'dispatch-allowed' && d.reasonCode === 'ui-dispatch-allowed'; }));
check('B11 合同 block + 原型编辑档（ui-design）派发 → dispatch-allowed ui-dispatch-allowed', (() => { const d = decideUiToolGate(gate({ tool: dispatch('ui-design') })); return d.action === 'dispatch-allowed' && d.reasonCode === 'ui-dispatch-allowed'; })());
check('B12 合同 block + 写能力档（general-purpose）派发 → 维持 block', (() => { const d = decideUiToolGate(gate({ tool: dispatch('general-purpose') })); return d.action === 'block' && d.reasonCode === 'ui-approval-pending'; })());
check('B13 合同 block + 未指定档位的派发（默认 general-purpose）→ 维持 block', decideUiToolGate(gate({ tool: dispatch(null) })).action === 'block');
check('B14 合同 block + 档位名大小写不敏感（Explore ≡ explore）→ 同样放行', decideUiToolGate(gate({ tool: dispatch('Explore') })).action === 'dispatch-allowed');
check('B15 放行不改变写侧判定：越出 change 目录的 mutation 仍 block / change 目录内仍放行', decideUiToolGate(gate({ tool: mut('front/app/pages/x.vue') })).action === 'block' && decideUiToolGate(gate({ tool: mut('openspec/changes/chg/ui-prototype/index.html') })).action === 'allow');

// ---------- §A' 业务先定（ISS-02 D1）：major 批准时伪代码节必须在 —— ----------

const PSEUDO_MISSING = '## Decision\nx\n';
const PSEUDO_NA_EMPTY = '## 核心逻辑（伪代码）\n\nN/A：\n';
const PSEUDO_NA_OK = '## 核心逻辑（伪代码）\n\nN/A：只改布局常量，无控制流变化\n';

check("A1' major approved + design.md 无伪代码节 → block ui-pseudocode-missing",
	isBlock(evaluateUiContract(contract({ designMd: PSEUDO_MISSING })), 'ui-pseudocode-missing'));
check("A2' major approved + design.md 缺失 → block ui-pseudocode-missing",
	isBlock(evaluateUiContract(contract({ designMd: null })), 'ui-pseudocode-missing'));
check("A3' major approved + N/A 理由为空 → block ui-pseudocode-missing",
	isBlock(evaluateUiContract(contract({ designMd: PSEUDO_NA_EMPTY })), 'ui-pseudocode-missing'));
check("A4' major approved + N/A 带理由（含单字「无」）→ pass",
	evaluateUiContract(contract({ designMd: PSEUDO_NA_OK })).kind === 'pass' && evaluateUiContract(contract({ designMd: '## 核心逻辑（伪代码）\n\nN/A：无\n' })).kind === 'pass');
check("A5' major approved + 有伪代码 → pass",
	evaluateUiContract(contract()).kind === 'pass');
check("A6' major pending 仍卡在审批（不看伪代码）",
	isBlock(evaluateUiContract(contract({ uiDesignText: UI_MAJOR_FULL('pending', 'ui-prototype/index.html'), designMd: PSEUDO_MISSING })), 'ui-approval-pending'));

// ---------- §B' 原型产出物依赖链（ISS-02 D2）：缺前一层就 block —— ----------

const passGate = (over = {}) => gate({ uiDesignText: UI_MAJOR_FULL('approved', 'ui-prototype/index.html'), ...over });
check("B1' 有 skeleton 时写 index.html → 放行",
	decideUiToolGate(passGate({ tool: mut('openspec/changes/chg/ui-prototype/index.html') })).action === 'allow');
check("B2' 无 skeleton 时写 index.html → block ui-chain-out-of-order",
	decideUiToolGate(passGate({ changeDirFiles: ['proposal.md', 'ui-design.md', 'ui-prototype/index.html'], tool: mut('openspec/changes/chg/ui-prototype/index.html') })).reasonCode === 'ui-chain-out-of-order');
check("B3' 写 skeleton 自身不要求 skeleton 先行 → 放行",
	decideUiToolGate(passGate({ changeDirFiles: ['proposal.md', 'ui-design.md'], tool: mut('openspec/changes/chg/ui-prototype/skeleton/index.html') })).action === 'allow');
// 合同 block 时「修当前 change 目录」本就被放行（既有设计），故这一条直测纯函数
check("B4' 八节不齐时合同本身 block（依赖链不重复查 L0）",
	isBlock(evaluateUiContract(contract({
		uiDesignText: '<!-- ui-impact: major -->\n<!-- ui-approval: approved -->\n<!-- ui-prototype: ui-prototype/index.html -->\n\n## User Journey\nx\n## Acceptance\ny',
	})), 'ui-design-missing'));
check("B5' 写 fixture 但无 index.html → block ui-chain-out-of-order",
	decideUiToolGate(passGate({
		uiDesignText: UI_MAJOR_FULL('approved', 'ui-prototype/skeleton/index.html'),
		changeDirFiles: ['proposal.md', 'ui-design.md', 'ui-prototype/skeleton/index.html'],
		tool: mut('openspec/changes/chg/ui-prototype/fixture.json'),
	})).reasonCode === 'ui-chain-out-of-order');
check("B6' 有 index.html 时写 fixture → 放行",
	decideUiToolGate(passGate({ tool: mut('openspec/changes/chg/ui-prototype/fixture.json') })).action === 'allow');
check("B7' ui-skip: skeleton(理由) 可跳过骨架环",
	decideUiToolGate(passGate({
		uiDesignText: UI_MAJOR_FULL('approved', 'ui-prototype/index.html') + '\n<!-- ui-skip: skeleton(纯样式微调，无新增区块) -->',
		changeDirFiles: ['proposal.md', 'ui-design.md'],
		tool: mut('openspec/changes/chg/ui-prototype/index.html'),
	})).action === 'allow');
check("B8' ui-skip 理由为空不算豁免 → 仍 block",
	decideUiToolGate(passGate({
		uiDesignText: UI_MAJOR_FULL('approved', 'ui-prototype/index.html') + '\n<!-- ui-skip: skeleton() -->',
		changeDirFiles: ['proposal.md', 'ui-design.md', 'ui-prototype/index.html'],
		tool: mut('openspec/changes/chg/ui-prototype/index.html'),
	})).reasonCode === 'ui-chain-out-of-order');
check("B10' 非原型路径不受依赖链影响",
	decideUiToolGate(passGate({ changeDirFiles: ['proposal.md', 'ui-design.md'], tool: mut('openspec/changes/chg/design.md') })).action === 'allow');

// ---------- §C/§D extension 集成 + telemetry ----------

const EVENTS_DDL = [
	'CREATE TABLE IF NOT EXISTS events (id INTEGER PRIMARY KEY AUTOINCREMENT, ts TEXT NOT NULL, session_id TEXT NOT NULL, kind TEXT NOT NULL, change TEXT, payload TEXT NOT NULL)',
	'CREATE INDEX IF NOT EXISTS idx_ev_session ON events(session_id, id)',
	'CREATE INDEX IF NOT EXISTS idx_ev_change ON events(change, id)',
	'CREATE INDEX IF NOT EXISTS idx_ev_kind_ts ON events(kind, ts)',
];
// 阶段族语义（stage-driven-constraint-injection D2/D7）：implementation 族 = implement/verify/archive；
// planning 族 = explore/propose/design。事实库写 stage.set（读方双读兼容旧 mode.set）。
function seedStageSet(cwd, sessionId, family, boundChange) {
	const dbDir = path.join(cwd, '.pi', 'harness');
	fs.mkdirSync(dbDir, { recursive: true });
	const db = new DatabaseSync(path.join(dbDir, 'events.db'));
	// harness-log 归属守卫：登记应用魔数 + schema 版本，否则 openDb 视为他人库拒写
	db.exec(`PRAGMA application_id = ${0x53594e54}`);
	db.exec('PRAGMA user_version = 1');
	for (const ddl of EVENTS_DDL) db.exec(ddl);
	const stage = family === 'implementation' ? 'implement' : 'propose';
	db.prepare('INSERT INTO events (ts, session_id, kind, change, payload) VALUES (?, ?, ?, ?, ?)')
		.run(new Date().toISOString(), sessionId, 'stage.set', boundChange, JSON.stringify({ stage, stageFamily: family, boundChange, source: 'command' }));
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
	'design.md': '## Decision\nx\n\n## 核心逻辑（伪代码）\n\nfn() {}\n',
	'ui-prototype/index.html': '<html>proto</html>',
	'ui-prototype/skeleton/index.html': '<html>skeleton</html>',
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
async function runTool(mod, cwd, sessionId, toolName, input, family = 'implementation', boundChange = 'major-chg', ctxOverride = null) {
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
		// C1 major pending：具备写能力的实现子线程派发被阻断 + block 事件（change 绑定）
		// （add-ui-prototype-editor-agent 后只读档/原型编辑档派发放行，阻断用例改用写能力档 general-purpose）
		const t1 = mktmp(); tmps.push(t1);
		writeChange(t1, 'major-chg', MAJOR_PENDING_FILES);
		seedStageSet(t1, 's1', 'implementation', 'major-chg');
		const r1 = await runTool(loadFresh('./.uige.cjs'), t1, 's1', 'Agent', { prompt: 'x', subagent_type: 'general-purpose' });
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
		// D3 放行锚点（add-change-lifecycle-retro D2）：批准前有未解除的 block(ui-approval-pending)
		// → 本次健康放行记 pass(ui-approval-approved) 恰一条，且不与既有 block 双写
		check('D3 批准后健康放行 → pass ui-approval-approved 恰一条（绑定 change）', (() => {
			const rows = policyRows(t1, 'ui-design-gate');
			return rows.length === 3 && rows[2].payload.action === 'pass' && rows[2].payload.reasonCode === 'ui-approval-approved' && rows[2].change === 'major-chg';
		})());
		// D3b 已解除后再次健康放行 → 零新增（健康放行零记录，pass 不重复）
		const r3b = await runTool(loadFresh('./.uige.cjs'), t1, 's1', 'Agent', { prompt: 'x', subagent_type: 'Explore' });
		check('D3b 解除后再健康放行零新增（pass 不重复记）', r3b.result === undefined && policyRows(t1, 'ui-design-gate').length === 3);

		// C3.5 补齐文档放行（ui-design-satisfied）：block(ui-prototype-missing) → 补齐原型 →
		//      新 session 健康放行（跨 session 解除：block 与放行常分属两会话，账本判定可靠）
		const t3b = mktmp(); tmps.push(t3b);
		writeChange(t3b, 'proto-chg', {
			'.openspec.yaml': SCHEMA_UI,
			'proposal.md': PROP_MAJOR,
			'ui-design.md': UI_MAJOR_FULL('approved', 'ui-prototype/broken.html'),
			'design.md': '## 核心逻辑（伪代码）\n\nfn() {}\n',
			'ui-prototype/broken.html': 'x', // 占位：C7 同款断链场景改用真实缺失
		});
		fs.rmSync(path.join(t3b, 'openspec/changes/proto-chg/ui-prototype/broken.html'));
		seedStageSet(t3b, 's-block', 'implementation', 'proto-chg');
		seedStageSet(t3b, 's-release', 'implementation', 'proto-chg');
		const r3c = await runTool(loadFresh('./.uige.cjs'), t3b, 's-block', 'Agent', { prompt: 'x', subagent_type: 'general-purpose' }); // 写能力档：合同 block 态仍阻断
		check('C3.5a 原型缺失 → block ui-prototype-missing', r3c.result && r3c.result.block === true && r3c.result.reason.includes('ui-prototype-missing'));
		// 用户补齐原型（change 目录内 mutation 放行，不触发 pass——verdict 仍为 block）
		fs.writeFileSync(path.join(t3b, 'openspec/changes/proto-chg/ui-prototype/broken.html'), '<html>fixed</html>');
		const r3d = await runTool(loadFresh('./.uige.cjs'), t3b, 's-release', 'Agent', { prompt: 'x', subagent_type: 'Explore' }); // 新 session 解除
		check('C3.5b 补齐后新 session 健康放行 → pass ui-design-satisfied 恰一条', (() => {
			const rows = policyRows(t3b, 'ui-design-gate');
			return r3d.result === undefined && rows.length === 2 && rows[1].payload.action === 'pass' && rows[1].payload.reasonCode === 'ui-design-satisfied' && rows[1].change === 'proto-chg';
		})());
		// C3.5c 无 block 在案的健康放行（none 档）→ 零 pass（首过不记）
		const t3c = mktmp(); tmps.push(t3c);
		writeChange(t3c, 'none2-chg', NONE_OK_FILES);
		seedStageSet(t3c, 's-none2', 'implementation', 'none2-chg');
		const r3e = await runTool(loadFresh('./.uige.cjs'), t3c, 's-none2', 'Agent', { prompt: 'x', subagent_type: 'Explore' });
		check('C3.5c 无 block 在案 → 健康放行零 pass', r3e.result === undefined && policyRows(t3c, 'ui-design-gate').length === 0);

		// C4 requirements 档静默：同 change 任意工具零介入零记账
		const t4 = mktmp(); tmps.push(t4);
		writeChange(t4, 'major-chg', MAJOR_PENDING_FILES);
		seedStageSet(t4, 's4', 'planning', 'major-chg');
		const r4 = await runTool(loadFresh('./.uige.cjs'), t4, 's4', 'Agent', { prompt: 'x', subagent_type: 'Explore' });
		check('C4 requirements 档 → 静默放行零记账', r4.result === undefined && policyRows(t4).length === 0);

		// C5 none 档合同齐备：front mutation 放行（零噪声）
		const t5 = mktmp(); tmps.push(t5);
		writeChange(t5, 'none-chg', NONE_OK_FILES);
		seedStageSet(t5, 's5', 'implementation', 'none-chg');
		const r5 = await runTool(loadFresh('./.uige.cjs'), t5, 's5', 'edit', { path: path.join(t5, 'backend-go/x.go'), edits: [] });
		check('C5 none 合同齐备 → 放行零记账', r5.result === undefined && policyRows(t5).length === 0);

		// C6 legacy：front mutation → warn 一次（policy warn）；同 session 第二次零新增；非 front 零提醒
		const t6 = mktmp(); tmps.push(t6);
		writeChange(t6, 'legacy-chg', { '.openspec.yaml': SCHEMA_LEGACY, 'proposal.md': '## Why\nx' });
		seedStageSet(t6, 's6', 'implementation', 'legacy-chg');
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
		seedStageSet(t7, 's7', 'implementation', 'sym-chg');
		const r7 = await runTool(loadFresh('./.uige.cjs'), t7, 's7', 'Agent', { prompt: 'x', subagent_type: 'general-purpose' });
		check('C7 断链 symlink 原型 → block ui-prototype-missing', r7.result && r7.result.block === true && r7.result.reason.includes('ui-prototype-missing'));

		// C8 内部异常 → fail-open 放行 + 显式告警 + fail-open 记账（不伪装成功）；
		//    模拟「check 内部 bug 但上下文可读」：getSessionId 首调抛错、auditPolicy 复调恢复
		const t8 = mktmp(); tmps.push(t8);
		seedStageSet(t8, 's8', 'implementation', 'major-chg');
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
		seedStageSet(t9, 's9', 'implementation', 'major-chg');
		const out9 = JSON.parse(execSync(`node -e ${JSON.stringify([
			`process.env.UI_DESIGN_GATE_BYPASS='1';`,
			`const mod=require(${JSON.stringify(path.resolve(__dirname, '.uige.cjs'))});`,
			`const h={};const msgs=[];const pi={on:(n,f)=>(h[n]=f),sendMessage:(m)=>msgs.push(m)};mod.default(pi);`,
			`h['tool_call']({toolName:'Agent',input:{prompt:'x'},toolCallId:'t'},{cwd:${JSON.stringify(t9)},sessionManager:{getSessionId:()=>'s9'}})`,
			`.then((r)=>console.log(JSON.stringify({pass:r===undefined,msgs:msgs.length})));`,
		].join(''))}`, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim());
		check('C9 bypass → 放行 + 显式告警不静默', out9.pass === true && out9.msgs === 1);
		check('D6 bypass → policy bypass explicit-bypass', (() => { const rows = policyRows(t9, 'ui-design-gate'); return rows.length === 1 && rows[0].payload.action === 'bypass' && rows[0].payload.reasonCode === 'explicit-bypass'; })());

		// C10 提取不到阶段（无 stage.set）→ 完全静默
		const t10 = mktmp(); tmps.push(t10);
		writeChange(t10, 'major-chg', MAJOR_PENDING_FILES);
		fs.mkdirSync(path.join(t10, '.pi/harness'), { recursive: true });
		const r10 = await runTool(loadFresh('./.uige.cjs'), t10, 's10', 'Agent', { prompt: 'x', subagent_type: 'Explore' });
		check('C10 无 stage.set 记录 → 静默放行零记账', r10.result === undefined && policyRows(t10).length === 0);

		// D7 reasonCode 白名单与 spec 九值对齐
		const expected = ['ui-impact-missing', 'ui-impact-mismatch', 'ui-design-missing', 'ui-prototype-missing', 'ui-approval-pending', 'ui-verification-missing', 'ui-dispatch-allowed', 'ui-pseudocode-missing', 'ui-chain-out-of-order', 'explicit-bypass', 'ui-gate-check-failed'];
		check('D7 UI_REASON_CODES 与 spec 十一值对齐', expected.every((c) => UI_REASON_CODES.includes(c)) && UI_REASON_CODES.length === 11);
		// D7b 放行侧 pass 词汇稳定集（add-change-lifecycle-retro）：两码有界、推导纯函数四分支
		check('D7b UI_PASS_REASON_CODES 稳定集 = ui-approval-approved / ui-design-satisfied',
			UI_PASS_REASON_CODES.length === 2 && UI_PASS_REASON_CODES.includes('ui-approval-approved') && UI_PASS_REASON_CODES.includes('ui-design-satisfied'));
		check('D7c passReasonForRelease：审批码→approved、工具侧四码→satisfied、归档/无/未知→null',
			passReasonForRelease('ui-approval-pending') === 'ui-approval-approved'
			&& passReasonForRelease('ui-design-missing') === 'ui-design-satisfied'
			&& passReasonForRelease('ui-prototype-missing') === 'ui-design-satisfied'
			&& passReasonForRelease('ui-impact-missing') === 'ui-design-satisfied'
			&& passReasonForRelease('ui-impact-mismatch') === 'ui-design-satisfied'
			&& passReasonForRelease('ui-verification-missing') === null
			&& passReasonForRelease(null) === null
			&& passReasonForRelease('some-unknown-code') === null);

		// D8 归档证据检查纯函数速测（规划制品保留项；验收实物对账已下放
		// scripts/harness/ui-verification-trace.sh，详测在 spec-gate.smoke.cjs 8 系真跑真脚本）
		check('D8 missingArchiveUiEvidence：legacy → 空（不硬阻断）', missingArchiveUiEvidence({ schema: 'spec-driven', proposalText: PROP_MAJOR, uiDesignText: null, changeDirFiles: [] }).length === 0);
		const UI_MAJOR_NO_DIFF = UI_MAJOR_FULL('approved', 'ui-prototype/index.html').replace('内容（Acceptance 节含实现与原型差异说明）', '只有普通说明，全篇无那两个字');
		check('D8 missingArchiveUiEvidence：major approved+原型在且无「差异」字样 → 空（tasks 字样/差异字样检查退役）', !UI_MAJOR_NO_DIFF.includes('差异') && missingArchiveUiEvidence({ schema: UI_SCHEMA, proposalText: PROP_MAJOR, uiDesignText: UI_MAJOR_NO_DIFF, changeDirFiles: FILES_WITH_PROTO }).length === 0);
		check('D8b major 审批 pending → 非空（规划制品检查保留）', missingArchiveUiEvidence({ schema: UI_SCHEMA, proposalText: PROP_MAJOR, uiDesignText: UI_MAJOR_FULL('pending', 'ui-prototype/index.html'), changeDirFiles: FILES_WITH_PROTO }).length > 0);
		check('D8b minor 档 → 空（映射实物对账全在脚本）', missingArchiveUiEvidence({ schema: UI_SCHEMA, proposalText: '<!-- ui-impact: minor -->\n## Why\nx', uiDesignText: '<!-- ui-impact: minor -->\n<!-- ui-approval: not-required -->\n<!-- ui-prototype: none -->\n\n## Acceptance\n待定', changeDirFiles: [] }).length === 0);

		// §E 派发放行与写侧不放宽（add-ui-prototype-editor-agent）
		//  E1 只读档派发在合同未就绪时放行并记 ui-dispatch-allowed（explore/plan/change-review）
		//  E2 原型编辑档（ui-design）派发放行并留痕
		//  E3 写能力档（general-purpose / 未指定档位）派发仍 block
		//  E4 原型档在子线程会话内 edit front/ 下文件 → 仍 block（写侧不放宽）
		//  E5 白名单漂移防护：每一项在 .pi/agents/ 有同名文件；通用实现档不在白名单
		//  E5c 原型编辑档 profile 前提断言（load_extensions: true + tools 含 edit/write）
		//  E6 反向漂移防护：.pi/agents/ 中无写工具的 profile 必须都在白名单内
		const tE = mktmp(); tmps.push(tE);
		writeChange(tE, 'major-chg', MAJOR_PENDING_FILES);
		seedStageSet(tE, 'sE', 'implementation', 'major-chg');
		const mE = loadFresh('./.uige.cjs');
		const rE1 = await runTool(mE, tE, 'sE', 'Agent', { prompt: 'x', subagent_type: 'explore' });
		check('E1a 只读档 explore 派发（合同未就绪）→ 放行并留痕 ui-dispatch-allowed', rE1.result === undefined && (() => { const rows = policyRows(tE, 'ui-design-gate'); return rows.length === 1 && rows[0].payload.action === 'warn' && rows[0].payload.reasonCode === 'ui-dispatch-allowed' && rows[0].change === 'major-chg'; })());
		const rE1b = await runTool(mE, tE, 'sE', 'Agent', { prompt: 'x', subagent_type: 'Explore' });
		check('E1b 只读档派发大小写不敏感（Explore ≡ explore）→ 同样放行留痕', rE1b.result === undefined && policyRows(tE, 'ui-design-gate').length === 2);
		const rE1c = await runTool(mE, tE, 'sE', 'Agent', { prompt: 'x', subagent_type: 'change-review' });
		check('E1c 只读档 change-review 派发 → 放行并留痕', rE1c.result === undefined && policyRows(tE, 'ui-design-gate').length === 3);
		const rE2 = await runTool(mE, tE, 'sE', 'Agent', { prompt: 'x', subagent_type: 'ui-design' });
		check('E2 原型编辑档 ui-design 派发 → 放行并留痕 ui-dispatch-allowed', rE2.result === undefined && (() => { const rows = policyRows(tE, 'ui-design-gate'); return rows.length === 4 && rows[3].payload.action === 'warn' && rows[3].payload.reasonCode === 'ui-dispatch-allowed' && rows[3].change === 'major-chg'; })());
		const rE3 = await runTool(mE, tE, 'sE', 'Agent', { prompt: 'x', subagent_type: 'general-purpose' });
		check('E3 写能力档 general-purpose 派发（合同未就绪）→ 仍 block', rE3.result && rE3.result.block === true && rE3.result.reason.includes('ui-approval-pending'));
		const rE3b = await runTool(mE, tE, 'sE', 'Agent', { prompt: 'x' }); // 未指定档位 = 默认 general-purpose
		check('E3b 未指定档位的派发（默认 general-purpose）→ 仍 block', rE3b.result && rE3b.result.block === true);
		// 子线程会话（档位继承 parent）：原型编辑档的越界写入仍被门禁逐个阻断（写侧不放宽）
		seedStageSet(tE, 'sE-child', 'implementation', 'major-chg');
		const rE4 = await runTool(mE, tE, 'sE-child', 'edit', { path: path.join(tE, 'front/app/pages/x.vue'), edits: [] });
		check('E4 原型档子线程会话内 edit front/ 下文件 → 仍 block（写侧不放宽）', rE4.result && rE4.result.block === true && rE4.result.reason.includes('major-chg'));
		const rE4b = await runTool(mE, tE, 'sE-child', 'edit', { path: path.join(tE, 'openspec/changes/major-chg/ui-prototype/index.html'), edits: [] });
		check('E4b 原型档子线程会话内改当前 change 的 ui-prototype/** → 放行', rE4b.result === undefined);
		// 漂移防护：白名单每一项在 .pi/agents/ 有同名文件
		const agentsDir = path.join(__dirname, '..', '..', 'agents');
		const agentFiles = fs.readdirSync(agentsDir);
		check('E5 白名单每一项在 .pi/agents/ 有同名文件（漂移防护）', UI_DISPATCH_ALLOWED_AGENTS.length > 0 && UI_DISPATCH_ALLOWED_AGENTS.every((a) => agentFiles.includes(`${a}.md`)));
		check('E5b 通用实现档 general-purpose 不在放行白名单（写能力档仍被拦）', !UI_DISPATCH_ALLOWED_AGENTS.includes('general-purpose'));

		// E5c 原型编辑档 profile 前提断言（delta spec 场景「原型编辑档带扩展派发后写入受门禁校验」的前提）：
		// profile 必须声明 load_extensions: true 且 tools 含 edit/write——删掉任一项，子线程就不再加载
		// UI 门禁扩展，写侧零执行，核心安全命题静默失效（字段名漂移须以实测暴露而非静默）。
		const profileFrontmatter = (name) => {
			try {
				const text = fs.readFileSync(path.join(agentsDir, `${name}.md`), 'utf8');
				return /^---\r?\n([\s\S]*?)\r?\n---/.exec(text)?.[1] ?? '';
			} catch {
				return '';
			}
		};
		const profileTools = (name) => {
			const m = /^\s*tools:\s*(.+)$/m.exec(profileFrontmatter(name));
			return m ? m[1].split(',').map((t) => t.trim()).filter(Boolean) : [];
		};
		const uiDesignFm = profileFrontmatter('ui-design');
		const uiDesignTools = profileTools('ui-design');
		const missingProfileBits = [];
		if (!agentFiles.includes('ui-design.md')) missingProfileBits.push('.pi/agents/ui-design.md 存在');
		if (!/^\s*load_extensions:\s*true\s*$/m.test(uiDesignFm)) missingProfileBits.push('load_extensions: true');
		if (!uiDesignTools.includes('edit')) missingProfileBits.push('tools 含 edit');
		if (!uiDesignTools.includes('write')) missingProfileBits.push('tools 含 write');
		check(`E5c 原型编辑档 profile（.pi/agents/ui-design.md）声明 load_extensions: true 且 tools 含 edit/write${missingProfileBits.length ? `——缺：${missingProfileBits.join('、')}` : ''}`, missingProfileBits.length === 0);

		// E6 反向漂移防护：.pi/agents/ 中「无写工具」的 profile 必须都在放行白名单内——
		// 后续新增只读档忘记同步白名单会被误拦（fail-closed 方向，但与本 change 要解决的「误伤」同类）。
		// 豁免：白名单内的写能力档（ui-design 含 edit/write）按设计豁免——写边界由写侧门禁逐个兜住（D4）。
		const WRITE_CAPABLE_EXEMPT = ['ui-design'];
		const readOnlyProfiles = agentFiles
			.filter((f) => f.endsWith('.md'))
			.map((f) => f.replace(/\.md$/, ''))
			.filter((n) => !WRITE_CAPABLE_EXEMPT.includes(n))
			.filter((n) => !profileTools(n).some((t) => t === 'edit' || t === 'write'));
		const notWhitelisted = readOnlyProfiles.filter((n) => !UI_DISPATCH_ALLOWED_AGENTS.includes(n));
		check(`E6 反向漂移防护：.pi/agents/ 中无写工具的档位（${readOnlyProfiles.join('、') || '无'}）都在放行白名单内${notWhitelisted.length ? `——待同步档名：${notWhitelisted.join('、')}` : ''}`, notWhitelisted.length === 0);
	// §G stage 记账集成（add-ui-stage-accounting D1/D4）：按路径触发 + 只 warn 不 block
	{
		const UI_STAGE_DOC = (stage) =>
			[
				'<!-- ui-impact: major -->',
				'<!-- ui-approval: pending -->',
				'<!-- ui-prototype: ui-prototype/index.html -->',
				`<!-- ui-stage: ${stage} -->`,
				'',
				MAJOR_SECTIONS,
			].join('\n');

		// G1 requirements 档（无 implementation）写 ui-prototype → 按路径记账且不阻断
		const tG = mktmp();
		tmps.push(tG);
		writeChange(tG, 'stage-chg', {
			'.openspec.yaml': SCHEMA_UI,
			'proposal.md': PROP_MAJOR,
			'ui-design.md': UI_STAGE_DOC('skeleton'),
			'ui-prototype/skeleton/a.html': '<html>s</html>',
		});
		seedStageSet(tG, 'sG', 'planning', 'stage-chg');
		// 同一模块实例代表真实会话内的扩展进程（loadFresh 每次会重置内存去重集合）
		const extG = loadFresh('./.uige.cjs');
		const g1 = await runTool(extG, tG, 'sG', 'write', {
			path: 'openspec/changes/stage-chg/ui-prototype/index.html',
			content: 'x',
		});
		const gRows = policyRows(tG, 'ui-design-gate');
		check(
			'G1 requirements 档写原型越域（stage=skeleton）→ ui-stage-scope-out warn 且不阻断',
			g1.result === undefined &&
				gRows.length === 1 &&
				gRows[0].payload.action === 'warn' &&
				gRows[0].payload.reasonCode === 'ui-stage-scope-out' &&
				gRows[0].change === 'stage-chg',
		);

		const g2 = await runTool(extG, tG, 'sG', 'write', {
			path: 'openspec/changes/stage-chg/ui-prototype/skeleton/b.html',
			content: 'x',
		});
		check('G2 skeleton 合规写入 → 零新增记账', g2.result === undefined && policyRows(tG, 'ui-design-gate').length === 1);

		const g3 = await runTool(extG, tG, 'sG', 'write', {
			path: 'openspec/changes/stage-chg/ui-prototype/x.html',
			content: 'x',
		});
		check('G3 同 session × change × 码 去重 → 仍 1 条', g3.result === undefined && policyRows(tG, 'ui-design-gate').length === 1);

		// G4 legacy：无 marker → ui-stage-missing，不要求补标记
		const tL = mktmp();
		tmps.push(tL);
		writeChange(tL, 'legacy-stage', {
			'.openspec.yaml': SCHEMA_UI,
			'proposal.md': PROP_MAJOR,
			'ui-design.md': UI_MAJOR_FULL('pending', 'ui-prototype/index.html').replace(/<!-- ui-stage: [a-z]+ -->\n/, ''),
			'ui-prototype/index.html': '<html>p</html>',
			'ui-prototype/skeleton/index.html': '<html>s</html>',
		});
		seedStageSet(tL, 'sL', 'planning', 'legacy-stage');
		const g4 = await runTool(loadFresh('./.uige.cjs'), tL, 'sL', 'write', {
			path: 'openspec/changes/legacy-stage/ui-prototype/i.html',
			content: 'x',
		});
		const lRows = policyRows(tL, 'ui-design-gate');
		check(
			'G4 无 ui-stage marker → ui-stage-missing warn 且不阻断',
			g4.result === undefined && lRows.length === 1 && lRows[0].payload.reasonCode === 'ui-stage-missing',
		);

		// G5 非法取值 → ui-stage-invalid
		const tI = mktmp();
		tmps.push(tI);
		writeChange(tI, 'bad-stage', {
			'.openspec.yaml': SCHEMA_UI,
			'proposal.md': PROP_MAJOR,
			'ui-design.md': UI_STAGE_DOC('design'),
			'ui-prototype/index.html': '<html>p</html>',
			'ui-prototype/skeleton/index.html': '<html>s</html>',
		});
		seedStageSet(tI, 'sI', 'planning', 'bad-stage');
		const g5 = await runTool(loadFresh('./.uige.cjs'), tI, 'sI', 'write', {
			path: 'openspec/changes/bad-stage/ui-prototype/i.html',
			content: 'x',
		});
		const iRows = policyRows(tI, 'ui-design-gate');
		check(
			'G5 非法 stage 取值 → ui-stage-invalid warn',
			g5.result === undefined && iRows.length === 1 && iRows[0].payload.reasonCode === 'ui-stage-invalid',
		);

		// G6 approved 阶段：全放行且零 stage 记账
		const tA = mktmp();
		tmps.push(tA);
		writeChange(tA, 'ok-stage', {
			'.openspec.yaml': SCHEMA_UI,
			'proposal.md': PROP_MAJOR,
			'ui-design.md': UI_STAGE_DOC('approved'),
			'ui-prototype/index.html': '<html>p</html>',
			'ui-prototype/skeleton/index.html': '<html>s</html>',
		});
		seedStageSet(tA, 'sA', 'planning', 'ok-stage');
		const g6 = await runTool(loadFresh('./.uige.cjs'), tA, 'sA', 'write', {
			path: 'openspec/changes/ok-stage/ui-prototype/i.html',
			content: 'x',
		});
		check('G6 approved 阶段 → 零 stage 记账', g6.result === undefined && policyRows(tA, 'ui-design-gate').length === 0);

		// G7 防假阳性①：写 ui-design.md 本身不记账（补 marker 的写入读到的是旧内容）
		const g7 = await runTool(extG, tG, 'sG', 'write', {
			path: 'openspec/changes/stage-chg/ui-design.md',
			content: '<!-- ui-stage: interaction -->',
		});
		check('G7 写 ui-design.md（补 marker）→ 零假阳性记账', g7.result === undefined && policyRows(tG, 'ui-design-gate').length === 1);

		// G8 防假阳性②：路径纯正则能解析、但 change 目录不存在 → 不记账
		const tP = mktmp();
		tmps.push(tP);
		const g8 = await runTool(extG, tP, 'sG', 'write', {
			path: '/nonexistent-root/openspec/changes/ghost/ui-prototype/a.html',
			content: 'x',
		});
		check('G8 仓库外路径（ghost change 不存在）→ 不记账', g8.result === undefined && policyRows(tP, 'ui-design-gate').length === 0);

		// G9 去重跨 session（子线程共用模块实例）：换 session 不新增
		seedStageSet(tG, 'sG2', 'planning', 'stage-chg');
		const g9 = await runTool(extG, tG, 'sG2', 'write', {
			path: 'openspec/changes/stage-chg/ui-prototype/y.html',
			content: 'x',
		});
		check('G9 换 session 同 change 同码 → 仍 1 条（去重键不含 sessionId）', g9.result === undefined && policyRows(tG, 'ui-design-gate').length === 1);
	}

	} catch (e) {
		console.error('FAIL', e);
		check('smoke 执行不抛异常', false);
	} finally {
		for (const t of tmps) fs.rmSync(t, { recursive: true, force: true });
	}

	// §F ui-stage 记账（add-ui-stage-accounting D1–D4）：纯函数三件套 + warn 码不相交
	{
		check('F1 无 ui-stage marker → missing', parseUiStage('<!-- ui-impact: major -->').kind === 'missing');
		check('F1 非法入参（null/空串）→ missing', parseUiStage(null).kind === 'missing' && parseUiStage('').kind === 'missing');
		check(
			'F1 五档取值均合法',
			UI_STAGES.every((s) => {
				const r = parseUiStage(`<!-- ui-stage: ${s} -->`);
				return r.kind === 'ok' && r.stage === s;
			}),
		);
		check('F1 词表外取值 → invalid', parseUiStage('<!-- ui-stage: design -->').kind === 'invalid');
		check('F1 重复 marker → invalid（歧义）', parseUiStage('<!-- ui-stage: l0 -->\n<!-- ui-stage: data -->').kind === 'invalid');
		check('F1 marker 内空白容忍', (() => { const r = parseUiStage('前\n<!--  ui-stage :  skeleton  -->\n后'); return r.kind === 'ok' && r.stage === 'skeleton'; })());

		check('F2 posix 路径解析 change 名', changeNameFromPath('openspec/changes/x/ui-prototype/index.html') === 'x');
		check('F2 windows 分隔符同等解析', changeNameFromPath('openspec\\changes\\y\\ui-design.md') === 'y');
		check('F2 绝对路径解析', changeNameFromPath('/home/u/repo/openspec/changes/z/tasks.md') === 'z');
		check('F2 archive 目录 → null（不记账）', changeNameFromPath('openspec/changes/archive/2026-01-01-x/tasks.md') === null);
		check('F2 非 change 路径 → null', changeNameFromPath('openspec/specs/foo/spec.md') === null && changeNameFromPath('front/app/x.vue') === null);

		check('F3 change 内相对路径', relInsideChange('openspec/changes/x/ui-prototype/a.html', 'x') === 'ui-prototype/a.html');
		check('F3 绝对路径下的相对段', relInsideChange('/r/openspec/changes/x/ui-design.md', 'x') === 'ui-design.md');
		check('F3 属别的 change → null', relInsideChange('openspec/changes/y/a.md', 'x') === null);

		const scopeCases = [
			['l0', 'ui-design.md', true],
			['l0', 'tasks.md', true],
			['l0', 'human-feedback.md', true],
			['l0', 'ui-prototype/index.html', false],
			['l0', 'ui-prototype/skeleton/a.html', false],
			['skeleton', 'tasks.md', true],
			['skeleton', 'ui-prototype/skeleton/a.html', true],
			['skeleton', 'ui-prototype/index.html', false],
			['skeleton', 'ui-prototype/fixture.json', false],
			['interaction', 'ui-prototype/index.html', true],
			['interaction', 'ui-prototype/fixture.json', true],
			['data', 'ui-prototype/fixture.json', true],
			['approved', 'ui-prototype/index.html', true],
		];
		const scopeBad = scopeCases
			.filter(([s, p, ok]) => (checkStageScope(s, p) === null) !== ok)
			.map(([s, p]) => `${s}:${p}`);
		check(`F4 stage 写域表驱动（${scopeCases.length} 组）${scopeBad.length ? `——不符：${scopeBad.join('、')}` : ''}`, scopeBad.length === 0);

		const overlap = UI_STAGE_WARN_REASON_CODES.filter((c) => UI_REASON_CODES.includes(c));
		check(`F5 三个 stage warn 码不与阻断码白名单相交${overlap.length ? `——交叠：${overlap.join('、')}` : ''}`, overlap.length === 0);
		check('F5 阻断码十一个（ISS-02 扩容 ui-pseudocode-missing / ui-chain-out-of-order）', UI_REASON_CODES.length === 11);
		check('F5 warn 码恰为三个', UI_STAGE_WARN_REASON_CODES.length === 3);
	}

	// §B'' 链按写入路径触发（R-1 修复）：planning 族 / pending 期也要拦
	const chainTmp = mktmp(); // 本段自管（§C 的 tmps 尚未定义）
	writeChange(chainTmp, 'chain-chg', {
		'.openspec.yaml': SCHEMA_UI,
		'proposal.md': PROP_MAJOR,
		'ui-design.md': UI_MAJOR_FULL('pending', 'ui-prototype/index.html'),
		'design.md': '## 核心逻辑（伪代码）\n\nfn() {}\n',
		'ui-prototype/index.html': '<html>p</html>',
	});
	{
		// planning 族（原型真正的生产阶段）+ 无 skeleton → 必须 block
		seedStageSet(chainTmp, 'sc-plan', 'planning', 'chain-chg');
		const rPlan = await runTool(loadFresh('./.uige.cjs'), chainTmp, 'sc-plan', 'write', {
			path: 'openspec/changes/chain-chg/ui-prototype/index.html', content: 'x',
		});
		check("B11'' planning 族缺骨架写 index.html → block（R-1 修复前放行）",
			rPlan.result && rPlan.result.block === true && rPlan.result.reason.includes('ui-chain-out-of-order'));

		// pending 期（合同本就是 block）→ 仍要 block 链
		seedStageSet(chainTmp, 'sc-pend', 'implementation', 'chain-chg');
		const rPend = await runTool(loadFresh('./.uige.cjs'), chainTmp, 'sc-pend', 'write', {
			path: 'openspec/changes/chain-chg/ui-prototype/fixture.json', content: '{}',
		});
		check("B12'' pending 期直接写 fixture → block（不再走「change 内放行」）",
			rPend.result && rPend.result.block === true && rPend.result.reason.includes('ui-chain-out-of-order'));

		// 补骨架后同一路径放行（目录可能已被前面的 rmSync 场景清掉，先建）
		fs.mkdirSync(path.join(chainTmp, 'openspec/changes/chain-chg/ui-prototype/skeleton'), { recursive: true });
		fs.writeFileSync(path.join(chainTmp, 'openspec/changes/chain-chg/ui-prototype/skeleton/index.html'), '<html>s</html>');
		const rOk = await runTool(loadFresh('./.uige.cjs'), chainTmp, 'sc-plan', 'write', {
			path: 'openspec/changes/chain-chg/ui-prototype/index.html', content: 'x',
		});
		check("B13'' 补骨架后 planning 族写 index.html → 放行",
			!rOk.result || rOk.result.block !== true);

		// 非原型路径不受影响
		const rOther = await runTool(loadFresh('./.uige.cjs'), chainTmp, 'sc-plan', 'write', {
			path: 'openspec/changes/chain-chg/design.md', content: 'x',
		});
		check("B14'' 非原型路径不触发链", !rOther.result || rOther.result.block !== true);
	}

	let fail = 0;
	for (const [name, ok] of checks) {
		console.log(`${ok ? '✅' : '❌'} ${name}`);
		if (!ok) fail++;
	}
	console.log(fail ? `\n${fail} 项失败` : '\nSMOKE OK');
	if (fail) process.exitCode = 1;
})();
