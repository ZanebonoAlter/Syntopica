<!-- ui-impact: major -->
<!-- ui-approval: approved -->
<!-- ui-prototype: ui-prototype/index.html -->

# UI Design: overview-lane-dynamics

## 1. User Journey

**入口**：叙事工坊 `/tags` → 左栏选版块 → 「板块内容」tab（默认 tab，改动前后一致）。

**主任务**：扫一眼每条关注/追踪的泳道最近 14 天发生了什么——先看一句话态势，感兴趣再扫发展时间线，想深挖点卡片进「话题总览」focus。

**次任务**：
- 瞥一眼候选栏，看有没有值得转正的新话题苗子（提示性质，转正去话题总览）
- 无日报的新板块：从空态引导一键生成日报（进度反馈后自动出内容）

## 2. Information Architecture

```
「板块内容」tab
├─ 构成标签管理区（保留，不动）
├─ 分隔线
└─ 泳道动态区（LaneDynamicsPanel，本 change 新建）
   ├─ 区头：标题「泳道动态」+ 窗口说明（近14天·随每日日报结算）
   ├─ 主卡片区：泳道卡网格（watch 角标卡与普通卡混排，按动态量排序）
   │    每卡：泳道名 + watch角标 + 一句话态势(含 as_of) + 发展时间线
   └─ 候选栏：单行紧凑列表（名称 + 最近动向 + 「去转正」弱链接跳话题总览）
```

信息层级：态势句是每卡第一视觉重心（字号大于时间线），时间线是第二层细节，候选栏是第三层边缘信息。

## 3. Interaction Contract

| 操作 | 类型 | 反馈 |
|------|------|------|
| 点泳道卡（整卡可点） | 主操作 | 切「话题总览」tab 并聚焦该话题 focus 视图（沿用现有联动） |
| 候选栏条目点击 | 次操作 | 同上跳转 focus；无独立弹层 |
| 时间线「还有 N 条」折叠 | 次操作 | 展开该日全部事件标题（就地展开，无弹层） |
| 空态「生成日报」按钮 | 次操作 | 按钮 loading + 进度提示（WS），完成后区域自动刷新（迁移自原态势版图空态） |
| hover 泳道卡 | 视觉 | 卡片边框/阴影轻微抬升，暗示可点 |

无危险操作、无多步流程、无拖拽。**本区不含任何写操作**（只读红线，见 specs）。

## 4. State Matrix

| 状态 | 触发 | 呈现 |
|------|------|------|
| loading | 首次进 tab / 切版块 | 区块骨架（卡片占位 shimmer，不用转圈盖层） |
| empty（无日报） | 板块 0 份报告 | 空态引导：「生成日报」按钮 + 说明文案（态势需要日报数据） |
| empty（有日报无活跃泳道） | 报告存在但 lanes=[] | 「暂无活跃泳道——在日报里孵化话题后出现」+ 候选栏若也空则整区只留此文案 |
| success（正常） | lanes 非空 | 卡片网格 + 候选栏 |
| success（部分待结算） | snapshot=null 的卡 | 该卡态势位置显示「待结算」浅占位，时间线照常渲染，卡片不置灰 |
| error | 端点失败 | 区块内联错误条 + 「重试」按钮（不弹 dialog）；保留上次数据则显示旧数据 + 顶部刷新提示条 |
| 结算滞后 | as_of 早于最新报告日 | 态势句旁小字标注「汇总截止 M/D」（如实标注不隐藏） |

错误恢复路径：error → 点重试 → 重新请求；结算失败不属于前端错误（快照旧值/缺失态自呈现）。

## 5. Layout Contract

- **layout mode：workspace**（TagsPage 版块详情内容区本就是 workspace 环境：左版块列表栏 + 右内容区；泳道动态区在内容区内填满可用宽度）
- 主卡片区网格：`repeat(auto-fill, minmax(360px, 1fr))`，卡片高度以内容为准（时间线超长卡片内部滚动，max-height ≈ 420px，超出区滚动条）
- 候选栏：单列紧凑列表，不做卡片
- 时间线：垂直时间线，左列日期（等宽小字）+ 竖向连接线 + 右列事件列表；日期节点间距 8px
- 目标视口：桌面 1440×900（主验证）与 1920×1080；内容区随 workspace 弹性，卡片网格自适应列数（1440 下预期 2 列，1920 下 2~3 列）
- 无 dialog（本 change 不引入弹层）

## 6. Component Reuse

| 复用 | 来源 |
|------|------|
| 空态生成日报 + WS 进度 | 迁移自 `TopicLandscapePanel.vue`（useDailyReportProgress composable 原样复用） |
| 点卡跳话题总览 focus | 复用 `TagsPage.handleLandscapeSelectTopic` 既有模式（emit → 切 tab 聚焦） |
| 主题 token | `--color-bg-elevated`（卡片底）、`--color-text-secondary`（时间线次要字）、`--color-info`（watch 角标）、`--color-border-subtle`（时间线连接线）等 main.css 既有变量 |
| 折叠交互 | 参照日报阅读层 thread 折叠的既有模式（「还有 N 条」就地展开） |

**新组件**：`LaneDynamicsPanel.vue`（容器：请求/状态/空态/候选栏）、`LaneDynamicsCard.vue`（单卡：态势 + 时间线 + 折叠）。均为本 change 新建，放 `features/tags/components/lane-dynamics/`。

## 7. Prototype

静态可丢弃原型：`ui-prototype/index.html`（独立 HTML，不连 API，含 4 个泳道卡：watch 角标卡 / 普通活跃卡 / 待结算卡 / 长时间线折叠卡 + 候选栏 + 空态示例段），引用 main.css 同名 token 值。**实现须覆盖双主题**（light/dark）：全部颜色走 main.css 语义 token（bg-elevated/text-*/border-*/info 等），不硬编码色值，暗色主题自动适配；原型仅为 light 态视觉基准。审批通过后实现以原型为视觉基准。（2026-09-09 用户对话批准，附双主题实现要求）

## 8. Acceptance

- opencli 主链路断言：进板块内容 tab → 泳道卡渲染 → 点卡片切话题总览 focus → 返回；候选栏条目点击跳转
- 1440×900 与 1920×1080 两档视觉检查证据（卡片网格列数、时间线不溢出、workspace 弹性正常）
- 待结算降级与 as_of 标注的视觉核对
- 与批准原型的差异说明（重大差异需重审）
