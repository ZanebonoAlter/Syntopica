/**
 * test-case-gate — 白盒用例文档门禁共享纯函数（case-first-testing 复杂度声明制）
 *
 * 自 spec-gate.ts 检查⑤抽取（change: test-case-entry-gate，design D2）：
 * entry-gate（动工入口 steer）与 spec-gate（归档措辞扫描 warn）同源调用，
 * 不出现两份判定逻辑。纯函数：不 import fs、不触网、不抛异常。
 *
 * 同步义务：改 COMPLEXITY_KEYWORDS 必同步 docs/reference/standard/shared/test-design.md
 * 「验收措辞规范」节禁用词表（双向声明，缺一不可）。
 */

/** ⑤a 复杂档兜底关键词（权威源：test-design.md「验收措辞规范」节；词表不扩容——
 *  全量校准（2026-08-25，75 change）：4 词 fire 33%，扩到 19 词 fire 69% 只稀释警报，
 *  主判定是 proposal 头 complexity 声明，词表仅兜底质询未声明/声明 simple 的档） */
export const COMPLEXITY_KEYWORDS = ["算法", "状态机", "解析", "协议"] as const;

export type ComplexityDeclaration = "complex" | "simple" | null;

/**
 * 识别 proposal 头部复杂度声明 `<!-- complexity: complex|simple -->`。
 * 容忍 `complexity:` 前后空白；多个声明取首个；值大小写敏感（complex/simple 小写）；
 * 其他值/缺声明/非字符串输入 → null（视为未声明）。
 */
export function parseComplexityDeclaration(
	proposalText: string | null | undefined,
): ComplexityDeclaration {
	if (typeof proposalText !== "string" || proposalText.length === 0) return null;
	const m = /<!--\s*complexity\s*:\s*(complex|simple)\s*-->/.exec(proposalText);
	return m ? ((m[1] as "complex" | "simple")) : null;
}

/** change 目录是否已检出白盒用例文档：test-cases*.md 前缀 或 *-test-cases.md 后缀 */
export function hasTestCaseDoc(changeDirFiles: string[]): boolean {
	const files = Array.isArray(changeDirFiles) ? changeDirFiles : [];
	return files.some(
		(f) => /^test-cases.*\.md$/.test(f) || /-test-cases\.md$/.test(f),
	);
}

/**
 * 兜底词法信号：扫 tasks.md 任务行（行首允许缩进的 `- [ ]`/`- [x]` 锚定，取到行尾）
 * 收集命中的复杂档关键词（按常量表序去重）。非任务行命中不算。
 */
export function scanComplexityKeywords(tasksMd: string | null | undefined): string[] {
	if (typeof tasksMd !== "string" || tasksMd.length === 0) return [];
	const hits = new Set<string>();
	for (const raw of tasksMd.split("\n")) {
		if (!/^\s*-\s\[[ x]\]/.test(raw)) continue;
		const text = raw.replace(/^\s*-\s\[[ x]\]\s*/, "");
		for (const kw of COMPLEXITY_KEYWORDS) {
			if (text.includes(kw)) hits.add(kw);
		}
	}
	return [...hits];
}

/** decideEntryReminder 入参（全量快照，纯函数可测） */
export interface EntryReminderInput {
	/** 最新 mode.set 的 mode；null = 本会话无档位记录（requirements/未激活同理归静默） */
	mode: string | null;
	/** 绑定的 change 名；null = 未绑定 */
	boundChange: string | null;
	/** change 目录文件列表（读不到 → 空数组，视为无文档但不必然提醒） */
	changeDirFiles: string[];
	/** proposal.md 文本；null = 读不到 */
	proposalText: string | null;
	/** tasks.md 文本；null = 读不到 */
	tasksMd: string | null;
	/** 本会话内该 change 是否已提醒过（去重） */
	alreadyWarned: boolean;
}

/** 判定结果；null = 静默 */
export interface EntryReminder {
	/** 声明 complex = 强提醒（确定性核对）；simple/null + 词表命中 = 兜底质询 */
	strong: boolean;
	declaration: ComplexityDeclaration;
	/** 兜底词法命中的关键词（强提醒时也可能携带，供文案） */
	kwHits: string[];
}

/**
 * 入口门禁纯决策（design D3）：
 * ① requirements 档 / 未绑定 / 已提醒过 → 静默；
 * ② 已有白盒用例文档 → 静默；
 * ③ 声明 complex + 缺文档 → 强提醒（主信号，零误报）；
 * ④ 声明 simple / 未声明 + 词表命中 → 兜底质询提醒；
 * ⑤ 其余（未声明未命中 / simple 未命中）→ 静默。
 */
export function decideEntryReminder(
	input: EntryReminderInput,
): EntryReminder | null {
	if (input.mode !== "implementation" || !input.boundChange) return null;
	if (input.alreadyWarned) return null;
	if (hasTestCaseDoc(input.changeDirFiles)) return null;

	const declaration = parseComplexityDeclaration(input.proposalText);
	const kwHits = scanComplexityKeywords(input.tasksMd);
	if (declaration === "complex") {
		return { strong: true, declaration, kwHits };
	}
	if (kwHits.length > 0) {
		return { strong: false, declaration, kwHits };
	}
	return null;
}

/** 入口提醒文案（steer 用）；strong 与兜底两档，修复路径都是补文档或改声明 */
export function buildEntryReminderMessage(
	change: string,
	r: EntryReminder,
): string {
	const lines: string[] = [
		`⚠️ [entry-gate] 「${change}」已切入实现档，但 change 目录未检出白盒用例文档（test-cases*.md / *-test-cases.md）。`,
	];
	if (r.strong) {
		lines.push(
			`- proposal 声明：complex —— 复杂档（状态机≥3状态 / 算法 / 多模块协议）按 case-first-testing MUST 产出白盒用例文档（分支表/边界值清单，断言判据主线程定）。`,
		);
	} else if (r.declaration === "simple") {
		lines.push(
			`- 声明与任务措辞矛盾：proposal 声明 simple，但 tasks.md 任务行命中复杂档关键词「${r.kwHits.join("、")}」。`,
		);
	} else {
		lines.push(
			`- 兜底信号：未声明复杂度，tasks.md 任务行命中关键词「${r.kwHits.join("、")}」——可能是复杂档。`,
		);
	}
	lines.push(
		`- 修复路径：补 test-cases.md（白盒用例文档），或修正 proposal 头声明（<!-- complexity: complex|simple -->）。确非复杂档可忽略本提醒。`,
	);
	return lines.join("\n");
}
