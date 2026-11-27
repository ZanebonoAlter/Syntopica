/**
 * quant-claim-gate — 可量化断言声明门禁（warn-only 观察期）
 *
 * 动机：change lane-first-recall 审查 R-11 —— design.md 声称「查询 MUST 用该表达式才能
 * 命中索引」，EXPLAIN 三份计划全部走 Sort。问题是「声称了量化结果但没有证据」。
 *
 * 本门禁只校验**声明合规性**（marker + 固定表 + 证据三列），绝不判断断言真假
 * （真假由 review 复核，见 review-brief 的必查维度）。判据落在结构化单元格上，
 * 不做正文关键词扫描——关键词靠措辞运气，换个说法就漏。
 *
 * 观察期策略（与 check-standards J 段同节奏）：**只 WARN、不 BLOCK**。
 * 存量活跃 change 未 retrofit（用户决策「旧的 change 先不整」），因此：
 *   - 每个 (session, change) 只提醒一次，避免每次工具调用刷屏
 *   - 累计一个迭代后用 harness-retro 看 warn 命中率，再决定是否转 block
 *
 * 责任边界（与既有验证载体的分工，权威见 docs/reference/开发执行规范.md）：
 *   ① spec Scenario（行为契约）② tasks Scenario→测试映射（scenario-trace）
 *   ③ tasks 验证命令（跑完即知 PASS/FAIL）④ 本门禁（对真实数据/真实计划的观测）
 *
 * fail-open：扩展自身异常一律放行 + console.warn + steer 留痕，绝不静默也绝不误伤。
 */
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { existsSync, readdirSync, readFileSync } from "node:fs";
import { join, sep } from "node:path";
import { logPolicyDecision } from "./lib/policy-decision";
import { checkQuantClaims, type QuantIssue } from "./lib/quant-claim";

/** 观察期总开关：0 = 完全关闭 */
const ENABLED = process.env.QUANT_CLAIM_GATE !== "0";

/** spec 声明的稳定标识 */
const POLICY = "quant-claim-gate";

/** 本会话已提醒过的 change（`${sessionId}|${change}` 去重） */
const warned = new Set<string>();

type GateCtx = {
	signal?: AbortSignal | undefined;
	cwd?: string;
	sessionManager?: { getSessionId?(): string | undefined } | undefined;
};

function steer(pi: ExtensionAPI, message: string): void {
	try {
		const anyPi = pi as unknown as {
			ui?: { notify?: (m: string) => void };
			sendMessage?: (m: string) => void;
		};
		if (typeof anyPi.sendMessage === "function") anyPi.sendMessage(message);
		else if (typeof anyPi.ui?.notify === "function") anyPi.ui.notify(message);
		else console.warn(message);
	} catch {
		console.warn(message);
	}
}

/** 活跃 change 目录名清单（跳过 archive；只看有 design.md 的） */
function listActiveChanges(cwd: string): string[] {
	const root = join(cwd, "openspec", "changes");
	if (!existsSync(root)) return [];
	try {
		return readdirSync(root, { withFileTypes: true })
			.filter((d) => d.isDirectory() && d.name !== "archive")
			.filter((d) => existsSync(join(root, d.name, "design.md")))
			.map((d) => d.name);
	} catch {
		return [];
	}
}

function formatIssues(change: string, issues: QuantIssue[]): string {
	const lines = issues
		.slice(0, 6)
		.map((i) => `  · [${i.reasonCode}] ${i.detail}`);
	const more = issues.length > 6 ? `\n  · …共 ${issues.length} 项` : "";
	return [
		`⚠️ [quant-claim-gate] openspec/changes/${change}/design.md 的可量化断言声明不合规（**warn 观察期，不阻断**）：`,
		...lines,
		`${more}`,
		"修法：无量化断言 → 头部写 `<!-- quant-claims: none -->`；有 → 写 `declared` 并补 `## 可量化断言` 表（列：类别 | 断言 | 观测值 | 复算方式 | 观测日期 | 状态）。",
		"判据：能被一次 `go test` 完整判定的写测试（Scenario），**不能的**（依赖真实数据集/真实查询计划/采样分布）才进表。",
	].join("\n");
}

function audit(ctx: GateCtx, change: string | null, reasons: string[]): void {
	if (!ctx.cwd) return;
	for (const reasonCode of reasons) {
		try {
			logPolicyDecision(ctx.cwd, {
				sessionId: ctx.sessionManager?.getSessionId?.() ?? null,
				policy: POLICY,
				action: "warn",
				reasonCode,
				target: change,
			});
		} catch {
			// 记账失败不影响主流程
		}
	}
}

/** 校验一个 change，返回问题列表（读文件失败视作无问题，fail-open） */
function checkChange(cwd: string, change: string): QuantIssue[] {
	const path = join(cwd, "openspec", "changes", change, "design.md");
	try {
		return checkQuantClaims(readFileSync(path, "utf8"));
	} catch {
		return [];
	}
}

export default function (pi: ExtensionAPI) {
	// 会话边界清零（startup 跳过：子线程共用模块实例，防误伤主会话去重）
	pi.on("session_start", async (event, ctx) => {
		if (!ENABLED) return;
		if (event.reason === "startup") return;
		warned.clear();
	});

	// ① 会话首次交互时扫一遍活跃 change（每 change 一条 warn）
	pi.on("session_start", async (_event, ctx) => {
		if (!ENABLED) return;
		if (!ctx.cwd) return;
		const sessionId = ctx.sessionManager?.getSessionId?.() ?? "";
		for (const change of listActiveChanges(ctx.cwd)) {
			const key = `${sessionId}|${change}`;
			if (warned.has(key)) continue;
			const issues = checkChange(ctx.cwd, change);
			if (issues.length === 0) continue;
			warned.add(key);
			steer(pi, formatIssues(change, issues));
			audit(ctx as GateCtx, change, [...new Set(issues.map((i) => i.reasonCode))]);
		}
	});

	// ② 写入 design.md 后复检（edit/write 两个结构化入口）
	pi.on("tool_call", async (event, ctx) => {
		if (!ENABLED) return;
		if (!ctx.cwd) return;
		const toolName = event.toolName;
		if (toolName !== "edit" && toolName !== "write") return;
		const rawPath = (event.input as { path?: unknown } | undefined)?.path;
		if (typeof rawPath !== "string" || !rawPath.endsWith("design.md")) return;
		const normalized = rawPath.split(sep).join("/");
		if (!normalized.includes("openspec/changes/")) return;
		if (normalized.includes("/archive/")) return;

		try {
			const sessionId = ctx.sessionManager?.getSessionId?.() ?? "";
			for (const change of listActiveChanges(ctx.cwd)) {
				if (!normalized.includes(`openspec/changes/${change}/`)) continue;
				const key = `${sessionId}|${change}`;
				if (warned.has(key)) continue;
				// write 之后才会落盘：本次不读（避免读到旧内容），只提示去自查
				const issues = checkChange(ctx.cwd, change);
				if (issues.length === 0) continue;
				warned.add(key);
				steer(pi, formatIssues(change, issues));
				audit(ctx as GateCtx, change, [...new Set(issues.map((i) => i.reasonCode))]);
			}
		} catch (err) {
			// fail-open：绝不因门禁自身异常拦住写入
			console.warn(`[quant-claim-gate] 检查异常，本次放行（fail-open）：${String(err)}`);
		}
	});
}
