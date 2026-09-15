## 1. 单镜像同源路径修复

- [x] 1.1 `Dockerfile` 前端构建阶段：加 `ARG NUXT_PUBLIC_API_BASE=/api` + `ENV NUXT_PUBLIC_API_BASE=${NUXT_PUBLIC_API_BASE}` 再 `RUN pnpm generate` —— 镜像内前端产物改走相对 base（后端 `internal/app/static.go` 本就同源托管静态产物，绝对 `localhost` 会把访问主机烘死）。验证：`grep -n 'NUXT_PUBLIC_API_BASE' Dockerfile` → 命中 ARG 与 ENV 两行
- [x] 1.2 注入接线核对：`ARG`/`ENV` 行号 < `RUN pnpm generate` 行号（`grep -n 'NUXT_PUBLIC_API_BASE\|RUN pnpm generate' Dockerfile`）；等价命令已实证注入生效（`NUXT_PUBLIC_API_BASE=/api pnpm generate` → `front/.output/public/index.html` 内含 `apiBase:"/api"`）。**完整 `docker build --target front-build` 未执行**——本机镜像源拉不到 `node:22-alpine`，留痕见 5.9

## 2. 同源反代制品

- [x] 2.1 新增 `deploy/same-origin/Caddyfile`：`:80` 单入口（`admin off` + `auto_https off`），四条后端路径反代到 `127.0.0.1:5100` —— `/api/*`、`/ws` 与 `/ws/*`、`/icons/*`、`/health`；其余路径 `root * /srv/www` + `try_files {path} /index.html` + `file_server`。不引入额外模块或外部文件。验证：`caddy validate` → `Valid configuration`（已跑，见 5.1）
- [x] 2.2 新增 `deploy/same-origin/docker-compose.yml`：`caddy:2-alpine` + `network_mode: host` + 挂载 `Caddyfile` 与 `www`（只读）+ 命名卷 `/data`、`/config`；**不含** `ports:`（host 网络下写了 compose 直接报错）。验证：见 5.2- [x] 2.3 新增 `deploy/same-origin/README.md`：前置条件 → PC 构建（含构建期注入）→ 打包拷贝 → 起容器 → 浏览器验收 → 排障表（漏注入的现场特征 / `ports:` 混写报错 / 80 端口占用 / 后端未起返回 502）→ 回滚。验证：README 内含路径的命令与 2.1/2.2 实际路径一致

## 3. 测试

<!-- 本 change 无产品逻辑改动（Dockerfile 构建参数 + 新增部署制品 + 文档），无自动化单测；交付账本见 §5 验证节 -->

- [x] 3.1 静态产物注入判据（命令可复现）：`cd front && NUXT_PUBLIC_API_BASE=/api pnpm generate` → `front/.output/public/index.html` 存在，且 `grep -rl 'localhost:5100' front/.output/public` 零命中
- [ ] 3.2 Pi 端到端验收（人工，**需用户执行**）：按 `deploy/same-origin/README.md` 部署后逐条执行 §5 验证节的人工判据

## 4. 文档

<!-- doc-impact: deployment, configuration -->

- [x] 4.1 `docs/reference/deployment.md`：修正前端服务描述 —— 删幽灵 `front` 容器与 `backend-go/Dockerfile`/`front/Dockerfile` 段落，改为实际形态（单一根 `Dockerfile` 出前后端合一镜像 + 后端 `static.go` 同源托管静态产物）；架构图同步；清掉已废弃的 `NUXT_PUBLIC_API_ORIGIN` 行
- [x] 4.2 `docs/reference/deployment.md`：新增「前端服务的三种形态」与「同源反代部署（Caddy）」两小节（适用条件、四步流程、指向 `deploy/same-origin/README.md`）
- [x] 4.3 「多机 / 远程访问」小节由 `fix-wsl-dev-networking` 任务 4.3 交付（其 spec 有对应 Requirement「多机访问口径文档化」）；本 change 只在 4.2 的小节里指向它，不重复声明该内容
- [x] 4.4 `docs/reference/configuration.md`：`NUXT_PUBLIC_API_BASE` 条目补「dev 启动时读取 / 静态构建期内联」语义；`CORS_ORIGINS` 条目补精确匹配语义；Docker Compose 变量表把失效的 `FRONT_PORT`/`BACKEND_PORT` 换成实际生效的 `PORT` 并附废弃说明；「Docker 推荐方式」小节同步为两服务

