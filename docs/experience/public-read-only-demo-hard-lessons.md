# 公开只读 Demo 本地构建与上线经验

这篇记录沉淀 `public-read-only-demo` 变更里最硬的几个问题：本地镜像构建、脱敏 seed 导入、只读 demo 行为、以及验证门禁。目标是下次开放公网 demo 时，不需要重新踩一遍同样的坑。

## 场景

公开 demo 需要把 Syntopica 打成一个自包含镜像：

- 前端静态产物内置在镜像里，通过 `/api` 访问同容器后端；
- 后端连接 compose 内的 PostgreSQL + pgvector；
- 容器首次启动时导入 `demo/seed/seed.sql`；
- `DEMO_READ_ONLY=1` 禁止写入、后台任务和 WebSocket；
- demo 数据来自真实库快照，但必须脱敏、去凭证、去向量大字段。

这个任务表面是“写 Dockerfile + compose”，实际难点在构建网络、启动顺序、数据库默认初始化、脱敏后约束冲突，以及验证口径的一致性。

## 硬骨头 1：Go 依赖下载在 Docker build 里不稳定

现象：

```text
go mod download
github.com/gabriel-vasile/mimetype@v1.4.13: unexpected EOF
```

根因不是代码编译失败，而是 Docker build 阶段访问默认 Go proxy 不稳定。后续还遇到 `# syntax=docker/dockerfile:1` 触发额外镜像拉取，Docker mirror 同样可能 EOF。

处理方式：

- 在 Go build stage 设置可覆盖的 `GOPROXY`，默认使用国内代理；
- 给 `go mod download` 加短重试；
- 移除不需要的 Dockerfile `# syntax` 行，减少构建期外部依赖。

经验：

- 先区分“编译失败”和“构建期网络失败”。如果错误发生在 `go mod download` 或拉 Dockerfile frontend，多半不是业务代码问题。
- 公网 demo 的 Dockerfile 要尽量少依赖构建期网络的隐式步骤。
- 可覆盖的 `ARG GOPROXY=...` 比写死环境更适合跨环境协作。

## 硬骨头 2：AutoMigrate 默认数据与 seed 导入冲突

现象：

demo 容器反复 restart，日志显示 seed 导入阶段出现 duplicate key，例如 `ai_settings`、`categories` 已存在。

根因：

entrypoint 为了等表结构 ready，会先启动后端。后端启动时不仅 AutoMigrate，还会写入一批默认数据。随后 `psql -f seed.sql` 再导入真实快照，就和默认数据撞唯一键。

处理方式：

- 后端先启动，等待 `/health` 确认迁移完成；
- seed 导入前显式 `TRUNCATE` demo 涉及的数据表；
- 再导入 `seed.sql`；
- 最后 `wait` 后端进程，容器保持前台。

经验：

- “等迁移完成”不等于“数据库仍为空”。如果应用启动会 bootstrap 默认数据，seed 导入必须先清场。
- `TRUNCATE ... RESTART IDENTITY CASCADE` 比按表手动 delete 更适合 demo fresh DB。
- 清场表必须覆盖所有被默认初始化或 seed 涉及的表，否则问题会变成偶发重复键。

## 硬骨头 3：脱敏会制造新的唯一约束冲突

现象：

导入 `feeds` 时撞 `uni_feeds_url`。原始 URL 本来不同，但脱敏时剥掉 query 后变成同一个 URL。

根因：

脱敏不是只“删敏感信息”，它也会改变数据分布。URL query、token、外部 ID 被清理后，原本唯一的数据可能合并成同一个值。

处理方式：

- `dump-sanitizer` 的 `ExportSpec` 增加 `ConflictClause`；
- `feeds` 使用 `ON CONFLICT (url) DO NOTHING`；
- 当前 seed 也补上同样冲突处理，保证 demo 可导入。

经验：

- 每个脱敏字段都要想一遍：清理后是否会影响唯一索引、外键、排序或业务分组。
- URL、邮箱、外部资源 ID 是最容易因脱敏产生碰撞的字段。
- 对 demo 快照来说，少量去重通常可接受，但必须是显式决策，不要靠导入失败来发现。

## 硬骨头 4：敏感 token 不只在凭证字段里

现象：

安全抽查时，`api_key` 不只出现在 `ai_providers.api_key` 列名附近，还出现在文章正文和配置 JSON 文本中。

根因：

真实业务数据会讨论技术内容，正文里可能自然包含 `api_key`、`API_KEY` 等字符串；配置类 JSON 也可能带内部地址或字段名。只清空凭证列不够。

处理方式：

- `ai_settings.value` 在 demo seed 中统一写成 `{}`；
- 对 `articles.content`、`articles.ai_content_summary` 增加 token 文本替换，再做截断；
- 抽查 seed：`api_key` 只允许作为 `INSERT INTO ai_providers (...,api_key,...)` 的列名出现，VALUES 侧为空字符串。

经验：

