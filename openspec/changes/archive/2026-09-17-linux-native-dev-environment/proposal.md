<!-- complexity: complex -->
<!-- ui-impact: none -->

## Why

开发主机已从 Windows + WSL2（`D:\project\Syntopica`）迁移到 Linux 树莓派（arm64 / Debian 13 / `~/software/Syntopica`），但仓库仍按「WSL 调 Windows 工具链」写死：工具链前提已反转（树莓派有 Go 1.27.1、golangci-lint 2.13.2、node 26 / pnpm 12 且 `front/node_modules` 已是 linux-arm64 平台包，WSL 那套「Linux 侧缺 native binding」的理由不复存在），而 `.pi/extensions/quality-gate.ts` 仍无条件探测 `cmd.exe` → Linux 上每轮 `turn_end` 门禁**整轮短路**，改后端/前端代码现在没有任何自动门禁兜底。同时 Docker Hub 直连不通导致 testcontainers 缺 `testcontainers/ryuk:0.13.0`，Ryuk 无法启动 → 每个集成测试泄漏一个 pgvector 容器（已堆积 36 个），而 `testutil.go` 的容器回收完全委托 Ryuk，等于集成测试跑一次脏一次。

## What Changes

- **BREAKING（对 Windows 开发机）**：无。门禁改造为**平台自适应**，Windows + cmd.exe 分支行为原样保留；仅当探测到 `cmd.exe` 不存在（ENOENT）时切「native 模式」（本机 `go` / `golangci-lint` / `pnpm` + `cwd` 定位，不再拼 `cd /d` 与 Windows 绝对路径）。
- **门禁平台分流**：`quality-gate` 的 interop 探测语义扩展为三态——`cmd.exe` 不存在 → native 模式正常执行门禁；存在但探测失败 → 维持既有短路语义（fail-open + `policy.decision(interop-down)`）；健康 → Windows 模式。native 模式下失败 steer 不再标 `（wsl环境）`，环境故障文案不再提示「重启 WSL」。
- **Docker 镜像可达性**：`/etc/docker/daemon.json` 配置 `registry-mirrors`（已实测可达的国内加速站），使 `docker pull` / compose / testcontainers 的**原始镜像名**照常工作，无需改任何代码或文档；补拉 `testcontainers/ryuk:0.13.0`；清理 ryuk 缺失期间泄漏的 36 个 pgvector 容器（保留 `syntopica-postgres`）。
- **镜像依赖落文档**：把「集成测试所需镜像清单（`pgvector/pgvector:pg18-trixie` + `testcontainers/ryuk:0.13.0`）与其来源」写进 reference，避免换机/清缓存后再踩同一个坑。
- **活文档平台中立化**：`AGENTS.md`、`front/AGENTS.md`、`docs/reference/**` 中「必须通过 Windows cmd 执行」类表述改写为平台中立（说明 Windows 走 cmd.exe、Linux 原生直跑），历史归档（`docs/archive`、`docs/v1.x`、`openspec/changes/archive`）不改写以保留溯源真实性。
- **spec 措辞平台化**：`change-scope-gate` / `scenario-trace-gate` 的「SHALL 在 WSL bash 环境可运行」（改为 bash 环境）、`dev-api-networking` 的「WSL 侧探测卫生约定」与「WSL2 mirrored 够不着」根因描述（改为平台中立表述，保留 `0.0.0.0` 绑定契约本身）。

## Capabilities

### New Capabilities

- `dev-image-reachability`: 开发/测试期 Docker 镜像的可达性契约——镜像加速来源配置、集成测试必需的镜像清单（testcontainers postgres + Ryuk）、因镜像缺失导致的容器泄漏识别与清理。

### Modified Capabilities

- `gate-interop-health`: interop 探测从「单一健康判定」扩展为「平台分流 + 健康判定」——新增 native 模式（无 `cmd.exe`）下门禁走本机工具链正常执行的 requirement，并规定 native 模式下失败 steer 不得标注 wsl 链路、环境故障文案不得提示 WSL 恢复动作；既有 Windows 模式探测短路/故障识别/粘性豁免语义不变。
- `change-scope-gate`: 「脚本 SHALL 在 WSL bash 可运行」→ 平台中立的 bash 环境；前端档位的「需 cmd.exe」标注改为「按平台决定是否需 cmd.exe」（native 下前端 typecheck/build/test:unit 不再受阻）。
- `scenario-trace-gate`: 同上，静态判定脚本的运行环境表述去 WSL 化。
- `dev-api-networking`: 根因与探测卫生约定表述去 WSL 化（`0.0.0.0` 绑定、CORS 白名单、`no_proxy` 探测卫生契约保留；把「WSL 侧」改为「本机/局域网侧」，把 v6-only 根因标注为 Windows 特有）。

## Impact

- **代码（harness，扩展源码已入库）**：`.pi/extensions/quality-gate.ts`——探测三态分流、`runWin` → 双模式 runner、硬编码 `D:\tool\Go\bin\go.exe` / `C:\Users\Admin\go\bin\golangci-lint.exe` 改 native PATH 查找、`winPath()` 仅 Windows 分支、steer 文案分流、头部注释更新。依赖的 `lib/failure-classify.ts`（interop 特征识别）仅在 Windows 分支生效，不改逻辑。
- **环境配置（机器级，不入库）**：`/etc/docker/daemon.json`（新建）。
- **文档**：`AGENTS.md`、`front/AGENTS.md`、`docs/reference/{standard/frontend/testing.md, standard/frontend/lint.md, standard/shared/commit-pr.md, testing.md, 开发执行规范.md, development.md, architecture/overview.md, deployment.md}`、`docs/reference/constraints-index.md`（若引用门禁分层则同步）、`docs/research/harness事实库.md`（门禁事件描述若含 WSL 前提）。
- **代码注释（措辞，无逻辑变更）**：`front/nuxt.config.ts`、`backend-go/configs/config.yaml`、以及前端一处工具注释。
- **spec**：4 个 delta（`gate-interop-health` / `change-scope-gate` / `scenario-trace-gate` / `dev-api-networking`）+ 1 个新 capability。
- **无**：后端业务逻辑、数据库 schema、API、界面结构均不变；不改 `deploy/`、不改历史归档文档。
- **已知不动项**：`docs/experience/wsl-node-error.md`（历史故障记录，属事实存档）；`ai-model-health` / `deployment-init` / `tool-output-spill` 里的 Windows 分支（跨平台兼容逻辑，Linux 上依然正确）。
