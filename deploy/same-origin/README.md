# 同源反代部署（Caddy / nginx）

浏览器只面对**一个 origin**——前端与后端 API / WebSocket / 图标由同一个入口提供。

适用于**浏览器与后端不同机**的部署：本文件按树莓派 5 写，任意 Linux 主机同样适用。两种入口任选其一：

| 入口 | 制品 | 何时选它 |
|---|---|---|
| **Caddy（Docker）** | `Caddyfile` / `Caddyfile.dev` + `docker-compose.yml` | 宿主有 Docker 且能拉 `caddy:2-alpine` 镜像 |
| **nginx（系统包）** | `nginx.conf` / `nginx.static.conf` + `install-nginx.sh` | 拉不到 Docker Hub（实测 Pi 上 `registry-1.docker.io` 超时）或不想养容器 |

## 先选模式

| 模式 | 前端跑什么 | 要不要构建 | 适用 |
|---|---|---|---|
| **A. dev 反代** | Pi 上的 `pnpm dev`（`:3000`） | **不用** | 日常开发 / 试用；改代码即生效，有 HMR |
| **B. 静态产物** | `pnpm generate` 出的 `.output/public` | 要（PC 或 Pi 上都行） | 常驻 / 对外；省一个常驻 Node 进程 |

两种模式的 Caddyfile / nginx conf 都是现成的，用 `CADDYFILE`（Caddy）或 `install-nginx.sh` 的参数（nginx）切换，其余步骤一样。

## 为什么用这个形态

| | 直连形态（默认） | 同源形态（本目录） |
|---|---|---|
| 前端 `apiBase` | `http://localhost:5100/api`（代码默认值） | `/api`（相对） |
| 适用场景 | 浏览器与后端**同一台机器** | 浏览器在**另一台机器**（PC 看树莓派） |
| 后端 CORS 白名单 | 必须逐个列出前端 origin | 不参与（不存在跨域） |
| 换设备 / 换网段 | 改 `CORS_ORIGINS` + 重配前端 | 什么都不用改 |

直连形态下浏览器把 `localhost` 解释成**浏览器自己那台机器**，所以搬到 Pi 上必然连不上（`ERR_CONNECTION_REFUSED`）；把地址改成 Pi 的 IP 之后又会撞后端 CORS 白名单（响应缺少 `Access-Control-Allow-Origin`）。同源形态一次消掉这两个必配项。

## 前置条件

- **入口**：Caddy 路线要 Linux + Docker（`docker --version`）；nginx 路线用系统包（见下节）
- **端口**：宿主 80 空闲 —— `ss -ltnp | grep ':80'`（换端口见「换端口」节）
- **后端已在跑**：监听 `127.0.0.1:5100` —— `curl -s http://127.0.0.1:5100/health` 有 JSON 响应
- Pi 上有本仓库（下文按 `~/syntopica` 写）

---

## 模式 A：dev 反代（零构建，推荐先试）

前端仍是你现在跑的 `pnpm dev`，Caddy 只负责把它和后端拼成一个 origin。

```bash
# 1. Pi：前端带相对 base 重启（dev 在启动时读环境变量，必须重启才生效）
cd ~/syntopica/front
NUXT_PUBLIC_API_BASE=/api pnpm dev

# 2. Pi：另开一个终端，起 Caddy 走 dev 变体
cd ~/syntopica
CADDYFILE=./Caddyfile.dev docker compose -f deploy/same-origin/docker-compose.yml up -d
```

然后浏览器开 **`http://<pi-ip>/`**（不是 `:3000`）。

> ⚠️ **副作用（预期行为，不是坏了）**：设了 `/api` 之后，直连 `http://<pi-ip>:3000` 会变成 404（页面能开，但 `/api/*` 打到 dev server 上没人代理）。同源入口只有 `:80` 一个，把旧书签换掉即可。想两边都能用就回到直连形态（前端 `NUXT_PUBLIC_API_BASE=http://<pi-ip>:5100/api` + 后端 `CORS_ORIGINS`）。

- `pnpm dev` 的 `devServer.host` 已是 `0.0.0.0`，Caddy 走 host 网络访问 `127.0.0.1:3000`，不冲突。
- Vite HMR 的 WebSocket 也经 Caddy 转发（`reverse_proxy` 透明处理 upgrade），热更新照常。
- 这个模式下 Caddy 几乎不占资源，但它**不会**让 dev server 变快——dev 模式本身吃内存、无压缩。

---

## 模式 B：静态产物

### 1. 构建（PC 或 Pi 上都行）

