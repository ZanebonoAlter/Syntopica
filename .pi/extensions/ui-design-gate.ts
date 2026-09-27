/**
 * ui-design-gate.ts — pi extension：UI 设计合同实现入口门禁（硬后备）
 *
 * change: make-ui-design-first-class（design D5：schema apply 前置为主，本扩展为逃逸后备）。
 *
 * 设计决策：
 * 1. 挂 tool_call，只拦截结构化入口：Agent 派发与 edit/write mutation（bash 写盘
 *    不做可靠判定，交 schema apply preflight 主门）；read/grep/bash 等一律放行。
 * 2. 介入条件：本会话最新 mode.set = implementation 且绑定了 change（同源
 *    entry-gate：constraint-injection 写入的 mode.set；无记录 = requirements → 静默）。
 * 3. 每次检查**重新读磁盘**（.openspec.yaml / proposal.md / ui-design.md / tasks.md /
 *    change 目录递归普通文件清单）——用户批准后不沿用旧 pending 缓存；判定全在
 *    lib/ui-design-gate.ts 纯函数（不读文件/不抛异常）。
 * 4. 新 schema（syntopica-ui）合同 block 时：Agent 派发与越出当前 change 目录的
 *    mutation 阻断；当前 change 目录内修改（ui-design.md / ui-prototype/** 等）放行。
 * 5. legacy schema：不硬阻断；mutation 触及 front/** 时每 session/change 提醒一次
 *    （warn-legacy + policy warn），其余静默。
 * 6. 逃生口 UI_DESIGN_GATE_BYPASS=1：仅在原本会 block 时生效，放行必须留痕
 *    （policy bypass + 显式 warning），不静默。
 * 7. 记账（lib/policy-decision）：block/warn/bypass/fail-open 全记
 *    （policy=ui-design-gate，reasonCode 白名单 8 值）；健康放行零记录。
 *    记账失败不改变裁决。
 * 8. 全程 fail-open：扩展自身异常 → 放行 + console.warn + steer 告警 +
 *    policy fail-open（ui-gate-check-failed），绝不把 agent 卡死。
 *
 * 配置：UI_DESIGN_GATE_ENABLE（默认开，"0"/"false"/"off" 关闭）、
 * UI_DESIGN_GATE_BYPASS=1（显式逃生口）。
 */
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { readdirSync, readFileSync } from "node:fs";
import { isAbsolute, join, relative, sep } from "node:path";
import { logPolicyDecision, type PolicyDecisionInput } from "./lib/policy-decision";
import { queryBySession } from "./lib/harness-log";
import {
	decideUiToolGate,
	type UiToolAction,
	type UiToolGateInput,
} from "./lib/ui-design-gate";

/** 总开关，默认开 */
const ENABLED = !["0", "false", "off"].includes(
	(process.env.UI_DESIGN_GATE_ENABLE ?? "").toLowerCase(),
);
/** 显式逃生口（仅在原本会 block 时生效，留痕不静默） */
const BYPASS = process.env.UI_DESIGN_GATE_BYPASS === "1";

/** 本会话已 legacy 提醒过的 change（`${sessionId}|${change}` 去重） */
const legacyWarned = new Set<string>();

/** 本扩展只用到的 ExtensionContext 子集（结构兼容，见 quota-gate GateCtx） */
type GateCtx = {
	signal?: AbortSignal | undefined;
	cwd?: string;
	sessionManager?: { getSessionId?(): string | undefined } | undefined;
};

