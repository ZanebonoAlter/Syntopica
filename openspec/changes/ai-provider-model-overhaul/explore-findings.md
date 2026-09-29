
## airouter thinking 纯函数与 vendor Store 层落地事实（任务 2.3/2.4 衔接）

ai-provider-model-overhaul 任务 1.3+2.1 已落地（未 commit，待主线程收口）：

1. thinking.go 纯函数：thinkingToWire(dialect, level, supportsXhighMax) map[string]any（nil=不发思考参数）、clampLevel、effectiveThinkingLevel(req,binding,model *string)、DefaultThinkingLevel=medium、ValidThinkingLevels。三函数带临时 //nolint:unused（消费方是 2.3 client payload 构造与 2.4 Router.Chat 适配），2.4 接入后必须删除 nolint。
2. store_vendor.go 新方法全挂 *Store：LoadRouteWithModels（过滤 link.Enabled && model.Enabled && status=ok && vendor.Enabled，返回 []RouteModelBinding{Model,Vendor,ThinkingLevel}，空→ErrNoProviders）、UpsertRouteModels（校验在事务外：ErrModelNotFound→stale/未启用拒「模型「%s」当前不可绑定（stale 或未启用）」→类型错「embedding 路由不能挂 llm 模型「%s」」→embedding 覆盖拒→thinking_level 非法+七档列表；写事务全删全建，空覆盖档存 NULL）、ResolvePrimaryModel、Vendor CRUD（Create/UpdateVendor 带 clearAPIKey 沿用语义、DeleteVendor 返回(deletedModels, detachedBindings) 计数）、UpsertModel（云端 vendor 拒 embedding，判定 hostIsLocal）、RefreshVendorModels(ctx, vendorID, fetch 闭包)→RefreshResult{Added,Stale,Visible}、包级函数 TestVendorConnection(ctx, vendor)。
3. models/ai_models.go 本任务加了两个纯 GORM 关联字段（A1 交付缺）：AIRoute.RouteModels []AIRouteModel（foreignKey:RouteID）、AIModel.Vendor AIVendor（foreignKey:VendorID）——nested preload（RouteModels.Model.Vendor）依赖它们；创建/更新时保持零值 GORM 不写关联。
4. DeleteVendor 实现要点：modelIDs 用 Pluck 取出，modelCount=len(modelIDs)（曾漏赋值导致计数 0，已修）；绑定计数/删除用 model_id IN (?)。
5. 测试分层现状：store_vendor_test.go 用 setupAIRouterTestDB（SQLite 内存，AutoMigrate 已加 AIVendor/AIModel/AIRouteModel）；UpsertRouteModels 的校验用例在 store_model_binding_test.go（testutil.SetupTestDB testcontainer PG，testing.Short 跳过）。W1 矩阵 28 格在 thinking_test.go。

**引用**：backend-go/internal/platform/airouter/thinking.go、backend-go/internal/platform/airouter/store_vendor.go、backend-go/internal/platform/airouter/store_model_binding_test.go

<!-- pinned 2026-09-28T13:06:11Z -->

## 3.1-3.3 残留点全图与端点实现要点

ai-provider-model-overhaul 任务 3.1+3.2+3.3 落地前的代码事实全图（已核实）：

**旧方法调用方（删 ResolvePrimaryProvider/ListProviders/UpsertProvider/LoadRouteWithProviders/UpsertRoute/旧 TestConnection 前必须切换）**：
1. internal/admin/handler/ai_handler.go GetSettings(283) + ListProviders/UpsertProvider/UpdateProvider/DeleteProvider/TestConnection/ListRoutes/UpdateRoute 全部重写；wire.go 36-44 re-export 同步改。
2. internal/discovery/service/discovery_run_service.go:758 requireCapabilities → ResolvePrimaryModel（Router 需新增 ResolvePrimaryModel 委托 store）；discovery_run_service_test.go newMockAIRouter 63-81 双链旧 provider 直挂删除 + 243-248 embRoute Strategy 块删除。
3. internal/reader/service/content_completion_service.go:370 hasRouteConfig → ResolvePrimaryModel（判 vendor.BaseURL+model.Name 非空）。
4. internal/reader/handler/content_completion_handler.go:34,44 loadCompletionAISettings → ResolvePrimaryModel + 兜底 ListModels（enabled 模型+enabled vendor）；其测试 TestCompleteArticleContentReloadsProviderWithoutSummaryRouteOrAPIKey 的 AIProvider fixture 需迁 vendor+model（该测试当前绿，不能弄红）。
5. admin/repository/repository.go ListProviders/GetProviderByID/UpsertProvider/DeleteProvider/ListRoutes/GetRouteByID/UpsertRoute 无任何调用方（grep 证实）→ 不在 brief 辖区，保留不动（结构体保留所以可编译）。

