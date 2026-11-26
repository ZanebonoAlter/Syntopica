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
// stage.md：**阶段唯一真相源**（stage-driven-constraint-injection D1）——fixture 处于
// implement 阶段（implementation 族）→ 路由清单含代码规范、change 级文件注入开启。
function writeFixture() {
	fs.mkdirSync(FIXTURE, { recursive: true });
	fs.writeFileSync(
		path.join(FIXTURE, 'proposal.md'),
		'# smoke fixture\n\n<!-- constraint-domains: daily-report, not-a-domain -->\n\n' +
			'后端 Controller 与 Service 调整。digest 阶段的 topic 参数与模型摘要截断（stage 判定）需要核对。\n',
	);
	fs.writeFileSync(path.join(FIXTURE, 'tasks.md'), '- [ ] smoke 任务\n');
	fs.writeFileSync(path.join(FIXTURE, 'stage.md'), '<!-- stage: implement -->\n\n# 阶段\n');
	fs.writeFileSync(
		path.join(FIXTURE, 'explore-findings.md'),
		'## 告警表结构\n\nalarm_record 按 tenant_id 分区。\n',
	);
}

// 域声明两处并集夹具（主线程追加契约）：proposal.md（存量位置）+ stage.md（实现期活口径）
//   dom-union：proposal 声明 reading + stage.md 声明 reading,scheduler → 生效域并集
//   dom-prop ：proposal 声明 daily-report + stage.md 无 constraint-domains → 沿用 proposal
const DOM_UNION_FIXTURE = path.join(REPO_ROOT, OPENSPEC_CHANGES_DIR_REL, 'smoke-fixture-dom-union');
const DOM_PROP_FIXTURE = path.join(REPO_ROOT, OPENSPEC_CHANGES_DIR_REL, 'smoke-fixture-dom-prop');
function writeDomainFixtures() {
	fs.mkdirSync(DOM_UNION_FIXTURE, { recursive: true });
	fs.writeFileSync(
		path.join(DOM_UNION_FIXTURE, 'proposal.md'),
		'# dom-union fixture\n\n<!-- constraint-domains: reading -->\n\n正文。\n',
	);
	fs.writeFileSync(
		path.join(DOM_UNION_FIXTURE, 'stage.md'),
		'<!-- stage: implement -->\n\n<!-- constraint-domains: reading,scheduler -->\n',
	);
	fs.mkdirSync(DOM_PROP_FIXTURE, { recursive: true });
	fs.writeFileSync(
		path.join(DOM_PROP_FIXTURE, 'proposal.md'),
		'# dom-prop fixture\n\n<!-- constraint-domains: daily-report -->\n\n正文。\n',
	);
	fs.writeFileSync(path.join(DOM_PROP_FIXTURE, 'stage.md'), '<!-- stage: implement -->\n');
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
	// 声明域 fixture（D7 关键词通道 + 预算降级用）：声明 kwdom → 关键词命中该域时进路由清单
	fs.mkdirSync(path.join(tmp, OPENSPEC_CHANGES_DIR_REL, 'smoke-fixture-budget'), { recursive: true });
	fs.writeFileSync(
		path.join(tmp, OPENSPEC_CHANGES_DIR_REL, 'smoke-fixture-budget', 'proposal.md'),
		'# budget fixture\n\n<!-- constraint-domains: kwdom -->\n\n正文。\n',
	);
	// stage.md：**阶段唯一真相源**——tmp 夹具 change 统一置 implement（implementation 族）
	for (const c of ['smoke-fixture-temp', 'smoke-fixture-decl', 'smoke-fixture-budget']) {
		fs.mkdirSync(path.join(tmp, OPENSPEC_CHANGES_DIR_REL, c), { recursive: true });
		fs.writeFileSync(
			path.join(tmp, OPENSPEC_CHANGES_DIR_REL, c, 'stage.md'),
			'<!-- stage: implement -->\n',
		);
	}

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
	// 无档位 JIT 记账夹具（enable-unbound-jit-injection）：域文档 + 标签信号。
	// 信号指向一个仓内不存在的路径（backend-go/internal/unboundmod/）——只在夹具里
	// 存在，用于断言「未激活档按编辑路径自愈」而不污染真实仓路径。
	const unboundItems = Array.from(
		{ length: 6 },
		(_, i) =>
			`${i + 1}. **UNBOUND-REDLINE-${i + 1} 该约束的自含红线句必须满足边界条件与例外说明**：细节层内容 DETAIL-UNBOUND-${i + 1}（不进入注入正文）。`,
	).join('\n');
	fs.writeFileSync(
		path.join(tmp, 'docs/reference/flow', 'unbound-dom.md'),
		'# unbound dom\n\n<!-- doc-impact-applies: backend-go/internal/unboundmod/ | section=业务约束与不变量 -->\n\n' +
			'## 业务约束与不变量\n\n' + unboundItems + '\n\n## 代码入口\n\n入口说明。\n',
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
		writeDomainFixtures();
		const checks = [];
		const check = (name, ok) => checks.push([name, ok]);

		// 0. 未绑定阶段：稳定层仅注索引（不借任何 change 的域/清单）
		await emit('session_start', { reason: 'new' });
		await emit('tool_execution_start', {
			toolName: 'read',
			args: { path: '/root/.pi/agent/skills/headroom/SKILL.md' },
		});
		let sp = await systemPrompt();

		check('未绑定阶段仅注索引（constraints-index）', sp.includes('### constraints-index.md'));
		check('未绑定阶段不注 flow 红线层', !sp.includes('auxiliary_label_dedupe_sim'));
		check('未绑定阶段不显示活跃变更（无 mtime 兜底错位）', /活跃变更：无/.test(sp));
		check('未绑定阶段不产出阶段路由清单（按清单标题锚定，防文档正文撞词）', !sp.includes('### 阶段路由清单（'));
		check('非 openspec skill 不绑定 change', !sp.includes(FIXTURE_NAME));

		// 1. 输入提及 change 名 → 绑定该 change；**阶段由其 stage.md 决定**（implement）
		await emit('input', { text: `smoke 绑定 ${FIXTURE_NAME}` });
		sp = await systemPrompt();

		check('阶段绑定生效（fixture stage.md=implement）', /阶段：implement（族：implementation）/.test(sp));
		check('绑定 fixture change', sp.includes(`活跃变更：${FIXTURE_NAME}（阶段绑定）`));
		check('约束索引注入（稳定层）', sp.includes('### constraints-index.md'));
		check('注入块头部宪法优先标注', sp.includes('与 AGENTS.md 优先级宪法冲突时，以宪法为准'));
		check('阶段路由清单注入（稳定层）', sp.includes('### 阶段路由清单（implement）'));
		check(
			'路由清单含声明域文档路径 + 取数命令（自主 read 指引）',
			sp.includes('docs/reference/flow/daily-report.md') &&
				sp.includes('bash scripts/harness/constraint-digest.sh --brief --domains daily-report'),
		);
		check('explore-findings 注入（implementation 族）', sp.includes('📌 探索阶段发现') && sp.includes('alarm_record'));
		check('阶段首推消息带阶段标签（📎 约束首推（阶段 implement））', sp.includes('📎 约束首推（阶段 implement）'));

		// 1.5 业务域声明红线层（阶段首推）：声明 daily-report → 送达其约束节**红线层**
		//      （顶层加粗句逐行 + 细节层取回指引）；**不含整节正文**（细节层自主 read）
		check(
			'声明注入 daily-report 红线层（标题标层 + 细节层指引尾行）',
			sp.includes('### daily-report.md') &&
				sp.includes('daily-report.md（红线层）') &&
				sp.includes('细节层：read `docs/reference/flow/daily-report.md`「业务约束与不变量」节'),
		);
		check(
			'红线层含首条红线句（顶层加粗块逐行）',
			sp.includes('同 board 同日重跑必须整份覆盖重建'),
		);
		check(
			'红线层不含细节层正文（整节不再主动注入）',
			!sp.includes('SaveReport` 按 `(semantic_board_id, period_date)` upsert'),
		);
		check('域声明头部提示（含未知域忽略）', /域声明：声明域：daily-report（忽略未知域：not-a-domain）/.test(sp));
		check('change 文本撞车词不注入 semantic-board', !sp.includes('### semantic-board.md'));
		check('change 文本撞车词不注入 topic-graph', !sp.includes('### topic-graph.md'));
		check('change 文本撞车词不注入 ai-summary', !sp.includes('### ai-summary.md'));

		// 3. 关键词通道**退役**（D4）：对话关键词命中不再送达整节正文——跨域词
		//    （「板块」→semantic-board、「cron」→scheduler）既不注正文也不进路由清单
		await emit('input', { text: '需要调整板块的标签展示' });
		sp = await systemPrompt();
		check('关键词通道退役: 跨域关键词命中零正文注入（semantic-board）', !sp.includes('### semantic-board.md'));
		check('关键词通道退役: 跨域关键词不进路由清单', !sp.includes('docs/reference/flow/semantic-board.md'));
		await emit('input', { text: 'cronjob 卡住需要修复' });
		sp = await systemPrompt();
		check('关键词通道退役: 跨域关键词零正文注入（scheduler）', !sp.includes('### scheduler.md'));
		// 声明域内的关键词 → 只在路由清单里**标注**（不新增文档、不注正文）
		await emit('input', { text: '日报的展示要调整' });
		sp = await systemPrompt();
		check(
			'关键词命中只作路由标注（声明域内条目标「本次会话提及」，零正文）',
			sp.includes('本次会话提及，优先') && !sp.includes('SaveReport` 按'),
		);

		// 4. 动态层 diff 稳态零重发（D3）：命中集合未变 → 后续回合零投递（缓存友好）
		const zeroSp = await systemPrompt();
		check('动态层稳态零重发（无新消息投递）', !zeroSp.includes('📎 约束补充'));

		// 5. 编辑路径命中（doc-impact-applies 标签）→ **零正文注入 + 进路由清单**
		//    （整节正文通道退役：改造前单次最坏 27.8KB；命中文档改由模型自主 read）
		await emit('tool_execution_start', {
			toolName: 'edit',
			args: { path: 'backend-go/internal/platform/airouter/router.go' },
		});
		sp = await systemPrompt();
		check('编辑路径命中零正文注入（ai-logging 不注正文）', !sp.includes('### ai-logging.md'));
		check('编辑路径命中零正文注入（ai-summary 不注正文）', !sp.includes('### ai-summary.md'));
		check(
			'编辑路径命中文档进路由清单（路径 + 节名，自主导入）',
			sp.includes('docs/reference/standard/backend/ai-logging.md') &&
				sp.includes('编辑路径命中，自主导入'),
		);
		const stableOnly5 = await systemPromptOnly();
		check(
			'稳定层不含命中文档正文（整节正文通道退役）',
			!stableOnly5.includes('所有 AI 调用必须经 airouter'),
		);

		// 6. pin_finding：已绑定阶段时落绑定 change
		const pinC = await invokePin({ title: 'pin-smoke-active', finding: '阶段内 pin 内容。' });
		check('pin（已绑定）落绑定 change', pinC.includes(`已存档到 ${OPENSPEC_CHANGES_DIR_REL}/${FIXTURE_NAME}`) && !pinC.includes('research'));
		check('pin（已绑定）写入绑定 change 文件', fs.readFileSync(path.join(FIXTURE, 'explore-findings.md'), 'utf8').includes('pin-smoke-active'));

		// 6.5 session_start reason 过滤（@bugfix）：startup = 子线程派发（pi-subagents
		//     createAgentSession 默认）不清零主会话状态；new/resume/fork/reload 才是真会话边界
		await emit('session_start', { reason: 'startup' });
		sp = await systemPrompt();
		check('session_start(startup=子线程派发) 不清零阶段', /阶段：implement/.test(sp));
		// startup 不清零：命中集合与指纹均保留 → 重复触碰同一路径零重投递（集合被清则重投）
		await emit('tool_execution_start', {
			toolName: 'edit',
			args: { path: 'backend-go/internal/platform/airouter/router.go' },
		});
		const afterStartupSp = await systemPrompt();
		check('session_start(startup) 不清零命中集合（重复命中零投递）', !afterStartupSp.includes('📎 约束补充'));

		// 7. session_start 会话边界重置：阶段/关键词命中/编辑路径命中/输入窗全部归零
		await emit('session_start', { reason: 'new' });
		sp = await systemPrompt();
		check('session_start 后阶段回落未绑定', /阶段：未绑定/.test(sp));
		check('session_start 后关键词命中清空', !sp.includes('auxiliary_label_dedupe_sim'));
		check('session_start 后编辑路径命中清空', !sp.includes('docs/reference/standard/backend/ai-logging.md'));

		// 8. 未绑定阶段时编辑命中标签路径 → **零正文注入**（整节通道退役；未绑定阶段
		//    也无路由清单 → 零动态层发送）
		await emit('tool_execution_start', {
			toolName: 'write',
			args: { path: 'backend-go/internal/platform/airouter/router.go' },
		});
		sp = await systemPrompt();
		check('未绑定阶段: 编辑命中路径零正文注入（ai-summary）', !sp.includes('### ai-summary.md'));
		check('未绑定阶段: 编辑命中路径零正文注入（ai-logging）', !sp.includes('### ai-logging.md'));
		check('未绑定阶段: 稳定层仍只索引（无路由清单）', /阶段：未绑定/.test(sp) && sp.includes('### constraints-index.md'));

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

		// 12.5 planning 族场景：读 openspec-explore skill → **不** mtime 兜底绑定
		//     （planning 族 = 新想法语境；兜底会绑到无关 change）——阶段不生效，pin 落 research
		await emit('session_start', { reason: 'new' });
		await emit('tool_execution_start', {
			toolName: 'read',
			args: { path: '/mnt/d/project/Syntopica/.agents/skills/openspec-explore/SKILL.md' },
		});
		sp = await systemPrompt();
		check('planning 族不兜底绑定（阶段未生效）', /阶段：未绑定/.test(sp) && /活跃变更：无/.test(sp));
		check('planning 族无绑定时不注 change 探索发现', !sp.includes('alarm_record'));
		const pinR = await invokePin({ title: 'pin-smoke-explore', finding: 'explore 新想法发现。' });
		check('未绑定阶段时 pin 落 research 库', pinR.includes('docs/research/explore-findings.md'));
		check('未绑定阶段时 pin 不落 change', !pinR.includes(OPENSPEC_CHANGES_DIR_REL));
		// 输入提及 change 名 → 正常绑定（阶段由其 stage.md 决定）
		await emit('input', { text: `/opsx-continue ${FIXTURE_NAME}` });
		sp = await systemPrompt();
		check('输入提及 change 名正常绑定', sp.includes(`活跃变更：${FIXTURE_NAME}（阶段绑定）`));
		await emit('session_start', { reason: 'new' }); // 收尾重置，不污染后续场景

		// 12. pin_finding：change 参数不存在 → research 库 + 警告
		const pinH = await invokePin({ title: 'pin-smoke-badchange', finding: '坏参数落点。', change: 'no-such-change-xyz' });
		check('pin（change 不存在）改存 research 库并警告', pinH.includes('⚠️') && pinH.includes('docs/research'));

		// 13. 临时 cwd：编辑路径命中（doc-impact-applies 标签）→ **零正文注入 + 进路由清单**；
		//     无声明 → 域声明提示。配置无 jitDocs（已移除）；命中源 = 文档头部标签。
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
		check(
			'标签命中零正文注入（FULLDOC-MARKER 不出现）',
			!tsp.includes('FULLDOC-MARKER') && !tsp.includes('REQ-MARKER'),
		);
		check(
			'标签命中文档进路由清单（路径 + 节名，自主导入）',
			tsp.includes('docs/reference/standard/backend/fixture-std.md') &&
				tsp.includes('编辑路径命中，自主导入'),
		);
		check(
			'残缺节文档同样只进路由清单（零正文）',
			tsp.includes('docs/reference/standard/backend/fixture-std2.md') && !tsp.includes('STD2-FULLDOC'),
		);
		check('无声明 → 域声明提示（纯工具链 change 可忽略）', /域声明：无域声明（纯工具链 change 可忽略）/.test(tsp));
		check('无声明 → 不注入 flow 约束节', !tsp.includes('业务约束与不变量'));
		check('阶段路由清单存在（implement 族）', tsp.includes('### 阶段路由清单（implement）'));

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
		// 整节正文通道退役：编辑路径命中的文档**零正文记账**（无 jit-path / keyword reason 行）
		check(
			'编辑路径命中零正文记账（无 jit-path / keyword 原因行）',
			!injects.some((p) => p.reason === 'jit-path' || p.reason === 'keyword'),
		);
		check(
			'路由清单单独记账（reason=routing + 伪路径 stage-routing）',
			injects.some((p) => p.reason === 'routing' && p.path === 'stage-routing'),
		);
		check('change 文件经动态层投递（探索发现标题在投递内容中）', tsp.includes('### 📌 探索阶段发现'));
		evDb.close();

		// 14.5 声明域红线层三态（@constraint-declaration-redline + review R-3）：规整节→红线层注入；
		//      无加粗节/低字节红线层→**取数指引**（零正文，整节回退已退役）；记账 layer 如实。
		//      结束绑回 temp（17 的 resume 断言依赖本会话最后一条 stage.set 是 smoke-fixture-temp）
		await emit('input', { text: '/opsx-apply smoke-fixture-decl' }, tmpCtx);
		const declSp = await systemPrompt(tmpCtx);
		check('decl: 规整节红线层注入（首尾红线句逐行可见）', declSp.includes('### redline-ok.md') && declSp.includes('1. **REDLINE-OK-1') && declSp.includes('6. **REDLINE-OK-6'));
		check('decl: 红线层不含细节层内容', !declSp.includes('DETAIL-OK-'));
		check('decl: 红线层附细节层取回指引尾行', declSp.includes('细节层：read `docs/reference/flow/redline-ok.md`「业务约束与不变量」节'));
		check('decl: 标题标红线层', declSp.includes('redline-ok.md（红线层）'));
		check('decl: 无加粗节 → 取数指引（零正文，含 digest 命令）', !declSp.includes('NOBOLD-DETAIL-2') && declSp.includes('redline-nobold.md（取数指引）') && declSp.includes('--brief --domains redline-nobold'));
		check('decl: 低字节红线层 → 取数指引（零正文）', !declSp.includes('DETAIL-TINY-1') && declSp.includes('redline-tiny.md（取数指引）'));
		const declDb = new EvDb(path.join(tmp, '.pi', 'harness', 'events.db'), { readOnly: true });
		const declRows = declDb.prepare("SELECT payload FROM events WHERE kind='constraint.inject' AND session_id='ci-smoke-pin' AND payload LIKE '%redline-%'").all().map((r) => JSON.parse(r.payload));
		check(
			'decl: 记账 layer 标记（ok=redline / nobold+tiny=pointer）',
			declRows.some((p) => p.reason === 'declaration' && p.path.endsWith('redline-ok.md') && p.layer === 'redline' && typeof p.bytes === 'number') &&
				declRows.some((p) => p.reason === 'declaration' && p.path.endsWith('redline-nobold.md') && p.layer === 'pointer') &&
				declRows.some((p) => p.reason === 'declaration' && p.path.endsWith('redline-tiny.md') && p.layer === 'pointer'),
		);
		check(
			'decl: 取数指引远小于全节回退的旧形态（瘦身收益可观测）',
			(declRows.find((p) => p.path.endsWith('redline-nobold.md') && p.layer === 'pointer') || {}).bytes < 600,
		);
		const allInjRows = declDb.prepare("SELECT payload FROM events WHERE kind='constraint.inject'").all().map((r) => JSON.parse(r.payload));
		check(
			'注入体量：declaration（红线层/取数指引）与 routing 条目均 ≤4000 字节（tasks 9.3）',
			allInjRows
				.filter((p) => p.reason === 'declaration' || p.reason === 'routing')
				.every((p) => typeof p.bytes === 'number' && p.bytes <= 4000),
		);
		declDb.close();
		await emit('input', { text: '/opsx-apply smoke-fixture-temp' }, tmpCtx);

		// 14.6 域声明两处并集（主线程追加契约）：proposal.md（存量位置）+ stage.md（实现期
		//      活口径）取**并集**；stage.md 不得收窄 proposal 的域（超集安全方向）。
		//      跑在 REPO_ROOT（真实 flow 域名 reading / scheduler / daily-report 才过域名校验）。
		await emit('session_start', { reason: 'new' });
		await emit('input', { text: '/opsx-apply smoke-fixture-dom-union' });
		const unionSp = await systemPrompt();
		check(
			'域声明并集: proposal(reading) + stage.md(reading,scheduler) → 生效域含两域',
			unionSp.includes('docs/reference/flow/reading.md') &&
				unionSp.includes('docs/reference/flow/scheduler.md'),
		);
		check(
			'域声明并集: 头部域声明提示含并集结果',
			/域声明：声明域：reading, scheduler/.test(unionSp),
		);
		await emit('session_start', { reason: 'new' });
		await emit('input', { text: '/opsx-apply smoke-fixture-dom-prop' });
		const propSp = await systemPrompt();
		check(
			'域声明并集: stage.md 无 constraint-domains → 沿用 proposal 的域',
			propSp.includes('docs/reference/flow/daily-report.md') &&
				!propSp.includes('docs/reference/flow/scheduler.md'),
		);
		check(
			'域声明并集: 头部域声明提示沿用 proposal 域',
			/域声明：声明域：daily-report/.test(propSp),
		);
		await emit('session_start', { reason: 'new' }); // 收尾重置（不污染后续场景）

		// 17. resume 阶段恢复（stage.set + 双读）：两段式取数 + new 不恢复 + 绑定失效回落
		await emit('session_start', { reason: 'new' }, tmpCtx);
		let rsp = await systemPrompt(tmpCtx);
		check('resume前置: new 清零且不恢复（db 有 stage.set 也不继承）', /阶段：未绑定/.test(rsp));
		await emit('session_start', { reason: 'resume' }, tmpCtx);
		rsp = await systemPrompt(tmpCtx);
		check('resume 同 sessionId 恢复阶段（queryLatestStageSet）', /阶段：implement/.test(rsp));
		check('resume 恢复 change 绑定', rsp.includes('活跃变更：smoke-fixture-temp'));
		check('resume 不恢复粘性命中（编辑路径命中清空，有意为之）', !rsp.includes('docs/reference/standard/backend/fixture-std.md'));
		const tmpCtxNew = { cwd: tmp, hasUI: false, sessionManager: { getSessionId: () => 'ci-smoke-resume-2' } };
		await emit('session_start', { reason: 'resume' }, tmpCtxNew);
		rsp = await systemPrompt(tmpCtxNew);
		check('resume 无自身阶段历史：不继承他窗口状态（无全局兑底，2026-08-23 实测回归）', /阶段：未绑定/.test(rsp) && !rsp.includes('smoke-fixture-temp'));
		await emit('session_start', { reason: 'reload' }, tmpCtxNew);
		rsp = await systemPrompt(tmpCtxNew);
		check('reload 无自身阶段历史：同样不继承（未绑定）', /阶段：未绑定/.test(rsp));
		const tmpChangeDir = path.join(tmp, OPENSPEC_CHANGES_DIR_REL, 'smoke-fixture-temp');
		fs.renameSync(tmpChangeDir, `${tmpChangeDir}-bak`);
		await emit('session_start', { reason: 'resume' }, tmpCtxNew);
		rsp = await systemPrompt(tmpCtxNew);
		check('resume 绑定 change 已不存在 → 回落未绑定', /阶段：未绑定/.test(rsp));
		fs.renameSync(`${tmpChangeDir}-bak`, tmpChangeDir);
	// R-1 回归（review High）：先绑定有阶段的 temp(implement)，再切到无 stage.md 的 legacy
	// change → 事实库最新 stage.set 是 stage=null 合法绑定行；resume 必须恢复「绑定 legacy、
	// 阶段未绑定」，MUST NOT 回退读到更早的 temp(implement) 行（门禁/继承同理走同一取数）。
	const legacyDir = path.join(tmp, OPENSPEC_CHANGES_DIR_REL, 'smoke-fixture-legacy');
	fs.mkdirSync(legacyDir, { recursive: true });
	fs.writeFileSync(path.join(legacyDir, 'proposal.md'), '<!-- complexity: minimal -->\n\n# legacy fixture（无 stage.md）\n');
	await emit('input', { text: '/opsx-apply smoke-fixture-legacy' }, tmpCtx);
	let rspR1 = await systemPrompt(tmpCtx);
	check('R-1前置: 绑定 legacy change → 阶段未绑定 + 活跃变更为该 change', /阶段：未绑定（仅索引）/.test(rspR1) && rspR1.includes('活跃变更：smoke-fixture-legacy'));
	await emit('session_start', { reason: 'resume' }, tmpCtx);
	rspR1 = await systemPrompt(tmpCtx);
	check(
		'R-1: resume 到 legacy 绑定行 → 恢复 legacy 绑定而非回退旧阶段（无 temp/implement）',
		/阶段：未绑定（仅索引）/.test(rspR1) &&
			rspR1.includes('活跃变更：smoke-fixture-legacy') &&
			!rspR1.includes('smoke-fixture-temp') &&
			!/阶段：implement/.test(rspR1),
	);
	await emit('input', { text: '/opsx-apply smoke-fixture-temp' }, tmpCtx); // 收尾绑回 temp（后续 startup 断言依赖）
		// startup 双面孔（@constraint-injection-tier-b 6.5 返工：真实链路 quit 重启走 startup）：
		// ① 冷启动（阶段空）+ 同 sessionId 有 stage.set → 第 1 段恢复，不全局兜底；
		// ② 子线程派发（阶段非空）→ 不清零不动；③ 全新会话 startup → 不继承其他会话状态
		await emit('session_start', { reason: 'startup' }, tmpCtx); // 场景17末尾恢复失败阶段空 → 冷启动路径
		rsp = await systemPrompt(tmpCtx);
		check('startup 冷启动：同 sessionId 恢复阶段（真实路径：quit 重启）', /阶段：implement/.test(rsp) && rsp.includes('活跃变更：smoke-fixture-temp'));
		check('startup 恢复不带回粘性命中（编辑路径命中仍空）', !rsp.includes('docs/reference/standard/backend/fixture-std.md'));
		await emit('tool_execution_start', { toolName: 'edit', args: { path: 'backend-go/internal/platform/airouter/router.go' } }, tmpCtx);
		await emit('session_start', { reason: 'startup' }, tmpCtx); // 阶段非空 = 子线程派发 → 不动
		rsp = await systemPrompt(tmpCtx);
		check('startup 子线程派发：不清零不重恢复（阶段/命中均保持）', /阶段：implement/.test(rsp) && rsp.includes('docs/reference/standard/backend/fixture-std.md'));
		const freshCtx = { cwd: tmp, hasUI: false, sessionManager: { getSessionId: () => 'ci-smoke-fresh-session' } };
		await emit('session_start', { reason: 'new' }, freshCtx); // 模拟新进程空状态（真实世界新会话=新模块实例）
		await emit('session_start', { reason: 'startup' }, freshCtx); // 冷启动 + 无本会话 stage.set
		rsp = await systemPrompt(freshCtx);
		check('startup 全新会话：不继承其他会话状态（无全局兜底）', /阶段：未绑定/.test(rsp));
		// reload 恢复：同会话扩展重载，id 不变 → 第 1 段必中
		await emit('session_start', { reason: 'reload' }, tmpCtx);
		rsp = await systemPrompt(tmpCtx);
		check('reload：清零后恢复阶段（同 sessionId 第 1 段）', /阶段：implement/.test(rsp) && rsp.includes('活跃变更：smoke-fixture-temp'));
		await emit('session_start', { reason: 'new' }); // 收尾重置（共享 ctx 场景隔离）
		// stage.set 记账核验（触发点①input 已在场景13触发）
		const msDb = new EvDb(path.join(tmp, '.pi', 'harness', 'events.db'), { readOnly: true });
		const msRows = msDb.prepare("SELECT payload FROM events WHERE kind='stage.set'").all().map((r) => JSON.parse(r.payload));
		check(
			'stage.set 记账存在（stage+stageFamily+boundChange）',
			msRows.length >= 1 &&
				msRows.some(
					(p) => p.stage === 'implement' && p.stageFamily === 'implementation' && p.boundChange === 'smoke-fixture-temp',
				),
		);
		check(
			'stage.set 停写 legacy mode.set（写侧已全部切 stage.set）',
			msDb.prepare("SELECT COUNT(*) AS n FROM events WHERE kind='mode.set'").get().n === 0,
		);
		msDb.close();

		// 18. 预算降级（@constraint-injection-tier-b D1-D4 + stage-driven 降级顺位调整）：
		//     分层顺序 = **红线层 → 路由清单 → change-file**（正文仍不真丢）
		const pad = (marker) => Array.from({ length: 60 }, (_, i) => `${marker} 填充行${i} ${'X'.repeat(40)}`).join('\n');
		const kwRedlines = Array.from({ length: 6 }, (_, i) =>
			`${i + 1}. **KWDOM-REDLINE-${i + 1} ${'约束主体必须满足的边界条件与例外说明以及适用范围的具体界定与失效处理路径说明'.repeat(6)}**：细节层 ${pad('BIGKW-MARKER').split('\n').join(' ')}`,
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
		check('预算: 红线层先降（细节层全文消失）', !dsp.includes('BIGKW-MARKER'));
		check(
			'预算: 永不真丢（kwdom 标题占位 + read 指引 + 占位预览）',
			dsp.includes('### kwdom.md') &&
				dsp.includes('read docs/reference/flow/kwdom.md') &&
				dsp.includes('预算已满'),
		);
		check('预算: 编辑路径命中文档零正文（bigjit 不注正文）', !dsp.includes('BIGJIT-MARKER') && !dsp.includes('### bigjit.md'));
		check('预算: 索引不降级（index 恒在）', dsp.includes('### idx.md'));
		check('预算: change-file 未到占位层（findings 全文保留）', dsp.includes('内容A'));
		check('预算: 头部模型可见降级通知', /预算降级/.test(dsp) && /kwdom\.md/.test(dsp));
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
		check(
			'编辑路径命中 test-design → 进路由清单（零正文）',
			sp.includes('docs/reference/standard/shared/test-design.md') && !sp.includes('### test-design.md'),
		);
		check('编辑路径命中零正文注入（摘要节本体不可见）', !sp.includes('五问句') && !sp.includes('问句①：节拍全吗'));

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
			'D3: 稳定层 key 用**阶段族**（同族切换不重建，跨族/换绑定才变）',
			ext.stableSnapshotKey('implementation', 'a') === ext.stableSnapshotKey('implementation', 'a') &&
				ext.stableSnapshotKey('planning', 'a') !== ext.stableSnapshotKey('implementation', 'a') &&
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
			'D1: splitPlanItems 稳定/动态分层（index+routing→稳定，declaration/change-file→动态）',
			(() => {
				const sp = ext.splitPlanItems([
					{ key: 'a', reason: 'index' },
					{ key: 'b', reason: 'routing' },
					{ key: 'c', reason: 'declaration' },
					{ key: 'd', reason: 'change-file' },
				]);
				return sp.stable.length === 2 && sp.dynamic.length === 2;
			})(),
		);

		// 17.2 行为：混合通道（tmp 场景，预算充足 + 声明域红线层）
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
		const readStageSet = (dir) => {
			const { DatabaseSync } = require('node:sqlite');
			const db = new DatabaseSync(path.join(dir, '.pi', 'harness', 'events.db'), { readOnly: true });
			const rows = db.prepare("SELECT payload FROM events WHERE kind='stage.set'").all().map((r) => JSON.parse(r.payload));
			db.close();
			return rows;
		};
		// stage.set 全量（含 session_id，供「记在谁名下」断言）；seedStageSet 造一条父会话历史
		// （模拟跨进程：父会话不在本进程内存，只有事实库记录）。
		const readStageSetFull = (dir) => {
			const { DatabaseSync } = require('node:sqlite');
			const db = new DatabaseSync(path.join(dir, '.pi', 'harness', 'events.db'), { readOnly: true });
			const rows = db
				.prepare("SELECT session_id, change, payload FROM events WHERE kind='stage.set' ORDER BY id")
				.all()
				.map((r) => ({ ...JSON.parse(r.payload), session_id: r.session_id, change: r.change }));
			db.close();
			return rows;
		};
		const seedStageSet = (dir, sessionId, stage, boundChange, source = 'skill') => {
			const { DatabaseSync } = require('node:sqlite');
			const db = new DatabaseSync(path.join(dir, '.pi', 'harness', 'events.db'));
			db.prepare("INSERT INTO events (ts, session_id, kind, change, payload) VALUES (?, ?, 'stage.set', ?, ?)").run(
				new Date().toISOString(),
				sessionId,
				boundChange,
				JSON.stringify({ stage, stageFamily: stage === 'implement' ? 'implementation' : 'planning', boundChange, source }),
			);
			db.close();
		};
		writeCfg(bigCfg, 1);
		await emit('session_start', { reason: 'new' }, tmpCtx);
		await emit('input', { text: '/opsx-apply smoke-fixture-budget' }, tmpCtx);
		const s17a = await systemPromptOnly(tmpCtx);
		const s17b = await systemPromptOnly(tmpCtx);
		check(
			'行为: 稳定层阶段生命周期内字节恒定',
			s17a === s17b && s17a.includes('活跃变更：smoke-fixture-budget') && s17a.includes('阶段路由清单（implement）'),
		);
		check('行为: 稳定层零业务域正文（红线层走动态层）', !s17a.includes('KWDOM-REDLINE-1'));
		const s17Dyn = await systemPrompt(tmpCtx);
		check('行为: 阶段首推红线层经动态层送达', s17Dyn.includes('KWDOM-REDLINE-1'));
		await emit('input', { text: '大预算词相关展示调整' }, tmpCtx);
		const kwSp = await systemPrompt(tmpCtx);
		check('行为: 关键词命中零正文注入（细节层不进上下文）', !kwSp.includes('BIGKW-MARKER'));
		const kwStable = await systemPromptOnly(tmpCtx);
		check('行为: 动态层内容不进 system prompt', !kwStable.includes('KWDOM-REDLINE-1'));
		check('行为: 动态层稳态零重发', !(await systemPrompt(tmpCtx)).includes('📎 约束补充'));
		await emit('session_compact', { reason: 'manual' }, tmpCtx);
		const csSp = await systemPrompt(tmpCtx);
		check('行为: compact 后重发约束快照', csSp.includes('约束快照（compact 后重发）') && csSp.includes('KWDOM-REDLINE-1'));
		// 17.3 行为：绑定污染修复（read/健康写不抢绑、兜底绑、turn 锁定、恢复记账、source 不变量）
		await emit('session_start', { reason: 'resume' }, tmpCtx);
		let msRows17 = readStageSet(tmp);
		check('行为: 恢复路径记账 source=recover', msRows17.some((r) => r.source === 'recover'));
		await emit('session_start', { reason: 'new' }, tmpCtx);
		await emit('input', { text: '/opsx-apply smoke-fixture-budget' }, tmpCtx);
		await systemPromptOnly(tmpCtx); // turn 起点（解锁）
		const msBefore17 = readStageSet(tmp).length;
		await emit('tool_execution_start', { toolName: 'read', args: { path: 'openspec/changes/smoke-fixture-temp/tasks.md' } }, tmpCtx);
		await emit('tool_execution_start', { toolName: 'write', args: { path: 'openspec/changes/smoke-fixture-temp/proposal.md' } }, tmpCtx);
		msRows17 = readStageSet(tmp);
		check('行为: read / 健康绑定写其他 change 目录零抢绑（零新增记账）', msRows17.length === msBefore17);
		for (const n of ['bind-a', 'bind-b', 'bind-c']) fs.mkdirSync(path.join(tmp, OPENSPEC_CHANGES_DIR_REL, n), { recursive: true });
		// bind-c 带声明域（kwdom）：使 19.6 的「B 绑定生效」可观测——绑定生效时其声明域
		// 红线层与路由清单可断言。
		fs.writeFileSync(
			path.join(tmp, OPENSPEC_CHANGES_DIR_REL, 'bind-c', 'proposal.md'),
			'# bind-c fixture\n\n<!-- constraint-domains: kwdom -->\n\n正文。\n',
		);
		// bind-a/b/c 置 **planning 族**（stage=design）：mtime 兜底只绑 implementation 族
		// → 新想法语境不兜底；绑定只由「写 change 目录」（edit-dir）触发。
		for (const n of ['bind-a', 'bind-b', 'bind-c']) {
			fs.writeFileSync(path.join(tmp, OPENSPEC_CHANGES_DIR_REL, n, 'stage.md'), '<!-- stage: design -->\n');
		}
		await emit('session_start', { reason: 'new' }, tmpCtx);
		await emit('input', { text: '/opsx-new' }, tmpCtx); // 工作流命令：记意图（不提 change）
		await systemPromptOnly(tmpCtx); // turn 起点（解锁）：planning 族活跃 change 不兜底
		await emit('tool_execution_start', { toolName: 'write', args: { path: 'openspec/changes/bind-a/x.md' } }, tmpCtx);
		await emit('tool_execution_start', { toolName: 'write', args: { path: 'openspec/changes/bind-b/x.md' } }, tmpCtx);
		msRows17 = readStageSet(tmp);
		const editDirBinds = msRows17.filter((r) => r.source === 'edit-dir').map((r) => r.boundChange);
		check('行为: 无绑定写 change 目录兜底绑 + 同 turn 锁定（仅首个生效）', editDirBinds.length === 1 && editDirBinds[0] === 'bind-a');
		await systemPromptOnly(tmpCtx); // 新 turn（解锁）
		fs.rmSync(path.join(tmp, OPENSPEC_CHANGES_DIR_REL, 'bind-a'), { recursive: true, force: true });
		await emit('tool_execution_start', { toolName: 'write', args: { path: 'openspec/changes/bind-b/x.md' } }, tmpCtx);
		msRows17 = readStageSet(tmp);
		check('行为: 锁定仅限同 turn（新 turn 内可再切绑）', msRows17.some((r) => r.source === 'edit-dir' && r.boundChange === 'bind-b'));
		check(
			'行为: stage.set 全路径带 source（隐性绑定不存在）',
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
		//     背景（2026-09-17 事实库）：绑定状态曾是模块级全局，他会话的绑定记账会决定本会话
		//     注入谁的约束（会话 01a0aaf4 零绑定记账，却在另一会话绑定 6 秒后拿到
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
			/阶段：未绑定/.test(cSp) &&
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
			/阶段：implement/.test(childSp) && childSp.includes('活跃变更：smoke-fixture-budget'),
		);
		check(
			'隔离: 同进程子线程继承父指纹 → 父已送达红线层不重投（零冗余）',
			!childSp.includes('### kwdom.md'),
		);
		check('隔离: 子线程稳定层含父会话阶段路由清单', childSp.includes('阶段路由清单（implement）'));
		const inheritRows = readStageSet(tmp).filter((r) => r.source === 'inherit');
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
			'隔离: 子会话自身命中生效（编辑路径进路由清单、零正文）',
			childHits.includes('docs/reference/standard/backend/bigjit.md') &&
				!childHits.includes('BIGKW-MARKER') &&
				!childHits.includes('### bigjit.md'),
		);
		markMessages();
		const aAfterChildHits = await systemPrompt(sessA);
		check('隔离: 子线程关键词命中不回写主会话命中集', !aAfterChildHits.includes('BIGKW-MARKER'));
		check('隔离: 子线程编辑路径命中不回写主会话命中集', !aAfterChildHits.includes('bigjit'));

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
		check('隔离: 父会话不可证 → 不继承（未激活）', /阶段：未绑定/.test(orphanSp));

		// 19.10 跨进程继承（事实库回退）：父会话**不在本进程内存**（模拟 pi-subagents
		//       子线程 = 独立 node 进程，2026-09-17 实测），但父会话的 stage.set 历史在共享
		//       事实库 → 子会话据 header 的 parentSession 采纳阶段与绑定。
		//       判别力：把 inheritFromParentHistory 短路（开头 return false）→ 本节3 条断言变红。
		const xprocParent = 'ci-smoke-parent-xproc';
		seedStageSet(tmp, xprocParent, 'implement', 'smoke-fixture-budget');
		const beforeXproc = readStageSetFull(tmp).length;
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
			/阶段：implement/.test(xprocSp) && xprocSp.includes('活跃变更：smoke-fixture-budget'),
		);
		check('跨进程: 子线程带父会话声明域红线层（kwdom）', xprocSp.includes('### kwdom.md'));
		const xprocInheritRows = readStageSetFull(tmp)
			.slice(beforeXproc)
			.filter((r) => r.source === 'inherit');
		check(
			'跨进程: 继承显式记账 source=inherit 且记在子会话名下',
			xprocInheritRows.length === 1 &&
				xprocInheritRows[0].boundChange === 'smoke-fixture-budget' &&
				xprocInheritRows[0].session_id === 'ci-smoke-child-xproc',
		);

		// 19.10b R-1 回归（review High）：父会话最新一条是 legacy 绑定行（stage=null 且
		//       stageFamily=null）时，跨进程子线程必须继承「绑定该 change、阶段未绑定」，
		//       不得回退读到父会话更早的 implement 行（借旧 change/旧阶段）。
		const r1Parent = 'ci-smoke-parent-r1';
		seedStageSet(tmp, r1Parent, 'implement', 'smoke-fixture-temp');
		{
			const { DatabaseSync } = require('node:sqlite');
			const db = new DatabaseSync(path.join(tmp, '.pi', 'harness', 'events.db'));
			db.prepare("INSERT INTO events (ts, session_id, kind, change, payload) VALUES (?, ?, 'stage.set', ?, ?)").run(
				new Date().toISOString(),
				r1Parent,
				'smoke-fixture-legacy',
				JSON.stringify({ stage: null, stageFamily: null, boundChange: 'smoke-fixture-legacy', source: 'fallback' }),
			);
			db.close();
		}
		const r1Child = {
			cwd: tmp,
			hasUI: false,
			sessionManager: {
				getSessionId: () => 'ci-smoke-child-r1',
				getHeader: () => ({
					parentSession: `/home/x/.pi/agent/sessions/dir/2026-09-17T00-00-00-000Z_${r1Parent}.jsonl`,
				}),
			},
		};
		await emit('session_start', { reason: 'startup' }, r1Child);
		const r1ChildSp = await systemPrompt(r1Child);
		check(
			'R-1: 跨进程继承 legacy 绑定行 → 继承绑定而非回退旧阶段（无 temp/implement）',
			/阶段：未绑定（仅索引）/.test(r1ChildSp) &&
				r1ChildSp.includes('活跃变更：smoke-fixture-legacy') &&
				!r1ChildSp.includes('smoke-fixture-temp') &&
				!/阶段：implement/.test(r1ChildSp),
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
			/阶段：implement/.test(pathChildSp) &&
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
			/阶段：未绑定/.test(await systemPromptOnly(noSrcChild)),
		);

		// 19.13 pi-web 子会话继承（harden-subagent-constraint-channel T3/D3）：pi-web Agent
		//       工具派发的实现档子线程与 pi-subagents 同形——独立 sessionId + session_start
		//       reason=startup + header parentSession（2026-09-18 实测样本 01a0b314 转录头）。
		//       期望：stage.set source=inherit 记在子会话名下 + 注入带父会话绑定的 change
		//       （稳定层 header 与 constraint.inject 记账 change 列均为绑定 change）。
		const piwebParent = 'ci-smoke-parent-piweb';
		seedStageSet(tmp, piwebParent, 'implement', 'bind-c');
		const beforePiweb = readStageSetFull(tmp).length;
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
			/阶段：implement/.test(piwebSp) && piwebSp.includes('活跃变更：bind-c（阶段绑定）') && piwebSp.includes('### kwdom.md'),
		);
		const piwebInherit = readStageSetFull(tmp)
			.slice(beforePiweb)
			.filter((r) => r.source === 'inherit');
		check(
			'pi-web: stage.set source=inherit 记在子会话名下且带绑定 change',
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
				initMsgs[0].msg.content.includes('阶段：未绑定') &&
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
			mainSp.startsWith('BASE') && mainSp.includes('### idx.md') && mainSp.includes('阶段：未绑定'),
		);
		const mainChan = ext.channelStateForTest('ci-smoke-main-t3rev');
		check(
			'T3-rev③: 主会话 childInitDelivered 恒 false',
			!!mainChan && mainChan.childInitDelivered === false && mainChan.stableSnapshotKey === 'none|none',
		);

		// 19.5 命中集按会话隔离（关键词 + JIT 各一例）
		await emit('input', { text: '大预算词相关展示调整' }, sessA);
		const aKw = await systemPrompt(sessA);
		check('隔离: A 关键词命中零正文注入（细节层不可见）', !aKw.includes('BIGKW-MARKER'));
		markMessages();
		await systemPromptOnly(sessB);
		check('隔离: 他会话关键词命中不投递给本会话', !dynamicText().includes('BIGKW-MARKER'));
		await emit(
			'tool_execution_start',
			{ toolName: 'edit', args: { path: 'backend-go/internal/bigmod/service.go' } },
			sessA,
		);
		const aJit = await systemPrompt(sessA);
		check(
			'隔离: A 编辑路径命中进路由清单（零正文）',
			aJit.includes('docs/reference/standard/backend/bigjit.md') && !aJit.includes('### bigjit.md'),
		);
		markMessages();
		await systemPromptOnly(sessB);
		check('隔离: 他会话编辑路径命中不投递给本会话', !dynamicText().includes('bigjit'));

		// 19.6 turn 绑定锁按会话隔离。
		//     判别力顺序（review P2-4 修正）：必须让 **A 先持锁**、再让 B 写 change 目录。
		//     旧顺序先在 B 上跑 before_agent_start —— turn 起点本就解 B 的锁，于是「锁是模块级全局」
		//     的实现也会被解开，断言恒绿（不具判别力）。固定顺序：B 进 turn（解 B 锁）→ A 命令绑定
		//     （置 A 锁）→ B 写 bind-c。全局锁实现下 B 绑不上（红）；per-session 锁下 B 正常绑定（绿）。
		await emit('session_start', { reason: 'new' }, sessB);
		await emit('input', { text: '/opsx-new' }, sessB); // requirements 档：无 mtime 兜底 → 无绑定
		await systemPromptOnly(sessB); // B 的 turn 起点（解 B 的锁）
		const msBeforeLock = readStageSet(tmp).length;
		await emit('input', { text: '/opsx-apply smoke-fixture-budget' }, sessA); // A 命令绑定 → 置 A 的锁
		const msAfterLock = readStageSet(tmp);
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
		const cBinds = readStageSet(tmp).filter((r) => r.source === 'edit-dir' && r.boundChange === 'bind-c');
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

		// 20. 指纹按 regime 标记（fix-injection-transition-fingerprint-wipe，阶段化后按**阶段族**）：
		//     阶段/绑定可 mid-turn（tool_execution_start）变化而快照重建延迟到下一 turn 起点，
		//     延迟重建的指纹清空仅当 regime≠新快照 key —— mid-turn 切阶段后同 turn 已投递的
		//     内容不被误清重投；真实 regime 切换（族切换）仍全量重投。
		writeCfg(
			{
				...bigCfg,
				skillSignals: { requirements: ['openspec-new-change'], implementation: ['openspec-apply-change'] },
			},
			10,
		);
		const sessW = { cwd: tmp, hasUI: false, sessionManager: { getSessionId: () => 'ci-smoke-wipe' } };
		const stageFile = path.join(tmp, OPENSPEC_CHANGES_DIR_REL, 'smoke-fixture-budget', 'stage.md');
		// 场景 A：同族内阶段切换（implement → verify，同属 implementation 族）
		//   → 快照键（stageFamily|boundChange）不变 → 稳定层不重建 → 已投递内容零重投
		await emit('session_start', { reason: 'new' }, sessW);
		await emit('input', { text: '/opsx-apply smoke-fixture-budget' }, sessW);
		const wSp1 = await systemPromptOnly(sessW);
		check('regime: 阶段绑定后稳定层建快照（implement 族路由清单）', wSp1.includes('阶段路由清单（implement）'));
		markMessages();
		fs.writeFileSync(stageFile, '<!-- stage: verify -->\n');
		await emit(
			'tool_execution_start',
			{ toolName: 'write', args: { path: 'openspec/changes/smoke-fixture-budget/stage.md' } },
			sessW,
		);
		check(
			'regime: mid-turn 阶段变更记账（source=edit-dir + stage=verify）',
			readStageSet(tmp).some((r) => r.source === 'edit-dir' && r.stage === 'verify'),
		);
		check('regime: mid-turn 切阶段后同 turn 已投递内容零重投', !dynamicText().includes('📎'));
		const wSp2 = await systemPromptOnly(sessW); // 下一 turn：同族 → 快照键不变 → 不重建
		check('regime: 同族切阶段 → 稳定层不重建（快照键=阶段族不变）', wSp2 === wSp1);
		markMessages();
		await systemPromptOnly(sessW); // 隔 N turn 仍零重投
		check('regime: 稳态零投递保持（指纹+regime 存续）', !dynamicText().includes('📎'));
		markMessages();
		// 场景 B（反向护栏）：真实 regime 切换（族切换 implementation → planning）
		//   → 指纹清空 → 全量重投
		fs.writeFileSync(stageFile, '<!-- stage: design -->\n');
		await emit(
			'tool_execution_start',
			{ toolName: 'write', args: { path: 'openspec/changes/smoke-fixture-budget/stage.md' } },
			sessW,
		);
		markMessages();
		const wSp3 = await systemPromptOnly(sessW); // 族切换 → 快照重建
		check(
			'regime: 族切换后稳定层重建（新阶段路由清单）',
			wSp3 !== wSp1 && wSp3.includes('阶段路由清单（design）'),
		);
		check(
			'regime: 真实 regime 切换仍全量重投（含已投递过的红线层）',
			dynamicText().includes('KWDOM-REDLINE-1'),
		);
		markMessages();
		// 场景 C（fork 变体）：父已投递、快照已重建 → 子会话继承父 channel（指纹 + regime）
		//   → 子首 turn 快照重建但父已投递内容不重发（fork 语义）
		const sessP = { cwd: tmp, hasUI: false, sessionManager: { getSessionId: () => 'ci-smoke-wipe-parent' } };
		await emit('session_start', { reason: 'new' }, sessP);
		await emit('input', { text: '/opsx-apply smoke-fixture-budget' }, sessP);
		await systemPromptOnly(sessP);
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
		await systemPromptOnly(childW); // 子首 turn：防双投（systemPrompt 不再拼稳定层块）
		check(
			'regime: fork 子会话继承父阶段（session_start 消息投递稳定层）',
			dynamicText().includes('阶段路由清单'),
		);
		check(
			'regime: fork 子会话父已投递内容不重发（fork 指纹继承语义保持）',
			!dynamicText().includes('KWDOM-REDLINE-1'),
		);
		markMessages();
		// 21. 编辑路径命中 → **只进路由清单**（整节正文通道退役，stage-driven D4）
		//     契约：命中文档**零正文注入**；已绑定阶段时进本阶段路由清单（路径 + 节名），
		//     由模型自主 read；未绑定阶段时无路由清单 → 零投递。
		// 21.1 未绑定阶段：edit airouter → 零正文、零动态层发送
		await emit('session_start', { reason: 'new' });
		markMessages();
		const uSp0 = await systemPromptOnly();
		const uStableBytes = Buffer.byteLength(uSp0, 'utf8');
		check(
			'route-hit-1: 未绑定阶段稳定层仅索引（无路由清单）',
			uSp0.includes('### constraints-index.md') && /阶段：未绑定/.test(uSp0) && !uSp0.includes('阶段路由清单'),
		);
		markMessages();
		await emit('tool_execution_start', {
			toolName: 'edit',
			args: { path: 'backend-go/internal/platform/airouter/embedding.go' },
		});
		const uJit = dynamicText();
		markMessages();
		check('route-hit-1: 未绑定阶段编辑命中零正文注入（ai-summary）', !uJit.includes('### ai-summary.md'));
		check('route-hit-1: 未绑定阶段编辑命中零动态层发送', uJit.trim() === '');
		const uSp1 = await systemPromptOnly();
		check(
			'route-hit-2: 未绑定阶段稳定层字节完全不变',
			uSp1 === uSp0 && Buffer.byteLength(uSp1, 'utf8') === uStableBytes,
		);

		// 21.2 已绑定阶段：编辑命中 → 下一 turn 路由清单差异送达（路径 + 节名，零正文）
		await emit('session_start', { reason: 'new' });
		await emit('input', { text: `smoke 绑定 ${FIXTURE_NAME}` });
		await systemPromptOnly();
		markMessages();
		await emit('tool_execution_start', {
			toolName: 'edit',
			args: { path: 'backend-go/internal/platform/airouter/embedding.go' },
		});
		const uRoute = await systemPrompt();
		check(
			'route-hit-3: 编辑命中文档进路由清单（路径 + 取数方式）',
			uRoute.includes('docs/reference/standard/backend/ai-logging.md') &&
				uRoute.includes('编辑路径命中，自主导入'),
		);
		check('route-hit-3: 编辑命中零正文注入（整节不进上下文）', !uRoute.includes('所有 AI 调用必须经 airouter'));

		// 21.3 粘性：同一路径重复编辑 → 路由清单不变 → 零重投
		markMessages();
		await emit('tool_execution_start', {
			toolName: 'edit',
			args: { path: 'backend-go/internal/platform/airouter/router.go' },
		});
		const uRoute2 = await systemPrompt();
		check('route-hit-4: 重复编辑零重投（命中集合只增不减 + 指纹去重）', !uRoute2.includes('📎 约束补充'));

		// 21.4 未映射路径 → 零动态层发送
		await emit('session_start', { reason: 'new' });
		await emit('input', { text: `smoke 绑定 ${FIXTURE_NAME}` });
		await systemPromptOnly();
		markMessages();
		await emit('tool_execution_start', {
			toolName: 'edit',
			args: { path: 'front/app/utils/useUnmappedHelper.ts' },
		});
		await systemPromptOnly();
		const uNone = dynamicText();
		markMessages();
		check('route-hit-5: 未映射路径零动态层发送', uNone.trim() === '');

		// 21.5 关键词通道保持关闭（未绑定阶段不按对话文本注域）
		await emit('session_start', { reason: 'new' });
		markMessages();
		await emit('input', { text: '日报与定时任务的展示要调整' });
		const uKw = await systemPrompt();
		check(
			'route-hit-6: 关键词通道保持关闭（不注 daily-report / scheduler 正文）',
			!uKw.includes('### daily-report.md') && !uKw.includes('### scheduler.md'),
		);

		// 21.6 巨域（daily-report）→ 零正文（未声明域不送红线层）
		await emit('session_start', { reason: 'new' });
		await emit('input', { text: `smoke 绑定 ${FIXTURE_NAME}` });
		await systemPromptOnly();
		markMessages();
		await emit('tool_execution_start', {
			toolName: 'edit',
			args: { path: 'backend-go/internal/topicgraph/service/daily_report_orchestrator.go' },
		});
		const uDeg = await systemPrompt();
		check(
			'route-hit-7: 巨域命中零正文（红线层只随声明域送达）',
			!uDeg.includes('### daily-report.md') && !uDeg.includes('read docs/reference/flow/daily-report.md」节'),
		);

		// 21.7 compact → 下一注入时机重发动态集合（红线层）
		await emit('session_start', { reason: 'new' });
		await emit('input', { text: `smoke 绑定 ${FIXTURE_NAME}` });
		await systemPromptOnly();
		markMessages();
		await emit('session_compact', { reason: 'manual' });
		const uCompact = await systemPrompt();
		check(
			'route-hit-8: compact 后重发动态集合（红线层快照）',
			uCompact.includes('约束快照（compact 后重发）') && uCompact.includes('同 board 同日重跑必须整份覆盖重建'),
		);

		// 21.8 reload → 命中集清零（后续编辑可重新命中）
		await emit('session_start', { reason: 'reload' });
		await emit('input', { text: `smoke 绑定 ${FIXTURE_NAME}` });
		await systemPromptOnly();
		markMessages();
		await emit('tool_execution_start', {
			toolName: 'edit',
			args: { path: 'backend-go/internal/platform/airouter/embedding.go' },
		});
		const uReload = await systemPrompt();
		check(
			'route-hit-9: reload 后命中集清零（重新编辑可再命中）',
			uReload.includes('docs/reference/standard/backend/ai-logging.md'),
		);

		// 21.9 子线程（父未绑定阶段）按自身路径自愈 → 零正文（无阶段 → 无路由清单）
		//      跑在 tmp 夹具 cwd（真实事件库落 tmp，不写真仓库事实库）
		const uParent = {
			cwd: tmp,
			hasUI: false,
			sessionManager: { getSessionId: () => 'ci-smoke-unbound-parent' },
		};
		await emit('session_start', { reason: 'new' }, uParent);
		markMessages();
		const uChild = {
			cwd: tmp,
			hasUI: false,
			sessionManager: {
				getSessionId: () => 'ci-smoke-unbound-child',
				getHeader: () => ({
					parentSession: '/x/2026-01-01T00-00-00-000Z_ci-smoke-unbound-parent.jsonl',
				}),
			},
		};
		await emit('session_start', { reason: 'startup' }, uChild);
		markMessages();
		await emit(
			'tool_execution_start',
			{ toolName: 'edit', args: { path: 'backend-go/internal/unboundmod/child.go' } },
			uChild,
		);
		const uChildJit = dynamicText();
		markMessages();
		check(
			'route-hit-10: 子线程（父未绑定）编辑命中零正文注入',
			!uChildJit.includes('### unbound-dom.md') && !uChildJit.includes('UNBOUND-REDLINE-1'),
		);

		// 21.10 记账归因（tier=none）+ 编辑路径命中不绑定 change
		//      夹具场景（tmp cwd，真实事件库落盘）：未绑定阶段 edit 命中夹具域标签。
		writeCfg({ ...bigCfg, channel: 'split' }, 12);
		const uCtx = {
			cwd: tmp,
			hasUI: false,
			sessionManager: { getSessionId: () => 'ci-smoke-unbound' },
		};
		await emit('session_start', { reason: 'new' }, uCtx);
		markMessages();
		await emit(
			'tool_execution_start',
			{ toolName: 'edit', args: { path: 'backend-go/internal/unboundmod/service.go' } },
			uCtx,
		);
		const uFixtureJit = dynamicText();
		markMessages();
		check(
			'route-hit-11: 未绑定阶段夹具域命中零正文注入',
			!uFixtureJit.includes('### unbound-dom.md'),
		);
		const uDb = new EvDb(path.join(tmp, '.pi', 'harness', 'events.db'), { readOnly: true });
		const uInjectRows = uDb
			.prepare("SELECT payload FROM events WHERE kind='constraint.inject' AND session_id='ci-smoke-unbound'")
			.all()
			.map((r) => JSON.parse(r.payload));
		check(
			'route-hit-11: 记账无 jit-path / keyword 原因行（整节正文通道退役）',
			!uInjectRows.some((p) => p.reason === 'jit-path' || p.reason === 'keyword'),
		);
		const uStageSetCount = uDb
			.prepare("SELECT COUNT(*) AS n FROM events WHERE kind='stage.set' AND session_id='ci-smoke-unbound'")
			.get().n;
		check('route-hit-12: 未绑定阶段编辑命中不绑定 change（无 stage.set 记账）', uStageSetCount === 0);
		uDb.close();

		// 21.11 legacy 通道不即时投递（下一 turn 落 system prompt，且仍无整节正文）
		writeCfg({ ...bigCfg, channel: 'legacy' }, 14);
		const uLegacyCtx = {
			cwd: tmp,
			hasUI: false,
			sessionManager: { getSessionId: () => 'ci-smoke-unbound-legacy' },
		};
		await emit('session_start', { reason: 'new' }, uLegacyCtx);
		markMessages();
		await emit(
			'tool_execution_start',
			{ toolName: 'edit', args: { path: 'backend-go/internal/unboundmod/other.go' } },
			uLegacyCtx,
		);
		const uLegacyJit = dynamicText();
		markMessages();
		check('route-hit-13: legacy 通道本 turn 零 steer', uLegacyJit.trim() === '');
		const uLegacySp = await systemPrompt(uLegacyCtx);
		check(
			'route-hit-13: legacy 下一 turn system prompt 无该文档正文',
			!uLegacySp.includes('### unbound-dom.md') && !uLegacySp.includes('UNBOUND-REDLINE-1'),
		);
		writeCfg(bigCfg, 16); // 还原默认配置（split 通道）
		// 21.12 读取记账（3.5 + review R-4）：read **成功回执**（tool_execution_end 且非
		//       error）才记 constraint.read；读普通源码不记（控事实库体量）；同一文档重复
		//       读只记一条（幂等）；发起但失败的读取不算证据。
		const crCtx = { cwd: tmp, hasUI: false, sessionManager: { getSessionId: () => 'ci-smoke-read' } };
		await emit('session_start', { reason: 'new' }, crCtx);
		const rd = (id, path) => [
			['tool_execution_start', { toolCallId: id, toolName: 'read', args: { path } }],
			['tool_execution_end', { toolCallId: id, toolName: 'read', isError: false }],
		];
		for (const [ev, args] of [
			...rd('rd-1', 'docs/reference/flow/redline-ok.md'),
			...rd('rd-2', 'docs/reference/standard/backend/testing.md'),
			...rd('rd-3', 'backend-go/internal/foo/bar.go'),
			...rd('rd-4', 'docs/reference/flow/redline-ok.md'),
		]) {
			await emit(ev, args, crCtx);
		}
		// 失败读取（isError）与发起后未回执的读取：均不得记账
		await emit('tool_execution_start', { toolCallId: 'rd-err', toolName: 'read', args: { path: 'docs/reference/flow/redline-ok.md' } }, crCtx);
		await emit('tool_execution_end', { toolCallId: 'rd-err', toolName: 'read', isError: true }, crCtx);
		await emit('tool_execution_start', { toolCallId: 'rd-hang', toolName: 'read', args: { path: 'docs/reference/standard/backend/lint.md' } }, crCtx);
		const crDb = new EvDb(path.join(tmp, '.pi', 'harness', 'events.db'), { readOnly: true });
		const crRows = crDb
			.prepare("SELECT payload FROM events WHERE kind='constraint.read' AND session_id='ci-smoke-read'")
			.all();
		crDb.close();
		const crPaths = crRows.map((r) => JSON.parse(r.payload).path);
		check(
			'constraint.read: 成功读约束文档才记账（flow + standard 各一条）',
			crPaths.includes('docs/reference/flow/redline-ok.md') &&
				crPaths.includes('docs/reference/standard/backend/testing.md'),
		);
		check('constraint.read: 读普通源码不记账', !crPaths.some((p) => p.includes('bar.go')));
		check(
			'constraint.read: 同一文档重复读只记一条（幂等）',
			crPaths.filter((p) => p === 'docs/reference/flow/redline-ok.md').length === 1,
		);
		check(
			'R-4: 失败读取（isError）不记账',
			crPaths.filter((p) => p === 'docs/reference/flow/redline-ok.md').length === 1,
		);
		check('R-4: 发起未回执（无 end）不算读', !crPaths.includes('docs/reference/standard/backend/lint.md'));

		// 22. stage 标识与阶段路由清单（stage-driven-constraint-injection 任务组 1：lib/stage.ts 纯函数）
		//     载体 = change 目录内 stage.md 头部 `<!-- stage: <值> -->` marker，三态 missing/invalid/ok。
		//     本组只测纯函数与清单装配（扩展管线接入属任务组 3）。
		const stage = require('../lib/stage.ts');
		check(
			'stage-parse-1: 恰好一条合法 marker → ok + stage',
			stage.parseStageMarker('<!-- stage: implement -->').kind === 'ok' &&
				stage.parseStageMarker('<!-- stage: implement -->').stage === 'implement',
		);
		check(
			'stage-parse-2: 无 marker → missing（legacy，不阻断）',
			stage.parseStageMarker('# 阶段\n\n正文没有 marker\n').kind === 'missing',
		);
		check(
			'stage-parse-3: 空串/null → missing',
			stage.parseStageMarker('').kind === 'missing' &&
				stage.parseStageMarker(null).kind === 'missing',
		);
		check(
			'stage-parse-4: marker 重复 → invalid（不取任一条生效）',
			stage.parseStageMarker('<!-- stage: implement -->\n<!-- stage: design -->').kind === 'invalid',
		);
		check(
			'stage-parse-5: 值不在取值域 → invalid',
			stage.parseStageMarker('<!-- stage: coding -->').kind === 'invalid',
		);
		check(
			'stage-parse-6: 大小写不匹配 → invalid（大小写敏感）',
			stage.parseStageMarker('<!-- stage: Implement -->').kind === 'invalid',
		);
		check(
			'stage-parse-7: 6 档取值域全覆盖解析为 ok',
			stage.STAGES.length === 6 &&
				stage.STAGES.every(
					(s) => stage.parseStageMarker(`<!-- stage: ${s} -->`).kind === 'ok',
				),
		);
		check(
			'stage-family-1: explore/propose/design → planning',
			stage.stageFamilyOf('explore') === 'planning' &&
				stage.stageFamilyOf('propose') === 'planning' &&
				stage.stageFamilyOf('design') === 'planning',
		);
		check(
			'stage-family-2: implement/verify/archive → implementation',
			stage.stageFamilyOf('implement') === 'implementation' &&
				stage.stageFamilyOf('verify') === 'implementation' &&
				stage.stageFamilyOf('archive') === 'implementation',
		);
		check(
			'stage-family-3: 非法 stage → null（不抛错）',
			stage.stageFamilyOf('coding') === null &&
				stage.stageFamilyOf(null) === null &&
				stage.stageFamilyOf(undefined) === null,
		);
		check(
			'stage-legacy: missing/invalid 判 legacy，ok 不判',
			stage.isLegacyStage({ kind: 'missing' }) === true &&
				stage.isLegacyStage({ kind: 'invalid' }) === true &&
				stage.isLegacyStage({ kind: 'ok', stage: 'design' }) === false,
		);
		const stageFixtureDir = path.join(
			tmp,
			OPENSPEC_CHANGES_DIR_REL,
			'smoke-fixture-stage',
		);
		fs.mkdirSync(stageFixtureDir, { recursive: true });
		fs.writeFileSync(
			path.join(stageFixtureDir, 'stage.md'),
			'<!-- stage: implement -->\n\n# 阶段\n',
		);
		const stageFileOk = stage.readStageFile(stageFixtureDir);
		check(
			'stage-file-1: stage.md 存在且 marker 合法 → ok + stage',
			stageFileOk.kind === 'ok' && stageFileOk.stage === 'implement',
		);
		fs.writeFileSync(path.join(stageFixtureDir, 'stage.md'), '');
		check(
			'stage-file-2: 空 stage.md → missing（legacy 降级）',
			stage.readStageFile(stageFixtureDir).kind === 'missing',
		);
		fs.rmSync(stageFixtureDir, { recursive: true, force: true });
		check(
			'stage-file-3: stage.md 不存在 → missing 且不抛错（legacy 不阻断）',
			stage.readStageFile(path.join(tmp, 'no-such-change')).kind === 'missing',
		);
		const codeDocPaths = [
			...stage.ROUTING_CODE_DOCS_BY_STACK.backend.map((d) => d.path),
			...stage.ROUTING_CODE_DOCS_BY_STACK.frontend.map((d) => d.path),
		];
		const exploreRouting = stage.buildStageRouting('explore', {
			domainDocs: ['daily-report'],
		});
		const designRouting = stage.buildStageRouting('design', {
			domainDocs: ['daily-report'],
		});
		const implementRouting = stage.buildStageRouting('implement', {
			domainDocs: ['daily-report'],
			stack: 'backend',
		});
		const verifyRouting = stage.buildStageRouting('verify', {
			domainDocs: ['daily-report'],
			stack: 'frontend',
		});
		check(
			'stage-routing-1: planning 族清单含索引 + 声明域 flow 文档',
			exploreRouting.some((i) => i.path === 'docs/reference/constraints-index.md') &&
				exploreRouting.some((i) => i.path === 'docs/reference/flow/daily-report.md'),
		);
		check(
			'stage-routing-2: planning 族清单不含代码规范文档（按栈条件文档不出现）',
			!exploreRouting.some((i) => codeDocPaths.includes(i.path)) &&
				!designRouting.some((i) => codeDocPaths.includes(i.path)),
		);
		check(
			'stage-routing-3: design 族加 UI 契约规范（layout + interaction）',
			designRouting.some((i) => i.path === 'docs/reference/standard/frontend/layout.md') &&
				designRouting.some((i) => i.path === 'docs/reference/standard/frontend/interaction.md'),
		);
		check(
			'stage-routing-4: implement 族含按栈命中的代码规范文档（backend 栈命中、frontend 栈不命中）',
			implementRouting.some((i) => i.path === 'docs/reference/standard/backend/testing.md') &&
				!implementRouting.some((i) => i.path === 'docs/reference/standard/frontend/code-style.md'),
		);
		check(
			'stage-routing-5: implement 族无栈命中 → 不含任何代码规范文档',
			!stage
				.buildStageRouting('implement', { domainDocs: [] })
				.some((i) => codeDocPaths.includes(i.path)),
		);
		check(
			'stage-routing-6: verify 族加验收与归档规范（test-design / commit-pr / doc-authoring）',
			verifyRouting.some((i) => i.path === 'docs/reference/standard/shared/test-design.md') &&
				verifyRouting.some((i) => i.path === 'docs/reference/standard/shared/commit-pr.md') &&
				verifyRouting.some((i) => i.path === 'docs/reference/standard/shared/doc-authoring.md'),
		);
		check(
			'stage-routing-7: 声明域为空 → 清单仍含索引（不崩）',
			stage.buildStageRouting('explore', { domainDocs: [] }).length >= 1 &&
				stage.buildStageRouting('explore', {}).length >= 1,
		);
		check(
			'stage-routing-8: 清单项只含路径 + 节名 + 取数方式，不内联正文（无换行、单项 ≤300 字节）',
			exploreRouting.every(
				(i) => Object.keys(i).sort().join(',') === 'how,path,section',
			) &&
				exploreRouting.every((i) => i.how.length <= 300 && !i.how.includes('\n')),
		);
		check(
			'stage-routing-9: 路由清单含取数命令（constraint-digest.sh --brief --domains <域>）',
			implementRouting.some((i) =>
				i.how.includes(
					'bash scripts/harness/constraint-digest.sh --brief --domains daily-report',
				),
			),
		);
		check(
			'read-path-1: 相对路径原样归一',
			stage.normalizeRepoPath('docs/reference/flow/daily-report.md') ===
				'docs/reference/flow/daily-report.md',
		);
		check(
			'read-path-2: 绝对路径剥仓库根前缀',
			stage.normalizeRepoPath(
				path.join(REPO_ROOT, 'docs/reference/flow/daily-report.md'),
				REPO_ROOT,
			) === 'docs/reference/flow/daily-report.md',
		);
		check(
			'read-path-3: Windows 反斜杠归一为正斜杠',
			stage.normalizeRepoPath('docs\\reference\\flow\\daily-report.md') ===
				'docs/reference/flow/daily-report.md',
		);
		check(
			'read-path-4: 路径含 .. → 解析后仍在仓库内',
			stage.normalizeRepoPath('docs/reference/flow/../flow/daily-report.md') ===
				'docs/reference/flow/daily-report.md',
		);
		check(
			'read-path-5: 路径含 .. 越出仓库根 → null',
			stage.normalizeRepoPath('../../outside.md') === null,
		);
		check(
			'read-path-6: 大小写差异保持原样（Linux 大小写敏感 → 不算同一文档）',
			stage.normalizeRepoPath('Docs/Reference/Flow/Daily-Report.md') ===
				'Docs/Reference/Flow/Daily-Report.md' &&
				stage.isConstraintDocPath(
					stage.normalizeRepoPath('Docs/Reference/Flow/Daily-Report.md'),
				) === false,
		);
		check(
			'read-path-7: 约束文档白名单（flow 顶层 .md + standard 任意层级 .md）',
			stage.isConstraintDocPath('docs/reference/flow/daily-report.md') === true &&
				stage.isConstraintDocPath('docs/reference/standard/backend/testing.md') === true &&
				stage.isConstraintDocPath('front/app.vue') === false &&
				stage.isConstraintDocPath('docs/reference/flow/diagrams/x.md') === false,
		);

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
		fs.rmSync(DOM_UNION_FIXTURE, { recursive: true, force: true });
		fs.rmSync(DOM_PROP_FIXTURE, { recursive: true, force: true });
		fs.rmSync(RESEARCH_FIXTURE, { recursive: true, force: true });
		if (poolExisted) fs.writeFileSync(RESEARCH_POOL, poolOrig);
		else fs.rmSync(RESEARCH_POOL, { force: true });
		fs.rmSync(tmp, { recursive: true, force: true });
	}
})().catch((e) => {
	console.error('FAIL', e);
	process.exitCode = 1;
});
