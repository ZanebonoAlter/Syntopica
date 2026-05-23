## Why

项目已有 Go 单元测试、Vitest 前端测试、Python UAT 和 Playwright E2E，但缺少"用户故事闭环"级别的验收测试。每次变更完成后靠手动验证，无法系统化回归。`unify-tag-hierarchy` 变更已完成 16 个 task 组共 132 项子任务，但无任何 E2E 验收覆盖其核心用户故事。

## What Changes

- 新增 `tests/acceptance/` 目录，建立 Python + uv + pytest + Playwright 的变更验收测试框架
- 验收测试按变更名称组织，每个变更包含 API 级别测试（后端 happy path）和 UI 级别测试（浏览器操作闭环）
- 真实环境运行：Go 后端 + Nuxt 前端 + 本地 LLM，不 mock 任何服务
- API 测试数据真实入库，UI 测试可见 API 测试创建的数据
- 选择器集中管理（helpers/selectors.py），后续按需补 data-testid
- 首个验收套件覆盖 `unify-tag-hierarchy` 变更的 10 个用户故事

## Capabilities

### New Capabilities

- `acceptance-framework`: 验收测试框架基础设施 — uv 项目配置、conftest 层次（环境就绪检查、Playwright fixtures）、API client 扩展、浏览器导航 helper、选择器常量管理
- `acceptance-api-stories`: API 级别用户故事验收 — Sector CRUD、层级配置读写、重建任务触发与进度、PendingChange 审批，每个 story 验证后端 API 的 happy path 闭环
- `acceptance-ui-stories`: UI 级别用户故事验收 — /tags 页面加载、Sector 列表交互、手动创建 Sector、层级树过滤、模板修改与重建进度、PendingChange 审批面板

### Modified Capabilities

## Impact

- **新增目录**: `tests/acceptance/`（pyproject.toml、conftest.py、helpers/、changes/）
- **新增依赖**: pytest、playwright（Python 版）、requests（已有）
- **无现有代码变更**: 纯新增测试，不改动 front/ 或 backend-go/ 代码
- **运行依赖**: Go 后端 localhost:5000 + Nuxt 前端 localhost:3000 + 本地 LLM 服务
- **数据影响**: 测试创建的 Sector 等数据真实入库，测试挂掉需用户手动清理
