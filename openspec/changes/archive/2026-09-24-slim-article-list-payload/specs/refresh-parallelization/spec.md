## ADDED Requirements

### Requirement: 定时刷新后列表更新为按需重取

前端自动刷新（`useAutoRefresh` 触发的 feed 刷新）完成后 MUST 只重取当前视图所需的列表数据（当前筛选条件下的当前页）；MUST NOT 以 `per_page: 10000` 或任何等价的全量参数重拉文章列表，MUST NOT 用整表替换的方式丢失选中行与滚动位置。

#### Scenario: 单 feed 刷新完成

- **WHEN** 某 feed 的定时刷新完成
- **THEN** 前端 SHALL 以当前筛选 + 当前页参数重取列表，请求的 `per_page` SHALL ≤ 100
- **AND** 列表选中行与滚动位置 SHALL 保持不变

#### Scenario: 不发起全量重拉

- **WHEN** 任意刷新流程完成
- **THEN** SHALL NOT 出现 `per_page=10000`（或任何 > 100）的文章列表请求

### Requirement: 定时刷新单调度器错峰触发

多个 feed 的自动刷新 MUST 由单一调度器按各自 due 时间依次触发，同一时刻至多一个 feed 刷新在执行；MUST NOT 为每个 feed 建立同相位、同周期的独立定时器（避免同一分钟批量触发）。

#### Scenario: 多 feed 同周期

- **WHEN** 23 个 feed 的刷新间隔同为 60 分钟且上次刷新时刻相近
- **THEN** 刷新 SHALL 依次串行执行（同一分钟至多 1 个刷新），MUST NOT 出现 20 个以上刷新同分钟并发

#### Scenario: 刷新过程中用户切换视图

- **WHEN** 调度器正在刷新某个 feed，用户切换筛选或分页
- **THEN** 刷新 SHALL 继续执行完毕，切换后的列表请求 SHALL 不被刷新阻塞（各自独立发起）

#### Scenario: 刷新失败不终止调度

- **WHEN** 某个 feed 的刷新请求失败
- **THEN** 调度器 SHALL 记录该失败并继续按 due 时间调度后续 feed，MUST NOT 停止全部定时刷新
