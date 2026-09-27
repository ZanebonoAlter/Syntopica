# 验收证据（add-image-proxy，2026-09-22）

## 1. 后端代理 curl 实测（任务 3.4 / 5.5）

基线（直连，探索期 2026-09-22）：

```
$ curl -sD - "https://cdnfile.sspai.com/2026/09/21/64a2...png?imageView2/..." | head
HTTP/2 403
server: Byte-nginx
x-exception-info: deny by referer access rule
byte-error-code: 00074
```

经代理（后端部署后）：

```
--- 首次 ---
200 image/webp 37628B
Content-Type: image/webp
X-Image-Proxy-Cache: MISS
--- 二次 ---
200 image/webp
X-Image-Proxy-Cache: HIT
--- 产物校验 ---
/tmp/img_proxy1.bin: RIFF (little-endian) data, WebP image, VP8 encoding, 853x1280
--- 非法 url（ftp://）---
400
```

结论：同一个原本 403 的 sspai 图，经代理 200 返回真实 WebP；缓存 MISS→HIT 生效；非法 url 400。

## 2. 列表页封面（浏览器 Network + DOM 断言）

- 封面 `<img src>` = `/api/image-proxy?url=http%3A%2F%2Fimg.jrjimg.cn%2F...`（改写生效）
- 两张封面 `complete: true, naturalWidth 636/625`（真实渲染，非破图）
- feed 本地图标保持 `http://localhost:5100/icons/feeds/33.ico` 直连（自有地址不进代理）
- Network：仅 2 条 `/api/image-proxy` 200，**零图床域名直连**
- 截图：`list-covers-proxied.png`

## 3. 侦探墙贴图（任务 3.4 后半）

打开叙事工坊 → 中东板块 → 话题总览 → 侦探墙：canvas 1280×633 正常渲染，Network 新增十余条贴图请求**全部经代理且 200**，覆盖多图床、零直连、零 403：

```
GET /api/image-proxy?url=http%3A%2F%2Fimg.jrjimg.cn%2F...  (Image) 200
GET /api/image-proxy?url=https%3A%2F%2Fx0.ifengimg.com%2F... (Image) 200
GET /api/image-proxy?url=https%3A%2F%2Fwpimg-wscn.awtmt.com%2F... (Image) 200
GET /api/image-proxy?url=https%3A%2F%2Fdss0.zbstatic5.com%2F... (Image) 200
（…共十余条，全部 200，无任何图床域名直连请求）
```

注：侦探墙 WebGL 场景的 `Page.captureScreenshot` CDP 超时未截到图，以本 Network 输出记录为验收证据（DOM/Canvas 断言 + 请求清单已在上）。

## 4. 缓存目录

- 实际落点 `backend-go/data/image-cache/`（后端 cwd=`backend-go/`，与 `data/icons/` 同惯例）
- 文件名 = sha256(原始 URL)；`git check-ignore` 命中 `.gitignore:9:data/`
