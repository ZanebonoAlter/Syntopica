// policy-decision smoke（harden-harness-policy-and-spill）：helper 有界收敛（normalizeDecision
// 白盒）+ logPolicyDecision 集成（change 归属三分支/写入失败旁路）+ quota-gate 行为（block/
// fail-open/fuzzy-warn/健康放行零记录/target 安全）+ test-scope-guard 行为（soft warn/hard
// block/off 与未命中/归档语境零记录）+ quality-gate/entry-gate 与 policy.decision 隔离复核。
// 由 run-harness-smoke.sh 调用（先 esbuild 产出 .pdec.cjs / .qgate.cjs / .tsg.cjs）。断言失败 exit 1。
const fs = require('fs');
const os = require('os');
const path = require('path');
const { execSync } = require('child_process');

// node:sqlite ExperimentalWarning 抑制（与 lib/harness-log 同策略，保输出干净）
const priorWarn = process.listeners('warning');
process.removeAllListeners('warning');
process.on('warning', (w) => {
	if (!(w.name === 'ExperimentalWarning' && /sqlite/i.test(w.message))) {
		for (const l of priorWarn) l(w);
	}
});
const { DatabaseSync } = require('node:sqlite');

const checks = [];
const check = (name, ok) => checks.push([name, ok]);
const mktmp = () => fs.mkdtempSync(path.join(os.tmpdir(), 'pdec-smoke-'));
const dbOf = (cwd) => path.join(cwd, '.pi', 'harness', 'events.db');
const policyRows = (cwd) => {
	if (!fs.existsSync(dbOf(cwd))) return [];
	const db = new DatabaseSync(dbOf(cwd));
	const rows = db.prepare("SELECT * FROM events WHERE kind='policy.decision' ORDER BY id").all();
	db.close();
	return rows.map((r) => ({ ...r, payload: JSON.parse(r.payload) }));
};
/** 重载 bundle（quota-gate 有模块级 cache/inflight，每场景需干净实例） */
const loadFresh = (p) => {
	delete require.cache[require.resolve(p)];
	return require(p);
};
function makePi() {
	const handlers = {};
	return { pi: { on: (name, fn) => (handlers[name] = fn) }, handlers };
}

