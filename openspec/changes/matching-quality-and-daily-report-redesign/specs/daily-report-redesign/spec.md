# daily-report-redesign

## Summary

日报生成管线增加质量筛选 + 聚类数限制 + 去掉 dynamics；后端新增聚类排序字段；前端改为多页报纸布局。

## Behavior

### 1. 日报质量筛选

`collectBoardTags` 查询携带 `match_reason` 和 `score`（包括 fallback 路径产生的标签），生成管线在聚类前增加筛选层：

1. 过滤 `direction_mismatch = true`
2. 保留 `match_reason ∈ {direct_hit, hit_rate, max_sim}`（含 downgraded）
3. 过滤 `weighted`（最弱规则）
4. 如果剩余 < 10 个标签，把 weighted 也拉回来（保底）
5. 如果剩余 > 30 个标签，按 `(tier, score)` 排序后截断到 top-30

**Fallback 标签同等对待**：fallback 路径（未匹配标签重新跑 `MatchTopicTag`）产生的标签也携带 `match_reason`/`score`，筛选规则完全一致。

### 2. 聚类数限制

`ClusterTags` prompt 按标签数量条件分支：

- 标签数 ≤ 15：不限制组数（自然分组即可）
- 标签数 16-25：分成 6-12 组
- 标签数 > 25：分成 8-15 组

在 `clusterSystemPrompt` 构建时根据 `len(tags)` 动态插入对应约束。

### 3. 去掉板块动态

- 移除 `GenerateDynamics`（LLM Call B）
- `BoardDailyReport.Dynamics` 字段保留（兼容历史），新报告留空
- prompt version 升级为 "2.0"
- 生成管线并发逻辑简化：只剩 Call A（Highlights）+ Call C×K（Threads）

### 4. 聚类排序字段

`DailyReportSection` 新增两个持久化字段：
- `BestTier int`：该 section 中所有 tag 的最高 tier（match_reason + downgraded 映射）
- `AvgScore float64`：该 section 中所有 tag 的 score 平均值

生成报告时后端计算并写入。前端用 `best_tier ASC, avg_score DESC` 排序聚类。

### 5. 长滚动报纸布局

纸张尺寸：`min(1100px, 92vw)` × `92vh`，单页长滚动（不分页）。

**布局结构**（从上到下）：
1. 报头：日期大标题
2. 今日重点：highlights 展示（title + reason）
3. **质量分区**：按 `best_tier` 将聚类分为区域
   - 核心事件（Tier 0-1）：双列 CSS Grid
   - 相关事件（Tier 2）：单列
   - 其他动态（Tier 3+）：单列
4. 每个分区显示区头标签（"核心事件"/"相关事件"/"其他动态"）+ 聚类数

**每个聚类卡片**:
- 聚类名称 + 文章数
- 叙事线索列表：**完整展示所有线索**（title + summary + status tag），不截断
- 线索可点击→弹出相关文章浮窗

**线索文章浮窗**:
- 点击线索→`@floating-ui/vue` 弹出浮窗（复用 `FeedActionMenu` 的 floating-ui 模式）
- 展示 `related_article_ids` 对应的文章标题列表
- 首批加载 5 篇（并发调 `getArticle`），点"加载更多"再加载 5 篇
- 点选文章→emit `openArticle(articleId)`→TagsPage 复用已有 `openArticlePreview` modal

**保留**：topbar 天级翻页（上一天/下一天），Escape 关闭

### 不变项

- `GenerateHighlights`（Call A）不变
- `GenerateClusterThreads`（Call C）不变
- 聚类连续性匹配（`matchPreviousThreads`）不变
- 历史报告展示兼容 Dynamics 为空

## Test Cases

- 质量筛选：weighted 标签默认不进入聚类
- 保底：筛选后 < 10 时 weighted 被拉回
- 截断：筛选后 > 30 时按质量截断
- Fallback 标签同样被筛选
- ClusterTags prompt 限制生效：≤15 不限，16-25 → 6-12 组，>25 → 8-15 组
- Dynamics 为空时前端不渲染"板块动态"区块
- 报纸布局：长滚动，质量分区（核心/相关/其他）
- 核心事件双列，其余单列
- 线索完整展示，不截断
- 线索点击弹出文章列表浮窗，分批加载
- DailyReportSection 正确计算 best_tier 和 avg_score
