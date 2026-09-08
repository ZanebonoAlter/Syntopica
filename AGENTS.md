# AGENTS.md

Agent guide for coding assistants working in `Syntopica` (`D:\project\Syntopica`).

## 规则冲突时谁说了算（优先级宪法）

本仓库同时加载 superpowers / openspec / context-mode 等外部规则体系，打架时按此优先级：

**用户当场指令 > 本文件 + `docs/reference/`（含开发执行规范） > openspec 流程 > superpowers skill > 工具型 skill（context-mode / headroom / caveman 等）**

superpowers 流程型 skill 在本仓库一律按下表替代执行，**不做两套**：

| superpowers skill | 本仓库做法 |
| ------ | ------ |
| brainstorming | openspec-explore（或开发执行规范 §3 脑暴） |
| writing-plans / executing-plans | openspec proposal/design + §0.6 编排六步 |
| subagent-driven-development / dispatching-parallel-agents | §0.6 六步 + 下方「子线程派发参考」模型表 |
| test-driven-development | 开发执行规范 §2（用例先行：specs Scenario 即用例+复杂档白盒用例，以 §2 表述为准） |
| verification-before-completion | quality-gate 自动门禁 + §11 归档门禁 |
| using-git-worktrees | 不用——主仓库直改（Windows 路径 + Docker DB） |
| finishing-a-development-branch | §11/§12 归档即家，无 feature branch 流程 |

- 保留互补的 superpowers skill：`systematic-debugging`、`requesting-code-review`、`receiving-code-review`（§0.6 review 纪律已引用后者）。
- superpowers bootstrap 的「1% 命中也必须先调 skill」规则在本仓库按上表执行，**不构成额外 MUST**；开发入口永远是 openspec 编排。
- caveman 系列仅用户显式 `/caveman` 时启用；description 里的 auto-trigger（"be brief"/"less tokens"）一律忽略，日常沟通保持中文大白话。
- 跑 `openspec update` 时加 `--tools pi`，避免重新生成 `source-command-opsx-*` 等重复 skill，也防止 `.claude/skills`、`.claude/commands` 等其他 harness 副本再生（曾于 2026-08 清理，备份在 `.agents/skills-library/`；2026-08-20 又清理过一轮 18 份漂移副本）。

## Project Snapshot

- Syntopica: Nuxt 4 frontend + Go backend (Gin/GORM), single-user, no auth.
- Frontend API: `http://localhost:5000/api`; WebSocket: `ws://localhost:5000/ws`.
- PostgreSQL + pgvector for persistence; Redis optional for job queues.
- 和用户沟通使用中文，开发环境 Windows, 返回的回答尽量用大白话，接地气，能让用户理解。
- **所有改动默认走 openspec**（代码/功能/接口/数据模型必须先开 change）；豁免清单与编排见 `docs/reference/开发执行规范.md` §0.6「准入总则」
- **UI 分档速查**（默认 schema `syntopica-ui`，make-ui-design-first-class）：新 change proposal 头声明 `<!-- ui-impact: none|minor|major -->`（与 complexity 正交）→ `ui-design.md` 必经制品（none=最小 N/A / minor=复用契约 / major=八节+`ui-prototype/` 原型）；**major 原型须用户明确批准（ui-approval: approved）才可进实现**，否则 ui-design-gate 拦截；旧 spec-driven change 不硬阻断（触及前端提醒一次迁移）。布局契约（page shell 四模式/dialog 四档/双视口验收）→ `docs/reference/standard/frontend/layout.md`

## 开发环境 (Development Environment)

| 项目 | 说明 |
| ------ | ------ |
| OS | **Windows**（WSL2 `bash` 可用，但路径使用 Windows 格式如 `D:/project/...`） |
| 数据库 | **Docker**：`docker compose -f docker-compose.pg.yml up -d` 启动 PostgreSQL（pgvector），默认端口 `5432`，用户/密码为 `postgres`，库名为 `syntopica`（对应 `docker-compose.pg.yml` 的 `POSTGRES_DB` 默认值）。数据持久化在 `./data/` 下。`docker compose -f docker-compose.pg.yml down` 停止。 |
| Python | **uv**：需要 Python 脚本/工具时使用 `uv`（如 `uv run script.py`、`uv add package`）。Python 集成测试位于 `tests/workflow/`、`tests/firecrawl/`。 |
| Node.js | `pnpm`（要求 corepack 启用）。详见 `front/AGENTS.md`。 |
| Go | 直接使用系统 Go 工具链。详见 `backend-go/AGENTS.md`。 |

## Headroom (Context Compression)

