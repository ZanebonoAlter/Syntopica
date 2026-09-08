// verify-structure.mjs — configure-dsh-energy-research 静态结构断言（tasks.md §5.1 复跑命令）
// 依赖：Node ≥18 + npx 缓存内 js-yaml（本机 dsh 0.1.2-rc.1 安装自带，零新增依赖）
// 用法：node openspec/changes/configure-dsh-energy-research/verify-structure.mjs
// 期望：打印 ASSERTIONS: ALL PASS，退出码 0
import { createRequire } from 'node:module';
import { readFileSync } from 'node:fs';
const req = createRequire('/mnt/c/Users/Admin/AppData/Local/npm-cache/_npx/1e7f6d9597241db0/node_modules/');
const yaml = req('js-yaml');

const base = 'config/dsh/presets/energy-research/';
const errs = [];
const chk = (c, m) => { if (!c) errs.push(m); };

const p = yaml.load(readFileSync(base + 'preset.yml', 'utf8'));
chk(JSON.stringify(Object.keys(p).sort()) === '["description","name","order"]', `preset keys ${Object.keys(p)}`);
chk(p.order === 20, 'order != 20');
chk(p.name === '能源研究', 'name');
chk(p.description.includes('仅网页') && p.description.includes('待接入'), 'desc');

const rows = yaml.load(readFileSync(base + 'agent.cordis.yml', 'utf8'));
chk(Array.isArray(rows), 'not a list');
const ids = rows.map(r => r.id);
chk(JSON.stringify(ids) === '["persona","tool-web","compaction","tool-ask-user"]', `top ids ${ids}`);
const by = Object.fromEntries(rows.map(r => [r.id, r]));
chk(by.persona.name === '@deepseek-ai/dsh-persona', 'persona pkg');
const pt = by.persona.config.text;
for (const kw of ['统计期','地域','单位','来源','估算','预测','荐股','价格','编造','指令','证据不足','EIA/JODI','MCP','不硬答'])
  chk(pt.includes(kw), `persona missing ${kw}`);
chk(!pt.includes('{{cwd}}'), 'cwd leak');
chk(JSON.stringify(by['tool-web'].config) === '{"fetch":true,"searchTimeoutMs":60000}', `web cfg ${JSON.stringify(by['tool-web'].config)}`);
const cg = by.compaction;
chk(cg.name === 'cordis:group' && cg.group === true, 'group head');
chk(JSON.stringify(cg.isolate) === '{"compaction":true,"toolResultPruner":true}', `isolate ${JSON.stringify(cg.isolate)}`);
const inner = Object.fromEntries(cg.config.map(r => [r.id, r]));
chk(JSON.stringify(Object.keys(inner).sort()) === '["command-compact","compaction-basic","tool-result-pruner"]', `inner ${Object.keys(inner)}`);
chk(JSON.stringify(inner['tool-result-pruner'].config) === '{"thresholdChars":8192,"headChars":4096,"tailChars":1024}', 'pruner cfg');
chk(by['tool-ask-user'].name === '@deepseek-ai/dsh-tool-ask-user', 'ask pkg');

const allnames = [];
const collect = rs => { for (const r of rs) { allnames.push(r.name ?? ''); if (Array.isArray(r.config)) collect(r.config); } };
collect(rows);
for (const d of ['bash','pwsh','terminal','tool-fs','fs-search','str_replace','agent-instructions','jobs','skill','goal','plan','subagent','workflow','ralph','todo','mcp'])
  for (const n of allnames) chk(!n.includes(d), `denylist hit: ${d} in ${n}`);
chk(!readFileSync(base + 'agent.cordis.yml', 'utf8').includes('!!'), 'custom yaml tag present');

console.log('ASSERTIONS:', errs.length ? 'FAIL' : 'ALL PASS');
for (const e of errs) console.log(' -', e);
process.exit(errs.length ? 1 : 0);
