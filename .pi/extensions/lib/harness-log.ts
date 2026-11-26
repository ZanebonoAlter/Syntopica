/**
 * harness-log — harness 事实库共享模块（唯一写入方）@Syntopica
 *
 * 迁移自源项目（快照 docs/research/lib/harness-log.ts），change: harness-facts-tier-a。
 * 存储：.pi/harness/events.db（SQLite WAL；单表 + JSON payload；append-only，除 TTL 清扫外不删）。
 * 设计文档：openspec/changes/harness-facts-tier-a/design.md（D1/D2/D6）。
 *
 * 事件类型（kind）与保留期（开库时清扫）：
 *   session.start      90 天
 *   session.rollup     90 天（harness-effectiveness-metrics：单会话效能汇总快照，同 session 取最新一条即终值）
 *   constraint.inject  30 天
 *   pin.read           30 天
 *   gate.check         30 天
 *   subagent.dispatch  30 天
 *   stage.set          30 天（stage-driven-constraint-injection D7：取代 mode.set——阶段/绑定变化记账）
 *   constraint.read    30 天（stage-driven-constraint-injection D6：约束文档读取记账；归档对账唯一事实源）
 *   mode.set           30 天（legacy：改造前既有行——只读 + TTL 自然消退，写侧已切 stage.set）
 *   spill.write        30 天（tool-output-spill：大工具结果 spill 记账）
 *   subagent.complete  30 天（harness-observability-fixes D3：后台子线程完成回填）
 *   policy.decision    30 天（harden-harness-policy-and-spill：spec-gate/quota-gate/test-scope-guard 显著裁决统一记账）
 *   edit.map           30 天（coordinate-concurrent-changes：change→编辑文件归属地图，quality-gate 落库侧并集快照）
 *   patrol.check       30 天（test-debt-patrol：测试欠账巡检分片流水，仅流水；欠账生命周期由台账表自身持久化）
 *   change.archive     30 天（task-cost-metrics：归档成功锚点——「普通成功放行零记录」低噪声约束的唯一成功侧事实例外，
 *                      归档低频、高价值锚点，retro B' 每任务成本的分母）
 *   change.abandon     30 天（add-change-lifecycle-retro：change 放弃终态，与 change.archive 家族对称；
 *                      专用登记脚本 scripts/harness/change-abandon.sh 直写，append-only 不去重；
 *                      消费侧同名 archive 并存时以 change.archive 为准并提示冲突）
 *   pin.write          永久
 *
 * 安全开库（design D2）：
 *   - 所有写 PRAGMA（含 journal_mode=WAL）必须在 application_id/user_version 校验通过之后
 *   - 他人库（app_id 未登记但已有用户表 / app_id 非本应用）与未来版本（version 更高）一律拒绝
 *     （append-only 审计账本不做 dsh 派生库的"重置重建"，丢失风险由 TTL 与 100MB 保险丝管）
 *
 * 设计原则：
 *   - 记账不靠模型：仅扩展调用，模型零参与
 *   - fail-loud：写入失败返回 false 并 console.error，调用方负责一次性 notify
 *
 * 用法：
 *   logEvent(cwd, { kind: "gate.check", sessionId, change, payload })
 *   queryBySession(cwd, sessionId, ["constraint.inject"])
 *   queryByChange(cwd, "my-change")
 *   logStageSet(cwd, sessionId, stage, stageFamily, boundChange, source)
 *   queryLatestStageSet(sessionId)        // 双读：stage.set → 旧 mode.set 按 mode 映射回族
 *   logConstraintRead(cwd, sessionId, docPath, boundChange)
 *   queryConstraintReads(sessionId) / queryConstraintReadsByChange(changeName)
 */
import { mkdirSync, statSync } from "node:fs";
import { join } from "node:path";
import { createRequire } from "node:module";
import {
	stageFamilyOf,
	stageFamilyOfLegacyMode,
	type Stage,
	type StageFamily,
} from "./stage";

/* ---------- node:sqlite 引入（含实验性 warning 外科抑制） ---------- */

// node:sqlite 在 Node 22 是实验性 API，首次 import 会向 stderr 打 ExperimentalWarning，
// 会花 TUI 屏幕。摘除 warning 监听后 require，再装回「过滤 sqlite、其余转发原监听器」的包装，
// 保证只吞这一条 warning，其他 deprecation 警告照常可见。
interface StatementSyncLike {
	run(...params: unknown[]): unknown;
	get(...params: unknown[]): unknown;
	all(...params: unknown[]): unknown[];
}
interface DatabaseSyncLike {
	exec(sql: string): void;
	prepare(sql: string): StatementSyncLike;
	close(): void;
}
type WarningListener = (warning: Error) => void;

