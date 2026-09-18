// failure-classify smoke（A4 白名单纯函数 + telemetry 集成）：
//   段一：classifyFailure —— 六类命中、有序优先、不中落 unknown 不透传原文、
//         diag 单行 ≤512B 剥控制字符、exitLike 提取、stage 三态判定
//   段二：harness-telemetry bundle 后回放 tool_result(Agent) —— 失败产出 failure 对象、
//         成功/取消不产出（isError 守卫）、事件落 tmp 事实库可查回
// 由 run-harness-smoke.sh 调用。断言失败 exit 1。
const fs = require('fs');
const os = require('os');
const path = require('path');

// node:sqlite ExperimentalWarning 抑制（与 lib/harness-log 同策略）
const priorWarn = process.listeners('warning');
process.removeAllListeners('warning');
process.on('warning', (w) => {
	if (!(w.name === 'ExperimentalWarning' && /sqlite/i.test(w.message))) {
		for (const l of priorWarn) l(w);
	}
});

const { classifyFailure, truncateDiag, truncateDiagGate, isInteropFailure, isToolNotFound, extractFailurePaths, isForeignFailure } = require('./.fcls.cjs');

const checks = [];
const check = (name, ok) => checks.push([name, ok]);

/* ---------- 段一：纯函数 ---------- */
{
	// quota-gate 真实 reason 形态（含 额度/剩余/窗口/重置/阻断）
	const quota = classifyFailure({
		errorText: '智谱 GLM（zai-coding-cn）额度不足，本次 Agent 派发已阻断。\n剩余情况：5h 窗口剩余 8%\n重置时间：2 小时后',
		started: undefined, // quota-gate 拦截：tool_call 未发生
	});
	check('quota-block 命中', quota.category === 'quota-block');
	check('quota 拦截 stage=dispatch', quota.stage === 'dispatch');
	check('quota diag 单行（首行）', !quota.diag.includes('\n') && quota.diag.includes('额度不足'));

	// 有序优先：timeout 文本同时含 model-error 关键词时 timeout 先中
	const both = classifyFailure({ errorText: 'tool timed out after 120000ms, provider retry exhausted', started: true, details: { status: 'error' } });
	check('有序表：timeout 优先于 model-error', both.category === 'timeout' && both.stage === 'run');

	const gate = classifyFailure({ errorText: '⚠️ 增量门禁未通过，请修复后继续', started: true, details: {} });
	check('gate-fail 命中', gate.category === 'gate-fail');

	const model = classifyFailure({ errorText: 'Error: 429 rate limit exceeded (provider overloaded)', started: true, details: { status: 'error' } });
	check('model-error 命中', model.category === 'model-error');

	const tool = classifyFailure({ errorText: 'bash: go: command not found (exit code 127)', started: true, details: { status: 'error' } });
	check('tool-error 命中', tool.category === 'tool-error');
	check('exitLike 提取 127', tool.exitLike === 127);

	// unknown：不含任何白名单关键词 → unknown 且不透传原文（仅有界 diag）
	const weird = '一种前所未见的奇怪错误形态xyzzy';
	const unk = classifyFailure({ errorText: weird, started: true, details: { status: 'error' } });
	check('不中落 unknown', unk.category === 'unknown');
	check('unknown 也不透传原文（diag 为有界摘要）', unk.diag.length <= 512 && unk.diag === weird.slice(0, unk.diag.replace(/…$/, '').length));

	// diag 截断：超长中文首行 → ≤512 字节 + 尾标
	const longLine = '错'.repeat(400); // 1200 字节
	const long = classifyFailure({ errorText: longLine, started: true, details: {} });
	check('diag ≤512 字节（中文 3B/字）', Buffer.byteLength(long.diag, 'utf8') <= 512);
	check('diag 截断带 …', long.diag.endsWith('…'));
	// 剥 ANSI 序列与控制字符（控制字符→空格，避免英文粘连）后取首个非空行
	const ctrl = truncateDiag('\n\n\x1b[31m第一\x07行\x1b[0m\n第二行');
	check('diag 剥 ANSI/控制字符压单行取首个非空行', ctrl === '第一 行');

	// stage=result：agent 已完成但结果装配失败
	const res = classifyFailure({ errorText: 'result assembly failed', started: true, details: { status: 'completed' } });
	check('status=completed → stage=result', res.stage === 'result');
	check('truncateDiag 空文本 → 空串', truncateDiag('') === '');

	// truncateDiagGate（harness-observability-fixes D2）：gate.check 记账专用，失败特征行优先
	const gateLint = truncateDiagGate('0 issues.\n# syntopica-backend/internal/topicgraph/service\n./foo.go:12:5: undefined: Foo');
	check('gate diag 跳过 "0 issues." 噪声命中编译错误锚点行', gateLint === '# syntopica-backend/internal/topicgraph/service');
	const gateTest = truncateDiagGate('? syntopica-backend/internal/topicgraph [no test files]\nFAIL syntopica-backend/internal/topicgraph [build failed]');
	check('gate diag 不丢 FAIL 行（go test）', gateTest === 'FAIL syntopica-backend/internal/topicgraph [build failed]');
	check('gate diag 无关键词回退首行', truncateDiagGate('一切正常输出\n没有任何问题') === '一切正常输出');
	check('gate diag 确定性（同输入同输出）', truncateDiagGate('a\nFAIL x') === truncateDiagGate('a\nFAIL x'));
	check('gate diag 复用截断规范（≤512B 单行）', truncateDiagGate('FAIL ' + '错'.repeat(400)).length <= 512 && Buffer.byteLength(truncateDiagGate('FAIL ' + '错'.repeat(400)), 'utf8') <= 512);
	check('gate diag 空文本 → 空串', truncateDiagGate('') === '');
}

