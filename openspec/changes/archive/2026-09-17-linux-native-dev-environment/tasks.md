# Tasks

## 1. 迁移环境与仓库卫生（dev-image-reachability spec：加速来源/清单/泄漏）

- [x] 1.1 新建 `/etc/docker/daemon.json`，写入 `registry-mirrors` 数组（含实测可达的多个来源，按序回退）；验证：`docker info | grep -A5 'Registry Mirrors'` 列出配置项，且 `docker compose -f docker-compose.pg.yml config` 仍可解析
- [x] 1.2 重启 docker daemon 后按**原始镜像名**验证不受仓库改动影响：`docker pull pgvector/pgvector:pg18-trixie` 与 `docker pull testcontainers/ryuk:0.13.0`（均不带加速站前缀）成功；验证：`docker images` 中出现无前缀的两个 tag，`git status` 确认仓库零改动
- [x] 1.3 清理 Ryuk 缺失期间泄漏的测试容器：先 `docker ps -a --format '{{.Names}} {{.Image}}'` 列出并按「非 `syntopica-postgres`」过滤核对清单，再 `docker rm -f` 批量删除；验证：清理后 `docker ps -aq | wc -l` 等于 1（仅 compose 库容器），`docker compose -f docker-compose.pg.yml ps` 显示 `syntopica-postgres` 仍 healthy，`./data` 目录未被触碰
- [x] 1.4 集成测试闭环验证容器回收：`cd backend-go && go test ./internal/reader/service -run TestArticleContentFormColumnInPGSchema -count=1`；验证：测试 PASS，且运行前后 `docker ps -aq | wc -l` 数量一致（Ryuk 正确回收）、`docker ps -a --filter name=ryuk` 在测试期可见 sidecar 容器
- [x] 1.5 补齐仓库脚本执行位：`find . -name '*.sh' -not -path './node_modules/*' -not -path './front/node_modules/*' -not -path './.git/*' -not -path './data/*' -print0 | xargs -0 chmod +x`（git 记录为 `100644`；WSL/DrvFS 忽略 POSIX 权限故此前"能跑"，真 Linux 上 `Permission denied`——`scripts/scenario-trace.smoke.sh` 直接调脚本即踩中）；验证：`find . -name '*.sh' ! -perm -u+x` 零命中、`bash scripts/scenario-trace.smoke.sh` 全过、`git status` 出现脚本 mode 变更

## 2. quality-gate 平台自适应改造（gate-interop-health spec：探测三态/native 执行/标注分流）

- [x] 2.1 探测三态分流：把现有 `cmd.exe /C echo ok` 探测的失败细分为「可执行文件不存在（spawn ENOENT）→ native 模式」与「存在但 exit≠0/超时/异常 → 故障短路」，native 判定在会话内缓存、`session_start` 重置；验证：Linux 上改后端文件触发 turn_end，本回合**不**短路、**不**发环境故障 steer，且 gate.check 事件正常落库（`sqlite3 .pi/harness/events.db` 查询本回合 gate.check）
- [x] 2.2 native 模式命令执行器：后端门禁改经 `pi.exec(可执行文件, 参数, { cwd: <repoRoot>/backend-go })`，可执行文件走 PATH（`go` / `golangci-lint`），移除 native 分支对 `D:\\tool\\Go\\bin\\go.exe`、`C:\\Users\\Admin\\go\\bin\\golangci-lint.exe` 与 `cd /d` 拼接的依赖；验证：Linux 上四条后端门禁（golangci-lint / go vet / go build / domain go test）全部真实执行并按既有标号记账
- [x] 2.3 前端门禁双模式：native 分支 `pi.exec("pnpm", ["exec","eslint",".", ...cacheArgs], { cwd: <repoRoot>/front })`，Windows 分支保留原 cmd.exe 形式；`eslintCacheOff` 兜底逻辑不变；验证：Linux 上改前端非 .md 文件触发 turn_end，`pnpm exec eslint` 真实执行并记 gate.check，且**不**执行 typecheck/build/test:unit（门禁范围未扩大）
- [x] 2.4 steer 文案分流：native 模式失败行不标 `（wsl环境）`、短路/环境故障文案在 native 下不出现、Windows 分支文案逐字保持；验证：扩展 `.pi/extensions/tests/quality-gate.behavior.smoke.cjs` 断言两种模式下的 steer 文本（native 无 wsl 字样、windows 分支文本与改造前一致）
- [x] 2.5 `winPath()` 与 interop 特征识别限定在 Windows 分支生效，`lib/failure-classify.ts` 逻辑不改；验证：native 模式下构造含 `UtilAcceptVsock` 字样的失败输出，断言其仍按真实代码失败归因（进粘性 + 分级 steer），不标环境故障
- [x] 2.6 更新 `quality-gate.ts` 头部注释（第 3、12 条）与内部控制流注释中的 WSL 前提表述；验证：文件内 grep 不再出现「必须走 cmd.exe」「WSL 仅有 Go 1.18」等已失效前提（保留 Windows 分支的说明）
- [x] 2.7 golangci-lint 加 `--allow-parallel-runners`（两个分支同加）：实测踩中 `parallel golangci-lint is running` exit 3 被标 `[回归]`——多会话/手动跑 lint 与门禁共享工作树时默认文件锁冲突，属环境冲突非代码回归；验证：behavior smoke 新增 A5/E6 断言命令含该参数（harness smoke 305 断言全绿）

