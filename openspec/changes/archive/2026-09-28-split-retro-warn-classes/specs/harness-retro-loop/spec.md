# split-retro-warn-classes — delta spec

## Modified Requirements

### Requirement: 报告分段与可回检指标

报告 SHALL 至少包含以下分段，且每段 MUST 输出该段的指标名与数值（供基线比对复用）：① 门禁失败聚类（按命令、按 domain 包、按失败特征归并的 diag 族）；② 回归翻转（同一 `(session, cmd)` 由绿转红）；③ harness 自身故障（单列，见下条）；④ 软提醒失效（**纠正型**，见下条；情报型 warn 在同段单列计数汇总，不进失效判定）；⑤ 重复失败热点（同一会话内同命令同 diag 的连续重复，作为死循环信号）；⑥ 注入面健康（`constraint.inject` 的文档命中次数与字节分布，暴露零命中死约束与注入膨胀）；⑦ 效能看板（插件 ROI、效率基线与返工信号，指标契约见「效能看板指标」Requirement）。聚类结果 MUST 可限定到单个 change（`change` 列归属）或全局。

失败特征归并 SHALL 将命令/解释器缺失类失败（diag 含 `command not found`）优先归入环境冲突族，该规则 MUST 先于任何按命令名关键词（如 `%lint%`、`%go%`）的族判断执行，使环境噪声不被误计为代码问题族。指标匹配口径（如注入通道 `reason` 枚举、注入域与编辑路径的域名映射）MUST 与账本实际词汇对齐：指标口径子集内（如 flow 文档注入）匹配集合覆盖不到的词汇 SHALL 被显式计数并在报告中呈现（或随词汇演进同步匹配集合），MUST NOT 静默归零——防止口径漂移把真实命中呈现为完全失效；口径子集外的合法词汇 MUST NOT 被误标为未识别。

#### Scenario: 六段齐备且各带指标

- **WHEN** 对含各类事件的 fixture 库运行报告
- **THEN** 输出包含①至⑥故障视角六段，每段至少一个「指标名 + 数值」对，且数值可由 fixture 输入独立复算

#### Scenario: 七段齐备且各带指标

- **WHEN** 对含各类事件（含 session.rollup 与 edit.map）的 fixture 库运行报告
- **THEN** 输出包含上述七段，每段至少一个「指标名 + 数值」对，且数值可由 fixture 输入独立复算

#### Scenario: 失败特征归并

- **WHEN** fixture 内含多个包的失败 diag（编译失败族与测试失败族）
- **THEN** 聚类按簇归并展示，且不把全部失败平铺成无分组清单

#### Scenario: 命令缺失类失败归入环境族

- **WHEN** fixture 内含 diag 为 `command not found` 的失败与按命令名关键词应归其它族的失败
- **THEN** 前者归入环境冲突族，且该归并先于命令名关键词判断执行

#### Scenario: block 与 bypass 不计入故障段

- **WHEN** 窗口内存在 `action=block` 与 `action=bypass` 的 policy.decision
- **THEN** 它们不出现在「harness 自身故障」段（该段只收 fail-open 语义的事件）

#### Scenario: 按 change 归属限定

- **WHEN** 指定只统计某个 change
- **THEN** 各段数值仅由 `change` 列归属该 change 的事件参与计算

#### Scenario: 注入通道词汇漂移不产生假零

- **WHEN** 账本内 flow 文档注入（`constraint.inject` 中 path 命中 flow 文档的子集）的 `reason` 含匹配集合未登记的新通道名
- **THEN** 报告对该部分事件显式计数呈现（如「未识别通道 N 条」），注入命中率分母/分子处理可由 fixture 独立复算，MUST NOT 将其静默排除后输出 0 命中；不注入 flow 文档的合法通道（如 `mode-base`/`change-file`/`index`）MUST NOT 被计入未识别

### Requirement: 软提醒失效判定

报告 SHALL 按 `policy + reasonCode` 统计 `action=warn` 的重复次数并做**语义分类**：

- **纠正型**（期待 agent 行为改变的提醒）：达到或超过可配置阈值时 MUST 列入「软提醒失效」段并给出重复次数；低于阈值 MUST NOT 进入该段。
- **情报型**（归属/状态告知，不期待服从动作；维护一份 reasonCode 白名单，至少含 `concurrent-mixed` / `concurrent-dirty-tree` / `foreign-breakage`，白名单与账本 reasonCode 词汇表同步演进）：SHALL 在 ④ 段内单列分组计数汇总（可作并发频率/成本趋势观察），MUST NOT 进入失效判定名单，MUST NOT 受 `--warn-threshold` 影响。

白名单之外的 reasonCode 默认归纠正型（未知词汇按最强监督口径处理，防新情报型词汇静默漏进失效名单——发现误归类时修白名单而非放宽缺省）。阈值 MUST 可通过参数（或等价配置）调整，缺省值在实现中给定。阈值参数非法（0 或非数字）时 MUST 拒绝执行并非 0 退出，MUST NOT 退化为「全部命中」的静默语义。

#### Scenario: 超阈值提醒纳入

- **WHEN** 某 `policy + reasonCode` 的 warn 计数达到阈值且不在情报型白名单（纠正型）
- **THEN** 该项出现在「软提醒失效」段，并显示重复次数

#### Scenario: 情报型 warn 不进失效名单

- **WHEN** 白名单内 reasonCode（如 `concurrent-mixed`）的 warn 计数达到或超过阈值
- **THEN** 该项不出现在失效判定名单，而在 ④ 段情报型汇总行中按分组计数呈现（不受阈值过滤）

#### Scenario: 混合两类时分开统计

- **WHEN** 同一窗口内同时存在超阈值的情报型与超阈值的纠正型 warn
- **THEN** 失效名单仅含纠正型组；情报型计数与纠正型计数分别可由 fixture 输入独立复算，`soft.warn_total` 等于两类之和

#### Scenario: 阈值边界按「等于即命中」

- **WHEN** 某纠正型组 warn 计数恰好等于阈值
- **THEN** 该项被列入「软提醒失效」段

#### Scenario: 低于阈值不产生噪声

- **WHEN** 另一纠正型 `policy + reasonCode` 的 warn 计数低于阈值
- **THEN** 该项不出现在报告的任何分段中
