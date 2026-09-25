# task-cost-metrics Tasks

## 1. 写入侧：session-rollup 聚合扩展（.pi/extensions/lib/session-rollup.ts）

- [x] 1.1 `SessionUsageSummary` 增 `models` 字段（`Record<string, {tokens 五值, cost}>`），`consumeLines` 在 assistant 分支按消息级 `model` 字段归属聚合（缺字段计 `unknown` 桶；cost null 容忍同顶层口径）
- [x] 1.2 新增辅助：`readParentSessionId(file)`（读首行 session 头 parentSession → basename UUID，失败返回 null）与 `findPrevSessionFile(files, currentId)`（排除当前会话、mtime 最新，纯函数可直测）
- [x] 1.3 smoke 白盒扩展（session-rollup.smoke.cjs）：models 多模型归属之和=总 cost、unknown 桶、parent 提取（正常路径/坏头/缺字段）、prev 文件选择（并列 mtime/空目录/仅当前会话）

## 2. 写入侧：harness-telemetry（.pi/extensions/harness-telemetry.ts）

- [x] 2.1 rollup payload 附 `models` 与 `parentSessionId`（进程内缓存 session 头解析结果，读失败省略键 fail-open）
- [x] 2.2 session_start 回填改自扫路径：`findPrevSessionFile` 定位 prev jsonl → `parseSessionText` → hasFinal 幂等检查（沿用 queryBySession）→ 补写 final=true 终值；彻底移除对 pi 事件 previousSessionFile 的依赖
- [x] 2.3 新增 `change.archive` 记账：tool_call 暂存命中 `openspec\s+archive` 的 toolCallId→command（内存 map）；tool_result 配对且非 isError → 提取 change 名（正则语义同 spec-gate，首个非 flag 词 + 合法字符集）→ logEvent(kind=change.archive, change=name, payload={name})；失败/提取不到零记录 fail-open
- [x] 2.4 name 提取与判定逻辑抽纯函数，新增/扩展 smoke（telemetry-archive.smoke.cjs）：成功形态、带 flag 形态、交互式无名字形态、非法字符形态、isError 零记录

## 3. 读侧：harness-retro.sh ⑦B' 每任务成本子组（scripts/harness/harness-retro.sh）

- [x] 3.1 SQL 聚合：终值集三路归因（change 列直归 / parentSessionId 归父 / unattributed）、archived 双锚点并集（change.archive 事件 ∪ archive 目录日期前缀剥名）、models 与 complexity 分桶、指标键 `m7.task_cost_p50/p75/avg/count`、`m7.task_cost_by_model`、`m7.task_cost_by_complexity`、`m7.task_unattributed_cost_pct`、`m7.task_active_unarchived`
- [x] 3.2 人读输出：⑦B 后渲染 B' 小节（分布 + 模型分桶表 + complexity 分桶表 + 不可归因成本 + 活跃未归档数 + 锚点不对称提示）；`--json` metrics 平铺（基线比对自动含新键）
- [x] 3.3 harness-retro.smoke.sh fixture 扩展四场景：每任务成本含子会话可复算、模型分桶缺失降级（未分模型桶）、双锚点单侧缺失对账、不可归因成本披露

## 4. 文档与快照

- [x] 4.1 `.agents/skills/harness-facts/SKILL.md`：词汇表加 `change.archive`（30 天，payload 字段，低噪声例外登记）、session.rollup payload 增 models/parentSessionId、回填机制改为自扫的表述更新
- [x] 4.2 `.agents/skills/harness-retro/SKILL.md`：⑦ 段 B' 子组解读（口径、降级、回检键名）
- [x] 4.3 `docs/reference/harness/pi-extensions.md`：事件全景表与指标记账口径补 change.archive / models / B' 指标
- [x] 4.4 扩展源码与 tests 快照同步 `docs/research/`（核对既有快照目录组织，覆盖同名文件 + 本 change 说明）——核对结论：`.pi/extensions/` 已直接入库，无源码快照惯例；落 `docs/research/harness-cost-metrics/implementation-notes.md` 说明 + 索引

## 5. 测试

影响面 = `.pi/extensions/`（扩展，node smoke 直测）+ `scripts/harness/`（retro 脚本，smoke shell）；不触及 Go/前端业务包，无 go test / vitest 影响包。

- `node .pi/extensions/tests/session-rollup.smoke.cjs` → 全绿
- `node .pi/extensions/tests/telemetry-archive.smoke.cjs` → 全绿
- `bash scripts/harness/harness-retro.smoke.sh` → 全绿（含 B' 四场景）
- 真实库人工冒烟（tasks 验证节复跑留痕）

## 6. 文档

