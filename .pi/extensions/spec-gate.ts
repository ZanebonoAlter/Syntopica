/**
 * spec-gate.ts — pi extension：openspec archive 归档硬门禁
 *
 * 设计决策（见 docs/reference/开发执行规范.md §11 归档门禁、§4.1 门禁分层表）：
 * 1. 挂 tool_call，仅 bash 命令命中 `openspec archive` 时介入；其余工具调用零开销直接放行
 * 2. 五项 block 级检查各自独立判定、不强制顺序（任一失败即 block）：
 *    ① `bash scripts/doc-impact.sh verify openspec/changes/<name>` 退出码 0
 *    ② `bash scripts/check-standards.sh --change <name>` 退出码 0（F 段只对账归档目标）
 *    ③ `<changeDir>/tasks.md` 含尾三节（「## N. 测试」「## N. 文档」「## N. 验证」各自独立
 *       命中一次，顺序不限）且含 `<!-- doc-impact:` 声明标记
 *    ④ `bash scripts/scenario-trace.sh <changeDir>` 退出码 0（Scenario→测试映射对账，
 *       scenario-test-mapping-gate 引入，格式约定见 openspec/specs/scenario-trace-gate）
 *    ⑤' `bash scripts/concurrency-status.sh --check <name>` 退出码 2 → warn 级提醒
 *       （coordinate-concurrent-changes：树上存在归属其他 active change 的未 commit 文件；
 *       exit 3 冷启动/库不可用零输出，绝不 block——归属是启发式，误 block 会卡死正常归档）
 *    ④' UI 验收证据检查（make-ui-design-first-class）：新 schema（syntopica-ui）change
 *       归档时按 ui-design.md 影响等级校验验收证据（major：approved+原型存在+opencli
 *       +1440×900/1920×1080+差异说明；minor：组件/opencli/人工映射；none：N/A 一致性），
 *       缺项记 policy=ui-design-gate block ui-verification-missing；旧 schema 零检查不阻断
 * 3. 失败 → { block: true, reason }：reason 中文，列失败项 + 每项尾部输出（tail 20 行）+
 *    修复指引（含 doc-impact-excuse 豁免机制与 --force / SPEC_GATE_BYPASS=1 逃生口）
 * 4. 豁免：命令带 --force 或环境变量 SPEC_GATE_BYPASS=1 → 放行，但 custom_message 记
 *    warning 留痕（复用 quality-gate 的 sendMessage customType 模式，不静默）
 * 5. 扩展自身异常 fail-open（console.warn + warning 留痕），不把 agent 卡死在门禁上；
 *    提取不到合法 change 名（交互式归档等）同样 fail-open 留痕
 * 6. 检查⑤为 warn 级措辞扫描（⑤a 复杂档关键词任务无 test-cases*.md / ⑤b 纯函数×SQLite
 *    分层错配）：命中仅走 spec-gate-warning 留痕，绝不进 block 判定；自身异常同样 fail-open；
 *    关键词表与 docs/reference/standard/shared/test-design.md「验收措辞规范」节禁用词表同步
 *
 * 配置：SPEC_GATE_ENABLE（默认开，"0"/"false"/"off" 关闭）、SPEC_GATE_BYPASS=1（豁免）、
 * SPEC_GATE_TIMEOUT_MS（默认 60000，①②两项脚本检查共用预算）。
 */
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { logPolicyDecision, type PolicyDecisionInput } from "./lib/policy-decision";
import { missingArchiveUiEvidence } from "./lib/ui-design-gate";

// ---------- 配置常量（环境变量可覆盖） ----------

/** 归档门禁总开关，默认开 */
const ENABLED = !["0", "false", "off"].includes(
	(process.env.SPEC_GATE_ENABLE ?? "").toLowerCase(),
);
/** 两项脚本检查（doc-impact verify / check-standards --change）共用的超时预算（毫秒） */
const TIMEOUT_MS = envNum("SPEC_GATE_TIMEOUT_MS", 60_000);
/** 尾三节标题名（各自独立匹配，不强制顺序） */
const TAIL_SECTIONS = ["测试", "文档", "验证"] as const;
/** tasks.md 里的 doc-impact 声明标记（含冒号，天然排除 -excuse 变体） */
const DOC_IMPACT_MARKER = "<!-- doc-impact:";

