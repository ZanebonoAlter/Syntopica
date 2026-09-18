/**
 * child-session — 「父子关系可证」共享判定（harden-subagent-constraint-channel T3/D4）
 *
 * 单一真相源：constraint-injection（子线程继承档位绑定）与 quality-gate（子线程 turn_end
 * 降载 bypass）对子线程的判定 MUST 同源同证据——两扩展对同一会话得出不同父子结论
 * （降载而未继承 / 继承而未降载）即 harness 行为分裂。判定实现原内联于
 * constraint-injection.ts（fork 继承链，2026-09-17 实测锚定），本模块原样抽出供双方复用。
 *
 * 判定证据（按可信度排序，两条都拿不到 = 非子线程，失败方向安全：主会话路径不受影响）：
 *   ① session header 的 `parentSession`（父会话**文件绝对路径**）：fork 子会话必带
 *      （2026-09-17 实测）；pi-web Agent 工具派发的子会话 header 同样带（2026-09-18
 *      实测样本 01a0b314 转录头实证）；
 *   ② `getSessionFile()` 路径兜底：fork 子会话文件位于
 *      `<父会话目录>/forks/<ts>_<子会话id>.jsonl`，而父会话目录名就是 `<ts>_<父会话id>`，
 *      从「forks 的上一级目录名」解出父 id。header 未落盘 / 契约变化时仍可解；
 *      非 fork 路径（文件不在 forks/ 下）→ null（不可证）。
 */

/** extension ctx 的最小判定面（constraint-injection 的 ExtCtx / pi ExtensionContext 的
 *  公共子集）：纯函数只读 sessionManager 三方法，smoke 以 stub 直跑判定矩阵 */
export interface ChildSessionCtx {
	sessionManager?: {
		getSessionId?: () => string | undefined;
		/** session header：fork/子线程凭证（parentSession = 父会话文件绝对路径） */
		getHeader?: () => { parentSession?: string } | null;
		/** 会话文件绝对路径——header 未落盘时经 `forks/` 路径兜底解父 id */
		getSessionFile?: () => string | undefined | null;
	};
}

/** `<ts>_<sessionId>.jsonl` → sessionId。会话文件（父会话文件路径、fork 子会话文件）都用
 *  这个形状：时间戳与 id 之间只有一个下划线，id 自身含连字符。 */
export function sessionIdFromSessionFile(file: string | undefined | null): string | null {
	if (!file) return null;
	const stem = String(file).split(/[\\/]/).pop()?.replace(/\.jsonl$/, "") ?? "";
	const sep = stem.indexOf("_");
	return sep >= 0 ? stem.slice(sep + 1) : null;
}

/** fork 子会话文件路径 → 父会话 id：子会话文件位于 `<父会话目录>/forks/<ts>_<子会话id>.jsonl`，
 *  而父会话目录名就是 `<ts>_<父会话id>` → 从「forks 的上一级目录名」解出父 id。
 *  非 fork 会话（文件不在 forks/ 下）→ null（不可证，不继承/不降载）。 */
export function parentSessionIdFromForkFile(file: string | undefined | null): string | null {
	if (!file) return null;
	const segs = String(file).split(/[\\/]/);
	const forksIdx = segs.lastIndexOf("forks");
	if (forksIdx < 1) return null;
	const parentDir = segs[forksIdx - 1] ?? "";
	const sep = parentDir.indexOf("_");
	return sep >= 0 ? parentDir.slice(sep + 1) : null;
}

/** 父会话 id（「父子关系可证」的唯一依据）。两条来源，按可信度排序：
 *  ① session header 的 `parentSession`（父会话**文件绝对路径**，fork 子会话必带，实测 2026-09-17）；
 *  ② `getSessionFile()` 的路径兜底——header 未落盘/契约变化时，fork 子会话仍可从
 *     `<父会话目录>/forks/` 反解父 id。
 *  两条都拿不到 → null（不可证 → 不继承/不降载，保持失败方向安全）。 */
export function parentSessionId(ctx: ChildSessionCtx | undefined): string | null {
	const fromHeader = sessionIdFromSessionFile(
		ctx?.sessionManager?.getHeader?.()?.parentSession,
	);
	if (fromHeader) return fromHeader;
	return parentSessionIdFromForkFile(ctx?.sessionManager?.getSessionFile?.());
}

/** 本会话是否子线程（父子关系可证）。quality-gate turn_end 降载短路（D4）与
 *  constraint-injection 父子继承共用同一判定面。 */
export function isChildSession(ctx: ChildSessionCtx | undefined): boolean {
	return parentSessionId(ctx) !== null;
}
