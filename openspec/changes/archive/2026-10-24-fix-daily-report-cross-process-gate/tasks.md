## 1. 域闸升级（generation_gate.go）

- [x] 1.1 实现 PG advisory lock 域闸：`TryAcquireDailyReportGeneration` 内 `db.Conn` 取独占连接 + `pg_try_advisory_lock(gateKey)`（D1/D2/D5：gate key 常量 fnv64 写死并注释来源；包级 Mutex 保护 acquire/release 竞争；连接不可达/查询失败回退进程内 `atomic.Bool` 并 WARN 记录回退原因，D3）；`ReleaseDailyReportGeneration` 同连接 unlock 后归还，unlock 错误吞掉记日志（连接已断时锁随会话终结自动释放）。函数签名与 seam 变量保持不变，验证：`cd backend-go && go build ./...` 通过且三个调用面（job/handler/wrapper）零改动
- [x] 1.2 gate 单测扩展（generation_gate_test.go）：①两条独立 testcontainer PG 连接模拟两实例互斥（A 持闸 B 必失败，A release B 成功）；②持闸连接 Close 模拟崩溃后 B 立即可取；③坏 DSN 模拟 DB 不可达回退进程内闸不阻断。验证：`cd backend-go && go test ./internal/topicgraph/service -run TestDailyReportGate -v` 全绿

## 2. SaveReport 串行化（daily_report_repository.go）

- [x] 2.1 `SaveReport` 事务开头加 `SELECT pg_advisory_xact_lock(classKey, objKey)`（D4/D5：objKey = boardID<<32 | periodDateEpochDay，同板同日互斥、跨板并行），后续 First→Updates/Create 与级联删除逻辑不动。验证：`cd backend-go && go build ./...` 通过
- [x] 2.2 repository 并发测试（新增 daily_report_repository_concurrency_test.go，testutil.SetupTestDB）：两 goroutine 并发 SaveReport 同 board 同日，断言收敛一条记录、无 23505、内容为后写覆盖（spec Scenario「并发落库同板同日不炸唯一约束」）。验证：`cd backend-go && go test ./internal/topicgraph/repository -run TestSaveReportConcurrent -v` 全绿

## 3. 测试

- [x] 3.1 编写 test-cases.md（complex 档强制）：主链路故事表（闸 acquire→release 全生命周期含跨实例/崩溃/DB 故障三变体）+ 五问句变体走查（幂等组含并发声明）+ 白盒附加（acquire/release 状态机分支表）+ 「继承与调整」表（既有闸测试与 SaveReport 测试逐行处置）。验证：文件存在于 change 目录且含上述四节
- [x] 3.2 影响包回归：`cd backend-go && go test ./internal/topicgraph/service ./internal/topicgraph/repository`（既有闸测试、SaveReport 集成测试不回归）

## 4. Review

- [x] 4.1 派子线程聚焦 review（审查制品 + diff），意见按 High/Medium/Low 处置后 review.md 建账

## 5. 文档

<!-- doc-impact: flow -->

- [x] 5.1 `docs/reference/flow/daily-report.md`：业务约束节补「生成闸为数据库级跨进程互斥（PG advisory lock），临时实例共享同库同样受限；DB 故障回退进程内闸」红线句；变更溯源节补本 change 链接。验证：`grep -n 'advisory' docs/reference/flow/daily-report.md` 有命中
- [x] 5.2 `docs/reference/flow/scheduler.md`：调度行为说明补一句闸升级影响（定时触发与手动触发跨实例互斥），验证：`grep -n '跨实例\|advisory' docs/reference/flow/scheduler.md` 有命中

## 6. 验证

- [x] 6.1 `cd backend-go && golangci-lint run ./internal/topicgraph/...` → 0 issues
- [x] 6.2 `cd backend-go && go vet ./internal/topicgraph/...` → 无输出
- [x] 6.3 `cd backend-go && go build ./...` → 退出码 0
- [x] 6.4 `cd backend-go && go test ./internal/topicgraph/service ./internal/topicgraph/repository` → 全部 PASS（testcontainer 缺失时相关集成测试按既有惯例 skip，不计失败）
- [x] 6.5 双闸行为冒烟：以 test-cases.md 故事 A2（TestDailyReportGateCrossInstance，同库两独立连接池模拟双实例，机械断言互斥与释放）替代人工临时实例冒烟——比手工起停实例更可重复；review.md 已留痕

### Scenario → 测试文件映射

| Scenario | 测试文件 |
| --- | --- |
| 创建日报记录 | backend-go/internal/topicgraph/repository/daily_report_repository_test.go |
| 日报记录唯一性 | backend-go/internal/topicgraph/repository/daily_report_repository_test.go, backend-go/internal/topicgraph/repository/daily_report_repository_concurrency_test.go |
| 并发落库同板同日不炸唯一约束 | backend-go/internal/topicgraph/repository/daily_report_repository_concurrency_test.go |
| 日报关联昨日报告 | backend-go/internal/topicgraph/repository/daily_report_repository_test.go |
| 线程存储在独立表中 | backend-go/internal/topicgraph/repository/daily_report_lane_integration_test.go |
| 触发单板日报生成 | backend-go/internal/topicgraph/handler/daily_report_handler_test.go |
| 触发全板日报生成 | backend-go/internal/topicgraph/handler/daily_report_handler_test.go |
| 生成进行中重复手动触发 | backend-go/internal/topicgraph/handler/daily_report_handler_test.go, backend-go/internal/topicgraph/service/generation_gate_test.go |
| 生成进行中调度到期 | backend-go/internal/topicgraph/service/generation_gate_test.go |
| 跨实例互斥 | backend-go/internal/topicgraph/service/generation_gate_test.go |
| 持闸实例崩溃后闸自动释放 | backend-go/internal/topicgraph/service/generation_gate_test.go |
| 数据库不可达时回退进程内互斥 | backend-go/internal/topicgraph/service/generation_gate_test.go |
| 管线结束后闸释放 | backend-go/internal/topicgraph/service/generation_gate_test.go |
| 手动触发与定时调度互斥不产生重复通知 | backend-go/internal/topicgraph/handler/daily_report_notification_test.go, backend-go/internal/topicgraph/service/generation_gate_test.go |
