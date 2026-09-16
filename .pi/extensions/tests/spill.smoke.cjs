// tool-output-spill smoke：超阈值替换/未超直通/read 直通(D7)/写盘失败降级(D6)/30 天清扫(D5)/
// 非正数禁用(D2)/非文本块保留(D3)/UTF-8 边界(D3)。
// 由 run-harness-smoke.sh 调用（先 esbuild 产出 .spill.cjs）。断言失败 exit 1。
const fs = require('fs');
const os = require('os');
const path = require('path');

const priorWarn = process.listeners('warning');
process.removeAllListeners('warning');
process.on('warning', (w) => {
	if (!(w.name === 'ExperimentalWarning' && /sqlite/i.test(w.message))) {
		for (const l of priorWarn) l(w);
	}
});
const { DatabaseSync } = require('node:sqlite');

const ext = require('./.spill.cjs').default;

const mktmp = () => fs.mkdtempSync(path.join(os.tmpdir(), 'spill-smoke-'));
const dbOf = (cwd) => path.join(cwd, '.pi', 'harness', 'events.db');
const spillRows = (cwd) => {
	if (!fs.existsSync(dbOf(cwd))) return []; // 未产生任何事件 = 无库文件（合法态）
	const db = new DatabaseSync(dbOf(cwd));
	const rows = db.prepare("SELECT payload FROM events WHERE kind='spill.write' ORDER BY id").all();
	db.close();
	return rows.map((r) => JSON.parse(r.payload));
};

function makePi() {
	const handlers = {};
	return { pi: { on: (name, fn) => (handlers[name] = fn) }, handlers };
}
const ctxOf = (cwd, sid = 's1') => ({ cwd, sessionManager: { getSessionId: () => sid } });
const text = (n) => ({ type: 'text', text: 'x'.repeat(n) });

