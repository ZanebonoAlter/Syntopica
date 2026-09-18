# Tasks

## 1. failure-classify 特征函数（用例先行）

- [x] 1.1 在 `.pi/extensions/tests/failure-classify.smoke.cjs` 先补用例：`isToolNotFound("bash: line 1: go: command not found")` 为 true；`isToolNotFound("internal/reader/handler/opml.go:51:1: commentFormatting...")`（lint 发现）、`isToolNotFound("FAIL ./internal/admin [build failed]")`（测试失败）、`isToolNotFound("")`（空输出）为 false。跑 `bash .pi/extensions/tests/run-harness-smoke.sh` 确认新用例红（函数未存在）
- [x] 1.2 在 `.pi/extensions/lib/failure-classify.ts` 实现 `isToolNotFound(output)`：匹配 `/command not found/i`（对齐 `isInteropFailure()` 的形态与注释风格，注明仅 native 模式参与判定）；重跑 1.1 命令至全绿

## 2. quality-gate native 工具链探测与短路

- [x] 2.1 在 `.pi/extensions/quality-gate.ts` 新增工具链可达性检查：复用 `cmdExeReachable()` 的 PATH accessSync 扫描模式实现 `exeReachable(name)`；模块级缓存 `(session, 侧)` 级探测结果（backend: go+golangci-lint 双检；frontend: pnpm 单检），`session_start`/`session_shutdown` 与 execPlatform 同点位重置（design D1/D2/D3）
- [x] 2.2 turn_end 门禁执行前（step 3.5 native 分支之后）：某侧工具链不可达 → 该侧门禁整体跳过（fail-open），另一侧照常；进入短路态的首个回合记一条 `policy.decision(action=fail-open, reasonCode=toolchain-down, target=backend|frontend)` 并发一条 steer（本机链路标注 + 非代码问题 + 恢复方向指向 pi 启动环境 PATH，禁「重启 WSL」类建议）；后续回合短路不重复记/发（design D5 边沿触发）
- [x] 2.3 `gateLog` 事后归因兜底：native 模式下失败输出命中 `isToolNotFound()` → 进 envFailures（不进 stickyFailures、不参与 [回归]/[中间态] 分级），文案按特征分流——toolchain 归因文案（工具链缺失，重启会话恢复）与 interop 文案（WSL interop，建议 wsl --shutdown）不得错发；windows 模式不参与 isToolNotFound 判定（design D4）

## 测试

- [x] T1 在 quality-gate 相关 smoke（`quality-gate.behavior.smoke.cjs` 或新增专用文件）补用例：① PATH 无 go 时后端门禁三件套零执行、零 gate.check、记一条 toolchain-down（不重复记）② PATH 无 pnpm 时前端 lint 跳过、后端照常 ③ 工具链齐全时行为与既有用例一致 ④ native 下 command not found 失败不进粘性、steer 归因环境 ⑤ windows 模式（cmd.exe 可达）不受工具链探测影响
- [x] T2 全量扩展 smoke 回归：`bash .pi/extensions/tests/run-harness-smoke.sh`（及 run-smoke.sh 若涉及 quality-gate bundle）无新增红
- [x] T3 实机演练一次短路路径（临时收窄 PATH 触发；若实机不可控则以 T1 smoke 覆盖为准并记录跳过理由），确认 steer 文案与边沿记账符合 design D5

## 文档

<!-- doc-impact: none(harness 机制文档不在 doc-impact 七域；pi-extensions.md 属 harness 扩展机制全景文档，随本 change 就地同步) -->

- [x] D1 `docs/reference/harness/pi-extensions.md`：quality-gate 节补「native 工具链探测短路（toolchain-down）」小节——触发条件/短路范围/边沿记账/恢复路径，与 interop-down 并列表述
- [x] D2 research finding 回链：`docs/research/harness-gate-hardening/explore-findings.md` 标注问题一已由本 change 处理（问题二仍开放）

## 验证

- [x] V1 `openspec validate harden-gate-native-toolchain` → 输出 valid
- [x] V2 `bash .pi/extensions/tests/run-harness-smoke.sh` → 全部断言通过（exit 0，含新增 isToolNotFound 与短路行为用例）
- [x] V3 `cd backend-go && golangci-lint run ./... && go build ./...` → 无告警、构建成功（确认扩展改动未波及后端构建面）
- [x] V4 `git status --short` → 本 change 变更仅含 `.pi/extensions/`（quality-gate.ts、failure-classify.ts、tests/×3）、`docs/reference/harness/pi-extensions.md`、`openspec/changes/harden-gate-native-toolchain/`（docs/research/ 按仓库 .gitignore:65 策略本地留档不入库，回链已写入）；另两 change 的归档移动（harness-effectiveness-metrics / harness-retro-sql-fixes → archive/）为无关脏改动，不属本 change
