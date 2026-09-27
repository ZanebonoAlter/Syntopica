<!-- complexity: complex -->
<!-- ui-impact: none -->

## Why

现有 `harness-retro.sh` 六段报告是纯故障视角（失败聚类/回归翻转/软提醒失效），只能回答"哪里坏了"，回答不了"插件（constraint-injection / quality-gate / pin 等）到底有没有用、代价多大"。同时 token 消耗与轮次数据只存在于 pi 的 session jsonl（滚动保留最近 ~2 天，实测 24 文件/79MB），不落账本就永远无法回溯——效能评价的第一步是把这两个维度的数据从 session 日志沉淀进 events.db。

## What Changes

- **新增 `session.rollup` 事件**（harness-telemetry 扩展）：session 进行中按 turn_end 节流写快照（轮次/步数/工具调用数/token 四元组/成本/时长/模型），session_start 时回填 prev session 终值；同 session 取最新一条即终值（复用 edit.map 快照覆盖语义）。
- **`harness-retro.sh` 报告扩展**：保留现有六段故障视角不动，新增"效能看板"段——四组维度：
  - A 插件 ROI：注入负载（每 session 注入字节/稳态零投递率）、注入命中率（注入域 vs 后续实际编辑路径重合度）、pin 复用率、门禁催修效率（block→转绿耗时/同 reasonCode 复发）
  - B 效率基线（rollup 驱动）：每 change/session 的轮次、token、成本、墙钟分布（中位数/P75）
  - C 返工信号：同文件跨 session 编辑波次、归档重试次数（现有②段回归翻转保留在故障段）
  - D 健康：现有③⑤⑥原样保留
- **基线 A/B 机制扩展**：`--save-baseline`/`--baseline` 的 JSON 覆盖新指标 key，支持"改一条 harness 规则前后效能指标对比"。
- **口径透明**：rollup 数据从部署起积累，报告对"窗口内 rollup 覆盖率不足"显式降级标注（沿用 T5/T6 白盒降级告警先例），不静默用小样本冒充分布。

## Capabilities

### New Capabilities

（无——全部落在既有 harness 词汇与报告契约上）

### Modified Capabilities

- `harness-fact-log`: 事件类型词汇新增 `session.rollup`（保留期 90 天、payload 契约、快照覆盖取终值语义、session_start 回填 prev 语义）
- `harness-retro-loop`: 报告分段从六段扩展为"六段故障视角 + 效能看板段"；可回检指标词汇表扩展效能指标；基线快照结构扩展

## Impact

- `.pi/extensions/harness-telemetry.ts`（rollup 埋点）、可能新增 `.pi/extensions/lib/` 辅助函数（jsonl usage 解析）
- `scripts/harness-retro.sh`（报告扩展）、`scripts/harness-retro.smoke.sh`（断言扩展）
- `.agents/skills/harness-facts/SKILL.md`、`.agents/skills/harness-retro/SKILL.md`（词汇与读法同步）
- `docs/research/harness-effectiveness-metrics/explore-findings.md`（探索发现，已在）
- 不触碰产品代码（front/backend-go 零影响）、不触碰注入通道配置、retro 只读契约不变（sqlite3 -readonly）
