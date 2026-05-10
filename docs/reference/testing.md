# 测试指南

本项目有三套独立的测试套件，分别覆盖 Go 后端、Nuxt 前端和跨系统集成流程。

## 测试框架与配置

| 套件 | 框架 | 位置 | 语言 |
|-------|-----------|----------|----------|
| 后端单元测试 | Go `testing` + `testify` | `backend-go/**/*_test.go` | Go |
| 前端单元测试 | Vitest + Vue Test Utils | `front/app/**/*.test.ts` | TypeScript |
| 前端 E2E 测试 | Playwright | `front/tests/e2e/*.spec.ts` | TypeScript |
| 集成测试 | pytest | `tests/workflow/` | Python |
| Firecrawl 检查 | Python 脚本 | `tests/firecrawl/` | Python |

### 后端（Go）

测试使用标准 `testing` 包和 `github.com/stretchr/testify` 断言。每个测试文件以 `*_test.go` 形式与源码放在一起。多数测试通过 `gorm.Open(sqlite.Open("file:...?mode=memory&cache=shared"))` 创建内存 SQLite 数据库，并自动迁移所需模型，因此不需要外部数据库。

> 注意：单元测试使用内存 SQLite（通过 `glebarez/sqlite` 驱动）进行隔离，而生产环境使用 PostgreSQL。

### 前端单元（Vitest）

Vitest 使用 `happy-dom` 作为 DOM 环境。配置在 `front/vitest.config.ts`。测试文件以 `*.test.ts` 命名约定与源文件同目录放在 `front/app/` 下。

### 前端 E2E（Playwright）

Playwright 配置在 `front/playwright.config.ts`。它在 `http://localhost:3000` 启动 Nuxt 开发服务器，然后运行浏览器测试。测试串行执行（`fullyParallel: false`、`workers: 1`）以保证启动稳定性。

### 集成测试（pytest）

`tests/workflow/` 中的集成测试验证端到端调度器行为和数据流。它们需要 Go 后端运行在 `http://localhost:5000`。配置在 `tests/workflow/config.py`。

## 运行测试

### 后端

```bash
# 所有后端测试
cd backend-go
go test ./...

# 单个包（详细输出）
go test ./internal/domain/feeds -v

# 按名称运行单个测试
go test ./internal/domain/feeds -run TestBuildArticleFromEntryTracksOnlyRunnableStates -v
```

### 前端单元测试

```bash
cd front

# 运行所有单元测试
pnpm test:unit

# 单个测试文件
pnpm test:unit -- app/utils/articleContentSource.test.ts

# 按名称模式运行单个测试
pnpm test:unit -- app/utils/articleContentSource.test.ts -t "prefers firecrawl"
```

### 前端 E2E 测试

```bash
cd front

# 运行所有 E2E 测试（自动启动开发服务器）
pnpm test:e2e

# 使用 Playwright UI 运行
pnpm test:e2e:ui

# 列出测试但不运行
pnpm test:e2e:list

# 传递额外 Playwright 参数
pnpm test:e2e:args -- --grep "topic-graph"
```

### Python 集成测试

```bash
# 设置环境（首次）
cd tests/workflow
uv venv
.venv\Scripts\activate    # Windows
uv pip install -r requirements.txt

# 运行所有集成测试
pytest test_*.py -v

# 单个文件
pytest test_schedulers.py -v

# 单个测试
pytest test_schedulers.py::TestAutoRefreshScheduler::test_scheduler_exists -v

# 带覆盖率
pytest --cov=. --cov-report=html
```

集成测试需要 Go 后端运行在 `http://localhost:5000`。从单独的终端启动：

```bash
cd backend-go
go run cmd/server/main.go
```

### Firecrawl 集成检查

独立的脚本验证 Firecrawl 服务连通性、抓取功能、文章内容更新和 AI 配置：

```bash
cd backend-go
go run cmd/server/main.go    # 在单独终端启动后端

cd tests/firecrawl
python test_firecrawl_integration.py
```

## 编写新测试

### 后端

- 测试文件放在源码旁，作为同包的 `*_test.go`。
- 多个用例共享相同逻辑时使用表驱动测试。
- 需要数据库的测试，创建内存 SQLite 实例并对所需模型调用 `AutoMigrate`。参考 `backend-go/internal/domain/feeds/service_test.go` 的 `setupFeedsTestDB` 模式。
- 仅当文件已经使用 `testify` 时才导入 `github.com/stretchr/testify`；否则直接使用标准 `testing` 包。
- 在 setup 函数中使用 `t.Helper()` 获得更清晰的错误追踪。

### 前端单元测试

- 测试文件与源文件同目录：`front/app/<path>/file.test.ts`。
- 使用 Vitest 的 `describe`/`it` 块：
  ```typescript
  import { describe, expect, it } from 'vitest'
  import { myFunction } from './myFunction'

  describe('myFunction', () => {
    it('does the expected thing', () => {
      expect(myFunction('input')).toBe('expected')
    })
  })
  ```
- 测试在 `happy-dom` 环境中运行 — 不需要真实浏览器。
- `front/tests/e2e/` 中的 E2E 测试文件通过 `vitest.config.ts` 排除在 Vitest 之外。

### 前端 E2E 测试

- spec 文件放在 `front/tests/e2e/*.spec.ts`。
- 使用 Playwright 的 `test` 和 `expect` 导入：
  ```typescript
  import { test, expect } from '@playwright/test'

  test('page loads', async ({ page }) => {
    await page.goto('/some-page')
    await expect(page.locator('body')).toBeVisible()
  })
  ```
- 测试按顺序在 Chromium 上运行。开发服务器通过 `playwright.config.ts` 的 `webServer` 配置自动启动。

### 集成测试

