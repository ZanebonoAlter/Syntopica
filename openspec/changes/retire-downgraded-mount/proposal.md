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
- **文档**：`docs/reference/flow/semantic-board.md` 业务约束与不变量节对齐（min(2,N) 放宽准入保留、降级标记退役；红线 5 措辞不涉及行为变化仅去 downgraded 引用）。

## Capabilities

### Modified Capabilities

- `semantic-board`（`openspec/specs/semantic-board/spec.md`）：挂载判定的 requirement 中 downgraded 相关 Scenario 删除；MatchTier 档位与归档弱证据口径 requirement 更新。

## Impact

- **代码**：后端 5 个文件（models/tag_models、semantic_board_matching、archive、board_match_handler、topicgraph 3 处）+ 前端 3 个文件 + 各自测试；一处 DDL 迁移脚本。
- **迁移**：`ALTER TABLE topic_tag_board_labels DROP COLUMN IF EXISTS downgraded`（幂等）；上线顺序 = 先发代码（停写停读）→ 后跑迁移，或迁移先行（GORM 模型删字段后读写均不再触碰该列，两序皆安全）。
- **行为**：版块挂载、日报、聚类、归档**全部不变**；唯一用户可见变化 = 前端「已降级」徽标消失。
- **回滚**：`git revert` + 恢复列（`ALTER TABLE ... ADD COLUMN downgraded boolean`，历史值已随 DROP 丢失，重算挂载后按新逻辑不再置位——可接受，标记本就无功能）。
