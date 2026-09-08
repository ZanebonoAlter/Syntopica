
## 升级建议生成主链路现状（后端）

主链路：`backend-go/internal/tagmanagement/service/board/semantic_board_upgrade.go`
- `GenerateSuggestions(ctx, mode)` (L166)：mode ∈ {discover_new(默认), expand_existing}，两 mode 的 LLM prompt 几乎相同（`buildSemanticBoardUpgradePrompt` L1007），决策空间都是 {create_new|merge_into_existing|skip}（`BuildSemanticBoardUpgradeSystemPrompt` L1003）。
- 簇分区（L212-230）：单例簇→`synthesizeWatchSuggestion`(watch,无LLM)；`highConfidenceMergeBoard`(shortlist top1-top2 双签名 margin≥MergeConfidenceMargin 默认0.05)→`synthesizeHighConfidenceMerge`(无LLM merge)；其余进 LLM。
- merge 兜底链：`validateMergeTargets`(L1205，target 不在 shortlist 时保留+evidence 标 target_off_shortlist——注释承认实测 17 条 LLM merge 全被 shortlist 拒)；缺 target_board_id 的 merge 降级 create_new（GenerateSuggestions 内 L239-244）。
- 决策枚举（L121-127）：create_new/merge_into_existing/skip/watch + compose（compose 枚举在 compose 文件）。
- `ConfirmSuggestion`(L328)：事务内按 decision 分支——create_new 建版块、merge 挂 board_composition + MarkConfirmed、compose 建组合标签（失败回滚建议保持 pending）。
- 配置加载 `LoadUpgradeConfig`(L631)：Margin 默认 0.05，配置键 `semantic_board_upgrade_*`。

compose 段：`semantic_board_compose.go`
- `generateComposeSuggestions`(upgrade.go L265)：`collectComposeCandidates`(L44) 收 co-tag 共现对/三元组（同文章内共现，窗口 CoTagWindowDays，阈值 composite_min_cooccurrence 默认10，组件 ref 达升级阈值）→ `buildComposeCandidatesPrompt`(L229，决策空间 {compose|skip}) → `filterComposeSuggestions`(L253)。冷启动（候选<RefCountThreshold）只跑 compose 段。compose 在两种 mode 下都跑。
- 三元组吸收二元组：`pairAbsorbed`(L189)。

持久化包装：`board_upgrade_suggestion_persist.go` L20-25（GenerateAndPersistSuggestions 包 GenerateSuggestions）；suggestion_hash 幂等 + dismiss 冷却期在 repository 层。

handler：`board_upgrade_handler.go` L60-84（mode query 参数透传）。

scheduler job：`backend-go/internal/admin/scheduler/job_board_upgrade_suggest.go`（注册见 runtime.go/wire.go）。

GC：watch 观察池 30 天回收 GCOldWatch（semantic_board_upgrade_gc.go 或同名文件，`semantic_board_upgrade_watch_gc_days`）。

本次退役面：watch 合成+GC、highConfidenceMergeBoard/synthesizeHighConfidenceMerge、validateMergeTargets/target_off_shortlist、缺target降级、mode=discover_new/expand_existing 参数语义。

<!-- pinned 2026-09-05T03:01:48Z -->

## 升级建议面板前端结构现状

`front/app/features/tags/components/UpgradeSuggestionPanel.vue`（1218 行）双区结构：
- 上半区（旧内存探索，本次退役）：props candidates/clusters/suggestions + upgradeMode radio（discover_new/expand_existing，L32/L451-460）+「获取 LLM 建议」按钮 emit('suggest', upgradeMode)（L466/L481）。
- 下半区（持久化，保留改造）：`<section class="usp-persisted">` L249 起，filter tabs（L45-51：all/merge_into_existing/create_new/watch/compose）+「生成建议」按钮 emit('generate')；行卡片按 decision 着色（decisionStyle L220）；merge 行「合并到...」下拉（L347-400，候选 shortlist 置顶+全量可搜，emit confirmRow decision=merge_into_existing）；create_new 行「创建新版块」+也可「合并到...」（L402-410）；compose 行确认（L192-193，emit decision=compose 带勾选组件子集）；勾选 aux 子集 per-row（selectedAuxIds L128）。
- 事件接口（L23-27）：suggest:[mode]/generate:[]/loadPersisted:[decision]/confirmRow/dismissRow。

接线：`TagsPage.vue` L48-70（composable 解构 upgradeCandidates/upgradeClusters/upgradeSuggestions/upgradeSuggesting 等旧链路 + handleUpgradeSuggest/handleGenerateUpgradeSuggestions/loadPersistedSuggestions/handleConfirmUpgradeRow）L160 upgrade-suggest、L333-343 UpgradeSuggestionPanel props/events。composable 在 front/app/features/tags/composables/（useSemanticBoardUpgrade 类）。API 层 front/app/api/semanticBoards.ts（suggest/persisted/confirm 调用）。

本次 UI：删上半区及旧 props/事件/链路；持久化区顶部改为两步模式选择（方向：创建版块/版块扩充 × 来源：单标签/组合标签）+ 扩充时版块单选下拉；filter tabs 去 watch；merge 行目标改锁定版块（去掉全量下拉挑选，target 固定）。

