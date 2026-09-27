/**
 * test-scope-guard.ts — pi extension：全量 go test 软守卫（默认 soft，不阻断）
 *
 * 设计决策（change: harden-test-scope-guard；AGENTS.md 测试范围规则：日常只跑影响包）：
 * 1. 判定纯函数 classifyTestRun(command, sessionCwd, repoRoot) → full|scoped|none，四步分层：
 *    ① `-c` 结构抽取（在原始文本上、先于掩蔽；单/双引号定界、支持 \ 转义；payload 递归走
 *    全部分层，并继承 `-c` 调用点之前解析出的 cwd）；② 文本掩蔽（引号内 / 行内 # 注释 /
 *    heredoc 正文 / cat 重定向目标，等长空格占位、保留换行；未闭合引号与未闭合 heredoc
 *    均保守掩至串尾）；③ 段切分与前缀剥离（;/&&/||/|/换行/(/)；timeout/env/nice/command/X=Y）；
 *    ④ 包模式（裸 `./...` 或 `...` token）+ 生效 cwd（段前最后一个字面 cd 解析，必须等于
 *    <repoRoot>/backend-go）。只有"裸 ./... + backend-go 根 cwd"算全量。
 * 2. 接受的漏报边界（design D7，宁漏报不误伤）：包模式来自 shell 变量/循环展开/命令替换
 *    （for d in $DOMAINS; do go test ./$d/...）、eval、变量化 cd 时判不出 → 按放行处理。
 * 3. 扫描通道：bash + ctx_execute（仅 language==="shell"，其余值不解析）+ ctx_batch_execute
 *    （逐 commands[].command）；quality-gate 等扩展内部 pi.exec 不经过 tool_call，天然边界。
 * 4. 豁免判定作用于原始命令文本（掩蔽前，注释里的 tag 不能被剥掉）：显式逃生注释
 *    （# archive-gate / # allow-full-test 两别名等价）、命令或最近 15 条会话条目命中
 *    /归档|§11|archive|验证节|pre-push|依赖变更|建议全量/（依赖变更为 change-scope.sh
 *    在 go.mod/go.sum 变更时的官方建议语）。守卫自身注入的 reason/notice 统一携带
 *    [test-scope-guard] 前缀，语境扫描过滤含该前缀的条目——防止首次 block 后同会话
 *    经自身文案静默豁免的自噬回路（TC-B8-07）。
 * 5. soft（默认）：ctx.ui.notify 软提醒，绝不 block；hard：block（reason 给出两条合法
 *    路径与两个逃生标签）；off：完全关闭。UI 不可用时静默降级。放行语境命中附
 *    test-patrol 登记指引（info；归档与依赖变更同分支）。
 * 6. 显著裁决记 policy.decision（harden-harness-policy-and-spill）：soft 提醒 → warn、
 *    hard 阻断 → block（reasonCode 均为 full-go-test）；off/未命中/豁免零记账。
 *
 * 配置：TEST_SCOPE_GUARD=soft|hard|off（默认 soft，非法/缺省值回退 soft）。
 */
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { logPolicyDecision, type PolicyDecisionInput } from "./lib/policy-decision";

type GuardMode = "soft" | "hard" | "off";

/** 守卫模式（环境变量 TEST_SCOPE_GUARD） */
const MODE: GuardMode = parseMode(process.env.TEST_SCOPE_GUARD);
/** 放行语境标记：命令原始文本或近期会话条目命中即视为合法场景（归档 + 依赖变更全量） */
const EXEMPT_CONTEXT_RE = /归档|§11|archive|验证节|pre-push|依赖变更|建议全量/;
/** 显式逃生注释（两别名等价，hard 模式逃生口） */
const ARCHIVE_TAG = "# archive-gate";
const ALLOW_TAG = "# allow-full-test";
/** 守卫自身文案统一前缀：语境扫描过滤含该前缀的条目（防自噬豁免，design D5） */
const GUARD_PREFIX = "[test-scope-guard]";
/** 语境判定扫描的最近会话条数（控制开销） */
const CONTEXT_WINDOW = 15;
/** `-c` 递归深度上限（防极端嵌套，超限保守放行） */
const CLASSIFY_DEPTH_LIMIT = 8;
/** go test 中「带独立取值参数」的 flag（下一 token 是取值，非包模式） */
const VALUE_FLAGS = new Set([
	"-run", "-count", "-timeout", "-tags", "-ldflags", "-bench", "-covermode",
	"-coverpkg", "-exec", "-o", "-mod", "-skip", "-fuzz", "-cpu", "-parallel",
	"-blockprofile", "-cpuprofile", "-memprofile", "-outputdir", "-coverprofile",
]);
/** 软提醒文案（两条合法路径 + 两个逃生标签） */
const SOFT_NOTICE =
	`${GUARD_PREFIX} 全量 go test 属两条合法场景之一：归档/pre-push 门禁，或 go.mod/go.sum 依赖变更后的全量验证；` +
	"日常请只跑影响包（AGENTS.md 测试范围规则）。确属上述场景请在命令尾部加 `# archive-gate` 或 `# allow-full-test` 注释。";
