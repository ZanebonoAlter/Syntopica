/**
 * stage — openspec change 阶段标识 + 阶段路由清单（change: stage-driven-constraint-injection）
 *
 * 权威契约：openspec/changes/stage-driven-constraint-injection/specs/constraint-injection/spec.md
 *   「stage 进度标识（替代档位机制）」+「阶段驱动的约束路由与自主导入」。
 * 决策来源：同 change design.md D1（载体 = change 目录内 stage.md）、D2（阶段族为派生值，
 *   不可独立写）、D5（阶段↔文档的对应关系属「仓库知识」，放常量表不写死在逻辑分支里）。
 *
 * 纯函数纪律（同 lib/ui-design-gate.ts）：parseStageMarker / stageFamilyOf / isLegacyStage /
 * buildStageRouting / normalizeRepoPath / isConstraintDocPath 不触网、不抛异常——同一输入
 * → 同一输出。**唯一 IO 边界**是 readStageFile（读 `<changeDir>/stage.md`）：文件不存在 →
 * missing，**不抛错、不记错误日志**（缺失 = legacy change，行为与改造前一致，不阻断任何流程，
 * 含归档）。
 *
 * 三态语义（严格对齐 lib/ui-design-gate.ts 的 parseUiStage 先例）：
 *   missing = 无 marker（含文件不存在/空文件）→ legacy，不阻断；
 *   invalid = marker 重复出现，或值不在 6 档取值域内（含大小写不匹配）→ legacy 降级，不阻断；
 *   ok      = 恰好一条合法 marker → stage 生效。
 */

import { readFileSync } from "node:fs";
import { join } from "node:path";

/* ---------- 阶段取值域与派生映射 ---------- */

/** 6 档阶段（单调阶梯，顺序即进度顺序）；契约见 spec「stage 进度标识」取值域表 */
export const STAGES = [
	"explore",
	"propose",
	"design",
	"implement",
	"verify",
	"archive",
] as const;
export type Stage = (typeof STAGES)[number];

/** 阶段族（派生值，**MUST NOT 作为独立可写状态**——spec 明文） */
export type StageFamily = "planning" | "implementation";

/** stage → 阶段族（派生唯一真相源；族只服务三处纯布尔判定） */
export const STAGE_FAMILY_MAP: Readonly<Record<Stage, StageFamily>> = {
	explore: "planning",
	propose: "planning",
	design: "planning",
	implement: "implementation",
	verify: "implementation",
	archive: "implementation",
};

/**
 * legacy `mode.set` → 阶段族映射（事实库双读过渡用，design D7）：
 * `requirements`→planning、`implementation`→implementation。
 * 30 天保留期自然消退后双读可移除，本表随之作废。
 */
export const LEGACY_MODE_FAMILY_MAP: Readonly<Record<string, StageFamily>> = {
	requirements: "planning",
	implementation: "implementation",
};

/** stage marker 文件名（change 目录内非 schema 制品，同 explore-findings.md 性质） */
export const STAGE_FILE_NAME = "stage.md";

/* ---------- marker 解析（纯函数） ---------- */

/** parseStageMarker 三态：missing（无 marker = legacy）/ invalid（重复或值非法）/ ok */
export type StageParse =
	| { kind: "missing" }
	| { kind: "invalid" }
	| { kind: "ok"; stage: Stage };

/**
 * 解析 `<!-- stage: <值> -->` marker。
 * 语义严格对齐 lib/ui-design-gate.ts 的 parseUiStage：
 * - 非字符串/空串 → missing；
 * - 0 条 marker → missing；
 * - >1 条 marker（无论值是否相同）→ invalid；
 * - 值不在 6 档取值域内（含大小写不匹配、尾随逗号）→ invalid。
 */
export function parseStageMarker(text: string | null | undefined): StageParse {
	if (typeof text !== "string" || text.length === 0) return { kind: "missing" };
	const matches = [
		...text.matchAll(/<!--\s*stage\s*:\s*([^>]*?)\s*-->/g),
	];
	if (matches.length === 0) return { kind: "missing" };
	if (matches.length > 1) return { kind: "invalid" };
	const v = matches[0][1].trim();
	return (STAGES as readonly string[]).includes(v)
		? { kind: "ok", stage: v as Stage }
		: { kind: "invalid" };
}

