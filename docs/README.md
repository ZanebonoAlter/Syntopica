# 项目文档 Wiki

Syntopica 全部文档入口。

---

## 快速开始

| 文档 | 说明 |
|------|------|
| [../README.md](../README.md) | 项目简介与启动 |
| [getting-started.md](getting-started.md) | 开发环境搭建 |

---

## Reference（跨里程碑活文档）

### 架构
| 文档 | 说明 |
|------|------|
| [reference/architecture/overview.md](reference/architecture/overview.md) | 系统总览 |
| [reference/architecture/backend.md](reference/architecture/backend.md) | 后端分层、目录结构、数据模型 |
| [reference/architecture/runtime.md](reference/architecture/runtime.md) | 启动顺序、调度器、优雅退出 |
| [reference/architecture/frontend.md](reference/architecture/frontend.md) | Nuxt 4 分层、feature 组织 |
| [reference/architecture/data-flow.md](reference/architecture/data-flow.md) | 主链路、前端状态、定时任务链路 |
| [reference/architecture/tracing.md](reference/architecture/tracing.md) | OpenTelemetry 集成 |

### API 参考
| 文档 | 路由前缀 |
|------|----------|
| [reference/api/_conventions.md](reference/api/_conventions.md) | 通用约定 |
| [reference/api/_index.md](reference/api/_index.md) | 完整索引 |
| [reference/api/system.md](reference/api/system.md) | `/`, `/health` |
| [reference/api/feeds.md](reference/api/feeds.md) | `/api/feeds` |
| [reference/api/articles.md](reference/api/articles.md) | `/api/articles` |
| [reference/api/summaries.md](reference/api/summaries.md) | `/api/summaries` |
| [reference/api/ai-admin.md](reference/api/ai-admin.md) | `/api/ai` |
| [reference/api/schedulers.md](reference/api/schedulers.md) | `/api/schedulers` |
| ... | 更多见完整索引 |

### 数据库
| 文档 | 说明 |
|------|------|
| [reference/database/DATABASE_FIELDS.md](reference/database/DATABASE_FIELDS.md) | 数据库字段详细说明 |

### 开发规范
| 文档 | 说明 |
|------|------|
| [reference/development.md](reference/development.md) | 构建、测试、编码规范 |
| [reference/configuration.md](reference/configuration.md) | 配置项说明 |
| [reference/deployment.md](reference/deployment.md) | 部署方式 |
| [reference/testing.md](reference/testing.md) | 测试指南 |

---

## 用户手册（User Guide）

| 文档 | 说明 |
|------|------|
| [userguide/reading.md](userguide/reading.md) | 阅读功能（布局、文章阅读、阅读偏好） |
| [userguide/feeds-and-categories.md](userguide/feeds-and-categories.md) | 订阅源与分类管理 |
| [userguide/ai-features.md](userguide/ai-features.md) | AI 总结与 Provider 管理 |
| [userguide/topic-graph.md](userguide/topic-graph.md) | Topic Graph 主题图谱 |
| [userguide/tags.md](userguide/tags.md) | 文章标签 |
| [userguide/narrative.md](userguide/narrative.md) | 叙事面板 |

---

## 里程碑（Milestones）

| 目录 | 说明 | 状态 |
|------|------|------|
| [v1.1-bugfixes/](v1.1-bugfixes/) | 业务漏洞修复 | 已完成 |
| [v1.2-tag-intelligence/](v1.2-tag-intelligence/) | 标签智能处理 | 已完成 |
| [v1.3-narrative-overhaul/](v1.3-narrative-overhaul/) | 叙事大修 | 进行中 |

每个里程碑包含：`SUMMARY.md` + `design/` + `user-guide/` + `changes/` + `debug/`

---

## 经验沉淀

| 文档 | 说明 |
|------|------|
| [experience/LESSONS_LEARNED.md](experience/LESSONS_LEARNED.md) | 踩坑记录 |
| [experience/ENCODING_SAFETY.md](experience/ENCODING_SAFETY.md) | Windows 编码安全 |

---

## 文档维护规则

- `reference/` 为唯一权威源，反映当前系统真实状态，随里程碑完成而更新
- 新里程碑在 `docs/` 下创建 `v{version}-{name}/` 目录
- 里程碑内按 `design/`、`user-guide/`、`changes/`、`debug/` 四类分组
