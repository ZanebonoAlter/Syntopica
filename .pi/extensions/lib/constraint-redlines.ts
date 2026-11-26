/**
 * constraint-redlines — 约束节红线层提取（add-dispatch-constraint-digest D2）
 *
 * 单一真相源：extension（`.pi/extensions/constraint-injection.ts`）与取数脚本
 * （`scripts/harness/constraint-digest.sh`，经 esbuild bundle 后 node 直调）**双引本模块**。
 * 禁止任何一方另写第二份正则——`docs/reference/standard/shared/doc-authoring.md`
 * 「约束节红线句格式」明令「格式规范与提取器语义同步演进，改一处必改另一处」；
 * 脚本内重写第二份正则 = 永久漂移承诺（design R1）。
 *
 * 本模块 MUST 保持**零依赖纯函数**（无 fs / 无时间 / 无随机）：同一节文本 → 同一输出
 * 字节序列，脚本与扩展两侧逐字节可复现（smoke 对 10 篇 flow 文档逐篇对账）。
 */

/** 红线层提取结果：lines 为「原文列表标记 + 加粗块」逐行序列，bytes 为 \n 拼接字节数 */
export interface RedlineLayer {
	lines: string[]; // 形如 `3. **红线句**`（保留原文顺序与编号）
	bytes: number;
}

/**
 * 提取约束节的红线层：顶层列表项（行首无缩进的 `N. ` / `- `）行取首个 `**...**`
 * 加粗块为一条红线句（保留原文顺序与编号）；无加粗块的列表项不取首行文本凑数
 * （细节句冒充红线比回退全节更糟）；引用块 / 自由段落 / 嵌套列表项不属红线层。
 * 0 条提取返回 null；bytes 低于节级最小字节下限时由调用方回退全节（同族 fail-safe）。
 * 纯函数：同一节文本 → 同一输出字节序列（smoke 直跑）。
 */
export function extractRedlines(sectionText: string): RedlineLayer | null {
	const lines: string[] = [];
	for (const line of sectionText.split(/\r?\n/)) {
		const m = line.match(/^(\d+\.|-)\s+(.*)$/);
		if (!m) continue; // 非顶层列表项（嵌套项/引用块/自由段落/标题）
		const bold = m[2].match(/\*\*(.+?)\*\*/);
		if (!bold) continue; // 无加粗块：不凑数，该条不进红线层
		lines.push(`${m[1]} **${bold[1]}**`);
	}
	if (!lines.length) return null;
	return { lines, bytes: Buffer.byteLength(lines.join("\n"), "utf8") };
}