/** 本扩展只用到的 ExtensionContext 子集（结构兼容，便于独立类型检查，见 quota-gate GateCtx） */
type GateCtx = {
	signal?: AbortSignal | undefined;
	cwd?: string;
	sessionManager?: { getSessionId?(): string | undefined } | undefined;
};

// ---------- 主入口 ----------

export default function (pi: ExtensionAPI) {
	pi.on("tool_call", async (event, ctx) => {
		// 1. 非归档命令零开销放行（先于一切检查）
		if (event.toolName !== "bash") return;
		const input = (event.input ?? {}) as Record<string, unknown>;
		const command = typeof input.command === "string" ? input.command : "";
		if (!/openspec\s+archive/.test(command)) return;
		if (!ENABLED) return;

		try {
			return await gateArchive(pi, command, ctx);
		} catch (err) {
			// 门禁自身异常绝不阻断归档（fail-open），但留痕不静默
			console.warn(`[spec-gate] 未预期异常，放行：${String(err)}`);
			warn(pi, `⚠️ [spec-gate] 归档门禁内部异常，本次放行（fail-open）：${String(err)}`);
			return;
		}
	});
}

// ---------- 四项归档检查 ----------

/** 对一次 openspec archive 执行四项归档检查（block 级）+ 检查⑤措辞扫描（warn 级不 block）；
 *  失败返回 { block: true, reason }，通过/豁免返回 undefined */
