// harness-log smoke：安全开库（新库初始化/他人库拒绝/未来版本拒绝）、写入查询、TTL 分级清扫。
// 由 run-harness-smoke.sh 调用（先 esbuild 产出 .hlog.cjs）。断言失败 exit 1。
//
// TTL 用例需冷开库（dbCache 缓存命中不重跑清扫）→ 主进程写入并篡改 ts 后，
// 以子进程重新 require bundle 查询验证（子进程 dbCache 为空，触发 openDb → TTL 清扫）。
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

const { logEvent, queryBySession, queryByChange, queryLatestByKind } = require('./.hlog.cjs');

const mktmp = () => fs.mkdtempSync(path.join(os.tmpdir(), 'hlog-smoke-'));
const dbOf = (cwd) => path.join(cwd, '.pi', 'harness', 'events.db');
const APP_ID = 0x53594e54;

function preExistingDb(setup) {
	const cwd = mktmp();
	fs.mkdirSync(path.dirname(dbOf(cwd)), { recursive: true });
	const db = new DatabaseSync(dbOf(cwd));
	setup(db);
	db.close();
	return cwd;
}

(async () => {
	const checks = [];
	const check = (name, ok) => checks.push([name, ok]);
	const tmps = [];

	try {
		// ---- 1. 首次开库初始化：app_id / user_version / 建表 ----
		const t1 = mktmp(); tmps.push(t1);
		const w1 = logEvent(t1, { kind: 'session.start', sessionId: 's1', payload: { reason: 'new' } });
		check('首次 logEvent 返回 true', w1 === true);
		const db1 = new DatabaseSync(dbOf(t1));
		const appId = db1.prepare('PRAGMA application_id').get().application_id;
		const uv = db1.prepare('PRAGMA user_version').get().user_version;
		const hasTable = db1.prepare("SELECT COUNT(*) AS n FROM sqlite_master WHERE type='table' AND name='events'").get().n;
		db1.close();
		check('application_id = 0x53594E54 ("SYNT")', appId === APP_ID);
		check('user_version = 1', uv === 1);
		check('events 表已建（三组索引随 DDL）', hasTable === 1);

		// ---- 2. 写入查询：queryBySession（含 kinds 过滤）/ queryByChange ----
		logEvent(t1, { kind: 'constraint.inject', sessionId: 's1', change: 'demo-change', payload: { path: 'docs/x.md' } });
		logEvent(t1, { kind: 'pin.write', sessionId: 's1', change: 'demo-change', payload: { title: 't' } });
		const all = queryBySession(t1, 's1');
		check('queryBySession 返回 3 行且按 id 升序', all.length === 3 && all[0].id < all[2].id);
		check('ts 为 ISO 8601 UTC', /^\d{4}-\d{2}-\d{2}T/.test(all[0].ts));
		const only = queryBySession(t1, 's1', ['constraint.inject']);
		check('kinds 过滤只回 constraint.inject', only.length === 1 && only[0].kind === 'constraint.inject');
		const byChange = queryByChange(t1, 'demo-change');
		check('queryByChange 绑定 change 列', byChange.length === 2 && byChange.every((r) => r.change === 'demo-change'));
		const none = queryBySession(t1, 'no-such-session');
		check('空会话查询返回空数组', none.length === 0);

		// ---- 2.5 mode.set + queryLatestByKind（@constraint-injection-tier-b D5/D6）----
		logEvent(t1, { kind: 'mode.set', sessionId: 's1', change: 'change-a', payload: { mode: 'implementation', boundChange: 'change-a' } });
		logEvent(t1, { kind: 'mode.set', sessionId: 's2', change: 'change-b', payload: { mode: 'implementation', boundChange: 'change-b' } });
		const latest = queryLatestByKind(t1, 'mode.set');
		check('queryLatestByKind 返回全局最新一条（跨会话）', latest && latest.session_id === 's2' && latest.change === 'change-b');
		check('queryLatestByKind 无匹配返回 null', queryLatestByKind(t1, 'pin.read') === null);
		const msRows = queryBySession(t1, 's1', ['mode.set']);
		check('mode.set 可按会话过滤查询（resume 第1段取数路径）', msRows.length === 1 && JSON.parse(msRows[0].payload).mode === 'implementation');

		// ---- 2.6 新 kind 随词汇扩展落库（harden-harness-policy-and-spill：十类词汇补齐）----
		logEvent(t1, { kind: 'policy.decision', sessionId: 's1', change: 'demo-change', payload: { policy: 'spec-gate', action: 'block', reasonCode: 'archive-check-failed' } });
		logEvent(t1, { kind: 'subagent.complete', sessionId: 's3', change: 'demo-change', payload: { agentId: 'a1', status: 'completed' } });
		const pdRows = queryBySession(t1, 's1', ['policy.decision']);
		check('policy.decision 可写可查（kinds 过滤 + payload 透传）', pdRows.length === 1 && JSON.parse(pdRows[0].payload).action === 'block' && pdRows[0].change === 'demo-change');
		const sacRows = queryBySession(t1, 's3', ['subagent.complete']);
		check('subagent.complete 可写可查（kind 为 TEXT 列，无 DDL）', sacRows.length === 1 && sacRows[0].kind === 'subagent.complete');
		// 既有 v1 库写入新 kind 后 user_version 不变（无 schema 迁移）
		const db1b = new DatabaseSync(dbOf(t1));
		check('既有库写入新 kind 后 user_version 仍为 1', db1b.prepare('PRAGMA user_version').get().user_version === 1);
		db1b.close();
		// append-only 幂等：重复同 payload 追加两条独立事件（不改写既有行）
		logEvent(t1, { kind: 'policy.decision', sessionId: 's1', payload: { policy: 'quota-gate', action: 'fail-open', reasonCode: 'quota-query-failed' } });
		logEvent(t1, { kind: 'policy.decision', sessionId: 's1', payload: { policy: 'quota-gate', action: 'fail-open', reasonCode: 'quota-query-failed' } });
		const dupRows = queryBySession(t1, 's1', ['policy.decision']);
		check('重复同 payload append-only 保留三条独立事件', dupRows.length === 3 && dupRows[1].id < dupRows[2].id);
		// 子进程冷开库：既有行不改写、新旧 kind 共存（事件追加不可变）
		const scriptImmut = `const m=${JSON.stringify(path.resolve('.hlog.cjs'))};const c=${JSON.stringify(t1)};const q=require(m);const rows=q.queryBySession(c,'s1');console.log(rows.length+':'+rows[0].kind)`;
		const outImmut = execSync(`node -e ${JSON.stringify(scriptImmut)}`, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
		check('冷开库重读：行数与首行 kind 不变（旧事件兼容共存）', outImmut === '7:session.start');

		// ---- 2.7 edit.map 词汇扩展（coordinate-concurrent-changes：归属地图累计快照）----
		logEvent(t1, { kind: 'edit.map', sessionId: 's1', change: 'demo-change', payload: { paths: ['a.go', 'b.go'], n: 2 } });
		logEvent(t1, { kind: 'edit.map', sessionId: 's1', change: 'demo-change', payload: { paths: ['a.go', 'b.go', 'c.go'], n: 3 } }); // 累计快照追加
		const emRows = queryBySession(t1, 's1', ['edit.map']);
		check('edit.map 可写可查（append-only 累计快照，两条独立事件）', emRows.length === 2 && JSON.parse(emRows[1].payload).n === 3);
		const emByChange = queryByChange(t1, 'demo-change', ['edit.map']);
		check('edit.map 按 change 查询取最新一条即全量集合', JSON.parse(emByChange[emByChange.length - 1].payload).paths.length === 3);

		// ---- 3. 拒绝他人库：app_id=0 且已有用户表 ----
		const t2 = preExistingDb((db) => db.exec('CREATE TABLE other_app(x)'));
		tmps.push(t2);
		check('他人未登记库（app_id=0 有表）拒绝 → logEvent false', logEvent(t2, { kind: 'session.start', sessionId: 's', payload: {} }) === false);

		// ---- 4. 拒绝他人库：app_id 为其他应用魔数 ----
		const t3 = preExistingDb((db) => { db.exec('CREATE TABLE foo(x)'); db.exec('PRAGMA application_id = 0x12345678'); });
		tmps.push(t3);
		check('他人库（app_id=0x12345678）拒绝 → logEvent false', logEvent(t3, { kind: 'session.start', sessionId: 's', payload: {} }) === false);

		// ---- 5. 拒绝未来版本：本应用魔数但 user_version 更高 ----
		const t4 = preExistingDb((db) => { db.exec('CREATE TABLE events(x)'); db.exec(`PRAGMA application_id = ${APP_ID}`); db.exec('PRAGMA user_version = 2'); });
		tmps.push(t4);
		check('未来版本库（version=2）拒绝 → logEvent false', logEvent(t4, { kind: 'session.start', sessionId: 's', payload: {} }) === false);

		// ---- 6. TTL 分级清扫：子进程冷开库触发 ----
		const t5 = mktmp(); tmps.push(t5);
		logEvent(t5, { kind: 'constraint.inject', sessionId: 's1', payload: {} });   // 30 天
		logEvent(t5, { kind: 'pin.write', sessionId: 's1', payload: {} });           // 永久
		logEvent(t5, { kind: 'session.start', sessionId: 's1', payload: {} });       // 90 天
		logEvent(t5, { kind: 'gate.check', sessionId: 's1', payload: {} });          // 30 天
		logEvent(t5, { kind: 'mode.set', sessionId: 's1', payload: {} });            // 30 天（tier-b）
		logEvent(t5, { kind: 'spill.write', sessionId: 's1', payload: {} });       // 30 天（tool-output-spill）
		logEvent(t5, { kind: 'subagent.complete', sessionId: 's1', payload: {} });   // 30 天
		logEvent(t5, { kind: 'policy.decision', sessionId: 's1', payload: {} });     // 30 天（本 change：过期对照）
		logEvent(t5, { kind: 'policy.decision', sessionId: 's1', payload: {} });     // 新鲜保留对照（30 天边界内）
		logEvent(t5, { kind: 'edit.map', sessionId: 's1', payload: { paths: ['x.go'], n: 1 } });      // 30 天（过期对照）
		logEvent(t5, { kind: 'edit.map', sessionId: 's1', payload: { paths: ['y.go'], n: 1 } });      // 新鲜保留对照
		const db5 = new DatabaseSync(dbOf(t5));
		const old30 = new Date(Date.now() - 40 * 86400e3).toISOString();
		const old90 = new Date(Date.now() - 100 * 86400e3).toISOString();
		db5.prepare("UPDATE events SET ts = ? WHERE kind = 'constraint.inject'").run(old30);
		db5.prepare("UPDATE events SET ts = ? WHERE kind = 'gate.check'").run(old30);
		db5.prepare("UPDATE events SET ts = ? WHERE kind = 'mode.set'").run(old30);
		db5.prepare("UPDATE events SET ts = ? WHERE kind = 'spill.write'").run(old30);
		db5.prepare("UPDATE events SET ts = ? WHERE kind = 'subagent.complete'").run(old30);
		db5.prepare("UPDATE events SET ts = ? WHERE kind = 'session.start'").run(old90);
		// 只把第一条 policy.decision 篡改为过期（按 id 定位，新鲜对照不动）
		const stalePd = db5.prepare("SELECT id FROM events WHERE kind = 'policy.decision' ORDER BY id LIMIT 1").get().id;
		db5.prepare('UPDATE events SET ts = ? WHERE id = ?').run(old30, stalePd);
		// 同样只把第一条 edit.map 篡改为过期（新鲜对照不动）
		const staleEm = db5.prepare("SELECT id FROM events WHERE kind = 'edit.map' ORDER BY id LIMIT 1").get().id;
		db5.prepare('UPDATE events SET ts = ? WHERE id = ?').run(old30, staleEm);
		db5.close();
		// 子进程冷开库：openDb 跑 TTL 清扫后查询
		const script = `const m=${JSON.stringify(path.resolve('.hlog.cjs'))};const c=${JSON.stringify(t5)};const {DatabaseSync}=require('node:sqlite');const q=m===null?null:require(m);const rows=q.queryBySession(c,'s1');console.log(rows.map(r=>r.kind).sort().join(','))`;
		const out = execSync(`node -e ${JSON.stringify(script)}`, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
		check('TTL 清扫：30 天过期行删除（含 mode.set/spill.write/subagent.complete/policy.decision/edit.map）、pin.write 永久保留', out === 'edit.map,pin.write,policy.decision');
		check('TTL 边界：30 天内的新鲜 policy.decision 与 edit.map 保留', out.split(',').includes('policy.decision') && out.split(',').includes('edit.map'));

		let fail = 0;
		for (const [name, ok] of checks) {
			console.log(`${ok ? '✅' : '❌'} ${name}`);
			if (!ok) fail++;
		}
		console.log(fail ? `\n${fail} 项失败` : '\nSMOKE OK');
		if (fail) process.exitCode = 1;
	} finally {
		for (const t of tmps) fs.rmSync(t, { recursive: true, force: true });
	}
})().catch((e) => {
	console.error('FAIL', e);
	process.exitCode = 1;
});
