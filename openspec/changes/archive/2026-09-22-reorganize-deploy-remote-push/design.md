## Context

见 proposal.md Why。当前约束：

- 4 个 `docker-compose*.yml` 平铺根目录，compose v2 的**项目目录 = compose 文件所在目录**（给定 `-f` 时），移动 compose 文件会连锁改变三件事的解析基准：`.env` 自动加载、`./data` 等相对 volume、`docker/postgres/init` 卷。
- `docker-compose.yml` 内 `build.context: .` + `dockerfile: Dockerfile`（根目录为上下文）；`demo/docker-compose.demo.yml` 用 `context: ..` + `dockerfile: Dockerfile.demo`。
- `.dockerignore` 在根目录（= build 上下文根），移动 Dockerfile 不影响它。
- `Dockerfile.pg` 全仓零引用（仅 proposal 提及），属遗留文件。
- `.gitignore` 已完整刻画「本地状态/运行产物」边界（`data/`、`backups/`、`logs/`、`front/.nuxt/`、`backend-go/frontend/`、`nohup.out`、`.pi/harness/` 等），可作为 rsync 排除清单的单一事实源。
- 本机 `~/.ssh/id_ed25519` 已存在；10.11.12.59:22 可达；无 CI，本机 rsync 直推是唯一远程部署通道。

## Goals / Non-Goals

**Goals:**
- 根目录只留约定入口（`docker-compose.pg.yml`、`.env*`、README/AGENTS），部署制品归拢进 `deploy/`。
- 一条命令完成「本机 → 10.11.12.59」全量部署，全程无 SSH 交互。
- 所有既有文档/脚本引用一次性修净（grep 零残留旧路径）。

**Non-Goals:**
- 不改容器拓扑、端口、数据持久化位置、同源语义。
- 不做 CI/CD、webhook、多环境（staging/prod）区分——单用户单机没有这个复杂度。
- 不动 `deploy/same-origin/` 内部结构（它已是目标形态）。
- 不迁移 `docker-compose.pg.yml`（日常开发最高频命令，AGENTS.md 写死，留在根目录）。

## Decisions

**D1：移动方案 = `deploy/compose/` + `deploy/docker/` + `deploy/`（根收口），compose 调用统一显式 `-f` + `--project-directory`。**
移动后规范调用（一律在仓库根执行）：

```bash
docker compose --project-directory . -f deploy/compose/docker-compose.yml up -d
```

`--project-directory .` 把项目目录钉回仓库根 → `.env` 仍从根加载、`./data` 仍解析到根 `data/`、`docker/postgres/init` 卷路径不变、`build.context: .` 语义不变（相对**调用 cwd**… 见 D2）。compose 文件内部因此**无需改任何 volume/context 路径**，只改 `dockerfile: deploy/docker/Dockerfile`（相对 build context）。
备选（否决）：① 改写所有相对路径为 `../../…`——路径噪音大、每个调用点都得带 `-f` 却仍依赖 cwd，易错；② compose 留根只移 Dockerfile——整理目标达成一半，根目录仍 4 个 yml。

**D2：Dockerfile 移动后 build context 仍为仓库根。**
compose 中 `build: context: .` 在 `--project-directory .`（仓库根执行）下解析为调用时的 cwd 即仓库根；`dockerfile:` 路径相对 context，故写 `deploy/docker/Dockerfile`。`demo/docker-compose.demo.yml` 保持 `context: ..`（demo/ 的上级=根）不变，仅改 `dockerfile: deploy/docker/Dockerfile.demo`。`.dockerignore` 留根（context 根），Dockerfile 内 `COPY` 路径全部相对 context，零改动。
备选（否决）：context 改为 `../..`（相对 compose 文件）——依赖各调用点 cwd 一致性差，且 init.sh 既有「根目录执行」契约（deployment-init spec）更契合 D1。

**D3：`init.sh`/`init.ps1` 移入 `deploy/`，脚本自己 `cd` 到仓库根。**
脚本首行 `cd "$(dirname "${BASH_SOURCE[0]}")/.."`（init.ps1 同理取 `$PSScriptRoot\..`），从此在任何 cwd 执行都等价；对用户仍是 `bash deploy/init.sh`。内部 compose 调用改为 D1 规范形式。spec delta 已同步（「在项目根目录执行」→「以仓库根为工作目录」）。
备选（否决）：留根目录只挪 compose——用户点名要收 init，且根入口本来就该只剩文档。

