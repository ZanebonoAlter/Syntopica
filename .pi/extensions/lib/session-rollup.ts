/**
 * session-rollup — 单会话效能汇总聚合器 @Syntopica
 *
 * change: harness-effectiveness-metrics（design D1/D2，spec「session 效能汇总记账」）。
 *
 * 职责：从 pi 的 session jsonl（~/.pi/agent/sessions/<dir>/<ISO>_<UUID>.jsonl）提取
 * 单会话效能汇总，供 harness-telemetry 写 `session.rollup` 快照事件：
 *   - turns        = user 消息数（真实用户轮次）
 *   - steps        = assistant 消息数（agent 步数；usage 100% 挂在 assistant 上）
 *   - toolCalls    = toolResult 消息数（工具调用次数）
 *   - tokens       = input/output/cacheRead/cacheWrite/total（total 以 usage.totalTokens
 *                    为准——供应商口径，不自行相加，design 白盒 B7）
 *   - cost         = cost.total（元，可空）
 *   - durationSec  = 首末 message 的 ts 差（跨度非净工时，报告口径已声明）
 *   - model        = 最后一条 model_change 的 modelId（可空）
 *   - models       = 逐模型聚合 map（task-cost-metrics D1：{[modelId]: {tokens 五值, cost}}，
 *                    按 assistant 消息自身携带的 message.model 归属，缺字段计 unknown 桶）
 *   - skippedLines = 不可解析行计数（fail-safe：跳过不崩，口径排查用）
 *
 * 增量语义：调用方（telemetry）持 RollupAccumulator {offset, remainder, summary}，
 * 每 turn 只读新增字节；offset 精确推进到最后一个完整 \n 之后，残行区域下次整行重读
 * （重读不拼接：多字节截断自愈，且不会把残行内容计两次——重复读同一文件零变化）。
 *
 * 纯函数直测：.pi/extensions/tests/session-rollup.smoke.cjs（对齐 parseSubagentSummary 先例）；
 * task-cost-metrics 扩展：models 逐模型聚合、readParentSessionId/findPrevSessionFile
 * （子会话父链提取 + prev 会话定位，D1/D2/D3）。
 */
import * as fs from "node:fs";
import * as path from "node:path";

/** 逐模型聚合桶（task-cost-metrics：按消息级 model 字段归属） */
export interface ModelUsageEntry {
	tokens: {
		input: number;
		output: number;
		cacheRead: number;
		cacheWrite: number;
		total: number;
	};
	cost: number | null;
}

/** 单会话效能汇总（payload 契约，spec「session 效能汇总记账」） */
export interface SessionUsageSummary {
	turns: number;
	steps: number;
	toolCalls: number;
	tokens: {
		input: number;
		output: number;
		cacheRead: number;
		cacheWrite: number;
		total: number;
	};
	cost: number | null;
	durationSec: number;
	model: string | null;
	models: Record<string, ModelUsageEntry>;
	skippedLines: number;
}

/** 增量聚合状态：offset = 已消费字节（含 remainder），remainder = 末尾残行 */
export interface RollupAccumulator {
	offset: number;
	remainder: string;
	summary: SessionUsageSummary;
	firstTs: string | null;
	lastTs: string | null;
}

export function emptySummary(): SessionUsageSummary {
	return {
		turns: 0,
		steps: 0,
		toolCalls: 0,
		tokens: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 },
		cost: null,
		durationSec: 0,
		model: null,
		models: {},
		skippedLines: 0,
	};
}

export function newAccumulator(): RollupAccumulator {
	return { offset: 0, remainder: "", summary: emptySummary(), firstTs: null, lastTs: null };
}

/** 把 chunk 按 \n 切成完整行；末尾残行返回 remainder 由调用方持有（白盒 B1 半行容忍） */
export function splitCompleteLines(chunk: string, prevRemainder = ""): {
	lines: string[];
	remainder: string;
} {
	const joined = prevRemainder + chunk;
	const lastNl = joined.lastIndexOf("\n");
	if (lastNl === -1) return { lines: [], remainder: joined };
	return { lines: joined.slice(0, lastNl).split("\n"), remainder: joined.slice(lastNl + 1) };
}

function num(v: unknown): number {
	return typeof v === "number" && Number.isFinite(v) ? v : 0;
}

