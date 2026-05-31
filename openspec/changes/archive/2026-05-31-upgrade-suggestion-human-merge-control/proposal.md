## Why

当前板块升级建议流程中，LLM 同时决定 `create_new`、`merge_into_existing` 和 `skip`，但 merge 决策需要理解已有板块的语义边界，容易出错且用户无法干预。此外，Phase A（候选与已有板块匹配）直接影响聚类结果，导致同一语义空间的候选被已有板块拆分，限制了新板块的发现。将 merge 决策权交给用户，让 LLM 只负责 create/skip 判断，可以提高准确性和用户控制感。

## What Changes

- **聚类阶段分离**：`clusterCandidates()` 中的 Phase A（匹配已有板块）不再影响聚类。所有候选统一走 Phase B（纯自聚类），Phase A 仅计算元数据（`board_affinities`）附加到每个簇上。
- **Cluster 结构新增 `BoardAffinities`**：每个簇附带相似已有板块列表（board_id、board_label、匹配候选数、平均距离），供前端展示。
- **LLM 决策简化**：LLM prompt 只产出 `create_new` 或 `skip`，移除 `merge_into_existing` 选项。
- **前端合并下拉 UI**：`UpgradeSuggestionPanel.vue` 每个建议卡增加"合并到..."下拉按钮，显示该簇的 board_affinities，用户可手动选择合并目标。
- **`ConfirmSuggestion` API 不变**：后端 `merge_into_existing` 路径保持不变，只是触发来源从 LLM 改为前端用户操作。

## Capabilities

### New Capabilities

- `board-affinity-metadata`: 簇级别计算已有板块亲和度（matching_candidates、avg_distance），作为参考元数据供前端展示，不影响聚类逻辑

### Modified Capabilities

- `board-upgrade`: 聚类阶段移除 Phase A 对聚类的直接影响；LLM 决策从三选一简化为 create_new / skip；前端新增人工合并触发 UI

## Impact

- **后端**：`backend-go/internal/domain/tagging/semantic_board_upgrade.go` — `clusterCandidates()`、`SemanticBoardUpgradeCluster` 结构体、`buildSemanticBoardUpgradePrompt()`、handler DTO 序列化
- **前端**：`front/app/features/tags/components/UpgradeSuggestionPanel.vue` — 新增 merge dropdown；`front/app/api/semanticBoards.ts` — TypeScript 接口更新
- **API 契约**：`/api/semantic-boards/upgrade-suggest` 响应中 suggestion 只含 create_new/skip，每个 suggestion 内嵌 `board_affinities`（从对应 cluster 汇总）；cluster 新增 board_affinities 字段；`/api/semantic-boards/upgrade-execute` 请求/响应不变
- **无破坏性变更**：`merge_into_existing` 在 ConfirmSuggestion 中仍然有效，只是不再由 LLM 自动产出
- **配置参数新增**：`ai_settings` 表新增 `semantic_board_upgrade_cluster_method`（`"average_link"` | `"centroid"`，默认 `"average_link"`），控制聚类算法选择，通过现有 `/api/semantic-boards/matching-config` 端点读写，前端"匹配参数"面板展示
