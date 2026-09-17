/**
 * dev-process-guard.ts — pi extension：孤儿 dev 进程治理（会话生命周期挂钩）@Syntopica
 *
 * change: dev-process-guard（design D1~D7；用例契约 openspec/changes/dev-process-guard/test-cases.md）。
 * 背景：AI 会话起 dev 服务用手搓 nohup setsid 全脱管，2026-09-17 实测孤儿 Nuxt 栈 +
 * 手搓后端 + agent-browser 无头 chromium 整树残留（load 9.17 / 内存 5.9G）。
 *
 * 挂点（D1）：
 *   - session_start：记会话窗口起点（Map<sessionId, epoch 秒>，按会话隔离；
 *     从不整体清空 Map——子线程共享模块实例时 session_start{startup} 只是记录自己的键，
 *     不会误伤主会话窗口；quality-gate/entry-gate 同款防御方向）。
 *   - session_shutdown：扫描泄漏类 dev 进程（八模式 cmdline + cwd 仓库内 + 无控制终端
 *     + 非 pidfile 接管 + 非自身组，五条件合取），启动时间 ∈ 本会话窗口 [start, now]
 *     （闭区间）的按组 TERM → 轮询 2s（150ms 间隔）→ 仍存活 KILL（D4 状态机）；
 *     同 pgid 去重只发一轮信号；单组失败 fail-open 跳过（EPERM 不重试）；
 *     pgid ≤ 1 组信号红线双保险；落 orphan-killed 事件。
 *   - turn_end：存在泄漏（含窗口外历史遗留）→ 每会话至多一次 steer 软提醒（区分窗口
 *     内外，给 start-dev.sh 处置建议）＋落 orphan-warn 事件；零泄漏零输出零记录。
 *
 * 记账（design D6）：直写 logEvent(kind:"policy.decision", payload.decision="orphan-killed"
 *   |"orphan-warn")——不走 logPolicyDecision：其 action 白名单（block|warn|bypass|fail-open）
 *   会让 harness-retro 既有分桶（③按 fail-open/④按 warn）误吸本扩展事件；用 decision 键
 *   而非 action 键，retro 按 $.action 过滤自然跳过，零污染且可独立考古。
 *
 * 平台边界（D7）：非 Linux 或 /proc 不可读 → 钩子注册但全程 no-op + 一次性 console.warn。
 * fail-open：扫描/单进程读取/组杀/记账任一异常跳过该目标继续，绝不阻断会话关闭链。
 *
 * 可测性缝（test-cases §0.2）：createDevProcessGuard(pi, deps) 工厂——deps 即依赖注入
 * 对象（readStat/readCmdline/readCwd/listProcDir/killGroup/now/platform/logEvent/
 * consoleWarn + readBtime/getClkTck/readPidfiles/groupAlive），全部可选、缺省接真实实现；
 * 冒烟经工厂注入桩回放 handler 级场景（I 节）。模块级 Map<sessionId, startedAt> 存窗口。
 *
 * 配置：DEV_GUARD_ENABLE（默认开，"0"/"false"/"off" 关闭，静默 no-op）。
 */
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { logEvent } from "./lib/harness-log";
import {
	computeStarttimeSec,
	decide,
	getClkTckDefault,
	groupAliveDefault,
	killGroupDefault,
	listProcDirDefault,
	parseStatLine,
	probeProcFsDefault,
	readBtimeDefault,
	readCmdlineDefault,
	readCwdDefault,
	readPidfilesDefault,
	readStatDefault,
	type ProcSnapshot,
} from "./lib/dev-process-scan";

/* ---------- 常量与依赖注入缝 ---------- */

/** 总开关，默认开 */
const ENABLED = !["0", "false", "off"].includes(
	(process.env.DEV_GUARD_ENABLE ?? "").toLowerCase(),
);
/** TERM 后组存活宽限（毫秒），硬顶防 shutdown 挂死 */
const TERM_GRACE_MS = 2000;
/** 宽限轮询间隔（毫秒） */
const POLL_INTERVAL_MS = 150;
/** 事件 procs/offenders 条数上限 */
const EVENT_PROCS_MAX = 20;
/** 事件内 cmdline 截断上限 */
const EVENT_CMDLINE_MAX = 120;

