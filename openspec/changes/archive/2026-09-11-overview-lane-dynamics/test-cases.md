# Test Cases: overview-lane-dynamics

主链路故事（用户视角）：选中版块 → 看「板块内容」tab 的泳道动态 → 每条活跃泳道一句话态势 + 14 天发展时间线 → 点卡进话题总览 focus → 瞥候选栏 → （空板块）引导生成日报后自动出内容。

## 0. 契约继承与调整（⓪ 改契约了吗）

本 change REMOVED board-topic-landscape 全部 9 Requirements（视图整体退役）。旧资产反查：`bash scripts/test-assets.sh board-topic-landscape`。

| 旧 Scenario | 处置 | 旧测试 | 动作 |
|---|---|---|---|
| 空态处理（无日报引导生成） | 迁移进 board-lane-dynamics 空态 Requirement | TopicLandscapePanel 空态组件测试 | 迁移到 LaneDynamicsPanel 测试（同端点同行为） |
| 态势卡片点击→话题总览 focus 联动 | 保留等价（卡片点击联动） | TagsPage handleLandscapeSelectTopic 相关断言 | 保留，事件源换成 LaneDynamicsCard |
| 活力顶栏/气泡图/卡片墙/stance 派生/mini-lifeline/待激活引导/后端聚合接口 | 退役 | 对应组件测试 + topic-landscape repository 测试 | 删除（tasks 4.2），grep 清残留 |

新增 capability board-lane-dynamics 全部为 ADDED，无旧测试遗留。

## 1. 主链路表（节拍）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| 1 | 请求 lane-dynamics（有活跃泳道板块） | 泳道动态批量端点·单请求聚合 | 单响应含 lanes+candidates，无 N+1 前端请求 | handler（testcontainer PG） | tasks 2.1 |
| 2 | 渲染卡片 | 泳道卡片构成·一句话态势展示 | 态势句 + as_of 小字渲染 | 组件（Vitest） | tasks 3.3 |
| 3 | 渲染发展时间线 | 发展时间线结构/单日多事件 | 日期分组有序、事件带 thread 标题、对应关系来自响应 | 组件（Vitest） | tasks 3.3 |
| 4 | 点卡片 | 卡片跳转联动·点卡片聚焦 | 切话题总览 tab + focus 该话题 | opencli 端到端 | tasks 5.2 |
| 5 | 候选栏点击 | 同上（候选条目） | 同样跳 focus | opencli 端到端 | tasks 5.2 |
| 6 | 空板块生成日报 | 空态处理·空态引导/生成后刷新 | 引导按钮 → 进度 → 完成后区域自动刷新 | 组件（Vitest）+ 人工 | tasks 3.2 |
| 7 | 日报完成后结算 | 日报后滚动态势结算·日报完成后结算 | SaveReport 后异步 upsert 快照（每活跃泳道一行） | service 单测 | tasks 1.2/1.3 |
| 8 | 结算失败 | 同上·结算失败不阻塞 | 报告正常返回、快照保持旧值、记日志 | service 单测 | tasks 1.3 |

## 2. 变体走查（五组固定清单）

- **输入变体**：泳道名超长（截断省略显示）｜态势句恰 100 字（边界不截断显示，卡片内部滚动）｜ section 标题含特殊字符（转义渲染）→ 组件测试覆盖 3.1 态势/名称；时间线事件 0 条 section（该日不出现日期节点，后端保证）→ repository 测试
- **前置变体**：空集（无日报→空态）｜单泳道单 section（最薄卡片）｜泳道近14天恰 14/15 天前边界（14 天含/15 天不含）→ repository 测试窗口边界；watch 关联泳道同时是 active（正常，标识叠加不重复出卡）→ repository 测试
- **时间窗口**：days=14 默认｜days=1（单日窗口）｜days=0（参数校验拒绝或全量，实现定并断言）→ handler 测试；as_of 滞后于最新报告日（结算未跑/失败）→ 前端如实标注不隐藏 → 组件测试
- **幂等**：同日重跑日报（覆盖重建）→ 结算再次触发快照覆盖（不产生第二行）→ service 单测；连续两天日报 → 快照 as_of 前移、窗口滚动 → service 单测；结算 goroutine panic → 主流程无感 → service 单测
- **可用性（UI 前三必检）**：误输入反馈（本区无输入控件，N/A）｜空态（无日报/无泳道两态）→ 组件测试｜错误态（端点 500 → 内联错误条 + 重试）→ 组件测试；加载态（骨架占位）→ 组件测试。超长候选动向（ellipsis 截断）→ 组件测试；重复提交（生成日报按钮 loading 防重）→ 迁移用例保留

## 3. 层选择说明

- 聚合 SQL/窗口边界/watch 判定：testcontainer PG（repository 禁 SQLite）
- 结算挂点/松耦合/幂等：service 单测（DB mock 或 testcontainer）
- 卡片渲染/降级/折叠/空态：Vitest 组件测试
- 完整交互故事（点卡→focus、空态→生成→刷新）：opencli 端到端（主链路至少一个 opencli 落点，tasks 5.2）
- 视觉（网格/时间线/占位）：k3 截图证据（tasks 5.3）

## 4. 效果核对（效果依赖断言外因素）

结算内容质量依赖 LLM 行为：部署后首个日报日核对——快照行数 = 活跃泳道数（clamp 20 内）、抽样 3 条态势句与该泳道时间线事实一致（无编造日期/数字）、`ai_call_logs` 有 operation=daily_report.lane_snapshot 记录且无失败堆积。量化结果记 tasks 5.4。

## 5. 展示字段盘点（改数据结构）

新增用户可见字段及来源语义锚：态势句/汇总截止（specs·泳道卡片构成）、追踪中角标（specs·展示范围与排序）、待结算占位（specs·批量端点·无快照降级）、候选栏动向（specs·候选栏）、时间线折叠计数（specs·单日多事件）。全部有 Requirement 锚，无隐式契约。