/* ---------- 段一·I：isInteropFailure（harden-gate-interop-health，白盒边界值见 test-cases.md） ---------- */
{
	// 真实样本（events.db 2026-09-03 故障期实测形态）必中
	check('I1 真实 vsock 故障样本命中', isInteropFailure('<3>WSL (1751 - ) ERROR: UtilAcceptVsock:251: accept4 failed 110') === true);
	check('I1b stderr fd 变体（<2>/<4>）命中', isInteropFailure('<2>WSL (443452 - ) ERROR: UtilAcceptVsock:251: accept4 failed 110') === true);
	// 双关键字并集：仅前缀形态（无 UtilAcceptVsock 关键字）仍命中——单形态漂移兑底
	check('I2 仅 <N>WSL ERROR 前缀形态命中（并集锚定）', isInteropFailure('<3>WSL (34217 - ) ERROR: SomeOtherVsockFailure:99: X') === true);
	// 多行输出：前缀形态在行首（m 标志）命中
	check('I3 多行输出行首前缀命中', isInteropFailure('running gate...\n<3>WSL (1751 - ) ERROR: UtilAcceptVsock:251: accept4 failed 110\ndone') === true);
	// 反例：正常门禁输出不命中
	check('I4 编译错误不命中（真实失败路径保持）', isInteropFailure('internal\\tagmanagement\\handler\\x.go:5:2: "strconv" imported and not used') === false);
	check('I5 空输出不命中', isInteropFailure('') === false);
	check('I6 普通 WSL 字样（非前缀形态）不命中', isInteropFailure('error: file at /mnt/d/project uses WSL path convention') === false);
	check('I7 行中非行首 WSL ERROR 不命中', isInteropFailure('log line mid text <3>WSL (1 - ) ERROR: X') === false);
	// 边界：null/undefined 输入不抛异常（防 defensive）
	check('I8 null/undefined 输入安全返回 false', isInteropFailure(null) === false && isInteropFailure(undefined) === false);
}

