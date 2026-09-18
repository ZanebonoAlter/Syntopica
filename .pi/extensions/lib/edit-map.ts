/**
 * edit-map — change 文件归属地图（共享模块）@Syntopica
 *
 * change: coordinate-concurrent-changes（design D2，实现修正见 change explore-findings §3）。
 *
 * 职责：把「会话本回合新增/变化路径」聚合到「会话绑定 change 名下的累计编辑集合」，
 * 以 edit.map 事件（全量快照）落事实库——归档门禁/spec-gate/doc-impact verify/
 * concurrency-status.sh 按此判定「树上脏文件归属哪个 change」。
 *
 * 语义要点：
 *   - boundChange 取同 session 最近一条 mode.set（档位绑定语义），非目录 mtime 启发式
 *     （detectActiveChange 语义太弱，不适合归属判定）；无 mode.set / boundChange=null
 *     → 无档会话，零事件不计入任何 change
 *   - 跨 session 并集：落库侧合并（非查询侧）——首次绑定（或档位切换）时从库读该
 *     change 名下最新一条 edit.map 的 paths 作 base，之后每回合 base ∪= trigger 后
 *     落全量快照；「取最新一条即完整集合」因此成立
 *   - 模块状态（base/bound）在 session_start/session_shutdown 由 quality-gate 调
 *     resetEditMapState() 重置（pi-subagents 共享模块实例，startup 不清——同
 *     quality-gate ownerSessionId 防御，由调用方保证时机）
 *   - 严格旁路：一切异常吞掉返回 false，绝不影响 quality-gate 门禁本身
 */
import {
	logEvent,
	queryBySession,
	queryByChange,
	type HarnessEventRow,
} from "./harness-log";
import { join } from "node:path";
import { createRequire } from "node:module";

/* ---------- node:sqlite 只读引入（latestEditMapByChange 专用） ----------
 *  harness-log 是唯一写入方且不导出连接/全表查询；本函数只读且不改 schema，
 *  为不扩 harness-log 导出面（编辑辖区限制），在此用同款警告抑制模式开只读连接。
 *  只读连接不做任何写 PRAGMA/DDL，无 schema 污染风险；表不存在/库损坏等异常
 *  由外层 try 吞掉返回空 Map（信号缺席 → 归因退化基线单信号，不扩大外部面）。
 *  构造器模块级缓存：latestEditMapByChange 每个 turn_end 都会调，若每次都重挂
 *  warning 转发 listener 会逐回合叠链（N 回合 N 层闭包），缓存后只建一次。 */
interface ReadonlyStatementLike {
	all(...params: unknown[]): unknown[];
}
interface ReadonlyDbLike {
	prepare(sql: string): ReadonlyStatementLike;
	close(): void;
}

let cachedSqliteCtor: (new (
	path: string,
	opts: { readOnly: boolean },
) => ReadonlyDbLike) | null = null;

function loadSqliteReadonly(): new (
	path: string,
	opts: { readOnly: boolean },
) => ReadonlyDbLike {
	if (cachedSqliteCtor) return cachedSqliteCtor;
	const prior = process.listeners("warning");
	process.removeAllListeners("warning");
	const require = createRequire(`${process.cwd()}/[edit-map]`);
	const sqlite = require("node:sqlite") as {
		DatabaseSync: new (path: string, opts: { readOnly: boolean }) => ReadonlyDbLike;
	};
	process.on("warning", (warning: Error) => {
		if (warning.name === "ExperimentalWarning" && /sqlite/i.test(warning.message)) {
			return;
		}
		for (const l of prior) {
			if (typeof l === "function") l(warning);
		}
	});
	cachedSqliteCtor = sqlite.DatabaseSync;
	return cachedSqliteCtor;
}

/** 从一条 mode.set 行解析 boundChange；行缺失/payload 损坏/无绑定 → null（跳过不落） */
export function parseModeSetPayload(
	row: Pick<HarnessEventRow, "payload"> | null | undefined,
): string | null {
	if (!row || typeof row.payload !== "string") return null;
	try {
		const p = JSON.parse(row.payload) as { boundChange?: unknown };
		return typeof p.boundChange === "string" && p.boundChange.length > 0
			? p.boundChange
			: null;
	} catch {
		return null;
	}
}

/** 并集 + 字典序排序去重（快照确定性：同集合任何时序产出相同 payload） */
export function mergePaths(
	base: Iterable<string>,
	add: Iterable<string>,
): string[] {
	const s = new Set<string>();
	for (const p of base) if (typeof p === "string" && p) s.add(p);
	for (const p of add) if (typeof p === "string" && p) s.add(p);
	return [...s].sort();
}

/**
 * 工具自管路径判定（fix-doc-impact-misattribution D1 采集侧）：
 * openspec CLI 的归档移动产物与元数据不属于任何 change 的真实工作范围，
 * 混入归属集合会污染 doc-impact 对账与并发态势对照。共享文档（AGENTS.md 等）
 * 不在此层剔除——并发冲突感知需要（design D1 双层分工，消费侧由 verify 黑名单过滤）。
 */
