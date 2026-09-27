# harness-retro-loop Delta

## MODIFIED Requirements

### Requirement: 报告分段与可回检指标

报告 SHALL 至少包含以下分段，且每段 MUST 输出该段的指标名与数值（供基线比对复用）：① 门禁失败聚类（按命令、按 domain 包、按失败特征归并的 diag 族）；② 回归翻转（同一 `(session, cmd)` 由绿转红）；③ harness 自身故障（单列，见下条）；④ 软提醒失效（见下条）；⑤ 重复失败热点（同一会话内同命令同 diag 的连续重复，作为死循环信号）；⑥ 注入面健康（`constraint.inject` 的文档命中次数与字节分布，暴露零命中死约束与注入膨胀）；⑦ 效能看板（插件 ROI、效率基线与返工信号，指标契约见「效能看板指标」Requirement）。聚类结果 MUST 可限定到单个 change（`change` 列归属）或全局。

#### Scenario: 六段齐备且各带指标

- **WHEN** 对含各类事件的 fixture 库运行报告
- **THEN** 输出包含①至⑥故障视角六段，每段至少一个「指标名 + 数值」对，且数值可由 fixture 输入独立复算

#### Scenario: 七段齐备且各带指标

- **WHEN** 对含各类事件（含 session.rollup 与 edit.map）的 fixture 库运行报告
- **THEN** 输出包含上述七段，每段至少一个「指标名 + 数值」对，且数值可由 fixture 输入独立复算

#### Scenario: 失败特征归并

- **WHEN** fixture 内含多个包的失败 diag（编译失败族与测试失败族）
- **THEN** 聚类按簇归并展示，且不把全部失败平铺成无分组清单

#### Scenario: 按 change 归属限定

- **WHEN** 指定只统计某个 change
- **THEN** 各段数值仅由 `change` 列归属该 change 的事件参与计算

### Requirement: 基线快照与回检比对

脚本 SHALL 支持落盘基线快照（含窗口参数、生成时间与各段指标键值——含效能看板段指标键），并在后续运行检测到基线时输出「较基线的变化量」。基线文件缺失或不可解析 MUST 降级为纯报告并给出提示（不失败、不静默丢弃），且 MUST NOT 因基线读写而写 events.db。

#### Scenario: 生成基线与差值

- **WHEN** 先保存基线，随后 fixture 中新增若干同类失败事件并再次运行
- **THEN** 报告对相应指标输出相对基线的正向变化量，且新增事件本身不改变基线文件

#### Scenario: 基线损坏时降级

- **WHEN** 基线路径存在但内容不可解析
- **THEN** 报告照常生成（退出码为 0），并输出「基线不可用、已跳过比对」的中文提示

## ADDED Requirements

### Requirement: 效能看板指标

⑦ 效能看板段 SHALL 输出四组指标，MUST 以中位数与 P75 表达分布、MUST NOT 输出加权合成的单一总分；参与效率分布的 session MUST 仅限有 `session.rollup` 终值者，且报告 MUST 显式标注窗口内 rollup 覆盖率（有终值快照的 session 占比），覆盖率不足时对分布指标输出降级标注而非静默使用小样本。

- **A 插件 ROI**：注入负载（每 session 注入字节中位数/P75，源 `constraint.inject`）；注入命中率（被注入域与该 change `edit.map` 实际编辑路径映射域的重合比例）；pin 复用率（`pin.write` 落盘数与后续 `pin.read` 引用数之比）；门禁催修效率（`policy.decision` block/warn 到同 `(session, cmd)` 转绿的时距中位数；同 reasonCode 在同 change 的复发次数）。
- **B 效率基线**：每 change 与每 session 的 turns、tokens（total 与 output 分列）、cost、durationSec 的中位数/P75（rollup 终值驱动）；子线程 token 占比（`subagent.dispatch`/`complete` tokens 合计 ÷ 含主线程的总量）。
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
