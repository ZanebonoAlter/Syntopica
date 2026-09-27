## MODIFIED Requirements

### Requirement: 改动范围→验证命令判定（change-scope 脚本）

系统 SHALL 提供 `scripts/change-scope.sh`，对给定 base ref（默认 `HEAD`）收集改动文件集合（committed/staged/unstaged/untracked，与 doc-impact.sh `changed_files()` 同口径），并按路径映射表输出建议执行的最小验证命令清单。脚本 SHALL 在 POSIX bash 环境（Linux / macOS / WSL）可运行且不触发任何编译命令的执行（只输出命令文本，不代替执行）。

映射表 SHALL 按目录结构自动发现 domain 档位，无需硬编码白名单维护：

| 改动路径 | 输出 |
|---|---|
| `backend-go/internal/<domain>/**`（domain = internal/ 下除 app/models/platform 外的目录，运行时发现） | `go test -short ./internal/<domain>/...`（递归；DB 集成测试在 -short 下自动 skip，无需 Docker） |
| `backend-go/internal/platform/<pkg>/**` | `go test ./internal/platform/<pkg>` + 升级提示（`go vet ./...`） |
| `backend-go/internal/{app,models}/**` 或 `backend-go/cmd/**` | `go build ./...` + `go vet ./...`（不自动 test） |
| `backend-go/go.mod` 或 `go.sum` | 提示全量 `go test ./...` |
| `front/**`（非 .md） | `pnpm lint`；typecheck/test:unit/build 按平台标注执行前置（Windows 经 cmd.exe，Linux/macOS 本机直跑） |
| `docs/**`、`*.md`、`scripts/**`、`.pi/**` | 无测试命令，提示文档/规范一致性检查 |

#### Scenario: domain 包改动

- **WHEN** 工作区改动含 `backend-go/internal/daily_report/service/foo.go`
- **THEN** 脚本输出含 `go test ./internal/daily_report`

#### Scenario: 新增 domain 零维护

- **WHEN** `backend-go/internal/` 下新增目录 `newdomain` 且改动含 `backend-go/internal/newdomain/x.go`
- **THEN** 脚本无需修改即输出 `go test ./internal/newdomain`

#### Scenario: 骨架变更不自动 test

- **WHEN** 改动仅含 `backend-go/internal/app/router.go`
- **THEN** 脚本输出 `go build ./...` 与 `go vet ./...`，且不输出任何 `go test` 命令

#### Scenario: 前端命令标注跨平台限制

- **WHEN** 改动含 `front/app/components/Foo.vue`
- **THEN** 脚本输出的 `pnpm lint` 可直接在 POSIX bash 执行，typecheck/test:unit/build 命令附带按平台区分的执行前置标注（Windows 需 cmd.exe；Linux/macOS 本机直跑），不暗示这些命令在所有平台都受限

#### Scenario: 未命中路径不猜测

- **WHEN** 改动含映射表未覆盖的路径（如仓库根新建 `tool.exe`）
- **THEN** 脚本输出「无法判定，请手动选择验证命令」，不输出任何猜测性测试命令

### Requirement: 机器可读输出（--json）

`scripts/change-scope.sh --json` SHALL 输出合法 JSON（单行或多行均可），包含字段：

- `base`：实际使用的 base ref
- `paths`：改动文件路径数组
- `testTargets`：判定出的可执行测试目标数组（每项含 `cmd` 命令文本与 `tier` 档位：`domain` / `platform` / `skeleton` / `frontend`）
- `notices`：提示文本数组（升级提示、平台执行前置标注、无法判定警告）

#### Scenario: quality-gate 消费

- **WHEN** 以 `--json` 运行且改动含 `backend-go/internal/topicgraph/`
- **THEN** 输出可被 `JSON.parse` 解析，且 `testTargets` 含 `{"cmd":"go test ./internal/topicgraph","tier":"domain"}`

#### Scenario: 平台标注随宿主变化