**AIRoute 删 Strategy/Priority/RouteProviders 字段的编译波及（go vet 编译测试文件，必须机械清字段引用）**：aihealth_test.go:72、ai_health_reprobe_test.go:36、analysis_pause_resume_probe_test.go:56、router_test.go:97、embed_cache_test.go:24、metadata_test.go:49(tagmanagement)、content_completion_form_test.go:31、content_completion_service_test.go:53、database/ai_provider_model_kind_test.go:50-51（同带 Priority: 0）、store_test.go（整体重写）、store_model_kind_test.go（整体删除，已被 store_model_binding_test.go 取代）、ai_handler_test.go（整体重写）。

**既有红绿基线（改动前）**：reader/service 红（2.4 Router 切链遗留，fixture 无 model 链——非本任务修复对象）；reader/handler 绿；tagmanagement/service/core 红 1 例（TestTagArticleStoresOnlyTopKeywordTags tag count 4≠3，与 provider 链无关）；topicgraph/handler 绿；aihealth/airouter/admin-handler 绿（brief 声明前置全绿）。

**关键实现点**：
- 观测事件分类：组内全败→type=fallback from/to=nil code=最后行 error_code（计 failures 不计 degradations）；有成功行且前面有失败行→若最后失败行与首成功行 request_meta 均含 structured_stage 且不同→type=structured code="X→Y"（T1 不产生 fallback 事件，W10-1 degradations=T1+T2=2）；否则 type=fallback from=最后失败行序号 to=成功行序号。事件时间取收尾行 created_at。
- store_vendor.go UpsertRouteModels OnConflict DoUpdates 只有 enabled/description/updated_at——PUT /ai/routes 的 max_concurrency 不会落库，需补进 AssignmentColumns（一行，汇报项）。
- gin 版本需确认 ≥1.7 才支持 /vendors/test 与 /vendors/:id 同段静/参兄弟路由；handler 测试走 gin.CreateTestContext 不注册路由，需另行验证 RegisterRoutes 不 panic。
- ErrRouteNotFound 的 LoadRouteWithModels 用例已存在于 store_vendor_test.go:492；旧 store_test.go 的 priority 排序语义需重写为 LoadRouteWithModels 版。
- TestVendorConnection(ctx, vendor models.AIVendor) []string 在 store_vendor.go，handler 直接用；test_connection.go 整文件删（TestResult 无其他消费方）。
- handler 测试 setup 用 glebarez/sqlite 内存库 + repository.InitRepository；observability 测试用 testutil.SetupTestDB（PG testcontainer，testing.Short 跳过）。
- AICallLog.RequestMeta 是 string（encodeMeta JSON 串）；解析 structured_stage 用 json.Unmarshal map 忽略错误。

**引用**：backend-go/internal/platform/airouter/store.go、backend-go/internal/platform/airouter/store_vendor.go、backend-go/internal/admin/handler/ai_handler.go、backend-go/internal/discovery/service/discovery_run_service.go、backend-go/internal/reader/handler/content_completion_handler.go

<!-- pinned 2026-09-28T14:19:55Z -->

## reader/tagmanagement fixture 迁移完成与 metadata_test 根因修正

3.3 收口补丁（本会话，未 commit）：reader/tagmanagement 测试 fixture 已全部迁到 vendor+model 链，-short 与 lint 全绿。重要修正：此前 explore 发现称「tagmanagement core 红 1 例 tag count 4≠3 与 provider 链无关」——判断错误。实测（PG testcontainer 全模式）根因正是旧链 fixture：Router 走 LoadRouteWithModels 查不到绑定 → ErrNoProviders → extractor_enhanced 按 err!=nil 降级 heuristic 提取 → 产出 4 标签 ≠ AI 路径 mock 的 top-3。metadata_test.go fixture 迁移后全模式转绿。测试侧参照模式：seedRouteModelChain（discovery）/seedChatBinding（airouter）/本会话新增 seedSummaryRouteFixture（reader/service，AutoMigrate 已补 AIVendor/AIModel/AIRouteModel，AIProvider/AIRouteProvider 表保留但 fixture 已零使用）。遗留观察项：tagmanagement/service/merge 在多包并行 -short 首跑出现过一次失败、单独与后续两次全树重跑均绿，疑似偶发（环境/并行干扰），非 AI fixture 类，未处理。

