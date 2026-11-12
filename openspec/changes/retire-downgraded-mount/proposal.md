<!-- complexity: moderate -->
<!-- ui-impact: minor -->
<!-- constraint-domains: semantic-board -->

# 退役 downgraded 降级标记（挂载行为不变）

## Why

`topic_tag_board_labels.downgraded` 是 `min(2, N)` 放宽准入的证据标记：tag 只有 1 个 active 辅助标签时，max_sim 挂载门槛被放宽到 1，挂载成功即置 true（`semantic_board_matching.go:486`）。2026-11-12 诊断（`docs/research/semantic-board-daily-report-diagnosis/explore-findings.md`）钉死三个事实：

- **无任何硬功能**：全文三个消费点全是弱用途——归档弱证据口径（`archive.go:148`）、MatchTier 排序档位（`board_match_handler.go:641`）、日报 quality_breakdown 快照记录。日报过滤根本不读它（读的是 `direction_mismatch` + `category='event'`）。
- **语义误导**：它标记的现象（单 aux 弱证据）真实存在，但「降级」二字在前端渲染成刺眼的「已降级 ↓」徽标，让用户误以为系统出了故障（本次诊断的起因正是用户看到版块「全是降级的 coding、AI Agent」）。
- **不可一刀切**：用户一度拍板「N=1 不挂载」，但真库实测推翻——N=1 不全是泛词（Coding/OpenAI），**还有大量正经实体标签**（Claude、英伟达、Agent 挂载 1.0 满分；Transformer 0.854、SGLang 0.808、AI 算力 0.802）：keyword 实体标签的设计是「自身 label+description 直接进辅助标签池」，天然单 aux，max_sim 放宽是它们挂版的**唯一途径**。一刀切会使 board 1980 近 5 天 **694 篇正常文章**从版块消失（含「百度文心发布 ERNIE Evolve」等正牌 AI 新闻）；且 score 分不出好坏（Coding 0.881 > Transformer 0.854），提阈值方案（≥0.92）同样误伤 746 篇。

用户最终拍板（2026-11-12）：**挂载行为完全不变，只退役 downgraded 标记本身**——「降级」降格为普通的板块标签命中方式（它本来就是 max_sim 命中的一种），文章零消失。

## What Changes

- **DDL**：`topic_tag_board_labels` 删除 `downgraded` 列。挂载行为不变，**不删任何挂载行**（与早先「删 downgraded=true 行」的一刀切方案不同，本方案零数据损失）。
- **后端匹配层**：删除 `MatchResult.Downgraded` 字段与 `semantic_board_matching.go` 的置位逻辑（:477/:486/:494/:845）；`min(2, N)` 放宽准入本身**保留**（行为不变）。
- **后端消费点**：
  - `archive.go`：`ApplyWeakEvidenceFilter` 的 strong 判定去掉 `NOT downgraded` 项；`ArchiveEvidenceRank` / `IsWeakEvidence` 签名简化（去掉 downgraded 参数）。
  - `board_match_handler.go`：两处 DTO 字段、三处 SQL Select/ORDER BY、`MatchTier` 的 max_sim 降级档位合并为统一 max_sim 档。
  - `topicgraph`：`daily_report_models.go` / `daily_report_orchestrator.go` / `daily_report_rebucket.go` 的 Downgraded 字段与 quality_breakdown JSON 键删除（快照兼容：历史 breakdown JSON 里的 downgraded 键读取端忽略即可）。
- **前端**：
  - `ArchiveMatchPopover.vue`：「已降级 ↓」徽标（:535）与列表「↓」标记（:735）删除；`matchReasonColor` 的 downgraded 参数移除。
  - `archiveMatchFlow.ts`：max_sim 判定文案的「⚠降级匹配」后缀删除。
  - `useBoardArchive.ts`：弱文章判定 `!tag.downgraded` 条项移除。
  - 相关测试断言同步。
