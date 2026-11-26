/**
 * constraint-injection — 阶段驱动的约束路由 + 自主导入（Syntopica 版）
 *
 * 源自用户另一项目实战 extension（快照：docs/research/extensions/extensions/constraint-injection.ts，
 * 移植 change：openspec/changes/port-constraint-injection，设计决策见其 design.md）。
 * 配置文件：.pi/constraint-injection.json（命令表/关键词表/栈信号）。
 *
 * 形态（stage-driven-constraint-injection D1~D8，本文件为任务组 3 的落地）：
 * - **进度真相源 = change 目录内 `stage.md` 的 `<!-- stage: -->` marker**（6 档：
 *   explore/propose/design/implement/verify/archive）；阶段族（planning/implementation）
 *   是**派生值**（lib/stage.ts），MUST NOT 独立写。marker 缺失/非法 = legacy change，
 *   按未绑定处理（不阻断任何流程，含归档）。
 * - **注入形态 = 红线层 + 路由清单**：业务域约束节的**整节正文不再主动注入**；
 *   ① 阶段首推（增量 steer 消息）送达该 change 声明域的**红线层**（顶层加粗句逐行，
 *   复用 lib/constraint-redlines.ts 的 extractRedlines，零提取/低于 minSectionBytes
 *   回退全节）；② 稳定层（system prompt）= **索引文档 + 阶段路由清单**（只有路径 +
 *   节名 + 取数方式，**零正文**）；③ 细节层由模型**自主 read**（清单给出
 *   `constraint-digest.sh --brief --domains <域>` 取红线摘要的命令形式）。
 * - **业务域声明两处并集**：proposal.md 头部 `<!-- constraint-domains: ... -->`（存量位置）
 *   + **stage.md 头部同名 marker**（实现期活口径——申报实施中新发现的域）；stage.md 不得
 *   收窄 proposal 的声明（超集安全方向）。生效域 → 红线层 + 路由清单。
 * - **两条整节正文通道退役**：keyword（对话关键词命中）与 jit-path（编辑路径命中）
 *   均不再送达正文——命中的文档只**进路由清单**（路径 + 节名），引导模型自行 read。
 *   证据：改造前 jit-path 单次最坏 27.8KB、keyword 跨域误伤 8KB（见 change proposal）。
 * - **稳定层快照键 = 阶段族 + 绑定 change**（`stageFamily|boundChange`）：同族内阶段
 *   切换（explore→propose→design）不重建稳定层（族内切换由稳定层差异机制经 steer
 *   送达）；族切换才整体重建一次（前缀缓存零失效的红线）。
 * - **事实库**：`stage.set` 取代 `mode.set`（payload：stage/stageFamily/boundChange/
 *   source；source 沿用 command/skill/edit-dir/recover/inherit/fallback 六来源）；
 *   新增 `constraint.read`（读约束文档时记账，归档对账的唯一事实源）。
 *
 * 职责：
 * 0. **会话作用域状态（per-session-constraint-binding）**：阶段/阶段族/绑定 change/
 *    turn 绑定锁/输入窗/命中集/pin 去重集/稳定层快照与指纹全部按 `sessionId` 隔离
 *    （`sessionStates`）——一个 pi 进程可同时有多会话（多窗口 + pi-subagents 子线程
 *    共用模块实例），隔离前它们共享同一套全局变量，导致「他会话刚绑定 = 本会话注入
 *    归属」（2026-09-17 事实库取证）。不变量：
 *    - 注入归属 / 稳定层快照 / 命中集只由**本会话**状态决定；
 *    - 本会话无状态时，仅当父子关系可证（session header 的 `parentSession`，即 fork /
 *      pi-subagents 子线程）才显式继承父会话（记 `stage.set source=inherit`），
 *      否则视为未绑定（仅索引）；**MUST NOT 借用任意他会话的状态**；
 *    - 会话边界事件（new/resume/fork/reload）只重置**本会话**条目；
 *    - 会话条目有界（上限 + LRU 淘汰），无 `sessionId` 的 stub 语境落单一兜底槽。
 *    回归面：`.pi/extensions/tests/constraint-injection.smoke.cjs` 第 19 组。
 * 1. input 事件：输入提及 change 名 → 绑定该 change（阶段由其 stage.md 决定）
 * 2. before_agent_start 事件：约束注入按**混合通道**送达：
 *    - **稳定层**（system prompt）：索引文档 + 阶段路由清单。快照
 *      key=stageFamily|boundChange，**同族阶段生命周期内字节恒定** → system prompt
 *      不变则其后全部 history 前缀缓存不失效。
 *    - **动态层**（追加消息，append-only 投递）：阶段首推的声明域红线层 + change 级
 *      文件（explore-findings / 词汇表）+ 命中文档的路由指针 + 稳定层差异通知。按内容
 *      指纹 diff 发送——指纹未变零投递，变化只发增量条目。
 *    - compaction 后（session_compact）下一注入时机重发一次约束快照（消息会被摘要掉）。
 *    - 配置 channel="legacy" 可回退「每 turn 全量进 system prompt」行为。
 *    未绑定阶段（无 stage.md / 未绑定 change）：**稳定层**仅注入约束索引
 *    （docs/reference/constraints-index.md）。
 * 3. pin_finding 工具：探索阶段持久化关键代码发现到活跃 change 的 explore-findings.md；
 *    research/问答语境（未绑定阶段）落 docs/research/<topic>/（无 topic 落通用池
 *    单文件 docs/research/explore-findings.md），不碰 openspec/changes/ 与 experience
 * 4. 事实库记账（harness-facts-tier-a）：constraint.inject（送达时记）/ constraint.read
 *    （读约束文档时记）/ stage.set（阶段与绑定变化记）/ pin.write / pin.read 经
 *    lib/harness-log 自报，模型零参与
 * 5. 注入预算与分层降级（constraint-injection-tier-b）：注入内容总量受 budgetBytes
 *    限制（缺省 32768；非正数禁用），超线按信号强度分层降级（红线层 → 路由清单 →
 *    change-file digest 收紧 → change-file 占位），永不真丢（占位行保留标题 + read 路径）
 * 6. 阶段持久化与绑定归因：阶段/绑定**所有**变化路径记 stage.set（payload 附 source：
 *    command/skill/edit-dir/recover/inherit/fallback，隐性绑定不存在）；绑定修正条件化
 *    （read 不抢绑、当前绑定健康时不抢绑、仅无绑定/绑定 change 消失时兜底）且同 turn 锁定；
 *    session_start{reason:resume} 按同 sessionId 恢复（`queryLatestStageSet` 双读：
 *    先 stage.set，无则回退 legacy mode.set 并映射回族），new/fork 清零
 * 7. 跨进程子线程继承（3.6）：读**父会话**最新 stage.set 取 stage/stageFamily/
 *    boundChange，**stage 随父会话快照写死，不在子线程重算**
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
import {
	logEvent,
	logStageSet,
	queryLatestStageSet,
	logConstraintRead,
	type HarnessEventRow,
	type StageSetInfo,
	type StageSetSource,
} from "./lib/harness-log";
import { isChildSession, parentSessionId } from "./lib/child-session";
import { extractRedlines } from "./lib/constraint-redlines";
import {
	readStageFile,
	stageFamilyOf,
	buildStageRouting,
	normalizeRepoPath,
	isConstraintDocPath,
	type Stage,
	type StageFamily,
	type StageRoutingItem,
} from "./lib/stage";
import {
	listChangeDirs,
	detectActiveChange,
	OPENSPEC_CHANGES_DIR,
	ARCHIVE_DIR,
} from "./lib/active-change";

interface ConditionalDoc {
	doc: string;
	ifStack: "backend" | "frontend";
}
/**
 * legacy 档位配置（stage-driven 改造后**仅用于工作流命令识别**——命令不再决定阶段，阶段恒由绑定 change 的 stage.md 决定）。
 * group 6 已把仓库配置改为 `stages` 段；本接口只为 legacy 配置形态（临时目录 / 烟测 fixture）保留读取兼容。
 */
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
	/** legacy 档位配置（可选：group 6 改配置后本段可能缺席——读取处一律容错） */
	modes?: Record<string, ModeConfig>;
	/** 阶段配置段（group 6 落地：`stages` 路由表 + 各阶段命令表；读取 commands 与 docs） */
	stages?: Record<string, {
		label?: string;
		commands?: string[];
		/**
		 * 本阶段必读文档集合（路由清单数据源；**只写路径**——节名由
		 * lib/stage.ts 的 ROUTING_DOC_CATALOG 唯一维护）：required 无条件进清单，
		 * conditional 命中栈才进清单。缺省本段 = 代码兜底（临时目录 / 配置未升级）。
		 */
		docs?: {
			required?: string[];
			conditional?: Partial<Record<"backend" | "frontend", string[]>>;
		};
	}>;
	keywordDocs: (DocEntry & { keywords: string[] })[];
	stackSignals: { frontend: string[]; backend: string[] };
	// skill 文件路径片段（agent 自动 read openspec skill 时触发绑定，覆盖斜杠命令外的真实工作流）
	skillSignals?: { requirements: string[]; implementation: string[] };
	indexDoc: string;
	findingsFile: string;
	findingsDigestThreshold: number;
	// change 级词汇表文件名（默认 ubiquitous-language.md）；实现档注入，主线程定义→子线程可见
	vocabularyFile?: string;
	// 最近 N 条用户输入参与关键词/栈判定（默认 5）
	recentInputWindow?: number;
	// 会话状态条目上限（默认 32）：仅用于长跑进程内存有界（LRU 淘汰最久未用会话条目），
	// 不影响注入语义（会话数未超限时行为与上限无关）；非法值（非数字/0/负数）回退默认。
	sessionStateLimit?: number;
	// 二级注入阈值（字节，默认 6144）：超过且有「## 硬规则速览」小节 → 仅注速览+全文目录
	digestDocThreshold?: number;
	// 节级注入最小字节下限（默认 512）：提取的节内容低于此值视为文档编辑中间态残缺节，
	// 回退注入全文（fail-safe，与节不存在回落全文同族）。harness-observability-fixes D4。
	minSectionBytes?: number;
	// JIT pathSignals 单一真相源 = 文档头部 `doc-impact-applies`
	// frontmatter 标签（docs/reference/flow + standard），见 scanAppliesTags（design D3）
	// ⚠ 改造后编辑路径命中**不再注入正文**——命中文档只进路由清单（路径 + 节名）。
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
// 预算缺省值（tier-b design D1）：实测重估——16K 会在常态上界触发降级，
// 且让烟测 REPO 场景落入边际降级区（断言随 flow 文档演进漂移）；预算管尾部不管常态
const DEFAULT_BUDGET_BYTES = 32768;
// research 库（未绑定阶段语境的 pin 落点；规则出处：openspec/specs/research-retention）
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
	/** 稳定层快照（D2/D3）：key=`stageFamily|boundChange`，同族阶段生命周期内冻结字节；内容差异走动态层 */
	stableSnapshot: { key: string; block: string; items: PlanItem[] } | null;
	/** 动态层已投递指纹（D3）：itemKey → contentHash；指纹未变则零投递 */
	sentFingerprint: Map<string, string>;
	/** 已投递指纹的 regime（fix-injection-transition-fingerprint-wipe）：sentFingerprint 各条目
	 *  投递时的 stableSnapshotKey(stageFamily, boundChange)；null = 尚无动态投递。阶段/绑定可
	 *  mid-turn 变化而快照重建延迟到下一 turn 起点——延迟重建的指纹清空只看 regime：
	 *  regime≠新快照 key 才清空，mid-turn 切阶段后同 turn 已投递的内容不被误清重投。 */
	fingerprintRegime: string | null;
	/** 已投递的阶段（阶段首推判据）：某 stage 首次送达 → 消息标「阶段首推」 */
	lastDeliveredStage: Stage | null;
	/** compaction 后待重发快照（D4）：session_compact 置位，下一注入时机重发并重置指纹 */
	pendingSnapshot: boolean;
	/** 子线程 session_start 已投递稳定层（harden-subagent-constraint-channel T3-rev/D3）：
	 *  真时 before_agent_start 冻结快照/稳定层记账且不返回 systemPrompt 块（防双投——
	 *  消息与 system prompt 二选一，稳定层已由消息通道承载）。仅子线程置位。 */
	childInitDelivered: boolean;
};
type SessionState = {
	sessionId: string;
	/** 本会话阶段（6 档；由绑定 change 的 stage.md marker 派生；null = 未绑定阶段/legacy） */
	stage: Stage | null;
	/** 阶段族（**派生值**：planning/implementation；legacy 恢复时可由 mode.set 映射回族） */
	stageFamily: StageFamily | null;
	/** 设置阶段时绑定的活跃 change 名；绑定 change 归档/删除后回落 null（防粘性常驻） */
	boundChange: string | null;
	/** turn 绑定锁定（D5）：before_agent_start 重置；turn 内首个生效绑定后置位，同 turn 不再切换 */
	turnBindLocked: boolean;
	/** 工作流意图（本会话见过 openspec 工作流命令/skill）：**mtime 兜底绑定的唯一前提**
	 *  （无意图时不得凭空把「最近改动的 change」当成当前工作对象——改造前对应「档位未激活」）。
	 *  ⚠ 意图**不决定阶段**：阶段恒由绑定 change 的 stage.md 派生；意图只回答「是否允许
	 *  mtime 兜底」（planning 意图 = 新想法语境，不兜底——同改造前 requirements 档规则）。 */
	workflowIntent: "planning" | "implementation" | null;
	/** 最近 N 条用户输入（ring buffer），与 change 文本合并后做关键词/栈命中 */
	recentInputs: string[];
	/** JIT 命中集合（会话内只增不减）：write/edit 路径命中 doc-impact-applies 标签即入列。
	 *  改造后**只进路由清单**（路径 + 节名），不再送达整节正文。 */
	jitDocHits: Map<string, DocEntry>;
	/** 关键词命中集合（会话内只增不减，@Syntopica）：命中源含滚动输入窗，词条滚出窗口
	 *  不移除——注入块缩水会让 system prompt 字节变化 → 全部历史前缀缓存报废。
	 *  改造后只作路由清单的「本次会话提及」标注，不送达正文。 */
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
		stage: null,
		stageFamily: null,
		boundChange: null,
		turnBindLocked: false,
		workflowIntent: null,
		recentInputs: [],
		jitDocHits: new Map(),
		keywordDocHits: new Map(),
		pinReadSeen: new Set(),
		channel: {
			stableSnapshot: null,
			sentFingerprint: new Map(),
			fingerprintRegime: null,
			pendingSnapshot: false,
			childInitDelivered: false,
		},
		lastDeliveredStage: null,
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

/** 烟测观测面（T3-rev 子线程投递通道）：指定会话的通道投递凭据（稳定层快照 key +
 *  childInitDelivered），仅供测试断言，不参与注入逻辑；会话不存在返回 null。 */
export function channelStateForTest(
	sessionId: string,
): { stableSnapshotKey: string | null; childInitDelivered: boolean } | null {
	const st = sessionStates.get(sessionId);
	if (!st) return null;
	return {
		stableSnapshotKey: st.channel.stableSnapshot?.key ?? null,
		childInitDelivered: st.channel.childInitDelivered,
	};
}

/* ---------- 父子关系判定（harden-subagent-constraint-channel T3 抽出共享）----------
 *  sessionIdFromSessionFile / parentSessionIdFromForkFile / parentSessionId 三函数原样
 *  移至 lib/child-session.ts（quality-gate 子线程降载同源复用，判定证据与语义不变，
 *  见该模块头部注释）；此处经 import 的 parentSessionId 供 inheritFromParent[History] 取父 id。 */

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
	state.stage = parent.stage;
	state.stageFamily = parent.stageFamily;
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
		fingerprintRegime: parent.channel.fingerprintRegime,
		pendingSnapshot: parent.channel.pendingSnapshot,
		// 父会话的该标记是父会话自己的投递凭据；子线程必须自行投递（session_start 末尾）
		childInitDelivered: false,
	};
	return true;
}

