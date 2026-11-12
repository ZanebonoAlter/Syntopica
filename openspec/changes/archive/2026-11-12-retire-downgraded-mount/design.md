# Design — retire-downgraded-mount

## D1. 删除范围（精确锚点）

### 后端

| 文件 | 位置 | 动作 |
| --- | --- | --- |
| `tagmanagement/models/tag_models.go` | :78 `Downgraded bool` | TopicTagBoardLabel 字段删 |
| `tagmanagement/service/board/semantic_board_matching.go` | :87 MatchResult.Downgraded；:477-486 置位；:494 赋值；:845 落库构造 | 全删；`min(2,N)` 门槛计算保留 |
| `tagmanagement/service/board/archive.go` | :148 ApplyWeakEvidenceFilter；:166 ArchiveEvidenceRank(matchReason, downgraded)；:185 IsWeakEvidence | strong 判定去 `NOT downgraded` 项；两函数签名去 downgraded 参数 |
| `tagmanagement/handler/board_match_handler.go` | :31/:69 DTO；:249/:254/:621 SQL；:278/:421 赋值；:285-286 排序调用；:298 弱证据调用；:641-649 MatchTier | 字段/SQL/赋值删；MatchTier 的 `max_sim && !downgraded` 档与降级档合并为统一 max_sim 档（tier=2），weighted 档位值不变 |
| `topicgraph/repository/daily_report_models.go` | :324 Downgraded | 字段删 |
| `topicgraph/service/daily_report_orchestrator.go` | :579 TagInput.Downgraded；:592 SELECT 列；:613 GROUP BY 列；:731-742 quality_breakdown 构造 | 全删；`buildQualityBreakdownJSON` 不再写 downgraded 键 |
| `topicgraph/service/daily_report_rebucket.go` | :191 Downgraded | 字段删 |
| `cmd/dump-sanitizer/tables.go` | :137 `topic_tag_board_labels` Columns 含 `downgraded`；:209/:213 `analysis_methods` / `reference_roles` 条目 | `downgraded` 从列清单删（否则演示 seed 导出在新 schema 上报列不存在）；两条 stale 表条目删（两表已被迁移 `restructure-settings-navigation` DROP） |
| `internal/platform/database/constraints_test.go` | :109 golden schema 用例含 `topic_tag_board_labels.downgraded` | 该行删（列已不存在，`information_schema` 查 0 行 → `no rows in result set`） |

### 前端

| 文件 | 动作 |
| --- | --- |
| `front/app/features/tags/components/console/ArchiveMatchPopover.vue` | :535「已降级 ↓」徽标、:735「↓」角标、:106 ruleColor downgraded 参数删 |
| `front/app/features/tags/components/console/archiveMatchFlow.ts` | :254「 ⚠降级匹配」后缀删 |
| `front/app/features/tags/composables/useBoardArchive.ts` | :83 弱文章判定 `!tag.downgraded` 条项删 |
| `front/app/features/tags/components/console/BoardArchiveView.vue` | :157/:218 TagView 字段；:549 `is-deg` class；:557 `↓` 角标 | 字段/class/角标删 |
| `front/app/features/tags/components/MatchDetailPanel.vue` | :210 「⚠降级匹配（原阈值…）」后缀 | 后缀删 |
| `front/app/features/tags/components/daily-report/SectionQualityExplore.vue` | :55 `--downgraded` chip class；:57 color 参数 | class/参数删 |
| `front/app/utils/matchQuality.ts` | :30 `matchReasonColor(reason, downgraded?)`；:38 `matchInfoLabel` 的 `↓` 后缀 | 函数单参化；后缀删 |
| `front/app/api/semanticBoards.ts` / `front/app/api/dailyReports.ts` | `BoardArticleTag` / 匹配详情 / quality_breakdown 类型的 `downgraded` 字段 | 字段删 |
| 对应 `.test.ts` | 断言同步 |

## D2. MatchTier 合并