- **WHEN** 在同一改动集上于 Linux 宿主与 Windows+WSL 宿主分别以 `--json` 运行
- **THEN** `testTargets` 的 `cmd` 与 `tier` 保持一致，仅 `notices` 中前端类型检查类命令的执行前置标注按平台不同

### Requirement: quality-gate turn_end 跑影响包 go test

`.pi/extensions/quality-gate.ts` 的 turn_end 门禁 SHALL 按「本回合新增变化」增量路由两侧门禁，并在本轮改动命中后端时调用 change-scope（`--json`）判定影响包，对 `tier=domain` 的目标自动执行对应 `go test`。

**增量路由**：门禁 SHALL 维护会话内改动快照——会话边界（session_start）以当时 git 状态（tracked diff + untracked）初始化，每回合 turn_end 门禁判定后更新为当前 git 状态。本回合触发集 = 当前 git 改动中相对快照新增或内容变化的路径。后端门禁（golangci-lint / go vet / go build / domain go test）仅当触发集命中后端路径（`backend-go/**.go` 等）时执行；前端门禁（仅 `pnpm lint`）仅当触发集命中前端路径（`front/` 非 .md）时执行。会话开始前已存在的工作区残留改动 SHALL NOT 触发任何门禁（非本会话 agent 所为，全量验证由归档门禁兜底）；被跳过的一侧 MUST 不产生 gate.check 事件（与「未运行不记账」一致）。

**执行约束**（既有语义保留）：

- 每个 go test 目标有单条超时保护（约 120s），总预算保护（约 5min）；超时输出 warn 不计失败。
- go test 统一带 `-short` 标志：DB 集成测试自动 skip（无 Docker 不挂、有 Docker 不耗时），完整集成测试由 agent 手动或归档门禁执行。
- `tier=platform` 在门禁中 SHALL 只执行 `go vet`；`tier=skeleton` 只执行 `go build` + `go vet`。
- 测试失败与 lint 失败同机制处理（steer 消息回喂，agent 必须修复或显式说明豁免理由）。
- 门禁 SHALL 只执行与平台无关或平台已适配的命令集；前端门禁维持仅 `pnpm lint`——这是门禁分层设计（typecheck/build/test:unit 与完整集成测试留给 agent 手动执行与归档门禁兜底），与执行平台无关，MUST NOT 因平台能力变化而扩大门禁范围。

#### Scenario: domain 改动触发自动测试

- **WHEN** 本轮编辑修改了 `backend-go/internal/reader/handler/list.go` 且 turn_end 门禁运行
- **THEN** 门禁执行 `go test ./internal/reader`，失败时以 steer 消息回喂失败输出

#### Scenario: DB 依赖包跳过

- **WHEN** 改动命中 `DB_DEPENDENT_PKGS` 清单内的包
- **THEN** 门禁跳过该包 go test，输出「跳过（需 Docker DB）：<pkg>」

#### Scenario: 前端改动行为不变

- **WHEN** 本轮有前端新变化（修改 `front/` 下非 .md 文件）
- **THEN** 门禁仅执行 `pnpm lint`，不执行 typecheck/build/test:unit 类命令

#### Scenario: 残留前端脏文件不触发前端门禁

- **WHEN** 会话开始前工作区已存在前端脏文件（残留 diff/untracked），本回合 agent 仅编辑后端 `.go` 文件
- **THEN** 门禁执行后端全套但不执行 `pnpm lint`，且无 pnpm lint 的 gate.check 事件

#### Scenario: 会话前残留改动不触发首轮门禁

- **WHEN** 会话启动时工作区含上个会话遗留的后端+前端脏文件，本回合 agent 未编辑任何代码（纯对话）
- **THEN** 触发集为空，不执行任何门禁命令，本回合零 gate.check 事件

#### Scenario: 修复循环每轮按新变化触发对应侧

- **WHEN** 前一回合后端编译失败（steer 回喂），本回合 agent 修复该 `.go` 文件
- **THEN** 该文件相对快照有新变化，后端门禁重新执行；若本回合未触碰前端，前端门禁继续跳过