/** 消费一批完整行，推进 summary（单调不减口径：tokens 累计、turns/steps/toolCalls 计数） */
export function consumeLines(acc: RollupAccumulator, lines: string[]): RollupAccumulator {
	const s = acc.summary;
	for (const line of lines) {
		if (line.trim() === "") continue; // 空行/纯空白不算坏行也不计数（白盒 B1 边界）
		let d: {
			type?: string;
			timestamp?: string;
			modelId?: string;
			message?: {
				role?: string;
				model?: string;
				usage?: {
					input?: number;
					output?: number;
					cacheRead?: number;
					cacheWrite?: number;
					totalTokens?: number;
					cost?: { total?: number };
				};
			};
		};
		try {
			d = JSON.parse(line);
		} catch {
			s.skippedLines += 1; // 坏 JSON 行：跳过计数，不崩（fail-safe）
			continue;
		}
		const ts = typeof d.timestamp === "string" ? d.timestamp : null;
		if (ts) {
			if (!acc.firstTs) acc.firstTs = ts;
			acc.lastTs = ts;
		}
		if (d.type === "model_change" && typeof d.modelId === "string") {
			s.model = d.modelId; // 取最后一条（白盒 B10）
			continue;
		}
		if (d.type !== "message" || !d.message) continue; // session/custom 等行忽略（白盒 B2）
		const role = d.message.role;
		const u = d.message.usage;
		if (role === "user") s.turns += 1;
		else if (role === "assistant") {
			s.steps += 1;
			if (u) {
				s.tokens.input += num(u.input);
				s.tokens.output += num(u.output);
				s.tokens.cacheRead += num(u.cacheRead);
				s.tokens.cacheWrite += num(u.cacheWrite);
				s.tokens.total += num(u.totalTokens); // 缺 usage 的 assistant：steps+1、tokens+0（白盒 B5）
				const c = u.cost?.total;
				const cNum = typeof c === "number" && Number.isFinite(c) ? c : null;
				if (cNum !== null) {
					s.cost = (s.cost ?? 0) + cNum; // cost 累计（各步费用之和）
				}
				// 逐模型聚合（task-cost-metrics D1）：按消息级 model 字段归属，缺字段计 unknown 桶；
				// cost null 容忍同顶层口径（桶保持 null，不伪造成 0）
				const mkey =
					typeof d.message.model === "string" && d.message.model
						? d.message.model
						: "unknown";
				let bucket = s.models[mkey];
				if (!bucket) {
					bucket = {
						tokens: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 },
						cost: null,
					};
					s.models[mkey] = bucket;
				}
				bucket.tokens.input += num(u.input);
				bucket.tokens.output += num(u.output);
				bucket.tokens.cacheRead += num(u.cacheRead);
				bucket.tokens.cacheWrite += num(u.cacheWrite);
				bucket.tokens.total += num(u.totalTokens);
				if (cNum !== null) bucket.cost = (bucket.cost ?? 0) + cNum;
			}
		} else if (role === "toolResult") s.toolCalls += 1;
	}
	if (acc.firstTs && acc.lastTs) {
		const dur =
			(Date.parse(acc.lastTs) - Date.parse(acc.firstTs)) / 1000;
		s.durationSec = Number.isFinite(dur) && dur > 0 ? Math.round(dur) : 0;
	}
	return acc;
}

/** 增量消费：读 [offset, EOF) 的完整行推进状态；返回同一 acc（就地推进）。
 *  文件不存在/读失败 → 返回 null（调用方零写入 fail-open，不抛出）。 */
export function accumulateFromFile(
	acc: RollupAccumulator,
	file: string,
): RollupAccumulator | null {
	let fd: number;
	try {
		fd = fs.openSync(file, "r");
	} catch {
		return null;
	}
	try {
		const size = fs.fstatSync(fd).size;
		if (size <= acc.offset) return acc; // 无新增（幂等：重复调零变化）
		const length = size - acc.offset;
		const buf = Buffer.alloc(length);
		let read = 0;
		while (read < length) {
			const n = fs.readSync(fd, buf, read, length - read, acc.offset + read);
			if (n <= 0) break;
			read += n;
		}
		const chunk = buf.toString("utf8");
		// 重读不拼接：offset 停在最后一个 \\n 之后（字节级精确），残行区域下次整行重读——
		// 多字节截断自愈，且不会把残行内容拼两次（旧实现 bug：offset 回退 + remainder 拼接叠加）
		const lastNl = chunk.lastIndexOf("\n");
		if (lastNl === -1) {
			acc.remainder += chunk; // 无完整行：全部是残行，offset 不动
			return acc;
		}
		consumeLines(acc, chunk.slice(0, lastNl).split("\n"));
		acc.offset += Buffer.byteLength(chunk.slice(0, lastNl + 1), "utf8");
		acc.remainder = chunk.slice(lastNl + 1);
		return acc;
	} catch (e) {
		console.error(`[session-rollup] read failed: ${(e as Error).message}`);
		return acc; // 已推进的部分保留，异常旁路（fail-safe）
	} finally {
		fs.closeSync(fd);
	}
}

