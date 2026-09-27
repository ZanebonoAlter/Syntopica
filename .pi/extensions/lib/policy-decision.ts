/**
 * policy-decision — 策略显著裁决统一记账 helper @Syntopica
 *
 * change: harden-harness-policy-and-spill（design D1/D2/D3）。
 *
 * 职责（只管记账，不管裁决）：
 *   - 为 spec-gate / quota-gate / test-scope-guard 提供统一的 policy.decision 事件收敛写入
 *   - action 固定四值：block | warn | bypass | fail-open（白名单外拒绝写入）
 *   - reasonCode 只接受稳定、非空、kebab-case 的有界代码；非法值归一为 unknown，
 *     绝不把自由文本（命令、密钥、远端响应、长错误）写进账本
 *   - target 仅允许 change/provider 等短摘要（截断 120 字符），禁止复制正文
 *   - change 归属：显式传参优先；undefined 时自动检测活跃 change；null = 明确不绑定
 *   - 严格旁路化（D3）：写入失败仅返回 false（logEvent 内部已 fail-loud console.error），
 *     调用方忽略返回值并继续原裁决，不得把记账成功作为 block/warn/fail-open 的前置条件
 *
 * 低噪声约束（D2）：普通成功放行/未命中/健康额度不得调用本 helper——事件分母不在此建。
 * 一次工具调用产生多个不同显著结果时，追加多条不同 action 的事件（append-only）。
 *
 * 用法：
 *   logPolicyDecision(cwd, { sessionId, policy: "spec-gate", action: "block",
 *     reasonCode: "archive-check-failed", target: "doc-impact,trace", change: name })
 */
import { logEvent } from "./harness-log";
import { detectActiveChange } from "./active-change";

export type PolicyAction = "block" | "warn" | "bypass" | "fail-open";

/** action 白名单（spec: 策略显著裁决统一记账） */
const POLICY_ACTIONS: readonly PolicyAction[] = [
	"block",
	"warn",
	"bypass",
	"fail-open",
];

/** reasonCode 合法形态：非空 kebab-case（小写字母/数字段，连字符分隔），长度 ≤64 */
const REASON_CODE_RE = /^[a-z0-9]+(-[a-z0-9]+)*$/;
const REASON_CODE_MAX = 64;
/** 非法 reasonCode 的归一代码：保留「裁决发生了」的事实，挡住自由文本 */
export const UNKNOWN_REASON_CODE = "unknown";
/** policy 稳定标识截断上限 */
const POLICY_MAX = 64;
/** target 短摘要截断上限 */
const TARGET_MAX = 120;

export interface PolicyDecisionInput {
	/** 稳定扩展标识，如 "spec-gate" */
	policy: string;
	action: PolicyAction;
	/** 稳定、非空、kebab-case 的有界代码，如 "archive-check-failed" */
	reasonCode: string;
	/** 有界短摘要（change/provider 等）；禁止命令、密钥、响应正文、长错误文本 */
	target?: string | null;
	/** 可选耗时（毫秒），非负 */
	durationMs?: number;
}

export interface LogPolicyDecisionArgs extends PolicyDecisionInput {
	sessionId: string;
	/** 显式绑定的 change；undefined = 自动检测活跃 change；null = 明确不绑定 */
	change?: string | null;
}

/** normalize 的失败原因（白名单外 action / policy 非字符串等结构性错误） */
export type NormalizedDecision =
	| { ok: true; payload: Record<string, unknown> }
	| { ok: false; error: string };

/**
 * 纯函数：把宽松输入收敛为有界 payload（白盒直测点）。
 * action 白名单外 / policy 非法 → ok:false（拒绝写入，不形成自由事件）；
 * reasonCode 非法 → 归一 unknown（干预事实优先于精确分类，防 producer 代码 bug 丢账）；
 * target 超长截断、非字符串省略；durationMs 非有限正数省略。
 */
export function normalizeDecision(
	d: PolicyDecisionInput,
): NormalizedDecision {
	if (!d || typeof d !== "object") return { ok: false, error: "decision 非对象" };
	if (!POLICY_ACTIONS.includes(d.action))
		return { ok: false, error: `action 白名单外：${String((d as { action?: unknown }).action)}` };
	if (typeof d.policy !== "string" || !d.policy.trim())
		return { ok: false, error: "policy 非空字符串" };

	const payload: Record<string, unknown> = {
		policy: clip(d.policy.trim(), POLICY_MAX),
		action: d.action,
		reasonCode:
			typeof d.reasonCode === "string" &&
			REASON_CODE_RE.test(d.reasonCode) &&
			d.reasonCode.length <= REASON_CODE_MAX
				? d.reasonCode
				: UNKNOWN_REASON_CODE,
	};
	if (typeof d.target === "string" && d.target.length > 0) {
		payload.target = clip(d.target, TARGET_MAX);
	}
	if (typeof d.durationMs === "number" && Number.isFinite(d.durationMs) && d.durationMs >= 0) {
		payload.durationMs = Math.round(d.durationMs);
	}
	return { ok: true, payload };
}

/**
 * 旁路记账：收敛后写入 events.db（kind=policy.decision，TTL 30 天）。
 * 返回 logEvent 结果（true=入账）；调用方必须忽略返回值、不得影响原裁决（D3）。
 */
export function logPolicyDecision(
	cwd: string,
	args: LogPolicyDecisionArgs,
): boolean {
	const normalized = normalizeDecision(args);
	if (!normalized.ok) {
		console.error(
			`[policy-decision] 拒绝记账：${normalized.error}（policy=${String(args.policy)}）`,
		);
		return false;
	}
	const change =
		args.change === undefined
			? (detectActiveChange(cwd)?.name ?? null)
			: args.change;
	return logEvent(cwd, {
		kind: "policy.decision",
		sessionId: args.sessionId,
		change,
		payload: normalized.payload,
	});
}

/** 截断（多字节安全按字符计），尾部加省略号标记 */
function clip(s: string, max: number): string {
	return s.length <= max ? s : `${s.slice(0, max)}…`;
}
