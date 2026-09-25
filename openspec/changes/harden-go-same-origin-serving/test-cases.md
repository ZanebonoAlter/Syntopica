# 用例设计 — harden-go-same-origin-serving

> 测试单元 = 一个 Requirement 的用户故事。本文件把 spec Scenario 串成完整故事；复杂档附白盒分支表与边界值。
> 故事锚点：**用户在公网低带宽链路上打开页面（首屏字节少、二次访问不重下）→ 界面上的状态/角标/芯片正常刷新（不被轮询拖慢）→ 后台标签页不偷流量。**

## 1. 主链路表（节拍）

| # | 步 / 动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| 1 | 冷启动请求 `/_nuxt/<hash>.js` | `http-response-compression` / 静态脚本被压缩 | `Content-Encoding: gzip`、`Vary` 含 `Accept-Encoding`、wire bytes 显著 < 原体积 | Go 中间件单测 + `curl -D -` | `backend-go/internal/platform/middleware/compress_test.go` |
| 2 | 冷启动请求 `/_nuxt/<hash>.woff2` 与 `/favicon.png` | 同 capability / 字体与图片不被重复压缩 | 无 `Content-Encoding`，体积等于磁盘文件 | Go 中间件单测 | 同上 |
| 3 | 请求 `GET /api/articles?per_page=100` | 同 capability / API JSON 被压缩 | gzip 且解压后 JSON 等价 | Go 中间件单测 | 同上 |
| 4 | 二次访问 `/_nuxt/<hash>.js` | `same-origin-deployment` / 哈希资源长缓存 | 响应含 `immutable`；浏览器二次访问零网络请求（disk cache） | `curl -D -` + 浏览器人工 | 验证节命令 + `evidence/` 截图 |
| 5 | 访问 `/` 与深层路由 `/tags` | 同 capability / HTML 不长缓存 | `Cache-Control: no-cache`，无长 `max-age` | `curl -D -` | 验证节命令 |
| 6 | 请求 `/favicon.png` | 同 capability / favicon 体积与缓存 | ≤ 64 KB 且带缓存语义 | `ls -l` + `curl -D -` | 验证节命令 |
| 7 | 页面常驻 1 分钟（空闲） | `client-poll-budget` / 空闲态低频 + 单页频次上限 | 常驻对账请求 ≤ 4 次且相邻间隔 ≥ 30s | Vitest（fake timers） | `front/app/composables/usePollBundle.test.ts` |
| 8 | 某任务执行中 | 同 capability / 热态高频有界 | 间隔 ≥ 5s，执行结束回到 ≥ 30s | Vitest（fake timers） | 同上 |
| 9 | 切到后台 10 分钟再切回 | 同 capability / 后台标签页无请求 + 恢复可见立即对账 | hidden 期间 0 请求；visible 后立即 1 次 | Vitest（mock `visibilitychange`） | 同上 |
| 10 | 对账请求失败一次 | 同 capability / 单次失败保留旧值 | 角标/芯片保留旧值且不退化为错误态 | Vitest | 同上 |
| 11 | 一次对账的数据来源 | 同 capability / 一次请求拿到三类计数 + 合并后旧端点仍可用 | 单请求含三类数据；三个旧端点仍 200 | Go handler 测 + `curl` | `backend-go/internal/admin/handler/*_test.go` |
| 12 | 只读模式请求调度器状态 | `scheduler-observability` / 只读模式返回 200 空集合 + 任务队列状态同样可用 | HTTP 200 + 空集合（可解析） | Go handler 测 + `curl` | `backend-go/internal/admin/handler/scheduler_handler_test.go` |

**负向节拍（SHALL NOT）**：任何一次页面加载都不得出现「400 KB 级 favicon 重下」「空态插画在未渲染空态时被请求」「后台标签页产生常驻对账请求」「未压缩的 `/_nuxt/*.js` 文本响应」「`/api/schedulers/status` 返回 5xx」。

## 2. 变体走查

