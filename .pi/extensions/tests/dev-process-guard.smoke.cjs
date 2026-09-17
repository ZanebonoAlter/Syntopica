// dev-process-guard smoke（change: dev-process-guard）：纯函数层（.dps.cjs：D/P/N/K/B 纯函数
// 部分）+ handler 层（.dpg.cjs 经 createDevProcessGuard 工厂注入桩回放 I 节与 B 节 handler 部分）
// + B-12 真实进程版（setsid bash trap "" TERM，Linux 本机真跑）。
// 用例契约：openspec/changes/dev-process-guard/test-cases.md（本冒烟覆盖 §1/§2/§3/§5 共 72 条；
// §4 SD 系列归 scripts/start-dev.smoke.sh，change 总数 82）。断言失败 exit 1。
const fs = require('fs');
const os = require('os');
const path = require('path');
const { spawn } = require('child_process');

const checks = [];
const check = (name, ok) => checks.push([name, ok]);
const loadFresh = (p) => {
	delete require.cache[require.resolve(p)];
	return require(p);
};
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// 全部场景的 kill 注入器记录（B-02 公共断言：贯穿所有场景组信号 pgid ≤ 1 出现 0 次）
const scenarioKillLogs = [];

/* ================= 纯函数层（.dps.cjs） ================= */
const scan = loadFresh('./.dps.cjs');
const { decide, isDevCmdline, joinCmdline, parseStatLine, computeStarttimeSec, parsePidfileContent } = scan;

const REPO = '/home/zanebono/software/Syntopica';
const P0 = { pid: 4242, pgid: 4242, cmdline: 'go run cmd/server/main.go', cwd: `${REPO}/backend-go`, ttyNr: 0, starttimeSec: 1010 };
const ENV = { repoRoot: REPO, window: { start: 1000 }, pidfilePgids: new Set([5555, 6666]), ownPgid: 7777, now: 2000 };
const pids = (arr) => arr.map((p) => p.pid).sort((a, b) => a - b);
const eqPids = (arr, want) => JSON.stringify(pids(arr)) === JSON.stringify([...want].sort((a, b) => a - b));
const runDecide = (procs, patch) => decide(procs, { ...ENV, ...(patch ?? {}) });

/* ---------- §1 D 节：decide 分支表（D-01～D-18） ---------- */
const dCases = [
	['D-01', [P0], {}, [4242], [4242]],
	['D-02', [{ ...P0, cmdline: 'pnpm build' }], {}, [], []],
	['D-03', [{ ...P0, cwd: '/tmp/other' }], {}, [], []],
	['D-04', [{ ...P0, ttyNr: 34817 }], {}, [], []],
	['D-05', [{ ...P0, pgid: 5555 }], {}, [], []],
	['D-06', [{ ...P0, pgid: 7777 }], {}, [], []],
	['D-07', [{ ...P0, starttimeSec: 999 }], {}, [], [4242]],
	['D-08', [{ ...P0, starttimeSec: 1000 }], {}, [4242], [4242]],
	['D-09', [{ ...P0, starttimeSec: 2000 }], {}, [4242], [4242]],
	['D-10', [{ ...P0, starttimeSec: 2001 }], {}, [], [4242]],
	['D-11', [P0], { window: undefined }, [], [4242]],
	['D-12', [
		{ ...P0, pid: 4200, pgid: 5555 },
		P0,
		{ ...P0, pid: 4300, pgid: 4300, starttimeSec: 999 },
		{ ...P0, pid: 5170, pgid: 5170, cmdline: 'pnpm test:unit' },
	], {}, [4242], [4242, 4300]],
	['D-13', [], {}, [], []],
	['D-14', [{ ...P0, pgid: 1 }], {}, [], [4242]],
	['D-15', [{ ...P0, cwd: REPO }], {}, [4242], [4242]],
	['D-16', [{ ...P0, cwd: `${REPO}-mirror` }], {}, [], []],
	['D-17', [P0, { ...P0, pid: 4243, cmdline: '/home/z/.cache/go-build/36/3609f1e2…-d/main' }], {}, [4242, 4243], [4242, 4243]],
];
for (const [id, procs, patch, wantKills, wantLeaks] of dCases) {
	const r = runDecide(procs, patch);
	check(`${id} decide 分支`, eqPids(r.kills, wantKills) && eqPids(r.leaks, wantLeaks));
}
{
	const rA = runDecide([P0]);
	const rB = runDecide([P0]);
	check('D-18 同输入连调两次 deep-equal（无隐藏随机性）', JSON.stringify(rA) === JSON.stringify(rB) && eqPids(rA.kills, [4242]));
}

