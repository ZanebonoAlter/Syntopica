/**
 * sql-safety-guard.ts — pi extension：破坏性 SQL（仅 TRUNCATE/DROP）执行前硬拦截（默认 hard）
 *
 * 设计决策（change: add-sql-safety-guard；背景 = 2026-09-22 psql -c 裸跑 TRUNCATE…CASCADE
 * 清空 7 张表事故——事后文档红线是软约束，本守卫把「误清真库」升级为工具边界的硬门禁）：
 * 1. 判定纯函数 classifySqlCommand(command, cwd) → pass|bypass|hit，顺序严格按 design D2：
 *    ① 双键粗筛：原文含 `psql` token（/i）∧（动词 `\b(TRUNCATE|DROP)\b`/i 命中 或 `-f`/`--file`
 *    形态）——任一不满足直接放行（只读 psql、文档 grep、DELETE/UPDATE/ALTER/INSERT 零感知，
 *    用户拍板只拦这两个词）；② 逃生注释：原文含 `# allow-truncate-drop` → 放行 + 记 bypass；
 *    ③ `BEGIN` 与 `ROLLBACK` 同现（/i）→ 放行（事务内验证是与逃生注释等价的硬证据）；
 *    ④ 一次性容器：`docker run`+`--rm` 或 `createdb`+`dropdb` 同现 → 放行；⑤ `-f`/`--file`
 *    形态：读文件扫两词——可读且文件与原文都不含动词 → 放行，文件含动词 → block（reason 注明
 *    文件来源），路径解析不出/不可读 → 保守 block（无法证明无害即拦）；⑥ 非 `-f` 形态双键
 *    命中 → block。
 * 2. 不做引号/注释掩蔽（design D1，与 test-scope-guard 相反——本守卫要拦的 SQL 正文就在
 *    引号里，掩蔽会把目标掩掉）；`/i` 大小写不敏感；DROP 不限定宾语（design D6，入口键已
 *    限定 psql）。嵌套 `bash -c` 不需递归抽取：payload 本就在命令原文里，粗筛天然覆盖。
 * 3. 逃生只认命令原文注释，不认会话语境（design D3：数据安全规则若认语境，说一句「在做清理」
 *    即可自我放行，规则即失效——spec 亦 SHALL NOT 设语境豁免）。
 * 4. 扫描通道：bash + ctx_execute（仅 language==="shell" 取 input.code）+ ctx_batch_execute
 *    （逐 commands[].command，任一条命中则整次调用 block）；判定自身异常 fail-open 放行
 *    （文档全景表 fail 策略口径）。
 * 5. 模式：hard（默认）命中 → block，reason 给出全部三条出路（`BEGIN; …; ROLLBACK;` 事务包裹 /
 *    命令尾部 `# allow-truncate-drop` 注释 / `SQL_SAFETY_GUARD=off`（或 soft））；soft →
 *    ctx.ui.notify 软提醒 + 记 warn 不阻断（UI 不可用 try/catch 静默）；off → 粗筛前直接
 *    return（零评估零记账）。守卫自身文案统一带 [sql-safety-guard] 前缀（语境扫描过滤
 *    依据，防自噬）。
 * 6. 记账（lib/policy-decision 旁路，仿 test-scope-guard 的 auditPolicy）：block→block、
 *    soft→warn、逃生注释放行→bypass（action 白名单无 info，bypass=显著放行的 info 级记录，
 *    供 harness-retro 回检逃生命中率）；off/未命中/BEGIN-ROLLBACK/容器放行零记账；无
 *    cwd/sessionId 不记；记账异常绝不影响裁决。
 *
 * 配置：SQL_SAFETY_GUARD=hard|soft|off（默认 hard，非法/缺省值回退 hard）。
 */
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { readFile } from "node:fs/promises";
import { isAbsolute, resolve as resolvePath } from "node:path";
import { logPolicyDecision, type PolicyDecisionInput } from "./lib/policy-decision";

type GuardMode = "hard" | "soft" | "off";

