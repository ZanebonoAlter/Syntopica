/**
 * tool-output-spill — 超大工具结果强制 spill 保险丝 @Syntopica
 *
 * change: tool-output-spill（harness-survey C 级判定 C12 落地）
 * 设计文档：openspec/changes/tool-output-spill/design.md（D1-D8）
 *
 * 职责：tool_result middleware（工具名无关，内置 + MCP 工具同链覆盖）——
 *   结果合并文本超阈值时无条件 spill：
 *     完整内容   → .pi/harness/spill/<sessionId>/<ts>-<tool>-<callHash>.txt
 *                 （callHash = SHA-256(toolCallId) 前 16 hex：同毫秒同名并行工具不同路径；
 *                  原始 toolCallId 不进文件名；排他创建意外同名不覆盖，harden-harness-policy-and-spill D4）
 *                 （POSIX：会话目录 0700、新文件 0600；权限收紧失败清理新文件后按写盘失败降级；
 *                  Windows best-effort 仅创建时 mode，D5）
 *     模型上下文 → 头 2048B + 取回路径标记 + 尾 512B 预览（UTF-8 安全边界）
 *     记账       → events.db spill.write {tool, bytes, path, ok}
 *
 * 边界（design）：
 *   - 入口替换（结果尚未进任何 LLM 请求）→ 零缓存代价（D0 前提）
 *   - 只处理 text 块；图片等非文本块原样保留（D3）
 *   - read 直通 spill 路径（取回不再 spill，防死循环 D7）
 *   - 写盘失败安全降级：原结果返回 + ok:false 记账，不阻塞工具链（D6）
 *   - 阈值：.pi/harness.json spillThresholdBytes（缺省 32768；显式非正数=禁用直通，D2）
 */
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { createHash } from "node:crypto";
import { chmodSync, mkdirSync, readFileSync, readdirSync, rmdirSync, statSync, unlinkSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { logEvent } from "./lib/harness-log";

const SPILL_DIR = ".pi/harness/spill";
const RETENTION_MS = 30 * 86400e3;
const DEFAULT_THRESHOLD = 32768;
const HEAD_BYTES = 2048;
const TAIL_BYTES = 512;

/* ---------- 配置（D2）：按 cwd 缓存单次；文件缺省/无字段=默认，显式非正数=禁用 ---------- */
const thresholdCache = new Map<string, number | null>(); // null=禁用
function loadThreshold(cwd: string): number | null {
	const hit = thresholdCache.get(cwd);
	if (hit !== undefined || thresholdCache.has(cwd)) return hit ?? null;
	let resolved: number | null = DEFAULT_THRESHOLD;
	try {
		const v = JSON.parse(readFileSync(join(cwd, ".pi/harness.json"), "utf8"))
			?.spillThresholdBytes as unknown;
		if (typeof v === "number" && !Number.isNaN(v)) resolved = v <= 0 ? null : Math.floor(v);
	} catch {
		/* 文件不存在/解析失败 → 默认 */
	}
	thresholdCache.set(cwd, resolved);
	return resolved;
}

/* ---------- UTF-8 安全切片（D3：中文内容不产生乱码字节） ---------- */
/** 位置落在多字节字符中间时回退到该字符起始边界（start/end 通用） */
function utf8Boundary(buf: Buffer, i: number): number {
	while (i > 0 && i < buf.length && (buf[i] & 0xc0) === 0x80) i--;
	return i;
}

function sanitizeTool(name: string): string {
	return (name.replace(/[^a-zA-Z0-9._-]/g, "_").slice(0, 40) || "tool");
}

/** 相对路径统一正斜杠（read 工具与跨平台显示友好） */
function toPosix(p: string): string {
	return p.split("\\").join("/");
}

/* ---------- 30 天清扫（D5）：扩展加载时执行，失败不影响主流程 ---------- */
function sweepSpillFiles(cwd: string): void {
	const root = join(cwd, SPILL_DIR);
	let sessions: string[];
	try {
		sessions = readdirSync(root);
	} catch {
		return; // 目录不存在（还没 spill 过）
	}
	const cutoff = Date.now() - RETENTION_MS;
	for (const sid of sessions) {
		const dir = join(root, sid);
		let st;
		try {
			st = statSync(dir);
		} catch {
			continue;
		}
		if (!st.isDirectory()) continue;
		let files: string[];
		try {
			files = readdirSync(dir);
		} catch {
			continue;
		}
		for (const f of files) {
			const p = join(dir, f);
			try {
				if (statSync(p).mtimeMs < cutoff) unlinkSync(p);
			} catch {
				/* 单文件失败跳过，下轮再试 */
			}
		}
		try {
			rmdirSync(dir); // 空目录顺手回收；非空抛错忽略
		} catch {
			/* 非空 = 会话仍有新鲜 spill，保留 */
		}
	}
}

export default function (pi: ExtensionAPI) {
	sweepSpillFiles(process.cwd());

	pi.on("tool_result", async (event, ctx) => {
		const threshold = loadThreshold(ctx.cwd);
		if (threshold === null) return; // 显式禁用直通

		// D7 死循环防护：read 取回 spill 文件是显式取回意图，直通不判阈值
		if (event.toolName === "read") {
			const p = (event.input as { path?: unknown } | undefined)?.path;
			if (typeof p === "string" && toPosix(p).includes(`${SPILL_DIR}/`)) return;
		}

		// 文本块收集（D3：非 text 块原样保留在替换后的 content 里）
		const blocks: unknown[] = Array.isArray(event.content) ? event.content : [];
		const texts: string[] = [];
		const others: unknown[] = [];
		for (const b of blocks) {
			const t = b as { type?: string; text?: unknown } | null;
			if (t?.type === "text" && typeof t.text === "string") texts.push(t.text);
			else others.push(b);
		}
		if (!texts.length) return;

		const buf = Buffer.from(texts.join("\n"), "utf8");
		if (buf.length <= threshold) return; // 未超阈值原样通过（不写文件不记账）

		const sessionId = ctx.sessionManager?.getSessionId?.() ?? "nosession";
		const relDir = `${SPILL_DIR}/${sessionId}`;
		// D4：callHash 由 toolCallId 确定性派生（SHA-256 前 16 hex）——同毫秒同名工具的并行结果
		// 写不同路径；原始 ID 不进文件名（不泄露内部标识）；相同输入同哈希（可重现）
		const callHash = createHash("sha256")
			.update(String(event.toolCallId ?? ""))
			.digest("hex")
			.slice(0, 16);
		const file = `${Date.now()}-${sanitizeTool(event.toolName)}-${callHash}.txt`;
		const relPath = `${relDir}/${file}`;
		const absDir = join(ctx.cwd, relDir);
		const absFile = join(ctx.cwd, relPath);
		// D5：POSIX 平台严格权限收敛；Windows 不承诺 POSIX mode 语义，仅保留创建时 mode 参数
		const posix = process.platform !== "win32";

		try {
			// 会话目录：新建带 0700；POSIX 平台对已存在目录显式收敛到 0700（失败按写盘失败降级）
			mkdirSync(absDir, { recursive: true, mode: 0o700 });
			if (posix) chmodSync(absDir, 0o700);
			// 新文件：0600 排他创建（意外同名不覆盖已有归档，走既有降级）；
			// POSIX 平台写后显式收敛，失败时清理本次新文件再抛入既有 catch（不留不安全路径假充成功）
			let created = false;
			try {
				writeFileSync(absFile, buf, { flag: "wx", mode: 0o600 });
				created = true;
				if (posix) chmodSync(absFile, 0o600);
			} catch (e) {
				if (created) {
					try {
						unlinkSync(absFile);
					} catch {
						/* 清理失败交给 30 天清扫兜底 */
					}
				}
				throw e;
			}

			// 预览（D3 格式）：头 2048 + 标记 + 尾 512（超阈值必然 > 头+尾，tail 恒存在）
			const headEnd = utf8Boundary(buf, Math.min(HEAD_BYTES, buf.length));
			const tailStart = utf8Boundary(buf, Math.max(0, buf.length - TAIL_BYTES));
			const head = buf.subarray(0, headEnd).toString("utf8");
			const tail = buf.subarray(tailStart).toString("utf8");
			const omitted = tailStart - headEnd;
			const preview = [
				`[spill] 完整输出 ${buf.length} 字节已存 ${relPath}，需要完整内容时用 read 读取该路径（已省略 ${omitted} 字节）`,
				`--- 头部 ${headEnd} 字节 ---`,
				head,
				`--- 尾部 ${buf.length - tailStart} 字节 ---`,
				tail,
			].join("\n");

			logEvent(ctx.cwd, {
				kind: "spill.write",
				sessionId,
				payload: { tool: event.toolName, bytes: buf.length, path: relPath, ok: true },
			});
			const kept = [{ type: "text", text: preview }, ...others];
			return { content: kept as unknown as typeof event.content };
		} catch (e) {
			// D6 失败安全：原结果原样通过，仅记账
			logEvent(ctx.cwd, {
				kind: "spill.write",
				sessionId,
				payload: {
					tool: event.toolName,
					bytes: buf.length,
					ok: false,
					error: String(e).slice(0, 200),
				},
			});
			return;
		}
	});
}
