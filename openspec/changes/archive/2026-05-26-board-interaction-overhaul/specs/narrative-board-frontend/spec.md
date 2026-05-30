## REMOVED Requirements

### Requirement: Single board-only navigation mode
**Reason**: 叙事功能完全迁移到 /tags 页面的 BoardNarrativeTimeline 组件，/topics 不再展示叙事面板
**Migration**: /topics 页面删除叙事 tab 和 NarrativePanel.vue、NarrativeBoardCanvas.vue 组件文件

### Requirement: Unified scope switching
**Reason**: 取消 global/category scope 区分，叙事时间线按 board 维度展示，无需 scope 切换
**Migration**: BoardNarrativeTimeline 组件通过 board id 查询叙事，无需 scope 参数。NarrativePanel.vue 中的 scope 切换逻辑随文件一起删除

### Requirement: Three-level navigation consistency
**Reason**: 叙事导航模型从 scope→board→narrative 简化为 board→narrative，由 /tags 页面的板块列表提供 board 选择
**Migration**: /tags 页面左侧板块列表即 board 选择器，右侧叙事时间线展示该 board 的叙事
