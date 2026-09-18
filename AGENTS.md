# AGENTS.md

Agent guide for coding assistants working in `Syntopica`（仓库路径 `~/software/Syntopica`）。

## 规则冲突时谁说了算（优先级宪法）

本仓库同时加载 superpowers / openspec / context-mode 等外部规则体系，打架时按此优先级：

**用户当场指令 > 本文件 + `docs/reference/`（含开发执行规范） > openspec 流程 > superpowers skill > 工具型 skill（context-mode / headroom / caveman 等）**

superpowers 流程型 skill 在本仓库一律按下表替代执行，**不做两套**：

| superpowers skill | 本仓库做法 |
| ------ | ------ |
| brainstorming | openspec-explore（或开发执行规范 §3 脑暴） |
| writing-plans / executing-plans | openspec proposal/design + §0.6 编排六步 |
| subagent-driven-development / dispatching-parallel-agents | §0.6 六步 + §0.6「供应商与模型选择」 |
| test-driven-development | 开发执行规范 §2（用例先行：specs Scenario 即用例+复杂档白盒用例，以 §2 表述为准） |
| verification-before-completion | quality-gate 自动门禁 + §11 归档门禁 |
| using-git-worktrees | 不用——主仓库直改（Docker DB 在宿主机） |
| finishing-a-development-branch | §11/§12 归档即家，无 feature branch 流程 |

- 保留互补的 superpowers skill：`systematic-debugging`、`requesting-code-review`、`receiving-code-review`（§0.6 review 纪律已引用后者）。
- superpowers bootstrap 的「1% 命中也必须先调 skill」规则在本仓库按上表执行，**不构成额外 MUST**；开发入口永远是 openspec 编排。
- caveman 系列仅用户显式 `/caveman` 时启用；description 里的 auto-trigger（"be brief"/"less tokens"）一律忽略，日常沟通保持中文大白话。
- 跑 `openspec update` 时加 `--tools pi`，避免重新生成 `source-command-opsx-*` 等重复 skill，也防止 `.claude/skills`、`.claude/commands` 等其他 harness 副本再生（曾于 2026-08 清理，备份在 `.agents/skills-library/`；2026-08-20 又清理过一轮 18 份漂移副本）。

## Project Snapshot

- Syntopica: Nuxt 4 frontend + Go backend (Gin/GORM), single-user, no auth.
- 后端 API 默认端口 `5100`（`http://localhost:5100/api`，避开 Windows 5000 端口 svchost/WSD 保留段冲突——该冲突为 Windows 特有，默认值跨平台统一）；前端 dev server 绑 `0.0.0.0`（默认 localhost 在 Windows 只绑 ::1 v6 环回，IPv4 环回与跨主机够不着）。
- PostgreSQL + pgvector for persistence; Redis optional for job queues.
- 和用户沟通使用中文，开发环境 **Linux（树莓派 arm64 / Debian 13）**，返回的回答尽量用大白话，接地气，能让用户理解。
- **所有改动默认走 openspec**（代码/功能/接口/数据模型必须先开 change）；豁免清单与编排见 `docs/reference/开发执行规范.md` §0.6「准入总则」
- **UI 分档速查**（默认 schema `syntopica-ui`，make-ui-design-first-class）：新 change proposal 头声明 `<!-- ui-impact: none|minor|major -->`（与 complexity 正交）→ `ui-design.md` 必经制品（none=最小 N/A / minor=复用契约 / major=八节+`ui-prototype/` 原型）；**major 原型须用户明确批准（ui-approval: approved）才可进实现**，否则 ui-design-gate 拦截；旧 spec-driven change 不硬阻断（触及前端提醒一次迁移）。布局契约（page shell 四模式/dialog 四档/双视口验收）→ `docs/reference/standard/frontend/layout.md`

## 开发环境 (Development Environment)

