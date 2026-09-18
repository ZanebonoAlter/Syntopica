
## suggest 归属轨落点与 change 名解析

文件 `scripts/harness/doc-impact.sh`（342 行，POSIX bash，开头 cd REPO_ROOT）：

- `changed_files()`（约 47-58 行）：全树输入源 = `git -c core.checkStat=minimal -c core.quotepath=false diff --name-only <base>` + `diff --cached --name-only` + `ls-files --others --exclude-standard`。suggest/verify 共用。
- `ownership_paths()`（约 66-78 行，**已存在，verify 在用**）：`sqlite3 "$db" -readonly "SELECT DISTINCT json_each.value FROM events AS e, json_each(e.payload,'$.paths') WHERE e.kind='edit.map' AND e.change='<name>' AND e.id=(SELECT MAX(id) FROM events WHERE kind='edit.map' AND change='<name>')" | head -2000`；守卫：非 git 仓库 / `sqlite3` 缺失 / 库不存在 / 查询失败 → 返回空（fail-open）；单引号经 `${change_name//\'/\'\'}` 转义。库路径 `$(git rev-parse --show-toplevel)/.pi/harness/events.db`。
- `filter_blacklist()`（约 84-92 行）：剔 `openspec/changes/archive/`、`openspec/changes/*/.openspec.yaml`、三个 AGENTS.md + `docs/reference/constraints-index.md`；与 `lib/edit-map.ts` 的 `isToolManagedPath()` 镜像。
- `heuristic_hit()`（约 105-140 行）：7 域正则（flow/api/database/architecture/standard/configuration/deployment），`$2` = 预计算的文件列表（不可在函数内重跑 git 扫描）。
- `cmd_suggest()`（约 155-190 行）：**当前 `files="$(changed_files "$base" | sort -u | filter_blacklist)"`（全树）**，7 域循环出 `[x]/[ ]` 预勾选，随后打印 tasks.md 声明模板。**cmd_suggest 无 change 参数解析，也无 `ownership_paths` 调用**。
- `cmd_verify()`（约 195-325 行）：已实现双轨——`heuristic_input` 优先 `ownership_paths <basename>`，空则回退 `$changed`；`report_heuristic_hit()` 按轨分级（ownership → FAIL；fallback → stderr 提示不判 FAIL）。`--base` 参数解析在 verify 内。

新查询落点（本 change 新增）：查**全部 change** 的最新归属集合 = `SELECT e.change, json_each.value FROM events e, json_each(e.payload,'$.paths') WHERE e.kind='edit.map' AND e.id IN (SELECT MAX(id) FROM events WHERE kind='edit.map' GROUP BY change)`——用于把脏文件分成「本 change / 其他 change / 无归属」三桶。

change 名三源解析数据通路：`PI_SESSION_ID`（pi 注入 shell 环境的 session id，实测 `env | grep PI_` 可见）→ `SELECT payload FROM events WHERE kind='mode.set' AND session_id='<sid>' ORDER BY id DESC LIMIT 1` → 解析 `$.boundChange`（语义同 `lib/edit-map.ts` 的 `parseModeSetPayload`；`mode.set` payload 形如 `{"mode":"implementation","boundChange":"<name>","source":"skill"}`，boundChange 可为 null）。

既有 smoke：`scripts/harness/doc-impact.smoke.sh`（fixture 复制被测脚本进临时仓库，`verify()` 包装函数捕获 exit；已有 fail-open/双轨/黑名单用例可参照）。

**引用**：scripts/harness/doc-impact.sh:changed_files、scripts/harness/doc-impact.sh:ownership_paths、scripts/harness/doc-impact.sh:cmd_suggest、scripts/harness/doc-impact.sh:cmd_verify、scripts/harness/doc-impact.sh:filter_blacklist、.pi/extensions/lib/edit-map.ts:parseModeSetPayload、.pi/extensions/lib/harness-log.ts:queryBySession

<!-- pinned 2026-09-18T06:39:06Z -->

## quality-gate 基线与失败报告落点（含数据证据与复现 SQL）

文件 `.pi/extensions/quality-gate.ts`（638 行）：