/** 依赖注入对象（全部可替换；冒烟经工厂注入桩回放） */
export interface DevGuardDeps {
	listProcDir: () => string[];
	readStat: (pid: number) => string | null;
	/** /proc/stat btime（epoch 秒）；无 btime 行 → null */
	readBtime: () => number | null;
	getClkTck: () => number;
	readCmdline: (pid: number) => string | null;
	readCwd: (pid: number) => string | null;
	readPidfiles: (repoRoot: string) => Set<number>;
	killGroup: (pgid: number, sig: "TERM" | "KILL") => void;
	/** 组存活谓词（B-12 经 deps 注入使宽限状态机可测） */
	groupAlive: (pgid: number) => boolean;
	/** epoch 秒 */
	now: () => number;
	platform: NodeJS.Platform;
	logEvent: typeof logEvent;
	consoleWarn: (msg: string) => void;
}

const defaultDeps: DevGuardDeps = {
	listProcDir: listProcDirDefault,
	readStat: readStatDefault,
	readBtime: readBtimeDefault,
	getClkTck: getClkTckDefault,
	readCmdline: readCmdlineDefault,
	readCwd: readCwdDefault,
	readPidfiles: readPidfilesDefault,
	killGroup: killGroupDefault,
	groupAlive: groupAliveDefault,
	// 亚秒精度：floor 到整秒会把闭区间右端压小，同秒内新起进程（starttime 带小数）会被
	// 误判到窗口外（B-12 真实进程版实测踩中）；桩世界注入整数不受影响
	now: () => Date.now() / 1000,
	platform: process.platform,
	logEvent,
	consoleWarn: (msg: string) => console.warn(msg),
};

/* ---------- 模块级会话状态（子线程共享模块实例，按 sessionId 隔离；loadFresh 即重置） ---------- */

/** 会话窗口起点（epoch 秒）——I-08 双会话隔离的载体 */
const windowsBySession = new Map<string, number>();
/** 已软提醒过的会话（每会话至多一次，I-04 二次静默） */
const warnedSessions = new Set<string>();
/** 平台 no-op 的一次性 warn 标记（I-07 恰 warn 一次） */
let platformWarned = false;
/** /proc 可用性（真实 fs 探测，不走注入缝——平台边界说的是真实机器） */
let procFsOk: boolean | null = null;

/* ---------- 小工具 ---------- */

const sleep = (ms: number) => new Promise<void>((res) => setTimeout(res, ms));

/** 尾斜杠规范化（保留根 "/"） */
function normalizeRepoRoot(p: string): string {
	const s = String(p ?? "").replace(/\/+$/, "");
	return s === "" ? "/" : s;
}

/** 事件用年龄（秒）：非有限值（btime 缺失等）→ 0 */
function ageOf(nowSec: number, starttimeSec: number): number {
	const d = nowSec - starttimeSec;
	return Number.isFinite(d) ? Math.max(0, Math.round(d)) : 0;
}

/** 事件用 cmdline 截断 */
function truncateCmdline(s: string): string {
	return s.length <= EVENT_CMDLINE_MAX ? s : s.slice(0, EVENT_CMDLINE_MAX);
}

/** 年龄人话（I-03：须命中 \\d+[hms]），如 "16m41s" */
function formatAgeSec(sec: number): string {
	const s = Number.isFinite(sec) && sec > 0 ? Math.floor(sec) : 0;
	const h = Math.floor(s / 3600);
	const m = Math.floor((s % 3600) / 60);
	const r = s % 60;
	const parts: string[] = [];
	if (h > 0) parts.push(`${h}h`);
	if (h > 0 || m > 0) parts.push(`${m}m`);
	parts.push(`${r}s`);
	return parts.join("");
}

/** 平台边界门（D7）：非 Linux 或 /proc 不可读 → 整体 no-op + 一次性 warn */
function supported(d: DevGuardDeps): boolean {
	if (!ENABLED) return false; // 关闭：静默 no-op，不 warn
	if (d.platform === "linux" && procAvailable()) return true;
	if (!platformWarned) {
		platformWarned = true;
		d.consoleWarn(
			`[dev-process-guard] 当前平台 ${String(d.platform)} 非 Linux 或 /proc 不可读，孤儿 dev 进程治理 no-op`,
		);
	}
	return false;
}

function procAvailable(): boolean {
	if (procFsOk === null) procFsOk = probeProcFsDefault();
	return procFsOk;
}

/** 自身进程组：读 /proc/self/stat 的 pgrp；读不到 → -1（正 pgid 永不相等，fail-open） */
function ownPgidOf(d: DevGuardDeps): number {
	try {
		const stat = d.readStat(process.pid);
		const parsed = stat === null ? null : parseStatLine(stat);
		if (parsed && Number.isInteger(parsed.pgrp) && parsed.pgrp > 0) {
			return parsed.pgrp;
		}
	} catch {
		/* fallthrough */
	}
	return -1;
}

