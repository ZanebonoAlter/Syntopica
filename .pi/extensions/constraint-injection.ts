/**
 * constraint-injection — 强制约束上下文注入 + 跨阶段知识中继（Syntopica 版）
 *
 * 源自用户另一项目实战 extension（快照：docs/research/extensions/extensions/constraint-injection.ts，
 * 移植 change：openspec/changes/port-constraint-injection，设计决策见其 design.md）。
 * 配置文件：.pi/constraint-injection.json（命令表/关键词表/JIT 路径信号/栈信号）。
 *
 * 职责：
 * 0. **会话作用域状态（per-session-constraint-binding）**：档位/绑定 change/turn 绑定锁/
 *    输入窗/JIT 与关键词命中集/pin 去重集/稳定层快照与指纹全部按 `sessionId` 隔离
 *    （`sessionStates`）——一个 pi 进程可同时有多会话（多窗口 + pi-subagents 子线程共用
 *    模块实例），隔离前它们共享同一套全局变量，导致「他会话刚绑定 = 本会话注入归属」
 *    （2026-09-17 事实库取证：会话 A 的注入在另一会话 mode.set 6 秒后漂移到 dedupe-
 *    rss-articles 并带上其声明域）。不变量：
 *    - 注入归属 / 稳定层快照 / 命中集只由**本会话**状态决定；
 *    - 本会话无状态时，仅当父子关系可证（session header 的 `parentSession`，即 fork /
 *      pi-subagents 子线程）才显式继承父会话（记 `mode.set source=inherit`），
 *     否则视为未绑定（仅索引）；**MUST NOT 借用任意他会话的状态**；
 *    - 会话边界事件（new/resume/fork/reload）只重置**本会话**条目；
 *    - 会话条目有界（上限 + LRU 淘汰），无 `sessionId` 的 stub 语境落单一兜底槽。
 *    回归面：`.pi/extensions/tests/constraint-injection.smoke.cjs` 第 19 组。
 * 1. input 事件：识别阶段命令，设置会话内 mode flag（需求/设计 vs 实现/评审）
 * 2. before_agent_start 事件：约束注入按**混合通道**送达（harden-constraint-injection-channel）：
 *    - **稳定层**（system prompt，本 handler 返回 systemPrompt）：索引 + mode-base + 声明域
 *      红线层。快照 key=mode|boundChange，档位生命周期内**字节恒定**（不再随输入/编辑路径
 *      /findings 变化）→ system prompt 不变则其后全部 history 前缀缓存不失效。
 *    - **动态层**（追加消息，append-only 投递）：关键词命中全节 + JIT 命中全节 + change 级
 *      文件 + 稳定层差异通知。按内容指纹 diff 发送——指纹未变零投递，变化只发增量条目。
 *      turn 起点走 before_agent_start 返回的 message；turn 中途（JIT 命中）走
 *      pi.sendMessage(deliverAs:"steer", triggerTurn:false) 即时送达。
 *    - compaction 后（session_compact）下一注入时机重发一次约束快照（消息会被摘要掉）。
 *    - 配置 channel="legacy" 可回退旧的「每 turn 全量进 system prompt」行为。
 *    未激活档（无 mode）：仅注入约束索引（docs/reference/constraints-index.md），
 *    不做 change 文本关键词命中——避免「最近活跃 change」的静态文本让无关规范
 *    全文常驻所有会话
 *    - 档位与设置时绑定的活跃 change 关联：change 归档/切换后自动回落未激活档
 *    - 业务域显式声明（constraint-domain-declaration / constraint-declaration-redline）：
 *      proposal.md 头部 `<!-- constraint-domains: daily-report, ... -->` 声明涉及域
 *      → 注入对应 flow 文档「业务约束与不变量」节的红线层（顶层列表项首个加粗块
 *      逐行 + 细节层取回指引尾行；提取 0 条或低于 minSectionBytes 回退全节，
 *      fail-safe；格式规范见 standard/shared/doc-authoring.md「约束节红线句格式」）
 *      （声明为主，每回合重解析不进粘性集合；keyword/jit 命中仍注全节——细节层
 *      经命中通道到达模型）；
 *      change 产物全文不再参与关键词命中——harness/AI 类 change 的正当词汇与业务域
 *      关键词本质重叠，隐式推断必致误注入（harness-facts-tier-a 实测撞车 4 域）
 *    - 节级注入：keywordDocs/jitDocs 带 section 字段时只注「## <节名>」节
 *      （flow 文档只注「业务约束与不变量」节，实测全文 8~20K vs 节 1.7~4.6K）；
 *      配置的节不存在时回落全文（fail-safe，不静默跳过）
 *    - 命中粘性：关键词命中与 JIT 命中会话内只增不减（命中源含最近 N 条输入滚动窗，
 *      若词条滚出窗口就移除会让内容集合抖动）；粘性后投递侧按内容指纹 diff——相同内容
 *      零重复投递
 *    - 关键词命中域限定（harden-constraint-injection-channel D7）：命中文档仅在
 *      「当前 change 声明域 ∪ 栈条件文档 ∪ baseDocs/索引」范围内生效，跨域词
 *      （如做 scheduler change 时聊到 discovery）不再误拉无关域全节
 *    - 二级注入：文档超 digestDocThreshold 且文首有「## 硬规则速览」→ 仅注速览
 *      + 全文目录；无速览小节回落全文（本仓文档暂无速览节，路径备用）
 * 3. pin_finding 工具：探索阶段持久化关键代码发现到活跃 change 的 explore-findings.md；
 *    research/问答语境（无激活档位）落 docs/research/<topic>/（无 topic 落通用池
 *    单文件 docs/research/explore-findings.md），不碰 openspec/changes/ 与 experience
 * 4. before_agent_start（实现档）：自动把 explore-findings.md 注入 system prompt
 * 5. 事实库记账（harness-facts-tier-a）：constraint.inject（每回合每文档）/ pin.write /
 *    pin.read（会话内去重）经 lib/harness-log 自报，模型零参与
 * 6. 注入预算与分层降级（constraint-injection-tier-b）：注入内容总量受 budgetBytes
 *    限制（缺省 32768；非正数禁用），超线按命中信号强度分层降级（keyword → jit →
 *    change-file digest 收紧 → 域声明 → change-file 占位），永不真丢（占位行保留
 *    标题 + read 路径）；降级确定性（同输入同输出字节）
 * 7. 档位持久化与绑定归因（constraint-injection-tier-b + D5/D6）：档位/绑定**所有**
 *    变化路径记 mode.set（payload 附 source：command/skill/edit-dir/recover/inherit/
 *    fallback，隐性绑定不存在）；绑定修正条件化（read 不抢绑、当前绑定健康时不抢绑、
 *    仅无绑定/绑定 change 消失时兜底）且同 turn 锁定；session_start{reason:resume}
 *    按同 sessionId 恢复，new/fork 清零
 * 8. 记账送达化（D6）：constraint.inject 在内容实际送达时记（稳定层快照变化时 /
 *    动态层消息发出时 / compact 快照重发时），同 session 同 path 内容未变不重复记
 *
 * 与 AGENTS.md 的关系：注入块头部固定标注「与 AGENTS.md 优先级宪法冲突时以宪法为准」
 * ——extension 注入的内容属"项目文档"层，不能悄悄升权。
 *
 * 可靠性：extension 在 harness 层注入，模型无法绕过。子线程靠 plan 内嵌约束兜底（§0.6）。
 */
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { Type } from "typebox";
import {
	readFileSync,
	existsSync,
	writeFileSync,
	statSync,
	mkdirSync,
	readdirSync,
} from "node:fs";
import { join, dirname } from "node:path";
import { logEvent, queryBySession, type HarnessEventRow } from "./lib/harness-log";
import {
	listChangeDirs,
	detectActiveChange,
	OPENSPEC_CHANGES_DIR,
	ARCHIVE_DIR,
} from "./lib/active-change";

type Mode = "requirements" | "implementation";

interface ConditionalDoc {
	doc: string;
	ifStack: "backend" | "frontend";
}
interface ModeConfig {
	label: string;
	commands: string[];
	baseDocs: string[];
	conditionalDocs?: ConditionalDoc[];
}
/** 文档条目：doc 路径 + 可选节名（带 section 时只注「## <节名>」节 + 全文指引） */
interface DocEntry {
	doc: string;
	section?: string;
}
interface ConstraintConfig {
	modes: Record<Mode, ModeConfig>;
	keywordDocs: (DocEntry & { keywords: string[] })[];
	stackSignals: { frontend: string[]; backend: string[] };
	// skill 文件路径片段 → 档位（agent 自动 read skill 时激活，覆盖斜杠命令外的真实工作流）
	skillSignals?: { requirements: string[]; implementation: string[] };
	indexDoc: string;
	findingsFile: string;
	findingsDigestThreshold: number;
	// change 级词汇表文件名（默认 ubiquitous-language.md）；实现档注入，主线程定义→子线程可见
	vocabularyFile?: string;
	// 最近 N 条用户输入参与关键词/栈命中（默认 5）
	recentInputWindow?: number;
	// 会话状态条目上限（默认 32）：仅用于长跑进程内存有界（LRU 淘汰最久未用会话条目），
	// 不影响注入语义（会话数未超限时行为与上限无关）；非法值（非数字/0/负数）回退默认。
	sessionStateLimit?: number;
	// 二级注入阈值（字节，默认 6144）：超过且有「## 硬规则速览」小节 → 仅注速览+全文目录
	digestDocThreshold?: number;
	// 节级注入最小字节下限（默认 512）：提取的节内容低于此值视为文档编辑中间态残缺节，
	// 回退注入全文（fail-safe，与节不存在回落全文同族）。harness-observability-fixes D4。
	minSectionBytes?: number;
	// （已移除 jitDocs）JIT pathSignals 单一真相源 = 文档头部 `doc-impact-applies`
	// frontmatter 标签（docs/reference/flow + standard），见 scanAppliesTags（design D3）
	// 注入块总预算（字节，tier-b design D1）：缺省 32768；非正数 = 显式禁用预算。
	// 32K = 常态上界（findings+词汇表双 6K + 2~3 域节约 10K）+ 余量，只截 43~55K 病态尾部
	budgetBytes?: number;
	// 注入通道（harden-constraint-injection-channel D8）：split（缺省，稳定层 system prompt
	// + 动态层追加消息）/ legacy（旧行为：全部每 turn 进 system prompt，应急回退）
	channel?: "split" | "legacy";
	// 动态层消息 TUI 展示（D8）：compact（缺省，精简展示）/ full（完整渲染正文）
	dynamicDisplay?: "compact" | "full";
}

const CONFIG_PATH = ".pi/constraint-injection.json";
// 预算缺省值（tier-b design D1）：实测重估——16K 会在常态上界触发 keyword 降级，
// 且让烟测 REPO 场景落入边际降级区（断言随 flow 文档演进漂移）；预算管尾部不管常态
const DEFAULT_BUDGET_BYTES = 32768;
// research 库（无档位语境的 pin 落点；规则出处：openspec/specs/research-retention）
const RESEARCH_DIR = "docs/research";

