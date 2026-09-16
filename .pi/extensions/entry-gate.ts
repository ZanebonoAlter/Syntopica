/**
 * entry-gate.ts — pi extension：白盒用例文档动工入口门禁（steer 软提示）
 *
 * 设计决策（见 openspec/changes/test-case-entry-gate/design.md，case-first-testing 声明制）：
 * 1. 挂 turn_end（quality-gate 验证过的 steer 模式）：档位切入 implementation 后的
 *    首回合末注入提醒，差半回合可接受；before_agent_start 的 sendMessage 语义未经验证不用。
 * 2. 档位状态从 harness 事实库读本会话最新 mode.set（constraint-injection 写入），
 *    不做跨扩展内存共享、不做全局兜底（多 pi 窗口并行，全局最新几乎必然属他窗口）。
 * 3. 判定同源 lib/test-case-gate.ts：声明 complex 缺文档 = 强提醒（零误报主信号）；
 *    simple/未声明 + 词表命中 = 兜底质询。词表不扩容（校准否决，见 design）。
 * 4. 每会话每 change 至多提醒一次（去重 Map）；文档补齐后自然静默；session 重启仍缺
 *    文档会再次提醒（持续存在而非一次性）。
 * 5. 软提示不阻断：deliverAs:"steer" 注入 agent 上下文；全程 fail-open（异常静默跳过
 *    + console.warn），门禁自身故障不阻断干活。
 * 6. 触发即记 gate.check（cmd=entry-gate，ok=!remind，payload 含 declaration/kwHits），
 *    事件库可考古。
 *
 * 配置：ENTRY_GATE_ENABLE（默认开，"0"/"false"/"off" 关闭）。
 */
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { logEvent, queryBySession } from "./lib/harness-log";
import {
	buildEntryReminderMessage,
	decideEntryReminder,
	type EntryReminderInput,
} from "./lib/test-case-gate";

/** 入口门禁总开关，默认开 */
const ENABLED = !["0", "false", "off"].includes(
	(process.env.ENTRY_GATE_ENABLE ?? "").toLowerCase(),
);

/** 本会话已提醒过的 change 集合（去重）；session_start 非 startup 清空 */
const warnedChanges = new Set<string>();

export default function (pi: ExtensionAPI) {
	// 会话边界清零（startup 跳过：pi-subagents 派发子线程时向同一共享模块实例发
	// session_start{startup}，清掉会把主会话去重记忆误伤——quality-gate 同款防御）
	pi.on("session_start", async (event) => {
		if (event.reason === "startup") return;
		warnedChanges.clear();
	});

	pi.on("turn_end", async (event, ctx) => {
		if (!ENABLED) return;
		try {
			// 1. 会话与档位：本会话最新 mode.set（无记录 = requirements/未激活 → 零成本跳过）
			const sessionId = ctx.sessionManager?.getSessionId?.();
			if (!sessionId) return;
			const cwd = ctx.cwd ?? process.cwd();
			const rows = queryBySession(cwd, sessionId, ["mode.set"]);
			if (rows.length === 0) return;
			let mode: string | null = null;
			let boundChange: string | null = null;
			try {
				const payload = JSON.parse(rows[rows.length - 1].payload) as {
					mode?: unknown;
					boundChange?: unknown;
				};
				mode = typeof payload.mode === "string" ? payload.mode : null;
				boundChange =
					typeof payload.boundChange === "string" ? payload.boundChange : null;
			} catch {
				return; // payload 损坏 → 无可靠档位信息，静默跳过
			}
			if (mode !== "implementation" || !boundChange) return;

			// 2. 读 change 目录快照（读不到 → 空值，判定函数按未声明/未命中静默处理）
			const changeDir = join(cwd, "openspec/changes", boundChange);
			const input: EntryReminderInput = {
				mode,
				boundChange,
				changeDirFiles: safeListDir(changeDir),
				proposalText: safeReadFile(join(changeDir, "proposal.md")),
				tasksMd: safeReadFile(join(changeDir, "tasks.md")),
				alreadyWarned: warnedChanges.has(boundChange),
			};
			const reminder = decideEntryReminder(input);

			// 3. 记账（无论是否提醒，审计可考古；写入失败不阻断）
			logEvent(cwd, {
				kind: "gate.check",
				sessionId,
				change: boundChange,
				payload: {
					cmd: "entry-gate",
					phase: "turn_end",
					ok: reminder === null,
					declaration: reminder?.declaration ?? null,
					kwHits: reminder?.kwHits ?? [],
				},
			});

			// 4. steer 提醒（每会话每 change 一次；文档补齐后 decide 自然静默）
			if (reminder) {
				warnedChanges.add(boundChange);
				pi.sendMessage(
					{
						customType: "entry-gate-reminder",
						content: buildEntryReminderMessage(boundChange, reminder),
					},
					{ deliverAs: "steer", triggerTurn: true },
				);
			}
		} catch (err) {
			// fail-open：门禁自身异常绝不影响干活，留痕不静默
			console.warn(`[entry-gate] turn_end 检查异常，跳过：${String(err)}`);
		}
	});
}

/** 目录列表；读不到 → 空数组（视为无文档，但不必然提醒——判定交给纯函数） */
function safeListDir(dir: string): string[] {
	try {
		return readdirSync(dir);
	} catch {
		return [];
	}
}

/** 文件读取；读不到 → null（视为未声明/未命中） */
function safeReadFile(path: string): string | null {
	try {
		return readFileSync(path, "utf8");
	} catch {
		return null;
	}
}