/** pidfile 白名单读取；异常 → 空集合（fail-open，宁可漏杀不可误杀） */
function safePidfiles(d: DevGuardDeps, repoRoot: string): Set<number> {
	try {
		return d.readPidfiles(repoRoot);
	} catch {
		return new Set<number>();
	}
}

/** 扫描 /proc 组装快照：单目标读取失败（null/抛错）一律保守跳过（B-04/B-13/I-06a/I-06c） */
function scanSnapshots(d: DevGuardDeps): ProcSnapshot[] {
	const btime = d.readBtime();
	const clkTck = d.getClkTck();
	const out: ProcSnapshot[] = [];
	for (const name of d.listProcDir()) {
		if (typeof name !== "string" || !/^\d+$/.test(name)) continue;
		const pid = Number(name);
		let statLine: string | null = null;
		try {
			statLine = d.readStat(pid);
		} catch {
			continue; // EACCES 等 → 保守跳过（I-06a）
		}
		if (typeof statLine !== "string") continue; // ENOENT（进程已退出）→ 跳过（B-04/I-06c）
		const stat = parseStatLine(statLine);
		if (stat === null) continue;
		let cmdline: string | null = null;
		try {
			cmdline = d.readCmdline(pid);
		} catch {
			continue;
		}
		if (typeof cmdline !== "string") continue;
		let cwd: string | null = null;
		try {
			cwd = d.readCwd(pid);
		} catch {
			continue; // EACCES（他人进程）→ 条件②无法证实 → 保守排除（B-13）
		}
		if (typeof cwd !== "string" || cwd.length === 0) continue;
		out.push({
			pid,
			pgid: stat.pgrp,
			cmdline,
			cwd,
			ttyNr: stat.ttyNr,
			starttimeSec: computeStarttimeSec(btime, stat.starttimeTicks, clkTck),
		});
	}
	return out;
}

/** D4 清理状态机：组 TERM → 轮询 2s → 仍存活 KILL。TERM 抛错上抛由调用方跳过整组（I-06d） */
async function terminateGroup(d: DevGuardDeps, pgid: number): Promise<void> {
	d.killGroup(pgid, "TERM");
	const t0 = Date.now();
	while (Date.now() - t0 < TERM_GRACE_MS) {
		await sleep(POLL_INTERVAL_MS);
		let alive = false;
		try {
			alive = d.groupAlive(pgid);
		} catch {
			alive = false; // 谓词故障 → 视为已死，避免误升级 KILL
		}
		if (!alive) return;
	}
	try {
		d.killGroup(pgid, "KILL");
	} catch {
		/* KILL 失败尽力而为 */
	}
}

/** turn_end 软提醒文本（I-03 要素：pid/pgid/年龄 \\d+[hms]/cmdline 子串/start-dev.sh 建议；区分窗口内外） */
function buildWarnMessage(
	leaks: ProcSnapshot[],
	kills: ProcSnapshot[],
	nowSec: number,
): string {
	const lines = leaks.slice(0, EVENT_PROCS_MAX).map(
		(p) =>
			`- pid ${p.pid} ｜ pgid ${p.pgid} ｜ 年龄 ${formatAgeSec(ageOf(nowSec, p.starttimeSec))} ｜ ${truncateCmdline(p.cmdline)}`,
	);
	const inWindow = kills.length;
	return [
		`[dev-process-guard] 检测到 ${leaks.length} 个泄漏类 dev 进程（cmdline 命中 dev/agent-browser 特征 + cwd 在仓库内 + 无控制终端 + 非 start-dev.sh pidfile 接管）：`,
		...lines,
		`其中 ${inWindow} 个启动于本会话窗口内（会话结束将被自动清理），${leaks.length - inWindow} 个为窗口外历史遗留（不自动杀，需人工确认后处置）。`,
		`建议：接管栈用 bash scripts/start-dev.sh stop 清理；无主进程先 ps 确认再 kill -- -<pgid>；新起 dev 服务一律走 bash scripts/start-dev.sh（自动接管，会话结束自动清理）。`,
	].join("\n");
}

/* ---------- 扩展入口 ---------- */

/**
 * 工厂（可测性缝）：deps 全部可选、缺省接真实实现；冒烟经此注入桩回放 I 节。
 * 会话状态在模块级（Map/Set），同模块多工厂实例共享——对应真实 pi 的
 * 子线程共享模块实例形态。
 */
