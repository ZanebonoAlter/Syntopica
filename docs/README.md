# 项目文档 Wiki

Syntopica 全部文档入口。

---

## 快速开始

| 文档 | 说明 |
|------|------|
| [../README.md](../README.md) | 项目简介与启动 |
| [reference/development.md](reference/development.md) | 开发环境搭建、构建命令 |

---

## Reference（跨里程碑活文档）

### 业务流程（Flow）— 五位一体活文档

| 文档 | 说明 |
|------|------|
| [reference/flow/](reference/flow/) | flow 索引：每个文档含「需求说明 / 链路设计 / 业务约束与不变量 / 代码入口 / 变更溯源」五段 |

> flow 文档**替代原 `user-guide/`** 承接"系统能做什么、怎么用"（面向使用视角的「需求说明」节）。apply 改代码前 constraint-injection extension 自动注入相关 flow 的业务约束节。

### 架构

| 文档 | 说明 |
| ------ | ------ |
| [reference/architecture/overview.md](reference/architecture/overview.md) | 系统总览 |
| [reference/architecture/backend.md](reference/architecture/backend.md) | 后端分层、目录结构、数据模型 |
| [reference/architecture/runtime.md](reference/architecture/runtime.md) | 启动顺序、调度器、优雅退出 |
| [reference/architecture/frontend.md](reference/architecture/frontend.md) | Nuxt 4 分层、feature 组织 |
| [reference/architecture/map.md](reference/architecture/map.md) | 业务域 → 流程 → 代码入口索引 |
| [reference/architecture/coupling-map.md](reference/architecture/coupling-map.md) | 跨功能传导耦合 |
| [reference/architecture/tracing.md](reference/architecture/tracing.md) | OpenTelemetry 集成 |

### API 参考

| 文档 | 路由前缀 |
| ------ | ---------- |
| [reference/api/_conventions.md](reference/api/_conventions.md) | 通用约定 |
| [reference/api/_index.md](reference/api/_index.md) | 完整索引 |
| [reference/api/system.md](reference/api/system.md) | `/`, `/health` |
| [reference/api/feeds.md](reference/api/feeds.md) | `/api/feeds` |
| [reference/api/articles.md](reference/api/articles.md) | `/api/articles` |
| [reference/api/ai-admin.md](reference/api/ai-admin.md) | `/api/ai` |
| [reference/api/schedulers.md](reference/api/schedulers.md) | `/api/schedulers` |
| ... | 更多见完整索引 |

### 数据库

| 文档 | 说明 |
| ------ | ------ |
| [reference/database/_index.md](reference/database/_index.md) | 数据库域文档导航 + 58 张表清单（按域分组归属速查） |
| [reference/database/tables/_conventions.md](reference/database/tables/_conventions.md) | 全局约定：FK 真相、6 条真实 DB FK、索引与约束总览、FK 引用矩阵 |
| [reference/database/DATA_LIFECYCLE.md](reference/database/DATA_LIFECYCLE.md) | 6 条数据链路的状态字段流转 |

### 开发规范

| 文档 | 说明 |
| ------ | ------ |
| [reference/development.md](reference/development.md) | 环境搭建、构建命令 |
| [reference/configuration.md](reference/configuration.md) | 配置项说明 |
| [reference/deployment.md](reference/deployment.md) | 部署方式 |
| [reference/testing.md](reference/testing.md) | 测试指南 |
| [reference/开发执行规范.md](reference/开发执行规范.md) | apply 执行纪律 + 门禁 + 归档 |

---

## 问题与经验

| 目录 | 说明 |
|------|------|
| [experience/](experience/) | 踩坑复盘（ocr 弃用、编码安全、demo 教训等） |
| [issues/](issues/) | 问题追踪（ollama 兼容、embedding、质量排序等） |

---

## 历史里程碑（快照，不再活跃维护）

`v1.1` ~ `v1.3.4` 各版本的设计 / 变更历史快照。**降级为可选**：平时 openspec archive 即终点，发版 / 做主题合集时才批量收进 `v1.x/changes/`（见 `reference/开发执行规范.md` §12.1）。

| 目录 | 说明 |
| ------ | ------ |
| [v1.1-bugfixes/](v1.1-bugfixes/) | 业务漏洞修复 |
| [v1.2-tag-intelligence/](v1.2-tag-intelligence/) | 标签智能处理 |
| [v1.3-narrative-overhaul/](v1.3-narrative-overhaul/) | 叙事大修 |
| [v1.3.1-board-bug-fix/](v1.3.1-board-bug-fix/) | 版块 bug 修复 |
| [v1.3.2-board-section/](v1.3.2-board-section/) | 版块分区 |
| [v1.3.3-good-taste/](v1.3.3-good-taste/) | 架构深化与一致性 |
| [v1.3.4-easy-use/](v1.3.4-easy-use/) | 使用简易化 |

---

## 文档维护规则

- `reference/` 为唯一权威源，反映当前系统真实状态
- flow 五段式 + 业务约束归属见 `reference/开发执行规范.md` §0.5
- SDD 文档流转（archive → reference，archive 即家）见 §12
- 归档门禁（验证节 + `doc-impact.sh verify` + 导航层死链检查）见 §11.4
- 死链由 `scripts/check-standards.sh` G 段校验