## 3. 活文档与 spec 去化（平台中立的执行指引）

- [x] 3.1 `AGENTS.md`：OS 行（改为 Linux 树莓派 + 本仓库当前工作路径）、L31 端口说明（标注 5000 冲突为 Windows 特有）、L115-121「必须通过 Windows cmd」段（改为平台条件化表述）、L134/L146 门禁全景描述（改为平台分流）；验证：`grep -n -i 'wsl' AGENTS.md` 剩余命中均为「Windows/WSL 场景下的历史事实或条件化说明」，无「必须走 Windows cmd」类绝对化前提
- [x] 3.2 `front/AGENTS.md` L15「⚠️ WSL 注意」段改为平台条件化；验证：该段说明 Linux/macOS 本机直跑、Windows 经 cmd.exe，且不再暗示 typecheck/build 在 Linux 不可用
- [x] 3.3 `docs/reference/standard/frontend/testing.md`（L53-67「跨平台运行」整节）与 `docs/reference/standard/frontend/lint.md`（L4）；验证：两文件不再声明「Linux 缺 native binding 会失败」，改为「Windows 宿主经 cmd.exe；Linux/macOS 本机直跑（本仓库 node_modules 已按当前平台安装）」
- [x] 3.4 `docs/reference/standard/shared/commit-pr.md`（L11、L35-37 节）；验证：grep 该文件无绝对化 cmd 前提
- [x] 3.5 `docs/reference/testing.md`（L32）、`docs/reference/development.md`（L163）、`docs/reference/开发执行规范.md`（L211、L227）；验证：localhost 探测卫生约定表述为「存在系统代理时需绕过」，不再绑定 WSL
- [x] 3.6 `docs/reference/architecture/overview.md`（L213）、`docs/reference/deployment.md`（L188）；验证：平台表述中立化，历史 change 引用保留
- [x] 3.7 `scripts/*.sh` 头注释（`start-dev.sh`、`change-scope.sh`、`doc-impact.sh`、`scenario-trace.sh`、`check-standards.sh`、`test-assets.sh`、`concurrency-status.sh`）中的「WSL bash 可跑」「DrvFS」「cmd.exe 限制」措辞改为 POSIX bash 中立表述；验证：`grep -rn -i 'wsl\|cmd\.exe' scripts/` 剩余命中均为条件化说明，且脚本行为零变更（`bash scripts/change-scope.sh --json` 输出结构不变）
- [x] 3.8 三处代码注释措辞：`backend-go/configs/config.yaml`（L2 端口理由）、`front/nuxt.config.ts`（L8-9 绑定理由）、`front/app/utils/api.ts`（L18 直连说明）；验证：三处注释不再以某个跨系统运行环境为唯一前提，且 `cd backend-go && go build ./...` 通过、前端该文件无类型层面改动
- [x] 3.9 spec 措辞去 WSL 化：delta specs 已写好，另直接就地修订 `openspec/specs/dev-api-networking/spec.md` 的 Purpose；验证：`openspec validate linux-native-dev-environment` 通过，`grep -rn -i 'wsl' openspec/specs/dev-api-networking/spec.md` 零命中