export function createDevProcessGuard(
	pi: ExtensionAPI,
	deps?: Partial<DevGuardDeps>,
): void {
	const d: DevGuardDeps = { ...defaultDeps, ...deps };

	pi.on("session_start", async (_event, ctx) => {
		try {
			if (!supported(d)) return;
			const sessionId = ctx.sessionManager?.getSessionId?.();
			if (!sessionId) return;
			// 仅记录/覆盖本会话键，从不清空整表（reason==="startup" 的子线程派发
			// 只是多记一个键，主会话窗口不受影响——D1 防御）
			windowsBySession.set(sessionId, d.now());
		} catch (err) {
			d.consoleWarn(
				`[dev-process-guard] session_start 记录异常，跳过：${String(err)}`,
			);
		}
	});

	pi.on("session_shutdown", async (_event, ctx) => {
		try {
			if (!supported(d)) return;
			const sessionId = ctx.sessionManager?.getSessionId?.();
			if (!sessionId) return;
			const repoRoot = normalizeRepoRoot(ctx.cwd ?? process.cwd());
			const windowStart = windowsBySession.get(sessionId);
			windowsBySession.delete(sessionId); // 只清自己的窗口，其他会话不动（I-08）
			const nowSec = d.now();
			const { kills } = decide(scanSnapshots(d), {
				repoRoot,
				window: windowStart === undefined ? undefined : { start: windowStart },
				pidfilePgids: safePidfiles(d, repoRoot),
				ownPgid: ownPgidOf(d),
				now: nowSec,
			});
			if (kills.length === 0) return; // 窗口外遗留/无窗口 → 不杀不记（I-02/I-09）
			// 按组去重杀（I-01 同组双进程只发一轮信号）；pgid ≤ 1 红线双保险（B-02）
			const seenPgids = new Set<number>();
			for (const proc of kills) {
				if (proc.pgid <= 1 || seenPgids.has(proc.pgid)) continue;
				seenPgids.add(proc.pgid);
				try {
					await terminateGroup(d, proc.pgid);
				} catch {
					// 单组失败 fail-open：跳过继续下一组（I-06d EPERM 不重试）
				}
			}
			// 记账（D6：decision 键非 action 键）；失败不阻断（I-06b）
			try {
				d.logEvent(repoRoot, {
					kind: "policy.decision",
					sessionId,
					change: null,
					payload: {
						decision: "orphan-killed",
						cmd: "dev-process-guard",
						procs: kills.slice(0, EVENT_PROCS_MAX).map((p) => ({
							pid: p.pid,
							pgid: p.pgid,
							ageSec: ageOf(nowSec, p.starttimeSec),
							cmdline: truncateCmdline(p.cmdline),
						})),
						windowStartedAt: windowStart ?? null,
					},
				});
			} catch {
				/* 记账失败不阻断清理 */
			}
		} catch (err) {
			d.consoleWarn(
				`[dev-process-guard] session_shutdown 清理异常，跳过：${String(err)}`,
			);
		}
	});

	pi.on("turn_end", async (_event, ctx) => {
		try {
			if (!supported(d)) return;
			const sessionId = ctx.sessionManager?.getSessionId?.();
			if (!sessionId || warnedSessions.has(sessionId)) return; // 每会话至多一次（I-04）
			const repoRoot = normalizeRepoRoot(ctx.cwd ?? process.cwd());
			const windowStart = windowsBySession.get(sessionId);
			const nowSec = d.now();
			const { kills, leaks } = decide(scanSnapshots(d), {
				repoRoot,
				window: windowStart === undefined ? undefined : { start: windowStart },
				pidfilePgids: safePidfiles(d, repoRoot),
				ownPgid: ownPgidOf(d),
				now: nowSec,
			});
			if (leaks.length === 0) return; // 零泄漏 → 零输出零记录（I-05）
			warnedSessions.add(sessionId);
			pi.sendMessage(
				{
					customType: "dev-process-guard-reminder",
					content: buildWarnMessage(leaks, kills, nowSec),
				},
				{ deliverAs: "steer", triggerTurn: true },
			);
			try {
				d.logEvent(repoRoot, {
					kind: "policy.decision",
					sessionId,
					change: null,
					payload: {
						decision: "orphan-warn",
						cmd: "dev-process-guard",
						offenders: leaks.slice(0, EVENT_PROCS_MAX).map((p) => ({
							pid: p.pid,
							pgid: p.pgid,
							ageSec: ageOf(nowSec, p.starttimeSec),
							cmdline: truncateCmdline(p.cmdline),
						})),
						inWindow: kills.length,
						windowStartedAt: windowStart ?? null,
					},
				});
			} catch {
				/* 记账失败不阻断提醒 */
			}
		} catch (err) {
			d.consoleWarn(
				`[dev-process-guard] turn_end 提醒异常，跳过：${String(err)}`,
			);
		}
	});
}

/** 默认入口（真实 deps；冒烟/测试走 createDevProcessGuard） */
export default function (pi: ExtensionAPI): void {
	createDevProcessGuard(pi);
}
