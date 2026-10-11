# harness-retro-loop Delta

## ADDED Requirements

### Requirement: 单 change 生命周期体检卡

harness-retro SHALL 提供 `--lifecycle <change>` 模式：以单个 change 为生命周期单位，只读消费事实账本与 openspec change 目录，输出该 change 的流程执行体检卡。体检卡 SHALL 含五段，各段数据缺席时输出降级标注而非静默省略：

1. **流程规范段**：档位绑定链（`mode.set` 序列与 boundChange）、约束注入概要（`constraint.inject` 通道/字节）、pin 接力（`pin.write`/`pin.read` 计数）、编辑归属（`edit.map` 路径数）、门禁执行概要（`gate.check` 按 cmd 的执行/失败计数）、归档锚点（`change.archive` 或 archive 目录名）。无归档锚点时 MUST 标注「未归档」并输出最后活动时间，MUST NOT 假定终态。
2. **UI 工作流段**：ui-design-gate 的卡点链（policy.decision 按 reasonCode 计数）与放行态——放行判定优先消费放行事件，放行事件缺失时按「末次 block 后无新 block」推断并 MUST 标注推断口径。
3. **成本卡**：该 change 归因会话的 `session.rollup` 终值聚合（cost / turns / toolCalls / durationSec / 会话数），MUST 按模型分桶披露（源 rollup payload `models` map）；rollup 覆盖不足或无终值时标注「数据积累中」。
4. **健康度段**：block 催修时距（按 policy 分派解除信号，口径同「效能看板指标」A 组）、回归翻转计数、返工波次（该 change 名下文件的跨 session 编辑次数）。
5. **人类反馈段**：解析 change 目录 `human-feedback.md`（路径规则同声明元数据：未归档读 `openspec/changes/<name>/`，已归档读 `openspec/changes/archive/` 目录剥日期前缀），输出条目计数（产品/流程二分）、归因分布（词表见「human-feedback 反馈建账」Requirement）、未处置条目数（状态为待处理/处理中）。文件缺席或无条目时输出「无反馈记录（或未启用）」降级标注；单条解析失败 MUST 计入「格式异常」单列披露，MUST NOT 中断整段输出。

体检卡 MUST 联合 openspec 侧声明元数据（proposal 头注释 `ui-impact` / `complexity` / `constraint-domains`；未归档读 `openspec/changes/<name>/proposal.md`，已归档读 `openspec/changes/archive/` 目录对应 proposal），并在头部逐项展示；元数据文件缺失时标注「声明缺席」。

体检卡 MUST 保持只读：不产生任何事件写入、不修改账本与 change 目录。输出不进入注入通道（同既有「输出形态不进入注入通道」约束）。

#### Scenario: 体检卡五段齐备

- **WHEN** 以一个已归档、有完整事件轨迹且含 human-feedback.md 的 change 运行 `--lifecycle`
- **THEN** 输出含流程规范、UI 工作流、成本卡、健康度、人类反馈五段，头部含 proposal 声明元数据

#### Scenario: 人类反馈段缺席降级

- **WHEN** 某 change 目录无 human-feedback.md
- **THEN** 人类反馈段输出「无反馈记录（或未启用）」降级标注，其余四段正常输出，不静默省略该段

#### Scenario: 格式异常条目单列披露

- **WHEN** human-feedback.md 存在词表外归因或缺失状态行的条目
- **THEN** 该条计入「格式异常」单列计数，其余可解析条目照常统计，段输出不中断

#### Scenario: UI 工作流放行口径标注

- **WHEN** 某 major UI change 曾被 ui-prototype-missing block 两次，补原型后放行但无放行事件（本能力引入前）
- **THEN** 体检卡 UI 工作流段列出卡点计数，放行态标注「推断（无放行事件）」；引入后同场景有放行事件时直接呈现事件时间

#### Scenario: 未归档 change 不假定终态

- **WHEN** 以一个仍有事件活动、无 change.archive 锚点的 change 运行 `--lifecycle`
- **THEN** 流程规范段标注「未归档」并显示最后活动时间戳，成本卡正常输出已积累数据

#### Scenario: 只读不变更账本

- **WHEN** 运行 `--lifecycle` 前后各对账本做事件计数
- **THEN** 计数不变（零写入），change 目录文件无修改

### Requirement: human-feedback 反馈建账

change 目录 `human-feedback.md` SHALL 作为人类反馈的一手建账文件（可选制品）：记录用户在 change 生命周期内对产物（proposal/design/ui-design/原型/实现/文档）的反馈，作为修订依据与 harness 计量输入。反馈条目 MUST 采用机器可解析的条目级格式契约：

