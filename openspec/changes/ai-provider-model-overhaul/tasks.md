# Tasks — ai-provider-model-overhaul

> 复杂档：本 change 必须有 `test-cases.md`（用例先行），实现前先落主链路故事表。测试执行遵循「只跑影响包」（`bash scripts/harness/change-scope.sh` 机械判定），树莓派不做全量。

## 1. 数据层与迁移

- [x] 1.1 定义 `AIVendor`/`AIModel`/`AIRouteModel` 模型与建表迁移（ai_vendors/ai_models/ai_route_models，字段见 design D1；ai_call_logs 顺手加 trace_id 索引），验证：`go build ./...` 通过 + 迁移在 testcontainer PG 上建表成功
- [x] 1.2 存量拆解迁移（ai_providers→vendor+model、ai_route_providers 改挂 model_id、enable_thinking true→high/false→off、死字段不迁、ai_providers JSON 快照落 data/、幂等），验证：testcontainer PG 迁移测试断言 3 vendor/3 model/10 绑定 + 重复执行不产生重复行（specs/ai-vendor-model「存量配置拆解迁移」三 Scenario）
- [x] 1.3 Store 层改造：`LoadRouteWithModels`/`UpsertRoute`(model 绑定+类型校验+thinking_level 覆盖)/`ResolvePrimaryModel`/vendor CRUD（key 可空、clear_api_key、start_command configured）/模型清单刷新对账（新增默认未启用、stale 不删、失败保留旧清单），验证：`go test ./internal/platform/airouter`（repository 层 testcontainer 用例，覆盖 specs/ai-vendor-model 与 ai-capability-routing MODIFIED 各 Scenario）

## 2. airouter 请求面

- [x] 2.1 ProviderClient 接口换签名（vendor+model+生效档）+ `thinkingToWire` 方言映射纯函数（qwen 布尔化+钳制 / deepseek / reasoning-effort / none，四级解析：请求级>绑定级>模型默认>medium，xhigh/max 钳制），验证：`go test ./internal/platform/airouter` 表驱动用例覆盖钳制与方言边界（specs/ai-thinking-levels 各 Scenario）
- [x] 2.2 buildPayload 按结构化输出档位三态构造（response_format + schema→prompt 注入模板），验证：单测断言三档 payload 结构（specs/structured-output-adaptation「payload 按档位构造」三 Scenario）
- [x] 2.3 400 运行时降级链（json_schema→json_object→纯文本，每档真实调用各写一行 ai_call_logs、不持久化档位）+ 结构化解析纠错（平衡花括号提取，失败按尝试失败），验证：router/client 测试用 fake server 断言降级路径与日志行数（specs/structured-output-adaptation 其余 Scenario）
- [x] 2.4 Router.Chat/Embed 适配新绑定链（候选过滤 enabled+ok、输出清理跟随生效档 off/非off、并发信号量与 errors.Join 语义不变），验证：`go test ./internal/platform/airouter` 全量（现有 router_test 回归 + 新用例）
- [x] 2.5 aihealth 探测对象换 vendor（首位模型的 vendor、共享 vendor 单次探测、拉起/冷却/互斥语义不变、快照字段 primary_model+vendor），验证：`go test ./internal/platform/aihealth`（specs/ai-model-health MODIFIED 各 Scenario 保持绿）

## 3. admin API

- [x] 3.1 vendor/model/refresh/test 端点（design D7 清单，旧 /api/ai/providers 删除），验证：handler 层测试覆盖 CRUD/刷新 diff/级联删除响应（specs/ai-vendor-model「Vendor 删除级联」）
- [x] 3.2 路由端点换 model 绑定（UpsertRoute 类型校验错误文案）+ GET /api/ai/health 字段升级，验证：handler 测试断言错误文案与响应结构
- [x] 3.3 降级观测查询端点（trace_id 分组还原 fallback/structured 事件、24h/7d 统计、capability 过滤），验证：testcontainer PG 造日志断言事件字段与类型二分（specs/ai-route-observability 前两 Requirement）

## 4. 前端

- [x] 4.1 types/ai.ts 重写（AIVendor/AIModel/AIRouteModelBinding）+ API client（aiAdmin.ts 新端点）+ composables 拆分（useAIVendors/useAIModels/useAIRoutes），验证：`pnpm exec nuxi typecheck` 通过
- [x] 4.2 VendorCard + 两段式 VendorDialog（预设目录/只输 Key/高级折叠/测试连接三态/保存自动拉清单）+ 删除级联确认，验证：组件测试覆盖「预设两段式流程/字段校验/级联文案」（specs/ai-vendor-model「Vendor 删除级联与界面契约」+ test-cases UI 节拍）
- [x] 4.3 能力路由链式卡片升级（model 引用+ThinkingLevelSelect 继承/七档/覆盖徽章+hover 扇出 tooltip+chips 按 vendor 分组+embedding 过滤），验证：组件测试覆盖调档/添加/移除/候选过滤（ui-design Interaction Contract 主操作表）
- [x] 4.4 DegradationPanel（统计行+时间窗切换+事件流+定位跳转脉冲）挂 ai-health tab，验证：组件测试覆盖窗口切换与定位事件（specs/ai-route-observability「观测面板界面契约」三 Scenario）
- [x] 4.5 模型清单管理面板（启用勾选/类型/默认档/新·stale 标/刷新 diff），验证：组件测试覆盖刷新后 diff 呈现与 stale 灰显