/** 阶段族派生：explore/propose/design → planning；implement/verify/archive → implementation。非法输入 → null（不抛错）。 */
export function stageFamilyOf(
	stage: string | null | undefined,
): StageFamily | null {
	if (typeof stage !== "string") return null;
	const v = stage.trim();
	return (STAGE_FAMILY_MAP as Record<string, StageFamily>)[v] ?? null;
}

/** legacy `mode.set` 的 mode 值 → 阶段族（双读过渡用）；未知 mode → null */
export function stageFamilyOfLegacyMode(
	mode: string | null | undefined,
): StageFamily | null {
	if (typeof mode !== "string") return null;
	return LEGACY_MODE_FAMILY_MAP[mode.trim()] ?? null;
}

/** legacy 判定：missing / invalid → true（按 legacy 降级，不阻断）；ok → false */
export function isLegacyStage(parse: StageParse | null | undefined): boolean {
	if (!parse) return true;
	return parse.kind !== "ok";
}

/* ---------- stage.md 读取（唯一 IO 边界） ---------- */

/** readStageFile 结果：kind 三态同 parseStageMarker；raw 为文件原文（读不到时为空串） */
export interface StageFileResult {
	kind: StageParse["kind"];
	/** kind=ok 时的阶段值 */
	stage?: Stage;
	/** 文件原文（不存在/不可读时为空串） */
	raw: string;
}

/** `<changeDir>/stage.md` 路径拼装 */
export function stageFilePath(changeDir: string): string {
	return join(changeDir, STAGE_FILE_NAME);
}

/** 仓库根相对路径拼装：`openspec/changes/<name>/stage.md` */
export function changeStageFilePath(changeName: string): string {
	return `openspec/changes/${changeName}/${STAGE_FILE_NAME}`;
}

/**
 * 读 change 目录内的 stage.md 并解析 marker。
 * 文件不存在 / 不可读 / 空文件 → missing（**不抛错、不记错误日志**——缺失是 legacy 的
 * 正常形态，MUST NOT 阻断任何流程）；marker 重复或值非法 → invalid（同样降级不阻断）。
 */
export function readStageFile(changeDir: string): StageFileResult {
	if (typeof changeDir !== "string" || changeDir.length === 0) {
		return { kind: "missing", raw: "" };
	}
	let raw = "";
	try {
		raw = readFileSync(stageFilePath(changeDir), "utf8");
	} catch {
		return { kind: "missing", raw: "" };
	}
	const parsed = parseStageMarker(raw);
	if (parsed.kind === "ok") return { kind: "ok", stage: parsed.stage, raw };
	return { kind: parsed.kind, raw };
}

/* ---------- 阶段路由清单（spec「阶段驱动的约束路由与自主导入」） ---------- */

/** 路由清单一项：文档路径 + 节名 + 取数方式（**SHALL NOT 内联文档正文**） */
export interface StageRoutingItem {
	/** 文档路径（仓库根相对，正斜杠） */
	path: string;
	/** 文档内节名；null = 整篇 */
	section: string | null;
	/** 取数方式（read 指引 / constraint-digest 命令） */
	how: string;
}

/** buildStageRouting 入参：声明域（业务红线文档）+ 命中栈（代码规范按栈条件命中） */
export interface StageRoutingOptions {
	/** change 声明的业务域（`<!-- constraint-domains: -->` 的域名列表） */
	domainDocs?: string[];
	/** 命中的技术栈；null = 未命中（implement 族无代码规范文档） */
	stack?: "backend" | "frontend" | null;
	/**
	 * 配置驱动的阶段文档清单（`.pi/constraint-injection.json` 的 `stages.<stage>.docs`）。
	 * 给了它 = 配置为该阶段的**真相源**（路径只选文档，节名仍取目录）；
	 * 不给 = 回退代码内的 STAGE_ROUTING_PLAN + 常量（无配置语境：临时目录 / 烟测）。
	 */
	select?: StageDocsSelect;
	/** 索引文档路径（缺省取 ROUTING_INDEX_DOC；配置 `indexDoc` 变更时由调用方传入） */
	indexDoc?: string;
}

