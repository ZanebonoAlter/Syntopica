## 1. 后端：请求结构与四格分发骨架

- [x] 1.1 定义 `UpgradeGenerateRequest`（direction/source/target_board_id/days）并重构 `GenerateSuggestions(ctx, req)` 签名，四格分发骨架落位（create×aux / create×composite 走现有管线占位，expand 两格先返回「未实现」错误占位）；`go build ./...` 通过
- [x] 1.2 suggest handler 改读新 query 参数（direction/source/target_board_id/days），实现参数校验（expand 缺 target / create 带 target / target 非活跃版块 → 400，旧 mode 参数拒绝）；`go test ./internal/tagmanagement/handler/` 对应用例通过

## 2. 后端：创建方向管线瘦身

- [x] 2.1 删除 watch/高置信合成链：`synthesizeWatchSuggestion`、`highConfidenceMergeBoard`、`synthesizeHighConfidenceMerge`、`GCOldWatch`/`LoadWatchGCDays` 及 `semantic_board_upgrade_watch_gc_days` 读取；相关单测同步删除/改写；`go test ./internal/tagmanagement/service/board/` 通过
- [x] 2.2 删除 shortlist 校验链：`computeShortlist`/`loadLaneBriefs`/`loadLaneAffinities`/`validateMergeTargets`/`mergeTargetInShortlist`/`buildShortlistByAux`/缺 target 降级兜底；`go build ./...` 与 board 包测试通过
- [x] 2.3 重写 create×aux prompt（候选簇 + co-tag 事件 + 全量活跃版块清单按簇质心相似度截断 top-60 → {create_new|skip}，防重复原则）；prompt 构造单测覆盖「版块清单进 prompt」「超 60 截断」
- [x] 2.4 补修（2026-09-07 报障：确认过的组合重复被建议）：`filterExistingComposeCandidates`——create 路排除组件集与既有 active composite 完全一致的候选（部分重叠不误伤）；接入 `generateComposeSuggestions`；单测 `TestFilterExistingComposeCandidates`

## 3. 后端：扩充召回与版块画像

- [x] 3.1 新建 `semantic_board_expand.go` 实现单标签召回：相似路（版块 embedding vs active aux merge_embedding，距离阈值 `semantic_board_expand_sim_distance` 默认 0.35，上限 40）+ 共现路（复用 compose 文章→aux 映射，与版块构成共现 ≥ `semantic_board_expand_coocurrence` 默认 3，上限 40），并集去重、排除已挂载/disabled；召回单测（相似命中/共现命中/排除已挂载/空召回）
- [x] 3.2 实现组合路过滤：compose 候选中至少一组件 ∈ 召回集 ∪ 版块构成集；过滤单测通过
- [x] 3.3 实现版块画像 prompt（版块描述 + 构成标签列表含组合标记 + 近期 section 标题 ≤8 条按版块直查）与二分类裁决（expand×aux → merge|skip；expand×composite → compose|skip 且服务端注入 target）；prompt 单测 + LLM mock 裁决单测（含 target 注入断言）
- [x] 3.4 expand 两路接入 `GenerateSuggestions` 分发（days 忽略）；`go test ./internal/tagmanagement/service/board/` 全绿

## 4. 后端：compose 挂载、hash、存量清理、定时任务

- [x] 4.1 `ConfirmSuggestion` compose 分支扩展：建议携带 target 时同事务建组合（含去重复用）+ 写 board_composition + 缓存失效，失败整体回滚建议保持 pending；单测覆盖「扩充确认挂载成功」「挂载失败回滚」「复用既有组合仍挂载」
- [x] 4.2 suggestion_hash 的 mode 值改为 `direction:source`（create:aux / create:composite / expand:aux / expand:composite）；hash 单测断言四格互异且带 target
- [x] 4.3 一次性迁移：幂等 DELETE `board_upgrade_suggestions WHERE decision='watch'`（追加在 postgres_migrations.go 尾部）；迁移测试（二次执行 no-op）
- [x] 4.4 `BoardUpgradeSuggestJob` 收窄：顺序跑 {create,aux} + {create,composite}，段失败仅记日志继续，删 watch GC 段与统计字段；scheduler 包测试通过
- [x] 4.5 补修（2026-09-07 报障：旧结果永久干扰）：expand 路 compose 候选接入挂载感知过滤（已挂载目标版块的组合排除，未挂载保留——挂载语义）；迁移 20260907_0001 幂算置 dismissed 存量 pending discover_new 行（150 条，留痕 legacy_discover_new_cleanup，dev 库 22:57 已自动应用）；迁移幂等测试 + 过滤单测通过

## 5. 后端：handler 清理

- [x] 5.1 删除 getUpgradeCandidates 端点（`/upgrade/candidates`）与路由注册，suggest 响应附带候选规模统计（召回数/送裁数）；路由测试更新，`go test ./internal/tagmanagement/...` 影响包通过

## 6. 前端：面板与链路重构

