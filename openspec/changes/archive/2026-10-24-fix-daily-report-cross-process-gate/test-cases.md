# Test Cases — fix-daily-report-cross-process-gate

> complex 档用例先行文档。断言判据主线程定。层选择遵循 standard/shared/test-design.md：advisory lock 为 PG 特性 → 一律 testcontainer PG（repository 禁 SQLite）。

## 一、主链路故事表

### 故事 A：跨实例生成互斥全生命周期（来源 Requirement「日报生成 API — 异步触发」）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| A1 | 实例 A（gateA@dbA）TryAcquire | 触发单板日报生成（受理路径） | true；进程内闸占用；PG 会话锁持有 | service 单测（testcontainer） | generation_gate_test.go `TestDailyReportGateCrossInstance` |
| A2 | 实例 B（gateB@dbB，同库独立连接）TryAcquire | 跨实例互斥 | false（被 A 的数据库锁拒绝） | 同上 | 同上 |
| A3 | A Release（管线正常结束） | 管线结束后闸释放 | unlock 成功、连接归还；B TryAcquire 变 true | 同上 | 同上 |
| A4 | B Release 后 C（gateA 复用）TryAcquire | 管线结束后闸释放 | true（无残留锁） | 同上 | 同上 |
| A5 | A TryAcquire 后持闸连接被 Close（模拟进程崩溃） | 持闸实例崩溃后闸自动释放 | PG 会话锁随连接终结释放；gateB TryAcquire 立即 true（无需人工清锁） | 同上 | `TestDailyReportGateCrashRelease` |
| A6 | dbSource 指向不可达地址（127.0.0.1:1）时 TryAcquire | 数据库不可达时回退进程内互斥 | return true（不阻断生成）；WARN 日志含回退原因；Release 安全 no-op | service 单测（坏 DSN，无需容器） | `TestDailyReportGateDBFallback` |
| A7 | 同进程内闸被持有时第二次 TryAcquire | 生成进行中重复手动触发 | false（进程内 CAS 先挡，行为与升级前一致） | service 纯单测（无 DB） | 既有 `TestDailyReportGenerationGate` 4 子测试（回退路径复跑） |

### 故事 B：SaveReport 并发落库收敛（来源 Requirement「日报数据模型」）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| B1 | goroutine-1 SaveReport(board 5, 2026-05-25, title=T1) 与 goroutine-2 SaveReport(board 5, 2026-05-25, title=T2) 并发 | 并发落库同板同日不炸唯一约束 | 两者均无 23505 错误；表收敛一条记录；title 为后提交者（后写覆盖） | repository 单测（testcontainer） | daily_report_repository_concurrency_test.go `TestSaveReportConcurrentSameBoardDate` |
| B2 | 并发 SaveReport(board 5, D) 与 SaveReport(board 6, D) | （同上，分键边界） | 互不阻塞均成功（跨板并行，xact 锁分键不冲突） | 同上 | 同上 `TestSaveReportConcurrentDifferentBoards` |
| B3 | 顺序重复 SaveReport 同板同日（既有行为回归） | 日报记录唯一性 | 更新已有记录不重建（升级前行为不变） | 既有 lane integration 等回归 | 3.2 影响包回归 |

## 二、⑤问句变体走查

- **输入**：闸无外部输入（gateKey 为编译期常量）——划除；SaveReport objKey 输入 boardID/periodDate 均为内部字段（NormalizeReportDate 既有归一化），边界见白盒表 V1/V2。
- **前置**：闸空闲/已持闸（A1/A2 覆盖）；SaveReport 前置"记录已存在/不存在"（B1/B3 覆盖）；**越界引用**：gateConn 在 Release 后置 nil 防重复 unlock（白盒 R1）。
- **时间窗口**：闸无窗口语义——划除；SaveReport 的 periodDate 为纯日期值（无窗口），归一化既有——划除留痕。
- **幂等**：重复 Release（无闸时调用）→ 安全 no-op（白盒 R1）；部分失败重试 → try_lock 查询失败走 A6 回退、下次触发重新探测；**并发**（声称线程安全）→ A7 的 100 并发单赢家既有用例 + gateMu 保护 acquire/release 竞争（白盒）。
- **可用性（UI）**：后端 change 无 UI——划除。

