# harness-retro-loop Specification

## Purpose
从 harness 事实账本（`.pi/harness/events.db`）生成**失败聚类报告**的能力契约：报告以何种口径统计（采样加权还原的分母）、如何把「harness 自身故障」与「agent 犯错」分开、如何用基线快照把一条 harness 规则上线前后的同类事件计数变化变成可回检的准 A/B 指标，以及窗口/空库/缺库等降级语义。目标是让「改 harness 规则」从只有加、没有回检，变成有数据、有基线、可复盘的闭环。

## Requirements

### Requirement: 只读消费事实账本

报告脚本 MUST 以只读方式打开 `.pi/harness/events.db`，MUST NOT 对该库发起任何写操作（不建表、不改 PRAGMA、不写入事件、不改既有行），也 MUST NOT 依赖任何新表或物化列。开库前 MUST 校验库身份（`application_id` 为本应用魔数 `0x53594E54`）：校验不通过或库不存在、库文件损坏时 MUST fail-loud（中文原因 + 非 0 退出码），MUST NOT 静默输出空报告或重建库。

#### Scenario: 只读打开不产生任何写入

- **WHEN** 对一份 fixture 库运行报告脚本
- **THEN** 库文件与同目录 sidecar 文件的字节内容不变，且库内事件行数不变

#### Scenario: 库不存在时 fail-loud

- **WHEN** 目标路径不存在 events.db
- **THEN** 输出中文原因（含目标路径）并以非 0 退出码结束，不输出看似正常的报告

#### Scenario: 拒绝非本应用的库

- **WHEN** 目标路径指向一个 `application_id` 非 `0x53594E54` 的 SQLite 文件
- **THEN** 拒绝读取并以非 0 退出码结束，不执行任何写操作

### Requirement: 采样加权还原的分母口径

统计门禁失败率时 MUST 按 `gate.check` 的采样记账协议还原「实际执行次数」作为分母：`ok=false` 条计 1；`ok=true` 且无采样标记（会话内首成功 / 失败转绿锚点）计 1；`ok=true` 且带采样标记的条按 payload 的 `n` 加权计数（`n` 缺失时回退该命令记录的缺省 N）。报告 MUST 展示还原后的分母与失败率，且 MUST 在报告中标注该口径来源，使读者不会把「落库条数」误当分母。

`n` 存在但取值非法（非数字或 ≤ 0）时 MUST 保守按 1 计并输出降级警告（MUST NOT 放大分母从而掩盖失败）；单行 payload 不可解析或缺少判定所需字段时 MUST 跳过该行并输出一次性降级警告，MUST NOT 计入任何分段。

#### Scenario: 按锚点与采样权重还原分母

- **WHEN** 某 `(session, cmd)` 在窗口内有 6 条 `ok=false`、1 条翻转锚点 `ok=true`、2 条采样 `ok=true`（`n=5`）
- **THEN** 报告还原总执行次数为 17、失败数为 6，并给出失败率 6/17（而非按 9 条落库记录计的 6/9）

#### Scenario: 采样条缺 n 时回退缺省权重

- **WHEN** 一条 `ok=true` 采样事件缺少 `n` 字段
- **THEN** 该条按缺省权重 N 参与还原，报告不因缺字段而崩溃或丢弃该条

#### Scenario: 非法采样权重保守计 1

- **WHEN** 一条 `ok=true` 采样事件的 `n` 为 0 / 负数 / 非数字
- **THEN** 该条按 1 计入执行次数（而非按缺省 N 放大分母），并输出降级警告

#### Scenario: 不可解析行跳过且告警

- **WHEN** 窗口内存在 payload 非法 JSON 或缺少判定字段的事件行
- **THEN** 该行不计入任何分段，报告输出一次性降级警告，其余指标照常计算

#### Scenario: 报告显式标注口径

- **WHEN** 报告输出门禁失败段
- **THEN** 该段同时输出还原后的分母说明（成功侧采样权重已还原），使失败率可被独立复算

### Requirement: 报告分段与可回检指标

报告 SHALL 至少包含以下分段，且每段 MUST 输出该段的指标名与数值（供基线比对复用）：① 门禁失败聚类（按命令、按 domain 包、按失败特征归并的 diag 族）；② 回归翻转（同一 `(session, cmd)` 由绿转红）；③ harness 自身故障（单列，见下条）；④ 软提醒失效（见下条）；⑤ 重复失败热点（同一会话内同命令同 diag 的连续重复，作为死循环信号）；⑥ 注入面健康（`constraint.inject` 的文档命中次数与字节分布，暴露零命中死约束与注入膨胀）。聚类结果 MUST 可限定到单个 change（`change` 列归属）或全局。

#### Scenario: 六段齐备且各带指标

- **WHEN** 对含各类事件的 fixture 库运行报告
- **THEN** 输出包含上述六段，每段至少一个「指标名 + 数值」对，且数值可由 fixture 输入独立复算

#### Scenario: 失败特征归并

- **WHEN** fixture 内含多个包的失败 diag（编译失败族与测试失败族）
- **THEN** 聚类按簇归并展示，且不把全部失败平铺成无分组清单

#### Scenario: 按 change 归属限定

- **WHEN** 指定只统计某个 change
- **THEN** 各段数值仅由 `change` 列归属该 change 的事件参与计算

### Requirement: harness 自身故障与 agent 失败分离

`policy.decision` 中 `action=fail-open` 的裁决（如 interop 链路故障、额度查询失败、门禁自身检查异常）MUST 归入「harness 自身故障」段单列，MUST NOT 计入 agent 失败数的分母；报告 MUST 明确标注该排除项，使读者不会把门禁自身不可用读成代码质量下降。

#### Scenario: fail-open 不计入 agent 失败

