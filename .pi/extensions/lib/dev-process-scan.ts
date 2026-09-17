/**
 * dev-process-scan — 孤儿 dev 进程扫描库：纯函数判定 + /proc IO 默认实现 @Syntopica
 *
 * change: dev-process-guard（design D2/D3/D8；用例契约 openspec/changes/dev-process-guard/test-cases.md §0）。
 * 分层（D8）：纯函数全部 export（esbuild bundle 成 .cjs 后直接 require 白盒断言）；
 * IO 给默认实现，扩展层经依赖注入缝（readStat/readCmdline/readCwd/listProcDir/...）整体替换为桩。
 *
 * 泄漏五条件（D3，合取缺一不杀）：
 *   ① cmdline 命中 dev 服务特征八模式；② cwd 在仓库根之下（含根本身）；
 *   ③ tty_nr == 0（无控制终端——用户在真实终端手起的服务永不自动杀，对「人」的豁免）；
 *   ④ PGID 不在 pidfile 白名单（经 start-dev.sh 起的 = 合法长驻，永不杀）；
 *   ⑤ PGID ≠ 扩展自身进程组。
 *
 * decide 输出契约（test-cases §0）：
 *   - kills ⊆ leaks；leaks 与窗口无关（供 turn_end 软提醒，含历史遗留）；
 *   - kills = leaks 中 starttimeSec ∈ [window.start, now]（闭区间）且 pgid > 1
 *     （kill(-1) 广播全系统属灾难性误杀，组信号红线只作用于 kills、不改 leaks 分类）；
 *   - window === undefined → kills = []（无法归因保守不杀）；
 *   - 路径规范化（尾斜杠）在判定函数内完成，decide 收到的输入无需预处理。
 *
 * pidfile fail-safe（B-09/B-10）：损坏（非纯数字/多行/空）/死组白名单值一律
 * 不贡献成员、不抛异常、无副作用——宁可漏杀不可误杀。
 */
import { execSync } from "node:child_process";
import { readdirSync, readFileSync, readlinkSync } from "node:fs";
import { join } from "node:path";

/* ---------- 类型 ---------- */

/** /proc 快照（scan 层已组装好的规范化输入，decide 只做纯判定） */
export interface ProcSnapshot {
	pid: number;
	pgid: number;
	/** /proc/<pid>/cmdline NUL→空格完整 join 后的整串 */
	cmdline: string;
	cwd: string;
	ttyNr: number;
	/** epoch 秒 */
	starttimeSec: number;
}

export interface DecideEnv {
	/** 仓库根绝对路径（判定函数内再做尾斜杠规范化） */
	repoRoot: string;
	/** 会话窗口起点（epoch 秒）；无 session_start 记录 = undefined */
	window: { start: number } | undefined;
	/** .pi/run/{backend,front}.pgid 解析出的白名单 */
	pidfilePgids: Set<number>;
	/** 扩展自身进程组 */
	ownPgid: number;
	/** epoch 秒 */
	now: number;
}

export interface DecideResult {
	kills: ProcSnapshot[];
	leaks: ProcSnapshot[];
}

/** /proc/<pid>/stat 解析出的所需字段 */
export interface ProcStatFields {
	pid: number;
	state: string;
	pgrp: number;
	ttyNr: number;
	starttimeTicks: number;
}

/* ---------- 纯函数：cmdline 六模式 ---------- */

/**
 * 八模式窄匹配（正则形状以 test-cases.md §2 给定为准；K 行邻界命中是回归锚点——
 * 防实现「顺手收紧/放宽」造成静漂移，要排除 K 类形状须另开 change 改模式）。
 * m2 的 `$` 锚定串尾（带参不命中，由父 go run m1 命中后整组清除兜底）；
 * m7/m8 治理 agent-browser 浏览器自动化残留（2026-09-17 实测：CLI + 无头 chromium
 * 整树 ~1.3GB 会话后残留）——m8 只锚 --user-data-dir=/tmp/agent-browser-chrome-
 * 临时 profile 前缀，用户自起 chromium 不带此标志，永不命中（N-15）。
 */
const DEV_CMDLINE_PATTERNS: readonly RegExp[] = [
	/go run .*cmd\/server\/main\.go/, // m1：go run 后端
	/\.cache\/go-build\/.*\/main$/, // m2：go run 编译产物子进程
	/\bsh -c nuxt dev\b/, // m3：nuxt dev 经 sh -c
	/nuxt\.mjs dev\b/, // m4：nuxt 入口
	/@nuxt\+cli.*dev\/index\.mjs/, // m5：@nuxt+cli dev worker
	/\bpnpm dev\b/, // m6：pnpm dev
	/\bagent-browser\b/, // m7：agent-browser CLI（AI 浏览器自动化）
	/--user-data-dir=\/tmp\/agent-browser-chrome-/, // m8：agent-browser 无头 chromium 临时 profile 窄锚点
];

