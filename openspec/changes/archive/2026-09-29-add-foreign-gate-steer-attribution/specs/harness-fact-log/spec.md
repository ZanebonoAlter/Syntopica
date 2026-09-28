# harness-fact-log Delta

## MODIFIED Requirements

### Requirement: 门禁记账（gate.check）

quality-gate SHALL 对每条实际执行的门禁命令（golangci-lint / go vet / go build / change-scope 判定的 domain go test / pnpm lint）各记一条 `gate.check` 事件，payload 含 cmd、phase、ok、ms；ok=false 时附带截断摘要 diag（≤512B、单行）与**归属判定结果 `ownership`**（`mine | foreign | mixed`；归属信号缺席或判定异常时记 `attribution-degraded`）。

ok=true 事件 SHALL 采样记账（成功运行诊断价值趋零，全量记账造成写放大）：以（session, cmd）为粒度维护连续成功计数——**会话内该命令的首个成功**及失败转绿后的首个成功 MUST 记账（翻转锚点，payload 附翻转标记），其后每 N 次连续成功记 1 条（N 可配置，缺省 5），payload 附采样标记与 N；连续成功段被失败打断后重新起算。ok=false MUST 全量记账，不采样。失败率统计 SHALL 以「失败条数 + 记到的成功条数按锚点 1、采样条 N 加权还原」为分母口径。

同根因短路：golangci-lint 输出含编译失败特征（typechecking error 族）时，quality-gate MAY 跳过本轮必然失败的 go vet 与 domain go test（跳过即未执行）；被跳过的命令 MUST NOT 记 gate.check（未执行零记账，与「纯文档回合零记录」同族语义）。

事件 MUST 绑定 `detectActiveChange` 检测到的活跃 change（无则 null），该检测 MUST 与 constraint-injection 共享同一实现。纯文档/纯对话回合（未触发门禁命令）MUST 零记录。修复回合不单独记账——失败→通过由相邻 gate.check 的 ok 翻转表达。

diag 提取 SHALL 失败特征优先：从命令输出（stdout+stderr 拼接）中优先取首个含失败特征关键词（如 `FAIL`、`error`、`# <pkg>`、`exit` 等关键词表）的行；无任何命中时回退首个非空行。截断规范不变（单行、剥控制字符、≤512 字节）。提取 MUST 确定性（相同输出字节产生相同 diag）。该优先规则仅适用于 gate.check 记账路径；子线程失败白名单的 diag 规范（首个非空错误行）不受影响。

`ownership` 字段向后兼容：本字段引入前的存量事件无该键，消费侧查询 MUST 判空回退现状（视为未判定），MUST NOT 因缺键报错。

#### Scenario: 门禁命令失败记账

- **WHEN** 某回合 golangci-lint exit 1、go vet exit 0（非短路场景）
- **THEN** 产生两条 gate.check：一条 ok=false 含 512B 内截断 diag 与 ownership 字段，一条 ok=true（按采样规则判定是否落库），两条 change 列均绑当前活跃 change

#### Scenario: 失败记账携带归属

- **WHEN** 某门禁命令失败且归属判定为 foreign（失败路径全部位于启动基线）
- **THEN** 该 ok=false 事件的 payload 含 `ownership: "foreign"`；判定为 mine/mixed 时分别记 `mine`/`mixed`

#### Scenario: 归属降级显式标注

- **WHEN** 某门禁命令失败但归属信号缺席（基线快照失败或事实库不可用），保守按本会话失败处理
- **THEN** 该 ok=false 事件的 ownership 记 `attribution-degraded`，且另有归因降级 policy.decision 一条（不双写失败事实本身）

#### Scenario: 存量事件无 ownership 消费兼容

- **WHEN** 消费侧查询窗口内含本字段引入前的存量 ok=false 事件
- **THEN** 读取侧对缺失 ownership 键的事件按未判定处理，查询不报错

#### Scenario: 失败全量记账不采样

- **WHEN** 同一（session, cmd）连续 6 次失败
- **THEN** 产生 6 条 ok=false 事件，每条含 diag 与 ownership，无一条被采样跳过

#### Scenario: 转绿翻转锚点必记

- **WHEN** 某（session, cmd）连续失败 3 次后首次成功，随后又连续成功 5 次（N=5）
- **THEN** 转绿首个成功记 1 条（payload 含翻转标记，计 1），其后再记 1 条采样事件（payload 含采样标记与 N=5）；该连续成功段共 6 次运行落库 2 条

#### Scenario: 分母按采样口径还原

