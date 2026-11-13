<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A Reason

本次改动不新增也不改动任何界面结构、交互模式或文案：关心的话题（`DailyReportTopicSection`）、泳道编年史（`LaneChronicleThread` / `LaneRelatedArticles`）、侦探墙（`TopicDetectiveWall`）、生命周期面板（`SectionLifecyclePanel`）四个既有渲染面继续消费同一个 `GET /api/daily-reports/:id` 响应，只是其中的 `related_article_ids` 数组变短、「N 篇」计数变准。无新组件、无新入口、无布局模式变化、无新交互，故不制作原型。
