
## quality-gate.ts 的 Windows/WSL 硬编码点（Linux 迁移改造清单）

文件：`.pi/extensions/quality-gate.ts`（436 行）。Linux（树莓派）上每轮门禁全瘫痪的根因与精确改造点：

**根因链**：`pi.on("turn_end")` 步骤 3.5（约 L215-250）无条件 `pi.exec("cmd.exe", ["/C","echo ok"], {timeout: 2000})`；Linux 无 cmd.exe → 抛异常 → `interopDown = true` → **整轮门禁短路**（只记一条 policy.decision(fail-open/interop-down) + 发 steer「WSL interop 环境故障…wsl --shutdown」），零 gate.check。即：改后端/前端代码在树莓派上没有自动门禁。

**改造点逐条**：
1. L215-250 interop 探测：需分流为「cmd.exe 不存在（ENOENT）→ native 模式」vs「存在但探测失败 → interopDown 短路（保留原语义）」。探测本身即可承担平台判定。
2. L320-326 `runWin()`：`pi.exec("cmd.exe", ["/C", `cd /d ${backendWin} && ${cmdline}`], {timeout:120_000})` → native 用 `pi.exec(sh 命令, {cwd})`。
3. L304-313 硬编码 Windows 绝对路径：`const goExe = "D:\\tool\\Go\\bin\\go.exe"`、`const linterExe = "C:\\Users\\Admin\\go\\bin\\golangci-lint.exe"`；native 直接 PATH 查 `go`/`golangci-lint`（树莓派已装 go1.27.1 + golangci-lint 2.13.2）。
4. L379-396 前端门禁：`cmd.exe /C cd /d ${winPath(repoRoot)}/front && pnpm exec eslint .${cacheArgs}` → native 用 `cwd=front` + `pnpm exec eslint .`（保留 `--cache --cache-location node_modules/.cache/eslint/.eslintcache`，`eslintCacheOff` 兜底不变）。
5. L429-435 `winPath()`：/mnt/d → D:/ 转换，仅 Windows 分支用。
6. steer 文案：gateLog 内失败前缀 `[${cmd} (wsl环境)]`、L245 短路文案、L408-416 envFailures 文案（含「wsl --shutdown 重启」）需按模式分文案；native 失败不标 wsl环境。
7. 依赖：`lib/failure-classify.ts` 的 `isInteropFailure`（UtilAcceptVsock 特征）仅 Windows 分支有意义，native 下不应命中；`lib/gate-sample.ts`、`lib/trigger-set.ts`、`scripts/change-scope.sh` 与平台无关，不动。
8. 头部注释（L1-32）第 3/12 条描述 WSL 前提，需更新。

**保持不变的契约**：增量路由（会话快照 diff）、失败粘性 stickyFailures、成功采样记账（lib/gate-sample）、lint 先行短路哨兵（typechecking error）、domain 测试 5min 预算、edit.map 记账、软 steer 不硬阻断。

**引用**：.pi/extensions/quality-gate.ts:runWin、.pi/extensions/quality-gate.ts:winPath、.pi/extensions/quality-gate.ts:turn_end、.pi/extensions/lib/failure-classify.ts:isInteropFailure

<!-- pinned 2026-09-16T15:48:19Z -->

## testcontainers 镜像链路 + ryuk 缺失根因（已验证修复）

**唯一集成测试 DB 入口**：`backend-go/internal/platform/testutil/testutil.go`
- `pgImage = "pgvector/pgvector:pg18-trixie"`（与 `docker-compose.pg.yml` 同镜像）
- 用 `tcpostgres.Run(...)` 起容器，cleanup **完全委托 Ryuk sidecar**（代码显式不调 TerminateContainer）
- 依赖版本：`github.com/testcontainers/testcontainers-go v0.42.0`（modules/postgres 同版本）

**缺失镜像**：`testcontainers/ryuk:0.13.0` —— 定义在 go mod cache `github.com/testcontainers/testcontainers-go@v0.42.0/internal/config/config.go:14` `const ReaperDefaultImage = "testcontainers/ryuk:0.13.0"`。pgvector 镜像本地已有（673MB），ryuk 从未拉下来过。

