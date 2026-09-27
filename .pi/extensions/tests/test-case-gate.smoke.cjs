// test-case-gate smoke：复杂度声明识别 / 文档检出 / 任务行词法兜底 / 入口判定全分支 /
// 提醒文案 / 真实数据自检（watch-materialized-topic 的 tasks.md 应命中兜底）。
// 由 run-harness-smoke.sh 调用（先 esbuild 产出 .tcg.cjs）。断言失败 exit 1。
const {
	parseComplexityDeclaration,
	hasTestCaseDoc,
	scanComplexityKeywords,
	decideEntryReminder,
	buildEntryReminderMessage,
} = require('./.tcg.cjs');

const checks = [];
const check = (name, ok) => checks.push([name, ok]);

try {
	// ---- 1. parseComplexityDeclaration：三态 + 容错 ----
	check('complex 声明', parseComplexityDeclaration('<!-- complexity: complex -->') === 'complex');
	check('simple 声明', parseComplexityDeclaration('<!-- complexity: simple -->\n\n## Why') === 'simple');
	check('容忍前后空白', parseComplexityDeclaration('x\n<!--complexity:  simple  -->\ny') === 'simple');
	check('多声明取首个', parseComplexance_multi());
	check('非法值视为未声明', parseComplexityDeclaration('<!-- complexity: hard -->') === null);
	check('大小写敏感（Complex 不认）', parseComplexityDeclaration('<!-- complexity: Complex -->') === null);
	check('无声明 → null', parseComplexityDeclaration('## Why\n普通 proposal') === null);
	check('null/undefined 输入 → null', parseComplexityDeclaration(null) === null && parseComplexityDeclaration(undefined) === null);
	check('空串 → null', parseComplexityDeclaration('') === null);

	// ---- 2. hasTestCaseDoc：前缀/后缀/反例 ----
	check('test-cases.md 检出', hasTestCaseDoc(['proposal.md', 'test-cases.md']) === true);
	check('test-cases-xxx.md 前缀检出', hasTestCaseDoc(['test-cases-watch.md']) === true);
	check('xxx-test-cases.md 后缀检出', hasTestCaseDoc(['watch-test-cases.md']) === true);
	check('普通 md 不检出', hasTestCaseDoc(['proposal.md', 'design.md', 'tasks.md']) === false);
	check('test-cases.txt 不检出（仅 .md）', hasTestCaseDoc(['test-cases.txt']) === false);
	check('空/非数组 → false', hasTestCaseDoc([]) === false && hasTestCaseDoc(null) === false);

	// ---- 3. scanComplexityKeywords：任务行锚定 ----
	const kwTasks = [
		'## 1. 任务',
		'- [ ] 1.1 实现正文解析逻辑',
		'- [x] 1.2 状态机迁移',
		'  - [ ] 1.3 缩进子任务含协议字段',
		'非任务行含解析算法协议状态机——不应命中',
		'- 不带 checkbox 的列表项含解析——不应命中',
	].join('\n');
	const kws = scanComplexityKeywords(kwTasks);
	check('命中三词（算法不在任务行，不命中）', kws.length === 3 && ['解析', '状态机', '协议'].every((k) => kws.includes(k)) && !kws.includes('算法'));
	check('空串/null → []', scanComplexityKeywords('').length === 0 && scanComplexityKeywords(null).length === 0);

	// ---- 4. decideEntryReminder：全分支（spec Scenario 对应） ----
	const base = { mode: 'implementation', boundChange: 'x', changeDirFiles: ['proposal.md', 'tasks.md'], proposalText: null, tasksMd: '- [ ] 1.1 干活', alreadyWarned: false };
	// S: requirements 档零触发
	check('requirements 档 → null', decideEntryReminder({ ...base, mode: 'requirements' }) === null);
	check('无 mode 记录 → null', decideEntryReminder({ ...base, mode: null }) === null);
	check('未绑定 change → null', decideEntryReminder({ ...base, boundChange: null }) === null);
	// S: 已有文档 → 静默
	check('有 test-cases.md → null', decideEntryReminder({ ...base, changeDirFiles: ['test-cases.md'] , proposalText: '<!-- complexity: complex -->' }) === null);
	// S: 声明 complex 且缺文档 → 强提醒
	const strong = decideEntryReminder({ ...base, proposalText: '<!-- complexity: complex -->' });
	check('complex 缺文档 → strong', strong !== null && strong.strong === true && strong.declaration === 'complex');
	// S: simple 且词表未命中 → 静默
	check('simple 未命中 → null', decideEntryReminder({ ...base, proposalText: '<!-- complexity: simple -->' }) === null);
	// S: simple 但命中 → 兜底质询
	const contra = decideEntryReminder({ ...base, proposalText: '<!-- complexity: simple -->', tasksMd: '- [ ] 1.1 实现状态机迁移' });
	check('simple+命中 → 非强提醒且 declaration=simple', contra !== null && contra.strong === false && contra.declaration === 'simple' && contra.kwHits.includes('状态机'));
	// 未声明 + 命中 → 兜底
	const fb = decideEntryReminder({ ...base, tasksMd: '- [x] 2.3 协议字段校验' });
	check('未声明+命中 → 兜底提醒', fb !== null && fb.strong === false && fb.declaration === null && fb.kwHits.includes('协议'));
	// 未声明未命中 → null
	check('未声明未命中 → null', decideEntryReminder(base) === null);
	// S: 去重
	check('alreadyWarned → null', decideEntryReminder({ ...base, proposalText: '<!-- complexity: complex -->', alreadyWarned: true }) === null);
	// 读不到 proposal/tasks（null）按未声明/未命中处理
	check('proposal=null 未命中 → null', decideEntryReminder({ ...base, proposalText: null }) === null);

	// ---- 5. buildEntryReminderMessage：三档文案 ----
	const msgStrong = buildEntryReminderMessage('watch-x', strong);
	check('强文案含声明与修复路径', msgStrong.includes('complex') && msgStrong.includes('test-cases') && msgStrong.includes('complexity'));
	const msgContra = buildEntryReminderMessage('watch-x', contra);
	check('矛盾文案含关键词与声明提示', msgContra.includes('状态机') && msgContra.includes('simple'));
	const msgFb = buildEntryReminderMessage('watch-x', fb);
	check('兜底文案含关键词', msgFb.includes('协议') && msgFb.includes('兜底'));

	// ---- 6. 真实数据自检：watch-materialized-topic（已归档，补齐 test-cases.md 后归档的实例） ----
	// 双断言：真实 tasks/proposal 文本 + 模拟无文档 → 兜底提醒；真实目录文件列表（含 test-cases.md）→ 静默 ----
	let real;
	let realFiles;
	try {
		const fs = require('node:fs');
		const dir = '../../../openspec/changes/archive/2026-08-25-watch-materialized-topic';
		const realTasks = fs.readFileSync(dir + '/tasks.md', 'utf8');
		const realProposal = fs.readFileSync(dir + '/proposal.md', 'utf8');
		realFiles = fs.readdirSync(dir);
		real = decideEntryReminder({ ...base, boundChange: 'watch-materialized-topic', changeDirFiles: ['proposal.md', 'tasks.md'], tasksMd: realTasks, proposalText: realProposal });
		check('真实考古现场：真实 tasks 文本 + 无文档 → 兜底提醒', real !== null && real.kwHits.includes('解析'));
		const silent = decideEntryReminder({ ...base, boundChange: 'watch-materialized-topic', changeDirFiles: realFiles, tasksMd: realTasks, proposalText: realProposal });
		check('真实补齐后：目录含 test-cases.md → 静默', silent === null);
	} catch {
		console.log('△ 真实数据自检跳过（仓库外跑冒烟或归档目录已移动，文件读不到）');
	}
} catch (err) {
	check('执行异常：' + String(err), false);
}

function parseComplexance_multi() {
	return parseComplexityDeclaration('<!-- complexity: simple --><!-- complexity: complex -->') === 'simple';
}

// ---- 汇总 ----
const failed = checks.filter(([, ok]) => !ok);
for (const [name, ok] of checks) console.log(`${ok ? '✓' : '✗'} ${name}`);
if (failed.length > 0) {
	console.error(`\ntest-case-gate smoke：${failed.length}/${checks.length} 项失败`);
	process.exit(1);
}
console.log(`\ntest-case-gate smoke：全部 ${checks.length} 项通过`);
