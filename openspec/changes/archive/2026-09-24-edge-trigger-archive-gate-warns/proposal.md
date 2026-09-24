<!-- complexity: complex -->
<!-- ui-impact: none -->
<!-- constraint-domains: 无（纯 harness change，不涉业务域代码） -->

## Why

spec-gate 的归档 warn（检查⑤ 验收措辞扫描、检查⑤' concurrent-dirty-tree）**每次 `openspec archive` 尝试都全量重发**：归档流程是「被 block → 修一项 → 重试」的循环，循环里脏文件清单和任务措辞通常不变，同一条 warning 原样再来一遍。事实库实锤（2026-09-19~23）：`relax-lane-snapshot-length-caps` 同会话 6 分钟内 warn 两次、`unify-feed-summary-toggles` / `task-cost-metrics` / `render-mermaid-diagrams` / `harden-archive-readiness` 各同会话两次、`harden-subagent-constraint-channel` 重试 7 次随 warn 5 次；harness-retro 已将 `spec-gate/concurrent-dirty-tree`（一周 31 次）判入「软提醒失效组」。昨日归档的 `aggregate-concurrent-gate-warns` 修了 quality-gate 半边（失败指纹边沿触发），但 proposal 明确把 spec-gate 排除在改动面外（理由「每尝试至多一条」）——「每尝试至多一条」× 尝试 N 次仍是噪声，本 change 补齐另半边。

## What Changes

1. **归档 warn 边沿触发**：检查⑤ / ⑤' 的 warning 改为 per-sessionId 指纹态驱动——同会话同指纹（warn 内容未变）静默；首见、指纹变化、session compact 后重发；上次 warn 过本次转绿（exit 0 / 零违例）发一行 ✓ 收尾（对齐 quality-gate 失败指纹状态机与 constraint-injection 指纹 diff 的既有语义）。状态**会话内存态、会话边界清零、有界（LRU）**，不做跨会话/磁盘持久化——新会话的 agent 没有旧警告上下文，首见警告必要；events.db 为 append-only 账本不适合存状态。
2. **指纹取值稳定化**：⑤' 对 `concurrency-status.sh --check` 的文件清单**排序去重后取指纹**（防输出顺序抖动造成假 diff）；⑤ 对违例文案列表 join 后取指纹。
3. **审计类 warning 不去重**：`--force` / `SPEC_GATE_BYPASS=1` 豁免放行、fail-open、提取不到 change 名的 warning 保持每尝试一条——它们是独立动作的留痕，不是重复提醒。
4. **block 裁决与记账不变**：检查①-④' 的 block 判定、`policy.decision(action=block)` 每尝试一条均不动；warn 的 `policy.decision` 记账与展示走同一边沿（同指纹会话内至多一条，对齐 aggregate-concurrent-gate-warns D2 口径）。词汇表（action/reasonCode 枚举）不变。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `concurrent-change-coordination`: 「归档前并发检查」Requirement 补充 warn 的边沿触发语义——同会话同指纹（文件清单未变）至多输出一条 warn、指纹变化/compact 后可重发、转绿一行收尾；记账与展示同边沿。
- `test-case-design`: 「归档措辞 warn 检查（spec-gate 检查⑤）」Requirement 补充同指纹静默语义（同会话同违例集合至多投递一轮），扫描纯函数与 fail-open 边界不变。

## Impact

- 代码：`.pi/extensions/spec-gate.ts`（唯一改动面，预计 ~40 行）：新增 per-sessionId 指纹态表（参照 constraint-injection `sessionStates` 的会话隔离 + LRU 有界先例与 quality-gate 失败指纹状态机）、`session_compact` 清指纹、⑤/⑤' 投递点改造。
- 事实库：`policy.decision` 词汇表不变；`concurrent-dirty-tree` 计数自然回落（重复重试部分消失）。
- 回检指标（归档后 7 天窗口）：同 `session_id + change` 的 `concurrent-dirty-tree` warn 账本条数——现状存在多条对（如 relax-lane-snapshot-length-caps 同会话 2 条），改造后每对至多 1 条（指纹变化重发除外）；⑤ 的 acceptance-wording 同口径。
- 风险：静默后若 agent 中途丢失 warn 内容（被 compact 摘要），compact 重发 + `concurrency-status.sh` / `gate-status.sh` 拉模式查询兜底（aggregate-concurrent-gate-warns 刚落的查询入口）。