export function isToolManagedPath(p: string): boolean {
	if (p.startsWith("openspec/changes/archive/")) return true;
	// active change 目录下的 .openspec.yaml（openspec 自管元数据，含 configure-* 误命中源）
	if (
		/^openspec\/changes\/[^/]+\/\.openspec\.yaml$/.test(p)
	)
		return true;
	return false;
}

/** edit.map payload 构造（白盒直测点：结构契约 { paths, n }） */
export function buildEditMapPayload(paths: string[]): {
	paths: string[];
	n: number;
} {
	return { paths, n: paths.length };
}

/* 模块级会话状态（resetEditMapState 重置；跨会话残留由 quality-gate session 边界清理） */
let editMapBase: string[] = [];
let editMapBound: string | null = null;

/** 会话边界重置（quality-gate session_start[reason≠startup]/session_shutdown 调用） */
export function resetEditMapState(): void {
	editMapBase = [];
	editMapBound = null;
}

/**
 * 归属聚合主入口：读同 session 最近 mode.set 取 boundChange，维护累计快照并落库。
 * 返回 true=落库成功；false=未落库（无档会话/异常，均属正常旁路）。
 * trigger 非空由调用方保证（quality-gate 在 trigger.size>0 时才调）。
 */
export function syncEditMap(
	cwd: string,
	sessionId: string,
	trigger: Iterable<string>,
): boolean {
	try {
		const rows = queryBySession(cwd, sessionId, ["mode.set"]);
		const bound = parseModeSetPayload(rows[rows.length - 1]);
		if (!bound) return false; // 无档会话不计入任何 change
		if (editMapBound !== bound) {
			// 首次绑定或档位切换：从库读该 change 最新快照作 base（跨 session 并集）。
			// 存量污染条目（工具自管路径）在读入时即剔除——append-only 不改历史事件，
			// 但新快照不再携带（采集侧止增，design「存量污染不清洗」的消费侧兜底同源）。
			const em = queryByChange(cwd, bound, ["edit.map"]);
			const last = em[em.length - 1];
			let base: string[] = [];
			if (last && typeof last.payload === "string") {
				const p = JSON.parse(last.payload) as { paths?: unknown };
				if (Array.isArray(p.paths))
					base = p.paths.filter(
						(x): x is string =>
							typeof x === "string" && !isToolManagedPath(x),
				);
			}
			editMapBase = base;
			editMapBound = bound;
		}
		// 采集侧黑名单：本回合 trigger 剔除工具自管路径后再聚合（fix 后新路径零污染）
		const triggerFiltered = [...trigger].filter(
			(p) => typeof p === "string" && !isToolManagedPath(p),
		);
		const merged = mergePaths(editMapBase, triggerFiltered);
		if (merged.length === 0) return false; // 全工具自管/空集：零事件不落库
		editMapBase = merged;
		return logEvent(cwd, {
			kind: "edit.map",
			sessionId,
			change: bound,
			payload: buildEditMapPayload(merged),
		});
	} catch {
		return false; // fail-open：归属落库失败绝不影响门禁
	}
}

/**
 * 只读查询：全部 change 的最新 edit.map 归属快照（attribute-concurrent-gate-noise 2.3）。
 * 每个 change 取 id 最大的一条（落库侧并集快照语义：最新一条即完整集合，与 syncEditMap
 * 读 base 同口径）；payload.paths 剔工具自管路径后返回 change → paths。
 * 严格旁路：库不存在/损坏/被锁/表缺失 → 空 Map（信号缺席，调用方退化基线单信号）。
 * 只读连接不建库不写 PRAGMA：库尚未创建时（新仓库首回合）直接 ENOENT → 空 Map。
 */
export function latestEditMapByChange(cwd: string): Map<string, string[]> {
	const out = new Map<string, string[]>();
	try {
		const DatabaseSync = loadSqliteReadonly();
		const db = new DatabaseSync(join(cwd, ".pi/harness", "events.db"), {
			readOnly: true,
		});
		try {
			const rows = db
				.prepare(
					"SELECT e.change AS change, e.payload AS payload FROM events e " +
						"WHERE e.kind = 'edit.map' AND e.change IS NOT NULL AND e.id IN " +
						"(SELECT MAX(id) FROM events WHERE kind = 'edit.map' GROUP BY change)",
				)
				.all() as { change: string; payload: unknown }[];
			for (const r of rows) {
				if (!r.change || typeof r.payload !== "string") continue;
				try {
					const p = JSON.parse(r.payload) as { paths?: unknown };
					if (!Array.isArray(p.paths)) continue;
					out.set(
						r.change,
						p.paths.filter(
							(x): x is string =>
								typeof x === "string" && !!x && !isToolManagedPath(x),
						),
					);
				} catch {
					/* 单行 payload 损坏：跳过该 change，不影响其余 */
				}
			}
		} finally {
			db.close();
		}
	} catch {
		/* 库不可用/未创建：fail-open 空 Map（不扩大外部面） */
	}
	return out;
}
