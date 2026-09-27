# Pre-change Baseline — harden-go-same-origin-serving

> 实测时间：2026-09-24。来源：① 公网 demo 实例（zanebono.top → 阿里云 47.110.71.194，浏览器 HAR 冷启动，2026-09-24 explore 阶段）；② 本机 :5100 直连（`curl --noproxy '*'`，change 目录内命令可复现）。
> 复现命令均针对本机 `:5100`；公网数值引自 explore-findings.md（实例为脱敏 demo，与本机同版前端构建）。

## 1. 首屏总字节（公网 HAR 冷启动）

- **2.06 MB / 40 请求 / 6.7s**，全部响应 `identity`（无压缩）、40/40 无 `Cache-Control`。
- 来源：explore-findings.md §公网路径实测（HAR 附件见该文档引用）。

## 2. entry.css

```bash
ls -l backend-go/frontend/_nuxt/entry.DBijpxhp.css | awk '{print $5}'
curl -s --noproxy '*' -o /dev/null -w '%{size_download}\n' -H 'Accept-Encoding: gzip, br' \
  'http://127.0.0.1:5100/_nuxt/entry.DBijpxhp.css'
```

- 磁盘 **656,196 B（640.8 KB）**，wire **656,196 B**（零压缩，响应无 `content-encoding`）。
- 公网 HAR 同款文件 640.8 KB → 目标 gzip 后 ≤ 300 KB。

## 3. entry.js

```bash
curl -s --noproxy '*' -o /dev/null -w '%{size_download}\n' -H 'Accept-Encoding: gzip, br' \
  'http://127.0.0.1:5100/_nuxt/DM3mcF8i.js'   # 本机构建入口（index.html 引用）
```

- 本机入口 `DM3mcF8i.js` wire **302,934 B（295.9 KB）**，无 `content-encoding`。
- 公网 HAR entry（`B2RVEoLI.js`）295.9 KB，同量级 → 目标 gzip 后 ≤ 130 KB。

## 4. /api/articles?per_page=100

```bash
curl -s --noproxy '*' -o /dev/null -w '%{size_download}\n' \
  'http://127.0.0.1:5100/api/articles?per_page=100'
```

- explore-findings 基线：**1,125 KB**（单发 3.7s，公网与本机当时一致；公网 demo per_page=20 为 259.7 KB）。
- 本机当下实测 **58,666 B**——本机库当前文章数远少于基线时点（事故恢复后的库状态不同），**量化判据以 explore-findings 公网基线 1,125 KB → 目标 ≤ 350 KB 为准**；本机复测时按同库前后对比。

## 5. favicon.png

```bash
curl -s --noproxy '*' -o /dev/null -w '%{size_download}\n' 'http://127.0.0.1:5100/favicon.png'
curl -s --noproxy '*' -D - -o /dev/null 'http://127.0.0.1:5100/favicon.png' | grep -i 'content-length\|cache-control'
```

- wire **329,292 B（321.6 KB）**，`Content-Length: 329292`，**无 `Cache-Control`**（每次访问重下 322 KB）。
- 源文件 `front/public/favicon.png` 1254×1254 RGBA（git 已跟踪）→ 目标拆两文件后 ≈33 KB。

## 6. 常驻对账频次

- 公网日志统计（explore-findings §轮询清单）：`/api/schedulers/status` 18,783 + `/api/tag-queue/status` 15,535 + `/api/notifications/unread-count` 9,806 = **44,124 次（占全部请求 53%）**；单客户端 ≈ **10 次/分钟**。
- 三处独立 `setInterval`：`useSchedulerStatus.ts` 8/15/30s 自适应、`useTagQueueProgress.ts` 60s、`useNotifications.ts` 60s；全仓无 `visibilitychange` 处理（后台标签页照常轮询）→ 目标合并后 ≤ 4 次/分钟 + 后台 0 请求。

## 7. 病灶头证据（本机 :5100 直连）

```bash
curl -s --noproxy '*' -D - -o /dev/null -H 'Accept-Encoding: gzip, br' 'http://127.0.0.1:5100/_nuxt/DM3mcF8i.js' | grep -ci 'content-encoding\|cache-control'
# 实测输出：0
```

- 四类响应（大 JS / entry.css / API JSON / favicon）均无 `content-encoding`、无 `cache-control` → 压缩与缓存契约完全缺位，且公网路径直达 Go 进程（`Server` 头为空、无反代），必须应用内实现。
