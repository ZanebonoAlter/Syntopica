
## provider 删除死锁链路与修复点

Bug：provider 挂线路即无法删除（死锁）。链路：①后端 DeleteProvider（backend-go/internal/admin/handler/ai_handler.go:161，路由 DELETE /providers/:provider_id 在 internal/admin/routes.go:14）见 AIRouteProvider 关联即 409 "provider is still used by one or more AI routes"；②前端 deleteBackupProvider（front/app/features/ai/composables/useAIRouterSettings.ts:393）顺序反了——先调删除接口被 409 拦下抛错，后面的 removeProviderFromRoute(:199) 循环永远执行不到，且该函数只改本地 routeSelections ref 不调 API；③手动解绑两条路也堵死：saveRoutes(:415) 对空线路 continue 跳过、UpdateRoute handler（ai_handler.go:254）对空 provider_ids 400（而 store 层 UpsertRoute (internal/platform/airouter/store.go:152) 本身支持空列表清空绑定，注释 "Empty providerIDs clears bindings — no check"）。

修复（用户已确认）：删除时级联解绑 + 允许线路为空；不改 UpdateRoute 语义。测试现状：ai_handler_test.go:31 TestDeleteProviderBlocksLinkedProvider 固化了旧 409 行为，须改写为级联解绑复现测试（sqlite 内存库 setupAIAdminTestDB 已具备）；:51 TestDeleteProviderRemovesUnusedProvider 保留。前端 deleteBackupProvider 成功后 loadData()→hydrateRouteSelections()(:125) 会用后端真值重建 routeSelections，故删除死代码循环即可。注意：工作区有他人会话 dataenrichment 中间态改动（wire.go lint 红，非本 change 归属）。

**引用**：backend-go/internal/admin/handler/ai_handler.go:DeleteProvider、front/app/features/ai/composables/useAIRouterSettings.ts:deleteBackupProvider、backend-go/internal/platform/airouter/store.go:UpsertRoute、backend-go/internal/admin/handler/ai_handler_test.go:TestDeleteProviderBlocksLinkedProvider

<!-- pinned 2026-09-22T14:01:12Z -->

## AI provider 删除按钮 disabled 漏洞 + useConfirm 全局确认弹窗方案

用户验收反馈补漏（2026-09-23）：原 change 漏看 AIRouterBackupProviders.vue 两个 UI 元素——删除按钮 `:disabled="ctx.isProviderLinked(provider.id)"`（105 行）和提示行「还挂在某条路由上，先移除再删」（111 行）。后端 DeleteProvider 已是级联解绑（ai_handler.go:162，单事务删 ai_route_providers 再删 provider，无 409），但按钮 disabled 让挂线路 provider 在 UI 上永远点不到删除。已确认方案：① 移除 disabled，提示行改告知文案「挂在线路上，删除将自动从所有线路解绑」；② 新增全局 promise 式确认弹窗：front/app/composables/useConfirm.ts（confirm(options):Promise<boolean>，useState 存 state、模块级 Map 存 resolve）+ front/app/components/ui/AppConfirmDialog.vue（复用 AppDialog size=sm、closeOnOverlay=false、showClose=false，Escape=取消，footer 用 AppButton cancel=secondary/confirm=danger?danger:primary），挂载 app.vue（与 NotifyContainer 同模式）。本 change 只接 deleteBackupProvider 一处；存量 13+ 处原生 confirm() 另立 change。deleteBackupProvider 现用 confirm() 在 useAIRouterSettings.ts:399，测试 mock 在 useAIRouterSettings.test.ts:180（vi.stubGlobal('confirm')），需改为 mock useConfirm。项目 ssr:false，AppDialog 测试模式见 AppDialog.test.ts（@vue/test-utils attachTo document.body）。

<!-- pinned 2026-09-23T02:38:12Z -->
