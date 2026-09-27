# Design — harden-gate-interop-health

## Context

后端门禁（lint/vet/build/域测试）与 pnpm lint 均经 `pi.exec("cmd.exe")` 跨 WSL→Windows interop（vsock）执行，这是既有正确架构（WSL Go 1.18 认不了 go 1.25 的 go.mod），不动。见 proposal.md - Why：vsock 故障期（实测 8/28、9/3、9/5 三次，单次持续 40+ 分钟）整条链路连环失败，每命令 ~10s 白等，且被 `stickyFailures` 粘性重跑放大为假 [回归] steer。

现状关键代码事实（`.pi/extensions/quality-gate.ts`）：

- `gateLog()` 在 `code !== 0` 时无差别 `stickyFailures.add(cmd)` 并推入 `[回归]/[中间态]` 失败列表 → 环境故障与代码失败不分。
- 后端命令 timeout 120s；vsock 故障时实际 ~10s 快速失败（interop 层报 `UtilAcceptVsock:251: accept4 failed 110` = ETIMEDOUT 后退出）。
- 正常时 `cmd.exe /C echo ok` 实测 ~40ms。
- 前端（pnpm lint）与后端走同一 interop 通道，9/3 故障期 pnpm lint 同样连环失败。

## Goals / Non-Goals

**Goals**

- 故障期单 turn 门禁开销从 5 命令 × ~10s 白等降为一次探测 ≤2s。
- 环境故障不再进粘性集合、不再以 [回归] 面目误导 agent。
- cmd 链路命令失败 steer 显式标注（wsl环境）。
- 事件账本可考古：故障短路记 policy.decision（interop-down）。

**Non-Goals**

- 不改门禁命令执行方式（仍 cmd.exe 调 Windows 工具链）、不做自动重试/自动 `wsl --shutdown`（故障持续期重试无意义，恢复动作交给人）。
- 不改 entry-gate / 其他扩展。
- 不处理 `.wslconfig` 运维调优（swap=0 等根因缓解另行决定，见 Open Questions）。

## Decisions

### D1: 探测命令 `cmd.exe /C echo ok`，超时 2s，每 turn 至多一次

正常实测 ~40ms，2s 是 50 倍余量（容忍宿主瞬时卡顿），又远小于故障期单命令 10s。探测仅在「本轮确实要执行 cmd 链路门禁」时发起（后端触发或前端触发，二者共享同一 interop 通道，探测一次覆盖两侧），纯对话 turn（无代码改动且无粘性）不探测。

替代方案否决：探测放 session_start——粒度太粗，session 跨数小时，故障期开始后仍会踩雷；每命令前各探测一次——多余，vsock 是通道级故障。

### D2: 三态结果处理（fail-open 短路 / 正常执行 / 探测自身异常）

- 探测失败（超时或 exit≠0）→ 本轮后端+前端 cmd 链路门禁整体跳过，steer 一段「WSL interop 环境故障，非代码问题，建议重启 WSL（wsl --shutdown）后继续」，记 policy.decision。
- 探测通过 → 走既有流程，零额外行为。
- 探测调用抛异常 → 按 fail-open 同短路（catch 后与超时同路径），不向 pi 抛错。

### D3: 环境故障特征识别——interop 层 stderr 前缀，非全文匹配

识别目标限定 WSL interop 注入的 stderr 前缀形态（`<3>WSL (… - ) ERROR:` 及 `UtilAcceptVsock` 关键字）。该前缀由 WSL 层写入，仅在 cmd.exe 未能正常启动/通信时出现，不会与正常门禁输出（编译错误、lint 发现）混合——cmd.exe 成功启动后命令输出不含此形态。因此「特征命中 ⇒ 环境故障」判定安全，不存在"编译错误被误判为环境故障"的路径。

兜底场景：探测通过但某命令执行中途 interop 挂（diag 含特征）→ gateLog 处识别特征：**不 add sticky、不推 [回归]/[中间态] failures**，改为单独环境故障提示段（含 (wsl环境) 标注）。

### D4: （wsl环境）标注嵌入既有 steer 格式

cmd 链路门禁命令失败行从 `[golangci-lint] exit 1` 变为 `[golangci-lint (wsl环境)] exit 1`；标注对真实代码失败同样生效（链路透明），不弱化修复义务——真实失败仍走粘性+分级。WSL 原生命令（当前门禁集里没有）不标注。

### D5: 记账路径——policy.decision 单写，gate.check 零假事件

- 探测短路：logEvent 一条 `{kind: policy.decision, policy: quality-gate, action: fail-open, reasonCode: interop-down, durationMs: 探测耗时}`；被跳过的门禁命令**不记 gate.check**（否则账本又回到连环假失败的老问题）。
- 探测健康：零记录（沿用低噪声约束——普通成功放行不记 policy.decision）。
- diag 特征命中的单命令环境失败：维持 gate.check 全量失败记账（ok=false，diag 本身含特征，考古可辨），但不进粘性、steer 归因环境。

### D6: 验证方式——sqlite 断言 + 故障注入，不建单测基建

项目无 harness 扩展单测先例（.pi/extensions gitignored，快照在 docs/research/）。验证配方：

1. 正常态：改一行后端代码触发 turn_end → 探测通过、门禁照常、gate.check 正常落库、无 policy.decision。
2. 短路态：临时把探测超时调至 1ms（或 mock exec）模拟故障 → 门禁跳过、policy.decision 落库、gate.check 零新增、下 turn 纯对话不再探测。
3. 兜底态：手工构造 diag 含 `UtilAcceptVsock` 的失败 → 不进粘性（下 turn 不重跑）、steer 含环境归因。

## Risks / Trade-offs

- [探测每 turn 额外 ~40ms 开销（正常态）] → 相对门禁命令秒级耗时可忽略；纯对话 turn 不探测。
- [vsock 半死状态：探测通过但命令执行中挂] → D3 兜底分支覆盖（diag 特征 → 粘性豁免 + 环境归因）。
- [`<3>WSL` 前缀形态随 WSL 版本变化] → 特征正则同时锚定 `UtilAcceptVsock` 与 `WSL (…ERROR` 双关键字，单一形态漂移仍可命中；两者都漂移时兜底失效、退化为既有行为（假回归重现），可考古发现后修。
- [环境故障误判导致真实失败被放过] → D3 论证 interop 前缀不可能与正常输出混合；且故障期门禁本就跑不出有效结果，fail-open 符合 quality-gate 既有失败策略。

## Migration Plan

单文件改动（quality-gate.ts）+ 文档同步（docs/research/harness事实库.md 词汇表、快照同步）。无数据迁移；events.db 旧数据不受影响（新 reasonCode 只增不改旧）。回滚 = 还原 quality-gate.ts。

## Open Questions

- `.wslconfig` 的 `swap=0` 是否加剧 vsock 故障（编译峰值内存无处去）？属运维层，可与本 change 并行观察（9/3 型故障复现频率），不影响本设计。