现档位：direct_hit=0 / hit_rate=1 / max_sim(未降级)=2 / max_sim(降级)|weighted=3。
新档位：direct_hit=0 / hit_rate=1 / max_sim=2 / weighted=3。
影响：30 条截断排序与分桶挑档中，原降级 max_sim 从 3 档升 2 档——**排序变化可接受**（降级 max_sim 本就是强相似度挂载，与 weighted 同档才是低估）；SQL ORDER BY（board_match_handler.go:621）同步。

### D2.1 派生排序口径变化清单（连带面，须在汇报中如实披露）

档位合并不止改一个排序键，凡消费 tier 的派生口径都会位移：

| 消费点 | 位置 | 变化 |
| --- | --- | --- |
| 版块文章列表质量序 + 分页 | `board_match_handler.go:210`（`Order("match_best_tier ASC, …")`）+ `:613-619` ORDER BY CASE | 原「降级 max_sim」文章 tier 3→2，位置前移；**先全局排序后分页** ⇒ 分页页内容随之变 |
| 日报 top-30 标签截断 | `daily_report_orchestrator.go:775-776`（`filterTagsByQuality` 按 `(tier, score)` 截断） | 原降级 max_sim 独占末档不入前 30；现入 tier 2 竞争 ⇒ **入选标签集合可能变化** ⇒ LLM 聚类输入随之变化 ⇒ section 归属可能漂移 |
| `daily_report_sections.best_tier` | `daily_report_orchestrator.go:241` | 3→2；前端 `sortDailyReportSections`（`dailyReportMagazine.ts:68`）按 `best_tier ASC, avg_score DESC` ⇒ **section 顺序变** |
| 归档行内标签次序 | `board_match_handler.go:280-290`（`ArchiveEvidenceRank` + score/id tiebreak） | 原降级 max_sim 落末档 rank 5；现与普通 max_sim 同为 rank 4，改由 score/id 决定 |

**不变项**（明确不受影响）：挂载行集合与分数、文章可见性、日报入选文章、归档弱证据**成员**（降级只在 max_sim 分支置位 ⇒ 原「已降级」行本就属 max_sim 弱证据类）、标签弱文章判定结果。

### D2.2 归档证据档位重排（ArchiveEvidenceRank）

改前：`case "max_sim" && !downgraded: return 4` / default 5。改后：`case "max_sim": return 4` / default 5。
⇒ 原「已降级 max_sim」在归档首条标签排序上由 rank 5（末位）升 rank 4，与普通 max_sim 同档，改由 score/id tiebreak 决定先后。**行成员不变，仅行内次序变**。

### D2.3 真库核对（迁移前后）

迁移后只读核对（已在真库执行，见 tasks 8）：
- `topic_tag_board_labels` 行数：迁移前 3627 → 迁移后 3627（零删除）；
- 迁移前 `downgraded=true` 1001 行 / `false` 2626 行；列已删（`information_schema` 0 行）；
- 后端 `/health` 200。

## D3. DDL 迁移

```sql
ALTER TABLE topic_tag_board_labels DROP COLUMN IF EXISTS downgraded;
```

- 幂等；**不 DELETE 行**（挂载行为不变，全部挂载行保留）。
- 顺序安全：GORM 模型删字段后代码不再 SELECT 该列（显式列名处全部清理），先迁移后发码 / 先发码后迁移两序皆可；唯一约束顺序风险 = 发码后老进程残留连接仍在写 downgraded → 列被删后报错。**采用：先停写（发码重启）→ 跑迁移**，与部署顺序绑定（tasks 中为独立步骤）。
- 回滚：`git revert` + `ADD COLUMN downgraded boolean`；历史标记值不可恢复（无功能，可接受，proposal Impact 已声明）。

## D4. 历史兼容

- `daily_report_sections.quality_breakdown` 历史 JSON 里的 `downgraded` 键：读取端（前端日报详情）不消费该键，直接忽略；新报告不再写入。
- 前后端两版本任意组合安全（ui-design.md §2 已论证）。

## D5. 明确不做

- `min(2, N)` 门槛计算、放宽准入行为、挂载行数据：**一律不动**。
- `direction_mismatch`：不动（它是真实过滤项）。
- AI 路由「降级观测」（DegradationPanel）：同名不同物，不动。
