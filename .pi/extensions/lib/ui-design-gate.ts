/**
 * ui-design-gate — UI 设计合同门禁共享纯函数（change: make-ui-design-first-class）
 *
 * 同源消费方：
 *   - .pi/extensions/ui-design-gate.ts（implementation 档 tool_call 拦截：Agent 派发 + 结构化 mutation）
 *   - .pi/extensions/spec-gate.ts（归档链 UI 证据检查⑤→⑥）
 *   - .pi/extensions/tests/ui-design-gate.smoke.cjs / spec-gate.smoke.cjs（白盒分支表直测）
 *
 * 纯函数纪律（同 lib/test-case-gate.ts）：不 import fs、不触网、不抛异常——
 * 空串/超长/畸形输入返回确定结果；文件存在性与目录清单由调用方快照传入。
 *
 * 契约权威：openspec/changes/make-ui-design-first-class/specs/ui-design-workflow/spec.md
 * + test-cases.md §4.1（合同解析分支表）/§4.2（工具动作分支表）。
 */

/** 新 UI 工作流 schema 名（openspec/schemas/syntopica-ui）；其余（含 spec-driven/null）视为 legacy */
export const UI_SCHEMA = "syntopica-ui";

/** spec 声明的 reasonCode 白名单（八个，稳定有界值；顺序即文档序） */
export const UI_REASON_CODES = [
	"ui-impact-missing",
	"ui-impact-mismatch",
	"ui-design-missing",
	"ui-prototype-missing",
	"ui-approval-pending",
	"ui-verification-missing",
	"explicit-bypass",
	"ui-gate-check-failed",
] as const;
export type UiReasonCode = (typeof UI_REASON_CODES)[number];

/** ui-impact 合法档位 */
export type UiImpact = "none" | "minor" | "major";
/** ui-approval 合法值（none/minor 档固定 not-required；major 只允许 pending/approved） */
export type UiApproval = "not-required" | "pending" | "approved";

/** 兜底词表（design D2：声明是主信号，词表只做反向质询，不扩容防误报） */
export const MAJOR_UI_KEYWORDS = [
	"新增页面",
	"新页面",
	"新增面板",
	"新面板",
	"新增弹窗",
	"新弹窗",
	"新增导航",
	"多步流程",
	"信息架构",
	"布局模式",
] as const;
/** proposal/tasks 中出现前端路径的信号（声明 none 时的反向质询） */
const FRONT_PATH_RE = /front\/(app|components|pages|features|layouts|plugins)\//;

/**
 * 解析 proposal 的 `<!-- ui-impact: none|minor|major -->` 声明。
 * 容忍注释内空白；值大小写敏感（仅小写枚举）；**出现多次（无论值是否相同）视为歧义** → null；
 * 缺失/非法/非字符串 → null。返回 null 时上层判 `ui-impact-missing`。
 */
export function parseUiImpactDeclaration(
	proposalText: string | null | undefined,
): UiImpact | null {
	if (typeof proposalText !== "string" || proposalText.length === 0) return null;
	const matches = [
		...proposalText.matchAll(/<!--\s*ui-impact\s*:\s*([^>]*?)\s*-->/g),
	];
	if (matches.length === 0) return null;
	if (matches.length > 1) return null; // 重复 marker = 歧义，阻断
	const v = matches[0][1];
	return v === "none" || v === "minor" || v === "major" ? v : null;
}

/** 解析 `<!-- ui-approval: ... -->`；多次出现/非法值 → null */
export function parseUiApprovalMarker(
	uiDesignText: string | null | undefined,
): UiApproval | null {
	return parseSingleMarker(uiDesignText, "ui-approval", [
		"not-required",
		"pending",
		"approved",
	]);
}

/** 解析 `<!-- ui-prototype: ... -->`；多次出现 → null；值为原始串（可空/可非法，由校验器判） */
export function parseUiPrototypeMarker(
	uiDesignText: string | null | undefined,
): string | null {
	if (typeof uiDesignText !== "string" || uiDesignText.length === 0) return null;
	const matches = [
		...uiDesignText.matchAll(/<!--\s*ui-prototype\s*:\s*([^>]*?)\s*-->/g),
	];
	if (matches.length !== 1) return null;
	return matches[0][1];
}