/** cmdline 是否命中 dev 服务特征（空串/非字符串一律 false，B-14 僵尸空 cmdline） */
export function isDevCmdline(cmdline: string): boolean {
	if (typeof cmdline !== "string" || cmdline.length === 0) return false;
	return DEV_CMDLINE_PATTERNS.some((re) => re.test(cmdline));
}

/**
 * /proc/<pid>/cmdline 原始内容 → NUL→空格完整 join（尾部 NUL 过滤）。
 * 禁按 NUL 截断只取 argv[0]——B-07 锚定完整 join（"sh -c nuxt dev --host"）。
 */
export function joinCmdline(raw: string | Buffer): string {
	const s = typeof raw === "string" ? raw : raw.toString("utf8");
	return s
		.split("\0")
		.filter((part) => part.length > 0)
		.join(" ");
}

/* ---------- 纯函数：/proc/<pid>/stat 解析 ---------- */

/**
 * stat 行解析。comm 可含空格/内层括号（如 "(next-server (v1.2))"）——
 * 必须取最后一个 ")" 之后的字段段（B-08 两行锚定）。
 * 字段序（")" 之后 0 起算）：0=state 2=pgrp 4=tty_nr 19=starttime。
 * 解析失败 → null（调用方跳过）。
 */
export function parseStatLine(line: string): ProcStatFields | null {
	if (typeof line !== "string") return null;
	const close = line.lastIndexOf(")");
	const open = line.indexOf("(");
	if (close < 0 || open < 0 || close <= open) return null;
	const pid = Number(line.slice(0, open).trim());
	if (!Number.isInteger(pid) || pid <= 0) return null;
	const rest = line.slice(close + 1).trim().split(/\s+/);
	if (rest.length < 20) return null;
	const pgrp = Number(rest[2]);
	const ttyNr = Number(rest[4]);
	const starttimeTicks = Number(rest[19]);
	if (
		!Number.isInteger(pgrp) ||
		!Number.isInteger(ttyNr) ||
		!Number.isInteger(starttimeTicks)
	) {
		return null;
	}
	return { pid, state: rest[0], pgrp, ttyNr, starttimeTicks };
}

/**
 * starttime（clock ticks）→ epoch 秒：btime + ticks / clkTck。
 * btime 为 null（/proc/stat 无 btime 行，B-05）→ NaN（NaN 落不进任何闭区间 → 保守不杀）。
 * clkTck 非法（≤0）回退 100。
 */
export function computeStarttimeSec(
	btime: number | null,
	ticks: number,
	clkTck: number,
): number {
	if (btime === null || !Number.isFinite(btime)) return Number.NaN;
	const tck = Number.isFinite(clkTck) && clkTck > 0 ? clkTck : 100;
	return btime + ticks / tck;
}

/* ---------- 纯函数：cwd 判定 ---------- */

/** 尾斜杠规范化（保留根 "/"） */
function normalizeDirPath(p: string): string {
	const s = String(p ?? "").replace(/\/+$/, "");
	return s === "" ? "/" : s;
}

/**
 * cwd 是否在仓库根之下（含根本身，D-15）。
 * 前缀判定必须带路径分隔符边界（repoRoot + "/"），禁裸 startsWith（D-16
 * "/repo-mirror" 不得误判为 "/repo" 之内）。
 */
export function isInsideRepo(cwd: string, repoRoot: string): boolean {
	const c = normalizeDirPath(cwd);
	const r = normalizeDirPath(repoRoot);
	if (c === "/" || r === "/") return c === r;
	return c === r || c.startsWith(r + "/");
}

/* ---------- 纯函数：泄漏判定 ---------- */

/**
 * 五条件合取 → { kills, leaks }（输出契约见文件头）。
 * 条件⑤按 **PGID** 比对（B-03：pid==ownPid 不豁免，pgid==ownPgid 才豁免）。
 */
export function decide(procs: ProcSnapshot[], env: DecideEnv): DecideResult {
	const leaks = procs.filter(
		(p) =>
			isDevCmdline(p.cmdline) &&
			isInsideRepo(p.cwd, env.repoRoot) &&
			p.ttyNr === 0 &&
			!env.pidfilePgids.has(p.pgid) &&
			p.pgid !== env.ownPgid,
	);
	const w = env.window;
	const kills =
		w === undefined
			? []
			: leaks.filter(
					(p) =>
						p.pgid > 1 && p.starttimeSec >= w.start && p.starttimeSec <= env.now,
				);
	return { kills, leaks };
}