**泄漏机制**：testcontainers 先创建 pg 容器、再启动 ryuk；ryuk 镜像拉取失败 → Run 报错返回，但已创建的 pg 容器无人回收 → 实测堆积 36 个（26 running + 10 created）随机名 pgvector 容器。清理时须保留 `syntopica-postgres`（docker compose 的生产库容器，Up 4 hours healthy）。

**已验证的修复路径**（2026-09-16 实测）：
1. `docker pull docker.1ms.run/testcontainers/ryuk:0.13.0` → 几秒拉到（13.8MB，arm64 有）
2. `docker tag docker.1ms.run/testcontainers/ryuk:0.13.0 testcontainers/ryuk:0.13.0`
3. `go test ./internal/reader/service -run TestArticleContentFormColumnInPGSchema -v -count=1` → **5.57s PASS**，日志可见 ryuk 容器启动 + pg 容器就绪；容器数 38→36（ryuk 正确回收）

**网络事实**：`registry-1.docker.io` 直连不通；`docker.1ms.run`(401 即通) / `docker.m.daocloud.io`(401) / `dockerproxy.net`(200) / `hub.rat.dev`(302) / `docker.1panel.live`(200) / `docker.xuanyuan.me`(401) 均可达。树莓派上无任何 HTTP 代理进程在跑（无 clash/v2ray），`/etc/docker/daemon.json` 不存在，`~/.docker/config.json` 不存在，`~/.testcontainers.properties` 不存在。Go 侧 `GOPROXY=https://goproxy.cn,direct`。

**引用**：backend-go/internal/platform/testutil/testutil.go:pgImage、backend-go/internal/platform/testutil/testutil.go:startContainerAndConnect、docker-compose.pg.yml

<!-- pinned 2026-09-16T15:48:19Z -->

## 树莓派工具链现状 + 活文档 WSL 命中清单

**树莓派（4 核 arm64 / 7.9GB RAM / Debian 13 trixie / WLAN 192.168.5.24 / ZeroTier 10.11.12.55）工具链实测**：
- Go `go1.27.1 linux/arm64`（/usr/local/go/bin/go）—— **不是**文档里写的 WSL Go 1.18
- golangci-lint `2.13.2`（Linux 原生，非 `C:\Users\Admin\go\bin\golangci-lint.exe`）
- node `v26.8.2` / pnpm `12.4.2`（nvm 安装）
- `front/node_modules` 已重装为 **linux-arm64 平台包**：`@rollup/rollup-linux-arm64-gnu@4.55.1`、`@oxc-parser/binding-linux-arm64-gnu@0.102.0`、`@esbuild+linux-arm64@0.25.12/0.27.2`；win32 平台包残留 **0 个**
- Docker `29.8.0 arm64`（overlayfs，无 daemon.json）；Docker Compose v5.5.1
- 存在：lsof / curl / setsid / nginx / python3。**缺失：uv**（AGENTS.md 要求 Python 用 uv，tests/workflow、tests/firecrawl 用得上）
- 磁盘 `/` 117G 用 21G（19%）

**结论**：文档中「必须走 Windows cmd，否则 WSL 缺 Linux native binding」的前置条件在树莓派上**已完全消失** —— front 的 typecheck/build/test:unit 可直接在 Linux 跑；后端门禁可直接用本机 Go 工具链。