## 三、⓪继承与调整表（capability: daily-report-system）

| 旧 Scenario | 处置 | 旧测试 | 动作 |
|---|---|---|---|
| 创建日报记录 / 日报记录唯一性 / 日报关联昨日报告 / 线程存储在独立表中 | 保留（行为不变） | repository 既有 SaveReport 集成测试（lane/centroid 等） | 不动，3.2 回归兜底 |
| 日报记录唯一性（顺序语义） | 扩展为含并发（新 Scenario「并发落库同板同日不炸唯一约束」） | 无并发测试 | 新增 B1/B2 |
| 触发单板/全板日报生成 | 保留 | handler 层既有测试 | 不动（端点行为零改动） |
| 生成进行中重复手动触发 / 生成进行中调度到期 / 管线结束后闸释放 / 手动触发与定时调度互斥不产生重复通知 | 语义扩展（互斥范围进程内→跨实例），进程内路径行为不变 | service/generation_gate_test.go 4 子测试 + handler 既有 409 测试 | 保留复跑（新实现下走无 DB 回退路径，断言仍成立）；新增 A1-A6 覆盖扩展语义 |
| （新增）跨实例互斥 / 持闸实例崩溃后闸自动释放 / 数据库不可达时回退进程内互斥 | 新增 | 无 | A2/A5/A6 |

## 四、白盒附加（acquire/release 状态机分支表）

TryAcquireDailyReportGeneration 分支：

| # | 分支 | 结果 |
|---|---|---|
| T1 | 进程内 atomic CAS 失败 | return false（进程内已持闸） |
| T2 | dbSource() == nil（测试/DB 未初始化） | 进程内模式 return true |
| T3 | sqlDB.Conn(ctx) 失败 | WARN + return true（D3 回退） |
| T4 | pg_try_advisory_lock 查询 error | killSession 终结会话（防服务端已获锁而客户端超时——锁漂移到归池连接变永久 already_running，review H-1）+ WARN + return true（D3 回退） |
| T5 | try_lock = false（他实例持有） | conn.Close + atomic 回滚 false + return false |
| T6 | try_lock = true | 记录 gateConn + return true |

ReleaseDailyReportGeneration 分支：

| # | 分支 | 结果 |
|---|---|---|
| R1 | gateConn == nil（进程内模式/已释放） | 仅清 atomic（幂等） |
| R2 | pg_advisory_unlock error | WARN + killSession 终结会话（unlock 超时但服务端已成功同理，review H-1）+ conn.Close + 清 atomic |
| R3 | unlock ok | conn.Close（归还池）+ 清 atomic |

边界值清单：

| # | 边界 | 判据 |
|---|---|---|
| V1 | gateKey 常量 | fnv64a("syntopica:daily-report:generation-gate")=2200772608100431505（0x1E8AB706E13C7691）；SaveReport xact 锁为单参 bigint：boardID<<32\|epochDay（无独立 classKey 常量——实现阶段从双参草案改单参，双参两槽均为 int32 装不下 64 位 fnv，见 design D5 修订）；注释标注来源 |
| V2 | objKey = boardID<<32 \| epochDay | boardID < 2^31（现网最大 246030）防溢出，注释声明假设；epochDay = periodDate.Unix()/86400（NormalizeReportDate 后稳定） |
| V3 | Conn 探测超时 | 5s（context.WithTimeout）：DB 抖动不长时间阻塞触发面 |
| V4 | 不适用划除 | 输入串/大小写/超长输入变体——闸与 objKey 无自由文本输入，划除留痕 |