function loadSqlite(): new (path: string) => DatabaseSyncLike {
	const prior = process.listeners("warning");
	process.removeAllListeners("warning");
	// 锚点用 cwd 假文件：node: 内建模块解析不依赖锚点存在，
	// 且规避 esbuild CJS bundle 下 import.meta.url 为空的问题
	const require = createRequire(`${process.cwd()}/[harness-log]`);
	const sqlite = require("node:sqlite") as {
		DatabaseSync: new (path: string) => DatabaseSyncLike;
	};
	process.on("warning", (warning: Error) => {
		if (
			warning.name === "ExperimentalWarning" &&
			/sqlite/i.test(warning.message)
		) {
			return; // 仅吞 node:sqlite 的实验性提示
		}
		for (const l of prior) {
			if (typeof l === "function") (l as WarningListener)(warning);
		}
	});
	return sqlite.DatabaseSync;
}
const DatabaseSync = loadSqlite();

/* ---------- 常量与类型 ---------- */

const DB_DIR = ".pi/harness";
const DB_FILE = "events.db";
// 保险丝：库文件超 100MB 删最老一半（正常量级十年碰不到，仅兜底）
const SIZE_FUSE_BYTES = 100 * 1024 * 1024;
// 安全开库魔数（ASCII "SYNT" 小端打包）与当前 schema 版本（design D2）
const APP_ID = 0x53594e54;
const SCHEMA_VERSION = 1;

/**
 * 事件 kind 词汇表。
 *
 * `stage.set` 取代 `mode.set`（stage-driven-constraint-injection D7）：写侧已切 `stage.set`，
 * `mode.set` 作为**过渡期 legacy kind** 保留——既有行不改写不删除（30 天保留期自然消退），
 * 读方过渡期双读（`queryLatestStageSet`）。30 天后可从本表移除 `mode.set`。
 */
export type HarnessEventKind =
	| "session.start"
	| "session.rollup"
	| "constraint.inject"
	| "constraint.read"
	| "pin.write"
	| "pin.read"
	| "gate.check"
	| "patrol.check"
	| "subagent.dispatch"
	| "stage.set"
	| "mode.set"
	| "spill.write"
	| "subagent.complete"
	| "policy.decision"
	| "edit.map"
	| "change.archive"
	| "change.abandon";

export interface HarnessEventInput {
	kind: HarnessEventKind;
	sessionId: string;
	/** 绑定的 openspec change 名，无则省略 */
	change?: string | null;
	payload: Record<string, unknown>;
}

export interface HarnessEventRow {
	id: number;
	ts: string;
	session_id: string;
	kind: string;
	change: string | null;
	payload: string;
}

const RETENTION_DAYS: Partial<Record<HarnessEventKind, number>> = {
	"session.start": 90,
	"session.rollup": 90,
	"constraint.inject": 30,
	"constraint.read": 30,
	"pin.read": 30,
	"gate.check": 30,
	"subagent.dispatch": 30,
	"stage.set": 30,
	"mode.set": 30, // legacy：既有行 30 天自然消退（不改写不删除）
	"spill.write": 30,
	"subagent.complete": 30,
	"policy.decision": 30,
	"edit.map": 30,
	"patrol.check": 30,
	"change.archive": 30,
	"change.abandon": 30,
	// pin.write 永久，不列
};

const DDL = `
CREATE TABLE IF NOT EXISTS events (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  ts         TEXT NOT NULL,
  session_id TEXT NOT NULL,
  kind       TEXT NOT NULL,
  change     TEXT,
  payload    TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ev_session ON events(session_id, id);
CREATE INDEX IF NOT EXISTS idx_ev_change  ON events(change, id);
CREATE INDEX IF NOT EXISTS idx_ev_kind_ts ON events(kind, ts);
`;

/* ---------- 开库（按路径缓存实例；session 切换 cwd 安全） ---------- */

const dbCache = new Map<string, DatabaseSyncLike>();

function pragmaNumber(db: DatabaseSyncLike, name: string): number {
	const row = db.prepare(`PRAGMA ${name}`).get() as
		| Record<string, unknown>
		| undefined;
	const v = row?.[name];
	return typeof v === "number" ? v : 0;
}

