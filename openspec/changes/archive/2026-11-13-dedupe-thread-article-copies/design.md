# Design: dedupe-thread-article-copies

## Context

见 proposal.md - Why。此处只补实现层约束：

- 现象落点：`backend-go/internal/topicgraph/service/daily_report_merge.go:17` 的 `populateThreadArticles` 只按 `article_id` 主键去重；`backend-go/internal/topicgraph/repository/daily_report_repository.go:423` 的 `GetReportByID` Preload 后原样透传。
- 「同一篇内容」在库里有两条来源：跨 feed 同 link 副本（存储层按设计不归并，见 `rss-article-dedup`）与多源转载（URL 不同）。
- 已有内容身份键：`backend-go/internal/platform/textutil/normalize.go:69` `NormalizeArticleLink`（`fix-duplicate-article-heat` 引入；现存服务面是簇内文章数与 related-events，story 线已退役）。
- 四个渲染面（关心的话题 / 泳道编年史 / 侦探墙 / 生命周期面板）都消费同一个 `GET /api/daily-reports/:id` 响应。

## Goals / Non-Goals

**Goals**
- 同一条线索的关联文章列表不再出现「同一篇内容的两份副本」。
- 折叠口径零误伤：只认归一 link 相同这一条可判定信号。
- 一处改动覆盖全部渲染面；存储口径与生成期口径不动。

**Non-Goals**
- 不改生成期口径（`populateThreadArticles`）、不改红线 45 的存储语义。
- 不做跨 feed 副本的存储层归并（改 `rss-article-dedup` spec，影响 feed 视图语义与阅读行为归属）。
- 不做「同标题不同 URL」的转载折叠（误伤证据见 D2）。
- 不改任何前端代码。

## Decisions

### D1. 折叠落在日报详情读路径，不落在生成期
- **结论**: 在 `GetReportByID` 返回前折叠每条 thread 的 `related_article_ids`；四个渲染面都消费该端点，一处折叠即全覆盖，前端零改动。
- **落点**: backend-go/internal/topicgraph/repository/daily_report_repository.go:GetReportByID
- **取舍**: 备选「生成期折叠（`populateThreadArticles`）」会改写红线 45（`related_article_ids` 不被任何身份/去重逻辑改写），且需存量回填 + 重跑日报才见效；备选「前端折叠」要改 4 个组件，且「N 篇」计数必须先拉齐全部文章标题才能算（额外 N+1 请求）。故选读路径：既不改存储事实，又一次修好计数。

### D2. 内容身份口径 = 仅「归一 link 相同」（保守档）
- **结论**: 复用 `textutil.NormalizeArticleLink` 的归一 link 作为唯一判重键；归一后为空（link 为空）不参与折叠。
- **落点**: backend-go/internal/platform/textutil/normalize.go:NormalizeArticleLink
- **取舍**: 备选「标题归一 + 发布时间接近 + 正文相似」可再覆盖全库 106 组转载，但实测会误伤：「提醒：日内请重点关注（以下均为北京时间）」在同 feed 同日有 06:30 版与 12:30 版两个版本，标题相同、内容 trigram 相似度 0.74~0.95，实为两篇不同内容。用户已选保守档；放宽口径需另开 change 并先解决该误伤。

### D3. 折叠算法 = 保序 + 首个出现为准
- **结论**: 沿用日报数组契约（红线 21 细节层「数组契约」：保序、去重以首个出现为准）；被折叠的副本直接丢弃，不改指 keeper。
- **落点**: backend-go/internal/topicgraph/repository/article_identity.go:FoldThreadArticleCopies
- **取舍**: 备选「把副本 id 改指 keeper」数组长度不变、前端仍显示两条，无收益；备选「保留 keeper 在前、其余在后重排」破坏原顺序语义（前端按数组顺序渲染前 10 条），否决。

### D4. 折叠所需 link 一次批量取，查询失败降级为不折叠
- **结论**: `GetReportByID` 内一次 `SELECT id, link FROM articles WHERE id IN ?` 拉齐本报告全部被引用文章的 link，在内存折叠；查询失败 → Warnf + 原样返回（不折叠、不阻断详情接口）。
- **落点**: backend-go/internal/topicgraph/repository/daily_report_repository.go:GetReportByID
- **取舍**: 备选「逐 id 查」= N+1 查询；备选「SQL 侧窗口函数折叠」不可行（归一化是 Go 侧纯函数，jsonb 数组元素在 DB 侧无法应用该规则）。

### D5. 折叠 helper 是纯函数、落在 repository 包
- **结论**: `FoldThreadArticleCopies(threads []DailyReportThread, linkByArticleID map[uint]string) []DailyReportThread` 为纯函数（无 IO），放 repository 包，便于无 DB 单测（单元层最便宜）。
- **落点**: backend-go/internal/topicgraph/repository/article_identity.go
- **取舍**: 备选放 `platform/textutil` —— 该包是叶子包（不依赖任何 syntopica-internal 包），引入 `topicgraph/repository` 会造成分层倒置，否决。

### D6. 下游消费方保持读库表，不随读路径折叠收窄
- **结论**: 关键事件注记证据（`key_event_markers.go`）、页边注引用白名单（`margin_notes_repository.go`）、board 文章 `in_report` 判定（`tagmanagement/service/board/archive.go`）继续直读库表，保持「超集」语义——用户引用被折叠的副本 id 仍算合法。
- **落点**: 无代码落点：既有三处直读实现已满足，因为本 change 不改写落库值，读路径折叠不影响它们
- **取舍**: 无备选：若把折叠下沉到生成期，这三处会被连带收窄，属超出本 change 范围的行为变更。

## Risks / Trade-offs

- [Risk] 同标题不同 URL 的转载仍会重复外显（全库 106 组未被覆盖）→ 缓解：保守档是用户选定的边界；后续若要放宽，先解决 D2 的误伤样本再另开 change。
- [Risk] 「N 篇」计数下降，用户可能以为少文章 → 缓解：折叠的是同一篇内容的重复副本，语义上仍是「这条线索关联几篇文章」；不做文案改动（ui-impact none）。
- [Risk] 读路径多一次 `articles` 查询 → 缓解：单次批量 IN 查询（单报告引用量级为百级），命中主键索引；失败降级不阻断。
- [Trade-off] 落库值与响应值不再一致（响应是落库值的投影）→ 缓解：spec 与 flow 文档均显式写明「折叠只作用于响应」，避免下游误以为库里已被去重。

## Migration Plan

- 无 schema 变更、无数据迁移、无存量回填。
- 发布 = 部署新二进制即生效（读路径行为变化）。
- 回滚 = 撤销 `GetReportByID` 的折叠调用，恢复原样返回；无数据残留。

## Open Questions

（无）