/* ---------- 段一·II：isToolNotFound（harden-gate-native-toolchain，native 工具链缺失特征） ---------- */
{
	// 真实样本（events.db 2026-09-17 事故实测形态）必中：bash: line 1: <exe>: command not found
	check('T1 go 缺失样本命中', isToolNotFound('bash: line 1: go: command not found') === true);
	check('T1b golangci-lint 缺失样本命中', isToolNotFound('bash: line 1: golangci-lint: command not found') === true);
	// 多行输出：特征行出现在中间（命令 stderr 前有正常 stdout）仍命中
	check('T2 多行输出中间命中', isToolNotFound('running gate...\nbash: line 1: go: command not found\ndone') === true);
	// 反例：真实代码失败不命中（保持既有粘性/分级语义）
	check('T3 lint 发现不命中', isToolNotFound('internal/reader/handler/opml.go:51:1: commentFormatting: put a space between `//` and comment text (gocritic)') === false);
	check('T4 测试失败不命中', isToolNotFound('FAIL syntopica-backend/internal/admin [build failed]') === false);
	check('T5 编译错误不命中', isToolNotFound('./foo.go:12:5: undefined: Foo') === false);
	check('T6 空输出不命中', isToolNotFound('') === false);
	// 边界：null/undefined 输入不抛异常（防 defensive，对齐 I8）
	check('T7 null/undefined 输入安全返回 false', isToolNotFound(null) === false && isToolNotFound(undefined) === false);
	// 语义划界：interop 特征不触发 toolchain 归因（两特征集正交）
	check('T8 interop 特征不命中 toolchain 归因', isToolNotFound('<3>WSL (1751 - ) ERROR: UtilAcceptVsock:251: accept4 failed 110') === false);
}

/* ---------- 段一·III：extractFailurePaths（attribute-concurrent-gate-noise，白盒用例 B1-B8） ---------- */
{
	// B1 golangci-lint 正斜杠文件锚点（path:LINE:COL:）
	check('B1 正斜杠文件锚点提取', JSON.stringify(extractFailurePaths('internal/tagmanagement/service/sourcestats/sourcestats.go:63:1: unused')) === JSON.stringify(['internal/tagmanagement/service/sourcestats/sourcestats.go']));
	// B2 Windows 反斜杠形态（实测存在）→ 归一化为 /
	check('B2 反斜杠形态归一为 /', JSON.stringify(extractFailurePaths('internal\\admin\\wire.go:97:1: File is not properly formatted (gofmt)')) === JSON.stringify(['internal/admin/wire.go']));
	// B3 go vet 包锚点 → 目录前缀（module 名 syntopica-backend 映射 backend-go/）
	check('B3 go vet 包锚点转目录前缀', JSON.stringify(extractFailurePaths('# syntopica-backend/internal/topicgraph/service')) === JSON.stringify(['backend-go/internal/topicgraph/service/']));
	// B4 go test FAIL 包锚点
	check('B4 go test FAIL 包锚点转目录前缀', JSON.stringify(extractFailurePaths('FAIL syntopica-backend/internal/admin/service [build failed]')) === JSON.stringify(['backend-go/internal/admin/service/']));
	// B5 eslint 绝对/相对双形态
	check('B5 eslint 绝对/相对双形态', JSON.stringify(extractFailurePaths('/home/u/repo/front/app/x.vue\nfront/app/y.vue')) === JSON.stringify(['/home/u/repo/front/app/x.vue', 'front/app/y.vue']));
	// B6 无路径可提取 → []（调用方保守处理）
	check('B6 无路径输出返回空数组', extractFailurePaths('0 issues.').length === 0
		&& extractFailurePaths('bash: line 1: go: command not found').length === 0
		&& extractFailurePaths('').length === 0);
	// B7 集合语义：同一路径去重为 1 项
	const trip = 'internal/admin/wire.go:1:1: expected declaration';
	check('B7 同一路径去重为 1 项', extractFailurePaths(`${trip}\n${trip}\n${trip}`).length === 1);
	// B8 非代码路径原样提取（是否外部由归属集判定，提取层不做业务过滤）
	check('B8 非代码路径原样返回', JSON.stringify(extractFailurePaths('openspec/changes/foo/tasks.md:12:1: ...')) === JSON.stringify(['openspec/changes/foo/tasks.md']));
	// 不变量：null/undefined 输入安全返回 []（对齐 I8/T7 防御风格）
	check('B-不变 null/undefined 输入安全返回 []', extractFailurePaths(null).length === 0 && extractFailurePaths(undefined).length === 0);
}