async function gateArchive(
	pi: ExtensionAPI,
	command: string,
	ctx: GateCtx,
): Promise<{ block: true; reason: string } | undefined> {
	// 0. 豁免：--force / SPEC_GATE_BYPASS=1 → 放行 + warning 留痕（不静默）
	const hasForce = /(^|\s)--force(\s|$)/.test(command);
	const hasEnvBypass = process.env.SPEC_GATE_BYPASS === "1";
	if (hasForce || hasEnvBypass) {
		const why = hasForce ? "命令带 --force" : "环境变量 SPEC_GATE_BYPASS=1";
		console.warn(`[spec-gate] ${why}，豁免放行（检查①-④'与⑤未执行）`);
		warn(
			pi,
			`⚠️ [spec-gate] ${why}，openspec archive 已豁免放行；五项归档检查（doc-impact verify / check-standards / tasks.md 尾三节 / scenario-trace 映射对账 / UI 验收证据）与检查⑤（措辞扫描）均未执行，留痕备查。`,
		);
		auditPolicy(ctx, extractChangeName(command) || undefined, {
			policy: "spec-gate",
			action: "bypass",
			reasonCode: "explicit-bypass",
		});
		return;
	}

	// 1. 提取 change 名；提取不到（交互式归档/异常写法）→ fail-open 留痕
	const name = extractChangeName(command);
	if (!name) {
		console.warn(`[spec-gate] 无法从命令提取 change 名，放行：${command}`);
		warn(
			pi,
			`⚠️ [spec-gate] 无法从命令提取 change 名，归档检查无法执行，本次放行：\n${command}`,
		);
		return;
	}
	const changeDir = `openspec/changes/${name}`;

	// 2. 四项检查（各自独立，不强制顺序）；failedChecks 只收检查名（短摘要入 policy.decision.target，不携输出正文）
	const failures: string[] = [];
	const failedChecks: string[] = [];

	const verify = await runScript(pi, ctx, ["scripts/doc-impact.sh", "verify", changeDir]);
	if (!verify.ok) {
		failures.push(`[doc-impact verify ${changeDir}] ${verify.detail}`);
		failedChecks.push("doc-impact");
	}

	const standards = await runScript(pi, ctx, ["scripts/check-standards.sh", "--change", name]);
	if (!standards.ok) {
		failures.push(`[check-standards] ${standards.detail}`);
		failedChecks.push("standards");
	}

	const tasks = await checkTasksMd(pi, ctx, changeDir);
	if (!tasks.ok) {
		failures.push(`[tasks.md 尾三节] ${tasks.detail}`);
		failedChecks.push("tasks");
	}

	const trace = await runScript(pi, ctx, ["scripts/scenario-trace.sh", changeDir]);
	if (!trace.ok) {
		failures.push(`[scenario-trace] ${trace.detail}`);
		failedChecks.push("trace");
	}

	// 2.5 检查④'：UI 验收证据（仅新 schema change；缺项记 ui-design-gate block 事件）
	const uiMissing = checkUiEvidence(changeDir, ctx.cwd ?? process.cwd());
	if (uiMissing.length > 0) {
		failures.push(`[UI 验收证据] ${uiMissing.join("；")}`);
		failedChecks.push("ui-evidence");
		auditPolicy(ctx, name, {
			policy: "ui-design-gate",
			action: "block",
			reasonCode: "ui-verification-missing",
			target: "archive",
		});
	}

	// 3. 检查⑤：验收措辞扫描（warn 级——仅留痕，不进 failures、不改下方裁决；
	//    豁免/无名 fail-open 已在上文 return 短路，天然不跑）
	await warnAcceptanceWording(pi, ctx, changeDir);

	// 3.5 检查⑤'：归档并发（coordinate-concurrent-changes，warn 级绝不 block）——树上
	//     存在归属其他 active change 的未 commit 文件时 steer 提醒（先拆 commit 或与对方
	//     协调收口）；exit 0 干净 / 3 冷启动跳过零输出；脚本异常 fail-open 零干预。
	const conc = await runScript(pi, ctx, ["scripts/concurrency-status.sh", "--check", name]);
	if (conc.code === 2) {
		console.warn(`[spec-gate] ${name} 归档时树上存在归属其他 active change 的未 commit 文件（warn 不 block）`);
		warn(
			pi,
			`⚠️ [spec-gate] 检查⑤'（warn 级，不影响归档裁决）：树上存在归属其他 active change 的未 commit 文件：\n${conc.detail}\n建议：按归属地图先拆主体 commit（bash scripts/concurrency-status.sh <change> 看人读态势），或与对方 change 协调收口时序`,
		);
		auditPolicy(ctx, name, {
			policy: "spec-gate",
			action: "warn",
			reasonCode: "concurrent-dirty-tree",
			target: "archive",
		});
	}

	if (failures.length === 0) {
		console.log(`[spec-gate] ${name} 五项归档检查通过，放行`);
		return; // 正常放行：零 policy.decision（低噪声约束）
	}
	console.warn(`[spec-gate] ${name} 归档检查未通过（${failures.length} 项失败），已阻断`);
	auditPolicy(ctx, name, {
		policy: "spec-gate",
		action: "block",
		reasonCode: "archive-check-failed",
		target: failedChecks.join(","),
	});
	return { block: true, reason: buildBlockReason(name, failures) };
}

/** 从命令行提取 change 名：`openspec archive [-flag...] <name>`（同一行内首个非 flag 词）；非法字符集（防路径逃逸）返回空串 */
function extractChangeName(command: string): string {
	const m = command.match(/openspec\s+archive\s+([^\n]+)/);
	if (!m) return "";
	const name = m[1].trim().split(/\s+/).find((tok) => !tok.startsWith("-")) ?? "";
	return /^[A-Za-z0-9][A-Za-z0-9._-]*$/.test(name) ? name : "";
}

/** 跑一个门禁脚本：code 为真实退出码（异常 -1）；ok = code===0；detail 含退出码 + 输出尾部 20 行。
 *  检查⑤'复用 code 字段区分 exit 2（warn）/ 3（冷启动跳过） */
async function runScript(
	pi: ExtensionAPI,
	ctx: GateCtx,
	args: string[],
): Promise<{ ok: boolean; code: number; detail: string }> {
	try {
		const r = await pi.exec("bash", args, { signal: ctx.signal, timeout: TIMEOUT_MS });
		const out = `${r.stdout}${r.stderr}`;
		return {
			ok: r.code === 0,
			code: r.code,
			detail: `exit ${r.code}${out.trim() ? `\n${tail(out, 20)}` : "（无输出）"}`,
		};
	} catch (err) {
		// 超时/无法启动等异常按该项检查失败处理（扩展自身 bug 才走上层 fail-open）；
		// 检查⑤'消费方凭 code=-1 fail-open 零干预
		return { ok: false, code: -1, detail: `执行异常（超时或无法启动）：${String(err)}` };
	}
}

