# Design — ai-provider-model-overhaul

## Context

现状关键事实（已从代码核实）：

- **airouter 数据面**：`Store.LoadRouteWithProviders` 按 capability 取 `ai_routes` → `ai_route_providers`(priority) → `ai_providers`；`Router.Chat` 顺序尝试 provider 列表，每次尝试 `LogCall` 写 `ai_call_logs`（成功记 `IsFallback: idx>0`，失败也记）。
- **`ai_call_logs` 已有还原降级事件所需的全部锚点**：`TraceID`（同一 `Router.Chat` 的 ctx span 下所有尝试共享）、`IsFallback`、`ProviderName`/`Model`、`ErrorCode`、`CreatedAt`。观测查询可纯 SQL 聚合，**不改该表 schema**（proposal 承诺成立）。
- **payload 构造点**：`openai_compatible.go` 的 `buildPayload(provider, req)`；response_format 目前写死 json_schema（deepseek 924×400 的事故源）。
- **前端**：`front/app/types/ai.ts`（AIProvider/AIRoute 单层）、`useAIRouterSettings.ts`、`AIProviderManagement.vue` + `AIRouterCapabilityRoutes.vue`（drag 链）。
- 红线（ai-summary 域）：Operation/Capability 非空校验、capability 枚举不动、并发按 capability 信号量、失败按序降级 + errors.Join、每次尝试必写审计、审计截断保护、provider_type 仅 openai_compatible/ollama 两类 client。

## Goals / Non-Goals

**Goals:**
- vendor/model 两层表 + 存量拆解迁移，airouter 数据面、admin API、设置页三层同步切换。
- 思考七档（四级解析 + 方言映射 + 钳制）与结构化输出档位（构造 + 400 降级链 + 解析纠错）进入请求构造路径。
- 降级观测面板纯查询实现（ai_call_logs 只读）。

**Non-Goals:**
- 不动 capability 枚举（五红线之二）、不动 `Chat/Embed + Capability + Operation` 调用方接口、不动 embed 白名单缓存、不动 ai_call_logs schema 与 7 天清理。
- 不做 vendor 预设目录的后端托管（预设清单前端内置常量，随版本更新）。
- 不做跨主机进程拉起（start_command 仍只在后端主机执行，留空=只探测）。

## Decisions

### D1 两层表 schema

```
ai_vendors:  id, name(unique), preset_id?, provider_type(openai_compatible|ollama),
             base_url, api_key, timeout_seconds, start_command(text,null),
             thinking_dialect(qwen-chat-template|deepseek|reasoning-effort|none),
             structured_output(json_schema|json_object|none),
             supports_embedding(bool), enabled, created_at, updated_at
ai_models:   id, vendor_id(FK), name, model_kind(llm|embedding),
             thinking_level(nullable 七档=模型默认档; embedding 恒 null),
             supports_xhigh_max(bool default false),
             enabled, status(ok|stale), last_seen_at, created_at, updated_at
             unique(vendor_id, name)
ai_route_models: id, route_id(FK ai_routes), model_id(FK ai_models),
             priority(1..n), thinking_level(nullable=场景覆盖档), enabled
```

- `ai_routes` 表保留（capability/name/enabled/max_concurrency 不变），删绑列换关联表；死列 `strategy`/`priority`/`metadata` 不迁不建。
- 勘误（2026-10-05 review）：①不建 `supports_embedding` 列——云端/本地判定按 spec「模型配置约束」用 hostIsLocal(base_url) 动态判定；②三张新表不建物理 FK（级联删除走应用层单事务，与「Vendor 删除级联」spec 自洽）；后续字段演进必须走版本化迁移（新表由迁移 DDL 创建，未注册 AutoMigrate）。
- vendor.name 唯一（继承原 provider.name 全局唯一语义，模型名唯一性收敛到 vendor 内）。
- 备选否决：vendor.name 不唯一 + 展示名拼接——单用户产品无重名诉求，唯一约束换来更简单的级联与引用。

### D2 存量迁移（幂等拆解）

