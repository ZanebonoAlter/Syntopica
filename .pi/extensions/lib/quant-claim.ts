/**
 * quant-claim-gate — 可量化断言声明门禁共享纯函数
 *
 * 动机（2026-11-27，change lane-first-recall 审查 R-11）：
 * design.md 里写了「查询 MUST 用该表达式才能命中索引」，审查员跑 EXPLAIN 三份计划全部
 * 走 Sort，索引一次没出现——**声称了性能结果但没有证据**。
 * 同类问题：数据口径断言（如「矩阵日格 = 该日各桶线索数之和」）在生产数据上从未被复算过。
 *
 * 边界（为什么不是关键词扫描）：关键词命中靠措辞运气，换个说法就漏；判据必须落在
 * 结构化字段上。因此本库只做一件事——校验 `<!-- quant-claims: ... -->` 声明与
 * `## 可量化断言` 固定表的**字面合规性**，不判断断言真假（真假由 review 复核）。
 *
 * 与既有验证载体的分工（不得重叠，见 docs/reference/开发执行规范.md）：
 *   ① spec Scenario        = 行为契约（对所有输入成立）
 *   ② tasks Scenario→测试  = 契约的测试映射（scenario-trace 门禁）
 *   ③ tasks 验证命令        = 跑完即知 PASS/FAIL 的门禁命令
 *   ④ 可量化断言（本库）    = 对某一时刻真实数据/真实查询计划的观测（会漂移，故必填观测日期）
 *
 * 判据一句话：**能被一次 `go test` 完整判定的，写测试；不能的（依赖真实数据集/真实查询
 * 计划/采样分布），才进本表。**
 *
 * 纯函数纪律：不 import fs、不触网、不抛异常——畸形输入返回确定问题列表；
 * 文件存在性由调用方读好正文后传入。
 */

/** 头部 marker 键（与 complexity / ui-impact 同级） */
export const QUANT_MARKER = "quant-claims";

/** marker 合法值 */
export const QUANT_MARKER_VALUES = ["none", "declared"] as const;
export type QuantMarkerValue = (typeof QUANT_MARKER_VALUES)[number];

/** 断言表所在节标题（逐字） */
export const QUANT_SECTION_TITLE = "## 可量化断言";

/** 表头列（逐字，顺序固定） */
export const QUANT_TABLE_HEADER = [
	"类别",
	"断言",
	"观测值",
	"复算方式",
	"观测日期",
	"状态",
] as const;

/** 类别合法值 */
export const QUANT_CATEGORIES = ["性能", "数据"] as const;

/** 状态合法值 */
export const QUANT_STATUSES = ["measured", "unmeasured", "n/a"] as const;
export type QuantStatus = (typeof QUANT_STATUSES)[number];

/** 问题码（稳定有界，入 policy 账本用） */
export const QUANT_REASON_CODES = [
	"quant-marker-missing",
	"quant-marker-invalid",
	"quant-section-missing",
	"quant-section-unexpected",
	"quant-table-header-missing",
	"quant-row-malformed",
	"quant-category-invalid",
	"quant-status-invalid",
	"quant-evidence-missing",
	"quant-claim-not-comparable",
] as const;
export type QuantReasonCode = (typeof QUANT_REASON_CODES)[number];

export interface QuantIssue {
	reasonCode: QuantReasonCode;
	/** 人类可读定位（行号 + 摘要），直接进 warn 文案 */
	detail: string;
}

/** 解析头部 15 行内的 `<!-- quant-claims: ... -->`（首个匹配即终止，与 domain marker 同策略） */
export function parseQuantMarker(content: string): { found: boolean; value: string } {
	const lines = content.split(/\r?\n/);
	for (let i = 0; i < Math.min(lines.length, 15); i++) {
		const line = lines[i] ?? "";
		const m = /<!--\s*quant-claims\s*:\s*([^>]*?)\s*-->/.exec(line);
		if (m) return { found: true, value: (m[1] ?? "").trim() };
	}
	return { found: false, value: "" };
}

