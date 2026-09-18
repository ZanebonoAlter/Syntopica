// quality-gate 行为级 smoke（harden-gate-interop-health test-cases 主链路步 1-7）：
// mock pi + ctx 驱动 quality-gate bundle 的 turn_end handler，真 tmp events.db 断言。
// 场景 A 正常态(windows) / B 探测故障短路 / C 兜底 diag 特征(粘性豁免) / D 真实失败粘性 /
// E native 模式正常态 / F native 失败归因与链路标注（linux-native-dev-environment 新增）/
// G native 工具链缺失侧级短路 / H native 工具缺失兑底归因 / I windows 不受工具链探测影响
// （harden-gate-native-toolchain 新增）/ J 子线程 turn_end 降载 bypass（零命令+一条记账）/
// K 主会话不受降载影响（harden-subagent-constraint-channel 新增）/
// L 失败指纹状态机（首现完整块→⟳单行→未修→✓转绿；粘性重跑不变）+ M 会话边界重置指纹 +
// N 启动基线外部归因/[外部]/不进粘性/基线不覆盖 + 混合与解析不出保守回现状（⑥）+
// O edit.map 归属外部/同指纹第 2 编辑回合不重发（C11）+ P 库不可用 fail-open（2.4）
// （attribute-concurrent-gate-noise 新增）。
// 模块级状态（sticky/snapshot/gateOkStates/execPlatform）由各场景前的 session_start 重置。
// 由 run-harness-smoke.sh 调用（先 esbuild 产出 .qgateb.cjs）。断言失败 exit 1。
//
// 平台控制：quality-gate 的 cmdExeReachable() 按 PATH 找 cmd.exe。场景 A-D 将假 cmd.exe
// 所在目录置于 PATH 首位以造出 windows 模式（保护既有 interop 语义不回归）；场景 E/F 将
// PATH 置空以造出 native 模式。cmdExeReachable 是 bundle 内部函数，PATH 是外部可注入的
// 唯一接缝——不为测试在生产代码里开后门。
const fs = require('fs');
const os = require('os');
const path = require('path');
const { DatabaseSync } = require('node:sqlite');

process.removeAllListeners('warning');
process.on('warning', (w) => {
	if (!(w.name === 'ExperimentalWarning' && /sqlite/i.test(w.message))) process.stderr.write(`${w.name}: ${w.message}\n`);
});

const qualityGate = require('./.qgateb.cjs').default;

const checks = [];
const check = (name, ok) => checks.push([name, ok]);

function mkrepo() {
	const repo = fs.mkdtempSync(path.join(os.tmpdir(), 'interop-smoke-'));
	fs.mkdirSync(path.join(repo, 'backend-go'), { recursive: true });
	fs.mkdirSync(path.join(repo, 'front', 'app'), { recursive: true });
	fs.writeFileSync(path.join(repo, 'backend-go', 'a.go'), 'package x\n');
	fs.writeFileSync(path.join(repo, 'backend-go', 'b.go'), 'package x\n');
	fs.writeFileSync(path.join(repo, 'front', 'app', 'x.ts'), 'export {}\n');
	return repo;
}

const mkctx = (sid) => ({ signal: undefined, sessionManager: { getSessionId: () => sid } });
/** 子线程 ctx（父子可证，harden-subagent-constraint-channel）：header parentSession 指向
 *  父会话文件（pi-web/pi-subagents 子会话实测形态；lib/child-session 判定矩阵见
 *  quality-gate.smoke.cjs CS 系列，此处只验 behavior）。cwd 供 bypass 记账用
 *  （constraint-injection 同源：无 git 兑底，直接 ctx.cwd 落 .pi/harness/events.db） */
const childCtx = (sid, cwd) => ({
	signal: undefined,
	cwd,
	sessionManager: {
		getSessionId: () => sid,
		getHeader: () => ({ parentSession: `/home/x/.pi/agent/sessions/dir/2026-09-18T00-00-00-000Z_${sid}-parent.jsonl` }),
	},
});
const EDIT_TURN = { toolResults: [{ toolName: 'edit' }] };
const CHAT_TURN = { toolResults: [{ toolName: 'read' }] };

/** git 三件套 mock：ls-files 按 hasFile 决定是否返回 a.go；rev-parse 返回真 repo 路径 */
const gitRouter = (repoRoot, files) => (cmd, args) => {
	if (cmd === 'git' && args[0] === 'diff') return { code: 0, stdout: '', stderr: '' };
	if (cmd === 'git' && args[0] === 'ls-files') return { code: 0, stdout: files, stderr: '' };
	if (cmd === 'git' && args[0] === 'rev-parse') return { code: 0, stdout: repoRoot, stderr: '' };
	throw new Error(`unexpected git: ${cmd} ${args.join(' ')}`);
};

/**
 * events.db 只读查询。用 Node 内置 node:sqlite（22.5+）而非系统 sqlite3 CLI：
 * 后者在最小化 Linux 装机（含树莓派）默认不存在，会让整个 harness smoke 退化为环境故障。
 */
const q = (repo, sql) => {
	const db = path.join(repo, '.pi', 'harness', 'events.db');
	if (!fs.existsSync(db)) return [];
	const conn = new DatabaseSync(db, { readOnly: true });
	try {
		return conn.prepare(sql).all();
	} finally {
		conn.close();
	}
};

/* 平台控制（见头部说明）：假 cmd.exe + PATH 注入 */
const fakeWinDir = fs.mkdtempSync(path.join(os.tmpdir(), 'fakewin-'));
fs.writeFileSync(path.join(fakeWinDir, 'cmd.exe'), '#!/bin/sh\nexit 0\n', { mode: 0o755 });
// native 全工具链（harden-gate-native-toolchain）：无 cmd.exe，有 go/golangci-lint/pnpm
const fakeNativeDir = fs.mkdtempSync(path.join(os.tmpdir(), 'fakenative-'));
for (const exe of ['go', 'golangci-lint', 'pnpm']) {
	fs.writeFileSync(path.join(fakeNativeDir, exe), '#!/bin/sh\nexit 0\n', { mode: 0o755 });
}
// 仅 pnpm（后端工具链缺失变体：go/golangci-lint 不在）
const fakePnpmOnlyDir = fs.mkdtempSync(path.join(os.tmpdir(), 'fakepnpm-'));
fs.writeFileSync(path.join(fakePnpmOnlyDir, 'pnpm'), '#!/bin/sh\nexit 0\n', { mode: 0o755 });
const REAL_PATH = process.env.PATH ?? '';
/** windows：假 cmd.exe 目录置于 PATH 首位；native：指向假工具链目录（无 cmd.exe）；
 *  native-pnpm-only：仅 pnpm；windows-nogo：cmd.exe 在但 go/golangci-lint/pnpm 不在 */
