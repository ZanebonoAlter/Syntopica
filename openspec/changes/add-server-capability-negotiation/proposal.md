<!-- complexity: complex -->
<!-- ui-impact: none -->
<!-- constraint-domains: scheduler -->

## Why

前端目前**硬假定「我访问的这台机器就是那台干活的服务器」**——全仓没有任何部署形态感知（`grep -rniE "client_mode|tier1|tier2"` 在 `front/app` 与 `backend-go` 均 0 命中）。后果已经可测：

- **「立即生成日报」永远点不动**：`front/app/composables/usePollBundle.ts:174-182` 硬等 embedding 队列就绪，而客户端形态下本机根本没有这条队列
- **「运行状态」面板整块空**：`front/app/features/settings/components/SettingsSectionRuntimeStatus.vue` 全是服务端调度器视图
- **文章处理状态恒显示「已完成」**：`front/app/features/articles/composables/useArticleProcessingStatus.ts:14-19` 只看 `firecrawl_status`/`summary_status`，客户端形态下这两个字段不存在
- **`isAIEnabled` 恒 false** → 手动摘要/打标入口消失：`front/app/composables/useAI.ts:41-53` 读的是服务端 `/api/settings`，而客户端的解析模型在用户自己机器上
- **发现页没有降级位**：客户端形态关闭供给侧管线后，该页只能整页消失或整页报错

根因是同一个：**服务端没有「我现在具备什么能力」的自述接口，前端只能靠猜**。本 change 交付这个自述接口 + 服务端侧的档位开关地基，让后续的客户端形态（瘦客户端 / 本地 sidecar / 远程壳）有可协商的契约，而不是各自打补丁。

## What Changes

- 新增 `GET /api/capabilities` 自述端点，返回：本进程档位（`client_mode` 三态）、**已注册 job 清单**、由 job 派生出的能力分组可用性、AI 层自述（embedding / chat 路由可达性 + embedding 空间标识）。
- 新增 `client_mode` 三态（`off` / `tier1` / `tier2`）的配置解析与**注册期过滤**机制：档位决定哪些调度 job 与哪些后台 worker 被注册——**不注册 = 进程内无该 goroutine**，不是运行时跳过。
- 新增 job → 能力分组的**单一映射表**（新 `clientmode` 包），同时供「注册期过滤」与「capabilities 派生」两处使用，杜绝第二份 job 清单（scheduler 域红线①）。
- 供给侧四组能力在 `tier1` 下不注册：发现四件套（4 job）、周期汇总 lifeline（3 job）、版块升级建议（1 job），合计 8 个 job——与 `docs/srs/syntopica-client/iter0-findings.md` 0-4 节的定稿矩阵一致。
- 档位配置来源：环境变量优先，其次本地配置文件，缺省 `off`（现状不变）。

**不含**（各自另开 change）：前端消费本端点、素材包同步协议（迭代 2）、多客户端 token 鉴权、桌面端 sidecar 生命周期。

## Capabilities

### New Capabilities

- `server-capability-negotiation`: 服务端能力自述端点的契约，以及客户端档位（`client_mode`）三态与注册期过滤的行为约定

### Modified Capabilities

（无）

## Impact

- **新增代码**：`backend-go/internal/platform/clientmode/`（新包：档位枚举 + job→分组映射 + 分组开关 + 档位解析 + 分组可用性派生）、`backend-go/internal/app/capabilities_handler.go`（自述端点 handler）。
- **改动代码**：
  - `backend-go/internal/app/runtime.go`：14 处 `registry.Register` 注册点接入档位过滤；`tagging.StartAllWorkers()` 调用点接入 worker 过滤。
  - `backend-go/internal/tagmanagement/service/core/workers.go`：3 个 worker 的启动点接入档位过滤。
  - `backend-go/internal/app/router.go`：注册 `GET /api/capabilities`。
- **不改**：`front/`（前端消费另开 change）、`backend-go/internal/platform/scheduler/registry.go`（注册表接口不变，过滤发生在调用侧）、数据库 schema（档位不入库）。
- **部署后用户可见变化**：现有服务器装法**行为不变**（`client_mode` 缺省 `off`，全部 job 注册、全部能力可用）；新增一个只读 GET 端点 `/api/capabilities`。档位差异只在显式配置 `client_mode=tier1` 的部署上出现。
- **旧数据降级**：不涉及数据库与数据迁移，无回滚脚本。