/** 守卫模式（环境变量 SQL_SAFETY_GUARD，模块加载时解析） */
const MODE: GuardMode = parseMode(process.env.SQL_SAFETY_GUARD);
/** 显式逃生注释（命令原始文本内出现即放行；test-cases 边界值：首部出现同样生效） */
const ESCAPE_TAG = "# allow-truncate-drop";
/** 守卫自身文案统一前缀（spec：语境扫描过滤依据，防自噬） */
const GUARD_PREFIX = "[sql-safety-guard]";
/** 入口键：psql token（大小写不敏感） */
const PSQL_RE = /\bpsql\b/i;
/** 拦截词：仅 TRUNCATE/DROP 两词，词边界 + 大小写不敏感，DROP 不限定宾语 */
const VERB_RE = /\b(TRUNCATE|DROP)\b/i;
const BEGIN_RE = /\bBEGIN\b/i;
const ROLLBACK_RE = /\bROLLBACK\b/i;
/** policy.decision 记账参数（reasonCode 固定 sql-safety） */
const POLICY_NAME = "sql-safety-guard";
const REASON_CODE = "sql-safety";
/** 软提醒文案（soft 模式专用，带守卫前缀） */
const SOFT_NOTICE =
	`${GUARD_PREFIX} 检出 psql 入口的 TRUNCATE/DROP 破坏性 SQL（SQL_SAFETY_GUARD=soft 不阻断）：` +
	"建议 `BEGIN; …; ROLLBACK;` 事务包裹，或命令尾部加 `# allow-truncate-drop` 注释留痕。";

/** 本扩展只用到的 ExtensionContext 子集（结构兼容，见 test-scope-guard GuardCtx） */
type GuardCtx = {
	ui: { notify(message: string, type?: "info" | "warning" | "error"): void };
	sessionManager?: {
		getSessionId?(): string | undefined;
		getEntries?(): unknown[];
		buildContextEntries?(): unknown[];
	};
	signal?: AbortSignal | undefined;
	cwd?: string;
};

// ---------- 主入口 ----------

export default function (pi: ExtensionAPI) {
	pi.on("tool_call", async (event, ctx) => {
		if (MODE === "off") return; // 零评估零记账（粗筛前直接返回）
		const input = (event.input ?? {}) as Record<string, unknown>;
		const commands = extractCommands(event.toolName, input);
		if (commands.length === 0) return;
		const cwd = typeof ctx.cwd === "string" ? ctx.cwd : "";
		for (const command of commands) {
			let verdict: SqlVerdict;
			try {
				verdict = await classifySqlCommand(command, cwd);
			} catch (err) {
				// fail-open：判定自身异常放行（文档全景表 fail 策略口径），不拦不记
				console.warn(`${GUARD_PREFIX} 判定异常 fail-open 放行：${String(err)}`);
				continue;
			}
			if (verdict.kind === "pass") continue;
			if (verdict.kind === "bypass") {
				console.log(`${GUARD_PREFIX} 逃生注释命中（${ESCAPE_TAG}），放行并记 bypass 留痕`);
				auditPolicy(ctx, "bypass");
				continue; // 批量通道里后续命令仍逐条判定（任一条命中则整次 block）
			}
			// hit：hard 才 block（soft 绝不阻断）
			if (MODE === "hard") {
				console.warn(`${GUARD_PREFIX} 破坏性 SQL 已阻断（hard 模式）`);
				auditPolicy(ctx, "block");
				return { block: true, reason: buildBlockReason(verdict.source) };
			}
			console.warn(`${GUARD_PREFIX} 破坏性 SQL 命中（soft 模式不阻断）`);
			auditPolicy(ctx, "warn");
			try {
				ctx.ui.notify(SOFT_NOTICE, "warning");
			} catch {
				// UI 不可用时静默降级（soft 模式本就不阻断）
			}
			return; // soft 首次命中即收口，避免批量通道重复提醒
		}
		return;
	});
}

// ---------- 通道扩展：按 toolName 分派取命令文本（复制自 test-scope-guard） ----------

