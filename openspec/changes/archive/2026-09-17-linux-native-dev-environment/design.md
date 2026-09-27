## Context

见 `proposal.md` — Why。影响本设计的技术现状：

- `.pi/extensions/quality-gate.ts` 是 turn_end 门禁的唯一实现，当前把「门禁可执行」与「cmd.exe interop 可用」绑成同一件事：探测 `cmd.exe` 失败即判定环境故障并短路整轮。该假设在 Linux 宿主上恒成立地失败。
- 门禁的既有契约（增量路由快照、失败粘性、成功采样记账、lint 同根因短路、domain 测试 5min 预算、edit.map 归属）与执行平台正交，`lib/gate-sample.ts`、`lib/trigger-set.ts`、`scripts/change-scope.sh` 均为平台中立实现，不需要改。
- testcontainers 的容器回收完全委托 Ryuk sidecar（`testutil.go` 显式不调 `TerminateContainer`），因此 Ryuk 镜像不可达 = 回收链断裂，而非单次失败。
- 开发主机（树莓派）无任何 HTTP 代理进程；Docker Hub 直连不通，但多个国内加速站可达（实测 `docker.1ms.run` 数秒拉到 Ryuk）。

## Goals / Non-Goals

**Goals:**

- 门禁在 Linux/macOS 宿主上**真实执行**（而非每轮短路），且在 Windows 宿主上行为与改造前逐字一致。
- 门禁失败提示的链路标注与恢复建议**永远与实际执行方式相符**，不出现误导性的跨系统措辞。
- 镜像可达性以「机器级配置」解决，仓库文件零改动；集成测试的镜像依赖与泄漏诊断路径落文档。

**Non-Goals:**

- **不扩大门禁范围**：native 模式下前端仍只跑 `pnpm lint`（typecheck/build/test:unit 与完整集成测试继续由 agent 手动执行 + 归档门禁兜底）。「Linux 上跑得动 typecgeck」不等于「门禁该跑它」——门禁分层是设计决定，不是平台能力推论。
- **不重构门禁架构**：探测、路由、采样、粘性、记账等既有机制保持原结构，只增加平台分流层。
- **不改历史归档文档**（`docs/archive`、`docs/v1.x`、`openspec/changes/archive`）——它们是当时事实的存档，改写会破坏溯源真实性。
- **不改跨平台兼容逻辑**：`ai-model-health` 的 `cmd /c` vs `sh -c`、`deployment-init` 的 Git Bash 分支、`tool-output-spill` 的 POSIX mode best-effort 在 Linux 上依然正确。
- 不引入私有 registry、镜像重打 tag、镜像名前缀改写等需要仓库配合的方案。

## Decisions

### D1. 平台判定复用现有探测，按失败性质分流（而非新增平台探测）

**决策**：把现有 `cmd.exe /C echo ok` 探测的失败分为两类——**调用因可执行文件不存在而失败（spawn ENOENT / 无 cmd.exe）→ native 模式**；**调用成功返回但 exit≠0 或超时/其他异常 → 故障短路**（保留既有语义）。

**理由**：探测量与语义都不增加（本来就要探），只把「失败」细化为「不存在」与「存在但坏掉」。用 `process.platform === "linux"` 判断会把「Linux 上装了 cmd.exe（如 wine）」「Windows 上 PATH 里没有 cmd.exe」这类边界判错，而现状探测天然反映真实可执行性。

**备选**：① 只用 `process.platform` 判断——边界判错，且无法解释「Windows 但 cmd 不可用」时应短路而非强行 native；② 新增独立的 `which cmd.exe` 探测——多一次进程调用且与健康探测信息重复。

### D2. native 模式用「可执行文件 + 参数 + cwd」，不复用 `cd /d` 拼接

