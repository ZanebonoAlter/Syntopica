# E2E 验证记录 — harden-go-same-origin-serving

> 实测时间：2026-09-24 晚。环境：本机 `:5100`（`deploy-frontend.sh` 全新构建铺盘 + 重启后端）。工具：curl（响应头/字节）+ agent-browser（HAR/截图/服务端日志频次）。

## 1. 压缩（http-response-compression）

| 资源 | 未压 wire | 压缩后 wire | 降幅 | 判据 |
|---|---|---|---|---|
| `/_nuxt/entry.DBijpxhp.css` | 656,196 B | **282,770 B** | −57% | ≤300 KB ✓（基线目标 640.8→≤300） |
| `/_nuxt/BE183wrF.js`（最大 chunk 1,469,325 B） | 1,469,325 B | **465,070 B** | −68% | 显著小于原体积 ✓ |
| `/api/articles?per_page=20` | — | gzip ✓ | — | `Content-Encoding: gzip` + `Vary: Accept-Encoding` ✓ |
| `/_nuxt/*.woff2`、`/favicon.png`、`/icons/*` | — | 未压（identity）✓ | — | 已压缩类型不重复压 ✓ |

- 大 JS 响应头断言：`content-encoding: gzip` + `vary: accept-encoding` 命中 2/2。
- HAR 冷启动：**21 个响应 gzip 压缩**（文本类全覆盖）；woff2/png/ico 未压。
- 中间件单测：`go test ./internal/platform/middleware/` 9 用例全绿（§5.1 分支表 + 协商变体 + 阈值边界 1023/1024/1025 + WS/SSE 白名单 + SSE 分块到达）。

## 2. 缓存头（same-origin-deployment）

- `GET /` → `Cache-Control: no-cache` ✓
- `GET /_nuxt/<hash>.js` → `Cache-Control: public, max-age=31536000, immutable` ✓
- `/favicon.png` → `public, max-age=86400` ✓（4,654 B ≤ 64 KB ✓，64×64）
- **二次访问零传输**：warm-load HAR（`e2e-first-load.har`）38 请求中 css/js/_nuxt 传输字节全部为 0（disk cache）✓

## 3. 首屏字节（效果核对）

- **冷启动模拟**（HAR 请求清单逐个 curl 实测 wire bytes，排除 image-proxy 文章封面业务图）：38 请求 **1,019 KB ≤ 1,024 KB** ✓（基线 2.06 MB）。
  - 文本类（html/css/js/api）压缩后约 120 KB（未压基线约 1.16 MB，↓90%）；剩余约 900 KB 为 Noto Serif SC 自托管字体分片（既有构建产物，不压缩，属 slim 字体加载另案）。
- 证据文件：`e2e-cold-load.har`（warm 二次访问）、`e2e-first-load.har`（首次）、`e2e-home.png`。

## 4. 轮询预算（client-poll-budget）

- 服务端日志 65 秒窗口（单页前台可见）：`/api/poll` **2 次 ≤ 4** ✓；旧三端点（schedulers/status / tag-queue/status / unread-count）**0 次**（已合并）✓。
- 注：多标签页各自独立调度（test-cases 变体表明确不跨页协调）；此前测得 9 次为 3 个遗留 browser session 叠加，单页口径 2 次。
- Vitest（`usePollBundle.test.ts` 9 用例）：空闲 ≥30s / 热态 ≥5s / 近期反馈 ≥15s / 60s ≤4（预算硬顶）/ hidden 10 分钟 0 请求 / visible 立即对账 / 失败保留旧值 + 退避 ≥ 空闲下限 / 字段缺失保留该类旧值——全绿。
- `/api/poll` 服务端耗时 0.0039 s（<0.05 判据）✓；响应含 schedulers + tag_queue + notifications.unread + server_time + 顶层 analysis_paused/ai_healthy。

## 5. 只读模式与旧端点（scheduler-observability）

- `GET /api/schedulers/status` 200 ✓；三旧端点全部 200 ✓。
- 空 registry 单测：`TestStatusEndpointsEmptyRegistry`（`Reg=nil` → 200 + 空集合 + 顶层字段保留）、正常模式语义测试全绿。
- **500 根因**：demo 模式 `StartRuntime` 跳过 → `admin.SetRegistry` 未执行 → `handler.Reg` nil → `Reg.OrderedNames()`/`Reg.Get()` nil receiver panic → `gin.Recovery` 500 空 body。修复：handler 层 nil-safe helper（`lookupScheduler`/`orderedSchedulerNames`），"无调度器"成为合法状态。

## 6. favicon 拆分

- `front/public/favicon.png`：64×64 RGBA，4,654 B（原 1254×1254 / 321.6 KB）✓
- `front/public/brand-mark.png`：720×720 P 模式（128 色调色板 + 透明索引），32,913 B ✓
- 引用收敛：`grep -rn '/favicon.png' front/app`（排除测试）仅 `AppHeaderView.vue`（顶栏 logo）与 `useAnalysisPauseFavicon.ts`（标签图标回退）两文件 ✓；`ArticleContentView.vue` 空态插画已指向 `/brand-mark.png`。
- 目视（`favicon-brand-empty-state.png`）：空态插画 360px 渲染清晰不模糊、边缘干净；顶栏 logo 32px 正常。

## 7. 已知偏差（如实记录）

- **`HEAD /health` 返回 404**：gin 的 GET 路由不自动响应 HEAD——**框架既有行为**（改动前同样 404，非本 change 引入）。tasks 2.4 验证命令的预期（2xx）与 gin 实际行为不符；压缩层对 HEAD 的安全性由单测 `TestCompressSkip/HEAD request safe`（显式注册 HEAD 路由）覆盖：200、头部语义一致、无压缩层异常。Range 请求实测 206 ✓。
- 首屏字节口径：目标 ≤1.0 MB 的构成中约 900 KB 是字体分片（未压二进制，非本 change 范围）；文本类降幅 90% 为本 change 贡献。