/** 跨进程继承回退（**真实链路主路径**）：父会话的内存态在另一个进程里拿不到，但父会话的
 *  `stage.set` 历史在共享事实库里 → 按父会话 id 取父会话最近一条**可恢复**记录，只采纳
 *  `stage` + `stageFamily` + `boundChange`（命中集/快照/输入窗是父进程内存态，拿不到就不编造，
 *  宁可少注入）。**stage 值随父会话快照写死，不在子线程重算**（D8）。
 *  取数走 `queryLatestStageSet`（已内置双读：先 stage.set，无则回退 legacy mode.set 并映射回族）。
 *  父 id 不可证、无记录、异常 → false（保持未绑定，**绝不**借用任意他会话状态）。 */
function inheritFromParentHistory(state: SessionState, ctx: ExtCtx | undefined): boolean {
	const pid = parentSessionId(ctx);
	if (!pid || !ctx?.cwd) return false;
	let rec: StageSetInfo | null = null;
	try {
		rec = queryLatestStageSet(pid, ctx.cwd);
	} catch (e) {
		console.error(
			`[constraint-injection] inherit from parent history failed: ${(e as Error).message}`,
		);
		return false;
	}
	if (!rec) return false;
	// 父会话绑定的 change 已归档/删除 → 不继承（回落未绑定）
	if (
		rec.boundChange &&
		!existsSync(join(ctx.cwd, OPENSPEC_CHANGES_DIR, rec.boundChange))
	) {
		return false;
	}
	state.stage = rec.stage;
	state.stageFamily = rec.stageFamily;
	state.boundChange = rec.boundChange;
	return true;
}