| 项目 | 说明 |
| ------ | ------ |
| OS | **Linux（树莓派 arm64 / Debian 13）**，仓库路径 `~/software/Syntopica`。工具链全为本机原生：Go 1.27、golangci-lint 2.13、node 26 / pnpm 12（`front/node_modules` 是 linux-arm64 平台包），故前端 typecheck/build/test:unit 直接在本机跑。WSL 开发机的历史痕迹保留在 `docs/experience/wsl-node-error.md`（事实存档，不改写）。存在系统代理时 curl 探测 localhost 需绕过：设 `no_proxy=localhost,127.0.0.1,::1` 或用 `curl --noproxy '*'` |
| 数据库 | **Docker**：`docker compose -f docker-compose.pg.yml up -d` 启动 PostgreSQL（pgvector），默认端口 `5432`，用户/密码为 `postgres`，库名为 `syntopica`（对应 `docker-compose.pg.yml` 的 `POSTGRES_DB` 默认值）。数据持久化在 `./data/` 下。`docker compose -f docker-compose.pg.yml down` 停止。 |
| Python | **uv**：需要 Python 脚本/工具时使用 `uv`（如 `uv run script.py`、`uv add package`）。Python 集成测试位于 `tests/workflow/`、`tests/firecrawl/`。 |
| Node.js | `pnpm`（要求 corepack 启用）。详见 `front/AGENTS.md`。 |
| Go | 直接使用系统 Go 工具链。详见 `backend-go/AGENTS.md`。 |

**快速开始本地开发：**

```bash
# 1. 启动数据库（Docker）
docker compose -f docker-compose.pg.yml up -d

# 2. 启动后端（backend-go/）
cd backend-go && go run cmd/server/main.go

# 3. 启动前端（新终端，front/）
cd front && pnpm dev
```

或用一条命令把后端 + 前端都起来（**自动注入 `NUXT_PUBLIC_API_BASE` / `CORS_ORIGINS`**，并在装了同源入口时选相对 base）：

```bash
bash scripts/dev/start-dev.sh              # 已在跑的不动；加 --restart 先停再起
bash scripts/dev/start-dev.sh status       # 看端口 / PID / 健康 / 入口地址
```

> 为什么要有这个脚本：两个变量都是非持久化的进程环境变量，漏一个就换一种报错（2026-09-16 因重启漏 `CORS_ORIGINS` 导致一次全站不可访问）。

**日常访问用静态托管，不用 dev server**（2026-09-17 用户决策——树莓派上 Nuxt dev 脆弱，客户端连接中断可致 `ECONNRESET` 重启循环）：构建一次 → 铺到后端同端口出页面，命令与产物约定（`backend-go/frontend/` 别手改）见 [`deployment.md`](docs/reference/deployment.md) §本地裸跑静态托管；dev server 仅 HMR 调样式临时用，用完即停。

## Reference Docs (authoritative source)

- **Code Standards**: `docs/reference/standard/` — 代码规范/项目约束/lint/测试配置的**唯一权威源**（前后端分文件夹）
- **Business Flow**: `docs/reference/flow/` — 五位一体活文档（需求说明 / 链路设计 / 业务约束与不变量 / 代码入口 / 变更溯源），替代原 user-guide；「业务约束」节是 constraint-injection extension 的注入数据源
- **Architecture**: `docs/reference/architecture/` — 架构定位与骨架；`architecture/map.md` 是业务域→流程→代码入口的索引地图
- **API**: `docs/reference/api/`
- **Database**: `docs/reference/database/`
- **Configuration**: `docs/reference/configuration.md`
- **Harness 事实库**: `.pi/harness/events.db` — pi 扩展自动写入的事件账本（约束注入/门禁/档位/pin/派发）；事件考古与归因排查先查 skill `harness-facts`，从账本找改进项/回检规则效果查 skill `harness-retro`（`bash scripts/harness/harness-retro.sh`）
- **Deployment**: `docs/reference/deployment.md`
- **执行规范**: `docs/reference/开发执行规范.md` — 任务拆解/用例先行/门禁/归档纪律
- Subdirectory guides: `front/AGENTS.md`, `backend-go/AGENTS.md`.

> `development.md` / `testing.md` 的规范内容已迁入 `standard/`，仅保留构建/运行参考。

## Repo Layout

