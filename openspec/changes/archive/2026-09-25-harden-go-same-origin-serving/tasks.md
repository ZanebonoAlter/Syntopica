## 1. 用例先行（复杂档）

- [x] 1.1 复核 `test-cases.md` §3「继承与调整」：确认反代形态相关旧判据的替代方案（Go 单进程等价判据），并在 `evidence/pre-change-baseline.md` 记录合并前基线（首屏 2.06 MB / 40 请求、`entry.css` 640.8 KB、`entry.js` 295.9 KB、`/api/articles?per_page=100` 1125 KB、`favicon.png` 321.6 KB、常驻对账 ≈10 次/分钟）。验证：文件存在且四项字节 + 一项频次均有实测命令与数值
- [x] 1.2 把 `test-cases.md` §5.1 压缩分支表落到测试骨架（先写会失败的断言）。验证：`cd backend-go && go test ./internal/platform/middleware/ -run TestCompress` 出现预期失败（红）而非编译错误

## 2. 后端：压缩中间件

- [x] 2.1 新增 `internal/platform/middleware/compress.go`：按 `Accept-Encoding` 协商 gzip（标准库 `compress/gzip`，无新增依赖），统一写 `Vary: Accept-Encoding`，跳过已压缩 MIME 与已带 `Content-Encoding` 的响应，阈值默认 1 KB（`COMPRESS_MIN_BYTES` 可覆盖）。验证：`go test ./internal/platform/middleware/ -run TestCompress` 覆盖 §5.1 前 8 行分支全绿
- [x] 2.2 处理必须跳过的路径与请求：`Upgrade: websocket` 的 `/ws`、AI 流式输出端点白名单（实现时先 grep 出所有流式 handler 路径并写入白名单常量）。验证：`go test ./internal/platform/middleware/ -run TestCompressSkip` 断言 WS 升级与流式路径不被压缩；且 `-run TestCompressStreaming` 断言流式响应按块到达（≥2 次 Write 观测）
- [x] 2.3 在 `internal/app/router.go` 挂载中间件（静态资源 + API 一并覆盖）。验证：`curl -sD - -o /dev/null -H 'Accept-Encoding: gzip' "http://127.0.0.1:5100/_nuxt/$(basename $(ls backend-go/frontend/_nuxt/*.js | head -1))"` 输出含 `Content-Encoding: gzip` 与 `Vary`
- [x] 2.4 复核 `HEAD` 与 `Range` 行为（§5.1 末两行）。验证：`curl -sI -H 'Accept-Encoding: gzip' http://127.0.0.1:5100/health` 返回 2xx 无异常；`curl -s -r 0-99 -o /dev/null -w '%{http_code}\n' http://127.0.0.1:5100/_nuxt/<file>.js` 返回 206 或 200（与设计一致即可）

## 3. 后端：静态缓存头与 favicon

- [x] 3.1 `internal/app/static.go` 按路径分档设置 `Cache-Control`：`/_nuxt/*` → `public, max-age=31536000, immutable`；`index.html` 与 SPA 兜底 → `no-cache`；其他静态文件 → `public, max-age=86400`。验证：`curl -sD - -o /dev/null http://127.0.0.1:5100/` 含 `no-cache`；同命令请求 `/_nuxt/<hash>.js` 含 `immutable, max-age=31536000`
- [x] 3.2 图标拆文件（实测尺寸定档）：`front/public/favicon.png` 替换为 **64×64**（标签图标 + 顶栏 logo，32 CSS px @2x；实测 ≈4.5 KB）；新增 `front/public/brand-mark.png` **720×720**（量化 128 色，实测 ≈29 KB）供阅读页空态插画（360 CSS px，retina 安全），并把 `ArticleContentView.vue:130` 的 `<img src="/favicon.png">` 改为 `src="/brand-mark.png"`（其余引用 MUST NOT 改）。验证：`python3 -c "from PIL import Image; im=Image.open('front/public/favicon.png'); print(im.size)"` 输出 `(64, 64)`；`ls -l front/public/favicon.png front/public/brand-mark.png | awk '{print $5}'` 均 ≤ 65536；`grep -rn '/favicon.png' front/app --include='*.vue' --include='*.ts' | grep -v test` 仅剩 `AppHeaderView.vue` 与 `useAnalysisPauseFavicon.ts` 两处命中
- [x] 3.3 目视验收（两档视口）：标签图标清晰、顶栏 logo 不模糊、空态插画不模糊（1440×900 与 1920×1080）。验证：人工：`pnpm dev` 或静态 :5100 下清空选中文章看空态插画，截图存 `evidence/favicon-brand-*.png`

## 4. 后端：只读模式状态端点与批量对账端点