/** 检查 ③：tasks.md 尾三节（测试/文档/验证各自独立命中一次，顺序不限）+ `<!-- doc-impact:` 声明标记 */
async function checkTasksMd(
	pi: ExtensionAPI,
	ctx: GateCtx,
	changeDir: string,
): Promise<{ ok: boolean; detail: string }> {
	let tasks: string;
	try {
		const r = await pi.exec("cat", [`${changeDir}/tasks.md`], {
			signal: ctx.signal,
			timeout: 5_000,
		});
		if (r.code !== 0) {
			return {
				ok: false,
				detail: `读取 ${changeDir}/tasks.md 失败（exit ${r.code}）：文件不存在或不可读`,
			};
		}
		tasks = r.stdout;
	} catch (err) {
		return { ok: false, detail: `读取 ${changeDir}/tasks.md 异常：${String(err)}` };
	}

	const missing: string[] = [];
	for (const sec of TAIL_SECTIONS) {
		// 行首锚定 + 严格两井号：命中 `## N. 测试`，不命中 `### N.x` 子节与正文引用
		if (!new RegExp(`^##\\s+\\d+\\.\\s*${sec}`, "m").test(tasks)) {
			missing.push(`「## N. ${sec}」`);
		}
	}
	if (!tasks.includes(DOC_IMPACT_MARKER)) missing.push("`<!-- doc-impact:` 声明标记");
	if (missing.length === 0) return { ok: true, detail: "" };
	return {
		ok: false,
		detail: `缺失：${missing.join("、")}。要求尾三节标题各自独立存在（顺序不限），且文内含 <!-- doc-impact: domain ... --> 声明标记`,
	};
}

// ---------- 检查④'：UI 验收证据（快照读取 + lib 纯函数判定） ----------

/** 读 change 快照并调用 lib/ui-design-gate 的归档证据检查；自身异常返回空数组（fail-open，
 *  legacy schema 恒为空数组——旧 change 不硬阻断） */
function checkUiEvidence(changeDir: string, cwd: string): string[] {
	try {
		const absDir = join(cwd, changeDir);
		return missingArchiveUiEvidence({
			schema: readSchemaLine(safeRead(join(absDir, ".openspec.yaml"))),
			proposalText: safeRead(join(absDir, "proposal.md")),
			uiDesignText: safeRead(join(absDir, "ui-design.md")),
			tasksMd: safeRead(join(absDir, "tasks.md")),
			changeDirFiles: listRegularFiles(absDir),
		});
	} catch {
		return [];
	}
}

/** 从 .openspec.yaml 文本提取 `schema:` 行；提取不到 → null（legacy 兼容） */
function readSchemaLine(text: string | null): string | null {
	if (text === null) return null;
	const m = /^schema:\s*(\S+)\s*$/m.exec(text);
	return m ? m[1] : null;
}

/** 读文本；读不到 → null */
function safeRead(path: string): string | null {
	try {
		return readFileSync(path, "utf8");
	} catch {
		return null;
	}
}

/** change 目录递归普通文件清单（相对 change 根，正斜杠；symlink/目录不入列） */
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
			const rel = prefix ? `${prefix}/${e.name}` : e.name;
			if (e.isDirectory()) walk(join(dir, e.name), rel);
			else if (e.isFile()) out.push(rel);
		}
	};
	walk(changeDir, "");
	return out;
}

// ---------- 检查⑤：验收措辞扫描（warn 级，绝不 block） ----------

import {
	COMPLEXITY_KEYWORDS,
	hasTestCaseDoc,
	parseComplexityDeclaration,
	scanComplexityKeywords,
} from "./lib/test-case-gate";
/** 兼容 re-export：冒烟脚本与「改表必同步」义务沿用 spec-gate 入口（常量家在 lib/test-case-gate.ts） */
export { COMPLEXITY_KEYWORDS };

/** 检查⑤：读 tasks.md + proposal.md + 列 change 目录，违例逐条走 spec-gate-warning 留痕（display: true）；
 *  自身异常 fail-open（console.warn + 留痕），绝不影响检查①-④ 的 block 裁决 */