export default function (pi: ExtensionAPI) {
	// 会话边界清零（startup 跳过：pi-subagents 派发子线程共用模块实例，防误伤主会话去重）
	pi.on("session_start", async (event) => {
		if (event.reason === "startup") return;
		legacyWarned.clear();
	});

	pi.on("tool_call", async (event, ctx) => {
		if (!ENABLED) return;

		// 只拦截结构化入口：Agent 派发 + edit/write mutation；其余零成本放行
		const tool = classifyTool(event.toolName, event.input);
		if (!tool) return;

		try {
			return await check(pi, ctx, tool);
		} catch (err) {
			// fail-open：内部异常放行但留痕（绝不伪装成功、绝不静默）
			console.warn(`[ui-design-gate] 检查异常，本次放行（fail-open）：${String(err)}`);
			steer(
				pi,
				`⚠️ [ui-design-gate] 门禁内部异常，本次放行（fail-open，reasonCode: ui-gate-check-failed）：${String(err)}`,
			);
			auditPolicy(ctx, null, {
				policy: "ui-design-gate",
				action: "fail-open",
				reasonCode: "ui-gate-check-failed",
			});
			return;
		}
	});
}

/** 归纳工具动作；返回 null = 不介入（read/bash/grep 等） */
function classifyTool(
	toolName: string,
	input: unknown,
): UiToolAction | null {
	if (toolName === "Agent") return { kind: "agent-dispatch", targetPath: null };
	if (toolName === "edit" || toolName === "write") {
		const p = (input as { path?: unknown } | null | undefined)?.path;
		return typeof p === "string" ? { kind: "mutation", targetPath: p } : null;
	}
	return null;
}

/** 主检查：档位 → 磁盘快照 → 纯函数判定 → 阻断/放行/legacy 提醒 */
async function check(
	pi: ExtensionAPI,
	ctx: GateCtx,
	tool: UiToolAction,
): Promise<{ block: true; reason: string } | undefined> {
	// 1. 会话与档位（无记录 = requirements/未激活 → 静默放行）
	const sessionId = ctx.sessionManager?.getSessionId?.();
	if (!sessionId) return;
	const cwd = ctx.cwd ?? process.cwd();
	const modeInfo = readLatestMode(cwd, sessionId);
	if (!modeInfo || modeInfo.mode !== "implementation" || !modeInfo.boundChange) {
		return;
	}
	const { boundChange } = modeInfo;
	const changeDir = join(cwd, "openspec/changes", boundChange);

	// 2. 磁盘快照（每次重读，不缓存——批准后即时生效）
	const input: UiToolGateInput = {
		schema: readSchemaName(changeDir),
		proposalText: safeRead(join(changeDir, "proposal.md")),
		tasksMd: safeRead(join(changeDir, "tasks.md")),
		uiDesignText: safeRead(join(changeDir, "ui-design.md")),
		changeDirFiles: listRegularFiles(changeDir),
		mode: modeInfo.mode,
		boundChange,
		tool: normalizeTarget(tool, cwd),
	};

	// 3. 纯函数判定
	const decision = decideUiToolGate(input);

	if (decision.action === "allow") return; // 健康放行零记录

	if (decision.action === "warn-legacy") {
		// 每 session/change 仅提醒与记账一次
		const key = `${sessionId}|${boundChange}`;
		if (legacyWarned.has(key)) return;
		legacyWarned.add(key);
		steer(pi, `⚠️ [ui-design-gate] ${decision.message}`);
		auditPolicy(ctx, boundChange, {
			policy: "ui-design-gate",
			action: "warn",
			reasonCode: "ui-design-missing",
		});
		return;
	}

	// block：显式逃生口 → 放行但留痕（不静默）；否则阻断
	if (BYPASS) {
		console.warn(`[ui-design-gate] UI_DESIGN_GATE_BYPASS=1，阻断被显式旁路：${boundChange}`);
		steer(
			pi,
			`⚠️ [ui-design-gate] 「${boundChange}」UI 合同未就绪（${decision.reasonCode}），本次操作经 UI_DESIGN_GATE_BYPASS=1 显式旁路放行；留痕备查，请尽快补齐 ui-design.md。`,
		);
		auditPolicy(ctx, boundChange, {
			policy: "ui-design-gate",
			action: "bypass",
			reasonCode: "explicit-bypass",
		});
		return;
	}
	auditPolicy(ctx, boundChange, {
		policy: "ui-design-gate",
		action: "block",
		reasonCode: decision.reasonCode,
	});
	return { block: true, reason: decision.message };
}