- [x] 4.1 定位并修复只读/无调度器模式下 `/api/schedulers/status`、`/api/tasks/status` 的 500 成因（在 handler 层把"无调度器"当合法状态，返回 200 + 空集合）。验证：`DEMO_READ_ONLY=1` 起的实例（或 handler 单测注入空 registry）请求两端点均返回 200 且可解析；`go test ./internal/admin/handler/ -run TestStatusEndpointsEmptyRegistry` 绿
- [x] 4.2 新增 `GET /api/poll` 单一批量对账端点：一次返回 `schedulers`（既有结构含顶层 `analysis_paused`/`ai_healthy`）、`tag_queue`（三类计数）、`notifications.unread`。验证：`curl -s --noproxy '*' http://127.0.0.1:5100/api/poll` 输出含三者键；`go test ./internal/admin/handler/ -run TestPollBundle` 绿
- [x] 4.3 保留三个旧端点不动（兼容旧标签页）。验证：`curl -s -o /dev/null -w '%{http_code}\n'` 对 `/api/schedulers/status`、`/api/tag-queue/status`、`/api/notifications/unread-count` 均返回 200
- [x] 4.4 复核 `/api/poll` 的服务端代价（复用既有查询，不新增重活）。验证：`curl -s -o /dev/null -w '%{time_total}\n' http://127.0.0.1:5100/api/poll` < 0.05（本机基线量级）；服务端日志无新增 SLOW SQL

## 5. 前端：轮询合并与可见性

- [x] 5.1 新增单例 `usePollBundle`（模块级唯一 timer + 自适应间隔 + 失败退避 + 结果分发到三处既有 state），替换 `useSchedulerStatus` / `useTagQueueProgress` / `useNotifications` 三处独立 `setInterval`。验证：`cd front && pnpm test:unit app/composables/usePollBundle.test.ts --maxWorkers=2` 覆盖「一次请求含三类数据」「间隔 ≥ 30s」「热态 ≥ 5s」全绿
- [x] 5.2 常驻间隔下限与单页频次上限（空闲 ≥ 30s / 近期反馈 ≥ 15s / 热态 ≥ 5s；60 秒内 ≤ 4 次）。验证：同测试文件 fake timers 推进 60 秒，断言请求数 ≤ 4
- [x] 5.3 `visibilitychange` → hidden 暂停、visible 立即对账一次。验证：同测试文件 mock `document.hidden` 切换，断言 hidden 期间 0 请求、visible 后立即 1 请求
- [x] 5.4 失败保留旧值（不清零、不转错误态）+ 退避。验证：同测试文件注入 5xx，断言角标/芯片值不变且间隔不小于空闲下限
- [x] 5.5 三个消费点（顶栏状态、队列芯片、未读角标）零 props/DOM 变更。验证：`pnpm test:unit app/features/shell app/composables --maxWorkers=2` 全绿；`git diff --stat front/app/features/shell/components` 无组件文件改动（仅数据来源）

## 6. 测试

- [x] 6.1 后端影响包全绿：`cd backend-go && go test ./internal/platform/middleware/... ./internal/app/... ./internal/admin/...`，期望全 PASS
- [x] 6.2 前端受影响测试全绿：`cd front && pnpm test:unit app/composables app/features/shell --maxWorkers=2`，期望全 PASS
- [x] 6.3 `test-cases.md` 主链路 12 个节拍逐行勾对落点测试通过证据（命令 + 结果），期望无缺漏落点、无未处理划除项

## 7. 文档

<!-- doc-impact: flow, api, deployment -->
<!-- doc-impact-excuse: database=并行 change daily-report-margin-notes 的 topicgraph/repository、models 脏文件命中启发式，与本 change 无关（无迁移无 schema 变更）; architecture=并行 change 的 topicgraph/handler、margin_notes 系列脏文件命中启发式，与本 change 无关 -->

- [x] 7.1 `docs/reference/deployment.md`：受支持形态收敛为 Go 单进程同域；`/srv/www` 与反代小节加「弃用（不再维护）」标注；前端静态副本声明唯一来源 `backend-go/frontend/`。验证：`grep -n 'srv/www' docs/reference/deployment.md` 命中行含「弃用」；`grep -n 'deploy/same-origin' docs/reference/deployment.md` 命中行不含「推荐」
- [x] 7.2 `deploy/same-origin/README.md` 顶部加弃用横幅（指向受支持形态）。验证：`head -12 deploy/same-origin/README.md` 含「弃用」
- [x] 7.3 `openspec/specs/same-origin-deployment/spec.md` 的 **Purpose** 段直接改写（主 spec 编辑，按 openspec 约定）：去掉「本 spec 同时约束两条路径」，改为只约束 Go 单进程形态。验证：`grep -n '两条' openspec/specs/same-origin-deployment/spec.md` 零命中
- [x] 7.4 `docs/reference/flow/scheduler.md`：补一行「只读 demo 模式下状态端点返回 200 + 空集合（非 5xx）」并在变更溯源表补本 change。验证：`grep -n 'harden-go-same-origin-serving' docs/reference/flow/scheduler.md` 有命中
- [x] 7.5 `docs/reference/api/` 补 `GET /api/poll` 条目（含三类数据与保留旧端点的说明）。验证：`grep -rn '/api/poll' docs/reference/api/` 有命中
- [x] 7.6 `bash scripts/harness/doc-impact.sh verify`，期望对账通过（无未声明域、无缺失溯源）