/** 配置里的阶段文档清单：路径集合（节名由 ROUTING_DOC_CATALOG 唯一维护） */
export interface StageDocsSelect {
	/** 阶段必读文档（无条件进入清单） */
	required?: string[];
	/** 按栈条件文档（命中栈才进入清单） */
	conditional?: Partial<Record<"backend" | "frontend", string[]>>;
}

/** 索引文档（各阶段共有：约束索引是「该去哪读」的入口） */
export const ROUTING_INDEX_DOC = {
	path: "docs/reference/constraints-index.md",
	section: "执行规范（how，写对代码）",
} as const;

/** UI 契约规范（design 及之后） */
export const ROUTING_UI_CONTRACT_DOCS: readonly {
	path: string;
	section: string;
}[] = [
	{ path: "docs/reference/standard/frontend/layout.md", section: "Requirements" },
	{
		path: "docs/reference/standard/frontend/interaction.md",
		section: "Requirements",
	},
];

/** 代码与测试规范（implement 及之后；按栈命中 backend / frontend） */
export const ROUTING_CODE_DOCS_BY_STACK: Readonly<
	Record<"backend" | "frontend", readonly { path: string; section: string }[]>
> = {
	backend: [
		{
			path: "docs/reference/standard/backend/code-style.md",
			section: "格式与静态分析",
		},
		{
			path: "docs/reference/standard/backend/lint.md",
			section: "推荐启用 linters",
		},
		{
			path: "docs/reference/standard/backend/package-layout.md",
			section: "Domain 三层结构（统一约束）",
		},
		{
			path: "docs/reference/standard/backend/testing.md",
			section: "测试分层",
		},
	],
	frontend: [
		{
			path: "docs/reference/standard/frontend/code-style.md",
			section: "1. 通用约定",
		},
		{
			path: "docs/reference/standard/frontend/lint.md",
			section: "启用规则",
		},
		{
			path: "docs/reference/standard/frontend/testing.md",
			section: "单元测试（Vitest）",
		},
	],
};

/** 验收与归档规范（verify / archive） */
export const ROUTING_ACCEPTANCE_DOCS: readonly {
	path: string;
	section: string;
}[] = [
	{
		path: "docs/reference/standard/shared/test-design.md",
		section: "永久业务测试只留两类",
	},
	{ path: "docs/reference/standard/shared/commit-pr.md", section: "提交前检查" },
	{ path: "docs/reference/standard/shared/doc-authoring.md", section: "目录职责表" },
];

/**
 * 每阶段的清单装配计划（累加式，对齐 spec 表格「上述 + …」）：
 * explore/propose = 索引 + 声明域；design 加 UI 契约；implement 加代码与测试规范；
 * verify/archive 再加验收与归档规范。
 *
 * **无配置兜底**：`.pi/constraint-injection.json` 的 `stages.<stage>.docs` 缺席时
 * （临时目录 / 烟测 / 配置未升级）由本表 + 上方常量装配清单，语义与配置形态等价。
 */
export const STAGE_ROUTING_PLAN: Readonly<
	Record<Stage, { ui: boolean; code: boolean; acceptance: boolean }>
> = {
	explore: { ui: false, code: false, acceptance: false },
	propose: { ui: false, code: false, acceptance: false },
	design: { ui: true, code: false, acceptance: false },
	implement: { ui: true, code: true, acceptance: false },
	verify: { ui: true, code: true, acceptance: true },
	archive: { ui: true, code: true, acceptance: true },
};

/**
 * 路由文档目录（path → 节名）：配置里的阶段清单按**路径**选取文档，节名只此一处维护
 * （配置不写节名 → 避免「配置 + 代码」两处维护同一节名导致漂移）。
 */
