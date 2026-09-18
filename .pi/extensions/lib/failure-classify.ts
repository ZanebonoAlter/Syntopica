/**
 * failure-classify — 子线程失败白名单纯函数（A4 / design D4）
 *
 * 输入 tool_result(Agent) 失败现场的原始信号，产出有界结构化失败事实：
 *   failure = { stage, category, exitLike, diag }
 *
 * dsh 纪律：「诊断是展示文本不是协议，程序不得按其分支」——category 仅用于统计/展示，
 * 不构成任何控制流依据。
 *
 * - category 限定白名单枚举，按有序关键词表首中映射；映射不进一律落 unknown，
 *   原始错误文本不透传（仅 ≤512B 安全摘要 diag）
 * - stage 按证据判定：未观察到 tool_call 起点（如 quota-gate 拦截）→ dispatch；
 *   起点存在且执行失败 → run；details.status 显示 agent 已完成但结果装配失败 → result
 * - diag：首个非空错误行、剥控制字符、压成单行、≤512 字节（截断加 …）——
 *   gate.check 的 diag 复用同一规范（truncateDiag）
 */

export type FailureStage = "dispatch" | "run" | "result";
export type FailureCategory =
	| "quota-block"
	| "timeout"
	| "gate-fail"
	| "model-error"
	| "tool-error"
	| "unknown";

export interface FailureFact {
	stage: FailureStage;
	category: FailureCategory;
	/** 可提取的数字退出码，否则 null */
	exitLike: number | null;
	/** 首个非空错误行的单行截断（≤512 字节，剥控制字符） */
	diag: string;
}

/** 有序关键词表：首个命中胜出；不命中 → unknown（不复制原值） */
const CATEGORY_RULES: readonly (readonly [FailureCategory, RegExp])[] = [
	// quota-gate block reason 中文文案（"额度不足/剩余情况/窗口剩余/重置时间"）+ 英文 quota
	["quota-block", /额度|剩余|窗口|重置|quota|阻断/i],
	["timeout", /timeout|timed\s*out|超时|ETIMEDOUT/i],
	["gate-fail", /增量门禁|门禁未通过|quality.?gate/i],
	["model-error", /rate limit|429|\b5\d{2}\b|provider|overloaded|context (?:length|window)|api key/i],
	["tool-error", /exit (?:code )?\d|not found|permission denied|ENOENT|EACCES|no such file|command failed/i],
];

function classifyCategory(errorText: string): FailureCategory {
	for (const [category, re] of CATEGORY_RULES) {
		if (re.test(errorText)) return category;
	}
	return "unknown";
}

function extractExitLike(errorText: string): number | null {
	const m = errorText.match(/exit(?:\s+code)?\s*[:=]?\s*(\d{1,3})/i);
	return m ? Number.parseInt(m[1], 10) : null;
}

/** 单行规范化：剥 ANSI/控制字符、压空格、≤512 字节（截断加 …）。 */
function truncateLine(firstLine: string): string {
	const stripped = firstLine
		// ANSI 转义序列整体剥除（颜色码等；先于单控制字符，避免留下 [31m 残渣）
		.replace(/\x1b\[[0-9;]*[A-Za-z]/g, "")
		.replace(/[\x00-\x1f\x7f]/g, " ")
		.replace(/\s+/g, " ")
		.trim();
	// 字节级截断（中文一字 3B），预留 …（3B）
	let out = stripped;
	while (out.length > 0 && Buffer.byteLength(`${out}…`, "utf8") > 512) {
		out = out.slice(0, -1);
	}
	return out.length < stripped.length ? `${out}…` : out;
}

/** A4 截断规范：首个非空行、剥控制字符、压单行、≤512 字节（截断加 …）。 */
export function truncateDiag(text: string): string {
	const firstLine =
		text
			.split(/\r?\n/)
			.map((l) => l.trim())
			.find((l) => l.length > 0) ?? "";
	return truncateLine(firstLine);
}

/** gate.check 专用 diag 提取（harness-observability-fixes design D2）：失败特征行优先。
 *  门禁命令失败时 stdout 首行常是成功文案（golangci-lint "0 issues."、go test
 *  "? pkg [no test files]"），真实错误在 stderr 后续行——首个非空行策略会把
 *  噪声记进 DB，事后审计无法还原失败原因（events.db 2026-08-25 实测 9 连假象）。
 *  按有序关键词表取首个命中行，无命中回退首个非空行；截断规范复用 truncateDiag。
 *  仅 gate.check 记账路径使用；classifyFailure（子线程白名单）语义不同，继续用 truncateDiag。 */
const GATE_DIAG_RULES: readonly RegExp[] = [
	/FAIL/, // go test 失败锚点（FAIL pkg [build failed]）
	/\berror\b/i, // 编译/lint 错误行（含 eslint error）
	/^#\s+\S/, // Go 工具链错误锚点：# syntopica-backend/internal/topicgraph/service
	/\bexit\b/i, // exit 1 / exit code 2
	/\bundefined\b/i, // TS/JS undefined 引用
	/\bcannot\b/i, // cannot find package / cannot use
	/\bdenied\b/i, // permission denied / EACCES 语义行
];

export function truncateDiagGate(text: string): string {
	const lines = text
		.split(/\r?\n/)
		.map((l) => l.trim())
		.filter((l) => l.length > 0);
	const hit =
		lines.find((l) => GATE_DIAG_RULES.some((re) => re.test(l))) ?? lines[0] ?? "";
	return truncateDiag(hit);
}

/** WSL interop 层环境故障特征（harden-gate-interop-health design D3 双关键字并集锚定，
 *  任一命中即环境故障）：
 *  1. UtilAcceptVsock 关键字（accept4 failed 110 = ETIMEDOUT，vsock 通道死）；
 *  2. <N>WSL (pid - ) ERROR 行首前缀形态（WSL 注入的 stderr，仅 interop 未正常
 *     启动/通信时出现，不会与正常门禁输出混合——cmd.exe 成功启动后无此形态）。
 *  单形态随 WSL 版本漂移时另一形态仍可命中；两者全漂移则退化为既有行为（可考古）。
 *  样本：`<3>WSL (1751 - ) ERROR: UtilAcceptVsock:251: accept4 failed 110` */
const INTEROP_FAILURE_RE = /UtilAcceptVsock|^<\d>WSL \([^)]*\) ERROR/m;

