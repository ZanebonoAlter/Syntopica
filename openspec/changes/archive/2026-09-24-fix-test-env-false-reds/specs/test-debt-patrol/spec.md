## MODIFIED Requirements

### Requirement: 滚动分片巡检

系统 SHALL 提供巡检脚本（`scripts/harness/test-patrol.sh`）将全量测试拆为有界分片滚动执行：后端按 `backend-go/internal/` domain 目录分片（`go test -short`，跳过 DB 集成，**且显式设置 `-timeout 120s` 超时上界**——防外部锁竞争（如 dev 后端 AutoMigrate 持表锁）把分片拖满 go test 默认 600s 后以无法归账的 panic 收场）；前端按测试文件组分片（`pnpm test:unit <filter...>`，**不带 `--`**——实测 `pnpm test:unit -- <filter>` 会吞掉 filter 静默跑全量，见 `standard/frontend/testing.md`；统一 `--maxWorkers=2`）。分片划分 SHALL 在脚本内静态可枚举（不依赖运行时发现），每片资源占用 SHALL 有上界。

巡检进度 SHALL 持久化（最近完成分片、时间、结果摘要），跨会话可续：连续巡检按「最久未巡的分片优先」推进，一轮全部完成后重新开始。支持一次跑一片（默认）与一次跑多片（显式参数）。

#### Scenario: 单分片巡检落账

- **WHEN** 执行 `bash scripts/harness/test-patrol.sh`（无参数，跑一片）
- **THEN** 选取最久未巡的分片执行，输出通过/失败摘要，失败测试自动登记台账，进度与时间落账

#### Scenario: 资源红线——低并发执行

- **WHEN** 前端分片执行
- **THEN** 以 `--maxWorkers=2` 运行；脚本 SHALL 检测其它 pi 会话高负载或并发 build/浏览器自动化时提示延后（提醒，不强制）

#### Scenario: 后端分片锁竞争时有界失败

- **WHEN** 后端分片巡检期间 dev 后端持有库表锁（如正在跑数据增强任务），分片内含依赖库结构初始化的测试
- **THEN** 该分片在 120 秒内以非零退出码结束（超时快速失败），巡检按既有环境错误路径记账，不拖满 go test 默认 600 秒

#### Scenario: 分片内测试全绿

- **WHEN** 某分片全部通过
- **THEN** 不产生任何台账记录变更，仅更新该分片的巡检进度
