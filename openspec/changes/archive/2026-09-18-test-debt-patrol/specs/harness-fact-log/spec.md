# harness-fact-log Delta

## ADDED Requirements

### Requirement: 巡检事件记账（patrol.check）

巡检脚本每执行一个分片 SHALL 向事实库追加一条 `patrol.check` 事件：`shard`（分片名）、`ok`（布尔）、`ms`（耗时）、`fails`（失败测试标识数组，成功为空数组）。事件归因 change 列为空（巡检是仓库级活动，不归属单个 change）。保留期按既有 gate.check 同级（30 天）；台账自身持久化欠账，事件仅作巡检流水。

#### Scenario: 分片巡检完成落事件

- **WHEN** 巡检脚本完成一个分片
- **THEN** 事实库新增一条 `patrol.check`，含分片名、结果、耗时与失败清单

#### Scenario: 台账登记不双写事件

- **WHEN** 巡检发现失败并登记台账
- **THEN** 台账记录与 `patrol.check` 事件各司其职：事件记流水，台账记欠账生命周期；事件删除（TTL）不影响台账