- **WHEN** 窗口内只有 3 条 `action=fail-open` 的 policy.decision、没有任何门禁失败
- **THEN** 「harness 自身故障」段为 3，门禁失败段为 0，且报告标注 fail-open 已被排除

#### Scenario: 阻断类裁决不混入故障段

- **WHEN** 窗口内存在 `action=block` 与 `action=bypass` 的 policy.decision
- **THEN** 它们不出现在「harness 自身故障」段（该段只收 fail-open 语义的事件）

### Requirement: 软提醒失效判定

报告 SHALL 按 `policy + reasonCode` 统计 `action=warn` 的重复次数，达到或超过可配置阈值时 MUST 列入「软提醒失效」段并给出重复次数；低于阈值的事件 MUST NOT 进入该段。阈值 MUST 可通过参数（或等价配置）调整，缺省值在实现中给定。阈值参数非法（0 或非数字）时 MUST 拒绝执行并非 0 退出，MUST NOT 退化为「全部命中」的静默语义。

#### Scenario: 超阈值提醒纳入

- **WHEN** 某 `policy + reasonCode` 的 warn 计数达到阈值
- **THEN** 该项出现在「软提醒失效」段，并显示重复次数

#### Scenario: 阈值边界按「等于即命中」

- **WHEN** 某组 warn 计数恰好等于阈值
- **THEN** 该项被列入「软提醒失效」段

#### Scenario: 低于阈值不产生噪声

- **WHEN** 另一 `policy + reasonCode` 的 warn 计数低于阈值
- **THEN** 该项不出现在报告的任何分段中

### Requirement: 基线快照与回检比对

脚本 SHALL 支持落盘基线快照（含窗口参数、生成时间与各段指标键值），并在后续运行检测到基线时输出「较基线的变化量」。基线文件缺失或不可解析 MUST 降级为纯报告并给出提示（不失败、不静默丢弃），且 MUST NOT 因基线读写而写 events.db。

#### Scenario: 生成基线与差值

- **WHEN** 先保存基线，随后 fixture 中新增若干同类失败事件并再次运行
- **THEN** 报告对相应指标输出相对基线的正向变化量，且新增事件本身不改变基线文件

#### Scenario: 基线损坏时降级

- **WHEN** 基线路径存在但内容不可解析
- **THEN** 报告照常生成（退出码为 0），并输出「基线不可用、已跳过比对」的中文提示

### Requirement: 窗口与 TTL 安全提示

报告窗口 MUST 受参数控制，且 MUST 按 UTC 计算窗口边界（左闭）。当请求窗口长度**严格大于**任一被消费事件类型的保留期（`gate.check` / `policy.decision` / `edit.map` / `constraint.inject` / `pin.read` 等为 30 天，`session.start` 为 90 天）时，报告 MUST 输出「该窗口可能已被 TTL 清扫，差值可能失真」的显式提示，MUST NOT 静默截断窗口或把「数据被清扫」呈现为「指标已改善」。窗口长度等于保留期时 MUST NOT 提示。

#### Scenario: 超期窗口给出提示

- **WHEN** 请求的窗口长度大于被消费事件类型的最短保留期
- **THEN** 输出窗口超期提示，且报告仍按请求窗口计算

#### Scenario: 常规窗口无提示

- **WHEN** 请求的窗口长度在保留期以内
- **THEN** 不出现窗口超期提示

### Requirement: 空库与「无发现」语义

报告成功生成时 MUST 以 0 退出（发现大量失败不构成错误）。当窗口内没有任何被消费事件时，报告 MUST 明确输出「窗口内无数据」提示，MUST NOT 输出伪造的 0% 失败率或空白段落。

#### Scenario: 空库不产出伪指标

- **WHEN** 对窗口内无任何事件的库运行报告
- **THEN** 退出码为 0，输出窗口内无数据的中文提示，且不出现失败率数值

#### Scenario: 有失败但退出码仍为 0

- **WHEN** 窗口内存在大量门禁失败事件
- **THEN** 退出码为 0（报告职责是呈现，不是判定）

### Requirement: 输出形态不进入注入通道

报告 MUST 仅以命令输出形态提供：脚本 MUST NOT 写任何 system prompt、MUST NOT 通过注入通道配置文件或事件写入影响后续会话上下文（避免时变内容破坏 prompt 前缀缓存），且 MUST NOT 新增任何 events.db 事件。

#### Scenario: 运行后账本零新增

- **WHEN** 运行报告脚本（含保存基线）
- **THEN** events.db 事件行数与运行前一致

#### Scenario: 不触碰注入配置

- **WHEN** 运行报告脚本
- **THEN** `.pi/constraint-injection.json` 与 `.pi/extensions/` 下文件均未被修改

### Requirement: 复盘方法论随脚本交付

本能力 SHALL 随附一份 agent 可加载的方法论文档（skill），内容至少覆盖：何时跑（归档后 / 定期 / 怀疑某规则无效时）、命令用法、报告各段的解读方式、**改进项产出判据**（每条改进项 MUST 绑定一个可回检指标与观察窗口；仅出现在单个 session 的偶发事件 MUST NOT 直接升格为规则）、以及与本仓库既有事实库考古能力（`harness-facts`）的分工。文档 MUST 在仓库既有技能注册点被引用，禁止成为孤立文档。

#### Scenario: 方法论文档存在且被注册引用

- **WHEN** 检查 skill 文档存在性与注册引用
- **THEN** 文档存在，且至少一处仓库级注册点（如根 `AGENTS.md`）引用该 skill

#### Scenario: 判据可与报告指标对齐

- **WHEN** 读者按方法论产出一条改进项
- **THEN** 该改进项能落到报告某个分段的指标名上（可回检），而不是无法度量的主观判断