/** 解析 `<!-- ui-impact: ... -->`（ui-design.md 内）；多次出现/非法值 → null */
export function parseUiImpactMarker(
	uiDesignText: string | null | undefined,
): UiImpact | null {
	const raw = parseSingleMarker(uiDesignText, "ui-impact", [
		"none",
		"minor",
		"major",
	]);
	return raw as UiImpact | null;
}

/** 单 marker 通用解析：恰好一次且值在枚举内才返回，否则 null */
function parseSingleMarker(
	text: string | null | undefined,
	name: string,
	legal: readonly string[],
): string | null {
	if (typeof text !== "string" || text.length === 0) return null;
	const re = new RegExp(`<!--\\s*${name}\\s*:\\s*([^>]*?)\\s*-->`, "g");
	const matches = [...text.matchAll(re)];
	if (matches.length !== 1) return null;
	const v = matches[0][1];
	return (legal as readonly string[]).includes(v) ? v : null;
}

/** heading 存在性检查（`## Xxx` 行首锚定，子节 ### 不算）；aliases 任一命中即算 */
function hasHeading(text: string, aliases: readonly string[]): boolean {
	const re = new RegExp(`^##\\s+.*(?:${aliases.join("|")})`, "im");
	return re.test(text);
}

/** major 八节（模板同序；中英别名任一命中） */
const MAJOR_SECTIONS: readonly (readonly string[])[] = [
	["User Journey", "用户旅程", "用户任务"],
	["Information Architecture", "信息架构"],
	["Interaction Contract", "交互契约", "交互合同"],
	["State Matrix", "状态矩阵"],
	["Layout Contract", "布局契约", "布局合同"],
	["Component Reuse", "组件复用"],
	["Prototype", "原型"],
	["Acceptance", "验收"],
];
/** minor 四项轻量契约（中英别名任一命中） */
const MINOR_SECTIONS: readonly (readonly string[])[] = [
	["入口", "Entry"],
	["状态", "State"],
	["复用", "布局", "Reuse", "Layout"],
	["验收", "Acceptance"],
];

/** none 档的 N/A 节别名 */
const NA_SECTION = ["N/A", "不适用"];

/**
 * 校验 major/minor 档必填节是否齐全；返回缺失项中英文名列表（空=齐）。
 * none 档用 hasHeading(text, NA_SECTION) 单独判。
 */
export function missingSections(
	uiDesignText: string,
	level: "major" | "minor",
): string[] {
	const table = level === "major" ? MAJOR_SECTIONS : MINOR_SECTIONS;
	return table.filter((aliases) => !hasHeading(uiDesignText, aliases)).map(
		(aliases) => aliases[0],
	);
}

/**
 * 原型路径校验（纯函数：不查盘，存在性由 changeDirFiles 快照判定）。
 * 返回 null=合法（change 根内 ui-prototype/ 下的普通文件且在清单内）；
 * 否则返回人类可读的非法原因。
 * 规则：非空；相对路径；不含 `..` 段；不以 `/` 或盘符开头；不以分隔符结尾（目录）；
 * 必须落在 `ui-prototype/` 前缀内；必须在 changeDirFiles 中精确存在（断链 symlink
 * 不会出现在普通文件清单里 → 自然判 missing）。
 */
export function validatePrototypePath(
	rawPath: string,
	changeDirFiles: readonly string[],
): string | null {
	const p = rawPath.trim();
	if (p.length === 0) return "原型路径为空";
	if (p !== rawPath || /[\r\n]/.test(p)) return "原型路径含非法字符";
	if (/^([a-zA-Z]:)?[\\/]/.test(p)) return "原型路径是绝对路径";
	if (p.endsWith("/") || p.endsWith("\\")) return "原型路径是目录";
	if (p.split(/[\\/]/).includes("..")) return "原型路径越出 change 目录（含 ..）";
	if (!p.startsWith("ui-prototype/")) return "原型路径不在 ui-prototype/ 内";
	if (!changeDirFiles.includes(p)) return "原型文件不存在（或为断链 symlink/目录）";
	return null;
}

