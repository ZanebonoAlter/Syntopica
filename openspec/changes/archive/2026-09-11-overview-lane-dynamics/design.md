# Design: overview-lane-dynamics

## Context

见 proposal.md「Why」与 specs。数据底座事实已 pin 在本 change 的 explore-findings.md（泳道锚定模型、lifeline week 停用真相、无批量端点缺口）。本设计定「结算挂点 / 存储 / 端点 / 前端结构」四个核心决策。

## Goals / Non-Goals

**Goals**
- 单请求渲染的 board 级聚合端点（泳道卡 + 时间线 + 候选栏一次查齐）
- 滚动 14 天态势随每日日报自动结算，失败不伤日报主流程
- 发展时间线数据结构显式携带「日期 → 事件」对应关系，前端不做推断

**Non-Goals**
- 不恢复 week 档定时（维持 fix-board-analysis-material 7.3 决策）；不动 topic_lifeline_context 表与 situation_cards 取材链
- 不动话题生命周期操作（转正/归档走既有入口）
- 不做态势句的手动重生成入口（首个版本靠每日结算自愈；用户手动需求出现再加）
- 沉寂泳道不做折叠区（直接不展示；出现需求再加）

## Decisions

**D1 结算挂点：日报管线尾部 detached goroutine（非独立 scheduler job）**
`GenerateAndSaveReport`（该板块当日报告落库成功）后起 goroutine 逐个结算该板块活跃泳道。理由：①「按每日日报结算」语义天然内聚——手动重跑（覆盖重建）后同样触发，快照随覆盖幂等自愈；②独立 job 需要管理「等日报完成」的时序依赖，复杂度不值。松耦合纪律：goroutine 内 recover 全吞、只记日志；analysis_paused 总闸在日报 job 层已生效（daily_report 属分析类清单），结算跟随日报无需单独判闸；AI 不可用导致结算失败自然由下个日报日重试。
- 并发限幅：单板块结算串行逐泳道；活跃泳道数上限 clamp 20（按 last_seen_date 降序截断，超出记日志）——对齐 situation 卡每板 12 卡的量级先例，防泳道膨胀板块拖慢。

**D2 存储：新表 `topic_lane_snapshots`（每泳道一行滚动覆盖）**
```go
type TopicLaneSnapshot struct {
    ID                uint
    PersistentTopicID uint   // UNIQUE index, FK ON DELETE CASCADE
    RollingSummary    string // type:text ≤100字态势句
    AsOfDate          time.Time // type:date 汇总截止（=最近一份已完成报告期）
    UpdatedAt         time.Time
}
```
不进 topic_lifeline_context：那张表是周期归档模型（topic, granularity, period 唯一），滚动 14 天窗口与自然周/月归档语义不同，混入会污染停用决策的边界。快照是纯派生缓存（可由日报数据重算），无迁移回填需求。

**D3 态势结算 LLM 调用：airouter 独立 operation，串行小 prompt**
operation=`daily_report.lane_snapshot`（写 AICallLog，遵循 ai-logging 规范）；输入 = 该泳道近 14 天「日期 + section 标题 + 前 3 条 thread 标题」逐行拼接（窗口锚定 `MAX(period_date)` 往回 14 个报告日，不用 now()，避免日报未跑时窗口漂移）；输出 ≤100 字中文态势句（prompt 约束：只基于所列事实、不预测，同 lifeline summarizeArchive 纪律）。

**D4 聚合端点：`GET /semantic-boards/:id/lane-dynamics?days=14`，topicgraph handler**
挂 `internal/topicgraph/handler/`（与 topics / section-timeline 同居）。响应形状：
```jsonc
{
  "window_days": 14,
  "lanes": [{
    "topic_id", "label", "watch_linked": true,      // watch 关联 = 该板块 active 的 sentence_topic watch 的 persistent_topic_id 命中
    "section_count_14d",                              // 排序键
    "snapshot": { "summary": "...", "as_of": "2026-09-09" } | null,  // null → 前端「待结算」
    "timeline": [ { "date": "2026-09-08",
                    "sections": [ { "section_id", "label", "events": ["thread标题", ...] } ] } ]
  }],
  "candidates": [{ "topic_id", "label", "last_seen_date", "recent_hint" }]  // recent_hint=最新 section 标题
}
```
时间线窗口与 `MAX(period_date)` 锚定同 D3；watch_linked 由 `BoardTopicWatch.PersistentTopicID`（FK 已存在）反查；候选 = topics 列表同口径（FilterVisibleTopics 可见门槛）+ 最新 section 标题。单实现 SQL：sections JOIN reports JOIN threads（每 section 取前 5 条 thread 标题，超限折叠计数），泳道集合 = active ∪ watch-linked，按 section_count_14d DESC。

**D5 前端：LaneDynamicsPanel 容器 + 卡片组件，删除 topic-landscape/**
- `BoardCompositionPanel.vue` 底部挂载点换为 `LaneDynamicsPanel`（构成管理区不动）
- 卡片：泳道名 + watch 角标 + 态势句（as_of 标注 / 待结算占位）+ 发展时间线（垂直时间线，日期节点 → 当日事件列表；单日超限折叠 + 「还有 N 条」）
- 空态生成日报 + WS 进度模式从 TopicLandscapePanel 迁移（同端点同行为）；「点卡片 → 话题总览 focus」沿用 TagsPage 现有 handleLandscapeSelectTopic 模式
- `topic-landscape/` 目录、topic-landscape API client 与后端路由一并删除

## Risks / Trade-offs

- [首日上线全量「待结算」，用户观感差] → 端点降级语义已定（时间线照常渲染）；可接受首个日报日自愈，不做回填脚本（快照纯派生，日报重跑也能触发）
- [日报 job 尾部 goroutine 生命周期失控] → 结算 goroutine 带 context timeout（单泳道 60s 上限）+ recover；服务器重启丢结算由下个日报日自愈
- [泳道多的大板块每天 N 次 LLM] → D1 clamp 20 + 串行 + 小 prompt（每泳道输入约一两百 token），量级 = 每板块每天 ≤20 次小调用，远低于当年 week 舰队风险
- [时间线 14 天 thread 标题过多撑爆卡片] → 每 section 事件标题截断前 5 + 折叠计数（D4），卡片整体超高可滚动（ui-design 定）

## Migration Plan

1. AutoMigrate 新表 `topic_lane_snapshots`（无破坏性）
2. 后端端点 + 结算上线；前端切换视图并删 topic-landscape（同版本）
3. 回滚：revert 提交；新表残留无害（无消费方）
4. **部署影响（须告知用户）**：话题态势版图（活力条/气泡图/卡片墙）消失，由泳道动态卡片区取代；topic-landscape API 移除；首批态势句在部署后第一个日报日（默认 21:00）生成，此前卡片显示「待结算」+ 时间线正常

## Open Questions

无（沉寂不展示、候选只读、watch 角标而非置顶——均已在 specs 定稿；卡片布局细节归 ui-design）。