- 条目标题行 `### FB-<N> <日期>【<类型>|<归因>】<标题>`，N 为 change 内自增编号；类型限定 `产品 | 流程`；归因限定封闭词表 `frontend | backend | test-gap | fact-check | process`（frontend=前端设计/交互/视觉；backend=后端业务/参数/数据；test-gap=测试该拦未拦；fact-check=探索或搬运时未核实；process=流程/harness 本身）。
- 条目含状态行，状态限定封闭词表 `待处理 | 处理中 | 已消化 | 已转挂 | 已放弃`。
- 同一问题多因时按根因归因（如既属未核实又属测试盲区时归 fact-check）。

`scripts/harness/feedback.sh` SHALL 提供三个子命令：

- `menu`：打印归因词表与说明（选择题而非自由发挥）。
- `add <change> --layer <归因> --type <产品|流程> "<反馈要点>"`：定位 change 目录（未归档 changes/、已归档拒绝建账提示用归档事实），human-feedback.md 不存在时按模板创建（含头部定位说明）；编号取现有最大 FB-N 自增；套条目模板写入，状态初始化「待处理」；词表外归因/类型非零退出且零写入。
- `verify <change-dir>`：扫描条目状态，存在待处理/处理中条目时输出处置提醒（转新 change 或更新状态）并退出码 1；全部处置或文件缺席时退出码 0。归档对账流程（archive-readiness）SHALL 顺带调用本 verify 并透传其结果语义。

human-feedback.md MUST NOT 写入 events.db（文件随 change 归档永久保存，账本有 TTL；文件为人写给人的修订账本，不要求存储友好格式）。

#### Scenario: add 建账编号自增

- **WHEN** 某 change 的 human-feedback.md 已有 FB-1 至 FB-10，执行 `feedback.sh add <change> --layer frontend --type 产品 "原话"`
- **THEN** 新条目编号 FB-11，标题行含【产品|frontend】，状态行为「待处理」，文件其余内容不变

#### Scenario: 词表外值零写入

- **WHEN** 执行 `feedback.sh add <change> --layer ui --type 产品 "原话"`（ui 不在归因词表）
- **THEN** 非零退出，human-feedback.md 无任何修改

#### Scenario: verify 拦截未处置条目

- **WHEN** 某 change 的 human-feedback.md 存在状态为「待处理」的条目，对其目录执行 `feedback.sh verify`
- **THEN** 输出该条目的处置提醒，退出码 1；全部条目已消化/已转挂/已放弃或文件缺席时退出码 0

#### Scenario: 已归档 change 拒绝建账

- **WHEN** 对归档目录中的 change 执行 `feedback.sh add`
- **THEN** 拒绝写入并提示走归档后修订流程，文件零修改

## MODIFIED Requirements

### Requirement: 效能看板指标

⑦ 效能看板段 SHALL 输出四组指标，MUST 以中位数与 P75 表达分布、MUST NOT 输出加权合成的单一总分；参与效率分布的 session MUST 仅限有 `session.rollup` 终值者，且报告 MUST 显式标注窗口内 rollup 覆盖率（有终值快照的 session 占比），覆盖率不足时对分布指标输出降级标注而非静默使用小样本。

- **A 插件 ROI**：注入负载（每 session 注入字节中位数/P75，源 `constraint.inject`）；注入命中率（被注入域与该 change `edit.map` 实际编辑路径映射域的重合比例）；pin 复用率（`pin.write` 落盘数与后续 `pin.read` 引用数之比）；门禁催修效率（`policy.decision` block/warn 到解除信号的时距中位数；同 reasonCode 在同 change 的复发次数）。**解除信号 MUST 按 policy 语义分派**：quality-gate 的裁决以同 `(session, cmd)` 的 `gate.check` ok=true 为解除；非 gate 类裁决（如 ui-design-gate 依赖补文档/审批解除、quota-gate 依赖换 provider）MUST NOT 用 gate.check 转绿判定，SHALL 以对应的放行/后续非阻断裁决事件为解除信号，信号缺席的样本 MUST 从时距统计中剔除并披露剔除数。
- **B 效率基线**：每 change 与每 session 的 turns、tokens（total 与 output 分列）、cost、durationSec 的中位数/P75（rollup 终值驱动）；子线程 token 占比（`subagent.dispatch`/`complete` tokens 合计 ÷ 含主线程的总量）。
- **B' 每任务成本**：窗口内**已归档 change** 的每任务成本分布（P50/P75/均值）。任务成本 = 该 change 归因会话的 rollup 终值 `cost` 合计——主会话按 rollup 的 change 列直归，子会话经 payload `parentSessionId` 归父后随父会话归因（不递归孙会话）。归档判定用**双锚点**：`change.archive` 事件为主，`openspec/changes/archive/` 目录名日期前缀（`YYYY-MM-DD-<name>`，剥前缀得 change 名）对账兜底；两锚点单侧缺失时以另一侧补齐并输出提示。MUST 按模型分桶（源 rollup payload `models` map；缺失该字段的存量快照整体计入「未分模型」桶）与按 complexity 分桶（源归档 change proposal 头 `<!-- complexity: ... -->`，无声明计「未声明」桶）。MUST 披露：不可归因成本占比（窗口内终值 cost 无法归属任何 change 的部分）、活跃未归档 change 数（对照幸存者偏差）。指标键 `m7.task_cost_*`。09-17（rollup 上线）前历史会话无成本维度，MUST 沿用 rollup 覆盖率降级标注，不得以小样本冒充全量。
- **C 返工信号**：同文件跨 session 编辑波次（`edit.map` 快照序列按 session 去重后的文件出现次数，Top 清单）；归档重试次数（spec-gate `archive-check-failed` block 到成功归档间的重复 block 次数）。
- **D 健康度**：沿用③⑤⑥段既有指标，不在本段重复计算。

