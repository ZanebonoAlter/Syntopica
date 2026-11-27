# dev-process-guard 规格增量

## MODIFIED Requirements

### Requirement: turn_end 软提醒

扩展 SHALL 在 `turn_end` 检查当前存活的泄漏类进程，**仅当存在本会话窗口内制造的泄漏**（`session_shutdown` 将被自动清理的那部分）时，以 steer 消息提醒：列出 pid/pgid/年龄/命令行，并给出处理建议（经 `scripts/dev/start-dev.sh` 接管或手动清理）；**纯窗口外历史遗留 MUST NOT 触发 steer、MUST NOT 记 `orphan-warn`**（跨会话残留由定时回收与人工处置承担，不在本会话收件箱里出现）。提醒事件记 `policy.decision`（decision=`orphan-warn`），payload MUST 含 `inWindow`（本会话制造的进程数）与 `historicalOnly`（本轮被静默的历史残留数）以便回检。同一进程组（pgid）在一个扩展进程生命周期内 MUST 至多提醒一次（跨会话去重）。同一会话至多一条 steer。零泄漏或仅有历史残留时 MUST 零输出；仅有历史残留时 MUST 零记账。

#### Scenario: 只提醒本会话制造的泄漏

- **WHEN** turn_end 检出本会话窗口内启动的泄漏进程（会被 session_shutdown 清理）
- **THEN** 发送一条 steer 提醒并记一条 `orphan-warn`（payload 含 `inWindow` > 0）

#### Scenario: 纯历史残留静默

- **WHEN** turn_end 只检出窗口外历史遗留进程，本会话未制造任何泄漏
- **THEN** MUST NOT 发送 steer、MUST NOT 写 events.db

#### Scenario: 同进程组跨会话只提醒一次

- **WHEN** 两个并发会话都检出了同一 pgid 的泄漏进程
- **THEN** 该 pgid 只触发一次提醒（先到者提醒，后者静默）

#### Scenario: 首次提醒后静默

- **WHEN** 同一会话已提醒过一次，后续 turn_end 进程仍在
- **THEN** 不再重复提醒（每会话至多一次）

#### Scenario: 零泄漏零输出

- **WHEN** 系统中不存在任何泄漏类进程
- **THEN** turn_end 无 steer 消息、无 events.db 写入
