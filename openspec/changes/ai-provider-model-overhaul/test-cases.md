# test-cases — ai-provider-model-overhaul

> 用例先行（开发执行规范 §2）。本文档是全部实现子线程的**断言判据锚点**：什么算对以此为准，实现细节不改变判据。覆盖 specs 全部 84 个 Scenario（变体走查表逐条映射）。
>
> 测试基建约定：
> - 纯逻辑（纯函数/内存态）→ SQLite 内存库或表驱动单测（`-short` 可跑）
> - 涉及真实 SQL 语义（迁移/级联/对账/类型校验/观测聚合）→ `testutil.SetupTestDB(t)` testcontainer PG（`-short` 跳过，Docker 必需）
> - HTTP 层 → `httptest.Server` fake provider server
> - 前端组件 → vitest + @vue/test-utils（既有模式）

## 0. 测试落点总览

| 落点文件 | 覆盖内容 |
| --- | --- |
| `backend-go/internal/platform/database/vendor_model_migration_test.go`（新） | W6 存量拆解迁移 |
| `backend-go/internal/platform/airouter/thinking_test.go`（新） | W1 方言矩阵 / W2 解析优先级+钳制 |
| `backend-go/internal/platform/airouter/structured_payload_test.go`（新） | W3 payload 三档 |
| `backend-go/internal/platform/airouter/structured_degrade_test.go`（新） | W4 降级链 / W5 解析纠错 |
| `backend-go/internal/platform/airouter/store.go`+`store_vendor_test.go`（新） | W7 清单刷新 / W8 vendor CRUD / W9 绑定校验 |
| `backend-go/internal/platform/airouter/router_test.go`（改） | Router 新绑定链回归 |
| `backend-go/internal/platform/airouter/observability_test.go`（新） | W10 观测还原 |
| `backend-go/internal/platform/aihealth/*_test.go`（改） | W11 vendor 化 |
| `backend-go/internal/admin/handler/ai_handler_test.go`（改） | 端点契约 |
| `front/app/features/ai/**/*.test.ts`（新/改） | UI 节拍 |

## 1. 主链路故事表（opencli E2E 断言，任务 5.2）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| ① | 设置页「AI 模型」→「+ 新增供应商」→ 选 DeepSeek 预设 → 只输 API Key → 保存 | vendor-model「vendor 承载协议」「新拉取默认未启用」 | 弹窗两段式；保存后新 VendorCard 出现且自动展开模型清单（全部未勾选）；卡片头显示 base_url/「新」标模型 | e2e | opencli（ui-design Acceptance ①） |
| ② | 勾选 deepseek-chat 启用 → 「能力路由」→ summary 链 + chips 添加该模型 | routing「stale 与未启用不可绑定」反向 | 链项显示「模型@供应商」；chips 按 vendor 分组；embedding 行只列 embedding 模型 | e2e | opencli（Acceptance ②） |
| ③ | 链项思考档从「继承」改「低」→ 保存 | thinking「绑定处按场景调档」 | 链项出现「覆盖」徽章；保存成功 toast；重新加载后值保持 | e2e | opencli（Acceptance ②） |
| ④ | hover 链项模型名 | ui-design Interaction Contract | tooltip「另承载 N 个能力」出现 | e2e | opencli（Acceptance ③） |
| ⑤ | 「运行状态」→ ai-health tab → 降级观测块切 7d → 点某事件「定位」 | observability「定位跳转能力路由」 | 跳能力路由 section，对应能力行滚动进入视口并脉冲高亮一次 | e2e | opencli（Acceptance ④） |
| ⑥（附加） | 删除该供应商 | vendor-model「删除确认弹窗列明级联」 | AppConfirmDialog sm 弹窗列明「N 模型 / M 绑定」级联影响；确认后卡片消失、路由链上对应链项同步消失 | e2e | opencli（Acceptance ⑤） |

## 2. 白盒用例（分支表 / 边界值清单，断言判据主线程定死）

### W1 thinkingToWire 方言 × 档位矩阵（`thinkingToWire(dialect, level) map[string]any`）

| dialect \ 生效档 | off | minimal | low | medium | high | xhigh | max |
| --- | --- | --- | --- | --- | --- | --- | --- |
| qwen-chat-template | `chat_template_kwargs.enable_thinking=false` | 同左 key=true | true | true | true | **钳 high**→true | **钳 high**→true |
| deepseek | `thinking={"type":"disabled"}` | `{"type":"enabled"}` | enabled | enabled | enabled | 钳 high→enabled | 钳 high→enabled |
| reasoning-effort | **不发任何思考字段** | `reasoning_effort="minimal"` | `"low"` | `"medium"` | `"high"` | 钳 high | 钳 high |
| none | 不发 | 不发 | 不发 | 不发 | 不发 | 不发 | 不发 |

