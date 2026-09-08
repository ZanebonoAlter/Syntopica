# explore-findings — coordinate-concurrent-changes

> 实现依赖事实（2026-09-04 探索锁定）。每条带 file:symbol 引用，实现阶段据此直接动手。

## 1. quality-gate.ts turn_end 挂点（edit.map 落库位置）

- 挂点在 `quality-gate.ts:turn_end` 的 `computeTriggerSet(snapshot, curr)` 之后、侧路由之前：trigger（本回合新增/变化路径，含非代码文件如 md）非空即候选落库，**不限于前后端路由命中**（change 的文档编辑也进归属地图）
- 纯对话回合在 step 1 `if (!touchedCode && stickyFailures.size === 0) return` 提前返回（快照不更新，edit.map 零事件）；CODE_TOOLS = `edit/write/bash/apply_patch`（`quality-gate.ts:CODE_TOOLS`），bash sed 改文件天然覆盖
- 模块级状态在 `session_start`（reason≠startup，防 pi-subagents 共享实例误清）重置；`ownerSessionId` 防子线程污染（`quality-gate.ts:ownerSessionId`）——editMapBase/cachedChange 新增模块状态同样在此重置
- 会话基线：`statGitFiles` + `git diff --name-only HEAD` + `git ls-files --others --exclude-standard`（turn_end 每回合重跑，repoRoot 用 `git rev-parse --show-toplevel`）

## 2. boundChange 来源：mode.set 查询（非 detectActiveChange）

- `lib/active-change.ts:detectActiveChange` 是 change 目录 mtime 最新启发式（gate.check 记账用它）——**语义太弱，不能用于归属判定**
- edit.map 用 mode.set 的 boundChange（档位绑定语义）：`lib/harness-log.ts:queryBySession(cwd, sessionId, ["mode.set"])` 返回按 id 升序行数组，取最后一条 + JSON.parse payload 取 `boundChange` 字段（null → 跳过）
- constraint-injection.ts:1072 `parseModeSetRow` 是同逻辑但未导出——不 import（侵入大），quality-gate 侧自写 3 行解析，抽到 lib/edit-map.ts 供白盒直测

## 3. 跨 session 合并 = 落库侧并集（design D2 实现修正）

- design D2 原文"查询侧取最新一条即得完整集合，跨 session 自然合并"**不成立**：session B 只落自己 trigger 会丢失 session A 的路径
- 修正实现：quality-gate 模块级 `editMapBase: Set<string> | null` + `editMapBound: string | null`；首次（或 boundChange 变化时）用 `lib/harness-log.ts:queryByChange(cwd, change, ["edit.map"])` 取最后一条的 payload.paths 作 base；每回合 base ∪= trigger 后落库（快照 = 全量累计）。查询侧"取最新一条"语义因此成立
- payload 结构：`{ paths: string[], n: number }`（排序去重）

## 4. harness-log.ts 词汇扩展点

- `lib/harness-log.ts:HarnessEventKind`（L89 union）加 `| "edit.map"`；`RETENTION_DAYS`（L118）加 `"edit.map": 30`——kind 为 TEXT 列零迁移（spill.write/policy.decision 同款先例）
- 写入走 `logEvent(cwd, {kind, sessionId, change, payload})`（L228）

## 5. spec-gate.ts 检查⑤'模式（warn 先例）

- warn 输出：`spec-gate.ts:warn()` → `pi.sendMessage({customType:"spec-gate-warning", content, display:true}, {deliverAs:"steer"})`（检查⑤ warnAcceptanceWording 同款）
- 记账：`spec-gate.ts:auditPolicy(ctx, change, {policy:"spec-gate", action:"warn", reasonCode:"concurrent-dirty-tree", target})`；`lib/policy-decision.ts:logPolicyDecision` action:"warn" 在白名单，reasonCode `concurrent-dirty-tree` 合法 kebab-case
- `spec-gate.ts:runScript` 只返回 {ok, detail}（ok = code===0）——⑤'需区分 exit 0/2/3，给 runScript 返回值加 code 字段（既有调用只用 ok/detail，无破坏）
- 挂点：gateArchive 内检查⑤（warnAcceptanceWording）之后、`if (failures.length === 0)` 之前，warn 不进 failures（不 block）
- change 名提取：`spec-gate.ts:extractChangeName`；ctx 需 cwd（auditPolicy 依赖 ctx.cwd）

## 6. doc-impact.sh verify 双轨改造点

- `scripts/doc-impact.sh:changed_files`（L26-37）产出全树改动（tracked diff + staged + untracked）；`heuristic_hit`（L41-77）按域正则命中
- cmd_verify 中 `changed="$(changed_files "$base" | sort -u)"` 是规则3/4（疑似遗漏/声明 none 但命中）与规则2/5（声明了未更新/路径不存在）的**共用输入**
- 改造：新增局部 `heuristic_input`——sqlite3 只读查 events.db 该 change 最新 edit.map paths，非空则 = 归属集合，空/查询失败/库缺/sqlite3 缺 = 回退 `$changed`；规则2/5 继续用 `$changed`（文档对账以 git 为准，spec 明确）
- excuse 解析在 L176-181（保留不动，兼容零破坏）；change 名 = `basename "$change_dir"`
- ⚠ 性能坑（L41 注释）：调用方必须预计算文件列表传入——concurrency-status.sh 同样一次 git 调用复用全部分段（DrvFS git 扫描秒级~30s 级）

## 7. smoke 体系

- extension 侧：`.pi/extensions/tests/run-harness-smoke.sh`（esbuild bundle 各 ts → .cjs 跑断言）；harness-log.smoke.cjs（TTL/词汇）/ quality-gate.smoke.cjs / spec-gate.smoke.cjs 是扩展点。纯函数抽 lib/edit-map.ts 供白盒直测（不 import fs 不触网，同 scanAcceptanceWording 先例）
- scripts 侧先例：`scripts/check-standards.smoke.sh`、`scripts/scenario-trace.smoke.sh`（bash smoke + fixtures 临时目录/临时库）

## 8. concurrency-status.sh 关键判定矩阵（--check 模式）

| 脏文件状态 | exit | 语义 |
|---|---|---|
| 空 / 全部归属本 change | 0 | 干净 |
| 存在归属其他 **active** change（openspec/changes/ 非 archive 目录）的文件 | 2 + stdout JSON | warn |
| 全部无归属（冷启动/纯 bash 会话） | 3 | 跳过零提醒 |
| sqlite3 不可用 / 库不存在 / 查询失败 | 3 | fail-open 跳过 |

- 归属其他但已 archive 的 change 视同无归属（归档即 commit，理论不脏；出现即无主文件）
- SQLite 只读：`sqlite3 "file:.pi/harness/events.db?mode=ro"` + busy_timeout 5000