- 测试文件放在 `tests/workflow/test_*.py`。
- 使用 `tests/workflow/utils/` 中的共享辅助工具：
  - `DatabaseHelper` — 直接数据库访问，用于断言数据库状态
  - `APIClient` — HTTP 客户端，用于调用后端 API 端点
  - `MockFirecrawl` / `MockAIService` — 模拟外部服务响应
- 每个测试类使用 `pytest.fixture(autouse=True)` 进行 setup/teardown，创建测试数据并在结束后清理。
- 测试配置见 `tests/workflow/config.py` 中的 `TestConfig` 和 `DatabaseConfig`。

## 覆盖率

没有配置最低覆盖率阈值。

- **前端**：Vitest 在 `vitest.config.ts` 中没有覆盖率设置。运行 `pnpm test:unit` 只得到通过/失败结果。
- **后端**：Go 测试命令中没有 `cover` 配置文件或阈值标志。
- **集成**：`pytest-cov` 在 `tests/workflow/requirements.txt` 中列为依赖。使用 `pytest --cov=. --cov-report=html` 生成报告。

## CI 集成

当前没有配置 CI/CD 流水线。仓库中没有 `.github/workflows/` 文件。

所有测试本地运行：

- **后端**：`backend-go/` 目录下运行 `go test ./...`
- **前端单元**：`front/` 目录下运行 `pnpm test:unit`
- **前端类型检查**：`front/` 目录下运行 `pnpm exec nuxi typecheck`
- **前端 E2E**：`front/` 目录下运行 `pnpm test:e2e`
- **集成**：`tests/workflow/` 目录下运行 `pytest test_*.py -v`（需要运行中的后端）

推荐的推送前验证序列：

```bash
cd backend-go && go test ./... && go build ./...
cd front && pnpm exec nuxi typecheck && pnpm test:unit && pnpm build
```

## 测试文件结构

```
backend-go/
├── internal/domain/feeds/service_test.go                 # Feed 解析、文章创建
├── internal/domain/summaries/summary_queue_test.go
├── internal/domain/summaries/ai_prompt_builder_test.go
├── internal/domain/digest/generator_test.go              # Digest 生成
├── internal/domain/digest/scheduler_test.go
├── internal/domain/digest/handler_test.go
├── internal/domain/digest/integration_test.go
├── internal/domain/digest/obsidian_test.go
├── internal/domain/digest/feishu_test.go
├── internal/domain/contentprocessing/firecrawl_job_queue_test.go
├── internal/domain/contentprocessing/content_completion_service_test.go
├── internal/domain/contentprocessing/content_completion_handler_test.go
├── internal/domain/topicextraction/tag_job_queue_test.go
├── internal/domain/topicextraction/metadata_test.go
├── internal/domain/topicextraction/extractor_test.go
├── internal/domain/topicgraph/handler_test.go
├── internal/domain/topicanalysis/analysis_queue_test.go
├── internal/domain/topicanalysis/abstract_tag_service_test.go
├── internal/domain/topicanalysis/embedding_test.go
├── internal/domain/topicanalysis/merge_tags_reembedding_test.go
├── internal/domain/topicanalysis/merge_reembedding_queue_test.go
├── internal/domain/preferences/handler_test.go
├── internal/domain/aiadmin/handler_test.go
├── internal/domain/narrative/collector_test.go
├── internal/domain/narrative/generator_test.go
├── internal/jobs/auto_refresh_test.go                    # 后台任务处理器
├── internal/jobs/firecrawl_test.go
├── internal/jobs/content_completion_test.go
├── internal/jobs/preference_update_test.go
├── internal/jobs/auto_summary_test.go
├── internal/jobs/handler_test.go
├── internal/jobs/scheduler_status_response_test.go
├── internal/jobs/trigger_now_status_code_test.go
├── internal/jobs/tag_quality_score_test.go
├── internal/platform/database/db_test.go                 # 数据库和迁移
├── internal/platform/database/datamigrate/writer_postgres_test.go
├── internal/platform/database/datamigrate/verify_test.go
├── internal/platform/config/config_test.go
├── internal/platform/airouter/router_test.go             # AI 模型路由
├── internal/platform/airouter/store_test.go
├── internal/platform/airouter/migration_test.go
├── internal/platform/opennotebook/client_test.go
├── internal/platform/aisettings/config_store_test.go
└── cmd/migrate-db/main_test.go

front/
├── app/utils/api.test.ts                                 # API 客户端工具
├── app/utils/articleContentSource.test.ts                # 内容源解析
├── app/utils/articleContentGuards.test.ts
├── app/utils/schedulerMeta.test.ts
├── app/features/articles/components/ArticleTagList.test.ts
├── app/features/digest/components/digestLayout.test.ts
├── app/features/topic-graph/utils/buildDisplayedTopicGraph.test.ts
├── app/features/topic-graph/utils/topicGraphCanvasLinks.test.ts
├── app/features/topic-graph/utils/buildTopicGraphViewModel.test.ts
├── app/features/topic-graph/components/TopicTimeline.test.ts
└── tests/e2e/
    ├── baseline.spec.ts                                  # 冒烟测试
    └── topic-graph.spec.ts                               # Topic graph E2E

tests/
├── workflow/
│   ├── test_schedulers.py                                # 调度器单元测试
│   ├── test_workflow_integration.py                      # 端到端工作流测试
│   ├── test_error_handling.py                            # 错误处理测试
│   ├── utils/                                            # 共享测试辅助
│   │   ├── database.py                                   # DatabaseHelper
│   │   ├── api_client.py                                 # APIClient
│   │   └── mock_services.py                              # MockFirecrawl, MockAIService
│   └── config.py                                         # 测试配置
└── firecrawl/
    └── test_firecrawl_integration.py                     # Firecrawl 集成检查
```
