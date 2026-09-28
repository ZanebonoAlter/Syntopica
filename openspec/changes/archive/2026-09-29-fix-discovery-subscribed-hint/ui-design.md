<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## 入口与入口变更

四个入口都在既有发现页（`/discovery`，`AppPageShell mode="contained"`）结构内部，不新增页面/弹窗/导航：

| 入口 | 文件 | 变更 |
| --- | --- | --- |
| 为你推荐卡 | `features/discovery/components/DiscoveryCard.vue` | 头部徽标行新增「已订阅」徽标（只读提示，`title` 列出已有订阅地址）；既有「问答 / 需填参数 / 一键订阅」徽标与订阅表单保持原样，订阅入口保留 |
| 查询结果卡 | `features/discovery/components/DiscoveryRunCard.vue` | 新增「已订阅」徽标；原先「会话内已订阅 → 按钮替换为『已订阅』文本」改为**徽标 + 订阅按钮并存**（路由级命中仍可订别的参数），是否重复由弹窗按最终地址拦截 |
| 候选源库行 | `features/discovery/components/CandidateLibrary.vue` | 复用既有「已订阅 / 未订阅」状态位，修正 RSSHub 条目取值（原先恒「未订阅」）；按钮规则：原生 RSS 已订阅仍隐藏（同址无新可订，同现状），RSSHub 条目保留订阅入口 |
| 订阅弹窗 | `features/discovery/components/CandidateSubscribeDialog.vue` | 新增「已有订阅」信息块（列出该路由已有订阅地址，只读）；当填参拼出的最终地址命中已有订阅时，就地锁定提交并说明原因。原生 RSS 路径与现状一致（详情已订阅 → 「已订阅」终态） |

## 受影响状态

| 状态 | 变化 |
| --- | --- |
| loading | 不变（列表骨架、弹窗加载态沿用） |
| empty | 不变（空库/空筛选/零结果文案沿用） |
| error | 不变（候选库失败不冒称空库、查询失败保留输入） |
| success（已订阅） | 新增徽标态：卡片头部「已订阅」+ 候选库状态位按真实取值；提示只读，不阻塞任何操作 |
| success（弹窗内地址已存在） | 提交按钮锁定 + 说明文案，区分「完全相同的地址已在订阅列表」与「同一路由已有其它订阅（仍可提交新参数）」 |

## 复用组件与布局模式

- **布局模式**：发现页 `AppPageShell mode="contained"`（1120 上限）不变；不改页面骨架、信息架构与区块划分；订阅弹窗沿用既有 dialog 尺寸档，不新增自由宽度。
- **复用组件/样式**：徽标复用卡片既有圆角胶囊样式族（`discovery-card__badge*` / `run-card__availability` 的 token：`--color-bg-sunken`、`--color-border-medium`、`--color-success`）与候选库 `library__status-value is-subscribed`；弹窗信息块用既有文本样式 + `AppButton`，**不新增基础组件**。
- **窄屏**：徽标行已是 `flex-wrap`（卡片头部），长地址提示走 `title` 属性不占布局宽度，无窄屏回归风险。
- **主题**：只用既有 CSS 变量，不引入硬编码色值，双主题（light/dark）自动适配。

## 验收映射

| 验收点 | 方式 |
| --- | --- |
| 推荐卡 / 查询结果卡显示「已订阅」且订阅入口保留 | 组件测试 `DiscoveryCard.test.ts`、`DiscoveryRunCard.test.ts` |
| 候选库 RSSHub 行显示「已订阅」，原生 RSS 已订阅仍隐藏按钮 | 组件测试 `CandidateLibrary.test.ts` |
| 弹窗区分「同址已订阅」与「同路由其它参数」并锁定/放行提交 | 组件测试 `CandidateSubscribeDialog.test.ts` |
| 后端订阅状态判定（路由级 / 精确地址 / 边界） | 后端单测 `subscription_state_test.go` + `candidate_catalog_service_test.go` / `discovery_run_service_test.go` / `recommendation_service_test.go` |
| 真实页面走查（三入口徽标可见、订阅入口未消失） | opencli 或人工走查发现页（minor 档：不新增布局结构，无强制双视口验收） |