/** 抽出 `## 可量化断言` 节的正文（到下一个同级或更高级标题为止） */
export function extractQuantSection(content: string): string | null {
	const lines = content.split(/\r?\n/);
	const start = lines.findIndex((l) => l.trim() === QUANT_SECTION_TITLE);
	if (start < 0) return null;
	const body: string[] = [];
	for (let i = start + 1; i < lines.length; i++) {
		const line = lines[i] ?? "";
		if (/^#{1,2}\s/.test(line)) break; // 下一个 h1/h2 结束本节（h3 允许，表格前可有小标题）
		body.push(line);
	}
	return body.join("\n");
}

/** 表格行解析：只认以 `|` 开头且非分隔行（|---|）的行 */
function tableRows(section: string): string[][] {
	const rows: string[][] = [];
	for (const raw of section.split(/\r?\n/)) {
		const line = raw.trim();
		if (!line.startsWith("|")) continue;
		if (/^\|[\s:|-]+\|$/.test(line)) continue; // 分隔行
		const cells = line
			.replace(/^\|/, "")
			.replace(/\|$/, "")
			.split("|")
			.map((c) => c.trim());
		rows.push(cells);
	}
	return rows;
}

/** 纯形容词/无判据的断言：去掉这些词后若没有数字或比较特征，即视为不可比较 */
const VAGUE_CLAIM_PATTERNS = [
	/^更快$/,
	/^更稳$/,
	/^更好$/,
	/^无明显(开销|影响|变化)$/,
	/^性能(更|有)?(好|优)$/,
	/^不(会)?(变慢|影响性能)$/,
];

/**
 * 断言是否「可比较」：含数字（含小数/百分比/毫秒），或含明确的计划/结构特征词。
 * 只挡纯形容词——例如「无明显开销」单独出现时无法判定该看什么。
 */
export function isComparableClaim(claim: string): boolean {
	const text = claim.trim();
	if (text === "") return false;
	if (/\d/.test(text)) return true;
	if (VAGUE_CLAIM_PATTERNS.some((re) => re.test(text))) return false;
	// 非数字但可判定的计划/结构特征
	return /(Index Scan|Seq Scan|Sort|索引|命中|走|次数|条数|占比|比例|分布|恒等|等于|率|量|阈值|=)/.test(text);
}

/**
 * 校验 design.md 正文。
 * 调用方负责：只对**活跃** change 调用（归档 change 不校验，避免存量集体报错）。
 *
 * @param content design.md 全文
 */
export function checkQuantClaims(content: string): QuantIssue[] {
	const issues: QuantIssue[] = [];
	const marker = parseQuantMarker(content);
	const section = extractQuantSection(content);

	// ① marker 必须存在且合法
	if (!marker.found) {
		issues.push({
			reasonCode: "quant-marker-missing",
			detail: `design.md 头部缺 <!-- ${QUANT_MARKER}: none|declared -->`,
		});
	} else if (!(QUANT_MARKER_VALUES as readonly string[]).includes(marker.value)) {
		issues.push({
			reasonCode: "quant-marker-invalid",
			detail: `marker 值 "quant-claims: ${marker.value}" 非法（只接受 none / declared）`,
		});
	}

	// ② marker=none 但正文有该节 → 声明与正文打架
	if (marker.found && marker.value === "none" && section !== null) {
		issues.push({
			reasonCode: "quant-section-unexpected",
			detail: `marker 声明 none，但正文存在「${QUANT_SECTION_TITLE}」节`,
		});
	}

	// ③ marker=declared → 节与表必须存在且合规
	if (marker.found && marker.value === "declared") {
		if (section === null) {
			issues.push({
				reasonCode: "quant-section-missing",
				detail: `marker 声明 declared，但缺「${QUANT_SECTION_TITLE}」节`,
			});
			return issues;
		}
		const rows = tableRows(section);
		if (rows.length === 0) {
			issues.push({
				reasonCode: "quant-table-header-missing",
				detail: `「${QUANT_SECTION_TITLE}」节内无表格`,
			});
			return issues;
		}
		const header = rows[0] ?? [];
		const headerOk = QUANT_TABLE_HEADER.every((col, idx) => header[idx] === col);
		if (!headerOk) {
			issues.push({
				reasonCode: "quant-table-header-missing",
				detail: `表头必须逐字为 | ${QUANT_TABLE_HEADER.join(" | ")} |`,
			});
			return issues;
		}
		const dataRows = rows.slice(1);
		if (dataRows.length === 0) {
			issues.push({
				reasonCode: "quant-row-malformed",
				detail: "断言表只有表头，没有数据行（无断言请改 marker 为 none）",
			});
			return issues;
		}
		dataRows.forEach((cells, i) => {
			const lineNo = i + 2; // 表内序号（表头为第 1 行）
			const [category = "", claim = "", observed = "", method = "", date = "", status = ""] = cells;
			if (cells.length < QUANT_TABLE_HEADER.length) {
				issues.push({
					reasonCode: "quant-row-malformed",
					detail: `第 ${lineNo} 行数据列不足 ${QUANT_TABLE_HEADER.length} 列`,
				});
				return;
			}
			if (!(QUANT_CATEGORIES as readonly string[]).includes(category)) {
				issues.push({
					reasonCode: "quant-category-invalid",
					detail: `第 ${lineNo} 行类别 "${category}" 非法（只接受 ${QUANT_CATEGORIES.join(" / ")}）`,
				});
			}
			if (!(QUANT_STATUSES as readonly string[]).includes(status)) {
				issues.push({
					reasonCode: "quant-status-invalid",
					detail: `第 ${lineNo} 行状态 "${status}" 非法（只接受 ${QUANT_STATUSES.join(" / ")}）`,
				});
			}
			if (!isComparableClaim(claim)) {
				issues.push({
					reasonCode: "quant-claim-not-comparable",
					detail: `第 ${lineNo} 行断言不可比较（需含数值或计划/结构判据）："${claim}"`,
				});
			}
			// measured 必须给出观测值 + 复算方式 + 观测日期
			if (status === "measured") {
				const missing: string[] = [];
				if (!observed || observed === "—" || observed === "-") missing.push("观测值");
				if (!method || method === "—" || method === "-") missing.push("复算方式");
				if (!date || date === "—" || date === "-") missing.push("观测日期");
				if (missing.length > 0) {
					issues.push({
						reasonCode: "quant-evidence-missing",
						detail: `第 ${lineNo} 行状态 measured 但缺：${missing.join("、")}`,
					});
				}
			}
		});
	}

	return issues;
}
