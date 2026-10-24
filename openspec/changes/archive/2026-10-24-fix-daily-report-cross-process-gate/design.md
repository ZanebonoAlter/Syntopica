# Design — fix-daily-report-cross-process-gate

## Context

现状（见 proposal Why）：日报生成互斥域闸 `TryAcquireDailyReportGeneration`（`backend-go/internal/topicgraph/service/generation_gate.go`）是进程内 `atomic.Bool`，guard-daily-report-generate 的"单进程部署"前提被临时第二实例破坏，2026-10-23 23:30 双跑实锤（AI 双份 + 5 个 board SaveReport 撞 `idx_board_daily_reports_board_period` 23505）。`SaveReport`（`backend-go/internal/topicgraph/repository/daily_report_repository.go:188`）在事务内先 `First` 查再 `Updates/Create`（TOCTOU）。闸的三个调用面（`DailyReportJob` 闭包 / handler `triggerGenerateDailyReport` / wrapper `TriggerNowWithDate`）通过 seam 变量注入，签名不变。

## Goals / Non-Goals

- **Goals**: ①互斥范围扩展到共享同一 PG 的全部实例；②持闸进程崩溃锁自动释放（零人工清理）；③DB 不可达时单实例行为不劣于现状；④SaveReport 并发写同 board 同日收敛一条、不炸约束；⑤三个触发面代码零改动（seam 不动）。
- **Non-Goals**: 不做多实例部署支持（仍单实例常驻 + 临时实例偶发并存）；不改 409/skip/通知等对外行为；不加配置项；不处理 articles/semantic_labels 的其他 23505（另案）；不做通用分布式锁设施（只做日报闸）。

## Decisions

### D1. 域闸用会话级 pg_try_advisory_lock + 独占连接持有
- **结论**: `TryAcquireDailyReportGeneration` 内部：`db.Conn(ctx)` 取独占连接 → `SELECT pg_try_advisory_lock(<gateKey>)` → 成功则持有该连接并返回 true；失败（或 nil db）则归还连接返回 false。`ReleaseDailyReportGeneration` 在同一连接 `SELECT pg_advisory_unlock(<gateKey>)` 后 `conn.Close()` 归还池。函数签名与 seam 不变，三个调用面零改动。
- **落点**: `backend-go/internal/topicgraph/service/generation_gate.go`
- **取舍**: 拒绝 `pg_advisory_xact_lock` 包住整条生成管线——管线含 30min 主 pass + backfill，长事务持快照阻塞 vacuum；拒绝标志位表（崩溃残留需 TTL 清理，复杂度高于随连接终结自动释放的 advisory lock）；拒绝只靠 SaveReport 原子 upsert 不上锁——闸要防的是 AI 管线双跑烧钱，不只是落库冲突。

### D2. 池化连接的锁漂移用独占连接根治
- **结论**: advisory lock 绑定单条连接；GORM 池化复用连接上执行 lock 后归还会导致"锁留在池里、release 落在别的连接上"的漂移。因此 acquire 成功后该 `*sql.Conn` 被 gate 结构体持有（进程内单 slot），release 必须用同一条连接。gate 内部用一个包级 `sync.Mutex` 保护 acquire/release 状态（防 release 与并发 acquire 竞争）。
- **落点**: `backend-go/internal/topicgraph/service/generation_gate.go`
- **取舍**: 拒绝"每goroutine从池里随手 Conn 取用"——池取到的连接不保证 release 时还是同一条；独占持有是 PG advisory lock 的标准用法。

### D3. DB 不可达回退进程内闸（fail-open 到现状）
- **结论**: `db.Conn` 或 try-lock 查询失败（网络/DB 重启）时：记一条 WARN 日志（含回退原因），退回现有 `atomic.Bool` 路径并照常开始生成；release 时原子闸清零（advisory 侧无持锁则无事可做，unlock 错误吞掉记日志——连接死锁已随连接终结自动释放）。
- **落点**: `backend-go/internal/topicgraph/service/generation_gate.go`；Scenario「数据库不可达时回退进程内互斥」
- **取舍**: 拒绝 fail-closed（DB 抖动直接阻断日报生成）——单用户部署里"生成偶尔双跑"比"生成被 DB 抖动卡死"代价小得多；DB 故障窗口内退化为现状（进程内互斥），正是升级前的行为。

### D4. SaveReport 用事务级分键 advisory lock 串行化，保留既有 upsert 逻辑
- **结论**: `SaveReport` 事务开头执行 `SELECT pg_advisory_xact_lock(<classKey>, <objKey>)`，objKey 由 `boardID` 与 `period_date` 合成（同 board 同日才互斥，跨 board 并行不阻塞）；随后的 `First → Updates/Create + 删旧 threads/sections + 插新` 逻辑保持不变——串行化后 TOCTOU 消失，双写时后到者在锁上等待、醒来后走 Updates 覆盖路径，与 spec「后写覆盖」一致。
- **落点**: `backend-go/internal/topicgraph/repository/daily_report_repository.go:SaveReport`；Scenario「并发落库同板同日不炸唯一约束」
- **取舍**: 拒绝整句 `INSERT ... ON CONFLICT DO UPDATE ... RETURNING id, (xmax=0)` 重写——GORM 对 RETURNING/xmax 表达不直接，且既有的字段级 Updates 清单、threads/sections 级联删除、embedding 分批插入全要重排，diff 面大收益等同；xact 锁随事务提交/回滚自动释放，无泄漏。阻塞等待无显式超时——闸层（D1）已挡双跑，此处为纵深防御，等待场景仅剩"闸故障窗口"，可接受。