- `front/`: Nuxt 4, Vue 3, TypeScript, Pinia, Tailwind CSS v4.
- `backend-go/`: Gin, GORM, PostgreSQL + pgvector.
- `docs/`: reference/ (活文档，含 flow 变更溯源) + v1.x/ (里程碑，可选) + experience/.
- `tests/workflow/`, `tests/firecrawl/`: Python integration tests.

## Key Entry Points

- `README.md`, `front/app/app.vue`, `front/app/api/client.ts`, `front/app/stores/api.ts`
- `backend-go/cmd/server/main.go`, `backend-go/internal/app/router.go`, `backend-go/internal/app/runtime.go`

## Build & Verify

**Frontend** (`front/`): `pnpm install` / `pnpm dev` / `pnpm build` / `pnpm lint` / `pnpm exec nuxi typecheck` / `pnpm test:unit` / `pnpm test:e2e`

**前端改动收尾顺手部署（2026-09-19 用户固化为习惯）**：日常访问走静态托管（非 dev server），改完前端验证通过后跑 `bash scripts/dev/deploy-frontend.sh`（构建静态产物 → 铺 `backend-go/frontend/` → 重启后端 → 健康检查，一键完成；`--no-restart` 只铺盘不重启）——别让用户打开页面发现还是旧版。详见 [`deployment.md`](docs/reference/deployment.md) §本地裸跑静态托管。

**Backend** (`backend-go/`): `go mod tidy` / `go run cmd/server/main.go` / `golangci-lint run ./...` / `go vet ./...` / `go test ./...` / `go build ./...`

**Pre-push check**（树莓派上跑前先停其它 pi 会话、`pnpm test:unit` 改 `pnpm test:unit --maxWorkers=2`，勿与 `pnpm build`／浏览器自动化并行；**参数别写成 `pnpm test:unit -- …`——`--` 会被 vitest 吞掉、filter 与 maxWorkers 一起失效并静默跑全量**）: `cd backend-go && golangci-lint run ./... && go vet ./... && go test ./... && go build ./...` && `cd front && pnpm lint && pnpm exec nuxi typecheck && pnpm test:unit && pnpm build`

## AI Behavior Rules