function openDb(cwd: string): DatabaseSyncLike | null {
	const file = join(cwd, DB_DIR, DB_FILE);
	if (dbCache.has(file)) return dbCache.get(file) ?? null;
	let db: DatabaseSyncLike | null = null;
	try {
		mkdirSync(join(cwd, DB_DIR), { recursive: true });
		db = new DatabaseSync(file);
		// —— 安全开库（design D2）：先只读校验，任何写 PRAGMA（含 WAL）都在检查通过之后 ——
		const appId = pragmaNumber(db, "application_id");
		const userVersion = pragmaNumber(db, "user_version");
		const tableCount = (
			db
				.prepare(
					"SELECT COUNT(*) AS n FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'",
				)
				.get() as { n?: number } | undefined
		)?.n ?? 0;

		if (appId === 0 && tableCount === 0) {
			// 全新库（node:sqlite 打开即建空文件，app_id=0 且无用户表）：登记魔数 + 版本
			db.exec(`PRAGMA application_id = ${APP_ID}`);
			db.exec(`PRAGMA user_version = ${SCHEMA_VERSION}`);
		} else if (appId === APP_ID && userVersion <= SCHEMA_VERSION) {
			// 本应用库（当前版本或半初始化的 version=0；DDL IF NOT EXISTS 幂等补齐）
		} else {
			const why =
				appId === 0
					? `app_id 未登记但已有 ${tableCount} 张用户表（他人未登记库，静默建表会污染对方 schema）`
					: appId !== APP_ID
						? `app_id 0x${appId.toString(16)} 非本应用魔数（他人库）`
						: `user_version ${userVersion} 高于当前支持的 ${SCHEMA_VERSION}（未来版本的库，降级写入会弄坏）`;
			console.error(`[harness-log] 拒绝打开 ${file}：${why}；不做任何写入`);
			db.close();
			return null;
		}
		// —— 检查通过：写 PRAGMA 与 DDL 才允许执行 ——
		db.exec("PRAGMA journal_mode = WAL");
		db.exec("PRAGMA synchronous = NORMAL");
		db.exec("PRAGMA busy_timeout = 5000");
		db.exec(DDL);
		// TTL 清扫：按 kind 分保留期（ISO 字符串字典序 = 时间序）
		for (const [kind, days] of Object.entries(RETENTION_DAYS)) {
			const cutoff = new Date(Date.now() - days * 86400e3).toISOString();
			db.prepare("DELETE FROM events WHERE kind = ? AND ts < ?").run(
				kind,
				cutoff,
			);
		}
		// 保险丝：超限删最老一半
		if (statSync(file).size > SIZE_FUSE_BYTES) {
			db.prepare(
				"DELETE FROM events WHERE id <= (SELECT id FROM events ORDER BY id DESC LIMIT 1 OFFSET (SELECT COUNT(*) / 2 FROM events))",
			).run();
		}
	} catch (e) {
		console.error(`[harness-log] open db failed: ${(e as Error).message}`);
		try {
			db?.close();
		} catch {
			/* ignore close failure after open error */
		}
		db = null;
	}
	// 仅缓存成功连接：锁冲突/权限等瞬时失败时，下一个事件可重试
	if (db) dbCache.set(file, db);
	return db;
}

/* ---------- 写入 / 查询 ---------- */

/** 追加一条事实。成功 true；失败 false（fail-loud，调用方负责一次性 notify）。 */
export function logEvent(cwd: string, ev: HarnessEventInput): boolean {
	const db = openDb(cwd);
	if (!db) return false;
	try {
		db.prepare(
			"INSERT INTO events (ts, session_id, kind, change, payload) VALUES (?, ?, ?, ?, ?)",
		).run(
			new Date().toISOString(),
			ev.sessionId,
			ev.kind,
			ev.change ?? null,
			JSON.stringify(ev.payload),
		);
		return true;
	} catch (e) {
		console.error(`[harness-log] insert failed: ${(e as Error).message}`);
		return false;
	}
}

/** 按会话查（dump 命令数据源）；kinds 省略 = 全部 */
export function queryBySession(
	cwd: string,
	sessionId: string,
	kinds?: HarnessEventKind[],
): HarnessEventRow[] {
	const db = openDb(cwd);
	if (!db) return [];
	try {
		if (!kinds || kinds.length === 0) {
			return db
				.prepare("SELECT * FROM events WHERE session_id = ? ORDER BY id")
				.all(sessionId) as HarnessEventRow[];
		}
		const placeholders = kinds.map(() => "?").join(", ");
		return db
			.prepare(
				`SELECT * FROM events WHERE session_id = ? AND kind IN (${placeholders}) ORDER BY id`,
			)
			.all(sessionId, ...kinds) as HarnessEventRow[];
	} catch (e) {
		console.error(`[harness-log] query failed: ${(e as Error).message}`);
		return [];
	}
}

