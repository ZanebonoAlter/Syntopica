## MODIFIED Requirements

（无——效能看板 requirement 的 D 组描述与实现存在历史漂移（实现为测试欠账巡检、spec 写健康度沿用），收敛留给独立 change，本 change 不夹带）

## ADDED Requirements

### Requirement: 生命周期面汇总

全局报告 ⑦ 效能看板 SHALL 输出 E 子组「生命周期面」，把单 change 体检卡第五段（人类反馈）与全局 C 组（返工波次）的数据在全局窗口上乘起来巡视；该子组 MUST 只读消费 change 目录文件与 events.db，MUST NOT 产生任何写入。

change 清单口径：MUST 覆盖全部活跃 change（`openspec/changes/` 非 archive 子项）与窗口内归档 change（`archive/` 目录名日期前缀落在统计窗口内）。反馈文件解析 MUST 复用 lifecycle 模式既有词表与降级语义：归因五类封闭词表（frontend/backend/test-gap/fact-check/process）、文件缺席不算异常、格式异常条目单列披露不炸段。

子组 MUST 输出三项：

- **反馈覆盖**：有反馈账 change 数 / 无账 change 数 / 未处置条目数（待处理+处理中+状态不明）
- **归因分布聚合**：窗口内全部反馈条目按归因五类的跨 change 聚合计数
- **漏采信号**：窗口内编辑波次 ≥3 session（复用 C 组波次口径）且反馈 0 条的 change 名单——多轮返工却零反馈建账即采集纪律信号

#### Scenario: 反馈覆盖计数

- **WHEN** 窗口内 5 个 change 中 2 个目录含 human-feedback.md（共 3 条，其中 1 条待处理）
- **THEN** E 子组输出有账 2 / 无账 3 / 未处置 1

#### Scenario: 归因分布聚合

- **WHEN** 窗口内反馈条目归因为 backend×2、test-gap×1
- **THEN** E 子组输出聚合分布 backend 2 / test-gap 1（其余类 0 或省略，口径可查）

#### Scenario: 漏采信号

- **WHEN** 活跃 change X 在窗口内有 4 个 session 的 edit.map 波次且其目录无 human-feedback.md
- **THEN** E 子组的漏采信号名单包含 X；仅有 ≤2 波次或已有反馈条目的 change 不进名单

#### Scenario: 反馈文件格式异常不炸段

- **WHEN** 某 human-feedback.md 存在词表外归因或缺失状态行的条目
- **THEN** 该条目计入「格式异常」单列披露，子组其余输出不受影响

#### Scenario: 零反馈数据降级

- **WHEN** 窗口内所有 change 均无 human-feedback.md
- **THEN** E 子组输出有账 0 与「无反馈记录」降级标注，不静默省略子组

#### Scenario: 全程只读

- **WHEN** 全局报告执行
- **THEN** events.db 以 readonly 打开，change 目录仅 stat/读取，无任何文件或库写入
