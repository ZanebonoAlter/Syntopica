## Purpose

开发与测试期 Docker 镜像的可达性契约：镜像加速来源的配置方式、集成测试必需的镜像清单，以及镜像缺失导致的容器泄漏如何识别与消除——保证换机、清缓存或换网络环境后，`docker compose` 与 testcontainers 仍能按仓库内的原始镜像名拉到镜像并正确回收容器。

## ADDED Requirements

### Requirement: 镜像加速来源以机器级配置提供

系统 MUST 通过在开发主机上配置 Docker 守护进程级镜像加速来源（`registry-mirrors`）解决镜像拉取可达性，而非在仓库内改写镜像名、拼接加速站前缀或引入私有 registry 映射。配置就位后，仓库内既有的原始镜像名（如 `pgvector/pgvector:pg18-trixie`）MUST 可直接 `docker pull`、被 `docker compose` 解析、并被 testcontainers 按默认镜像名使用，无需修改 `docker-compose.pg.yml`、`backend-go/internal/platform/testutil/testutil.go` 等任何仓库文件。

#### Scenario: 原始镜像名经加速来源拉取成功

- **WHEN** 开发主机已配置镜像加速来源，执行 `docker pull pgvector/pgvector:pg18-trixie`（不带任何加速站前缀）
- **THEN** 拉取成功，且仓库内没有任何文件因该配置而被修改

#### Scenario: 仓库内不出现加速站硬编码

- **WHEN** 对**被程序或部署消费的文件**执行 `grep -rn 'docker.1ms.run\|daocloud\|dockerproxy' --include='*.yml' --include='*.yaml' --include='*.go' .`
- **THEN** 零命中：加速来源只存在于开发主机的 `/etc/docker/daemon.json`，MUST NOT 写进 `docker-compose*.yml`、Go 代码、testutil 等任何会被程序读取的位置
- **AND** 文档若给出配置示例，该示例 MUST 标注为「示例，需自行验证当前可达性」——文档示例不构成可靠默认值，也不得被复制成代码常量

#### Scenario: 加速来源不可用时的降级说明

- **WHEN** 配置的单个加速来源失效导致拉取失败
- **THEN** 参考文档给出去除失效来源、改用其他已实测可达来源或改用本机 HTTP 代理的操作路径，且无需改动仓库文件

### Requirement: 集成测试镜像清单文档化

参考文档 MUST 记录集成测试运行所需的全部 Docker 镜像及其用途与来源（至少包含 testcontainers 的 postgres 镜像与 Ryuk sidecar 镜像），并 MUST 使清单与代码中实际引用的镜像名、版本保持一致。

#### Scenario: 清单覆盖代码引用的全部镜像

- **WHEN** 从 `backend-go/` 提取代码与依赖中引用的镜像常量（testutil 的 `pgImage`、testcontainers 的 `ReaperDefaultImage`）
- **THEN** 文档清单包含其中每一个镜像名与标签，无遗漏

#### Scenario: 按清单可补齐缺失镜像

- **WHEN** 新机器或清理镜像缓存后，按文档清单依次拉取
- **THEN** 集成测试可直接运行，无需运行时才发现缺镜像

### Requirement: Ryuk 缺失导致的容器泄漏可识别与清理

文档 MUST 说明 Ryuk sidecar 镜像缺失时的运行时症状（集成测试创建的业务容器不被回收、`docker ps -a` 容器数持续增长）、判定方法（是否存在 Ryuk 容器）与清理方式（批量删除泄漏容器并保留 compose 管理的生产数据库容器）。

#### Scenario: 泄漏症状可被识别

- **WHEN** 连续运行集成测试后统计 `docker ps -aq | wc -l`，数量只增不减，且 `docker ps -a --filter name=ryuk` 无输出
- **THEN** 可依据文档判定为 Ryuk 镜像缺失导致的回收失效，而非测试逻辑缺陷

#### Scenario: 泄漏容器清理不误伤生产库

- **WHEN** 按文档批量清理泄漏容器
- **THEN** Ryuk 缺失期间泄漏的随机名业务容器被删除，`docker-compose.pg.yml` 管理的持久化容器（`syntopica-postgres`）与 `./data` 数据目录 MUST NOT 被删除

#### Scenario: 补齐 Ryuk 后泄漏停止

- **WHEN** 补齐 Ryuk 镜像后运行任一集成测试并等待其结束
- **THEN** 测试期间创建的容器被自动回收，容器总数回到运行前水平