/* ---------- §2 P/N/K 节：八模式正反例（P-01～09、N-01～16、K-01～03） ---------- */
const posCases = [
	['P-01', 'go run cmd/server/main.go'],
	['P-02', '/home/z/.cache/go-build/36/3609f1e2…-d/main'],
	['P-03', 'sh -c nuxt dev --host'],
	['P-04', 'node /home/z/software/Syntopica/front/node_modules/nuxt/bin/nuxt.mjs dev --host'],
	['P-05', 'node /home/z/software/Syntopica/front/node_modules/.pnpm/@nuxt+cli@3.32.0/node_modules/nuxi/dist/pages/dev/index.mjs --host'],
	['P-06', '/home/z/software/Syntopica/front/node_modules/.pnpm/@pnpm+exe@10.15.0/node_modules/pnpm/bin/pnpm dev --host'],
	['P-07', 'go run -tags=embed cmd/server/main.go'],
	['P-08', '/home/z/.nvm/versions/node/v26.8.2/lib/node_modules/agent-browser/bin/agent-browser-linux-arm64'],
	['P-09', '/usr/lib/chromium/chromium --headless=new --remote-debugging-port=0 --user-data-dir=/tmp/agent-browser-chrome-03421e1a-5ce2-4303-9dd2-146decaa6db3 --window-size=1280,720'],
];
for (const [id, cmd] of posCases) check(`${id} 正例命中八模式`, isDevCmdline(cmd) === true);

const negCases = [
	['N-01', 'pnpm generate'],
	['N-02', 'pnpm test:unit'],
	['N-03', 'pnpm build'],
	['N-04', 'go build ./...'],
	['N-05', 'next-server (v16.3.1)'],
	['N-06', 'chromium --type=renderer --field-trial-handle=1,x'],
	['N-07', 'go run ./cmd/other/main.go'],
	['N-08', 'npm run dev'],
	['N-09', 'node …/nuxt/bin/nuxt.mjs build'],
	['N-10', 'node …/nuxt/bin/nuxt.mjs generate'],
	['N-11', 'bash -c nuxt dev --host'],
	['N-12', 'go vet ./cmd/server/main.go'],
	['N-13', 'sh -c nuxt build'],
	['N-14', '/home/z/.cache/go-build/36/3609f1e2…-d/main --port 5100'],
	['N-15', '/usr/lib/chromium/chromium --headless=new --window-size=1280,720'],
	['N-16', '/usr/lib/chromium/chrome_crashpad_handler --monitor-self --database=/home/z/.config/chromium/Crash Reports …'],
];
for (const [id, cmd] of negCases) check(`${id} 反例不命中`, isDevCmdline(cmd) === false);

for (const [id, cmd] of [
	['K-01', 'pnpm dev:ssg'],
	['K-02', 'pnpm dev-remote'],
	['K-03', 'grep -rn nuxt.mjs dev front/app'],
]) check(`${id} 邻界命中（回归锚点，禁静漂移）`, isDevCmdline(cmd) === true);