#### Scenario: rollup 覆盖率显式标注

- **WHEN** 窗口内多数 session 无 rollup 终值（数据积累初期）
- **THEN** 效率分布指标旁输出覆盖率数值与「数据积累中」降级标注，分布仍仅由有终值的 session 计算

#### Scenario: 注入命中率可复算

- **WHEN** fixture 内某 change 声明域注入了 domain-A 约束，其 edit.map 实际编辑路径全部落在 domain-B
- **THEN** 命中率指标反映 0 命中，且域映射规则（路径→域）在报告口径说明中可查

#### Scenario: 门禁催修时距可复算

- **WHEN** fixture 内某 `(session, cmd)` 先 block 后在 40 分钟后同键转绿
- **THEN** 催修时距中位数计入 40 分钟量级的样本（具体边界由实现定义，报告口径说明可查）

#### Scenario: 非 gate 裁决解除信号按语义分派

- **WHEN** fixture 内某 change 被 ui-design-gate block（ui-prototype-missing），2 分钟后补齐原型文档放行（无任何 gate.check 变化），另有某 quality-gate block 40 分钟后同 (session, cmd) gate.check 转绿
- **THEN** ui-design-gate 样本以放行事件计时距（约 2 分钟），不得按 gate.check 口径计为假慢；quality-gate 样本计入 40 分钟；无解除信号的样本被剔除并披露剔除数

#### Scenario: 返工波次清单

- **WHEN** fixture 内同一文件在 ≥3 个 session 的 edit.map 快照中出现
- **THEN** 返工信号组输出该文件及其跨 session 波次计数

#### Scenario: 禁止综合总分

- **WHEN** 报告渲染效能看板段
- **THEN** 不存在任何把多组指标合成为单一分数的输出行；基线差值也按各指标分别表达

#### Scenario: 子线程 token 占比可复算

- **WHEN** fixture 内 subagent.complete 带有 tokens 值而部分 dispatch 无值
- **THEN** 占比分子仅由有值样本构成并标注样本数，不把缺失值当零累计

#### Scenario: 每任务成本含子会话且可复算

- **WHEN** fixture 内 change X 已归档（archive 目录存在 `YYYY-MM-DD-X`），其主会话 rollup 终值 cost=1.2，另有带 parentSessionId 指向该主会话的子会话终值 cost=0.3
- **THEN** 每任务成本子组中 X 的成本为 1.5，P50/P75/均值由全部归档 change 的成本独立复算

#### Scenario: 模型分桶缺失降级

- **WHEN** fixture 内部分归档 change 的 rollup 快照无 `models` 字段
- **THEN** 该部分成本计入「未分模型」桶并在分桶输出中单列，不并入任何具体模型桶

#### Scenario: 双锚点单侧缺失对账

- **WHEN** fixture 内某 change 存在 archive 目录但无 change.archive 事件（事件被 TTL 清扫或手工移动目录）
- **THEN** 该 change 仍进入每任务成本统计（目录锚点兜底），报告输出锚点不对称提示

#### Scenario: 不可归因成本披露

- **WHEN** fixture 内窗口存在 change 列为空且无 parentSessionId 归属路径的 rollup 终值（如纯问答会话）
- **THEN** 其 cost 汇总为「不可归因成本」单独披露，不计入任何任务成本均值，活跃未归档 change 数单独输出
