# 后端包结构（Package Layout）

> **权威源**：本文件是后端包结构与 domain 白名单的**唯一权威**（`backend-go/AGENTS.md` 与《开发执行规范》§4.4 都指向本文件）。
> L1 验收脚本据此校验 `internal/*/` 都在白名单内、且都有 `handler/` 子目录。

## Domain 三层结构（统一约束）

所有 domain 包遵循统一三层结构：

```
internal/<domain>/
├── routes.go        # 路由注册（由 app/router.go 调用）
├── wire.go          # 单例初始化 + 对外 re-export
├── handler/         # Gin handler（package handler）
├── service/         # 业务逻辑（package service）
└── repository/      # 数据访问（package repository）
```

**规则：**

- 根包**只**放 `routes.go` 和 `wire.go`，禁止放 handler/service/repository 文件
- Handler 必须在 `handler/` 子包（package 名 `handler`）
- `wire.go` 对外 re-export 外部包需要的类型/函数，调用方**只 import 根包**
- `routes.go` import `handler/` 并把路由接到 handler 函数

## Domain 白名单

当前合法 domain（`L1` 脚本据此校验）：

| Domain | 职责 |
|--------|------|
| `admin` | 管理后台（handler, repository, scheduler〔域内 job 定义〕, routes, wire；AI 运维/poll/comtrade） |
| `reader` | 订阅与文章域 |
| `tagmanagement` | 标签系统域 |
| `topicgraph` | 主题图谱域 |
| `dataenrichment` | 数据增强编排域（板块↔数据源配置、分层新闻汇总循环A、三角色增强循环B、review judge 认知循环） |
| `datasources` | 外部研究数据源域（EIA/JODi/WDI/Comtrade 取数器 + 目录/probe API + wiring 装配） |
| `discovery` | 订阅发现域（feed-discovery：候选/召回/推荐/可用性/目录同步/偏好画像；decouple-backend-domains 自 admin 迁出） |

> 新增 domain 时必须先在本表登记，再创建包；脚本会拦截未登记的 `internal/<新名>/`。
>
> 域间边界规则（跨域只走 root 门面 + depguard 拦截）见 spec `backend-package-boundaries` 与 `.golangci.yml` depguard 配置；单域独占模型放域内 `models/` 子包（AutoMigrate 经 `database.RegisterModels` 自注册）。

## 共享目录（非 domain）

| 目录 | 职责 |
|------|------|
| `cmd/server/` | 应用入口 |
| `internal/app/` | HTTP 路由、中间件、运行时装配 |
| `internal/models/` | 共享 GORM 模型 |
| `internal/platform/` | 共享基础设施（config, database, ws, airouter, aisettings, middleware, tracing, jsonutil, testutil） |
| `configs/` | 配置文件 |

> `internal/app`、`internal/models`、`internal/platform` 是**基础设施层**，不是业务 domain，不走三层结构，也不在 domain 白名单校验范围内。

## Anti-Patterns（硬禁）

- ❌ 业务逻辑写进 `router.go`
- ❌ Handler 直接访问 DB（绕过 service/repository）
- ❌ `panic` 处理错误
- ❌ handler 文件放 domain 根包（必须在 `handler/` 子包）
- ❌ 绕过 `wire.go` 对外暴露符号（外部需要就加 re-export）

## 路由与业务位置

- HTTP 路由注册在 `internal/app/router.go`
- 业务逻辑在 `internal/<domain>/service/`，不在 handler 或 router 中

## 资料来源

收敛自原 `backend-go/AGENTS.md`（Package Organization Convention / Anti-Patterns）与 `development.md` §后端目录约定。
