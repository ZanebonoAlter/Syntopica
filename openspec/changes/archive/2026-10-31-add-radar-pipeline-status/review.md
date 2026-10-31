# Review — add-radar-pipeline-status

> 审查对象：本 change 的实现（后端 4 文件 + 前端 8 文件）与三份制品。
> 审查方式：子线程只读审查（`change-review-glm`，绑定 zai-coding-cn/glm-5.3 high）+ 主线程复核 + 修复轮复核。
> 结论：**0 High / 3 Medium / 4 Low → 3 Medium 全部闭环，4 Low 全部处理或显式记录**；复核轮另发现 4 项缺口（G1~G4）亦全部闭环。

## 一、审查轮次

| 轮次 | 范围 | 结论 |
| --- | --- | --- |
| R-1 轮审 | 首轮实现（后端 + 前端） | 0 High / 3 Medium / 4 Low；判定「暂不可进归档验收」，列 3 个阻断项 |
| R-2 轮审 | 修复轮（M1/M2/M3 + L1~L4） | 3 个阻断项全部闭环；复核发现终态计数口径不一致（G1） |
| R-3 轮审 | 口径对齐轮（WS 终态三态 + 通知文案） | 手动路径对齐；复核发现定时路径未对齐（G2/G3） |
| R-4 轮审 | 定时路径对齐 + 闸 T4 连接泄漏根因修复 | 手动与定时两路径口径统一；根因修复并撤掉绕过式测试 stub（G4） |

## 二、审查意见建账

### R-1 2026-10-30【High】生成门槛可经「确认生成」按钮绕过
- **状态**：已修复（确认按钮与 composable 入口双重守卫；补组件与 composable 用例）

### R-2 2026-10-30【High】「今日已出」重生成无警示，design 承诺的缓解未兑现
- **状态**：已修复（按钮文案改「重新生成今日日报」+ 替换警示 + 「确认替换」；不引弹窗；补 3 例组件测试 + spec Scenario + ui-design 状态表）

### R-3 2026-10-30【Medium】两个 spec Scenario 无测试覆盖（重复板块折叠 / 集合字段类型非法）
- **状态**：已修复（补两个后端用例；`test-cases.md` 分支表与 `tasks.md` 映射表同步补行）

### R-4 2026-10-30【Medium】终态计数口径不一致（前后端把「当日无内容」错算成失败）
- **状态**：已修复（后端终态消息补 `failed_count`/`empty_count`，恒等式 `total_saved + failed_count + empty_count == total_boards`；前端直读 + 旧后端两级回落）

### R-5 2026-10-30【Medium】终态通知文案与 WS 终态消息口径不一致（总版面数差一 / 条目数误导）
- **状态**：已修复（新增 `notification.DailyReportTerminal(date, DailyReportCounts)` 三态唯一写入口；旧两参入口保留签名并委托；文案改为「共 N 个版面，K 个版面已产出日报」「共 N 个版面，S 成功 F 失败」「…其中 M 个当日无内容」）

### R-6 2026-10-30【Medium】定时调度路径未对齐（同一动作两套「共 N 个版面」口径）
- **状态**：已修复（定时路径也切三态；`adjudicateDailyReportTerminal` 改调三态写入口；收集阶段早退统一为 `{TotalBoards:1, Failed:1}`）

### R-7 2026-10-30【High】闸探测失败路径连接泄漏（生产侧真 bug；测试侧整包永久挂死）
- **状态**：已修复（T4 分支补 `_ = conn.Close()`，位置与 T5/`release()` 一致、顺序固定「先 kill 后 Close」；删除绕过式测试 stub `stubDailyReportGateInProcess`；加回归护栏 `TestDailyReportGateProbeFailureReturnsConnection`，已反向验证撤掉该行即 FAIL）

### R-8 2026-10-30【Low】WS 终态归属不按 job_id 过滤；onDone 兜底口径不严谨
- **状态**：已修复（`generate()` 捕获 `job_id`，`routeMessage()` 校验匹配才处理；终态计数改为直读三态字段）

### R-9 2026-10-30【Low】board_id 类型非法静默降级为全量生成
- **状态**：已修复（类型错误 400 判定放宽到任意 `*json.UnmarshalTypeError`；spec 条款扩展为「`board_id`/`board_ids` 任一字段类型非法」并补 Scenario。理由：畸形请求触发全站 LLM 管线与本 change 立场不自洽）

