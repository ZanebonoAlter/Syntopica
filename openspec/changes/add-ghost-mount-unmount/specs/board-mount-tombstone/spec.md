# board-mount-tombstone Spec

## ADDED Requirements

### Requirement: 墓碑表与匹配防复活

系统 SHALL 以 `topic_tag_board_label_tombstones` 表持久化已卸载的 tag-board 挂载对（(topic_tag_id, semantic_board_id) 唯一键），匹配管道（MatchTopicTag）在候选按 MaxBoards 截断**之前** SHALL 过滤掉墓碑命中的 tag-board 对，保证卸载后增量匹配 / rematch-all / backfill 任何路径都不再挂回；删除墓碑行并重算该 tag SHALL 恢复正常匹配。

规则：
- 过滤点在截断前：墓碑行不得占用 MaxBoards 名额挤掉合法候选。
- 墓碑只作用于单 tag-board 对：tag 的其它板挂载、其它 tag 的挂载不受影响。
- 墓碑行不携带 score/reason 语义参与匹配，仅作排除名单。
- composite_hit 候选同样受墓碑过滤（用户卸载的意图高于规则命中）。

#### Scenario: 卸载后 rematch 不复活
- **WHEN** tag T 在板 B 的挂载被卸载（墓碑 (T,B) 存在）AND rematch-all 重算 T
- **THEN** B 不出现在 T 的挂载结果中（墓碑过滤生效），T 在其它板的合法挂载不受影响

#### Scenario: 墓碑行不占 MaxBoards 名额
- **WHEN** tag T 匹配出 4 个候选（含 1 个墓碑命中）AND MaxBoards=3
- **THEN** 墓碑命中者在截断前被过滤，其余候选按分数正常竞争 3 个名额

#### Scenario: 撤销墓碑恢复匹配
- **WHEN** 墓碑 (T,B) 被删除 AND T 触发重算
- **THEN** B 重新参与 T 的正常匹配判定（能否挂上由匹配规则决定，非必然挂回）

### Requirement: 卸载 API

`POST /api/semantic-boards/:id/unmount-tags` SHALL 接受批量 topic_tag_ids，在同一事务内删除对应 `topic_tag_board_labels` 行并写入墓碑；操作 SHALL 幂等：挂载行不存在则跳过该 ID（不报错），墓碑已存在则忽略（不重复写）。非 board 或不存在 → 404。

#### Scenario: 批量卸载同事务
- **WHEN** 请求 body {topic_tag_ids: [a, b, c]} 且三者均挂载于该板
- **THEN** 三行挂载删除、三条墓碑写入同事务提交；部分失败整体回滚

#### Scenario: 幂等重放
- **WHEN** 同请求重放（挂载已删、墓碑已存在）
- **THEN** 返回成功（no-op），不产生重复墓碑或错误

#### Scenario: 版块不存在
- **WHEN** :id 非 board 类型或不存在
- **THEN** 404

### Requirement: 墓碑列表与撤销

`GET /api/semantic-boards/:id/mount-tombstones` SHALL 返回该板墓碑列表（含 tag label、板名、卸载时间，按时间倒序）；`DELETE /api/semantic-boards/:id/mount-tombstones/:topicTagId` SHALL 删除对应墓碑并同步触发该 tag 的重算（恢复匹配）。

#### Scenario: 列表带标签名
- **WHEN** 板存在墓碑 (T,B)
- **THEN** 返回条目含 T 的 label 与卸载时间，可读展示

#### Scenario: 撤销触发重算
- **WHEN** DELETE 墓碑 (T,B) 成功
- **THEN** T 的挂载立即重算（B 重新参与判定），响应含重算结果摘要

#### Scenario: 撤销不存在的墓碑
- **WHEN** (T,B) 墓碑不存在
- **THEN** 404