/** 合同判定结果；legacy = 旧 schema（不硬阻断，由工具动作层决定 warn-once） */
export type UiContractVerdict =
	| { kind: "legacy" }
	| { kind: "pass" }
	| { kind: "block"; reasonCode: UiReasonCode; detail: string };

/** evaluateUiContract 入参（全量快照，纯函数可测；fileRefs 均由调用方读取） */
export interface UiContractInput {
	/** change 绑定的 schema 名（.openspec.yaml 的 schema:）；null = 读不到 */
	schema: string | null;
	/** proposal.md 文本；null = 读不到 */
	proposalText: string | null;
	/** tasks.md 文本（mismatch 反向质询用）；null = 读不到 */
	tasksMd: string | null;
	/** ui-design.md 文本；null = 文件缺失/读不到 */
	uiDesignText: string | null;
	/** change 目录递归普通文件清单（相对 change 根，正斜杠） */
	changeDirFiles: readonly string[];
}

/**
 * 合同判定主入口（test-cases.md §4.1 分支表的机器化）：
 * ① schema ≠ syntopica-ui（含 null）→ legacy（调用方决定 warn-once/pass，不硬阻断）
 * ② proposal ui-impact 缺失/非法/重复 → block: ui-impact-missing
 * ③ 反向质询：声明 none/minor 但 proposal/tasks 命中 major 特征词、或声明 none 却
 *    出现前端路径 → block: ui-impact-mismatch（声明是主信号，词表仅兜底）
 * ④ ui-design.md 缺失 / marker 缺失重复非法 / 与 proposal 声明不一致 → block: ui-design-missing
 * ⑤ 按档位校验必填内容与 marker 组合（详见各档注释）→ pass / 对应 block
 */
export function evaluateUiContract(input: UiContractInput): UiContractVerdict {
	if (input.schema !== UI_SCHEMA) return { kind: "legacy" };

	const declared = parseUiImpactDeclaration(input.proposalText);
	if (!declared) {
		return {
			kind: "block",
			reasonCode: "ui-impact-missing",
			detail:
				"proposal.md 缺失/非法/重复 `<!-- ui-impact: none|minor|major -->` 声明（与 complexity 正交，三选一，小写，仅一条）",
		};
	}

	const mismatch = scanImpactMismatch(
		declared,
		input.proposalText,
		input.tasksMd,
		input.uiDesignText,
	);
	if (mismatch) {
		return { kind: "block", reasonCode: "ui-impact-mismatch", detail: mismatch };
	}

	if (input.uiDesignText === null) {
		return {
			kind: "block",
			reasonCode: "ui-design-missing",
			detail: "change 目录缺失 ui-design.md（新 schema 下必经制品）",
		};
	}

	const designImpact = parseUiImpactMarker(input.uiDesignText);
	const approval = parseUiApprovalMarker(input.uiDesignText);
	const prototypeRaw = parseUiPrototypeMarker(input.uiDesignText);
	if (!designImpact || !approval || prototypeRaw === null || designImpact !== declared) {
		return {
			kind: "block",
			reasonCode: "ui-design-missing",
			detail:
				"ui-design.md 头部三条 marker（ui-impact/ui-approval/ui-prototype）必须各恰好一条、值合法，且 ui-impact 与 proposal 声明一致",
		};
	}

	if (declared === "none") {
		if (approval !== "not-required" || prototypeRaw !== "none") {
			return {
				kind: "block",
				reasonCode: "ui-design-missing",
				detail: "none 档要求 ui-approval: not-required 与 ui-prototype: none",
			};
		}
		if (!hasHeading(input.uiDesignText, NA_SECTION)) {
			return {
				kind: "block",
				reasonCode: "ui-design-missing",
				detail: "none 档正文需含 `## N/A Reason`（或「不适用」）最小记录节",
			};
		}
		return { kind: "pass" };
	}

	if (declared === "minor") {
		if (approval !== "not-required") {
			return {
				kind: "block",
				reasonCode: "ui-design-missing",
				detail: "minor 档要求 ui-approval: not-required（人工审批仅对 major 强制）",
			};
		}
		const missing = missingSections(input.uiDesignText, "minor");
		if (missing.length > 0) {
			return {
				kind: "block",
				reasonCode: "ui-design-missing",
				detail: `minor 档四项轻量契约缺节：${missing.join("、")}（入口/状态/复用或布局/验收）`,
			};
		}
		if (prototypeRaw !== "none") {
			const bad = validatePrototypePath(prototypeRaw, input.changeDirFiles);
			if (bad) {
				return {
					kind: "block",
					reasonCode: "ui-prototype-missing",
					detail: `minor 档提供了原型引用但校验失败：${bad}`,
				};
			}
		}
		return { kind: "pass" };
	}

	// major
	if (approval === "not-required") {
		return {
			kind: "block",
			reasonCode: "ui-design-missing",
			detail: "major 档 ui-approval 只允许 pending|approved（not-required 属 none/minor 档）",
		};
	}
	const missing = missingSections(input.uiDesignText, "major");
	if (missing.length > 0) {
		return {
			kind: "block",
			reasonCode: "ui-design-missing",
			detail: `major 档八节缺节：${missing.join("、")}`,
		};
	}
	const badProto = validatePrototypePath(prototypeRaw, input.changeDirFiles);
	if (badProto) {
		return {
			kind: "block",
			reasonCode: "ui-prototype-missing",
			detail: `major 档原型校验失败：${badProto}（只接受 change 根内 ui-prototype/ 下普通文件）`,
		};
	}
	if (approval === "pending") {
		return {
			kind: "block",
			reasonCode: "ui-approval-pending",
			detail:
				"major UI 原型尚未获得用户明确确认——回到 requirements 档展示 ui-prototype/ 原型并请求确认后，把 ui-approval 改为 approved",
		};
	}
	return { kind: "pass" };
}

