## Context

日报系统的线索（thread）是当前叙事展示的核心维度，每个聚类 section 下有 1-3 条 thread。实际使用中 thread 数量过多（每天 10-30 条），导致独立的总览 Gantt 图不可读，报纸 Modal 内线索平铺也分散注意力。

当前数据模型中，`DailyReportSection` 没有 `status` 和 `prev_section_id`，生命周期信息只存在于 thread 层面的 `prev_thread_id`。用户实际关注的是"话题"（即 section/聚类）如何跨天演进，thread 只是话题内的叙事细节。

## Goals / Non-Goals

**Goals:**
- 将 section（聚类）提升为生命周期展示的核心维度
- 总览 Gantt 图从 thread 粒度改为 section 粒度，节点数量降至可读范围
- 报纸 Modal 内线索折叠，减少默认信息密度
- 生命周期面板改为 section 维度，定位在 Modal 右侧外侧
- Section 状态由后端通过数据关系推导（cluster_tag_ids Jaccard 相似度），不依赖 LLM

**Non-Goals:**
- 不修改 thread 的生成逻辑和 prompt
- 不修改 thread 的 prev_thread_id 匹配机制
- 不引入 splitting/merging 等复杂状态，只保留 emerging/continuing/ending 三个状态
- 不改变日报生成的触发和进度广播机制

## Decisions

### D1: Section 匹配算法 — cluster_tag_ids Jaccard 相似度

**选择**：比较两天 section 的 `cluster_tag_ids` 的 Jaccard 相似度。

**阈值**：交集 ≥ 2 **或** Jaccard ≥ 0.3（满足其一即可建立匹配）。

**匹配策略**：每个当天 section 取前一天相似度最高的 section 作为 `prev_section_id`，且必须超过阈值。

**备选方案**：
- 复用 thread tag 匹配再聚合：间接推导，准确性依赖中间层
- 文章标签重叠：需要额外加载文章数据，计算量大
- LLM 语义判断：增加生成成本，且用户明确不希望 LLM 参与状态判断

**理由**：`cluster_tag_ids` 是 LLM 聚类时选出的核心标签，直接定义了 section 的话题语义，计算简单且准确。

### D2: Section 状态推导规则

| 条件 | 状态 | 颜色 |
|------|------|------|
| `prev_section_id = NULL` | emerging（新出现） | 绿 |
| 有 `prev_section_id` | continuing（持续） | 蓝 |
| 有 `prev_section_id`，但无后续 section 指向它，且非最新一天 | ending（结束） | 灰 |

"ending" 在查询时推导（非存储字段）：总览 API 返回时，如果某 section 的 ID 不出现在任何其他 section 的 `prev_section_id` 中，且它不是时间范围内的最新一天 → 标记为 ending。数据库中只存储 `prev_section_id`，`status` 在生成时根据 prev 是否为空设为 emerging/continuing，ending 由前端或 API 运行时判断。

### D3: Section Lifecycle Panel 定位

面板以 `position: fixed` 定位在 viewport 右侧（`right: 0`），不移动 Modal。面板宽度 320px，Modal 保持不动。两者可独立滚动。

**备选方案**：
- Modal 缩窄腾出空间：影响内容阅读体验
- 面板叠加在 Modal 上：遮挡内容

### D4: BoardThreadBrowser 改造策略

改造现有组件（而非新建），数据源从 `getBoardThreadTimeline` 改为新的 `getBoardSectionTimeline`。行代表 section 生命周期（通过 `prev_section_id` 串联），列代表日期。每行显示 `cluster_label`。

### D5: 线索在 cluster card 中的展示

默认折叠，只显示 section 级状态徽章和「N 条线索 ▸」。点击展开后显示线索标题 + 摘要 + 文章图标。线索不再显示独立状态徽章。

## Risks / Trade-offs

**[匹配阈值可能需要调整]** → 首版使用交集 ≥ 2 或 Jaccard ≥ 0.3，上线后根据实际数据观察调整。阈值配置在代码常量中，易于修改。

**[旧 thread 数据的 prev_section_id 回填]** → 现有历史 section 没有 `prev_section_id`。方案：生成逻辑只对新日报生效，历史数据不做回填。总览 Gantt 图从新数据开始显示生命周期，旧 section 显示为孤立节点。

**[ending 状态的判断延迟]** → ending 依赖"后续天无 section 指向"，最新一天的 section 无法判断是否结束。接受这个限制：最新一天的 section 只显示 emerging 或 continuing。

**[SaveReport upsert 悬空引用]** → 当重新生成某天的日报时，`SaveReport` 会删除旧 sections。后一天 section 的 `prev_section_id` 会指向被删除的 section，产生悬空引用。方案：在 `SaveReport` 的 upsert 分支中增加清理逻辑，将所有指向即将删除的 section 的 `prev_section_id` 置为 NULL（与现有 `prev_thread_id` 清理逻辑对称）。

**[多对一匹配（话题分裂）]** → 首版允许多个当天 section 匹配到同一个前一天 section（即话题可以"分裂"），这是预期行为。匹配算法不做 1:1 贪心分配，每个当天 section 独立取最高相似度。

**[ending 推导的一致性]** → 所有返回 section status 的 API 统一做 ending 推导：section timeline API、section lifecycle API、日报详情 API（sections 内嵌时）。确保 Modal 内和 Gantt 图内颜色一致。

## Migration Plan

1. 数据库 migration：`daily_report_sections` 加 `status VARCHAR(20) DEFAULT 'emerging'` 和 `prev_section_id UINT NULL`
2. 后端 generator 修改：section 保存时增加匹配和状态推导
3. 新增 2 个 API endpoint
4. 前端改造：无 breaking change，旧 API 保留兼容
5. 无需回滚策略（新增字段和 API，不破坏现有功能）
