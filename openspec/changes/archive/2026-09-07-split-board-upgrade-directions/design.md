# Design — split-board-upgrade-directions

## Context

现状 `GenerateSuggestions(ctx, mode)` 单管线混跑：cluster 轮（LLM 全决策空间 + watch/高置信合成）+ compose 轮（无 target）。LLM merge 产出不可信（17 条全 off-shortlist、缺 target 降级），watch 观察池无意义，旧内存探索 UI 与持久化区并存。动机见 proposal.md。

关键现状锚点（探索已确认）：

- 决策枚举 `SemanticBoardUpgradeDecision`：create_new/merge_into_existing/skip/watch/compose（`semantic_board_upgrade.go` L121-127）。
- `ComputeSuggestionHash(mode, decision, targetBoardID, auxIDs)`（`board_upgrade_suggestion_persist.go` L106）已含 mode 与 target——direction:source 作为新 mode 值后，hash 自动区分新旧建议，存量 pending 不会误命中幂等/冷却。
- 共现统计无独立表：`collectComposeCandidates` 从 `article_topic_tags ⋈ topic_tag_semantic_labels` 应用层现算（窗口 `CoTagWindowDays`）。
- 近期内容查询：shortlist 的 `RecentSections`（≤5 条 section 标题，L913-960 `loadLaneBriefs`）可改为按版块直查。
- 定时任务 `BoardUpgradeSuggestJob`（`admin/scheduler/job_board_upgrade_suggest.go`）：调 `GenerateAndPersist(ctx, "discover_new")` + `GCOldWatch`，失败仅记日志返回 nil（flow 红线 10）。
- 存量清理惯例：`postgres_migrations.go` one-shot 幂等 DELETE（参照 `watchMaterializedHintCleanupMigration`）。
- 并行约束：本 change 退役 watch 时，`watch-materialize-llm-adjudication` 等 change 在 topicgraph 域并行推进，不共享文件。

## Goals / Non-Goals

**Goals:**
- 生成入口参数化：`direction × source × target_board_id` 四格分发，每格单一 LLM 决策空间。
- 扩充召回与版块画像 prompt 的新实现。
- watch / 高置信合成 / shortlist 校验链 / 旧内存 UI 的完整退役与存量清理。
- 定时任务收窄为创建方向。

**Non-Goals:**
- 不动匹配四规则（tag→board matching）、compose 去重（L1/L2）、suggestion 生命周期（hash 幂等/冷却期/confirmed 状态机）。
- 不动 ConfirmSuggestion 的 create_new/merge 既有执行路径语义（仅 compose 分支加挂载）。
- 不做多版块批量扩充（用户明确选单选）。
- 不做 watch 决策值枚举删除（保留枚举值兼容存量行读取，仅不再产生）。

## Decisions

### D1. 请求结构与四格分发

`GenerateSuggestions(ctx, req)` 改结构化请求：

```go
type UpgradeGenerateRequest struct {
    Direction string // "create" | "expand"
    Source    string // "aux" | "composite"
    TargetBoardID uint // direction=expand 必填；create 必须为 0
    Days      int    // 仅 create 方向生效
}
```

分发矩阵：

| Direction | Source | 管线 |
| --- | --- | --- |
| create | aux | 现有 cluster 管线（减 watch/高置信/shortlist），prompt 重写为 create 专用 |
| create | composite | 现有 compose 管线不变（无 target） |
| expand | aux | 新召回 + 版块画像 prompt + 二分类 |
| expand | composite | compose 管线 + 版块相关过滤 + target 注入 |

handler 层 query 参数直映射 + 校验（expand 缺 target / create 带 target / target 非活跃版块 → 400）。**备选**：保留旧 mode 参数做兼容——放弃，前端同仓同步切换无外部调用方，留兼容层只会让两条路都难测。

### D2. 创建×单标签管线瘦身

保留：CollectCandidates（含 days 时间窗）→ ClusterCandidates → loadCoTagEventContext → LLM。
删除：单例簇 watch 合成、高置信 merge 合成、computeShortlist/loadLaneBriefs/loadLaneAffinities 整条 shortlist 链、validateMergeTargets/mergeTargetInShortlist/buildShortlistByAux、缺 target 降级 create_new 兜底。
prompt 重写：候选簇 + co-tag 事件 + **全量活跃版块清单**（名称 + 一句描述；超 60 个版块按与簇质心相似度截断 top-60，防 token 膨胀）→ 决策空间 {create_new|skip}，防重复原则：主题与既有版块重复的簇输出 skip。

### D3. 扩充召回（新文件 `semantic_board_expand.go`）

- **相似召回**：目标版块 embedding vs 全体 active aux 的 `merge_embedding` 余弦距离（GORM + pgvector，一次查询），取距离 ≤ `semantic_board_expand_sim_distance`（默认 0.35，可调）的 aux，上限 40。
- **共现召回**：复用 compose 的文章→aux 映射查询（同 `CoTagWindowDays` 窗口），统计每个未挂载 aux 与「版块构成标签集」的同文章共现次数 ≥ `semantic_board_expand_coocurrence`（默认 3），上限 40。
- 两路并集去重（相似路优先，共现作证据补充），排除：已在目标版块构成的 aux、已 disabled 的 aux。days 参数在扩充路忽略（共现已有自己的窗口，相似是全库语义）。
- **组合路过滤**：compose 候选（对/三元组）中至少一个组件 ∈ 上述召回集 ∪ 目标版块构成集，其余丢弃。