## 8. 验证

- [x] `cd backend-go && golangci-lint run ./...`，期望 0 issue
- [x] `cd backend-go && go vet ./... && go build ./...`，期望退出码 0
- [x] `cd backend-go && go test ./internal/platform/middleware/... ./internal/app/... ./internal/admin/...`，期望全 PASS
- [x] `JS=$(ls backend-go/frontend/_nuxt/*.js | head -1 | xargs basename); curl -sD - -o /dev/null -H 'Accept-Encoding: gzip' "http://127.0.0.1:5100/_nuxt/$JS" | grep -icE 'content-encoding: gzip|^vary: accept-encoding'`，期望输出 2
- [x] `curl -sD - -o /dev/null -H 'Accept-Encoding: gzip' http://127.0.0.1:5100/favicon.png | grep -ic 'content-encoding'`，期望输出 0（已压缩类型不重复压）
- [x] `curl -sD - -o /dev/null http://127.0.0.1:5100/ | grep -i 'cache-control'`，期望含 `no-cache`；`curl -sD - -o /dev/null "http://127.0.0.1:5100/_nuxt/$JS" | grep -i 'cache-control'`，期望含 `immutable` 与 `max-age=31536000`
- [x] `ls -l backend-go/frontend/favicon.png | awk '{print $5}'`，期望 ≤ 65536
- [x] `curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:5100/api/schedulers/status`，期望 200；只读模式由 `go test ./internal/admin/handler/ -run TestStatusEndpointsEmptyRegistry` 覆盖（公网 demo 实例上线后人工复验 200）
- [x] `curl -s --noproxy '*' http://127.0.0.1:5100/api/poll | grep -c 'tag_queue'`，期望 ≥ 1
- [x] `cd front && pnpm test:unit app/composables/usePollBundle.test.ts --maxWorkers=2`，期望全 PASS
- [x] `cd front && pnpm lint && pnpm exec nuxi typecheck`，期望 0 error
- [x] 人工：agent-browser 抓 HAR 复核首屏总字节 ≤ 1.0 MB、二次访问 `/_nuxt/*` 请求数为 0、后台标签页 10 分钟无 `/api/poll` 请求（输出与截图存 `evidence/`）

### Scenario → 测试文件映射（scenario-trace 对账）

| Scenario | 测试文件 |
|---|---|
| 静态脚本被压缩 | backend-go/internal/platform/middleware/compress_test.go |
| API JSON 被压缩 | backend-go/internal/platform/middleware/compress_test.go |
| 未声明压缩能力时返回原文 | backend-go/internal/platform/middleware/compress_test.go |
| 字体与图片不被重复压缩 | backend-go/internal/platform/middleware/compress_test.go |
| 小响应不压缩 | backend-go/internal/platform/middleware/compress_test.go |
| WebSocket 升级不受影响 | backend-go/internal/platform/middleware/compress_test.go |
| HEAD 请求安全 | backend-go/internal/platform/middleware/compress_test.go |
| 哈希资源长缓存 | backend-go/internal/app/static_test.go |
| HTML 不长缓存 | backend-go/internal/app/static_test.go |
| favicon 体积与缓存 | backend-go/internal/app/static_test.go |
| 一次请求拿到三类计数 | backend-go/internal/admin/handler/poll_test.go |
| 合并后旧端点仍可用 | backend-go/internal/admin/handler/poll_test.go |
| 空闲态低频 | front/app/composables/usePollBundle.test.ts |
| 热态高频有界 | front/app/composables/usePollBundle.test.ts |
| 单页频次上限 | front/app/composables/usePollBundle.test.ts |
| 后台标签页无请求 | front/app/composables/usePollBundle.test.ts |
| 恢复可见立即对账 | front/app/composables/usePollBundle.test.ts |
| 单次失败保留旧值 | front/app/composables/usePollBundle.test.ts |
| 只读模式返回 200 空集合 | backend-go/internal/admin/handler/scheduler_handler_test.go |
| 任务队列状态同样可用 | backend-go/internal/admin/handler/scheduler_handler_test.go |
| 读模式不影响生产模式语义 | backend-go/internal/admin/handler/scheduler_handler_test.go |
| 部署步骤可复现 | 人工：按 deployment.md 同源小节操作至浏览器可用（evidence/） |
| 文档与实际部署形态一致 | 人工：grep -n 'srv/www' docs/reference/deployment.md 命中行含「弃用」（2026-09-25 实测通过，L285 弃用标注） |
| 弃用形态可判据 | 人工：grep -n 'deploy/same-origin' docs/reference/deployment.md 不含「推荐」（2026-09-25 实测通过） |