**决策**：native 模式下命令经 `pi.exec(executable, args, { cwd })` 执行（后端 cwd = `backend-go`，前端 cwd = `front`），可执行文件由 PATH 解析（`go` / `golangci-lint` / `pnpm`）；Windows 分支保留原有 `cmd.exe /C "cd /d <winPath> && <cmdline>"` 与 Windows 绝对路径形式。

**理由**：`cd /d` 与 `winPath()` 是 cmd.exe 专属概念，native 下 cwd 参数由进程 API 直接承担，不依赖 shell 内建，也不受路径空格/引号吞噬影响。`change-scope.sh` 输出的 `go test -short ./internal/<domain>/...` 是相对路径命令，用 cwd 定位 backend-go 即可直接复用，无需解析改写。

**备选**：① native 下也拼 `sh -c "cd x && cmd"`——多一层 shell 且丢失退出码语义细节；② 把 change-scope 输出改成绝对路径——污染平台中立的脚本契约。

### D3. 平台判定在会话内稳定，不做逐回合重判

**决策**：探测结果（native / windows）在会话内缓存并在 `session_start` 重置；仅「故障短路」判定逐回合执行（它与宿主环境健康相关，必须每轮重判）。若已判定 native，该会话不再探测 cmd.exe。

**理由**：spec 明确要求「平台判定 MUST 在会话内稳定，MUST NOT 因单回合探测抖动在模式下反复切换」。平台身份在一台机器上不会逐回合变化，而 interop 健康会（vsock 故障期），两者生命周期本就不同。

**备选**：每回合重判平台——探测抖动会导致 native/windows 来回切换、命令构造方式不稳定、记账口径也随之中断。

### D4. 镜像可达性走 daemon.json `registry-mirrors`，仓库零改动

**决策**：在开发主机写 `/etc/docker/daemon.json` 的 `registry-mirrors` 指向已实测可达的加速站（多个，按序回退）；补拉 `testcontainers/ryuk:0.13.0`；清理泄漏容器时只删随机名测试容器，`syntopica-postgres` 与 `./data` 数据目录不动。

**理由**：`registry-mirrors` 对 `docker pull` / compose / testcontainers（按默认镜像名创建容器）同时生效，且拉下来的镜像 tag 就是原始名——仓库里的 `pgvector/pgvector:pg18-trixie`、testcontainers 的 `testcontainers/ryuk:0.13.0` 全都不需要改。相比「在仓库里写加速站前缀」或「自建 registry 映射」，零仓库耦合、零代码改动。

**备选**：① 仓库内改写镜像名加前缀——污染仓库且 CI/他人机器失效；② 用本机 HTTP 代理（`proxies` 配置）——树莓派上无代理进程，且整机流量绕代理对只缺镜像可达性的场景过重；③ 直接把加速站镜像 tag 成原名（`docker tag`）——能救急但不解决「下次拉新镜像还会失败」，不入文档主线。

### D5. 文档去化按「活文档 vs 历史归档」切分

**决策**：改 `AGENTS.md`、`front/AGENTS.md`、`docs/reference/**`、`scripts/*.sh` 头注释与 spec 中的平台前提表述；不改 `docs/archive/**`、`docs/v1.x/**`、`openspec/changes/archive/**`、`docs/experience/wsl-node-error.md`。

**理由**：活文档是「当前该怎么做」的权威源，写错会误导后续所有会话（本次问题的直接来源）；历史归档是「当时发生了什么」的事实记录，改写它等于伪造历史，且会破坏变更溯源。

**备选**：全量替换（含归档）——grep 更「干净」，但牺牲溯源真实性，且归档文档里的 WSL 描述在当时是正确前提。

### D6. 表述采用「平台差异」而非「去 Windows 化」

**决策**：活文档中的绝对化表述（「必须通过 Windows cmd 执行」）改写为条件化表述（「Windows 上经 cmd.exe；Linux/macOS 本机直跑」），而非删除 Windows 相关内容。

**理由**：仓库的 `deploy/`、`ai-model-health` 等仍含 Windows 支持路径，且 AGENTS.md 本身仍是跨平台协作文档。删除 Windows 分支会让回 Windows 开发的会话失去指引；条件化表述同时服务两类宿主。