/** 放行语境命中时的记账指引（test-debt-patrol：撞见非本 change 红 → 登记台账；归档与依赖变更同分支） */
const EXEMPT_DEBT_NOTICE =
	`${GUARD_PREFIX} 全量验证撞见非本 change 引起的红测试（本 change 影响包之外）→ ` +
	"用 `bash scripts/harness/test-patrol.sh --register <test_id> --context <change名>` 登记台账后继续（本 change 自身的红仍须修复）；" +
	"台账汇总看 `bash scripts/harness/test-patrol.sh --report`。";

/** 本扩展只用到的 ExtensionContext 子集（结构兼容，便于独立类型检查，见 quota-gate GateCtx） */
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

/** resolveRepoRoot 依赖的 pi.exec 最小结构面（ExtensionAPI 结构兼容） */
type ExecLike = {
	exec(command: string, args: string[], options?: { cwd?: string; timeout?: number }): Promise<{ stdout?: string }>;
};

// ---------- 主入口 ----------

export default function (pi: ExtensionAPI) {
	pi.on("tool_call", async (event, ctx) => {
		if (MODE === "off") return;
		const input = (event.input ?? {}) as Record<string, unknown>;
		const commands = extractCommands(event.toolName, input);
		if (commands.length === 0) return;
		// 零开销粗筛：既无 `go test` 也无 `...` 的文本不可能命中全量
		const candidates = commands.filter((c) => c.includes("go test") && c.includes("..."));
		if (candidates.length === 0) return;

		const cwd = typeof ctx.cwd === "string" ? ctx.cwd : "";
		const repoRoot = cwd ? await resolveRepoRoot(pi, cwd) : null;
		let hit: string | null = null;
		for (const c of candidates) {
			if (classifyTestRun(c, cwd, repoRoot ?? "") === "full") {
				hit = c;
				break;
			}
		}
		if (!hit) return;

		// 1. 豁免判定（作用于原始命令文本，掩蔽前）：逃生注释 / 命令自身语境 / 近期会话语境
		if (
			hit.includes(ARCHIVE_TAG) ||
			hit.includes(ALLOW_TAG) ||
			EXEMPT_CONTEXT_RE.test(hit) ||
			hasExemptContext(ctx)
		) {
			console.log(`${GUARD_PREFIX} 放行语境命中（归档/pre-push 或依赖变更全量），放行`);
			try {
				ctx.ui.notify(EXEMPT_DEBT_NOTICE, "info");
			} catch {
				// UI 不可用时静默降级，不影响放行
			}
			return;
		}

		// 2. 非豁免语境：hard 才 block（soft 绝不 block）
		if (MODE === "hard") {
			console.warn(`${GUARD_PREFIX} 非豁免语境的全量 go test 已阻断（hard 模式）`);
			auditPolicy(ctx, "block");
			return {
				block: true,
				reason: [
					`${GUARD_PREFIX} 全量 go test 仅两条合法场景：归档/pre-push 门禁、go.mod/go.sum 依赖变更后的全量验证；日常请只跑影响包（AGENTS.md 测试范围规则），本次命令已阻断（TEST_SCOPE_GUARD=hard）。`,
					"建议：改为只跑本次修改影响的包（如 go test ./internal/domain/xxx）。",
					`确属上述场景：命令尾部加 \`${ARCHIVE_TAG}\` 或 \`${ALLOW_TAG}\` 注释后重跑，或设 TEST_SCOPE_GUARD=soft/off。`,
				].join("\n"),
			};
		}

		// 3. soft：只提醒不阻断（UI 提醒用法同 quota-gate；不可用时静默）
		console.warn(`${GUARD_PREFIX} 非豁免语境跑全量 go test，已软提醒`);
		auditPolicy(ctx, "warn");
		try {
			ctx.ui.notify(SOFT_NOTICE, "warning");
		} catch {
			// UI 不可用时静默（soft 模式本就不阻断）
		}
		return;
	});
}

