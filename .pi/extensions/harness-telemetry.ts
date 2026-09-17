/**
 * harness-telemetry — harness 事实库采集扩展 @Syntopica
 *
 * 迁移自源项目（快照 docs/research/harness-telemetry.ts），change: harness-facts-tier-a。
 * 设计文档：openspec/changes/harness-facts-tier-a/design.md（D1/D4）。
 *
 * 职责：挂 pi 事件钩子，把 harness 层事实写入事实库（模型零参与）：
 *   session_start              → session.start（会话锚点：reason/cwd）+ prev 回填
 *                               session.rollup 终值（harness-effectiveness-metrics：
 *                               prev jsonl 存在且账本无 final 快照时补写 final=true）
 *   turn_end                    → session.rollup 节流快照（每 5 turn 或 token 增量>20%；
 *                               轮次/步数/工具调用/token 五值/成本/时长/模型，final=false，
 *                               同 session 取最新一条即终值——查询语义对齐 edit.map）
 *   tool_call(Agent) 暂存起点
 *   tool_result(Agent)         → subagent.dispatch（类型/模型/摘要/耗时/token；失败附白名单 failure；
 *                               后台派发时暂存 agentId→change 绑定供完成回填复用）
 *   tool_result(get_subagent_result) → subagent.complete（后台子线程完成回填，
 *                               harness-observability-fixes D3：解析结果摘要首两行；
 *                               13/17 条 dispatch 永停 background 态的审计断链修复）
 *
 * 其余采集点在各自扩展内自报（logEvent 是共享唯一写入方）：
 *   constraint.inject / pin.write / pin.read → constraint-injection.ts
 *   gate.check                                 → quality-gate.ts
 *
 * 边界：
 *   - 只记 harness 层语义，pi session JSONL 已有的消息/参数明细不重复记
 *   - fail-loud：写入失败一次性 notify，不静默丢账
 *   - 子线程内部事件已实测不冒泡，dispatch 摘要覆盖父会话核心诉求
 */
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { logEvent, queryBySession } from "./lib/harness-log";
import { classifyFailure, type FailureFact } from "./lib/failure-classify";
import { detectActiveChange } from "./lib/active-change";
import {
	accumulateFromFile,
	newAccumulator,
	parseSessionText,
	shouldBackfillPrev,
	shouldWriteSnapshot,
	type RollupAccumulator,
} from "./lib/session-rollup";
import * as fs from "node:fs";
import * as path from "node:path";

/* 会话内状态（session_start 重置） */
let logFailNotified = false;
// Agent 派发起点暂存（toolCallId → 起始时间戳），tool_result 配对消费
const agentStarts = new Map<string, number>();
// 后台派发暂存 agentId → 当时 change 绑定（subagent.complete 复用，不重检测）
const agentChangeBindings = new Map<string, string | null>();
// 已回填完成的 agentId（幂等去重：同一 agent 至多一条 complete）
const completedAgents = new Set<string>();
// rollup 增量状态（session_start 重置）：jsonl 偏移 + 上次快照 token 基线 + turn 计数
let rollupAcc: RollupAccumulator = newAccumulator();
let rollupTurnCount = 0;
let rollupLastTokens: number | null = null;

/** 从 session jsonl 文件名提取 session UUID（形如 <ISO>_<UUID>.jsonl；UUID 含 '-'，
 *  ISO 段也含 '-'，故取最后一个 '_' 之后）。提不出返回 null。纯函数，smoke 直测。 */
export function sessionIdFromSessionFile(file: string): string | null {
	const base = path.basename(file).replace(/\.jsonl$/, "");
	const idx = base.lastIndexOf("_");
	if (idx <= 0) return null;
	const id = base.slice(idx + 1);
	return /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(id)
		? id
		: null;
}

function truncate(s: unknown, max: number): string {
	const t = typeof s === "string" ? s : "";
	return t.length > max ? `${t.slice(0, max)}…` : t;
}

/** Agent 工具 details.tokens 实测为 "37.4k"；规范化为整数，无法解析返回 null。 */
function parseTokenCount(value: unknown): number | null {
	if (typeof value === "number" && Number.isFinite(value)) return value;
	if (typeof value !== "string") return null;
	const m = value.trim().toLowerCase().replace(/,/g, "").match(/^([\d.]+)\s*([km])?/);
	if (!m) return null;
	const n = Number(m[1]);
	if (!Number.isFinite(n)) return null;
	const factor = m[2] === "m" ? 1_000_000 : m[2] === "k" ? 1_000 : 1;
	return Math.round(n * factor);
}

/** tool_result.content 的 text 部分拼接（A4 失败分类的原始信号；非文本内容丢弃） */
function errorTextOf(content: unknown): string {
	if (!Array.isArray(content)) return "";
	return content
		.filter((c): c is { type: "text"; text: string } => {
			const t = c as { type?: string } | null;
			return t?.type === "text";
		})
		.map((c) => c.text)
		.join("\n");
}