/** bash→input.command；ctx_execute→仅 language==="shell" 取 input.code；
 *  ctx_batch_execute→逐 input.commands[].command；其余 toolName 返回空（零开销放行） */
export function extractCommands(toolName: string, input: Record<string, unknown>): string[] {
	if (toolName === "bash") {
		return typeof input.command === "string" && input.command ? [input.command] : [];
	}
	if (toolName === "ctx_execute") {
		if (input.language !== "shell") return [];
		return typeof input.code === "string" && input.code ? [input.code] : [];
	}
	if (toolName === "ctx_batch_execute") {
		if (!Array.isArray(input.commands)) return [];
		const out: string[] = [];
		for (const c of input.commands) {
			if (c && typeof c === "object") {
				const cmd = (c as Record<string, unknown>).command;
				if (typeof cmd === "string" && cmd) out.push(cmd);
			}
		}
		return out;
	}
	return [];
}

// ---------- 判定纯函数（design D2 顺序） ----------

/** 判定结果：pass=放行（零记账）/ bypass=逃生注释放行（记 bypass）/ hit=命中（hard block、soft warn） */
export type SqlVerdict =
	| { kind: "pass" }
	| { kind: "bypass" }
	| { kind: "hit"; source: HitSource };

export type HitSource = "command" | "file" | "file-unverified";

/**
 * 判定单条命令是否构成 psql 入口的 TRUNCATE/DROP 破坏性 SQL。
 * 顺序严格按 design D2：双键粗筛 → 逃生注释 → BEGIN/ROLLBACK → 一次性容器 → `-f` 文件扫描 → block。
 * cwd 用于解析 `-f` 相对路径（会话语境）；文件读取失败保守判「无法验证」型命中（不抛异常）。
 */
export async function classifySqlCommand(command: string, cwd: string): Promise<SqlVerdict> {
	if (!command || !command.trim()) return { kind: "pass" };
	// ① 双键粗筛：无 psql 入口直接放行；有入口还需 动词 或 -f 形态 之一（只读 psql -c 放行）
	if (!PSQL_RE.test(command)) return { kind: "pass" };
	const fileArg = extractFileArg(command);
	const hasVerb = VERB_RE.test(command);
	if (!hasVerb && !fileArg) return { kind: "pass" };
	// ② 逃生注释：命令原始文本（判定在任何掩蔽/截断之前）含逃生标签 → 放行（调用方记 bypass）
	if (command.includes(ESCAPE_TAG)) return { kind: "bypass" };
	// ③ BEGIN 与 ROLLBACK 同现（/i）→ 事务包裹放行（低噪声：零记账）
	if (BEGIN_RE.test(command) && ROLLBACK_RE.test(command)) return { kind: "pass" };
	// ④ 一次性容器：docker run --rm / createdb+dropdb 配对 → 放行（零记账）
	if (isOneShotEnv(command)) return { kind: "pass" };
	// ⑤ -f/--file 形态：读文件扫两词（无法证明无害即拦）
	if (fileArg) {
		if (!fileArg.path) return { kind: "hit", source: "file-unverified" };
		const content = await readFileArg(fileArg.path, cwd);
		if (content === null) return { kind: "hit", source: "file-unverified" };
		if (VERB_RE.test(content)) return { kind: "hit", source: "file" };
		if (hasVerb) return { kind: "hit", source: "command" }; // 文件干净但命令原文带词
		return { kind: "pass" };
	}
	// ⑥ 非 -f 形态双键命中 → block（D1：判定基于命令原文，不掩蔽引号）
	return { kind: "hit", source: "command" };
}

// ---------- `-f`/`--file` 形态解析与文件读取 ----------

/** `-f file` / `-f<file>` / `-f=<file>` / `--file file` / `--file=<file>`；
 *  flag 存在但取值解析不出 → { path: null }（保守：无法验证）；非 -f 形态 → null */
