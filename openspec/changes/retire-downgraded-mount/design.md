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

### 前端

| 文件 | 动作 |
| --- | --- |
| `front/app/features/tags/components/console/ArchiveMatchPopover.vue` | :535「已降级 ↓」徽标、:735「↓」角标、:106 ruleColor downgraded 参数删 |
| `front/app/features/tags/components/console/archiveMatchFlow.ts` | :254「 ⚠降级匹配」后缀删 |
| `front/app/features/tags/composables/useBoardArchive.ts` | :83 弱文章判定 `!tag.downgraded` 条项删 |
| 对应 `.test.ts` | 断言同步 |

## D2. MatchTier 合并

现档位：direct_hit=0 / hit_rate=1 / max_sim(未降级)=2 / max_sim(降级)|weighted=3。
新档位：direct_hit=0 / hit_rate=1 / max_sim=2 / weighted=3。
影响：30 条截断排序与分桶挑档中，原降级 max_sim 从 3 档升 2 档——**排序变化可接受**（降级 max_sim 本就是强相似度挂载，与 weighted 同档才是低估）；SQL ORDER BY（board_match_handler.go:621）同步。

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
