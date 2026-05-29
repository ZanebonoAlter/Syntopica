# Purpose

定义 Firecrawl 自部署的 docker-compose 编排方案，包括 Firecrawl API/Worker、Redis 和 Playwright 服务，并与核心 compose 网络共享。

## Requirements

### Requirement: docker-compose.firecrawl.yml 提供自部署 Firecrawl 栈
系统 SHALL 提供 `docker-compose.firecrawl.yml` 文件，定义 Firecrawl 自部署所需的全部服务。

#### Scenario: 完整 Firecrawl 栈
- **WHEN** docker-compose.firecrawl.yml 被加载
- **THEN** 定义 `firecrawl` 服务（API + worker）、Redis 服务、Playwright 微服务，Firecrawl API 暴露端口 3002

#### Scenario: 无需认证
- **WHEN** Firecrawl 服务启动
- **THEN** `USE_DB_AUTHENTICATION` 设为 `false`，无需 Supabase 配置

#### Scenario: Redis 连接
- **WHEN** Firecrawl API 和 worker 启动
- **THEN** `REDIS_URL` 指向 compose 内的 redis 服务 `redis://firecrawl-redis:6379`

### Requirement: 与核心 compose 网络共享
docker-compose.firecrawl.yml 的服务 SHALL 加入核心 compose 的共享网络，使 backend 能通过容器名 `firecrawl` 访问。

#### Scenario: 后端连接 Firecrawl
- **WHEN** backend 调用 Firecrawl 抓取
- **THEN** 通过 `http://firecrawl:3002` 访问，无需宿主机端口映射

### Requirement: 独立于核心服务
docker-compose.firecrawl.yml SHALL 可独立于核心服务启动和停止。

#### Scenario: 单独管理 Firecrawl
- **WHEN** 用户执行 `docker compose -f docker-compose.firecrawl.yml down`
- **THEN** 仅停止 Firecrawl 相关容器，核心服务不受影响

### Requirement: 数据持久化
Firecrawl 栈 SHALL 通过 Docker volume 持久化 Redis 数据。

#### Scenario: 重启后数据保留
- **WHEN** Firecrawl 栈被重启
- **THEN** Redis 中的队列和任务数据不丢失