- Do not add linters, formatters, or tooling unless asked.
- Do not assume Python backend; the product backend is Go.
- Ignore unrelated dirty-worktree changes. Verify smallest relevant command after edits.
- git提交使用 zanebonoalter <380207345@qq.com>
- **测试只跑本次修改影响的包**，不要跑全量 `go test ./...`。影响包用 `bash scripts/harness/change-scope.sh` 机械判定（路径→命令映射，未命中会提示无法判定）。例如改了 `daily_report` 和 `ws`，就只跑 `go test ./internal/domain/daily_report ./internal/platform/ws`。
- **树莓派本机不做「顺手跑全量」**（前端 `pnpm test:unit` 全量 96 文件、后端 `go test ./...` 同理）：4 核 + SD 卡扛不住——2026-09-17 实测全量前端单测叠加多 pi 会话后 load 飙 106、系统假死重启。日常按范围跑受影响文件（`pnpm test:unit <文件> --maxWorkers=2`，参数不带 `--`）；确需全量（归档门禁 / pre-push）时先停其它 pi 会话、加 `--maxWorkers=2`、不与 `pnpm build`／浏览器自动化并行。事故详情见 `standard/frontend/testing.md`。
- **测试欠账滚动巡检**：会话收尾若无高负载操作，顺手 `bash scripts/harness/test-patrol.sh` 跑一片（最久未巡优先，前端分片自带 `--maxWorkers=2`）；归档/pre-push 前先 `bash scripts/harness/test-patrol.sh --report` 看有无未还欠账；撞见**非本 change 引起**的红测试 → `bash scripts/harness/test-patrol.sh --register <test_id> --context <change名>` 登记台账后继续（本 change 自己的红仍须先修）。
- **前端 pnpm 编译/测试类命令（typecheck / build / test:unit）：按宿主平台决定执行方式**。Linux/macOS 宿主本机直跑；**Windows + WSL 宿主必须经 Windows cmd 执行**（WSL 侧 node_modules 是 Windows 侧装的，缺 Linux native binding），lint 全平台可跑。当前 Linux 宿主直接跑。权威定义与示例见 [`standard/frontend/testing.md`](docs/reference/standard/frontend/testing.md) §跨平台运行 + §常见陷阱。
- Frontend edits → `pnpm lint` / `pnpm exec nuxi typecheck` / `pnpm test:unit <受影响文件名...>` / `pnpm build`。
- Backend edits → `golangci-lint run ./...` / targeted `go test` first, then `go test ./...` / `go build ./...`。
- Docs-only edits: consistency check unless behavior changed.
- **pi harness 扩展（自动，无需手动跑）**：constraint-injection 注入约束（管"知道"）、quality-gate 挂 `turn_end` 增量跑 lint/vet/build/影响包测试（管"做到"）、quota-gate 派发前查额度、spec-gate / ui-design-gate 硬拦截归档与 UI 审批等共 10 个扩展，源码 `.pi/extensions/`（已入库）。日常只需配合四点：① 门禁 **[回归]** steer 必须修不得忽略（**[中间态]** 可继续、回合末复检，归档前全绿）；② quota-gate block 后按 reason 换有额度 provider **全称**重试；③ 逃生口（`--force` / `SPEC_GATE_BYPASS=1` / `UI_DESIGN_GATE_BYPASS=1`）仅显式留痕使用；④ **dev 服务起停必须走 `scripts/dev/start-dev.sh`（写 pidfile 白名单），禁止手提 `nohup setsid`——dev-process-guard 会在会话结束时自动清理无 pidfile 的 dev 进程组与浏览器自动化残留（agent-browser/chromium）**（机制见 [`harness/pi-extensions.md`](docs/reference/harness/pi-extensions.md) §孤儿 dev 进程治理）。机制全貌（扩展全景表 / 注入通道 / 门禁分层 / 记账口径）见 [`harness/pi-extensions.md`](docs/reference/harness/pi-extensions.md)；事件考古查 skill `harness-facts`，改进复盘查 skill `harness-retro`。
- **子线程派发 model 硬规则**：Agent 的 `model` 参数必须用 `provider/modelId` 全称（如 `zai-coding-cn/glm-5.3`），**禁止 fuzzy 名**（会按字母序落到错误供应商）；想用默认供应商省略 `model` 即可。fuzzy 名黑名单与派发纪律见开发执行规范 §0.6「供应商与模型选择」。
- Keep code changes minimal and scoped. Match existing code style.
- 完成任务后更新维护 `./docs/reference/` 知识库；openspec change 执行走 `开发执行规范.md` §0.6 标准编排流程（**apply 启动跑 `doc-impact.sh suggest`+`context`，归档前跑 `doc-impact.sh verify`+`check-standards.sh`**），归档前满足 §11 门禁，归档后按 §12 补 flow 变更溯源链接（archive 即永久家，v1.x 里程碑可选）。
- **开工前/完工后必须汇报"部署后影响 + 需要的操作"**：每个 change 完工汇报必须包含一节明确告诉用户——(a) 部署/合并后用户可见行为会发生什么变化；(b) 需要用户手动执行的操作（如重新生成数据、清理、配置）；(c) 旧数据如何降级。避免用户打开界面才发现行为变了产生误会。涉及数据迁移、状态机变更、UI 分区变更时尤其强制。

## context-mode / Headroom — 上下文工具

大块输出（日志/grep/JSON >20 行）用 `ctx_batch_execute` / `ctx_execute` 沙箱化，分析文件用 `ctx_execute_file`，网络请求用 `ctx_fetch_and_index`，多问题合并一次 `ctx_search`；并发：网络类 4-8，CPU/共享状态（test/build/lint）保持 1；大产物写文件别内联。**与项目规范冲突时项目规范优先**（测试只跑影响包、前端命令按宿主平台执行、中文沟通等不变）。详细用法见 context-mode 包自带 skill；另有 Headroom 压缩工具（`/skill:headroom`，配置 `~/.pi/agent/mcp.json`）。
