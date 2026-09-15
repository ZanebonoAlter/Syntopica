# 同源反代部署（Caddy）

浏览器只面对**一个 origin**——前端静态产物与后端 API / WebSocket / 图标由同一个入口提供。

适用于**浏览器与后端不同机**的常驻部署：本文件按树莓派 5 写，任意 Linux 主机同样适用。

## 为什么用这个形态

| | 直连形态（默认） | 同源形态（本目录） |
|---|---|---|
| 前端 `apiBase` | `http://localhost:5100/api`（代码默认值） | `/api`（构建期注入） |
| 适用场景 | 浏览器与后端**同一台机器**（Windows 本地开发） | 浏览器在**另一台机器**（PC 看树莓派） |
| 后端 CORS 白名单 | 必须逐个列出前端 origin | 不参与（不存在跨域） |
| 换设备 / 换网段 | 改 `CORS_ORIGINS` + 重新构建前端 | 什么都不用改 |

直连形态下浏览器把 `localhost` 解释成**浏览器自己那台机器**，所以搬到 Pi 上必然连不上（`ERR_CONNECTION_REFUSED`）；把地址改成 Pi 的 IP 之后又会撞后端 CORS 白名单（响应缺少 `Access-Control-Allow-Origin`）。同源形态一次消掉这两个必配项。

## 前置条件

- **Pi**：Linux + Docker（`docker --version`）
- **端口**：宿主 80 空闲 —— `ss -ltnp | grep ':80'`
- **后端已跑**：监听 `127.0.0.1:5100` —— `curl -s http://127.0.0.1:5100/health` 有 JSON 响应
- **PC**：Node + pnpm（只用来构建前端；Pi 上不需要 Node 工具链）
- Pi 上有本仓库副本（下文按 `~/syntopica` 写）

## 1. PC：构建前端静态产物

> ⚠️ **必须带 `NUXT_PUBLIC_API_BASE=/api`。**
> 静态 SPA 的 `runtimeConfig.public` 在**构建时内联**进产物 —— 部署之后再设这个名字的环境变量**没有任何作用**。这是本形态最容易踩的坑。

Windows 上用 PowerShell 或 cmd（不要在 WSL 里跑，缺 native binding）：

```powershell
cd D:\project\Syntopica\front
$env:NUXT_PUBLIC_API_BASE = '/api'
pnpm generate
```

自检（**应当零输出**，有输出说明注入没生效）：

```powershell
Get-ChildItem -Recurse .\.output\public -File | Select-String -SimpleMatch 'localhost:5100' | Select-Object -First 5
```

## 2. 打包并拷到 Pi

```powershell
cd D:\project\Syntopica\front
tar -czf $env:TEMP\syntopica-web.tgz -C .output\public .
scp $env:TEMP\syntopica-web.tgz pi@<pi-ip>:~/
```

## 3. Pi：解包并起入口容器

```bash
cd ~/syntopica/deploy/same-origin
mkdir -p www
tar -xzf ~/syntopica-web.tgz -C www
ls www/index.html                      # 必须存在

docker compose -f docker-compose.yml up -d
docker compose -f docker-compose.yml ps
```

## 4. 验收

Pi 上（`<pi-ip>` 换成实际地址）：

```bash
curl -sI http://127.0.0.1/                     | head -1   # HTTP/1.1 200 —— 静态页
curl -sI http://127.0.0.1/tags                 | head -1   # HTTP/1.1 200 —— 深层路由兜底
curl -s  http://127.0.0.1/api/categories       | head -c 200   # 分类 JSON
curl -sI http://127.0.0.1/icons/feeds/42.png   | grep -i content-type   # image/png（id 换成真实存在的）
curl -s  http://127.0.0.1/health                           # 后端健康 JSON
```

PC 浏览器：

1. 打开 `http://<pi-ip>/`（**注意不带 `:3000`**）→ 列表页有数据
2. DevTools → Network：请求全部发往 `http://<pi-ip>/api/...`，无跨域、无 CORS 预检
3. Console：`new WebSocket('ws://<pi-ip>/ws')` → 触发 `open` 事件

## 排障

| 现象 | 原因 | 处理 |
|---|---|---|
| 页面能开但列表空；Console 里请求发往 `localhost:5100` | 构建时没带 `NUXT_PUBLIC_API_BASE=/api` | 回第 1 步重新构建（可先 `rm -rf front/.output`） |
| `docker compose up` 报 `conflicting options: port publishing and the container type network mode` | 同时写了 `ports:` 与 `network_mode: host` | 删掉 `ports:` |
| 根路径 404 或目录列表 | `www/` 空或挂载路径不对 | `ls deploy/same-origin/www/index.html` |
| API 返回 502 | 后端没在 5100 上跑 | `curl -s http://127.0.0.1:5100/health`，先把后端起来 |
| 图标破图（响应体是 HTML） | `/icons/*` 没转发 | 核对 `Caddyfile` 的 `@backend` path 列表 |
| 事件流连不上、反复重连 | `/ws` 没转发 | `curl -sI http://127.0.0.1/ws` 应返回后端响应而非 HTML 兜底页 |
| 容器起不来，`bind: address already in use` | 宿主 80 被占 | `ss -ltnp \| grep ':80'`；或把 `Caddyfile` 的 `:80` 改成别的端口 |
| 换了产物浏览器还是旧的 | 浏览器缓存 | 硬刷新 `Ctrl+Shift+R` |

## 回滚

```bash
docker compose -f docker-compose.yml down
```

回到旧的 dev 形态需要**两个**环境变量（缺一个就是另一种报错）：Pi 上的前端设 `NUXT_PUBLIC_API_BASE=http://<pi-ip>:5100/api`，后端设 `CORS_ORIGINS=http://<pi-ip>:3000`。

## 相关

- 两种形态的适用边界与失败症状：`docs/reference/deployment.md`
- `NUXT_PUBLIC_API_BASE` 的构建期语义：`docs/reference/configuration.md`
- 为什么不用 Nitro `devProxy` / Vite `server.proxy` 做同源：`openspec/changes/fix-wsl-dev-networking/design.md` D2（WS upgrade 过不去，已实测否决）
