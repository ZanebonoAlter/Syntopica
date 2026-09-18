// constraint-injection smoke（Syntopica 版）：stub pi API，以真实事件形状回放
// 「读实现类 skill → 切档 → 绑定 change → before_agent_start 注入（声明/节级/粘性/JIT）→ pin_finding 三级落点」。
// 由 run-smoke.sh 调用（先 esbuild 产出 .bundle.cjs）。断言失败 exit 1。
//
// fixture change（openspec/changes/smoke-fixture-constraint）与 research fixture
// 自建自清（finally 删除）——不依赖真实活跃 change（归档节奏会让断言随机漂移）。
// 通用池 docs/research/explore-findings.md 若已存在则备份原文、测后还原。
//
// @constraint-domain-declaration 新增回归面：
// - 业务域声明注入（proposal constraint-domains 标记 → flow 约束节，reason=declaration）
// - change 文本退出关键词命中（fixture proposal 刻意埋 digest/topic/模型 撞车词，断言不注入）
// - ASCII 关键词词边界（cronjob 不命中 cron/job；独立词 cron 命中）
// - JIT 单一真相源（doc-impact-applies 标签扫描，json 无 jitDocs）
// - 无声明 → 不注入 flow 节 + 「无域声明」提示
const fs = require('fs');
const path = require('path');
const os = require('os');
const ext = require('./.bundle.cjs');

const OPENSPEC_CHANGES_DIR_REL = 'openspec/changes';

const REPO_ROOT = path.resolve(__dirname, '../../..');
const FIXTURE_NAME = 'smoke-fixture-constraint';
const FIXTURE = path.join(REPO_ROOT, OPENSPEC_CHANGES_DIR_REL, FIXTURE_NAME);
const RESEARCH_TOPIC = 'smoke-fixture-research';
const RESEARCH_FIXTURE = path.join(REPO_ROOT, 'docs/research', RESEARCH_TOPIC);
const RESEARCH_POOL = path.join(REPO_ROOT, 'docs/research', 'explore-findings.md');

// fixture proposal：声明 daily-report（合法）+ not-a-domain（未知）；正文刻意埋
// harness-facts-tier-a 事故撞车词（digest/topic/模型/摘要/stage）——change 文本已退出
// 关键词命中源，这些词不得触发任何 flow 约束节注入。前端栈信号不含（无 .vue/组件）。
function writeFixture() {
	fs.mkdirSync(FIXTURE, { recursive: true });
	fs.writeFileSync(
		path.join(FIXTURE, 'proposal.md'),
		'# smoke fixture\n\n<!-- constraint-domains: daily-report, not-a-domain -->\n\n' +
			'后端 Controller 与 Service 调整。digest 阶段的 topic 参数与模型摘要截断（stage 判定）需要核对。\n',
	);
	fs.writeFileSync(path.join(FIXTURE, 'tasks.md'), '- [ ] smoke 任务\n');
	fs.writeFileSync(
		path.join(FIXTURE, 'explore-findings.md'),
		'## 告警表结构\n\nalarm_record 按 tenant_id 分区。\n',
	);
}

// 临时 cwd 场景：自建配置 + fixture 文档（doc-impact-applies 标签指向不存在的节），
// 验证「标签 JIT 命中 → 配置的节不存在 → 回落全文」
function makeTempScenario() {
	const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'ci-smoke-'));
	fs.mkdirSync(path.join(tmp, '.pi'), { recursive: true });
	fs.mkdirSync(path.join(tmp, 'docs/reference/standard/backend'), { recursive: true });
	fs.mkdirSync(path.join(tmp, OPENSPEC_CHANGES_DIR_REL, 'smoke-fixture-temp'), { recursive: true });
	fs.writeFileSync(
		path.join(tmp, '.pi', 'constraint-injection.json'),
		JSON.stringify({
			modes: {
				requirements: { label: '需求', commands: ['/opsx-new'], baseDocs: [] },
				implementation: { label: '实现', commands: ['/opsx-apply'], baseDocs: [] },
			},
			keywordDocs: [],
			stackSignals: { frontend: ['.vue'], backend: ['internal/'] },
			indexDoc: 'docs/idx.md',
			findingsFile: 'explore-findings.md',
			findingsDigestThreshold: 6144,
		}),
	);
	// JIT 单一真相源：文档头部标签（section 指向不存在的节 → 回落全文）
	fs.writeFileSync(
		path.join(tmp, 'docs/reference/standard/backend', 'fixture-std.md'),
		'# fixture std\n\n<!-- doc-impact-applies: backend-go/internal/platform/airouter/ | section=不存在的节 -->\n\n' +
			'## 概述\n\nFULLDOC-MARKER 全文独有内容。\n\n## Requirements\n\nREQ-MARKER\n',
	);
	// 节存在但 <minSectionBytes（缺省512）→ 残缺节回退全文（harness-observability-fixes D4；
	// 现场：content-enrichment.md 编辑中间态 133B 节被注入）
	fs.writeFileSync(
		path.join(tmp, 'docs/reference/standard/backend', 'fixture-std2.md'),
		'# fixture std2\n\n<!-- doc-impact-applies: backend-go/internal/platform/airouter/ | section=短节 -->\n\n' +
			'## 概述\n\nSTD2-FULLDOC 全文独有内容。\n\n## 短节\n\nSHORT-SEC 残缺节（编辑中间态）。\n',
	);
	fs.writeFileSync(path.join(tmp, 'docs', 'idx.md'), '# idx\n');
	// 无声明 proposal（验证「无域声明」提示 + 不注入 flow 节）
	fs.writeFileSync(
		path.join(tmp, OPENSPEC_CHANGES_DIR_REL, 'smoke-fixture-temp', 'proposal.md'),
		'# temp fixture\n',
	);
	// 声明域 fixture（D7 关键词通道 + 预算降级用）：声明 kwdom → 关键词命中该域时投递全节
	fs.mkdirSync(path.join(tmp, OPENSPEC_CHANGES_DIR_REL, 'smoke-fixture-budget'), { recursive: true });
	fs.writeFileSync(
		path.join(tmp, OPENSPEC_CHANGES_DIR_REL, 'smoke-fixture-budget', 'proposal.md'),
		'# budget fixture\n\n<!-- constraint-domains: kwdom -->\n\n正文。\n',
	);
	fs.writeFileSync(
		path.join(tmp, OPENSPEC_CHANGES_DIR_REL, 'smoke-fixture-budget', 'explore-findings.md'),
		'## 临时发现A\n\n内容A。\n',
	);
	// @constraint-declaration-redline 红线层三态 fixture：redline-ok（规整加粗节，红线层≥512B）/
	// redline-nobold（列表无加粗 → 零提取回退全节）/ redline-tiny（唯一加粗项红线层<512B → 回退全节）；
	// 结构补白段落保证节本身≥512B（走节级回退而非全文回退）。声明三域的 fixture change 见下。
	const secPad = '（结构补白：验证节级最小字节阈值的填充句，不属于任何约束条目，不含加粗标记。）'.repeat(6);
	fs.mkdirSync(path.join(tmp, 'docs/reference/flow'), { recursive: true });
	const rlOkItems = Array.from({ length: 6 }, (_, i) =>
		`${i + 1}. **REDLINE-OK-${i + 1} 第${i + 1}条自含红线句：约束主体必须满足的边界条件与例外说明**：细节层内容 DETAIL-OK-${i + 1}（不得进入红线层注入）。`,
	).join('\n');
	fs.writeFileSync(
		path.join(tmp, 'docs/reference/flow', 'redline-ok.md'),
		'# redline ok\n\n## 业务约束与不变量\n\n' + rlOkItems + '\n\n## 代码入口\n\n入口说明。\n',
	);
	fs.writeFileSync(
		path.join(tmp, 'docs/reference/flow', 'redline-nobold.md'),
		'# redline nobold\n\n## 业务约束与不变量\n\n1. 无加粗的列表约束句一 NOBOLD-DETAIL-1。\n2. 无加粗的列表约束句二 NOBOLD-DETAIL-2。\n\n' + secPad + '\n\n## 代码入口\n\n入口说明。\n',
	);
	fs.writeFileSync(
		path.join(tmp, 'docs/reference/flow', 'redline-tiny.md'),
		'# redline tiny\n\n## 业务约束与不变量\n\n1. **唯一短红线**：细节层内容 DETAIL-TINY-1，红线层不足 512B 而节超过 512B，因此堆叠细节文本 TINY-PAD-A TINY-PAD-B TINY-PAD-C TINY-PAD-D TINY-PAD-E TINY-PAD-F TINY-PAD-G。\n\n' + secPad + '\n\n## 代码入口\n\n入口说明。\n',
	);
	fs.mkdirSync(path.join(tmp, OPENSPEC_CHANGES_DIR_REL, 'smoke-fixture-decl'), { recursive: true });
	fs.writeFileSync(
		path.join(tmp, OPENSPEC_CHANGES_DIR_REL, 'smoke-fixture-decl', 'proposal.md'),
		'# decl fixture\n\n<!-- constraint-domains: redline-ok, redline-nobold, redline-tiny -->\n\n正文。\n',
	);
	// 事实库记账验证 fixture：两个 ## 标题（其一带 research 锚点，验证 pin.read 解析剥除）
	fs.writeFileSync(
		path.join(tmp, OPENSPEC_CHANGES_DIR_REL, 'smoke-fixture-temp', 'explore-findings.md'),
		'## 临时发现A\n\n内容A。\n\n## 临时发现B <!-- pin:ab12cd34 -->\n\n内容B。\n',
	);
	return tmp;
}

const handlers = {};
let pinTool = null;
// 混合通道（harden-constraint-injection-channel）：动态层经 pi.sendMessage 投递，
// stub 记录全部消息供断言复用；messages 为 append-only，msgCursor 标记已消费位置。
const messages = [];
let msgCursor = 0;
const pi = {
	on: (name, fn) => { (handlers[name] ??= []).push(fn); },
	registerTool: (def) => { if (def.name === 'pin_finding') pinTool = def; },
	sendMessage: (msg, opts) => { messages.push({ msg, opts }); },
};
ext.default(pi);
const ctx = { cwd: REPO_ROOT, hasUI: false };

async function invokePin(params, c = ctx) {
	const r = await pinTool.execute('t', params, null, null, c);
	return r.content[0].text;
}

async function emit(name, event, c = ctx) {
	let r;
	for (const fn of handlers[name] ?? []) r = (await fn(event, c)) ?? r;
	return r;
}
/** 本 turn 新投递的动态层消息正文（未消费部分） */
function dynamicText() {
	return messages.slice(msgCursor).map((x) => x.msg.content).join('\n\n');
}
/** 标记当前消息已消费（下一个动作只看新消息） */
function markMessages() { msgCursor = messages.length; }
/** 仅稳定层（system prompt）——不含动态层消息 */
async function systemPromptOnly(c = ctx) {
	const r = await emit('before_agent_start', { systemPrompt: 'BASE' }, c);
	return (r && r.systemPrompt) || '';
}
/** 稳定层 + 本 turn 新投递动态消息（既有断言复用：内容无论走哪个通道都可见） */
async function systemPrompt(c = ctx) {
	const r = await emit('before_agent_start', { systemPrompt: 'BASE' }, c);
	const sp = (r && r.systemPrompt) || '';
	const dyn = messages.slice(msgCursor).map((x) => x.msg.content).join('\n\n');
	msgCursor = messages.length;
	return dyn ? `${sp}\n\n${dyn}` : sp;
}