**活文档 WSL 命中清单**（排除 docs/archive、docs/v1.x、openspec/changes/archive 等历史归档）：
- `AGENTS.md`：L31（5000 端口/WSL 够不着）、L41（OS 行写 Windows + WSL2 路径 D:/project）、L115-121（前端必须走 cmd 段）、L134（quality-gate 表格行 cmd.exe interop）、L146（pi 增量门禁整段 cmd.exe/WSL 描述）
- `front/AGENTS.md`：L15（WSL 注意段）
- `docs/reference/standard/frontend/testing.md`：L53-67「跨平台运行（WSL 必须切 Windows cmd）」整节
- `docs/reference/standard/frontend/lint.md`：L4（Lint 可在 WSL 跑，typecheck/build 走 cmd）
- `docs/reference/standard/shared/commit-pr.md`：L11、L35-37「WSL / Windows cmd 注意」节
- `docs/reference/testing.md`：L32（WSL 侧探测加 --noproxy）
- `docs/reference/开发执行规范.md`：L211（typecheck/build/test:unit 必须走 cmd）、L227（opencli 用 WSL2 localhost 镜像直达）
- `docs/reference/development.md`：L163（WSL 侧探测 no_proxy）
- `docs/reference/architecture/overview.md`：L213（Windows/WSL 均可达）
- `docs/reference/deployment.md`：L188（引用 fix-wsl-dev-networking change 的历史说明，措辞可轻改）
- `scripts/start-dev.sh`（头注释「需要：Linux / WSL bash」+ 树莓派前端冷启动 60~90s 注释）、`scripts/{change-scope,doc-impact,scenario-trace,check-standards,test-assets,concurrency-status}.sh`（头注释「WSL bash 可跑」「DrvFS」「quotepath」等措辞）
- `backend-go/configs/config.yaml`、`backend-go/internal/platform/database/{db.go,slow_logger.go}`、`backend-go/internal/tagmanagement/{handler/tag_merge_preview_handler.go,service/core/tagger.go}`、`backend-go/internal/platform/testutil/testutil.go`（注释性提及）
- `front/nuxt.config.ts`、`front/app/utils/api.ts`、`front/app/features/tags/composables/useBoardEnrichment.ts`（注释性提及）
- `docs/experience/wsl-node-error.md`（历史经验文档，属事实记录，不应改写）

<!-- pinned 2026-09-16T15:48:24Z -->

## harness 账本现状与失败聚类（可当准 benchmark）

.pi/harness/events.db 实况（2026-09-16 采样）：单表 events(id,ts,session_id,kind,change,payload)，共 26717 条。kind 分布：gate.check 20521 / constraint.inject 3462 / pin.read 950 / edit.map 452 / session.start 436 / mode.set 266 / subagent.dispatch 225 / spill.write 125 / pin.write 105 / policy.decision 103 / subagent.complete 72。

payload 结构（json_extract 可查）：
- gate.check: {cmd, phase, ok, declaration, kwHits, ms, diag, flip, sampled, n}。ok=0 共 1772 条（ok=1 共 18751）。失败热点 cmd：golangci-lint 565、go vet 371、go test -short ./internal/dataenrichment/... 307、go test -short ./internal/admin/... 161、go build 145、topicgraph 95、tagmanagement 82、pnpm lint 21、entry-gate 20。diag 大量为 typechecking error / [build failed] 级联。
- flip=1（上回合绿→本回合红，回归标记）共 651 条 —— 高频真实信号。
- edit.map: {paths:[...], n}。**只有路径数组，无 per-file 累计计数**；但可用 json_each(payload,'$.paths') 按 session_id/change 展开做离线聚类，零改代码即可实现 loop 检测。
- policy.decision: {policy, action, reasonCode, durationMs}。分布：quality-gate/interop-down/fail-open 42、test-scope-guard/full-go-test/warn 41、ui-design-gate/ui-impact-mismatch/block 7、spec-gate/concurrent-dirty-tree/warn 6、spec-gate/archive-check-failed/block 5、ui-design-gate 各 1。

结论：账本数据充足但当前只有 harness-facts skill 做人工考古（事后归因），**没有"失败聚类 → 产出 harness 改进项"的消费端**；改一条 harness 规则后同类事件计数变化可直接当 A/B 指标（准 benchmark）。

**引用**：.pi/harness/events.db、.pi/extensions/quality-gate.ts、.pi/extensions/lib/edit-map.ts、.agents/skills/harness-facts/SKILL.md

<!-- pinned 2026-09-16T16:18:12Z -->
