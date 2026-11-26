/**
 * router-discipline —— 主线程「该派出去就派出去」的行为信号软提醒（ISS-06 / ISS-02 讨论）
 *
 * 背景：主会话当 Router 用是仓库纪律（AGENTS.md），但它是**靠自觉**的。可核对的事实是
 * 计数：一次会话主线程 995 次工具调用 vs 25 次派发（探索与实现派得极少）。
 * （早期草稿写过「$44.96 vs $0.015 差 2600 倍」——那是两个不同口径的数相除，已按 review R-3
 *  撤回：`session.rollup.models` 是主线程自己切模型的聚合，`subagent.dispatch` 的 cost 全为 null。）
 * 违规集中在两处可观测行为上：
 *   ① 单次 read 大文件（>200 行）——规范限「约 200 行内定点核对」
 *   ② 同一文件连续 edit（≥3 次）——说明进了「改—跑—挂—改」调试循环
 *
 * 本扩展只在上述信号出现时 steer 一句提醒，**不阻断、不改裁决**——判据是行为信号，
 * 不是"该不该派"的主观判断。真正该派的活派不派仍由模型决定，但至少它会在当下上下文里
 * 看到这句话，而不是埋在 800 行文档里。
 *
 * 设计纪律：
 * - 只观测、不阻断（软提醒；与「靠自觉 vs 靠机制」那条教训呼应——但硬阻断会误伤小修）
 * - 每会话每类信号各提醒一次（低噪声，避免刷屏）
 * - 全程 fail-open：扩展自身异常绝不影响工具执行
 * - 记录 policy.decision（policy=router-discipline, action=warn）供 retro 回检
 */
import { existsSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import {
	logPolicyDecision,
} from "./lib/policy-decision";
import { isChildSession } from "./lib/child-session";

const READ_LINE_LIMIT = 200; // 规范「约 200 行内定点核对」
const EDIT_STREAK_LIMIT = 3; // 同一文件连续 edit 次数

interface SessionNudgeState {
	readWarned: boolean;
	editWarned: boolean;
	/** 最近连续 edit 的同一路径 → 次数（换文件即重置为该文件 1） */
	editStreak: Map<string, number>;
}

const states = new Map<string, SessionNudgeState>();

/**
 * 豁免（scripts/harness/router-discipline.sh --excuse "<理由>" 写入）：
 * 读到即静音本会话提醒，并把理由记入事实库（action=bypass）——留痕可回检，
 * 不是静默跳过。每个 session 只记一次账。
 */
function readExcuse(cwd: string, sessionId: string): string | null {
	try {
		const f = join(cwd, ".pi/tmp/router-discipline", `${sessionId}.json`);
		if (!existsSync(f)) return null;
		const parsed = JSON.parse(readFileSync(f, "utf8")) as { reason?: unknown };
		const reason = typeof parsed.reason === "string" ? parsed.reason.trim() : "";
		return reason.length > 0 ? reason : null;
	} catch {
		return null;
	}
}
const excusedLogged = new Set<string>();

function stateFor(sessionId: string): SessionNudgeState {
	let st = states.get(sessionId);
	if (!st) {
		st = { readWarned: false, editWarned: false, editStreak: new Map() };
		states.set(sessionId, st);
	}
	return st;
}

/** 目标文件的真实行数（相对 cwd 解析；读不到返回 null = 按超限处理，宁可误提醒） */
function countLines(cwd: string, p: unknown): number | null {
	try {
		if (typeof p !== "string" || p.length === 0) return null;
		const abs = p.startsWith("/") ? p : join(cwd, p);
		const st = statSync(abs);
		if (!st.isFile()) return null;
		// 只统计到 READ_LINE_LIMIT+1 行即停，避免大文件全量扫
		const buf = readFileSync(abs, "utf8");
		let n = 0;
		for (let i = 0; i < buf.length; i++) {
			if (buf.charCodeAt(i) === 10) {
				n++;
				if (n > READ_LINE_LIMIT) return n;
			}
		}
		return n + (buf.length > 0 ? 1 : 0);
	} catch {
		return null;
	}
}

/** steer 一句（沿用 ui-design-gate 的 custom_message 模式，不打断回合） */
function nudge(pi: any, content: string): void {
	try {
		pi.sendMessage(
			{ customType: "router-discipline-nudge", content, display: true },
			{ deliverAs: "steer" },
		);
	} catch (err) {
		console.warn(`[router-discipline] steer 落盘失败：${String(err)}`);
	}
}

export default function (pi: any) {
	// read 大文件：tool_call 时按 offset/limit 估算"这次要读多少行"
	pi.on("tool_call", async (event: any, ctx: any) => {
		try {
			const sessionId = ctx.sessionManager?.getSessionId?.();
			if (!sessionId) return;
			// 子会话（explore / general-purpose / change-review 等）本来就该干活——
			// 对它们喊「你该派子线程」是荒谬的，且会污染子线程上下文（review R-2）。
			if (isChildSession(ctx)) return;
			const st = stateFor(sessionId);

			// 豁免优先：有理由即静音本会话全部提醒（记账一次，action=bypass）
			const excuse = readExcuse(ctx.cwd ?? process.cwd(), sessionId);
			if (excuse) {
				if (!excusedLogged.has(sessionId)) {
					excusedLogged.add(sessionId);
					try {
						logPolicyDecision(ctx.cwd ?? process.cwd(), {
							sessionId,
							policy: "router-discipline",
							action: "bypass",
							reasonCode: "router-excused",
							target: excuse, // 白名单只保留 target；理由文本走这里
						});
					} catch {
						/* 记账失败不改变行为 */
					}
				}
				return;
			}

			if (event.toolName === "read") {
				const input = (event.input ?? {}) as {
					path?: unknown;
					limit?: unknown;
					offset?: unknown;
				};
				const limit = typeof input.limit === "number" ? input.limit : null;
				// 请求的 limit 优先；未指定 limit 时**查真实行数**（review R-4：原来按
				// LIMIT+1 假定"读全文"，对 150 行的小文件也会误报）。行数读不到时按超限处理。
				let willRead = limit;
				if (willRead === null) {
					willRead = countLines(ctx.cwd ?? process.cwd(), input.path);
				}
				if (willRead !== null && willRead > READ_LINE_LIMIT && !st.readWarned) {
					st.readWarned = true;
					nudge(
						pi,
						[
							`💡 [router-discipline] 你正在主线程读一个超过 ${READ_LINE_LIMIT} 行的文件（limit=${limit ?? "未指定，读全文"}）。`,
							"规范：主线程 read 仅限「约 200 行内定点核对 / 派发前定位 / 验收子线程产出」。",
							"探索类阅读（理清结构、跨文件追链路）应派 explore 子线程并 pin_finding 落盘——主线程只消费结论（file:line + 要点）。",
							"本次会话此类提醒只出一次；继续读由你判断。",
						].join("\n"),
					);
				}
				return;
			}

			if (event.toolName === "edit") {
				const input = (event.input ?? {}) as { path?: unknown };
				const path = typeof input.path === "string" ? input.path : null;
				if (!path) return;
				const prev = st.editStreak.get(path) ?? 0;
				// 换文件即重置：清掉其它文件的计数（只关心"连续同一文件"）
				if (st.editStreak.size > 0 && !st.editStreak.has(path)) st.editStreak.clear();
				const next = prev + 1;
				st.editStreak.set(path, next);
				if (next >= EDIT_STREAK_LIMIT && !st.editWarned) {
					st.editWarned = true;
					nudge(
						pi,
						[
							`💡 [router-discipline] 你对同一文件连续第 ${next} 次 edit（${path}）——这通常是「改—跑—挂—改」调试循环。`,
							"这类循环（实现 + 调试 + 反复验证）应派 general-purpose 子线程，主线程只留判断与验收（§主线程纪律的五类活分工表）。",
							"确实不值得派时：bash scripts/harness/router-discipline.sh --excuse \"<理由>\"（理由进事实库可回检）。",
							"本次会话此类提醒只出一次；继续由你判断。",
						].join("\n"),
					);
				}
				try {
					logPolicyDecision(ctx.cwd ?? process.cwd(), {
						sessionId,
						policy: "router-discipline",
						action: "warn",
						reasonCode: "router-edit-streak",
						target: path,
					});
				} catch {
					/* 记账失败不改变行为 */
				}
			}
		} catch (err) {
			console.warn(`[router-discipline] 检查异常，本次放行（fail-open）：${String(err)}`);
		}
	});

	pi.on("session_shutdown", async (_event: any, ctx: any) => {
		try {
			const sessionId = ctx.sessionManager?.getSessionId?.();
			if (sessionId) states.delete(sessionId);
		} catch {
			/* 忽略 */
		}
	});
}
