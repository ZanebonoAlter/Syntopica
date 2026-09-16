## MODIFIED Requirements

### Requirement: interop 探测与门禁短路

quality-gate 在 turn_end 执行门禁（后端 Go 三件套、域测试、pnpm lint）之前，MUST 先做一次执行链路判定，判定结果 MUST 分三态：

1. **native 模式**：探测表明 `cmd.exe` 不存在（调用因可执行文件缺失而失败）——本机即工具链宿主，门禁 MUST 用本机可执行文件（`go` / `golangci-lint` / `pnpm`，经 PATH 查找）正常执行，不短路、不产生环境故障提示；命令的工作目录 MUST 通过进程工作目录参数指定，MUST NOT 依赖跨系统路径拼接与 `cd /d` 之类的 shell 内建。
2. **Windows 模式**：`cmd.exe` 存在且廉价健康探测（短超时，量级远小于门禁命令超时）通过——按既有 cmd.exe interop 链路执行门禁。
3. **故障短路**：`cmd.exe` 存在但探测失败或超时——本轮全部门禁命令整体跳过（fail-open），不得逐条执行后逐条失败；探测调用自身异常时同样按短路处理（跳过门禁，不阻断回合）。

两种正常模式下，既有门禁行为（触发路由、粘性重跑、采样记账、lint 同根因短路、domain 测试预算）MUST 完全一致。平台判定 MUST 在会话内稳定，MUST NOT 因单回合探测抖动在模式下反复切换。

#### Scenario: 无 cmd.exe 时门禁走本机工具链执行

- **WHEN** turn_end 触发门禁且探测发现 `cmd.exe` 不存在（Linux/macOS 开发主机）
- **THEN** 命中的门禁命令以本机可执行文件正常执行并照常记账 gate.check，不出现「环境故障」措辞的 steer 消息，本回合改动被门禁真实覆盖

#### Scenario: 探测失败短路整轮 cmd 链路门禁

- **WHEN** turn_end 触发门禁且 `cmd.exe` 存在但健康探测超时或失败（如 WSL vsock 故障期）
- **THEN** 本轮后端门禁命令与 pnpm lint 均不执行，回合正常结束，steer 消息提示 interop 环境故障及恢复建议，不出现任何门禁命令的失败记录

#### Scenario: 探测成功不改变既有门禁行为

- **WHEN** `cmd.exe` 存在且探测通过后按既有路由执行门禁
- **THEN** 触发集命中、粘性重跑、采样记账、lint 同根因短路等行为与探测机制引入前完全一致

#### Scenario: 探测调用异常也 fail-open

- **WHEN** `cmd.exe` 存在但探测命令自身抛出异常（非超时返回）
- **THEN** 按 interop 故障短路处理（跳过本轮门禁），不向 agent 抛错、不阻断回合

### Requirement: 环境故障识别与失败粘性豁免

在 cmd.exe interop 链路下执行门禁时，gate.check 执行失败 MUST 识别输出中的 interop 故障特征（UtilAcceptVsock / WSL ERROR 类 stderr 噪声）。命中环境故障特征的失败 MUST NOT 计入失败粘性集合（即下回合不得因此重跑该命令），且 steer 消息 MUST 归因为环境问题而非代码回归；未命中特征的失败维持既有粘性与 [回归]/[中间态] 分级语义不变。native 模式下不存在跨系统链路，MUST NOT 将命令输出误判为 interop 环境故障——该模式下的失败一律按真实代码失败处理。

#### Scenario: 环境故障失败不触发粘性重跑

- **WHEN** cmd.exe 链路下探测通过但某门禁命令执行中途 interop 故障，diag 含 UtilAcceptVsock 特征
- **THEN** 该命令失败不进入粘性集合，下回合纯对话时不重跑该命令，steer 归因为 interop 环境故障

#### Scenario: 真实代码失败保持既有语义

- **WHEN** 门禁命令失败且 diag 不含环境故障特征（如编译错误、lint 发现、测试失败）
- **THEN** 维持既有行为：计入粘性集合、下回合重跑催修、按 [回归]/[中间态] 分级 steer

#### Scenario: native 模式失败一律按代码问题归因

- **WHEN** native 模式下门禁命令失败且输出恰含与 interop 故障特征相同的字符串
- **THEN** 该失败仍按真实代码失败处理（进粘性集合、分级 steer），不标注为环境故障、不建议重启任何跨系统运行时

### Requirement: cmd 链路失败提示显式标注执行链路

门禁命令失败时，其 steer 提示 MUST 显式标注执行链路，标注 MUST 与实际执行方式一致：经 cmd.exe interop 执行的标注为跨系统链路（wsl环境），native 模式执行的标注为本机原生链路。标注不得弱化或掩盖真实代码失败的修复义务；native 模式的环境故障文案 MUST NOT 出现「重启 WSL」之类的跨系统恢复建议。

#### Scenario: 后端门禁失败提示含链路标注

- **WHEN** 任一 cmd.exe 链路门禁命令失败并产生 steer 提示
- **THEN** 提示中含（wsl环境）链路标注，表明该检查经跨系统 interop 执行，存在环境故障可能

#### Scenario: native 模式失败提示标注本机链路

- **WHEN** native 模式下某门禁命令失败并产生 steer 提示
- **THEN** 提示标注为本机原生执行，不出现 wsl环境 字样与跨系统恢复建议

#### Scenario: 环境故障归因与链路标注同时呈现

- **WHEN** 失败被识别为环境故障（探测短路或 diag 特征命中）
- **THEN** steer 明确提示为 interop 环境故障、非代码问题、含恢复建议，并保留（wsl环境）链路标注