- 断言形态：表驱动，`thinkingToWire` 返回 `map[string]any`（nil 值键=不发）；payload 断言「包含/不包含」该键。
- 边界：**钳制发生在解析之后、wire 之前**（`clampLevel(level, supportsXhighMax)` 独立纯函数先钳后映射）；`xhigh/max` 未声明支持 → high（矩阵右两列）。
- qwen 语义保留：**显式发 false**（不发 ≠ off，沿用现有 buildPayload 注释的理由）。

### W2 四级解析优先级（`effectiveThinkingLevel(req, binding, model) string`）

| # | 请求级 | 绑定级 | 模型默认 | 期望 |
| --- | --- | --- | --- | --- |
| 1 | high | low | off | **high**（请求级压一切） |
| 2 | "" | high | medium | **high**（绑定级 > 模型默认） |
| 3 | "" | "" | off | **off**（模型默认） |
| 4 | "" | "" | ""（null） | **medium**（全局默认） |
| 5 | xhigh | low | "" | xhigh 输入→未声明支持钳 **high**；声明支持保持 xhigh |
| 6 | "" | ultra（非法） | — | 保存时已被 UpsertRoute 拒绝；运行时遇到按 medium 回落（防御） |

### W3 结构化 payload 三档（`buildPayload` 按 vendor.structured_output）

| 档位 | response_format | schema 去向 | 断言 |
| --- | --- | --- | --- |
| json_schema | `{type:json_schema, json_schema:{name:"response",strict:true,schema:<原 schema>}}` | 不注入 prompt | payload 含完整 schema 对象 |
| json_object | `{type:json_object}` | **注入最后一条 user message 末尾**（模板：中文指令+序列化 schema，要求只输出符合结构的 JSON、不带代码块围栏） | payload **不含** json_schema 结构；最后一条 message content 含注入模板前缀标识 |
| none | **无 response_format 键** | 同上注入 | payload 无 response_format；message 含注入 |
| （无 schema 请求） | 任一档都不发 response_format、不注入 | — | 非 JSONMode 请求行为不变 |

- ollama 特例：`provider_type=ollama` 时 wire 名换 `format`——json_schema→`format=<schema 对象>`；json_object→`format="json"`+注入；none→无 format+注入。
- 注入模板锚句（观测/测试断言用）：`你必须只输出符合以下 JSON 结构的 JSON 文本，不要包含任何其他文字、解释或代码块围栏：` + compact JSON。

### W4 400 运行时降级链（client 单次尝试内，仅当请求带 JSONSchema）

状态机：`json_schema --4xx--> json_object --4xx--> plain --失败--> 该尝试失败`

- 触发条件：**仅 4xx**（429/5xx 不降级——那是限流/服务端问题，降档救不了，按该尝试失败走 provider 降级）。
- 每档一次真实 HTTP 调用（fake server 计数断言 1/2/3 次）。
- **每档各写一行 ai_call_logs**：失败档 success=false（error_code=底层错误如 http_400），成功档 success=true；降级链行 RequestMeta 追加 `"structured_stage":"json_schema|json_object|plain"`（观测还原用，不算 schema 变更）。
- 中途 2xx 即止：json_schema 400 → json_object 200 → 返回 json_object 档内容；**vendor 档位保持 json_schema 不变**（不持久化，重跑仍从 json_schema 起步）。
- 三档全 4xx 或任一档非 4xx 失败 → 该 provider 尝试失败 → Router 按绑定顺序试下一个模型 → 全部失败 errors.Join。

### W5 结构化解析纠错（`extractBalancedJSON(s) (string, bool)`）

| # | 响应内容 | 期望 |
| --- | --- | --- |
| 1 | `{"a":1}` | 原样，成功 |
| 2 | `以下是结果：{"a":{"b":2}}\n完毕` | 提取首个平衡 `{...}` 块，成功 |
| 3 | `前缀 {嵌套{深}层} 尾部 {"second":1}` | 提取**首个**平衡块 `{嵌套{深}层}` |
| 4 | `完全没有任何花括号` | 纠错失败 → 该次尝试失败（ProviderError code=parse_error, retryable），**不得**把原文当结构化结果返回 |
| 5 | `{"a":1` （不平衡） | 失败，同 4 |
| 6 | 纠错位置：Router.Chat 拿到 2xx 内容后、返回 ChatResult 前执行；ChatResult.Content = 提取后的纯 JSON 文本 | 调用方直接 Unmarshal 成功 |

