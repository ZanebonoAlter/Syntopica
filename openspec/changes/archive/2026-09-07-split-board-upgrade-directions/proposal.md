<!-- complexity: complex -->
<!-- ui-impact: minor -->
<!-- constraint-domains: semantic-board -->

## Why

升级建议生成让 LLM 一次背三种决策（create_new / merge_into_existing / skip），实测输出质量差：17 条 LLM merge 的目标全部不在算法 shortlist 内、经常缺 target_board_id 被迫降级 create_new，前端只能兜底成「每条 merge 建议都让用户重新人工挑目标」——建议的指向价值名存实亡。discover_new / expand_existing 两个 mode 的 prompt 几乎逐字相同，选择形同虚设；watch 观察池与高置信自动合并两类合成产出实践证明无意义；旧内存探索 UI（候选/簇/内存建议）与持久化建议区并存造成混乱。

## What Changes

- **生成入口重构为「方向 × 来源」两步选择**：方向 = 创建版块 / 版块扩充；来源 = 单标签（聚类）/ 组合标签（共现对）。LLM 每轮只面对单一决策空间：
  - 创建×单标签：簇 → `create_new | skip`
  - 创建×组合：共现对 → `compose | skip`（现状）
  - 扩充×单标签：候选 → `属于版块 B 吗`（merge，target 天然 = 锁定版块）
  - 扩充×组合：共现对 → `值得组合且属于 B 吗`（确认 = 创建组合标签 + 挂载进 B）
- **版块扩充锁定单版块**：扩充方向下用户先从下拉单选一个目标版块，只对该版块生成扩充建议；LLM 上下文聚焦该版块完整画像（描述 + 现有构成标签 + 近期内容标题），从「全库检索指目标」降为「二分类判断」。
- **扩充候选召回新逻辑**：与目标版块 embedding 相似的未挂载 active aux + 与版块构成标签高频共现的 aux（单标签路）；与版块相关（组件相似/共现）的共现对（组合路）。
- **watch 观察池整体退役**：单例簇不再产 watch 建议，`GCOldWatch` 退役，存量 pending watch 行清理，UI 过滤 tab 移除，flow 红线 8 同步废止。
- **高置信自动合并退役**：`highConfidenceMergeBoard` / `synthesizeHighConfidenceMerge`（无 LLM 双签名合成 merge）删除——其产出目标为 shortlist top1（实测视野窄），且与「扩充必须手动选版块」的新原则冲突。
- **merge 目标校验链路简化**：锁定版块模式下 target 恒为选定版块，`validateMergeTargets` / `target_off_shortlist` 保留标注 / 缺 target 降级 create_new 等兜底逻辑随全量决策空间一起删除。
- **定时任务 `job_board_upgrade_suggest` 收窄为只跑创建方向**（建版块 + 建组合，自动涌现）；版块扩充纯手动（必须选版块触发）。
- **旧内存探索 UI 退役**（**BREAKING**）：`UpgradeSuggestionPanel` 上半区（候选列表/簇/内存建议/「获取 LLM 建议」链路）与 `suggest` 事件链路、candidates/clusters props、composable 旧链路整体删除；保留持久化建议区并重构为新的模式选择入口。`suggestUpgrades` 内存建议 API 与 mode 参数语义由新入口取代（mode 取值 `discover_new`/`expand_existing` 由 `create`/`expand` + 版块参数取代）。
- **存量数据**：pending watch 行清理；高置信 merge 存量 pending 行保留可确认（生命周期不变，仅不再新生成）。

## Capabilities

### New Capabilities

（无——全部为既有能力的行为变更）

### Modified Capabilities

- `board-upgrade`: LLM 决策空间按「方向×来源」拆分为四个单选项模式；watch 观察池与高置信自动合并两类合成建议退役；compose 建议新增扩充方向（创建组合标签 + 挂载目标版块的确认路径）；merge 目标 shortlist 校验链路删除；定时生成收窄为创建方向。
- `board-upgrade-expand`: `expand_existing` 模式定义整体重写——从「LLM 在全决策空间中指目标」改为「用户锁定单版块 + 二分类裁决 + 版块画像上下文」；新增扩充候选召回（相似 aux / 共现 aux / 相关共现对）。（`upgrade-candidate-time-window` 复核过：days 参数契约不受影响，不需要 delta。）

## Impact

- **后端**：`backend-go/internal/tagmanagement/service/board/`（`semantic_board_upgrade.go` 主流程拆分、`semantic_board_compose.go` compose 段扩展、新增扩充召回逻辑、watch/高置信合并删除）、`semantic_board_upgrade_gc.go`（GCOldWatch 退役）、handler 层（`board_upgrade_handler.go` suggest/generate API 参数重构）、scheduler job 注册（`job_board_upgrade_suggest` 参数收窄）。
- **数据库**：`board_upgrade_suggestions` 存量 pending watch 行一次性清理（迁移或启动清理，design 定）；无表结构变更（decision 值域变化为应用层约束）。
- **前端**：`UpgradeSuggestionPanel.vue` 大改（旧上半区删除、模式选择器 + 版块单选下拉新增）、`TagsPage.vue` / composable（`useSemanticBoardUpgrade` 等）旧 suggest 链路删除、`api/semanticBoards.ts`（suggest API 调整）。
- **文档**：`docs/reference/flow/semantic-board.md`（红线 8 watch 废止、升级建议流程重写、变更溯源）、`docs/reference/api/`（suggest API 参数）、`docs/reference/standard/frontend/layout.md` 不动（复用 dialog 契约）。
- **兼容性**：suggest API 参数语义变化为前端与后端同仓同步切换，无外部调用方；watch 建议消失为用户可见行为变化（预期内）。
