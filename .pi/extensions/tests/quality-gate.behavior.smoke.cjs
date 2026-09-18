// quality-gate 行为级 smoke（harden-gate-interop-health test-cases 主链路步 1-7）：
// mock pi + ctx 驱动 quality-gate bundle 的 turn_end handler，真 tmp events.db 断言。
// 场景 A 正常态(windows) / B 探测故障短路 / C 兜底 diag 特征(粘性豁免) / D 真实失败粘性 /
// E native 模式正常态 / F native 失败归因与链路标注（linux-native-dev-environment 新增）/
// G native 工具链缺失侧级短路 / H native 工具缺失兑底归因 / I windows 不受工具链探测影响
// （harden-gate-native-toolchain 新增）。
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

	let fail = 0;
	for (const [name, ok] of checks) {
		console.log(`${ok ? '✅' : '❌'} ${name}`);
		if (!ok) fail++;
	}
	console.log(fail ? `\n${fail} 项失败` : '\nSMOKE OK');
	if (fail) process.exitCode = 1;
})().catch((e) => { console.error('SMOKE FAIL', e); process.exitCode = 1; });
