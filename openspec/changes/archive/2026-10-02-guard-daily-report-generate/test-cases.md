# test-cases — guard-daily-report-generate

> 单元 = 「日报生成管线全局互斥」一个用户故事；复杂度 simple。锚点：`standard/shared/test-design.md`。
> 契约锚：change specs/daily-report-system/spec.md「日报生成 API — 异步触发」（MODIFIED）。

## ⓪ 继承与调整（改契约了：MODIFIED）

执行 `bash scripts/harness/test-assets.sh daily-report-system` 反查旧资产后逐行处置：

| 旧 Scenario | 处置 | 旧测试 | 动作 |
| --- | --- | --- | --- |
| 触发单板日报生成 | 继承（行为不变） | 无既有直接断言（offline-catchup 前异步受理路径无专门用例） | 新增 `TestTriggerGenerateDailyReportReleasesGateAfterRun`（单板受理 200 processing）承接 |
| 触发全板日报生成 | 继承（行为不变） | 无既有直接断言（同上） | `AllowsInsideWindow`（全板 200 processing）+ `ReleasesGateAfterRun` 受理断言承接 |
| 超窗拒绝（design D6，既有） | 继承 | `TestTriggerGenerateDailyReportRetentionGuardRejectsOutsideWindow` | 保持绿；另补 `RebuildWindowCheckedBeforeGate` 证顺序（超窗 400 优先于 409，不碰闸） |
| 窗口内日期正常重建（含边界） | 继承 | `AllowsInsideWindow`（4 子用例） | 保持绿；受理后异步占真闸会引起连续子测试相互 409——装恒真闸 seam 隔离（互斥行为另由闸专门用例覆盖） |
| 停机缺档自动补齐 / 队列未清顺延 / 只补缺不重建 | 继承（闸不改变补档语义） | `job_daily_report_test.go` | 保持绿（接闸在闭包开头，不影响等待/补档逻辑） |

## 主链路节拍表

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 空闲时手动触发全板生成 | 触发全板日报生成 | 200 `{job_id, status:"processing"}`，闸被占 | handler | `daily_report_handler_test.go` |
| 2 | 管线在跑，再次手动触发（单板/全板各一） | 生成进行中重复手动触发 | 409 `reason:"already_running"`，无第二条管线（generate 桩未再被调），无进度/终态广播 | handler | 同上 |
| 3 | 手动管线在跑，调度到期 | 生成进行中调度到期 | job 返回 `skipped:"already_running"`，生成桩未被调，无终态通知 | 单测 | `job_daily_report_test.go` |
| 4 | 管线结束（成功/panic 两态）后再触发 | 管线结束后闸释放 | 均能正常受理（闸已释放，无残留 409） | 单测+handler | gate + handler 测试 |

## 变体走查

- 输入变体：**不适用**（无文本解析面；date/board_id 校验为既有行为，不在本 change 契约内）
- 前置变体：闸占用（主链路 2/3 已覆盖）✓｜闸从未占过（主链路 1 空闲首触）✓｜闸泄漏态——由 panic 释放用例证伪（主链路 4 panic 态）✓
- 时间窗口变体：**不适用**（闸无窗口概念；rebuild-window 校验属既有契约，顺序上先于闸）
- 幂等/并发变体：重复执行=409（主链路 2）✓｜并发双触发——100 goroutine 并发 TryAcquire 恰好一胜（CAS 原子性，1.1 用例）✓｜部分失败重试——管线失败返回后闸释放、可重新触发（主链路 4 成功态）✓
- 可用性（UI）：前端零改动，409 文案走既有 `success:false` 错误 toast（`LaneDynamicsPanel.test.ts` 已有该路径 mock）；**人工**：实现后在 TagsPage 触发面板观察重复点击提示「正在生成中」

## 层选择说明

闸=CAS 纯逻辑→函数单测；409 拒绝=HTTP 行为→handler 层（httptest）；调度 skip→job 函数单测（`generateAndSaveReport` seam 已有桩机制）。非前端交互 change，opencli 豁免。

## 效果核对

不适用——互斥行为可由断言直接验证，无 LLM/数据覆盖率依赖。

## 白盒附加（simple 档，仅记边界）

- CAS 竞争：并发 TryAcquire 恰一胜（1.1）
- defer 顺序：job 闭包与 handler goroutine 的 Release 必须 defer（panic 路径也执行）；job 内 recover 不得先于 Release 吞掉 defer
- 闸校验顺序：rebuild-window 409 优先于 already_running（不占闸即拒绝超窗日期）
