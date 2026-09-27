## 1. 后端图片代理

- [x] 1.1 新建 `backend-go/internal/platform/imageproxy/` 包实现 `GET /api/image-proxy` 核心：`url` 参数校验（缺失 / 非 http(s) / 指向代理自身 host:port 一律 4xx 且零上游请求）、Referer 注入（默认图片自身 `scheme://host/`，预留 per-host 覆盖表）、常量浏览器 UA、上游 15s 超时与非 200 状态码透传；配套 httptest 单测覆盖三类拒绝与 Referer 断言。验证：`cd backend-go && go test ./internal/platform/imageproxy/` 通过。
- [x] 1.2 同包实现磁盘缓存：`sha256(原始URL)` 命名落 `data/image-cache/`，命中 touch mtime 作 LRU，仅 200 且 `Content-Type` 为 image/* 落盘（其余透传不缓存），目录总量超上限按 mtime 淘汰至 90% 水位，上限默认 256MB 由 `IMAGE_CACHE_MAX_MB` 覆盖，命中响应带 `X-Image-Proxy-Cache: HIT`；单测覆盖命中不再打上游、`text/html` 不缓存、超限淘汰保留新文件。验证：同上 `go test` 通过。
- [x] 1.3 在 `backend-go/internal/app/router.go` 挂载 `GET /api/image-proxy`（中间件/CORS 与现有 `/api` 路由一致），补路由级测试。验证：`go test ./internal/app/` 通过。
- [x] 1.4 确认缓存目录被 git 忽略（`data/` 覆盖不足则补 `.gitignore` 条目 `data/image-cache/`）。验证：`git check-ignore -v data/image-cache/probe` 命中规则。

## 2. 前端统一改写

- [x] 2.1 新增 `front/app/utils/imageProxy.ts` 导出 `proxiedImageUrl(url)`：`http(s)` 外链改写为同源 `/api/image-proxy?url=${encodeURIComponent(url)}`；空值、相对路径、`data:`、`blob:`、已含 `/api/image-proxy` 前缀的地址原样返回；单测覆盖上述全部分支（对应 ui-design 验收映射）。验证：`cd front && pnpm test:unit app/utils/imageProxy.test.ts --maxWorkers=2` 通过。
- [x] 2.2 `ArticleCardView.vue` 的 `coverSrc` computed 接入 `proxiedImageUrl`，保留 `@error` 降级 FeedIcon 逻辑不动。验证：`pnpm test:unit app/features/articles/components/ArticleCardView.test.ts --maxWorkers=2` 既有降级用例通过。
- [x] 2.3 `ArticleContentPreviewPanel.vue` 头图 `articleImageUrl` 接入改写。验证：`pnpm exec nuxi typecheck` 无该文件报错、既有测试通过。
- [x] 2.4 正文渲染管线接入：`useArticleContentView.ts` / `utils/markdown.ts` 渲染产物对 `<img src>` 做 `proxiedImageUrl` 改写（沿用既有 img 正则处理风格），补单测（外链 img 被改写、data: 与相对路径不动）。验证：对应 `pnpm test:unit` 文件通过。
- [x] 2.5 侦探墙 `CardGroup.ts`：`nodeImageUrl` 出口接入改写，并删除 `image.referrerPolicy = 'no-referrer'`。验证：`grep -rn "no-referrer" front/app --include='*.ts' --include='*.vue'` 零命中。
- [x] 2.6 排查残余外链图片加载点：`grep -rn ":src=\|<img" front/app --include='*.vue'` 逐条确认为已改写、非外链或无需改写（`FeedIcon.vue` 重点确认）。验证：排查清单记录在本任务下，零未处理外链直连。
  - 排查清单（2026-09-22）：`ArticleCardView:125` ✓已改写｜`ArticleContentPreviewPanel:122` ✓已改写｜`FeedIcon:58` ✓已改写（仅 isUrl 外链分支走代理；isLocalPath 后端自有地址保持直连，避免代理自指 400；`FeedIcon.test.ts` 本地路径断言原样锁定该行为）｜`ArticleIframeView:32` 不适用——iframe 加载文章原页整页，非图片，属 Non-Goals（仅图片 GET）｜`CardGroup:286` ✓已改写｜`TopicWallScene:206`/`SetDressing:115` 不适用——均为本地 `/textures/` 相对路径（F4 原样）｜`background-image:url(http` 零命中｜`new Image()` 仅 CardGroup 一处已改写。零未处理外链图片直连。

## 3. 测试

- [x] 3.1 `bash scripts/harness/change-scope.sh` 确定影响包后按输出跑目标 Go 测试，不顺手全量。（已跑：本 change Go 影响面 = `internal/platform/imageproxy` 新包 + `internal/app` 挂载，`go test ./internal/app/ ./internal/platform/imageproxy/` 绿；输出中其余 domain 包为其他 active change 脏文件，按规范忽略）
- [x] 3.2 前端受影响文件测试用 `pnpm test:unit <文件...> --maxWorkers=2`（参数不带 `--`），全量留归档/pre-push 按规范执行。（已跑：imageProxy/ArticleCardView/PreviewPanel/FeedIcon 4 文件 41 tests + BoardEnrichmentPanel×2/articleContentGuards 3 文件 30 tests 绿）
- [x] 3.3 spec Scenario 落点核对：10 个 delta Scenario 全部展开，映射表见 §5 验证节（scenario-trace 对账用，表头/位置按 scenario-trace-gate 约定），计划不冒充证据。

- [x] 3.4 人工：后端起来后 `curl --noproxy '*' -o /dev/null -w '%{http_code} %{content_type}\n' "http://localhost:5100/api/image-proxy?url=<urlencoded sspai 图>"`，预期 `200 image/...`；再开侦探墙看该图贴图正常、Network 中对该图床无直连 403。（实测 200 image/webp 37628B + MISS→HIT；侦探墙十余条贴图全经代理 200 零直连。证据 `evidence/verification.md`）

## 4. 文档

<!-- doc-impact: flow, api, configuration -->

- [x] 4.1 apply 启动时先跑 `bash scripts/harness/doc-impact.sh suggest add-image-proxy` 与 `context`，以 suggest 结果核对本节域声明（上表为核心域，图片链路触及的 flow 文档以工具输出为准补全），再注入 flow 业务约束。（已跑；`context` 子命令已退役由 constraint-injection 扩展取代，约束注入正常；声明已按 suggest 域枚举修正为 flow, api, configuration）
- [x] 4.2 `docs/reference/flow/reading.md`、`docs/reference/flow/content-enrichment.md` 补「外链图片经 `/api/image-proxy` 加载、Referer 注入缘由、原始 URL 仍存库」的链路说明与变更溯源。（链路说明已补，reading.md FeedIcon「直连」过时描述同步修正；变更溯源表行归档后按 §12 补，归档前填会是死链）
- [x] 4.3 `docs/reference/configuration.md` 记录 `IMAGE_CACHE_MAX_MB`（默认 256）与缓存目录 `data/image-cache/`（含整目录删除热清缓存的操作说明）。
- [x] 4.4 `docs/reference/api/` 补 `GET /api/image-proxy`（参数、Referer 注入行为、错误码 400/502/504、`X-Image-Proxy-Cache` 头）。（api/system.md 新节 + _index.md 索引行）

## 5. 验证

- [x] 5.1 `openspec validate add-image-proxy --strict`，预期通过。（valid）
- [x] 5.2 `bash scripts/harness/doc-impact.sh verify openspec/changes/add-image-proxy` 与 `bash scripts/harness/check-standards.sh --change add-image-proxy`，预期文档对账无遗漏。（doc-impact：通过，声明 flow/api/configuration 3 文件对账一致；check-standards 归档前复跑）
- [x] 5.3 `cd backend-go && golangci-lint run ./... && go vet ./... && go build ./...`，预期通过；测试只跑 3.1 确定的范围。（lint 0 issues / vet 通过 / build ok）
- [x] 5.4 `cd front && pnpm lint && pnpm exec nuxi typecheck`，预期通过；`pnpm test:unit <受影响文件> --maxWorkers=2` 预期绿。（lint 0 errors；typecheck exit 0；受影响 7 测试文件 71 tests 绿）
- [x] 5.5 3.4 的 curl 实测与侦探墙人工验收留截图/输出记录，作为「不再 403」的验收证据。（`evidence/verification.md` + `evidence/list-covers-proxied.png`；侦探墙 WebGL 截图 CDP 超时，以 Network 输出记录为证）
- [x] 5.6 `bash scripts/dev/deploy-frontend.sh`，预期静态部署与健康检查通过（串行于测试之后，不与测试并行）。（部署完成，/health 200，入口 http://localhost:5100/）
- [x] 5.7 `bash scripts/harness/test-patrol.sh --report`，预期无本 change 未解阻塞；归档另获用户指令。（--report 全绿，各分片 last_ok=1；归档待用户指令）
- [x] 5.8 `bash scripts/harness/scenario-trace.sh openspec/changes/add-image-proxy` → exit 0（10 Scenario：自动 9 / 人工 1）；`check-standards.sh --change add-image-proxy` 的 A-D/F/G/I 段零失败（E 段 15 个未溯源欠账均为 2026-09-17~19 其他已归档 change 的 §12 存量欠账，非本 change 引起，域外留痕不代修；E 段本 change 行按 §12 归档后补）；域外无测试红需记账（test-patrol --report 全绿）。

### Scenario 映射

| Scenario | 测试文件 |
| --- | --- |
| 禁空 Referer 图床经代理成功 | backend-go/internal/platform/imageproxy/handler_test.go |
| 非法 url 被拒绝 | backend-go/internal/platform/imageproxy/handler_test.go |
| 上游拒绝时透传状态码 | backend-go/internal/platform/imageproxy/handler_test.go |
| 二次请求命中缓存 | backend-go/internal/platform/imageproxy/cache_test.go |
| 缓存超限滚动淘汰 | backend-go/internal/platform/imageproxy/cache_test.go |
| 非图片响应不缓存 | backend-go/internal/platform/imageproxy/cache_test.go |
| 外链封面改写为代理地址 | front/app/utils/imageProxy.test.ts |
| 非外链地址不改写 | front/app/utils/imageProxy.test.ts |
| 侦探墙贴图不再 403 | 人工：静态 :5100 侦探墙 Network 断言（3.4/5.5，证据 evidence/verification.md） |
| 代理透传 403 时封面降级 | front/app/features/articles/components/ArticleCardView.test.ts |
| （补充）外链 favicon 走代理、本地路径直连 | front/app/components/feed/FeedIcon.test.ts |