## 4. 文档

<!-- doc-impact: standard, architecture, deployment, configuration -->
<!-- doc-impact-excuse: flow=启发式命中系其他在途 change 的后端脏文件（topicgraph/service/daily_report_article_filter*.go 等，未追踪新文件），本 change 零业务链路改动; api=启发式命中系其他在途 change 的后端脏文件（reader/handler/feed_handler.go 等），本 change 零 API 改动; database=同上脏文件干扰（platform/database/postgres_migrations.go 含 ALTER TABLE 字样），本 change 零 schema/数据模型改动 -->

- [x] 4.1 `docs/reference/development.md` 新增「Docker 镜像来源与可达性」小节：加速来源配置方式（daemon.json `registry-mirrors`）、备选路径（本机 HTTP 代理 / 直接带前缀拉取后 tag）、以及**不把加速站写进仓库**的约定；验证：该节含配置示例与「仓库零改动」说明，`grep -rn 'daemon.json' docs/reference/development.md` 命中
- [x] 4.2 `docs/reference/testing.md` 补集成测试镜像清单（`pgvector/pgvector:pg18-trixie`、`testcontainers/ryuk:0.13.0` 及来源）与 Ryuk 缺失的泄漏症状/判定/清理方式；验证：清单与代码中 `pgImage`、`ReaperDefaultImage` 常量逐字一致（grep 比对），且含「清理不得删除 `syntopica-postgres`」的警示
- [x] 4.3 修正扩展存储描述：`AGENTS.md` 的「`.pi/extensions/` gitignored + 快照同步 docs/research」已过时（实际源码已入库，30 个文件被 git 追踪）；同步更新 `docs/reference/开发执行规范.md` §4.1 门禁分层表的平台分流描述；验证：`grep -n 'gitignored' AGENTS.md` 零命中，门禁分层表含平台分流
- [x] 4.4 完工汇报：按 AGENTS.md 要求说明部署影响（用户可见行为无变化；开发机需一次性 Docker 配置；门禁在 Linux 上从「静默跳过」变为「真实检查」）与需要用户手动执行的操作（daemon.json 落地 + docker 重启 + 一次性容器清理）；验证：汇报文本含 (a)(b)(c) 三节（行为变化/手动操作/旧数据降级）

## 5. 测试

- [x] 5.1 扩展 `.pi/extensions/tests/quality-gate.behavior.smoke.cjs` 覆盖 native 分支：探测 ENOENT → native 模式不短路、命令经 cwd 构造、失败文案无 wsl 字样；同时保留 Windows 分支断言；验证：`node .pi/extensions/tests/quality-gate.behavior.smoke.cjs` 全断言通过（含新增 native 用例）
- [x] 5.2 harness smoke 全量回归：`bash .pi/extensions/tests/run-harness-smoke.sh`；验证：全部 smoke 文件通过、0 失败（Windows 分支既有断言不回归）
- [x] 5.3 脚本烟测回归：`bash scripts/scenario-trace.smoke.sh` 与 `bash scripts/change-scope.sh --json`；验证：烟测通过、`--json` 输出可被 `JSON.parse` 解析且 `testTargets`/`notices` 字段齐全
- [x] 5.4 修 `scripts/check-standards.smoke.sh` 的 I 段 fixture 缺位（**既有 bug**，非迁移引入）：fixture 只建了 `openspec/changes` 而 I 段要求 `openspec validate --specs` 校验 ≥1 项且零失败，故 I 段引入后本 smoke 必红；补一个最小主 spec fixture；验证：`bash scripts/check-standards.smoke.sh` → 通过 19 / 失败 0

## 6. 验证