- **文档**：`docs/reference/flow/semantic-board.md` 业务约束与不变量节对齐（min(2,N) 放宽准入保留、降级标记退役；红线 5 措辞不涉及行为变化仅去 downgraded 引用）；`docs/reference/api/semantic-boards.md` 示例 JSON 去 `downgraded` 键；`docs/reference/database/tables/semantic-labels.md` 与 `_conventions.md` 的列描述清理。
- **构建产物**：`backend-go/cmd/dump-sanitizer/tables.go` 的 `topic_tag_board_labels` 列清单去 `downgraded`（否则演示 seed 导出在新 schema 上报列不存在）；同步移除已被迁移 `restructure-settings-navigation` 删除的 `analysis_methods` / `reference_roles` 两条 stale 表条目；`demo/seed/seed.sql` 按新列清单重新生成。

## Capabilities

### Modified Capabilities

- `tag-to-board-matching`（`openspec/specs/tag-to-board-matching/spec.md`）：挂载判定的三规则与多 board 挂载 requirement——min(2,N) 放宽准入保留，downgraded 标记与字段描述删除。
- `match-detail-ondemand`（`openspec/specs/match-detail-ondemand/spec.md`）：匹配详情响应不再返回 `downgraded` 字段（`effective_min_hits` 保留）。
- `board-article-api`（`openspec/specs/board-article-api/spec.md`）：`filtered_tags` 匹配证据字段去 `downgraded`；弱证据过滤口径去「已降级」类别。
- `match-score-visualization`（`openspec/specs/match-score-visualization/spec.md`）：chip 淡化色与 "↓" 后缀删除；`matchReasonColor` 单参化；匹配详情面板不再显示「⚠ 降级匹配」。
- `workshop-article-archive`（`openspec/specs/workshop-article-archive/spec.md`）：归档行标签带去「降级半透明」；弱证据筛选与 popover「已降级 ↓」徽标删除。
- `daily-report-system`（`openspec/specs/daily-report-system/spec.md`）：质量筛选与 `BestTier` 映射不再读降级标记（max_sim 统一 tier=2）。

## Impact

- **代码**：后端 8 个文件（models/tag_models、semantic_board_matching、archive、board_match_handler、topicgraph 3 处、cmd/dump-sanitizer）+ `backend-go/internal/platform/database/constraints_test.go`（golden schema 用例）；前端 9 个文件（ArchiveMatchPopover、archiveMatchFlow、useBoardArchive、BoardArchiveView、MatchDetailPanel、SectionQualityExplore、utils/matchQuality、api/semanticBoards、api/dailyReports）+ 各自测试；一处 DDL 迁移脚本。
- **迁移**：`ALTER TABLE topic_tag_board_labels DROP COLUMN IF EXISTS downgraded`（幂等）；**上线顺序 = 先发代码并重启（停写停读）→ 后跑迁移**——老进程残留连接写已删列会报错，不接受迁移先行。
- **行为（分两层，必须区分）**：
  - **不变**：版块挂载集合、匹配分数、文章可见性、日报入选文章、归档弱证据**成员**（降级只在 max_sim 分支置位 ⇒ 原「已降级」行本就属 max_sim 弱证据类，删标记不改变成员）、标签弱文章判定结果。
  - **变（派生排序口径，随标记档位合并连带发生）**：① 版块文章列表 `match_best_tier` 质量序与分页——原「降级 max_sim」由 tier 3 升 tier 2，位置前移；② 日报 top-30 标签截断集合——原降级 max_sim 独占末档，现入 tier 2 参与竞争，入选标签集合可能变化（进而影响 LLM 聚类输入）；③ `daily_report_sections.best_tier`（3→2）→ 前端 section 排序；④ 归档行内标签次序——`ArchiveEvidenceRank` 原降级 max_sim 落末档 rank 5，现与普通 max_sim 同为 rank 4，改由 score/id tiebreak。
  - 用户可见的**唯一结构性变化**是前端「已降级 ↓」徽标消失；其余为排序/分页/截断的口径位移，无文章消失、无分数变化。
- **回滚**：`git revert` + 恢复列（`ALTER TABLE ... ADD COLUMN downgraded boolean`，历史值已随 DROP 丢失，重算挂载后按新逻辑不再置位——可接受，标记本就无功能）。