/** 按 change 查（阶段收尾审计；edit.map 归属取数），按时间线排序；kinds 省略 = 全部 */
export function queryByChange(
	cwd: string,
	changeName: string,
	kinds?: HarnessEventKind[],
): HarnessEventRow[] {
	const db = openDb(cwd);
	if (!db) return [];
	try {
		if (!kinds || kinds.length === 0) {
			return db
				.prepare("SELECT * FROM events WHERE change = ? ORDER BY id")
				.all(changeName) as HarnessEventRow[];
		}
		const placeholders = kinds.map(() => "?").join(", ");
		return db
			.prepare(
				`SELECT * FROM events WHERE change = ? AND kind IN (${placeholders}) ORDER BY id`,
			)
			.all(changeName, ...kinds) as HarnessEventRow[];
	} catch (e) {
		console.error(`[harness-log] query failed: ${(e as Error).message}`);
		return [];
	}
}

/**
 * 全局按 kind 查最近一条（constraint-injection-tier-b design D6：resume 档位恢复的
 * 第二段兑底——resume 生成新 sessionId 时按全局最新 mode.set 恢复）。
 * 无匹配返回 null。idx_ev_kind_ts 索引现成。
 */
export function queryLatestByKind(
	cwd: string,
	kind: HarnessEventKind,
): HarnessEventRow | null {
	const db = openDb(cwd);
	if (!db) return null;
	try {
		return (
			(db
				.prepare("SELECT * FROM events WHERE kind = ? ORDER BY id DESC LIMIT 1")
				.get(kind) as HarnessEventRow | undefined) ?? null
		);
	} catch (e) {
		console.error(`[harness-log] query failed: ${(e as Error).message}`);
		return null;
	}
}

/* ---------- 阶段记账（stage.set）与约束文档读取记账（constraint.read） ---------- */

/** 阶段/绑定变化的归因来源（stage.set 与 legacy mode.set 同枚举） */
export type StageSetSource =
	| "command"
	| "skill"
	| "edit-dir"
	| "recover"
	| "inherit"
	| "fallback";

/** 阶段读取结果（stage.set 新事件 / legacy mode.set 双读回退） */
export interface StageSetInfo {
	/** 阶段（6 档取值）；legacy mode.set 行无 stage → null */
	stage: Stage | null;
	/** 阶段族（**恒由 stage 派生**；legacy 行按 mode 映射回族——D2：族不可独立写） */
	stageFamily: StageFamily | null;
	/** 绑定的 change 名（可空） */
	boundChange: string | null;
	/** 归因来源（可空） */
	source: string | null;
	/** 事件 kind（stage.set 或 legacy mode.set） */
	kind: "stage.set" | "mode.set";
	/** 事件时间戳（ISO 8601 UTC） */
	ts: string;
	/** 事件所属会话 */
	sessionId: string;
}

/**
 * 阶段/绑定变化记账（stage.set）。payload 含 stage / stageFamily / boundChange / source，
 * change 列同步写绑定 change 名。
 * stageFamily 入参只作兼容位——**落库值恒为 stage 派生值**（D2：阶段族不可独立写）。
 * 记账失败返回 false（fail-loud）；**MUST NOT 阻断阶段激活本身**（调用方忽略返回值即可）。
 */
export function logStageSet(
	cwd: string,
	sessionId: string,
	stage: Stage,
	stageFamily: StageFamily,
	boundChange: string | null,
	source: StageSetSource | string,
): boolean {
	const family = stageFamilyOf(stage) ?? stageFamily;
	return logEvent(cwd, {
		kind: "stage.set",
		sessionId,
		change: boundChange ?? null,
		payload: {
			stage,
			stageFamily: family,
			boundChange: boundChange ?? null,
			source,
		},
	});
}

/**
 * 查某会话的最新阶段（**双读过渡**，design D7）：
 * 先查 `stage.set`；无结果时查 legacy `mode.set`，按 `requirements`→planning、
 * `implementation`→implementation 映射回族返回（**不视为错误**）；两者皆无 → null。
 * 既有 mode.set 行不改写不删除（30 天保留期自然消退）——本函数只读。
 * cwd 省略时取 process.cwd()（事实库随 cwd 定位）。
 */
export function queryLatestStageSet(
	sessionId: string,
	cwd: string = process.cwd(),
): StageSetInfo | null {
	const stageRows = queryBySession(cwd, sessionId, ["stage.set"]);
	for (const row of [...stageRows].reverse()) {
		const info = stageSetInfoFromRow(row, "stage.set");
		if (info) return info;
	}
	const legacyRows = queryBySession(cwd, sessionId, ["mode.set"]);
	for (const row of [...legacyRows].reverse()) {
		const info = stageSetInfoFromRow(row, "mode.set");
		if (info) return info;
	}
	return null;
}

