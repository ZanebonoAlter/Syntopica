## 1. 后端：模型与数据库 Migration

- [x] 1.1 `DailyReportSection` 模型新增 `Status string` 和 `PrevSectionID *uint` 字段（models.go）
- [x] 1.2 创建数据库 migration：`daily_report_sections` 加 `status VARCHAR(20) DEFAULT 'emerging'` 和 `prev_section_id UINT NULL`

## 2. 后端：Section 匹配逻辑

- [x] 2.1 新增 `findPreviousSections(boardID, date) []DailyReportSection` 函数：查询前一天日报的所有 sections（含 ClusterTagIDs），返回给匹配逻辑使用。注意：现有 `findPreviousReport` 返回扁平 thread 列表并丢弃 section 信息，无法直接复用
- [x] 2.2 实现 `matchPreviousSections` 函数：接收当天 sections 和前一天 sections，通过 `cluster_tag_ids` 的 Jaccard 相似度匹配，阈值交集 ≥ 2 或 Jaccard ≥ 0.3，设置 `PrevSectionID` 和 `Status`。允许多对一匹配（话题分裂是预期行为）
- [x] 2.3 在 generator 流水线中集成：**所有 cluster goroutine 汇总后**（WaitGroup 之后）、保存之前调用 `matchPreviousSections`。需确保所有当天 sections 已组装完毕再执行匹配
- [x] 2.4 在 `SaveReport` 的 upsert 分支中增加 `prev_section_id` 悬空清理逻辑：删除旧 section 前，将所有 `prev_section_id` 指向旧 section 的记录置为 NULL（与现有 `prev_thread_id` 清理逻辑对称）

## 3. 后端：API Endpoint

- [x] 3.1 实现 `GET /api/semantic-boards/:id/section-timeline?days=14`：查询板块最近 N 天所有 section，返回扁平列表（含 ending 推导），按 period_date 倒序
- [x] 3.2 实现 `GET /api/daily-reports/sections/:id/lifecycle`：沿 `prev_section_id` 向前追溯至头，向后扩展所有以该 section 为 prev 的后续 section，返回完整链（按时间正序）
- [x] 3.3 日报详情 API（`GET /daily-reports/:id`）返回的 sections 也需做 ending 推导，确保前端 Modal 内颜色与 Gantt 图一致
- [x] 3.4 在 router.go 注册新路由

## 4. 前端：API 层

- [x] 4.1 新增 `SectionTimelineNode` 接口（id, report_id, period_date, cluster_label, status, article_count, thread_count, prev_section_id）
- [x] 4.2 新增 `SectionLifecycleNode` 接口（同上 + threads 详情可选）
- [x] 4.3 `DailyReportSection` 接口新增 `status` 和 `prev_section_id` 字段
- [x] 4.4 新增 `getBoardSectionTimeline(boardId, days)` 和 `getSectionLifecycle(sectionId)` API 方法

## 5. 前端：报纸 Modal 改造

- [x] 5.1 cluster card 添加 section 级状态徽章（emerging=绿/continuing=蓝），移除 thread 级状态徽章
- [x] 5.2 线索默认折叠：只显示「N 条线索 ▸」，点击展开显示线索标题+摘要+文章图标
- [x] 5.3 cluster card header 区域（名称+状态）点击 → 打开 SectionLifecyclePanel
- [x] 5.4 上下天导航（↑↓键、按钮）时，如果 Lifecycle Panel 打开且当前 section 有延续关系，自动高亮新一天的对应 section

## 6. 前端：SectionLifecyclePanel

- [x] 6.1 将 `ThreadLineagePanel.vue` 改造为 `SectionLifecyclePanel.vue`：数据源改为 `getSectionLifecycle`，展示 section 跨天链（日期+聚类名+状态+文章数+线索数）
- [x] 6.2 面板定位改为 `position: fixed`，right: 0，不移动 Modal，宽度 320px
- [x] 6.3 面板内点击节点 → 切换 Modal 到对应日期并滚动到 section
- [x] 6.4 面板关闭逻辑（✕ 按钮、切换 section、关闭 Modal）

## 7. 前端：话题总览（BoardThreadBrowser 改造）

- [x] 7.1 数据源从 `getBoardThreadTimeline` 改为 `getBoardSectionTimeline`
- [x] 7.2 行改为 section 生命周期（通过 prev_section_id 串联），显示 cluster_label
- [x] 7.3 节点颜色改为 section 状态色（emerging=绿/continuing=蓝/ending=灰）
- [x] 7.4 点击圆点 → 打开该 section 的日报 Modal + SectionLifecyclePanel
- [x] 7.5 切换按钮文案从「线程总览」改为「话题总览」

## 8. 清理

- [x] 8.1 确认 `getBoardThreadTimeline` 前端不再调用，后端旧路由保留（向后兼容）
- [x] 8.2 端到端验证：生成新日报 → 检查 section status/prev_section_id → 验证总览 Gantt → 验证 Lifecycle Panel → 验证上下天联动