const setPlatform = (mode) => {
	process.env.PATH =
		mode === 'windows' ? `${fakeWinDir}${path.delimiter}${REAL_PATH}`
		: mode === 'native' ? fakeNativeDir
		: mode === 'native-pnpm-only' ? fakePnpmOnlyDir
		: mode === 'windows-nogo' ? `${fakeWinDir}${path.delimiter}${fakePnpmOnlyDir}`
		: '';
};

/** 注册扩展到 mock pi，返回 { handlers, messages, execCalls, execOpts } */
const setup = (router) => {
	const handlers = {};
	const messages = [];
	const execCalls = [];
	const execOpts = [];
	qualityGate({
		on: (e, h) => { handlers[e] = h; },
		exec: async (cmd, args, opts) => {
			execCalls.push(`${cmd} ${args.join(' ')}`);
			execOpts.push({ cmd, args, opts });
			return router(cmd, args, opts);
		},
		sendMessage: (m, o) => { messages.push({ m, o }); },
	});
	return { handlers, messages, execCalls, execOpts };
};

(async () => {
	// ============ 场景 A：正常态（探测通过 → 既有门禁行为不变） ============
	{
		setPlatform('windows');
		const repo = mkrepo();
		let fileAdded = false;
		const router = async (cmd, args) => {
			if (cmd === 'git') return gitRouter(repo, fileAdded ? 'backend-go/a.go' : '')(cmd, args);
			if (cmd === 'bash') return { code: 0, stdout: '{"testTargets":[]}', stderr: '' };
			if (cmd === 'cmd.exe') {
				const cl = args.join(' ');
				if (cl.includes('echo ok')) return { code: 0, stdout: 'ok', stderr: '' };
				return { code: 0, stdout: '0 issues.', stderr: '' }; // lint/vet/build 全绿
			}
			throw new Error(`unexpected: ${cmd}`);
		};
		const { handlers, messages, execCalls } = setup(router);
		await handlers['session_start']({ reason: 'new' }, mkctx('smoke-a')); // 空基线
		fileAdded = true;
		await handlers['turn_end'](EDIT_TURN, mkctx('smoke-a'));
		check('A1 探测通过后门禁照常执行（gate.check ≥3 条落库）', q(repo, "SELECT COUNT(*) n FROM events WHERE kind='gate.check'")[0].n >= 3);
		check('A2 正常态零 policy.decision（低噪声）', q(repo, "SELECT COUNT(*) n FROM events WHERE kind='policy.decision'")[0].n === 0);
		check('A3 全绿零 steer', messages.length === 0);
		check('A4 探测命令确被执行', execCalls.some((c) => c.includes('echo ok')));
		check('A5 lint 命令带 --allow-parallel-runners（防同机并发锁假回归）', execCalls.some((c) => c.includes('--allow-parallel-runners')));
	}

	// ============ 场景 B：探测故障短路（fail-open + 记账 + 不双写） ============
	{
		setPlatform('windows');
		const repo = mkrepo();
		let fileAdded = false;
		const router = async (cmd, args) => {
			if (cmd === 'git') return gitRouter(repo, fileAdded ? 'backend-go/a.go' : '')(cmd, args);
			if (cmd === 'cmd.exe') throw new Error('Command timed out'); // vsock 死：全部 reject
			throw new Error(`unexpected: ${cmd}`);
		};
		const { handlers, messages, execCalls } = setup(router);
		await handlers['session_start']({ reason: 'new' }, mkctx('smoke-b'));
		fileAdded = true;
		await handlers['turn_end'](EDIT_TURN, mkctx('smoke-b'));
		const gateB = q(repo, "SELECT COUNT(*) n FROM events WHERE kind='gate.check'")[0].n;
		const polB = q(repo, "SELECT payload FROM events WHERE kind='policy.decision'");
		check('B1 短路零 gate.check（被跳过命令不双写）', gateB === 0);
		check('B2 恰一条 policy.decision(quality-gate/fail-open/interop-down)', polB.length === 1
			&& polB[0].payload.includes('"policy":"quality-gate"')
			&& polB[0].payload.includes('"action":"fail-open"')
			&& polB[0].payload.includes('"reasonCode":"interop-down"'));
		check('B3 durationMs 非负入账', polB.length === 1 && typeof JSON.parse(polB[0].payload).durationMs === 'number' && JSON.parse(polB[0].payload).durationMs >= 0);
		check('B4 steer 提示 WSL interop 故障与 wsl --shutdown 建议', messages.some(({ m }) => m.customType === 'quality-gate-interop-down' && m.content.includes('wsl --shutdown') && m.content.includes('非代码问题')));
		check('B5 门禁命令零执行（仅探测一次 cmd.exe）', execCalls.filter((c) => c.startsWith('cmd.exe')).length === 1);
	}

	// ============ 场景 C：兜底 diag 特征（环境归因 + 粘性豁免） ============
	{
		setPlatform('windows');
		const repo = mkrepo();
		let fileAdded = false;
		let turn = 1;
		let cmdCallsTurn2 = 0;
		const VSOCK = '<3>WSL (1751 - ) ERROR: UtilAcceptVsock:251: accept4 failed 110';
		const router = async (cmd, args) => {
			if (cmd === 'git') return gitRouter(repo, fileAdded ? 'backend-go/a.go' : '')(cmd, args);
			if (cmd === 'bash') return { code: 0, stdout: '{"testTargets":[]}', stderr: '' };
			if (cmd === 'cmd.exe') {
				if (turn === 2) cmdCallsTurn2++;
				const cl = args.join(' ');
				if (cl.includes('echo ok')) return { code: 0, stdout: 'ok', stderr: '' };
				return { code: 1, stdout: '', stderr: VSOCK }; // 执行中途 vsock 挂
			}
			throw new Error(`unexpected: ${cmd}`);
		};
		const { handlers, messages } = setup(router);
		await handlers['session_start']({ reason: 'new' }, mkctx('smoke-c'));
		fileAdded = true;
		await handlers['turn_end'](EDIT_TURN, mkctx('smoke-c'));
		check('C1 兜底失败 gate.check 全量落库（ok=false ≥3）', q(repo, "SELECT COUNT(*) n FROM events WHERE kind='gate.check' AND json_extract(payload,'$.ok')=0")[0].n >= 3);
		check('C2 探测健康本身零 policy.decision', q(repo, "SELECT COUNT(*) n FROM events WHERE kind='policy.decision'")[0].n === 0);
		check('C3 环境归因 steer（interop-failure 含 UtilAcceptVsock）', messages.some(({ m }) => m.customType === 'quality-gate-env-failure' && m.content.includes('UtilAcceptVsock')));
		check('C4 不混入门禁失败 steer（无 [回归]/[中间态]）', !messages.some(({ m }) => m.customType === 'quality-gate-failure'));
		turn = 2;
		await handlers['turn_end'](CHAT_TURN, mkctx('smoke-c')); // 纯对话：粘性豁免
		check('C5 粘性豁免：下 turn 纯对话零 cmd.exe 调用', cmdCallsTurn2 === 0);
	}

	// ============ 场景 D：真实失败粘性（既有语义回归） ============
	{
		setPlatform('windows');
		const repo = mkrepo();
		let fileAdded = false;
		let turn = 1;
		let lintReran = 0;
		const router = async (cmd, args) => {
			if (cmd === 'git') return gitRouter(repo, fileAdded ? 'backend-go/a.go' : '')(cmd, args);
			if (cmd === 'bash') return { code: 0, stdout: '{"testTargets":[]}', stderr: '' };
			if (cmd === 'cmd.exe') {
				const cl = args.join(' ');
				if (cl.includes('echo ok')) return { code: 0, stdout: 'ok', stderr: '' };
				if (cl.includes('golangci-lint')) {
					if (turn === 2) lintReran++;
					return turn === 2
						? { code: 0, stdout: '0 issues.', stderr: '' }
						: { code: 1, stdout: 'internal\\x\\a.go:1:1: expected declaration, found package', stderr: '' };
				}
				return { code: 0, stdout: '', stderr: '' };
			}
			throw new Error(`unexpected: ${cmd}`);
		};
		const { handlers, messages } = setup(router);
		await handlers['session_start']({ reason: 'new' }, mkctx('smoke-d'));
		fileAdded = true;
		await handlers['turn_end'](EDIT_TURN, mkctx('smoke-d'));
		check('D1 真实失败 steer 含 [中间态] 分级（既有语义）', messages.some(({ m }) => m.customType === 'quality-gate-failure' && m.content.includes('[中间态]')));
		check('D2 真实失败 steer 含 (wsl环境) 标注', messages.some(({ m }) => m.customType === 'quality-gate-failure' && m.content.includes('(wsl环境)')));		turn = 2;
		await handlers['turn_end'](CHAT_TURN, mkctx('smoke-d')); // 纯对话：粘性重跑
		check('D3 粘性重跑：纯对话 turn 仍重跑失败命令（既有语义）', lintReran === 1);
	}

	// ============ 场景 E：native 模式正常态（linux-native-dev-environment） ============
	// Linux/macOS 宿主：cmd.exe 不可达 → 门禁走本机工具链，不探测、不短路。
	{
		setPlatform('native');
		const repo = mkrepo();
		let fileAdded = false;
		const backendCmds = [];
		const router = async (cmd, args, opts) => {
			if (cmd === 'git') return gitRouter(repo, fileAdded ? 'backend-go/a.go' : '')(cmd, args);
			if (cmd === 'bash') {
				const cl = args.join(' ');
				if (cl.includes('change-scope')) return { code: 0, stdout: '{"testTargets":[]}', stderr: '' };
				backendCmds.push({ cl, cwd: opts?.cwd });
				return { code: 0, stdout: '0 issues.', stderr: '' };
			}
			throw new Error(`unexpected: ${cmd}`); // native 下不应出现 cmd.exe
		};
		const { handlers, messages, execCalls } = setup(router);
		await handlers['session_start']({ reason: 'new' }, mkctx('smoke-e'));
		fileAdded = true;
		await handlers['turn_end'](EDIT_TURN, mkctx('smoke-e'));
		check('E1 native 模式零 cmd.exe 调用（不探测）', !execCalls.some((c) => c.startsWith('cmd.exe')));
		check('E2 native 门禁真实执行（gate.check ≥3 落库）', q(repo, "SELECT COUNT(*) n FROM events WHERE kind='gate.check'")[0].n >= 3);
		check('E3 native 正常态零 policy.decision（零 interop-down）', q(repo, "SELECT COUNT(*) n FROM events WHERE kind='policy.decision'")[0].n === 0);
		check('E4 后端门禁经 ExecOptions.cwd 定位于 backend-go', backendCmds.length >= 3 && backendCmds.every((b) => (b.cwd ?? '').endsWith('backend-go')));
		check('E5 native 全绿零 steer（不发环境故障提示）', messages.length === 0);
		check('E6 native lint 命令同样带 --allow-parallel-runners', backendCmds.some((b) => b.cl.includes('--allow-parallel-runners')));
	}

	// ============ 场景 F：native 失败归因（不误判 interop + 本机链路标注） ============
	// 输出故意含 interop 特征串：native 无跨系统链路，MUST 仍按真实代码失败归因。
	{
		setPlatform('native');
		const repo = mkrepo();
		let fileAdded = false;
		let turn = 1;
		let lintReran = 0;
		const VSOCK = '<3>WSL (1751 - ) ERROR: UtilAcceptVsock:251: accept4 failed 110';
		const router = async (cmd, args) => {
			if (cmd === 'git') return gitRouter(repo, fileAdded ? 'backend-go/a.go' : '')(cmd, args);
			if (cmd === 'bash') {
				const cl = args.join(' ');
				if (cl.includes('change-scope')) return { code: 0, stdout: '{"testTargets":[]}', stderr: '' };
				if (cl.includes('golangci-lint')) {
					if (turn === 2) lintReran++;
					return turn === 2
						? { code: 0, stdout: '0 issues.', stderr: '' }
						: { code: 1, stdout: `internal/x/a.go:1:1: expected declaration\n${VSOCK}`, stderr: '' };
				}
				return { code: 0, stdout: '', stderr: '' };
			}
			throw new Error(`unexpected: ${cmd}`);
		};
		const { handlers, messages } = setup(router);
		await handlers['session_start']({ reason: 'new' }, mkctx('smoke-f'));
		fileAdded = true;
		await handlers['turn_end'](EDIT_TURN, mkctx('smoke-f'));
		check('F1 native 下 interop 特征串不触发环境归因', !messages.some(({ m }) => m.customType === 'quality-gate-env-failure'));
		check('F2 native 失败仍走分级 steer（[中间态]）', messages.some(({ m }) => m.customType === 'quality-gate-failure' && m.content.includes('[中间态]')));
		check('F3 native 失败 steer 标本机链路、不含 wsl环境', messages.some(({ m }) => m.customType === 'quality-gate-failure' && m.content.includes('(本机)') && !m.content.includes('wsl环境')));
		turn = 2;
		await handlers['turn_end'](CHAT_TURN, mkctx('smoke-f'));
		check('F4 native 失败进粘性：纯对话 turn 重跑（既有语义）', lintReran === 1);
	}

	// ============ 场景 G：native 工具链缺失侧级短路（harden-gate-native-toolchain） ============
	// PATH 仅含 pnpm：后端探测（go+golangci-lint 双检）不可达 → 整侧短路；前端照常。
	{
		setPlatform('native-pnpm-only');
		const repo = mkrepo();
		let files = ''; // 空基线（session_start 后再放文件，与场景 A-F 同模式）
		const router = async (cmd, args) => {
			if (cmd === 'git') return gitRouter(repo, files)(cmd, args);
			if (cmd === 'bash') {
				const cl = args.join(' ');
				if (cl.includes('change-scope')) return { code: 0, stdout: '{"testTargets":[]}', stderr: '' };
				if (cl.includes('eslint')) return { code: 0, stdout: '', stderr: '' }; // 前端 lint 全绿
				throw new Error(`unexpected bash gate cmd: ${cl}`); // 后端命令被短路，不应到达
			}
			throw new Error(`unexpected: ${cmd}`);
		};
		const { handlers, messages, execCalls } = setup(router);
		await handlers['session_start']({ reason: 'new' }, mkctx('smoke-g'));
		files = 'backend-go/a.go\nfront/app/x.ts'; // 双侧同时触发：验证前端不受后端短路影响
		await handlers['turn_end'](EDIT_TURN, mkctx('smoke-g'));
		const polG = q(repo, "SELECT payload FROM events WHERE kind='policy.decision'");
		check('G1 后端短路零 gate.check 假失败（仅前端 ok=true 锚点入账）',
			q(repo, "SELECT COUNT(*) n FROM events WHERE kind='gate.check' AND json_extract(payload,'$.ok')=0").length === 0 || q(repo, "SELECT COUNT(*) n FROM events WHERE kind='gate.check' AND json_extract(payload,'$.ok')=0")[0].n === 0);
		check('G1b gate.check 无后端命令记录', !q(repo, "SELECT payload FROM events WHERE kind='gate.check'").some((r) => /golangci|go (vet|build|test)/.test(r.payload)));
		check('G2 恰一条 toolchain-down(target=backend) 边沿记账', polG.length === 1
			&& polG[0].payload.includes('"reasonCode":"toolchain-down"')
			&& polG[0].payload.includes('"target":"backend"'));
		check('G3 短路 steer 提示 PATH 恢复方向、无 WSL 建议', messages.some(({ m }) => m.customType === 'quality-gate-toolchain-down'
			&& m.content.includes('PATH') && !m.content.includes('wsl --shutdown')));
		check('G4 后端门禁命令零执行（无 go/golangci-lint/change-scope）', !execCalls.some((c) => c.includes('golangci-lint') || /\bgo (vet|build|test)\b/.test(c) || c.includes('change-scope')));
		check('G5 前端照常执行（pnpm eslint 跑了）', execCalls.some((c) => c.includes('eslint')));
		files = 'backend-go/a.go\nbackend-go/b.go\nfront/app/x.ts'; // 新后端文件再触发：仍短路但不重复记
		await handlers['turn_end'](EDIT_TURN, mkctx('smoke-g'));
		check('G6 边沿触发：第二回合短路不重复记账不发 steer', q(repo, "SELECT COUNT(*) n FROM events WHERE kind='policy.decision'")[0].n === 1
			&& messages.filter(({ m }) => m.customType === 'quality-gate-toolchain-down').length === 1);
	}

	// ============ 场景 H：native 工具缺失兑底归因（探测后文件被删窗口期） ============
	// 探测时工具链在（PATH 假工具），执行时 bash 返回 command not found：不进粘性、不分级。
	{
		setPlatform('native');
		const repo = mkrepo();
		let files = '';
		let turn = 1;
		let gateCmdsTurn2 = 0;
		const NOT_FOUND = 'bash: line 1: go: command not found';
		const router = async (cmd, args) => {
			if (cmd === 'git') return gitRouter(repo, files)(cmd, args);
			if (cmd === 'bash') {
				const cl = args.join(' ');
				if (cl.includes('change-scope')) return { code: 0, stdout: '{"testTargets":[]}', stderr: '' };
				if (turn === 2) gateCmdsTurn2++;
				if (cl.includes('golangci-lint')) return { code: 127, stdout: '', stderr: 'bash: line 1: golangci-lint: command not found' };
				return { code: 127, stdout: '', stderr: NOT_FOUND }; // vet/build
			}
			throw new Error(`unexpected: ${cmd}`);
		};
		const { handlers, messages } = setup(router);
		await handlers['session_start']({ reason: 'new' }, mkctx('smoke-h'));
		files = 'backend-go/a.go';
		await handlers['turn_end'](EDIT_TURN, mkctx('smoke-h'));
		check('H1 兑底失败 gate.check 全量落库（ok=false ≥3，diag 含特征可考古）', q(repo, "SELECT COUNT(*) n FROM events WHERE kind='gate.check' AND json_extract(payload,'$.ok')=0")[0].n >= 3);
		check('H2 环境归因 steer 归因工具链而非代码（含 command not found）', messages.some(({ m }) => m.customType === 'quality-gate-env-failure'
			&& m.content.includes('command not found') && m.content.includes('非代码问题')));
		check('H3 工具链归因文案给 PATH 建议、不给 wsl --shutdown', messages.some(({ m }) => m.customType === 'quality-gate-env-failure'
			&& m.content.includes('PATH') && !m.content.includes('wsl --shutdown')));
		check('H4 不混入门禁失败 steer（无 [回归]/[中间态] 分级）', !messages.some(({ m }) => m.customType === 'quality-gate-failure'));
		check('H5 零 policy.decision（兑底路径不记 toolchain-down，探测是过的）', q(repo, "SELECT COUNT(*) n FROM events WHERE kind='policy.decision'")[0].n === 0);
		turn = 2;
		await handlers['turn_end'](CHAT_TURN, mkctx('smoke-h')); // 纯对话：粘性豁免
		check('H6 粘性豁免：下 turn 纯对话零后端命令重跑', gateCmdsTurn2 === 0);
	}

	// ============ 场景 I：windows 模式不受工具链探测影响 ============
	// cmd.exe 可达但 PATH 无 go/golangci-lint/pnpm：windows 用 Windows 绝对路径执行，
	// 不经 bash PATH 查找，MUST 不探测不短路。
	{
		setPlatform('windows-nogo');
		const repo = mkrepo();
		let files = '';
		const router = async (cmd, args) => {
			if (cmd === 'git') return gitRouter(repo, files)(cmd, args);
			if (cmd === 'bash') return { code: 0, stdout: '{"testTargets":[]}', stderr: '' };
			if (cmd === 'cmd.exe') {
				const cl = args.join(' ');
				if (cl.includes('echo ok')) return { code: 0, stdout: 'ok', stderr: '' };
				return { code: 0, stdout: '0 issues.', stderr: '' }; // 全绿
			}
			throw new Error(`unexpected: ${cmd}`);
		};
		const { handlers, messages } = setup(router);
		await handlers['session_start']({ reason: 'new' }, mkctx('smoke-i'));
		files = 'backend-go/a.go';
		await handlers['turn_end'](EDIT_TURN, mkctx('smoke-i'));
		check('I1 windows 下 PATH 缺工具链不短路（gate.check ≥3 照常）', q(repo, "SELECT COUNT(*) n FROM events WHERE kind='gate.check'")[0].n >= 3);
		check('I2 零 toolchain-down 记账', !q(repo, "SELECT payload FROM events WHERE kind='policy.decision'").some((r) => r.payload.includes('toolchain-down')));
		check('I3 零环境 steer', messages.length === 0);
	}

	// ============ 场景 J：子线程 turn_end 降载（harden-subagent-constraint-channel D4） ============
	// 父子关系可证（header parentSession）→ turn_end 入口整体短路：git/探测/门禁命令一个
	// 不跑，恰一条 bypass(child-session) 记账，零 gate.check，回合正常放行（零 steer）。
	{
		setPlatform('native');
		const repo = mkrepo();
		const router = async (cmd, args) => { throw new Error(`unexpected exec in child session: ${cmd} ${args.join(' ')}`); };
		const { handlers, messages, execCalls } = setup(router);
		await handlers['session_start']({ reason: 'startup' }, childCtx('smoke-j', repo)); // 子线程派发：startup 早退（既有语义）
		await handlers['turn_end'](EDIT_TURN, childCtx('smoke-j', repo));
		const polJ = q(repo, "SELECT payload FROM events WHERE kind='policy.decision'");
		check('J1 子线程 turn_end 零 exec（git 快照/探测/门禁全不跑）', execCalls.length === 0);
		check('J2 恰一条 policy.decision 且形状精确（quality-gate/bypass/child-session）', polJ.length === 1
			&& JSON.stringify(JSON.parse(polJ[0].payload)) === JSON.stringify({ policy: 'quality-gate', action: 'bypass', reasonCode: 'child-session' }));
		check('J3 零 gate.check（没跑命令零记账）', q(repo, "SELECT COUNT(*) n FROM events WHERE kind='gate.check'")[0].n === 0);
		check('J4 回合正常放行零 steer', messages.length === 0);
		await handlers['turn_end'](CHAT_TURN, childCtx('smoke-j', repo)); // 纯对话回合同样降载
		check('J5 纯对话回合同样 bypass 记账（置顶于 touchedCode 早退，每回合一条）',
			q(repo, "SELECT COUNT(*) n FROM events WHERE kind='policy.decision'")[0].n === 2);
	}

	// ============ 场景 K：主会话不受降载影响（harden-subagent-constraint-channel） ============
	// 有会话文件证据位但非 forks/ 路径、无 parentSession → 判定为假 → 门禁照常、零 bypass 记账
	// （判定默认方向回归：防 isChildSession 误报把主会话门禁静默掉）。
	{
		setPlatform('native');
		const repo = mkrepo();
		let files = '';
		const router = async (cmd, args) => {
			if (cmd === 'git') return gitRouter(repo, files)(cmd, args);
			if (cmd === 'bash') {
				const cl = args.join(' ');
				if (cl.includes('change-scope')) return { code: 0, stdout: '{"testTargets":[]}', stderr: '' };
				return { code: 0, stdout: '0 issues.', stderr: '' };
			}
			throw new Error(`unexpected: ${cmd}`);
		};
		const { handlers, messages } = setup(router);
		const mainCtx = (sid) => ({
			signal: undefined,
			cwd: repo,
			sessionManager: {
				getSessionId: () => sid,
				getSessionFile: () => `/home/x/.pi/agent/sessions/dir/2026-09-18T00-00-00-000Z_${sid}.jsonl`,
			},
		});
		await handlers['session_start']({ reason: 'new' }, mainCtx('smoke-k'));
		files = 'backend-go/a.go';
		await handlers['turn_end'](EDIT_TURN, mainCtx('smoke-k'));
		check('K1 主会话（非 fork 路径）门禁照常执行（gate.check ≥3）', q(repo, "SELECT COUNT(*) n FROM events WHERE kind='gate.check'")[0].n >= 3);
		check('K2 零 child-session 记账（零 policy.decision）', q(repo, "SELECT COUNT(*) n FROM events WHERE kind='policy.decision'")[0].n === 0);
		check('K3 零 steer', messages.length === 0);
	}

	// ============ 场景 L：失败指纹状态机（attribute-concurrent-gate-noise 4.1 ①②④ + A 组） ============
	// 首次完整块 → 同指纹 ⟳ 单行（第 3 回合带未修）→ 转绿 ✓ 收尾；粘性重跑语义不变。
	{
		setPlatform('windows');
		const repo = mkrepo();
		let files = '';
		let lintCalls = 0;
		let lintGreen = false; // 第 4 回合翻绿验证转绿收尾
		const LINT_FAIL = 'internal/x/a.go:1:1: expected declaration, found package';
		const router = async (cmd, args) => {
			if (cmd === 'git') return gitRouter(repo, files)(cmd, args);
			if (cmd === 'bash') return { code: 0, stdout: '{"testTargets":[]}', stderr: '' };
			if (cmd === 'cmd.exe') {
				const cl = args.join(' ');
				if (cl.includes('echo ok')) return { code: 0, stdout: 'ok', stderr: '' };
				if (cl.includes('golangci-lint')) {
					lintCalls++;
					return lintGreen
						? { code: 0, stdout: '0 issues.', stderr: '' }
						: { code: 1, stdout: LINT_FAIL, stderr: '' };
				}
				return { code: 0, stdout: '', stderr: '' }; // vet/build 全绿
			}
			throw new Error(`unexpected: ${cmd}`);
		};
		const { handlers, messages } = setup(router);
		await handlers['session_start']({ reason: 'new' }, mkctx('smoke-l'));
		files = 'backend-go/a.go';
		await handlers['turn_end'](EDIT_TURN, mkctx('smoke-l'));
		const failMsg1 = messages.find(({ m }) => m.customType === 'quality-gate-failure');
		check('L1 ①首次失败完整块（[中间态] + exit + 输出尾部）', !!failMsg1
			&& failMsg1.m.content.includes('[中间态]') && failMsg1.m.content.includes('exit 1')
			&& failMsg1.m.content.includes('expected declaration'));
		const lintAfterT1 = lintCalls;
		await handlers['turn_end'](CHAT_TURN, mkctx('smoke-l'));
		const failMsg2 = messages.filter(({ m }) => m.customType === 'quality-gate-failure').pop();
		check('L2 ①同指纹第 2 回合单行（⟳ + 回合数，无输出尾部重灌）', !!failMsg2
			&& failMsg2.m.content.includes('⟳') && failMsg2.m.content.includes('第 2 回合未变化')
			&& failMsg2.m.content.includes(LINT_FAIL) && !failMsg2.m.content.includes('exit 1'));
		check('L3 ④粘性在纯对话回合仍重跑（lint 重跑一次）', lintCalls === lintAfterT1 + 1);
		await handlers['turn_end'](CHAT_TURN, mkctx('smoke-l'));
		const failMsg3 = messages.filter(({ m }) => m.customType === 'quality-gate-failure').pop();
		check('L4 ①连续 3 回合未变化附加「未修」', !!failMsg3
			&& failMsg3.m.content.includes('第 3 回合未变化') && failMsg3.m.content.includes('未修'));
		files = ''; // 转绿回合：改文件已还原（无新增触发，仅粘性重跑）
		lintGreen = true;
		await handlers['turn_end'](CHAT_TURN, mkctx('smoke-l'));
		const greenMsg = messages.find(({ m }) => m.customType === 'quality-gate-green');
		check('L5 ②转绿收尾一行 ✓', !!greenMsg && greenMsg.m.content.includes('✓')
			&& greenMsg.m.content.includes('golangci-lint') && greenMsg.m.content.includes('已转绿'));
		const lintBeforeIdle = lintCalls;
		const msgBeforeIdle = messages.length;
		await handlers['turn_end'](CHAT_TURN, mkctx('smoke-l'));
		check('L6b ②b 转绿后纯对话零门禁零消息（A7 静默）', lintCalls === lintBeforeIdle && messages.length === msgBeforeIdle);
	}

	// ============ 场景 M：会话边界重置指纹（4.1 ③ + A9） ============
	// 新会话同 diag 仍出完整块（跨会话不复用指纹）。
	{
		setPlatform('windows');
		const repo = mkrepo();
		let files = '';
		const LINT_FAIL = 'internal/x/a.go:1:1: expected declaration, found package';
		const router = async (cmd, args) => {
			if (cmd === 'git') return gitRouter(repo, files)(cmd, args);
			if (cmd === 'bash') return { code: 0, stdout: '{"testTargets":[]}', stderr: '' };
			if (cmd === 'cmd.exe') {
				const cl = args.join(' ');
				if (cl.includes('echo ok')) return { code: 0, stdout: 'ok', stderr: '' };
				if (cl.includes('golangci-lint')) return { code: 1, stdout: LINT_FAIL, stderr: '' };
				return { code: 0, stdout: '', stderr: '' };
			}
			throw new Error(`unexpected: ${cmd}`);
		};
		const { handlers, messages } = setup(router);
		await handlers['session_start']({ reason: 'new' }, mkctx('smoke-m'));
		files = 'backend-go/a.go';
		await handlers['turn_end'](EDIT_TURN, mkctx('smoke-m'));
		files = 'backend-go/a.go\nbackend-go/b.go';
		await handlers['session_start']({ reason: 'new' }, mkctx('smoke-m')); // 会话边界重置
		await handlers['turn_end'](EDIT_TURN, mkctx('smoke-m'));
		const failMsg = messages.filter(({ m }) => m.customType === 'quality-gate-failure').pop();
		check('M1 ③新会话同 diag 仍出完整块（不跨会话复用指纹）', !!failMsg
			&& failMsg.m.content.includes('[中间态]') && failMsg.m.content.includes('exit 1')
			&& !failMsg.m.content.includes('⟳'));
	}

	// ============ 场景 N：启动基线外部归因（4.1 ⑤ + C1/C9/C10 + 2.1 基线不覆盖） ============
	// 会话启动时 y.go 已脏（别人的半成品）进基线；本会话只触发 a.go——lint 报 y.go
	// 应判 [外部]：不进粘性、无 [回归]/[中间态]、一行提示 + foreign-breakage 记账。
	// 第 3 编辑回合 y.go 已被物主修走（不在 git 脏集）但 lint 仍报同错误：基线不随
	// turn_end 覆盖（2.1），仍判外部同指纹静默——若基线被覆盖，此处会退化成完整块。
	{
		setPlatform('native');
		const repo = mkrepo();
		fs.mkdirSync(path.join(repo, 'backend-go', 'internal', 'x'), { recursive: true });
		fs.mkdirSync(path.join(repo, 'backend-go', 'internal', 'mine'), { recursive: true });
		fs.writeFileSync(path.join(repo, 'backend-go', 'internal', 'x', 'y.go'), 'package x\n');
		for (const f of ['a.go', 'c.go', 'd.go', 'e.go']) fs.writeFileSync(path.join(repo, 'backend-go', 'internal', 'mine', f), 'package x\n');
		let files = 'backend-go/internal/x/y.go'; // session_start 时的脏集（别人改的）
		const Y_FAIL = 'internal/x/y.go:1:1: boom';
		let lintOut = Y_FAIL;
		const router = async (cmd, args) => {
			if (cmd === 'git') return gitRouter(repo, files)(cmd, args);
			if (cmd === 'bash') {
				const cl = args.join(' ');
				if (cl.includes('change-scope')) return { code: 0, stdout: '{"testTargets":[]}', stderr: '' };
				if (cl.includes('golangci-lint')) return { code: 1, stdout: lintOut, stderr: '' };
				return { code: 0, stdout: '', stderr: '' }; // vet/build
			}
			throw new Error(`unexpected: ${cmd}`);
		};
		const { handlers, messages, execCalls } = setup(router);
		await handlers['session_start']({ reason: 'new' }, mkctx('smoke-n'));
		files = 'backend-go/internal/x/y.go\nbackend-go/internal/mine/a.go';
		await handlers['turn_end'](EDIT_TURN, mkctx('smoke-n'));
		const foreignMsgs = messages.filter(({ m }) => m.customType === 'quality-gate-foreign');
		check('N1 ⑤基线失败标 [外部] 一行（含路径，非本会话所致）', foreignMsgs.length === 1
			&& foreignMsgs[0].m.content.includes('[外部]') && foreignMsgs[0].m.content.includes('golangci-lint')
			&& foreignMsgs[0].m.content.includes('internal/x/y.go') && foreignMsgs[0].m.content.includes('非本会话所致'));
		check('N2 ⑤无 [回归]/[中间态] 失败块（不催修本会话）', !messages.some(({ m }) => m.customType === 'quality-gate-failure'));
		const fbRows = () => q(repo, "SELECT payload FROM events WHERE kind='policy.decision' AND payload LIKE '%foreign-breakage%'");
		check('N3 ⑤账本一条 foreign-breakage(warn, target=golangci-lint)', fbRows().length === 1
			&& JSON.parse(fbRows()[0].payload).action === 'warn'
			&& JSON.parse(fbRows()[0].payload).target === 'golangci-lint');
		check('N4 ⑤gate.check 照记失败事实（ok=false，diag 可考古）', q(repo, "SELECT payload FROM events WHERE kind='gate.check' AND json_extract(payload,'$.ok')=0").length >= 1
			&& q(repo, "SELECT payload FROM events WHERE kind='gate.check' AND json_extract(payload,'$.ok')=0").some((r) => r.payload.includes(Y_FAIL)));
		const lintCallsN = () => execCalls.filter((c) => c.includes('golangci-lint')).length;
		const lintN1 = lintCallsN();
		await handlers['turn_end'](CHAT_TURN, mkctx('smoke-n')); // 纯对话：外部失败不进粘性不重跑
		check('N5 ⑤C9 外部失败不进粘性：纯对话回合零重跑零新提示', lintCallsN() === lintN1
			&& messages.filter(({ m }) => m.customType === 'quality-gate-foreign').length === 1);
		files = 'backend-go/internal/mine/a.go\nbackend-go/internal/mine/c.go'; // y.go 已被物主修离脏集；本会话继续编辑
		await handlers['turn_end'](EDIT_TURN, mkctx('smoke-n'));
		check('N6 2.1 基线不随 turn_end 覆盖：y.go 仍判外部同指纹静默（重命中记账照追加）', lintCallsN() === lintN1 + 1
			&& messages.filter(({ m }) => m.customType === 'quality-gate-foreign').length === 1
			&& !messages.some(({ m }) => m.customType === 'quality-gate-failure')
			&& fbRows().length === 2);
		// —— 场景 N 续：混合路径 / 解析不出 → 现状（4.1 ⑥ + C4/C5） ——
		files = 'backend-go/internal/mine/a.go\nbackend-go/internal/mine/c.go\nbackend-go/internal/mine/d.go';
		lintOut = 'internal/mine/a.go:1:1: e1\ninternal/x/y.go:9:9: e2'; // 混合：a.go 本会话触发过，y.go 基线
		await handlers['turn_end'](EDIT_TURN, mkctx('smoke-n'));
		const mixedMsg = messages.filter(({ m }) => m.customType === 'quality-gate-failure').pop();
		check('N7 ⑥混合路径保守按本会话失败（完整块 + 记账不升级）', !!mixedMsg
			&& mixedMsg.m.content.includes('[中间态]') && mixedMsg.m.content.includes('e1')
			&& messages.filter(({ m }) => m.customType === 'quality-gate-foreign').length === 1
			&& fbRows().length === 2);
		files = 'backend-go/internal/mine/a.go\nbackend-go/internal/mine/c.go\nbackend-go/internal/mine/d.go\nbackend-go/internal/mine/e.go';
		lintOut = 'something went terribly wrong without any paths'; // 解析不出
		await handlers['turn_end'](EDIT_TURN, mkctx('smoke-n'));
		check('N8 ⑥C5 解析不出保守按本会话失败', (() => {
			const msg = messages.filter(({ m }) => m.customType === 'quality-gate-failure').pop();
			return !!msg && msg.m.content.includes('[中间态]')
				&& messages.filter(({ m }) => m.customType === 'quality-gate-foreign').length === 1
				&& fbRows().length === 2;
		})());
	}

	// ============ 场景 O：edit.map 归属外部 + 同指纹不重发（4.1 ⑦ + C2/C11） ============
	// 别的 change（ch-other）的 edit.map 归属 legacy.vue；本会话绑定 ch-mine（mode.set）。
	// eslint 报 legacy.vue（front 侧相对形态）→ [外部]；第 2 编辑回合同指纹静默，
	// 但 gate.check 与 foreign-breakage 记账照记（C11/C10）。
	{
		setPlatform('native');
		const repo = mkrepo();
		fs.writeFileSync(path.join(repo, 'front', 'app', 'new.vue'), '<template/>\n');
		fs.writeFileSync(path.join(repo, 'front', 'app', 'new2.vue'), '<template/>\n');
		const hlog = require('./.hlog.cjs'); // 事实库种子（走真 writer，app_id/schema 合法）
		hlog.logEvent(repo, { kind: 'mode.set', sessionId: 'smoke-o', change: 'ch-mine', payload: { mode: 'implementation', boundChange: 'ch-mine', source: 'skill' } });
		hlog.logEvent(repo, { kind: 'edit.map', sessionId: 'seed-other', change: 'ch-mine', payload: { paths: ['backend-go/internal/mine/ok.go'], n: 1 } });
		hlog.logEvent(repo, { kind: 'edit.map', sessionId: 'seed-other2', change: 'ch-other', payload: { paths: ['front/app/legacy.vue'], n: 1 } });
		let files = '';
		let eslintCalls = 0;
		const router = async (cmd, args) => {
			if (cmd === 'git') return gitRouter(repo, files)(cmd, args);
			if (cmd === 'bash') {
				const cl = args.join(' ');
				if (cl.includes('change-scope')) return { code: 0, stdout: '{"testTargets":[]}', stderr: '' };
				if (cl.includes('eslint')) {
					eslintCalls++;
					return { code: 1, stdout: 'app/legacy.vue\n  1:1  error  boom  no-rule', stderr: '' };
				}
				throw new Error(`unexpected bash gate cmd: ${cl}`);
			}
			throw new Error(`unexpected: ${cmd}`);
		};
		const { handlers, messages, execCalls } = setup(router);
		await handlers['session_start']({ reason: 'new' }, mkctx('smoke-o'));
		files = 'front/app/new.vue';
		await handlers['turn_end'](EDIT_TURN, mkctx('smoke-o'));
		const foreignMsgs = messages.filter(({ m }) => m.customType === 'quality-gate-foreign');
		check('O1 ⑦其他 change 归属识别为外部（front 相对形态前缀命中）', foreignMsgs.length === 1
			&& foreignMsgs[0].m.content.includes('[外部]') && foreignMsgs[0].m.content.includes('app/legacy.vue')
			&& !messages.some(({ m }) => m.customType === 'quality-gate-failure'));
		check('O2 ⑦外部行走指纹机（pnpm lint 首现一行）', foreignMsgs.length === 1 && foreignMsgs[0].m.content.includes('pnpm lint'));
		const fbCount = () => q(repo, "SELECT COUNT(*) n FROM events WHERE kind='policy.decision' AND payload LIKE '%foreign-breakage%'")[0].n;
		const gateFails = () => q(repo, "SELECT COUNT(*) n FROM events WHERE kind='gate.check' AND json_extract(payload,'$.ok')=0 AND payload LIKE '%pnpm lint%'")[0].n;
		check('O3 ⑦首现记账（1 条 foreign-breakage + 1 条 gate.check 失败）', fbCount() === 1 && gateFails() === 1);
		const eslintT1 = execCalls.filter((c) => c.includes('eslint')).length;
		files = 'front/app/new.vue\nfront/app/new2.vue';
		await handlers['turn_end'](EDIT_TURN, mkctx('smoke-o'));
		check('O4 ⑦C11 同指纹第 2 编辑回合不重发提示行（门禁照跑）', execCalls.filter((c) => c.includes('eslint')).length === eslintT1 + 1
			&& messages.filter(({ m }) => m.customType === 'quality-gate-foreign').length === 1);
		check('O5 ⑦C10/C11 记账不受抑制影响（各累计 2 条）', fbCount() === 2 && gateFails() === 2);
	}

	// ============ 场景 P：库不可用 fail-open（2.4 + C6/C7） ============
	// events.db 损坏 → 归属查询/记账全部旁路：门禁照常执行、失败照旧进粘性重跑。
	{
		setPlatform('native');
		const repo = mkrepo();
		fs.mkdirSync(path.join(repo, '.pi', 'harness'), { recursive: true });
		fs.writeFileSync(path.join(repo, '.pi', 'harness', 'events.db'), 'this is not a sqlite database');
		let files = '';
		let lintCalls = 0;
		const router = async (cmd, args) => {
			if (cmd === 'git') return gitRouter(repo, files)(cmd, args);
			if (cmd === 'bash') {
				const cl = args.join(' ');
				if (cl.includes('change-scope')) return { code: 0, stdout: '{"testTargets":[]}', stderr: '' };
				if (cl.includes('golangci-lint')) {
					lintCalls++;
					return { code: 1, stdout: 'internal/x/a.go:1:1: bad', stderr: '' };
				}
				return { code: 0, stdout: '', stderr: '' };
			}
			throw new Error(`unexpected: ${cmd}`);
		};
		const { handlers, messages } = setup(router);
		await handlers['session_start']({ reason: 'new' }, mkctx('smoke-p'));
		files = 'backend-go/a.go';
		await handlers['turn_end'](EDIT_TURN, mkctx('smoke-p'));
		const failMsg = messages.find(({ m }) => m.customType === 'quality-gate-failure');
		check('P1 2.4 库不可用门禁照常执行（失败完整块 + 分级现状）', !!failMsg
			&& failMsg.m.content.includes('[中间态]') && failMsg.m.content.includes('exit 1'));
		await handlers['turn_end'](CHAT_TURN, mkctx('smoke-p'));
		check('P2 2.4 失败照旧进 sticky：纯对话回合重跑', lintCalls === 2);
		check('P3 2.4 库不可用不误判外部（路径不在基线 → C6 保守）', !messages.some(({ m }) => m.customType === 'quality-gate-foreign'));
	}

	let fail = 0;
	for (const [name, ok] of checks) {
		console.log(`${ok ? '✅' : '❌'} ${name}`);
		if (!ok) fail++;
	}
	console.log(fail ? `\n${fail} 项失败` : '\nSMOKE OK');
	if (fail) process.exitCode = 1;
})().catch((e) => { console.error('SMOKE FAIL', e); process.exitCode = 1; });