### D5. lock key 常量约定（无迁移、无配置）
- **结论**: 会话闸 key = 单 bigint 常量（`fnv64a("syntopica:daily-report:generation-gate")` = 2200772608100431505 = 0x1E8AB706E13C7691，编译期写死并注释来源）；SaveReport xact 锁同样用单 bigint：`boardID<<32 | periodDateEpochDay`（单参形式；实现阶段从双参草案改为单参——双参 advisory lock 的两个参数是 int32，装不下 64 位 fnv 值，单参 bigint 天然适配，且两把锁 key 值域实践不相交：gate key 高 32 位 ≈ 5.1e8 远超现网 boardID 量级 ~2.5e5，注释声明假设）。两者均为代码常量，无 DDL、无配置项。
- **落点**: `backend-go/internal/topicgraph/service/generation_gate.go`、`backend-go/internal/topicgraph/repository/daily_report_repository.go:dailyReportSaveXactKey`
- **取舍**: 拒绝双参 `pg_advisory_xact_lock(classID, objID)` 形式——两参均为 int32，fnv64 classKey 装不下，board+date 也不够分；拒绝运行时随机/重启重算 key（跨实例不稳定致互斥失效）。

### D6. 测试在 testcontainer PG 上验证跨连接语义（complex 档）
- **结论**: 闸测试扩展 `generation_gate_test.go`：①两条独立 `gorm.DB` 连接模拟两实例，A 持闸时 B try-acquire 必失败、A release 后 B 成功；②持闸连接被 `conn.Close()` 模拟崩溃后，B 立即可取（会话锁随连接终结释放）；③坏 DSN 构造 DB 不可达，断言回退进程内闸且生成不阻断。SaveReport 并发测试进 repository 层（`testutil.SetupTestDB` testcontainer 惯例）：两 goroutine 并发 SaveReport 同 board 同日，断言收敛一条记录、无 23505、内容为后写覆盖。完整故事进 test-cases.md（complex 档强制）。
- **落点**: `backend-go/internal/topicgraph/service/generation_gate_test.go`、`backend-go/internal/topicgraph/repository/daily_report_repository_concurrency_test.go`（新增）
- **取舍**: 拒绝 SQLite 内存库——advisory lock 是 PG 特性（standard/backend/testing.md：repository 禁 SQLite）；拒绝真库手测——有副作用红线且不可重复。

## Risks / Trade-offs

- [持闸期间 PG 重启 → 连接死，release 时 unlock 报错] → unlock 失败路径主动 terminate 会话（killSession）：连接已断时 PG 侧锁已随会话终结释放，terminate 失败无害吞掉；未断但 unlock 超时服务端已成功时 terminate 保证锁必释放（review H-1 双向窗口闭合）。
- [探测超时但服务端已获锁 → 锁漂移到归还池的健康会话，永久 already_running] → T4 与 release 失败路径统一走 killSession 自终结会话，池丢弃坏连接，锁随会话终结必释放（review H-1）。
- [持闸期间 PG 重启但连接未被客户端感知（等待窗口醒来后照常跑）→ 会话锁已消失而持闸者不自知，临时第二实例可获闸并发] → advisory lock 固有局限：PG 重启会清空全部会话锁，双方对等回到无锁世界，行为不劣于升级前（进程内互斥兜底）；单用户部署「PG 重启 + 21:00~23:30 临时实例并存」联合概率极低，留痕接受，不做生成前 liveness 自查（侵入 job 闭包收益不成比例）（review M-3）。
- [闸 acquire 多一次 DB roundtrip] → 触发频率低（手动点击/每日一次/skip 重试每分钟一次），可忽略；无内存 fast-path（避免两把锁的顺序问题）。
- [db.Conn 占用一条池连接直至管线结束] → 生成管线本就持有工作连接，默认池大小足够；极端池耗尽表现为 acquire 失败走 D3 回退，不 hang。
- [SaveReport xact 锁阻塞等待无超时] → 仅闸故障窗口可达成；GORM Transaction 未绑定管线 ctx，取消不向上传播到锁等待（review L-1 修正：无自动缓解），依赖对端事务短（秒级提交）+ 等待方依赖外层 30min 生成预算兜底。

## Migration Plan

部署即生效（代码常量，无迁移无配置）。回滚 = revert 代码提交，无状态残留（会话锁随进程退出释放）。

## Open Questions

（无）