/* ---------- 段一·IV：isForeignFailure（attribute-concurrent-gate-noise，真值表 C1-C8） ---------- */
{
	const F = (paths, mine, foreign) => isForeignFailure({ paths, mine: new Set(mine), foreign: new Set(foreign) });
	// C1 路径在本会话启动基线中 → 外部
	check('C1 启动基线路径判外部', F(['f.go'], [], ['f.go']) === true);
	// C2 其他 active change 归属 → 外部
	check('C2 其他 change 归属判外部', F(['f.vue'], ['mine.go'], ['f.vue', 'other.ts']) === true);
	// C3 本会话触发过 → 本会话失败（现状）
	check('C3 本会话触发过不判外部', F(['f.go'], ['f.go'], ['f.go']) === false);
	// C4 混合（a∈mine, b∈foreign）→ 本会话失败（混合即保守）
	check('C4 混合路径保守不判外部', F(['a.go', 'b.go'], ['a.go'], ['b.go']) === false);
	// C5 解析不出（P=∅）→ 本会话失败（不判外部）
	check('C5 P 为空不判外部', F([], [], ['f.go']) === false);
	// C6 未归属新脏文件（两边都不在）→ 本会话失败（宁可多报）
	check('C6 未归属新脏文件不判外部（保守分支）', F(['c.go'], [], []) === false);
	// C7 标志库/edit.map 不可用（foreign 仅剩会话基线）→ 单信号仍成立
	check('C7 仅剩基线信号时仍判外部', F(['f.go'], [], ['f.go']) === true);
	// C8 无 git / 无绑定 change（mine 仅本会话触发集）→ 外部
	check('C8 mine 仅触发集时仍判外部', F(['f.go'], ['t1.go'], ['f.go']) === true);
	// 包锚点按前缀匹配（design D6：目录前缀对集合成员做前缀比对）
	check('C-前缀 包锚点命中 foreign 子文件', F(['backend-go/internal/admin/service/'], [], ['backend-go/internal/admin/service/x.go']) === true);
	check('C-前缀 包锚点命中 mine 子文件→本会话失败', F(['backend-go/internal/admin/service/'], ['backend-go/internal/admin/service/y.go'], ['backend-go/internal/admin/service/x.go']) === false);
	check('C-前缀 前缀不覆盖的成员不命中', F(['backend-go/internal/admin/'], [], ['backend-go/internal/other/x.go']) === false);
}

