<!-- complexity: complex -->
<!-- ui-impact: minor -->
<!-- constraint-domains: semantic-board -->

# Proposal: add-ghost-mount-unmount

## Why

direction_mismatch 挂载目前只标记不摘除——科技巨头板 177 个 ghost（40%）全是「Claude Opus/GPT 发布」类真家在生成式AI 的双归属内容，长期占用板块内容位、拉低体检观感；且全代码库没有任何 mismatch 摘除动线（2026-10-21 explore 结论）。真库实证：177 个中 **167 个（94%）是双挂形态**（真家已有干净挂载，摘掉次优挂载安全），仅 10 个单挂（摘后裸奔，不进候选）；美国板 31/50、影视 7/11 同口径。

## What Changes

- **墓碑表防复活**：新表 `topic_tag_board_label_tombstones`（(topic_tag_id, semantic_board_id) 唯一）；匹配管道在候选截断（MaxBoards）**前**过滤墓碑命中的 tag-board 对——不解决防复活，卸载会被 rematch-all/增量重算的弱证据路径挂回来，动线白做。
- **卸载 API**：`POST /api/semantic-boards/:id/unmount-tags`（批量 tag IDs，同事务删挂载行+写墓碑，幂等：挂载行不存在跳过、墓碑已存在忽略）。
- **墓碑管理**：`GET /api/semantic-boards/:id/mount-tombstones`（列表，带 tag label/时间）+ `DELETE /api/semantic-boards/:id/mount-tombstones/:topicTagId`（删墓碑+触发该 tag 重算，一键恢复）。
- **体检附属卸载候选**：快照新增 `unmount_candidates` 列（对标 uncaptured_candidates 模式，红线 20 兼容——快照附属产物、零 pending 行）：`direction_mismatch=true` 且同 tag 在其它板有干净挂载（双挂口径）的 top N 清单（tag label + mounted/best cosine + 真家板名）。
- **UI（minor，复用契约）**：体检面板 BoardHealthPanel 复用「未收编候选」卡片模式新增「卸载建议」清单（勾选+批量卸载）与墓碑列表查看/撤销入口。

## Capabilities

### New Capabilities

- `board-mount-tombstone`：墓碑表、匹配防复活过滤、卸载 API、墓碑列表/撤销 API。

### Modified Capabilities

- `board-health-checkup`（`openspec/specs/board-health-checkup/spec.md`）：新增「体检快照附属卸载候选」Requirement（零 pending、双挂口径、top N）。

## Impact

- **数据模型**：新表（GORM AutoMigrate 注入注册）+ board_health_snapshots 加列（nullable，向后兼容）。
- **代码**：`tagmanagement/models/tag_models.go`（墓碑模型）、`semantic_board_matching.go` MatchTopicTag（截断前过滤）、新 handler（卸载/墓碑 3 端点）、boardhealth（候选收集+快照列）、路由注册；前端 `BoardHealthPanel.vue` + api client。
- **行为**：卸载后该 tag-board 不再被任何匹配路径挂回（增量/rematch-all/backfill 统一经 replace 前过滤）；体检快照多一列附属产物（纯读轮次不变）。
- **风险**：墓碑过滤放在 MaxBoards 截断前，避免墓碑行占名额挤掉合法候选；卸载不可逆性由墓碑撤销兜底。