async function warnAcceptanceWording(
	pi: ExtensionAPI,
	ctx: GateCtx,
	changeDir: string,
): Promise<void> {
	try {
		const r = await pi.exec("cat", [`${changeDir}/tasks.md`], {
			signal: ctx.signal,
			timeout: 5_000,
		});
		if (r.code !== 0) return; // tasks.md 读不到：检查③已按缺文件阻断并给出指引，⑤静默跳过
		let files: string[];
		try {
			const l = await pi.exec("ls", [changeDir], { signal: ctx.signal, timeout: 5_000 });
			files =
				l.code === 0
					? l.stdout.split("\n").map((s) => s.trim()).filter(Boolean)
					: ["test-cases.md"]; // 目录列表拿不到 → 伪造成「已检出」抑制⑤a（证明不了缺失就不提醒），⑤b 不依赖文件列表照常跑
		} catch {
			files = ["test-cases.md"];
		}
		let proposalText: string | null = null;
		try {
			const p = await pi.exec("cat", [`${changeDir}/proposal.md`], {
				signal: ctx.signal,
				timeout: 5_000,
			});
			proposalText = p.code === 0 ? p.stdout : null; // 读不到 → 视为未声明，⑤a 走兜底词表
		} catch {
			proposalText = null;
		}
		for (const w of scanAcceptanceWording(r.stdout, files, proposalText)) {
			warn(pi, `⚠️ [spec-gate] 检查⑤（warn 级，不影响归档裁决）：${w}`);
			auditPolicy(ctx, changeDir.split("/").pop() ?? null, {
				policy: "spec-gate",
				action: "warn",
				reasonCode: "acceptance-wording",
			});
		}
	} catch (err) {
		console.warn(`[spec-gate] 检查⑤（措辞扫描）异常，跳过：${String(err)}`);
		warn(
			pi,
			`⚠️ [spec-gate] 检查⑤（措辞扫描）执行异常，已跳过（fail-open，不影响归档裁决）：${String(err)}`,
		);
	}
}

/**
 * 检查⑤纯函数：扫描 tasks.md 任务行（`- [ ]`/`- [x]` 起到行尾）的验收措辞违例，
 * 返回警告文案数组（空数组=无违例）。
 * ⑤a（声明优先，test-case-entry-gate）：change 目录无白盒用例文档时——
 *     声明 complex → 一条强违例；声明 simple + 词表命中 → 一条反向质询；
 *     未声明 + 词表命中 → 每个命中关键词一条（按常量表序去重）；
 *     声明 simple/未声明且未命中 / 已有文档 → 无违例。
 *     proposalText 传 null/undefined = 视为未声明（兼容旧两参调用）。
 * ⑤b：同一任务行含「纯函数」且含「SQLite」→ 每行一条。
 * 纯函数：不 import fs、不触网、不抛异常（空串/超长行/畸形输入返回合理结果）。
 */
export function scanAcceptanceWording(
	tasksMd: string,
	changeDirFiles: string[],
	proposalText?: string | null,
): string[] {
	const warnings: string[] = [];
	if (typeof tasksMd !== "string" || tasksMd.length === 0) return warnings;
	for (const raw of tasksMd.split("\n")) {
		// 任务行锚定：行首（允许缩进的子任务）`- [ ]` / `- [x]` 起，取到行尾
		if (!/^\s*-\s\[[ x]\]/.test(raw)) continue;
		const text = raw.replace(/^\s*-\s\[[ x]\]\s*/, "");
		if (text.includes("纯函数") && text.includes("SQLite")) {
			warnings.push(
				`⑤b 分层错配（任务行「${clip(text)}」）：纯函数用例按 testing.md 分层应落 *_unit_test.go 无 DB；若任务确需 DB 请修正任务描述`,
			);
		}
	}
	if (!hasTestCaseDoc(changeDirFiles)) {
		const declaration = parseComplexityDeclaration(proposalText);
		if (declaration === "complex") {
			warnings.push(
				`⑤a 声明复杂档但缺白盒用例文档：proposal 声明 <!-- complexity: complex -->，case-first-testing 要求产出白盒用例文档（分支表/边界值清单）；请补 test-cases.md 或修正声明`,
			);
		} else {
			const hitKeywords = scanComplexityKeywords(tasksMd);
			if (declaration === "simple" && hitKeywords.length > 0) {
				warnings.push(
					`⑤a 声明与任务措辞矛盾（声明 simple，任务行命中「${hitKeywords.join("、")}」）：改声明为 complex 并补白盒用例文档，或调整任务措辞并确非复杂档`,
				);
			} else {
				for (const kw of hitKeywords) {
					warnings.push(
						`⑤a 白盒用例缺失（关键词「${kw}」）：case-first-testing 要求复杂档产出白盒用例文档（分支表/边界值清单），当前 change 目录未检出；确非复杂档可忽略`,
					);
				}
			}
		}
	}
	return warnings;
}

