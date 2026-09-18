# Tasks — harness-effectiveness-metrics

## 1. telemetry 扩展：session.rollup 埋点

- [x] 1.1 新增 `.pi/extensions/lib/session-rollup.ts`：纯函数 jsonl 聚合器（`parseSessionUsage(lines) → {turns, steps, toolCalls, tokens, cost, durationSec, model, skippedLines}` + 增量 offset 状态管理），坏行跳过计数，全部 fail-safe（对齐 parseSubagentSummary 先例）
- [x] 1.2 `harness-telemetry.ts` 挂 `turn_end`：`ctx.sessionManager.getSessionFile()` → 增量读 → 节流（turn%5==0 或 tokens 增量>20%）写 `session.rollup` 快照（`final:false`，change 列用 `detectActiveChange`，payload 契约见 spec「session 效能汇总记账」）
- [x] 1.3 `session_start` 分支扩展：`event.previousSessionFile` 存在且账本中该 session 无 `final=true` 快照时，全量解析 prev jsonl 补写终值（`final:true`，session_id 取文件名 UUID）；jsonl 缺失/解析失败零写入 + console 告警
- [x] 1.4 `harness-log.ts` 词汇表登记 `session.rollup`（保留期 90 天，TTL 表驱动的话加一行）
- [x] 1.5 直测：`.pi/extensions/tests/` 补 session-rollup 聚合器用例（正常 fixture/坏行/空文件/offset 增量/终值 final 标记），跑通现有测试命令

## 2. retro 脚本：⑦效能看板段

- [x] 2.1 rollup 终值查询视图（`MAX(id) GROUP BY session_id` 模式）与覆盖率计算，注入现有统计装配段
- [x] 2.2 A 组指标 SQL：注入负载（per session Σbytes 中位数/P75 + degraded 计数）、pin 复用率、催修时距（gate.check 同 `(session,cmd)` 红→绿 ts 差 + policy.decision block→绿）、block 复发清单
- [x] 2.3 域映射提取器：解析 `docs/reference/flow/*.md` 头部 `doc-impact-applies`（实现前核对一篇实际格式）→ 路径前缀→域表；解析失败输出「域映射不可用」降级标注
- [x] 2.4 注入命中率：inject(declaration/keyword/edit) 的域 × edit.map 路径映射域，逐 change 命中率 → 中位数
- [x] 2.5 B 组指标 SQL：per change/per session 的 turns、tokens(total/output 分列)、cost、durationSec 中位数+P75（PERCENT_RANK）；子线程 token 占比（非 null 样本 + 样本数标注）
- [x] 2.6 C 组指标 SQL：返工波次（edit.map 按 session×json_each(paths) 去重 → 文件→sessions 集合 Top10，≥3 才列）、归档重试（per change archive-check-failed 计数）
- [x] 2.7 渲染：⑦段中文标题 + 指标行 + 覆盖率头部标注（<50% 追加「数据积累中」）；确认无任何综合分行；JSON 输出模式同构
- [x] 2.8 基线：`--save-baseline` JSON 新增 `metrics7` 键组；`--baseline` 差值覆盖；旧基线无 metrics7 → 跳过该组比对并提示

## 3. 测试（smoke 与验收）

- [x] 3.1 `harness-retro.smoke.sh` 扩展：fixture 加 rollup 快照序列 / 无 rollup session / 跨域注入 change 三类数据；断言七段齐备、终值取最新快照不累加、覆盖率正确、命中率=0、降级标注出现、无综合分
- [x] 3.2 真实库手动验收：`bash scripts/harness/harness-retro.sh --days 30` 看⑦段降级形态；连续两次 `--save-baseline` 后跑 `--baseline` 看 metrics7 差值
- [x] 3.3 telemetry 冒烟：起一个 pi 会话跑几轮 turn 后查 `session.rollup` 快照落库与节流行为；重启 pi 确认 prev 终值回填
  - 验证结论（2026-09-17T12:51Z 实测）：① turn_end 节流快照 ✓——重启后连续两条快照数值单调不减（steps 208→209、tokens.total 29.51M→29.74M、cost ¥8.94→¥9.00、model=glm-5.3），turns=7 与会话实际用户轮次一致；② retro ⑦段即时消费成功（覆盖率 1/59、per-session 分布出真值、子线程占比 2%）。
  - 已知边界（非 bug，留痕）：a) pi 重启的 session_start 事件 `previousSessionFile=null`（内核行为），prev 终值回填路径在此场景不触发——turn_end 快照持续覆盖已保证数据不丢（取最新一条即终值），回填保留给带 prev 的场景；b) change 列用 `detectActiveChange` 磁盘检测语义（design D2 如此，对齐 subagent.dispatch 既有语义）——多会话共享 cwd 时归属可能漂移到其他活跃 change，属已知边界。