/** get_subagent_result 结果摘要解析（harness-observability-fixes D3）。
 *  实测形态（session JSONL 考古 2026-08-24）：
 *    "Agent: 7d1245e1-35ca-45a"
 *    "Type: Agent | Status: completed | Tool uses: 79 | 2.0M token | Context: 54% | Duration: 3944.8s"
 *  提不出 agentId / "Agent not found" / 非终态（running）→ 返回 null（不记账，fail-safe：
 *  pi 升级改格式时退化为现状而非误记账）。纯函数，smoke 直测。 */
export interface SubagentSummary {
	agentId: string;
	status: string;
	toolUses: number | null;
	tokens: number | null;
	ms: number | null;
}

export function parseSubagentSummary(text: string): SubagentSummary | null {
	if (/Agent not found|cleaned up/i.test(text)) return null;
	const idMatch = text.match(/^Agent:\s*([0-9a-f-]{6,})/m);
	if (!idMatch) return null;
	const agentId = idMatch[1];
	const statusMatch = text.match(/\bStatus:\s*(\w+)/);
	if (!statusMatch) return null;
	const status = statusMatch[1];
	if (status.toLowerCase() === "running") return null; // 非终态（wait=false 轮询）不算完成
	const toolUsesMatch = text.match(/Tool uses:\s*(\d+)/);
	const tokenMatch = text.match(/([\d.]+)\s*([KMkm])\s*token/); // "2.0M token"
	const durMatch = text.match(/Duration:\s*([\d.]+)s/);
	return {
		agentId,
		status,
		toolUses: toolUsesMatch ? Number.parseInt(toolUsesMatch[1], 10) : null,
		tokens: tokenMatch ? parseTokenCount(`${tokenMatch[1]}${tokenMatch[2]}`) : null,
		ms: durMatch ? Math.round(Number.parseFloat(durMatch[1]) * 1000) : null,
	};
}

/* fail-loud 一次性提示 */
function guardedLog(
	cwd: string,
	ctx: { hasUI?: boolean; ui?: { notify?: (m: string, k: string) => void } },
	ev: Parameters<typeof logEvent>[1],
): void {
	if (!logEvent(cwd, ev) && ctx.hasUI && !logFailNotified) {
		logFailNotified = true;
		ctx.ui?.notify?.(
			"⚠️ harness 事实库写入失败（事件丢失，详见日志）",
			"error",
		);
	}
}