**备选**：只做相似召回——放弃，共现能捞到语义向量近但措辞迥异的标签（如「美债拍卖」与「国债期货」），与匹配四规则的 direct_hit 思路一致。

### D4. 版块画像 prompt 与二分类

prompt 结构（expand 两路共用画像头）：

```
【目标版块】美债（ID=42）：版块描述
现有构成：<label 列表，含组合标签标「组合」>
近期内容：≤8 条近期匹配 section 标题（按版块直查，复用 RecentSections 查询改版块版）

任务（单标签路）：逐个判断候选辅助标签是否属于该版块 → decision=merge|skip
任务（组合路）：逐个判断共现组合「值得创建组合标签且属于该版块」→ decision=compose|skip
候选：带证据（相似距离 / 共现次数 / 代表文章标题）
```

target 由服务端注入（= TargetBoardID），LLM 输出不含目标字段，杜绝缺 target/off-target 兜底逻辑复活。System prompt 按 direction+source 生成单一 schema。

### D5. compose 确认挂载与 suggestion 语义

- compose 建议结构不变，`TargetBoardID` 复用现有字段：expand 路生成时注入，create 路为 nil。
- `ConfirmSuggestion` compose 分支扩展：建组合（含 L1/L2 去重复用）后，若建议 target 非空则同事务写 `board_composition`（组合 → 版块挂载行）；任一步失败整体回滚（flow 红线 16 精神同源）。
- 挂载后触发 board match cache 失效（复用现有 confirm 后失效路径）。
- hash 的 mode 值统一用 `direction:source`（如 `expand:composite`），新旧建议 hash 空间天然隔离。

### D6. watch 与高置信存量处理

- 一次性迁移：`DELETE FROM board_upgrade_suggestions WHERE decision='watch'`（幂等，参照 watchMaterializedHintCleanupMigration 模式）。
- 高置信 merge 存量 pending 行保留，确认/dismiss 路径不变（confirm 的 merge 分支不动）。
- `GCOldWatch`/`LoadWatchGCDays`/`semantic_board_upgrade_watch_gc_days` 配置键随代码删除（残留配置行无害，不迁移清理）。
- 决策枚举保留 watch 值（DTO/前端过滤兼容），仅生成侧不再产生。

### D7. 定时任务收窄

`BoardUpgradeSuggestJob` 改为顺序跑两段创建方向：`{create, aux}` → `{create, composite}`。任一段失败仅记日志继续下一段（红线 10 语义：不阻塞兄弟 job），JobResult 统计字段保留 inserted/skipped/cooldown、删 watch_gc。生成时间点配置（06:30）不变。

### D8. 前端重构

- `UpgradeSuggestionPanel.vue`：删上半区（candidates/clusters/suggestions props、upgradeMode radio、suggest 事件）；持久化区上方新增生成入口（方向两选 → 来源两选 → 扩充时版块单选下拉复用 `usp-merge-dropdown--search` 样式族 + days 下拉仅创建方向启用）。
- 建议卡片：扩充建议展示锁定版块徽标；删「合并到...」行内下拉；per-row aux 勾选保留（确认子集）。
- composable/`TagsPage.vue`：删旧 suggest 链路（handleSuggestUpgrade/handleUpgradeSuggest/upgradeCandidates 等）；`api/semanticBoards.ts`：suggest 调用改新参数，删 candidates API。
- filter tabs：删 watch tab，决策类型剩 全部/新建/合并/组合。

## Risks / Trade-offs

- [并行 change 同仓编辑] → 本 change 触及 `postgres_migrations.go`（迁移注册）与 topicgraph 域有并行工作，编辑前先 `git status` 核对，迁移追加在文件尾部不碰他域。
- [全量版块清单 token 膨胀] → 按簇质心相似度截断 top-60 + 名称/一句话描述；后续版块规模过大再考虑两级 LLM。
- [扩充召回阈值过严导致空建议] → 两个阈值 ai_settings 可配；空结果返回正常空列表，UI 提示「版块可能已充分覆盖」。
- [LLM 二分类误挂] → 确认前 per-row 勾选子集兜底（沿用现有交互）；挂错可经版块编辑移除。
- [watch 存量删除不可逆] → 观察池行本就是 30 天 GC 的垃圾数据，删除可接受；迁移幂等。
- [hash mode 值变更] → 旧 pending 建议 hash 不再匹配新生成，冷却期按新空间计算——预期行为（模式语义已变）。

## Migration Plan

1. 合并部署顺序：后端（新 API 参数 + 迁移 + 定时任务）→ 前端（新入口）同仓一次交付。
2. 部署后：迁移自动清 watch pending 行；旧 UI 已删，无回滚入口需求。
3. 回滚：代码回滚即可（无表结构变更）；watch 行删除不恢复（垃圾数据）。
4. 用户可见变化（完工汇报须含）：建议面板入口全换、watch 建议消失、定时任务不再产扩充建议——扩充改为手动选版块触发。

## Open Questions

（无——召回阈值默认值上线后按实际效果在 ai_settings 调，不阻塞本 change。）