/** 会话边界重置（session_start 的 new/resume/fork/reload）：**只清本会话条目**，
 *  他会话状态 MUST NOT 被清零（隔离前是清零全局，等于替所有会话重置）。 */
function resetSessionState(state: SessionState): void {
	state.stage = null;
	state.stageFamily = null;
	state.boundChange = null;
	state.turnBindLocked = false;
	state.workflowIntent = null;
	state.recentInputs = [];
	state.jitDocHits = new Map();
	state.keywordDocHits = new Map();
	state.pinReadSeen = new Set();
	state.channel = {
		stableSnapshot: null,
		sentFingerprint: new Map(),
		fingerprintRegime: null,
		pendingSnapshot: false,
		childInitDelivered: false,
	};
	state.lastDeliveredStage = null;
}

let configCache: { config: ConstraintConfig; mtime: number } | null = null;
// doc-impact-applies 标签扫描缓存（design D3）：文档相对路径 → mtime + 解析结果（无标签为 null）
const appliesTagCache = new Map<
	string,
	{ mtime: number; entry: { signals: string[]; section: string | null } | null }
>();
// 合法业务域缓存（docs/reference/flow/*.md basename）：目录 mtime + 名单
let flowDomainsCache: { mtime: number; names: string[] } | null = null;
// read 调用在途登记（review R-4）：tool_execution_start 记 toolCallId → 原始 path，
// tool_execution_end（成功回执）取回记账后删除；end 事件不带 args，只能经此中转。
const pendingReadCalls = new Map<string, string>();
// 关键词匹配器预编译缓存（ASCII 词边界正则；按 config 实例 WeakMap，热更新自然失效）
const matcherCache = new WeakMap<ConstraintConfig, Map<string, RegExp | null>>();
/* 混合通道状态（harden-constraint-injection-channel D2~D5）与 pin.read 去重集：
 * per-session-constraint-binding 起收进 SessionState.channel / SessionState.pinReadSeen，
 * 不再有模块级全局（旧 resetChannelState 由 resetSessionState 取代）。
 * 指纹按 regime 标记（fix-injection-transition-fingerprint-wipe）：sentFingerprint 写入时
 * 同步记投递时的 stableSnapshotKey 到 fingerprintRegime；快照重建的指纹清空仅当
 * regime≠新快照 key——mid-turn 切档（skill 读取激活/edit-dir 绑定）与延迟快照重建之间
 * 的窗口内已投递条目不再被误清重投（2026-09-17 取证：session 01a0ae99 事件 29644~29652，
 * 同会话代内同节同字节重投 ~19 组，全部为 regime 未变却被无条件清空所致）。 */

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
/* 实现已下沉到 lib/constraint-redlines.ts（单一真相源：extension 与取数脚本
   scripts/harness/constraint-digest.sh 双引，规则一字不改 —— design D2 / R1）。
   此处 re-export 保持对外导出面不变（smoke 由 bundle 调 ext.extractRedlines）。 */
export { extractRedlines, type RedlineLayer } from "./lib/constraint-redlines";

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
	layer?: "redline" | "pointer";
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
	// 改造后顺位：**红线层 → 路由清单 → change-file（digest 收紧 → 占位）**
	// （keyword / jit-path 整节正文通道已退役，不再有降级层）
	const actions: { item: PlanItem; to: "digest" | "placeholder" }[] = [];
	const add = (reason: PlanItem["reason"], to: "digest" | "placeholder") => {
		for (const it of current) {
			if (it.reason !== reason) continue;
			if (to === "digest" && !it.digestAltBody) continue;
			actions.push({ item: it, to });
		}
	};
	add("declaration", "placeholder");
	add("routing", "placeholder");
	add("change-file", "digest");
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

/** constraint.inject 记账条目（design D5）：注入内容的路径/形态/命中原因/字节数；degraded 仅预算降级回合携带（tier-b D4） */
interface InjectedDocEntry {
	path: string;
	mode: "section" | "digest" | "full";
	/**
	 * 注入原因（改造后词汇收敛）：
	 *   index       = 约束索引文档（稳定层）
	 *   routing     = 阶段路由清单（稳定层；只含路径 + 节名，**零正文**）
	 *   declaration = 声明域红线层（动态层，阶段首推）
	 *   change-file = change 级文件（explore-findings / 词汇表）
	 *   route-hit   = 编辑路径/关键词命中文档的**路由指针**（零正文，不单独记账）
	 * 历史词汇（mode-base / stack-conditional / keyword / jit-path）随整节正文通道退役而不再产生。
	 */
	reason:
		| "index"
		| "routing"
		| "declaration"
		| "route-hit"
		| "change-file";
	bytes: number;
	degraded?: boolean;
	/** 声明域注入层级（@constraint-declaration-redline）：redline=红线层 / full=回退全节或全文；其余 reason 不携带 */
	layer?: "redline" | "pointer";
}

/** pin.read 记账条目（design D5）：explore-findings.md 的 pin 标题（research 锚点已剥除） */
interface PinReadEntry {
	title: string;
	doc: string;
	digested: boolean;
}

/**
 * 注入阶段族（append-only 记账字段 tier）：
 * none=未绑定阶段 / planning / implementation（**派生值**，与 InjectedDocEntry.mode
 * ——文档形态 section/digest/full——语义不同，MUST NOT 复用；历史记录无 tier → 统计计
 * 「未知」并剔除分母）。
 */
type InjectionTier = "none" | "planning" | "implementation";

const tierOf = (stageFamily: StageFamily | null): InjectionTier =>
	stageFamily ?? "none";

interface InjectionPlan {
	/** 当前阶段（6 档；null = 未绑定阶段/legacy change） */
	stage: Stage | null;
	/** 当前阶段族（派生值；null = 未绑定） */
	stageFamily: StageFamily | null;
	/** 阶段展示标签（implement / 未绑定） */
	stageLabel: string;
	changeName: string | null;
	/** 绑定是否来自阶段绑定（false = mtime 兜底/无；header 展示 `（阶段绑定）` 后缀用） */
	bound: boolean;
	docList: string[]; // 实际注入的文档（用于状态展示）
	docEntries: InjectedDocEntry[]; // 事实库记账（constraint.inject）
	pinReads: PinReadEntry[]; // 事实库记账（pin.read，注入 explore-findings 成功时）
	// 业务域声明状态（design D2/D5）：注入块头部 + widget 展示；null = 未绑定 change
	declNote: string | null;
	// 预算用量展示（widget；null = 预算禁用或未绑定阶段）：如「预算 12.3K/32K（降级2）」
	budgetNote: string | null;
	// 降级后条目（混合通道按 reason 切分稳定层/动态层；legacy 模式不用）
	items: PlanItem[];
	/** 本回合被预算降级的条目短名（动态层降级通知用；tier-b） */
	degraded: string[];
	block: string; // 追加进 system prompt 的完整文本
}

