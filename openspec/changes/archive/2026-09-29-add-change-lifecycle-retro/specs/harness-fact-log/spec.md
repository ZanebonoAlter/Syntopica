# harness-fact-log Delta

> 注：本 delta 与 add-foreign-gate-steer-attribution 的 harness-fact-log delta 修改同一 Requirement（策略显著裁决统一记账）。该 change 已于 2026-09-29 先行归档 sync（主 spec 已含归因降级词汇与 scenario），本文本为合并后状态（已补入其「归因降级」scenario，见 design D4）。

## MODIFIED Requirements

### Requirement: 策略显著裁决统一记账

spec-gate、quota-gate、test-scope-guard SHALL 将显著裁决追加为 `policy.decision` 事件；quality-gate 的 interop 探测短路与**归因降级** SHALL 作为其 policy.decision 场景追加（前者 action=fail-open，reasonCode=interop-down；后者 action=warn，reasonCode 为表达「归属信号缺席/判定异常而保守回退」语义的稳定 kebab-case 词汇，具体值由设计阶段定义并登记词汇表）；ui-design-gate 的**用户审批通过与放行** SHALL 作为放行侧高价值锚点追加（action=pass，reasonCode 表达放行语义，具体词汇由设计阶段定义；与 change.archive 的「低频高价值成功锚点」同族例外）。payload MUST 含 `policy`、`action`、`reasonCode`：`policy` 限定为稳定扩展标识，`action` 限定为 `block | warn | bypass | fail-open | pass`，`reasonCode` MUST 是稳定、非空、kebab-case 的有界代码；按需附加的 `target` MUST 是不含密钥、完整命令和远端响应正文的有界摘要，`durationMs` 若存在 MUST 为非负数。事件的 change 列 SHALL 优先绑定裁决明确指向的 change，否则使用当前可检测的活跃 change，无法确定时为 null。

dev-process-guard 的孤儿 dev 进程治理事件是上述 payload 形状的**唯一显式豁免**：其 `policy.decision` payload SHALL 为 `decision`（`orphan-killed | orphan-warn`）键 + 进程摘要（`cmd`/`offenders` 等有界字段），MUST NOT 含 `action` 键，且豁免场景**不要求** `policy`/`reasonCode` 键（豁免的是整个键形状而非仅 action→decision 替换；白名单语义为「对用户操作的裁决」，不适用于对进程组的处置；且避免 harness-retro ③段 fail-open 分桶与④段软提醒聚类、催修时距误吸本扩展事件）。该豁免 MUST 登记于事件词汇表文档；除豁免场景外，任何扩展写入 `policy.decision` MUST 遵守 `policy`/`action`/`reasonCode` 形状，词汇表文档与实现 SHALL 保持一致。

普通成功放行 MUST NOT 写 `policy.decision`，**唯一例外**是 ui-design-gate 的用户审批通过（用户在对话中明确批准原型，低频高价值锚点，与 change.archive 同族）；quality-gate 其余裁决与 entry-gate 继续使用 `gate.check`，同一裁决 MUST NOT 双写（interop 短路记 policy.decision 后，被跳过的门禁命令 MUST NOT 再逐条记 gate.check；归因降级记账 MUST NOT 与 gate.check 的失败事实构成双写）。记账失败 MUST NOT 改变原策略的放行、提醒、阻断或 fail-open 结果。

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

#### Scenario: dev-process-guard 孤儿治理事件按豁免形状记账

- **WHEN** dev-process-guard 在会话结束清理中杀死或警告孤儿 dev 进程组
- **THEN** 追加 payload 含 `decision`（orphan-killed 或 orphan-warn）与进程摘要的 policy.decision，MUST NOT 含 `action` 键；harness-retro 的③段（fail-open）、④段（warn 聚类）与催修时距计算 MUST NOT 吸入该事件

#### Scenario: interop 探测短路被记账且不双写

- **WHEN** quality-gate 的 interop 健康探测失败导致本轮 cmd 链路门禁整体短路
- **THEN** 追加一条 policy=quality-gate、action=fail-open、reasonCode=interop-down 的 policy.decision，且本轮被跳过的门禁命令不再产生任何 gate.check 事件

#### Scenario: 归因降级被记账且不双写失败事实

- **WHEN** 某门禁命令失败，归属判定因基线快照失败或信号缺席保守回退本会话失败
- **THEN** gate.check 记失败事实（diag + ownership=attribution-degraded）一条，policy.decision 记归因降级原因一条，两者各司其职不构成同事实双写

#### Scenario: UI 审批通过与放行被记录

- **WHEN** 某 ui-impact: major 的 change 原型经用户在对话中明确批准，ui-design-gate 将 ui-approval 从 pending 置为 approved 并放行后续制品
- **THEN** 追加一条 policy=ui-design-gate、action=pass 的 policy.decision（change 绑定该 change）；补齐 ui-design 文档后从 block 到放行的路径同理解除并记 pass

#### Scenario: 正常放行零记录

- **WHEN** spec-gate 的归档检查全部通过、quota-gate 判定额度充足、命令未命中 test-scope-guard，或 interop 探测健康后门禁正常执行；或 ui-design-gate 对 ui-impact: none/minor 的 change 不要求审批而直接放行
- **THEN** 不产生 policy.decision 事件（探测健康本身零记录；非审批性放行不构成高价值锚点）

#### Scenario: 记账故障不改变裁决

- **WHEN** events.db 不可写且策略本应阻断、提醒、豁免或 fail-open
- **THEN** 策略仍执行原裁决，记账仅走既有 fail-loud/fail-safe 错误路径

## ADDED Requirements

### Requirement: change 终态记账（change.abandon）

change 的**放弃/关闭终态** SHALL 记为 `change.abandon` 事件（与 `change.archive` 家族对称）：payload 含 `name`（终态 change 名，正则语义同 change.archive 的名字提取），change 列同名；append-only 不去重（重跑登记记多条，消费侧按名聚合天然幂等）。终态语义：`change.abandon` 表达「该 change 不再推进且不会归档」；同一 change 名下 `change.archive` 与 `change.abandon` 并存时，消费侧 SHALL 以 `change.archive` 为准并输出冲突提示。保留期与 TTL 清扫口径同 `change.archive`（30 天）。

写入方为**专用登记脚本直写账本**（先例：`scripts/harness/test-patrol.sh` 直写 `patrol.check`，session_id 取 PI_SESSION_ID 或脚本名）：脚本参数即 change 名，不依赖 bash 命令配对拦截；脚本不可用/参数非法时零记录（fail-open，不伪造终态）。具体脚本形态由设计阶段定义。

#### Scenario: 放弃终态被记录

- **WHEN** 用户对某不再推进的 change（有事件轨迹但无归档）运行终态登记脚本
- **THEN** 账本新增一条 change.abandon 事件（payload 含 name，change 列同名），后续 `--lifecycle` 与幸存者偏差统计可将该 change 区分为「已放弃」而非「活跃」

#### Scenario: 终态与归档冲突以归档为准

- **WHEN** 某 change 名下同时存在 change.archive 与 change.abandon 事件（误操作或先弃后归）
- **THEN** 消费侧以 change.archive 判定终态为已归档，并输出冲突提示行

#### Scenario: 参数非法零记录

- **WHEN** 终态登记脚本收到的 change 名不存在于 openspec/changes/（或参数为空）
- **THEN** 不记 change.abandon（fail-open 零伪造），脚本以非零退出提示
