## Context

前一轮 `board-direction-check-and-board-editing` 实现了 `max_sim` 方向校验、`direction_mismatch` 字段、board embedding backfill 等基础设施。验证过程中发现方向校验不应只覆盖 `max_sim`，`hit_rate` 和 `weighted` 同样存在辅助标签语义相近但板块方向错误的问题。

同时，板块下文章展示和日报生成缺乏质量分层——所有匹配标签同等对待，导致日报聚类过多（35 个叙事）、文章展示无优先级。

## Goals / Non-Goals

**Goals:**
- 将方向校验从 `max_sim` 扩展到 `hit_rate` + `weighted`
- 文章展示按匹配质量分层排序
- 日报输入端质量筛选 + 聚类数控制（条件分支限制）
- 前端日报改为多页报纸布局
- 去掉板块动态（GenerateDynamics）

**Non-Goals:**
- 不修改匹配评分算法本身（方向校验仍是布尔门控）
- 不修改聚类算法，仅调 prompt 限制组数
- 不做 Canvas 渲染（用 CSS Grid + DOM 实现）
- 不做自适应网格布局（用固定结构 + 弹性行）
- 不添加 `?sort=time` 兼容参数（YAGNI）

## Decisions

### D1: 方向校验统一覆盖

**选择**: 对 `hit_rate`、`max_sim`、`weighted` 统一执行方向校验

**实现**: 将方向校验代码从 `case max_sim` 移到 switch 之后、`if matchReason != ""` 分支内统一执行。`direct_hit` 仍跳过（精确重叠已足够可靠）。

### D2: 文章排序规则

**选择**: 排序 key 为 `(tier, score, publish_time)`，其中 tier 数值越小越优先：

| Tier | match_reason | 条件 |
|------|-------------|------|
| 0 | direct_hit | — |
| 1 | hit_rate | — |
| 2 | max_sim | !downgraded |
| 3 | max_sim | downgraded |
| 3 | weighted | — |

- 同 tier 内按 `score` 降序
- 同 score 内按 `publish_time` 倒序
- 文章有多个 tag 时取最高 tier

**实现位置**: `getBoardArticles` handler 的 Go 端内存排序。`filtered_tags` 已在内存中携带 `match_reason`/`score`/`downgraded`，在 Go 端聚合 tier 后排序比 SQL 窗口函数更清晰、更易测试。

**不做**: ~~不添加 `?sort=time` 兼容参数（YAGNI）~~。后调整为支持 `sort=time` 参数，前端增加排序切换按钮。

### D3: 日报质量筛选

**选择**: 方案 A — 硬阈值 + 保底

筛选规则：
1. 过滤 `direction_mismatch = true`
2. 保留 `match_reason ∈ {direct_hit, hit_rate, max_sim}`（含 downgraded）
3. 过滤 `weighted`（最弱规则）
4. 如果剩余 < 10，把 weighted 也拉回来
5. 如果剩余 > 30，按 tier + score 排序后截断到 top-30

需要 `collectBoardTags` 携带 `match_reason` 和 `score` 字段。

**Fallback 标签同等对待**：`collectBoardTags` 的 fallback 路径（未匹配标签重新跑 `MatchTopicTag`）产生的标签也携带 `match_reason`/`score`，筛选逻辑一致——同样过滤 weighted、同样适用保底/截断规则。

### D4: 聚类数限制

**选择**: 在 ClusterTags prompt 中按标签数条件分支限制组数

当前 prompt 只规定"每组 2-8 个标签"，不限制组数。修改为：
- 标签数 ≤ 15：不限制组数（自然分组即可）
- 标签数 16-25：分成 6-12 组
- 标签数 > 25：分成 8-15 组

在 `clusterSystemPrompt` 构建时根据 `len(tags)` 动态插入对应约束。

### D5: 去掉板块动态

**选择**: 移除 `GenerateDynamics`（LLM Call B）

**原因**: dynamics 是把所有事件揉成一段文本，信息密度低且质量不稳定。改为各聚类区域的叙事线自然替代"板块动态"功能。

