## Context

quality-gate.ts 现有三态执行链路判定（`linux-native-dev-environment` 引入）：windows 模式每回合做 cmd.exe interop 健康探测、失败短路 + `interop-down` 记账；native 模式「不探测、不短路」。2026-09-17 事故（events.db 实证：两个会话 945 条 `command not found` 假失败，占 7 天窗口失败 56%）证明 native 有自己的环境故障形态——pi 启动环境 PATH 缺工具链，命令秒败经粘性放大。根因与挂载点已探明（docs/research/harness-gate-hardening/explore-findings.md）。既有可抄先例：`cmdExeReachable()` 的 PATH accessSync 扫描、interop-down 短路分支、`isInteropFailure()` 特征归因。

## Goals / Non-Goals

**Goals:**

- native 模式工具链不可达时短路对应侧门禁，账本零假 gate.check 失败
- 工具缺失特征的漏网失败不进粘性、不按代码失败分级
- 行为对齐 gate-interop-health 的既有约束（会话内判定稳定、windows 模式不变、链路标注如实）

**Non-Goals:**

- 不做 windows 模式的工具链探测（该模式用 Windows 绝对路径执行，故障形态由 interop 探测覆盖）
- 不做工具链版本/兼容性探测（PATH 可达 ≠ 版本对，版本问题按真实失败暴露）
- 不改 harness-retro.sh 分类（修复后 `command not found` 归并自然失去输入）
- 不做探测失败自动重试或自动恢复（恢复交给人：修 pi 启动环境后重启会话）

## Decisions

### D1 探测方式：PATH 目录 accessSync 扫描，不 exec 探测命令

复用 `cmdExeReachable()` 的模式（quality-gate.ts:105-121）：遍历 `process.env.PATH` 目录，对可执行名做 `accessSync(join(dir,name), X_OK)`。理由：pi.exec 底层 spawn 的 ENOENT 被 execCommand catch 统一转成 `{code:1, stdout:"", stderr:""}`（quality-gate.ts:99-104 注释已锚定），「文件不存在」与「命令真实失败」在执行结果上不可区分；且文件可达性检查零开销。备选（exec `which go` / `go version`）被否：返回码语义混淆 + exec 开销 + 超时复杂度。

### D2 探测粒度：侧级短路（后端 / 前端各自判定）

后端侧探测 `go` **与** `golangci-lint`（三件套 + 域测试共用 go，lint 单用 golangci-lint；任一缺失即整侧短路——工具链不完整说明环境烂透了，跑一半只会制造半截信号，且 lint 先行哨兵逻辑依赖 lint 可执行）；前端侧探测 `pnpm`。两侧独立：后端短路不影响前端门禁照常执行。备选（命令级短路，缺哪个跳哪个）被否：保留覆盖的收益仅覆盖"部分缺失"罕见场景，代价是 lint 哨兵分支复杂化与提示碎片化。

### D3 探测缓存：按 (session, 侧) 缓存，会话内稳定

探测结果模块级缓存，`session_start` 重置（对齐 `execPlatform` 缓存先例与 gate-interop-health「平台判定 MUST 会话内稳定」约束）。理由：PATH 是 pi 进程属性，运行中不会变；事故场景的恢复路径就是重启会话。备选（逐回合探测，对齐 interop-down）被否：interop 逐回合探测是因为 vsock 健康是逐回合属性，工具链可达不是。

### D4 事后归因：`isToolNotFound()` 特征函数，仅 native 模式判定

`lib/failure-classify.ts` 新增特征函数，匹配 `/command not found/i`（bash 标准文案 `bash: line 1: go: command not found`）。命中 → 进 `envFailures` 通道（不进 stickyFailures、不参与 [回归]/[中间态] 分级、steer 归因环境）。仅 native 模式参与判定——与 `isInteropFailure()` 仅 windows 判定对称，防止 windows 侧同名字符串误触发跨语义归因。`envFailures` 现有文案为 interop 专属，按特征分流文案。这是探测缓存后的兜底（探测时在、执行时被删的窗口期），预期极少触发。

### D5 记账与提示：边沿触发（进入短路态首个回合记一条）

`policy.decision(action=fail-open, reasonCode=toolchain-down)`，target 标注侧（backend/frontend）。进入某侧短路态的首个回合记一条 + 发一条 steer；后续回合短路不再重复记/发。理由：探测会话缓存后结果恒定，每回合重复记 223 条同值事件无信息增量，违反 policy-decision 低噪声约束（「普通成功放行不得调用」同精神）；interop-down 每回合记是因为其探测结果逐回合可变。steer 文案：本机链路标注 + 非代码问题定性 + 恢复方向指向 pi 启动环境 PATH（禁「重启 WSL」类跨系统建议，对齐 gate-interop-health 链路标注约束）。

### D6 域测试与 change-scope 链路随侧短路，不做单独处理

域测试命令经 `runBackend` 同链路执行，被后端侧短路自然覆盖；`change-scope.sh` 用绝对路径 + bash（恒在），不受影响，短路侧的域测试预算逻辑随之跳过。

## Risks / Trade-offs

- **探测假阳性**（PATH 有目录但工具实际跑不动，如版本不符）：按真实失败暴露，不归因环境——可接受，版本问题本就该修
- **会话内误短路持续整个会话**：PATH 进程属性不会中途变，接受；用户修环境重启会话即恢复（与 execPlatform 同生命周期语义）
- **envFailures 文案分流复杂度**：interop 与 toolchain 两类环境故障共用通道，文案必须按特征分流，错发会给出错误的恢复建议（如 native 场景建议重启 WSL）——smoke 用例覆盖
- **边沿记账的可见性**：长故障期账本只有 1-2 条记录，retro ③ 段的故障时长感知变弱——trade-off 接受（噪声危害大于时长感知损失，且 steer/日志仍有痕迹）
