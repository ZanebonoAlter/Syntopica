/**
 * archive-accounting — 归档成功记账纯函数 @Syntopica
 *
 * change: task-cost-metrics（design D4）。
 *
 * 职责：判定 bash 命令是否 `openspec archive` 调用、从命令行提取 change 名、
 * 以及 tool_result 配对后的记账判定。正则语义与 spec-gate 的 extractChangeName
 * 完全一致（同一事实源口径，防两处漂移）：
 *   - `openspec archive` 后同一行内首个非 flag 词为 change 名
 *   - 合法字符集 [A-Za-z0-9][A-Za-z0-9._-]*（防路径逃逸）
 *
 * 纯函数直测：.pi/extensions/tests/telemetry-archive.smoke.cjs。
 */

/** 判定 bash 命令是否 openspec archive 调用 */
export function isArchiveCommand(command: string): boolean {
	return /openspec\s+archive/.test(command);
}

/** 从归档命令行提取 change 名：`openspec archive [-flag...] <name>`（同一行内首个非
 *  flag 词）；非法字符集（防路径逃逸）返回空串——语义与 spec-gate extractChangeName 一致 */
export function extractArchiveChangeName(command: string): string {
	const m = command.match(/openspec\s+archive\s+([^\n]+)/);
	if (!m) return "";
	const name = m[1].trim().split(/\s+/).find((tok) => !tok.startsWith("-")) ?? "";
	return /^[A-Za-z0-9][A-Za-z0-9._-]*$/.test(name) ? name : "";
}

/** tool_result 记账判定：归档命令 + 非错误 + 能提取合法名 → 返回 change 名；
 *  否则 null（零记录 fail-open：被 block / CLI 失败 / 交互式无名字）。纯函数，smoke 直测。 */
export function decideArchiveEvent(
	command: string | undefined,
	isError: boolean,
): string | null {
	if (!command || isError) return null;
	if (!isArchiveCommand(command)) return null;
	const name = extractArchiveChangeName(command);
	return name || null;
}
