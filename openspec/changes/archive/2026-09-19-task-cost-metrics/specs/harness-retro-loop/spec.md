# harness-retro-loop Delta

## MODIFIED Requirements

### Requirement: 效能看板指标

⑦ 效能看板段 SHALL 输出四组指标，MUST 以中位数与 P75 表达分布、MUST NOT 输出加权合成的单一总分；参与效率分布的 session MUST 仅限有 `session.rollup` 终值者，且报告 MUST 显式标注窗口内 rollup 覆盖率（有终值快照的 session 占比），覆盖率不足时对分布指标输出降级标注而非静默使用小样本。

- **A 插件 ROI**：注入负载（每 session 注入字节中位数/P75，源 `constraint.inject`）；注入命中率（被注入域与该 change `edit.map` 实际编辑路径映射域的重合比例）；pin 复用率（`pin.write` 落盘数与后续 `pin.read` 引用数之比）；门禁催修效率（`policy.decision` block/warn 到同 `(session, cmd)` 转绿的时距中位数；同 reasonCode 在同 change 的复发次数）。
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