/**
 * 反向质询（design D2）：声明与内容矛盾才报告，不自动升级档位。
 * - 声明 none/minor + major 特征词命中（proposal+tasks 文本）→ mismatch
 * - 声明 none + 前端路径信号 → mismatch；ui-design.md 含显式豁免注释
 *   <!-- ui-front-path-excuse: 理由 --> 时放行（错误文案承诺的"正文说明"修复
 *   路径的机器化兑现：存在即豁免，理由不解析，风格对齐 doc-impact-excuse）
 * 返回 null = 无矛盾。词表有界（MAJOR_UI_KEYWORDS），不扩容。
 */
const FRONT_PATH_EXCUSE_RE = /<!--[\s]*ui-front-path-excuse:[^\n]*-->/;

export function scanImpactMismatch(
	declared: UiImpact,
	proposalText: string | null | undefined,
	tasksMd: string | null | undefined,
	uiDesignText?: string | null,
): string | null {
	if (declared === "major") return null;
	const corpus = `${proposalText ?? ""}\n${tasksMd ?? ""}`;
	const hits = MAJOR_UI_KEYWORDS.filter((kw) => corpus.includes(kw));
	if (hits.length > 0) {
		return `声明 ui-impact: ${declared} 但 proposal/tasks 命中 major 特征词（${hits.join("、")}）——新增页面/面板/弹窗/导航、多步流程、信息架构或布局模式变化必须声明 major；修正声明或收窄范围`;
	}
	if (declared === "none" && FRONT_PATH_RE.test(corpus)) {
		if (uiDesignText && FRONT_PATH_EXCUSE_RE.test(uiDesignText)) return null;
		return "声明 ui-impact: none 但 proposal/tasks 出现前端路径（front/app、front/components 等）——涉及用户可见界面请改声明 minor/major，纯工具链改动请在 ui-design.md 加 <!-- ui-front-path-excuse: 理由 --> 豁免注释说明该路径与本 change 无关";
	}
	return null;
}

// ---------------------------------------------------------------------------
// 工具动作层（test-cases.md §4.2 分支表）——extension 逐 tool_call 调用
// ---------------------------------------------------------------------------

/** 工具动作分类（extension 侧从 event 归纳；bash 不做写盘判定，归 other） */
export interface UiToolAction {
	/** agent 子线程派发（Agent 工具） */
	kind: "agent-dispatch" | "mutation" | "other";
	/** mutation 的仓库根相对目标路径（正斜杠）；其他动作为 null */
	targetPath: string | null;
}