/**
 * 单行 → 阶段读取结果。
 * stage.set 的**合法 legacy 绑定行**（stage=null 且 stageFamily=null，由 logStageBinding /
 * logInvalidStageMarker 写入）必须作为「当前状态 = 绑定 boundChange、未绑定阶段」返回——
 * 若当损坏行跳过，queryLatestStageSet 会回退读到更旧的阶段行，导致恢复/继承/门禁
 * 拿到陈旧 change 与阶段（review R-1）。真正跳过的只有：payload 损坏、stage 非法字符串、
 * stage 与 stageFamily 矛盾（stage=null 但 family 非空）的行。
 */
function stageSetInfoFromRow(
	row: HarnessEventRow,
	kind: "stage.set" | "mode.set",
): StageSetInfo | null {
	let payload: {
		stage?: unknown;
		stageFamily?: unknown;
		boundChange?: unknown;
		source?: unknown;
		mode?: unknown;
	};
	try {
		payload = JSON.parse(row.payload) as typeof payload;
	} catch {
		return null;
	}
	if (!payload || typeof payload !== "object") return null;
	const boundChange =
		typeof payload.boundChange === "string" && payload.boundChange.length > 0
			? payload.boundChange
			: null;
	const source = typeof payload.source === "string" ? payload.source : null;

	if (kind === "stage.set") {
		const stage = typeof payload.stage === "string" ? payload.stage : null;
		// 合法 legacy 绑定行：stage=null 且 stageFamily=null → 绑定仍在、无阶段（不是损坏）
		if (stage === null && payload.stageFamily == null) {
			return {
				stage: null,
				stageFamily: null,
				boundChange,
				source,
				kind,
				ts: row.ts,
				sessionId: row.session_id,
			};
		}
		const family = stageFamilyOf(stage);
		if (!stage || !family) return null;
		return {
			stage: stage as Stage,
			stageFamily: family,
			boundChange,
			source,
			kind,
			ts: row.ts,
			sessionId: row.session_id,
		};
	}
	// legacy mode.set：行内无 stage 字段，按 mode 映射回族（requirements→planning）
	const family = stageFamilyOfLegacyMode(
		typeof payload.mode === "string" ? payload.mode : null,
	);
	if (!family) return null;
	return {
		stage: null,
		stageFamily: family,
		boundChange,
		source,
		kind,
		ts: row.ts,
		sessionId: row.session_id,
	};
}

/**
 * 约束文档读取记账（constraint.read）：payload = { path }（文档的仓库根相对路径），
 * change 列为读取时刻会话绑定的 change（可空）。
 * **写入失败 MUST NOT 阻断 agent 动作**——返回 false 由调用方忽略即可（fail-loud 记账、流程照常）。
 * 幂等：同一会话 + 同一文档 + 同一 change 只记一条（避免事实库膨胀）。
 */
export function logConstraintRead(
	cwd: string,
	sessionId: string,
	docPath: string,
	boundChange: string | null,
): boolean {
	if (typeof docPath !== "string" || docPath.trim().length === 0) {
		console.error("[harness-log] constraint.read 缺文档路径，未记账");
		return false;
	}
	const p = docPath.trim();
	for (const row of queryConstraintReads(sessionId, cwd)) {
		if ((row.change ?? null) !== (boundChange ?? null)) continue;
		if (constraintReadPath(row) !== p) continue;
		return true; // 已记过——不重复追加
	}
	return logEvent(cwd, {
		kind: "constraint.read",
		sessionId,
		change: boundChange ?? null,
		payload: { path: p, boundChange: boundChange ?? null },
	});
}

/** constraint.read 行的文档路径（payload 损坏 → null） */
export function constraintReadPath(row: HarnessEventRow): string | null {
	try {
		const payload = JSON.parse(row.payload) as { path?: unknown };
		return typeof payload.path === "string" ? payload.path : null;
	} catch {
		return null;
	}
}

/** 按会话查约束文档读取记录（kind=constraint.read，按 id 升序） */
export function queryConstraintReads(
	sessionId: string,
	cwd: string = process.cwd(),
): HarnessEventRow[] {
	return queryBySession(cwd, sessionId, ["constraint.read"]);
}

/** 按 change 查约束文档读取记录（kind=constraint.read，按 id 升序） */
export function queryConstraintReadsByChange(
	changeName: string,
	cwd: string = process.cwd(),
): HarnessEventRow[] {
	return queryByChange(cwd, changeName, ["constraint.read"]);
}