### W6 存量拆解迁移（testcontainer PG，migration `2026xxxx_split_ai_providers`）

输入 fixture：ai_providers 3 行（qwen :8080 带 start_command+enable_thinking=true；qwen3-embedding :8081 enable_thinking=false、model_kind=embedding；deepseek-flash 云端）+ ai_routes 5 条 + ai_route_providers 10 条。

| # | 断言 |
| --- | --- |
| 1 | 迁移后 ai_vendors=3（qwen/qwen3-embedding/deepseek-flash；vendor 携带 base_url/api_key/timeout/start_command/thinking_dialect 按名字推断：qwen*→qwen-chat-template，deepseek→deepseek，其余 none；structured_output 推断：qwen*→json_schema，deepseek→json_object） |
| 2 | ai_models=3（name=原 provider.model；model_kind 沿用；**enable_thinking true→high / false→off** 写入 thinking_level（embedding 行 thinking_level=null）；enabled 沿用；last_seen_at=now；status=ok） |
| 3 | ai_route_models=10 条，model_id 指向对应拆出模型，priority 顺序与原 ai_route_providers 一致，thinking_level 全 null（继承） |
| 4 | 新层表**无** metadata/strategy/priority(ai_routes) 字段；ai_routes 表本身保留（capability/name/enabled/max_concurrency），死列不再被代码引用 |
| 5 | **幂等**：重复执行迁移 → vendor/model/绑定计数不变（legacy_provider_id 标记判重） |
| 6 | **快照**：迁移前 ai_providers 全表 JSON 快照落 `data/`（文件存在且行数=3） |
| 7 | ai_call_logs 加 `idx_call_logs_trace ON ai_call_logs(trace_id)`（幂等 CREATE INDEX IF NOT EXISTS） |
| 8 | 冲突告警路径：造一条「同一模型挂 embedding 路由 + llm 路由」→ 迁移不失败，日志 WARN 列出模型名 |

### W7 模型清单刷新对账（`RefreshVendorModels(vendorID)`，fake /models server）

| # | 上游返回 | 库内 | 期望 |
| --- | --- | --- | --- |
| 1 | `[deepseek-chat, deepseek-reasoner]` | 空 | 新增 2 条，**enabled=false**（默认未启用），last_seen_at=now，status=ok；响应 added=[两个] |
| 2 | `[deepseek-chat]` | chat+reasoner | reasoner **标 stale（status=stale）不删**，其路由绑定保留；chat 更新 last_seen_at |
| 3 | 超时/非 2xx | chat+reasoner(stale) | 返回 error；库内清单**原样保留**（含 stale 标记不变） |
| 4 | 刷新后 stale 模型又出现在清单 | reasoner(stale) | status 回 ok、更新 last_seen_at（复活） |
| 5 | embedding 语义：刷新新增的模型 model_kind 一律 llm（用户手动改 embedding） | — | 新增行 kind=llm |

### W8 vendor CRUD 与 key 语义

| # | 场景 | 期望 |
| --- | --- | --- |
| 1 | 创建 openai_compatible vendor、api_key 空 | 成功（**无** "api_key is required"）；client 层该 vendor 请求**无 Authorization 头**（fake server 断言） |
| 2 | 更新携带 clear_api_key=true | APIKey 置空 |
| 3 | 更新 api_key 空、无 clear 标记 | 沿用原值（key 不丢） |
| 4 | 创建云端 vendor（base_url 公网）+ 模型 model_kind=embedding | 拒绝：「云端 vendor 不能配置 embedding 模型『X』」（本地判定=hostIsLocal：loopback 或 RFC1918 私网；纯函数表驱动：127.0.0.1/localhost/::1/10.x/172.16-31.x/192.168.x=本地，其余=云端） |
| 5 | start_command 保存与回显 | 列表/详情返回 start_command_configured 布尔（原文不下发）；创建/更新持久化原文 |
| 6 | vendor.name 唯一 | 重名创建拒绝（沿用现有语义）；改名到他人名下拒绝 |
| 7 | 删除被 7 条路由绑定、含 2 模型的 vendor | 单事务：解绑 7 → 删模型 2 → 删 vendor；响应 `{deleted_models:2, detached_bindings:7}`；**不因引用拒绝** |
| 8 | 删除后路由被摘空 | 路由行保留；调用该能力 → 既有 ErrNoProviders 错误路径（不 panic） |

### W9 路由绑定校验（UpsertRoute 换 model 绑定）