export const ROUTING_DOC_CATALOG: Readonly<Record<string, string | null>> =
	Object.freeze(
		Object.fromEntries([
			[ROUTING_INDEX_DOC.path, ROUTING_INDEX_DOC.section],
			...ROUTING_UI_CONTRACT_DOCS.map((d) => [d.path, d.section]),
			...ROUTING_CODE_DOCS_BY_STACK.backend.map((d) => [d.path, d.section]),
			...ROUTING_CODE_DOCS_BY_STACK.frontend.map((d) => [d.path, d.section]),
			...ROUTING_ACCEPTANCE_DOCS.map((d) => [d.path, d.section]),
			// 图与伪代码约定（marker 在 propose 产出，故 explore/propose 即可读）
			[
				"docs/reference/开发执行规范.md",
				"13. change 头部声明与图表达（stage / 域 / diagrams marker）",
			],
		]),
	);

/** 目录查节名：未知路径 → null（整篇读，fail-open 不丢文档） */
export function catalogSection(path: string): string | null {
	return ROUTING_DOC_CATALOG[path] ?? null;
}

/** flow 文档的约束节名（业务红线所在节） */
const FLOW_SECTION = "业务约束与不变量";

/** read 取数指引（不含正文，只给路径 + 节名） */
function readHow(path: string, section: string | null): string {
	return section
		? `read \`${path}\`「${section}」`
		: `read \`${path}\`（整篇）`;
}

/** 域红线摘要取数命令（与 scripts/harness/constraint-digest.sh --brief 同形） */
function digestHow(domain: string): string {
	return `bash scripts/harness/constraint-digest.sh --brief --domains ${domain}`;
}

/** 合法业务域名（防路径穿越：只允许字母数字点下划线连字符，且不含 `..`） */
function isLegalDomain(domain: string): boolean {
	return /^[A-Za-z0-9._-]+$/.test(domain) && !domain.includes("..");
}

/**
 * 按当前 stage 装配路由清单（**只含路径 + 节名 + 取数方式，不内联任何文档正文**）。
 * 组成：索引文档 → 声明域 flow 文档（红线层已随注入送达，清单给细节层取数方式）→
 * 阶段文档清单（options.select = 配置里的 `stages.<stage>.docs`；未给时回退
 * STAGE_ROUTING_PLAN + 常量）。未知/非法 stage → 空清单（不抛错）。
 */
export function buildStageRouting(
	stage: Stage | string | null | undefined,
	options?: StageRoutingOptions,
): StageRoutingItem[] {
	const plan =
		typeof stage === "string"
			? STAGE_ROUTING_PLAN[stage as Stage]
			: undefined;
	if (!plan) return [];

	const items: StageRoutingItem[] = [];
	const seen = new Set<string>();
	const push = (path: string, section: string | null, how: string): void => {
		if (seen.has(path)) return;
		seen.add(path);
		items.push({ path, section, how });
	};

	// ① 索引文档（各阶段共有；路径取配置的 indexDoc，节名取目录）
	const indexPath = options?.indexDoc ?? ROUTING_INDEX_DOC.path;
	const indexSection = catalogSection(indexPath);
	push(indexPath, indexSection, readHow(indexPath, indexSection));

	// ② 声明域 flow 文档（业务红线：细节层指引 + 红线摘要命令）
	const domains = options?.domainDocs ?? [];
	for (const raw of domains) {
		const domain = typeof raw === "string" ? raw.trim() : "";
		if (domain.length === 0 || !isLegalDomain(domain)) continue;
		const p = `docs/reference/flow/${domain}.md`;
		push(
			p,
			FLOW_SECTION,
			`${digestHow(domain)}（红线摘要）+ ${readHow(p, FLOW_SECTION)}（细节层）`,
		);
	}

	// ③ 配置驱动的阶段清单（`stages.<stage>.docs`）：配置为真相源，节名取目录
	//    （容错：配置写成非数组/非字符串时跳过该项，MUST NOT 抛错——注入 fail-open）
	const select = options?.select;
	if (select) {
		const pushByPath = (path: unknown): void => {
			if (typeof path !== "string" || path.length === 0) return;
			const section = catalogSection(path);
			push(path, section, readHow(path, section));
		};
		const required = select.required;
		if (Array.isArray(required)) for (const path of required) pushByPath(path);
		const stack = options?.stack;
		if (stack === "backend" || stack === "frontend") {
			const conditional = select.conditional?.[stack];
			if (Array.isArray(conditional)) for (const path of conditional) pushByPath(path);
		}
		return items;
	}

	// ④ 无配置清单 → 代码兜底（STAGE_ROUTING_PLAN + 常量；临时目录 / 烟测语境）
	//    UI 契约规范（design 及之后）
	if (plan.ui) {
		for (const d of ROUTING_UI_CONTRACT_DOCS) {
			push(d.path, d.section, readHow(d.path, d.section));
		}
	}

	// ⑤ 代码与测试规范（implement 及之后，按栈命中）
	if (plan.code) {
		const stack = options?.stack;
		if (stack === "backend" || stack === "frontend") {
			for (const d of ROUTING_CODE_DOCS_BY_STACK[stack]) {
				push(d.path, d.section, readHow(d.path, d.section));
			}
		}
	}

	// ⑥ 验收与归档规范（verify / archive）
	if (plan.acceptance) {
		for (const d of ROUTING_ACCEPTANCE_DOCS) {
			push(d.path, d.section, readHow(d.path, d.section));
		}
	}

	return items;
}