**实现**: 数据库 `BoardDailyReport.Dynamics` 字段保留（兼容历史数据），新报告生成时跳过 Call B、Dynamics 留空。prompt version 升级为 "2.0"。

### D6: 长滚动报纸布局 + 质量分区

**选择**: 单页长滚动（不分页），按质量分区，纸张放大适配大屏

纸张尺寸：`min(1100px, 92vw)` × `92vh`

布局结构（单页长滚动）：
1. 报头（日期）+ highlights（今日重点）
2. **质量分区**——按 `best_tier` 将聚类分组为区域：
   - 核心事件（Tier 0-1）：双列网格
   - 相关事件（Tier 2）：单列
   - 其他动态（Tier 3+）：单列
3. 每个聚类卡片完整展示所有叙事线索（title + summary + status），不截断
4. 线索可点击→弹出相关文章浮窗

**聚类排序指标由后端提供**：`DailyReportSection` 新增 `BestTier int` 和 `AvgScore float64` 字段。生成报告时，后端根据 section 中各 tag 的 `match_reason`/`downgraded` 计算 best_tier，根据 `score` 计算 avg_score，一并持久化到数据库。前端直接使用这两个字段排序和分区。

**线索文章浮窗**：点击线索→`@floating-ui/vue` 弹出浮窗展示 `related_article_ids` 对应的文章标题列表，分批加载（首批 5 篇并发，点"加载更多"再加载 5 篇）。点选文章→emit `openArticle`→TagsPage 复用已有 article preview modal。

**实现**: CSS Grid 布局，报纸纸纹背景，长滚动容器。

## Risks

- **聚类数限制可能影响信息完整性**: 15 组上限可能在极端情况下（>80 标签）导致合并过于粗粒度，但结合质量筛选后预期输入在 15-30 标签范围，8-15 组足够
- **排序逻辑改变用户体验**: 文章不再是纯时间线，需要观察用户是否适应
- **去掉 dynamics 影响历史报告兼容**: 前端需处理 Dynamics 为空的情况（已预留）
- **Fallback 标签筛选一致性**: fallback 路径产生的标签质量可能偏低，保底机制会兜住

## P6 补充决策

### D7: 文章弹窗 z-index 修复

**问题**: `.tags-article-modal` z-index 为 80，低于日报 `.np-overlay` 的 z-index 200。从日报中点击线索文章打开预览弹窗时，弹窗被日报遮挡。

**方案**: 将 `.tags-article-modal` z-index 从 80 提升到 210，确保始终在日报弹窗之上。

### D8: 文章排序切换

**问题**: 原设计（D2）只支持按质量排序，但用户有时也需要按时间浏览文章。

**方案**: `getBoardArticles` 新增 `sort` 查询参数：
- `quality`（默认）: 原有 tier + score + pub_date 排序
- `time`: DB 排序 `pub_date DESC`，跳过内存质量排序

前端文章列表 header 新增「质量/时间」切换按钮组，点击切换后重新加载文章。

**不做**: 不做 z-index 分层规范（仅局部修复），不做记住排序偏好。

### D9: 匹配参数配置补全

**问题**: 方向校准阈值 `DirectionSimThreshold` 和 direct_hit 最小重叠数 `DirectHitMinOverlap` 在后端已支持 DB 配置（`ai_settings` 表），但前端匹配参数 UI 缺少这两个字段，导致用户无法在 Web UI 中调整。

**方案**: 前端 `MatchingConfig` TS 类型和 `MatchingConfigDialog.vue` 表单补全这两个配置项。后端无需改动（已有完整的 CRUD 支持）。

### D10: 匹配参数 UI 按规则分组 + LaTeX 公式

**问题**: 参数配置 UI 原来是扁平列表，用户难以搞清楚每个参数属于哪个匹配规则、改了会影响什么。

**方案**: 将参数按匹配规则链分组（基础→①direct_hit→②hit_rate→③max_sim→④weighted→后置→升级），每组展示对应的 LaTeX 公式。参数 label 使用数学符号（θ<sub>sim</sub>、α、w<sub>sim</sub>等）与公式变量对应。

复用已有 `KaTeXRender` 组件渲染公式。公式直接从 `semantic_board_matching.go` 的实际代码逻辑推导。
