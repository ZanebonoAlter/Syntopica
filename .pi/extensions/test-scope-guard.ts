/**
 * test-scope-guard.ts — pi extension：全量 go test 软守卫（默认 soft，不阻断）
 *
 * 设计决策（AGENTS.md 测试范围规则：日常只跑影响包，全量 go test ./... 属归档/pre-push 场景）：
 * 1. 挂 tool_call，仅 bash 命令命中 `go test[^#]*\.\.\.`（行内注释 # 之前出现 ./...）时介入，
 *    其余工具调用零开销直接放行
 * 2. 语境判定：命令本身或近期会话上下文（最近 15 条）含归档语境标记
 *    （归档|§11|archive|验证节|pre-push）→ 视为合法场景直接放行；拿不到会话上下文时
 *    只认命令内的 `# archive-gate` 注释（该注释含 "archive"，与语境标记判定天然兼容）
 * 3. soft（默认）：ctx.ui.notify 软提醒，绝不 block；hard：block，命令带 `# archive-gate`
 *    注释仍是显式逃生口；off：完全关闭
 * 4. soft 模式的提醒走 ctx.ui.notify（同 quota-gate 的 UI 提醒用法）；UI 不可用时静默降级，
 *    不改用阻断
 * 5. 显著裁决记 policy.decision（harden-harness-policy-and-spill）：soft 提醒 → warn、
 *    hard 阻断 → block（reasonCode 均为 full-go-test）；off/未命中/归档语境零记录
 *
 * 配置：TEST_SCOPE_GUARD=soft|hard|off（默认 soft，非法/缺省值回退 soft）。
 */
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { logPolicyDecision, type PolicyDecisionInput } from "./lib/policy-decision";

type GuardMode = "soft" | "hard" | "off";

/** 守卫模式（环境变量 TEST_SCOPE_GUARD） */
const MODE: GuardMode = parseMode(process.env.TEST_SCOPE_GUARD);
/** 全量 go test 识别：`go test` 到行内注释 `#` 前出现 `./...` */
const FULL_GO_TEST_RE = /go test[^#]*\.\.\./;
/** 归档语境标记：命令或近期会话上下文命中即视为合法场景 */
const ARCHIVE_CONTEXT_RE = /归档|§11|archive|验证节|pre-push/;
/** 显式归档注释：hard 模式下的逃生口 */
const ARCHIVE_TAG = "# archive-gate";
/** 语境判定扫描的最近会话条数（控制开销） */
const CONTEXT_WINDOW = 15;
/** 软提醒文案（AGENTS.md 测试范围规则） */
const SOFT_NOTICE =
	"全量 go test 属归档/pre-push 场景；日常请只跑影响包（AGENTS.md 测试范围规则）。确属归档场景请在命令尾部加 `# archive-gate` 注释。";

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

// ---------- 主入口 ----------

export default function (pi: ExtensionAPI) {
	pi.on("tool_call", async (event, ctx) => {
		if (MODE === "off") return;
		if (event.toolName !== "bash") return;
		const input = (event.input ?? {}) as Record<string, unknown>;
		const command = typeof input.command === "string" ? input.command : "";
		if (!FULL_GO_TEST_RE.test(command)) return;

		// 1. 语境判定：显式注释 / 命令或近期会话上下文含归档标记 → 合法场景直接放行
		if (
			command.includes(ARCHIVE_TAG) ||
			ARCHIVE_CONTEXT_RE.test(command) ||
			hasArchiveContext(ctx)
		) {
			console.log("[test-scope-guard] 归档/pre-push 语境，全量 go test 放行");
			return;
		}

		// 2. 非归档语境：hard 才 block（soft 绝不 block）
		if (MODE === "hard") {
			console.warn("[test-scope-guard] 非归档语境的全量 go test 已阻断（hard 模式）");
			auditPolicy(ctx, "block");
			return {
				block: true,
				reason: [
					"全量 go test 属归档/pre-push 场景，日常请只跑影响包（AGENTS.md 测试范围规则），本次命令已阻断（TEST_SCOPE_GUARD=hard）。",
					"建议：改为只跑本次修改影响的包（如 go test ./internal/domain/xxx）。",
					`确属归档/pre-push 场景：命令尾部加 \`${ARCHIVE_TAG}\` 注释后重跑，或设 TEST_SCOPE_GUARD=soft/off。`,
				].join("\n"),
			};
		}

		// 3. soft：只提醒不阻断（UI 提醒用法同 quota-gate；不可用时静默）
		console.warn("[test-scope-guard] 非归档语境跑全量 go test，已软提醒");
		auditPolicy(ctx, "warn");
		try {
			ctx.ui.notify(`[test-scope-guard] ${SOFT_NOTICE}`, "warning");
		} catch {
			// UI 不可用时静默（soft 模式本就不阻断）
		}
		return;
	});
}

// ---------- 语境判定 ----------

/** 近期会话上下文是否含归档语境标记；拿不到上下文（无 sessionManager/空会话/异常）返回 false */
function hasArchiveContext(ctx: GuardCtx): boolean {
	try {
		const sm = ctx.sessionManager;
		const entries = sm?.buildContextEntries?.() ?? sm?.getEntries?.();
		if (!Array.isArray(entries) || entries.length === 0) return false;
		const recent = entries.slice(-CONTEXT_WINDOW);
		// 条目结构随版本演进（message/custom_message/...），整体序列化后做标记匹配最稳
		return ARCHIVE_CONTEXT_RE.test(recent.map((e) => JSON.stringify(e)).join("\n"));
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
		console.warn(`[test-scope-guard] policy.decision 记账异常（旁路忽略）：${String(err)}`);
	}
}

/** TEST_SCOPE_GUARD 解析：soft|hard|off，非法/缺省回退 soft */
function parseMode(v: string | undefined): GuardMode {
	return v === "hard" || v === "off" ? v : "soft";
}
