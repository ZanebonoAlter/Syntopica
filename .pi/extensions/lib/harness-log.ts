/**
 * harness-log — harness 事实库共享模块（唯一写入方）@Syntopica
 *
 * 迁移自源项目（快照 docs/research/lib/harness-log.ts），change: harness-facts-tier-a。
 * 存储：.pi/harness/events.db（SQLite WAL；单表 + JSON payload；append-only，除 TTL 清扫外不删）。
 * 设计文档：openspec/changes/harness-facts-tier-a/design.md（D1/D2/D6）。
 *
 * 事件类型（kind）与保留期（开库时清扫）：
 *   session.start      90 天
 *   constraint.inject  30 天
 *   pin.read           30 天
 *   gate.check         30 天
 *   subagent.dispatch  30 天
 *   mode.set           30 天（constraint-injection-tier-b：resume 档位恢复取数）
 *   spill.write        30 天（tool-output-spill：大工具结果 spill 记账）
 *   subagent.complete  30 天（harness-observability-fixes D3：后台子线程完成回填）
 *   policy.decision    30 天（harden-harness-policy-and-spill：spec-gate/quota-gate/test-scope-guard 显著裁决统一记账）
 *   edit.map           30 天（coordinate-concurrent-changes：change→编辑文件归属地图，quality-gate 落库侧并集快照）
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
 */
import { mkdirSync, statSync } from "node:fs";
import { join } from "node:path";
import { createRequire } from "node:module";

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

export type HarnessEventKind =
	| "session.start"
	| "constraint.inject"
	| "pin.write"
	| "pin.read"
	| "gate.check"
	| "subagent.dispatch"
	| "mode.set"
	| "spill.write"
	| "subagent.complete"
	| "policy.decision"
	| "edit.map";

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
	"constraint.inject": 30,
	"pin.read": 30,
	"gate.check": 30,
	"subagent.dispatch": 30,
	"mode.set": 30,
	"spill.write": 30,
	"subagent.complete": 30,
	"policy.decision": 30,
	"edit.map": 30,
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