版本化迁移三步，全在一个事务性 migration 内：
1. `ai_providers` 每行 → 1 vendor（协议/端点/key/超时/start_command/enable_thinking 丢弃）+ 1 model（name=provider.model、model_kind 沿用、thinking_level=enable_thinking?high:off、enabled 沿用、last_seen_at=now）。
2. `ai_route_providers` 逐行改挂：provider_id → 对应拆出的 model_id，priority 保留 → 写 `ai_route_models`（thinking_level 置 null=继承）。
3. `ai_providers` 停写（代码层全部走新表；物理表保留一个版本周期作回滚保险，下次大版本清理）。

幂等实现：按 `ai_providers.id` 查已迁移标记（vendor 侧存 legacy_provider_id 列，迁移完可留作对账），重复执行跳过已迁移行。回滚：旧表未删 + 代码回退即可回旧路径（新产生的 vendor/model 配置丢失，可接受——单用户，配置量小）。

### D3 airouter 数据面：绑定链从 provider 换 model

- `Store.LoadRouteWithProviders` → `LoadRouteWithModels(capability)`：返回 `[]RouteModelBinding{Model, Vendor, ThinkingLevel(绑定覆盖)}`，过滤 enabled=true 且 model.enabled 且 status=ok。
- `ProviderClient` 接口签名：`Chat(ctx, provider, req)` → `Chat(ctx, vendor, model, effLevel, req)`；`Router.Chat` 循环里算生效档（请求级>绑定级>模型默认>medium，钳制 xhigh/max）后传入。
- `ResolvePrimaryProvider` → `ResolvePrimaryModel`（aihealth 用）。
- 候选过滤（UpsertRoute 校验 embedding/llm 类型）在 store 层校验后写 `ai_route_models`。

### D4 思考档方言映射（初版，实现时按厂商文档校准）

| dialect | wire 参数 | 档位→值 |
|---|---|---|
| qwen-chat-template | `chat_template_kwargs.enable_thinking` | off→false；minimal..high→true（钳制 xhigh/max→high） |
| deepseek | DeepSeek `thinking` 参数 | off→关闭态；其余→开启态（具体枚举实现时校准） |
| reasoning-effort | `reasoning_effort` | off→不发该字段；minimal/low/medium/high 直传；xhigh/max 钳 high |
| none | 不发参数 | —（档位仅影响输出清理） |

映射表集中在 airouter 一个纯函数（`thinkingToWire(dialect, level)`），单测覆盖钳制与方言边界。输出清理 `<think>` 剥离按生效档 off/非off 二值，替换原 `enable_thinking` 清理标记的引用点。

### D5 结构化输出构造与 400 降级链

- `buildPayload` 按 vendor.structured_output 三档构造 response_format + schema→prompt 注入（注入模板：要求只输出符合给定 JSON 结构的文本）。
- 降级链在 **client 单次尝试内**：json_schema 发出遇 4xx → 降 json_object + schema入prompt 重试 → 再 4xx 降纯文本重试；**每一档是一次真实 HTTP 调用、各写一行 ai_call_logs**（红线 5「每次尝试必写审计」本就要求如此；deepseek 924×400 在新账本里每行可见）。降级方向由行序可还原，无需新字段。
- 降级链不持久化 vendor 档位；解析纠错（平衡花括号提取）在结构化响应解析处，失败按该次尝试失败走 provider 降级。

### D6 降级观测 = ai_call_logs 纯查询

`GET /api/ai/observability/degradations?window&capability`：
- 统计：窗口内 `count(*)` / `count(is_fallback=true or 组内多行)` / `count(success=false 且组内无成功行)`。
- 事件：按 `trace_id` 分组（同一 Router.Chat 的全部尝试），组内按 created_at 排序还原：fallback 事件 = 失败行(N) → 成功行(is_fallback=true) 的顺位迁移；全链失败 = 组内全败；structured 事件 = 同 provider 相邻两行（4xx → 次档成功）还原降级方向。
- 无新表、无 schema 变更；7d 窗口受 7 天清理约束（查询按现存日志计算）。