| # | 场景 | 期望 |
| --- | --- | --- |
| 1 | embedding 路由挂 llm 模型 | 拒绝，文案含「embedding 路由不能挂 llm 模型『<模型名>』」 |
| 2 | summary 路由挂 embedding 模型 | 拒绝，含「llm 路由不能挂 embedding 模型『<模型名>』」 |
| 3 | 同类绑定 | 放行 |
| 4 | 绑定 thinking_level=ultra | 拒绝，文案含「thinking_level 非法」（合法七档列出） |
| 5 | embedding 绑定携带 thinking_level | 拒绝（embedding 无思考语义） |
| 6 | 顺位调整+覆盖档同一次保存 | 一次 UpsertRoute 原子落库（新顺序+新覆盖值） |
| 7 | 绑定候选过滤 | LoadRouteWithModels 只返回 link.enabled && model.enabled && model.status=ok && vendor.enabled 的绑定；stale/disabled 模型不出现在 GET /api/ai/models 候选标记 usable 中 |
| 8 | 不存在 model_id | ErrModelNotFound |

### W10 降级观测还原（`QueryDegradations(window, capability)`，testcontainer PG 造日志）

造数：trace T1（A 失败→A structured 降级 json_object 成功）；trace T2（A 超时→B 成功 fallback）；trace T3（A、B 全失败）。

| # | 断言 |
| --- | --- |
| 1 | stats：calls=distinct trace 组数（3）；degradations=事件数（T1 structured + T2 fallback = 2）；failures=全链失败组数（T3 = 1） |
| 2 | T1 → 事件 `{type:structured, model:<模型名>, code:"json_schema→json_object", capability:<cap>}`（code 由相邻行 structured_stage 推导组装） |
| 3 | T2 → 事件 `{type:fallback, from:0, to:1, model:B 的模型名, code:<底层错误 timeout/http_*>}` |
| 4 | T3 → 事件 `{type:fallback, from/to=空, model:最后尝试模型, 全链失败标记}` |
| 5 | capability=topic_tagging 过滤 → events 只含该能力；stats 保持全窗口口径 |
| 6 | window=24h/7d 边界：造 25h 前一行 → 24h 不含、7d 含 |
| 7 | occurred_at 按 created_at 降序 |

### W11 aihealth vendor 化（既有 27 场景语义保持，对象换 vendor）

| # | 断言 |
| --- | --- |
| 1 | RouteHealth/DTO 字段：`primary_model`（模型名）+ `vendor`（vendor 名）替代 primary_provider；GET /api/ai/health 响应结构含两字段 |
| 2 | 探测对象=首位模型所属 vendor；两路由首位模型同 vendor → **单次探测**（probeFn 调用计数=1）复用结果 |
| 3 | 拉起/冷却（10min 窗口）/全局互斥（TryLock skip）/auto_start 总开关语义原样（既有测试改 fixture 为 vendor 链后保持绿） |
| 4 | 核心健康判定：embedding + topic_tagging 双核心的**首位模型 vendor** 可达；云端可达不救命；路由摘空 fail-closed |
| 5 | 快照 routes[] 携带 model_kind（取自模型而非 provider） |

## 3. 继承与调整（⓪ 改契约了：ai-capability-routing / ai-model-health 有 MODIFIED+REMOVED）

> 旧测试跑绿≠对，逐行处置。来源：`bash scripts/harness/test-assets.sh ai-capability-routing` / `ai-model-health`。

