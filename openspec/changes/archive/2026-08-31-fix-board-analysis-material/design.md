# Design: fix-board-analysis-material

## Context

board-level-deep-analysis 端到端验证发现素材断供：态势卡取材链只读 week 档（生产库 67 泳道 month/year 在库、week 仅 2），降级产物为「泳道名 (N篇)」零信息指纹；`get_lane_detail` 只渲染 section 时间线，到不了 `topic_lifeline_context`。数据侧事实（生产库实测，2026-08-27）：

- `topic_lifeline_context`：month 67 泳道（800~2500 字符/期）、year 67、week 2（`lifeline_weekly` 从未执行）。
- `daily_report_threads` 有 `title` + `summary` 实质内容；`dbLifelineReader.GetTopicLifeline` 已把 `ThreadTitles` 装进 `TimelineSectionNode`——指纹提质零新增查询。
- D9 新鲜度门已会补齐 month 档（8/27 实测 5 次调用补到 08-26），缺的只是消费方。

本 change 是读取路径修复：无 schema 变更、无迁移、不动编排结构。

## Goals / Non-Goals

**Goals:**

- 态势卡在 week 缺失时以 month 摘要为主要事实源（生产形态下 97% 泳道走此路径）
- `get_lane_detail` 附带 month/year 背景记忆（预算内），agent 下钻可读历史记忆
- section 指纹降级携带 thread 标题实质内容
- 前端工作台信息架构收口（单一下拉、去单 tab 栏）
- 测试 fixture 以生产形态为准（month 在、week 缺）——修上一轮「自造 week 数据测分支」的盲区

**Non-Goals:**

- 不做 week 档首份全量补建（M4.7 护栏维持；week 覆盖交还 `lifeline_weekly` 定时任务）
- 不动三角色编排、参考角色库、evidence schema、D9 新鲜度门逻辑
- 不改模型路由（用户已手动配置）
- 不做 LLM 产出质量评测体系（素材修复后先观察）

## Decisions

### D1 态势卡取材链插 month 兜底层

`laneFactsDigest`（situation_cards.go）取材链从 `week → section指纹 → description → none` 扩为：

```
week（最新2期拼接，现状不变）
  → month（最新1-2期拼接压缩）      ← 新增，facts_source=lifeline_month
  → section指纹（D2 提质后）          facts_source=section_fingerprint
  → description / none（现状不变）
```

- month 读取复用 `ListTopicLifelineContextsByGranularity(topicID, "month")`（已有 repo 方法），取最新 `situationCardLifelineMonths=2` 期，`compressSpaces` 后按既有 `situationCardDigestRunes`（full 120）/`situationCardBriefRunes`（48）截断——卡片预算常量不动（态势卡契约是「态势感知+下钻取详情」，详情由 D3 补）。
- 质量信号密度分（`situationDensity`）从「仅近期文章数」扩为计入 month 可用性：month 在库 +2、week 在库 +2（有背景记忆的泳道加权），公式在测试断言中固化。
- `FactsSource` 枚举扩 `lifeline_month`，透出在卡片 JSON（可追溯性），前端不消费（无 UI 改动）。

### D2 section 指纹改用 thread 标题

指纹从 `[日期] cluster_label (N篇)` 改为 `[日期] thread标题1 | thread标题2 (N篇)`：`recentLaneSections` 返回的 `TimelineSectionNode.ThreadTitles` 已装填（production_wiring 逐 section 查询 thread titles），单 section 取前 3 条标题拼接，整体仍受 `situationCardDigestRunes` 截断。无 thread 时退回 cluster_label（不比现在差）。零新增 SQL。

### D3 `get_lane_detail` 附带历史背景记忆

- `LifelineReader` 接口扩一个方法：`GetTopicLifelineArchive(topicID uint) ([]LifelineArchiveRow, error)`，`LifelineArchiveRow{Granularity, Period, AsOfDate, Content}`；生产实现查 `topic_lifeline_context` 的 month（最新 2 期）+ year（最新 1 期）。
- `RenderLifelineForAgent`（lifeline_renderer.go）在「逐日演进」后追加 `## 历史背景记忆（月/年档案）` 段：每行 `### [month 2026-08] (as_of 2026-08-26)` + 压缩正文；**总预算 `archiveRenderRunes=4000`**（约等于 1.5 期 month 全文），超预算按 month 新→旧、year 兜底顺序截断，末尾标注 `[档案截断]`。无归档行时输出 `（无背景记忆归档）`——不静默省略段落（spec 场景要求）。
- 挂点在 renderer 而非 tool_registry：单泳道 QA/其他消费 `RenderLifelineForAgent` 的链路同样受益，且 tool 层零改动。
- mock 适配：既有测试的 fake LifelineReader 补空实现（返回空切片）即维持原行为。