[Headroom](https://headroom-docs.vercel.app/) 通过 MCP 集成，用于压缩大量工具输出（日志、grep、JSON 等）以节省 token。详细用法见 `/skill:headroom`。配置文件：`~/.pi/agent/mcp.json`。

**快速开始本地开发：**

```bash
# 1. 启动数据库（Docker）
docker compose -f docker-compose.pg.yml up -d

# 2. 启动后端（backend-go/）
cd backend-go && go run cmd/server/main.go

# 3. 启动前端（新终端，front/）
cd front && pnpm dev
```

## Reference Docs (authoritative source)

- **Code Standards**: `docs/reference/standard/` — 代码规范/项目约束/lint/测试配置的**唯一权威源**（前后端分文件夹）
- **Business Flow**: `docs/reference/flow/` — 五位一体活文档（需求说明 / 链路设计 / 业务约束与不变量 / 代码入口 / 变更溯源），替代原 user-guide；「业务约束」节是 constraint-injection extension 的注入数据源
- **Architecture**: `docs/reference/architecture/` — 架构定位与骨架；`architecture/map.md` 是业务域→流程→代码入口的索引地图
- **API**: `docs/reference/api/`
- **Database**: `docs/reference/database/`
- **Configuration**: `docs/reference/configuration.md`
- **Harness 事实库**: `.pi/harness/events.db` — pi 扩展自动写入的事件账本（约束注入/门禁/档位/pin/派发）；事件考古与归因排查先查 skill `harness-facts`
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

**Backend** (`backend-go/`): `go mod tidy` / `go run cmd/server/main.go` / `golangci-lint run ./...` / `go vet ./...` / `go test ./...` / `go build ./...`

**Pre-push check**: `cd backend-go && golangci-lint run ./... && go vet ./... && go test ./... && go build ./...` && `cd front && pnpm lint && pnpm exec nuxi typecheck && pnpm test:unit && pnpm build`

## AI Behavior Rules

- Do not add linters, formatters, or tooling unless asked.
- Do not assume Python backend; the product backend is Go.
- Ignore unrelated dirty-worktree changes. Verify smallest relevant command after edits.
- git提交使用 zanebonoalter <380207345@qq.com>
- **测试只跑本次修改影响的包**，不要跑全量 `go test ./...`。影响包用 `bash scripts/change-scope.sh` 机械判定（路径→命令映射，未命中会提示无法判定）。例如改了 `daily_report` 和 `ws`，就只跑 `go test ./internal/domain/daily_report ./internal/platform/ws`。
- **前端 pnpm 编译/测试类命令（typecheck / build / test:unit）必须通过 Windows cmd 执行**，WSL 环境缺少 native binding（typecheck 如 `@oxc-parser/binding-linux-x64-gnu`；test:unit 经 Vite→rollup 缺 `@rollup/rollup-linux-x64-gnu`）会失败。lint 可在 WSL 跑。权威定义见 [`standard/frontend/testing.md`](docs/reference/standard/frontend/testing.md) §跨平台运行 + §常见陷阱。示例：

  ```bash
  # lint — WSL 可用
  cd front && pnpm lint
  # typecheck / build / test:unit — 必须用 cmd
  cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm exec nuxi typecheck"
  cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm build"
  cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm test:unit 2>&1"
  ```

- Frontend edits → `pnpm lint` / `pnpm exec nuxi typecheck` / `pnpm test:unit` / `pnpm build`。
- Backend edits → `golangci-lint run ./...` / targeted `go test` first, then `go test ./...` / `go build ./...`。
- Docs-only edits: consistency check unless behavior changed.
- **pi 扩展全景**（`.pi/extensions/`，gitignored；快照同步 `docs/research/`）：

| 扩展 | 挂点 | 触发 | 软硬 | fail 策略 | 事件库记账 |
| --- | --- | --- | --- | --- | --- |
| constraint-injection | `before_agent_start` | 每 turn 重建 system prompt 注入块 | 软（不干预工具） | fail-open（注入失败不阻断） | constraint.inject / pin.* / mode.set |
| quality-gate | `turn_end` | 增量路由命中后端/前端；另落 `edit.map` 归属地图（增量路径 × boundChange 聚合，coordinate-concurrent-changes） | 软 steer 催修 | fail-open（门禁故障放行） | gate.check / edit.map |
| quota-gate | Agent 派发前 | 子线程派发前查额度 | 硬 block（低额度阻断派发） | fail-open（查询失败放行） | policy.decision（quota-low/exhausted=block、quota-query-failed=fail-open、fuzzy-model-resolve=warn） |
| spec-gate | `tool_call` | bash 命中 `openspec archive` | 硬 block（归档门禁五检查：doc-impact/standards/尾三节/scenario-trace/UI 验收证据；另检查⑤'归档并发 warn 不 block：树上存在归属其他 active change 的未 commit 文件 → steer 提醒 + concurrent-dirty-tree 记账，冷启动零输出） | `--force` / `SPEC_GATE_BYPASS=1` 逃生口留痕 | policy.decision（archive-check-failed=block、explicit-bypass=bypass、acceptance-wording=warn、concurrent-dirty-tree=warn；UI 缺证据另记 ui-design-gate block ui-verification-missing） |
| ui-design-gate | `tool_call` | implementation 档绑定 syntopica-ui schema change：Agent 派发与 edit/write 项目代码，major 原型未批准（合同 block）时拦截；当前 change 的 ui-design.md/ui-prototype/** 修复不受限 | 硬 block（legacy schema 仅 front mutation 每会话/change warn 一次） | fail-open（检查异常放行+告警+记账）；`UI_DESIGN_GATE_BYPASS=1` 显式旁路留痕 | policy.decision（ui-impact-missing/ui-impact-mismatch/ui-design-missing/ui-prototype-missing/ui-approval-pending=block、explicit-bypass=bypass、ui-gate-check-failed=fail-open；健康放行零记录） |
| entry-gate | `turn_end` | 实现档切入后 complex 缺 test-cases 文档 | 软 steer 提醒 | fail-open | gate.check（cmd=entry-gate） |
| test-scope-guard | `tool_call` | bash 跑全量 `go test ./...` | 软提醒（日常只跑影响包） | fail-open | policy.decision（full-go-test：soft=warn / hard=block） |
| tool-output-spill | `tool_result` | 工具输出 >32KB（`.pi/harness.json`） | 落盘替换+有界预览 | fail-open（spill 失败原样通过） | spill.write |
| harness-telemetry | `session_start`/`tool_call`/`tool_result` | 通用事实采集（不干预） | — | fail-safe（断链不伪造） | session.start / subagent.* |

- **约束注入（自动，管"知道"）**：`.pi/extensions/constraint-injection.ts` 在 `before_agent_start` 每 turn 注入约束上下文——openspec 档位/change 绑定、flow「业务约束与不变量」节按 **proposal.md 业务域声明**（头部 `<!-- constraint-domains: 域, ... -->`，域名=flow 文档 basename 如 `daily-report`；纯工具链 change 可不写，widget 提示无域声明属预期；**声明域注入=红线层**——约束节内顶层列表项首个加粗红线句逐行 + 细节层取回指引，红线层提取 0 条或低于 512B 回退全节，格式见 `standard/shared/doc-authoring.md`「约束节红线句格式」）+ 对话输入关键词命中（**全节注入**，细节层通道，只增不减保前缀缓存）、standard/flow 文档按头部 `doc-impact-applies` 标签对编辑路径 JIT 命中（**全节注入**）、`pin_finding` 工具持久化探索发现（档激活落 change 的 explore-findings.md，无档落 `docs/research/`）。配置 `.pi/constraint-injection.json`，常驻索引 `docs/reference/constraints-index.md`（旧 `doc-impact.sh context` 已退役）。
- **pi 增量门禁（自动，管"做到"）**：`.pi/extensions/quality-gate.ts` 已挂 `turn_end`，按**会话内增量路由**触发——会话开始时的 git 脏文件进基线不触发，仅本回合新增/变化路径命中后端（`backend-go/**.go`）才跑 `golangci-lint`+`go vet`+`go build`+影响包 `go test -short`（经 `scripts/change-scope.sh` 判定，DB 集成测试 -short 下自动 skip），命中前端（`front/` 非 .md）才跑 eslint（经 cmd.exe 的 Windows 原生 `--cache` 增量调用，实测 2~6s；WSL DrvFS 跑同命令 ~17 倍慢；`front/package.json` 的 `pnpm lint` 仍全量供人工/归档用）；lint 先行作短路哨兵——lint 报编译失败（typechecking error）时 vet/build/test 必红同因，跳过执行不记账，无短路时 vet/build 并行。上回合失败未转绿的命令粘性重跑（催修，防"口头修复"漂移）。失败以 `steer` 消息分级喂回：**[回归]**（上回合尚绿，agent 必须修，不得忽略）与**[中间态]**（从未绿的新代码中间态，agent 若正在推进可继续、回合末复检）——归档前全绿硬要求不变（§11）。成功事件采样记账（会话首条与转绿锚点必记，其后每 5 连续成功记 1 条），失败全量记账。不跑前端 typecheck/build（需 cmd.exe）与完整集成测试（不带 -short 的 go test），这些仍由 agent 手动跑 + §11 归档门禁兜底。门禁分层见 `docs/reference/开发执行规范.md` §4.1。
- Keep code changes minimal and scoped. Match existing code style.
- 完成任务后更新维护 `./docs/reference/` 知识库；openspec change 执行走 `开发执行规范.md` §0.6 标准编排流程（**apply 启动跑 `doc-impact.sh suggest`+`context`，归档前跑 `doc-impact.sh verify`+`check-standards.sh`**），归档前满足 §11 门禁，归档后按 §12 补 flow 变更溯源链接（archive 即永久家，v1.x 里程碑可选）。
- **开工前/完工后必须汇报"部署后影响 + 需要的操作"**：每个 change 完工汇报必须包含一节明确告诉用户——(a) 部署/合并后用户可见行为会发生什么变化；(b) 需要用户手动执行的操作（如重新生成数据、清理、配置）；(c) 旧数据如何降级。避免用户打开界面才发现行为变了产生误会。涉及数据迁移、状态机变更、UI 分区变更时尤其强制。

子线程派发参考（pi 的 subagent 派发如何选供应商与模型）:

> ⚠️ **硬规则：Agent 的 `model` 参数必须用 `provider/modelId` 全称**（如 `zai-coding-cn/glm-5.3`），**禁止用 fuzzy 名**（如 `glm-5.3`）。
>
> 原因：pi 里有 10 个 model id 跨多个 provider 重复注册（`glm-5.3`/`glm-5.1`/`glm-4.7`/`glm-5-turbo`/`glm-4.5-air`/`glm-5v-turbo`/`deepseek-v4-pro`/`deepseek-v4-flash`/`mimo-v2.5`/`mimo-v2.5-pro`）。fuzzy 名会按字母序解析到**非预期**的供应商——实测传 `glm-5.3` 会落到 `opencode-go`（字母序最先），而不是默认供应商 `zai-coding-cn`。想用默认供应商（`zai-coding-cn/glm-5.3`）时，**省略 `model` 参数即可**；一旦显式传 fuzzy 名反而绕过默认、落到错误供应商。实时清单查 `pi --list-models`。

> glm 系列统一走当前默认供应商 `zai-coding-cn`（国内 coding 专用），优于 `zai`（国际站）/ `opencode-go`（聚合网关）。
>
> change 执行的完整编排（主线程调度 + 子线程派发六步）见 `docs/reference/开发执行规范.md` §0.6。

> 🚦 **额度门禁（quota-gate）**：`.pi/extensions/quota-gate.ts` 会在每次 Agent 派发前自动查目标 provider 剩余额度（GLM/Kimi 查 5h/周窗口——GLM 老套餐仅 5h 窗口，MCP 的 TIME_LIMIT 不参与判定；DeepSeek 查余额；opencode-go 无 API 直接放行）。窗口剩余 <10% 或余额 <¥1 时派发被 **block**，reason 含剩余情况/重置时间/建议。收到阻断 reason 后：按 reason 提示换有额度的 provider 全称重试，或等窗口重置。阈值可用环境变量 `QUOTA_GATE_WINDOW_PCT` / `QUOTA_GATE_MIN_BALANCE` 调整；查询失败一律 fail-open 放行。

## context-mode — 上下文路由（简版）

context-mode 提供 11 个 `ctx_*` 工具，把大块输出（日志/grep/JSON）沙箱化、索引进 FTS5，避免灌爆上下文窗口。**与本文件其他规则冲突时，项目规范优先**（前端编译走 Windows cmd、测试只跑影响包、中文沟通等不变）。

- **Shell 输出 >20 行**：用 `ctx_batch_execute`（多命令一次跑 + 自动索引）或 `ctx_execute`；bash 只留给 git/mv/ls/install 等小输出命令。
- **读文件**：为了编辑 → 正常 read；为了分析/总结 → `ctx_execute_file`。
- **curl/wget 与内联 HTTP 会被拦截** → 用 `ctx_fetch_and_index` + `ctx_search`。
- **多问题合并**一次 `ctx_search(queries: [...])`；resume 后先 `ctx_search(sort: "timeline")` 搜历史再问用户。
- **并发**：网络类 `concurrency: 4-8`；CPU/共享状态（test/build/lint）保持 1。
- **大产物写文件**，别内联进上下文。

详细用法见 context-mode 包自带 skill；`ctx stats/doctor/upgrade/purge` 对应同名 MCP 工具。