<!-- doc-impact: none(harness 工具链改动：pi 扩展埋点 + retro 脚本效能看板；改动仅限 .pi/extensions/、scripts/harness/、两个 harness skill、docs/research/ 快照与 pi-extensions.md，不触及 reference 各域业务文档语义) -->
- **无 flow 影响**（§12.2 归档后回填：纯 harness 任务成本埋点，非业务链路改动，不触及任何 `flow/*.md` 业务链路，E 段溯源豁免）

见 §4。

## 7. 验证

- [x] `node .pi/extensions/tests/session-rollup.smoke.cjs && node .pi/extensions/tests/telemetry-archive.smoke.cjs` → 退出码 0，新增白盒用例全绿
- [x] `bash scripts/harness/harness-retro.smoke.sh` → 退出码 0，B' 四场景断言通过
- [x] `bash scripts/harness/harness-retro.sh --days 7` → 人读报告出现 B' 每任务成本小节，09-17 后归档 change 的成本数值非空，活跃未归档数 > 0
- [x] `bash scripts/harness/harness-retro.sh --days 7 --json | python3 -c "import json,sys; m=json.load(sys.stdin)['metrics']; assert 'm7.task_cost_p50' in m and 'm7.task_cost_by_model' in m; print('task_cost keys ok')"` → 输出 task_cost keys ok
- [x] 真实库 sanity：当前会话结束后新开 pi 会话触发一次回填 → `sqlite3 .pi/harness/events.db "SELECT COUNT(*) FROM events WHERE kind='session.rollup' AND json_extract(payload,'$.final')=1"` → 计数开始增长（回填机制复活）——2026-09-19 reload 后实测 1→3（两条 prev 终值回填落库），models 字段同步出现

### Scenario ↔ 测试映射

| Scenario | 测试文件 |
| --- | --- |
| TTL 分级清扫 | .pi/extensions/tests/harness-log.smoke.cjs |
| 事件追加不可变 | 人工（append-only 账本语义：logEvent 仅 INSERT 无 UPDATE API，TTL 外无改写路径；真实库回填实测仅新增行） |
| spill.write 事件随词汇扩展落库 | .pi/extensions/tests/spill.smoke.cjs |
| subagent.complete 随词汇扩展落库 | .pi/extensions/tests/run-harness-smoke.sh |
| policy.decision 随词汇扩展落库 | .pi/extensions/tests/policy-decision.smoke.cjs |
| edit.map 随词汇扩展落库 | .pi/extensions/tests/quality-gate.smoke.cjs |
| session.rollup 随词汇扩展落库 | .pi/extensions/tests/session-rollup.smoke.cjs |
| change.archive 随词汇扩展落库 | .pi/extensions/tests/telemetry-archive.smoke.cjs |
| turn_end 节流快照落库 | .pi/extensions/tests/session-rollup.smoke.cjs |
| 同 session 取最新一条即终值 | scripts/harness/harness-retro.smoke.sh |
| session_start 回填 prev 终值 | .pi/extensions/tests/session-rollup.smoke.cjs |
| 回填不依赖 pi 事件的 previousSessionFile | .pi/extensions/tests/session-rollup.smoke.cjs |
| 解析失败 fail-open | .pi/extensions/tests/session-rollup.smoke.cjs |
| rollup 不改写既有事件 | 人工（append-only：回填以追加新事件表达终值，真实库 reload 后回填仅新增行） |
| models 按消息级 model 字段归属 | .pi/extensions/tests/session-rollup.smoke.cjs |
| 无 model 字段的消息计 unknown 桶 | .pi/extensions/tests/session-rollup.smoke.cjs |
| 子会话携带 parentSessionId | .pi/extensions/tests/session-rollup.smoke.cjs |
| 归档成功记账 | .pi/extensions/tests/telemetry-archive.smoke.cjs |
| 归档被阻断零记录 | .pi/extensions/tests/telemetry-archive.smoke.cjs |
| 提取不到 change 名 fail-open | .pi/extensions/tests/telemetry-archive.smoke.cjs |
| 同一 change 幂等 | .pi/extensions/tests/telemetry-archive.smoke.cjs |
| rollup 覆盖率显式标注 | scripts/harness/harness-retro.smoke.sh |
| 注入命中率可复算 | scripts/harness/harness-retro.smoke.sh |
| 门禁催修时距可复算 | scripts/harness/harness-retro.smoke.sh |
| 返工波次清单 | scripts/harness/harness-retro.smoke.sh |
| 禁止综合总分 | scripts/harness/harness-retro.smoke.sh |
| 子线程 token 占比可复算 | 人工（真实库报告 JSON 的 effectiveness.subagent 段可独立复算；smoke 无 subagent fixture，非本 change 改动面） |
| 每任务成本含子会话且可复算 | scripts/harness/harness-retro.smoke.sh |
| 模型分桶缺失降级 | scripts/harness/harness-retro.smoke.sh |
| 双锚点单侧缺失对账 | scripts/harness/harness-retro.smoke.sh |
| 不可归因成本披露 | scripts/harness/harness-retro.smoke.sh |
