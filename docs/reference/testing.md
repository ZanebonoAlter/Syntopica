# 测试指南

> **规范已迁移**：测试框架、分层约定、编写规范、集成测试陷阱、DSN 安全红线的**权威定义**已迁至 [`standard/`](standard/)：
> - 后端：[`standard/backend/testing.md`](standard/backend/testing.md)（含 testcontainer、`SetupTestDB`、vector 列陷阱、🛑 DSN 安全红线）
> - 前端：[`standard/frontend/testing.md`](standard/frontend/testing.md)（Vitest + Playwright）
>
> 本文件只保留**运行命令速查**，方便快速查询。

## 运行命令速查

### 后端（在 `backend-go/` 目录执行）

```bash
go test ./...                                      # 全部（含集成测试，需 Docker）
go test -short ./...                               # 仅单元测试
go test ./internal/reader/service -v               # 单个包（详细输出）
go test ./internal/reader/service -run TestName -v # 按名称运行单个测试
```

### 前端（在 `front/` 目录执行）

```bash
pnpm test:unit                                                     # 所有单元测试
pnpm test:unit -- app/utils/articleContentSource.test.ts           # 单个测试文件
pnpm test:unit -- app/utils/articleContentSource.test.ts -t "prefers firecrawl"  # 按名称
pnpm test:e2e                                                      # 所有 E2E（自动启动 dev server）
pnpm test:e2e:ui                                                   # Playwright UI
```

### Python 集成测试（在 `tests/workflow/` 目录执行）

需后端运行在 `localhost:5100`（WSL 侧探测加 `--noproxy '*'` 或设 `no_proxy=localhost,127.0.0.1,::1`，防系统代理劫持）：

```bash
uv venv
.venv\Scripts\activate    # Windows
uv pip install -r requirements.txt
pytest test_*.py -v
```

---

> 🛑 **后端 DSN 安全红线**：`testutil` 无默认 DSN、不读环境变量，禁止重引入「默认数据库连接」（历史事故曾清空业务数据）。全文见 [`standard/backend/testing.md`](standard/backend/testing.md)。