- [x] 6.1 `api/semanticBoards.ts`：suggest 改新参数（direction/source/target_board_id/days），删 candidates 调用；类型定义同步
- [x] 6.2 `UpgradeSuggestionPanel.vue`：删旧上半区（candidates/clusters/suggestions props、upgradeMode radio、suggest 事件、内存建议卡片区）；新增生成入口（方向两选 → 来源两选 → 扩充版块单选下拉复用 usp-merge-dropdown 样式族、days 仅创建方向启用、未选版块禁用生成）；filter tabs 删 watch；扩充建议卡片展示锁定版块徽标、删「合并到...」行内下拉；`pnpm lint` 通过
- [x] 6.3 composable 与 `TagsPage.vue`：删 handleSuggestUpgrade/handleUpgradeSuggest/upgradeCandidates/upgradeClusters/upgradeSuggestions 旧链路，接新生成入口；`pnpm exec nuxi typecheck`（Windows cmd）通过
- [x] 6.4 面板组件测试：模式选择两步交互、扩充未选版块禁用、生成调用参数正确、锁定版块徽标渲染、watch tab 不存在；`pnpm test:unit`（Windows cmd）通过
- [x] 6.5 补修（2026-09-07）：恢复被误删的 `.usp-overlay` 定位样式（fixed/inset/z-index，弹窗曾退化为流内 div 坠到页面底部）；补浮层展示锚单测（样式规则锚 + Teleport 挂载锚）；`pnpm vitest run app/features/tags/components/UpgradeSuggestionPanel.gen.test.ts` 11/11 通过

## 7. 测试

- [x] 7.1 test-cases.md 对账更新：实现中按「继承与调整」表逐行处置旧测试（改断言/删除留痕），补充实际落点文件路径；效果核对（扩充建议 target 恒定、四格 hash 隔离在单测量化）记入
- [x] 7.2 opencli 主链路：tags 页 → 升级建议面板 → 选「版块扩充+单标签+指定版块」→ 生成 → 建议列表出现且卡片带锁定版块；双视口（1440×900 / 1920×1080）截图留证（ui-design Acceptance）。（2026-09-07 补齐：截图实物落 `ui-verification/`，浏览器层 computed style 断言 fixed/inset:0/z-index:100，视觉子代理目检 PASS；补修前该任务勾选但无实物，属验收空转）

## 8. 文档

<!-- doc-impact: flow api database standard -->
<!-- doc-impact-excuse: configuration=启发式 config.*yaml 正则误命中 openspec 元数据 .openspec.yaml（configure-dsh 未跟踪 change 目录），本 change 无配置行为变更 -->

- [x] 8.1 `docs/reference/flow/semantic-board.md`：升级建议流程重写（四格入口/扩充召回/版块画像）、红线 8 watch 废止、红线 10 job 范围更新、变更溯源链接
- [x] 8.2 `docs/reference/api/`：suggest 端点参数（direction/source/target_board_id/days）、candidates 端点删除、generate 四格参数、execute compose 挂载语义
- [x] 8.3 `docs/reference/database/`：board_upgrade_suggestions 行为注记（decision 值域变化 + 迁移 20260905_0002 一次性 watch 清理，无表结构变更）

## 9. 验证

- [x] 9.1 `cd backend-go && golangci-lint run ./...` 退出码 0（本 change 域无 issue；topicgraph 并行 worktree 中间态另行留痕）
- [x] 9.2 `cd backend-go && go vet ./... && go build ./...` 退出码 0
- [x] 9.3 `cd backend-go && go test ./internal/tagmanagement/... ./internal/admin/scheduler ./internal/platform/database -short` 全绿（影响包经 `scripts/change-scope.sh` 判定）
- [x] 9.4 `cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm lint && pnpm exec nuxi typecheck && pnpm test:unit"` 全部退出码 0
- [x] 9.5 `bash scripts/doc-impact.sh verify openspec/changes/split-board-upgrade-directions/` 声明域（flow/api）无缺失
- [x] 9.6 `bash scripts/scenario-trace.sh openspec/changes/split-board-upgrade-directions/` 退出码 0（下表映射齐全）
- [x] 9.7 归档前增量复验（2026-09-07 晚两批补修后最新状态）：`cd backend-go && golangci-lint run ./... && go vet ./... && go build ./...` 退出码 0；`go test ./...` 全绿；`cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm lint && pnpm exec nuxi typecheck && pnpm test:unit && pnpm build"` 全部退出码 0（含浮层展示锚测试 TestFilterExistingComposeCandidates/迁移测试 TestLegacyDiscoverNewPendingDismissMigrationIdempotent/前端弹窗浮层展示锚 11/11）

### Scenario → 测试文件映射