- 模块级会话状态块（约 130-160 行）：`snapshot: Map<string,FileStat> | null`（git 脏文件 {mtimeMs,size}）、`stickyFailures: Set<string>`、`gateOkStates: Map<cmd,GateOkState>`、`eslintCacheOff`、`ownerSessionId`。session_start（reason "startup" 跳过）与 session_shutdown 重置，且仅 owner 会话生效（子线程防御）。
- `statGitFiles(repoRoot, files)`（约 165-180 行）：`statSync` 取 `{mtimeMs: Math.round(...), size}`，消失文件跳过。
- turn_end：step 1 早退（`CODE_TOOLS = {edit,write,bash,apply_patch}` 无命中且 sticky 空时 return）；step 2 全树 `git diff --name-only HEAD` + `ls-files --others --exclude-standard` → `curr`；`if (snapshot === null) snapshot = curr`（lazy 兜底）；`trigger = computeTriggerSet(snapshot, curr)`；**`snapshot = curr`（约 250 行）→ 会话启动基线在此丢失，需新增 `baselinePaths` 不再覆盖的变量**。
- step 2.5 `syncEditMap(repoRoot, sidEm, trigger)`：仅 `EDIT_TOOLS = {edit,write,apply_patch}` 回合记账（bash 回合不记，防 mtime 串扰）。
- step 3 路由：`trigBackend = trigger 含 ^backend-go/.*\.go$`；`trigFrontend = trigger 含 ^front/` 且非 .md；`stickyBackend/stickyFrontend` 由 `stickyFailures` 反推（`"pnpm lint"` = 前端侧）。
- step 3.5/3.6：execPlatform（native/windows）+ interop 探测 + native 工具链探测短路。
- step 4 `gateLog(cmd, code, ms, output)`（约 430-490 行）：`stepGateOk()` 给 `failPrefix`（`[回归]`/`[中间态]`）、`stickyFailures.add/delete`、`envFailures` 分流（isInteropFailure 仅 windows / isToolNotFound 仅 native）、`logEvent(gate.check)` 记账（含 truncateDiagGate diag）。
- **step 5 报告（约 560-590 行）：`if (failures.length > 0) pi.sendMessage({customType:"quality-gate-failure", content: ...failures.join("\n\n")}, {deliverAs:"steer", triggerTurn:true})`——每回合重发完整块（每块含 `tail(output, 30)`），无任何去重**。step 5.5 是 envFailures 的同款 steer（本次不动）。
- 门禁命令：后端 `golangci-lint run --allow-parallel-runners ./...` → `go vet ./...` + `go build ./...`（并行）→ change-scope 出的 domain `go test -short`（5min 预算）；前端 `pnpm exec eslint .`（--cache）。**全仓执行，与触发文件无关**（这就是外部文件也报错的机制）。

纯函数落点 `.pi/extensions/lib/failure-classify.ts`：`truncateDiag(text)` / `truncateDiagGate(text)`（GATE_DIAG_RULES 特征行，gate.check diag 同源）/ `isInteropFailure` / `isToolNotFound` / `classifyFailure`。新增 `extractFailurePaths()` 与归属判定纯函数应放这里（同族）。

`.pi/extensions/lib/edit-map.ts`：`syncEditMap` / `resetEditMapState` / `isToolManagedPath` / `mergePaths` / `buildEditMapPayload`；无「查全部 change 最新归属」的函数（需新增 `latestEditMapByChange`）。`.pi/extensions/lib/policy-decision.ts`：`logPolicyDecision(cwd,{sessionId,policy,action,reasonCode,target,change})`，action 白名单 block|warn|bypass|fail-open，reasonCode 必须 kebab-case ≤64。

smoke 夹具：`.pi/extensions/tests/quality-gate.behavior.smoke.cjs`（mock pi/ctx + 真 tmp events.db + `node:sqlite` 只读查询；`mkrepo()` 造 tmp 仓库；`gitRouter(repoRoot, files)` mock git 三件套；`EDIT_TURN`/`CHAT_TURN`；场景 A-I 已有；`run-harness-smoke.sh` 先 esbuild 产 `.qgateb.cjs`）。`failure-classify.smoke.cjs` 是纯函数单测。

数据证据（events.db，2026-09-18 采集）：
- 近 21 天失败行 2398；其中 1608 行（67%）±30min 内存在另一 session 报同一 (cmd,diag)：`with f as (select session_id sid, json_extract(payload,'$.cmd') cmd, substr(json_extract(payload,'$.diag'),1,50) diag, cast(strftime('%s',ts) as integer) e from events where kind='gate.check' and json_extract(payload,'$.ok')=0 and ts > datetime('now','-21 days')) select count(*) from f a where exists (select 1 from f b where b.cmd=a.cmd and b.diag=a.diag and b.sid<>a.sid and abs(b.e-a.e)<=1800);`
- 同会话同 (cmd,diag) 重复 ≥3 的组 151 / 行 1773，其中 1622 行（91%）是第 2..N 次（去掉 toolchain 事故后仍有 11-22 连击的组）。
- 现场样本：2026-09-18 06:08–06:13，session 01a0b317 / 01a0b31a 各 19 次 `golangci-lint` 报 `internal/tagmanagement/service/sourcestats/sourcestats.go:63`（该目录当时 `??` 未跟踪）；同期 gate.check 的 change 列被 `detectActiveChange` 目录 mtime 启发式挂到纯运维 change `add-pg-key-tables-backup` 名下。剔除 toolchain 事故的重复组查询：`... and json_extract(payload,'$.diag') not like '%command not found%'`。

**引用**：.pi/extensions/quality-gate.ts:turn_end、.pi/extensions/quality-gate.ts:gateLog、.pi/extensions/quality-gate.ts:statGitFiles、.pi/extensions/lib/trigger-set.ts:computeTriggerSet、.pi/extensions/lib/failure-classify.ts:truncateDiagGate、.pi/extensions/lib/gate-sample.ts:stepGateOk、.pi/extensions/lib/edit-map.ts:syncEditMap、.pi/extensions/lib/policy-decision.ts:logPolicyDecision、.pi/extensions/tests/quality-gate.behavior.smoke.cjs

<!-- pinned 2026-09-18T06:39:06Z -->