// ---------- 磁盘快照（全部 fail-soft：读不到 → null/[]，判定交给纯函数） ----------

/** 读 .openspec.yaml 的 `schema:` 行；读不到/解析不出 → null（视为 legacy，不硬阻断） */
function readSchemaName(changeDir: string): string | null {
	const text = safeRead(join(changeDir, ".openspec.yaml"));
	if (text === null) return null;
	const m = /^schema:\s*(\S+)\s*$/m.exec(text);
	return m ? m[1] : null;
}

/** 本会话最新 mode.set（payload {mode, boundChange}）；无记录/损坏 → null */
function readLatestMode(
	cwd: string,
	sessionId: string,
): { mode: string | null; boundChange: string | null } | null {
	const rows = queryBySession(cwd, sessionId, ["mode.set"]);
	if (rows.length === 0) return null;
	try {
		const payload = JSON.parse(rows[rows.length - 1].payload) as {
			mode?: unknown;
			boundChange?: unknown;
		};
		return {
			mode: typeof payload.mode === "string" ? payload.mode : null,
			boundChange: typeof payload.boundChange === "string" ? payload.boundChange : null,
		};
	} catch {
		return null;
	}
}

/** 读文本；读不到 → null */
function safeRead(path: string): string | null {
	try {
		return readFileSync(path, "utf8");
	} catch {
		return null;
	}
}

/** change 目录递归普通文件清单（相对 change 根，正斜杠；symlink/目录不入列——断链自然判缺失） */
function listRegularFiles(changeDir: string): string[] {
	const out: string[] = [];
	const walk = (dir: string, prefix: string): void => {
		let entries;
		try {
			entries = readdirSync(dir, { withFileTypes: true });
		} catch {
			return;
		}
		for (const e of entries) {
			if (e.name === ".openspec.yaml" && prefix === "") continue; // schema 声明不算制品
			const rel = prefix ? `${prefix}/${e.name}` : e.name;
			if (e.isDirectory()) {
				walk(join(dir, e.name), rel);
			} else if (e.isFile()) {
				out.push(rel);
			}
		}
	};
	walk(changeDir, "");
	return out;
}

/** mutation 目标归一为仓库根相对正斜杠路径（绝对/相对均可；越出 cwd 保留原样） */
function normalizeTarget(tool: UiToolAction, cwd: string): UiToolAction {
	if (tool.kind !== "mutation" || tool.targetPath === null) return tool;
	let p = tool.targetPath.replaceAll("\\", "/");
	if (isAbsolute(p) || p.includes(":")) {
		const rel = relative(cwd, p).replaceAll(sep, "/").replaceAll("\\", "/");
		p = rel.startsWith("..") ? p : rel;
	}
	return { ...tool, targetPath: p };
}

// ---------- 记账与留痕 ----------

/** policy.decision 旁路记账（同 spec-gate：无 cwd/sessionId 不记；失败不影响裁决） */
function auditPolicy(
	ctx: GateCtx,
	change: string | null | undefined,
	decision: PolicyDecisionInput,
): void {
	try {
		const sessionId = ctx.sessionManager?.getSessionId?.();
		if (!ctx.cwd || !sessionId) return;
		logPolicyDecision(ctx.cwd, { sessionId, change, ...decision });
	} catch (err) {
		console.warn(`[ui-design-gate] policy.decision 记账异常（旁路忽略）：${String(err)}`);
	}
}

/** 落 custom_message 留痕（steer：不打断当前回合；复用 spec-gate 的 customType 模式） */
function steer(pi: ExtensionAPI, content: string): void {
	try {
		pi.sendMessage(
			{ customType: "ui-design-gate-warning", content, display: true },
			{ deliverAs: "steer" },
		);
	} catch (err) {
		console.warn(`[ui-design-gate] custom_message 落盘失败：${String(err)}`);
	}
}
