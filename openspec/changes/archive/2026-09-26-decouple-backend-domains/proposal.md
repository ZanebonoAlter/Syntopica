<!-- complexity: complex -->
<!-- ui-impact: none -->
<!-- constraint-domains: scheduler, discovery -->

## Why

后端历经多版演化后，包结构已不适应现状：`internal/models` 是 44 个 GORM struct 的上帝共享内核（29 个包 import，`platform/airouter` 等基础设施反向依赖业务模型）；`admin` 域塞了 discovery/AI 运维/偏好画像/阅读行为/通知等 7+ 个不相干子域；调度器框架（`JobFunc`/`SchedulerRegistry`）住在 `admin/scheduler`，迫使 dataenrichment 与 platform 反向依赖 admin；跨域依赖大量走深路径子包甚至直接调对方 handler 层。结果是「改一个业务域容易牵连其他域」（coupling-map.md 登记的传导事故即此类），编译耦合面与心智负担持续扩大。

## What Changes

按「先搬家框架、再拆域、再下放模型、最后固化边界」四步重组后端包结构（**纯结构重构：API 路径、DB schema、运行时行为全部不变**）：

- **调度器框架搬家**：`internal/admin/scheduler` 的框架部分（`base.go` JobFunc/JobResult、`registry.go`、`pause.go`、`persistence.go`）迁至 `internal/platform/scheduler`，消除 `dataenrichment → admin` 最畸形的反向依赖边；admin 域内具体 job 文件留在原地引用新框架位置。
- **discovery 拆独立域**：`admin` 中 discovery 子域（candidate/recall/recommendation/interests/catalog/rsshub 相关 service+handler+repository，约 34 文件）整体迁出为 `internal/discovery/`，对外 API 路径 `/api/discovery/*` 不变。
- **models 保守下放**：44 个模型中 26 个单域独占的（discovery 11、tagmanagement 11、reader 1、topicgraph 1、admin 1〔PreferenceVector〕、SchedulerTask 1 随框架）迁至所属域 `models/` 子包；18 个真共享模型保留在 `internal/models`——12 个多业务域共享（Article/Feed/TagJob/SemanticLabel/TopicTag 等）+ 6 个跨 platform 层共享（AIProvider/AIRoute/AIRouteProvider/AICallLog/AIEmbeddingCache/Notification，platform/airouter 与 platform/notification 为真实消费方）。
- **域间依赖只走门面**：跨域 import 收敛到各域 root 门面包（tagmanagement 已有 `wire.go` 模式，推广到全部域），消除 `admin → tagmanagement 五个子包`、`poll_handler → tagmanagement/handler` 这类深路径/翻墙引用。
- **depguard 固化边界**：golangci-lint 增加 depguard 规则，编译期强制「业务域之间只许 import 对方 root 包 + platform/models」，防止边界回潮。

## Capabilities

### New Capabilities
- `backend-package-boundaries`: 后端业务域包边界规则——域间依赖只走 root 门面包、调度器等横切设施归 platform、共享模型白名单、depguard 强制执行与豁免流程。

### Modified Capabilities
- `architecture-docs`: 包结构重组后 backend.md / runtime.md 的结构描述更新——internal/ 树新增 `discovery/`、platform/ 新增 `scheduler/`、调度器框架描述从 `admin/scheduler` 改为 `platform/scheduler`（框架与 job 定义分家）。

> 行为类 spec（feed-discovery / scheduler-accuracy / tagging-domain 等）的要求均不变：纯结构重构，API/DB/运行时行为不变。

## Impact

- **代码**：`backend-go/internal/` 全部业务域目录结构（admin 显著瘦身、新增 discovery 域、platform/scheduler 新增、models 收缩至 18 项白名单）；~40+ 生产文件 import 路径调整；跨域调用点改走门面（14 个生产文件，含跨域测试共 23 个）。
- **API**：不变（路由注册位置内部调整，对外路径与行为完全一致）。
- **数据库**：不变（AutoMigrate 的模型类型只挪包不改定义，无 schema 变化）。
- **工具链**：`backend-go/.golangci.yml` 新增 depguard 配置；`change-scope.sh` 路径→命令映射需覆盖新包路径。
- **文档**：`docs/reference/architecture/backend.md`（四层结构描述）、`map.md`（代码入口索引）、`coupling-map.md`（解除的耦合边登记）同步更新。
- **测试**：既有测试随包迁移，断言不变；新增 depguard 边界守卫（golangci-lint 配置本身即执行器）。