| 旧资产 | 旧 Scenario | 处置 | 动作 |
| --- | --- | --- | --- |
| airouter/store_test.go（UpsertProvider* 系列） | Provider API Key 可空/显式清除（REMOVED→vendor-model） | **改写** | fixture AIProvider→AIVendor，断言语义平移到 vendor CRUD（W8） |
| airouter/store_model_kind_test.go | 路由绑定类型约束（MODIFIED：provider→model） | **改写** | UpsertRoute 换 model 绑定签名，错误文案断言保留（W9-1/2/3） |
| airouter/openai_compatible_test.go（buildPayload enable_thinking 断言） | enable_thinking 输出清理（REMOVED→thinking-levels） | **改写** | 删 enable_thinking 断言，换 thinkingToWire 矩阵断言（W1）+ payload 三档（W3） |
| airouter/router_test.go（provider 链降级/errors.Join） | 无（既有回归） | **保留+适配** | fake client 签名换 vendor+model+effLevel，降级/errors.Join/审计行断言语义不变 |
| airouter/embed_cache_router_test.go | embed 白名单缓存（Non-Goal 不动） | **保留+适配** | 仅签名适配 |
| admin/handler/ai_handler_test.go（providers CRUD/删除级联） | Provider 删除级联（REMOVED→vendor 级联） | **改写** | 端点换 /api/ai/vendors*，级联响应断言 deleted_models/detached_bindings（W8-7） |
| admin/handler/ai_health_handler_test.go | 查询健康快照（MODIFIED 字段升级） | **改写** | primary_provider 断言→primary_model+vendor（W11-1） |
| platform/aihealth/*_test.go（4 文件） | 拉起/冷却/互斥/心跳/核心判定（MODIFIED 对象换 vendor） | **保留+适配** | fixture 换 vendor+model 链，探测对象断言换 vendor，全部语义保持绿（W11） |
| platform/analysispause/health_gate_compose_test.go | 健康判定回归（云端不救命等） | **保留+适配** | 快照字段名适配，判定口径断言不变 |
| front AIRouterBackupProviders.vue/.test.ts | 备用供应商表单（UI 重构替换） | **删除** | 由 VendorCard/VendorDialog+新测试接管 |
| front useAIRouterSettings.ts/.test.ts | 单 composable（拆分） | **拆分重写** | useAIVendors/useAIModels/useAIRoutes 三份+各自测试 |
| front AIRouterCapabilityRoutes.vue（drag 链） | 链式排序（保留骨架） | **保留+升级** | provider→model 引用+思考档列，drag/↑↓× 断言保留 |
| components/ui/AppConfirmDialog.test.ts | 删除确认弹窗（通用组件不动） | **保留不动** | — |

## 4. 变体走查（五组固定清单）

### 输入变体
| 变体 | 答案 |
| --- | --- |
| 空串 | vendor name/base_url 空 → 保存拒绝「vendor fields are incomplete」；thinking_level 空串=继承（绑定级）/medium（模型级解析回落），与 null 等价 |
| 纯空白（全角/tab） | name/base_url TrimSpace 后按空串处理；api_key Trim 后空+无 clear=沿用原值（W8-3 变体） |
| 大小写 | thinking_dialect/structured_output/model_kind 枚举精确匹配（不 lowercase）；capability 精确匹配（沿用现有） |
| 超长 | vendor name ≤100（DB size 限制）；模型名超长靠 DB size:100 截断报错→保存失败明确错误 |

### 前置变体
| 变体 | 答案 |
| --- | --- |
| 空集 | 无 vendor → 列表空数组+前端空态引导；无绑定路由 → ErrNoProviders 既有路径（W8-8） |
| 重复 | vendor 重名拒绝（W8-6）；同 model 重复挂同一路由 → ai_route_models 无 unique(route_id,model_id) 约束但 UpsertRoute 全删全建幂等覆盖（重复项后写覆盖前写，顺序为准） |
| 越界引用 | 绑定不存在 model_id → ErrModelNotFound（W9-8）；刷新时 vendor 不存在 → not found 错误 |
| 部分满足 | 降级链中途成功即止（W4）；绑定列表部分 disabled/stale → LoadRouteWithModels 过滤后只返回可用项（W9-7） |

### 时间窗口变体
| 变体 | 答案 |
| --- | --- |
| 边界两端 | window=24h：`created_at >= now()-24h`（含边界）；7d 同理（W10-6 造 25h 前行断言不含） |
| 空窗口 | stats 全 0 + events 空数组（前端空态文案，observability「空窗口态」） |
| 跨窗口 | 7d 窗口受 ai_call_logs 7 天清理约束，按现存日志计算（spec 明示） |
| 归一化 | window 参数非法值（非 24h/7d）→ 400 错误「window must be 24h or 7d」 |

### 幂等变体
| 变体 | 答案 |
| --- | --- |
| 重复执行 | 存量迁移幂等（W6-5，legacy_provider_id 判重）；UpsertRoute 全删全建天然幂等；refresh 重复调用=对账重跑（结果确定） |
| 部分失败重试 | 迁移单事务失败整体回滚（无半拆状态）；vendor 删除级联单事务（W8-7） |
| 并发 | Router 信号量语义不变（既有）；aihealth probeMu 互斥保持（W11-3）；观测查询只读无并发问题 |

### 可用性变体（UI 必检前三）
| 变体 | 答案 |
| --- | --- |
| 误输入反馈 | VendorDialog 字段级校验（U1）；非法 thinking_level 保存被后端拒+toast 错误文案（W9-4 文案） |
| 空态 | 无供应商引导（ui-design State Matrix）；观测空窗口文案（U6）；空链虚线引导（沿用） |
| 错误态 | 列表加载失败内联条+重试；刷新失败 toast 保留旧清单（W7-3/U3） |
| 加载态 | 卡片/route-section/观测骨架行；saving 锁（State Matrix） |
| 超长文本 | 模型名/base_url/能力名/错误码 ellipsis+title（Layout Contract） |
| 重复提交 | 保存按钮 loading 锁（沿用 saving 语义） |

### 效果核对（问句 4）
| 项 | 答案 |
| --- | --- |
| 结构化降级链真实止血效果 | 自动化层用 fake server 验证路径正确（W4）；真实 provider 行为差异靠上线后 ai_call_logs 观测面板量化（7.5 冒烟：一次真实调用后观测出数）——人工核对留痕 tasks 7.5 |
| 迁移拆解正确性 | testcontainer PG 全量断言计数（W6），不依赖真实库效果 |

### 展示字段盘点（问句 5）
改的数据结构=vendor/model 两层，用户可见字段全部有 spec 锚：vendor 列表字段（ai-vendor-model 实体契约）、health DTO（ai-model-health「健康快照与查询 API」逐字段列出）、观测事件字段（ai-route-observability「降级事件字段」逐字段列出）、路由绑定链项（ui-design Interaction Contract）。无隐式契约新增。

## 5. Scenario → 落点映射（84 条逐条）

### ai-vendor-model（17）

| # | Scenario | 落点 |
| --- | --- | --- |
| 1 | Scenario: vendor 承载协议与端点 | ✅ store_vendor_test.go：创建 llama.cpp vendor 断言协议/端点/超时/方言/结构化档全落 vendor 行 |
| 2 | Scenario: model 挂属 vendor 并声明类别 | ✅ store_vendor_test.go：vendor 下建模型断言 FK/类别/默认档 |
| 3 | Scenario: 无 key vendor 创建成功 | ✅ W8-1（store_vendor_test.go） |
| 4 | Scenario: 无 key vendor 调用不带认证头 | ✅ W8-1 后半（openai_compatible fake server 断言无 Authorization） |
| 5 | Scenario: 清除已存 key | ✅ W8-2/3 |
| 6 | Scenario: 配置本地启动命令 | ✅ W8-5 |
| 7 | Scenario: 云端供应商留空 | ✅ W8-5（空 start_command 保存+configured=false） |
| 8 | Scenario: 刷新发现新模型 | ✅ W7-1 |
| 9 | Scenario: 上游消失的模型标 stale 不删除 | ✅ W7-2 |
| 10 | Scenario: 刷新失败保留旧清单 | ✅ W7-3 |
| 11 | Scenario: 云端 vendor 不能配 embedding 模型 | ✅ W8-4（hostIsLocal 表驱动+拒绝文案） |
| 12 | Scenario: 新拉取模型默认未启用 | ✅ W7-1 |
| 13 | Scenario: 删除被路由引用的 vendor | ✅ W8-7 |
| 14 | Scenario: 删除确认弹窗列明级联 | ✅ U5（VendorCard.test.ts） |
| 15 | Scenario: 三行存量拆解为两层 | ✅ W6-1/2/3 |
| 16 | Scenario: 死字段不迁移 | ✅ W6-4 |
| 17 | Scenario: enable_thinking 布尔映射默认档 | ✅ W6-2 |

### ai-thinking-levels（11）

| # | Scenario | 落点 |
| --- | --- | --- |
| 1 | Scenario: 未配置任何档位时请求用全局默认 | ✅ W2-4（thinking_test.go） |
| 2 | Scenario: 绑定覆盖压过模型默认 | ✅ W2-2 |
| 3 | Scenario: 绑定未覆盖时用模型默认 | ✅ W2-3 |
| 4 | Scenario: 绑定处按场景调档 | ✅ W9-6 + U4 |
| 5 | Scenario: 非法档位被拒绝 | ✅ W9-4 |
| 6 | Scenario: qwen 方言映射与档位钳制 | ✅ W1 矩阵（thinking_test.go 表驱动） |
| 7 | Scenario: none 方言不发思考参数 | ✅ W1 none 行 |
| 8 | Scenario: 未声明模型钳制到 high | ✅ W2-5 分支一（clampLevel 单测） |
| 9 | Scenario: 声明支持的模型全档可选 | ✅ W2-5 分支二 + U4（下拉候选断言） |
| 10 | Scenario: 非 off 档清理 think 标签 | ✅ router_test.go：fake server 返回 `<think>` 内容，high 档断言剥离 |
| 11 | Scenario: off 档保留原始内容 | ✅ router_test.go：off 档断言原样（含标签）；reasoning_content 独立字段仅取 content 保持 |

### ai-capability-routing（11）

| # | Scenario | 落点 |
| --- | --- | --- |
| 1 | Scenario: 新建 provider 默认 llm | ✅ W7-5 + models API DTO（handler 测试断言默认 llm） |
| 2 | Scenario: 列表返回 model_kind | ✅ handler 测试：GET /api/ai/models 每项含 model_kind |
| 3 | Scenario: 显式声明 embedding 类型 | ✅ W8-4 正向（本地 vendor 允许 embedding） |
| 4 | Scenario: model_kind 与 provider_type 正交 | ✅ store_vendor_test.go：openai_compatible vendor + embedding 模型接受 |
| 5 | Scenario: embedding 路由拒绝 llm provider | ✅ W9-1（store_model_kind_test.go 改写） |
| 6 | Scenario: llm 路由拒绝 embedding provider | ✅ W9-2 |
| 7 | Scenario: 同类绑定放行 | ✅ W9-3 |
| 8 | Scenario: 存量迁移后冲突告警 | ✅ W6-8 |
| 9 | Scenario: stale 与未启用模型不可绑定 | ✅ W9-7（候选过滤+提交拒绝） |
| 10 | Scenario: 线路被摘空后调用该能力 | ✅ W8-8（store_test.go 改写，ErrNoProviders） |
| 11 | Scenario: 顺位调整与覆盖档同时保存 | ✅ W9-6 |

### structured-output-adaptation（10）

| # | Scenario | 落点 |
| --- | --- | --- |
| 1 | Scenario: DeepSeek 预设携带宽松档位 | ✅ 前端预设常量测试（presets.spec.ts：DeepSeek.structured_output=json_object） |
| 2 | Scenario: 档位可改 | ✅ handler 测试：PUT vendor 改档位持久化+后续 payload 按新档（W3 复用） |
| 3 | Scenario: json_schema 档携带 schema | ✅ W3 行 1（structured_payload_test.go） |
| 4 | Scenario: json_object 档 schema 入 prompt | ✅ W3 行 2 |
| 5 | Scenario: none 档纯文本 | ✅ W3 行 3 |
| 6 | Scenario: json_schema 400 后降级成功 | ✅ W4（structured_degrade_test.go：fake server 400→200，断言成功+档位不变+事件行） |
| 7 | Scenario: 降级链穷尽后走 provider 降级 | ✅ W4（400,400,400→下一模型→errors.Join） |
| 8 | Scenario: 降级不持久化档位 | ✅ W4（二次调用仍从 json_schema 起步断言） |
| 9 | Scenario: 响应带包裹文本时纠错提取 | ✅ W5-2/3（extractBalancedJSON 表驱动） |
| 10 | Scenario: 纠错失败按尝试失败处理 | ✅ W5-4/5 |

### ai-model-health（27，全部 ♻️ 适配保持绿）

| # | Scenario | 落点 |
| --- | --- | --- |
| 1 | Scenario: 总开关关时不拉起 | ♻️ aihealth_launch_guard_test.go（fixture vendor 链） |
| 2 | Scenario: 总开关开且不通时拉起并复探 | ♻️ aihealth_launch_guard_test.go |
| 3 | Scenario: 无 start_command 的 provider 不被拉起 | ♻️ aihealth_launch_guard_test.go（vendor.start_command） |
| 4 | Scenario: 已可达则不重复拉起 | ♻️ aihealth_launch_guard_test.go |
| 5 | Scenario: 进程生命周期不被托管 | ♻️ launch_unix.go 语义不变（无 PID 记录断言保持） |
| 6 | Scenario: 同 provider 多路由共享时只拉起一次 | ♻️ aihealth_launch_guard_test.go（共享 vendor 单次探测断言 W11-2） |
| 7 | Scenario: 冷却窗口内 reprobe 不再拉起 | ♻️ aihealth_launch_guard_test.go |
| 8 | Scenario: 冷却过期后可再次拉起 | ♻️ aihealth_launch_guard_test.go |
| 9 | Scenario: 探测互斥，进行中时不重复探测 | ♻️ reprobe_test.go（probeMu TryLock 保持） |
| 10 | Scenario: 两类各通一条即健康 | ♻️ aihealth_test.go（核心判定 vendor 化） |
| 11 | Scenario: 缺 embedding 则不健康 | ♻️ aihealth_test.go |
| 12 | Scenario: 缺 llm 则不健康 | ♻️ aihealth_test.go |
| 13 | Scenario: 云端 LLM 可达不救命（2026-09-28 回归） | ♻️ aihealth_test.go + analysispause/health_gate_compose_test.go |
| 14 | Scenario: 打标路由未配置不健康 | ♻️ aihealth_test.go |
| 15 | Scenario: 未配置任何路由不健康 | ♻️ aihealth_test.go |
| 16 | Scenario: 查询健康快照 | ✅ ai_health_handler_test.go 改写：DTO 含 primary_model+vendor（W11-1） |
| 17 | Scenario: 设置自动拉起总开关 | ♻️ ai_health_handler_test.go（不动） |
| 18 | Scenario: 手动触发异步重探 | ♻️ ai_health_reprobe_test.go（不动） |
| 19 | Scenario: 探测 in-flight 时手动重探跳过 | ♻️ ai_health_reprobe_test.go（不动） |
| 20 | Scenario: 调度器状态携带健康摘要 | ♻️ scheduler status 测试（字段适配） |
| 21 | Scenario: 启动时探测每条路由主 provider | ♻️ aihealth_test.go（探测对象=首位模型 vendor） |
| 22 | Scenario: 仅探主 provider 不探 fallback | ♻️ aihealth_test.go |
| 23 | Scenario: 无 provider 的路由跳过 | ♻️ aihealth_test.go（无绑定模型跳过） |
| 24 | Scenario: ListRoutes 瞬态失败时重试 | ♻️ aihealth_test.go（重试环不动） |
| 25 | Scenario: 快照健康后仍按心跳周期复检且可降级 | ♻️ heartbeat_degrade_test.go |
| 26 | Scenario: 快照不健康时定时复检直至自愈 | ♻️ reprobe_test.go |
| 27 | Scenario: 共享 vendor 只探测一次 | ♻️ aihealth_test.go（W11-2：两路由首位模型同 vendor，probeFn 调用计数=1） |

### ai-route-observability（8）

| # | Scenario | 落点 |
| --- | --- | --- |
| 1 | Scenario: 24h 窗口统计 | ✅ W10-1（observability_test.go） |
| 2 | Scenario: 按 capability 过滤事件流 | ✅ W10-5 |
| 3 | Scenario: 结构化降级事件 | ✅ W10-2 |
| 4 | Scenario: 供应商失败顺位迁移事件 | ✅ W10-3 |
| 5 | Scenario: 全链失败事件 | ✅ W10-4 |
| 6 | Scenario: 时间窗切换刷新统计 | ✅ U6（DegradationPanel.test.ts） |
| 7 | Scenario: 定位跳转能力路由 | ✅ U6 + opencli ⑤ |
| 8 | Scenario: 空窗口态 | ✅ W10-1 空 fixture + U6 空态断言 |
## 6. UI 组件测试节拍（vitest）

| # | 组件 | 断言 |
| --- | --- | --- |
| U1 | VendorDialog | 两段式：预设目录点击→第二段只露 API Key（llama.cpp 另露 start_command，自定义露全量）；高级折叠默认收起；测试连接三态；字段级校验（名称/base_url 必填） |
| U2 | VendorCard | 卡片头（名称/base_url/模型数/承载 N 能力徽章）渲染；点击展开模型清单；清单行（启用勾选/类型/默认档/新·stale 标） |
| U3 | 模型清单面板 | 刷新后 diff：新模型带「新」标；stale 行灰显；刷新 loading 锁 + 失败 toast 保留旧清单 |
| U4 | ThinkingLevelSelect | 下拉项：继承（显示模型默认 N）+ 七档文字；未声明 xhigh/max 的模型只有到 high 的选项；选中非继承→父链项「覆盖」徽章 |
| U5 | 删除确认 | AppConfirmDialog sm；正文含「将删除 N 个模型、解除 M 条绑定」 |
| U6 | DegradationPanel | 统计行随窗口切换重新请求（mock api 断言参数）；事件行字段（时间/能力/顺位/模型/类型标/错误码）；「定位」emit 事件携带 capability；空态文案 |
| U7 | 能力路由链 | 调档/添加/移除/换位即时反映到本地状态 + 保存后 PUT payload 结构（[{model_id, thinking_level?}]）；chips 按 vendor 分组渲染；embedding 行 chips 只含 embedding 模型 |

## 7. 双视口验收（任务 5.3）

- 1440×900 与 1920×1080：SettingsWorkspace 框架（sidebar+content）不变；VendorDialog ≤92vw；页面无横向滚动条；截图存 `openspec/changes/ai-provider-model-overhaul/screenshots/`。