| Scenario | 测试文件 |
| --- | --- |
| LLM 判断创建新 board | backend-go/internal/tagmanagement/service/board/semantic_board_upgrade_test.go |
| LLM 不再产出 merge_into_existing | backend-go/internal/tagmanagement/service/board/semantic_board_upgrade_test.go |
| 扩充×单标签产出挂载建议 | backend-go/internal/tagmanagement/service/board/semantic_board_expand_test.go |
| LLM 判断跳过 | backend-go/internal/tagmanagement/service/board/semantic_board_upgrade_test.go |
| API default mode backward compatible | backend-go/internal/tagmanagement/handler/semantic_board_handler_test.go |
| API expand mode | backend-go/internal/tagmanagement/handler/semantic_board_handler_test.go |
| 创建方向调用 | backend-go/internal/tagmanagement/handler/semantic_board_handler_test.go |
| 扩充方向调用 | backend-go/internal/tagmanagement/handler/semantic_board_handler_test.go |
| 扩充缺目标版块被拒 | backend-go/internal/tagmanagement/handler/semantic_board_handler_test.go |
| 旧 mode 参数被移除 | backend-go/internal/tagmanagement/handler/semantic_board_handler_test.go |
| 高频共现对产出 compose 建议 | backend-go/internal/tagmanagement/service/board/semantic_board_compose_test.go |
| 已存在组合不再被建议创建 | backend-go/internal/tagmanagement/service/board/semantic_board_compose_test.go |
| 已挂载组合不再被扩充建议 | backend-go/internal/tagmanagement/service/board/semantic_board_compose_test.go |
| 扩充方向的组合建议携带目标 | backend-go/internal/tagmanagement/service/board/semantic_board_compose_test.go |
| LLM 裁决无意义组合被过滤 | backend-go/internal/tagmanagement/service/board/semantic_board_compose_test.go |
| 同 hash 建议幂等 | backend-go/internal/tagmanagement/service/board/semantic_board_compose_test.go |
| dismissed 冷却期内拦截 | backend-go/internal/tagmanagement/service/board/semantic_board_compose_test.go |
| 确认创建组合标签 | backend-go/internal/tagmanagement/service/board/semantic_board_upgrade_test.go |
| 确认扩充方向的组合建议 | backend-go/internal/tagmanagement/service/board/semantic_board_upgrade_test.go |
| 创建失败回滚 | backend-go/internal/tagmanagement/service/board/semantic_board_upgrade_test.go |
| 挂载失败回滚 | backend-go/internal/tagmanagement/service/board/semantic_board_upgrade_test.go |
| 确认时命中既有组合去重 | backend-go/internal/tagmanagement/service/board/semantic_board_upgrade_test.go |
| compose 建议卡片 | front/app/features/tags/components/UpgradeSuggestionPanel.gen.test.ts |
| 扩充组合建议展示目标版块 | front/app/features/tags/components/UpgradeSuggestionPanel.gen.test.ts |
| 决策过滤包含组合 | front/app/features/tags/components/UpgradeSuggestionPanel.gen.test.ts |
| 单例簇不产建议 | backend-go/internal/tagmanagement/service/board/semantic_board_upgrade_test.go |
| 存量 watch 建议被清理 | backend-go/internal/platform/database/watch_suggestion_cleanup_migration_test.go |
| 存量高置信合并建议保留可确认 | backend-go/internal/tagmanagement/service/board/semantic_board_upgrade_test.go |
| 定时任务跑创建方向 | backend-go/internal/admin/scheduler/job_board_upgrade_suggest_test.go |
| 扩充不自动执行 | backend-go/internal/admin/scheduler/job_board_upgrade_suggest_test.go |
| 选择创建版块方向 | front/app/features/tags/components/UpgradeSuggestionPanel.gen.test.ts |
| 扩充方向必须选定版块 | front/app/features/tags/components/UpgradeSuggestionPanel.gen.test.ts |
| 旧内存探索区移除 | front/app/features/tags/components/UpgradeSuggestionPanel.gen.test.ts |
| 锁定版块生成 | backend-go/internal/tagmanagement/service/board/semantic_board_expand_test.go |
| 目标版块被禁用后确认失败 | backend-go/internal/tagmanagement/service/board/semantic_board_upgrade_test.go |
| 相似标签被召回 | backend-go/internal/tagmanagement/service/board/semantic_board_expand_test.go |
| 共现标签被召回 | backend-go/internal/tagmanagement/service/board/semantic_board_expand_test.go |
| 已挂载标签不召回 | backend-go/internal/tagmanagement/service/board/semantic_board_expand_test.go |
| 无关标签不被召回 | backend-go/internal/tagmanagement/service/board/semantic_board_expand_test.go |
| 组合路候选相关性过滤 | backend-go/internal/tagmanagement/service/board/semantic_board_expand_test.go |
| 召回为空 | backend-go/internal/tagmanagement/service/board/semantic_board_expand_test.go |
| 版块画像进 prompt | backend-go/internal/tagmanagement/service/board/semantic_board_expand_test.go |
| 二分类裁决 | backend-go/internal/tagmanagement/service/board/semantic_board_expand_test.go |
