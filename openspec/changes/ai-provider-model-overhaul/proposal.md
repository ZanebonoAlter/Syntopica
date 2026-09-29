<!-- complexity: complex -->
<!-- ui-impact: major -->
<!-- constraint-domains: ai-summary -->

## Why

`ai_providers` 一张表混装厂商/端点/模型/进程管理四种实体（一 provider 一 model、name 全局唯一、改 key 要逐行改），且携带 `metadata`（死字段）、`ai_routes.strategy`（半死）、`ai_routes.priority`（死列）等历史遗留；模型名手填无从校验、思考只有 `enable_thinking` bool（曾语义翻转一次全表重置）、response_format 写死 `json_schema` 导致 deepseek 924 次连续 400、兜底链路从未生效。要接新厂商（通用云端 agent）只能继续复制粘贴整行配置。

## What Changes

- **引入 `ai_vendors` / `ai_models` 两层表**（参考 pi-sdk `models.json` 的 provider→models[] 结构）：vendor 管协议/认证/端点/进程拉起，model 管模型身份/类别/思考档位/能力声明；`ai_providers` 拆解迁移后废弃，死字段（`metadata`、`ai_routes.strategy`、`ai_routes.priority`）不迁移。
- **思考七档照抄 pi**（`off / minimal / low / medium / high / xhigh / max`，全局默认 `medium`）：模型可配默认档，**capability 路由绑定可按场景覆盖思考等级（绑定的模型选中后才可配，类似 pi 的 session 级调节）**；wire 参数按厂商方言适配（`chat_template_kwargs.enable_thinking` / deepseek `thinking` / `reasoning_effort`），档位解析优先级：请求级 > capability 绑定级 > 模型默认 > 全局默认；`xhigh/max` 仅在模型声明支持时暴露（同 pi）。
- **模型列表支持接口刷新**：对 vendor 触发 `GET {base}/models` 拉取可用模型清单入库（`last_seen_at` 时间戳），上游消失的模型标 stale 不静默删；模型选择从手填字符串变为清单勾选。
- **结构化输出能力档位收编**（原 response-format-adaptation 的核心）：`structured_output` 档位落 vendor（`json_schema | json_object | none`）+ `buildPayload` 按档构造 + 400 运行时降级链（`json_schema`→`json_object`+schema 入 prompt→纯文本）+ schema→prompt 注入，治好 deepseek 兜底从未生效的出血点。
- **存量配置迁移**（除 deepseek 外均为 llama.cpp 启动）：`qwen`（:8080，start_command 迁 vendor）→ vendor+model；`qwen3-embedding`（:8081）同理；`deepseek-flash`（云端）→ vendor+model；`ai_route_providers` 绑定改挂 model。
- **UI major 重设计**：AI 设置改为供应商卡片 + 模型清单管理（刷新/勾选/档位配置）、能力路由绑定处按场景配思考等级、新增路由降级观测面板（近 24h 成功/降级/失败统计 + 事件流，数据源 `ai_call_logs`）；**新增供应商为预设目录三步接入（选供应商 → 只输 API Key → 自动拉模型清单勾选）**，预设携带 base_url/思考方言/结构化输出能力，自定义（OpenAI 兼容）走全量表单。
- **无 BREAKING**：业务调用方（extractor / dataenrichment / topicgraph 等）的 `Chat/Embed + Capability + Operation` 接口不变；变化收敛在 airouter 内部、admin API 与设置页。

## Capabilities

### New Capabilities

- `ai-vendor-model`: vendor/model 两层注册表的实体契约、CRUD 与迁移语义、模型清单刷新（GET /models、stale 标记）。
- `ai-thinking-levels`: 思考七档值域、四级解析优先级、按场景（capability 绑定）分配、方言→wire 参数映射、档位可用性钳制。
- `structured-output-adaptation`: provider 结构化输出能力档位声明、payload 构造、schema→prompt 注入、400 运行时降级链与解析纠错兜底。
- `ai-route-observability`: 路由降级观测（调用统计、降级事件流、按 capability/模型聚合查询）。

### Modified Capabilities

- `ai-capability-routing`: 路由绑定的承载实体从 provider 改为 model，绑定项可携带思考等级（按场景分配的落点）。
- `ai-model-health`: `start_command` 进程拉起归属从 provider 上移到 vendor，探测/拉起/冷却语义的对象改为 vendor。

## Impact

- **后端**：`internal/models/`（新 `AIVendor`/`AIModel`，`AIProvider` 废弃）；`internal/platform/database/postgres_migrations.go`（两层表建表 + 存量拆解迁移 + 死字段不迁）；`internal/platform/airouter/`（`buildPayload` 档位构造与方言映射、降级链、`Store` 改挂 model、模型刷新服务）；`internal/admin/handler/ai_handler.go`（vendor/model CRUD + 刷新 + 降级观测查询）；`internal/platform/aihealth/`（探测对象改 vendor）。
- **前端**：`front/app/features/ai/`（供应商卡片、模型清单、思考档位选择器、降级观测面板，major 重设计）；`front/app/features/settings/`（AI 配置分区结构调整）；`front/app/types/ai.ts` 与 `useAIRouterSettings.ts` 重写。
- **数据**：`ai_providers`（3 行）→ `ai_vendors`+`ai_models`；`ai_route_providers`（10 行）改挂 model_id；`ai_call_logs` 仅读不改 schema；`ai_routes` 死列不迁移。存量 `enable_thinking` bool 按 `true→high / false→off` 迁默认档。
- **文档**：`docs/reference/flow/ai-summary.md`（AI 调用/路由约束 + 变更溯源）、`docs/reference/database/`（表结构）；`standard/backend/ai-logging.md` 记账口径不变不动。