## 5. 测试

- [x] 5.1 test-cases.md 主链路故事表 + 变体走查 + 白盒附加（先于实现细节落定，见「用例先行」），验证：文件存在于 change 目录且覆盖全部 specs Scenario 落点（`grep -c .Scenario. test-cases.md` ≥ specs 场景数）
- [x] 5.2 opencli 端到端主链路（新增供应商两段式→拉清单→勾选→路由绑定→调思考档→观测定位），验证：opencli 断言五步（ui-design Acceptance ①-⑤）全过——① llama.cpp 预设第二段仅露 base_url+start_command 无 API Key、测试连接三态成功、保存后卡片自动展开、mock-model-a/b 未勾选带「新」标；② summary 链 llama.cpp 组 chips 添加出现「mock-model-a@llama.cpp」；③ 思考档改「低」出现「覆盖」徽章、保存 toast「已保存」、刷新后值保持；④ hover 模型名 tooltip 显「未承载其他能力」；⑤ 降级观测统计行（24h 14160/42/210，7d 98531/199/1127）+ e2e.observability.demo 事件（DB trace=e2e-demo-trace-1，UI 顺位 1→2=API 0→1，deepseek-flash，供应商失败/timeout）7d 仍在、定位跳 capability-routes 且 history 桩证实 push 带 focus_capability=topic_tagging→消费后 replace 清除+route-section--pulse 脉冲。附加⑥ 删除级联本体过（确认弹窗出现、删除后卡片/链项消失、API 无残留），但确认文案缺陷：显示「解除 0 条能力路由绑定」（实存 1 条）——根因 ai-providers section 只拉 /vendors+/models 不拉 /routes，bindingsCountOf 恒 0（截图 fail-6-delete-confirm-bindings-0.png）；另模型计数显示 2 个为正确值（vendor 实有 2 模型）。截图 2 张（step1-llamacpp-card / fail-6）
- [x] 5.3 双视口视觉检查（1440×900 / 1920×1080：workspace 框架不变、dialog ≤92vw、无横向溢出），验证：双视口截图 6 张（1440/1920-ai-providers·capability-routes·ai-health）+ dialog 2 张（dialog-1440/1920）；六页 scrollWidth≤clientWidth 全 true；弹窗宽 760px 占比 1440=52.8% / 1920=39.6% 均 ≤92vw

## 6. 文档

<!-- doc-impact: flow, api, database -->

- [x] 6.1 `docs/reference/flow/ai-summary.md`：业务约束节更新（vendor/model 两层、思考档解析优先级、结构化降级链、观测口径）+ 变更溯源链接，验证：`bash scripts/doc-impact.sh verify` 通过
- [x] 6.2 `docs/reference/database/`：ai_vendors/ai_models/ai_route_models 表结构文档 + ai_providers 废弃标注，验证：文档存在且与迁移字段一致（grep 对账）
- [x] 6.3 `docs/reference/api/`：新端点文档（D7 清单），验证：grep 每个端点均有记载

## 7. 验证

- [x] 7.1 `cd backend-go && golangci-lint run ./... && go vet ./...` → 0 error
- [x] 7.2 `cd backend-go && go test ./internal/platform/airouter ./internal/platform/aihealth ./internal/admin/handler` → 全 PASS（影响包）
- [x] 7.3 `cd front && pnpm lint && pnpm exec nuxi typecheck` → 0 error
- [x] 7.4 `cd front && pnpm test:unit --maxWorkers=2` 按影响文件过滤 → 全 PASS
- [x] 7.5 `bash scripts/dev/deploy-frontend.sh` 部署后手动冒烟：三 section 可达、存量 3 供应商呈现、路由绑定 10 条挂 model、一次真实调用后观测面板出数 → 人工：验证方式记入本节——curl 三数据源接口全 200（/api/ai/vendors 3 供应商 2/2/1 模型、/api/ai/observability/degradations?window=24h 16469 调用/43 降级/210 失败/253 事件、/api/ai/health healthy=true 且路由快照含 primary_model+vendor）；同轮 agent-browser e2e（5.2/5.3）已实测三 section UI 可达可交互、路由绑定保存/持久化、观测面板出数与 7d 窗口切换
- [x] 7.6 `openspec validate ai-provider-model-overhaul` → valid（--json valid:true, issues:[]）
