# gate-concurrency-control Specification

## ADDED Requirements

### Requirement: 门禁命令全局互斥

quality-gate 在执行任一门禁命令（lint / vet / build / 域测试 / eslint）之前，MUST 先原子抢占仓库级门禁锁（`​.pi/harness/gate.lock`）；锁内容 MUST 至少含持有会话标识与时间戳供事件考古。同一时刻同一仓库 MUST 至多一个会话在执行门禁命令。

抢占失败（锁已被其他会话持有且未超时）时，本轮门禁 MUST 整体跳过：所有门禁命令不执行（未执行零 gate.check 记账），失败粘性集合 MUST 保留不变（下回合锁空闲时照常重跑催修），并 MUST 显式记一条策略跳过记账（policy=quality-gate、reasonCode=gate-lock-held），MUST NOT 静默。

锁文件带 TTL：持有时长超过 TTL（须大于单条门禁命令超时上限，默认 180s）的锁 MUST 视为持有方进程已崩溃的残留锁，允许直接覆盖抢占，MUST NOT 因残留锁永久阻塞门禁。门禁命令全部结束（含异常路径）后 MUST 释放锁。

#### Scenario: 抢到锁正常执行门禁

- **WHEN** turn_end 触发门禁且锁空闲
- **THEN** 写入锁文件后照常执行门禁命令，命令全部结束后删除锁文件；门禁行为与互斥机制引入前一致

#### Scenario: 锁被并发会话持有时本轮跳过

- **WHEN** turn_end 触发门禁，但锁文件存在、未超 TTL 且持有者为其他会话
- **THEN** 本轮所有门禁命令不执行、零 gate.check 记账，不向会话注入失败 steer；记一条 skip（gate-lock-held）策略记账；既有粘性失败集合原样保留

#### Scenario: 残留锁超 TTL 后可抢占

- **WHEN** 锁文件存在但持有时长已超过 TTL（如持有方进程崩溃）
- **THEN** 视为 stale，覆盖写入新锁后照常执行门禁，不永久阻塞

#### Scenario: 门禁命令异常时锁仍释放

- **WHEN** 门禁命令执行中抛异常或超时
- **THEN** 锁文件仍被释放（finally 语义），不因异常残留锁

### Requirement: 单会话门禁资源上限

quality-gate 触发的 go 工具链命令 MUST 限制并行度上限为 2：go 编译类命令（vet / build / test）经运行环境变量等效 `GOMAXPROCS=2` 执行，golangci-lint 经自身并发参数等效 `--concurrency=2` 执行。后端 vet 与 build MUST 串行执行，MUST NOT 在单会话内并行发起。前端 eslint（单进程）行为不变。

#### Scenario: go 命令以限核参数执行

- **WHEN** 门禁执行 go vet / go build / 域测试
- **THEN** 命令以等效 GOMAXPROCS=2 的并行度运行，单会话门禁 CPU 峰值不超过约 2 核

#### Scenario: vet 与 build 串行

- **WHEN** 后端门禁 lint 通过后继续执行 vet 与 build
- **THEN** 两者先后执行，命令超时预算各自独立不变

### Requirement: 混合归属失败降级

门禁命令失败且失败路径集合同时含本会话归属文件与其他会话归属文件（混合归属：P∩mine≠∅ ∧ P∩foreign≠∅）时，MUST 按混合归属降级处理：该失败 MUST NOT 标 [回归] 分级，MUST 以 [并发] 前缀提示，提示中 MUST 分别列出他人会话与本会话的文件路径（各截断至合理条数），并 MUST 说明「可能非本会话所致，归档前仍需全绿」。该失败 MUST 照常计入失败粘性集合（本会话部分仍需修复，回合末复检不断），MUST 记一条混合归因记账（reasonCode=concurrent-mixed）。

纯外部归属（P∩mine=∅ ∧ P⊆foreign）维持既有 [外部] 语义不变；纯本会话失败维持既有 [回归]/[中间态] 分级与粘性语义不变。归因信号缺席（无绑定 change / 库不可用 / 解析异常）时维持既有保守策略（视同本会话失败）。归档前全绿的硬要求 MUST NOT 因本降级放松。

#### Scenario: 混合归属失败标 [并发] 不标 [回归]

- **WHEN** go build 失败，失败路径同时含本会话编辑过的文件与其他会话归属的文件
- **THEN** steer 提示以 [并发] 前缀呈现，列出双方文件路径与「可能非本会话所致」说明，不出现 [回归] 必须修措辞

#### Scenario: 混合归属照进粘性重跑

- **WHEN** 上一回合产生混合归属失败
- **THEN** 该命令计入粘性失败集合，下回合门禁照常重跑该命令直至转绿

#### Scenario: 纯归属两极不变

- **WHEN** 失败路径完全属于其他会话，或完全属于本会话
- **THEN** 分别维持既有 [外部]（不计粘性、不催修）与 [回归]/[中间态]（计粘性、催修）语义，与本需求引入前一致

#### Scenario: 归因信号缺席时保守回退

- **WHEN** mine/foreign 集合构建所需信号缺席（无绑定 change、账本库不可用或解析异常）
- **THEN** 失败视同本会话失败按既有分级处理，不因信号缺席降级或漏催修