- **WHEN** 某（session, cmd）落库 6 条 ok=false、1 条翻转锚点 ok=true、2 条采样 ok=true（N=5）
- **THEN** 统计侧还原总执行次数 = 6 + 1 + 2×5 = 17，失败率 = 6/17（锚点计 1、采样条按 N 加权）

#### Scenario: 同根因短路未执行不记账

- **WHEN** 某回合 golangci-lint exit 1 且输出含 typechecking error，quality-gate 据此跳过 go vet 与 domain go test
- **THEN** 该回合仅产生 1 条 gate.check（golangci-lint，ok=false 含 diag 与 ownership）；go vet / go test 无事件（未执行零记账）

#### Scenario: 未运行不记账

- **WHEN** 回合只改了 docs/ 下 markdown（门禁放行，未执行任何命令）
- **THEN** 该回合不产生任何 gate.check 事件

#### Scenario: diag 失败特征优先（stdout 噪声行不掩盖真实错误）

- **WHEN** golangci-lint exit 1，stdout 首行为 `0 issues.`，stderr 后续行含编译错误（如 `# syntopica-backend/internal/topicgraph/service`）
- **THEN** ok=false 的 gate.check 事件 diag 含编译错误特征行（`# syntopica-backend/...`），而非 `0 issues.`

#### Scenario: go test 记账不丢 FAIL 行

- **WHEN** `go test -short` exit 1，输出首行为父包正常行 `? pkg [no test files]`，后续行含 `FAIL pkg [build failed]`
- **THEN** 事件 diag 含 FAIL 行（失败特征优先），而非父包正常行

### Requirement: 策略显著裁决统一记账

spec-gate、quota-gate、test-scope-guard SHALL 将显著裁决追加为 `policy.decision` 事件；quality-gate 的 interop 探测短路与**归因降级** SHALL 作为其 policy.decision 场景追加（前者 action=fail-open，reasonCode=interop-down；后者 action=warn，reasonCode 为表达「归属信号缺席/判定异常而保守回退」语义的稳定 kebab-case 词汇，具体值由设计阶段定义并登记词汇表）。payload MUST 含 `policy`、`action`、`reasonCode`：`policy` 限定为稳定扩展标识，`action` 限定为 `block | warn | bypass | fail-open`，`reasonCode` MUST 是稳定、非空、kebab-case 的有界代码；按需附加的 `target` MUST 是不含密钥、完整命令和远端响应正文的有界摘要，`durationMs` 若存在 MUST 为非负数。事件的 change 列 SHALL 优先绑定裁决明确指向的 change，否则使用当前可检测的活跃 change，无法确定时为 null。

dev-process-guard 的孤儿 dev 进程治理事件是上述 payload 形状的**唯一显式豁免**：其 `policy.decision` payload SHALL 为 `decision`（`orphan-killed | orphan-warn`）键 + 进程摘要（`cmd`/`offenders` 等有界字段），MUST NOT 含 `action` 键，且豁免场景**不要求** `policy`/`reasonCode` 键（豁免的是整个键形状而非仅 action→decision 替换；白名单语义为「对用户操作的裁决」，不适用于对进程组的处置；且避免 harness-retro ③段 fail-open 分桶与④段软提醒聚类、催修时距误吸本扩展事件）。该豁免 MUST 登记于事件词汇表文档；除豁免场景外，任何扩展写入 `policy.decision` MUST 遵守 `policy`/`action`/`reasonCode` 形状，词汇表文档与实现 SHALL 保持一致。

普通成功放行 MUST NOT 写 `policy.decision`；quality-gate 其余裁决与 entry-gate 继续使用 `gate.check`，同一裁决 MUST NOT 双写（interop 短路记 policy.decision 后，被跳过的门禁命令 MUST NOT 再逐条记 gate.check；归因降级记账 MUST NOT 与 gate.check 的失败事实构成双写——gate.check 记失败本身与 ownership，policy.decision 只记降级原因）。记账失败 MUST NOT 改变原策略的放行、提醒、阻断或 fail-open 结果。

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

#### Scenario: 正常放行零记录

- **WHEN** spec-gate 的归档检查全部通过、quota-gate 判定额度充足、命令未命中 test-scope-guard，或 interop 探测健康后门禁正常执行
- **THEN** 不产生 policy.decision 事件（探测健康本身零记录）

#### Scenario: 记账故障不改变裁决

- **WHEN** events.db 不可写且策略本应阻断、提醒、豁免或 fail-open
- **THEN** 策略仍执行原裁决，记账仅走既有 fail-loud/fail-safe 错误路径