**D4：`deploy-remote.sh` 采用「rsync 显式排除 + `--delete`」，排除清单镜像 `.gitignore`。**
rsync `-a --delete` 配 `--exclude`（`.git/ node_modules/ data/ backups/ logs/ front/.nuxt/ front/.output/ backend-go/frontend/ backend-go/syntopica nohup.out .codegraph/ .pi/harness/ .pi/run/ .env …`），接收端被排除路径不受 `--delete` 影响（spec 场景据此写）。提供 `--dry-run` 打印将执行的 rsync/ssh 远程命令而不落地——离线可测（smoke 测试据此断言排除清单与命令拼装）。
备选（否决）：`git ls-files | rsync --files-from`——只推受管内容很优雅，但与 `--delete` 不兼容（目标端删除同步失效），且目标端 `.env` 保全依赖排除语义更难验证。

**D5：部署脚本执行流 = 自检 → 本机预编译 → 推送 → 远程 compose → `/health` 轮询。**
1. 自检：`rsync/ssh` 存在 → `ssh -o BatchMode=yes <target> true` 验免密（失败输出一条 `ssh-copy-id` 指引后退出）→ 远程 `docker compose version` + `docker info`。
2. 本机预编译：`CGO_ENABLED=0 go build -o backend-go/syntopica ./cmd/server`——镜像运行阶段拷入的就是这个预编二进制（alpine 需静态链接），不先编会推旧产物；本机与目标同为 aarch64，产物直接可用。
3. 推送：rsync 到 `${REMOTE_PATH:-software/Syntopica}`。
3. 远程：`cd $REMOTE_PATH && docker compose --project-directory . -f deploy/compose/docker-compose.yml up --build -d`。
4. 健康：轮询 `http://<host>:${PORT:-5100}/health` 至 200（超时默认 300s，Pi 构建慢）；失败打印 `docker compose ps` + 容器日志尾部，非零退出。
目标地址参数化 `[user@]host`，默认 `zanebono@10.11.12.59`（免密实测通过的用户），`DEPLOY_TARGET`/首参可覆盖；`REMOTE_PATH` 默认 `software/Syntopica`（相对远端 ~，Open Questions 已定替换旧 demo 栈）。

**D6：文档引用修复 = 一次性 grep 收口，不搞兼容垫片。**
不留根级 stub（如根放个转发 `docker-compose.yml`）——单人项目，垫片只会延长混乱；README/AGENTS/deployment.md/standard/ 全部改为新路径，归档门禁用 grep 断言旧路径零残留（openspec archive 与 specs 里的历史 change 文档除外）。

## Risks / Trade-offs

- [`--project-directory` 语义被忽略，用户手敲 `-f` 时不带它] → 项目目录退回 `deploy/compose/`，`.env`/`data/` 全部错位。缓解：deployment.md 与 README 每处示例**永远连写** `-f` 与 `--project-directory`；init.sh/deploy-remote.sh 封装调用；grep 收口保证无「裸 -f」示例残留。
- [rsync `--delete` 误删目标端状态] → 排除清单必须完整镜像 `.gitignore` 运行产物 + `.env`；smoke 测试断言排除清单含 `.env`/`data/`/`backups/`；`--dry-run` 先行人工核对一次。
- [Pi 4 `--build` 构建超时] → 健康轮询默认 300s 且可调；超时输出诊断而非静默。
- [BREAKING 命令路径，用户 muscle-memory 失效] → 归档汇报强制给新旧命令对照表（AGENTS.md 汇报纪律）；D6 grep 收口保证文档内不再有旧路径。
- [`Dockerfile.pg` 删除是不可逆信息丢失] → 它零引用且内容 35 字节（FROM 一行）；随移动一并挪进 `deploy/docker/` 保留而非删除，零风险。

## Migration Plan

1. `git mv` 移动 9 个文件（3 compose + 3 Dockerfile + 2 init + 见 tasks），同一 commit 完成路径修正，保证任意 commit 可用。
2. 本地验证：`docker compose --project-directory . -f deploy/compose/docker-compose.yml config` 渲染无误；`bash deploy/init.sh --help`（或等价冒烟）；`pnpm` 相关不受影响。
3. 免密配好后 `deploy-remote.sh --dry-run` 人工核对 → 真跑一次全量 → `/health` 200 收口。
4. 回滚：单 commit revert 即回到旧路径（无状态迁移、无数据格式变化）。

## Open Questions（已全部关闭）

- ~~远端用户名/路径~~ → 已确认：`zanebono@10.11.12.59`（免密已通，docker 组权限已配）；远端已有 `~/software/Syntopica`（仅旧 demo 残留：`docker-compose.yml` + 2 个 tar）且 `syntopica-demo`/`syntopica-demo-postgres` 栈在跑（:5080）。
- ~~旧 demo 栈处置~~ → 用户 2026-09-22 决策：**直接替换**。`REMOTE_PATH` 默认 `~/software/Syntopica`；首次部署先 `docker compose down`（或 docker rm -f）旧 demo 两容器，新全量栈成为唯一部署；旧 demo 的 `*.tar` 镜像包按排除清单保留不删（与 .gitignore 一致）。