### D4 前端工作台收口（BoardEnrichmentPanel.vue）

- 删除顶部旧「数据增强·认知工作台」toolbar（含旧泳道下拉、刷新按钮；刷新入口移到版块分析区头部）。
- 泳道选择唯一化：聚焦分析折叠区下拉成为唯一泳道选择控件（`selectedTopicId` 单一绑定点）。
- 「新闻背景」从单 tab 栏改为与聚焦区一致的折叠 section（默认收起，展开行为不变：周期筛选/翻历史/inline 编辑）。tab 栏组件移除。
- QAPanel 保持挂聚焦区（追问本体是单泳道 result 的能力，版块级 QA 不在本期）；聚焦区展开即达，不额外移动。

### D5 测试策略：生产形态 fixture

- service 层单测与 DB 集成测试的造数起点一律「month 在库、week 缺失、threads 带标题」——对应用例断言 `facts_source=lifeline_month`、指纹含 thread 标题。
- 保留 week 在库的用例作回归（week 优先级不变）。
- 效果核对（test-design 四问之④，真库量化）：对生产库快照跑 `AssembleSituationCardsForTest`，断言 facts_source 分布中 `lifeline_month` 占比（预期 ≥ 80% 泳道）；不达标记为素材缺口回流到 lifeline 补齐议题。

## Risks / Trade-offs

- [month 摘要截断到 120 rune 丢上下文] → 态势卡本就是态势感知层，深度细节由 D3 的 get_lane_detail 档案段（4000 rune）承接；后续观察 interpret 质量再调预算。
- [get_lane_detail 输出变长推高 agent loop token] → 4000 rune 预算上限 + 只在 month/year 有归档时追加；对照现状（agent 拿不到记忆只能多发 web_search）是净收益。
- [密度信号公式变化影响卡片排序] → 公式变化有测试固化；排序只是注入顺序与详略，非行为契约。
- [前端删顶栏触碰既有交互习惯] → 顶栏唯一功能（选泳道）在聚焦区完整保留；新闻背景入口形态变化在报告中说明。

### D6 补全门（追加 7.x：分析前新闻背景补全）

用户反馈升级：分析触发时应先把新闻背景补全，而非事后手动补。旧 M4.7「无记录≠落后」护栏废除（成本前提不成立：D9 只遍历本次分析的活跃泳道，非全库）。

- 检查集 month/year（week 退出：近期记忆归 14 天窗口详情，长期归 month/year；`lifeline_weekly` 停用防全库 heal 爆量，存量 week 行保留可被取材链消费）。
- 判定按「有料周期集」（从 section dates 推导）：无行→补建（含首份）、行 UpdatedAt 距今 >72h→重算覆盖（已结束周期得完整版，修复 7 月半月档；进行中得至今快照）。统一走 `RefreshPeriod`。
- 全局限额 40 次 LLM，溢出 `budget_exhausted` 降级留日志；失败降级不阻塞；串行。
- `refreshArchive` 写入 as_of = min(周期边界, now)——修未来日期脏数据（year as_of=2027、手动补生成 09-01）；旧存量脏行由写入后 clamp 兼容清理。

生产预估（中东板块 6 泳道）：首跑 ≈15 次重算（4 老泳道×〔6/7 月截断+year 旧档〕+ 1204 year 缺 + 1205 首建），次日再触发 0 次。

## Migration Plan

1. 纯读取路径变更，部署即生效；无迁移、无回滚风险。
2. 部署后第一次板块分析即可消费 month 档（无需任何手动操作）。
3. 旧 result（素材空泛的历史报告）不重生成——result 不可变红线；用户可手动重新触发获得新素材版本。

## Open Questions

- month 取 2 期 vs 1 期（当前 2 期压缩后各占 ~60 rune）——实现时看压缩效果微调，不阻塞。
- 档案段预算 4000 rune 是否需要按 agent loop 轮次动态收缩——后置观察项。
