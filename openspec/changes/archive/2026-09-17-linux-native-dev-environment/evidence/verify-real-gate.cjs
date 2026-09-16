// 真实环境端到端验证：用真实 child_process 执行，跑一次真实 turn_end，
// 断言 quality-gate 在 Linux 上的判定、命令构造、cwd 传递与失败文案。
// 与 behavior smoke 的区别：那里 exec 是 mock，这里全部真跑。
const fs = require('fs');
const os = require('os');
const path = require('path');
const { spawn, execSync } = require('node:child_process');
const { DatabaseSync } = require('node:sqlite');

const qualityGate = require('/tmp/qg-real.cjs').default;

const checks = [];
const check = (name, ok, extra = '') => checks.push([name, ok, extra]);

/** 真实 pi.exec：child_process.spawn，记录每次调用 */
const execCalls = [];
const realExec = (cmd, args, opts) =>
	new Promise((resolve) => {
		execCalls.push({ cmd, args, cwd: opts?.cwd });
		const p = spawn(cmd, args, { cwd: opts?.cwd });
		let stdout = '', stderr = '';
		p.stdout?.on('data', (d) => (stdout += d));
		p.stderr?.on('data', (d) => (stderr += d));
		p.on('error', (e) => resolve({ code: 1, stdout, stderr: String(e) }));
		p.on('close', (c) => resolve({ code: c ?? 0, stdout, stderr }));
	});

(async () => {
	const repo = fs.mkdtempSync(path.join(os.tmpdir(), 'realgate-'));
	fs.mkdirSync(path.join(repo, 'backend-go'), { recursive: true });
	fs.mkdirSync(path.join(repo, 'front'), { recursive: true });
	execSync('git init -q && git config user.email t@t && git config user.name t', { cwd: repo });
	// 必须有初始 commit：quality-gate 的基线用 `git diff HEAD`，无 HEAD 时命令失败 → 门禁静默放行
	execSync('git commit -q --allow-empty -m init', { cwd: repo });

	const messages = [];
	const handlers = {};
	qualityGate({
		on: (e, h) => { handlers[e] = h; },
		exec: realExec,
		sendMessage: (m, o) => { messages.push({ m, o }); },
	});

	const ctx = { signal: undefined, sessionManager: { getSessionId: () => 'real-check' } };
	// 扩展的 git 调用不带 cwd（真实 pi 进程 cwd 就是项目根）——验证需切到 tmp repo
	process.chdir(repo);
	await handlers['session_start']({ reason: 'new' }, ctx);

	// 触发后端门禁：新增一个 .go 文件（相对基线的新变化）
	fs.writeFileSync(path.join(repo, 'backend-go', 'a.go'), 'package x\n');
	const t0 = Date.now();
	await handlers['turn_end']({ toolResults: [{ toolName: 'edit' }] }, ctx);
	const elapsed = Date.now() - t0;

	console.log('=== 全部 exec 调用 ===');
	for (const c of execCalls) console.log(`  [${c.cmd}] ${c.args.join(' ').slice(0, 80)} cwd=${c.cwd ?? '-'}`);
	console.log('=== git 实际输出 ===');
	console.log('ls-files --others:', execSync('git ls-files --others --exclude-standard', { cwd: repo }).toString().trim());
	console.log('diff --name-only HEAD:', JSON.stringify(execSync('git diff --name-only HEAD', { cwd: repo }).toString()));
	console.log('rev-parse --show-toplevel:', execSync('git rev-parse --show-toplevel', { cwd: repo }).toString().trim(), '(repo =', repo, ')');
	console.log('a.go 存在:', fs.existsSync(path.join(repo, 'backend-go', 'a.go')));

	const cmdExeCalls = execCalls.filter((c) => c.cmd === 'cmd.exe');
	const bashCalls = execCalls.filter((c) => c.cmd === 'bash');
	const gateCmds = bashCalls.filter((c) => !String(c.args).includes('change-scope'));

	console.log(`真实 turn_end 耗时: ${elapsed}ms`);
	console.log(`exec 总调用: ${execCalls.length}（cmd.exe: ${cmdExeCalls.length}）`);
	console.log('门禁命令实际调用:');
	for (const c of gateCmds) console.log(`  [${c.cmd}] ${c.args.join(' ').slice(0, 70)}  cwd=${c.cwd}`);

	check('R1 零 cmd.exe 调用（真实环境不探测）', cmdExeCalls.length === 0);
	check('R2 门禁经本机 bash 真实执行', gateCmds.length >= 1, `实际 ${gateCmds.length} 条`);
	check('R3 命令 cwd 定位于 backend-go', gateCmds.length > 0 && gateCmds.every((c) => (c.cwd ?? '').endsWith('backend-go')));
	check('R4 执行的是本机工具链（非 Windows 绝对路径）',
		gateCmds.some((c) => String(c.args).includes('golangci-lint run')) && !gateCmds.some((c) => String(c.args).includes('D:\\')));

	const db = new DatabaseSync(path.join(repo, '.pi', 'harness', 'events.db'), { readOnly: true });
	const gcs = db.prepare("SELECT payload FROM events WHERE kind='gate.check'").all();
	const pds = db.prepare("SELECT payload FROM events WHERE kind='policy.decision'").all();
	db.close();

	check('R5 gate.check 真实落库', gcs.length >= 1, `实际 ${gcs.length} 条`);
	check('R6 零 policy.decision(interop-down)', !pds.some((p) => p.includes('interop-down')));
	// tmp repo 无 go.mod → lint 失败 → 短路哨兵应跳过 vet/build（设计与平台无关，native 下同样生效）
	check('R9 lint 失败触发短路哨兵（vet/build 未执行）',
		gcs.length === 1 && !gateCmds.some((c) => String(c.args).includes('go vet')), `gate.check=${gcs.length}`);

	const failMsgs = messages.filter((x) => x.m.customType === 'quality-gate-failure');
	const envMsgs = messages.filter((x) => x.m.customType === 'quality-gate-interop-failure');
	check('R7 无环境故障 steer（native 不产生）', envMsgs.length === 0);
	check('R8 失败 steer 标注 (本机) 且不含 wsl环境',
		failMsgs.length > 0 && failMsgs[0].m.content.includes('(本机)') && !failMsgs[0].m.content.includes('wsl环境'));
	if (failMsgs.length) console.log('\n失败 steer 首行示例:\n' + failMsgs[0].m.content.split('\n').slice(0, 3).join('\n'));

	console.log('');
	let fail = 0;
	for (const [name, ok, extra] of checks) {
		console.log(`${ok ? '✅' : '❌'} ${name}${extra && !ok ? ` (${extra})` : ''}`);
		if (!ok) fail++;
	}
	console.log(fail ? `\n${fail} 项失败` : '\n真实环境验证全过 ✅');
	process.exitCode = fail ? 1 : 0;
	fs.rmSync(repo, { recursive: true, force: true });
})().catch((e) => { console.error('VERIFY FAIL', e); process.exitCode = 1; });