| 组 | 变体 | 答案 |
|---|---|---|
| 输入 | `Accept-Encoding` 缺失 / `gzip` / `br`（不支持）/ `deflate, gzip;q=0.8` / 大小写 `GZIP` | 缺失或仅 br → 不压（identity）且仍带 `Vary`；含 gzip → 压；q 值降级 → 采 gzip；大小写按 RFC 大小写不敏感处理 |
| 前置 | 空文件 / 0 字节资源 / 已带 `Content-Encoding` 的响应 / 流式端点在压 / `Upgrade: websocket` | 空/0 字节 → 不压；已压缩 → 直通；流式端点 → 白名单跳过（MUST 保持逐块到达）；WS 升级 → 中间件不介入 |
| 时间窗口 | 边界两端 / 空窗口 / 跨窗口 / 归一化 | **划除**：本 change 无日期/窗口语义（轮询间隔是相对计时，不涉日历） |
| 幂等 | 重复执行（同一 URL 连续请求）/ 部分失败重试（合并端点 5xx）/ 并发（多标签页同时轮询） | 重复请求 → 压缩结果一致（无状态）；5xx → 保留旧值 + 退避；多标签页 → 各自独立调度（不在本 change 声称跨标签页协调），单页频次上限仍成立 |
| 可用性（UI 必检前三） | 加载态 / 空态 / 错误态 / 超长文本 / 重复提交 | 空态：只读模式空集合 → 渲染既有空态非错误态；错误态：合并失败保留旧值；加载态/超长/重复提交 → 不适用（本 change 不新增界面元素）**划除留痕** |

## 3. 继承与调整（⓪ 契约变更反查）

反查命令：`bash scripts/harness/test-assets.sh same-origin-deployment` / `... scheduler-observability`。

| 旧 Scenario（capability） | 处置 | 旧测试资产 | 动作 |
|---|---|---|---|
| `same-origin-deployment` / 同源反代部署制品·后端路径全量转发 | **契约移除**（反代形态弃用） | 无自动化（历史记录为人工 curl 判据） | 归档时随 REMOVED 生效；本 change 用「Go 单进程形态」等价判据替代（静态 + API 同 origin 验证） |
| `same-origin-deployment` / 同源反代部署制品·WebSocket 同源转发 | **契约移除** | 无自动化（人工 ws 连接） | 同上；新增压缩不干扰 WS 的回归断言 |
| `same-origin-deployment` / 同源反代部署制品·SPA 深层路由兜底 | 语义保留（形态换成 Go 单进程静态托管） | 无自动化（人工 curl `/tags`） | 保留人工判据，路径改为 `:5100` |
| `same-origin-deployment` / 同源反代部署制品·dev 模式前端反代 | **契约移除**（dev 反代随弃用一起下线） | 无自动化 | 无 |
| `same-origin-deployment` / 同源部署路径与边界文档化·文档与实际部署形态一致 | 语义变更（收敛为单一受支持形态） | 历史人工判据 `grep -rn 'front/Dockerfile\|NUXT_PUBLIC_API_ORIGIN' docs/reference/` 零命中 | 保留该 grep 判据 + 新增弃用标注 grep 判据（见验证节） |
| `same-origin-deployment` / 同源构建期注入相对 base（3 Scenario） | 不受影响 | 历史人工判据（`grep -rl 'localhost:5100' front/.output/public` 零命中） | 回归跑，不改 |
| `scheduler-observability` / 全部 5 Requirement（15 Scenario） | 不受影响（本 change 只增"无调度器模式"契约） | 无自动化映射可查（历史 change 早期） | 回归跑既有测试，确认新增分支不改变既有 200 路径结构 |
| `tag-queue-progress-chip` / 进度芯片常驻展示等 5 Scenario | 数据来源与节奏变更、UI 行为不变 | 组件测试 | 断言：合并后芯片计数显示不回归（组件测 + 人工） |

## 4. 效果核对（实测量化）

