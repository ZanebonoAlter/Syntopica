<!-- ui-impact: major -->
<!-- ui-approval: approved -->
<!-- ui-prototype: ui-prototype/index.html -->

<!--
  marker 契约（ui-design-gate 机器读取）：
  - ui-impact 与 proposal 一致（major）
  - approval 只有用户对话明确确认后才可改 approved
  - 信息架构/主流程/布局模式修订后 MUST 重置 pending
-->

## User Journey

**入口**：全局 header（AppHeaderView）右上角图标区，两处新入口：
1. **铃铛按钮**（NotificationBell）：位于既有 header-btn 序列（暂停分析 / AI健康 / 刷新 / 全部已读 / 分隔线 / 设置 / 引导 / 主题）中「全部已读」与「分隔线」之间，与现有图标按钮同规格（20×20 icon、统一 hover）。
2. **标签队列进度芯片**（TagQueueProgressChip）：铃铛左侧，仅当标签队列存在 pending/processing 任务时显示（空闲自动隐藏）。

**主任务**：
- 用户离开页面后回来 → 点铃铛 → 在下拉面板看到错过的日报结果通知（生成完成/失败汇总）→ 标已读或全部已读。
- 用户发起批量打标后 → 不停留在设置页 → header 进度芯片持续显示「分析中 n/total」实时推进 → 完成后芯片消失（可配置短暂停留展示完成态）。

**次任务**：通知面板内点击与文章相关的通知 → 跳转对应文章/日报详情；点击失败类通知 → 跳设置页对应队列区。

## Information Architecture

- 不新增页面、不改变现有页面导航结构，全部为 **header 层新增全局组件**（两处）：
  - NotificationBell：图标按钮（含未读角标）+ 锚定下拉面板（内含通知列表）。
  - TagQueueProgressChip：紧凑进度芯片（图标 + 文案 + 迷你进度条）。
- 通知面板信息层级（单层列表，无子导航）：未读分组置顶（视觉强调）→ 已读按时间倒序；每条通知 = 图标（成功/失败/信息三态）+ 标题 + 摘要 + 相对时间。通知类型白名单 = 日报生成终态（完成/失败），稳态每天 1~2 条。
- 面板头部：标题「通知」+ 未读数 + 「全部标已读」次操作；底部：「查看全部/设置」弱化入口跳设置页。

## Interaction Contract

| 操作 | 等级 | 交互 | 反馈 |
| --- | --- | --- | --- |
| 点击铃铛 | 主 | toggle 下拉面板；再次点击/点击面板外/Esc 关闭 | 面板出现时未读数清零计入「已读」策略见下 |
| 全部标已读 | 次 | 点击后批量标记 | 角标消失、未读区强调解除；无需 confirm |
| 单条标已读 | 次 | 每条通知 hover 显「标已读」次操作 | 该条未读强调解除 |
| 点击通知跳转 | 主 | 跳对应详情页（完成→日报详情；失败→日报页/设置页）并自动标已读、关闭面板 | 路由跳转 |
| 清空通知 | 危险 | 面板底部弱化入口旁「清空」 | **需 confirm**（复用 AppDialog sm 档确认框） |
| 点击进度芯片 | 主 | 跳设置页队列区（/settings 队列 section） | 路由跳转 |
| 新通知到达 | — | 角标 +1；面板开着时列表顶部插入新条 | 无 toast 打断（终态事件 toast 由既有 useNotify 调用方继续，行为不变） |

**已读策略**：打开面板即视为「浏览」，未读数清零但保留未读视觉记录（条目强调在本次浏览会话内保留，刷新后消失）——避免用户关面板就丢线索。

## State Matrix

| 组件 | loading | empty | error | success |
| --- | --- | --- | --- | --- |
| 铃铛角标 | — | 未读数 0：无角标 | 拉取失败：角标不显示，WS 重连恢复 | 未读数 >0：角标显示数字（>99 显示 99+） |
| 通知列表 | 首次拉取：面板内骨架行 | 无任何通知：「暂无通知」空态 + 说明文案 | 拉取失败：面板内错误态 + 重试按钮（恢复路径：点击重试 / WS 重连后自动重拉） | 列表渲染；分页加载更多（滚动触底） |
| 进度芯片 | — | 队列空闲：组件不渲染（自动隐藏） | status API 失败：芯片隐藏不报错（WS tag 事件到达即恢复） | 「分析中 n/total」+ 进度百分比；全失败时变红提示「n 个失败」可点击跳队列区 |