### R-10 2026-10-30【Low】「未到/已过生成时刻」文案非响应式
- **状态**：已修复（依赖加入 `lastReconciledAt`，每次对账成功必变；未引入定时器，守 client-poll-budget）

### R-11 2026-10-30【Low】窄屏媒体查询死代码（`grid-template-columns` 对 flex 元素不生效）
- **状态**：已修复（删除无效声明；注释改为「清单单列由 flex 布局保证」）

### R-12 2026-10-30【Low】flow/semantic-board.md 仍称右栏「只读」，与新增手动生成入口不符
- **状态**：已修复（补写右栏「管线状态」块与手动生成入口的语义，含队列门槛与同日替换警示）

### R-13 2026-10-30【Low】layout.md 相对链接错误（`shared/test-design.md` 少一级）
- **状态**：已修复（改为 `../shared/test-design.md`）

### R-14 2026-10-30【Medium】状态块只盯标签队列，会把「定时任务其实不会出日报」说反
- **状态**：已修复（新增 D14 双队列判据：状态块两行并列呈现两个队列，手动生成门槛改为双队列均清空才放行；`/api/poll` 新增 `data.embedding_queue` 段随同一下载、不另建轮询）

### R-15 2026-10-30【Low】`features/**` 不在 Nuxt 自动导入目录，`useEmbeddingQueueProgress` 未定义
- **状态**：已修复（`useRadarPipelineStatus.ts` 补显式 import；组件测试当场报出）

### R-16 2026-10-30【Low】双队列计数段缺失时会被误读为「已清空」
- **状态**：已修复（新增 `embedding-queue-progress:ready` 标记：响应不含该段时呈现「状态未知」并挡住生成，不谎称已清空——R-17）

### R-17 2026-10-30【Low】`client-poll-budget` 用 MODIFIED 会丢掉旧 Scenario
- **状态**：已修复（openspec 校验报出：MODIFIED 块必须含旧 Scenario「一次请求拿到三类计数」；改用 ADDED 新 Requirement「合并端点下发日报双队列计数」，不动原 requirement）

### R-18 2026-10-30【Medium】右栏可视高度被状态块挤占：1440×900 下「今天可能升温」被推出折叠线
- **状态**：已修复（压缩 Embedding 队列缺料说明的措辞，减少一行折行；内容区 `.rd-rail-scroll` 仍独立可滚，该块未被裁掉）

### R-19 2026-10-30【Low】Scenario→测试文件映射表用 `+` 连接多路径，门禁不认
- **状态**：已修复（门禁按**逗号**分隔多路径；已改正并复跑 scenario-trace 全绿）

## 三、must-verify 清单逐条核实（R-1 必查清单，20 条）

