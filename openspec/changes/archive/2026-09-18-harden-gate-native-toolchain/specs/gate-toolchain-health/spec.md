## Purpose

quality-gate 增量门禁对 native 模式（Linux/macOS 本机工具链宿主）的工具链健康防护：启动环境 PATH 缺失 go / golangci-lint / pnpm 时短路对应侧门禁并显式区分"环境烂"与"代码烂"，防止 command-not-found 假失败经粘性重跑放大污染账本与误导修复（2026-09-17 实测单次事故 945 条假失败，占 7 天窗口失败 56%）。

## ADDED Requirements

### Requirement: native 工具链探测与门禁短路

quality-gate 在 turn_end 以 native 模式执行某侧门禁（后端 Go 三件套与域测试 / 前端 lint）之前，MUST 先探测该侧所需工具链可执行文件（后端：go 与 golangci-lint；前端：pnpm）在进程 PATH 上可达。探测 MUST 为文件可达性检查，MUST NOT 依赖执行探测命令的返回码（可执行文件缺失导致的失败与命令真实失败在执行结果上不可区分）。探测结果 MUST 在会话内缓存稳定，MUST NOT 逐回合重判。

工具链不可达时，该侧门禁 MUST 整体跳过（fail-open），不得逐条执行后逐条失败；另一侧（工具链可达侧）的门禁行为 MUST 不受影响。windows 模式（cmd.exe interop 链路）的既有探测与执行行为 MUST 保持不变。

#### Scenario: PATH 缺 go 时后端门禁整体短路

- **WHEN** native 模式下 turn_end 触发后端门禁，且探测发现 go 或 golangci-lint 不在 PATH 上
- **THEN** 本轮后端门禁命令均不执行，回合正常结束，steer 提示工具链缺失为环境问题及恢复方向；前端门禁（若触发且 pnpm 可达）照常执行

#### Scenario: 工具链齐全时门禁行为不变

- **WHEN** native 模式下探测发现全部所需工具链可达
- **THEN** 触发集命中、粘性重跑、采样记账、lint 同根因短路、域测试预算等既有门禁行为与本防护引入前完全一致

#### Scenario: 探测结果会话内稳定

- **WHEN** 同一会话内多个回合触发门禁探测
- **THEN** 工具链可达性判定结果保持稳定，不逐回合重判、不因单回合抖动在短路/执行间反复切换

#### Scenario: windows 模式不受影响

- **WHEN** cmd.exe interop 链路下的门禁回合
- **THEN** 沿用既有 interop 健康探测语义，不引入本需求的工具链探测与环境归因

### Requirement: 工具缺失失败归因与粘性豁免

native 模式下门禁命令执行失败且输出含工具缺失特征（如 `command not found`）时，MUST 归因为环境故障：该失败 MUST NOT 计入失败粘性集合（下回合不得因此重跑该命令），MUST NOT 按 [回归]/[中间态] 代码失败分级，steer 消息 MUST 明确提示为工具链环境问题而非代码回归。未命中工具缺失特征的失败维持既有粘性与分级语义不变。

#### Scenario: 工具缺失失败不触发粘性重跑

- **WHEN** native 模式下某门禁命令失败，输出含 command not found 特征（如探测缓存后 PATH 内文件被移除的兜底场景）
- **THEN** 该命令失败不进入粘性集合，下回合纯对话时不重跑该命令，steer 归因为工具链环境问题

#### Scenario: 真实代码失败保持既有语义

- **WHEN** native 模式下门禁命令失败且输出不含工具缺失特征（编译错误、lint 发现、测试失败）
- **THEN** 维持既有行为：计入粘性集合、下回合重跑催修、按 [回归]/[中间态] 分级 steer

### Requirement: 短路记账与环境提示

工具链短路 MUST 记一条 policy.decision 事件（action=fail-open，reasonCode 为工具链缺失的有界稳定代码），被跳过的门禁命令 MUST NOT 产生 gate.check 失败记账（防止账本连环假失败）。steer 提示 MUST 标注本机原生链路，MUST NOT 出现「重启 WSL」之类跨系统恢复建议；恢复方向 MUST 指向修复 pi 启动环境的 PATH（工具链安装位置或启动方式）。

#### Scenario: 短路记账一条且零 gate.check

- **WHEN** 某回合因工具链不可达短路门禁
- **THEN** 账本新增一条 fail-open 的 policy.decision 记账，本轮无任何被跳过命令的 gate.check 事件

#### Scenario: 提示文案标注本机链路与恢复方向

- **WHEN** 工具链短路或工具缺失归因触发 steer 提示
- **THEN** 提示含本机链路标注与「非代码问题」定性，恢复建议指向 pi 启动环境 PATH，不含跨系统运行时操作建议