export default function (pi: ExtensionAPI) {
	pi.on("session_start", async (event, ctx) => {
		logFailNotified = false;
		agentStarts.clear();
		agentChangeBindings.clear();
		completedAgents.clear();
		// rollup 状态重置：resume/重启后 offset 从 0 重读全量（rollup 是全会话累计口径，
		// 快照覆盖取最新一条即终值，重读只浪费一次 IO 不影响正确性）
		rollupAcc = newAccumulator();
		rollupTurnCount = 0;
		rollupLastTokens = null;
		const sessionId = ctx.sessionManager?.getSessionId?.();
		if (!sessionId) return;
		guardedLog(ctx.cwd, ctx, {
			kind: "session.start",
			sessionId,
			payload: {
				reason: event.reason,
				cwd: ctx.cwd,
				prev: event.previousSessionFile ?? null,
			},
		});

		// prev 回填（harness-effectiveness-metrics design D1②）：prev jsonl 仍存在且账本
		// 中该 session 无 final=true 快照时，全量解析补写终值。fail-open：任何失败仅旁路告警。
		const prevFile = event.previousSessionFile;
		if (prevFile && ctx?.cwd) {
			try {
				const prevId = sessionIdFromSessionFile(prevFile);
				if (prevId) {
				const rows = queryBySession(ctx.cwd, prevId, ["session.rollup"]);
				const hasFinal = rows.some(
					(r) => {
						try {
							return JSON.parse(r.payload).final === true;
					} catch {
						return false;
					}
					},
				);
				let text: string;
				try {
					text = fs.readFileSync(prevFile, "utf8");
				} catch {
					text = ""; // jsonl 缺失 → 零写入（spec「session_start 回填 prev 终值」）
				}
				if (text) {
					const summary = parseSessionText(text);
					if (shouldBackfillPrev(prevFile, summary, hasFinal)) {
						guardedLog(ctx.cwd, ctx, {
							kind: "session.rollup",
							sessionId: prevId,
							payload: { ...summary, final: true },
						});
					}
				}
			}
			} catch (e) {
				console.error(`[harness-telemetry] prev rollup backfill failed: ${(e as Error).message}`);
			}
		}
	});

	// session.rollup 节流快照（harness-effectiveness-metrics design D1①）：每 turn 增量读
	// 当前会话 jsonl（offset 推进，残行重读），节流命中才落库。时序容忍：当前 turn 的
	// assistant 消息可能尚未 flush——数值滞后一回合自然补齐，单调不减。
	pi.on("turn_end", async (_event, ctx) => {
		const sessionId = ctx.sessionManager?.getSessionId?.();
		if (!sessionId || !ctx?.cwd) return;
		const file = ctx.sessionManager?.getSessionFile?.();
		if (!file) return; // 路径不可得：跳过（design 风险清单）
		rollupTurnCount += 1;
		const acc = accumulateFromFile(rollupAcc, file);
		if (!acc) return; // 文件读失败：零写入 fail-open
		const total = acc.summary.tokens.total;
		if (!shouldWriteSnapshot(rollupTurnCount, rollupLastTokens, total)) return;
		rollupLastTokens = total;
		const s = acc.summary;
		guardedLog(ctx.cwd, ctx, {
			kind: "session.rollup",
			sessionId,
			change: detectActiveChange(ctx.cwd)?.name ?? null,
			payload: {
				turns: s.turns,
				steps: s.steps,
				toolCalls: s.toolCalls,
				tokens: s.tokens,
				cost: s.cost,
				durationSec: s.durationSec,
				model: s.model,
				final: false,
				...(s.skippedLines > 0 ? { skippedLines: s.skippedLines } : {}),
			},
		});
	});

	pi.on("tool_call", async (event, ctx) => {
		const sessionId = ctx.sessionManager?.getSessionId?.();
		if (!sessionId) return;

		// Agent 派发暂存起点，tool_result 配对后落 subagent.dispatch
		if (event.toolName === "Agent") {
			agentStarts.set(event.toolCallId, Date.now());
		}
	});

	pi.on("tool_result", async (event, ctx) => {
		// 后台子线程完成回填（harness-observability-fixes D3）：父线程取结果走
		// get_subagent_result，返回首两行是结构化摘要——解析后追加 subagent.complete
		// （append-only，既有 dispatch 行不改写）。解析不出/not found/running 不伪造（fail-safe）。
		if (event.toolName === "get_subagent_result") {
			const sessionId = ctx.sessionManager?.getSessionId?.();
			if (!sessionId) return;
			const summary = parseSubagentSummary(errorTextOf(event.content));
			if (!summary) return;
			if (completedAgents.has(summary.agentId)) return; // 幂等：至多一条 complete
			completedAgents.add(summary.agentId);
			const change = agentChangeBindings.get(summary.agentId) ?? null; // reload 丢 map 落 null（parent_session_id 兕底）
			guardedLog(ctx.cwd, ctx, {
				kind: "subagent.complete",
				sessionId,
				change,
				payload: {
					agentId: summary.agentId,
					status: summary.status, // completed/cancelled/error 等 pi 枚举原样透传
					ms: summary.ms,
					tokens: summary.tokens,
					toolUses: summary.toolUses,
					turnCount: null, // 结果摘要无此字段
					isError: event.isError === true || /error|fail/i.test(summary.status),
				},
			});
			return;
		}
		if (event.toolName !== "Agent") return;
		const sessionId = ctx.sessionManager?.getSessionId?.();
		if (!sessionId) return;
		const started = agentStarts.get(event.toolCallId);
		agentStarts.delete(event.toolCallId);

		const input = event.input as {
			subagent_type?: string;
			description?: string;
			model?: string;
			prompt?: string;
			run_in_background?: boolean;
		};
		// pi 的通用 ToolResultMessage 支持 usage；当前 Agent 工具实测 usage=null，
		// token/耗时放在 details.tokens（如 "37.4k"）/details.durationMs，故两路兼容。
		const usage = event.usage as
			| { totalTokens?: number; cost?: { total?: number } }
			| undefined;
		const details = event.details as
			| {
					tokens?: string | number;
					durationMs?: number;
					status?: string;
					agentId?: string;
					toolUses?: number;
					turnCount?: number;
			  }
			| undefined;
		const tokenLabel = details?.tokens ?? null;
		const tokens = usage?.totalTokens ?? parseTokenCount(tokenLabel);

		// 后台派发暂存 agentId → 当时 change 绑定（subagent.complete 复用，D3）
		if (details?.agentId) {
			agentChangeBindings.set(
				details.agentId,
				detectActiveChange(ctx.cwd)?.name ?? null,
			);
		}

		// A4 失败白名单：仅失败产出 failure（成功/取消不产出）；diag≤512B，原文不透传
		let failure: FailureFact | undefined;
		if (event.isError === true) {
			failure = classifyFailure({
				errorText: errorTextOf(event.content),
				details,
				started,
			});
		}

		guardedLog(ctx.cwd, ctx, {
			kind: "subagent.dispatch",
			sessionId,
			payload: {
				parent_session_id: sessionId,
				type: input.subagent_type ?? null,
				model: input.model ?? null,
				desc: truncate(input.description, 160),
				promptChars: input.prompt?.length ?? 0,
				background: input.run_in_background === true,
				ms: details?.durationMs ?? (started ? Date.now() - started : null),
				tokens,
				tokenLabel,
				cost: usage?.cost?.total ?? null,
				status: details?.status ?? null,
				agentId: details?.agentId ?? null,
				toolUses: details?.toolUses ?? null,
				turnCount: details?.turnCount ?? null,
				isError: event.isError === true,
				...(failure ? { failure } : {}),
			},
		});
	});
}
