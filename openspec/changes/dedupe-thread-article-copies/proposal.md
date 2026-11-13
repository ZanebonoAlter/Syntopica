<!-- complexity: complex -->
<!-- ui-impact: none -->
<!-- constraint-domains: daily-report, content-enrichment -->

## Why

日报「关心的话题」页面的同一条线索下会出现两篇一模一样的文章。实测样本：thread 17700「特朗普与内塔尼亚胡就伊朗军事行动爆发激烈争吵」挂着 `[174103, 174198]`——一篇华尔街见闻快讯、一篇凤凰网转载，标题完全相同，计数显示「2 篇」，用户点开看到两条重复条目。

根因不在生成期，而在**内容身份口径只落到了 story/heat 层，没下沉到线索→文章层**：

- 同一篇文章被两个 feed 各抓一份 → 存储层按设计**不跨 feed 归并**（`rss-article-dedup`：「跨 feed 同 link 各自保留」），但**打标结果跨 feed 复用**（`source='reuse'`）→ 两行文章共享同一批事件标签；
- 同一篇稿被多源转载（**URL 不同**）→ 各源独立 AI 打标**都命中同一个 event 标签**（样本：两篇都拿到 tag 274650「特朗普与内塔尼亚胡激烈争吵」score 0.7 source=llm）；
- 两者汇入同一线索时，`populateThreadArticles` 只按 `article_id` 主键去重（`seen map[uint]bool`）→ 主键不同就不折叠；读路径 `GetReportByID` 原样透传；前端「N 篇」= 数组长度 → 重复外显。

全库实测：**136 个重复组、影响 134 条线索、多出 145 篇文章**（30 组同 link 副本 / 106 组转载）。仓库里已有内容身份键 `textutil.NormalizeArticleLink`（`fix-duplicate-article-heat` 引入），但从未用于线索→文章层——这就是缺口。

## What Changes

- **日报详情读路径折叠同内容副本**：`GET /api/daily-reports/:id` 返回前，把每条 thread 的 `related_article_ids` 按「归一 link 相同即同一篇内容」折叠，保留首个出现、顺序不变；`link` 为空的文章不参与折叠（内容身份不可判定）。
- **归一规则复用现成实现**：`textutil.NormalizeArticleLink`（去 fragment、去 query、scheme/host 小写），不新造口径。
- **折叠只发生在读路径投影**：落库值 `daily_report_threads.related_article_ids` 仍是「该 thread `tag_ids` 对应文章的并集」，红线 45 的存储语义不变；生成期口径、存储层归并策略、跨 feed 副本存储策略全部不动。
- **四个渲染面同步受益**：关心的话题（`DailyReportTopicSection`）、泳道编年史（`LaneChronicleThread` / `LaneRelatedArticles`）、侦探墙（`TopicDetectiveWall`）、生命周期面板（`SectionLifecyclePanel`）——四者都消费同一个 `GET /api/daily-reports/:id` 响应，故一处折叠即全覆盖，前端零改动。
- **口径边界（保守档）**：只认「归一 link 相同」。同标题不同 URL 的转载**不折叠**（见 design D2 的误伤证据）；空 link 不折叠。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `daily-report-system`：修改「日报查询 API」Requirement —— 详情端点返回的 thread `related_article_ids` SHALL 是按归一 link 折叠同内容副本后的**读路径投影**，落库值 SHALL NOT 被改写。

## Impact

- **后端**：`backend-go/internal/topicgraph/repository/`（新增纯函数折叠 helper + `GetReportByID` 调用点）；`backend-go/internal/platform/textutil/normalize.go` 只复用、不改。
- **前端**：零改动（四个渲染面继续消费同一响应，只是数组变短、计数变准）。
- **API**：`GET /api/daily-reports/:id` 的 `related_article_ids` 数组长度可能变短（被折叠的副本），元素仍是合法 article id、顺序不变。
- **数据**：无 schema 变更、无迁移、无存量数据改写、无回填脚本。
- **下游消费方不受影响**：关键事件注记证据（`key_event_markers.go`）、页边注引用白名单（`margin_notes_repository.go`）、board 文章 `in_report` 判定（`tagmanagement/service/board/archive.go`）均**直读库表**，保持超集语义。
