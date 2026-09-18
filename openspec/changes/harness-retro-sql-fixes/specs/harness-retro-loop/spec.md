# harness-retro-loop Delta

## MODIFIED Requirements

### Requirement: 报告分段与可回检指标

报告 SHALL 至少包含以下分段，且每段 MUST 输出该段的指标名与数值（供基线比对复用）：① 门禁失败聚类（按命令、按 domain 包、按失败特征归并的 diag 族）；② 回归翻转（同一 `(session, cmd)` 由绿转红）；③ harness 自身故障（单列，见下条）；④ 软提醒失效（见下条）；⑤ 重复失败热点（同一会话内同命令同 diag 的连续重复，作为死循环信号）；⑥ 注入面健康（`constraint.inject` 的文档命中次数与字节分布，暴露零命中死约束与注入膨胀）。聚类结果 MUST 可限定到单个 change（`change` 列归属）或全局。

失败特征归并 SHALL 将命令/解释器缺失类失败（diag 含 `command not found`）优先归入环境冲突族，该规则 MUST 先于任何按命令名关键词（如 `%lint%`、`%go%`）的族判断执行，使环境噪声不被误计为代码问题族。指标匹配口径（如注入通道 `reason` 枚举、注入域与编辑路径的域名映射）MUST 与账本实际词汇对齐：指标口径子集内（如 flow 文档注入）匹配集合覆盖不到的词汇 SHALL 被显式计数并在报告中呈现（或随词汇演进同步匹配集合），MUST NOT 静默归零——防止口径漂移把真实命中呈现为完全失效；口径子集外的合法词汇 MUST NOT 被误标为未识别。

#### Scenario: 六段齐备且各带指标

- **WHEN** 对含各类事件的 fixture 库运行报告
- **THEN** 输出包含上述六段，每段至少一个「指标名 + 数值」对，且数值可由 fixture 输入独立复算

#### Scenario: 失败特征归并

- **WHEN** fixture 内含多个包的失败 diag（编译失败族与测试失败族）
- **THEN** 聚类按簇归并展示，且不把全部失败平铺成无分组清单

#### Scenario: 命令缺失类失败归入环境族

- **WHEN** fixture 内含 diag 为 `golangci-lint: command not found` 的 `gate.check` 失败
- **THEN** 该失败被归入并发/环境冲突族，MUST NOT 因命令名含 `lint` 被计入 lint 规则族

#### Scenario: 按 change 归属限定

- **WHEN** 指定只统计某个 change
- **THEN** 各段数值仅由 `change` 列归属该 change 的事件参与计算

#### Scenario: 注入通道词汇漂移不产生假零

- **WHEN** 账本内 flow 文档注入（`constraint.inject` 中 path 命中 flow 文档的子集）的 `reason` 含匹配集合未登记的新通道名
- **THEN** 报告对该部分事件显式计数呈现（如「未识别通道 N 条」），注入命中率分母/分子处理可由 fixture 独立复算，MUST NOT 将其静默排除后输出 0 命中；不注入 flow 文档的合法通道（如 `mode-base`/`change-file`/`index`）MUST NOT 被计入未识别