- [x] 6.1 `bash .pi/extensions/tests/run-harness-smoke.sh` → 期望 0 失败
- [x] 6.2 `grep -rn -i -E 'wsl|cmd\.exe|/mnt/[a-z]/' AGENTS.md front/AGENTS.md docs/reference/ scripts/ --include='*.md' --include='*.sh'` → 期望剩余命中全部是平台条件化说明或 Windows 分支描述，无「必须走 Windows cmd」类绝对化前提
- [x] 6.3 `grep -rn 'docker.1ms.run\|daocloud\|dockerproxy' --include='*.yml' --include='*.yaml' --include='*.go' .` → 期望零命中（被程序消费的文件里无加速站域名；文档示例不计入，但需带时效标注）
- [x] 6.4 `cd backend-go && go test ./internal/reader/service -run TestArticleContentFormColumnInPGSchema -count=1` → 期望 PASS 且运行前后容器数一致（Ryuk 回收生效）
- [x] 6.5 `bash scripts/doc-impact.sh verify openspec/changes/linux-native-dev-environment` 与 `bash scripts/check-standards.sh` → 期望无 FAIL
- [x] 6.6 真实门禁回归：`node openspec/changes/linux-native-dev-environment/evidence/verify-real-gate.cjs`（真实 child_process，非 mock）→ 已实测 9/9 全过（零 cmd.exe、cwd=backend-go、本机 golangci-lint 真执行、gate.check 落库、零 interop-down、steer 标 (本机)）；**会话级印证**：本回合改 `testutil.go` 触发真实 turn_end，下回合查 `.pi/harness/events.db` 确认 gate.check 落库且零 interop-down
- [x] 6.7 `openspec validate linux-native-dev-environment` → 期望 valid

| Scenario | 测试文件 |
|---|---|
| 原始镜像名经加速来源拉取成功 | 人工（docker pull 原始名执行核对） |
| 仓库内不出现加速站硬编码 | 人工（6.3 grep 零命中） |
| 加速来源不可用时的降级说明 | 人工（4.1 文档评审：含代理与直拉后 tag 两条备选路径） |
| 清单覆盖代码引用的全部镜像 | 人工（4.2 grep 比对 pgImage / ReaperDefaultImage 常量） |
| 按清单可补齐缺失镜像 | 人工（按 4.2 清单依次 docker pull 成功） |
| 泄漏症状可被识别 | 人工（docker ps -aq 计数 + docker ps -a --filter name=ryuk 为空） |
| 泄漏容器清理不误伤生产库 | 人工（1.3 清理后核对 syntopica-postgres healthy 且 ./data 未动） |
| 补齐 Ryuk 后泄漏停止 | backend-go/internal/reader/service/content_form_pg_test.go |
| 无 cmd.exe 时门禁走本机工具链执行 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 探测失败短路整轮 cmd 链路门禁 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 探测成功不改变既有门禁行为 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 探测调用异常也 fail-open | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 环境故障失败不触发粘性重跑 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 真实代码失败保持既有语义 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| native 模式失败一律按代码问题归因 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 后端门禁失败提示含链路标注 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| native 模式失败提示标注本机链路 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 环境故障归因与链路标注同时呈现 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| domain 包改动 | 人工（bash scripts/change-scope.sh --json 输出核对） |
| 新增 domain 零维护 | 人工（bash scripts/change-scope.sh --json 输出核对） |
| 骨架变更不自动 test | 人工（bash scripts/change-scope.sh --json 输出核对） |
| 前端命令标注跨平台限制 | 人工（bash scripts/change-scope.sh --json 输出核对） |
| 未命中路径不猜测 | 人工（bash scripts/change-scope.sh --json 输出核对） |
| quality-gate 消费 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 平台标注随宿主变化 | 人工（bash scripts/change-scope.sh --json 输出核对） |
| domain 改动触发自动测试 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| DB 依赖包跳过 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 前端改动行为不变 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 残留前端脏文件不触发前端门禁 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 会话前残留改动不触发首轮门禁 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 修复循环每轮按新变化触发对应侧 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 映射齐全通过 | scripts/scenario-trace.smoke.sh |
| 缺映射阻断 | scripts/scenario-trace.smoke.sh |
| 映射文件不存在阻断 | scripts/scenario-trace.smoke.sh |
| 人工映射合法 | scripts/scenario-trace.smoke.sh |
| 无 delta Scenario 直接过 | scripts/scenario-trace.smoke.sh |
| WSL 可达前端 dev server | 人工（start-dev.sh 起前端后从 127.0.0.1 与局域网 IP 访问） |
| 文档口径一致 | 人工（grep docs/reference 校验 dev 口径无过时指引） |
| 探测卫生约定平台中立 | 人工（grep docs/reference 校验表述以「存在系统代理时」为条件） |