/** 判定门禁命令输出是否为 WSL interop 环境故障（非代码问题）。
 *  供 quality-gate gateLog 分流：命中 → 不进粘性、归因环境；未命中 → 既有语义。 */
export function isInteropFailure(output: string): boolean {
	return INTEROP_FAILURE_RE.test(output ?? "");
}

/** native 模式工具链缺失特征（harden-gate-native-toolchain design D4）：
 *  bash 经 PATH 查找失败的标准文案 `bash: line 1: <exe>: command not found`
 *  （events.db 2026-09-17 事故实测形态，单次事故 945 条假失败）。
 *  仅 native 模式参与判定（quality-gate 侧门控）：windows 模式用 Windows 绝对路径执行，
 *  不经 bash PATH 查找，同名字符串不得触发跨语义环境归因——与 isInteropFailure 
 *  仅 windows 判定对称。两特征集正交（interop 特征不含 command not found，反之亦然），
 *  错发恢复建议（如 native 场景建议重启 WSL）由调用侧文案分流防。
 *  样本：`bash: line 1: go: command not found` */
const TOOL_NOT_FOUND_RE = /command not found/i;

/** 判定门禁命令输出是否为 native 工具链缺失（非代码问题）。
 *  供 quality-gate gateLog 分流：命中 → 不进粘性、归因环境（工具链缺失）；
 *  未命中 → 既有语义（真实代码失败照进粘性/分级）。 */
export function isToolNotFound(output: string): boolean {
	return TOOL_NOT_FOUND_RE.test(output ?? "");
}

function classifyStage(
	started: boolean | undefined,
	status: unknown,
): FailureStage {
	if (started == null) return "dispatch"; // 未观察到 tool_call 起点（如门禁拦截）
	const s = typeof status === "string" ? status.toLowerCase() : "";
	if (/complet|done|finish|success/.test(s)) return "result"; // agent 完成但结果装配失败
	return "run";
}

/**
 * 对一次失败的 Agent tool_result 做白名单分类。
 * 成功与用户取消不得调用本函数（telemetry 侧以 isError 守卫，产出端不出现 failure 对象）。
 */
export function classifyFailure(input: {
	/** 失败时的错误文本（tool_result.content 的 text 部分拼接）；非字符串按空处理 */
	errorText?: unknown;
	/** tool_result.details（读取 status 判定 result 态） */
	details?: { status?: unknown } | Record<string, unknown> | null;
	/** agentStarts 是否观察到本次 tool_call 起点；undefined = dispatch 态 */
	started?: boolean;
}): FailureFact {
	const errorText =
		typeof input.errorText === "string" ? input.errorText : "";
	const details = (input.details ?? {}) as { status?: unknown };
	return {
		stage: classifyStage(input.started, details.status),
		category: classifyCategory(errorText),
		exitLike: extractExitLike(errorText),
		diag: truncateDiag(errorText),
	};
}