function extractFileArg(command: string): { path: string | null } | null {
	const tokens = command.split(/\s+/);
	for (let i = 0; i < tokens.length; i++) {
		const t = tokens[i];
		if (t === "-f" || t === "--file") {
			const next = tokens[i + 1];
			return { path: next && !next.startsWith("-") ? stripQuotes(next) || null : null };
		}
		if (t.startsWith("--file=")) {
			return { path: stripQuotes(t.slice("--file=".length)) || null };
		}
		if (t.startsWith("-f") && t.length > 2 && !t.startsWith("--")) {
			return { path: stripQuotes(t.slice(2).replace(/^=/, "")) || null };
		}
	}
	return null;
}

/** 去掉成对包裹引号（`'x.sql'` / `"x.sql"`）；不成对则原样返回 */
function stripQuotes(t: string): string {
	if (t.length >= 2 && (t[0] === '"' || t[0] === "'") && t[t.length - 1] === t[0]) {
		return t.slice(1, -1);
	}
	return t;
}

/** 读取 -f 指向的文件内容；相对路径按 cwd 解析。读不到/解析不出 → null（调用方保守 block） */
async function readFileArg(filePath: string, cwd: string): Promise<string | null> {
	try {
		if (!filePath) return null;
		const p = isAbsolute(filePath) ? filePath : cwd ? resolvePath(cwd, filePath) : null;
		if (!p) return null;
		return await readFile(p, "utf8");
	} catch {
		return null;
	}
}

// ---------- 一次性容器判定 ----------

/** `docker run` + `--rm` 同现，或 `createdb` + `dropdb` 配对同现 → 一次性环境 */
function isOneShotEnv(command: string): boolean {
	if (/\bdocker\s+run\b/i.test(command) && /(?:^|\s)--rm(?:\s|$)/.test(command)) return true;
	return /\bcreatedb\b/i.test(command) && /\bdropdb\b/i.test(command);
}

// ---------- block 文案（三条合法出路全给） ----------

const HIT_SOURCE_LABEL: Record<HitSource, string> = {
	command: "命令原文命中 psql 入口 + TRUNCATE/DROP",
	file: "psql -f 指向的文件内容命中 TRUNCATE/DROP（文件来源）",
	"file-unverified": "psql -f 文件无法验证（路径解析不出或不可读，无法证明无害即拦）",
};

function buildBlockReason(source: HitSource): string {
	return [
		`${GUARD_PREFIX} ${HIT_SOURCE_LABEL[source]}，已阻断破坏性 SQL（SQL_SAFETY_GUARD=hard）。拦截范围仅 TRUNCATE/DROP（DELETE/UPDATE/ALTER/INSERT 不拦）。`,
		"合法出路三选一：",
		"1) 事务包裹：`BEGIN; …; ROLLBACK;`（事务内验证后回滚，同现即自动放行）；",
		`2) 命令尾部加 \`${ESCAPE_TAG}\` 注释（显式授权，记 bypass 可回检）；`,
		"3) 设 `SQL_SAFETY_GUARD=off`（或 `soft`）关闭/降级本守卫。",
	].join("\n");
}

// ---------- 小工具 ----------

/** policy.decision 旁路记账（仿 test-scope-guard auditPolicy）：
 *  无 cwd/sessionId 上下文不记；记账异常忽略，绝不影响裁决 */
function auditPolicy(ctx: GuardCtx, action: "block" | "warn" | "bypass"): void {
	try {
		const sessionId = ctx.sessionManager?.getSessionId?.();
		if (!ctx.cwd || !sessionId) return;
		const decision: PolicyDecisionInput = {
			policy: POLICY_NAME,
			action,
			reasonCode: REASON_CODE,
		};
		logPolicyDecision(ctx.cwd, { sessionId, ...decision }); // change=undefined → 自动检测
	} catch (err) {
		console.warn(`${GUARD_PREFIX} policy.decision 记账异常（旁路忽略）：${String(err)}`);
	}
}

/** SQL_SAFETY_GUARD 解析：hard|soft|off，非法/缺省回退 hard（spec：非法配置按 hard 行为） */
function parseMode(v: string | undefined): GuardMode {
	return v === "soft" || v === "off" ? v : "hard";
}
