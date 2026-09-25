# remote-push-deploy Specification

## Purpose

定义本机对远程 Linux/Docker 主机（如 10.11.12.59 树莓派 4）的免密一键全量部署能力：SSH 免密前置自检、代码增量推送、远程 compose 构建启动与健康检查收敛，使部署不再依赖手动 SSH 操作。

## Requirements

### Requirement: 一键推送部署入口
系统 SHALL 提供 `scripts/deploy/deploy-remote.sh` 作为对远程主机的一键全量部署入口，单次执行完成：前置自检 → 代码推送 → 远程构建启动 → 健康检查，全程不要求用户手动 SSH 交互。目标主机与远程路径 SHALL 可通过参数覆盖，默认目标为 10.11.12.59。

#### Scenario: 免密就绪时一键完成部署
- **WHEN** 本机已对目标主机配置 SSH 免密，用户执行 `bash scripts/deploy/deploy-remote.sh`
- **THEN** 脚本推送代码、在目标主机执行 compose 构建启动，并轮询目标 `/health` 直到返回 200 后报告成功

#### Scenario: 免密未配置时给出可执行指引
- **WHEN** SSH 自检失败（需密码或密钥未装到目标主机）
- **THEN** 脚本以非零码退出，并输出一条可直接复制执行的 `ssh-copy-id` 指引命令，不进行半程推送

#### Scenario: 远程健康检查失败时报告
- **WHEN** 远程 compose 启动后 `/health` 在超时内未返回 200
- **THEN** 脚本以非零码退出并输出远程服务状态摘要（含容器状态），不静默成功

### Requirement: 推送内容与排除清单
部署推送 SHALL 仅同步仓库受版本管理的部署所需内容，SHALL 排除 `.git`、`node_modules`、`data/`、`backups/`、`logs/`、`.env` 等本地状态与运行产物，SHALL NOT 覆盖目标主机上的 `.env` 与持久化数据。

#### Scenario: 目标主机 .env 不被覆盖
- **WHEN** 目标主机已存在 `.env`（含本地配置），执行一次部署推送
- **THEN** 目标 `.env` 内容保持不变，本地 `.env` 不被推送覆盖

#### Scenario: 运行数据不被同步
- **WHEN** 本机存在 `data/`、`backups/`、`logs/` 内容时执行部署推送
- **THEN** 目标主机的同名目录内容不被本次推送删除或替换（`--delete` 不作用于被排除路径）

### Requirement: Demo 镜像推送部署
部署脚本 SHALL 提供 demo 模式（`--demo`）：镜像在**本机**构建（`deploy/docker/Dockerfile.demo`，含 seed 烘焙）→ `docker save` 管道推送远端 `docker load`（远端零构建）→ 以 `demo/docker-compose.run.yml` 拉起只读演示栈 → 健康检查 `:5080/health`。demo seed（`demo/seed/seed.sql`）缺失时 SHALL 在构建前以非零码停止并给出 `dump-sanitizer` 生成指引；推送内容 SHALL NOT 包含 seed.sql（已在镜像内）。

#### Scenario: demo 一键部署
- **WHEN** seed 就绪、本机执行 `bash scripts/deploy/deploy-remote.sh --demo`
- **THEN** 本机构建 demo 镜像、推送载入远端、远端以 `DEMO_TAG=<构建tag>` 起 run.yml 栈，`/health` 返回 200 后报告 demo 入口地址（默认 :5080）

#### Scenario: seed 缺失时拒绝构建
- **WHEN** `demo/seed/seed.sql` 不存在或为空时执行 `--demo`
- **THEN** 脚本以非零码退出，输出 `cd backend-go && go run ./cmd/dump-sanitizer` 生成指引，不构建不推送

#### Scenario: 远端旧镜像 tag 保留可回滚
- **WHEN** 以新 `DEMO_IMAGE_TAG` 部署成功后需要回滚
- **THEN** 远端仍保留旧 tag 镜像，以 `DEMO_TAG=<旧tag>` 重跑 run.yml 即可切回（部署不删除旧 tag）

### Requirement: 部署前置环境自检
deploy-remote.sh SHALL 在推送前自检：本机 `rsync`/`ssh` 可用、SSH 免密生效、目标主机 Docker 与 docker compose 可用。任一自检失败 SHALL 立即停止并给出具体失败项与修复动作。

#### Scenario: 目标主机无 Docker
- **WHEN** 目标主机上 `docker compose` 不可用
- **THEN** 脚本在推送前以非零码退出，明确报告 Docker/compose 缺失，不推送任何文件
