<!-- complexity: complex -->
<!-- ui-impact: none -->

## Why

retro ⑦B 效率基线已有 per-session/per-change 的成本分布，但回答不了「平均每个任务（change）完成要花多少钱」：分母（归档成功）不在账本——spec-gate 按低噪声约束对成功归档零记录，分母只能靠 openspec/changes/archive/ 目录当外部锚点。同时分子侧有四个实测缺口：① subagent.dispatch 的 cost 字段 270 条全空（Agent 工具 usage=null，tokens 只有 "37.4k" 标签近似值）；② final=true 终值回填从未生效（pi 的 session_start 事件 30/30 不带 previousSessionFile，harness-effectiveness-metrics D1② 机制事实死亡，会话尾段 <5 turn 漏记）；③ 成本拆不出模型（rollup 只记会话末 model 单值，而 session jsonl 每条 assistant 消息其实自带 model+provider+逐条 cost.total——数据在、没聚合）；④ 子会话 rollup 缺 parent 链，成本归因到任务断链（子会话已实测在写自己的 rollup）。数据源头全部可修（已实测验证），补齐写入侧并把归档锚点接进 retro，指标即可持续运转。

## What Changes

- **session.rollup payload 增两个字段**（harness-telemetry / lib/session-rollup.ts）：
  - `models`：`{[modelId]: {tokens 五值, cost}}` 逐模型聚合，按 assistant 消息的 model 字段归属，无 model 字段的消息计入 `unknown` 桶；
  - `parentSessionId`：子会话从自身 jsonl session 头的 `parentSession` 提取，主会话省略——子线程成本经父链归因到任务。
- **final 回填机制修复**（harness-telemetry）：不再依赖 pi 事件的 previousSessionFile（从不携带）；session_start 时自扫 sessions 目录（同 cwd 编码目录下、mtime 最新且非本会话的 jsonl）回填 prev 终值 final=true，堵尾段漏记。
- **新增 `change.archive` 事件**（harness-telemetry）：tool_result 监听 bash 命中 `openspec archive` 且成功 → 记归档成功事实（change 名复用 spec-gate 的提取正则语义）。成功侧记账是对「普通成功放行零记录」低噪声约束的一次显式例外，理由：归档低频、高价值锚点。
- **retro ⑦B 扩「每任务成本」子组**（scripts/harness/harness-retro.sh）：窗口内归档 change（双锚点：change.archive 事件为主，openspec/changes/archive/ 目录日期前缀对账兜底）× 成本归因（主会话 rollup.change 直归 + 子会话经 parentSessionId 归父）× Σcost（主+子），输出按模型分桶、按 complexity 分桶的每任务成本分布（P50/P75/均值）；不可归因成本占比与活跃未归档 change 数单独披露（幸存者偏差对照）；新增 `m7.task_cost_*` 指标键。
- **subagent.dispatch/complete 不改**：子线程成本由子会话自身 rollup 覆盖（实测子会话已在写 rollup），避免补 cost 双计数。
- **文档同步**：harness-facts skill（词汇表 + payload 字段）、harness-retro skill（⑦B 解读）、docs/reference/harness/pi-extensions.md、扩展源码快照同步 docs/research/。

## Capabilities

- **Modified**: `harness-fact-log` — rollup payload 新字段（models / parentSessionId）、final 回填来源改为自扫 sessions 目录、新事件 `change.archive`。
- **Modified**: `harness-retro-loop` — ⑦B 新增每任务成本子组与 `m7.task_cost_*` 指标键。

## Impact

- `.pi/extensions/harness-telemetry.ts`、`.pi/extensions/lib/session-rollup.ts`（gitignored 本机源码，快照同步 docs/research/）、`.pi/extensions/tests/session-rollup.smoke.cjs`（白盒用例扩展）。
- `scripts/harness/harness-retro.sh`（新子组 SQL + 输出段；`--json` 新增指标键）。
- `.agents/skills/harness-facts/SKILL.md`、`.agents/skills/harness-retro/SKILL.md`、`docs/reference/harness/pi-extensions.md`。
- 数据兼容：rollup 新字段向后兼容（老读者忽略）；09-17 前历史会话无 rollup 属永久盲区（报告标注覆盖起点）；存量无 models 字段的 rollup 计入「未分模型」桶。
