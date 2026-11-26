/**
 * ui-design-gate.ts — pi extension：UI 设计合同实现入口门禁（硬后备）
 *
 * change: make-ui-design-first-class（design D5：schema apply 前置为主，本扩展为逃逸后备）。
 *
 * 设计决策：
 * 1. 挂 tool_call，只拦截结构化入口：Agent 派发与 edit/write mutation（bash 写盘
 *    不做可靠判定，交 schema apply preflight 主门）；read/grep/bash 等一律放行。
 * 2. 介入条件：本会话最新 stage.set 的**阶段族 == implementation** 且绑定了 change
 *    （stage.set 由 constraint-injection 写入；取数走 queryLatestStageSet
 *    ——双读，legacy mode.set 自动映射回族；无记录 = 未绑定阶段 → 静默）。
 * 3. 每次检查**重新读磁盘**（.openspec.yaml / proposal.md / ui-design.md / tasks.md /
 *    change 目录递归普通文件清单）——用户批准后不沿用旧 pending 缓存；判定全在
 *    lib/ui-design-gate.ts 纯函数（不读文件/不抛异常）。
 * 4. 新 schema（syntopica-ui）合同 block 时：**具备写能力的** Agent 派发与越出当前 change 目录的
 *    mutation 阻断；只读档（explore/plan/change-review）与原型编辑档（ui-design）派发放行并留痕
 *    （policy warn，reasonCode=ui-dispatch-allowed——放行的是「派发」这个动作，子线程经 edit/write
 *    工具的每次写入仍逐个过本门禁，写侧边界不放宽；shell 类通道不在本门禁覆盖范围——既有边界）；
 *    当前 change 目录内修改（ui-design.md / ui-prototype/** 等）放行。
 * 5. legacy schema：不硬阻断；mutation 触及 front/** 时每 session/change 提醒一次
 *    （warn-legacy + policy warn），其余静默。
 * 6. 逃生口 UI_DESIGN_GATE_BYPASS=1：仅在原本会 block 时生效，放行必须留痕
 *    （policy bypass + 显式 warning），不静默。
 * 7. 记账（lib/policy-decision）：block/warn/bypass/fail-open 全记
 *    （policy=ui-design-gate，reasonCode 白名单 9 值）；健康放行零记录，**例外**：
 *    ① 此前有未解除的 block 且本次合同健康放行 → 记一条 action=pass
 *    （ui-approval-approved / ui-design-satisfied，add-change-lifecycle-retro design D2，
 *    与 change.archive 同族的低频高价值成功锚点；账本查「最近未解除 block」判定，
 *    跨 session 可靠）。② 合同未就绪时放行只读档/原型编辑档派发 → 记一条 action=warn
 *    （ui-dispatch-allowed，add-ui-prototype-editor-agent design D5）。
 *    归档侧 ui-verification-missing 的解除由 spec-gate 归档路径记。
 *    记账失败不改变裁决。
 * 8. 全程 fail-open：扩展自身异常 → 放行 + console.warn + steer 告警 +
 *    policy fail-open（ui-gate-check-failed），绝不把 agent 卡死。
 *
 * 配置：UI_DESIGN_GATE_ENABLE（默认开，"0"/"false"/"off" 关闭）、
 * UI_DESIGN_GATE_BYPASS=1（显式逃生口）。
 */
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { existsSync, readdirSync, readFileSync } from "node:fs";
import { isAbsolute, join, relative, sep } from "node:path";
import { logPolicyDecision, type PolicyDecisionInput } from "./lib/policy-decision";
import { queryByChange, queryLatestStageSet } from "./lib/harness-log";
import {
	UI_SCHEMA,
	changeNameFromPath,
	checkStageScope,
	parseUiStage,
	checkPrototypeChain,
	relInsideChange,
	decideUiToolGate,
	passReasonForRelease,
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

/** stage 记账去重（`${sessionId}|${change}|${reasonCode}`）；键含 session 故无需清空 */
const stageWarned = new Set<string>();

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
	if (toolName === "Agent") {
		// 派发档位名（pi-web 的 Agent 工具参数 subagent_type；缺省 = general-purpose = 写能力档）
		const st = (input as { subagent_type?: unknown } | null | undefined)
			?.subagent_type;
		return {
			kind: "agent-dispatch",
			subagentType: typeof st === "string" ? st : null,
		};
	}
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

	// 0. 按写入路径触发的 stage 记账（add-ui-stage-accounting D1/D4）：不看 mode、不改裁夺。
	if (tool.kind === "mutation") accountStageScope(ctx, normalizeTarget(tool, cwd));

	// 0.5 原型产出物依赖链（ISS-02 D2）：**按写入路径触发**，不看阶段族、不看 verdict。
	//     原实现挂在 decideUiToolGate 的 pass 分支里，而 pass 要求「已批准 + 原型文件已在盘」
	//     ——原型早写完了，链一次都拦不到（events.db 实测：39 条越级写入发生在 planning 族 15 /
	//     未绑定 22 / implement 1，新链拦截数 0）。here 对齐 accountStageScope 的触发方式。
	if (tool.kind === "mutation") {
		const chainBlock = checkChainByPath(ctx, cwd, normalizeTarget(tool, cwd));
		if (chainBlock) return chainBlock;
	}

	const stageInfo = readLatestStage(cwd, sessionId);
	if (!stageInfo || stageInfo.stageFamily !== "implementation" || !stageInfo.boundChange) {
		return;
	}
	const { boundChange } = stageInfo;
	const changeDir = join(cwd, "openspec/changes", boundChange);

	// 2. 磁盘快照（每次重读，不缓存——批准后即时生效）
	const input: UiToolGateInput = {
		schema: readSchemaName(changeDir),
		proposalText: safeRead(join(changeDir, "proposal.md")),
		tasksMd: safeRead(join(changeDir, "tasks.md")),
		uiDesignText: safeRead(join(changeDir, "ui-design.md")),
		designMd: safeRead(join(changeDir, "design.md")),
		changeDirFiles: listRegularFiles(changeDir),
		stageFamily: stageInfo.stageFamily,
		boundChange,
		tool: normalizeTarget(tool, cwd),
	};

	// 3. 纯函数判定
	const decision = decideUiToolGate(input);

	if (decision.action === "allow") {
		// 放行侧锚点（add-change-lifecycle-retro D2）：新 schema 合同健康放行且账本上有
		// 未解除的 block → 记 action=pass；普通健康放行 / requirements / legacy 零记录。
		if (decision.verdict?.kind === "pass") {
			recordReleasePass(ctx, boundChange);
		}
		return;
	}

	if (decision.action === "dispatch-allowed") {
		// 合同未就绪时放行只读档/原型编辑档派发（add-ui-prototype-editor-agent D5）：
		// 留痕一条 warn（不改裁决、不打断会话），供事后回检「未就绪期间派了什么档」。
		auditPolicy(ctx, boundChange, {
			policy: "ui-design-gate",
			action: "warn",
			reasonCode: "ui-dispatch-allowed",
		});
		return;
	}

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

/**
 * 本会话最新阶段（**双读**：先 stage.set，无则回退 legacy mode.set 并映射回族——
 * stage-driven-constraint-injection D7 过渡期不断链）；无记录/损坏 → null。
 * 判定只看**阶段族**（implementation 才介入；族为派生值，D2）。
 */
function readLatestStage(
	cwd: string,
	sessionId: string,
): { stage: string | null; stageFamily: string | null; boundChange: string | null } | null {
	const info = queryLatestStageSet(sessionId, cwd);
	if (!info) return null;
	return {
		stage: info.stage,
		stageFamily: info.stageFamily,
		boundChange: info.boundChange,
	};
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

/**
 * 放行侧 pass 记账（add-change-lifecycle-retro design D2）：查账本中该 change 名下
 * policy=ui-design-gate 的 block/pass 序列，最近一条未解除的 block 在案且本次合同健康
 * 放行 → 记一条 action=pass。跨 session 可靠（block 与放行常分属两会话）；已有 pass
 * 在后（已解除）或无 block 在案 → 零记录。归档侧 ui-verification-missing 不在本路径
 * 解除（由 spec-gate 归档路径记）；纯旁路：任何异常仅告警不影响放行。
 */
function recordReleasePass(ctx: GateCtx, change: string): void {
	try {
		const sessionId = ctx.sessionManager?.getSessionId?.();
		if (!ctx.cwd || !sessionId) return;
		const rows = queryByChange(ctx.cwd, change, ["policy.decision"]);
		let lastBlockReason: string | null = null;
		let released = false;
		for (const r of rows) {
			let p: { policy?: unknown; action?: unknown; reasonCode?: unknown };
			try {
				p = JSON.parse(r.payload) as typeof p;
			} catch {
				continue;
			}
			if (p.policy !== "ui-design-gate") continue;
			if (p.action === "block") {
				lastBlockReason =
					typeof p.reasonCode === "string" ? p.reasonCode : null;
				released = false;
			} else if (p.action === "pass") {
				released = true;
			}
		}
		if (!lastBlockReason || released) return; // 无 block 在案 / 已解除 → 零记录
		const reasonCode = passReasonForRelease(lastBlockReason);
		if (!reasonCode) return; // 归档侧码 / 未知码：非本路径解除
		auditPolicy(ctx, change, {
			policy: "ui-design-gate",
			action: "pass",
			reasonCode,
		});
	} catch (err) {
		console.warn(`[ui-design-gate] 放行记账异常（旁路忽略）：${String(err)}`);
	}
}

/** policy.decision 旁路记账（同 spec-gate：无 cwd/sessionId 不记；失败不影响裁决） */
/**
 * 依赖链的路径触发入口（ISS-02 D2 修复）：只看「写的是不是某 change 的 ui-prototype/**」，
 * 不看阶段族与 verdict——原型在生产期（planning 族、未绑定档、pending 期）被写，
 * 挂到 pass 分支等于永不生效。
 *
 * legacy 豁免：change 无 .openspec.yaml（旧 schema）不查。
 * 返回 null = 放行；否则为 block 结果。
 */
function checkChainByPath(
	ctx: GateCtx,
	cwd: string,
	tool: UiToolAction,
): { block: true; reason: string } | null {
	try {
		if (tool.kind !== "mutation" || !tool.targetPath) return null;
		const change = changeNameFromPath(tool.targetPath);
		if (!change) return null;
		const rel = relInsideChange(tool.targetPath, change);
		if (!rel) return null;
		if (rel !== "ui-prototype" && !rel.startsWith("ui-prototype/")) return null;

		const changeDir = join(cwd, "openspec/changes", change);
		if (!existsSync(changeDir)) return null;
		if (readSchemaName(changeDir) !== UI_SCHEMA) return null; // legacy 不追溯

		const detail = checkPrototypeChain(
			{
				schema: UI_SCHEMA,
				proposalText: safeRead(join(changeDir, "proposal.md")),
				tasksMd: safeRead(join(changeDir, "tasks.md")),
				uiDesignText: safeRead(join(changeDir, "ui-design.md")),
				designMd: safeRead(join(changeDir, "design.md")),
				changeDirFiles: listRegularFiles(changeDir),
			},
			rel,
		);
		if (!detail) return null;

		auditPolicy(ctx, change, {
			policy: "ui-design-gate",
			action: "block",
			reasonCode: "ui-chain-out-of-order",
		});
		return {
			block: true,
			reason: [
				`⛔ [ui-design-gate] 「${change}」原型产出物越级（reasonCode: ui-chain-out-of-order），本次写入已阻断。`,
				`缺失项：${detail}`,
				"顺序：design.md 伪代码 → ui-design.md 八节 → ui-prototype/skeleton/ → ui-prototype/index.html → ui-prototype/fixture.json。",
			].join("\n"),
		};
	} catch (err) {
		console.warn(`[ui-design-gate] 依赖链检查异常，本次放行（fail-open）：${String(err)}`);
		return null;
	}
}

/**
 * 按写入路径触发的 stage 记账（add-ui-stage-accounting design D1/D4）：
 * **不看 mode**——原型工作在 requirements / 无档位发生，而 `ui-approval` 的 mode 触发
 * 完全看不见它（实测 34 次 ui-design 派发 100% 落在门外）。本函数只记账、不返回裁决，
 * 也不读写 decideUiToolGate 的任何输出。
 *
 * 低噪声口径：只对 `ui-prototype/` 内的写入与 `ui-design.md` 本身记账，其余路径直接返回。
 */
function accountStageScope(ctx: GateCtx, tool: UiToolAction): void {
	try {
		if (tool.kind !== "mutation" || !tool.targetPath || !ctx.cwd) return;
		const change = changeNameFromPath(tool.targetPath);
		if (!change) return;
		const rel = relInsideChange(tool.targetPath, change);
		if (!rel) return;

		// 只对原型目录记账（其余路径零噪声）。
		if (rel !== "ui-prototype" && !rel.startsWith("ui-prototype/")) return;

		// 基线事实 1：change 目录必须真实存在——纯正则能把仓库外路径
		// （如 /tmp/x/openspec/changes/ghost/…）解析成一个不存在的 change。
		if (!existsSync(join(ctx.cwd, "openspec/changes", change))) return;

		const uiDesignText = safeRead(
			join(ctx.cwd, "openspec/changes", change, "ui-design.md"),
		);
		const parsed = parseUiStage(uiDesignText);

		let reasonCode: string | null = null;
		if (parsed.kind === "missing") reasonCode = "ui-stage-missing";
		else if (parsed.kind === "invalid") reasonCode = "ui-stage-invalid";
		else if (checkStageScope(parsed.stage, rel) !== null) reasonCode = "ui-stage-scope-out";
		if (!reasonCode) return;

		// 去重键不带 sessionId：子线程与主线程共用模块实例，带 session 会让
		// 一次派发乘出 N 条，「低噪声」目标在主场景反而落空。
		const key = `${change}|${reasonCode}`;
		if (stageWarned.has(key)) return;
		stageWarned.add(key);
		auditPolicy(ctx, change, {
			policy: "ui-design-gate",
			action: "warn",
			reasonCode,
		});
	} catch (err) {
		console.warn(`[ui-design-gate] stage 记账异常（旁路忽略）：${String(err)}`);
	}
}

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