/* ---------- 会话作用域状态（per-session-constraint-binding） ---------- */
// 为什么按会话隔离：pi 一个进程里可同时存在多个会话（多窗口 + pi-subagents 子线程共用同一
// 模块实例）。这些状态曾是模块级全局，于是「谁最后绑定」决定所有人注入谁的约束——2026-09-17
// 事实库取证：会话 01a0aaf4 无自身 mode.set，注入归属却在另一会话绑定 6 秒后漂移到
// dedupe-rss-articles 并带上其声明的域。不变式（spec「会话作用域状态隔离」）：
//   ① 注入归属 / 稳定层快照 / 命中集只由本会话状态决定，他会话绑定变化不得影响本会话；
//   ② 会话无自身状态时，只有能证明父子关系（session header 的 parentSession）才继承父会话，
//      否则视为未绑定；MUST NOT 借用任意他会话的状态。
// 无 sessionId 的非真实会话语境（烟测 stub）落单一兜底槽，行为与隔离前等价。
type ChannelState = {
	/** 稳定层快照（D2）：key=`mode|boundChange`，档位生命周期内冻结字节；内容差异走动态层 */
	stableSnapshot: { key: string; block: string; items: PlanItem[] } | null;
	/** 动态层已投递指纹（D3）：itemKey → contentHash；指纹未变则零投递 */
	sentFingerprint: Map<string, string>;
	/** compaction 后待重发快照（D4）：session_compact 置位，下一注入时机重发并重置指纹 */
	pendingSnapshot: boolean;
};
type SessionState = {
	sessionId: string;
	/** 本会话档位（requirements / implementation），未激活为 null */
	mode: Mode | null;
	/** 设置档位时绑定的活跃 change 名；绑定 change 归档/删除后回落 null（防粘性常驻） */
	boundChange: string | null;
	/** turn 绑定锁定（D5）：before_agent_start 重置；turn 内首个生效绑定后置位，同 turn 不再切换 */
	turnBindLocked: boolean;
	/** 最近 N 条用户输入（ring buffer），与 change 文本合并后做关键词/栈命中 */
	recentInputs: string[];
	/** JIT 命中集合（会话内只增不减）：write/edit 路径命中 doc-impact-applies 标签即入列 */
	jitDocHits: Map<string, DocEntry>;
	/** 关键词命中集合（会话内只增不减，@Syntopica）：命中源含滚动输入窗，词条滚出窗口
	 *  不移除——注入块缩水会让 system prompt 字节变化 → 全部历史前缀缓存报废 */
	keywordDocHits: Map<string, DocEntry>;
	/** pin.read 会话内去重（design D5）：实现档每回合重建注入块，不去重会重复计数 */
	pinReadSeen: Set<string>;
	channel: ChannelState;
	/** LRU 淘汰时间戳（多会话长跑进程不得无界增长） */
	lastUsedAt: number;
};
/** 无 sessionId 的语境（烟测 stub / 非真实会话）共用的兜底槽 */
const FALLBACK_SESSION_KEY = "__no-session__";
/** 会话条目上限默认值（超限淘汰最久未用者）；cfg.sessionStateLimit 可覆盖 */
const DEFAULT_SESSION_STATE_LIMIT = 32;

/** 解析会话条目上限：cfg 合法正整数覆盖值否则默认（缺 cfg / 非数字 / 0 / 负数 / NaN 均回退默认）。
 *  上限只影响内存有界，MUST NOT 影响注入语义——调小只会在会话数超限时提前淘汰旧条目。 */
function resolveSessionStateLimit(cfg?: ConstraintConfig | null): number {
	const v = cfg?.sessionStateLimit;
	return typeof v === "number" && Number.isFinite(v) && v >= 1
		? Math.floor(v)
		: DEFAULT_SESSION_STATE_LIMIT;
}
const sessionStates = new Map<string, SessionState>();

function newSessionState(sessionId: string): SessionState {
	return {
		sessionId,
		mode: null,
		boundChange: null,
		turnBindLocked: false,
		recentInputs: [],
		jitDocHits: new Map(),
		keywordDocHits: new Map(),
		pinReadSeen: new Set(),
		channel: { stableSnapshot: null, sentFingerprint: new Map(), pendingSnapshot: false },
		lastUsedAt: Date.now(),
	};
}

/** 会话 key：无 sessionId（烟测 stub / 非真实会话）落兜底槽 */
function sessionKey(ctx: ExtCtx | undefined): string {
	const sid = ctx?.sessionManager?.getSessionId?.();
	return sid ? sid : FALLBACK_SESSION_KEY;
}

/** 有界回收：超上限淘汰最久未用条目（淘汰不做额外副作用）。limit 由 resolveSessionStateLimit 得出 */
function evictSessionStates(limit: number): void {
	while (sessionStates.size > limit) {
		let oldestKey: string | null = null;
		let oldestAt = Number.POSITIVE_INFINITY;
		for (const [key, st] of sessionStates) {
			if (st.lastUsedAt < oldestAt) {
				oldestAt = st.lastUsedAt;
				oldestKey = key;
			}
		}
		if (oldestKey === null) return;
		sessionStates.delete(oldestKey);
	}
}

/** 会话状态唯一读写入口：缺失即建 + 刷新 LRU 时间戳。cfg 仅用于解析条目上限（可省/可为 null） */
function stateFor(ctx: ExtCtx | undefined, cfg?: ConstraintConfig | null): SessionState {
	const key = sessionKey(ctx);
	let st = sessionStates.get(key);
	if (!st) {
		st = newSessionState(key);
		sessionStates.set(key, st);
		evictSessionStates(resolveSessionStateLimit(cfg));
	}
	st.lastUsedAt = Date.now();
	return st;
}

/** 真实 sessionId（兜底槽 → undefined）：记账用，避免把 stub 会话写进事实库 */
function realSessionId(state: SessionState): string | undefined {
	return state.sessionId === FALLBACK_SESSION_KEY ? undefined : state.sessionId;
}

/** 烟测观测面（session 作用域状态有界性）：当前存活会话 key 列表，按 LRU 时间戳排序
 *  （旧→新）。仅供测试断言，不参与注入逻辑。 */
export function sessionStateKeysForTest(): string[] {
	return [...sessionStates.values()]
		.sort((a, b) => a.lastUsedAt - b.lastUsedAt)
		.map((s) => s.sessionId);
}

/** 烟测观测面：会话条目上限默认值（cfg 覆盖生效值见 resolveSessionStateLimit） */
export const SESSION_STATE_LIMIT_FOR_TEST = DEFAULT_SESSION_STATE_LIMIT;
/** 烟测观测面：上限解析（非法值回退默认） */
export const resolveSessionStateLimitForTest = resolveSessionStateLimit;

/** `<ts>_<sessionId>.jsonl` → sessionId。会话文件（父会话文件路径、fork 子会话文件）都用
 *  这个形状：时间戳与 id 之间只有一个下划线，id 自身含连字符。 */
function sessionIdFromSessionFile(file: string | undefined | null): string | null {
	if (!file) return null;
	const stem = String(file).split(/[\\/]/).pop()?.replace(/\.jsonl$/, "") ?? "";
	const sep = stem.indexOf("_");
	return sep >= 0 ? stem.slice(sep + 1) : null;
}

/** fork 子会话文件路径 → 父会话 id：子会话文件位于 `<父会话目录>/forks/<ts>_<子会话id>.jsonl`，
 *  而父会话目录名就是 `<ts>_<父会话id>` → 从「forks 的上一级目录名」解出父 id。
 *  非 fork 会话（文件不在 forks/ 下）→ null（不可证，不继承）。 */
function parentSessionIdFromForkFile(file: string | undefined | null): string | null {
	if (!file) return null;
	const segs = String(file).split(/[\\/]/);
	const forksIdx = segs.lastIndexOf("forks");
	if (forksIdx < 1) return null;
	const parentDir = segs[forksIdx - 1] ?? "";
	const sep = parentDir.indexOf("_");
	return sep >= 0 ? parentDir.slice(sep + 1) : null;
}

/** 父会话 id（「父子关系可证」的唯一依据）。两条来源，按可信度排序：
 *  ① session header 的 `parentSession`（父会话**文件绝对路径**，fork 子会话必带，实测 2026-09-17）；
 *  ② `getSessionFile()` 的路径兜底——header 未落盘/契约变化时，fork 子会话仍可从
 *     `<父会话目录>/forks/` 反解父 id。
 *  两条都拿不到 → null（不可证 → 不继承，保持失败方向安全：未激活只注索引）。 */
function parentSessionId(ctx: ExtCtx | undefined): string | null {
	const fromHeader = sessionIdFromSessionFile(
		ctx?.sessionManager?.getHeader?.()?.parentSession,
	);
	if (fromHeader) return fromHeader;
	return parentSessionIdFromForkFile(ctx?.sessionManager?.getSessionFile?.());
}

/** fork / 子线程继承（**同进程**内存路径）：**父会话有状态才继承**（否则不借他会话）。
 *  含快照与指纹——子会话的对话是父会话的拷贝，父会话已投递过的动态层条目不该重发。
 *  ⚠ 真实链路极少命中：pi-subagents 子线程跑在**独立 node 进程**（2026-09-17 实测
 *  `pi-subagents/src/runs/background/subagent-runner.ts`），父子不共享模块实例 → 子进程的
 *  sessionStates 是新空 Map。跨进程场景由 inheritFromParentHistory 兜底。 */
function inheritFromParent(state: SessionState, ctx: ExtCtx | undefined): boolean {
	const pid = parentSessionId(ctx);
	if (!pid) return false;
	const parent = sessionStates.get(pid);
	if (!parent) return false;
	state.mode = parent.mode;
	state.boundChange = parent.boundChange;
	state.turnBindLocked = parent.turnBindLocked;
	state.recentInputs = [...parent.recentInputs];
	state.jitDocHits = new Map(parent.jitDocHits);
	state.keywordDocHits = new Map(parent.keywordDocHits);
	state.pinReadSeen = new Set(parent.pinReadSeen);
	state.channel = {
		stableSnapshot: parent.channel.stableSnapshot
			? { ...parent.channel.stableSnapshot, items: [...parent.channel.stableSnapshot.items] }
			: null,
		sentFingerprint: new Map(parent.channel.sentFingerprint),
		pendingSnapshot: parent.channel.pendingSnapshot,
	};
	return true;
}

/** 跨进程继承回退（**真实链路主路径**）：父会话的内存态在另一个进程里拿不到，但父会话的
 *  `mode.set` 历史在共享事实库里 → 按父会话 id 取父会话最近一条**可恢复**记录，只采纳
 *  `mode` + `boundChange`（命中集/快照/输入窗是父进程内存态，拿不到就不编造，宁可少注入）。
 *  复用 recoverMode：同 id 单段 + 绑定 change 目录存在性校验（父会话绑定已归档 → 未激活）。
 *  父 id 不可证、无记录、异常 → false（保持未激活，**绝不**借用任意他会话状态）。 */
function inheritFromParentHistory(state: SessionState, ctx: ExtCtx | undefined): boolean {
	const pid = parentSessionId(ctx);
	if (!pid || !ctx?.cwd) return false;
	let rec: { mode: Mode; boundChange: string | null } | null = null;
	try {
		rec = recoverMode(ctx.cwd, pid);
	} catch (e) {
		console.error(
			`[constraint-injection] inherit from parent history failed: ${(e as Error).message}`,
		);
		return false;
	}
	if (!rec) return false;
	state.mode = rec.mode;
	state.boundChange = rec.boundChange;
	return true;
}

/** 会话边界重置（session_start 的 new/resume/fork/reload）：**只清本会话条目**，
 *  他会话状态 MUST NOT 被清零（隔离前是清零全局，等于替所有会话重置）。 */
function resetSessionState(state: SessionState): void {
	state.mode = null;
	state.boundChange = null;
	state.turnBindLocked = false;
	state.recentInputs = [];
	state.jitDocHits = new Map();
	state.keywordDocHits = new Map();
	state.pinReadSeen = new Set();
	state.channel = { stableSnapshot: null, sentFingerprint: new Map(), pendingSnapshot: false };
}

let configCache: { config: ConstraintConfig; mtime: number } | null = null;
// doc-impact-applies 标签扫描缓存（design D3）：文档相对路径 → mtime + 解析结果（无标签为 null）
const appliesTagCache = new Map<
	string,
	{ mtime: number; entry: { signals: string[]; section: string | null } | null }
>();
// 合法业务域缓存（docs/reference/flow/*.md basename）：目录 mtime + 名单
let flowDomainsCache: { mtime: number; names: string[] } | null = null;
// 关键词匹配器预编译缓存（ASCII 词边界正则；按 config 实例 WeakMap，热更新自然失效）
const matcherCache = new WeakMap<ConstraintConfig, Map<string, RegExp | null>>();
/* 混合通道状态（harden-constraint-injection-channel D2~D5）与 pin.read 去重集：
 * per-session-constraint-binding 起收进 SessionState.channel / SessionState.pinReadSeen，
 * 不再有模块级全局（旧 resetChannelState 由 resetSessionState 取代）。 */

/* ---------- 配置加载（按 mtime 热更新） ---------- */

function loadConfig(cwd: string): ConstraintConfig | null {
	const path = join(cwd, CONFIG_PATH);
	if (!existsSync(path)) return null;
	const mtime = statSync(path).mtimeMs;
	if (configCache && configCache.mtime === mtime) return configCache.config;
	try {
		const raw = readFileSync(path, "utf8");
		const config = JSON.parse(raw) as ConstraintConfig;
		configCache = { config, mtime };
		return config;
	} catch {
		return null;
	}
}

/* ---------- change 目录检测：已提取至 lib/active-change.ts（quality-gate 共享，design D1） ---------- */