/* ---------- §3 B 节纯函数部分 ---------- */
{
	const at = (st) => runDecide([{ ...P0, starttimeSec: st }]);
	check('B-01 窗口边界三连＋时钟跳变（闭区间）',
		eqPids(at(1000).kills, [4242]) && eqPids(at(2000).kills, [4242]) &&
		eqPids(at(999).kills, []) && eqPids(at(999).leaks, [4242]) &&
		eqPids(at(2001).kills, []) && eqPids(at(2001).leaks, [4242]));
}
{
	const rPid = runDecide([{ ...P0, pid: 7777 }]);
	const rPgid = runDecide([{ ...P0, pgid: 7777 }]);
	check('B-03 条件⑤按 PGID 判（pid==ownPid 仍杀，pgid==ownPgid 才豁免）',
		eqPids(rPid.kills, [7777]) && rPgid.kills.length === 0);
}
check('B-06 computeStarttimeSec 用注入 CLK_TCK（非硬编码 100）',
	computeStarttimeSec(1000, 1280, 128) === 1010 &&
	computeStarttimeSec(1000, 1280, 100) === 1012.8 &&
	computeStarttimeSec(1000, 1280, 128) !== computeStarttimeSec(1000, 1280, 100));
{
	const joined = joinCmdline('sh\0-c\0nuxt dev --host\0');
	check('B-07 cmdline NUL→空格完整 join 后命中 m3（禁按 NUL 截断）',
		joined === 'sh -c nuxt dev --host' && isDevCmdline(joined) === true);
}
{
	const l1 = '4242 (next-server (v1.2)) S 1 4242 4242 34817 -1 4146 100 0 2 0 5 3 0 0 80 0 12 0 7654321 123456 4567';
	const l2 = '9000 (sh -c nuxt dev) S 8999 9001 9001 0 -1 4146 10 0 0 0 1 1 0 0 80 0 1 0 100 4096 100';
	const p1 = parseStatLine(l1);
	const p2 = parseStatLine(l2);
	check('B-08 parseStatLine（comm 含空格/内层括号取最后一个 ")"）',
		p1 && p1.pid === 4242 && p1.pgrp === 4242 && p1.ttyNr === 34817 && p1.starttimeTicks === 7654321 && p1.state === 'S' &&
		p2 && p2.pid === 9000 && p2.pgrp === 9001 && p2.ttyNr === 0 && p2.starttimeTicks === 100);
}
{
	let threw = false;
	let a; let b; let c;
	try {
		a = parsePidfileContent('abc\n');
		b = parsePidfileContent('');
		c = parsePidfileContent('123\n456\n');
	} catch {
		threw = true;
	}
	const r = runDecide([{ ...P0, pgid: 123 }], { pidfilePgids: new Set() });
	check('B-09 损坏 pidfile（非数字/空/多行）不贡献白名单、不抛异常',
		!threw && a === null && b === null && c === null && eqPids(r.kills, [4242]));
}
{
	const r = runDecide([P0], { pidfilePgids: new Set([999999]) });
	check('B-10 死组白名单值无副作用', eqPids(r.kills, [4242]));
}

/* ================= handler 层（.dpg.cjs 工厂注入桩） ================= */
// 合成 stat 行：")" 之后 0 起算 0=state 2=pgrp 4=tty_nr 19=starttime（13 个占位 0 补齐 6..18）
function statLine(pid, pgid, ttyNr, ticks) {
	const zeros = Array(13).fill(0).join(' ');
	return `${pid} (fake-proc) S 1 ${pgid} ${pgid} ${ttyNr} -1 ${zeros} ${ticks} 4096 100`;
}
// 泄漏形状快照（默认窗口内：btime=0、clk=100 → starttimeSec = ticks/100）
const leak = (pid, pgid, o = {}) => ({
	pid, pgid, cmdline: 'go run cmd/server/main.go', starttimeSec: 1010, ...o,
});