引用：backend-go/internal/reader/service/content_completion_service_test.go、backend-go/internal/tagmanagement/service/core/metadata_test.go

**引用**：backend-go/internal/reader/service/content_completion_service_test.go、backend-go/internal/reader/service/content_completion_form_test.go、backend-go/internal/tagmanagement/service/core/metadata_test.go

<!-- pinned 2026-09-28T15:02:49Z -->

## 4.1 前端数据层落地事实与桥接契约（4.2-4.5 衔接）

任务 4.1 已落地（未 commit，待主线程收口），lint/typecheck/受影响测试全绿：

**新数据层（4.2-4.5 直接用）**：
1. front/app/types/ai.ts 重写：AIVendor/AIVendorUpsertRequest/AIVendorCreateResult/AIModel/AIRouteModelBinding/AIRoute/RouteSaveRequest/AIModelUpdateRequest/VendorModelsRefreshResult/VendorDeleteResult + 观测三型（DegradationEvent/Stats/Report）；AIHealthRoute 已是 primary_model+vendor。字段以 ai_handler.go 为准。
2. front/app/api/aiAdmin.ts 重写：listVendors/createVendor/updateVendor/deleteVendor/refreshVendorModels/testVendor/testVendorInline/listModels/updateModel/listRoutes/saveRoute(capability,data)/getDegradations(window,capability)/getHealth/reprobeHealth/setAutoStartModels/getSettings/saveSettings。测试连接响应 message 带「共 N 个可用模型」；删除响应 data 含级联计数。
3. 新三件套 composables（features/ai/composables/）：useAIVendors（vendors 缓存+CRUD+refresh/test，动作失败写 error 返回 null/false 不抛）、useAIModels（usableModels/usableModelsForCapability(kind 过滤)/byVendor 分组/updateModel 后 refetch）、useAIRoutes（routes+routeByCapability 索引+saveRoute saving 锁，成功后 refetch；导出 routeLabels/capabilityOrder 常量=渲染白名单）。各带单测（mock ~/api 层）。

**桥接契约（4.2-4.5 组件重写后必须删除）**：
- useAIRouterSettings.ts 已重写为临时桥（文件头 TODO(4.2-4.5) 标记）：组合新三件套，保持旧 API 形状，旧组件（AIProviderManagement/AIRouterCapabilityRoutes/AIRouterBackupProviders/SettingsSectionCapabilityRoutes）零改动 typecheck 绿。
- 桥语义：「provider 行」=vendor 投影（LegacyProviderRow，model 字段空占位）；routeSelections 存 model_id，saveRoutes 直发 bindings（真实生效）；旧「主模型同步路由」概念已死，savePrimaryProvider 仅落 vendor，primaryAllowedForCapability 恒 false（「挂主模型」按钮隐藏）；deleteBackupProvider 走 useConfirm+removeVendor（确认文案带 models_count，成功提示带级联计数）。
- 消费方注意：SettingsSectionCapabilityRoutes 用 ctx.loading.value（须返回 ref 非 reactive）；两个注入组件用本地 AIRouterCtx 接口（any[] 字段），与新类型解耦。

**已做的最小引用点适配（UI 不动）**：SettingsSectionAiHealth.vue 单元格 primary_provider→primary_model（表头文案仍「主 provider」，4.5 重写时处理）+ 测试 fixture 同步；useAI.ts testConnection 内部改走 testVendorInline（不再传 model，导出签名未动）。

**遗留**：types/scheduler.ts SchedulerAIHealthRoute.primary_provider 与 /schedulers/status 现响应（primary_model+vendor）漂移，scheduler 域类型不在本任务辖区；桥 shim 无独立单测（临时件，语义由三件套测试+旧组件测试间接覆盖）。

<!-- pinned 2026-09-28T15:35:25Z -->

## 4.2/4.5 后端契约口径与前端落地事实

4.2/4.5 实现依赖的后端契约与工程事实（已核实源码）：