// ---------- 通道扩展：按 toolName 分派取命令文本（design D4） ----------

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

// ---------- 判定纯函数（design D1 四步分层） ----------

export type TestRunVerdict = "full" | "scoped" | "none";

/** 判定命令是否真·全量后端测试：裸 `./...`/`...` + 生效 cwd === <repoRoot>/backend-go。
 *  纯函数、无 IO、无事件依赖；任何解析失败一律 `none`（宁漏报不误伤）。 */
export function classifyTestRun(command: string, sessionCwd: string, repoRoot: string): TestRunVerdict {
	if (!command || !sessionCwd || !repoRoot) return "none";
	try {
		return classifyLevel(command, sessionCwd, repoRoot, 0);
	} catch {
		return "none";
	}
}

/** 单层判定：① -c 原文抽取递归 → ② 掩蔽 → ③④ 切段扫描（cd 追踪 + 候选段判定） */
function classifyLevel(text: string, baseCwd: string, repoRoot: string, depth: number): TestRunVerdict {
	if (depth > CLASSIFY_DEPTH_LIMIT) return "none";
	for (const { payload, index } of extractDashC(text)) {
		// `-c` 调用点之前的文本段解析继承 cwd；解析失败保守放行
		const prefixScan = scanSegments(maskShellText(text.slice(0, index)), baseCwd, repoRoot);
		if (prefixScan.cwdBroken) return "none";
		if (classifyLevel(payload, prefixScan.cwd, repoRoot, depth + 1) === "full") return "full";
	}
	return scanSegments(maskShellText(text), baseCwd, repoRoot).verdict;
}

// ---------- ① `-c` 结构抽取（原始文本，先于掩蔽） ----------