(async () => {
	const tmps = [];
	try {
		/* ---------- 1. normalizeDecision 白盒：action 白名单 / reasonCode 有界 / target / durationMs ---------- */
		const { normalizeDecision, logPolicyDecision, UNKNOWN_REASON_CODE } = loadFresh('./.pdec.cjs');
		for (const a of ['block', 'warn', 'bypass', 'fail-open']) {
			const r = normalizeDecision({ policy: 'spec-gate', action: a, reasonCode: 'full-go-test' });
			check(`四种合法 action 入白名单（${a}）`, r.ok === true && r.payload.action === a);
		}
		check('action 白名单外（"allow"）拒绝 → ok:false', normalizeDecision({ policy: 'x', action: 'allow', reasonCode: 'r' }).ok === false);
		check('action 空串拒绝', normalizeDecision({ policy: 'x', action: '', reasonCode: 'r' }).ok === false);
		check('policy 非字符串拒绝', normalizeDecision({ policy: 42, action: 'warn', reasonCode: 'r' }).ok === false);
		// reasonCode：合法 kebab-case 保留；空串/大写/空格/特殊字符/超长一律归一 unknown（不写自由文本）
		const kebab = normalizeDecision({ policy: 'quota-gate', action: 'block', reasonCode: 'quota-exhausted' });
		check('合法 kebab-case reasonCode 原样保留', kebab.ok && kebab.payload.reasonCode === 'quota-exhausted');
		for (const bad of ['', 'BadCode', 'has space', 'a_b', '中文代码', 'x'.repeat(65)]) {
			const r = normalizeDecision({ policy: 'x', action: 'warn', reasonCode: bad });
			const label = bad === '' ? '空串' : bad === 'x'.repeat(65) ? '超长65' : bad;
			check(`非法 reasonCode 归一 unknown（${label}）`, r.ok && r.payload.reasonCode === UNKNOWN_REASON_CODE);
		}
		// target：null/省略 → 无字段；超长截断有界
		const noTarget = normalizeDecision({ policy: 'x', action: 'warn', reasonCode: 'r', target: null });
		check('target=null → payload 省略 target 字段', noTarget.ok && !('target' in noTarget.payload));
		const longTarget = normalizeDecision({ policy: 'x', action: 'warn', reasonCode: 'r', target: 'z'.repeat(500) });
		check('target 超长截断至 120+省略号', longTarget.ok && longTarget.payload.target.length === 121 && String(longTarget.payload.target).endsWith('…'));
		// durationMs：负数/NaN 省略；合法取整
		const noDur = normalizeDecision({ policy: 'x', action: 'warn', reasonCode: 'r', durationMs: -1 });
		check('durationMs 负数省略', noDur.ok && !('durationMs' in noDur.payload));
		const dur = normalizeDecision({ policy: 'x', action: 'warn', reasonCode: 'r', durationMs: 12.6 });
		check('durationMs 合法值取整保留', dur.ok && dur.payload.durationMs === 13);

		/* ---------- 2. logPolicyDecision 集成：change 归属三分支 + 写入失败旁路 ---------- */
		const t2 = mktmp(); tmps.push(t2);
		fs.mkdirSync(path.join(t2, 'openspec/changes/fake-active-change'), { recursive: true }); // 自动检测锚点
		logPolicyDecision(t2, { sessionId: 's1', policy: 'spec-gate', action: 'block', reasonCode: 'archive-check-failed', change: 'explicit-change' });
		logPolicyDecision(t2, { sessionId: 's1', policy: 'quota-gate', action: 'warn', reasonCode: 'fuzzy-model-resolve' }); // undefined → 自动检测
		logPolicyDecision(t2, { sessionId: 's1', policy: 'test-scope-guard', action: 'block', reasonCode: 'full-go-test', change: null }); // null → 明确不绑定
		const rows2 = policyRows(t2);
		check('显式 change 优先绑定', rows2.length >= 1 && rows2[0].change === 'explicit-change');
		check('change=undefined 自动检测活跃 change', rows2.length >= 2 && rows2[1].change === 'fake-active-change');
		check('change=null 明确不绑定', rows2.length === 3 && rows2[2].change === null);
		// 写入失败旁路（D3）：.pi/harness 被文件占位 → openDb 失败 → 返回 false 不抛异常
		const t2b = mktmp(); tmps.push(t2b);
		fs.mkdirSync(path.join(t2b, '.pi'), { recursive: true });
		fs.writeFileSync(path.join(t2b, '.pi/harness'), '占位文件');
		let sideEffectOnly = true;
		try {
			sideEffectOnly = logPolicyDecision(t2b, { sessionId: 's1', policy: 'x', action: 'block', reasonCode: 'r' }) === false;
		} catch {
			sideEffectOnly = false;
		}
		check('记账故障只返回 false（不抛异常、不改调用方控制流）', sideEffectOnly);

		/* ---------- 3. quota-gate 行为：block / fail-open / fuzzy-warn / 健康放行零记录 ---------- */
		// mock fetch：GLM 适配器响应形态；scenario 控制剩余百分比或抛错
		const quotaScenario = async (cwd, fetchImpl, input) => {
			const realFetch = globalThis.fetch;
			globalThis.fetch = fetchImpl;
			try {
				const mod = loadFresh('./.qgate.cjs');
				const { pi, handlers } = makePi();
				mod.default(pi);
				return await handlers['tool_call'](
					{ toolName: 'Agent', input, toolCallId: 'qc1' },
					{
						model: { provider: 'zai-coding-cn' },
						modelRegistry: {
							getAvailable: () => [
								{ id: 'glm-x', provider: 'opencode-go' },
								{ id: 'glm-x', provider: 'zai-coding-cn' },
							],
							getProviderAuth: async () => ({ auth: { apiKey: 'sk-test-secret' } }),
						},
						ui: { notify() {} },
						sessionManager: { getSessionId: () => 'pq1' },
						cwd,
					},
				);
			} finally {
				globalThis.fetch = realFetch;
			}
		};
		const glmResp = (pct) => async () => ({
			ok: true,
			text: async () => JSON.stringify({ data: { limits: [{ type: 'TOKENS_LIMIT', percentage: pct, nextResetTime: Date.now() + 3600e3 }] } }),
		});

		// 3a. 健康放行（剩 50%）：undefined + 零 policy.decision（低噪声）
		const t3a = mktmp(); tmps.push(t3a);
		const r3a = await quotaScenario(t3a, glmResp(50), { subagent_type: 'Explore' });
		check('quota 健康 → 放行 undefined', r3a === undefined);
		check('quota 健康放行零 policy.decision（低噪声）', policyRows(t3a).length === 0);

		// 3b. low 阻断（剩 5%）：block + block 事件（target 仅 provider，无 key/响应正文）
		const t3b = mktmp(); tmps.push(t3b);
		const r3b = await quotaScenario(t3b, glmResp(95), { subagent_type: 'Explore' });
		const rows3b = policyRows(t3b);
		check('quota low → block true + reason 含额度', r3b && r3b.block === true && /额度不足/.test(r3b.reason));
		check('quota low → policy.decision(block, quota-low) 恰一条', rows3b.length === 1 && rows3b[0].payload.action === 'block' && rows3b[0].payload.reasonCode === 'quota-low');
		check('quota target 仅含 provider 摘要', rows3b[0].payload.target === 'zai-coding-cn');
		const raw3b = JSON.stringify(rows3b[0].payload);
		check('quota payload 不含 API key 与响应正文', !raw3b.includes('sk-test-secret') && !raw3b.includes('TOKENS_LIMIT'));

		// 3c. 查询异常 fail-open：放行 + fail-open 事件
		const t3c = mktmp(); tmps.push(t3c);
		const r3c = await quotaScenario(t3c, async () => { throw new Error('network down'); }, { subagent_type: 'Explore' });
		const rows3c = policyRows(t3c);
		check('quota 查询失败 → fail-open 放行 undefined', r3c === undefined);
		check('quota 查询失败 → policy.decision(fail-open, quota-query-failed)', rows3c.length === 1 && rows3c[0].payload.action === 'fail-open' && rows3c[0].payload.reasonCode === 'quota-query-failed');

		// 3d. fuzzy 裸模型名 warn：解析到非默认 provider → warn 事件（此时无适配器/额度健康）
		const t3d = mktmp(); tmps.push(t3d);
		const r3d = await quotaScenario(t3d, glmResp(50), { model: 'glm-x', subagent_type: 'Explore' });
		const rows3d = policyRows(t3d);
		check('fuzzy 解析 + 无适配器 → 仍放行', r3d === undefined);
		check('fuzzy 裸模型名 → policy.decision(warn, fuzzy-model-resolve)', rows3d.length === 1 && rows3d[0].payload.action === 'warn' && rows3d[0].payload.reasonCode === 'fuzzy-model-resolve' && rows3d[0].payload.target === 'opencode-go');

		/* ---------- 4. test-scope-guard 判定矩阵与行为（change: harden-test-scope-guard，
		     用例源：openspec/changes/harden-test-scope-guard/test-cases.md B1–B8） ----------
		     注：表格期望 none/scoped 均为行为放行（§1 前置契约：scoped 与 none 行为等价），
		     非全量用例统一断 !== 'full'；full 用例精确断 === 'full'。 */
		// fixture：临时仓库根 + backend-go/internal 子目录 + git init（供真 git rev-parse 链路）
		const makeFixture = () => {
			const root = fs.realpathSync(mktmp()); tmps.push(root);
			execSync('git init -q', { cwd: root, stdio: ['ignore', 'ignore', 'ignore'] });
			fs.mkdirSync(path.join(root, 'backend-go/internal'), { recursive: true });
			return root;
		};
		// pi 桩：exec 走真 git（与 quality-gate 同源链路；resolveRepoRoot 依赖）
		const makeTsgPi = () => {
			const handlers = {};
			return {
				handlers,
				pi: {
					on: (name, fn) => (handlers[name] = fn),
					exec: (cmd, args, opts) => Promise.resolve({
						code: 0,
						stdout: execSync([cmd, ...args].join(' '), { cwd: opts && opts.cwd, encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }),
					}),
				},
			};
		};
		const tsgMod = loadFresh('./.tsg.cjs');
		const { classifyTestRun } = tsgMod;
		const ROOT4 = makeFixture();
		const BW4 = path.join(ROOT4, 'backend-go');
		const BWI4 = path.join(BW4, 'internal');
		/** 钩子级场景：toolName/input/会话条目/模式可注入；默认 cwd = fixture 根（命令自带 cd backend-go） */
		const tsgRun = async ({ root, cwd, toolName = 'bash', input, entries = [], mode } = {}) => {
			if (!cwd) cwd = root;
			if (mode) return tsgChild({ root, cwd, toolName, input, entries, mode });
			const { pi, handlers } = makeTsgPi();
			const notices = [];
			tsgMod.default(pi);
			const result = await handlers['tool_call'](
				{ toolName, input, toolCallId: 'tc1' },
				{
					ui: { notify: (m) => notices.push(m) },
					sessionManager: { getSessionId: () => 'ts1', getEntries: () => entries, buildContextEntries: () => entries },
					cwd,
				},
			);
			return { result, notices };
		};
		// hard / off 是模块加载时常量（TEST_SCOPE_GUARD），用子进程逐模式验证
		const tsgChild = ({ root, cwd, toolName, input, entries, mode }) => {
			const script = [
				'process.env.TEST_SCOPE_GUARD=' + JSON.stringify(mode) + ';',
				'const cp=require("child_process");',
				'const mod=require(' + JSON.stringify(path.resolve('.tsg.cjs')) + ');',
				'const handlers={};',
				'mod.default({on:(n,f)=>handlers[n]=f,exec:(c,a,o)=>Promise.resolve({code:0,stdout:cp.execSync([c,...a].join(" "),{cwd:o&&o.cwd,encoding:"utf8",stdio:["ignore","pipe","ignore"]})})});',
				'const entries=' + JSON.stringify(entries) + ';',
				'handlers["tool_call"]({toolName:' + JSON.stringify(toolName) + ',input:' + JSON.stringify(input) + ',toolCallId:"t"},',
				'  {ui:{notify(){}},sessionManager:{getSessionId:()=>"ts2",getEntries:()=>entries,buildContextEntries:()=>entries},cwd:' + JSON.stringify(cwd) + '})',
				'.then((r)=>console.log(JSON.stringify({blocked:!!(r&&r.block),reason:(r&&r.reason)||null})));',
			].join('');
			const out = execSync('node -e ' + JSON.stringify(script), { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
			return { result: JSON.parse(out), notices: [] };
		};
		/** 白盒断言：full 精确断，非全量（表格 none/scoped）断 !== 'full' */
		const checkClassify = (id, expectFull, command, sessionCwd) => {
			const v = classifyTestRun(command, sessionCwd || ROOT4, ROOT4);
			check(id + ' → ' + (expectFull ? 'full' : '非全量（放行）'), expectFull ? v === 'full' : v !== 'full');
		};

		// ---- B1 调用段定位（切分/前缀/嵌套 -c；cwd=仓库根） ----
		checkClassify('TC-B1-01', true, 'cd backend-go && go test ./...');
		checkClassify('TC-B1-02', true, 'cd backend-go && timeout 600 go test ./...');
		checkClassify('TC-B1-03', true, 'cd backend-go && TESTCONTAINERS_RYUK_DISABLED=true go test ./...');
		checkClassify('TC-B1-04', true, 'cd backend-go && env FOO=1 go test -short ./...');
		checkClassify('TC-B1-05', true, 'cd backend-go && go test ./internal/reader/... && go test ./...');
		checkClassify('TC-B1-06', true, "cd backend-go && timeout 1500 bash -c 'golangci-lint run ./... && go test ./...'");
		checkClassify('TC-B1-07', false, 'cd backend-go && go test -short ./internal/... | tail -5');
		checkClassify('TC-B1-08', false, 'cd backend-go && golangci-lint run ./... || go vet ./...');
		checkClassify('TC-B1-09', true, 'cd backend-go && go test ./... > /tmp/x.log 2>&1; echo done');
		checkClassify('TC-B1-10', true, 'cd backend-go && gofmt -l . && go build ./... && go test ./...');
		checkClassify('TC-B1-11', false, 'cd backend-go && go test');
		checkClassify('TC-B1-12', true, '(cd backend-go && go test ./...)');
		checkClassify('TC-B1-13', true, "bash -c 'cd backend-go && go test ./...'");

		// ---- B2 包模式提取（cwd=backend-go，前缀省略） ----
		checkClassify('TC-B2-01', true, 'go test ./...', BW4);
		checkClassify('TC-B2-02', false, 'go test -short ./internal/reader/...', BW4);
		checkClassify('TC-B2-03', false, 'go test -short ./internal/... ./cmd/...', BW4);
		checkClassify('TC-B2-04', false, 'go test ./internal/platform/articlerefs/... ./internal/reader/...', BW4);
		checkClassify('TC-B2-05', false, 'go test -count=1 ./internal/admin/scheduler/', BW4);
		checkClassify('TC-B2-06', true, "go test -run 'TestFoo' ./...", BW4);
		checkClassify('TC-B2-07', false, "go test -run 'TestDeleteFeedCascadeHandlesFeedWithoutArticles' ./internal/reader/repository/", BW4);
		checkClassify('TC-B2-08', true, 'go test -v -count=1 -short ./...', BW4);
		checkClassify('TC-B2-09', true, 'go test ./... ./internal/...', BW4);
		checkClassify('TC-B2-10', false, 'go test "$PKGS"', BW4);

		// ---- B3 引号遮蔽、行内注释与转义 ----
		checkClassify('TC-B3-01', false, 'grep -rn "go test ./..." docs/');
		checkClassify('TC-B3-02', false, "rg 'go test ./\\.\\.\\.' docs/reference/");
		checkClassify('TC-B3-03', false, 'echo "run go test ./... in backend-go"');
		checkClassify('TC-B3-04', true, 'cd backend-go && go test ./... # 全量回归');
		checkClassify('TC-B3-05', false, 'cd backend-go && echo hi # go test ./...');
		checkClassify('TC-B3-06', true, 'cd backend-go && go test ./... ; echo "done # not a comment inside quotes"');
		checkClassify('TC-B3-07', false, 'cd backend-go && echo "unclosed && go test ./..."');
		checkClassify('TC-B3-08', true, "cd backend-go && echo 'a\\'b' && go test ./...");
		checkClassify('TC-B3-09', false, 'cat > /tmp/x.md <<EOF\ncd backend-go && go test ./...');

		// ---- B4 生效 cwd 解析（会话 cwd 变体） ----
		checkClassify('TC-B4-01', true, 'go test ./...', BW4);
		checkClassify('TC-B4-02', false, 'go test ./...', ROOT4);
		checkClassify('TC-B4-03', true, 'cd backend-go && go test ./...', ROOT4);
		checkClassify('TC-B4-04', true, 'cd ' + BW4 + ' && go test ./...', ROOT4);
		checkClassify('TC-B4-05', false, 'cd backend-go/ && go test -short ./internal/reader/...', ROOT4);
		checkClassify('TC-B4-06', false, 'cd backend-go/internal/domain/x && go test ./...', ROOT4);
		checkClassify('TC-B4-07', false, 'cd backend-go && cd internal && go test ./...', ROOT4);
		checkClassify('TC-B4-08', false, 'cd "$DIR" && go test ./...', ROOT4);
		checkClassify('TC-B4-09', false, 'cd backend-go && go test ./...', BW4);
		checkClassify('TC-B4-10', true, 'cd .. && go test ./...', BWI4);

		// ---- B5 文本掩蔽（heredoc / 重定向） ----
		checkClassify('TC-B5-01', false, 'cd ' + ROOT4 + " && cat >> openspec/changes/x/apply-report.md <<'EOF'\nsome text go test ./internal/reader/...\nEOF", ROOT4);
		checkClassify('TC-B5-02', false, 'cat > /tmp/x.md <<EOF\ncd backend-go && go test ./...\nEOF', ROOT4);
		checkClassify('TC-B5-03', false, "printf 'cd backend-go && go test ./...\\n' > /tmp/y.sh", ROOT4);
		checkClassify('TC-B5-04', true, "cd backend-go && go test ./... && cat > report.txt <<'EOF'\nok\nEOF", ROOT4);
		checkClassify('TC-B5-05', false, 'cat go.mod', ROOT4);
		checkClassify('TC-B5-06', true, 'cd backend-go && go test ./... | cat > /tmp/log', ROOT4);

		// ---- B6 通道 × 模式矩阵（行为级，独立 fixture 隔离记账） ----
		const FULL_CMD = 'cd backend-go && go test ./...';
		{
			const root = makeFixture();
			const { result, notices } = await tsgRun({ root, toolName: 'bash', input: { command: FULL_CMD } });
			check('TC-B6-01 bash+soft：提醒 1 次 + warn 记账 1 条 + 放行', result === undefined && notices.length === 1 && policyRows(root).length === 1);
		}
		{
			const root = makeFixture();
			const { result, notices } = await tsgRun({ root, toolName: 'ctx_execute', input: { language: 'shell', code: FULL_CMD } });
			check('TC-B6-02 ctx_execute(shell)+soft：与 bash 一致（现状盲区回归）', result === undefined && notices.length === 1 && policyRows(root).length === 1);
		}
		{
			const root = makeFixture();
			const { result, notices } = await tsgRun({ root, toolName: 'ctx_batch_execute', input: { commands: [{ label: 'a', command: FULL_CMD }, { label: 'b', command: 'go vet ./...' }] } });
			check('TC-B6-03 ctx_batch_execute：命中命令触发，提醒 1 次 + 记账 1 条', result === undefined && notices.length === 1 && policyRows(root).length === 1);
		}
		{
			const root = makeFixture();
			const { result, notices } = await tsgRun({ root, toolName: 'ctx_execute', input: { language: 'javascript', code: "execSync('go test ./...', {cwd:'backend-go'})" } });
			check('TC-B6-04 ctx_execute(js)：零提醒零记账（非 shell 不解析）', result === undefined && notices.length === 0 && policyRows(root).length === 0);
		}
		{
			const root = makeFixture();
			const { result, notices } = await tsgRun({ root, toolName: 'ctx_execute', input: { language: 'python', code: "subprocess.run('go test ./...')" } });
			check('TC-B6-05 ctx_execute(python)：零提醒零记账', result === undefined && notices.length === 0 && policyRows(root).length === 0);
		}
		{
			const root = makeFixture();
			const { result } = await tsgRun({ root, mode: 'hard', toolName: 'bash', input: { command: FULL_CMD } });
			check('TC-B6-06 bash+hard：block + reason 含两条逃生路径', result.blocked === true && result.reason.includes('# archive-gate') && result.reason.includes('# allow-full-test'));
			const rows = policyRows(root);
			check('TC-B6-06 bash+hard：policy.decision(block, full-go-test) 恰 1 条', rows.length === 1 && rows[0].payload.action === 'block' && rows[0].payload.reasonCode === 'full-go-test');
		}
		{
			const root = makeFixture();
			const { result } = await tsgRun({ root, mode: 'hard', toolName: 'ctx_execute', input: { language: 'shell', code: FULL_CMD } });
			check('TC-B6-07 ctx_execute(shell)+hard：通道一致 block', result.blocked === true);
		}
		{
			const root = makeFixture();
			const { result } = await tsgRun({ root, mode: 'off', toolName: 'bash', input: { command: FULL_CMD } });
			check('TC-B6-08 off：零提醒零记账零阻断', result.blocked === false && policyRows(root).length === 0);
		}
		{
			const root = makeFixture();
			const { result } = await tsgRun({ root, mode: 'hard', toolName: 'bash', input: { command: 'cd backend-go && go test -short ./internal/reader/...' } });
			check('TC-B6-09 hard 影响包：放行不拦（现状误报回归）', result.blocked === false && policyRows(root).length === 0);
		}

		// ---- B7 放行语境（soft 主进程） ----
		{
			const root = makeFixture();
			const { result, notices } = await tsgRun({ root, input: { command: FULL_CMD + ' # archive-gate' } });
			check('TC-B7-01 逃生注释 archive-gate：放行零记账 + info 指引', result === undefined && notices.length === 1 && policyRows(root).length === 0);
		}
		{
			const root = makeFixture();
			const { result, notices } = await tsgRun({ root, input: { command: FULL_CMD + ' # allow-full-test' } });
			check('TC-B7-02 逃生注释 allow-full-test：放行零记账（新别名等价）', result === undefined && notices.length === 1 && policyRows(root).length === 0);
		}
		{
			const root = makeFixture();
			const entries = [{ role: 'assistant', content: '准备归档，先跑归档门禁' }];
			const { result, notices } = await tsgRun({ root, entries, input: { command: FULL_CMD } });
			check('TC-B7-03 归档语境（会话条目）：放行零记账 + 附登记指引', result === undefined && notices.length === 1 && String(notices[0]).includes('test-patrol.sh --register') && policyRows(root).length === 0);
		}
		{
			const root = makeFixture();
			const entries = [{ role: 'tool', content: '依赖变更（go.mod/go.sum）：建议全量 go test ./...（不自动执行）' }];
			const { result, notices } = await tsgRun({ root, entries, input: { command: FULL_CMD } });
			check('TC-B7-04 依赖变更语境（change-scope 官方建议语）：放行零记账 + 附登记指引', result === undefined && notices.length === 1 && policyRows(root).length === 0);
		}
		{
			const root = makeFixture();
			const { result, notices } = await tsgRun({ root, input: { command: FULL_CMD + ' # 依赖变更后全量验证' } });
			check('TC-B7-05 命令自身含依赖变更文本（掩蔽前判定）：放行零记账', result === undefined && notices.length === 1 && policyRows(root).length === 0);
		}
		{
			const root = makeFixture();
			const { result, notices } = await tsgRun({ root, input: { command: FULL_CMD } });
			check('TC-B7-06 无放行标记：软提醒文案含归档与依赖变更两条合法路径', result === undefined && notices.length === 1 && String(notices[0]).includes('归档/pre-push') && String(notices[0]).includes('依赖变更') && policyRows(root).length === 1);
		}
		{
			const root = makeFixture();
			const entries = [{ role: 'assistant', content: '进入归档流程' }];
			const { result, notices } = await tsgRun({ root, entries, input: { command: 'cd backend-go && go test -short ./internal/reader/...' } });
			check('TC-B7-07 影响包 + 归档语境：零记账零提醒（语境不二次记账）', result === undefined && notices.length === 0 && policyRows(root).length === 0);
		}

		// ---- B8 记账与提醒行为（含低噪声与自噬防御） ----
		{
			const root = makeFixture();
			await tsgRun({ root, input: { command: FULL_CMD } });
			const rows = policyRows(root);
			const p = (rows[0] && rows[0].payload) || {};
			check('TC-B8-01 记账 payload 有界：仅 policy/action/reasonCode，无命令正文', rows.length === 1 && p.policy === 'test-scope-guard' && p.action === 'warn' && p.reasonCode === 'full-go-test' && !JSON.stringify(p).includes('go test'));
		}
		{
			const root = makeFixture();
			await tsgRun({ root, input: { command: FULL_CMD } });
			await tsgRun({ root, input: { command: FULL_CMD } });
			check('TC-B8-02 同会话两次真全量：2 条 warn（不做去重）', policyRows(root).length === 2);
		}
		{
			const root = makeFixture();
			const { pi, handlers } = makeTsgPi();
			tsgMod.default(pi);
			const result = await handlers['tool_call'](
				{ toolName: 'bash', input: { command: FULL_CMD }, toolCallId: 'tc1' },
				{ ui: { notify: () => { throw new Error('ui down'); } }, sessionManager: { getSessionId: () => 'ts1', getEntries: () => [], buildContextEntries: () => [] }, cwd: root },
			);
			check('TC-B8-03 UI 不可用且 soft：静默降级放行仍记 warn', result === undefined && policyRows(root).length === 1);
		}
		{
			const root = makeFixture();
			const { pi, handlers } = makeTsgPi();
			tsgMod.default(pi);
			const result = await handlers['tool_call'](
				{ toolName: 'bash', input: { command: FULL_CMD }, toolCallId: 'tc1' },
				{ ui: { notify: () => {} }, sessionManager: { getEntries: () => [], buildContextEntries: () => [] } },
			);
			check('TC-B8-04 无 cwd/sessionId：不记账不抛错（旁路语义）', result === undefined && policyRows(root).length === 0);
		}
		{
			const root = makeFixture();
			const { result, notices } = await tsgRun({ root, entries: [{ role: 'assistant', content: '归档语境' }], input: { command: FULL_CMD } });
			check('TC-B8-05 归档语境放行：1 条 info 指引（带守卫前缀）+ 零 policy.decision', result === undefined && notices.length === 1 && String(notices[0]).includes('[test-scope-guard]') && policyRows(root).length === 0);
		}
		{
			const root = makeFixture();
			const { result } = await tsgRun({ root, mode: 'hard', toolName: 'ctx_execute', input: { language: 'shell', code: FULL_CMD } });
			const rows = policyRows(root);
			check('TC-B8-06 ctx_execute(shell) hard 阻断：记账仅 1 条（block），不双写 warn', result.blocked === true && rows.length === 1 && rows[0].payload.action === 'block');
		}
		{
			const root = makeFixture();
			const cwd = root;
			// hard 是模块加载时常量：两次调用须在子进程内完成（第二次 entries 注入首次 block reason）
			const script = [
				'process.env.TEST_SCOPE_GUARD="hard";',
				'const cp=require("child_process");',
				'const mod=require(' + JSON.stringify(path.resolve('.tsg.cjs')) + ');',
				'const handlers={};',
				'mod.default({on:(n,f)=>handlers[n]=f,exec:(c,a,o)=>Promise.resolve({code:0,stdout:cp.execSync([c,...a].join(" "),{cwd:o&&o.cwd,encoding:"utf8",stdio:["ignore","pipe","ignore"]})})});',
				'const cwd=' + JSON.stringify(cwd) + ';',
				'const ctxOf=(entries)=>({ui:{notify(){}},sessionManager:{getSessionId:()=>"tsN",getEntries:()=>entries,buildContextEntries:()=>entries},cwd});',
				'(async()=>{',
				'  const r1=await handlers["tool_call"]({toolName:"bash",input:{command:' + JSON.stringify(FULL_CMD) + '},toolCallId:"t1"},ctxOf([]));',
				'  const reasonText=(r1&&typeof r1.reason==="string")?r1.reason:"";',
				'  const r2=await handlers["tool_call"]({toolName:"bash",input:{command:' + JSON.stringify(FULL_CMD) + '},toolCallId:"t2"},ctxOf([{role:"tool",content:reasonText}]));',
				'  console.log(JSON.stringify({firstBlocked:!!(r1&&r1.block),secondBlocked:!!(r2&&r2.block),reasonHasPrefix:reasonText.includes("[test-scope-guard]")}));',
				'})().catch((e)=>{console.log(JSON.stringify({error:String(e)}));});',
			].join('');
			const out = execSync('node -e ' + JSON.stringify(script), { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
			const r = JSON.parse(out);
			check('TC-B8-07 首次 hard block 生效', r.firstBlocked === true);
			check('TC-B8-07 自噬防御：含守卫前缀的 reason 条目不构成放行语境，仍 block', r.reasonHasPrefix === true && r.secondBlocked === true);
		}

		/* ---------- 5. 隔离复核（2.4，harden-gate-interop-health 修订；harden-gate-
		     native-toolchain / harden-subagent-constraint-channel 扩展；attribute-
		     concurrent-gate-noise 扩展）：quality-gate / entry-gate 只写 gate.check；
		     quality-gate 例外 = 环境短路 fail-open（interop-down 1 处 + toolchain-down
		     backend/frontend 2 处）+ 子线程降载 bypass（child-session 1 处）+ 外部失败
		     归因 warn（foreign-breakage 1 处，spec gate-failure-reporting「外部失败不进
		     粘性且记 warn 归因」），且被短路跳过的命令零 gate.check 双写 ---------- */
		const qg = fs.readFileSync(path.resolve('../quality-gate.ts'), 'utf8');
		const eg = fs.readFileSync(path.resolve('../entry-gate.ts'), 'utf8');
		check('quality-gate 源码 logPolicyDecision 恰五处（interop×1 + toolchain×2 + child-session bypass×1 + foreign-breakage×1）',
			(qg.match(/logPolicyDecision\(/g) ?? []).length === 5 &&
			qg.includes('reasonCode: "interop-down"') &&
			qg.includes('reasonCode: "toolchain-down"') &&
			qg.includes('reasonCode: "child-session"') &&
			qg.includes('"quality-gate"'));
		check('entry-gate 源码无 policy.decision 引用（同裁决无双写）', !eg.includes('policy.decision') && !eg.includes('logPolicyDecision'));

		let fail = 0;
		for (const [name, ok] of checks) {
			console.log(`${ok ? '✅' : '❌'} ${name}`);
			if (!ok) fail++;
		}
		console.log(fail ? `\n${fail} 项失败` : '\nSMOKE OK');
		if (fail) process.exitCode = 1;
	} catch (e) {
		console.error('FAIL', e);
		process.exitCode = 1;
	} finally {
		for (const t of tmps) fs.rmSync(t, { recursive: true, force: true });
	}
})();
