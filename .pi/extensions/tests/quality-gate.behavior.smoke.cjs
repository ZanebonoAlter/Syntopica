// quality-gate 行为级 smoke（harden-gate-interop-health test-cases 主链路步 1-7）：
// mock pi + ctx 驱动 quality-gate bundle 的 turn_end handler，真 tmp events.db 断言。
// 场景 A 正常态 / B 探测故障短路 / C 兜底 diag 特征（粘性豁免）/ D 真实失败粘性。
// 模块级状态（sticky/snapshot/gateOkStates）由各场景前的 session_start 重置。
// 由 run-harness-smoke.sh 调用（先 esbuild 产出 .qgate.cjs）。断言失败 exit 1。
const fs = require('fs');
const os = require('os');
const path = require('path');
const { execSync } = require('child_process');

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
	fs.writeFileSync(path.join(repo, 'backend-go', 'a.go'), 'package x\n');
	return repo;
}

const mkctx = (sid) => ({ signal: undefined, sessionManager: { getSessionId: () => sid } });
const EDIT_TURN = { toolResults: [{ toolName: 'edit' }] };
const CHAT_TURN = { toolResults: [{ toolName: 'read' }] };

/** git 三件套 mock：ls-files 按 hasFile 决定是否返回 a.go；rev-parse 返回真 repo 路径 */
const gitRouter = (repoRoot, hasFile) => (cmd, args) => {
	if (cmd === 'git' && args[0] === 'diff') return { code: 0, stdout: '', stderr: '' };
	if (cmd === 'git' && args[0] === 'ls-files') return { code: 0, stdout: hasFile ? 'backend-go/a.go' : '', stderr: '' };
	if (cmd === 'git' && args[0] === 'rev-parse') return { code: 0, stdout: repoRoot, stderr: '' };
	throw new Error(`unexpected git: ${cmd} ${args.join(' ')}`);
};

const q = (repo, sql) => {
	const db = path.join(repo, '.pi', 'harness', 'events.db');
	if (!fs.existsSync(db)) return [];
	const out = execSync(`sqlite3 -json "${db}" "${sql}"`, { encoding: 'utf8' });
	return out.trim() ? JSON.parse(out) : [];
};

/** 注册扩展到 mock pi，返回 { handlers, messages, execCalls } */
const setup = (router) => {
	const handlers = {};
	const messages = [];
	const execCalls = [];
	qualityGate({
		on: (e, h) => { handlers[e] = h; },
		exec: async (cmd, args, opts) => { execCalls.push(`${cmd} ${args.join(' ')}`); return router(cmd, args, opts); },
		sendMessage: (m, o) => { messages.push({ m, o }); },
	});
	return { handlers, messages, execCalls };
};

(async () => {
	// ============ 场景 A：正常态（探测通过 → 既有门禁行为不变） ============
	{
		const repo = mkrepo();
		let fileAdded = false;
		const router = async (cmd, args) => {
			if (cmd === 'git') return gitRouter(repo, fileAdded)(cmd, args);
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
	}

	// ============ 场景 B：探测故障短路（fail-open + 记账 + 不双写） ============
	{
		const repo = mkrepo();
		let fileAdded = false;
		const router = async (cmd, args) => {
			if (cmd === 'git') return gitRouter(repo, fileAdded)(cmd, args);
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
		const repo = mkrepo();
		let fileAdded = false;
		let turn = 1;
		let cmdCallsTurn2 = 0;
		const VSOCK = '<3>WSL (1751 - ) ERROR: UtilAcceptVsock:251: accept4 failed 110';
		const router = async (cmd, args) => {
			if (cmd === 'git') return gitRouter(repo, fileAdded)(cmd, args);
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
		check('C3 环境归因 steer（interop-failure 含 UtilAcceptVsock）', messages.some(({ m }) => m.customType === 'quality-gate-interop-failure' && m.content.includes('UtilAcceptVsock')));
		check('C4 不混入门禁失败 steer（无 [回归]/[中间态]）', !messages.some(({ m }) => m.customType === 'quality-gate-failure'));
		turn = 2;
		await handlers['turn_end'](CHAT_TURN, mkctx('smoke-c')); // 纯对话：粘性豁免
		check('C5 粘性豁免：下 turn 纯对话零 cmd.exe 调用', cmdCallsTurn2 === 0);
	}

	// ============ 场景 D：真实失败粘性（既有语义回归） ============
	{
		const repo = mkrepo();
		let fileAdded = false;
		let turn = 1;
		let lintReran = 0;
		const router = async (cmd, args) => {
			if (cmd === 'git') return gitRouter(repo, fileAdded)(cmd, args);
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
		check('D2 真实失败 steer 含 (wsl环境) 标注', messages.some(({ m }) => m.customType === 'quality-gate-failure' && m.content.includes('(wsl环境)')));
		turn = 2;
		await handlers['turn_end'](CHAT_TURN, mkctx('smoke-d')); // 纯对话：粘性重跑
		check('D3 粘性重跑：纯对话 turn 仍重跑失败命令（既有语义）', lintReran === 1);
	}

	let fail = 0;
	for (const [name, ok] of checks) {
		console.log(`${ok ? '✅' : '❌'} ${name}`);
		if (!ok) fail++;
	}
	console.log(fail ? `\n${fail} 项失败` : '\nSMOKE OK');
	if (fail) process.exitCode = 1;
})().catch((e) => { console.error('SMOKE FAIL', e); process.exitCode = 1; });
