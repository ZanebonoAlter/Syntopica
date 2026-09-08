# Purpose

并发 change 协调机制：让同一共享工作树上的多个并发 change 互相知晓存在与文件归属，消除归档验证的重复执行、重复探索与交叉污染。核心手段是事实库聚合的文件归属地图（change → 编辑文件集合）+ 拉模式态势查询 + commit 与归档解耦。明确不走 system prompt 注入通道（时变数据会破坏前缀缓存）。

## ADDED Requirements

### Requirement: 文件归属地图落库

harness 层 SHALL 将"归属某 change 的会话累计编辑过的仓库文件路径"聚合为 `edit.map` 事件落事实库：quality-gate 在 turn_end 检出会话内增量路径时，若会话已绑定 change（mode.set 的 boundChange），SHALL 将本回合新增/变化路径合并进该 change 的归属集合（事件 change 列为绑定的 change，payload 含累计路径集合）。无绑定 change 的会话路径 SHALL NOT 计入任何 change 归属。同一文件被两个及以上 change 的归属集合同时包含时，concurrency-status 输出中 MUST 将该文件标记为冲突文件。

#### Scenario: 编辑路径按绑定 change 聚合

- **WHEN** 会话绑定 change `foo` 且本回合编辑 `backend-go/internal/dataenrichment/a.go`，另一会话绑定 change `bar` 编辑 `backend-go/internal/admin/b.go`
- **THEN** 事实库中 change `foo` 与 `bar` 的归属集合分别含各自文件，互不串扰

#### Scenario: 同文件双 change 触碰标记冲突

- **WHEN** change `foo` 与 change `bar` 的归属集合均含 `backend-go/internal/app/router.go`
- **THEN** concurrency-status 输出中该文件带冲突标记

#### Scenario: 无档会话不计入归属

- **WHEN** 未绑定 change 的会话编辑 `front/app/a.vue`
- **THEN** 该路径不出现在任何 change 的归属集合中

### Requirement: 并发态势拉取单一入口

`scripts/concurrency-status.sh` SHALL 作为并发态势的唯一拉取入口，只读查询事实库与 openspec 状态，输出三段：①活跃 change 清单（名称、绑定 session、最近活动时间）；②当前 git 脏文件 × 归属地图对照（归属本 change / 归属其他 active change / 无归属三类分列，冲突文件带标记）；③近期（默认 6 小时）gate.check 验证流水摘要（change、命令、结果、时刻）。态势数据 MUST NOT 进入 system prompt 注入通道（时变内容破坏前缀缓存），仅在 Agent 主动查询、spec-gate 检查、编排流程步骤三类挂点以命令输出形态出现。脚本 MUST 只读（不写事实库、不写 git）。

#### Scenario: 归档验证前拉取态势

- **WHEN** Agent 在 change `foo` 归档验证前运行 `bash scripts/concurrency-status.sh foo`
- **THEN** 输出列出树上脏文件的归属（含归属其他 active change 的文件清单）与近 6 小时验证流水，Agent 据此判断是否需要先拆 commit

#### Scenario: 脚本只读安全

- **WHEN** 运行 concurrency-status.sh 时事实库被其他进程写入
- **THEN** 脚本以只读连接查询（WAL 并发读），无任何写入或锁阻塞失败

### Requirement: 归档前并发检查

spec-gate 在归档命令拦截时 SHALL 增加并发检查：git 脏文件中存在归属其他 active change 且未 commit 的文件时，输出 warn 级提示（steer 消息通道，含文件清单与归属 change 名），SHALL NOT block（归属为启发式聚合，误 block 会卡死正常归档）。归属地图冷启动期（事件词汇上线前的存量脏文件无归属记录）或全部脏文件无归属时，SHALL 跳过该检查不产生 warn。

#### Scenario: 树上有其他 change 归属文件时 warn

- **WHEN** 归档 change `foo` 时 git 脏文件含归属 active change `bar` 的 `backend-go/x.go`
- **THEN** spec-gate 输出 warn 提示（含文件与 `bar` 名），归档命令本身不被阻断

#### Scenario: 冷启动跳过

- **WHEN** 脏文件均无归属记录（edit.map 上线前的存量文件）
- **THEN** spec-gate 跳过并发检查，零额外输出

### Requirement: commit 与归档解耦约定

change 主体实现完成且满足主体 commit 资格（quality-gate turn_end 增量门禁全绿 + 影响包测试通过）时，Agent SHALL 先做主体 commit（按归属地图 `git add` 本 change 文件），MUST NOT 等归档才 commit。归档流程 SHALL 另起归档 commit（spec 同步 + flow 溯源收尾）。主体 commit 后挂在 active 状态等待实战验证的 change（如 harness 类）SHALL 保持磁盘文件不变（commit 不改变工作树内容，运行时行为不受影响）；实战验证发现问题的修复以 fixup commit 追加。主线程派发子线程前 SHALL 运行一次并发态势拉取作为编排步骤。

#### Scenario: 门禁绿即主体 commit

- **WHEN** change `foo` 主体实现完成、增量门禁全绿且影响包 `go test` 通过，树上另有 change `bar` 的脏文件
- **THEN** Agent 按归属地图仅 add `foo` 的文件做主体 commit，`bar` 的文件留在工作树

#### Scenario: harness change 挂验证不受 commit 影响

- **WHEN** harness 类 change `h` 主体 commit 后仍在 active 状态等待其他 change 执行中实战验证
- **THEN** `h` 的磁盘文件内容与 commit 前一致，pi 运行时加载行为不变；验证发现问题后修复以 fixup commit 追加且可 revert
