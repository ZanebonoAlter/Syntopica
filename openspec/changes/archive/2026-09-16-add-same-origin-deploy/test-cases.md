# 用例：同源反代部署（add-same-origin-deploy）

> 本 change 为部署制品 + 文档，无产品代码改动，故无自动化单测；交付账本以「Pi 上一次真实部署」为主链路故事，全部落点为可执行命令或人工目视。

## 主链路故事

用户从 PC 浏览器访问常驻在 Pi 上的 Syntopica。三条产出路径共用同一目标（浏览器只面对一个 origin）：

- **路径 A（dev 反代）**：Pi 上继续 `NUXT_PUBLIC_API_BASE=/api pnpm dev`（`:3000`），Caddy 以 `Caddyfile.dev` 把前端路径反代给它、把四条后端路径反代给 `:5100`。零构建，改代码即生效（含 HMR）。
- **路径 B（静态产物 + 反代）**：后端按原方式裸跑在 `:5100`，Caddy 在 `:80` 提供 `pnpm generate` 的静态产物并反代四条后端路径。
- **路径 C（单镜像，仓库既有）**：`docker compose up` 建出前后端合一的镜像，后端 `internal/app/static.go` 托管静态产物，`:5100` 单入口。

主链路节拍（以路径 A 为交付主线，路径 B/C 由 tasks 1.1/1.2 与 5.1/5.10 分别保障）：

| # | 动作（节拍） | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| 1 | 构建期注入相对 base（镜像或手工） | 镜像内产物不含宿主地址 / 手工静态产物可判据 | 产物内零 `localhost:5100` | 构建产物 | `grep -rl 'localhost:5100' front/.output/public` 零命中 |
| 2 | 产物解包到 `deploy/same-origin/www/` | 部署步骤可复现 | README 命令逐条可跑 | 制品 | README 验收节 |
| 3 | `docker compose up -d` 起 Caddy | 部署步骤可复现 | 容器 running | 制品 | `docker compose -f deploy/same-origin/docker-compose.yml ps` |
| 4 | 浏览器访问 `http://<pi>/` | 后端路径全量转发 | 列表页有数据；API 同源 | 端到端 | 人工：浏览器 DevTools |
| 5 | 同页加载 feed 图标 | 后端路径全量转发 | 图标正常显示 | 端到端 | `curl -sI http://<pi>/icons/feeds/<id>.png` → `image/*` |
| 6 | 页面建立事件流 | WebSocket 同源转发 | WS open；事件到达 | 端到端 | 人工：浏览器 console `new WebSocket('ws://<pi>/ws')` → open |
| 7 | 刷新深层路由 `http://<pi>/tags` | SPA 深层路由兜底 | 200 + `index.html` | 端到端 | `curl -sI http://<pi>/tags` → 200 |
| 8 | 查部署文档 | 文档与实际部署形态一致 | 无幽灵制品指引 | 文档 | `grep -n 'front/Dockerfile\|NUXT_PUBLIC_API_ORIGIN' docs/reference/deployment.md` 零命中 |

## 变体走查

| 组 | 变体 | 答案 |
|---|---|---|
| 输入 | 空/超长/特殊字符路径 | 不适用（无解析逻辑；`try_files` 兜底由 Caddy 保证） |
| 前置 | 后端未启动 | Caddy 仍启动；请求 `/api/*` 返回 502，静态页仍可打开（故障可定位） |
| 前置 | `www/` 为空目录 | 根路径返回 404/目录列表不可用——README 排障表列出「先构建再拷贝」 |
| 前置 | 80 端口被占 | Caddy 启动失败；README 给 `ss -ltnp \| grep :80` 排查 |
| 时间窗口 | 不适用 | 无时间语义 |
| 幂等 | 重复 `docker compose up -d` | 容器重建，静态目录挂载不变；无副作用 |
| 幂等 | 重复覆盖 `www/` | 新产物即时生效（静态文件无缓存层），浏览器需硬刷新清客户端缓存 |
| 可用性 | 移动端 / 局域网其他设备访问 | 同源入口下自动可用（无需改 CORS 白名单）——这是本形态相对直连的主要收益 |
| 可用性 | HTTPS 页面 | 不适用（`auto_https off` 的 HTTP 局域网形态；公网 TLS 属 Non-Goals） |

## 不适用与留痕

- **白盒分支表**：无算法/状态机/协议实现，分支仅 Caddy 路由匹配（4 条 handle），由 Scenario「后端路径全量转发」覆盖，不另出白盒用例。
- **效果核对**：唯一依赖外部条件的效果是「浏览器可见数据」，由主链路节拍 4 承担；不涉及数据覆盖率或 LLM 行为。
- **展示字段盘点**：不适用（无数据结构变更）。
- **继承与调整（五问句 ⓪）**：本 change 无 MODIFIED/REMOVED Requirements（纯新增 capability），无旧测试资产需要反查。
