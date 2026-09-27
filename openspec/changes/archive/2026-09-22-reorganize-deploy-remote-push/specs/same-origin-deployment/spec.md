## MODIFIED Requirements

### Requirement: 同源构建期注入相对 base

前端静态产物在**构建期** SHALL 注入 `NUXT_PUBLIC_API_BASE=/api`，且 SHALL 支持构建期覆盖（`--build-arg`）。因静态 SPA 的 `runtimeConfig.public` 在构建时内联，产物 SHALL NOT 依赖运行期环境变量确定 API 地址；默认的绝对地址 `http://localhost:5100/api` 会把访问主机名烘死进产物，非本机浏览器打开即失败。

#### Scenario: 镜像内产物不含宿主地址

- **WHEN** 以默认参数构建 `deploy/docker/Dockerfile` 的镜像
- **THEN** 产物中不出现 `http://localhost:5100`，浏览器从任意主机访问该镜像暴露的端口时，请求发往同一 origin 的 `/api/...`

#### Scenario: 构建期可覆盖

- **WHEN** 以 `--build-arg NUXT_PUBLIC_API_BASE=http://api.example.com/api` 构建
- **THEN** 产物内 API 地址为该绝对地址（覆盖生效，仍不读运行期环境变量）

#### Scenario: 手工静态产物可判据

- **WHEN** 在 `front/` 以 `NUXT_PUBLIC_API_BASE=/api` 执行静态产物生成
- **THEN** 产物目录内 `localhost:5100` 零命中，且 `index.html` 存在

### Requirement: 同源部署路径与边界文档化

`docs/reference/deployment.md` SHALL 收录同源部署的可用路径（单镜像静态托管 / 反代制品）及各自适用条件，SHALL 说明部署中前端服务的真实形态，并 SHALL NOT 残留已不存在的制品描述（幽灵容器、幽灵 Dockerfile、幽灵环境变量）。`docs/reference/configuration.md` 的 `NUXT_PUBLIC_API_BASE` 条目 SHALL 标明「dev 启动时读取 / 静态构建期内联」两种语义差异。`deploy/same-origin/README.md` SHALL 记录从零到可访问的完整步骤与常见失败对照。

#### Scenario: 部署步骤可复现

- **WHEN** 开发者按 `deploy/same-origin/README.md` 或 deployment.md 的同源小节逐步操作
- **THEN** 每一步都有可直接执行的命令与期望结果，最终浏览器访问同源入口可见列表页数据

#### Scenario: 文档与实际部署形态一致

- **WHEN** 对 `docs/reference/deployment.md` 检索前端服务描述
- **THEN** 描述与仓库实际制品一致（`deploy/compose/docker-compose.yml` 无 `front` 服务、`front/Dockerfile` 不存在、`NUXT_PUBLIC_API_ORIGIN` 已废弃），不存在指向幽灵制品的指引
