# task-cost-metrics 白盒用例（complexity: complex）

用例先行锚点：开发执行规范 §2 + standard/shared/test-design.md。覆盖三块白盒核心：session-rollup 聚合边界、telemetry 提取/判定纯函数、retro B' 口径。每用例标注 spec Scenario 来源。

## A. session-rollup models 聚合（session-rollup.smoke.cjs）

| # | 输入 | 期望 | Scenario |
| --- | --- | --- | --- |
| A1 | 会话先用 modelA 3 条 assistant（cost 0.1/0.1/0.1）再切 modelB 2 条（cost 0.05/0.05） | models.A.cost=0.3、models.B.cost=0.1；两桶之和 = 顶层 cost=0.4 | models 按消息级 model 字段归属 |
| A2 | 1 条 assistant 带 usage 无 model 字段（cost 0.2） | models.unknown.cost=0.2，顶层 cost=0.2，无静默丢弃 | 无 model 字段的消息计 unknown 桶 |
| A3 | 1 条 assistant model=modelA 但 usage.cost 缺失 | models.A 存在且 cost=null（容忍，与顶层 cost null 同口径） | models 按消息级 model 字段归属 |
| A4 | 同一会话分两次 consumeLines 增量推进（模拟节流快照间推进） | 第二次快照 models 各桶 ≥ 第一次（单调不减） | turn_end 节流快照落库 |
| A5 | tokens 五值分桶 | models.A.tokens.total = A 消息 totalTokens 之和，input/output/cacheRead/cacheWrite 同理 | models 按消息级 model 字段归属 |
| A6 | 坏 JSON 行 + 空白行混入 | skippedLines 计数，models 不受影响 | 解析失败 fail-open |

## B. parentSessionId / findPrevSessionFile（session-rollup.smoke.cjs）

| # | 输入 | 期望 | Scenario |
| --- | --- | --- | --- |
| B1 | session 头 `{"session":{"parentSession":"/path/to/2026-.._01a0b810-….jsonl"}}` | readParentSessionId 返回 01a0b810-…（basename 剥时间戳前缀） | 子会话携带 parentSessionId |
| B2 | 首行坏 JSON / 无 parentSession 字段 / 文件不存在 | 返回 null，不抛出 | 解析失败 fail-open |
| B3 | 目录下 [当前(mtime 最新), other.jsonl(次新), third.jsonl(最旧)] | findPrevSessionFile 返回 other.jsonl（排除当前会话后取最新） | session_start 回填 prev 终值 |
| B4 | 目录下仅当前会话一个文件 / 目录空 | 返回 null → 零写入 | session_start 回填 prev 终值（jsonl 缺失零写入） |
| B5 | 两文件 mtime 完全相同 | 返回其中确定一个（排序稳定，不断言具体哪个） | session_start 回填 prev 终值 |

## C. change.archive 判定（telemetry-archive.smoke.cjs）

| # | 输入 | 期望 | Scenario |
| --- | --- | --- | --- |
| C1 | `openspec archive task-cost-metrics` + isError=false | 提取 name=task-cost-metrics，记账一次 | 归档成功记账 |
| C2 | `openspec archive --force some-change`（flag 前置） | name=some-change（跳过 flag 词） | 归档成功记账 |
| C3 | `openspec archive`（无名字，交互式） | 返回空 → 零记录 | 提取不到 change 名 fail-open |
| C4 | `openspec archive ../evil\\path`（非法字符集） | 校验拒绝 → 零记录 | 提取不到 change 名 fail-open |
| C5 | tool_result isError=true（CLI 失败/被 block） | 零记录 | 归档被阻断零记录 |
| C6 | tool_call 暂存后 tool_result 未到达（进程重启丢 map） | 零记录、不抛出（fail-open） | 归档被阻断零记录 |

## D. retro B' 口径（harness-retro.smoke.sh fixture）

| # | fixture | 期望 | Scenario |
| --- | --- | --- | --- |
| D1 | change X：archive 目录 `2026-09-18-X` + 主会话终值 cost 1.2（change=X）+ 子会话终值 cost 0.3（parentSessionId→主会话） | X 任务成本 1.5；P50/P75/均值可手算复算 | 每任务成本含子会话且可复算 |
| D2 | change Y：rollup 无 models 字段，cost 0.8 | Y 成本计入「未分模型」桶，不并入具体模型桶 | 模型分桶缺失降级 |
| D3 | change Z：仅 archive 目录存在、无 change.archive 事件 | Z 仍计入统计 + 输出锚点不对称提示 | 双锚点单侧缺失对账 |
| D4 | 会话 W：change 列 null、无 parentSessionId、cost 0.5 | 不可归因成本 0.5 单独披露，不入任何任务均值；活跃未归档 change 数输出 | 不可归因成本披露 |
| D5 | models map：X 主会话 {glm-5.3: 1.0, glm-5.3-flash: 0.2}、子会话 {glm-5.3-flash: 0.3} | by_model：glm-5.3=1.0、glm-5.3-flash=0.5 | 每任务成本含子会话且可复算 |
| D6 | 归档目录 proposal.md 头 complexity=simple / 无头 | 分桶分别落 simple 桶与「未声明」桶 | （complexity 分桶实现口径） |

## E. 真实库人工冒烟（不进自动 smoke）

| # | 操作 | 期望 |
| --- | --- | --- |
| E1 | 本 change 归档流程走真实 `openspec archive` | 账本出现 change.archive 一条（name=task-cost-metrics） |
| E2 | 归档后新开 pi 会话（触发 session_start 回填） | final=true 终值计数开始增长（机制复活） |
| E3 | `harness-retro.sh --days 7` | B' 小节出现且 09-17 后归档 change 成本非空 |