> ⚠️ **必须带 `NUXT_PUBLIC_API_BASE=/api`。**
> 静态 SPA 的 `runtimeConfig.public` 在**构建时内联**进产物 —— 部署之后再设这个名字的环境变量**没有任何作用**。这是本模式最容易踩的坑。

在 Pi 上（依赖已装好，ARM 上慢一点，一次性）：

```bash
cd ~/syntopica/front
NUXT_PUBLIC_API_BASE=/api pnpm generate
ls .output/public/index.html
```

在 PC 上（PowerShell，快）：

```powershell
cd D:\project\Syntopica\front
$env:NUXT_PUBLIC_API_BASE = '/api'
pnpm generate
# 自检：应当零输出，有输出说明注入没生效
Get-ChildItem -Recurse .\.output\public -File | Select-String -SimpleMatch 'localhost:5100'
```

### 2. 把产物放到 `www/`

Pi 上构建的（无拷贝）：

```bash
cd ~/syntopica/deploy/same-origin
mkdir -p www && cp -r ../../front/.output/public/. www/
```

PC 上构建的：

```powershell
cd D:\project\Syntopica\front
tar -czf $env:TEMP\syntopica-web.tgz -C .output\public .
scp $env:TEMP\syntopica-web.tgz pi@<pi-ip>:~/
```

```bash
# Pi 上解包
cd ~/syntopica/deploy/same-origin
mkdir -p www && tar -xzf ~/syntopica-web.tgz -C www
ls www/index.html                      # 必须存在
```

### 3. 起入口容器

```bash
cd ~/syntopica
docker compose -f deploy/same-origin/docker-compose.yml up -d
docker compose -f deploy/same-origin/docker-compose.yml ps
```

---

## nginx 变体（系统包，零 Docker）

Docker Hub 拉不到时（实测 Pi 上 `registry-1.docker.io` i/o timeout）用系统 nginx，行为与 Caddy 两条模式一一对应：

| 模式 | 模板 | 安装 | 前端 |
|---|---|---|---|
| **A. dev 反代** | `nginx.conf` | `sudo bash deploy/same-origin/install-nginx.sh dev` | `cd front && NUXT_PUBLIC_API_BASE=/api pnpm dev --host` |
| **B. 静态产物** | `nginx.static.conf` | 先 `NUXT_PUBLIC_API_BASE=/api pnpm generate` + 产物铺到 `/srv/www`，再 `sudo bash deploy/same-origin/install-nginx.sh static` | 不需要常驻进程 |

安装脚本做四件事（幂等、可重复跑）：写 `/etc/nginx/conf.d/syntopica.conf`（旧文件先备份）→ 移除 `/etc/nginx/sites-enabled/default` **软链**（它同样声明 `default_server`，留着 `nginx -t` 直接报 `a duplicate default server`）→ `nginx -t` 校验（失败自动回滚旧配置）→ `systemctl enable + reload`。

转发规则与 Caddyfile 等价：`/api` `/icons` `/health` → `127.0.0.1:5100`；`/ws` 与 `/_nuxt/`（Vite HMR）带 `Upgrade` 头分别到 5100 / 3000；其余到 dev server（模式 A）或静态产物 + SPA 兜底（模式 B）。`$connection_upgrade` 用 `map` 产出，非 WS 请求取 `close`——直接写死 `"upgrade"` 会拖住 keepalive。

**实机验证**（2026-09-16，Pi 5 / nginx 1.26.3 / 模式 A）：`/`、`/health`、`/api/categories`、`/icons/feeds/2.ico` 全 200；`/ws` → **`101 Switching Protocols`**；HMR 的 `/_nuxt/` 升级 → `101`；浏览器 369 个请求全部同源（无一条打 `:5100`），订阅源页 11 个 `<img>` 图标 0 破图。

## 验收（两种模式通用）

```bash
curl -sI http://127.0.0.1/                     | head -1   # HTTP/1.1 200 —— 入口可达
curl -sI http://127.0.0.1/tags                 | head -1   # HTTP/1.1 200 —— 深层路由不 404
curl -s  http://127.0.0.1/api/categories       | head -c 200   # 分类 JSON
curl -s  http://127.0.0.1/health                           # 后端健康 JSON
curl -sI http://127.0.0.1/icons/feeds/42.png   | grep -i content-type   # image/png（id 换成真实存在的）
```

PC 浏览器：

1. 打开 `http://<pi-ip>/` → 列表页有数据
2. DevTools → Network：请求全部发往 `http://<pi-ip>/api/...`，无跨域、无 CORS 预检
3. Console：`new WebSocket('ws://<pi-ip>/ws')` → 触发 `open` 事件

