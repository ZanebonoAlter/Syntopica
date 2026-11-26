<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

> **档位说明**：本 change 声明 `ui-impact: minor`——在 L0 雷达总览页**既有右栏结构内**增加一个常驻信息区块与单步操作，不新增页面、弹窗、导航入口，不改变信息架构与布局模式（沿用 `workspace` + 已登记的固定视口栅格例外）。按 syntopica-ui schema，minor 档不制作可视原型，approval=`not-required`，验收映射以组件测试指名道姓 + `ui-verification/report.md` 建账。

## 入口与入口变更

- **入口**：L0 今日雷达总览页 `/tags`（`front/app/pages/tags/index.vue` → `BoardRadarView`），阅读右栏底部。
- **入口变更**：无既有入口被替换或移除。`/tags` 路由、`/tags/:boardId` 系列路由、`/tags` 之外的任何页面均不受影响；顶栏 `TagQueueProgressChip`（挂在 `AppHeaderView`，仅 `FeedLayoutShell` 链路上渲染）保持原样，本 change 不动它。
- **新增能力**：右栏底部新增管线状态块 → 手动生成日报入口（状态块内就地展开板块清单，不跳页、不弹窗）。

## 受影响状态

状态块覆盖 loading / empty / error / success 四态中受影响的全部组合，逐个交代：

| 态 | 触发条件 | 呈现 |
| --- | --- | --- |
| **loading** | 状态块挂载后对账从未成功过 | **各行全部**呈现「读取中」（两个队列行 / 日报行 / 生成窗口行 / 当日状态行）；**不呈现**「今日日报未生成」或任何等价业务结论（R-17：初始零值不是结论）。生成窗口行一并置为「读取中」的理由：`21:00/23:30` 是**兜底常量**而非本站真实配置，对账未成功即展示等于把默认值当事实（与 R-17 同源风险），故未取到真值前不呈现这两个时刻 |
| **loading（局部）** | 对账成功、总览载荷取数中 | 两个队列行与生成窗口行照常；日报行与当日状态行呈现「读取中」 |
| **empty** | 对账成功、总览取数成功，但无任何板块有日报 | 「最新日报：暂无」；日报行呈现「今日未生成」（这是**成功空集**下的真实业务结论，与 loading 态区分） |
| **empty（队列空闲）** | 对账成功、两个队列均 `pending + processing = 0`、`completed_today = 0` | 两个队列行各呈现「队列空闲」；生成按钮可用 |
| **error（对账失败）** | 曾成功过、后续对账失败 | **保留上一次的值**并标注「更新于 HH:mm」；不清零、不转错误态（client-poll-budget 契约） |
| **error（总览失败）** | 总览载荷取数失败 | 两个队列行照常显示；日报行呈现「读取失败」；**不判定**「今日未生成」 |
| **error（生成请求失败）** | 手动生成被拒（409 `already_running`）或参数非法（400） | 按钮恢复可用，状态块内联显示失败原因文案 |
| **error（Embedding 队列计数未知）** | 对账响应不含 `embedding_queue` 段（旧后端） | Embedding 队列行呈现「状态未知」（**不谎称已清空**）；生成按钮**禁用** + 缺料说明「Embedding 队列状态未知：无法确认是否已清空」 |
| **success（队列有积压）** | 任一队列 `pending + processing > 0` | 对应队列行显示待处理数 + 各自的今日完成进度条（标签队列分母按**文章**、Embedding 队列分母按**标签**）；生成按钮**禁用** + **分队列**缺料说明 |
| **success（清单展开期间队列复堵）** | 清单已展开、已勾选板块，期间任一队列 `pending + processing` 增长 | 确认生成按钮**禁用**（与入口按钮同款判据，不给绕过口） |
| **success（双队列清空 + 今日已出）** | 两个队列均无活跃量、某板块 `report_date` == 本地今天 | 「今日日报已出（N/M 板块）」；生成按钮可用（重生成语义），入口文案改为「重新生成今日日报」；清单内给出**替换警示**（标题、章节、线索全部重写），确认按钮文案为「确认替换」（同日替换是破坏性动作） |
| **success（双队列清空 + 今日未出）** | 两个队列均无活跃量、无板块 `report_date` == 本地今天 | 「今日日报未生成」+ 缺料原因：按**本地时刻与生成时刻比较**分两种文案——未到生成时刻 → 「尚未到生成时刻（21:00）」；已过生成时刻 → 「已过生成时刻（21:00）：今日暂无可用素材或生成未完成」；生成按钮可用 |

**失败态不得伪装成业务结论**：任何 error 态都不得渲染「今日日报未生成」「队列空闲」这类结论性文案。

## 复用组件与布局模式