- 安全检查不要只搜“真实 key 值”，也要搜敏感字段名、内部域名、内网 IP、token 形态。
- 技术文章正文可能包含看似敏感的字面量；公网 demo 宁可保守替换。
- 配置 JSON 最适合整体清空，避免陷入字段级别猜测。

## 硬骨头 5：只读 demo 不只是拦 POST

demo 模式最终需要同时满足：

- 写请求返回 `405 {"error":"read-only demo"}`；
- `OPTIONS` 预检放行；
- 流式 merge preview 这类 GET 写语义接口也要拦；
- 后台任务不启动；
- `/ws` 不注册，让前端静默降级；
- 生产模式 unset `DEMO_READ_ONLY` 时保持透传。

经验：

- 只读中间件不能只按 HTTP method 判断。项目里可能存在 GET stream、scan、evaluate 这类带副作用或高成本语义的接口。
- WebSocket 和后台任务属于 demo 的“隐性写入面”，也要关掉。
- 必须有非 demo 模式测试，防止中间件影响本地正常开发和生产。

## 硬骨头 6：5000 端口可能不是 demo 容器

现象：

验证初期访问 `localhost:5000` 命中了本地已有服务，导致结果和容器状态对不上。

处理方式：

- 先确认 `docker compose -f demo/docker-compose.demo.yml ps` 的端口绑定；
- 如果本地已有服务占用 5000，先关掉本地服务，或用 `PORT=其他端口` 启 demo；
- HTTP 验证优先使用 `127.0.0.1:5000`，减少解析差异。

经验：

- “curl 有响应”不代表响应来自目标容器。
- 端到端验证前先看 compose ps、容器日志、响应体特征三件套。
- 本地服务和 demo 容器共用默认端口时，排障会非常容易跑偏。

## 推荐排查顺序

1. `docker compose version`：先确认 Docker 可用。
2. `cd backend-go && go build ./...`：确认不是 Go 代码基线坏了。
3. `docker compose -f demo/docker-compose.demo.yml build syntopica-demo`：单独看镜像构建。
4. `docker compose -f demo/docker-compose.demo.yml up -d --no-build`：构建和启动分开看。
5. `docker compose -f demo/docker-compose.demo.yml logs --tail=120 syntopica-demo`：优先看 entrypoint、seed import、后端启动日志。
6. `curl http://127.0.0.1:5000/health`：确认命中 demo 容器。
7. 验证读接口、写接口、`/ws`：

```powershell
curl.exe -s -w "`nHTTP_STATUS:%{http_code}`n" http://127.0.0.1:5000/health
curl.exe -s -w "`nHTTP_STATUS:%{http_code}`n" http://127.0.0.1:5000/api/categories
curl.exe -s -X POST -H "Content-Type: application/json" -d "{}" -w "`nHTTP_STATUS:%{http_code}`n" http://127.0.0.1:5000/api/categories
curl.exe -s -w "`nHTTP_STATUS:%{http_code}`n" http://127.0.0.1:5000/ws
```

## 归档前检查清单

- `go vet ./...` 通过；
- `go test ./internal/platform/middleware` 通过；
- `go build ./...` 通过；
- `golangci-lint run ./...` 无新增问题；如果仓库已有失败项，需要在归档说明中标明；
- `seed.sql` 中无 `INSERT INTO ai_call_logs`；
- `seed.sql` 中无 `INSERT INTO schema_migrations`；
- `api_key` 只允许作为列名出现，VALUES 侧必须为空或已脱敏；
- 向量列应为 `NULL`，避免 seed 体积膨胀；
- `docker compose -f demo/docker-compose.demo.yml up -d --build` 后两个容器 healthy；
- 首页、`/topics`、`/tags`、`/settings` 能打开；
- `front/` 不应有本 change 引入的 diff。

## 这次仍需注意的尾巴

这次 demo 功能已跑通，但归档前还有几个门禁要单独处理：

- 本地真实运行库和近期数据前置条件需要重新确认；
- `go run ./cmd/dump-sanitizer` 需要在真实库上重新生成一次 seed；
- `golangci-lint run ./...` 当前失败项来自既有代码，需要修掉或在归档说明中明确；
- 当前 `front/` 有无关 dirty changes，因此不能把前端零改动门禁直接标绿；
- 归档说明需要注明本 change 不涉及 `architecture`、`api`、`database` 的结构性 reference 更新。

## 一句话原则

公开 demo 的难点不是“能不能跑起来”，而是让构建、导入、脱敏、只读和验证都可重复。每个临时修复都要落到 Dockerfile、entrypoint、sanitizer 或 tasks 门禁里，否则下一次构建就会把同一个问题重新翻出来。

## 另见

- [`2026-09-22-remote-demo-deploy-pitfalls.md`](2026-09-22-remote-demo-deploy-pitfalls.md)：首次把 demo 部署到远程服务器（10.11.12.59）的 7 个坑（seed 列白名单缺口 / 清场缺表重启循环 / pnpm patch 静默丢失 / OOM / CRLF / 时间窗语义 / psql 无 dry-run）。