/* ---------- 约束文档路径归一化（constraint.read 白名单判定用） ---------- */

/** 约束文档白名单：flow 域文档顶层 .md + standard 规范文档（任意层级 .md） */
const FLOW_DOC_RE = /^docs\/reference\/flow\/[^/]+\.md$/;
const STANDARD_DOC_RE = /^docs\/reference\/standard\/.+\.md$/;

/**
 * 读取路径 → 仓库根相对路径（正斜杠）。纯函数（不查盘）：
 * - 反斜杠归一（Windows 风格路径）；
 * - 绝对路径剥仓库根前缀；不在仓库根内 → null；
 * - `.`/`..` 段解析；越出仓库根 → null；
 * - 重复斜杠与前导 `./` 归一。
 * 大小写**保持原样**（Linux 大小写敏感：大小写不一致的路径视为非同一文档）。
 */
export function normalizeRepoPath(
	rawPath: string | null | undefined,
	repoRoot?: string | null,
): string | null {
	if (typeof rawPath !== "string") return null;
	const trimmed = rawPath.trim();
	if (trimmed.length === 0) return null;

	let p = trimmed.replaceAll("\\", "/");
	const root =
		typeof repoRoot === "string"
			? repoRoot.replaceAll("\\", "/").replace(/\/+$/, "")
			: "";
	const isAbsolute = p.startsWith("/");
	if (root.length > 0) {
		if (p === root) return null;
		if (p.startsWith(`${root}/`)) p = p.slice(root.length + 1);
		else if (isAbsolute) return null; // 仓库根之外的绝对路径
	} else if (isAbsolute) {
		p = p.replace(/^\/+/, "");
	}

	p = p.replace(/\/{2,}/g, "/").replace(/^\.\//, "");
	const out: string[] = [];
	for (const seg of p.split("/")) {
		if (seg.length === 0 || seg === ".") continue;
		if (seg === "..") {
			if (out.length === 0) return null; // 越出仓库根
			out.pop();
			continue;
		}
		out.push(seg);
	}
	if (out.length === 0) return null;
	return out.join("/");
}

/** 路径是否命中约束文档白名单（flow 域文档 / standard 规范文档） */
export function isConstraintDocPath(relPath: string | null | undefined): boolean {
	if (typeof relPath !== "string" || relPath.length === 0) return false;
	const p = relPath.replaceAll("\\", "/");
	return FLOW_DOC_RE.test(p) || STANDARD_DOC_RE.test(p);
}
