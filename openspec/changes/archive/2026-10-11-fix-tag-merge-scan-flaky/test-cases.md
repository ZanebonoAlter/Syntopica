# Test Cases — fix-tag-merge-scan-flaky

## 用户故事

标签合并建议的全量扫描在单元测试环境下可稳定验证（不再因 goroutine 泄漏随机崩掉测试进程），且任何环境下扫描 goroutine 不会因仓库未初始化而 panic 崩掉宿主进程。

## 主链路

| 步 | 动作 | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 扫描进行中（running=true）时调 `StartFullScan()` | 返回 false（拒绝并发启动），不起第二个 goroutine | 函数单测 | `tag_merge_suggest_test.go` `TestStartFullScan_RejectsConcurrent`（重写：持锁模拟 running 态，不真启动） |
| 2 | `repository.Repo == nil` 时 `runFullScan` 执行 | 向 progress channel 发 `Status:"error"` 进度后安全 return；进程不 panic | 函数单测 | `tag_merge_suggest_test.go` 新增 `TestRunFullScan_NilRepo_NoPanic`（同包直调私有函数，先持锁置 progress channel，断言收到 error 后 channel 关闭） |
| 3 | 同包其它测试（如 `TestIsScanRunning_Initially`）在上述测试之后运行 | 状态已被 defer 恢复（running=false），互不污染 | 函数单测 | 既有测试不改动即通过 |

## 变体走查（五组固定清单）

- 输入：~~空串/空白/分隔符/单token/大小写/特殊字符/超长~~——不适用：被测函数无字符串输入（`StartFullScan()` 无参），划除留痕。
- 前置：单元素（一个扫描在跑→拒绝，主链路步 1 覆盖）；「无扫描在跑→首次启动返回 true」路径**声明不单测**（design D2：真起 goroutine 依赖 DB+embedding，属集成/生产路径，强行单测即重新引入本 bug）。
- 时间窗口：~~边界/空窗口/跨窗口/归一化~~——不适用：无窗口语义，划除留痕。
- 幂等：重复执行——测试连跑 10 次结果一致（flaky 根治的直接验收，见 tasks 验收命令）；并发——`StartFullScan` 声称互斥安全（mu 保护），拒绝路径即并发行为的主张面，主链路步 1 覆盖。
- 可用性：~~误输入/空态/错误态/加载态/超长文本/重复提交~~——不适用：无 UI，划除留痕。

## 继承与调整（⓪ 改契约了吗：否）

无 M/R capability，无旧 spec 资产反查需求。既有测试 `TestStartFullScan_RejectsConcurrent` 的断言目标（第二次调用返回 false）保持不变，仅改变置位方式（真启动→模拟态）。