- **触发原因**：压缩收益依赖真实资源构成（文本 vs 已压缩二进制）与响应大小分布，fixture 不能代表；轮询收益依赖真实触发节奏。
- **方法**：① 对 `:5100` 直连与公网地址分别用同一请求测 `content-encoding` 与 wire bytes（`curl -s -D - -o /dev/null -H 'Accept-Encoding: gzip'`）；② 用 agent-browser 抓 HAR 统计首屏总字节与二次访问请求数；③ 服务器日志按分钟统计常驻对账请求数（合并前基线：单客户端约 10 次/分钟）。
- **量化结果（合并前基线）**：`entry.css` 640.8 KB → 目标 ≤ 300 KB；`entry.js` 295.9 KB → 目标 ≤ 130 KB；`/api/articles?per_page=100` 1125 KB → 目标 ≤ 350 KB；`favicon.png` 321.6 KB → 目标 64×64 ≈4.5 KB（+ 新增 `brand-mark.png` 720×720 ≈29 KB，仅空态加载）；首屏合计 2.06 MB → 目标 ≤ 1.0 MB；常驻对账 ≤ 4 次/分钟。
- **结论 / 达标判据**：以上五项实测达标 + 后台标签页 0 请求；两次数值（前/后）写入 `evidence/`。

## 5. 白盒附加（复杂档）

### 5.1 压缩中间件分支表

| 请求特征 | 期望行为 |
|---|---|
| `Accept-Encoding: gzip` + `text/html` 8 KB | 压缩，`Content-Encoding: gzip`、`Vary` 存在 |
| `Accept-Encoding: gzip` + `text/html` 900 B（< 阈值） | 不压缩（identity），`Vary` 仍存在 |
| `Accept-Encoding: gzip` + `application/json` 50 KB | 压缩，解压后等价 |
| `Accept-Encoding: gzip` + `font/woff2` | 不压缩（已压缩类型） |
| `Accept-Encoding: gzip` + `image/png` | 不压缩 |
| 无 `Accept-Encoding` | 不压缩，`Vary` 存在 |
| `Accept-Encoding: br`（仅） | 不压缩（未实现 br），`Vary` 存在 |
| `Accept-Encoding: deflate, gzip;q=0.5` | 压缩（gzip 可用） |
| `Upgrade: websocket` 的 `/ws` | 中间件跳过，升级成功 |
| 流式端点（白名单内，如 AI 流式输出路径） | 跳过压缩，客户端按块收到 |
| 响应已带 `Content-Encoding: gzip` | 不二次压缩 |
| `HEAD /_nuxt/x.js` | 与 GET 一致的头部语义，无错误 |
| `Range: bytes=0-99` 请求静态文件 | 部分内容响应正确（压缩与 206 不冲突或按设计跳过压缩） |

### 5.2 边界值

| 项 | 边界 | 期望 |
|---|---|---|
| 压缩阈值 | 1023 / 1024 / 1025 字节 | 前两者不压（或按 `<1024` 判据），1025 压缩 |
| 缓存 max-age | `/_nuxt/*` | `31536000` + `immutable` |
| favicon 体积 | 两个图标文件 | `favicon.png` 尺寸 = 64×64 且 ≤ 64 KB；`brand-mark.png` 尺寸 = 720×720 且 ≤ 64 KB（实测断言具体字节数） |
| 图标引用点 | 拆分后 | `grep -rn '/favicon.png' front/app` 仅剩顶栏与 favicon composable；`ArticleContentView.vue` 已指向 `brand-mark.png` |
| 轮询间隔 | 空闲 / 近期反馈 / 热态 | ≥ 30s / ≥ 15s / ≥ 5s |
| 单页频次 | 连续 60 秒 | ≤ 4 次请求 |
| 失败退避 | 连续 2 次失败 | 间隔不小于空闲下限，且不清零既有数值 |
| 合并端点字段缺失 | 三类中某一类缺失 | 其余两类正常更新，缺失类保留旧值 |

### 5.3 划除留痕

- 时间窗口组（边界两端/空窗口/跨窗口/归一化）：本 change 无日历/窗口语义 → 划除。
- UI 加载态/超长文本/重复提交变体：不新增界面元素与提交动作 → 划除。
- DB 迁移层：无迁移 → 不适用。

### 5.4 层选择结论（五问句 ③）

压缩与响应头 → Go 中间件/静态层单测（最便宜且直接断言字节与头部）；只读模式状态端点 → handler 测 + `curl`；轮询预算 → Vitest fake timers（时间行为只能在计时层断言）；**完整交互故事（首屏字节 + 二次访问命中缓存 + 角标/芯片正常）→ agent-browser 端到端**，无自动化部分按「人工：验证方式」留痕。