### D7. 门禁的 lint 调用加 `--allow-parallel-runners`

**决策**：`golangci-lint run` 统一加 `--allow-parallel-runners`（两个模式都加）。

**理由**：实测踩中 `parallel golangci-lint is running` exit 3 —— golangci-lint 默认对同机实例加文件锁，而本仓库约定「并发 change 共享工作树」，多会话/手动跑 lint 与会话门禁重叠时后到者必然报错。该报错是环境冲突而非代码回归，被现有分级机制归为 `[回归]`，会诱导 agent 去修不存在的代码问题（正是本次改造要消除的那类假信号）。

**备选**：① `--allow-serial-runners`（串行等待锁）—— 不报错但会让门禁阻塞在别人的 lint 上（树莓派 4 核上可能等到 120s 超时）；② 把锁冲突字符串加进 `isInteropFailure` 归为环境故障 —— 治的是症状（下次换一种冲突字符串又漏），不如消除冲突源；③ 不动 —— 假 `[回归]` 会持续干扰。

## Risks / Trade-offs

- **[镜像加速站失效]** → 配置中列多个来源按序回退；文档记录「改用本机 HTTP 代理」的备选路径与症状识别；加速站不可达不影响已拉取的本地镜像。
- **[native 模式命令构造在 macOS 上未实测]** → 设计上用 POSIX 通用形态（PATH 查找 + cwd 参数），不引入 Linux 专有命令；文档注明实测环境为 Linux arm64。
- **[Windows 分支无回归测试环境]** → Windows 分支代码路径不改逻辑（只被分流条件包裹），并以现有 smoke 测试（`.pi/extensions/tests/quality-gate.behavior.smoke.cjs`）覆盖「cmd.exe 存在时的行为不变」；同时新增 native 分支的 smoke 覆盖。
- **[平台判定缓存导致「会话中途装了 cmd.exe」不生效]** → 影响可忽略（装机后开新会话即生效），换来判定稳定性；`session_start` 重置已足够。
- **[清理泄漏容器误伤生产库]** → 清理前先列出待删容器清单核对（按「非 `syntopica-postgres` 名」过滤），且只 `docker rm -f` 容器、不碰 volume 与 `./data`。
- **[去化后措辞过度泛化、丢失有价值的根因信息]** → 保留根因（`svchost` 抢占 5000、v6-only 监听）但标注为平台特有，不简单删除；`docs/experience/wsl-node-error.md` 作为历史根因存档不动。

## Migration Plan

1. **环境先行**（可独立回滚）：写 `daemon.json` 加速来源 → 重启 docker（会短暂中断 compose 容器，`restart: unless-stopped` 会自动拉起）→ `docker pull` 拉取原始镜像名验证 → 补拉 Ryuk → 清理泄漏容器。
2. **门禁改造**：改 `quality-gate.ts` 平台分流 + 双模式 runner + 文案分流（扩展源码已入库，回滚 = `git checkout` 还原）。
3. **文档/spec 去化**：按 D5 边界改活文档与 delta specs；`dev-api-networking` 主 spec 的 Purpose 直接就地修订。
4. **验证**：跑现有 smoke 测试（Windows 分支不回归）+ native 分支用例；真实改动后端文件触发一轮门禁，确认 `gate.check` 记账出现且无「环境故障」字样；跑一个 DB 集成测试确认容器回收。
5. **回滚**：环境配置删除 `daemon.json` 重启 docker 即回到原状；harness 文件还原；文档改动可整体 revert（无数据/状态迁移）。

## Open Questions

- 是否把「补拉镜像」步骤写进 `scripts/start-dev.sh` 或独立脚本？本 change 先只落文档（脚本化需要处理「加速站与代理两条路径」的分支，超出当前范围）——后续若换机动作变频繁再单独开 change。