/** decideUiToolGate 入参 */
export interface UiToolGateInput extends UiContractInput {
	/** 当前档位（implementation 才介入）；null = 无档位记录 */
	mode: string | null;
	/** 绑定的 change 名；null = 未绑定 */
	boundChange: string | null;
	tool: UiToolAction;
}

/** 工具动作判定结果 */
export type UiToolDecision =
	| { action: "allow"; silent: true }
	| { action: "block"; reasonCode: UiReasonCode; message: string }
	| { action: "warn-legacy"; reasonCode: "ui-design-missing"; message: string };

/**
 * 工具动作门禁（§4.2）：
 * - 非 implementation 档 / 未绑定 change → allow（requirements 静默）
 * - legacy schema：mutation 触及 front/** → warn-legacy（调用方每 session/change 去重一次）；其余 allow
 * - 新 schema 合同 pass → allow（健康放行零噪声）
 * - 新 schema 合同 block：
 *   · other（读/验证/bash）→ allow
 *   · mutation 且目标在当前 change 目录内（修 ui-design.md / ui-prototype/** 等规划制品）→ allow
 *   · mutation 越出当前 change 目录 / Agent 派发 → block
 * bypass 与 fail-open 由 extension 层处理（本函数不感知环境变量/异常）。
 */
export function decideUiToolGate(input: UiToolGateInput): UiToolDecision {
	if (input.mode !== "implementation" || !input.boundChange) {
		return { action: "allow", silent: true };
	}
	const verdict = evaluateUiContract(input);

	if (verdict.kind === "legacy") {
		if (
			input.tool.kind === "mutation" &&
			input.tool.targetPath !== null &&
			input.tool.targetPath.startsWith("front/")
		) {
			return {
				action: "warn-legacy",
				reasonCode: "ui-design-missing",
				message: `「${input.boundChange}」仍绑定旧 spec-driven schema 且缺少 ui-design.md：本提示每会话仅一次，不阻断；建议迁移到 ${UI_SCHEMA} schema（补 ui-impact 声明与 ui-design.md 制品）`,
			};
		}
		return { action: "allow", silent: true };
	}

	if (verdict.kind === "pass") return { action: "allow", silent: true };

	// block 裁决 → 按工具动作分流
	if (input.tool.kind === "other") return { action: "allow", silent: true };
	if (
		input.tool.kind === "mutation" &&
		input.tool.targetPath !== null &&
		input.tool.targetPath.startsWith(`openspec/changes/${input.boundChange}/`)
	) {
		return { action: "allow", silent: true }; // 修当前 change 的 UI 合同/原型/规划制品
	}
	return {
		action: "block",
		reasonCode: verdict.reasonCode,
		message: buildBlockMessage(input.boundChange, verdict.reasonCode, verdict.detail),
	};
}

/** 阻断文案：change + reasonCode + 修复路径（三要素齐备；detail 已有界） */
export function buildBlockMessage(
	change: string,
	reasonCode: UiReasonCode,
	detail: string,
): string {
	return [
		`⛔ [ui-design-gate] 「${change}」UI 设计合同未就绪（reasonCode: ${reasonCode}），本次操作已阻断。`,
		`缺失项：${detail}`,
		"修复路径：切回 requirements 档完善 openspec/changes/" + change + "/ui-design.md（major 需在 ui-prototype/ 提供原型并由用户明确确认后改 ui-approval: approved）；完成前不得派发实现子线程或编辑项目代码（当前 change 目录内的修改不受限）。",
	].join("\n");
}

// ---------------------------------------------------------------------------
// 归档证据检查（task 3.5，spec-gate 调用；spec「UI 完成验收必须对照批准基线」）
// ---------------------------------------------------------------------------

/** 归档 UI 证据检查入参 */
export interface UiArchiveInput {
	/** change 绑定 schema；null/legacy → 不检查（旧 change 不硬阻断） */
	schema: string | null;
	proposalText: string | null;
	uiDesignText: string | null;
	tasksMd: string | null;
	/** change 目录递归普通文件清单 */
	changeDirFiles: readonly string[];
}