## 4. 文档

<!-- doc-impact: none(harness 工具链改动：pi 扩展埋点 + retro 脚本效能看板段；改动仅限 .pi/extensions/、脚本、skill 文档、AGENTS.md、docs/research/，不触及 reference 各域文档语义) -->

- [x] 4.1 `.agents/skills/harness-facts/SKILL.md`：词汇表加 `session.rollup` 行（保留期/payload/快照覆盖取终值/prev 回填语义）
- [x] 4.2 `.agents/skills/harness-retro/SKILL.md`：报告结构六段→七段说明 + ⑦段读法（覆盖率/降级标注/禁综合分）
- [x] 4.3 `AGENTS.md` pi 扩展全景表 harness-telemetry 行补 rollup 职责；`docs/reference/harness/pi-extensions.md` 词汇同步（harness 机制权威源已收敛至 reference/harness，词汇实际落该文档与 harness-facts skill，见 4.1；原计划的 research 池单文件未建，不作声明）
- [x] 4.4 `docs/reference/开发执行规范.md` §12.5 若引用报告段数则同步（检索“六段”字样更新）

## 5. 验证

- `bash scripts/harness/harness-retro.smoke.sh` → 退出码 0，通过 57 / 失败 0（2026-09-18 归档前复跑）
- `bash .pi/extensions/tests/run-harness-smoke.sh` → 退出码 0，全套 SMOKE OK（含 harness-log / spec-gate / quality-gate / quality-gate.behavior / policy-decision / dev-process-guard / session-rollup 等，2026-09-18 归档前复跑）
- `bash scripts/harness/harness-retro.sh --days 7 --json` → JSON 可读，`m7.domain_hit_pct=58`（≥30，真实库 2026-09-18）

> 注：harness 脚本自 2026-09-18 scripts 目录重组后位于 `scripts/harness/`（原 `scripts/`），上表命令按新路径实测。

| Scenario | 测试文件 |
| --- | --- |
| TTL 分级清扫 | .pi/extensions/tests/harness-log.smoke.cjs |
| 事件追加不可变 | .pi/extensions/tests/harness-log.smoke.cjs |
| spill.write 事件随词汇扩展落库 | .pi/extensions/tests/harness-log.smoke.cjs |
| subagent.complete 随词汇扩展落库 | .pi/extensions/tests/harness-log.smoke.cjs |
| policy.decision 随词汇扩展落库 | .pi/extensions/tests/policy-decision.smoke.cjs |
| edit.map 随词汇扩展落库 | .pi/extensions/tests/harness-log.smoke.cjs |
| session.rollup 随词汇扩展落库 | 人工（tasks 3.3 真实账本冒烟留痕：快照落库/节流/取终值实测通过；91 天 TTL 为 harness-log.ts 表驱动登记） |
| turn_end 节流快照落库 | .pi/extensions/tests/session-rollup.smoke.cjs |
| 同 session 取最新一条即终值 | scripts/harness/harness-retro.smoke.sh |
| session_start 回填 prev 终值 | .pi/extensions/tests/session-rollup.smoke.cjs |
| 解析失败 fail-open | .pi/extensions/tests/session-rollup.smoke.cjs |
| rollup 不改写既有事件 | .pi/extensions/tests/session-rollup.smoke.cjs |
| 六段齐备且各带指标 | scripts/harness/harness-retro.smoke.sh |
| 七段齐备且各带指标 | scripts/harness/harness-retro.smoke.sh |
| 失败特征归并 | scripts/harness/harness-retro.smoke.sh |
| 按 change 归属限定 | scripts/harness/harness-retro.smoke.sh |
| 生成基线与差值 | scripts/harness/harness-retro.smoke.sh |
| 基线损坏时降级 | scripts/harness/harness-retro.smoke.sh |
| rollup 覆盖率显式标注 | scripts/harness/harness-retro.smoke.sh |
| 注入命中率可复算 | scripts/harness/harness-retro.smoke.sh |
| 门禁催修时距可复算 | scripts/harness/harness-retro.smoke.sh |
| 返工波次清单 | scripts/harness/harness-retro.smoke.sh |
| 禁止综合总分 | scripts/harness/harness-retro.smoke.sh |
| 子线程 token 占比可复算 | 人工（真实库 --json 复核 m7.sub_samples=44>0，分子仅含有值样本并标注样本数；fixture 未覆盖子线程事件，疑点已由 harness-retro-sql-fixes 登记观察至 2026-10-01） |
