# task-cost-metrics Design

## Context

实测账本（2026-09-19，.pi/harness/events.db 3.4 万事件）四个缺口：① subagent.dispatch cost 字段 270 条全空（Agent 工具 usage=null，tokens 仅 "37.4k" 标签近似值）；② final=true 终值回填从未生效——pi 的 session_start 事件 30/30 不带 previousSessionFile，D1② 回填机制事实死亡，尾段（<5 turn 未触发节流）漏记；③ rollup 只记会话末 model 单值，而 session jsonl 每条 assistant 消息自带 `model`/`provider`/`usage.cost.total`（消息级成本，含 input/output/cacheRead 分项）——数据在、没聚合；④ 子会话已在写自己的 rollup（实测 14/30 近期会话文件为带 parentSession 头的子会话），但 payload 无父链，成本归因到任务断链。分母侧无归档成功事件（spec-gate 成功零记录），外部锚点 = openspec/changes/archive/ 目录 131 个全部带 `YYYY-MM-DD-name` 日期前缀。

## Goals / Non-Goals

**Goals**：rollup 分模型成本（models map）；子会话→父→任务归因链（parentSessionId）；final 回填修复；归档成功锚点事件（change.archive）；retro ⑦B' 每任务成本子组（模型/complexity 分桶 + 不可归因披露 + 幸存者偏差对照）。

**Non-Goals**：不改 subagent.dispatch/complete payload（子线程成本由子会话自身 rollup 覆盖，补 cost 会双计数）；不追溯 09-17 前（rollup 上线前）历史成本（永久盲区，报告标注）；不追溯已删除/放弃的 change（目录删除无痕，活跃未归档数仅作对照）；不递归孙会话归因（pi 普通子线程不再派发子线程）；不自建单价表估算（成本全部取 pi 记账的 `usage.cost`，绝不自行 tokens×单价）。

## Decisions

### D1 models 聚合：lib/session-rollup.ts 消息级归属

`SessionUsageSummary` 增 `models: Record<string, {tokens: 五值, cost: number|null}>`；`consumeLines` 在既有 assistant 分支内按 `d.message.model` 归属聚合（缺 model 字段 → `unknown` 桶），单调不减口径与 tokens 一致。payload 新增可选键 `models`，老读者忽略。
备选（telemetry 层二次解析 jsonl）被否：双份解析逻辑必然漂移。

### D2 parentSessionId：写入侧一次提取

telemetry turn_end 已持有当前会话文件路径 → 读首行 session 头 JSON 的 `parentSession`（指向父会话文件路径）→ basename 提取 UUID 为 `parentSessionId`；进程内缓存一次（头不变）；主会话缺字段/提取失败省略键（fail-open）。
备选（retro 读侧扫全量会话文件建父子索引）被否：慢、跨机状态不可依赖、写入侧一次搞定永久有效。

### D3 final 回填：自扫 sessions 目录定位 prev

session_start 时：当前会话文件所在目录（`~/.pi/agent/sessions/<encoded-cwd>/`）下列 `*.jsonl`，排除当前 session id，取 mtime 最新一个为 prev 候选；`queryBySession` 查该 prev 无 `final=true` 时回填终值（幂等保护沿用）。文件选择抽纯函数 `findPrevSessionFile(files, currentId)` 可直测。
已知近似：多会话并发时 prev 候选可能是另一并行活跃会话——接受，因为「同 session 取最新一条即终值」语义下，其后续快照自然覆盖误回填的中间值，且 hasFinal 幂等保证不重写。
备选（继续等 pi 事件带 previousSessionFile）被否：实测 30/30 从不携带，机制已死。

### D4 change.archive：telemetry 挂 tool_result 配对记账

tool_call 时暂存命中 `openspec\s+archive` 的 `toolCallId → command`（内存 map，与既有 agentStarts 同模式）；tool_result 配对且非 isError → 提取 change 名（正则语义同 spec-gate：命令行 `openspec archive` 后首个非 flag 词 + 合法字符集校验）→ `logEvent(kind="change.archive", change=name, payload={name})`。CLI 失败（isError）、被 block（结果为错误）、提取不到合法名（交互式）→ 零记录 fail-open。挂在 telemetry 而非 spec-gate：telemetry 是事实记账方，spec-gate 是裁决方且只挂 tool_call（命令执行前）看不到结果。
这是「普通成功放行零记录」低噪声约束的唯一成功侧事实例外（归档低频、高价值锚点），已在 spec 与 skill 词汇表登记。

### D5 retro 归因链与 B' 指标口径

终值集（每 session 最新一条 rollup）三路归因：主会话（无 parentSessionId）按 change 列直归 change；子会话（有 parentSessionId）→ 父会话终值的 change 列；无 change 且无归因路径 → unattributed（Σcost 披露占比，不计入任务均值）。archived 双锚点 = 窗口内 change.archive 事件 ∪ archive 目录日期前缀（剥前缀）；单侧缺失以另一侧补齐并输出不对称提示。per-change cost = Σ 归因终值 cost（主+子）；models 桶 = Σ models map（缺字段 → 「未分模型」桶）；complexity 桶 = 归档目录 proposal.md 头 `<!-- complexity: -->`（缺 → 「未声明」桶）。指标键：`m7.task_cost_p50/p75/avg/count`、`m7.task_cost_by_model`、`m7.task_cost_by_complexity`、`m7.task_unattributed_cost_pct`、`m7.task_active_unarchived`。
双计数防护：主/子会话 session_id 天然不同不重复；subagent.dispatch 的 tokens 标签值不参与成本口径。

### D6 输出与基线

⑦B 之后渲染 B' 小节（人读表：分布 + 两分桶 + 披露行）；`--json` 沿用 metrics 平铺机制自动进 `--save-baseline`/`--baseline` 比对。

### D7 快照同步

`.pi/extensions/` gitignored：改动后按 harness-lifecycle 先例同步源码与 tests 到 `docs/research/` 入库快照（实现时核对既有快照目录组织，覆盖同名文件并附本 change 说明）。

### D8 兼容降级矩阵

| 存量数据形态 | 降级行为 |
| --- | --- |
| rollup 无 models 字段 | 计入「未分模型」桶 |
| rollup 无 parentSessionId | 子会话成本进 unattributed（随数据换代自然衰减） |
| change.archive 缺失（TTL/手工移目录） | archive 目录锚点兜底 + 不对称提示 |
| 09-17 前会话无 rollup | rollup 覆盖率降级标注（既有机制） |

## Risks / Trade-offs

- 回填并发近似：见 D3，接受并文档声明。
- models 数值与顶层 cost 的对账：各桶之和应等于会话总 cost（消息级同源累加，天然成立）；smoke 断言。
- 归档后手工重命名目录：名称对不上 → 该 change 成本进 unattributed；低概率接受。
- B' SQL 每行 json_extract 开销：窗口 7 天万行级无压力（既有 ⑦ 段同量级先例）。

## Migration Plan

无 DB schema 迁移：payload JSON 扩展可选键 + 新 kind（TEXT 列）。telemetry 新代码在 pi 会话 reload 后生效；存量事件不动。改造后新字段即时开始积累，B' 子组对存量自动降级（D8 矩阵）。

## Open Questions

无——分模型可行性（消息级 model+cost）、子会话定位（parentSession 头）、归档锚点（目录日期前缀）均已在真实数据实测验证（docs/research/harness-cost-metrics/explore-findings.md）。