/** 全量解析一个 jsonl 文本（prev 回填路径用；坏行容忍同上） */
export function parseSessionText(text: string): SessionUsageSummary {
	const acc = newAccumulator();
	const { lines } = splitCompleteLines(text, "");
	consumeLines(acc, lines);
	return acc.summary;
}

/** 节流判定（design D1：turn%5==0 或 token 增量>20%；首次必写 bootstrap）。
 *  严格大于 20%（=20.0% 整不写，白盒边界表）。 */
export function shouldWriteSnapshot(
	turnCount: number,
	lastTotalTokens: number | null,
	currentTotalTokens: number,
): boolean {
	if (lastTotalTokens === null) return true;
	if (turnCount > 0 && turnCount % 5 === 0) return true;
	return currentTotalTokens > lastTotalTokens * 1.2;
}

/** prev 回填判定：文件路径在 && 会话非空（turns>0）&& 账本尚无 final 终值 */
export function shouldBackfillPrev(
	prevFile: string | null | undefined,
	summary: SessionUsageSummary,
	hasFinalInLedger: boolean,
): boolean {
	if (!prevFile) return false;
	if (hasFinalInLedger) return false;
	return summary.turns > 0;
}

/** 从 session jsonl 文件名提取会话 UUID（形如 <ISO>_<UUID>.jsonl；UUID 含 '-'，ISO 段
 *  也含 '-'，故取最后一个 '_' 之后）。提不出返回 null。纯函数，smoke 直测。 */
export function sessionFileIdOf(file: string): string | null {
	const base = path.basename(file).replace(/\.jsonl$/, "");
	const idx = base.lastIndexOf("_");
	if (idx <= 0) return null;
	const id = base.slice(idx + 1);
	return /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(id)
		? id
		: null;
}

/** 读 session jsonl 首行 session 头的 parentSession（父会话文件完整路径）→ 提取父会话
 *  UUID（task-cost-metrics D2：子会话→父→任务归因链）。主会话（parentSession null/缺）
 *  或任何读/解析失败 → null（fail-open，调用方省略键）。只读首 8KB 内的首行。 */
export function readParentSessionId(file: string): string | null {
	let fd: number;
	try {
		fd = fs.openSync(file, "r");
	} catch {
		return null;
	}
	try {
		const buf = Buffer.alloc(8192);
		const n = fs.readSync(fd, buf, 0, buf.length, 0);
		const seg = buf.subarray(0, n);
		const nl = seg.indexOf(10); // \n 字节
		const head = (nl === -1 ? seg : seg.subarray(0, nl)).toString("utf8");
		const d = JSON.parse(head) as { parentSession?: unknown };
		if (typeof d.parentSession !== "string" || !d.parentSession) return null;
		return sessionFileIdOf(d.parentSession);
	} catch {
		return null;
	} finally {
		fs.closeSync(fd);
	}
}

/** 从同目录候选集中定位 prev 会话文件（task-cost-metrics D3）：排除当前会话与提不出
 *  UUID 的条目，取 mtime 最新者；mtime 并列时取文件名字典序大者（ISO 前缀新者胜，
 *  保证确定性）。空目录/仅当前会话 → null。纯函数，smoke 直测。 */
export function findPrevSessionFile(
	entries: ReadonlyArray<{ file: string; mtimeMs: number }>,
	currentId: string,
): string | null {
	let best: { file: string; mtimeMs: number } | null = null;
	for (const e of entries) {
		const id = sessionFileIdOf(e.file);
		if (!id || id === currentId) continue;
		if (
			!best ||
			e.mtimeMs > best.mtimeMs ||
			(e.mtimeMs === best.mtimeMs && e.file > best.file)
		) {
			best = e;
		}
	}
	return best?.file ?? null;
}
