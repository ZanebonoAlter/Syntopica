# Review: slim-lane-snapshot-trend

> 审查方式：change-review-glm 子线程（zai-coding-cn/glm-5.3）聚焦审查制品 + diff，2026-10-24。审查报告全文见会话记录；条目按 R-N 建账处置如下。

### R-1 2026-10-24【Medium】量级信号每日行窗口比事实清单窗口少一天，anchor-14 计数取回即弃
- **状态**：已修复
- 处置：`renderLaneMagnitudeSignal` 的 `windowStart` 从 `anchor-13` 改为 `anchor-14`（与素材窗口 `[anchor-14, anchor]` 完全同口径，15 槽铺满，anchor-14 计数不再被弃）；注释自相矛盾处一并修正；`TestRenderLaneMagnitudeSignal_FullWindowAndRollups` 补窗首（anchor-14）无报告行断言并修正「素材窗口起点」的日期锚。

### R-2 2026-10-24【Medium】settle 链路的量级信号注入零测试覆盖
- **状态**：已修复
- 处置：`TestSettleLaneSnapshot_PlainTextSingleVersionUpsert` 把 recorder 提为变量，补 `assert.Contains(rec.users[0], "量级信号")` 接线断言——删除 `ListDailyThreadCounts` 调用时该用例必红。

### R-3 2026-10-24【Medium】console 泳道演进带仍消费存量 detail，三处制品「均只读 summary」陈述失实
- **状态**：已修复
- 处置：定夺为「组件不动 + 制品如实措辞」——`console/BoardLaneEvolution.vue` 的 detail 消费是存量透出（读旧值无害，新结算 detail=nil 后优雅退化为 summary 全文、折叠按钮随长度消失，无功能破坏），随该泳道下个日报日自愈；修正三处制品（specs/board-lane-dynamics Migration、proposal Impact 消费方核对、flow/daily-report.md 消费方句）为「日报阅读视图趋势区只读 summary；console 演进带保留存量 detail 透出至自愈」。

### R-4 2026-10-24【Medium】基线 spec「泳道动态批量端点」长版条款未随修订
- **状态**：已修复
- 处置：board-lane-dynamics delta 补 `## MODIFIED Requirements`「泳道动态批量端点」——态势对象口径改为「summary 单版 + 存量 detail 透出（新结算恒空、不出现）」，删除「长版缺失明示」两级降级 Scenario（随形态废除），换为「存量长版透出」Scenario。

### R-5 2026-10-24【Medium】tasks.md 验证节缺 `| Scenario | 测试文件 |` 映射表，scenario-trace 门禁必挂
- **状态**：已修复
- 处置：tasks.md 验证节补 11 行映射表（board-lane-dynamics 8 + lane-trend-overview 3），实测 scenario-trace 通过。

### R-6 2026-10-24【Low】design D5「温度 0」与代码不符（实际 0.3 未变）
- **状态**：已修复
- 处置：design.md D5 措辞改为「温度 0.3（未动）」。

### R-7 2026-10-24【Low】「缺日补零」措辞与实现行为不一致（实际输出「无报告」）
- **状态**：已修复
- 处置：design D1 与 test-cases U3 措辞统一为「缺日明示无报告」（实现行为不变）。

### R-8 2026-10-24【Low】⓪ 表「继承不动」行与实际改动不符（SN-8/前端 FD-3、D6 有机械适配）
- **状态**：已修复
- 处置：test-cases ⓪ 表相应行改为「继承改写（机械适配：响应/断言随纯文本与新 testid 适配）」。

### R-9 2026-10-24【Low】ui-design「rolling_summary 为空 → 仍走既有空态占位」描述失实（不可达分支）
- **状态**：已修复
- 处置：ui-design.md 受影响状态一节该句改为「summary 空串不可达（结算空守卫保证新写非空）」。

### R-10 2026-10-24【Low】存量 detail 读路径注释陈旧（「降级」概念已废）+ 前端测试 describe 标题沿用「与降级」
- **状态**：已修复
- 处置：`LaneDynamicsSnapshot.Detail`、`TopicLaneSnapshot.RollingDetail` 注释改为「仅存量历史值透出，新结算恒空」；前端测试 describe 标题同步 spec 改名（14 天档内容）。

### R-11 2026-10-28【High】真实结算产物 38/38 顶格、37/38 句子被从字中间切断（机械保护变常态路径）
- **状态**：已修复
- 依据：归档前人工效果核对（tasks 5.3）拉真库 `topic_lane_snapshots`：10-25/26/27 三个日报日新写 38 条快照，全部 ≥200 rune，37 条以半截词结尾（「…停用并移除DeepSeek」「…OpenAI营收预」「…SK」）。内容判据（阶段/驱动、无逐日罗列、无预测、量级数字）全过，唯一红的形态就是可读性——读者在泳道趋势区看到的是断句。
- 处置：不改字数口径（不放宽判据），改两处——①`laneSnapshotSystemPrompt` 加篇幅硬约束（总长含标点 ≤200 字、必须以句号收尾、写不下压缩/合并阶段）；②`parseLaneSnapshotOutput` 改走 `clampLaneSnapshotRunes`：超限优先在 200 字窗口内最后一个句末标点（`。！？…`）收口，标点过早（<60% 预算）或整段无标点才退回 rune 硬切。delta spec「机械截断保护」Scenario 同步，新增 `TestParseLaneSnapshotOutput_ClampsAtSentenceBoundary` 三支断言；证据与复验 SQL 落 `effect-check.md`。
- 未闭环：真实 LLM 复验要等下个日报日结算（tasks 5.5），合格前不归档。
