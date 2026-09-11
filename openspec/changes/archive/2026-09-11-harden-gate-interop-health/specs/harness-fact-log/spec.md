## MODIFIED Requirements

### Requirement: 策略显著裁决统一记账

spec-gate、quota-gate、test-scope-guard SHALL 将显著裁决追加为 `policy.decision` 事件；quality-gate 的 interop 探测短路 SHALL 作为其唯一的 policy.decision 场景追加（action=fail-open，reasonCode=interop-down）。payload MUST 含 `policy`、`action`、`reasonCode`：`policy` 限定为稳定扩展标识，`action` 限定为 `block | warn | bypass | fail-open`，`reasonCode` MUST 是稳定、非空、kebab-case 的有界代码；按需附加的 `target` MUST 是不含密钥、完整命令和远端响应正文的有界摘要，`durationMs` 若存在 MUST 为非负数。事件的 change 列 SHALL 优先绑定裁决明确指向的 change，否则使用当前可检测的活跃 change，无法确定时为 null。

普通成功放行 MUST NOT 写 `policy.decision`；quality-gate 其余裁决与 entry-gate 继续使用 `gate.check`，同一裁决 MUST NOT 双写（interop 短路记 policy.decision 后，被跳过的门禁命令 MUST NOT 再逐条记 gate.check）。记账失败 MUST NOT 改变原策略的放行、提醒、阻断或 fail-open 结果。

#### Scenario: spec-gate 阻断归档被记录

- **WHEN** spec-gate 因归档前检查失败阻断 `openspec archive <change>`
- **THEN** 追加一条 policy=spec-gate、action=block 的 policy.decision，reasonCode 表示归档检查失败且 change 绑定被归档 change

#### Scenario: spec-gate 显式豁免被记录

- **WHEN** 归档命令通过 `--force` 或 `SPEC_GATE_BYPASS=1` 绕过检查
- **THEN** 追加一条 policy=spec-gate、action=bypass 的 policy.decision；放行行为保持不变

#### Scenario: quota-gate 阻断与 fail-open 被区分

- **WHEN** quota-gate 分别因额度不足阻断一次派发、因额度查询失败放行一次派发
- **THEN** 分别追加 action=block 与 action=fail-open 的 policy.decision，target 仅含 provider 等安全摘要，不含 API key 或响应正文

#### Scenario: test-scope 软硬模式被记录

- **WHEN** test-scope-guard 在 soft 模式提醒一次、在 hard 模式阻断一次非归档语境的全量测试
- **THEN** 分别追加 action=warn 与 action=block、reasonCode=full-go-test 的 policy.decision

#### Scenario: interop 探测短路被记账且不双写

- **WHEN** quality-gate 的 interop 健康探测失败导致本轮 cmd 链路门禁整体短路
- **THEN** 追加一条 policy=quality-gate、action=fail-open、reasonCode=interop-down 的 policy.decision，且本轮被跳过的门禁命令不再产生任何 gate.check 事件

#### Scenario: 正常放行零记录

- **WHEN** spec-gate 的归档检查全部通过、quota-gate 判定额度充足、命令未命中 test-scope-guard，或 interop 探测健康后门禁正常执行
- **THEN** 不产生 policy.decision 事件（探测健康本身零记录）

#### Scenario: 记账故障不改变裁决

- **WHEN** events.db 不可写且策略本应阻断、提醒、豁免或 fail-open
- **THEN** 策略仍执行原裁决，记账仅走既有 fail-loud/fail-safe 错误路径