**引用**：front/app/features/tags/components/UpgradeSuggestionPanel.vue、front/app/features/tags/components/TagsPage.vue、front/app/api/semanticBoards.ts

<!-- pinned 2026-09-05T03:01:48Z -->

## 弹窗坠底事故归因与浮层展示锚补修

【事故】实现期重写 <style scoped> 误删 .usp-overlay 定位规则（fixed/inset:0/z-index:100），模板 Teleport to="body" 与 class 都在，弹窗退化为流内 div 坠到页面底部。lint/typecheck/build、组件单测（断言全是 data-testid 存在性）、opencli 功能断言全部天然失明——CSS 不参与静态检查与 happy-dom 渲染（且 vitest 未启用 css:true，getComputedStyle 在测试环境恒空串=假绿）。tasks 7.2 勾了"双视口截图留证"但截图文件不存在、事件库无视觉验证子代理派发记录——验收空转无人拦截。

【修复】UpgradeSuggestionPanel.vue <style scoped> 开头恢复 .usp-overlay 块（git HEAD 614 行原文）。

【锚】gen.test.ts 末尾新增"弹窗浮层展示锚"describe：①样式规则锚 readFileSync(`${import.meta.dirname}/UpgradeSuggestionPanel.vue`) 正则断言 .usp-overlay 含 position:fixed/inset:0/z-index（注意 new URL(import.meta.url) 在 Nuxt vitest 下报 "The URL must be of scheme file"，必须用 import.meta.dirname）；②Teleport 挂载锚：不 stubs.teleport 挂载，断言 overlay 在 document.body 且不在组件原地，unmount 后清空。11/11 绿。

【流程补丁】①开发执行规范 §5.3 新增"前端验收四维度"：功能正确性/展示合理性/制品完整性/证据存在性——minor 档浮层变更至少 1 个机械锚，major 加双视口视觉子代理；截图留证必须落 openspec/changes/<name>/ui-verification/ 有实物可对账。②skill ui-verify 新增"浮层展示锚模板"节 + 踩坑案例。③layout.md 新增 Requirement"浮层组件展示合理性锚"。④本 change ui-design.md 补 Acceptance 展示合理性条目 + 差异与修复记录；tasks.md 补 6.5。

【验证证据】浏览器层（playwright + page.route mock API，无需后端）：computed position=fixed/inset=0px/z-index=100/parentIsBody=true/display=flex；双视口截图 ui-verification/tags-upgrade-panel-{1440x900,1920x1080}.png，glm-5.3-flash 视觉目检 PASS。前端三连：lint 0 errors / typecheck 过 / test:unit 858 全绿。

<!-- pinned 2026-09-07T14:21:28Z -->

## 旧建议干扰与重复组合报障修复

【报障 2026-09-07 第二批】(1) 旧结果永久干扰：旧 discover_new 管线 150 条 pending 建议（133 create_new + 8 compose + 9 merge）hash 含旧 mode 值、永不被新四格幂等命中，前端列表 status=pending+decision 过滤不分 mode，永久混排。(2) 重复组合：collectComposeCandidates 与 generateExpandCompose 均无"排除已存在组合"过滤；幂等唯一索引只挡 pending（uq_board_upgrade_suggestions_hash WHERE status='pending'），confirmed 后同内容可再插（实测 541/601 同 hash 双 confirmed，613 第三次）；用户 22:40 expand:composite 批里"美联储沃什/加息"等 09-03 已确认过的组合再次出现。

【修复】①semantic_board_compose.go 新增 filterExistingComposeCandidates(ctx, candidates, targetBoardID)：查 active composite 组件集（composite_components join semantic_labels）构造 key set（composeComponentKey 排序拼接）；create 路（target=nil）完全一致即排除；expand 路查 board_composition（board_id+auxiliary_label_id IN composites），已挂载 target 才排除、未挂载保留（挂载语义，确认时 CreateCompositeLabel L1/L2 复用）；部分重叠不排除。接入点：generateComposeSuggestions（semantic_board_upgrade.go，collect 后）+ generateExpandCompose（semantic_board_expand.go，相关性过滤后）。②迁移 20260907_0001 legacyDiscoverNewPendingDismissMigration：UPDATE pending discover_new 置 dismissed + dismiss_reason='legacy_discover_new_cleanup' 留痕（非物理删）。dev 库 22:57 已随用户重启自动应用（150 条已清）。

【测试】TestFilterExistingComposeCandidates（compose_test 尾：create 排除/expand 未挂载保留/已挂载排除/部分重叠不误伤）；TestLegacyDiscoverNewPendingDismissMigrationIdempotent（watch 迁移测试同文件，参照 ExportedPostgresMigrations 按 Version 找 Up）。board + database 两包全量 + lint + vet 全绿。

【注意】(a) expand:aux merge 建议 aux 数组多标签现象（606-609，每条 6-8 个 aux id）未处置——spec 说扩充×单标签是单候选二分类，filterExpandMergeSuggestions 保留 LLM 输出的多标签 ids，疑似打包行为，后续观察。(b) 用户本地 Windows 后端会在重启时自动跑迁移框架。(c) spec delta board-upgrade "co-tag 高频共现对产出组合标签建议"节已补防重复段 + 两 Scenario。

<!-- pinned 2026-09-07T14:59:37Z -->
