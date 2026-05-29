# SQLite Archive Docker Design

## Goal

为当前 SQLite 分支补齐一套可长期保留的容器启动方式，让后续在这个分支上做 bugfix 时，仍然可以直接用 `docker compose up --build` 启动前后端，并把 SQLite 数据文件明确落到仓库根目录下的 `data/`。

## Scope

- 新增根目录 `docker-compose.yml`
- 新增根目录 `.env.example`
- 为 `backend-go/` 和 `front/` 分别新增生产构建用 Dockerfile
- 后端支持通过环境变量覆盖端口、SQLite 路径、CORS 来源
- 前端统一改成运行时 API/WS 地址解析，避免继续写死 `localhost:5000`
- 增加最小测试覆盖配置解析逻辑

## Decisions

### Compose 形态

- 使用单个根目录 `docker-compose.yml`
- 使用 Dockerfile 预构建前后端镜像，而不是运行时即时安装依赖
- 继续保留 SQLite，不引入额外数据库服务

### 数据落点

- 宿主机目录固定为 `./data`
- SQLite 文件名可配置，默认 `rss_reader.db`
- 容器内统一映射到 `/app/data/<db-file>`

### 端口策略

- 宿主机暴露端口通过 `.env` 配置
- 默认前端 `3000`，后端 `5000`
- 容器内部端口保持固定，减少镜像复杂度

### 前端 API 策略

- 浏览器侧请求走 `NUXT_PUBLIC_API_BASE` / `NUXT_PUBLIC_API_ORIGIN`
- Nuxt 服务端渲染场景走内部地址 `API_INTERNAL_BASE`
- WebSocket 只从公开 API Origin 派生，保证浏览器可连通

## Risks

- 当前前端存在少量写死的 `localhost:5000`，必须一起收敛，否则容器启动后会出现部分接口仍访问本地开发端口的问题
- CORS 默认配置只允许本地端口，容器部署必须允许从 `.env` 派生出的前端地址覆盖
- SQLite bind mount 目录如果不忽略，容易把运行数据误提交进仓库

## Verification

- 后端单测：环境变量能覆盖默认配置
- 前端单测：运行时配置能正确解析 public/internal API 地址
- 后端构建：`go test ./internal/platform/config -v`
- 前端构建：`pnpm test:unit -- app/utils/api.test.ts`

## Notes

- 这次只做归档分支的可用性，不引入新的部署抽象
- 不创建 git commit，除非后续单独要求
