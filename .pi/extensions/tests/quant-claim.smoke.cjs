// quant-claim smoke（harden-quant-claim-declaration）：lib/quant-claim 纯函数白盒。
// 覆盖：marker 解析三态 / 节抽取边界 / 表头逐字校验 / 状态枚举 / measured 证据三列 /
// 断言可比较性 / marker 与正文打架。断言失败 exit 1。
// 由 run-harness-smoke.sh 调用（先 esbuild 产出 .qc.cjs）。
const { checkQuantClaims, parseQuantMarker, isComparableClaim, extractQuantSection } = require('./.qc.cjs');

const checks = [];
const check = (name, ok) => checks.push([name, ok]);

const HEADER = '| 类别 | 断言 | 观测值 | 复算方式 | 观测日期 | 状态 |';
const SEP = '| --- | --- | --- | --- | --- | --- |';

const doc = (marker, body = '') =>
	`<!-- complexity: simple -->\n<!-- ui-impact: none -->\n${marker}\n\n# 设计\n\n${body}`;

const declared = (rows) =>
	doc('<!-- quant-claims: declared -->', `## 可量化断言\n\n${HEADER}\n${SEP}\n${rows.join('\n')}\n`);

const codes = (issues) => issues.map((i) => i.reasonCode);

// ── marker 解析 ────────────────────────────────────────────────
check('marker: 缺失', parseQuantMarker('# 设计\n').found === false);
check('marker: none', parseQuantMarker('<!-- quant-claims: none -->').value === 'none');
check('marker: declared 带空格', parseQuantMarker('<!--  quant-claims :  declared  -->').value === 'declared');
check('marker: 15 行外不算', parseQuantMarker(Array(16).fill('x').join('\n') + '\n<!-- quant-claims: none -->').found === false);

// ── 节抽取 ─────────────────────────────────────────────────────
const sec = extractQuantSection('## 可量化断言\n\n| a |\n\n## 下一节\n\n| b |');
check('section: 到下一个 h2 截止', sec !== null && sec.includes('| a |') && !sec.includes('| b |'));
check('section: h3 不截断', (extractQuantSection('## 可量化断言\n\n### 子标题\n\n| a |') || '').includes('### 子标题'));
check('section: 缺失返回 null', extractQuantSection('## 别的\n') === null);

// ── 断言可比较性 ────────────────────────────────────────────────
check('claim: 带数字可比较', isComparableClaim('372 = 372') === true);
check('claim: 带计划特征可比较', isComparableClaim('三份 EXPLAIN 均走 Sort') === true);
check('claim: 纯形容词不可比较', isComparableClaim('更快') === false);
check('claim: 无判据不可比较', isComparableClaim('性能有明显改善') === false);
check('claim: 空串不可比较', isComparableClaim('  ') === false);

// ── 四态组合 ───────────────────────────────────────────────────
check('无 marker → marker-missing', codes(checkQuantClaims('# 设计\n')).includes('quant-marker-missing'));
check('marker 非法 → marker-invalid', codes(checkQuantClaims(doc('<!-- quant-claims: yes -->'))).includes('quant-marker-invalid'));
check(
	'none + 有节 → 打架',
	codes(checkQuantClaims(doc('<!-- quant-claims: none -->', `## 可量化断言\n\n${HEADER}\n${SEP}\n`))).includes(
		'quant-section-unexpected',
	),
);
check('none 且无节 → 零问题', checkQuantClaims(doc('<!-- quant-claims: none -->')).length === 0);
check('declared 无节 → section-missing', codes(checkQuantClaims(doc('<!-- quant-claims: declared -->'))).includes('quant-section-missing'));
check(
	'declared 表头不符 → header-missing',
	codes(checkQuantClaims(doc('<!-- quant-claims: declared -->', '## 可量化断言\n\n| 类别 | 断言 |\n| --- | --- |\n| 性能 | x |\n'))).includes(
		'quant-table-header-missing',
	),
);
check(
	'declared 只有表头 → row-malformed',
	codes(checkQuantClaims(doc('<!-- quant-claims: declared -->', `## 可量化断言\n\n${HEADER}\n${SEP}\n`))).includes(
		'quant-row-malformed',
	),
);

// ── 行级校验 ───────────────────────────────────────────────────
check(
	'合规 measured 行 → 零问题',
	checkQuantClaims(declared(['| 性能 | 召回查询命中 HNSW | 未命中，走 Sort | `EXPLAIN ...` | 2026-11-27 | measured |'])).length === 0,
);
check(
	'measured 缺证据 → evidence-missing',
	codes(
		checkQuantClaims(declared(['| 性能 | 召回查询命中 HNSW | — | — | — | measured |'])),
	).includes('quant-evidence-missing'),
);
check(
	'unmeasured 允许空证据',
	checkQuantClaims(declared(['| 性能 | 单次 ANN < 50ms | — | — | — | unmeasured |'])).length === 0,
);
check(
	'类别非法 → category-invalid',
	codes(checkQuantClaims(declared(['| 速度 | 命中 HNSW 索引 | 未命中 | `EXPLAIN` | 2026-11-27 | measured |']))).includes(
		'quant-category-invalid',
	),
);
check(
	'状态非法 → status-invalid',
	codes(checkQuantClaims(declared(['| 性能 | 命中 HNSW 索引 | 未命中 | `EXPLAIN` | 2026-11-27 | ok |']))).includes(
		'quant-status-invalid',
	),
);
check(
	'纯形容词断言 → not-comparable',
	codes(checkQuantClaims(declared(['| 性能 | 更快 | 未命中 | `EXPLAIN` | 2026-11-27 | measured |']))).includes(
		'quant-claim-not-comparable',
	),
);
check(
	'数据类断言可比较',
	checkQuantClaims(
		declared(['| 数据 | 矩阵日格 = 该日各桶线索数之和 | 372 = 372 | `matrix_cell.sql` | 2026-11-26 | measured |']),
	).length === 0,
);
check(
	'多行时逐行校验',
	codes(
		checkQuantClaims(
			declared([
				'| 性能 | 命中 HNSW 索引 | 未命中 | `EXPLAIN` | 2026-11-27 | measured |',
				'| 数据 | 更快 | x | y | 2026-11-26 | measured |',
			]),
		),
	).includes('quant-claim-not-comparable'),
);

// ── 汇总 ───────────────────────────────────────────────────────
let failed = 0;
for (const [name, ok] of checks) {
	if (!ok) {
		failed++;
		console.error(`  ✗ ${name}`);
	}
}
console.log(`quant-claim smoke: ${checks.length - failed}/${checks.length} 通过`);
if (failed > 0) process.exit(1);
