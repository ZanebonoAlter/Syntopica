# backend-package-boundaries

## Purpose

把后端业务域之间的包依赖边界固化为可执行契约：域间只走 root 门面包、横切设施归 platform、共享模型走白名单、由 golangci-lint depguard 在 lint 期强制执行，防止结构重构后边界回潮、单域改动跨域传染。

## Requirements

### Requirement: 业务域之间只许 import 对方 root 门面包

业务域（`internal/` 下 handler/service/repository 分层结构的顶层域目录）之间的 Go import SHALL 只指向目标域的 root 门面包（如 `internal/tagmanagement`）；import 目标域的子包（`handler/`、`service/`、`repository/`、`models/` 等任何深路径）MUST 被 golangci-lint depguard 拒绝；跨域引用域内独占模型类型 SHALL 经对方 root 门面 re-export，不得直接 import 对方 `models/` 子包。`internal/app`（装配层）与 `internal/platform/*`（基础设施）不受此限制。

#### Scenario: 深路径跨域 import 被拒
- **WHEN** 域 A 的非测试文件 import `internal/tagmanagement/handler`
- **THEN** `golangci-lint run ./...` MUST 在 depguard 阶段报错并指出合法替代（root 门面包）

#### Scenario: root 门面 import 通过
- **WHEN** 域 A 的非测试文件 import `internal/tagmanagement`（root 门面）
- **THEN** depguard MUST 放行

#### Scenario: 测试文件豁免深路径
- **WHEN** 域 A 的 `*_test.go` 文件 import 域 B 子包（如测试夹具构造）
- **THEN** depguard MUST 放行（测试文件不产生生产耦合）

### Requirement: 横切基础设施归 platform

跨多个业务域复用的框架性代码（调度器框架、暂停闸门等）SHALL 位于 `internal/platform/`；业务域内的此类代码 MUST 迁出，且业务域之间的基础设施借用边（如 dataenrichment 依赖 admin 的调度器类型）MUST 为零。

#### Scenario: 业务域不依赖其他域的框架类型
- **WHEN** 检查 `internal/dataenrichment` 对 `internal/admin` 的 import
- **THEN** MUST 为零（调度器框架位于 `internal/platform/scheduler`）

#### Scenario: 域内 job 定义引用 platform 框架
- **WHEN** 任一业务域定义 scheduler JobFunc
- **THEN** 该 job 文件 import `internal/platform/scheduler` 且 lint 通过

### Requirement: 共享模型白名单

`internal/models` SHALL 只保留被 2 个及以上业务域引用的共享模型；单一业务域独占的模型 MUST 位于所属域包内。共享模型清单 SHALL 以 depguard 配置旁的白名单注释为唯一权威登记处（当前 18 项 = 12 个多业务域共享〔AISettings、Article、ArticleTopicTag、Category、EmbeddingQueue、Feed、FirecrawlJob、ReadingBehavior、SemanticLabel、TagJob、TopicTag、TopicTagRelation〕+ 6 个跨 platform 层共享〔AIProvider、AIRoute、AIRouteProvider、AICallLog、AIEmbeddingCache、Notification〕，增删以注释为准），清单外模型落回 models 时 MUST 在同 change 内更新白名单与理由。

#### Scenario: 单域模型不落在 models
- **WHEN** 新增 GORM 模型仅一个业务域使用
- **THEN** 该模型 MUST 定义在所属域包内，`internal/models` 不得新增该模型

#### Scenario: 白名单模型新增需登记
- **WHEN** 一个域内模型出现第二个业务域消费者
- **THEN** 迁回 `internal/models` 时 MUST 同步在白名单注释登记模型名与消费域

### Requirement: depguard 边界规则持续生效

包边界规则 SHALL 以 `backend-go/.golangci.yml` 的 depguard 配置为唯一执行器，`golangci-lint run ./...` 通过是边界合规的验收命令；临时豁免 MUST 以配置内注释登记豁免路径、所属 change 名与解除条件，禁止删除规则绕过。

#### Scenario: lint 通过即边界合规
- **WHEN** `golangci-lint run ./...` 退出码为 0
- **THEN** 后端包边界 SHALL 视为合规

#### Scenario: 豁免留痕
- **WHEN** 某 change 需要临时保留一条深路径跨域 import
- **THEN** depguard 配置 MUST 含该路径的豁免注释（change 名 + 解除条件），无注释的豁免视为违规