| # | 断言 | 结论 | 依据 |
| --- | --- | --- | --- |
| A1 | 集合生成只占一个闸 / 一个 `job_id` / 一次终态广播 / 一条通知 | 通过 | `TestTriggerGenerateDailyReportBoardScopeSharesGateAndTerminal`（gate acquire=1/release=1、done 长 1、通知行数 1） |
| A2 | 闸释放覆盖全部退出路径（成功/部分失败/panic） | 通过 | `defer releaseGenerationGate()` 为函数首条语句，与既有两条生成路径同构；panic 释放有既有单测范式 |
| A3 | 单板失败不中断其余；终态含成功与失败计数 | 通过 | `continue` 不中断循环；终态通知含「2 成功 1 失败」 |
| A4 | 互斥 400 在取闸之前 | 通过 | 测试断言 `gate.acquired()==0`、无管线、无广播、无通知 |
| A5 | 重建窗口守卫未被集合路径绕过 | 通过 | 顺序 互斥 → 窗口 → 闸 不变，三分支均在闸内 |
| A6 | 既有 8 条旧 Scenario 断言全部保留 | 通过 | 测试 diff 零删除行，原断言原样通过 |
| B7 | 失败/未取数态不呈现业务结论 | 通过 | 四行文案均先判 `pollReady`；组件断言 `not.toContain('今日日报未生成')`（loading/error/总览失败三态） |
| B8 | 对账失败保留旧值 | 通过 | 走 `usePollBundle` 既有「保留旧值不回写」路径，有组件断言 |
| B9 | 门槛口径与 `useTagQueueProgress` 一致；`failed` 不误禁用 | 通过 | 直接复用同一 `useState` 源；`failed>0` 仅红色提示、断言不禁用 |
| B10 | 按钮禁用无 DOM 可点漏洞 | 通过 | 入口与确认两处 + composable 入口守卫三重 |
| C11 | 未新建周期性轮询 | 通过 | 无 `setInterval`、无新端点；测试断言「请求次数 = 1」 |
| C12 | `todayKey` 未用 `toISOString()` | 通过 | 本地 `getFullYear/getMonth/getDate`；测试含 UTC±跨天双向回归 |
| C13 | 只用 Layer 2 token | 通过 | 全部语义 token + `color-mix` 派生；无硬编码色、无 `--raw-*` |
| C14 | 勾选 = 参与生成、默认全选、全不选禁用确认 | 通过 | 组件断言 + 实页实测（`ui-verification/report.md` A-8/A-9） |
| C15 | 右栏两段容器未破坏既有断点与 `firstLoadFailed` 语义 | 通过 | `console/` 目录整体 untracked 无法比对 git 历史，改以实页几何补证：1440/1920 无滚动无溢出、375 线性化正确 |
| D16 | spec 行为全部落实、无越权自主决定 | 通过 | 去重与类型 400 两项自主决定已回写 spec；终态三态与通知口径已回写 |
| D17 | design D1~D13 逐条落地 | 通过 | D12/D13 为复核后新增；无被绕过的决策 |
| D18 | 测试断言真实行为、无空断言 | 通过 | 闸计数 / 广播计数 / 通知行 / DOM 文本 / 请求体；前端 77 用例 + 后端目标用例实跑全绿 |
| E19 | 无竞态 / goroutine 泄漏 / 定时器未清理 | 通过 | `boardStatuses` 替换式更新；WS 订阅 `onUnmounted` 清理；无新增定时器 |
| E20 | 无 N+1 / 无界循环 / 内存增长 | 通过 | `dailyReportBoardName` 逐板查询与既有全量路径同款；`dedupeBoardIDs` O(n) |

## 四、遗留观察（不阻断，已在 `ui-verification/report.md` §四 建账）

1. 定时路径 WS 终态消息（`daily_report_complete`）与手动路径（`daily_report_done`）不同型；全仓无前者消费方。
2. 定时路径收集阶段失败时不广播 WS 终态消息（手动路径会广播）。
3. `notification.DailyReportSuccess`/`DailyReportFailedSummary` 已无生产调用方，为待清理死代码。
4. 主 spec `notification-center` 与 `docs/reference/{database,api}` 仍有旧的「保存 42 条条目」示例文案，与新口径漂移。

## 五、review 结论

**通过**。3 个阻断项（R-1~R-3）与 4 个复核新缺口（R-4~R-7）全部闭环，每项均有对应回归用例；must-verify 清单 20 条逐条核实无 High。

**双队列增量轮（R-14~R-19）**：新增 1 个 Medium（R-14 双队列判据缺失，用户实页发现）与 1 个 Medium（R-18 右栏密度折损），加 4 个 Low，共 6 条全部闭环。新增 must-verify 断言 6 条：

| # | 断言 | 结论 | 依据 |
| --- | --- | --- | --- |
| F21 | `/api/poll` 同一下发两个队列计数，两字段独立 | 通过 | `backend-go/internal/admin/handler/poll_test.go`（六个键各自存在） |
| F22 | Embedding 快照的计数映射与日界正确 | 通过 | `backend-go/internal/tagmanagement/handler/embedding_queue_status_test.go`（三例） |
| F23 | 清空判据与调度器同款：failed 是终态不计入活跃量 | 通过 | 同上 `TestEmbeddingQueueStatusSnapshot_DrainPredicate` + 组件「失败任务不阻断生成」用例 |
| F24 | 标签队列清空但 Embedding 积压 → 仍挡住生成 | 通过 | 组件用例 + 实页证据（`ui-verification/report.md` A-4） |
| F25 | Embedding 计数未知 → 挡住生成且不谎称已清空 | 通过 | 组件用例（响应不含该段）+ 实页 A-6 的对照 |
| F26 | 手动生成入口守卫用双队列判据 | 通过 | composable 用例「embedding 有积压时入口不放行：绕过 UI 直接调用也不发请求」 |