## 5. 验证

- [x] 5.1 Caddyfile 校验与路由语义（本机已实测）：① `caddy validate` → `Valid configuration`，无格式告警；② 同一容器内 `caddy file-server --root /fake --listen 127.0.0.1:5100` 作假后端 + 挂载**未改动的原版 Caddyfile** + `-p 18080:80`，实测 `/` 200 SPA 壳、`/tags` 200 SPA 壳、`/settings/anything` 200 兜底、`/api/categories` 200 后端 JSON、`/health` 200 后端 JSON、`/_nuxt/app.js` 200 `text/javascript`、`/icons/feeds/42.png` 200 `image/png`；带 upgrade 头的 `/ws` 与 `/ws/` 均 404（假后端无此文件）且响应体不含 SPA 壳 → 后端路径未被前端兜底吃掉
- [ ] 5.2 host 网络与端口映射不混写：`grep -c 'ports:' deploy/same-origin/docker-compose.yml` → `0`，且 `python3 -c "import yaml;m=yaml.safe_load(open('deploy/same-origin/docker-compose.yml'))['services']['caddy'];assert 'ports' not in m and m['network_mode']=='host';print('OK')"` → `OK`
- [x] 5.3 静态产物注入生效：`grep -rl 'localhost:5100' front/.output/public` → 无输出（零命中）
- [x] 5.4 Dockerfile 注入参数在场：`grep -cE '^(ARG|ENV) NUXT_PUBLIC_API_BASE' Dockerfile` → `2`
- [x] 5.5 Scenario 映射对账：`bash scripts/scenario-trace.sh openspec/changes/add-same-origin-deploy` → 退出码 0（8/8 映射齐全）
- [x] 5.6 文档域对账：`bash scripts/doc-impact.sh verify openspec/changes/add-same-origin-deploy` → 退出码 0
- [x] 5.7 规范自检：`bash scripts/check-standards.sh --change add-same-origin-deploy` → 143 通过 / 0 失败
- [ ] 5.8 Pi 端到端（人工，**需用户执行**）：浏览器访问 `http://<pi-ip>/` 列表页有数据，Network 面板请求全部发往 `http://<pi-ip>/api/...`
- [x] 5.9 环境受限留痕：`docker build --target front-build` **未执行** —— 本机两个镜像源均拉不到 `node:22-alpine`（`image-mirror.r2.daocloud.vip` EOF / `dockerproxy.net` 亦失败）。用户侧若需完整验证：`docker build --target front-build -t syntopica-front-probe .`

| Scenario | 测试文件 |
|---|---|
| 镜像内产物不含宿主地址 | 人工：`grep -n 'NUXT_PUBLIC_API_BASE' Dockerfile` 命中 ARG/ENV 且行号先于 `RUN pnpm generate`；`front/.output/public/index.html` 含 `apiBase:"/api"` |
| 构建期可覆盖 | 人工：`docker build --target front-build --build-arg NUXT_PUBLIC_API_BASE=http://api.example.com/api .`（本机镜像源受限未执行，见 5.9） |
| 手工静态产物可判据 | 人工：`grep -rl 'localhost:5100' front/.output/public` 零命中（tasks 3.1 已执行） |
| 后端路径全量转发 | 人工：本机已实测（见 5.1②）路由与内容类型；Pi 上复核 `curl -sI http://<pi-ip>/icons/feeds/<id>.png` 返回 `image/*`、`curl -s http://<pi-ip>/api/categories` 返回分类 JSON、`curl -s http://<pi-ip>/health` 返回后端健康 JSON |
| WebSocket 同源转发 | 人工：本机已实测 `/ws` 路由归属（见 5.1②）；Pi 上浏览器 console 执行 `new WebSocket('ws://<pi-ip>/ws')` 触发 open 事件 |
| SPA 深层路由兜底 | 人工：本机已实测 `/tags` 与 `/settings/anything` 返回 200 SPA 壳（见 5.1②）；Pi 上复核 `curl -sI http://<pi-ip>/tags` 返回 200 |
| 部署步骤可复现 | 人工：按 `deploy/same-origin/README.md` 逐步操作至浏览器可用 |
| 文档与实际部署形态一致 | 人工：`grep -rn 'front/Dockerfile\|NUXT_PUBLIC_API_ORIGIN' docs/reference/` 零命中；`docker-compose.yml` 服务清单为 `postgres` + `syntopica` |