## 排障

| 现象 | 原因 | 处理 |
|---|---|---|
| 页面能开但列表空；Console 里请求发往 `localhost:5100` | 前端没带 `NUXT_PUBLIC_API_BASE=/api` | 模式 A：带变量重启 dev；模式 B：重新构建 |
| 直连 `:3000` 时 `/api/*` 报 404 | 预期行为：带 `/api` 运行时前端只认同源入口 | 改用 `http://<pi-ip>/`（见模式 A 的副作用说明） |
| `docker compose up` 报 `conflicting options: port publishing and the container type network mode` | 同时写了 `ports:` 与 `network_mode: host` | 删掉 `ports:` |
| 根路径 404 或目录列表（模式 B） | `www/` 空或挂载路径不对 | `ls deploy/same-origin/www/index.html` |
| 根路径返回 502（模式 A） | dev server 没在 `:3000` 上跑 | 先起 `pnpm dev` |
| API 返回 502 | 后端没在 5100 上跑 | `curl -s http://127.0.0.1:5100/health` |
| 图标破图（响应体是 HTML） | `/icons/*` 没转发 | 核对 Caddyfile 的 `@backend` path 列表 / nginx 的 `location ~ ^/(api\|icons\|health)(/\|$)` |
| nginx `-t` 报 `a duplicate default server for 0.0.0.0:80` | Debian 默认站点（`sites-enabled/default`）仍在，它也声明 `default_server` | `sudo rm -f /etc/nginx/sites-enabled/default`（安装脚本已自动处理） |
| nginx 装好后页面能开但列表空、Console 报 CORS | 前端还是绝对 base（没带 `/api` 重启） | 带变量重启 dev，或重新 `pnpm generate` |
| 事件流连不上、反复重连 | `/ws` 没转发 | `curl -sI http://127.0.0.1/ws` 应返回后端响应而非前端页面 |
| 容器起不来，`bind: address already in use` | 宿主 80 被占 | `ss -ltnp \| grep ':80'`；或按「换端口」改 |
| 改了产物浏览器还是旧的 | 浏览器缓存 | 硬刷新 `Ctrl+Shift+R` |

### 换端口

Caddy：改对应 Caddyfile 里的 `:80` 为其他端口（host 网络下直接绑宿主端口）：

```bash
sed -i 's/^:80 {/:8080 {/' deploy/same-origin/Caddyfile      # 或 Caddyfile.dev
docker compose -f deploy/same-origin/docker-compose.yml up -d
```

nginx：改 `deploy/same-origin/nginx*.conf` 里的两条 `listen`（`:80` 与 `[::]:80`），再重跑 `sudo bash deploy/same-origin/install-nginx.sh <变体>`。

## 切模式 / 回滚

```bash
# Caddy — 从 dev 切到静态：先在 www/ 准备产物，再重建容器（不加 CADDYFILE 即默认静态）
docker compose -f deploy/same-origin/docker-compose.yml up -d --force-recreate

# Caddy — 从静态切回 dev
CADDYFILE=./Caddyfile.dev docker compose -f deploy/same-origin/docker-compose.yml up -d --force-recreate

# Caddy — 完全撤掉同源入口（回到直连形态）
docker compose -f deploy/same-origin/docker-compose.yml down

# nginx — 切换变体（dev ↔ static）
sudo bash deploy/same-origin/install-nginx.sh static

# nginx — 撤掉入口：删配置 + 恢复 Debian 默认站点 + reload
sudo rm -f /etc/nginx/conf.d/syntopica.conf
sudo ln -s /etc/nginx/sites-available/default /etc/nginx/sites-enabled/default
sudo nginx -t && sudo systemctl reload nginx
```

撤掉后回到直连形态需要**两个**环境变量（缺一个就是另一种报错）：前端 `NUXT_PUBLIC_API_BASE=http://<pi-ip>:5100/api`，后端 `CORS_ORIGINS=http://<pi-ip>:3000`。

## 相关

- 三种前端形态的适用边界与失败症状：`docs/reference/deployment.md`
- `NUXT_PUBLIC_API_BASE` 的构建期语义：`docs/reference/configuration.md`
- 为什么不用 Nitro `devProxy` / Vite `server.proxy` 做同源：`openspec/changes/fix-wsl-dev-networking/design.md` D2（WS upgrade 过不去，已实测否决）
- nginx 变体的安装/切换/回滚命令：本文件「nginx 变体」与「切模式 / 回滚」两节