/* ---------- 阶段路由清单装配（stage-driven D4/D5） ---------- */

/** 栈判定 → 路由清单的栈参数（backend 优先：双栈命中时本仓以 Go 后端为主栈） */
function stackForRouting(stacks: {
	backend: boolean;
	frontend: boolean;
}): "backend" | "frontend" | null {
	if (stacks.frontend && !stacks.backend) return "frontend";
	if (stacks.backend) return "backend";
	return null;
}

/**
 * 阶段路由清单装配（lib/stage.ts 的 buildStageRouting + 命中文档追加）。
 * 清单项**只含路径 + 节名 + 取数方式，零正文**——细节层由模型自主 read。
 *
 * 命中文档分两类：
 *   - 编辑路径命中（jit）：文件系统事实（doc-impact-applies 标签）→ 直接进清单；
 *   - 关键词命中：仍受**域限定**（D7）约束——仅落在基础清单（声明域 ∪ 栈文档 ∪ 索引）
 *     内的命中才进清单，跨域词（如做 scheduler change 时聊到 discovery）不误拉无关域。
 */
function buildRoutingItems(
	stage: Stage,
	declDomains: string[],
	stacks: { backend: boolean; frontend: boolean },
	jitHits: DocEntry[],
	keywordHits: DocEntry[],
	config: ConstraintConfig,
): StageRoutingItem[] {
	const seen = new Set<string>();
	const items: StageRoutingItem[] = [];
	const push = (it: StageRoutingItem): void => {
		if (seen.has(it.path)) return;
		seen.add(it.path);
		items.push(it);
	};
	// ① 基础清单（索引 + 声明域 + 配置的阶段文档清单 / 无配置回退代码常量）
	const stack = stackForRouting(stacks);
	for (const it of buildStageRouting(stage, {
		domainDocs: declDomains,
		stack,
		select: config.stages?.[stage]?.docs,
		indexDoc: config.indexDoc,
	})) {
		push(it);
	}
	// ② 关键词命中（域限定内）→ **标注**基础清单里对应条目（不新增文档、不注正文）
	//    （D7 域限定：集合外命中丢弃——跨域词不再把无关域拉进清单）
	const allowed = new Set(items.map((i) => i.path));
	for (const d of filterKeywordDocs(keywordHits, allowed)) {
		const hit = items.find((i) => i.path === d.doc);
		if (!hit) continue;
		if (hit.how.includes('本次会话提及')) continue; // 已标注过（命中粘性）
		hit.how = `${hit.how}（本次会话提及，优先）`;
	}
	// ③ 编辑路径命中 → 进清单（路径标签是文件系统事实，零歧义）
	for (const d of jitHits) {
		push({
			path: d.doc,
			section: d.section ?? null,
			how: d.section
				? `read \`${d.doc}\`「${d.section}」（编辑路径命中，自主导入）`
				: `read \`${d.doc}\`（编辑路径命中，自主导入）`,
		});
	}
	return items;
}

/** 路由清单渲染（路径 + 节名 + 取数方式；**不含任何文档正文**；标题由装配条目的 heading 承担） */
function renderRoutingList(items: StageRoutingItem[]): string {
	const lines = [
		`> 正文不再主动注入——需要细节时按下列路径自行 read；取红线摘要用 \`bash scripts/harness/constraint-digest.sh --brief --domains <域>\`。`,
		``,
	];
	for (const it of items) {
		const sec = it.section ? `「${it.section}」` : "（整篇）";
		lines.push(`- \`${it.path}\`${sec} — ${it.how}`);
	}
	return lines.join("\n");
}

/**
 * 声明域红线层条目（阶段首推的核心载体）：取 flow 文档「业务约束与不变量」节的
 * **顶层加粗红线句逐行**（复用 lib/constraint-redlines.ts 的 extractRedlines，零第二份正则）。
 * 红线层为空/过短/节缺失 → **回退为取数指引行**（路径 + 节名 + digest 命令，review R-3：
 * 整节正文回退已退役——「正文永不主动推」对全部域恒成立，注入体量断言才有意义）。
 */
function buildRedlineItem(
	cwd: string,
	domain: string,
	minSectionBytes: number,
): PlanItem | null {
	const rel = `docs/reference/flow/${domain}.md`;
	const content = readDoc(cwd, rel);
	if (content == null) return null;
	const display = basename(rel);
	const digestCmd = `bash scripts/harness/constraint-digest.sh --brief --domains ${domain}`;
	const sec = extractSection(content, "业务约束与不变量");
	let body: string;
	let injectMode: InjectedDocEntry["mode"] = "section";
	let layer: "redline" | "pointer" = "redline";
	if (sec) {
		const red = extractRedlines(sec);
		if (red && red.bytes >= minSectionBytes) {
			body = `${red.lines.join("\n")}\n\n细节层：read \`${rel}\`「业务约束与不变量」节`;
		} else {
			// 红线层为空 / 低于最小字节 → 取数指引（零正文）
			layer = "pointer";
			body = `红线层提取为空或过短（< ${minSectionBytes}B）——正文不自动注入。\n细节层：read \`${rel}\`「业务约束与不变量」节\n红线摘要：\`${digestCmd}\``;
		}
	} else {
		// 节不存在 → 取数指引（零正文，不静默跳过）
		layer = "pointer";
		body = `「业务约束与不变量」节缺失——正文不自动注入。\n细节层：read \`${rel}\`（整篇）\n红线摘要：\`${digestCmd}\``;
	}
	return {
		key: rel,
		heading: `### ${display}（${layer === "redline" ? "红线层" : "取数指引"}）`,
		body,
		preview: firstLine(body, 120),
		relPath: rel,
		short: display,
		reason: "declaration",
		mode: injectMode,
		docLabel: `${display}(${layer === "redline" ? "红线层" : "取数指引"})`,
		layer,
	};
}

