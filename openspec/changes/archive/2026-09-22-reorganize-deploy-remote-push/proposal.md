<!-- complexity: simple -->
<!-- ui-impact: none -->

## Why

根目录被部署制品淹没：4 个 `docker-compose*.yml`、3 个 `Dockerfile*`、`init.sh`/`init.ps1` 全平铺在最外层，与 `deploy/`、`docker/`、`scripts/` 三个职责重叠的目录并存，找入口成本高（2026-09-22 用户明确提出要整理）。同时部署到既有服务器 10.11.12.59（树莓派 4，Docker Compose 形态）目前只能手动 SSH 操作，没有免密一键推送通道——仓库无 CI，本机直推是唯一可行路径。

## What Changes

- **整理根目录部署制品**：`docker-compose.yml`、`docker-compose.firecrawl.yml`、`docker-compose.rsshub.yml` 移入 `deploy/compose/`；`Dockerfile`、`Dockerfile.demo`（及未被引用的 `Dockerfile.pg`）移入 `deploy/docker/`；`init.sh`、`init.ps1` 移入 `deploy/`。**BREAKING**：所有既有命令路径变化（`docker compose -f docker-compose.firecrawl.yml …` → `-f deploy/compose/…`，`bash init.sh` → `bash deploy/init.sh`）。
- **保留根目录最小入口**：`docker-compose.pg.yml` 留在根目录（日常开发最高频命令，AGENTS.md 写死路径，不动）；`.env` / `.env.example` 留在根目录（compose 默认从工作目录读取）。
- **新增一键免密远程部署**：`scripts/deploy/deploy-remote.sh` —— 本机 rsync 推送代码（排除 `.git`/`node_modules`/`data/`/`backups/`/`logs/`/`.env`）→ 远程 `docker compose up --build -d` → 轮询 `/health` 直到 200；前置自检 SSH 免密（未配则提示 `ssh-copy-id` 一条命令）。
- **同步修全部引用**：README、AGENTS.md、`docs/reference/deployment.md`、`docs/reference/standard/`、init.sh 内部路径、compose 内 `build.dockerfile`/`context`/`docker/postgres/init` volume 相对路径、`demo/docker-compose.demo.yml` 的 `dockerfile:` 指向、`start-dev.sh`/`deploy-frontend.sh` 中涉及路径的注释与逻辑。
- **deployment.md 增补「远程推送部署」一节**：免密配置 + 一键命令 + 与既有 5 种部署形态的选型表。

## Capabilities

### New Capabilities
- `remote-push-deploy`: 本机对远程主机（10.11.12.59 等 Linux/Docker 主机）的免密一键全量部署：SSH 免密前置自检、rsync 增量推送、远程 compose 构建启动、健康检查收敛与失败报告。

### Modified Capabilities
- `deployment-init`: `init.sh` 从项目根目录移至 `deploy/`，「部署唯一入口」的执行路径要求从 `bash init.sh` 改为 `bash deploy/init.sh`（三阶段流程本身不变）。
- `compose-firecrawl`: `docker-compose.firecrawl.yml` 文件路径从仓库根移至 `deploy/compose/`，所有 `-f` 命令路径随之更新。
- `same-origin-deployment`: spec 中对根目录 `Dockerfile`、`docker-compose.yml` 的路径引用更新为 `deploy/docker/Dockerfile`、`deploy/compose/docker-compose.yml`（同源行为要求本身不变）。

## Impact

- **文件移动**：`docker-compose{,.firecrawl,.rsshub}.yml`、`Dockerfile{,.demo,.pg}`、`init.sh`、`init.ps1` 离开根目录。
- **文档**：`README.md`（目录结构树、快速部署命令）、`AGENTS.md`（快速开始）、`docs/reference/deployment.md`（部署方式表、init.sh 流程、Compose 拓扑、回滚步骤）、`docs/reference/standard/` 相关命令片段、`docker-compose.pg.yml` 之外所有 compose 的交叉引用。
- **脚本**：`init.sh`/`init.ps1` 内部 compose 调用与相对路径；`demo/docker-compose.demo.yml` build 指向；`scripts/dev/start-dev.sh`、`scripts/dev/deploy-frontend.sh` 若有硬编码路径。
- **新增**：`scripts/deploy/deploy-remote.sh`（+ shell 测试，仿 `scripts/dev/*.smoke.sh` 形态）。
- **运行时行为不变**：容器拓扑、端口、数据持久化、同源语义全部不动；纯路径与流程编排调整 + 新增部署通道。
- **部署后影响**：既有用户的 muscle-memory 命令（`bash init.sh`、根目录 `docker compose -f docker-compose.firecrawl.yml`）失效，需按新路径执行——归档汇报必须明确列出新旧命令对照。
