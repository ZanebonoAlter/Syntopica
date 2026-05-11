# Purpose

TBD

# Requirements

## Requirement: Reference directory as single truth source
`docs/reference/` SHALL 作为跨里程碑活文档的唯一权威位置。架构文档、API 参考、数据库文档、开发规范 SHALL 只在 reference/ 下维护一份，不在其他位置保留副本。

### Scenario: Architecture documentation location
- **WHEN** 需要查阅后端架构
- **THEN** 唯一位置为 `docs/reference/architecture/backend.md`，不存在 `backend-go/ARCHITECTURE.md`

### Scenario: Frontend architecture documentation location
- **WHEN** 需要查阅前端架构
- **THEN** 唯一位置为 `docs/reference/architecture/frontend.md`，不存在 `front/ARCHITECTURE.md`

## Requirement: Reference directory structure
`docs/reference/` SHALL 包含以下子目录和文件：
- `architecture/` — 系统总览、后端架构、前端架构、组件、数据流、运行时、链路追踪
- `api/` — API 参考文档（按路由前缀拆分）
- `database/` — 数据库字段参考
- `development.md` — 开发规范（构建、测试、代码风格、目录约定、提交检查）
- 其他跨里程碑功能指南（configuration.md、deployment.md、testing.md 等）

### Scenario: Reference directory listing
- **WHEN** 列出 `docs/reference/`
- **THEN** 可见 architecture/、api/、database/ 目录和若干 .md 文件

## Requirement: Reference docs are living documents
`docs/reference/` 下的文档 SHALL 反映当前系统真实状态，随每次里程碑完成而更新。

### Scenario: Post-milestone reference update
- **WHEN** v1.3 完成并引入新的后端模块
- **THEN** `docs/reference/architecture/backend.md` 更新以反映新模块

## Requirement: Duplicate architecture files removal
以下冗余架构文档 SHALL 被删除：
- `front/ARCHITECTURE.md`
- `backend-go/ARCHITECTURE.md`
- `docs/developer/frontend-architecture.md`
- `docs/operations/architecture/` 目录

### Scenario: No duplicate architecture files
- **WHEN** 迁移完成后在项目根目录执行搜索
- **THEN** 不存在 `front/ARCHITECTURE.md`、`backend-go/ARCHITECTURE.md`、`docs/developer/` 目录

## Requirement: Existing docs migration to reference
以下现有文档 SHALL 移动到 `docs/reference/` 对应位置：

| 原位置 | 目标位置 |
|--------|----------|
| `docs/architecture/*` | `docs/reference/architecture/*` |
| `docs/api/*` | `docs/reference/api/*` |
| `docs/database/*` | `docs/reference/database/*` |
| `docs/operations/development.md` | `docs/reference/development.md` |
| `docs/guides/configuration.md` | `docs/reference/configuration.md` |
| `docs/guides/deployment.md` | `docs/reference/deployment.md` |
| `docs/guides/testing.md` | `docs/reference/testing.md` |
| `docs/guides/content-processing.md` | `docs/reference/content-processing.md` |
| `docs/guides/reading-preferences.md` | `docs/reference/reading-preferences.md` |
| `docs/guides/frontend-features.md` | `docs/reference/frontend-features.md` |

### Scenario: Architecture docs moved
- **WHEN** 迁移完成后查看 `docs/reference/architecture/`
- **THEN** 包含 overview.md、backend.md（原 backend-go.md）、frontend.md、frontend-components.md、data-flow.md、runtime.md（原 backend-runtime.md）、tracing.md

## Requirement: Getting started promotion
`docs/guides/getting-started.md` SHALL 提升到 `docs/getting-started.md`（docs 根目录），作为新用户入口。

### Scenario: New user entry point
- **WHEN** 新用户打开 docs 目录
- **THEN** `docs/getting-started.md` 直接可见，无需进入子目录
