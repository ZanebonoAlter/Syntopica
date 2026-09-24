## MODIFIED Requirements

### Requirement: 归档前并发检查

spec-gate 在归档命令拦截时 SHALL 增加并发检查：git 脏文件中存在归属其他 active change 且未 commit 的文件时，输出 warn 级提示（steer 消息通道，含文件清单与归属 change 名），SHALL NOT block（归属为启发式聚合，误 block 会卡死正常归档）。归属地图冷启动期（事件词汇上线前的存量脏文件无归属记录）或全部脏文件无归属时，SHALL 跳过该检查不产生 warn。

warn 输出 SHALL 为**边沿触发**（同会话同指纹至多一条）：指纹取自本次文件清单**排序去重后**的内容摘要，清单未变的重试尝试 MUST NOT 重复投递 warn；文件清单变化时视同首见重新输出；上一次 warn 过、本次检查转干净（exit 0）时 SHALL 输出一行「已转净」收尾并清除指纹态。指纹态 MUST 与会话生命周期绑定（会话边界清零、session compact 后重发一次），MUST NOT 跨会话复用。`policy.decision(action=warn, reasonCode=concurrent-dirty-tree)` 记账与展示走同一边沿（同指纹会话内至多一条）。

#### Scenario: 树上有其他 change 归属文件时 warn

- **WHEN** 归档 change `foo` 时 git 脏文件含归属 active change `bar` 的 `backend-go/x.go`
- **THEN** spec-gate 输出 warn 提示（含文件与 `bar` 名），归档命令本身不被阻断

#### Scenario: 冷启动跳过

- **WHEN** 脏文件均无归属记录（edit.map 上线前的存量文件）
- **THEN** spec-gate 跳过并发检查，零额外输出

#### Scenario: 同指纹重试静默

- **WHEN** 同一会话内对同一 change 连续多次归档尝试，脏文件清单未变（排序去重后指纹相同）
- **THEN** 仅首次尝试输出 warn 与一条 `concurrent-dirty-tree` 记账，后续尝试零 steer 消息、零 warn 记账（block 判定与检查①-④' 结果不受影响）

#### Scenario: 清单变化重新输出

- **WHEN** 前一次尝试已 warn，本次尝试脏文件清单变化（新增/减少归属他 change 的文件）
- **THEN** 本次按新指纹重新输出完整 warn 并再记一条

#### Scenario: 转净收尾一行

- **WHEN** 前一次尝试已 warn，本次检查 exit 0（树上无归属其他 change 的未 commit 文件）
- **THEN** 输出一行「已转净」收尾提示并清除指纹态，后续尝试恢复首见语义

#### Scenario: 新会话首见重新提醒

- **WHEN** 归档 warn 在上一会话已投递，新会话再次归档且清单未变
- **THEN** 新会话首次尝试仍输出 warn（会话边界清零，MUST NOT 跨会话去重）