错误恢复统一原则：静默降级 + WS 事件/重连驱动自动恢复，不打断用户主流程。

## Layout Contract

- **layout mode：不适用**（两个组件均挂靠全局 header，不新增页面、不改变页面 layout mode）。
- **通知下拉面板**：锚定 popover（非模态 dialog），**自由宽度 380px，理由**：单列通知卡片最适可读宽度（标题+摘要一行半），与 header 锚点右对齐；**目标视口**：1440×900 / 1920×1080；上限 `min(380px, 92vw)` 防窄屏溢出。面板高度 max 480px 内部滚动。
- **进度芯片**：自适应内容宽（约 140~180px），右侧锚进 header-btn 序列，高度与 header-btn 一致（36px 内联）。
- 双视口验收（1440×900 / 1920×1080）：面板不与右侧既有按钮重叠、不横向溢出；芯片在两档下均不出换行/截断。

## Component Reuse

- **复用**：AppButton（面板内按钮）、AppDialog（仅「清空通知」confirm 用 sm 档）、`useEventStream`（WS 订阅）、`useNotify`（不动，其调用方行为不变）、TagQueuePanel（设置页队列区，芯片点击的落点；其状态区计数展示语义同步调整：新增活跃量主展示 pending+leased，completed 改显「今日完成」，配合队列行每天重置后总量回归正常量级）、`getApiOrigin`/apiClient。
- **新增**：
  - `NotificationBell.vue`（铃铛 + 角标 + 锚定面板壳）
  - `NotificationPanel.vue`（列表 + 状态矩阵，Teleport 到 body）
  - `NotificationItem.vue`（单条）
  - `TagQueueProgressChip.vue`
  - `useNotifications` composable（未读数/列表/标已读/WS 订阅）
  - `useTagQueueProgress` composable（status 拉取 + tag 事件驱动）

## Prototype

`ui-prototype/index.html`：静态原型，含两处入口的完整状态演示——
- header 复刻（现有按钮序 + 新增铃铛/芯片位置）
- 铃铛：未读角标态 → 点击展开面板（fixture 对齐白名单：日报失败/日报完成 未读条 + 历史已读条）→ 空态
- 芯片：进行中态（3/40 进度条）→ 失败态（红色）→ 空闲隐藏
- 纯静态 fixture 数据、不连真实 API、引用现有 CSS 变量（--color-* token）与主题方向；浏览器直接打开即可查看。

## Acceptance

**2026-09-18 验收已执行（浏览器实测）**：
1. opencli 主链路断言（agent-browser，同源入口 localhost）：✓ 点铃铛→面板展开（「已全部浏览」）→角标清零→后端 unread=0 对账（M2 落库 read-all 生效）→Esc 关面板；✓ 芯片「分析中 0/374」可见→点击跳 `/settings?section=queues&queue=tag`。断言输出留痕会话记录。
2. 双视口视觉检查：1440×900 与 1920×1080 截图存 `/tmp/notif-acceptance/`（v5-1440x900.png / v6-1920x1080.png），机械断言两档均 overflow=false、panelW=380px 锚定铃铛下方，与铃铛/芯片无重叠。
3. 组件单测 60/60（7 文件）+ 机械锚（Teleport 挂载锚 + 浮层样式锚 + 空闲不渲染锚）。
4. **与批准原型差异说明**：无信息架构/交互流程/布局差异。实现差异仅两处（均不重审）：①面板头部右侧用「全部已读」文字按钮（原型同位置一致，视觉微调）；②空态文案与原型一致。openPanel 落库语义（design Decision 4）在原型演示「打开即清角标」之上增加了持久化，属设计内决策非偏差。

实现完成后补齐（major 双层验收）：
1. **opencli 主链路断言**：铃铛点击 → 面板展开 → 未读数清零 → 全部标已读 → 角标消失；芯片在有任务时出现、点击跳设置页队列区。
2. **双视口视觉检查**：1440×900 与 1920×1080 两档截图目检（面板锚定位置/无重叠/无溢出；芯片不换行）。
3. **组件单测**：NotificationPanel 状态矩阵（loading/empty/error/success）+ 芯片空闲隐藏逻辑 + 机械锚（Teleport 挂载锚 + 浮层样式锚）。
4. **差异说明**：与批准原型的差异回写本节；重大差异（信息架构/交互流程）需重新审批。
