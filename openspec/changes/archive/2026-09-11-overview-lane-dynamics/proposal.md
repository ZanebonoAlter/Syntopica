<!-- complexity: complex -->
<!-- ui-impact: major -->
<!-- constraint-domains: daily-report, data-enrichment, semantic-board -->

## Why

版块详情「板块内容」tab（用户口中的「总览」）现在展示的是话题态势版图：活力条、节奏气泡图、姿态卡片墙——全是计数和统计图，看完不知道每条泳道最近**发生了什么**。用户要的是叙事级信息：每条关注/追踪的泳道最近 14 天有什么动静，一眼看懂。数据底子是现成的（每天日报生成的 section 锚定到泳道、thread 带标题叙事），缺的只是聚合视图和滚动结算。

## What Changes

- **删除话题态势版图**：`TopicLandscapePanel`（VitalityBar / TopicRhythmChart / StanceCardWall 三件套）及 `GET /semantic-boards/:id/topic-landscape` 端点整体退役（**BREAKING**：该 API 移除；空态「生成日报」入口与「点卡片→话题总览 focus」联动在新视图中保留）
- **新建「泳道动态」主视图**（板块内容 tab 下，构成标签管理区保留在上方）：
  - 每条泳道一张卡：泳道名 + **一句话态势**（LLM 结算，见下）+ **3~5 条要点**（近 14 天该泳道锚定 section 的 thread 标题，按天分组，机械聚合零 LLM 成本）+ 点卡片跳「话题总览」focus 视图
  - 展示范围：**关注（active）+ 我在追踪（watch 关联）** 的泳道，按近 14 天 section 数排序；watch 泳道置顶或加标识（细节在 ui-design 定）
  - 沉寂泳道（近 14 天无 section 的 active）默认不展示（保守默认，可后续加折叠区）
  - **候选栏**：底部小栏目列出最近达门槛可见的 candidate 泳道（名字 + 最近动向），只读提示「值得注意」，转正仍走原有人工入口
- **日报后滚动结算（方案乙）**：每天日报 pipeline 完成（SaveReport 之后）为该板块活跃泳道各刷一条「滚动 14 天一句话态势」——输入 = 该泳道近 14 天 sections 的 thread 标题列表（小 prompt），输出 ≤100 字中文态势句；存储为新滚动快照表（每泳道一行，`rolling_14d_summary` + `as_of_date`，随每日结算覆盖更新）；结算失败不阻塞日报主流程（松耦合，记日志）
  - 不恢复 week 档定时（维持 fix-board-analysis-material 7.3 停用决策）；结算成本 = 每板块每天「活跃泳道数」次小 LLM 调用，分散在日报 job 内，无全库突发
- **新增 board 级批量端点**：`GET /semantic-boards/:id/lane-dynamics?days=14` 一次返回该板块全部泳道卡数据（泳道元信息 + 滚动态势 + 14 天要点 + 候选清单），前端单请求渲染
- 前端消费 `front/app/api/` 新增 lane-dynamics client 与 `LaneDynamicsCard` 等组件

## Capabilities

**New Capabilities:**

- `board-lane-dynamics`：泳道动态视图（卡片 + 候选栏 + 批量端点）+ 日报后滚动 14 天态势结算（结算时机、输入输出、松耦合不阻塞日报）

**Modified Capabilities:**

- `board-topic-landscape`：整体移除（REMOVED）——话题态势版图视图与 topic-landscape 端点退役，其承载的「板块首屏概览」职责由 board-lane-dynamics 接管

## Impact

- 后端：
  - 新表 `topic_lane_snapshots`（滚动快照，无外键级联需求，topic 删除时随删）
  - 结算挂点：`backend-go/internal/topicgraph/service/daily_report_orchestrator.go`（GenerateDailyReport 管线尾部，异步 detached，参照 fix-board-analysis-material 的 202 异步模式）或 scheduler 松耦合 job——design.md 定
  - 新端点挂在 `internal/topicgraph/handler/`
- 前端：`BoardCompositionPanel.vue` 改造（态势版图三件套 → 泳道动态区）、`topic-landscape/` 目录删除、新增 `lane-dynamics/` 组件
- 依赖方核对：topic-landscape API 仅 TopicLandscapePanel 消费（已验证无其他调用方）；situation_cards 的 stance 派生逻辑属数据增强内部取材，不受本 change 影响
- **部署影响**：上线后旧「话题态势版图」消失，需用户知晓；topic-landscape 缓存（packageBoardCache 如有涉及）清理；首批快照为空直到第一个日报日，端点需对「无快照」降级（只显示要点、态势标「待结算」）