1. isLocal 判定 = 后端 hostIsLocal(base_url)（store_vendor.go:551）：localhost（不分大小写）/loopback（127/8、::1）/RFC1918（10/8、172.16-31、192.168/16）为本地；空串/裸域名/解析失败=云端（收紧方向）。前端 VendorCard 的 embedding 选项开关必须镜像此口径（云端 vendor 配 embedding 会被 UpsertModel 拒绝，报错「云端 vendor 不能配置 embedding 模型」）。
2. UpdateAIModel（ai_handler.go:425）：ThinkingLevel *string —— JSON null = 指针 nil = 不更新！清思考档必须发空串 ""（UpsertModel 归一为 NULL）。enabled/model_kind/supports_xhigh_max 同为「非 nil 才更新」。
3. UpdateVendor 清除标志优先级：clear_api_key=true → 置空（压过 api_key 字段）；false 且 api_key 空串 → 沿用已存值。start_command 同理：clear_start_command=true 优先；空串=沿用；非空=覆盖。编辑表单：key/命令原文不回显，清除必须显式开关。
4. POST /ai/vendors 创建后自动 RefreshVendorModels，失败不回滚：响应 {id, models_refreshed, refresh_error, added}。
5. TestVendorConnection 对所有 provider_type 统一 TrimRight(base_url,"/")+"/models" → Ollama 预设 baseUrl 必须带 /v1（http://localhost:11434/v1）。
6. backend normalizeVendor 默认：provider_type 空→openai_compatible，timeout<=0→120，dialect 空→none，so 空→json_schema。自定义预设默认值取 none/json_schema 与后端一致。
7. DeleteVendor 响应 {deleted_models, detached_bindings}；单事务解绑→删模型→删 vendor，不因引用拒绝。
8. 工程事实：front/vitest.config.ts include 只有 app/**/*.test.ts —— brief 说的 presets.spec.ts 必须落为 presets.test.ts 才会被收集。组件 ui（AppDialog/AppButton/AppInput/AppToggle/AppSectionHeader）在 Nuxt 走 auto-import，但新组件显式 import（同 AppConfirmDialog.vue 先例）可在 vitest 直测。useAIVendors() 每次 call 新建 refs（非 useState），leaf 组件（VendorCard 测试连接）可安全自建实例做无缓存动作；列表数据必须由父层单实例下发 props。
9. AIProviderManagement 重写后不再 provide 'ai-router-ctx'；该 key 的剩余消费方 AIRouterCapabilityRoutes/AIRouterBackupProviders 由 SettingsSectionCapabilityRoutes 提供 ctx（4.3 辖区），AIRouterBackupProviders 将成孤儿组件（测试仍绿，留给 4.3 处置）。

**引用**：backend-go/internal/platform/airouter/store_vendor.go:hostIsLocal、backend-go/internal/admin/handler/ai_handler.go:UpdateAIModel、front/vitest.config.ts

<!-- pinned 2026-09-28T15:49:50Z -->

## H1 三个图标名不存在于 MDI，替换映射经操作者确认

H1 图标子集再生时发现：brief 要求新增的六个图标中有三个在 MDI 图标集中不存在（live api.iconify.design 返回 not_found，本地 @iconify-icons/mdi@1.2.48 与最新版均无）：mdi:whale-variant（DeepSeek 预设）、mdi:route（OpenRouter 预设）、mdi:database-network-outline（设置侧栏「数据源与网络」）——MDI 连 mdi:whale 都没有。子集一致性测试要求「源码引用 ⊆ 子集」，这三个名字永远无法进子集，不改引用名则测试无法转绿。经操作者确认的替换映射：DeepSeek→mdi:fish（保留深海意象）、OpenRouter→mdi:routes、「数据源与网络」→mdi:lan-connect（存量已在用）。presets.test.ts 只断言 /^mdi:/ 前缀，change 制品未指定具体图标名，替换不受制品约束。涉及文件：front/app/features/ai/presets.ts、front/app/features/settings/components/SettingsWorkspace.vue + SettingsSidebar.test.ts、再生后的 app/assets/iconify-subset.json（185 图标，含 alpha-z-box-outline/server/monitor-dashboard）。

**引用**：front/app/features/ai/presets.ts、front/app/assets/iconify-subset.json、front/scripts/generate-icon-subset.mjs

<!-- pinned 2026-09-29T01:40:01Z -->