/* 文本里提到的 change（目录名子串匹配；目录名含连字符足够独特，误命中概率低） */
function detectMentionedChange(
	cwd: string,
	text: string,
): { name: string; dir: string } | null {
	if (!text) return null;
	for (const c of listChangeDirs(cwd)) {
		if (text.includes(c.name)) return c;
	}
	return null;
}

/* ---------- change 产物文本（用于栈判定 + 关键词命中） ---------- */

let changeTextCache: {
	changeName: string;
	text: string;
	mtime: number;
} | null = null;

function readChangeText(change: { name: string; dir: string }): string {
	const candidates = ["proposal.md", "tasks.md", "design.md"];
	// 以候选文件最新 mtime 作缓存失效信号：change 文档更新后关键词命中需重算
	let mtime = 0;
	for (const f of candidates) {
		const p = join(change.dir, f);
		try {
			if (existsSync(p)) mtime = Math.max(mtime, statSync(p).mtimeMs);
		} catch {
			/* ignore */
		}
	}
	if (
		changeTextCache &&
		changeTextCache.changeName === change.name &&
		changeTextCache.mtime === mtime
	) {
		return changeTextCache.text;
	}
	const parts: string[] = [];
	for (const f of candidates) {
		const p = join(change.dir, f);
		if (existsSync(p)) {
			try {
				parts.push(readFileSync(p, "utf8"));
			} catch {
				/* ignore */
			}
		}
	}
	const text = parts.join("\n\n");
	changeTextCache = { changeName: change.name, text, mtime };
	return text;
}

/* ---------- 业务域声明解析（constraint-domain-declaration design D1/D2） ---------- */

const DECLARATION_RE = /<!--[\s]*constraint-domains:([\s\S]*?)-->/g;

/**
 * 解析 proposal.md 里的 `<!-- constraint-domains: daily-report, topic-graph -->` 标记；
 * 多个标记块/跨行内容合并，按首次出现顺序去重返回。不做合法性校验（调用方对照
 * flow 文档 basename 白名单，未知域名宽容忽略）。
 */
export function parseDomainDeclaration(text: string): string[] {
	const out: string[] = [];
	const seen = new Set<string>();
	for (const m of text.matchAll(DECLARATION_RE)) {
		for (const raw of m[1].split(/[,\n]/)) {
			const name = raw.trim();
			if (name && !seen.has(name)) {
				seen.add(name);
				out.push(name);
			}
		}
	}
	return out;
}

/** 合法业务域名单 = docs/reference/flow/*.md 的 basename（README 除外），目录 mtime 缓存 */
function listFlowDomains(cwd: string): string[] {
	const dir = join(cwd, "docs/reference/flow");
	try {
		const mtime = statSync(dir).mtimeMs;
		if (flowDomainsCache && flowDomainsCache.mtime === mtime) {
			return flowDomainsCache.names;
		}
		const names = readdirSync(dir)
			.filter((f) => f.endsWith(".md") && f !== "README.md")
			.map((f) => f.slice(0, -3));
		flowDomainsCache = { mtime, names };
		return names;
	} catch {
		return [];
	}
}

/* ---------- 栈判定 / 关键词命中 ---------- */

function detectStacks(
	text: string,
	config: ConstraintConfig,
): { backend: boolean; frontend: boolean } {
	const has = (arr: string[]) => arr.some((k) => text.includes(k));
	return {
		backend: has(config.stackSignals.backend),
		frontend: has(config.stackSignals.frontend),
	};
}

/* 关键词匹配器预编译：纯 ASCII 关键词 → `\b<kw>\b` 词边界正则（修 "stage" 含 "tag"
   子串类误伤）；CJK / 混合关键词 → null（调用方用 includes，\b 不识别 CJK 边界） */
function keywordMatchers(
	config: ConstraintConfig,
): Map<string, RegExp | null> {
	let m = matcherCache.get(config);
	if (m) return m;
	m = new Map();
	for (const entry of config.keywordDocs) {
		for (const k of entry.keywords) {
			if (m.has(k)) continue;
			m.set(
				k,
				/^[\x21-\x7e]+$/.test(k)
					? new RegExp(`\\b${k.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}\\b`)
					: null,
			);
		}
	}
	matcherCache.set(config, m);
	return m;
}

/* 关键词命中 → 写入本会话粘性集合（Map 保持首次命中顺序，只增不减）。
   命中源仅限对话输入（见 planInjection），ASCII 词整词匹配。 */
function matchKeywordDocs(
	text: string,
	config: ConstraintConfig,
	state: SessionState,
): void {
	if (!text) return;
	const matchers = keywordMatchers(config);
	for (const entry of config.keywordDocs) {
		if (state.keywordDocHits.has(entry.doc)) continue;
		if (
			entry.keywords.some((k) => {
				const re = matchers.get(k);
				return re ? re.test(text) : text.includes(k);
			})
		) {
			state.keywordDocHits.set(entry.doc, { doc: entry.doc, section: entry.section });
		}
	}
}

/* ---------- doc-impact-applies 标签扫描（JIT 单一真相源，design D3） ---------- */

/* 兼容两种形态：裸行 `doc-impact-applies: path, ...`（ai-logging 块注释内）与
   单行 `<!-- doc-impact-applies: ... -->`；可选 `| section=节名` 指定节级注入目标 */
const APPLIES_TAG_RE = /^\s*(?:<!--[\s]*)?doc-impact-applies:\s*(.+?)(?:\s*-->)?\s*$/;

function parseAppliesLine(raw: string): {
	signals: string[];
	section: string | null;
} | null {
	const parts = raw.split("|");
	const signals = parts[0]
		.split(",")
		.map((s) => s.trim())
		.filter(Boolean);
	if (!signals.length) return null;
	let section: string | null = null;
	for (const p of parts.slice(1)) {
		const m = p.trim().match(/^section=(.+)$/);
		if (m) section = m[1].trim();
	}
	return { signals, section };
}

function* walkDocs(
	cwd: string,
	relDir: string,
	depth: number,
): Generator<string> {
	let entries;
	try {
		entries = readdirSync(join(cwd, relDir), { withFileTypes: true });
	} catch {
		return;
	}
	for (const e of entries) {
		const rel = `${relDir}/${e.name}`;
		if (e.isDirectory() && depth > 0) yield* walkDocs(cwd, rel, depth - 1);
		else if (e.isFile() && e.name.endsWith(".md") && e.name !== "README.md")
			yield rel;
	}
}

/**
 * 扫描 flow + standard 文档头部 15 行的 `doc-impact-applies` 标签，构建
 * 文档 → { pathSignals, section } 映射（JIT 命中的单一真相源，json 不再手写）。
 * 按文件 mtime 缓存，仅重读变更文件。
 */
export function scanAppliesTags(
	cwd: string,
): Map<string, { signals: string[]; section: string | null }> {
	const out = new Map<
		string,
		{ signals: string[]; section: string | null }
	>();
	const files = [
		...walkDocs(cwd, "docs/reference/flow", 1),
		...walkDocs(cwd, "docs/reference/standard", 2),
	];
	for (const rel of files) {
		const abs = join(cwd, rel);
		let mtime: number;
		try {
			mtime = statSync(abs).mtimeMs;
		} catch {
			continue;
		}
		const cached = appliesTagCache.get(rel);
		if (cached && cached.mtime === mtime) {
			if (cached.entry) out.set(rel, cached.entry);
			continue;
		}
		let entry: { signals: string[]; section: string | null } | null = null;
		try {
			const head = readFileSync(abs, "utf8").split(/\r?\n/).slice(0, 15);
			for (const line of head) {
				const m = line.match(APPLIES_TAG_RE);
				if (!m) continue;
				entry = parseAppliesLine(m[1]);
				break;
			}
		} catch {
			/* ignore */
		}
		appliesTagCache.set(rel, { mtime, entry });
		if (entry) out.set(rel, entry);
	}
	return out;
}

/* ---------- 文档读取 ---------- */

function readDoc(cwd: string, rel: string): string | null {
	const p = join(cwd, rel);
	if (!existsSync(p)) return null;
	try {
		return readFileSync(p, "utf8");
	} catch {
		return null;
	}
}

function basename(rel: string): string {
	const parts = rel.split(/[\\/]/);
	return parts[parts.length - 1];
}

/* ---------- 节提取（@Syntopica：flow 只注「业务约束与不变量」节） ---------- */

/**
 * 从文档全文提取「## <节名>」节内容（到下一个 ## 标题为止）。
 * 节不存在返回 null（调用方回落全文注入——fail-safe，不静默跳过）。
 */