/* ---------- 段二：telemetry 集成（成功不产出 failure） ---------- */
{
	const tel = require('./.tel.cjs');
	const { queryBySession } = require('./.hlog.cjs');
	const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'tel-smoke-'));
	const handlers = {};
	const pi = { on: (name, fn) => { (handlers[name] ??= []).push(fn); } };
	tel.default(pi);
	const ctx = { cwd: tmp, hasUI: false, sessionManager: { getSessionId: () => 'tel-s1' } };
	const emit = async (name, event) => {
		for (const fn of handlers[name] ?? []) await fn(event, ctx);
	};

	(async () => {
		await emit('session_start', { reason: 'new' });
		// 失败派发：quota 拦截（无 tool_call 起点）
		await emit('tool_result', {
			toolName: 'Agent',
			toolCallId: 'tc1',
			isError: true,
			input: { subagent_type: 'Explore', description: '探索X', model: 'zai-coding-cn/glm-5.3', run_in_background: false, prompt: 'p'.repeat(100) },
			content: [{ type: 'text', text: '智谱 GLM 额度不足，本次 Agent 派发已阻断。\n剩余情况：窗口剩余 3%' }],
			details: { status: 'error' },
		});
		// 成功派发：不产出 failure（后台派发带 agentId，验证绑定暂存路径）
		await emit('tool_call', { toolName: 'Agent', toolCallId: 'tc2' });
		await emit('tool_result', {
			toolName: 'Agent',
			toolCallId: 'tc2',
			isError: false,
			input: { subagent_type: 'Explore', description: '成功派发', model: null, run_in_background: true, prompt: 'q' },
			content: [{ type: 'text', text: 'done' }],
			details: { tokens: '37.4k', durationMs: 12345, status: 'background', agentId: '7d1245e1-35ca-45a' },
		});

		// 完成回填（harness-observability-fixes D3）：真实样本回放（session JSONL 考古 2026-08-24）
		const doneText = 'Agent: 7d1245e1-35ca-45a\nType: Agent | Status: completed | Tool uses: 79 | 2.0M token | Context: 54% | Duration: 3944.8s\n\nDescription: Luna 真实 UI 验收\n\n验收报告正文……';
		await emit('tool_result', {
			toolName: 'get_subagent_result', toolCallId: 'tc3', isError: false,
			input: { agent_id: '7d1245e1-35ca-45a', wait: true },
			content: [{ type: 'text', text: doneText }],
		});
		// 同一 agent 取两次结果 → 幂等（至多一条 complete）
		await emit('tool_result', {
			toolName: 'get_subagent_result', toolCallId: 'tc4', isError: false,
			input: { agent_id: '7d1245e1-35ca-45a' },
			content: [{ type: 'text', text: doneText }],
		});
		// 取消样本：status=cancelled 透传且 isError=false
		await emit('tool_result', {
			toolName: 'get_subagent_result', toolCallId: 'tc5', isError: false,
			input: { agent_id: 'aaa-bbb-ccc' },
			content: [{ type: 'text', text: 'Agent: aaa-bbb-ccc\nType: Agent | Status: cancelled | Duration: 10.5s\n\n（用户取消）' }],
		});
		// 断链/清理形态：不伪造完成事件
		await emit('tool_result', {
			toolName: 'get_subagent_result', toolCallId: 'tc6', isError: false,
			input: { agent_id: 'c8e0d181-ba74-4ce' },
			content: [{ type: 'text', text: 'Agent not found: "c8e0d181-ba74-4ce". It may have been cleaned up.' }],
		});
		// running 非终态：不算完成
		await emit('tool_result', {
			toolName: 'get_subagent_result', toolCallId: 'tc7', isError: false,
			input: { agent_id: 'ddd-eee-fff' },
			content: [{ type: 'text', text: 'Agent: ddd-eee-fff\nType: Agent | Status: running' }],
		});

		const rows = queryBySession(tmp, 'tel-s1', ['subagent.dispatch']);
		check('telemetry 落库 subagent.dispatch 2 条', rows.length === 2);
		const fail = rows[0] && JSON.parse(rows[0].payload);
		const ok = rows[1] && JSON.parse(rows[1].payload);
		check('失败派发带 failure {stage:dispatch, category:quota-block}', fail && fail.failure && fail.failure.stage === 'dispatch' && fail.failure.category === 'quota-block');
		check('失败派发原文不透传（payload 不含第二行原文）', fail && !JSON.stringify(fail).includes('窗口剩余 3%'));
		check('失败派发 diag 为首行有界摘要', fail && fail.failure.diag.includes('额度不足') && Buffer.byteLength(fail.failure.diag, 'utf8') <= 512);
		check('成功派发无 failure 对象', ok && ok.failure === undefined);
		check('成功派发 tokens 规范化（37.4k→37400）', ok && ok.tokens === 37400);

		// subagent.complete 断言（D3）
		const comps = queryBySession(tmp, 'tel-s1', ['subagent.complete']);
		check('完成回填幂等（两次取结果仅一条 complete）', comps.length === 2); // completed + cancelled 各一
		const done = comps.find((r) => JSON.parse(r.payload).agentId === '7d1245e1-35ca-45a');
		const doneP = done && JSON.parse(done.payload);
		check('complete 解析真实样本（status/toolUses/tokens/ms）', doneP && doneP.status === 'completed' && doneP.toolUses === 79 && doneP.tokens === 2000000 && doneP.ms === 3944800);
		check('complete 复用派发时 change 绑定（tmp 无 change → null）', doneP && done.change === null);
		const cancelled = comps.find((r) => JSON.parse(r.payload).agentId === 'aaa-bbb-ccc');
		const cancelledP = cancelled && JSON.parse(cancelled.payload);
		check('cancelled 透传且 isError=false', cancelledP && cancelledP.status === 'cancelled' && cancelledP.isError === false);
		check('断链（not found/cleaned up）不伪造 complete', !comps.some((r) => JSON.parse(r.payload).agentId === 'c8e0d181-ba74-4ce'));
		check('running 非终态不记 complete', !comps.some((r) => JSON.parse(r.payload).agentId === 'ddd-eee-fff'));
		check('session.start 落库', queryBySession(tmp, 'tel-s1', ['session.start']).length === 1);

		fs.rmSync(tmp, { recursive: true, force: true });

		let fail2 = 0;
		for (const [name, okName] of checks) {
			console.log(`${okName ? '✅' : '❌'} ${name}`);
			if (!okName) fail2++;
		}
		console.log(fail2 ? `\n${fail2} 项失败` : '\nSMOKE OK');
		if (fail2) process.exitCode = 1;
	})().catch((e) => {
		console.error('FAIL', e);
		process.exitCode = 1;
	});
}