/* ---------- 纯函数：pidfile 白名单解析 ---------- */

/**
 * pidfile 单文件内容 → PGID；损坏（非纯数字/多行/空/溢出）→ null（B-09 fail-safe）。
 * "654321\n"（start-dev.sh echo 写入带换行）→ 654321；"abc\n"/""/"123\n456\n" → null。
 */
export function parsePidfileContent(content: string): number | null {
	const t = (content ?? "").trim();
	if (!/^[0-9]+$/.test(t)) return null;
	const v = Number(t);
	return Number.isSafeInteger(v) && v > 0 ? v : null;
}

/* ---------- IO 默认实现（真实 /proc；扩展层依赖注入缝可整体替换） ---------- */

/** /proc/<pid>/stat 读取；ENOENT（进程已退出）等 → null，调用方跳过 */
export function readStatDefault(pid: number): string | null {
	try {
		return readFileSync(`/proc/${pid}/stat`, "utf8");
	} catch {
		return null;
	}
}

/** /proc/stat 的 btime（epoch 秒）；无 btime 行/不可读 → null（无法归因保守不杀，B-05） */
export function readBtimeDefault(): number | null {
	try {
		for (const line of readFileSync("/proc/stat", "utf8").split("\n")) {
			const m = /^btime\s+(\d+)\s*$/.exec(line);
			if (m) return Number(m[1]);
		}
	} catch {
		/* fallthrough：读不到 → null */
	}
	return null;
}

let cachedClkTck: number | null = null;

/** CLK_TCK（getconf，模块级记忆化）；失败回退 100（D2） */
export function getClkTckDefault(): number {
	if (cachedClkTck === null) {
		try {
			const v = Number(
				execSync("getconf CLK_TCK", { encoding: "utf8", timeout: 3000 }).trim(),
			);
			cachedClkTck = Number.isInteger(v) && v > 0 ? v : 100;
		} catch {
			cachedClkTck = 100;
		}
	}
	return cachedClkTck;
}

/** cmdline join 后整串；不可读 → null；僵尸/内核线程 → ""（isDevCmdline 自然排除，B-14） */
export function readCmdlineDefault(pid: number): string | null {
	try {
		return joinCmdline(readFileSync(`/proc/${pid}/cmdline`));
	} catch {
		return null;
	}
}

/** cwd readlink；EACCES（他人进程）/ENOENT → null（条件②无法证实 → 调用方保守排除，B-13） */
export function readCwdDefault(pid: number): string | null {
	try {
		return readlinkSync(`/proc/${pid}/cwd`);
	} catch {
		return null;
	}
}

/** /proc 数字项列表（进程 pid 字符串）；不可读 → 空数组 */
export function listProcDirDefault(): string[] {
	try {
		return readdirSync("/proc").filter((n) => /^\d+$/.test(n));
	} catch {
		return [];
	}
}

/**
 * .pi/run/{backend,front}.pgid 白名单（D5 pidfile 契约：start-dev.sh 起的服务 =
 * 合法长驻）。读不到/损坏 → 不贡献成员（fail-open，绝不把 pidfile 服务误判为泄漏）。
 * 死组白名单值无副作用（B-10：白名单只是排除名单，命中不了就无影响）。
 */
export function readPidfilesDefault(repoRoot: string): Set<number> {
	const out = new Set<number>();
	for (const name of ["backend.pgid", "front.pgid"]) {
		try {
			const v = parsePidfileContent(
				readFileSync(join(repoRoot, ".pi", "run", name), "utf8"),
			);
			if (v !== null) out.add(v);
		} catch {
			/* 读不到 → 跳过 */
		}
	}
	return out;
}

/** 组信号 kill(-pgid, sig)；EPERM 等异常由调用方按单目标 fail-open 处理（I-06d 不重试） */
export function killGroupDefault(pgid: number, sig: "TERM" | "KILL"): void {
	process.kill(-pgid, sig === "TERM" ? "SIGTERM" : "SIGKILL");
}

/** 组存活探测（kill(-pgid, 0)）；ESRCH → 死；EPERM → 存在但无权限 → 视为活 */
export function groupAliveDefault(pgid: number): boolean {
	try {
		process.kill(-pgid, 0);
		return true;
	} catch (e) {
		return (e as NodeJS.ErrnoException)?.code !== "ESRCH";
	}
}

/** /proc 文件系统可用性探测（平台边界 D7：不可用 → 扩展整体 no-op）。走真实 fs，不经注入缝 */
export function probeProcFsDefault(): boolean {
	try {
		return readFileSync("/proc/self/stat", "utf8").length > 0;
	} catch {
		return false;
	}
}