function extractSection(content: string, sectionName: string): string | null {
	const heading = `## ${sectionName}`;
	const lines = content.split(/\r?\n/);
	const start = lines.findIndex((l) => l.trimEnd() === heading);
	if (start < 0) return null;
	const out: string[] = [];
	for (let i = start + 1; i < lines.length; i++) {
		if (/^##\s+/.test(lines[i])) break;
		out.push(lines[i]);
	}
	const text = out.join("\n").trim();
	return text || null;
}

/* ---------- 红线层提取（@constraint-declaration-redline design D1/D2/D4） ---------- */

/** 红线层提取结果：lines 为「原文列表标记 + 加粗块」逐行序列，bytes 为 \n 拼接字节数 */
export interface RedlineLayer {
	lines: string[]; // 形如 `3. **红线句**`（保留原文顺序与编号）
	bytes: number;
}

/**
 * 提取约束节的红线层：顶层列表项（行首无缩进的 `N. ` / `- `）行取首个 `**...**`
 * 加粗块为一条红线句（保留原文顺序与编号）；无加粗块的列表项不取首行文本凑数
 * （细节句冒充红线比回退全节更糟）；引用块 / 自由段落 / 嵌套列表项不属红线层。
 * 0 条提取返回 null；bytes 低于节级最小字节下限时由调用方回退全节（同族 fail-safe）。
 * 纯函数：同一节文本 → 同一输出字节序列（smoke 直跑）。
 */
export function extractRedlines(sectionText: string): RedlineLayer | null {
	const lines: string[] = [];
	for (const line of sectionText.split(/\r?\n/)) {
		const m = line.match(/^(\d+\.|-)\s+(.*)$/);
		if (!m) continue; // 非顶层列表项（嵌套项/引用块/自由段落/标题）
		const bold = m[2].match(/\*\*(.+?)\*\*/);
		if (!bold) continue; // 无加粗块：不凑数，该条不进红线层
		lines.push(`${m[1]} **${bold[1]}**`);
	}
	if (!lines.length) return null;
	return { lines, bytes: Buffer.byteLength(lines.join("\n"), "utf8") };
}

/* ---------- 大文档二级注入（硬规则速览 + 全文目录；本仓文档暂无速览节，路径备用） ---------- */

const CORE_SECTION_HEADING = "## 硬规则速览";

/**
 * 文档超阈值且文首有「## 硬规则速览」→ 返回「速览 + 全文目录」注入文本；否则返回 null（回落全文）。
 * 速览小节 = 该标题到下一个 ## 标题之间的内容；目录 = 全文所有 ## 标题（除速览自身）。
 */
function extractDocDigest(
	rel: string,
	content: string,
	threshold: number,
): string | null {
	if (Buffer.byteLength(content, "utf8") <= threshold) return null;
	const lines = content.split(/\r?\n/);
	const start = lines.findIndex((l) => l.trimEnd() === CORE_SECTION_HEADING);
	if (start < 0) return null;
	const core: string[] = [];
	for (let i = start + 1; i < lines.length; i++) {
		if (/^##\s+/.test(lines[i])) break;
		core.push(lines[i]);
	}
	const toc: string[] = [];
	for (const l of lines) {
		const m = l.match(/^##\s+(.+?)\s*$/);
		if (m && m[1] !== "硬规则速览") toc.push(m[1]);
	}
	return `${core.join("\n").trim()}\n\n**全文目录** — 需要细节请 \`read ${rel}\` 对应小节：\n${toc.join(" / ")}`;
}

/* ---------- 注入块构造（tier-b：装配条目 + 预算降级） ---------- */

/** 装配条目：注入块中一个可独立降级的渲染单元（tier-b design D3） */
export interface PlanItem {
	/** 记账 path（文档相对路径 / change 文件相对路径） */
	key: string;
	/** `### xxx` 标题行——降级只换正文，标题恒在（永不真丢） */
	heading: string;
	/** 当前正文（记账 bytes 依据；降级时被替换） */
	body: string;
	/** 占位行预览（≤120 字） */
	preview: string;
	/** 占位行 read 指引路径 */
	relPath: string;
	/** 降级通知/展示用的短名 */
	short: string;
	/** 注入块 docList 展示标签（如 `daily-report.md(红线层)`；稳定层 header 复用） */
	docLabel?: string;
	reason: InjectedDocEntry["reason"];
	mode: InjectedDocEntry["mode"];
	/** 强制 digest 变体（change-file 专属：未超自身阈值时预算可再压一层） */
	digestAltBody?: string;
	/** 降级后状态（applyBudget 写入）：digest=收紧摘要 placeholder=占位行 */
	degradedTo?: "digest" | "placeholder";
	/** 声明域注入层级（redline=红线层 / full=回退全节或全文）；非 declaration 条目不携带 */
	layer?: "redline" | "full";
}

const placeholderBody = (it: PlanItem): string =>
	`（预算已满，全文已省略）\n> ${it.preview}\n> 全文可 \`read ${it.relPath}\``;

/** 字节数 → 「12.3K」展示（header 降级行 / widget 预算用量） */
const fmtK = (bytes: number): string => `${(bytes / 1024).toFixed(1)}K`;

const renderSections = (items: PlanItem[]): string[] =>
	items.map((it) => `${it.heading}\n\n${it.body}`);

const renderBlockBytes = (header: string, sections: string[]): number =>
	Buffer.byteLength(`\n\n${header}\n---\n\n${sections.join("\n\n---\n\n")}\n`, "utf8");

export interface BudgetOutcome {
	/** 降级后的条目副本（记账 bytes/degraded 依据；未降级 = 原样副本） */
	items: PlanItem[];
	sections: string[];
	/** 已降级条目（按降级发生顺序；header 通知与记账用），未降级为空 */
	degraded: PlanItem[];
	/** 最终渲染块字节（含 header） */
	totalBytes: number;
}

/**
 * 预算降级纯函数（tier-b design D3 两遍装配）：
 * 第一遍全量装配测字节，未超预算原样返回（常态零降级）；超线按 D2 分层顺序
 * 逐动作降级重测（keyword → jit → change-file digest 收紧 → 声明域 →
 * change-file 占位），进预算即停；动作耗尽仍超 = 地板（baseDocs + 占位行集合，
 * 永不真丢）。确定性：输入顺序稳定 + 无随机无时间依赖 → 同输入同输出字节，
 * 推线回合前缀缓存只抖一次。
 */
export function applyBudget(
	items: PlanItem[],
	budgetBytes: number,
	buildHeader: (degraded: PlanItem[] | null) => string,
): BudgetOutcome {
	const current = items.map((it) => ({ ...it }));
	const degraded: PlanItem[] = [];
	const measure = () =>
		renderBlockBytes(
			buildHeader(degraded.length ? degraded : null),
			renderSections(current),
		);
	if (measure() <= budgetBytes) {
		return {
			items: current,
			sections: renderSections(current),
			degraded: [],
			totalBytes: measure(),
		};
	}
	// 降级程序（D2 分层顺序；层内按装配顺序 = 首次命中顺序）
	const actions: { item: PlanItem; to: "digest" | "placeholder" }[] = [];
	const add = (reason: PlanItem["reason"], to: "digest" | "placeholder") => {
		for (const it of current) {
			if (it.reason !== reason) continue;
			if (to === "digest" && !it.digestAltBody) continue;
			actions.push({ item: it, to });
		}
	};
	add("keyword", "placeholder");
	add("jit-path", "placeholder");
	add("change-file", "digest");
	add("declaration", "placeholder");
	add("change-file", "placeholder");
	for (const a of actions) {
		if (measure() <= budgetBytes) break;
		if (a.item.degradedTo) continue; // 已降级条目不重复降（change-file 两层）
		if (a.to === "digest") a.item.body = a.item.digestAltBody ?? a.item.body;
		else a.item.body = placeholderBody(a.item);
		a.item.degradedTo = a.to;
		degraded.push(a.item);
	}
	return {
		items: current,
		sections: renderSections(current),
		degraded,
		totalBytes: measure(),
	};
}

/** 首个非空、非标题行作预览（≤120 字） */
function firstLine(text: string, max: number): string {
	for (const l of text.split(/\r?\n/)) {
		const t = l.trim();
		if (t && !t.startsWith("#")) return t.length > max ? t.slice(0, max) + "…" : t;
	}
	return "";
}

/**
 * 构建 change 目录内单个 md 文件的装配条目；超自身阈值走 digest（digestAltBody 仅在
 * 未超阈值时提供——预算 digest 收紧层对已 digest 条目为 no-op）。文件不存在/空 → null。
 */
function buildChangeFileItem(
	changeDir: string,
	filename: string,
	relPath: string,
	label: string,
	short: string,
	statusPrefix: string,
	threshold: number,
	digest: (raw: string) => string,
): { status: string | null; raw: string | null; item: PlanItem | null } {
	const p = join(changeDir, filename);
	if (!existsSync(p)) return { status: null, raw: null, item: null };
	try {
		const raw = readFileSync(p, "utf8").trim();
		if (!raw) return { status: null, raw: null, item: null };
		const count = (raw.match(/^##\s+/gm) || []).length;
		const over = raw.length > threshold;
		const injected = over ? digest(raw) : raw;
		const firstHeading = raw.match(/^##[ \t]+(.+?)[ \t]*$/m)?.[1] ?? "";
		const item: PlanItem = {
			key: relPath,
			heading: `### ${label}`,
			body: injected,
			preview: firstHeading
				? `首个小节：${firstHeading.replace(/[ \t]*<!--[ \t]*pin:[0-9a-f]{8}[ \t]*-->[ \t]*$/, "").trim()}`
				: firstLine(raw, 120),
			relPath,
			short,
			reason: "change-file",
			mode: over ? "digest" : "full",
			docLabel: `${statusPrefix}:${count}条${over ? "(摘要)" : ""}`,
			...(over ? {} : { digestAltBody: digest(raw) }),
		};
		return {
			status: `${statusPrefix}:${count}条${over ? "(摘要)" : ""}`,
			raw,
			item,
		};
	} catch {
		return { status: null, raw: null, item: null };
	}
}

/** constraint.inject 记账条目（design D5）：注入文档的路径/形态/命中原因/字节数；degraded 仅预算降级回合携带（tier-b D4） */
interface InjectedDocEntry {
	path: string;
	mode: "section" | "digest" | "full";
	reason:
		| "index"
		| "mode-base"
		| "stack-conditional"
		| "declaration"
		| "keyword"
		| "jit-path"
		| "change-file";
	bytes: number;
	degraded?: boolean;
	/** 声明域注入层级（@constraint-declaration-redline）：redline=红线层 / full=回退全节或全文；其余 reason 不携带 */
	layer?: "redline" | "full";
}

/** pin.read 记账条目（design D5）：explore-findings.md 的 pin 标题（research 锚点已剥除） */
interface PinReadEntry {
	title: string;
	doc: string;
	digested: boolean;
}

interface InjectionPlan {
	mode: Mode | null;
	modeLabel: string;
	changeName: string | null;
	/** 绑定是否来自档位绑定（false = mtime 兜底/无；header 展示 `（档位绑定）` 后缀用） */
	bound: boolean;
	docList: string[]; // 实际注入的文档（用于状态展示）
	docEntries: InjectedDocEntry[]; // 事实库记账（constraint.inject）
	pinReads: PinReadEntry[]; // 事实库记账（pin.read，注入 explore-findings 成功时）
	// 业务域声明状态（design D2/D5）：注入块头部 + widget 展示；null = 非档位语境
	declNote: string | null;
	// 预算用量展示（widget；null = 预算禁用或未激活档）：如「预算 12.3K/32K（降级2）」
	budgetNote: string | null;
	// 降级后条目（混合通道按 reason 切分稳定层/动态层；legacy 模式不用）
	items: PlanItem[];
	/** 本回合被预算降级的条目短名（动态层降级通知用；tier-b） */
	degraded: string[];
	block: string; // 追加进 system prompt 的完整文本
}

function planInjection(
	cwd: string,
	config: ConstraintConfig,
	state: SessionState,
	fallbackChange: { name: string; dir: string } | null,
): InjectionPlan {
	const mode = state.mode;
	const boundChangeName = state.boundChange;
	const docList: string[] = [];
	const docEntries: InjectedDocEntry[] = [];
	const pinReads: PinReadEntry[] = [];

	// 文本分析用的 change：优先档位绑定的 change，其次 mtime 最新
	const bound =
		mode && boundChangeName
			? (listChangeDirs(cwd).find((c) => c.name === boundChangeName) ?? null)
			: null;
	// 分析源 change：implementation 档未绑定时 mtime 兜底（/opsx-apply 无参语境正确）；
	// requirements 档与未激活档不兜底——explore/propose 是新想法语境，拿无关 change 的
	// proposal 文本做关键词命中会让 explore 内容错位（@Syntopica bugfix：新会话 explore
	// 默认绑 mtime 最新 change，pin 落错库）。此时命中源只剩对话输入本身，聊什么注什么。
	const analysisChange =
		mode === "implementation" ? (bound ?? fallbackChange) : bound;

	// 未激活档（无 mode）：仅注入索引文档，不做 change 文本分析。
	// 否则「最近活跃 change」的静态文本会让命中的规范全文常驻本仓库所有会话（命中源错位）。
	// 关键词命中源 = 仅最近 N 条用户输入（@constraint-domain-declaration：change 产物全文
	// 退出命中源——harness/AI 类 change 的正当词汇与业务域关键词本质重叠，整会话误注入；
	// change 文本仅保留给 detectStacks 栈判定，栈信号为路径类词无误伤面）
	const changeText =
		mode && analysisChange ? readChangeText(analysisChange) : "";
	const matchText = mode ? state.recentInputs.join("\n") : "";
	const stacks = mode
		? detectStacks(`${changeText}\n${state.recentInputs.join("\n")}`, config)
		: { backend: false, frontend: false };
	if (mode) matchKeywordDocs(matchText, config, state); // 命中写本会话粘性集合（只增不减）

	// 业务域声明（design D2 主层）：档位激活 + 有分析源 change → 解析 proposal 头部
	// constraint-domains 标记；声明域注入对应 flow 约束节。每回合从文件重解析（proposal
	// 编辑立即生效），不进粘性集合——声明是持久文件不存在滚出窗口问题。
	const declDomains: string[] = [];
	const declUnknown: string[] = [];
	if (mode && analysisChange) {
		const proposalText = readDoc(
			cwd,
			`${OPENSPEC_CHANGES_DIR}/${analysisChange.name}/proposal.md`,
		);
		if (proposalText != null) {
			const valid = new Set(listFlowDomains(cwd));
			for (const d of parseDomainDeclaration(proposalText)) {
				if (valid.has(d)) declDomains.push(d);
				else declUnknown.push(d);
			}
		}
	}

	// 基础文档（按 mode）；无 mode → 仅索引。reason 随 entry 携带（事实库记账的命中原因）
	let ordered: (DocEntry & { reason: InjectedDocEntry["reason"] })[] = [];
	if (mode) {
		ordered = config.modes[mode].baseDocs.map((d) => ({
			doc: d,
			reason: "mode-base" as const,
		}));
		// 条件文档（仅实现档）
		if (
			mode === "implementation" &&
			config.modes.implementation.conditionalDocs
		) {
			for (const cd of config.modes.implementation.conditionalDocs) {
				if (cd.ifStack === "backend" && (stacks.backend || !stacks.frontend))
					ordered.push({ doc: cd.doc, reason: "stack-conditional" });
				if (cd.ifStack === "frontend" && stacks.frontend)
					ordered.push({ doc: cd.doc, reason: "stack-conditional" });
			}
		}
	} else {
		ordered = [{ doc: config.indexDoc, reason: "index" }];
	}

	// 声明域文档（紧跟 baseDocs，语义最相关）→ 关键词文档（粘性集合，首次命中顺序）
	// → JIT 追加（粘性集合）——后两者只增不减
	const seen = new Set(ordered.map((e) => e.doc));
	for (const d of declDomains) {
		const doc = `docs/reference/flow/${d}.md`;
		if (!seen.has(doc)) {
			seen.add(doc);
			ordered.push({
				doc,
				section: "业务约束与不变量",
				reason: "declaration" as const,
			});
		}
	}
	// 关键词命中域限定（D7）：仅声明域 ∪ 栈条件文档 ∪ baseDocs/索引 内的命中生效；
	// 集合外命中丢弃（跨域词不再误拉无关域全节）
	const allowedKeywordDocs = new Set<string>([
		config.indexDoc,
		...(mode ? config.modes[mode].baseDocs : []),
		...declDomains.map((d) => `docs/reference/flow/${d}.md`),
		...(mode === "implementation" && config.modes.implementation.conditionalDocs
			? config.modes.implementation.conditionalDocs
					.filter((cd) =>
						cd.ifStack === "backend"
							? stacks.backend || !stacks.frontend
							: stacks.frontend,
					)
					.map((cd) => cd.doc)
			: []),
	]);
	for (const e of filterKeywordDocs(state.keywordDocHits.values(), allowedKeywordDocs)) {
		// 关键词命中声明的域 → 以全节形态（细节层）投递，替代声明项（红线层是全集子集，
		// 避免同文档双重注入）。稳定层快照仍保留激活时刻的红线层条目（冻结语义）。
		const dup = ordered.findIndex((o) => o.doc === e.doc && o.reason === "declaration");
		if (dup >= 0) {
			ordered.splice(dup, 1);
			seen.delete(e.doc);
		}
		if (!seen.has(e.doc)) {
			seen.add(e.doc);
			ordered.push({ doc: e.doc, section: e.section, reason: "keyword" });
		} else {
			// 同 doc 已因其他 reason 入列（如栈条件）→ 升级为关键词形态（全节）
			const same = ordered.findIndex((o) => o.doc === e.doc);
			if (same >= 0) ordered[same] = { doc: e.doc, section: e.section, reason: "keyword" };
		}
	}
	for (const e of state.jitDocHits.values()) {
		if (!seen.has(e.doc)) {
			seen.add(e.doc);
			ordered.push({ doc: e.doc, section: e.section, reason: "jit-path" });
		}
	}

	const digestThreshold = config.digestDocThreshold ?? 6144;
	const items: PlanItem[] = [];
	for (const entry of ordered) {
		const content = readDoc(cwd, entry.doc);
		if (content == null) continue;
		const rel = entry.doc;
		const display = basename(rel);
		// 节级注入：配置了 section 且文档存在该节 → 只注节内容 + 全文指引；
		// 节不存在或节字节数 < minSectionBytes（编辑中间态残缺节）回落全文（fail-safe）
		const minSectionBytes = config.minSectionBytes ?? 512;
		let injected: string | null = null;
		let injectMode: InjectedDocEntry["mode"] = "full";
		// 注入块 docList 展示标签（与 docList.push 同步；split 模式稳定层 header 复用）
		let docLabel = display;
		// 声明域注入层级（@constraint-declaration-redline）：redline=红线层命中；
		// full=回退（全节或全文）——记账 bytes 如实记实际注入层级字节数
		let layer: "redline" | "full" | undefined =
			entry.reason === "declaration" ? "full" : undefined;
		docLabel = display;
		if (entry.section) {
			const sec = extractSection(content, entry.section);
			if (
				sec &&
				Buffer.byteLength(sec, "utf8") >= minSectionBytes
			) {
				// 声明域先试红线层（design D2~D4）：顶层列表项首个加粗块逐行 +
				// 细节层取回指引尾行；0 条或拼接低于 minSectionBytes 回退全节
				// （fail-safe，不注残缺红线层）；keyword / jit-path 命中仍注全节
				// （细节层通道，防误伤）
				if (entry.reason === "declaration") {
					const red = extractRedlines(sec);
					if (red && red.bytes >= minSectionBytes) {
						injected = `${red.lines.join("\n")}\n\n细节层：read \`${rel}\`「${entry.section}」节`;
						injectMode = "section";
						layer = "redline";
						docLabel = `${display}(红线层)`;
						docList.push(docLabel);
					}
				}
				if (injected == null) {
				injected = `${sec}\n\n（本节摘自 \`${rel}\` 的「${entry.section}」节，全文可 \`read ${rel}\`）`;
				injectMode = "section";
				docLabel = `${display}(${entry.section.slice(0, 8)}节)`;
				docList.push(docLabel);
				}
			}
		}
		if (injected == null) {
			const digest = extractDocDigest(rel, content, digestThreshold);
			if (digest) {
				docLabel = `${display}(速览)`;
				docList.push(docLabel);
				items.push({
					key: rel,
					heading: `### ${display}（硬规则速览）`,
					body: digest,
					preview: firstLine(digest, 120),
					relPath: rel,
					short: display,
					reason: entry.reason,
					mode: "digest",
					docLabel,
				});
				continue;
			}
			docList.push(display);
			injected = content.trim();
		}
		items.push({
			key: rel,
			heading: `### ${display}`,
			body: injected,
			preview: entry.section ? `「${entry.section}」节` : firstLine(injected, 120),
			relPath: rel,
			short: display,
			reason: entry.reason,
			mode: injectMode,
			docLabel,
			...(layer ? { layer } : {}),
		});
	}

	// change 级文件注入（仅实现档 + 有活跃 change）：探索发现 + 词汇表（主线程定义→子线程可见）
	let findingsRaw: string | null = null;
	let findingsRel = "";
	if (mode === "implementation" && analysisChange) {
		findingsRel = `${OPENSPEC_CHANGES_DIR}/${analysisChange.name}/${config.findingsFile}`;
		const vocabRel = `${OPENSPEC_CHANGES_DIR}/${analysisChange.name}/${config.vocabularyFile ?? "ubiquitous-language.md"}`;
		const changeFiles = [
			{
				rel: findingsRel,
				file: config.findingsFile,
				label: "📌 探索阶段发现（explore-findings.md）",
				short: "探索发现",
			},
			{
				rel: vocabRel,
				file: config.vocabularyFile ?? "ubiquitous-language.md",
				label: "📖 词汇表（ubiquitous-language.md）—— 命名单一真相源，「废除别名」列禁用",
				short: "词汇表",
			},
		];
		for (const cf of changeFiles) {
			const r = buildChangeFileItem(
				analysisChange.dir,
				cf.file,
				cf.rel,
				cf.label,
				cf.short,
				cf.short,
				config.findingsDigestThreshold,
				digestFindings,
			);
			if (r.status) docList.push(r.status);
			if (r.item) items.push(r.item);
			if (cf.short === "探索发现") findingsRaw = r.raw;
		}
	}

	// 声明状态（widget + 注入块头部展示；design D5）：有声明列域，无声明显式提示
	// （纯工具链 change 合法不阻断），未知域名恒提示
	const declNote =
		mode && analysisChange
			? declDomains.length
				? `声明域：${declDomains.join(", ")}${declUnknown.length ? `（忽略未知域：${declUnknown.join(", ")}）` : ""}`
				: `无域声明（纯工具链 change 可忽略）${declUnknown.length ? `；忽略未知域：${declUnknown.join(", ")}` : ""}`
			: null;

	// —— 预算装配（tier-b design D1/D3）：条目 → applyBudget → sections/记账/header ——
	const budgetBytes = config.budgetBytes ?? DEFAULT_BUDGET_BYTES;
	const budgetActive = budgetBytes > 0;
	const declLine = declNote ? `- 域声明：${declNote}` : null;
	const modeLine = `- 档位：${mode ? config.modes[mode].label : "未激活（仅索引）"}`;
	const changeLine = `- 活跃变更：${analysisChange ? analysisChange.name : "无"}${bound ? "（档位绑定）" : ""}`;
	const docLine = `- 命中文档：${docList.length ? docList.join("、") : "无"}`;
	const buildHeader = (degraded: PlanItem[] | null): string =>
		[
			`## 🔒 项目约束（constraint-injection extension 强制注入，本阶段必须遵守）`,
			``,
			`> 与 AGENTS.md 优先级宪法冲突时，以宪法为准。`,
			``,
			modeLine,
			changeLine,
			...(declLine ? [declLine] : []),
			docLine,
			...(degraded
				? [
						`- ⚠️ 预算降级（上限 ${fmtK(budgetBytes)}）：已省略 ${degraded.map((d) => d.short).join("、")}——占位保留 read 路径，需要时自行补取全文`,
					]
				: []),
			``,
		].join("\n");
	let outcome: BudgetOutcome;
	if (budgetActive) {
		outcome = applyBudget(items, budgetBytes, buildHeader);
	} else {
		outcome = {
			items,
			sections: renderSections(items),
			degraded: [],
			totalBytes: renderBlockBytes(buildHeader(null), renderSections(items)),
		};
	}

	// 记账：bytes = 降级后正文；降级条目附 degraded 标记（tier-b D4，月度 SQL 瘦身依据）
	for (const it of outcome.items) {
		docEntries.push({
			path: it.key,
			mode: it.mode,
			reason: it.reason,
			bytes: Buffer.byteLength(it.body, "utf8"),
			...(it.degradedTo ? { degraded: true } : {}),
			...(it.layer ? { layer: it.layer } : {}),
		});
	}

	// pin.read（A1/D5）：explore-findings 注入成功后按 ## 二级标题解析（research 锚点剥除）；
	// digest 模式标题仍保留在摘要里照常记；预算降级到占位行时模型未见标题 → 不记
	if (findingsRaw) {
		const fItem = outcome.items.find((i) => i.key === findingsRel);
		if (fItem?.degradedTo !== "placeholder") {
			for (const m of findingsRaw.matchAll(/^##[ \t]+(.+?)[ \t]*$/gm)) {
				const title = m[1]
					.replace(/\r$/, "")
					.replace(/[ \t]*<!--[ \t]*pin:[0-9a-f]{8}[ \t]*-->[ \t]*$/, "")
					.trim();
				if (title) {
					pinReads.push({
						title,
						doc: findingsRel,
						digested: fItem?.degradedTo === "digest" || fItem?.mode === "digest",
					});
				}
			}
		}
	}

	const header = buildHeader(outcome.degraded.length ? outcome.degraded : null);
	const block = docList.length
		? `\n\n${header}\n---\n\n${outcome.sections.join("\n\n---\n\n")}\n`
		: "";
	const budgetNote =
		budgetActive && mode
			? `预算 ${fmtK(outcome.totalBytes)}/${fmtK(budgetBytes)}${outcome.degraded.length ? `（降级${outcome.degraded.length}）` : ""}`
			: null;

	return {
		mode,
		modeLabel: mode ? config.modes[mode].label : "未激活",
		changeName: analysisChange ? analysisChange.name : null,
		bound: bound !== null,
		docList,
		docEntries,
		pinReads,
		declNote,
		budgetNote,
		items: outcome.items,
		degraded: outcome.degraded.map((d) => d.short),
		block,
	};
}

function digestFindings(raw: string): string {
	const lines = raw.split("\n");
	const out: string[] = [];
	let i = 0;
	while (i < lines.length) {
		const line = lines[i];
		if (/^##\s+/.test(line)) {
			out.push(line);
			// 找下一个非空行作为预览
			for (let j = i + 1; j < lines.length; j++) {
				const t = lines[j].trim();
				if (t && !t.startsWith("#")) {
					out.push(`> ${t.length > 120 ? t.slice(0, 120) + "…" : t}`);
					break;
				}
				if (/^##\s+/.test(lines[j])) break;
			}
			out.push("");
		}
		i++;
	}
	out.push(
		"> 篇幅过大仅展示摘要，详情请 `read openspec/changes/<change>/explore-findings.md` 对应小节",
	);
	return out.join("\n");
}

/** research 语境 pin 锚点（design D5）：8 位 16 进制，文件内碰撞概率可忽略 */
function hex8(): string {
	let s = "";
	for (let i = 0; i < 8; i++) s += Math.floor(Math.random() * 16).toString(16);
	return s;
}

/* ---------- 档位持久化记账（tier-b design D5 + harden D6） ---------- */

/** 绑定来源（可归因；隐性绑定不存在） */
type ModeSetSource =
	| "command"
	| "skill"
	| "edit-dir"
	| "recover"
	| "inherit"
	| "fallback";

/**
 * 档位激活/绑定修正时自报 mode.set（payload 含 mode 与绑定 change；change 列同步写）。
 * 每次设置/修正各记一条（审计语义是「何时设的」，resume 取「最近一条」）；
 * 记账失败不阻断档位激活（logEvent 已 fail-loud，此处静默——激活是主路径）。
 * 无 sessionId（非真实会话，如烟测 stub）→ 跳过。
 */
function logModeSet(
	cwd: string,
	sessionId: string | undefined,
	mode: Mode,
	boundChange: string | null,
	source: ModeSetSource,
): void {
	if (!sessionId) return;
	logEvent(cwd, {
		kind: "mode.set",
		sessionId,
		change: boundChange,
		payload: { mode, boundChange, source },
	});
}

/** 解析单条 mode.set 行为 {mode, boundChange}；payload 损坏/非法 → null */
function modeSetFromRow(
	row: HarnessEventRow,
): { mode: Mode; boundChange: string | null } | null {
	try {
		const payload = JSON.parse(row.payload) as {
			mode?: string;
			boundChange?: string | null;
		};
		if (payload.mode !== "requirements" && payload.mode !== "implementation") {
			return null;
		}
		return { mode: payload.mode, boundChange: payload.boundChange ?? null };
	} catch {
		return null; // payload 损坏 → 回落未激活
	}
}

/** 同一条 mode.set → 恢复结果（含绑定 change 存在性校验）；不可恢复 → null */
function recoverFromRow(
	cwd: string,
	row: HarnessEventRow,
): { mode: Mode; boundChange: string | null } | null {
	const rec = modeSetFromRow(row);
	if (!rec) return null;
	// 绑定 change 不存在（已归档/删除）→ 回落未激活，不恢复无绑定的旧档
	if (
		rec.boundChange &&
		!existsSync(join(cwd, OPENSPEC_CHANGES_DIR, rec.boundChange))
	) {
		return null;
	}
	return rec;
}

/** resume/reload 档位恢复（fix-mode-recovery-cross-session：仅同 sessionId 单段）：
 *  三条恢复路径统一只按本会话自己的 mode.set 取数，MUST NOT 全局兜底——多 pi
 *  窗口并行是常态，「全局最新 mode.set」几乎必然属其他窗口（实测 2026-08-23：
 *  探索窗口 reload 被灌入他窗口 implementation 档）；/resume 切到目标会话文件，
 *  sessionId 即目标会话 id，同 id 取数必中，全局兜底防御的场景不存在 */
function recoverMode(
	cwd: string,
	sessionId: string | undefined,
): { mode: Mode; boundChange: string | null } | null {
	if (!sessionId) return null;
	const rows = queryBySession(cwd, sessionId, ["mode.set"]);
	if (!rows.length) return null;
	return recoverFromRow(cwd, rows[rows.length - 1]);
}

/* ---------- 命令匹配 ---------- */

function matchCommand(text: string, config: ConstraintConfig): Mode | null {
	const firstToken = text.trim().split(/\s+/)[0]?.toLowerCase();
	if (!firstToken) return null;
	for (const mode of ["requirements", "implementation"] as Mode[]) {
		if (config.modes[mode].commands.some((c) => c.toLowerCase() === firstToken))
			return mode;
	}
	return null;
}

/* ---------- skill 文件路径匹配（agent 自动 read skill → 阶段激活） ---------- */

function matchSkillPath(path: string, config: ConstraintConfig): Mode | null {
	const signals = config.skillSignals;
	if (!signals) return null;
	for (const mode of ["requirements", "implementation"] as Mode[]) {
		for (const name of signals[mode] ?? []) {
			if (
				path.includes(`/skills/${name}/`) ||
				path.includes(`\\skills\\${name}\\`)
			) {
				return mode;
			}
		}
	}
	return null;
}

/* ---------- 混合通道纯函数（D2~D5/D7；可脱离 pi harness smoke 直跑） ---------- */

/**
 * change 目录路径 → change 名（`openspec/changes/<name>/...`）；非 change 路径 → null。
 * ARCHIVE_DIR 下的归档产物不算活跃 change。
 */
export function changeNameFromPath(path: string): string | null {
	const m = path.match(/openspec[/\\]changes[/\\]([^/\\]+)[/\\]/);
	if (!m) return null;
	if (m[1] === ARCHIVE_DIR) return null;
	return m[1];
}

/** 绑定修正判定输入 */
export interface BindActionInput {
	tool: string;
	path: string;
	currentBound: string | null;
	boundExists: boolean;
}

/**
 * 写 change 目录的绑定修正判定（D5）：read 永不抢绑；命中 change 目录时仅当当前绑定
 * 不健康（未绑定 / 绑定 change 目录已消失）才兜底绑定；当前绑定健康时不抢绑。
 */
export function resolveBindAction(input: BindActionInput): "noop" | "rebind" {
	if (input.tool !== "write" && input.tool !== "edit") return "noop";
	const name = changeNameFromPath(input.path);
	if (!name) return "noop";
	const healthy = input.currentBound !== null && input.boundExists;
	if (healthy) return "noop";
	return name === input.currentBound ? "noop" : "rebind";
}

/** turn 绑定锁定（D5）：锁定后同 turn 不再应用任何绑定变化 */
export function shouldApplyBind(action: "noop" | "rebind", locked: boolean): boolean {
	return action === "rebind" && !locked;
}

/** 稳定层快照 key（D2）：仅 mode + 绑定 change；内容变化（proposal 编辑/文档更新）不改 key，
 *  改走动态层差异通知——保证 system prompt 在档位生命周期内字节恒定 */
export function stableSnapshotKey(
	mode: Mode | null,
	boundChange: string | null,
): string {
	return `${mode ?? "none"}|${boundChange ?? "none"}`;
}

/** 动态层条目（投递/指纹最小单元） */
export interface DynamicItem {
	key: string;
	heading: string;
	body: string;
	relPath: string;
	short: string;
	reason: InjectedDocEntry["reason"];
	mode: InjectedDocEntry["mode"];
	/** 本条目已被预算降级（记账 degraded 标记） */
	degraded?: boolean;
}

/** FNV-1a 32 位十六进制（内容指纹；抗碰撞要求不高，仅用于 diff 判定） */
export function hashContent(text: string): string {
	let h = 0x811c9dc5;
	for (let i = 0; i < text.length; i++) {
		h ^= text.charCodeAt(i);
		h = (h + ((h << 1) + (h << 4) + (h << 7) + (h << 8) + (h << 24))) >>> 0;
	}
	return h.toString(16).padStart(8, "0");
}

/**
 * 动态层增量计算（D3）：与已投递指纹比对，返回新增/内容变化的条目 + 新指纹。
 * 相同 key 且内容 hash 未变 → 非增量（零投递）。已投递但当前不在集合中的条目被丢弃
 * （档位切换后集合重置）。
 */
export function computeDynamicDiff(
	current: DynamicItem[],
	sent: Map<string, string>,
): { increments: DynamicItem[]; fingerprint: Map<string, string> } {
	const fingerprint = new Map<string, string>();
	const increments: DynamicItem[] = [];
	for (const item of current) {
		const h = hashContent(`${item.reason}|${item.body}`);
		fingerprint.set(item.key, h);
		if (sent.get(item.key) !== h) increments.push(item);
	}
	return { increments, fingerprint };
}

/**
 * 关键词命中域限定（D7）：仅保留落在允许集合内的命中文档（声明域 ∪ 栈条件文档 ∪
 * baseDocs/索引）。集合外命中丢弃——跨域词不再误拉无关域全节。
 */
export function filterKeywordDocs<T extends { doc: string }>(
	hits: Iterable<T>,
	allowed: Set<string>,
): T[] {
	const out: T[] = [];
	for (const h of hits) if (allowed.has(h.doc)) out.push(h);
	return out;
}

/** 动态层消息正文渲染（D3） */
export function renderDynamicMessage(
	items: DynamicItem[],
	reasonLabel = "📎 约束补充（增量）",
): { content: string; display: boolean } | null {
	if (!items.length) return null;
	const lines: string[] = [
		`## ${reasonLabel}`,
		``,
		`> 与 AGENTS.md 优先级宪法冲突时，以宪法为准。`,
		``,
	];
	for (const it of items) {
		lines.push(it.heading, "", it.body, "");
	}
	return { content: lines.join("\n"), display: false };
}

/** 稳定层/动态层在 system prompt 中的分界线（D1）；legacy 模式下不分层 */
export function splitPlanItems(items: PlanItem[]): {
	stable: PlanItem[];
	dynamic: PlanItem[];
} {
	const stableReasons = new Set(["index", "mode-base", "stack-conditional", "declaration"]);
	const stable: PlanItem[] = [];
	const dynamic: PlanItem[] = [];
	for (const it of items) {
		if (stableReasons.has(it.reason)) stable.push(it);
		else dynamic.push(it);
	}
	return { stable, dynamic };
}

/* ---------- 混合通道投递与记账辅助（D2~D4/D6） ---------- */

/** extension ctx 的最小需求面（便于测试与子调用） */
type ExtCtx = {
	cwd: string;
	sessionManager?: {
		getSessionId?: () => string | undefined;
		/** session header：fork/子线程凭证（parentSession = 父会话文件路径）——
		 *  per-session-constraint-binding 用它判定「父子关系可证」才继承 */
		getHeader?: () => { parentSession?: string } | null;
		/** 会话文件绝对路径——header 未落盘时经 `forks/` 路径兜底解父 id */
		getSessionFile?: () => string | undefined | null;
	};
};

/** PlanItem → 记账条目（D6：bytes 取降级后正文） */
function itemToDocEntry(it: PlanItem): InjectedDocEntry {
	return {
		path: it.key,
		mode: it.mode,
		reason: it.reason,
		bytes: Buffer.byteLength(it.body, "utf8"),
		...(it.degradedTo ? { degraded: true } : {}),
		...(it.layer ? { layer: it.layer } : {}),
	};
}

/** PlanItem → 动态层条目 */
function toDynamicItems(items: PlanItem[]): DynamicItem[] {
	return items.map((it) => ({
		key: it.key,
		heading: it.heading,
		body: it.body,
		relPath: it.relPath,
		short: it.short,
		reason: it.reason,
		mode: it.mode,
		...(it.degradedTo ? { degraded: true } : {}),
	}));
}

/** 稳定层 header（split 模式）：只含稳定层信息——动态文档不入 header，保证字节恒定（D2） */
function buildStableHeader(plan: InjectionPlan, stableItems: PlanItem[]): string {
	const docList = stableItems.map((it) => it.docLabel ?? it.short);
	const lines = [
		`## 🔒 项目约束（constraint-injection extension 强制注入，本阶段必须遵守）`,
		``,
		`> 与 AGENTS.md 优先级宪法冲突时，以宪法为准。`,
		``,
		`- 档位：${plan.modeLabel}`,
		`- 活跃变更：${plan.changeName ?? "无"}${plan.bound ? "（档位绑定）" : ""}`,
	];
	if (plan.declNote) lines.push(`- 域声明：${plan.declNote}`);
	lines.push(`- 命中文档：${docList.length ? docList.join("、") : "无"}`, ``);
	return lines.join("\n");
}

/** 稳定层块渲染（与 planInjection 的块格式一致）；无稳定条目但有动态内容时至少保留 header */
function renderStableBlock(header: string, items: PlanItem[]): string {
	if (!items.length) return header ? `\n\n${header}\n` : "";
	return `\n\n${header}\n---\n\n${renderSections(items).join("\n\n---\n\n")}\n`;
}

/** 稳定层变化检测（D2）：当前条目 hash 与快照不同的（含新增）→ 作为动态层差异条目 */
function diffStableItems(snapshot: PlanItem[], current: PlanItem[]): PlanItem[] {
	const snap = new Map<string, string>();
	for (const it of snapshot) snap.set(it.key, hashContent(`${it.reason}|${it.body}`));
	const changed: PlanItem[] = [];
	for (const it of current) {
		if (snap.get(it.key) !== hashContent(`${it.reason}|${it.body}`)) changed.push(it);
	}
	return changed;
}

/** constraint.inject 批量记账（D6：送达时记；source 仅 compact 重发携带） */
function logConstraintInjects(
	ctx: ExtCtx,
	sessionId: string | undefined,
	changeName: string | null,
	entries: InjectedDocEntry[],
	source?: string,
): void {
	if (!sessionId) return;
	for (const d of entries) {
		logEvent(ctx.cwd, {
			kind: "constraint.inject",
			sessionId,
			change: changeName,
			payload: source ? { ...d, source } : { ...d },
		});
	}
}

/** 动态层消息标签（含预算降级通知；turn 起点与 JIT 即时两条路径共用） */
function dynamicLabelWithDegrad(
	base: string,
	plan: InjectionPlan,
	cfg: ConstraintConfig,
): string {
	return plan.degraded.length
		? `${base} — 预算降级（上限 ${fmtK(cfg.budgetBytes ?? DEFAULT_BUDGET_BYTES)}）：已省略 ${plan.degraded.join("、")}，见下方占位行`
		: base;
}

/**
 * 动态层投递（D3）：算增量 → 非空（或 force）则经 steer 通道发送 → 写指纹。
 * `force` 用于 compact 快照重发（无视指纹）。返回投递条目数（0 = 零投递）。
 */
function deliverDynamic(
	pi: ExtensionAPI,
	ctx: ExtCtx,
	cfg: ConstraintConfig,
	state: SessionState,
	changeName: string | null,
	items: DynamicItem[],
	label: string,
	force = false,
): number {
	const { increments, fingerprint } = computeDynamicDiff(
		items,
		state.channel.sentFingerprint,
	);
	if (!increments.length && !force) return 0;
	const payloadItems = increments.length ? increments : items;
	const rendered = renderDynamicMessage(payloadItems, label);
	if (!rendered) return 0;
	state.channel.sentFingerprint = fingerprint;
	pi.sendMessage(
		{
			customType: "constraint-injection",
			content: rendered.content,
			display: cfg.dynamicDisplay === "full",
		},
		{ deliverAs: "steer", triggerTurn: false },
	);
	logConstraintInjects(
		ctx,
		realSessionId(state),
		changeName,
		payloadItems.map((it) => ({
			path: it.key,
			mode: it.mode,
			reason: it.reason,
			bytes: Buffer.byteLength(it.body, "utf8"),
			...(it.degraded ? { degraded: true } : {}),
		})),
		force ? "compact-resend" : undefined,
	);
	return increments.length;
}

/* ---------- extension 主体 ---------- */

export default function (pi: ExtensionAPI) {
	// 0. 会话边界重置（pi 生命周期契约：session_shutdown 清理 / session_start 重建）。
	//    只对真实会话边界（new/resume/fork/reload）重置：/new、/resume、fork 会重绑
	//    extension，但 ESM 模块缓存可能让模块级状态跨会话残留（JIT 命中/关键词命中/
	//    档位/输入窗泄漏进下一个 research 会话）→ 显式清零。
	//    ⚠ reason "startup" 必须跳过（@bugfix）：pi-subagents 派发子线程时
	//    createAgentSession 默认也发 session_start{reason:"startup"}——不过滤会把主会话
	//    档位/JIT/命中清零（实测：apply 中派子线程后主线程注入掉回未激活）。子线程
	//    拿到完整约束块是期望行为（快照注入能带上档位基座块与声明域）。
	//    ⚠ 2026-09-17 实测修正：子线程是**独立 node 进程**（pi-subagents/subagent-runner.ts），
	//    父子不共享模块实例 —— “不重置”只保住了主会话，子线程**拿不到**父会话内存态；
	//    因此子线程走显式继承（内存路径 → 事实库回退，见 inheritFromParent[History]）。
	pi.on("session_start", async (event, ctx) => {
		const reason = (event as { reason?: string }).reason ?? "";
		// cfg 仅用于解析会话条目上限（拿不到就退回默认，不影响注入语义）
		const state = stateFor(ctx, ctx?.cwd ? loadConfig(ctx.cwd) : null);
		// startup 双面孔（真实链路实测 6.5）：① pi 进程冷启动恢复会话——quit 重启后
		// reason 是 startup 而非 resume；② pi-subagents 派发子线程 / pi /fork——独立进程、
		// 各自 sessionId，session header（或 forks/ 路径）带 parentSession。
		// per-session-constraint-binding：本会话条目为空时按「父子可证 → 继承父会话（内存 →
		// 事实库）→ 否则按同 sessionId 第 1 段恢复 → 都没有则保持未激活」处理；
		// MUST NOT 借用他会话的状态（隔离前的 bug：全局值被他会话改写后本会话被动继承）。
		if (reason === "startup") {
			if (state.mode === null) {
				// 子线程/fork 继承：内存路径（同进程）→ 事实库回退（跨进程，真实链路主路径）
				const inherited =
					inheritFromParent(state, ctx) || inheritFromParentHistory(state, ctx);
				if (inherited) {
					if (state.mode) {
						logModeSet(ctx.cwd, realSessionId(state), state.mode, state.boundChange, "inherit");
					}
					return;
				}
				const sessionId = realSessionId(state);
				const rows = sessionId
					? queryBySession(ctx.cwd, sessionId, ["mode.set"])
					: [];
				if (rows.length) {
					const rec = recoverFromRow(ctx.cwd, rows[rows.length - 1]);
					if (rec) {
						state.mode = rec.mode;
						state.boundChange = rec.boundChange;
						// 恢复路径显式记账（D6）：隐性绑定不存在；source=recover
						logModeSet(ctx.cwd, sessionId, rec.mode, rec.boundChange, "recover");
					}
				}
			}
			return;
		}
		if (!["new", "resume", "fork", "reload"].includes(reason)) return;
		// 会话边界重置**只清本会话条目**（他会话的档位/命中集 MUST NOT 被清零——
		// 隔离前这里清的是全局，等于替所有会话重置）。
		resetSessionState(state);
		// fork：父子可证 → 继承父会话（同一份对话继续干同一件事）；不可证 → 未激活。
		// （旧行为是 fork 一律清零，靠全局值偶然继承，与「无隐性绑定」相悖。）
		// 继承两条路：内存（同进程）→ 事实库回退（跨进程，真实链路主路径）。
		if (reason === "fork") {
			const inherited =
				inheritFromParent(state, ctx) || inheritFromParentHistory(state, ctx);
			if (inherited && state.mode) {
				logModeSet(ctx.cwd, realSessionId(state), state.mode, state.boundChange, "inherit");
			}
			return;
		}
		// resume/reload 档位恢复（fix-mode-recovery-cross-session：仅同 sessionId 单段）：
		// resume = /resume 切换目标会话（id 即目标会话 id，必中），reload = 同会话扩展
		// 重载（id 不变，必中）；无本会话记录不恢复（不继承他窗口档位）。粘性命中集合
		// 不恢复（有意为之——域声明
		// 节每回合重解析照常注入、JIT 改代码即重新命中，且避免恢复出与「只增不减」历史
		// 不一致的半状态）。new/fork 维持清零语义（不恢复）。
		if ((reason === "resume" || reason === "reload") && ctx?.cwd) {
			const sessionId = realSessionId(state);
			const rec = sessionId ? recoverMode(ctx.cwd, sessionId) : null;
			if (rec) {
				state.mode = rec.mode;
				state.boundChange = rec.boundChange;
				// 恢复路径显式记账（D6）：source=recover
				logModeSet(ctx.cwd, sessionId, rec.mode, rec.boundChange, "recover");
			}
		}
	});
	// 1. 阶段识别（不阻断，仅设 mode flag）
	pi.on("input", async (event, ctx) => {
		const cfg = loadConfig(ctx.cwd);
		if (!cfg) return;
		const state = stateFor(ctx, cfg);
		// 最近 N 条用户输入入窗（无论是否命令），参与关键词/栈命中（入窗限本会话）
		const text = event.text.trim();
		if (text) {
			state.recentInputs.push(text.length > 500 ? text.slice(0, 500) : text);
			const max = cfg.recentInputWindow ?? 5;
			if (state.recentInputs.length > max) {
				state.recentInputs = state.recentInputs.slice(-max);
			}
		}
		const mode = matchCommand(event.text, cfg);
		if (mode) {
			state.mode = mode;
			// 档位绑定：仅明确提及（命令参数/输入文本）
			// mtime 兜底仅 implementation 档（/opsx-apply 无参语境）；requirements 档
			// （explore/propose）是新想法语境，兜底会绑到无关 change，explore 内容错位
			const mentioned = detectMentionedChange(ctx.cwd, event.text)?.name ?? null;
			state.boundChange =
				mentioned ??
				(mode === "implementation"
					? (detectActiveChange(ctx.cwd)?.name ?? null)
					: null);
			// mode.set 记账（tier-b D5 触发点①：input 命令命中；source=command）
			logModeSet(
				ctx.cwd,
				realSessionId(state),
				mode,
				state.boundChange,
				"command",
			);
			state.turnBindLocked = true;
		}
	});

	// 1.5 skill 文件读取即激活（agent 自动 read skill 是斜杠命令外的真实触发源）；
	//     档位已激活时，写 openspec/changes/<name>/ 下文件修正绑定（propose 先建 change 后写产物 / apply 勾选 tasks 的场景）
	pi.on("tool_execution_start", async (event, ctx) => {
		const cfg = loadConfig(ctx.cwd);
		if (!cfg) return;
		if (
			event.toolName !== "read" &&
			event.toolName !== "write" &&
			event.toolName !== "edit"
		)
			return;
		// 注意：ToolExecutionStartEvent 的参数字段是 args（不是 input）
		const p = (event.args as { path?: string } | undefined)?.path ?? "";
		if (!p) return;
		const state = stateFor(ctx, cfg);
		const sessionId = realSessionId(state);
		const skillMode = matchSkillPath(p, cfg);
		if (skillMode) {
			state.mode = skillMode;
			// 同 input 事件：mtime 兜底仅 implementation 档（agent 读 apply/verify/archive
			// skill 就是要干活）；requirements skill（explore/propose）仅明确提及才绑定
			const mentioned =
				detectMentionedChange(ctx.cwd, state.recentInputs.join("\n"))?.name ?? null;
			state.boundChange =
				mentioned ??
				(skillMode === "implementation"
					? (detectActiveChange(ctx.cwd)?.name ?? null)
					: null);
			// mode.set 记账（tier-b D5 触发点②：skill 路径命中；source=skill）
			logModeSet(ctx.cwd, sessionId, skillMode, state.boundChange, "skill");
			state.turnBindLocked = true;
			return;
		}
		// JIT 栈细化（design D3 单一真相源）：写/编辑路径命中文档头部 doc-impact-applies
		// 标签的 pathSignals → 对应文档会话内追加注入。仅档激活时生效：未激活会话写代码
		// 不追加（保「未激活=仅索引」不变量，模型按需自读）
		let jitAdded = false;
		if (state.mode && (event.toolName === "write" || event.toolName === "edit")) {
			for (const [doc, tag] of scanAppliesTags(ctx.cwd)) {
				if (tag.signals.some((s) => p.includes(s))) {
					if (!state.jitDocHits.has(doc)) {
						state.jitDocHits.set(doc, { doc, section: tag.section ?? undefined });
						jitAdded = true;
					}
				}
			}
		}
		// 绑定修正条件化 + turn 锁定（D5，harden-constraint-injection-channel）：
		// read 永不抢绑；当前绑定健康时不抢绑；仅未绑定/绑定 change 目录消失时兜底绑定。
		// per-session-constraint-binding：判定读的是**本会话**的绑定与 turn 锁
		// （他会话的工具调用不得锁住/改写本会话的绑定时机）。
		if (state.mode) {
			const bindAction = resolveBindAction({
				tool: event.toolName,
				path: p,
				currentBound: state.boundChange,
				boundExists:
					state.boundChange != null &&
					existsSync(join(ctx.cwd, OPENSPEC_CHANGES_DIR, state.boundChange)),
			});
			if (shouldApplyBind(bindAction, state.turnBindLocked)) {
				const name = changeNameFromPath(p);
				if (name) {
					state.boundChange = name;
					state.turnBindLocked = true;
					// mode.set 记账（tier-b D5 触发点③：写 change 目录兜底绑定；source=edit-dir）
					logModeSet(ctx.cwd, sessionId, state.mode, state.boundChange, "edit-dir");
				}
			}
		}
		// JIT 命中即时投递（D3 turn 中途通道）：新命中 → 立即算增量并经 steer 送出
		if (jitAdded && state.mode && cfg.channel !== "legacy") {
			const plan = planInjection(
				ctx.cwd,
				cfg,
				state,
				detectActiveChange(ctx.cwd),
			);
			const { dynamic } = splitPlanItems(plan.items);
			deliverDynamic(
				pi,
				ctx,
				cfg,
				state,
				plan.changeName,
				toDynamicItems(dynamic),
				dynamicLabelWithDegrad("📎 约束补充（JIT 命中）", plan, cfg),
			);
		}
	});

	// 2. system prompt 注入（每 turn 重建 → 幂等；前缀缓存 → 便宜；抗 compaction）
	// 2. 约束注入（混合通道 D1~D4）：稳定层进 system prompt（档位生命周期内字节恒定）；
	//    动态层走追加消息（指纹 diff，零变化零投递）。channel="legacy" 回退旧全量行为。
	pi.on("before_agent_start", async (event, ctx) => {
		const cfg = loadConfig(ctx.cwd);
		if (!cfg) return;
		const state = stateFor(ctx, cfg);
		// turn 起点释放绑定锁（D5：同 turn 锁定、跳 turn 再可切）——只解本会话的锁
		state.turnBindLocked = false;
		const activeChange = detectActiveChange(ctx.cwd);
		// 档位回落：仅当绑定的 change 已不存在（归档/删除）→ 回落未激活档；
		// 不因其他 change 目录 mtime 更新而误掉档（多 change 并行是常态）
		if (
			state.mode &&
			state.boundChange &&
			!existsSync(join(ctx.cwd, OPENSPEC_CHANGES_DIR, state.boundChange))
		) {
			state.mode = null;
			state.boundChange = null;
		}
		const plan = planInjection(ctx.cwd, cfg, state, activeChange);
		const sessionId = realSessionId(state);
		// mtime 兜底显式化（D6）：implementation 档无显式绑定时，将 planInjection 内部的
		// mtime 兜底升为显式绑定并记账（source=fallback）——消除「注入挂在 X 名下、
		// boundChange 仍为空」的账实不符与静默漂移（多 change 并行污染根源之一）
		if (
			state.mode === "implementation" &&
			state.boundChange === null &&
			plan.changeName &&
			existsSync(join(ctx.cwd, OPENSPEC_CHANGES_DIR, plan.changeName))
		) {
			state.boundChange = plan.changeName;
			logModeSet(ctx.cwd, sessionId, state.mode, state.boundChange, "fallback");
		}

		// 状态展示（始终，让用户看到注入了什么 + 预算用量）
		if (ctx.hasUI) {
			const parts: string[] = [`[约束] ${plan.modeLabel}`];
			if (plan.changeName) parts.push(plan.changeName);
			if (plan.declNote) parts.push(plan.declNote);
			if (plan.budgetNote) parts.push(plan.budgetNote);
			if (plan.docList.length) parts.push(plan.docList.join("|"));
			else parts.push("无命中");
			if (cfg.channel !== "legacy") parts.push("通道:split");
			ctx.ui.setWidget?.("constraint-injection", [parts.join(" | ")]);
		}

		// —— legacy 回退：全量进 system prompt + 每 turn 记账（应急开关，D8）——
		if (cfg.channel === "legacy") {
			if (sessionId) {
				logConstraintInjects(ctx, sessionId, plan.changeName, plan.docEntries);
				for (const p of plan.pinReads) {
					// 去重集已按会话隔离 → key 用标题即可（不再拼 sessionId）
					if (state.pinReadSeen.has(p.title)) continue;
					state.pinReadSeen.add(p.title);
					logEvent(ctx.cwd, {
						kind: "pin.read",
						sessionId,
						change: plan.changeName,
						payload: { ...p },
					});
				}
			}
			if (plan.block) return { systemPrompt: event.systemPrompt + plan.block };
			return;
		}

		// —— 混合通道（split）——
		// 快照/指纹都在本会话的 channel 里：他会话绑定变化不会刷新本会话快照
		const { stable, dynamic } = splitPlanItems(plan.items);
		const key = stableSnapshotKey(state.mode, state.boundChange);
		// 无稳定条目但有内容时仍出 header（档位/活跃变更/域声明对模型可见）
		const wantBlock = plan.items.length > 0;
		let stableBlock = "";
		let extraDynamic: PlanItem[] = [];
		if (
			wantBlock &&
			(!state.channel.stableSnapshot || state.channel.stableSnapshot.key !== key)
		) {
			// 档位/绑定切换（快照已存在但 key 变）→ 重置动态指纹（旧 change 的命中无关）；
			// 首次建快照（如 JIT 已在快照前投递过）不重置，避免重复投递
			const isTransition =
				state.channel.stableSnapshot !== null &&
				state.channel.stableSnapshot.key !== key;
			stableBlock = renderStableBlock(buildStableHeader(plan, stable), stable);
			state.channel.stableSnapshot = { key, block: stableBlock, items: stable };
			if (isTransition) state.channel.sentFingerprint = new Map();
			logConstraintInjects(
				ctx,
				sessionId,
				plan.changeName,
				stable.map(itemToDocEntry),
			);
		} else {
			// 快照冻结（字节恒定）；稳定层内容后来的变化（如 proposal 新增声明域）
			// 作为动态层差异条目送达，不改写 system prompt（D2）
			stableBlock = state.channel.stableSnapshot
				? state.channel.stableSnapshot.block
				: "";
			extraDynamic = state.channel.stableSnapshot
				? diffStableItems(state.channel.stableSnapshot.items, stable)
				: [];
		}

		// pin.read 记账（explore-findings 经动态层送达；会话内去重）
		if (sessionId && plan.pinReads.length) {
			const dynamicKeys = new Set([...dynamic, ...extraDynamic].map((it) => it.key));
			for (const p of plan.pinReads) {
				if (!dynamicKeys.has(p.doc)) continue;
				if (state.pinReadSeen.has(p.title)) continue;
				state.pinReadSeen.add(p.title);
				logEvent(ctx.cwd, {
					kind: "pin.read",
					sessionId,
					change: plan.changeName,
					payload: { ...p },
				});
			}
		}

		// 动态层投递：compact 后先重发快照（无视指纹），否则常规 diff 增量
		const dynamicItems = toDynamicItems([...dynamic, ...extraDynamic]);
		let delivered = 0;
		if (state.channel.pendingSnapshot) {
			state.channel.pendingSnapshot = false;
			delivered = deliverDynamic(
				pi,
				ctx,
				cfg,
				state,
				plan.changeName,
				dynamicItems,
				"📎 约束快照（compact 后重发）",
				true,
			);
		} else {
			const label = dynamicLabelWithDegrad(
				extraDynamic.length ? "📎 约束补充（稳定层差异）" : "📎 约束补充（增量）",
				plan,
				cfg,
			);
			delivered = deliverDynamic(
				pi,
				ctx,
				cfg,
				state,
				plan.changeName,
				dynamicItems,
				label,
			);
		}
		void delivered;

		if (stableBlock) return { systemPrompt: event.systemPrompt + stableBlock };
	});

	// 2.5 compaction 补偿（D4）：消息会被摘要，下一注入时机重发一次约束快照
	pi.on("session_compact", async (_event, ctx) => {
		const cfg = loadConfig(ctx.cwd);
		if (!cfg || cfg.channel === "legacy") return;
		// 只置本会话的待重发标记（他会话的 compact 不得触发本会话重发）
		stateFor(ctx, cfg).channel.pendingSnapshot = true;
	});

	// 3. pin_finding 工具（探索阶段持久化关键代码发现）
	pi.registerTool({
		name: "pin_finding",
		label: "Pin Explore Finding",
		description:
			"把关键代码事实（表结构/调用链/集成点/数据流/边界枚举/关键算法）持久化到活跃 change 的 explore-findings.md（档激活时实现/评审阶段自动注入 system prompt，避免重复探索）；" +
			"research/问答语境（无激活档位）则落 docs/research/<topic>/（未传 topic 落通用池单文件），不碰 openspec/changes/。",
		promptSnippet: "持久化探索阶段的关键代码发现，供实现阶段自动读取",
		promptGuidelines: [
			"Use pin_finding when exploring requirements (explore/propose stage) and you discover a key code fact worth persisting for the implementation stage — e.g. table schema, call chain, integration point, data flow, boundary enum, or core algorithm.",
		],
		parameters: Type.Object({
			title: Type.String({
				description: "短标题，如「告警表结构」「订阅源发现调用链」",
			}),
			finding: Type.String({
				description:
					"代码事实正文。写清定位/字段/流向/边界，实现阶段据此直接动手而无需重探。",
			}),
			fileRefs: Type.Optional(
				Type.Array(Type.String(), {
					description: "相关 file:symbol 引用列表（可选）",
				}),
			),
			change: Type.Optional(
				Type.String({
					description: "目标 change 名（覆盖自动检测的活跃 change，可选）",
				}),
			),
			topic: Type.Optional(
				Type.String({
					description:
						"research 语境（无激活档位且未传 change）时的主题目录名，落 docs/research/<topic>/explore-findings.md；省略则落通用池单文件 docs/research/explore-findings.md",
				}),
			),
		}),
		async execute(_toolCallId, params, _signal, _onUpdate, ctx) {
			const cfg = loadConfig(ctx.cwd);
			if (!cfg) {
				return {
					content: [
						{
							type: "text",
							text: "⚠️ 未找到 .pi/constraint-injection.json，pin_finding 不可用。",
						},
					],
				};
			}
			// 目标解析（防 research 落错库，@2026-08-20；@Syntopica 修订）：
			//   ① 显式 change 参数（目录存在即用，无论档位）
			//   ② 档激活：modeBoundChange → implementation 档 mtime 兜底（apply 语境落点合理）；
			//      requirements 档（explore 新想法）不兜底，无绑定 → research 库
			//   ③ 无档（research 语境）：research 库 docs/research/[topic/]，
			//      不再 mtime 兜底抓无关 change（08-19 源项目 pin 落进无关 change 的教训）
			const changes = listChangeDirs(ctx.cwd);
			const explicit = params.change
				? (changes.find((c) => c.name === params.change) ?? null)
				: null;
			let warn = "";
			if (params.change && !explicit) {
				warn = `⚠️ change「${params.change}」不存在，已改存 research 库。\n\n`;
			}
			let label: string;
			let filePath: string;
			const state = stateFor(ctx, cfg);
			if (explicit) {
				label = `${OPENSPEC_CHANGES_DIR}/${explicit.name}`;
				filePath = join(explicit.dir, cfg.findingsFile);
			} else {
				// 绑定读**本会话**的 state（per-session-constraint-binding）：他会话的绑定不得
				// 决定本会话 pin 落点（隔离前正是全局值造成的落错库）
				const bound =
					state.mode && state.boundChange
						? (changes.find((c) => c.name === state.boundChange) ?? null)
						: null;
				const active =
					bound ??
					(state.mode === "implementation"
						? detectActiveChange(ctx.cwd)
						: null);
				if (active) {
					label = `${OPENSPEC_CHANGES_DIR}/${active.name}`;
					filePath = join(active.dir, cfg.findingsFile);
				} else {
					const topic = String(params.topic ?? "")
						.trim()
						.replace(/[^\w\u4e00-\u9fff.-]/g, "")
						.slice(0, 60);
					label = topic
						? `${RESEARCH_DIR}/${topic}/${cfg.findingsFile}`
						: `${RESEARCH_DIR}/${cfg.findingsFile}`;
					filePath = join(ctx.cwd, label);
				}
			}
			const ts = new Date().toISOString().replace(/\.\d+Z$/, "Z");
			const refs =
				params.fileRefs && params.fileRefs.length
					? `\n\n**引用**：${params.fileRefs.join("、")}`
					: "";
			const intoChange = label.startsWith(`${OPENSPEC_CHANGES_DIR}/`);
			// research 语境锚点（design D5）：标题行尾 ` <!-- pin:<8hex> -->` 作为持久身份，
			// 为二期 pin.touch 预留；change 语境不加（(change, title) 复合键已够）。
			// pin.read 解析时剥除，digestFindings 的标题整行保留无妨
			const anchor = intoChange ? "" : ` <!-- pin:${hex8()} -->`;
			const section = `\n## ${params.title}${anchor}\n\n${params.finding.trim()}${refs}\n\n<!-- pinned ${ts} -->\n`;
			try {
				mkdirSync(dirname(filePath), { recursive: true });
				writeFileSync(filePath, section, { flag: "a" });
			} catch (e) {
				return {
					content: [
						{ type: "text", text: `⚠️ 写入失败：${(e as Error).message}` },
					],
				};
			}
			// pin.write 记账（design D5）：仅成功路径；research 语境 change 列为 null
			const sessionId = realSessionId(state);
			if (sessionId) {
				logEvent(ctx.cwd, {
					kind: "pin.write",
					sessionId,
					change: intoChange
						? label.slice(`${OPENSPEC_CHANGES_DIR}/`.length)
						: null,
					payload: {
						title: params.title,
						topic: params.topic ?? null,
						path: label,
					},
				});
			}
			return {
				content: [
					{
						type: "text",
						text: intoChange
							? `${warn}✅ 已存档到 ${label} → 「${params.title}」。实现/评审阶段将自动注入。`
							: `${warn}✅ 已存档到 ${label} → 「${params.title}」（research 库：当前会话无激活档位）。如需归入某个 change，重试时传 change: "<change名>"。`,
					},
				],
			};
		},
	});
}