/**
 * 归档证据检查（block 级缺项清单；空数组 = 通过）。
 * - legacy schema → 空数组（不硬阻断）
 * - none：N/A 一致性（marker 与 proposal 声明一致 + N/A 节在）
 * - minor：ui-design.md Acceptance/验收节映射到 组件测试|opencli|人工 至少其一
 * - major：approval=approved + 原型存在 + tasks 验证节有 opencli 与 1440×900、1920×1080
 *   证据 + ui-design.md 有差异说明（「差异」字样）
 * 前置的合同合法性（marker 缺失等）由 evaluateUiContract 保证——归档时若合同本身
 * 不合法也列缺项（不区分 ui-design-missing/ui-approval-pending，统一
 * ui-verification-missing 语义由调用方记账）。
 */
export function missingArchiveUiEvidence(input: UiArchiveInput): string[] {
	if (input.schema !== UI_SCHEMA) return [];
	const missing: string[] = [];

	const declared = parseUiImpactDeclaration(input.proposalText);
	if (!declared) {
		return ["proposal 缺失/非法 ui-impact 声明（归档前必须补齐）"];
	}
	if (input.uiDesignText === null) {
		return ["ui-design.md 缺失（新 schema change 归档前必须存在）"];
	}

	if (declared === "none") {
		const designImpact = parseUiImpactMarker(input.uiDesignText);
		if (designImpact !== "none") missing.push("ui-design.md 的 ui-impact 与 proposal 声明(none)不一致");
		if (!hasHeading(input.uiDesignText, NA_SECTION)) missing.push("none 档缺 `## N/A Reason` 记录节");
		return missing;
	}

	if (declared === "minor") {
		const acc = extractSection(input.uiDesignText, ["Acceptance", "验收"]);
		if (!/(组件测试|component|opencli|人工)/i.test(acc)) {
			missing.push("minor 档缺验收映射（ui-design.md 验收节须映射到 组件测试/opencli/人工验证 至少其一）");
		}
		return missing;
	}

	// major
	const approval = parseUiApprovalMarker(input.uiDesignText);
	if (approval !== "approved") missing.push("major 档 ui-approval 必须为 approved（原型已获用户确认）");
	const protoRaw = parseUiPrototypeMarker(input.uiDesignText);
	if (protoRaw === null || protoRaw === "none") {
		missing.push("major 档缺 ui-prototype 引用");
	} else if (validatePrototypePath(protoRaw, input.changeDirFiles) !== null) {
		missing.push(`major 档原型文件不可用（${protoRaw}）`);
	}
	const tasks = input.tasksMd ?? "";
	const tail = extractTailSections(tasks);
	if (!/opencli/i.test(tail)) missing.push("tasks.md 验证节缺 opencli 主交互链路断言证据");
	if (!tail.includes("1440") || !tail.includes("1920")) {
		missing.push("tasks.md 验证节缺 1440×900 与 1920×1080 两档视觉检查证据");
	}
	const accSection = extractSection(input.uiDesignText, ["Acceptance", "验收"]);
	if (!accSection.includes("差异")) {
		missing.push("ui-design.md Acceptance 节缺实现与批准原型的差异说明");
	}
	return missing;
}

/** 提取首个命中别名的 `## 节` 正文（到下一个 ## 或文末）；无命中返回空串 */
function extractSection(text: string, aliases: readonly string[]): string {
	const re = new RegExp(`^##\\s+.*(?:${aliases.join("|")}).*$`, "im");
	const m = re.exec(text);
	if (!m) return "";
	const start = m.index + m[0].length;
	const rest = text.slice(start);
	const next = rest.search(/^##\s+/m);
	return (next === -1 ? rest : rest.slice(0, next)).trim();
}

/** 提取 tasks.md 尾部「测试/文档/验证」三节文本（从首个 `## N. 测试` 起到文末；找不到退全文） */
function extractTailSections(tasksMd: string): string {
	const m = /^##\s+\d+\.\s*测试/m.exec(tasksMd);
	return m ? tasksMd.slice(m.index) : tasksMd;
}
