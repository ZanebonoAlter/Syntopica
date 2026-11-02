# board-health-checkup Delta

## ADDED Requirements

### Requirement: 体检快照附属卸载候选

体检轮次 SHALL 在快照中写入 `unmount_candidates` 附属产物（对标 uncaptured_candidates 模式）：`direction_mismatch=true` 且同 tag 在其它板存在非 mismatch 挂载（双挂口径——摘掉次优挂载后真家保住，tag 不裸奔）的挂载清单，按 mounted cosine 升序（最跑题优先）取 top N（默认 20，配置键 `board_health_unmount_candidates_limit`），条目含 topic_tag_id、tag label、mounted cosine、全库最优板名与 cosine。单挂形态（无其它干净挂载）MUST NOT 进入候选（裸奔风险，归板构成治理另行处理）。体检轮次 MUST 保持纯读边界：候选只是快照附属，不落任何 suggestion pending 行、不写匹配结果表（红线 20）；卸载执行是用户显式操作（board-mount-tombstone 的卸载 API）。

#### Scenario: 双挂 mismatch 进入候选
- **WHEN** tag T 挂载板 B（direction_mismatch=true）且 T 在板 A 有非 mismatch 挂载
- **THEN** T-B 条目进入快照 unmount_candidates（含 A 的名称与 cosine）

#### Scenario: 单挂 mismatch 不进候选
- **WHEN** tag T 仅挂载板 B（direction_mismatch=true，无其它干净挂载）
- **THEN** 不进入候选（摘后裸奔，另行处理）

#### Scenario: 候选数受配置上限
- **WHEN** 双挂 mismatch 超 N 条 AND `board_health_unmount_candidates_limit`=N
- **THEN** 快照只含最跑题的 N 条（mounted cosine 最低优先）

#### Scenario: 体检轮次零副作用
- **WHEN** 体检轮次执行
- **THEN** 除快照本体（含新附属列）外零写入：board_upgrade_suggestions 零新增、topic_tag_board_labels 零变更、墓碑表零变更
