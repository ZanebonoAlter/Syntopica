## Purpose

quality-gate 增量门禁对 cmd.exe interop 链路（WSL 调 Windows Go 工具链 / pnpm）的健康防护：环境故障时短路整轮 cmd 链路门禁并显式区分"环境烂"与"代码烂"，防止假回归 steer 与粘性重跑放大。

## ADDED Requirements

### Requirement: interop 探测与门禁短路

quality-gate 在 turn_end 执行 cmd.exe interop 链路门禁（后端 Go 三件套、域测试、pnpm lint）之前，MUST 先对该链路做一次廉价健康探测（短超时，量级远小于门禁命令超时）。探测失败时，本轮全部 cmd.exe 链路门禁命令 MUST 整体跳过（fail-open），不得逐条执行后逐条失败；探测通过时，既有门禁行为（触发路由、采样记账、lint 同根因短路）MUST 保持不变。探测调用自身异常时 MUST 按 fail-open 处理（跳过门禁，不阻断回合）。

#### Scenario: 探测失败短路整轮 cmd 链路门禁

- **WHEN** turn_end 触发门禁且 interop 健康探测超时或失败（如 WSL vsock 故障期）
- **THEN** 本轮后端门禁命令与 pnpm lint 均不执行，回合正常结束，steer 消息提示 WSL interop 环境故障及恢复建议（如重启 WSL），不出现任何门禁命令的失败记录

#### Scenario: 探测成功不改变既有门禁行为

- **WHEN** interop 探测通过后按既有路由执行门禁
- **THEN** 触发集命中、粘性重跑、采样记账、lint 同根因短路等行为与探测机制引入前完全一致

#### Scenario: 探测调用异常也 fail-open

- **WHEN** 探测命令自身抛出异常（非超时返回）
- **THEN** 按 interop 故障短路处理（跳过本轮 cmd 链路门禁），不向 agent 抛错、不阻断回合

### Requirement: 环境故障识别与失败粘性豁免

gate.check 执行失败时，MUST 识别输出中的 WSL interop 故障特征（UtilAcceptVsock / WSL ERROR 类 stderr 噪声）。命中环境故障特征的失败 MUST NOT 计入失败粘性集合（即下回合不得因此重跑该命令），且 steer 消息 MUST 归因为环境问题而非代码回归；未命中特征的失败维持既有粘性与 [回归]/[中间态] 分级语义不变。

#### Scenario: 环境故障失败不触发粘性重跑

- **WHEN** 探测通过但某门禁命令执行中途 interop 故障，diag 含 UtilAcceptVsock 特征
- **THEN** 该命令失败不进入粘性集合，下回合纯对话时不重跑该命令，steer 归因为 WSL 环境故障

#### Scenario: 真实代码失败保持既有语义

- **WHEN** 门禁命令失败且 diag 不含环境故障特征（如编译错误、lint 发现、测试失败）
- **THEN** 维持既有行为：计入粘性集合、下回合重跑催修、按 [回归]/[中间态] 分级 steer

### Requirement: cmd 链路失败提示显式标注执行链路

凡通过 cmd.exe interop 执行的门禁命令，其失败 steer 提示 MUST 显式标注执行链路（wsl环境），标注不得弱化或掩盖真实代码失败的修复义务。

#### Scenario: 后端门禁失败提示含链路标注

- **WHEN** 任一 cmd.exe 链路门禁命令失败并产生 steer 提示
- **THEN** 提示中含（wsl环境）链路标注，表明该检查经 WSL interop 跨系统执行，存在环境故障可能

#### Scenario: 环境故障归因与链路标注同时呈现

- **WHEN** 失败被识别为环境故障（探测短路或 diag 特征命中）
- **THEN** steer 明确提示为 WSL interop 环境故障、非代码问题、含恢复建议，并保留（wsl环境）链路标注