async function scenario(opts = {}) {
	const clk = opts.clk ?? 100;
	const btime = opts.btime ?? 0;
	const table = new Map((opts.table ?? []).map((p) => [p.pid, { ttyNr: 0, cwd: `${REPO}/backend-go`, ...p }]));
	const dead = new Set(); // killGroup 标记死组（模拟 /proc 条目消失，I-08 二次扫描口径）
	const killCalls = [];
	const logCalls = [];
	const warnCalls = [];
	const steerCalls = [];
	const statThrow = new Set(opts.statThrow ?? []);
	const statNull = new Set(opts.statNull ?? []);
	const cwdThrow = new Set(opts.cwdThrow ?? []);
	const killThrow = new Set(opts.killThrow ?? []);
	const nowState = { v: opts.now ?? 2000 };
	const deps = {
		listProcDir: () => [...table.keys()].map(String),
		readStat: (pid) => {
			if (statThrow.has(pid)) throw Object.assign(new Error('stat EACCES'), { code: 'EACCES' });
			if (statNull.has(pid)) return null;
			const p = table.get(pid);
			if (!p || dead.has(p.pgid)) return null;
			return statLine(pid, p.pgid, p.ttyNr, Math.round((p.starttimeSec - btime) * clk));
		},
		readBtime: () => (opts.noBtime ? null : btime),
		getClkTck: () => clk,
		readCmdline: (pid) => {
			const p = table.get(pid);
			if (!p || dead.has(p.pgid)) return null;
			return p.cmdline;
		},
		readCwd: (pid) => {
			if (cwdThrow.has(pid)) throw Object.assign(new Error('readlink EACCES'), { code: 'EACCES' });
			const p = table.get(pid);
			if (!p || dead.has(p.pgid)) return null;
			return p.cwd;
		},
		readPidfiles: () => new Set(opts.pidfilePgids ?? []),
		killGroup: (pgid, sig) => {
			killCalls.push({ pgid, sig, ts: Date.now() });
			if (killThrow.has(pgid)) throw Object.assign(new Error('kill EPERM'), { code: 'EPERM' });
			dead.add(pgid);
		},
		groupAlive: opts.alivePred ?? ((pgid) => !dead.has(pgid)), // 默认「TERM 后即死」
		now: () => nowState.v,
		platform: opts.platform ?? 'linux',
		logEvent: (cwd, ev) => {
			if (opts.logThrow) throw new Error('events.db write failed');
			logCalls.push({ cwd, ...ev });
		},
		consoleWarn: (m) => warnCalls.push(m),
	};
	scenarioKillLogs.push(killCalls);
	const mod = loadFresh('./.dpg.cjs');
	const handlers = {};
	const pi = {
		on: (n, f) => { handlers[n] = f; },
		sendMessage: (msg, options) => steerCalls.push({ msg, options }),
	};
	mod.createDevProcessGuard(pi, deps);
	const ctx = (sid) => ({ cwd: opts.cwd ?? REPO, sessionManager: { getSessionId: () => sid } });
	const at = (v) => { if (typeof v === 'number') nowState.v = v; };
	const start = async (sid, nowAt) => { at(nowAt); await handlers['session_start']({ reason: 'new' }, ctx(sid)); };
	const shutdown = async (sid, nowAt) => { at(nowAt); await handlers['session_shutdown']({}, ctx(sid)); };
	const turnEnd = async (sid, nowAt) => { at(nowAt); await handlers['turn_end']({}, ctx(sid)); };
	return { handlers, killCalls, logCalls, warnCalls, steerCalls, start, shutdown, turnEnd };
}

const duoTable = [
	{ pid: 4242, pgid: 4242, cmdline: 'go run cmd/server/main.go', starttimeSec: 1010 },
	{ pid: 4243, pgid: 4242, cmdline: '/home/z/.cache/go-build/36/3609f1e2…-d/main', starttimeSec: 1010 },
];