### D7 admin API 面（旧端点直接替换，不留兼容窗口）

```
GET/POST        /api/ai/vendors            列表/新增（新增后自动触发一次清单刷新）
GET/PUT/DELETE  /api/ai/vendors/:id        详情（key/start_command 只回 configured 布尔）/更新（含 clear_api_key）/删除（级联）
POST            /api/ai/vendors/:id/models/refresh   清单刷新（diff 返回新增/消失）
GET             /api/ai/models             全部模型（含 vendor 摘要，供绑定候选）
PUT             /api/ai/models/:id         模型配置（enabled/kind/thinking_level/supports_xhigh_max）
GET/PUT         /api/ai/routes             路由列表/保存（绑定项=[{model_id, thinking_level?}]，UpsertRoute 校验类型）
POST            /api/ai/vendors/:id/test   测试连接（GET /models 探测）
GET             /api/ai/observability/degradations?window&capability
```
旧 `/api/ai/providers*` 端点删除（单用户，前后端同 change 原子切换）。`GET /api/ai/health` 响应字段 primary_provider → primary_model + vendor（ai-model-health spec 已定）。

### D8 前端重构映射

| 旧 | 新 |
|---|---|
| `types/ai.ts` AIProvider/AIRoute | AIVendor/AIModel/AIRouteModelBinding（重写） |
| `AIProviderManagement.vue`（主+备表单） | `VendorCard.vue`（卡片+扇出徽章+展开清单）+ `VendorDialog.vue`（两段式） |
| `AIRouterCapabilityRoutes.vue` | 保留链式骨架，链项 provider→model 引用 + `ThinkingLevelSelect.vue`（继承/七档/覆盖徽章） |
| `SettingsSectionAiHealth.vue` | 追加 `DegradationPanel.vue`（统计+事件流+定位，挂 ai-health tab） |
| `useAIRouterSettings.ts` | 拆 `useAIVendors.ts` / `useAIModels.ts` / `useAIRoutes.ts` |

## Risks / Trade-offs

- [迁移写错拆坏存量配置] → 迁移前自动备份 ai_providers 全表到 JSON 快照落 `data/`；迁移幂等 + 拆解后断言（vendor/model 计数、绑定计数对账日志）；失败回滚事务。
- [方言映射初版与真实 API 不符（qwen 布尔化丢失档位细分、deepseek thinking 取值待校准）] → 映射集中单函数 + 用例表驱动；上线后按 ai_call_logs 实际 4xx 反馈调表，不改架构。
- [观测按 trace_id 分组在日志暴量时查询变慢] → window 限 24h/7d 两档 + created_at 索引已有；trace_id 无索引——迁移顺手加（同表加索引非 schema 语义变更，允许；若视为变更则退化为按 is_fallback+时间窗直查，事件流精度降为逐行）。
- [providers 旧端点删除与前端不同步部署] → 单二进制同进程部署（前端静态产物铺在后端），天然原子；无外部 API 消费者。
- [qwythos 单 vendor 承载 7 能力的单点风险只是可见化，未自动缓解] → 非目标；扇出徽章 + 观测面板给人工调整依据。

## Migration Plan

1. 合并部署单次完成：迁移 → 后端新端点 → 前端新页面（静态产物随二进制）。
2. 部署前确认 04:00 自动备份存在（pg-scheduled-backup）+ 迁移自带 ai_providers JSON 快照。
3. 回滚策略：代码回退旧版二进制（旧表未删，旧路径可跑）；迁移新增的新表数据保留无害。
4. 部署后验证清单（tasks.md 验证节展开）：vendor/model 计数=3/3、路由绑定 10 条挂 model、首次调用全链成功、观测面板出数。

## Open Questions

- deepseek `thinking` 参数取值域与 qwen 方言在 minimal..high 间是否有更细 wire 差异——实现期按厂商文档校准 D4 映射表，不影响架构。
- 预设目录初始清单（除 DeepSeek/小米/ZAI/OpenRouter/Ollama/llama.cpp/自定义外是否再加）——前端常量，随时可补，不阻塞。