/** 截断过长任务行文本，避免 warning 文案膨胀 */
function clip(s: string, n = 60): string {
	return s.length <= n ? s : `${s.slice(0, n)}…`;
}

// ---------- 阻断 reason（中文，风格对齐 quota-gate buildBlockReason） ----------

function buildBlockReason(name: string, failures: string[]): string {
	return [
		`openspec archive 归档门禁未通过（change: ${name}），本次命令已阻断。`,
		"失败项（四项各自独立判定）：",
		...failures.map((f, i) => `${i + 1}. ${f}`),
		"",
		"修复指引：",
		"- doc-impact verify：按输出在 tasks.md 补 <!-- doc-impact: domain=理由; ... --> 声明；确属误报的域可加 <!-- doc-impact-excuse: domain=理由 --> 豁免",
		"- check-standards：按输出逐项修复（docs/reference/standard 结构约束）",
		"- tasks.md 尾三节：补齐「## N. 测试」「## N. 文档」「## N. 验证」三节 + doc-impact 声明标记后重试",
		"- scenario-trace：在 tasks.md「N. 验证」节补 | Scenario | 测试文件 | 映射表（每行一个 delta Scenario 标题 + 仓库根相对测试路径或「人工…」说明）；无 delta Scenario 的 change 直接通过",
		"- 归档撞见非本 change 的红测试：用 `bash scripts/test-patrol.sh --register <test_id> --context <本change名>` 登记台账（或用 --report 确认已在账），在验证节记一句「已记账」后可继续——本 change 影响包内的红不适用该通道，必须修复（test-debt-patrol）",
		"- UI 验收证据：major 需 ui-approval: approved + 原型存在 + tasks 验证节含 opencli 与 1440×900/1920×1080 证据 + ui-design.md 差异说明；minor 需验收映射；none 需 N/A 一致性",
		"- 确认要跳过检查：命令加 --force 或设 SPEC_GATE_BYPASS=1（豁免会记 warning 留痕）",
	].join("\n");
}

// ---------- 小工具 ----------

/** policy.decision 旁路记账（change: harden-harness-policy-and-spill D2/D3）：
 *  无 cwd/sessionId 上下文（独立运行/桩缺省）不记；记账异常忽略，绝不影响裁决 */
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
		console.warn(`[spec-gate] policy.decision 记账异常（旁路忽略）：${String(err)}`);
	}
}

/** 落 custom_message 留痕（steer：不打断当前回合，下次 LLM 调用前送达；复用 quality-gate 的 customType 模式） */
function warn(pi: ExtensionAPI, content: string): void {
	try {
		pi.sendMessage(
			{ customType: "spec-gate-warning", content, display: true },
			{ deliverAs: "steer" },
		);
	} catch (err) {
		console.warn(`[spec-gate] custom_message 落盘失败：${String(err)}`);
	}
}

/** 取字符串尾部 n 行，避免 payload 过大（同 quality-gate） */
function tail(s: string, n: number): string {
	const lines = s.split("\n");
	return lines.slice(Math.max(0, lines.length - n)).join("\n");
}

/** 环境变量数值解析：非法值回退默认（同 quota-gate） */
function envNum(name: string, def: number): number {
	const v = Number(process.env[name]);
	return Number.isFinite(v) && v > 0 ? v : def;
}
