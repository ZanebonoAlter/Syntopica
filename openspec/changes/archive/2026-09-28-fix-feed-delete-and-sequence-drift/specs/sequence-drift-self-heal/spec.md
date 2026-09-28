## Purpose

数据库的整型主键自增序列（`<table>_id_seq`）与表内实际数据保持一致：启动期检测「序列落后于表内 `max(id)`」的漂移并自动对齐，使用户的新增操作不会因 `nextval` 分配到已被占用的 id 而失败。漂移的典型来源是「清库脚本 `TRUNCATE ... RESTART IDENTITY` + 带 `-t` 表过滤的数据恢复」——数据回来了、序列停在 1，而 `setval` 不受事务回滚保护，漏了就静默留着。

## ADDED Requirements

### Requirement: 启动期必须检测并修复序列漂移

应用启动阶段 SHALL 扫描数据库里「同时存在 `id` 列与对应 `<table>_id_seq` 序列」的表，对每张表判定是否漂移：当下一次 `nextval` 将分配的值（`is_called` 为真时是 `last_value + 1`，否则是 `last_value`）小于或等于该表的 `COALESCE(MAX(id), 0)` 时，判定为漂移。

对漂移的表 SHALL 执行 `setval(<seq>, COALESCE(MAX(id), 0) + 1, false)`，使下一次 `nextval` 返回 `max(id) + 1`。

检测 SHALL 在调度器与 HTTP 服务启动之前完成（与 `resetStaleStates` 同时点），且 SHALL 幂等：未漂移的表 MUST NOT 产生任何写操作；重复执行 MUST NOT 让序列倒退（当序列值已经大于 `max(id)` 时不得回退）。

#### Scenario: 落后的序列被对齐到 max(id)+1

- **WHEN** 某表 `feeds_id_seq` 的 `last_value` 为 1、`is_called` 为真，而表内 `max(id)` 为 34
- **THEN** 启动后该序列被设置为 35（`is_called=false`），下一次新增订阅源分配到 id=35，不产生主键冲突

#### Scenario: 未漂移的序列不被改动

- **WHEN** 某表序列的下一次分配值大于表内 `max(id)`（正常状态）
- **THEN** 该序列不被 `setval`，不产生任何写入

#### Scenario: 空表与从未调用过的序列不被误修

- **WHEN** 某表的 `MAX(id)` 为 NULL（空表）或序列 `is_called=false` 且 `last_value` 大于 `max(id)`
- **THEN** 该表被判定为未漂移，不做修改

#### Scenario: 重复执行不使序列倒退

- **WHEN** 自愈连续执行两次，第一次已把序列对齐到 `max(id)+1`
- **THEN** 第二次不产生任何 `setval`（序列值不变）

#### Scenario: 五张已知漂移表全部被覆盖

- **WHEN** 在 2026-09-28 事故后的库（`feeds` 1/34、`categories` 1/5、`articles` 4501/133738、`article_topic_tags` 15681/225551、`user_preferences` 1/15920）上启动
- **THEN** 五张表的序列全部被对齐到各自 `max(id)+1`，日志逐表记录漂移前后值

### Requirement: 自愈过程必须可观测且不得阻塞启动

自愈 SHALL 对每张被修复的表记录 `WARN` 级别日志，内容含表名、序列名、修复前的下一次分配值、表内 `max(id)` 与对齐后的目标值，并提示漂移通常来自「清库 + 带表过滤的恢复漏 setval」。

单表检测或修复失败 SHALL 只记录日志（`WARN`/`ERROR`）并继续处理其余表，MUST NOT 阻塞或中止应用启动。探测失败（如目录查询报错）同样 SHALL NOT 阻塞启动。

#### Scenario: 修复留痕

- **WHEN** 启动时修复了 `feeds` 表的漂移
- **THEN** 日志出现该表名、`feeds_id_seq`、漂移前值、`max(id)` 与对齐目标值的 `WARN` 记录

#### Scenario: 单表失败不影响启动

- **WHEN** 某张表的 `setval` 执行报错（如权限不足）
- **THEN** 该错误被记录，其余表继续处理，应用照常完成启动

#### Scenario: 目录查询失败不影响启动

- **WHEN** 列举候选表（`id` 列 + `_id_seq` 序列）的目录查询报错
- **THEN** 记录日志，应用照常启动

### Requirement: 漂移判定逻辑必须可独立测试

漂移判定与对齐目标计算 SHALL 实现为不依赖数据库连接的纯函数（输入 `last_value`、`is_called`、表内 `max(id)`，输出「是否漂移」与「对齐目标值」），以便在无序列语义的单元测试环境（内存 SQLite）覆盖边界；涉及真实序列的端到端行为由 testcontainer PostgreSQL 集成测试覆盖。

#### Scenario: 判定边界被单测覆盖

- **WHEN** 运行序列漂移判定的单元测试
- **THEN** 覆盖「已调用且落后」「已调用且领先」「未调用且落后」「未调用且领先」「空表（max=0）」「恰好等于 max」六类输入组合

#### Scenario: 真实序列行为由集成测试覆盖

- **WHEN** 在 testcontainer PostgreSQL 上运行集成测试
- **THEN** 人为把序列 `setval` 到落后值后触发自愈，插入新行能拿到 `max(id)+1` 且不冲突
