## MODIFIED Requirements

### Requirement: Reference directory structure
docs/reference/ SHALL 包含以下子目录和文件：
- architecture/ — 系统总览、后端架构、前端架构、数据流、运行时、链路追踪
- api/ — API 参考文档（按路由前缀拆分）
- database/ — 数据库字段参考
- development.md — 开发规范（构建、测试、代码风格、目录约定、提交检查）
- 其他跨里程碑功能指南（configuration.md、deployment.md、testing.md 等）

以下功能说明文档 SHALL NOT 出现在 docs/reference/ 中：
- frontend-features.md — 已移至 docs/archive/，内容拆分到 docs/userguide/
- content-processing.md — 已移至 docs/archive/，用户可见部分拆分到 docs/userguide/reading.md
- reading-preferences.md — 已移至 docs/archive/，用户可见部分拆分到 docs/userguide/reading.md

#### Scenario: Reference directory listing
- **WHEN** 列出 docs/reference/
- **THEN** 可见 architecture/、api/、database/ 目录和 development.md、configuration.md、deployment.md、testing.md、开发执行规范.md 等文件
- **THEN** 不存在 frontend-features.md、content-processing.md、reading-preferences.md

### Requirement: Reference docs are living documents
docs/reference/ 下的文档 SHALL 反映当前系统真实状态。architecture/ 下的代码路径引用 SHALL 与实际代码目录一致。

#### Scenario: Backend architecture paths are correct
- **WHEN** 阅读 docs/reference/architecture/backend.md
- **THEN** 引用 backend-go/internal/domain/content/ 而非 contentprocessing/
- **THEN** 引用 backend-go/internal/domain/tagging/ 而非 topicanalysis/ 或 topicextraction/

#### Scenario: Frontend architecture routes are correct
- **WHEN** 阅读 docs/reference/architecture/frontend.md
- **THEN** 引用 front/app/pages/tags.vue 作为标签管理页面
- **THEN** 不引用已删除的 pages/digest/ 路由

## REMOVED Requirements

### Requirement: Duplicate architecture files removal
**Reason**: 此要求已在之前的 milestone 中完成，无需重复执行。
**Migration**: 保持现状，不再检查。

### Requirement: Existing docs migration to reference
**Reason**: 迁移已完成，本变更做的是反向操作——将不属于 reference 的功能文档迁出。
**Migration**: 功能文档迁至 docs/userguide/ 和 docs/archive/。