function planInjection(
	cwd: string,
	config: ConstraintConfig,
	state: SessionState,
	fallbackChange: { name: string; dir: string } | null,
): InjectionPlan {
	const stage = state.stage;
	const stageFamily = state.stageFamily;
	const boundChangeName = state.boundChange;
	const docList: string[] = [];
	const docEntries: InjectedDocEntry[] = [];
	const pinReads: PinReadEntry[] = [];

	// 分析源 change：绑定优先；未绑定时按 mtime 兜底（**仅 implementation 族**——
	// explore/propose 是新想法语境，拿无关 change 的 proposal 做分析会让内容错位）
	const bound = boundChangeName
		? (listChangeDirs(cwd).find((c) => c.name === boundChangeName) ?? null)
		: null;
	const analysisChange =
		stageFamily === "implementation" ? (bound ?? fallbackChange) : bound;

	// 关键词命中源 = 仅最近 N 条用户输入（change 产物全文早退出命中源——harness/AI 类
	// change 的正当词汇与业务域关键词本质重叠，隐式推断必致误注入）。
	// 改造后关键词命中**不再送达正文**，只作路由清单的「本次会话提及」标注。
	const changeText = analysisChange ? readChangeText(analysisChange) : "";
	if (analysisChange) {
		matchKeywordDocs(state.recentInputs.join("\n"), config, state);
	}
	const stacks = detectStacks(
		`${changeText}\n${state.recentInputs.join("\n")}`,
		config,
	);

	// 业务域声明（design D2 主层）：绑定 change → 解析 constraint-domains 标记。
	// **两处并集**（stage-driven 主线程追加契约）：
	//   ① proposal.md 头部（存量 change 的既有位置，保持不变）；
	//   ② stage.md 头部（**实现期活口径**——申报实施过程中新发现的业务域）。
	// stage.md **不得收窄** proposal.md 已声明的域（域声明按超集安全方向收敛：多声明
	// 只是多推几行红线，不会漏推）。每回合从文件重解析（proposal/stage 编辑立即生效）。
	const declDomains: string[] = [];
	const declUnknown: string[] = [];
	if (analysisChange) {
		const valid = new Set(listFlowDomains(cwd));
		const seen = new Set<string>();
		const addDomains = (text: string | null): void => {
			if (!text) return;
			for (const d of parseDomainDeclaration(text)) {
				if (seen.has(d)) continue;
				seen.add(d);
				if (valid.has(d)) declDomains.push(d);
				else declUnknown.push(d);
			}
		};
		// ① proposal.md（存量位置）
		addDomains(
			readDoc(cwd, `${OPENSPEC_CHANGES_DIR}/${analysisChange.name}/proposal.md`),
		);
		// ② stage.md（实现期活口径；readStageFile 的 raw 即文件全文，含同名 marker）
		addDomains(readStageFile(analysisChange.dir).raw);
	}

	const items: PlanItem[] = [];
	const minSectionBytes = config.minSectionBytes ?? 512;

	// ① 索引文档（稳定层）：「该去哪读」的入口，全阶段共有
	const indexContent = readDoc(cwd, config.indexDoc);
	if (indexContent != null) {
		const display = basename(config.indexDoc);
		docList.push(display);
		items.push({
			key: config.indexDoc,
			heading: `### ${display}`,
			body: indexContent.trim(),
			preview: firstLine(indexContent, 120),
			relPath: config.indexDoc,
			short: display,
			reason: "index",
			mode: "full",
			docLabel: display,
		});
	}

	// ② 阶段路由清单（稳定层）：路径 + 节名 + 取数方式，**零正文**
	if (stage) {
		const routing = buildRoutingItems(
			stage,
			declDomains,
			stacks,
			[...state.jitDocHits.values()],
			[...state.keywordDocHits.values()],
			config,
		);
		items.push({
			key: "stage-routing",
			heading: `### 阶段路由清单（${stage}）`,
			body: renderRoutingList(routing),
			preview: `${routing.length} 个文档（自主 read）`,
			relPath: "stage-routing",
			short: "路由清单",
			reason: "routing",
			mode: "full",
			docLabel: `路由清单(${stage})`,
		});
		docList.push(`路由清单(${stage})`);
	}

	// ③ 声明域红线层（动态层：阶段首推送达；同阶段内零重复——指纹去重）
	for (const d of declDomains) {
		const item = buildRedlineItem(cwd, d, minSectionBytes);
		if (!item) continue;
		docList.push(item.docLabel ?? item.short);
		items.push(item);
	}

	// ④ 命中文档（编辑路径 / 关键词）已并入 ② 路由清单——**零正文**（随清单变化走稳定层差异）

	// ⑤ change 级文件（**implementation 族** + 有活跃 change）：探索发现 + 词汇表
	let findingsRaw: string | null = null;
	let findingsRel = "";
	if (stageFamily === "implementation" && analysisChange) {
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

	// 声明状态（widget + 注入块头部展示）：有声明列域，无声明显式提示（不阻断），未知域名恒提示
	const declNote = analysisChange
		? declDomains.length
			? `声明域：${declDomains.join(", ")}${declUnknown.length ? `（忽略未知域：${declUnknown.join(", ")}）` : ""}`
			: `无域声明（纯工具链 change 可忽略）${declUnknown.length ? `；忽略未知域：${declUnknown.join(", ")}` : ""}`
		: null;

	// —— 预算装配（tier-b design D1/D3）：条目 → applyBudget → sections/记账/header ——
	const budgetBytes = config.budgetBytes ?? DEFAULT_BUDGET_BYTES;
	const budgetActive = budgetBytes > 0;
	const declLine = declNote ? `- 域声明：${declNote}` : null;
	const stageLine = `- 阶段：${stage ? `${stage}（族：${stageFamily}）` : "未绑定（仅索引）"}`;
	const changeLine = `- 活跃变更：${analysisChange ? analysisChange.name : "无"}${bound ? "（阶段绑定）" : ""}`;
	const docLine = `- 命中文档：${docList.length ? docList.join("、") : "无"}`;
	const buildHeader = (degraded: PlanItem[] | null): string =>
		[
			`## 🔒 项目约束（constraint-injection extension 强制注入，本阶段必须遵守）`,
			``,
			`> 与 AGENTS.md 优先级宪法冲突时，以宪法为准。`,
			``,
			stageLine,
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
		budgetActive && (stage || stageFamily)
			? `预算 ${fmtK(outcome.totalBytes)}/${fmtK(budgetBytes)}${outcome.degraded.length ? `（降级${outcome.degraded.length}）` : ""}`
			: null;

	return {
		stage,
		stageFamily,
		stageLabel: stage ?? "未绑定",
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

/* ---------- 阶段持久化记账（stage.set；取代 legacy mode.set） ---------- */

/**
 * 阶段/绑定激活记账：写 `stage.set`（payload：stage/stageFamily/boundChange/source）。
 * **stage 与 stageFamily 恒由 change 的 stage.md 派生**（MUST NOT 独立写）；
 * legacy change（无/非法 marker）→ stage=null，仅记绑定归因（boundChange 列），
 * 供 edit-map / doc-impact 等「只用 boundChange 名」的消费点继续归属。
 * 记账失败不阻断阶段激活（logEvent 已 fail-loud，此处静默——激活是主路径）。
 * 无 sessionId（非真实会话，如烟测 stub）→ 跳过。
 */
function logStageBinding(
	cwd: string,
	sessionId: string | undefined,
	stage: Stage | null,
	stageFamily: StageFamily | null,
	boundChange: string | null,
	source: StageSetSource,
): void {
	if (!sessionId) return;
	if (stage) {
		logStageSet(cwd, sessionId, stage, stageFamily ?? "planning", boundChange, source);
		return;
	}
	logEvent(cwd, {
		kind: "stage.set",
		sessionId,
		change: boundChange,
		payload: {
			stage: null,
			stageFamily: stageFamily ?? null,
			boundChange,
			source,
		},
	});
}

/** 非法 stage marker 的 warn 记账（spec：非法/重复 marker → 降级不阻断 + 记 warn 事件） */
function logInvalidStageMarker(
	cwd: string,
	sessionId: string | undefined,
	boundChange: string | null,
): void {
	if (!sessionId) return;
	logEvent(cwd, {
		kind: "stage.set",
		sessionId,
		change: boundChange,
		payload: {
			stage: null,
			stageFamily: null,
			boundChange,
			source: "fallback",
			warn: "invalid-stage-marker",
		},
	});
}

/** change 目录 → 阶段解析（stage.md marker 三态；legacy/invalid → stage=null，不阻断） */
function resolveStageOfChange(
	cwd: string,
	changeName: string | null,
): { stage: Stage | null; family: StageFamily | null; invalid: boolean } {
	if (!changeName) return { stage: null, family: null, invalid: false };
	const res = readStageFile(join(cwd, OPENSPEC_CHANGES_DIR, changeName));
	if (res.kind === "ok" && res.stage) {
		return {
			stage: res.stage,
			family: stageFamilyOf(res.stage),
			invalid: false,
		};
	}
	return { stage: null, family: null, invalid: res.kind === "invalid" };
}

/**
 * mtime 兜底候选：仅当活跃 change 处于 **implementation 族** 才可兜底绑定。
 * （planning 族 = explore/propose/design 新想法语境，兜底会绑到无关 change——与改造前
 * 「requirements 档不兜底」同款规则；阶段化后判据从档位换成阶段族。）
 */
function fallbackBindableChange(cwd: string): string | null {
	const active = detectActiveChange(cwd);
	if (!active) return null;
	const res = resolveStageOfChange(cwd, active.name);
	return res.family === "implementation" ? active.name : null;
}

/**
 * 绑定 change + 派生阶段 + 记账（stage.set）。
 * 返回 true = 阶段/绑定发生变化（调用方据此决定是否需要重新投递）。
 * 阶段恒为 stage.md 派生值；legacy change → stage=null（按未绑定阶段处理）。
 */
function bindStage(
	cwd: string,
	state: SessionState,
	changeName: string | null,
	source: StageSetSource,
): boolean {
	const res = resolveStageOfChange(cwd, changeName);
	const stage = res.stage;
	const family = stage ? stageFamilyOf(stage) : null;
	const changed = stage !== state.stage || changeName !== state.boundChange;
	state.stage = stage;
	state.stageFamily = family;
	state.boundChange = changeName;
	logStageBinding(
		cwd,
		realSessionId(state),
		stage,
		family,
		changeName,
		source,
	);
	if (res.invalid) {
		logInvalidStageMarker(cwd, realSessionId(state), changeName);
	}
	return changed;
}

/**
 * 恢复路径取数（resume/reload/startup）：`queryLatestStageSet`（**双读**：先 stage.set，
 * 无结果时回退 legacy mode.set 并按 requirements→planning / implementation→implementation
 * 映射回族）。仅按**本会话**自己的记录取数，MUST NOT 全局兜底（多 pi 窗口并行是常态，
 * 「全局最新」几乎必然属其他窗口）。绑定 change 目录已不存在 → 不可恢复（回落未绑定）。
 */
function recoverStage(
	cwd: string,
	sessionId: string | undefined,
): StageSetInfo | null {
	if (!sessionId) return null;
	let info: StageSetInfo | null = null;
	try {
		info = queryLatestStageSet(sessionId, cwd);
	} catch (e) {
		console.error(
			`[constraint-injection] recover stage failed: ${(e as Error).message}`,
		);
		return null;
	}
	if (!info) return null;
	if (
		info.boundChange &&
		!existsSync(join(cwd, OPENSPEC_CHANGES_DIR, info.boundChange))
	) {
		return null;
	}
	return info;
}

/* ---------- 命令匹配（工作流命令识别；**不决定阶段**） ---------- */

/**
 * 工作流命令匹配：返回 legacy 档位标签（requirements / implementation）作为**工作流意图**。
 * 改造前命令直接决定 requirements/implementation 档位；现在命令**不决定阶段**——
 * 阶段恒由绑定 change 的 stage.md 派生（D1：唯一真相源）。命令只回答一件事：
 * 「是否允许 mtime 兜底绑定」（implementation 意图才兜底；planning = 新想法语境）。
 */
function matchWorkflowCommand(
	text: string,
	config: ConstraintConfig,
): "requirements" | "implementation" | null {
	const firstToken = text.trim().split(/\s+/)[0]?.toLowerCase();
	if (!firstToken) return null;
	// 命令表两处兼容：legacy `modes`（改造前配置）与新 `stages` 段（group 6 配置形态）
	const legacyModes = config.modes;
	if (legacyModes) {
		for (const [mode, cfgMode] of Object.entries(legacyModes)) {
			if (cfgMode.commands?.some((c) => c.toLowerCase() === firstToken)) {
				return mode === "implementation" ? "implementation" : "requirements";
			}
		}
	}
	const stages = config.stages;
	if (stages) {
		for (const [stage, cfgStage] of Object.entries(stages)) {
			if (cfgStage.commands?.some((c) => c.toLowerCase() === firstToken)) {
				// 阶段名本身即族线索：implement/verify/archive → implementation，其余 → planning
				return stage === "implement" || stage === "verify" || stage === "archive"
					? "implementation"
					: "requirements";
			}
		}
	}
	return null;
}

/* ---------- skill 文件路径匹配（agent 自动 read openspec skill → 工作流意图） ---------- */

/**
 * skill 路径 → 工作流意图（implementation / planning）；非 openspec skill → null。
 * 同 matchWorkflowCommand：**只决定是否允许 mtime 兜底**，不决定阶段。
 */
function skillWorkflowIntent(
	path: string,
	config: ConstraintConfig,
): "planning" | "implementation" | null {
	const signals = config.skillSignals;
	if (!signals) return null;
	const hit = (names: string[]): boolean =>
		names.some(
			(name) =>
				path.includes(`/skills/${name}/`) || path.includes(`\\skills\\${name}\\`),
		);
	if (hit(signals.implementation ?? [])) return "implementation";
	if (hit(signals.requirements ?? [])) return "planning";
	return null;
}

/** change 目录内的 stage.md 路径判定（写它 → 阶段即时更新） */
function isStageFilePath(path: string): boolean {
	const p = path.replaceAll("\\", "/");
	return /(^|\/)openspec\/changes\/[^/]+\/stage\.md$/.test(p);
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

/**
 * 稳定层快照 key（D2/D3）：**阶段族** + 绑定 change。
 * 阶段族而非完整 stage 入键——同族内切换（explore→propose→design）不重建稳定层
 * （前缀缓存命中率与快照重建频率直接相关）；族切换才整体重建一次。
 */
export function stableSnapshotKey(
	stageFamily: StageFamily | null,
	boundChange: string | null,
): string {
	return `${stageFamily ?? "none"}|${boundChange ?? "none"}`;
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
	/** 声明域注入层级（redline 红线层 / pointer 取数指引）——记账 layer 标记用（动态层首推） */
	layer?: "redline" | "pointer";
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

/** 稳定层/动态层在 system prompt 中的分界线（D1）；legacy 模式下不分层
 *  （改造后稳定层 = 索引 + **阶段路由清单**——零业务域正文；动态层 = 红线层 +
 *  change 级文件 + 路由指针） */
export function splitPlanItems(items: PlanItem[]): {
	stable: PlanItem[];
	dynamic: PlanItem[];
} {
	const stableReasons = new Set(["index", "routing"]);
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

/** PlanItem / DynamicItem → 记账条目（D6：bytes 取降级后正文；layer = 声明域注入层级） */
function itemToDocEntry(it: {
	key: string;
	mode: InjectedDocEntry["mode"];
	reason: InjectedDocEntry["reason"];
	body: string;
	/** PlanItem 的降级标记（digest / placeholder） */
	degradedTo?: string;
	/** DynamicItem 的降级标记（布尔） */
	degraded?: boolean;
	layer?: "redline" | "pointer";
}): InjectedDocEntry {
	return {
		path: it.key,
		mode: it.mode,
		reason: it.reason,
		bytes: Buffer.byteLength(it.body, "utf8"),
		...((it.degradedTo || it.degraded) ? { degraded: true } : {}),
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
		...(it.layer ? { layer: it.layer } : {}),
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
		`- 阶段：${plan.stage ? `${plan.stage}（族：${plan.stageFamily}）` : "未绑定（仅索引）"}`,
		`- 活跃变更：${plan.changeName ?? "无"}${plan.bound ? "（阶段绑定）" : ""}`,
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
	tier: InjectionTier,
	source?: string,
): void {
	if (!sessionId) return;
	for (const d of entries) {
		logEvent(ctx.cwd, {
			kind: "constraint.inject",
			sessionId,
			change: changeName,
			payload: source ? { ...d, tier, source } : { ...d, tier },
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
	// 指纹随投递记录 regime（fix-injection-transition-fingerprint-wipe）：延迟重建的
	// 清空判定据此区分「mid-turn 切档后同 turn 已投递（不清）」与「真实 regime 切换（清）」。
	// force 且零增量时重写同值，无害。
	state.channel.fingerprintRegime = stableSnapshotKey(
		state.stageFamily,
		state.boundChange,
	);
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
		payloadItems.map(itemToDocEntry),
		tierOf(state.stageFamily),
		force ? "compact-resend" : undefined,
	);
	return increments.length;
}

/**
 * 子线程稳定层投递（harden-subagent-constraint-channel T3-rev/D3 实测修订）：pi-web 托管
 * 的子线程 SDK 没有 before_agent_start 事件（编译码词汇表零命中），稳定层 system prompt
 * 到不了子模型；改在 session_start 末尾经 sendMessage(steer) 投递（session_start 在其
 * 事件词汇表内，实测可达，与 compact 重发 / JIT 同款机制）。
 * 复用 before_agent_start 混合通道的稳定层构建（splitPlanItems + buildStableHeader +
 * renderStableBlock，key=stableSnapshotKey）；未绑定档自然只含索引（不投任何 change 文本）。
 * 随投递记 constraint.inject(source=child-init) 并置 stableSnapshot 与 childInitDelivered
 * ——后者令 before_agent_start 冻结快照/稳定层记账且不再返回 systemPrompt 块（防双投）。
 * 计划为空（无任何可注入条目）跳过。主会话（父子不可证）不进入本函数。
 */
function deliverChildInitStable(
	pi: ExtensionAPI,
	ctx: ExtCtx,
	cfg: ConstraintConfig,
	state: SessionState,
): void {
	const plan = planInjection(ctx.cwd, cfg, state, detectActiveChange(ctx.cwd));
	if (!plan.items.length) return;
	const { stable, dynamic } = splitPlanItems(plan.items);
	const key = stableSnapshotKey(state.stageFamily, state.boundChange);
	const stableBlock = renderStableBlock(buildStableHeader(plan, stable), stable);
	pi.sendMessage(
		{
			customType: "constraint-injection",
			content: stableBlock,
			display: cfg.dynamicDisplay === "full",
		},
		{ deliverAs: "steer", triggerTurn: false },
	);
	state.channel.stableSnapshot = { key, block: stableBlock, items: stable };
	state.channel.childInitDelivered = true;
	logConstraintInjects(
		ctx,
		realSessionId(state),
		plan.changeName,
		stable.map(itemToDocEntry),
		tierOf(state.stageFamily),
		"child-init",
	);
	// 动态层同批送达（红线层 + change 级文件）：pi-web 子会话**无 before_agent_start**，
	// 红线层若只走动态层会丢失（改造前红线层在稳定层内，随稳定层一并发）。
	if (dynamic.length) {
		deliverDynamic(
			pi,
			ctx,
			cfg,
			state,
			plan.changeName,
			toDynamicItems(dynamic),
			"📎 约束首推（子线程初始化）",
		);
	}
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
		// 事实库）→ 否则按同 sessionId 第 1 段恢复 → 都没有则保持未绑定」处理；
		// MUST NOT 借用他会话的状态（隔离前的 bug：全局值被他会话改写后本会话被动继承）。
		if (reason === "startup") {
			if (state.stage === null && state.boundChange === null) {
				// 子线程/fork 继承：内存路径（同进程）→ 事实库回退（跨进程，真实链路主路径）
				const inherited =
					inheritFromParent(state, ctx) || inheritFromParentHistory(state, ctx);
				if (inherited) {
					// 继承记账：source=inherit（阶段随父会话快照写死，不在子线程重算——D8）
					logStageBinding(
						ctx.cwd,
						realSessionId(state),
						state.stage,
						state.stageFamily,
						state.boundChange,
						"inherit",
					);
				} else {
					// 同 sessionId 恢复（queryLatestStageSet 双读：stage.set → legacy mode.set）
					const sessionId = realSessionId(state);
					const rec = sessionId ? recoverStage(ctx.cwd, sessionId) : null;
					if (rec) {
						state.stage = rec.stage;
						state.stageFamily = rec.stageFamily;
						state.boundChange = rec.boundChange;
						// 恢复路径显式记账（D6）：隐性绑定不存在；source=recover
						logStageBinding(
							ctx.cwd,
							sessionId,
							rec.stage,
							rec.stageFamily,
							rec.boundChange,
							"recover",
						);
					}
				}
			}
			// 子线程稳定层投递（harden-subagent-constraint-channel T3-rev/D3）：继承/恢复逻辑
			// 收口后、return 前执行——pi-web 子线程 SDK 无 before_agent_start，稳定层改由本
			// 事件经 sendMessage(steer) 承载（deliverChildInitStable）；配置缺失则跳过（保持
			// 下方 return 语义）。主会话（父子不可证）零变化。
			if (ctx?.cwd && isChildSession(ctx)) {
				const cfg = loadConfig(ctx.cwd);
				if (cfg) deliverChildInitStable(pi, ctx, cfg, state);
			}
			return;
		}
		if (!["new", "resume", "fork", "reload"].includes(reason)) return;
		// 会话边界重置**只清本会话条目**（他会话的阶段/命中集 MUST NOT 被清零——
		// 隔离前这里清的是全局，等于替所有会话重置）。
		resetSessionState(state);
		// fork：父子可证 → 继承父会话（同一份对话继续干同一件事）；不可证 → 未绑定。
		// 继承两条路：内存（同进程）→ 事实库回退（跨进程，真实链路主路径）。
		if (reason === "fork") {
			const inherited =
				inheritFromParent(state, ctx) || inheritFromParentHistory(state, ctx);
			if (inherited) {
				logStageBinding(
					ctx.cwd,
					realSessionId(state),
					state.stage,
					state.stageFamily,
					state.boundChange,
					"inherit",
				);
			}
			return;
		}
		// resume/reload 阶段恢复（仅同 sessionId 单段）：resume = /resume 切换目标会话
		// （id 即目标会话 id，必中），reload = 同会话扩展重载（id 不变，必中）；无本会话
		// 记录不恢复（不继承他窗口状态）。粘性命中集合不恢复（有意为之——声明域红线层
		// 每回合重解析照常送达、编辑路径改代码即重新命中）。
		if ((reason === "resume" || reason === "reload") && ctx?.cwd) {
			const sessionId = realSessionId(state);
			const rec = sessionId ? recoverStage(ctx.cwd, sessionId) : null;
			if (rec) {
				state.stage = rec.stage;
				state.stageFamily = rec.stageFamily;
				state.boundChange = rec.boundChange;
				// 恢复路径显式记账（D6）：source=recover
				logStageBinding(
					ctx.cwd,
					sessionId,
					rec.stage,
					rec.stageFamily,
					rec.boundChange,
					"recover",
				);
			}
		}
	});
	// 1. 输入 → 绑定 change（阶段由其 stage.md 决定；命令/skill 不再直接决定档位）
	pi.on("input", async (event, ctx) => {
		const cfg = loadConfig(ctx.cwd);
		if (!cfg) return;
		const state = stateFor(ctx, cfg);
		// 最近 N 条用户输入入窗（无论是否命令），参与关键词/栈判定（入窗限本会话）
		const text = event.text.trim();
		if (text) {
			state.recentInputs.push(text.length > 500 ? text.slice(0, 500) : text);
			const max = cfg.recentInputWindow ?? 5;
			if (state.recentInputs.length > max) {
				state.recentInputs = state.recentInputs.slice(-max);
			}
		}
		// turn 绑定锁：同 turn 内首个生效绑定后不再切换
		if (state.turnBindLocked) return;
		// ① 输入提及 change 名 → 绑定该 change（source=command）
		const mentioned = detectMentionedChange(ctx.cwd, event.text)?.name ?? null;
		if (mentioned) {
			bindStage(ctx.cwd, state, mentioned, "command");
			state.turnBindLocked = true;
			return;
		}
		// ② 工作流命令（斜杠命令）→ 记工作流意图；**mtime 兜底绑定在 turn 起点执行**
		//    （source=fallback；仅 implementation 意图——planning = 新想法语境）
		const cmdIntent = matchWorkflowCommand(event.text, cfg);
		if (cmdIntent) {
			state.workflowIntent =
				cmdIntent === "implementation" ? "implementation" : "planning";
		}
	});

	// 1.5 skill 文件读取即绑定（agent 自动 read skill 是斜杠命令外的真实触发源）；
	//     已绑定时，写 openspec/changes/<name>/ 下文件修正绑定（propose 先建 change 后写产物 / apply 勾选 tasks 的场景）
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

		// ① 读取记账已移至 tool_execution_end（review R-4）：只在 read **成功完成**时
		//    记 constraint.read（归档对账唯一事实源）——发起即记会把失败读取（路径拼错/
		//    文件不存在）也算成证据；start 只登记在途 toolCallId → path。
		if (event.toolName === "read" && event.toolCallId) {
			pendingReadCalls.set(event.toolCallId, p);
		}

		// ② openspec skill 读取 → 记工作流意图 + 绑定 change（阶段由其 stage.md 决定；source=skill）
		const skillIntent = skillWorkflowIntent(p, cfg);
		if (skillIntent) {
			state.workflowIntent = skillIntent;
			const mentioned =
				detectMentionedChange(ctx.cwd, state.recentInputs.join("\n"))?.name ?? null;
			if (mentioned) {
				bindStage(ctx.cwd, state, mentioned, "skill");
				state.turnBindLocked = true;
			}
			return;
		}

		// ③ stage.md 写入/编辑 → 阶段即时更新（marker 是阶段唯一真相源；source=edit-dir）
		let stageChanged = false;
		if (
			(event.toolName === "write" || event.toolName === "edit") &&
			isStageFilePath(p)
		) {
			const name = changeNameFromPath(p);
			if (name && (state.boundChange === null || state.boundChange === name)) {
				const before = state.stage;
				bindStage(ctx.cwd, state, name, "edit-dir");
				stageChanged = state.stage !== before;
				state.turnBindLocked = true;
			}
		}

		// ④ 编辑路径命中（doc-impact-applies 标签）→ **只进路由清单**（零正文）。
		//    整节正文通道已退役（改造前最坏 27.8KB/条）；命中文档改为「路径 + 节名」
		//    出现在路由清单里，由模型自主 read。
		let jitAdded = false;
		if (event.toolName === "write" || event.toolName === "edit") {
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
				bindStage(ctx.cwd, state, name, "edit-dir");
				state.turnBindLocked = true;
			}
		}
		// 即时投递（turn 中途通道）：阶段变更 / 新命中路由 → 立即算增量并经 steer 送出。
		// 改造后送达内容为**红线层 + 路由清单**（无整节正文）。
		if ((stageChanged || jitAdded) && cfg.channel !== "legacy") {
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
				dynamicLabelWithDegrad(
					stageChanged
						? "📎 约束首推（阶段变更）"
						: "📎 约束补充（路由更新）",
					plan,
					cfg,
				),
			);
		}
	});

	// 1b. 读取成功回执 → constraint.read 记账（review R-4，归档对账唯一事实源）。
	//     只在 tool_execution_end 且非 error 时记：发起即记会把失败读取（路径拼错/
	//     文件不存在）也算成证据；局部读取（offset/limit）成功送达内容同样算读（宽松）。
	pi.on("tool_execution_end", async (event, ctx) => {
		if (event.toolName !== "read") return;
		const rawPath = pendingReadCalls.get(event.toolCallId) ?? null;
		pendingReadCalls.delete(event.toolCallId);
		if (!rawPath || event.isError) return;
		const cfg = loadConfig(ctx.cwd);
		if (!cfg) return;
		const rel = normalizeRepoPath(rawPath, ctx.cwd);
		if (!rel || !isConstraintDocPath(rel)) return;
		const state = stateFor(ctx, cfg);
		const sessionId = realSessionId(state);
		if (sessionId) logConstraintRead(ctx.cwd, sessionId, rel, state.boundChange);
	});

	// 2. system prompt 注入（每 turn 重建 → 幂等；前缀缓存 → 便宜；抗 compaction）
	// 2. 约束注入（混合通道 D1~D4）：稳定层进 system prompt（阶段族生命周期内字节恒定）；
	//    动态层走追加消息（指纹 diff，零变化零投递）。channel="legacy" 回退旧全量行为。
	pi.on("before_agent_start", async (event, ctx) => {
		const cfg = loadConfig(ctx.cwd);
		if (!cfg) return;
		const state = stateFor(ctx, cfg);
		// turn 起点释放绑定锁（D5：同 turn 锁定、跳 turn 再可切）——只解本会话的锁
		state.turnBindLocked = false;
		const activeChange = detectActiveChange(ctx.cwd);
		// 绑定回落：仅当绑定的 change 已不存在（归档/删除）→ 回落未绑定；
		// 不因其他 change 目录 mtime 更新而误掉绑定（多 change 并行是常态）
		if (
			state.boundChange &&
			!existsSync(join(ctx.cwd, OPENSPEC_CHANGES_DIR, state.boundChange))
		) {
			state.stage = null;
			state.stageFamily = null;
			state.boundChange = null;
		}
		// 阶段漂移检测：绑定 change 的 stage.md marker 可能在会话中被改（外部编辑）
		// → 重新派生阶段（source=edit-dir：marker 就在 change 目录里）
		if (state.boundChange) {
			const res = resolveStageOfChange(ctx.cwd, state.boundChange);
			if (res.stage !== state.stage) {
				bindStage(ctx.cwd, state, state.boundChange, "edit-dir");
			}
		}
		// mtime 兜底显式化（D6）：**implementation 工作流意图**且未绑定时，把活跃 change
		// 升为显式绑定并记账（source=fallback）——消除「注入挂在 X 名下、boundChange
		// 仍为空」的账实不符与静默漂移（多 change 并行污染根源之一）。
		// ⚠ planning 意图 = 新想法语境（explore/propose/design），**不兜底**（同改造前
		// 「requirements 档不兜底」规则）；无工作流意图时同样不兜底。
		if (!state.boundChange && state.workflowIntent === "implementation") {
			const fallback = fallbackBindableChange(ctx.cwd);
			if (fallback) bindStage(ctx.cwd, state, fallback, "fallback");
		}
		const plan = planInjection(ctx.cwd, cfg, state, activeChange);
		const sessionId = realSessionId(state);

		// 状态展示（始终，让用户看到注入了什么 + 预算用量）
		if (ctx.hasUI) {
			const parts: string[] = [`[约束] ${plan.stageLabel}`];
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
				logConstraintInjects(
					ctx,
					sessionId,
					plan.changeName,
					plan.docEntries,
					tierOf(state.stageFamily),
				);
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
		const key = stableSnapshotKey(state.stageFamily, state.boundChange);
		// 无稳定条目但有内容时仍出 header（阶段/活跃变更/域声明对模型可见）
		const wantBlock = plan.items.length > 0;
		let stableBlock = "";
		let extraDynamic: PlanItem[] = [];
		if (state.channel.childInitDelivered) {
			// 子线程 session_start 已投递（harden-subagent-constraint-channel T3-rev/D3）：
			// 稳定层由消息通道承载，本 handler MUST NOT 再返回 systemPrompt 块（防双投）；
			// 快照/稳定层记账冻结不重复——稳定层后续内容变化照常经 extraDynamic 差异送达
			extraDynamic = state.channel.stableSnapshot
				? diffStableItems(state.channel.stableSnapshot.items, stable)
				: [];
		} else if (
			wantBlock &&
			(!state.channel.stableSnapshot || state.channel.stableSnapshot.key !== key)
		) {
			// 阶段族/绑定切换（快照已存在但 key 变）→ 按指纹 regime 判定是否重置动态指纹
			// （fix-injection-transition-fingerprint-wipe）：阶段/绑定可 mid-turn 变化、快照重建
			// 延迟到下一 turn 起点——mid-turn 切阶段后同 turn 已投递的条目其指纹 regime 已是新 key，
			// MUST NOT 被当作旧 regime 残留清空而原样重投；仅真实 regime 切换（如 edit-dir 绑定
			// 修正、绑定回落）才清空全量重投。首次建快照（如路由更新已在快照前投递过）不重置，
			// 避免重复投递。
			const isTransition =
				state.channel.stableSnapshot !== null &&
				state.channel.stableSnapshot.key !== key;
			stableBlock = renderStableBlock(buildStableHeader(plan, stable), stable);
			state.channel.stableSnapshot = { key, block: stableBlock, items: stable };
			if (isTransition && state.channel.fingerprintRegime !== key)
				state.channel.sentFingerprint = new Map();
			logConstraintInjects(
				ctx,
				sessionId,
				plan.changeName,
				stable.map(itemToDocEntry),
				tierOf(state.stageFamily),
			);
		} else {
			// 快照冻结（字节恒定）；稳定层内容后来的变化（如 proposal 新增声明域、
			// 编辑路径新增命中 → 路由清单变化）作为动态层差异条目送达，不改写 system prompt（D2）
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
			// 阶段首推 vs 增量：某 stage 首次送达 → 标「阶段首推」（可观测的形态切换点）
			const isStageFirstPush =
				plan.stage !== null && state.lastDeliveredStage !== plan.stage;
			const baseLabel = isStageFirstPush
				? `📎 约束首推（阶段 ${plan.stage}）`
				: extraDynamic.length
					? "📎 约束补充（稳定层差异）"
					: "📎 约束补充（增量）";
			const label = dynamicLabelWithDegrad(baseLabel, plan, cfg);
			delivered = deliverDynamic(
				pi,
				ctx,
				cfg,
				state,
				plan.changeName,
				dynamicItems,
				label,
			);
			if (plan.stage) state.lastDeliveredStage = plan.stage;
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
			"把关键代码事实（表结构/调用链/集成点/数据流/边界枚举/关键算法）持久化到活跃 change 的 explore-findings.md（有活跃 change 时实现/评审阶段自动注入 system prompt，避免重复探索）；" +
			"research/问答语境（无活跃 change）则落 docs/research/<topic>/（未传 topic 落通用池单文件），不碰 openspec/changes/。",
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
					state.boundChange
						? (changes.find((c) => c.name === state.boundChange) ?? null)
						: null;
				const active =
					bound ??
					(state.stageFamily === "implementation"
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