(async () => {
	const checks = [];
	const check = (name, ok) => checks.push([name, ok]);
	const tmps = [];
	const origCwd = process.cwd();

	try {
		// ---- 1. 超阈值替换：预览有界 + 文件落盘 + 记账 ----
		const t1 = mktmp(); tmps.push(t1);
		const { pi: pi1, handlers: h1 } = makePi();
		ext(pi1);
		const big = Buffer.alloc(80 * 1024, 'a').toString();
		const r1 = await h1['tool_result'](
			{ toolName: 'bash', toolCallId: 'c1', input: {}, content: [{ type: 'text', text: big }], isError: false },
			ctxOf(t1),
		);
		check('超阈值返回替换 content', r1 && Array.isArray(r1.content) && r1.content[0].type === 'text');
		const prev1 = r1.content[0].text;
		check('预览含取回路径与省略字节数', prev1.includes('.pi/harness/spill/s1/') && prev1.includes('省略'));
		check('预览有界（<4KB）', Buffer.byteLength(prev1, 'utf8') < 4096);
		const relPath = (prev1.match(/\.pi\/harness\/spill\/s1\/\S+\.txt/) || [])[0];
		check('spill 文件存在且为完整原文', relPath && fs.existsSync(path.join(t1, relPath)) && fs.statSync(path.join(t1, relPath)).size === 80 * 1024);
		const ev1 = spillRows(t1);
		check('spill.write 记账 ok:true', ev1.length === 1 && ev1[0].ok === true && ev1[0].tool === 'bash' && ev1[0].bytes === 80 * 1024);

		// ---- 2. 未超直通：无返回值、无文件、无记账 ----
		const t2 = mktmp(); tmps.push(t2);
		const { pi: pi2, handlers: h2 } = makePi();
		ext(pi2);
		const r2 = await h2['tool_result'](
			{ toolName: 'bash', toolCallId: 'c2', input: {}, content: [text(20 * 1024)], isError: false },
			ctxOf(t2),
		);
		check('未超阈值原样通过（无替换）', r2 === undefined);
		check('未超阈值无 spill 文件/记账', !fs.existsSync(path.join(t2, '.pi/harness/spill')) && spillRows(t2).length === 0);

		// ---- 3. read 取回直通（D7）：spill 路径 + 超大 → 不再 spill ----
		const r3 = await h1['tool_result'](
			{ toolName: 'read', toolCallId: 'c3', input: { path: relPath }, content: [{ type: 'text', text: big }], isError: false },
			ctxOf(t1),
		);
		check('read 取回 spill 文件直通（D7）', r3 === undefined);
		const r3b = await h1['tool_result'](
			{ toolName: 'read', toolCallId: 'c3b', input: { path: 'docs/normal.md' }, content: [{ type: 'text', text: big }], isError: false },
			ctxOf(t1),
		);
		check('read 普通路径仍会被 spill（防漏）', r3b !== undefined);

		// ---- 4. 写盘失败降级（D6）：session 目录被同名文件占位 → mkdir 抛错 ----
		const t4 = mktmp(); tmps.push(t4);
		fs.mkdirSync(path.join(t4, '.pi/harness/spill'), { recursive: true });
		fs.writeFileSync(path.join(t4, '.pi/harness/spill/s1'), '占位文件'); // 目录路径被文件挡住
		const { pi: pi4, handlers: h4 } = makePi();
		ext(pi4);
		const r4 = await h4['tool_result'](
			{ toolName: 'bash', toolCallId: 'c4', input: {}, content: [{ type: 'text', text: big }], isError: false },
			ctxOf(t4),
		);
		check('写盘失败原结果通过（不阻塞）', r4 === undefined);
		const ev4 = spillRows(t4);
		check('失败记账 ok:false 含 error', ev4.length === 1 && ev4[0].ok === false && typeof ev4[0].error === 'string');

		// ---- 5. 30 天清扫（D5）：旧文件删、新文件留、空会话目录回收 ----
		const t5 = mktmp(); tmps.push(t5);
		const old5 = path.join(t5, '.pi/harness/spill/old-sid/a.txt');
		const fresh5 = path.join(t5, '.pi/harness/spill/new-sid/b.txt');
		fs.mkdirSync(path.dirname(old5), { recursive: true });
		fs.mkdirSync(path.dirname(fresh5), { recursive: true });
		fs.writeFileSync(old5, 'old');
		fs.writeFileSync(fresh5, 'fresh');
		fs.utimesSync(old5, new Date(Date.now() - 31 * 86400e3), new Date(Date.now() - 31 * 86400e3));
		process.chdir(t5);
		ext(makePi().pi); // apply 时清扫 process.cwd()
		process.chdir(origCwd);
		check('30 天前 spill 文件被清扫', !fs.existsSync(old5));
		check('新鲜 spill 文件保留', fs.existsSync(fresh5));
		check('清扫后空会话目录回收', !fs.existsSync(path.join(t5, '.pi/harness/spill/old-sid')));

		// ---- 6. 显式非正数禁用（D2） ----
		const t6 = mktmp(); tmps.push(t6);
		fs.mkdirSync(path.join(t6, '.pi'), { recursive: true });
		fs.writeFileSync(path.join(t6, '.pi/harness.json'), '{"spillThresholdBytes": 0}');
		const { pi: pi6, handlers: h6 } = makePi();
		ext(pi6);
		const r6 = await h6['tool_result'](
			{ toolName: 'bash', toolCallId: 'c6', input: {}, content: [{ type: 'text', text: big }], isError: false },
			ctxOf(t6),
		);
		check('spillThresholdBytes=0 显式禁用直通', r6 === undefined);

		// ---- 7. 非文本块保留（D3）+ UTF-8 边界（D3） ----
		const t7 = mktmp(); tmps.push(t7);
		const { pi: pi7, handlers: h7 } = makePi();
		ext(pi7);
		const zh = '中文内容测试'.repeat(10 * 1024); // 80KB+，必然跨多字节边界
		const img = { type: 'image', data: 'fake', mimeType: 'image/png' };
		const r7 = await h7['tool_result'](
			{ toolName: 'playwright_browser_snapshot', toolCallId: 'c7', input: {}, content: [{ type: 'text', text: zh }, img], isError: false },
			ctxOf(t7),
		);
		check('图片块原样保留在替换后 content', r7.content.some((c) => c.type === 'image' && c === img));
		check('中文头尾切片无替换字符（UTF-8 边界安全）', !r7.content[0].text.includes('\uFFFD'));
		check('工具名进入记账（MCP 同链）', spillRows(t7)[0].tool === 'playwright_browser_snapshot');

		// ---- 8. 同毫秒同名工具并行唯一性（harden-harness-policy-and-spill D4）：
		//      固定 Date.now + 两个 toolCallId → 不同文件、各自内容完整、命名含确定性哈希且不暴露原始 ID ----
		const t8 = mktmp(); tmps.push(t8);
		const { pi: pi8, handlers: h8 } = makePi();
		ext(pi8);
		const realNow = Date.now;
		const fixedTs = realNow();
		Date.now = () => fixedTs; // 同毫秒
		const bufA = 'A'.repeat(60 * 1024);
		const bufB = 'B'.repeat(60 * 1024);
		const r8a = await h8['tool_result'](
			{ toolName: 'bash', toolCallId: 'secret-call-alpha-1111', input: {}, content: [{ type: 'text', text: bufA }], isError: false },
			ctxOf(t8, 'par'),
		);
		const r8b = await h8['tool_result'](
			{ toolName: 'bash', toolCallId: 'secret-call-beta-2222', input: {}, content: [{ type: 'text', text: bufB }], isError: false },
			ctxOf(t8, 'par'),
		);
		Date.now = realNow;
		const pathA = (r8a.content[0].text.match(/\.pi\/harness\/spill\/par\/\S+\.txt/) || [])[0];
		const pathB = (r8b.content[0].text.match(/\.pi\/harness\/spill\/par\/\S+\.txt/) || [])[0];
		check('同毫秒同名工具不同 toolCallId → 两个不同路径', pathA && pathB && pathA !== pathB);
		const fileA = path.basename(pathA);
		const fileB = path.basename(pathB);
		check('文件名格式 <ts>-<tool>-<16hex>.txt（确定性哈希）', /^\d+-bash-[0-9a-f]{16}\.txt$/.test(fileA) && /^\d+-bash-[0-9a-f]{16}\.txt$/.test(fileB));
		check('文件名不含原始 toolCallId（不泄露内部标识）', !fileA.includes('secret-call-alpha-1111') && !fileB.includes('secret-call-beta-2222'));
		check('两个文件各自保存完整原文（不覆盖）',
			fs.readFileSync(path.join(t8, pathA), 'utf8') === bufA && fs.readFileSync(path.join(t8, pathB), 'utf8') === bufB);
		check('确定性：相同 toolCallId 同哈希（与 9 对照）', fileA.split('-').pop().replace('.txt', '').length === 16);

		// POSIX 最小权限（D5）：目录 0700 + 新文件 0600（Windows 仅 best-effort，跳过 mode 断言）
		if (process.platform !== 'win32') {
			const dirMode = fs.statSync(path.join(t8, '.pi/harness/spill/par')).mode & 0o777;
			const fileMode = fs.statSync(path.join(t8, pathA)).mode & 0o777;
			check('POSIX：会话目录收敛 0700', dirMode === 0o700);
			check('POSIX：新写 spill 文件 0600', fileMode === 0o600);
		} else {
			check('Windows：mode 断言跳过（best-effort，命名与内容断言仍生效）', true);
		}

		// ---- 9. 排他创建不覆盖（D4）：同时间戳 + 同 toolCallId 重复 → 第二次 fail-open，原文不动 ----
		const t9 = mktmp(); tmps.push(t9);
		const { pi: pi9, handlers: h9 } = makePi();
		ext(pi9);
		Date.now = () => fixedTs;
		const r9a = await h9['tool_result'](
			{ toolName: 'bash', toolCallId: 'dup-call-9999', input: {}, content: [{ type: 'text', text: bufA }], isError: false },
			ctxOf(t9, 'dup'),
		);
		const r9b = await h9['tool_result'](
			{ toolName: 'bash', toolCallId: 'dup-call-9999', input: {}, content: [{ type: 'text', text: bufB }], isError: false },
			ctxOf(t9, 'dup'),
		);
		Date.now = realNow;
		const path9 = (r9a.content[0].text.match(/\.pi\/harness\/spill\/dup\/\S+\.txt/) || [])[0];
		check('同 toolCallId 同毫秒重复：第二次按写盘失败 fail-open（原结果直通）', r9b === undefined);
		check('排他创建不覆盖：既有文件内容仍是第一次原文', fs.readFileSync(path.join(t9, path9), 'utf8') === bufA);
		const ev9 = spillRows(t9);
		check('排他冲突记账：一次 ok:true + 一次 ok:false', ev9.length === 2 && ev9[0].ok === true && ev9[1].ok === false);

		// ---- 10. 既有目录权限收敛（D5）：0777 目录再 spill → 收回 0700，既有文件内容不改写 ----
		if (process.platform !== 'win32') {
			const t10 = mktmp(); tmps.push(t10);
			const dir10 = path.join(t10, '.pi/harness/spill/relax');
			fs.mkdirSync(dir10, { recursive: true });
			fs.chmodSync(dir10, 0o777); // 模拟历史宽松目录
			const { pi: pi10, handlers: h10 } = makePi();
			ext(pi10);
			await h10['tool_result'](
				{ toolName: 'bash', toolCallId: 'relax-call-1', input: {}, content: [{ type: 'text', text: bufA }], isError: false },
				ctxOf(t10, 'relax'),
			);
			check('既有目录权限收敛回 0700', (fs.statSync(dir10).mode & 0o777) === 0o700);
		} else {
			check('Windows：既有目录收敛断言跳过（best-effort）', true);
		}

		// ---- 11. POSIX 文件 chmod 失败：仅新文件权限收敛失败 → 清理新文件 + fail-open（D5/D6） ----
		if (process.platform !== 'win32') {
			const t11 = mktmp(); tmps.push(t11);
			const { pi: pi11, handlers: h11 } = makePi();
			const realChmodSync = fs.chmodSync;
			fs.chmodSync = (target, mode) => {
				if (mode === 0o600 && String(target).endsWith('.txt')) throw new Error('simulated spill file chmod failure');
				return realChmodSync(target, mode);
			};
			let r11;
			try {
				delete require.cache[require.resolve('./.spill.cjs')];
				const patchedExt = require('./.spill.cjs').default;
				patchedExt(pi11);
				r11 = await h11['tool_result'](
					{ toolName: 'bash', toolCallId: 'chmod-fail-11', input: {}, content: [{ type: 'text', text: bufA }], isError: false },
					ctxOf(t11, 'chmod-fail'),
				);
			} finally {
				fs.chmodSync = realChmodSync;
			}
			const dir11 = path.join(t11, '.pi/harness/spill/chmod-fail');
			const ev11 = spillRows(t11);
			check('文件 chmod 失败仅降级原结果直通', r11 === undefined);
			check('文件 chmod 失败后无新 spill 文件残留', !fs.existsSync(dir11) || fs.readdirSync(dir11).filter((f) => f.endsWith('.txt')).length === 0);
			check('文件 chmod 失败记账 ok:false 含失败原因',
				ev11.length === 1 && ev11[0].ok === false && ev11[0].tool === 'bash' && ev11[0].bytes === bufA.length
					&& typeof ev11[0].error === 'string' && ev11[0].error.includes('simulated spill file chmod failure'));
			check('文件 chmod mock 不影响会话目录收敛', (fs.statSync(dir11).mode & 0o777) === 0o700);
		} else {
			check('Windows：文件 chmod 失败分支跳过（best-effort）', true);
		}
	} finally {
		process.chdir(origCwd);
		for (const t of tmps) fs.rmSync(t, { recursive: true, force: true });
	}

	const failed = checks.filter(([, ok]) => !ok);
	for (const [name, ok] of checks) console.log(`${ok ? '✓' : '✗'} ${name}`);
	console.log(`\nspill smoke: ${checks.length - failed.length}/${checks.length} 通过`);
	if (failed.length) process.exit(1);
})();