/** 链路断言容忍预算降级（harden-harness-policy-and-spill 排查结论）：主场景用真实文档集，
 *  flow 文档演进（如活跃 change 加长约束节）会把多命中叠加场景推进 32K 预算尾部，
 *  tier-b 按信号强度把 keyword/jit 条目降级到占位行（标题保留，永不真丢）——这是正确行为；
 *  预算降级本身另有 fixture 场景（budgetBytes:2048）专测。链路断言只要求条目在（正文或占位）。 */
const present = (sp, marker, heading) =>
	sp.includes(marker) || (sp.includes(heading) && sp.includes('预算已满'));

(async () => {
	writeFixture();
	const poolExisted = fs.existsSync(RESEARCH_POOL);
	const poolOrig = poolExisted ? fs.readFileSync(RESEARCH_POOL, 'utf8') : null;
	const tmp = makeTempScenario();
	try {
		const checks = [];
		const check = (name, ok) => checks.push([name, ok]);

		// 0. 输入提到 fixture change 名（确定性绑定）
		await emit('input', { text: `smoke 绑定 ${FIXTURE_NAME}` });
		// 1. 非 openspec skill（headroom）不激活档位
		await emit('tool_execution_start', {
			toolName: 'read',
			args: { path: '/root/.pi/agent/skills/headroom/SKILL.md' },
		});
		let sp = await systemPrompt();

		check('非 openspec skill 不激活档位', /档位：未激活/.test(sp));
		check('未激活档仅注索引（constraints-index）', sp.includes('### constraints-index.md'));
		check('未激活档不注 flow 约束节', !sp.includes('auxiliary_label_dedupe_sim'));
		check('未激活档不显示活跃变更（无 mtime 兜底错位）', /活跃变更：无/.test(sp));

		// 2. openspec 族 skill 激活：读 openspec-apply-change → 实现/评审档 + 绑定 change。
		//    注意字段是 args（pi ToolExecutionStartEvent.args），不是 input。
		await emit('tool_execution_start', {
			toolName: 'read',
			args: { path: '/mnt/d/project/Syntopica/.agents/skills/openspec-apply-change/SKILL.md' },
		});
		sp = await systemPrompt();

		check('切档为 实现/评审', /档位：实现\/评审/.test(sp));
		check('绑定 fixture change', sp.includes(`活跃变更：${FIXTURE_NAME}（档位绑定）`));
		check('baseDocs: 约束索引注入', sp.includes('### constraints-index.md'));
		check('注入块头部宪法优先标注', sp.includes('与 AGENTS.md 优先级宪法冲突时，以宪法为准'));
		check('explore-findings 注入', sp.includes('📌 探索阶段发现') && sp.includes('alarm_record'));

		// 2.5 业务域声明（@constraint-declaration-redline）：声明 daily-report → 注入其
		//     约束节红线层（首个加粗块逐行 + 细节层取回指引）；proposal 正文撞车词
		//     （digest/topic/模型/摘要/stage）不得注入任何域
		check('声明注入 daily-report 红线层（docList 标记 + 细节层指引尾行）', sp.includes('### daily-report.md') && sp.includes('daily-report.md(红线层)') && sp.includes('细节层：read `docs/reference/flow/daily-report.md`「业务约束与不变量」节'));
		check('域声明头部提示（含未知域忽略）', /域声明：声明域：daily-report（忽略未知域：not-a-domain）/.test(sp));
		check('change 文本撞车词不注入 semantic-board', !sp.includes('### semantic-board.md'));
		check('change 文本撞车词不注入 topic-graph', !sp.includes('### topic-graph.md'));
		check('change 文本撞车词不注入 ai-summary', !sp.includes('### ai-summary.md'));

		// 3. 关键词通道域限定（D7，harden-constraint-injection-channel）：fixture 声明 daily-report，
		//    跨域关键词（「板块」→semantic-board、「cron」→scheduler）命中被拦截——不再注入无关域全节
		await emit('input', { text: '需要调整板块的标签展示' });
		sp = await systemPrompt();
		check('D7: 跨域关键词命中被拦截（semantic-board 不注入）', !sp.includes('### semantic-board.md'));
		await emit('input', { text: 'cronjob 卡住需要修复' });
		sp = await systemPrompt();
		check('D7: 跨域关键词命中被拦截（scheduler 不注入）', !sp.includes('### scheduler.md'));

		// 4. 动态层 diff 稳态零重发（D3）：命中集合未变 → 后续回合零投递（缓存友好）
		const zeroSp = await systemPrompt();
		check('动态层稳态零重发（无新消息投递）', !zeroSp.includes('📎 约束补充'));

		// 5. JIT 单一真相源（doc-impact-applies 标签）：edit airouter 路径 →
		//    ai-logging（Requirements 节）+ ai-summary（业务约束节，标签同样声明 airouter 辖区）
		await emit('tool_execution_start', {
			toolName: 'edit',
			args: { path: 'backend-go/internal/platform/airouter/router.go' },
		});
		sp = await systemPrompt();
		check('JIT: edit airouter 后追加 ai-logging（标签源）', sp.includes('### ai-logging.md'));
		check('JIT 节级注入含 Requirements 内容', sp.includes('所有 AI 调用必须经 airouter'));
		check('JIT 节级注入不含其他节（截断策略节内容）', !sp.includes('prompt / response 完整记录'));
		check('JIT: airouter 同时命中 ai-summary 约束节（标签辖区重叠）', sp.includes('### ai-summary.md'));
		const stableOnly5 = await systemPromptOnly();
		check('JIT 命中经动态层投递（稳定层 systemPrompt 不含 JIT 文档）', !stableOnly5.includes('### ai-logging.md') && !stableOnly5.includes('### ai-summary.md'));

		// 6. pin_finding：档激活时落档位绑定的 change
		const pinC = await invokePin({ title: 'pin-smoke-active', finding: '档内 pin 内容。' });
		check('pin（有档位）落绑定 change', pinC.includes(`已存档到 ${OPENSPEC_CHANGES_DIR_REL}/${FIXTURE_NAME}`) && !pinC.includes('research'));
		check('pin（有档位）写入绑定 change 文件', fs.readFileSync(path.join(FIXTURE, 'explore-findings.md'), 'utf8').includes('pin-smoke-active'));

		// 6.5 session_start reason 过滤（@bugfix）：startup = 子线程派发（pi-subagents
		//     createAgentSession 默认）不清零主会话状态；new/resume/fork/reload 才是真会话边界
		await emit('session_start', { reason: 'startup' });
		sp = await systemPrompt();
		check('session_start(startup=子线程派发) 不清零档位', /档位：实现\/评审/.test(sp));
		// startup 不清零：命中集合与指纹均保留 → 重复触碰同一路径零重投递（集合被清则重投）
		await emit('tool_execution_start', {
			toolName: 'edit',
			args: { path: 'backend-go/internal/platform/airouter/router.go' },
		});
		const afterStartupSp = await systemPrompt();
		check('session_start(startup) 不清零命中集合（重复命中零投递）', !afterStartupSp.includes('📎 约束补充'));

		// 7. session_start 会话边界重置：档位/关键词命中/JIT 命中/输入窗全部归零
		await emit('session_start', { reason: 'new' });
		sp = await systemPrompt();
		check('session_start 后档位回落未激活', /档位：未激活/.test(sp));
		check('session_start 后关键词命中清空', !sp.includes('auxiliary_label_dedupe_sim'));
		check('session_start 后 JIT 命中清空', !sp.includes('### ai-logging.md'));

		// 8. JIT 门控：未激活会话写 airouter 不追加（保「未激活=仅索引」不变量）
		await emit('tool_execution_start', {
			toolName: 'write',
			args: { path: 'backend-go/internal/platform/airouter/router.go' },
		});
		sp = await systemPrompt();
		check('JIT 门控: 未激活时写 airouter 不注入 ai-logging', !sp.includes('### ai-logging.md'));
		check('JIT 门控: 未激活时写 airouter 不注入 ai-summary', !sp.includes('### ai-summary.md'));

		// 9. pin_finding：无档位（research 语境）带 topic → docs/research/<topic>/
		const pinF = await invokePin({ title: 'pin-smoke-research', finding: 'research pin 内容。', topic: RESEARCH_TOPIC });
		check('pin（无档位带 topic）落 research 库', pinF.includes(`docs/research/${RESEARCH_TOPIC}/explore-findings.md`));
		check('pin（无档位）不碰 openspec/changes', !pinF.includes(OPENSPEC_CHANGES_DIR_REL));
		check('pin（无档位带 topic）文件已写入', fs.existsSync(path.join(RESEARCH_FIXTURE, 'explore-findings.md')));
		check(
			'research pin 锚点已写入标题行（8hex）',
			/## pin-smoke-research <!-- pin:[0-9a-f]{8} -->/.test(fs.readFileSync(path.join(RESEARCH_FIXTURE, 'explore-findings.md'), 'utf8')),
		);
		check(
			'change 语境 pin 不加锚点（复合键已够）',
			!/<!-- pin:[0-9a-f]{8} -->/.test(fs.readFileSync(path.join(FIXTURE, 'explore-findings.md'), 'utf8')),
		);

		// 10. pin_finding：无档位无 topic → 通用池单文件 docs/research/explore-findings.md
		const pinP = await invokePin({ title: 'pin-smoke-pool', finding: '通用池 pin。' });
		check('pin（无档位无 topic）落通用池单文件', pinP.includes('docs/research/explore-findings.md') && !pinP.includes(`/${RESEARCH_TOPIC}/`));
		check('pin（无档位无 topic）通用池文件已写入', fs.existsSync(RESEARCH_POOL) && fs.readFileSync(RESEARCH_POOL, 'utf8').includes('pin-smoke-pool'));

		// 11. pin_finding：显式 change 参数优先（即使无档位）
		const pinG = await invokePin({ title: 'pin-smoke-explicit', finding: '显式落点。', change: FIXTURE_NAME });
		check('pin（显式 change）落指定 change', pinG.includes(`已存档到 ${OPENSPEC_CHANGES_DIR_REL}/${FIXTURE_NAME}`));

		// 12.5 explore 新会话场景（@Syntopica bugfix）：读 explore skill → requirements 档，
		//     无 change 提及时不 mtime 兜底绑定（fixture 是 mtime 最新但不应被绑），pin 落 research
		await emit('session_start', { reason: 'new' });
		await emit('tool_execution_start', {
			toolName: 'read',
			args: { path: '/mnt/d/project/Syntopica/.agents/skills/openspec-explore/SKILL.md' },
		});
		sp = await systemPrompt();
		check('explore 档无提及不兜底绑定（活跃变更：无）', /档位：需求\/设计/.test(sp) && /活跃变更：无/.test(sp));
		check('explore 档无绑定时不注 change 探索发现', !sp.includes('alarm_record'));
		const pinR = await invokePin({ title: 'pin-smoke-explore', finding: 'explore 新想法发现。' });
		check('explore 档无绑定时 pin 落 research 库', pinR.includes('docs/research/explore-findings.md'));
		check('explore 档无绑定时 pin 不落 change', !pinR.includes(OPENSPEC_CHANGES_DIR_REL));
		// 输入提及 change 名 → requirements 档正常绑定（/opsx-continue 语境）
		await emit('input', { text: `/opsx-continue ${FIXTURE_NAME}` });
		sp = await systemPrompt();
		check('requirements 档提及 change 名正常绑定', sp.includes(`活跃变更：${FIXTURE_NAME}（档位绑定）`));
		await emit('session_start', { reason: 'new' }); // 收尾重置，不污染后续场景

		// 12. pin_finding：change 参数不存在 → research 库 + 警告
		const pinH = await invokePin({ title: 'pin-smoke-badchange', finding: '坏参数落点。', change: 'no-such-change-xyz' });
		check('pin（change 不存在）改存 research 库并警告', pinH.includes('⚠️') && pinH.includes('docs/research'));

		// 13. 临时 cwd：标签 JIT + 节不存在回落全文 + 无域声明提示。
		//     配置无 jitDocs（已移除）；JIT 命中源 = 文档头部 doc-impact-applies 标签。
		const tmpCtx = {
			cwd: tmp,
			hasUI: false,
			// 事实库记账需要 sessionId（stub）；REPO_ROOT 场景不带 → 不写真实库
			sessionManager: { getSessionId: () => 'ci-smoke-pin' },
		};
		await emit('input', { text: `/opsx-apply smoke-fixture-temp` }, tmpCtx);
		await emit('tool_execution_start', {
			toolName: 'edit',
			args: { path: 'backend-go/internal/platform/airouter/router.go' },
		}, tmpCtx);
		const tsp = await systemPrompt(tmpCtx);
		check('标签 JIT 命中 + 节不存在回落全文注入（FULLDOC-MARKER 可见）', tsp.includes('FULLDOC-MARKER') && tsp.includes('REQ-MARKER'));
		check('残缺节（<512B）回退全文注入（STD2-FULLDOC 可见而非仅短节）', tsp.includes('STD2-FULLDOC'));
		check('残缺节回退不标节级模式（无「短节节」docList 项）', !/fixture-std2\.md\(短节节\)/.test(tsp));
		check('无声明 → 域声明提示（纯工具链 change 可忽略）', /域声明：无域声明（纯工具链 change 可忽略）/.test(tsp));
		check('无声明 → 不注入 flow 约束节', !tsp.includes('业务约束与不变量'));

		// 14. 事实库记账（harness-facts-tier-a）：pin.read 标题解析（含锚点剥除）+
		//     会话内去重 + constraint.inject 命中原因；docList 渲染不受 docEntries 增量影响
		await systemPrompt(tmpCtx); // 第二次注入（每回合重建注入块，验证去重）
		const { DatabaseSync: EvDb } = require('node:sqlite');
		const evDb = new EvDb(path.join(tmp, '.pi', 'harness', 'events.db'), { readOnly: true });
		const pinReads = evDb.prepare("SELECT payload FROM events WHERE kind='pin.read' AND session_id='ci-smoke-pin'").all().map((r) => JSON.parse(r.payload));
		check(
			'pin.read：两个标题各记一次，research 锚点剥除',
			pinReads.length === 2 &&
				pinReads.some((p) => p.title === '临时发现A') &&
				pinReads.some((p) => p.title === '临时发现B'),
		);
		check('pin.read 会话内去重（二次注入不重复记）', evDb.prepare("SELECT COUNT(*) AS n FROM events WHERE kind='pin.read'").get().n === 2);
		const injects = evDb.prepare("SELECT payload FROM events WHERE kind='constraint.inject' AND session_id='ci-smoke-pin'").all().map((r) => JSON.parse(r.payload));
		check(
			'constraint.inject 记账含 change-file 命中原因与字节',
			injects.some((p) => p.reason === 'change-file' && p.path.endsWith('explore-findings.md') && typeof p.bytes === 'number'),
		);
		// 残缺节回退全文记账：mode=full + bytes=实际注入全文字节数（不虚标节字节）
		const std2Full = fs.readFileSync(path.join(tmp, 'docs/reference/standard/backend', 'fixture-std2.md'), 'utf8').trim();
		check(
			'残缺节回退全文记账（mode=full + bytes=全文字节）',
			injects.some((p) => p.path.endsWith('fixture-std2.md') && p.mode === 'full' && p.bytes === Buffer.byteLength(std2Full, 'utf8')),
		);
		check('change 文件经动态层投递（探索发现标题在投递内容中）', tsp.includes('### 📌 探索阶段发现'));
		evDb.close();

		// 14.5 声明域红线层三态（@constraint-declaration-redline）：规整节→红线层注入；
		//      无加粗节/低字节红线层→回退全节；记账 layer 如实。结束绑回 temp（17 的
		//      resume 断言依赖本会话最后一条 mode.set 是 smoke-fixture-temp）
		await emit('input', { text: '/opsx-apply smoke-fixture-decl' }, tmpCtx);
		const declSp = await systemPrompt(tmpCtx);
		check('decl: 规整节红线层注入（首尾红线句逐行可见）', declSp.includes('### redline-ok.md') && declSp.includes('1. **REDLINE-OK-1') && declSp.includes('6. **REDLINE-OK-6'));
		check('decl: 红线层不含细节层内容', !declSp.includes('DETAIL-OK-'));
		check('decl: 红线层附细节层取回指引尾行', declSp.includes('细节层：read `docs/reference/flow/redline-ok.md`「业务约束与不变量」节'));
		check('decl: docList 标红线层', declSp.includes('redline-ok.md(红线层)'));
		check('decl: 无加粗节回退全节（细节可见）', declSp.includes('NOBOLD-DETAIL-2') && declSp.includes('本节摘自'));
		check('decl: 低字节红线层回退全节', declSp.includes('DETAIL-TINY-1') && !declSp.includes('redline-tiny.md(红线层)'));
		const declDb = new EvDb(path.join(tmp, '.pi', 'harness', 'events.db'), { readOnly: true });
		const declRows = declDb.prepare("SELECT payload FROM events WHERE kind='constraint.inject' AND session_id='ci-smoke-pin' AND payload LIKE '%redline-%'").all().map((r) => JSON.parse(r.payload));
		check(
			'decl: 记账 layer 标记（ok=redline / nobold+tiny=full）',
			declRows.some((p) => p.reason === 'declaration' && p.path.endsWith('redline-ok.md') && p.layer === 'redline' && typeof p.bytes === 'number') &&
				declRows.some((p) => p.reason === 'declaration' && p.path.endsWith('redline-nobold.md') && p.layer === 'full') &&
				declRows.some((p) => p.reason === 'declaration' && p.path.endsWith('redline-tiny.md') && p.layer === 'full'),
		);
		check(
			'decl: 红线层字节远小于全节回退（瘦身收益可观测）',
			(declRows.find((p) => p.path.endsWith('redline-ok.md') && p.layer === 'redline') || {}).bytes <
				(declRows.find((p) => p.path.endsWith('redline-nobold.md') && p.layer === 'full') || {}).bytes,
		);
		declDb.close();
		await emit('input', { text: '/opsx-apply smoke-fixture-temp' }, tmpCtx);

		// 17. resume 档位恢复（@constraint-injection-tier-b D6）：两段式取数 + new 不恢复 + 绑定失效回落
		await emit('session_start', { reason: 'new' }, tmpCtx);
		let rsp = await systemPrompt(tmpCtx);
		check('resume前置: new 清零且不恢复（db 有 mode.set 也不继承）', /档位：未激活/.test(rsp));
		await emit('session_start', { reason: 'resume' }, tmpCtx);
		rsp = await systemPrompt(tmpCtx);
		check('resume 同 sessionId 恢复档位（第1段 queryBySession）', /档位：实现/.test(rsp));
		check('resume 恢复 change 绑定', rsp.includes('活跃变更：smoke-fixture-temp'));
		check('resume 不恢复粘性命中（JIT 清空，有意为之）', !rsp.includes('FULLDOC-MARKER'));
		const tmpCtxNew = { cwd: tmp, hasUI: false, sessionManager: { getSessionId: () => 'ci-smoke-resume-2' } };
		await emit('session_start', { reason: 'resume' }, tmpCtxNew);
		rsp = await systemPrompt(tmpCtxNew);
		check('resume 无自身档位历史：不继承他窗口档位（无全局兑底，2026-08-23 实测回归）', /档位：未激活/.test(rsp) && !rsp.includes('smoke-fixture-temp'));
		await emit('session_start', { reason: 'reload' }, tmpCtxNew);
		rsp = await systemPrompt(tmpCtxNew);
		check('reload 无自身档位历史：同样不继承（未激活）', /档位：未激活/.test(rsp));
		const tmpChangeDir = path.join(tmp, OPENSPEC_CHANGES_DIR_REL, 'smoke-fixture-temp');
		fs.renameSync(tmpChangeDir, `${tmpChangeDir}-bak`);
		await emit('session_start', { reason: 'resume' }, tmpCtxNew);
		rsp = await systemPrompt(tmpCtxNew);
		check('resume 绑定 change 已不存在 → 回落未激活', /档位：未激活/.test(rsp));
		fs.renameSync(`${tmpChangeDir}-bak`, tmpChangeDir);
		// startup 双面孔（@constraint-injection-tier-b 6.5 返工：真实链路 quit 重启走 startup）：
		// ① 冷启动（档位空）+ 同 sessionId 有 mode.set → 第 1 段恢复，不全局兜底；
		// ② 子线程派发（档位非空）→ 不清零不动；③ 全新会话 startup → 不继承其他会话档位
		await emit('session_start', { reason: 'startup' }, tmpCtx); // 场景17末尾恢复失败档位空 → 冷启动路径
		rsp = await systemPrompt(tmpCtx);
		check('startup 冷启动：同 sessionId 恢复档位（真实路径：quit 重启）', /档位：实现/.test(rsp) && rsp.includes('smoke-fixture-temp'));
		check('startup 恢复不带回粘性命中（JIT 仍空）', !rsp.includes('FULLDOC-MARKER'));
		await emit('tool_execution_start', { toolName: 'edit', args: { path: 'backend-go/internal/platform/airouter/router.go' } }, tmpCtx);
		await emit('session_start', { reason: 'startup' }, tmpCtx); // 档位非空 = 子线程派发 → 不动
		rsp = await systemPrompt(tmpCtx);
		check('startup 子线程派发：不清零不重恢复（档位/JIT 均保持）', /档位：实现/.test(rsp) && rsp.includes('FULLDOC-MARKER'));
		const freshCtx = { cwd: tmp, hasUI: false, sessionManager: { getSessionId: () => 'ci-smoke-fresh-session' } };
		await emit('session_start', { reason: 'new' }, freshCtx); // 模拟新进程空状态（真实世界新会话=新模块实例）
		await emit('session_start', { reason: 'startup' }, freshCtx); // 冷启动 + 无本会话 mode.set
		rsp = await systemPrompt(freshCtx);
		check('startup 全新会话：不继承其他会话档位（无全局兜底）', /档位：未激活/.test(rsp));
		// reload 恢复：同会话扩展重载，id 不变 → 第 1 段必中
		await emit('session_start', { reason: 'reload' }, tmpCtx);
		rsp = await systemPrompt(tmpCtx);
		check('reload：清零后恢复档位（同 sessionId 第 1 段）', /档位：实现/.test(rsp) && rsp.includes('smoke-fixture-temp'));
		await emit('session_start', { reason: 'new' }); // 收尾重置（共享 ctx 场景隔离）
		// mode.set 记账核验（触发点①input 已在场景13触发）
		const msDb = new EvDb(path.join(tmp, '.pi', 'harness', 'events.db'), { readOnly: true });
		const msRows = msDb.prepare("SELECT payload FROM events WHERE kind='mode.set'").all().map((r) => JSON.parse(r.payload));
		check('mode.set 记账存在（mode+boundChange）', msRows.length >= 1 && msRows.some((p) => p.mode === 'implementation' && p.boundChange === 'smoke-fixture-temp'));
		msDb.close();

		// 18. 预算降级（@constraint-injection-tier-b D1-D4）：分层顺序 / 永不真丢 / 确定性 / 降级记账
		const pad = (marker) => Array.from({ length: 60 }, (_, i) => `${marker} 填充行${i} ${'X'.repeat(40)}`).join('\n');
		const kwRedlines = Array.from({ length: 6 }, (_, i) =>
			`${i + 1}. **KWDOM-REDLINE-${i + 1} 约束主体必须满足的边界条件与例外说明以及适用范围的具体界定与失效处理路径说明**：细节层 ${pad('BIGKW-MARKER').split('\n').join(' ')}`,
		).join('\n');
		fs.writeFileSync(
			path.join(tmp, 'docs/reference/flow', 'kwdom.md'),
			'# kwdom\n\n导语段（占位预览取首行非标题文本）。\n\n## 业务约束与不变量\n\n' + kwRedlines + '\n\n## 代码入口\n\n入口说明。\n',
		);
		fs.writeFileSync(
			path.join(tmp, 'docs/reference/standard/backend', 'bigjit.md'),
			'# bigjit\n\n<!-- doc-impact-applies: backend-go/internal/bigmod/ -->\n\n## 概述\n\n' + pad('BIGJIT-MARKER') + '\n',
		);
		fs.writeFileSync(
			path.join(tmp, '.pi', 'constraint-injection.json'),
			JSON.stringify({
				...JSON.parse(fs.readFileSync(path.join(tmp, '.pi', 'constraint-injection.json'), 'utf8')),
				budgetBytes: 2048,
				keywordDocs: [{ doc: 'docs/reference/flow/kwdom.md', section: '业务约束与不变量', keywords: ['大预算词'] }],
			}),
		);
		await emit('session_start', { reason: 'new' }, tmpCtx);
		await emit('input', { text: '/opsx-apply smoke-fixture-budget' }, tmpCtx);
		await emit('input', { text: '调整大预算词相关展示' }, tmpCtx);
		await emit('tool_execution_start', { toolName: 'edit', args: { path: 'backend-go/internal/bigmod/service.go' } }, tmpCtx);
		let dsp = await systemPrompt(tmpCtx);
		check('预算: keyword 层先降（BIGKW-MARKER 全文消失）', !dsp.includes('BIGKW-MARKER'));
		check('预算: 永不真丢（kwdom 标题占位 + read 指引 + 占位预览）', dsp.includes('### kwdom.md') && dsp.includes('read docs/reference/flow/kwdom.md') && dsp.includes('「业务约束与不变量」节'));
		check('预算: jit 层随后降（BIGJIT-MARKER 全文消失、标题保留）', !dsp.includes('BIGJIT-MARKER') && dsp.includes('### bigjit.md'));
		check('预算: 未配 baseDocs 时索引不注（实现档 baseDocs 为空）', !dsp.includes('### idx.md'));
		check('预算: change-file 未到占位层（findings 全文保留）', dsp.includes('内容A'));
		check('预算: 头部模型可见降级通知', /预算降级/.test(dsp) && /kwdom\.md、bigjit\.md|bigjit\.md、kwdom\.md/.test(dsp));
		const dspStable = await systemPromptOnly(tmpCtx);
		const dsp2 = await systemPrompt(tmpCtx);
		check('预算: 降级确定性（同输入零重投递 + 稳定层字节一致）', !dsp2.includes('📎 约束补充') && dsp2 === dspStable);
		const budDb = new EvDb(path.join(tmp, '.pi', 'harness', 'events.db'), { readOnly: true });
		const degRows = budDb.prepare("SELECT payload FROM events WHERE kind='constraint.inject' AND payload LIKE '%\"degraded\":true%'").all().map((r) => JSON.parse(r.payload));
		check('预算: 降级记账（degraded=true + 降级后字节）', degRows.some((p) => p.path.endsWith('kwdom.md') && typeof p.bytes === 'number'));
		budDb.close();

		// 15. 声明解析纯函数（@constraint-domain-declaration 4.1）
		const parse = ext.parseDomainDeclaration;
		check(
			'解析: 单行多域 + 空白容忍',
			JSON.stringify(parse('x\n<!-- constraint-domains: daily-report ,  topic-graph -->\ny')) === '["daily-report","topic-graph"]',
		);
		check(
			'解析: 多标记块合并 + 首现去重',
			JSON.stringify(parse('<!-- constraint-domains: reading -->\n正文\n<!-- constraint-domains: reading, scheduler -->')) === '["reading","scheduler"]',
		);
		check(
			'解析: 跨行内容合并',
			JSON.stringify(parse('<!-- constraint-domains: daily-report,\n  discovery -->')) === '["daily-report","discovery"]',
		);
		check('解析: 无标记返回空', parse('# title\n\n正文没有标记').length === 0);
		check('解析: 空域名忽略', parse('<!-- constraint-domains: , , daily-report ,, -->').length === 1);

		// 15.5 红线层提取纯函数（@constraint-declaration-redline 2.1）：顶层列表项首个加粗块
		//      逐行；无加粗不凑数；嵌套/引用块不算；0 条返回 null（调用方回退全节）
		const ex = ext.extractRedlines;
		const okRes = ex('引导段落。\n\n1. **红线一**：细节甲。\n2. 无加粗细节凑数项。\n3. **红线二**：细节乙。\n\n> 引用块 **引用加粗** 不算。\n\n  - **嵌套加粗** 不算。\n\n- **无序红线**：细节丙。\n');
		check(
			'红线提取: 规整节逐行提取（保序保编号，跳过无加粗/嵌套/引用）',
			!!okRes && JSON.stringify(okRes.lines) === JSON.stringify(['1. **红线一**', '3. **红线二**', '- **无序红线**']) && okRes.bytes === Buffer.byteLength(okRes.lines.join('\n'), 'utf8'),
		);
		check('红线提取: 无加粗节返回 null', ex('纯段落文本没有任何列表项。\n\n也没有加粗标记。') === null);
		check('红线提取: 空串/仅引用块返回 null', ex('') === null && ex('> **引用**') === null);
		const mid = ex('- 前缀文字 **首个加粗块**：细节');
		check('红线提取: 取行内首个加粗块（前缀文字丢弃）', !!mid && mid.lines[0] === '- **首个加粗块**');

		// 16. 标签扫描（@constraint-domain-declaration 4.4）：9 个 flow 文档标签齐全、
		//     节指向「业务约束与不变量」、所列路径在仓库真实存在
		const tags = ext.scanAppliesTags(REPO_ROOT);
		const flowDomains = ['semantic-board', 'topic-graph', 'daily-report', 'content-enrichment', 'data-enrichment', 'ai-summary', 'discovery', 'reading', 'scheduler'];
		for (const d of flowDomains) {
			const rel = `docs/reference/flow/${d}.md`;
			const t = tags.get(rel);
			check(`标签: ${d}.md 有 doc-impact-applies`, !!t);
			if (t) {
				check(`标签: ${d}.md 节指向业务约束与不变量`, t.section === '业务约束与不变量');
				check(
					`标签: ${d}.md 路径真实存在（${t.signals.length} 条）`,
					t.signals.length > 0 && t.signals.every((s) => fs.existsSync(path.join(REPO_ROOT, s))),
				);
			}
		}
		check('标签: ai-logging.md（standard）命中且节为 Requirements', tags.get('docs/reference/standard/backend/ai-logging.md')?.section === 'Requirements');

		// 16.5 test-design.md JIT 标签（@test-case-design-standard 4.1）：防「标签与正文漂移」
		//     与「信号误配」。注意四信号是子串模式（_test.go 等）而非真实目录，不走
		//     flow 域的 existsSync 校验；命中判定镜像 extension 的 tag.signals.some(s => p.includes(s))。
		const TD_REL = 'docs/reference/standard/shared/test-design.md';
		const tdTag = tags.get(TD_REL);
		check('标签: test-design.md 有 doc-impact-applies', !!tdTag);
		if (tdTag) {
			check(
				'标签: test-design.md 四信号齐全（openspec/changes/、_test.go、.spec.ts、.test.ts）',
				['openspec/changes/', '_test.go', '.spec.ts', '.test.ts'].every((s) => tdTag.signals.includes(s)),
			);
			check('标签: test-design.md 节指向 JIT 注入摘要', tdTag.section === 'JIT 注入摘要');
			check(
				'标签: 编辑 bar_test.go 按既有命中逻辑（path.includes(signal)）命中 test-design.md',
				tdTag.signals.some((s) => 'backend-go/internal/foo/bar_test.go'.includes(s)),
			);
			check(
				'标签: .spec.ts / .test.ts / openspec/changes 路径同样命中',
				['front/app/foo.spec.ts', 'front/tests/foo.test.ts', 'openspec/changes/xyz/tasks.md'].every((p) =>
					tdTag.signals.some((s) => p.includes(s)),
				),
			);
		}
		check(
			'标签: test-design.md 正文真实存在「## JIT 注入摘要」节（标签不与正文漂移）',
			/^## JIT 注入摘要\s*$/m.test(fs.readFileSync(path.join(REPO_ROOT, TD_REL), 'utf8')),
		);

		// 16.6 端到端兜底（@test-case-design-standard 4.1）：激活档位后 edit _test.go
		//     路径 → test-design「JIT 注入摘要」节经真实 JIT 链路注入（防匹配逻辑被改后
		//     数据驱动断言仍绿的盲区）。REPO_ROOT 无 sessionManager → 不写真实事件库。
		await emit('session_start', { reason: 'new' });
		await emit('tool_execution_start', {
			toolName: 'read',
			args: { path: '/mnt/d/project/Syntopica/.agents/skills/openspec-apply-change/SKILL.md' },
		});
		await emit('tool_execution_start', {
			toolName: 'edit',
			args: { path: 'backend-go/internal/foo/bar_test.go' },
		});
		sp = await systemPrompt();
		check('JIT: edit *_test.go 追加 test-design（JIT 注入摘要节）', sp.includes('### test-design.md'));
		check(
			'JIT: 注入内容为摘要节本体（结构性关键词可见）',
			present(sp, '五问句', '### test-design.md') && present(sp, '双轨', '### test-design.md'),
		);
		check('JIT: 摘要节外正文不注入（问句①主文档标题不可见）', !sp.includes('问句①：节拍全吗'));

		// 17. 混合注入通道 + 绑定污染修复（harden-constraint-injection-channel）
		// 17.1 纯函数（可脱离 pi 直跑）
		const rb = ext.resolveBindAction;
		check('D5: read 永不抢绑', rb({ tool: 'read', path: 'openspec/changes/x/a.md', currentBound: 'y', boundExists: true }) === 'noop');
		check('D5: 绑定健康时写其他 change 不抢绑', rb({ tool: 'write', path: 'openspec/changes/x/a.md', currentBound: 'y', boundExists: true }) === 'noop');
		check('D5: 无绑定时写 change 目录兜底绑', rb({ tool: 'write', path: 'openspec/changes/x/a.md', currentBound: null, boundExists: false }) === 'rebind');
		check('D5: 绑定目录消失时兜底绑', rb({ tool: 'edit', path: 'openspec/changes/x/a.md', currentBound: 'y', boundExists: false }) === 'rebind');
		check('D5: turn 锁定拦截绑定切换', ext.shouldApplyBind('rebind', true) === false && ext.shouldApplyBind('rebind', false) === true);
		check(
			'D5: changeNameFromPath（归档不计活跃 change）',
			ext.changeNameFromPath('openspec/changes/foo/tasks.md') === 'foo' &&
				ext.changeNameFromPath('openspec/changes/archive/2026-01-01-x/a.md') === null &&
				ext.changeNameFromPath('backend-go/x.go') === null,
		);
		check(
			'D2: 稳定层 key 仅随档位/绑定变',
			ext.stableSnapshotKey('implementation', 'a') === ext.stableSnapshotKey('implementation', 'a') &&
				ext.stableSnapshotKey('implementation', 'a') !== ext.stableSnapshotKey('implementation', 'b'),
		);
		const dItem = { key: 'd1', heading: '### d1', body: 'B1', relPath: 'd1', short: 'd1', reason: 'keyword', mode: 'section' };
		const dDiff1 = ext.computeDynamicDiff([dItem], new Map());
		check('D3: diff 首次全为增量 + 建指纹', dDiff1.increments.length === 1 && !!dDiff1.fingerprint.get('d1'));
		check('D3: diff 指纹相同零增量（稳态零投递）', ext.computeDynamicDiff([dItem], dDiff1.fingerprint).increments.length === 0);
		check('D3: diff 内容变化为增量', ext.computeDynamicDiff([{ ...dItem, body: 'B2' }], dDiff1.fingerprint).increments.length === 1);
		check('D7: 关键词域过滤剔除集合外命中', ext.filterKeywordDocs([{ doc: 'a' }, { doc: 'b' }], new Set(['a'])).length === 1);
		check(
			'D3: 渲染含宪法尾注 / 空集返回 null',
			/与 AGENTS.md 优先级宪法冲突时，以宪法为准/.test(ext.renderDynamicMessage([dItem]).content) &&
				ext.renderDynamicMessage([]) === null,
		);
		check(
			'D1: splitPlanItems 稳定/动态分层（declaration→稳定，keyword→动态）',
			(() => {
				const sp = ext.splitPlanItems([
					{ key: 'a', reason: 'mode-base' },
					{ key: 'b', reason: 'declaration' },
					{ key: 'c', reason: 'keyword' },
					{ key: 'd', reason: 'change-file' },
				]);
				return sp.stable.length === 2 && sp.dynamic.length === 2;
			})(),
		);

		// 17.2 行为：混合通道（tmp 场景，预算充足 + 声明域关键词）
		const tmpCfgPath = path.join(tmp, '.pi', 'constraint-injection.json');
		const baseCfg = JSON.parse(fs.readFileSync(tmpCfgPath, 'utf8'));
		const bigCfg = {
			...baseCfg,
			budgetBytes: 32768,
			keywordDocs: [{ doc: 'docs/reference/flow/kwdom.md', section: '业务约束与不变量', keywords: ['大预算词'] }],
		};
		const writeCfg = (cfg, tick) => {
			fs.writeFileSync(tmpCfgPath, JSON.stringify(cfg));
			const t = new Date(Date.now() + tick * 2000);
			fs.utimesSync(tmpCfgPath, t, t);
		};
		const readModeSet = (dir) => {
			const { DatabaseSync } = require('node:sqlite');
			const db = new DatabaseSync(path.join(dir, '.pi', 'harness', 'events.db'), { readOnly: true });
			const rows = db.prepare("SELECT payload FROM events WHERE kind='mode.set'").all().map((r) => JSON.parse(r.payload));
			db.close();
			return rows;
		};
		// mode.set 全量（含 session_id，供「记在谁名下」断言）；seedModeSet 造一条父会话历史
		// （模拟跨进程：父会话不在本进程内存，只有事实库记录）。
		const readModeSetFull = (dir) => {
			const { DatabaseSync } = require('node:sqlite');
			const db = new DatabaseSync(path.join(dir, '.pi', 'harness', 'events.db'), { readOnly: true });
			const rows = db
				.prepare("SELECT session_id, change, payload FROM events WHERE kind='mode.set' ORDER BY id")
				.all()
				.map((r) => ({ ...JSON.parse(r.payload), session_id: r.session_id, change: r.change }));
			db.close();
			return rows;
		};
		const seedModeSet = (dir, sessionId, mode, boundChange, source = 'skill') => {
			const { DatabaseSync } = require('node:sqlite');
			const db = new DatabaseSync(path.join(dir, '.pi', 'harness', 'events.db'));
			db.prepare("INSERT INTO events (ts, session_id, kind, change, payload) VALUES (?, ?, 'mode.set', ?, ?)").run(
				new Date().toISOString(),
				sessionId,
				boundChange,
				JSON.stringify({ mode, boundChange, source }),
			);
			db.close();
		};
		writeCfg(bigCfg, 1);
		await emit('session_start', { reason: 'new' }, tmpCtx);
		await emit('input', { text: '/opsx-apply smoke-fixture-budget' }, tmpCtx);
		const s17a = await systemPromptOnly(tmpCtx);
		const s17b = await systemPromptOnly(tmpCtx);
		check('行为: 稳定层档位生命周期内字节恒定', s17a === s17b && s17a.includes('### constraints-index.md') === false && s17a.includes('活跃变更：smoke-fixture-budget'));
		check('行为: 稳定层不含关键词/JIT 动态内容（声明域红线层在稳定层）', !s17a.includes('BIGKW-MARKER') && !s17a.includes('### bigjit.md'));
		await emit('input', { text: '大预算词相关展示调整' }, tmpCtx);
		const kwSp = await systemPrompt(tmpCtx);
		check('行为: 声明域内关键词命中投递全节（细节层）', kwSp.includes('BIGKW-MARKER'));
		const kwStable = await systemPromptOnly(tmpCtx);
		check('行为: 动态层内容不进 system prompt', !kwStable.includes('BIGKW-MARKER'));
		check('行为: 动态层稳态零重发', !(await systemPrompt(tmpCtx)).includes('📎 约束补充'));
		await emit('session_compact', { reason: 'manual' }, tmpCtx);
		const csSp = await systemPrompt(tmpCtx);
		check('行为: compact 后重发约束快照', csSp.includes('约束快照（compact 后重发）') && csSp.includes('BIGKW-MARKER'));

		// 17.3 行为：绑定污染修复（read/健康写不抢绑、兜底绑、turn 锁定、恢复记账、source 不变量）
		await emit('session_start', { reason: 'resume' }, tmpCtx);
		let msRows17 = readModeSet(tmp);
		check('行为: 恢复路径记账 source=recover', msRows17.some((r) => r.source === 'recover'));
		await emit('session_start', { reason: 'new' }, tmpCtx);
		await emit('input', { text: '/opsx-apply smoke-fixture-budget' }, tmpCtx);
		await systemPromptOnly(tmpCtx); // turn 起点（解锁）
		const msBefore17 = readModeSet(tmp).length;
		await emit('tool_execution_start', { toolName: 'read', args: { path: 'openspec/changes/smoke-fixture-temp/tasks.md' } }, tmpCtx);
		await emit('tool_execution_start', { toolName: 'write', args: { path: 'openspec/changes/smoke-fixture-temp/proposal.md' } }, tmpCtx);
		msRows17 = readModeSet(tmp);
		check('行为: read / 健康绑定写其他 change 目录零抢绑（零新增记账）', msRows17.length === msBefore17);
		for (const n of ['bind-a', 'bind-b', 'bind-c']) fs.mkdirSync(path.join(tmp, OPENSPEC_CHANGES_DIR_REL, n), { recursive: true });
		// bind-c 带声明域（kwdom）：使 19.6 的「B 绑定生效」可观测——无声明时 requirements 档
		// 零稳定条目 → 稳定层 header 不输出（wantBlock 门），绑定生效与否无处可断言。
		fs.writeFileSync(
			path.join(tmp, OPENSPEC_CHANGES_DIR_REL, 'bind-c', 'proposal.md'),
			'# bind-c fixture\n\n<!-- constraint-domains: kwdom -->\n\n正文。\n',
		);
		await emit('session_start', { reason: 'new' }, tmpCtx);
		await emit('input', { text: '/opsx-new' }, tmpCtx); // requirements 档：无 mtime 兜底 → 无绑定
		await systemPromptOnly(tmpCtx); // turn 起点（解锁）
		await emit('tool_execution_start', { toolName: 'write', args: { path: 'openspec/changes/bind-a/x.md' } }, tmpCtx);
		await emit('tool_execution_start', { toolName: 'write', args: { path: 'openspec/changes/bind-b/x.md' } }, tmpCtx);
		msRows17 = readModeSet(tmp);
		const editDirBinds = msRows17.filter((r) => r.source === 'edit-dir').map((r) => r.boundChange);
		check('行为: 无绑定写 change 目录兜底绑 + 同 turn 锁定（仅首个生效）', editDirBinds.length === 1 && editDirBinds[0] === 'bind-a');
		await systemPromptOnly(tmpCtx); // 新 turn（解锁）
		fs.rmSync(path.join(tmp, OPENSPEC_CHANGES_DIR_REL, 'bind-a'), { recursive: true, force: true });
		await emit('tool_execution_start', { toolName: 'write', args: { path: 'openspec/changes/bind-b/x.md' } }, tmpCtx);
		msRows17 = readModeSet(tmp);
		check('行为: 锁定仅限同 turn（新 turn 内可再切绑）', msRows17.some((r) => r.source === 'edit-dir' && r.boundChange === 'bind-b'));
		check(
			'行为: mode.set 全路径带 source（隐性绑定不存在）',
			msRows17.length > 0 &&
				msRows17.every((r) => ['command', 'skill', 'edit-dir', 'recover', 'inherit', 'fallback'].includes(r.source)),
		);

		// 17.4 行为：legacy 回退通道（应急开关，恢复旧「全量进 system prompt」）
		writeCfg({ ...bigCfg, channel: 'legacy' }, 2);
		await emit('session_start', { reason: 'new' }, tmpCtx);
		await emit('input', { text: '/opsx-apply smoke-fixture-budget' }, tmpCtx);
		const legacySp = await systemPrompt(tmpCtx);
		check('行为: legacy 通道全量进 system prompt（零消息投递）', legacySp.includes('### kwdom.md') && messages.length === msgCursor);

		// 19. 会话作用域状态隔离（per-session-constraint-binding）
		//     背景（2026-09-17 事实库）：绑定状态曾是模块级全局，他会话的 mode.set 会决定本会话
		//     注入谁的约束（会话 01a0aaf4 零 mode.set，却在另一会话绑定 6 秒后拿到
		//     dedupe-rss-articles 及其声明域）。本组用例按 spec「会话作用域状态隔离」逐条对账。
		writeCfg(bigCfg, 3); // 回到 split 通道（上组 17.4 写成了 legacy）
		const sessA = { cwd: tmp, hasUI: false, sessionManager: { getSessionId: () => 'ci-smoke-A' } };
		const sessB = { cwd: tmp, hasUI: false, sessionManager: { getSessionId: () => 'ci-smoke-B' } };
		const sessC = { cwd: tmp, hasUI: false, sessionManager: { getSessionId: () => 'ci-smoke-C' } };

		// 19.1 两会话交叉绑定互不污染（A 绑 budget=声明 kwdom，B 绑 temp=无声明）
		await emit('session_start', { reason: 'new' }, sessA);
		await emit('session_start', { reason: 'new' }, sessB);
		await emit('input', { text: '/opsx-apply smoke-fixture-budget' }, sessA);
		const aSp1 = await systemPrompt(sessA);
		await emit('input', { text: '/opsx-apply smoke-fixture-temp' }, sessB);
		const bSp1 = await systemPrompt(sessB);
		const aSp2 = await systemPromptOnly(sessA);
		check('隔离: A 绑定不被他会话改写（仍绑 budget）', aSp2.includes('活跃变更：smoke-fixture-budget'));
		check('隔离: A 声明域红线层在（kwdom）', aSp1.includes('### kwdom.md'));
		check('隔离: B 注入归属为 temp（非 budget）', bSp1.includes('活跃变更：smoke-fixture-temp'));
		check('隔离: B 不注入 A 的声明域', !bSp1.includes('### kwdom.md'));

		// 19.2 他会话绑定变化不刷新本会话稳定层（字节恒定 + 零新增稳定层记账）
		const aStableBefore = await systemPromptOnly(sessA);
		await emit('session_start', { reason: 'new' }, sessB);
		await emit('input', { text: '/opsx-apply smoke-fixture-budget' }, sessB); // B 改绑到 A 同一个 change
		const aStableAfter = await systemPromptOnly(sessA);
		check(
			'隔离: 他会话绑定变化不改本会话稳定层字节',
			aStableBefore === aStableAfter && aStableAfter.includes('活跃变更：smoke-fixture-budget'),
		);

		// 19.3 无自身绑定且无父子关系 → 仅索引（不借他会话）
		await emit('session_start', { reason: 'startup' }, sessC);
		markMessages();
		const cSp = await systemPromptOnly(sessC);
		check(
			'隔离: 无自身绑定且无父子关系 → 未激活（不借他会话绑定/域）',
			/档位：未激活/.test(cSp) &&
				cSp.includes('活跃变更：无') &&
				!cSp.includes('### kwdom.md') &&
				!dynamicText().includes('kwdom'),
		);

		// 19.4 子线程显式继承父会话（parentSession 可证）+ 记账 source=inherit + 主会话不被清零
		const childCtx = {
			cwd: tmp,
			hasUI: false,
			sessionManager: {
				getSessionId: () => 'ci-smoke-child',
				getHeader: () => ({
					parentSession:
						'/home/x/.pi/agent/sessions/dir/2026-09-17T00-00-00-000Z_ci-smoke-A.jsonl',
				}),
			},
		};
		await emit('session_start', { reason: 'startup' }, childCtx);
		const childSp = await systemPrompt(childCtx);
		check(
			'隔离: 子线程继承父会话档位与绑定',
			/档位：实现/.test(childSp) && childSp.includes('活跃变更：smoke-fixture-budget'),
		);
		check('隔离: 子线程带父会话声明域红线层', childSp.includes('### kwdom.md'));
		const inheritRows = readModeSet(tmp).filter((r) => r.source === 'inherit');
		check(
			'隔离: 继承显式记账 source=inherit + boundChange',
			inheritRows.length === 1 && inheritRows[0].boundChange === 'smoke-fixture-budget',
		);
		const aAfterChild = await systemPromptOnly(sessA);
		check('隔离: 子线程活动不清零主会话档位', aAfterChild.includes('活跃变更：smoke-fixture-budget'));

		// 19.4c 子线程活动 MUST NOT 改写父会话命中集（spec「子线程显式继承父会话」第三条 AND）。
		//       判别力：把 inheritFromParent 的 `new Map(parent.jitDocHits)` / `new Map(parent.keywordDocHits)`
		//       改成浅共享（直接引用父 Map）时，子会话命中会写进父会话集合 → 本节断言变红。
		await emit('input', { text: '大预算词相关展示调整' }, childCtx); // 子会话关键词命中（kwdom）
		await emit(
			'tool_execution_start',
			{ toolName: 'edit', args: { path: 'backend-go/internal/bigmod/service.go' } },
			childCtx,
		); // 子会话 JIT 命中（bigjit）
		const childHits = await systemPrompt(childCtx);
		check(
			'隔离: 子会话自身命中生效（关键词 + JIT）',
			childHits.includes('BIGKW-MARKER') && childHits.includes('### bigjit.md'),
		);
		markMessages();
		const aAfterChildHits = await systemPrompt(sessA);
		check('隔离: 子线程关键词命中不回写主会话命中集', !aAfterChildHits.includes('BIGKW-MARKER'));
		check('隔离: 子线程 JIT 命中不回写主会话命中集', !aAfterChildHits.includes('bigjit'));

		// 19.4b 父会话不可证（header 指向不存在会话）→ 不继承
		const orphanCtx = {
			cwd: tmp,
			hasUI: false,
			sessionManager: {
				getSessionId: () => 'ci-smoke-orphan',
				getHeader: () => ({ parentSession: '/x/2026-09-17T00-00-00-000Z_no-such-parent.jsonl' }),
			},
		};
		await emit('session_start', { reason: 'startup' }, orphanCtx);
		const orphanSp = await systemPrompt(orphanCtx);
		check('隔离: 父会话不可证 → 不继承（未激活）', /档位：未激活/.test(orphanSp));

		// 19.10 跨进程继承（事实库回退）：父会话**不在本进程内存**（模拟 pi-subagents
		//       子线程 = 独立 node 进程，2026-09-17 实测），但父会话的 mode.set 历史在共享
		//       事实库 → 子会话据 header 的 parentSession 采纳档位与绑定。
		//       判别力：把 inheritFromParentHistory 短路（开头 return false）→ 本节3 条断言变红。
		const xprocParent = 'ci-smoke-parent-xproc';
		seedModeSet(tmp, xprocParent, 'implementation', 'smoke-fixture-budget');
		const beforeXproc = readModeSetFull(tmp).length;
		const xprocChild = {
			cwd: tmp,
			hasUI: false,
			sessionManager: {
				getSessionId: () => 'ci-smoke-child-xproc',
				getHeader: () => ({
					parentSession: `/home/x/.pi/agent/sessions/dir/2026-09-17T00-00-00-000Z_${xprocParent}.jsonl`,
				}),
			},
		};
		await emit('session_start', { reason: 'startup' }, xprocChild);
		const xprocSp = await systemPrompt(xprocChild);
		check(
			'跨进程: 子线程据父会话事实库历史继承档位与绑定',
			/档位：实现/.test(xprocSp) && xprocSp.includes('活跃变更：smoke-fixture-budget'),
		);
		check('跨进程: 子线程带父会话声明域红线层（kwdom）', xprocSp.includes('### kwdom.md'));
		const xprocInheritRows = readModeSetFull(tmp)
			.slice(beforeXproc)
			.filter((r) => r.source === 'inherit');
		check(
			'跨进程: 继承显式记账 source=inherit 且记在子会话名下',
			xprocInheritRows.length === 1 &&
				xprocInheritRows[0].boundChange === 'smoke-fixture-budget' &&
				xprocInheritRows[0].session_id === 'ci-smoke-child-xproc',
		);

		// 19.11 路径兜底解析：getHeader 不可用（未落盘/契约变化），但 getSessionFile() 形如
		//       `<父会话目录>/forks/<ts>_<子会话id>.jsonl` → 从父目录名解出父 id 并继承。
		const pathChild = {
			cwd: tmp,
			hasUI: false,
			sessionManager: {
				getSessionId: () => 'ci-smoke-child-path',
				getHeader: () => null,
				getSessionFile: () =>
					`/home/x/.pi/agent/sessions/dir/2026-09-17T00-00-00-000Z_${xprocParent}/forks/2026-09-17T00-01-00-000Z_ci-smoke-child-path.jsonl`,
			},
		};
		await emit('session_start', { reason: 'startup' }, pathChild);
		const pathChildSp = await systemPrompt(pathChild);
		check(
			'跨进程: getHeader 缺失时按 fork 路径解父 id 并继承',
			/档位：实现/.test(pathChildSp) &&
				pathChildSp.includes('活跃变更：smoke-fixture-budget') &&
				pathChildSp.includes('### kwdom.md'),
		);

		// 19.12 两条父子证据都无（非 fork 会话）→ 不继承（红线：不得借用任意他会话状态）
		const noSrcChild = {
			cwd: tmp,
			hasUI: false,
			sessionManager: {
				getSessionId: () => 'ci-smoke-child-nosrc',
				getHeader: () => null,
				getSessionFile: () =>
					'/home/x/.pi/agent/sessions/dir/2026-09-17T00-00-00-000Z_ci-smoke-child-nosrc.jsonl',
			},
		};
		await emit('session_start', { reason: 'startup' }, noSrcChild);
		check(
			'跨进程: 无父子证据（非 fork 路径）→ 不继承（未激活）',
			/档位：未激活/.test(await systemPromptOnly(noSrcChild)),
		);

		// 19.13 pi-web 子会话继承（harden-subagent-constraint-channel T3/D3）：pi-web Agent
		//       工具派发的实现档子线程与 pi-subagents 同形——独立 sessionId + session_start
		//       reason=startup + header parentSession（2026-09-18 实测样本 01a0b314 转录头）。
		//       期望：mode.set source=inherit 记在子会话名下 + 注入带父会话绑定的 change
		//       （稳定层 header 与 constraint.inject 记账 change 列均为绑定 change）。
		const piwebParent = 'ci-smoke-parent-piweb';
		seedModeSet(tmp, piwebParent, 'implementation', 'bind-c');
		const beforePiweb = readModeSetFull(tmp).length;
		const piwebChild = {
			cwd: tmp,
			hasUI: false,
			sessionManager: {
				getSessionId: () => 'ci-smoke-child-piweb',
				getHeader: () => ({
					parentSession: `/home/x/.pi/agent/sessions/dir/2026-09-18T00-00-00-000Z_${piwebParent}.jsonl`,
				}),
			},
		};
		await emit('session_start', { reason: 'startup' }, piwebChild);
		const piwebSp = await systemPrompt(piwebChild);
		check(
			'pi-web: 子会话继承父绑定（注入带绑定 change + 其声明域）',
			/档位：实现/.test(piwebSp) && piwebSp.includes('活跃变更：bind-c（档位绑定）') && piwebSp.includes('### kwdom.md'),
		);
		const piwebInherit = readModeSetFull(tmp)
			.slice(beforePiweb)
			.filter((r) => r.source === 'inherit');
		check(
			'pi-web: mode.set source=inherit 记在子会话名下且带绑定 change',
			piwebInherit.length === 1 &&
				piwebInherit[0].session_id === 'ci-smoke-child-piweb' &&
				piwebInherit[0].boundChange === 'bind-c',
		);
		{
			const { DatabaseSync } = require('node:sqlite');
			const piwebDb = new DatabaseSync(path.join(tmp, '.pi', 'harness', 'events.db'), { readOnly: true });
			const injectRows = piwebDb.prepare("SELECT change FROM events WHERE kind='constraint.inject' AND session_id='ci-smoke-child-piweb'").all();
			piwebDb.close();
			check(
				'pi-web: constraint.inject 记账 change 列 = 绑定 change（至少一条）',
				injectRows.length >= 1 && injectRows.every((r) => r.change === 'bind-c'),
			);
		}

		// 19.14 子线程 session_start 稳定层投递（harden-subagent-constraint-channel T3-rev/D3）：
		//       pi-web 子线程 SDK 无 before_agent_start 事件 → 稳定层改在 session_start 末尾
		//       经 sendMessage(steer) 投递（source=child-init 记账 + stableSnapshot/
		//       childInitDelivered 置位）；随后 before_agent_start 冻结（不返回 systemPrompt
		//       块、零重复记账）；主会话 startup 通道零变化。
		// ① 未绑定档子线程：父会话无事实库记录 → 继承失败 → 只投索引（不投任何 change 文本）
		const t3Parent = 'ci-smoke-parent-unbound';
		const unboundChild = {
			cwd: tmp,
			hasUI: false,
			sessionManager: {
				getSessionId: () => 'ci-smoke-child-unbound',
				getHeader: () => ({
					parentSession: `/home/x/.pi/agent/sessions/dir/2026-09-18T00-00-00-000Z_${t3Parent}.jsonl`,
				}),
			},
		};
		const countInjects = (sessionId) => {
			const { DatabaseSync } = require('node:sqlite');
			const db = new DatabaseSync(path.join(tmp, '.pi', 'harness', 'events.db'), { readOnly: true });
			const n = db.prepare("SELECT COUNT(*) AS n FROM events WHERE kind='constraint.inject' AND session_id=?").get(sessionId).n;
			db.close();
			return n;
		};
		markMessages();
		await emit('session_start', { reason: 'startup' }, unboundChild);
		const initMsgs = messages.slice(msgCursor);
		check(
			'T3-rev①: 子线程 startup 触发 sendMessage（customType + steer + 不触发 turn）',
			initMsgs.length === 1 &&
				initMsgs[0].msg.customType === 'constraint-injection' &&
				initMsgs[0].opts.deliverAs === 'steer' &&
				initMsgs[0].opts.triggerTurn === false,
		);
		check(
			'T3-rev①: 内容含索引/档位块（未绑定档仅索引，无 change 文本）',
			initMsgs[0].msg.content.includes('### idx.md') &&
				initMsgs[0].msg.content.includes('档位：未激活') &&
				initMsgs[0].msg.content.includes('活跃变更：无') &&
				initMsgs[0].msg.content.includes('与 AGENTS.md 优先级宪法冲突时，以宪法为准') &&
				!initMsgs[0].msg.content.includes('smoke-fixture'),
		);
		check(
			'T3-rev①: constraint.inject 带 source=child-init（索引条目）',
			countInjects('ci-smoke-child-unbound') === 1 &&
				(() => {
					const { DatabaseSync } = require('node:sqlite');
					const db = new DatabaseSync(path.join(tmp, '.pi', 'harness', 'events.db'), { readOnly: true });
					const rows = db.prepare("SELECT payload FROM events WHERE kind='constraint.inject' AND session_id='ci-smoke-child-unbound'").all().map((r) => JSON.parse(r.payload));
					db.close();
					return rows[0].source === 'child-init' && rows[0].path === 'docs/idx.md';
				})(),
		);
		const t3Chan = ext.channelStateForTest('ci-smoke-child-unbound');
		check(
			'T3-rev①: stableSnapshot 置位（key=none|none）+ childInitDelivered 置位',
			!!t3Chan && t3Chan.childInitDelivered === true && t3Chan.stableSnapshotKey === 'none|none',
		);
		// ② 同会话随后的 before_agent_start：不再追加 systemPrompt 块、零重复记账、零消息重投
		const injectsBeforeBa = countInjects('ci-smoke-child-unbound');
		const msgsBeforeBa = messages.length;
		const baResult = await emit('before_agent_start', { systemPrompt: 'BASE' }, unboundChild);
		check(
			'T3-rev②: before_agent_start 不返回 systemPrompt 块（防双投）且零消息重投',
			(!baResult || !baResult.systemPrompt) && messages.length === msgsBeforeBa,
		);
		check('T3-rev②: 零重复记账', countInjects('ci-smoke-child-unbound') === injectsBeforeBa);
		markMessages();
		// ③ 主会话 startup 回归：零 sendMessage；后续 before_agent_start 行为与现状一致
		const mainCtx = { cwd: tmp, hasUI: false, sessionManager: { getSessionId: () => 'ci-smoke-main-t3rev' } };
		const msgsBeforeMain = messages.length;
		await emit('session_start', { reason: 'startup' }, mainCtx);
		check('T3-rev③: 主会话 startup 零 sendMessage', messages.length === msgsBeforeMain);
		const mainSp = await systemPromptOnly(mainCtx);
		check(
			'T3-rev③: 主会话 before_agent_start 照常返回稳定层 systemPrompt（现状回归）',
			mainSp.startsWith('BASE') && mainSp.includes('### idx.md') && mainSp.includes('档位：未激活'),
		);
		const mainChan = ext.channelStateForTest('ci-smoke-main-t3rev');
		check(
			'T3-rev③: 主会话 childInitDelivered 恒 false',
			!!mainChan && mainChan.childInitDelivered === false && mainChan.stableSnapshotKey === 'none|none',
		);

		// 19.5 命中集按会话隔离（关键词 + JIT 各一例）
		await emit('input', { text: '大预算词相关展示调整' }, sessA);
		const aKw = await systemPrompt(sessA);
		check('隔离: A 关键词命中投递全节（细节层）', aKw.includes('BIGKW-MARKER'));
		markMessages();
		await systemPromptOnly(sessB);
		check('隔离: 他会话关键词命中不投递给本会话', !dynamicText().includes('BIGKW-MARKER'));
		await emit(
			'tool_execution_start',
			{ toolName: 'edit', args: { path: 'backend-go/internal/bigmod/service.go' } },
			sessA,
		);
		const aJit = await systemPrompt(sessA);
		check('隔离: A JIT 命中注入 bigjit', aJit.includes('### bigjit.md'));
		markMessages();
		await systemPromptOnly(sessB);
		check('隔离: 他会话 JIT 命中不投递给本会话', !dynamicText().includes('bigjit'));

		// 19.6 turn 绑定锁按会话隔离。
		//     判别力顺序（review P2-4 修正）：必须让 **A 先持锁**、再让 B 写 change 目录。
		//     旧顺序先在 B 上跑 before_agent_start —— turn 起点本就解 B 的锁，于是「锁是模块级全局」
		//     的实现也会被解开，断言恒绿（不具判别力）。固定顺序：B 进 turn（解 B 锁）→ A 命令绑定
		//     （置 A 锁）→ B 写 bind-c。全局锁实现下 B 绑不上（红）；per-session 锁下 B 正常绑定（绿）。
		await emit('session_start', { reason: 'new' }, sessB);
		await emit('input', { text: '/opsx-new' }, sessB); // requirements 档：无 mtime 兜底 → 无绑定
		await systemPromptOnly(sessB); // B 的 turn 起点（解 B 的锁）
		const msBeforeLock = readModeSet(tmp).length;
		await emit('input', { text: '/opsx-apply smoke-fixture-budget' }, sessA); // A 命令绑定 → 置 A 的锁
		const msAfterLock = readModeSet(tmp);
		check(
			'隔离: A 置锁前置成立（A 命令绑定已记账）',
			msAfterLock.length === msBeforeLock + 1 &&
				msAfterLock[msAfterLock.length - 1].source === 'command' &&
				msAfterLock[msAfterLock.length - 1].boundChange === 'smoke-fixture-budget',
		);
		await emit(
			'tool_execution_start',
			{ toolName: 'write', args: { path: 'openspec/changes/bind-c/x.md' } },
			sessB,
		);
		const cBinds = readModeSet(tmp).filter((r) => r.source === 'edit-dir' && r.boundChange === 'bind-c');
		check('隔离: turn 锁按会话独立（B 的兜底绑不被 A 的锁拦住）', cBinds.length >= 1);
		const bBoundSp = await systemPrompt(sessB);
		check(
			'隔离: B 的兜底绑确实生效（注入归属 bind-c + 其声明域）',
			bBoundSp.includes('活跃变更：bind-c') && bBoundSp.includes('### kwdom.md'),
		);
		const aStillBound = await systemPromptOnly(sessA);
		check('隔离: A 在本会话内保持原绑定', aStillBound.includes('活跃变更：smoke-fixture-budget'));

		// 19.7 无 sessionId 兜底槽 + 同 sessionId 多视图共享状态
		const noSessCtx = { cwd: tmp, hasUI: false };
		await emit('session_start', { reason: 'new' }, noSessCtx);
		await emit('input', { text: '/opsx-apply smoke-fixture-budget' }, noSessCtx);
		const nSp = await systemPrompt(noSessCtx);
		check('兜底: 无 sessionId 语境自洽（单槽位）', nSp.includes('活跃变更：smoke-fixture-budget'));
		const sessA2 = {
			cwd: tmp,
			hasUI: false,
			sessionManager: { getSessionId: () => 'ci-smoke-A' },
		};
		const a2Sp = await systemPromptOnly(sessA2);
		check('兜底: 同 sessionId 多视图共享状态（不互相重置）', a2Sp.includes('活跃变更：smoke-fixture-budget'));

		// 19.8 会话条目有界（LRU 淘汰，防长跑进程无界增长）
		const limit = ext.SESSION_STATE_LIMIT_FOR_TEST;
		const firstKey = ext.sessionStateKeysForTest()[0];
		for (let i = 0; i < limit + 4; i++) {
			await emit(
				'session_start',
				{ reason: 'new' },
				{ cwd: tmp, hasUI: false, sessionManager: { getSessionId: () => `ci-smoke-lru-${i}` } },
			);
		}
		const keysAfter = ext.sessionStateKeysForTest();
		check(
			'隔离: 会话条目有界（不超上限 + 新会话在内）',
			keysAfter.length <= limit && keysAfter.includes(`ci-smoke-lru-${limit + 3}`),
		);
		check('隔离: 最久未用条目被淘汰', !keysAfter.includes(firstKey));

		// 19.9 会话条目上限可配（design D4「可配」）：cfg.sessionStateLimit 覆盖 + 非法值回退默认。
		//      判别力：若上限写死 32，limit=1 的断言（≤1）变红；若非法值不回退（直接取 0），
		//      建 3 个会话后 Map 会被清空、全在断言变红。
		writeCfg({ ...bigCfg, sessionStateLimit: 1 }, 4);
		for (const n of ['limit1-a', 'limit1-b', 'limit1-c']) {
			await emit(
				'session_start',
				{ reason: 'new' },
				{ cwd: tmp, hasUI: false, sessionManager: { getSessionId: () => `ci-smoke-${n}` } },
			);
			await new Promise((r) => setTimeout(r, 3)); // 拉开毫秒级 LRU 时间戳，使「最新存活」确定
		}
		const keysLimit1 = ext.sessionStateKeysForTest();
		check(
			'隔离: cfg.sessionStateLimit=1 生效（存活条目 ≤1 且最新会话在内）',
			keysLimit1.length === 1 && keysLimit1.includes('ci-smoke-limit1-c'),
		);
		writeCfg({ ...bigCfg, sessionStateLimit: 0 }, 6); // 非法值：回退默认 32
		for (const n of ['bad-a', 'bad-b', 'bad-c']) {
			await emit(
				'session_start',
				{ reason: 'new' },
				{ cwd: tmp, hasUI: false, sessionManager: { getSessionId: () => `ci-smoke-${n}` } },
			);
		}
		const keysBad = ext.sessionStateKeysForTest();
		check(
			'隔离: 非法上限（0）回退默认（3 个新会话全在，未被误淘汰）',
			['ci-smoke-bad-a', 'ci-smoke-bad-b', 'ci-smoke-bad-c'].every((k) => keysBad.includes(k)),
		);
		check(
			'隔离: 上限解析回退默认（纯函数：0/负数/NaN/缺省/小数取整）',
			ext.resolveSessionStateLimitForTest({ sessionStateLimit: 0 }) === ext.SESSION_STATE_LIMIT_FOR_TEST &&
				ext.resolveSessionStateLimitForTest({ sessionStateLimit: -3 }) === ext.SESSION_STATE_LIMIT_FOR_TEST &&
				ext.resolveSessionStateLimitForTest({ sessionStateLimit: Number.NaN }) === ext.SESSION_STATE_LIMIT_FOR_TEST &&
				ext.resolveSessionStateLimitForTest(null) === ext.SESSION_STATE_LIMIT_FOR_TEST &&
				ext.resolveSessionStateLimitForTest({ sessionStateLimit: 2.7 }) === 2,
		);
		writeCfg(bigCfg, 8); // 还原默认配置

		// 20. 指纹按 regime 标记（fix-injection-transition-fingerprint-wipe）：
		//     档位/绑定可 mid-turn（tool_execution_start）变化而快照重建延迟到下一 turn 起点，
		//     延迟重建的指纹清空仅当 regime≠新快照 key —— mid-turn 切档后同 turn 已投递的
		//     JIT 条目不被误清重投；真实 regime 切换（edit-dir 绑定）仍全量重投；fork 继承
		//     陈旧快照 key 同理不重发。判别力：把清空条件改回无条件 isTransition → 场景 A/C 红；
		//     把清空去掉（永不清空）→ 场景 B 红。用 requirements 档：implementation 有
		//     mtime 兜底绑定会污染快照 key。
		writeCfg(
			{
				...bigCfg,
				skillSignals: { requirements: ['openspec-new-change'], implementation: ['openspec-apply-change'] },
			},
			10,
		);
		const sessW = { cwd: tmp, hasUI: false, sessionManager: { getSessionId: () => 'ci-smoke-wipe' } };
		// 场景 A（bug 复刻）：未激活 turn 建快照 → mid-turn skill 激活 → 同 turn JIT 即时投递
		// → 下一 turn 快照重建：稳定层重建发生，但已投递节零重投。
		//   断言动态层一律 systemPromptOnly（不消费消息）+ dynamicText()：systemPrompt()
		//   会消费新消息，之后 dynamicText() 恒空、断言失去判别力。
		await emit('session_start', { reason: 'new' }, sessW);
		const wSp1 = await systemPromptOnly(sessW); // 未激活 turn：建快照 key=none|none
		check('regime: 未激活 turn 建快照（索引注入）', wSp1.includes('idx.md'));
		markMessages();
		await emit(
			'tool_execution_start',
			{ toolName: 'read', args: { path: '/x/.pi/agent/skills/openspec-new-change/SKILL.md' } },
			sessW,
		); // mid-turn requirements 激活（快照不重建）
		check(
			'regime: mid-turn skill 激活记账（source=skill, requirements）',
			readModeSet(tmp).some((r) => r.source === 'skill' && r.mode === 'requirements' && !r.boundChange),
		);
		await emit(
			'tool_execution_start',
			{ toolName: 'edit', args: { path: 'backend-go/internal/platform/airouter/router.go' } },
			sessW,
		); // 同 turn JIT 命中 → 即时投递（指纹 regime=requirements|none）
		const wJit = dynamicText();
		markMessages();
		check('regime: 同 turn JIT 即时投递（fixture 全文回落）', wJit.includes('FULLDOC-MARKER') && wJit.includes('STD2-FULLDOC'));
		const wSp2 = await systemPromptOnly(sessW); // 下一 turn：延迟快照重建
		check('regime: 下一 turn 稳定层按新档位重建（需求档 header）', /档位：需求/.test(wSp2));
		check(
			'regime: mid-turn 切档后同 turn 已投递内容零重投（本案修复点）',
			!dynamicText().includes('📎') && !dynamicText().includes('FULLDOC-MARKER'),
		);
		markMessages();
		await systemPromptOnly(sessW); // 隔 N turn 仍零重投
		check('regime: 稳态零投递保持（指纹+regime 存续）', !dynamicText().includes('📎'));
		markMessages();
		// 场景 B（反向护栏）：真实 regime 切换（edit-dir 兜底绑定）→ 指纹清空 → 全量重投
		await emit(
			'tool_execution_start',
			{ toolName: 'edit', args: { path: 'openspec/changes/smoke-fixture-budget/proposal.md' } },
			sessW,
		);
		check(
			'regime: edit-dir 兜底绑定记账（source=edit-dir）',
			readModeSet(tmp).some((r) => r.source === 'edit-dir' && r.boundChange === 'smoke-fixture-budget'),
		);
		const wSp3 = await systemPromptOnly(sessW); // 绑定切换后快照重建
		check(
			'regime: 绑定切换后稳定层重建（新绑定 + 声明域红线层）',
			wSp3.includes('活跃变更：smoke-fixture-budget') && wSp3.includes('### kwdom.md'),
		);
		check(
			// requirements 档不注 change 级文件（仅实现档），全量重投以粘性 JIT 条目为证
			'regime: 真实 regime 切换仍全量重投（含已投递过的 JIT 节）',
			dynamicText().includes('FULLDOC-MARKER') && dynamicText().includes('STD2-FULLDOC'),
		);
		markMessages();
		// 场景 C（fork 变体）：父 mid-turn 切档已投递、快照未及重建（陈旧 none|none）→
		// 子会话继承父 channel → 子首 turn 快照重建但父已投递内容不重发（fork 语义）
		const sessP = { cwd: tmp, hasUI: false, sessionManager: { getSessionId: () => 'ci-smoke-wipe-parent' } };
		await emit('session_start', { reason: 'new' }, sessP);
		await systemPromptOnly(sessP); // 未激活快照 none|none（保持陈旧，不重建）
		markMessages();
		await emit(
			'tool_execution_start',
			{ toolName: 'read', args: { path: '/x/.pi/agent/skills/openspec-new-change/SKILL.md' } },
			sessP,
		);
		await emit(
			'tool_execution_start',
			{ toolName: 'edit', args: { path: 'backend-go/internal/platform/airouter/router.go' } },
			sessP,
		); // 父同 turn JIT 投递（指纹 regime=requirements|none，快照仍 none|none）
		markMessages();
		const childW = {
			cwd: tmp,
			hasUI: false,
			sessionManager: {
				getSessionId: () => 'ci-smoke-wipe-child',
				getHeader: () => ({
					parentSession: '/home/x/.pi/agent/sessions/dir/2026-09-17T00-00-00-000Z_ci-smoke-wipe-parent.jsonl',
				}),
			},
		};
		await emit('session_start', { reason: 'startup' }, childW); // 继承父 channel（指纹 regime=新 key）
		// T3-rev：子会话 startup 末尾即投递稳定层消息（header 块），首 turn system prompt 冻结
		await systemPromptOnly(childW); // 子首 turn：防双投（systemPrompt 不再拼稳定层块）
		check(
			'regime: fork 子会话继承父档位（需求档，经 session_start 消息投递）',
			/档位：需求/.test(dynamicText()),
		);
		check(
			'regime: fork 子会话父已投递内容不重发（fork 指纹继承语义保持）',
			!dynamicText().includes('📎') && !dynamicText().includes('FULLDOC-MARKER'),
		);
		markMessages();

		let fail = 0;
		for (const [name, ok] of checks) {
			console.log(`${ok ? '✅' : '❌'} ${name}`);
			if (!ok) fail++;
		}
		if (fail) {
			console.error(`\n${fail} 项失败。注入块头部：\n${sp.slice(0, 600)}`);
			process.exitCode = 1; // 不用 process.exit()：它不同步展开栈，finally 清理不会执行
		} else {
			console.log('\nSMOKE OK');
		}
	} finally {
		fs.rmSync(FIXTURE, { recursive: true, force: true });
		fs.rmSync(RESEARCH_FIXTURE, { recursive: true, force: true });
		if (poolExisted) fs.writeFileSync(RESEARCH_POOL, poolOrig);
		else fs.rmSync(RESEARCH_POOL, { force: true });
		fs.rmSync(tmp, { recursive: true, force: true });
	}
})().catch((e) => {
	console.error('FAIL', e);
	process.exitCode = 1;
});