(async () => {
	const tmps = [];
	try {
		/* ---------- §5 I 节：集成场景 ---------- */
		{
			const s = await scenario({ table: duoTable });
			await s.start('s1', 1000);
			await s.shutdown('s1', 2000);
			const ev = s.logCalls[0];
			check('I-01 同组双进程按组去重（1 轮 TERM）＋orphan-killed 事件（procs 双项）',
				s.killCalls.length === 1 && s.killCalls[0].pgid === 4242 && s.killCalls[0].sig === 'TERM' &&
				s.logCalls.length === 1 && ev.kind === 'policy.decision' && ev.payload.decision === 'orphan-killed' &&
				ev.payload.procs.length === 2 &&
				[4242, 4243].every((pid) => ev.payload.procs.some((p) => p.pid === pid)) &&
				ev.payload.procs.every((p) => p.pgid === 4242 && typeof p.cmdline === 'string' && p.cmdline.length > 0));
		}
		{
			const s = await scenario({ table: duoTable.map((p) => ({ ...p, starttimeSec: 999 })) });
			let threw = false;
			try { await s.start('s1', 1000); await s.shutdown('s1', 2000); } catch { threw = true; }
			check('I-02 窗口外遗留：零杀零事件不抛异常', !threw && s.killCalls.length === 0 && s.logCalls.length === 0);
		}
		{
			const s = await scenario({ table: [{ pid: 4242, pgid: 4343, cmdline: 'go run cmd/server/main.go', starttimeSec: 999 }] });
			await s.turnEnd('s1', 2000);
			const st = s.steerCalls[0];
			const text = st ? String(st.msg.content) : '';
			check('I-03 turn_end 软提醒（pid/pgid/年龄\\d+[hms]/cmdline 子串/start-dev.sh）',
				s.steerCalls.length === 1 && !!st.options && st.options.deliverAs === 'steer' &&
				text.includes('4242') && text.includes('4343') && text.includes('go run') &&
				/\d+[hms]/.test(text) && text.includes('start-dev.sh'));
			check('I-03 补充：orphan-warn 事件落库（spec 要求，test-cases 未断言）',
				s.logCalls.length === 1 && s.logCalls[0].kind === 'policy.decision' &&
				s.logCalls[0].payload.decision === 'orphan-warn' &&
				s.logCalls[0].payload.offenders.some((p) => p.pid === 4242));
			await s.turnEnd('s1', 2000);
			check('I-04 二次 turn_end 静默（每会话至多一次）', s.steerCalls.length === 1 && s.logCalls.length === 1);
		}
		{
			const s = await scenario({ table: [] });
			await s.turnEnd('s1', 2000);
			check('I-05 零泄漏零输出零记录', s.steerCalls.length === 0 && s.logCalls.length === 0 && s.killCalls.length === 0);
		}
		{
			const s = await scenario({ table: [leak(4242, 4242), leak(5151, 5151)], statThrow: [4242] });
			await s.start('s1', 1000);
			await s.shutdown('s1', 2000);
			const ev = s.logCalls[0];
			check('I-06a readStat 抛 EACCES：该目标跳过，余者照杀进事件',
				!!ev && ev.payload.procs.length === 1 && ev.payload.procs[0].pid === 5151 &&
				s.killCalls.length === 1 && s.killCalls[0].pgid === 5151);
		}
		{
			const s = await scenario({ table: [leak(4242, 4242)], logThrow: true });
			let threw = false;
			try { await s.start('s1', 1000); await s.shutdown('s1', 2000); } catch { threw = true; }
			check('I-06b logEvent 抛错：记账失败不阻断清理', !threw && s.killCalls.length === 1 && s.killCalls[0].sig === 'TERM');
		}
		{
			const s = await scenario({ table: [leak(4241, 4241), leak(4242, 4242)], statNull: [4241] });
			await s.start('s1', 1000);
			await s.shutdown('s1', 2000);
			check('I-06c readStat 返回 null（扫描中途退出）：跳过该目标',
				s.killCalls.length === 1 && s.killCalls[0].pgid === 4242 &&
				s.logCalls[0].payload.procs.every((p) => p.pid === 4242));
		}
		{
			const s = await scenario({ table: [leak(4242, 4242), leak(5151, 5151)], killThrow: [4242] });
			let threw = false;
			try { await s.start('s1', 1000); await s.shutdown('s1', 2000); } catch { threw = true; }
			const c42 = s.killCalls.filter((k) => k.pgid === 4242);
			check('I-06d killGroup EPERM：无重试风暴（同组 ≤2 次），他组照常 TERM',
				!threw && c42.length <= 2 && s.killCalls.some((k) => k.pgid === 5151 && k.sig === 'TERM'));
		}
		{
			const s = await scenario({ table: [leak(4242, 4242)], platform: 'darwin' });
			await s.start('s1', 1000);
			await s.turnEnd('s1', 2000);
			await s.shutdown('s1', 2000);
			check('I-07 非 Linux：全程恰 warn 一次，整体 no-op',
				s.warnCalls.length === 1 && s.killCalls.length === 0 && s.logCalls.length === 0 && s.steerCalls.length === 0);
		}
		{
			const s = await scenario({ table: [leak(4242, 4242, { starttimeSec: 1100 }), leak(5151, 5151, { starttimeSec: 1600 })] });
			await s.start('s1', 1000);
			await s.start('s2', 1500);
			await s.shutdown('s2', 2000);
			const firstOk = s.killCalls.length === 1 && s.killCalls[0].pgid === 5151 &&
				s.logCalls.length === 1 && s.logCalls[0].payload.decision === 'orphan-killed' &&
				s.logCalls[0].payload.procs.every((p) => p.pid === 5151);
			await s.shutdown('s1', 2000);
			const secondOk = s.killCalls.length === 2 && s.killCalls[1].pgid === 4242 &&
				s.logCalls.length === 2 && s.logCalls[1].payload.procs.every((p) => p.pid === 4242);
			check('I-08 双会话窗口隔离（s2 只杀自己窗口内，不碰 s1 的）', firstOk && secondOk);
		}
		{
			const s = await scenario({ table: [leak(4242, 4242)] });
			let threw = false;
			try { await s.shutdown('s1', 2000); } catch { threw = true; }
			check('I-09 无窗口记录（热加载后直接 shutdown）：保守不杀零事件',
				!threw && s.killCalls.length === 0 && s.logCalls.length === 0);
		}

		/* ---------- §3 B 节 handler 部分 ---------- */
		{
			const s = await scenario({ table: [leak(4242, 1)] });
			await s.start('s1', 1000);
			await s.shutdown('s1', 2000);
			check('B-02 pgid≤1 组信号红线：零信号零杀零事件', s.killCalls.length === 0 && s.logCalls.length === 0);
		}
		{
			const s = await scenario({ table: [leak(4301, 4301), leak(4302, 4302)], statNull: [4301] });
			let threw = false;
			try { await s.start('s1', 1000); await s.shutdown('s1', 2000); } catch { threw = true; }
			check('B-04 readStat null（ENOENT）：跳过退出进程，余者照杀不抛异常',
				!threw && s.killCalls.length === 1 && s.killCalls[0].pgid === 4302 &&
				s.logCalls[0].payload.procs.length === 1 && s.logCalls[0].payload.procs[0].pid === 4302);
		}
		{
			const s = await scenario({ table: [leak(4242, 4242)], noBtime: true });
			let threw = false;
			try { await s.start('s1', 1000); await s.shutdown('s1', 2000); await s.turnEnd('s1', 2000); } catch { threw = true; }
			check('B-05 无 btime：无法归因保守不杀，leaks/turn_end 提醒照常',
				!threw && s.killCalls.length === 0 &&
				s.logCalls.filter((l) => l.payload.decision === 'orphan-killed').length === 0 &&
				s.steerCalls.length === 1);
		}
		{
			const s = await scenario({ table: [leak(4242, 4242)] }); // 默认存活谓词「TERM 后即死」
			await s.start('s1', 1000);
			await s.shutdown('s1', 2000);
			check('B-11 TERM 即死：序列恰 [TERM] 无 KILL，事件照常',
				JSON.stringify(s.killCalls.map((k) => ({ pgid: k.pgid, sig: k.sig }))) === JSON.stringify([{ pgid: 4242, sig: 'TERM' }]) &&
				s.logCalls.length === 1 && s.logCalls[0].payload.decision === 'orphan-killed' &&
				s.logCalls[0].payload.procs.some((p) => p.pid === 4242));
		}
		{
			const s = await scenario({ table: [leak(4242, 4242)], alivePred: () => true });
			await s.start('s1', 1000);
			await s.shutdown('s1', 2000);
			const [t, k] = s.killCalls;
			check('B-12 桩版 TERM→轮询 2s→KILL（同 pgid，时差 ∈ [1.8s, 2.6s]）',
				s.killCalls.length === 2 && t.sig === 'TERM' && k.sig === 'KILL' &&
				t.pgid === k.pgid && k.pgid === 4242 && k.ts - t.ts >= 1800 && k.ts - t.ts <= 2600);
		}
		{
			const s = await scenario({ table: [leak(4301, 4301), leak(4302, 4302)], cwdThrow: [4301] });
			let threw = false;
			try { await s.start('s1', 1000); await s.shutdown('s1', 2000); } catch { threw = true; }
			check('B-13 readCwd 抛 EACCES：条件②无法证实保守排除，余者照杀',
				!threw && s.killCalls.length === 1 && s.killCalls[0].pgid === 4302 &&
				s.logCalls[0].payload.procs.every((p) => p.pid === 4302));
		}
		{
			const s = await scenario({ table: [{ pid: 4242, pgid: 4242, cmdline: '', starttimeSec: 1010 }] });
			await s.start('s1', 1000);
			await s.shutdown('s1', 2000);
			check('B-14 僵尸空 cmdline：isDevCmdline("")==false 被排除，零杀零事件',
				isDevCmdline('') === false && s.killCalls.length === 0 && s.logCalls.length === 0);
		}

		/* ---------- 补充断言（非编号用例） ---------- */
		{
			// DEV_GUARD_ENABLE 模块加载时常量：逐值子进程外还要在本进程验证静默 no-op
			let allOff = true;
			for (const v of ['0', 'off', 'false']) {
				process.env.DEV_GUARD_ENABLE = v;
				const s = await scenario({ table: [leak(4242, 4242)] }); // loadFresh 在 env 设置后
				await s.start('s1', 1000);
				await s.shutdown('s1', 2000);
				await s.turnEnd('s1', 2000);
				if (s.warnCalls.length !== 0 || s.killCalls.length !== 0 || s.logCalls.length !== 0 || s.steerCalls.length !== 0) allOff = false;
			}
			delete process.env.DEV_GUARD_ENABLE;
			check('配置项 DEV_GUARD_ENABLE=0/off/false 关闭（静默 no-op）', allOff);
		}
		{
			const mod = loadFresh('./.dpg.cjs');
			const handlers = {};
			mod.default({ on: (n, f) => { handlers[n] = f; }, sendMessage() {} });
			check('默认入口注册三钩子（session_start/shutdown/turn_end）',
				typeof handlers['session_start'] === 'function' &&
				typeof handlers['session_shutdown'] === 'function' &&
				typeof handlers['turn_end'] === 'function');
		}
		check('B-02 公共断言（贯穿全部场景）：kill 注入器 pgid≤1 出现 0 次',
			scenarioKillLogs.every((calls) => calls.every((k) => k.pgid > 1)));

		/* ---------- B-12 真实进程版（Linux 本机真跑，约 2.5s） ---------- */
		const b12Real = async () => {
			const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'dpg-real-'));
			tmps.push(tmp);
			const scriptDir = path.join(tmp, '.cache', 'go-build', 'smoke');
			fs.mkdirSync(scriptDir, { recursive: true });
			const script = path.join(scriptDir, 'main'); // 路径命中 m2（$ 锚定串尾）
			fs.writeFileSync(script, '#!/bin/bash\ntrap "" TERM\nwhile :; do sleep 0.5; done\n');
			fs.chmodSync(script, 0o755);
			// 真实默认 deps 驱动；repoRoot 用临时目录（只让本测试进程命中五条件，不误伤真实仓库服务）
			const mod = loadFresh('./.dpg.cjs');
			const handlers = {};
			const pi = { on: (n, f) => { handlers[n] = f; }, sendMessage() {} };
			mod.createDevProcessGuard(pi); // deps 缺省 = 真实实现
			const ctx = { cwd: tmp, sessionManager: { getSessionId: () => 's-real' } };
			await handlers['session_start']({ reason: 'new' }, ctx); // 先记窗口
			// 垫 1.3s：/proc/stat 的 btime 是整数秒（向下截断），算出的 starttimeSec 会比真实
			// 启动时刻低估 0~1s（产品语义上只可能漏杀不可能误杀，保守方向）；不垫的话
			// session_start 落在秒相位前段时 child 会被算到窗口外，测试变闪
			await sleep(1300);
			const child = spawn('setsid', ['bash', script], { stdio: 'ignore', cwd: tmp });
			const pgid = child.pid; // setsid 语义：PID == PGID
			const exited = new Promise((res) => child.on('exit', () => res(true)));
			let ready = false;
			for (let i = 0; i < 60 && !ready; i++) {
				await sleep(50);
				try {
					const stat = fs.readFileSync(`/proc/${pgid}/stat`, 'utf8');
					const after = stat.slice(stat.lastIndexOf(')') + 1).trim().split(/\s+/);
					const cmdline = fs.readFileSync(`/proc/${pgid}/cmdline`, 'utf8').split('\0').filter(Boolean).join(' ');
					ready = Number(after[2]) === pgid && cmdline.includes('.cache/go-build');
				} catch { /* not yet */ }
			}
			if (!ready) {
				try { process.kill(-pgid, 'SIGKILL'); } catch { /* already gone */ }
				return false;
			}
			await handlers['session_shutdown']({}, ctx); // TERM 被忽略 → ~2s → KILL
			const gone = await Promise.race([exited, sleep(5000).then(() => false)]);
			await sleep(400); // 等组内 sleep 子进程收尸
			let groupGone = false;
			try { process.kill(-pgid, 0); } catch (e) { groupGone = e.code === 'ESRCH'; }
			try { process.kill(-pgid, 'SIGKILL'); } catch { /* 兜底清理，已死则忽略 */ }
			return !!gone && groupGone;
		};
		check('B-12 真实进程版（setsid bash trap "" TERM → KILL 清组）', await b12Real());

		/* ---------- 汇总 ---------- */
		let fail = 0;
		for (const [name, ok] of checks) {
			console.log(`${ok ? '✅' : '❌'} ${name}`);
			if (!ok) fail++;
		}
		const total = checks.length;
		console.log(fail ? `\n${fail}/${total} 项失败` : `\nSMOKE OK（${total} 项断言全绿）`);
		if (fail) process.exitCode = 1;
	} catch (e) {
		console.error('FAIL', e);
		process.exitCode = 1;
	} finally {
		for (const t of tmps) fs.rmSync(t, { recursive: true, force: true });
	}
})();
