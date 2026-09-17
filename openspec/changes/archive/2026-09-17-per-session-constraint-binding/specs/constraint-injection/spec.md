# constraint-injection Delta

## ADDED Requirements

### Requirement: 会话作用域状态隔离

注入器的**全部会话语义状态** SHALL 按 `sessionId` 隔离存储与读取：档位（`mode`）、绑定 change（`boundChange`）、turn 绑定锁、最近输入窗、JIT 命中集、关键词命中集、pin 注入去重集、稳定层快照/指纹/待发快照。任一时刻，某会话的注入内容（含归因字段与 `declaration` 域选择）SHALL 只由该会话自身的状态决定；他会话的绑定变化 MUST NOT 改变本会话的注入内容或归属。

**无自身状态时的取用规则**（防跨会话借用）：会话有自身状态 → 用自身的；会话为父会话的 fork/子线程（父子关系可证）→ SHALL 显式继承父会话状态并以 `mode.set source=inherit` 记账；其余情况（无状态、无父子关系、无法确定来源）→ SHALL 视为未绑定（仅注入索引），MUST NOT 借用任意他会话的状态。按 `sessionId` 从事实库恢复档位（`resume` / `reload` / `startup`）的既有语义保持不变。

**有界性**：会话条目 SHALL 有数量上限与最近使用淘汰（长跑进程不得无界增长）；extension rebind 时 SHALL 只重置本会话条目。无 `sessionId` 的非真实会话语境（烟测 stub）SHALL 使用单一兜底槽位，行为与隔离前等价。

#### Scenario: 两会话交叉绑定互不污染

- **WHEN** 同一进程内会话 A 绑定 change X、会话 B 绑定 change Y，且两者交替编辑各自 change 目录
- **THEN** A 的注入归属恒为 X（含 X 声明的域），B 的注入归属恒为 Y
- **AND** 任一时刻都不出现「A 的注入带 Y」或「B 的注入带 X」

#### Scenario: 他会话绑定变化不刷新本会话稳定层

- **WHEN** 会话 B 在同一进程内切换绑定（或首次绑定），会话 A 的档位与绑定未变
- **THEN** A 的稳定层快照字节不变、不产生新的稳定层注入记账
- **AND** 动态层指纹不因 B 的变化而变化

#### Scenario: 无自身绑定且无父子关系时不借用他会话

- **WHEN** 会话 C 从未绑定任何 change（无 `mode.set` 记录），而同进程会话 D 已绑定 change Y
- **THEN** C 只注入常驻索引（未激活档），其注入归因 MUST 为未绑定
- **AND** C 的注入内容中 MUST NOT 出现 Y 的档位基础块或 Y 声明的域约束

#### Scenario: 子线程显式继承父会话

- **WHEN** 主会话绑定 change X 后派发子线程（pi-subagents 子线程跑在**独立进程**、独立 sessionId、`session_start` reason=startup；其会话文件 header 带 `parentSession` 指向父会话文件）
- **THEN** 子线程注入带 X 的完整约束块（档位基础块 + X 声明的域）
- **AND** 事实库出现一条该子会话的 `mode.set`，`source=inherit`、`boundChange=X`
- **AND** 子线程活动 MUST NOT 清零或改写主会话的档位与命中集

> 实现口径：跨进程时父会话的内存状态不可见，继承 SHALL 经**事实库回退**（用父会话 id 查其最近一条可恢复 `mode.set`）完成；父子关系不可证（无 `parentSession` 且路径不可解）时 SHALL 退化为未激活，MUST NOT 借用他会话状态。

#### Scenario: 命中集按会话隔离

- **WHEN** 会话 A 编辑代码命中 JIT 文档、或输入命中关键词；同一进程会话 B 未命中
- **THEN** B 的注入内容 MUST NOT 包含 A 命中的 JIT 文档或关键词文档
- **AND** A 命中集「只增不减」的缓存稳定性在本会话内保持不变

#### Scenario: turn 绑定锁按会话隔离

- **WHEN** 会话 A 在本 turn 内已完成一次生效绑定，同 turn 会话 B 发生工具调用
- **THEN** A 的绑定在本 turn 内不再切换（锁定语义只受 A 自身事件影响）

#### Scenario: 会话条目有界淘汰

- **WHEN** 新建会话数超过条目上限
- **THEN** 最久未使用的会话条目被淘汰，且淘汰不影响存活会话的档位、绑定与快照

#### Scenario: 无 sessionId 语境行为等价

- **WHEN** 事件上下文缺少 `sessionId`（烟测 stub / 非真实会话）
- **THEN** 全部状态读写落在单一兜底槽位，行为与隔离前一致（单例语义）