- **布局模式**：沿用 L0 雷达已登记的固定视口栅格例外（`docs/reference/standard/frontend/layout.md` §已登记例外，`rework-workshop-into-radar-and-desk` 登记项）。**不新增第五区、不改布局模式枚举**——状态块是既有右栏列（`clamp(17rem, 22vw, 29rem)`）内部的一个纵向区块，不改变页面级滚动模型。
- **结构变化（唯一一处）**：右栏从「单列可滚内容」改为「上滚下驻」两段容器——上段（今日该读 / 今天可能升温）保留既有独立滚动，下段为状态块常驻底部。页面本身仍 `overflow: hidden`，仅右栏内部滚动行为不变。
- **复用既有组件**：
  - `AppButton`（生成按钮 / 全选切换）；
  - `AppPageShell`（沿用 `mode="workspace"`，不新建壳）；
  - 既有 `usePollBundle` 对账分发（`tag-queue-progress:status` + `embedding-queue-progress:status` + `poll:schedulers`），**不新建轮询**；
  - 既有 `useEventStream` + `EVENT_TYPES.DAILY_REPORT_PROGRESS/DAILY_REPORT_DONE`（生成进度）；
  - 既有 `useDailyReportsApi().generateDailyReport`（仅扩展参数形态）；
  - 既有 `boardRadarHelpers` 的日期格式化（不复用 `toISOString()` 的 UTC 截断写法）。
- **新建组件**：`BoardRadarStatusBlock.vue`（状态块，独立组件以守住右栏组件「纯展示」边界）+ `useRadarPipelineStatus.ts`（聚合 composable）+ `useEmbeddingQueueProgress.ts`（Embedding 队列计数 composable）。均置于 `features/tags/` 私有目录（除 embedding 计数 composable 需被 `app/composables/` 自动导入），不上移共享层。
- **双队列的量纲**：标签队列单位是**文章**（`tag_jobs` 行按 `article_id` 排），Embedding 队列单位是**标签**（`embedding_queues` 行按 `tag_id` 排）。两个队列**各占一行、各带自己的计数与进度条**，进度文案分别写「今日完成 X / Y」与「今日完成 X / Y 个标签」，SHALL NOT 把两个分母混为同一量纲。
- **视觉**：只用 Layer 2 语义 token（`--color-text-primary/secondary/muted`、`--color-border-subtle/medium`、`--color-accent`、`--color-warning`、`--color-error`、`--color-info`、`--color-bg-hover`、`--color-bg-sunken`），**不写死颜色、不引用 `--raw-*`**；底色用 `color-mix(in srgb, var(--color-accent) N%, transparent)` 派生（与既有 `.rd-warm-block` 同款写法），明暗双主题自动跟随。数字用 `--font-mono`，标题用 `--font-serif`（与右栏既有排版一致）。
- **窄屏**：`< 768px` 时右栏降级为页面下方纵向区块（既有媒体查询已如此），状态块随之落到页面底部；清单改为单列，**不产生横向溢出**；`375px` 基线不得破版。

## 验收映射

| 验收项 | 实物 |
| --- | --- |
| 状态块渲染与五项事实（含双队列两行） | `front/app/features/tags/components/console/BoardRadarStatusBlock.test.ts` |
| 聚合口径（双队列数 / 完成进度 / 生成与兜底时刻 / 最新落点 / 当日判定 / 双队列门槛） | `front/app/features/tags/composables/useRadarPipelineStatus.test.ts` |
| 后端 Embedding 队列快照计数与清空判据 | `backend-go/internal/tagmanagement/handler/embedding_queue_status_test.go` |
| 合并端点下发 Embedding 队列计数 | `backend-go/internal/admin/handler/poll_test.go` |
| 本地日期 key（UTC 截断回归） | `front/app/utils/todayKey.test.ts` |
| 右栏两段容器改造 | `front/app/features/tags/components/console/BoardRadarView.test.ts` |
| 后端 `board_ids` 作用域与互斥 | `backend-go/internal/topicgraph/handler/daily_report_handler_test.go` |
| 后端 `schedule_deadline` 暴露与回落 | `backend-go/internal/admin/handler/scheduler_handler_test.go` |
| 双视口（1440×900 / 1920×1080）+ 明暗双主题 + 窄屏基线 | `ui-verification/report.md` 建账（agent-browser 实页证据） |

**与 UI 复审全系统回归的关系**：本 change 触及 `/tags` 路由（layout.md §UI 复审的全系统与产品任务覆盖 → 路由矩阵条款），归档前需在 `ui-verification/report.md` 覆盖路由矩阵中 `/tags` 条目在 editorial 与 dark 两主题下的桌面 1440×900 检查，并含 dark 截图；其余路由不因本 change 变更（共享壳、导航、其余路由均未改动），此判定须在报告中显式写明，不得默认略过。