/** `bash|sh|zsh -c '<payload>'`（单/双引号定界、支持 \ 转义）；未闭合引号不抽取（保守） */
const DASH_C_RE = /\b(?:bash|sh|zsh)[ \t]+-c[ \t]*(['"])((?:\\.|(?!\1).)*)\1/g;

function extractDashC(text: string): { payload: string; index: number }[] {
	const out: { payload: string; index: number }[] = [];
	for (const m of text.matchAll(DASH_C_RE)) {
		out.push({ payload: m[2], index: m.index });
	}
	return out;
}

// ---------- ② 文本掩蔽（等长空格占位，保留换行与结构位置） ----------

/** 趟序固定：heredoc 正文 → 引号内 → 行内 # 注释 → cat 重定向目标。
 *  占位 = 空格（token 化后自然消失，不产生假 `#`/假引号边界）。 */
function maskShellText(text: string): string {
	let out = maskHeredoc(text);
	out = maskQuotes(out);
	out = maskLineComments(out);
	out = maskCatRedirectTargets(out);
	return out;
}

/** `<<[>-]? DELIM`：正文从其后首个换行起掩至「整行等于 DELIM」的闭合行（含），未闭合掩至串尾 */
const HEREDOC_RE = /<<-?[ \t]*(['"]?)([A-Za-z_][A-Za-z0-9_]*)\1/g;

function maskHeredoc(text: string): string {
	const chars = text.split("");
	for (const m of text.matchAll(HEREDOC_RE)) {
		const delim = m[2];
		const nl = text.indexOf("\n", m.index + m[0].length);
		if (nl === -1) continue; // delimiter 后无正文（无换行）——无可掩正文
		const closeRe = new RegExp(`^[ \\t]*${escapeRegExp(delim)}[ \\t]*$`, "m");
		const cm = closeRe.exec(text.slice(nl + 1));
		const end = cm ? nl + 1 + cm.index + cm[0].length : text.length;
		for (let i = nl + 1; i < end; i++) if (chars[i] !== "\n") chars[i] = " ";
	}
	return chars.join("");
}

/** 单/双引号内文本掩为空格（引号界定符保留；引号内支持 \ 转义；未闭合掩至串尾） */
function maskQuotes(text: string): string {
	const chars = text.split("");
	let state: "n" | "s" | "d" = "n";
	for (let i = 0; i < chars.length; i++) {
		const c = chars[i];
		if (state === "n") {
			if (c === "\\") { i++; continue; } // normal 下 \x 两字符原样（转义不开启引号）
			if (c === "'") state = "s";
			else if (c === '"') state = "d";
			continue;
		}
		if (c === "\\") { // 引号内 \x：两字符均掩，转义不闭合引号
			chars[i] = " ";
			if (i + 1 < chars.length) chars[i + 1] = " ";
			i++;
			continue;
		}
		if ((state === "s" && c === "'") || (state === "d" && c === '"')) {
			state = "n";
			continue;
		}
		if (c !== "\n") chars[i] = " "; // 未闭合引号：其后全部视为引号内（换行保留，保守掩至串尾）
	}
	return chars.join("");
}

/** 行内 # 之后掩至行尾（引号内 # 已在上一趟被掩，不会误判） */
function maskLineComments(text: string): string {
	const chars = text.split("");
	for (let i = 0; i < chars.length; i++) {
		if (chars[i] !== "#") continue;
		for (let j = i; j < chars.length && chars[j] !== "\n"; j++) chars[j] = " ";
	}
	return chars.join("");
}

/** `cat` / `cat >>` 的重定向目标（同段首个 > / >> 后的非空白 token）掩为空格 */
const CAT_REDIRECT_RE = /\bcat\b[^\n;&|()]*?(>>?)[ \t]*([^ \t;&|()<>\n]+)/g;

function maskCatRedirectTargets(text: string): string {
	return text.replace(CAT_REDIRECT_RE, (match, _gt, target: string) =>
		match.slice(0, match.length - target.length) + " ".repeat(target.length),
	);
}

function escapeRegExp(s: string): string {
	return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

// ---------- ③④ 段切分、前缀剥离、包模式 + cwd ----------

/** 切分符：; && || | 换行 ( )（&&/|| 的重复字符切出空段，跳过即可；`2>&1` 切段无害） */
const SEG_SPLIT_RE = /[;|&\n()]/;
/** X=Y 形式的前置变量赋值 */
const ASSIGN_RE = /^[A-Za-z_][A-Za-z0-9_]*=/;
/** timeout 的时长参数形态（600 / 1.5s / 10m） */
const DURATION_RE = /^\d+(\.\d+)?[smhd]?$/;

type ScanResult = { verdict: TestRunVerdict; cwd: string; cwdBroken: boolean };

/** 线性扫描掩蔽产物：追踪最后一个字面 `cd` 更新生效 cwd；候选段（go test + 裸 ./...）
 *  按 cwd 是否等于 <repoRoot>/backend-go 判 full/scoped；cd 解析失败 → cwdBroken（保守 none） */
function scanSegments(masked: string, sessionCwd: string, repoRoot: string): ScanResult {
	const backendRoot = normalizePath(repoRoot + "/backend-go");
	let cwd = normalizePath(sessionCwd);
	let cwdBroken = false;
	let verdict: TestRunVerdict = "none";
	for (const rawSeg of masked.split(SEG_SPLIT_RE)) {
		const tokens = rawSeg.split(/[ \t]+/).filter(Boolean);
		if (tokens.length === 0) continue;
		const stripped = stripPrefixes(tokens);
		if (stripped.length === 0) continue;
		if (stripped[0] === "cd") {
			// `cd` 无参（回 HOME，cwd 不可知）或目标被掩/含变量残片 → 解析失败，保守放行
			const target = stripped.length >= 2 ? resolvePath(cwd, stripped[1]) : null;
			if (!target) {
				cwdBroken = true;
				continue;
			}
			cwd = target;
			continue;
		}
		if (stripped[0] === "go" && stripped[1] === "test") {
			if (hasBarePattern(stripped.slice(2))) {
				if (!cwdBroken && normalizePath(cwd) === backendRoot) verdict = "full";
				else if (verdict !== "full") verdict = "scoped";
			} else if (verdict === "none") {
				verdict = "scoped"; // 无包参数（等同当前目录单包）或子树/单包 → 非全量
			}
		}
	}
	if (cwdBroken) return { verdict: "none", cwd, cwdBroken };
	return { verdict, cwd, cwdBroken };
}

/** 剥离前置修饰：timeout <dur>、env、nice（含 -n N / -N）、command、X=Y 赋值（循环直至首词为真实命令） */
function stripPrefixes(tokens: string[]): string[] {
	let t = tokens.slice();
	for (;;) {
		if (t.length === 0) return t;
		const h = t[0];
		if (h === "timeout") {
			t = t.slice(1);
			if (t.length > 0 && DURATION_RE.test(t[0])) t = t.slice(1);
			continue;
		}
		if (h === "nice") {
			t = t.slice(1);
			if (t[0] === "-n" && t.length >= 2) t = t.slice(2);
			else if (t.length > 0 && /^-\d+$/.test(t[0])) t = t.slice(1);
			continue;
		}
		if (h === "env" || h === "command") {
			t = t.slice(1);
			continue;
		}
		if (ASSIGN_RE.test(h)) {
			t = t.slice(1);
			continue;
		}
		return t;
	}
}

/** 包参数中是否存在裸 `./...` / `...` token（跳过 flag 及其取值；子树/单包不算） */
function hasBarePattern(args: string[]): boolean {
	let bare = false;
	for (let i = 0; i < args.length; i++) {
		const t = args[i];
		if (t.startsWith("-")) {
			if (t.includes("=")) continue; // -flag=value 自带取值
			if (VALUE_FLAGS.has(t) && i + 1 < args.length) i++; // -flag value：取值一并跳过
			continue;
		}
		if (t === "./..." || t === "...") bare = true;
	}
	return bare;
}

/** 相对会话/继承 cwd 解析 cd 目标并规整；目标为空或含变量/引号残片 → null（解析失败） */
function resolvePath(base: string, target: string): string | null {
	if (!target || /[$'"]/.test(target)) return null;
	if (target.startsWith("/")) return normalizePath(target);
	return normalizePath(base + "/" + target);
}

/** posix 路径规整：处理 `.`、`..`、重复与尾随斜杠 */
function normalizePath(p: string): string {
	const isAbs = p.startsWith("/");
	const out: string[] = [];
	for (const part of p.split("/")) {
		if (!part || part === ".") continue;
		if (part === "..") {
			if (out.length > 0 && out[out.length - 1] !== "..") out.pop();
			else if (!isAbs) out.push("..");
			continue;
		}
		out.push(part);
	}
	const joined = out.join("/");
	return isAbs ? "/" + joined : joined || "/";
}

// ---------- 仓库根解析（quality-gate 同源：git rev-parse --show-toplevel，按 cwd 会话内缓存） ----------

let repoRootCache: { cwd: string; root: string | null } | null = null;

async function resolveRepoRoot(pi: ExecLike, cwd: string): Promise<string | null> {
	if (repoRootCache && repoRootCache.cwd === cwd) return repoRootCache.root;
	let root: string | null = null;
	try {
		const r = await pi.exec("git", ["rev-parse", "--show-toplevel"], { cwd, timeout: 10_000 });
		const out = r.stdout?.trim();
		root = out ? out : null;
	} catch {
		root = null; // 非 git 仓库 / git 不可用 → 判定退化 none（放行侧）
	}
	repoRootCache = { cwd, root };
	return root;
}

// ---------- 豁免语境判定 ----------

/** 近期会话上下文是否含放行语境标记；守卫自身文案（含 GUARD_PREFIX 前缀）的条目被过滤——
 *  防止首次 block 后经自身 reason 静默豁免的自噬回路；拿不到上下文时返回 false */
function hasExemptContext(ctx: GuardCtx): boolean {
	try {
		const sm = ctx.sessionManager;
		const entries = sm?.buildContextEntries?.() ?? sm?.getEntries?.();
		if (!Array.isArray(entries) || entries.length === 0) return false;
		const recent = entries
			.slice(-CONTEXT_WINDOW)
			.filter((e) => !JSON.stringify(e).includes(GUARD_PREFIX));
		if (recent.length === 0) return false;
		return EXEMPT_CONTEXT_RE.test(recent.map((e) => JSON.stringify(e)).join("\n"));
	} catch {
		return false;
	}
}

// ---------- 小工具 ----------

/** policy.decision 旁路记账（change: harden-harness-policy-and-spill D2/D3）：
 *  无 cwd/sessionId 上下文不记；记账异常忽略，绝不影响裁决 */
function auditPolicy(ctx: GuardCtx, action: "warn" | "block"): void {
	try {
		const sessionId = ctx.sessionManager?.getSessionId?.();
		if (!ctx.cwd || !sessionId) return;
		const decision: PolicyDecisionInput = {
			policy: "test-scope-guard",
			action,
			reasonCode: "full-go-test",
		};
		logPolicyDecision(ctx.cwd, { sessionId, ...decision }); // change=undefined → 自动检测
	} catch (err) {
		console.warn(`${GUARD_PREFIX} policy.decision 记账异常（旁路忽略）：${String(err)}`);
	}
}

/** TEST_SCOPE_GUARD 解析：soft|hard|off，非法/缺省回退 soft */
function parseMode(v: string | undefined): GuardMode {
	return v === "hard" || v === "off" ? v : "soft";
}
