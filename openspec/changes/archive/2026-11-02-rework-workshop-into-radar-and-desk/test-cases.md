# test-cases: rework-workshop-into-radar-and-desk

> **获批8C增量故事S11**：[当日其他话题用例账本](test-cases-matrix-day-others.md)包含冷启动/成熟矩阵/获批跨日观察与返回（S11-A/B/C）、五组变体、旧资产M/R处置与白盒。与[实施合同](implementation-matrix-day-others.md)配套；其他话题不替换原工作面或改变生命周期。此入口不改旧53/1/2及A-ID历史状态，实际产品/浏览器结果待逐包验收登记。

> 复杂度声明：`complexity: complex`（多模块交互协议：路由/状态映射 + 矩阵派生 + 后端聚合契约）。
> 权威行为用例 = `specs/**/spec.md` 的 Scenario；本文件把它们串成故事（主链路），补 Scenario 未覆盖的变体与白盒分支。
> 判据原则：① 每条只断言一个可机械观察的结果；② 负向行为用反向断言（选择器为空 / 返回 null / 写请求计数 0）；③ 渲染用例统一 mock 网络与 `@iconify/vue`；④ 数值取「阈值前 / 阈值上 / 阈值后」三点，写清进位方向。
> 落点文件名为计划名（tasks §9 落地）；阶段执行顺序见 tasks 阶段①–④。

---

## 故事 S1：版块态势（锚 `workshop-board-console` + `board-lane-dynamics`）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 从雷达/直接地址进入 `/tags/:boardId` | 默认展示泳道动态区 | 渲染面包屑、版块栏、矩阵舞台、治理右轨；矩阵为默认视图 | 组件 | `console/BoardConsoleShell.test.ts` |
| 2 | 观察矩阵 | 渲染近 14 天矩阵 | 6 行 × 14 列（行 = 非休眠泳道），行尾线索总数（title 给分区数）、今天列高亮 | 组件 | `console/BoardMatrixView.test.ts` |
| 3 | 观察断档格 | 断档可读 | 空格文本为空（不渲染 0），带断档样式 | 组件 | `console/BoardMatrixView.test.ts` |
| 4 | 点有分区的格子 | 查看某日事件 | 打开 section 抽屉（32rem overlay），标题含日期 + 泳道名，列出该日全部 section/事件 | 组件 | `console/SectionDrawer.test.ts` |
| 5 | 点空格 | 空格提示 | 提示「这一天这条泳道没有分区」，不打开抽屉 | 组件 | `console/BoardMatrixView.test.ts` |
| 6 | 按 Esc / 点遮罩 | 只读语义（关闭保持） | 抽屉关闭，选中泳道不变 | 组件 | `console/SectionDrawer.test.ts` |
| 7 | 点另一行泳道名 | 切换泳道 | 演进带切换为该泳道，矩阵行高亮 | 组件 | `console/BoardMatrixView.test.ts` |
| 8 | 选中泳道当日无 section | 断档日期不造假节点 | 演进带该日期为空心节点、无事件标签 | 组件 | `console/BoardLaneEvolution.test.ts` |
| 9 | 观察演进带节点 | 发展时间线结构 | 节点按日期排列，≥3 条线索显示标签，0 条空心；数据与响应一致 | 组件 | `console/BoardLaneEvolution.test.ts` |
| 10 | 观察态势句 | 一句话态势展示 | 有 `snapshot` 时显示 summary + `as_of`，长句折叠；`null` 时显示待结算占位且时间线照常 | 组件 | `console/BoardLaneEvolution.test.ts` |
| 11 | 同日多分区 | 单日多事件 | 抽屉列出全部 section 全部事件；后端截断时提示折叠数 | 组件 | `console/SectionDrawer.test.ts` |
| 12 | 存在 ≤2 分区的活跃泳道 | 折叠 | 主矩阵不渲染该行，显示「休眠泳道 N」折叠条 | 纯逻辑 + 组件 | `composables/useBoardConsole.test.ts`、`console/BoardMatrixView.test.ts` |
| 13 | 点休眠折叠条 | 展开休眠 | 该行以弱化样式出现，仍可选中/开抽屉 | 组件 | `console/BoardMatrixView.test.ts` |
| 14 | 带 `?lane=<topicId>` 打开 | 恢复上次选中泳道 | 选中该泳道并渲染演进带；不存在时回退线索数最多的泳道 | 纯逻辑 + 组件 | `composables/useBoardConsole.test.ts` |
| 15 | 点面包屑「叙事工坊」 | 面包屑返回雷达 | 导航到 `/tags` | 组件 | `console/BoardConsoleShell.test.ts` |
| 16 | 版块有体检快照，评分 85 | 右轨展示体检信号 | 右轨显示 85 + 分档色 + 趋势迷你条 + 幽灵匹配数 | 组件 | `BoardHealthPanel.test.ts`（信号区）、`console/BoardConsoleShell.test.ts`（右轨装配） |
| 17 | 版块无体检快照 | 无体检快照 | 显示「等待首轮体检」，构成管理入口仍可点 | 组件 | `BoardHealthPanel.test.ts` |
| 18 | 找匹配回填入口 | 回填入口 | 常驻工具栏无回填/升级建议/匹配参数；入口在版块「更多」菜单/右轨版块治理，全库动作有范围提示 | 组件 | `console/BoardConsoleShell.test.ts`（阶段③） |
| 19 | 右轨体检接口 500 | 右轨失败不阻塞（3.3） | 矩阵/版块栏正常渲染，右轨局部错误 + 重试 | 组件 | `console/BoardGovernanceRail.test.ts`（阶段③） |
| 20 | 双视口走主链路 A-1…A-2 | major UI 验收 | 1440/1920 无页面级纵向滚动；三栏不挤压；抽屉为 overlay | 端到端 | `ui-verification/report.md`（agent-browser） |

### 变体走查

| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 前置：版块无日报 | 空态「该版块还没有日报」+ 生成入口；不渲染空矩阵 | 组件 | `console/BoardMatrixView.test.ts` |
| 2 | 前置：有日报无活跃泳道 | 空态不把「无泳道」当「无内容」：显示回退条 + 最新日报/归档入口，无休眠条 | 组件 + 浏览器 | `console/BoardMatrixView.test.ts` / `BoardConsoleShell.test.ts` |
| 3 | 前置：单元素（1 条泳道） | 1 行矩阵 + 演进带；不报错 | 组件 | `console/BoardMatrixView.test.ts` |
| 4 | 前置：越界引用（API 500 / 不存在 board） | 内联错误 + 重试；版块栏可用；不白屏 | 组件 | `console/BoardConsoleShell.test.ts` |
| 5 | 时间：窗口切换 7/30/全部 | 轴长与请求一致；选中泳道保持 | 纯逻辑 | `composables/useBoardConsole.test.ts` |
| 6 | 时间：跨月/跨年轴 | 日期连续无重复 | 纯逻辑 | `composables/useBoardConsole.test.ts` |
| 7 | 幂等：重复点同一格 / 重复刷新 | 抽屉不叠加；数据不重复追加 | 组件 | `console/SectionDrawer.test.ts` |
| 8 | 可用性：加载态（慢响应） | 网格骨架；右轨骨架独立 | 组件 | `console/BoardMatrixView.test.ts` |
| 9 | 可用性：错误态（刷新失败） | 保留旧数据 + 「刷新失败」提示条 + 重试 | 组件 | `console/BoardMatrixView.test.ts` |
| 10 | 可用性：超长泳道名 / 超长 section 标题 | ellipsis 截断，title 可达全文；不撑破行 | 组件 | `console/BoardMatrixView.test.ts` |
| 11 | 输入：`?lane=` 空串 / 纯空白 / 负数 / 非数字 | 全部按非法回退默认泳道 | 纯逻辑 | `composables/useBoardConsole.test.ts` |
| 12 | 输入：窗口参数非法（days=0/-1/abc） | 忽略，回落默认 14 | 纯逻辑 | `composables/useBoardConsole.test.ts` |
| 13 | 并发：切版块时旧请求晚到 | 旧版块响应 MUST NOT 覆盖新版块（请求序号守卫） | 纯逻辑 | `composables/useBoardConsole.test.ts` |

---

## 故事 S2：文章归档（锚 `workshop-article-archive` + `board-article-api`）

### 主链路

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 版块态势切「归档」 | 展示日期统计 | 默认数据锚点当天；统计条显示文章 / 已入 / 未入 / 弱证据，并显示当前起止日期 | 组件 | `console/BoardArchiveView.test.ts` |
| 2 | 所选日期无文章 | 无文章 | 空态说明该日期没有匹配文章 | 组件 | `console/BoardArchiveView.test.ts` |
| 3 | 同一天多篇文章 | 按日期平铺 | 日期头下平铺文章行，不按话题/标签分组 | 纯逻辑 + 组件 | `console/BoardArchiveView.test.ts` |
| 4 | 未打标文章 | 无标签 | 行内「无标签」；抽屉说明不会进日报 | 组件 | `console/BoardArchiveView.test.ts` |
| 5 | 选择「未纳入日报」 | 筛选遗漏 | 只保留未入日报的单篇文章 | 组件 | `console/BoardArchiveView.test.ts` |
| 6 | 选择「弱证据」 | 弱证据筛选 | 只保留无标签 / max_sim / 降级文章 | 组件 | `console/BoardArchiveView.test.ts` |
| 7 | 点文章，规则 max_sim 且降级 | 解释弱证据 | 抽屉显示「相似度 x.xx ↓」+ 弱证据说明 | 组件 | `console/BoardArchiveView.test.ts` |
| 8 | 方向校验未通过 | 方向校验标记 | 抽屉标注方向校验未通过并说明降级隐藏 | 组件 | `console/BoardArchiveView.test.ts` |
| 9 | 自由日期范围 / 近 7 天 | 日期筛选 | 重新请求所选起止日期；日期优先排序，不按遗漏跨日期打乱 | 纯逻辑 + 组件 | `console/BoardArchiveView.test.ts` |
| 10 | 浏览任意文章 | 无写动作 | 无「补进日报 / 重跑」控件 | 组件 | `console/BoardArchiveView.test.ts` |
| 11 | 后端窗口覆盖 12/20 | 标记未入日报 | 12 篇 `in_report=true`，8 篇 false | handler | `backend-go/internal/tagmanagement/handler/board_article_handler_test.go` |
| 12 | 请求 `report_status=not_in_report` | 按遗漏过滤 | 全部 `in_report=false` | handler | 同上 |
| 13 | 请求 `report_status=weak` | 弱证据过滤 | 仅无标签 / max_sim / downgraded | handler | 同上 |

### 变体走查

| # | 变体 | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 输入：`report_status=bogus` | 抛非法参数，HTTP 400 | handler | `board_article_handler_test.go` |
| 2 | 输入：窗口参数 0 / -1 / 超大 | 回落默认窗口 / 上限钳制，不 500 | service | 同上 |
| 3 | 前置：文章 tags 多标签 | 行内显示全部命中标签与规则/分数，不重复出现 | 纯逻辑 | `console/BoardArchiveView.test.ts` |
| 4 | 时间：文章在窗口外日报线索 | `in_report=false` | handler | 同上 |
| 5 | 可用性：列表加载失败 | 错误条 + 重试，右轨/版块栏可用 | 组件 | `console/BoardArchiveView.test.ts` |
| 6 | 可用性：超长标题/来源名 | ellipsis + title 全文 | 组件 | `console/BoardArchiveView.test.ts` |

---

## 故事 S3：新建泳道编排态（锚 `workshop-lane-compose`）

### 主链路

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 矩阵点「新建泳道」 | 进入编排态 | 全屏编排态；画布 = 日期列 × active 泳道行 + 待确认行；顶栏含名称/成员/平均距离/离群/取消/保存 | 组件 | `console/BoardComposeOverlay.test.ts` |
| 2 | 点取消 / Esc | 取消 | 退出编排态，清空选择，矩阵与既有泳道不变 | 组件 | 同上 |
| 3 | 点候选「采纳」 | 采纳预填 | 名称预填候选 label，成员全选（上限 20），质量指标刷新 | 纯逻辑 + 组件 | 同上 |
| 4 | 候选只出现 1 天 | 连续天数不足 | 「确认启用」置灰 + 提示「需先满足连续多天出现条件」 | 组件 | 同上 |
| 5 | 搜索框输入关键词 | 搜索过滤 | 只保留话题名/section 名包含关键词的候选 | 组件 | 同上 |
| 6 | 点画布节点 | 点击节点推荐关联 | 画 top-6 连线，侧栏列关联 section 与距离（3 位小数） | 组件 | 同上 |
| 7 | 点侧栏关联项 | 从推荐项加入 | 该 section 加入成员并显示勾选 | 组件 | 同上 |
| 8 | 节点无 relation | 无关联 | 侧栏说明无关联，画布不画线 | 组件 | 同上 |
| 9 | 采纳含 2 个 section 且距离 0.131 | 采纳后质量刷新 | 成员 2、平均距离 0.131、离群 0 | 纯逻辑 | `composables/useInlineCompose.test.ts`（既有）+ 组件 |
| 10 | 成员间无任何已知关系 | 无距离数据 | 平均距离显示「—」而非 0 | 纯逻辑 | 同上 |
| 11 | 有成员、名称为空点保存 | 缺名称 | 提示「给新泳道起个名字」且不发创建请求 | 组件 | `console/BoardComposeOverlay.test.ts` |
| 12 | 名称与成员齐备点保存 | 保存成功 | 创建 1 次 → 退出编排态 → 回矩阵高亮新泳道 | 组件 | 同上 |
| 13 | 375px 点新建泳道 | 窄屏提示 | 提示「新建泳道需要宽屏或横屏」，不进入编排态 | 组件 | `console/BoardConsoleShell.test.ts` |

### 变体走查

| # | 变体 | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 输入：搜索纯空白 / 大小写 / 特殊字符 | 纯空白=不过滤；大小写不敏感；特殊字符按字面匹配 | 组件 | `console/BoardComposeOverlay.test.ts` |
| 2 | 输入：超长泳道名 | 输入保留全文，顶栏 ellipsis；保存不截断 | 组件 | 同上 |
| 3 | 前置：无待确认 section | 空态「暂无未分类 section」；仍可取消 | 组件 | 同上 |
| 4 | 前置：成员为空保存 | 提示选择成员；不发请求 | 组件 | 同上 |
| 5 | 前置：预勾上限 20/25 | 取日期升序前 20 | 纯逻辑 | `composables/useInlineCompose.test.ts` |
| 6 | 幂等：重复点保存 | 请求中按钮 disabled，只创建一次 | 组件 | `console/BoardComposeOverlay.test.ts` |
| 7 | 可用性：保存失败 | 行内/顶栏错误 + 重试；成员保留 | 组件 | 同上 |

---

## 故事 S4：今日雷达（锚 `workshop-radar`）

### 主链路

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 侧栏进叙事工坊（无版块） | 打开工坊首屏 | 渲染雷达列表（关注度降序）；不渲染辅助/组合标签池 | 组件 | `console/BoardRadarView.test.ts` |
| 2 | 点雷达行 | 从雷达进入版块 | 路由 `/tags/:boardId`，渲染矩阵 | 组件 | 同上 |
| 3 | 当日最高且环比上升、关注度 ≥50 | 热点行带理由与密度 | 排最前 + 「今天要看」+ 最新日报标题 + 泳道密度条；未达 50 只排前不带标 | 组件 | 同上 |
| 4 | 近 14 天无分区 | 常温版块折叠 | 只出现在「常温」摘要行 | 组件 | 同上 |
| 5 | 当日 8，近 3 日日均 3 | 升温判定 | 列入「今天可能升温」并显示约 2.7 倍 | 纯逻辑 + 组件 | 同上 |
| 6 | 无版块满足 | 无升温 | 文案「今天没有明显升温的版块」 | 组件 | 同上 |
| 7 | 首屏加载 | 一次请求渲染雷达 | 只调用 `/semantic-boards/overview`；不为每版块并行请求 | 组件 | 同上 |
| 8 | 某活跃版块无数据 | 无数据版块 | 端点返回 0 值行（非 404），前端归入常温 | handler + 组件 | `overview_handler_test.go`、`console/BoardRadarView.test.ts` |

### 变体走查

| # | 变体 | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 前端：overview 单版块 >5s | 该行 0 值 + `data_delayed=true`，其余行正常 | handler | `overview_handler_test.go` |
| 2 | 前端：overview 整体 500 | 顶部错误条 + 重试；已加载行保留 | 组件 | `console/BoardRadarView.test.ts` |
| 3 | 前置：0 个活跃版块 | 空态「今天没有需要看的版块」 | 组件 | 同上 |
| 4 | 时间：跨零点生成日报 | 今日口径按 Asia/Shanghai 日历日 | service | `overview_handler_test.go` |
| 5 | 幂等：重复进入雷达 | 不写任何表 | handler | 同上（写探针 0） |
| 6 | 可用性：最新日报标题超长 | ellipsis + title 全文 | 组件 | `console/BoardRadarView.test.ts` |

---

## 故事 S5：日报阅读台（锚 `daily-report-system` MODIFIED）

### 主链路

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 打开版块日报 | 展示日报卡片列表 | 索引顶部日期下拉按日期倒序列出本版块日报 | 组件 | `components/daily-report/DailyReportDeskShell.test.ts` |
| 2 | 点某日报 / 「打开今日日报」 | 打开全屏日报详情 | 路由 `/tags/:boardId/:date`；渲染「索引 + 正文」两栏 | 组件 | 同上 |
| 3 | 1920 / 1100 / 720 打开 | 宽屏两栏与窄屏降级 | 两栏 / 收起索引 / 单栏且无横向溢出；无最左右重复栏 | 组件（resize） | 同上 |
| 4 | 点索引第 3 项 | 分区索引联动 | 正文滚到第 3 分区并高亮 | 组件 | 同上 |
| 5 | 切换 editorial/dark | 双主题渲染 | 全部 surface/文字/边框用语义 token；暗色下雷达/控制台/阅读台无白底白字 | 组件 + 浏览器 | 同上 + ui-verification |
| 6 | 2 active + 1 candidate + 1 未归属 | 按持久话题状态分区 | 三区分别 2/1/1 | 纯逻辑（既有） | `reportPresentation.test.ts`（既有，继承） |
| 7 | 首次展开话题卡再收起再展开 | 展开并缓存话题生命线 | 仅调用一次 `getTopicLifeline`；复用缓存 | 组件 | `BoardDailyReportTimeline.test.ts`（迁移宿主） |
| 8 | 周一/周三有节点、周二无 | identity 连线跨越空白日期 | 贝塞尔跨天线、无周二假节点、弱化不透明度 | 纯逻辑 + 组件 | `BoardDailyReportTimeline.test.ts` |
| 9 | lifeline 接口失败 | lifeline 加载失败 | 话题卡局部错误 + 重试，不影响其他话题/阅读台 | 组件 | 同上 |
| 10 | 两 thread 引用同一文章，依次展开 | 展开 thread 文章并复用缓存 | 只请求一次文章详情；点文章开 60% 抽屉并保留上下文 | 组件 | 同上 |
| 11 | active 话题点「在矩阵中查看这条线」 | 进入完整话题生命线 | 回 `/tags/:boardId?lane=<topicId>` 并选中泳道；无 topic id 隐藏入口 | 组件 | `components/daily-report/DailyReportDeskShell.test.ts` |
| 12 | 阅读台按 Esc | 键盘关闭详情 | 回矩阵并恢复进入前选中泳道；焦点回入口 | 组件 | 同上 |
| 13 | 版块无日报 | 空状态 | 「暂无日报」+ 生成入口 | 组件 | `BoardDailyReportTimeline.test.ts` |
| 14 | days=28 连续两次加载更早 | 加载超过 30 天 | 依次请求 35 / 42 并替换列表（不累加） | 组件 | `BoardDailyReportTimeline.test.ts` |
| 15 | 切换 board | 切换 board 重置状态 | days 重置 14，清理旧 board 展开状态 | 组件 | 同上 |
| 16 | 深链 `/tags/1974/2026-10-23?annotation=8` | 深链定位（A-8） | 打开对应日报并定位批注；失效时退回最新一期 + 提示 | 组件 + 端到端 | 同上、`ui-verification/report.md` |

### 变体走查

| # | 变体 | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 输入：非法日期段 `/tags/1974/not-a-date` | 反查该版块最近一期并 replace + 提示；不白屏 | 组件 | `pages/tags/[boardId]/[date].vue`、`components/daily-report/DailyReportDeskShell.test.ts` |
| 2 | 前置：该版块有日报但指定日期无日报 | 保留该日期空态 + 「看最近一期」动作（不自动跳走，允许当场生成） | 组件 | `DailyReportReader.test.ts` |
| 3 | 时间：跨零点 | 「今天」高亮按 Asia/Shanghai | 纯逻辑 | 同上 |
| 4 | 可用性：详情加载失败 | 错误条 + 重试；队列与右栏保留 | 组件 | 同上 |
| 5 | 可用性：超长 masthead 标题 | 不溢出、不遮挡；ellipsis/换行规则 | 组件 | 同上 |
| 6 | 幂等：重复点同一分区 | 只滚动一次目标，不重复请求 | 组件 | 同上 |

---

## 故事 S6：来源构成（锚 `source-board-hit-rate` MODIFIED）

### 主链路

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 进入版块态势 | 显示来源构成 | 右轨按窗口展示前 3 个来源的篇数与占比条 | 组件 | `console/BoardGovernanceRail.test.ts`（阶段③） |
| 2 | 点「全部 N 个来源」 | 打开完整来源表 | 治理抽屉打开并定位来源页签，含窗口/排序控件 | 组件 | `console/BoardGovernanceDrawer.test.ts`（阶段③） |
| 3 | 版块 #5 → #8 | 切换板块重新拉取 | 按新板块请求并展示；旧响应不覆盖 | 组件 | `console/BoardGovernanceRail.test.ts` |
| 4 | 近 7 天无命中 | 空态引导切窗口 | 「近 7 天没有文章归入本板块」+ 30/90 天提示 | 组件 | 同上 |
| 5 | 查看来源行 | 面板无写动作 | 无停用/退订/改配置控件 | 组件 | 同上 |

### 变体走查

| # | 变体 | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 窗口非法（0/-1/abc） | 回落默认 7 天 | 组件 | `console/BoardGovernanceRail.test.ts` |
| 2 | 来源数 0 / 1 / 3 / >3 | 0=空态；1-3 全列；>3 只列前 3 + 入口 | 组件 | 同上 |
| 3 | 排序切换（篇数↓ / 入板块率↑） | 表格顺序变化，默认篇数降序 | 组件 | `console/BoardGovernanceDrawer.test.ts` |

---

## 故事 S7：2026-10-25 产品复审补充（暗色 / 导航 / 功能可达 / 日期 / 去重）

### 主链路

| 步 | 动作 | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | dark 下逐页打开雷达/控制台/阅读台/编排 | 根底色 `#080c12`，文字与边框可读，无白底白字 | 浏览器 | `system-regression/dark-*-1440.png` |
| 2 | 雷达/控制台/阅读台/研究台点「主页」 | 均回 `/` | 浏览器 | ui-verification A-34 |
| 3 | 雷达空态/正常总览找新建版块 | 两处均有入口；创建成功回新版块 | 组件 + 浏览器 | `BoardRadarView.test.ts` / A-35 |
| 4 | 控制台「更多」逐项 | 扩充版块/匹配回填/匹配规则/标签合并/标签池可打开 | 浏览器 | A-36；`tagMergePreview.test.ts` |
| 5 | 归档默认/自由日期 | 默认数据日；改起止日期重新取数；文章按日期平铺并显示全部标签 | 组件 + 浏览器 | `BoardArchiveView.test.ts` / A-37 |
| 6 | 编排态点击「按天选事件」 | 完整标题+勾选框，点击与画布同步 | 组件 | `BoardComposeOverlay.test.ts` / A-38 |
| 7 | 阅读台 1440/1100/375 | 两栏 / 单栏；无最左历史栏、无右栏演进；日期下拉切换 | 组件 + 浏览器 | `DailyReportReader.test.ts` / A-39 |
| 8 | 全系统 8 路由 × editorial/dark | 无横向溢出、关键功能可达 | 浏览器 | A-40 / `system-regression/` |

### 变体走查

| # | 变体 | 期望答案 |
| --- | --- | --- |
| 1 | 旧功能对照 | 匹配规则/扩充/回填/合并/标签池全部有入口，不再出现「暂未开放」 |
| 2 | 归档日期早于/晚于数据锚点 | 空态明确，不自动跳走；不会写库 |
| 3 | 暗色下打开弹窗/抽屉 | 弹窗/抽屉仍用主题 token，不白底白字 |
| 4 | 375 打开新页面 | 无横向溢出；编排入口提示宽屏 |

## 故事 S8：已批准总览报刊化（FB6 / 8A.1-8A.2，ui-design approved 仅总览）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 打开 /tags 总览 | workshop-radar·打开工坊首屏 | 报头「N 个板块值得关注，M 处出现升温」+ 资料日期；关注行完整横比（序/标题/日报元信息/小矩阵/关注强度/3 日变化/进入） | 组件 | `BoardRadarView.test.ts` 8A describe |
| 2 | 点泳道名/小矩阵格子 | workshop-radar·从雷达进入版块 | 进 `/tags/:boardId?lane=:topicId` 选中泳道 | 组件 | 同上（泳道小矩阵去向） |
| 3 | 点日报标题/今日该读 | workshop-radar·热点行带理由与密度 | 进 `/tags/:boardId/:date` 阅读台；无泳道有日报仍可读 | 组件 | 同上（无泳道有日报） |
| 4 | 常温展开 | workshop-radar·常温版块折叠 | 紧凑索引↔完整横比（含分档与小矩阵），两种去向均进版块 | 组件 | 同上（常温紧凑/展开） |
| 5 | 升温右栏 | workshop-radar·升温判定/无升温 | `N.N×`+理由句+阈值脚注；无升温「暂未触发升温条件」 | 组件 | 同上（升温/无升温） |
| 6 | 数据延迟/不同锚日 | workshop-radar·无数据版块 | 「数据延迟 · 截至 MM/DD」+当日分区口径带日期 | 组件 | 同上（延迟可辨） |

### 本阶段继承表（旧用例 → 处置）

| 旧用例（BoardRadarView.test.ts） | 处置 | 说明 |
| --- | --- | --- |
| 热点行带「今天要看」 | 保留 is-hot/折叠/点行进 L1；badge 文案换「热点」分档 | 已批准原型层级：分档标签随关注强度列（ui-design 冻结） |
| 常温摘要可展开 | 保留展开断言；新增完整横比与两跳去向 | 紧凑索引（原型 cold-digest）+展开横比均为批准形态 |
| 今日该读 | 保留标题断言；新增 10/24 · N 篇文章 · M 个事件元信息 | 原型 story-meta/READING LIST |
| 升温倍数「约 2.7 倍」 | 阈值断言不变；文案换「2.7×」+理由句 | 原型 warming-entry 文案 |
| 无升温「今天没有明显升温的版块」 | 文案换「暂未触发升温条件」+阈值脚注 | 原型文案；阈值仍当日 ≥3 且 ≥2× |
| 加载骨架/错误重试 | 原样保留 | 不动 |
| 全部常温空态 | 「常温 1 个版块」→「常温 · 1 个版块」 | 原型 cold-control 文案；空态主文案不变 |
| 冷启动/新建版块 | 原样保留 | 真实 dialogs/回程不动 |

### 白盒附加（8A）

| # | 条件/分支 | 输入 | 期望 | 测试用例名 |
| --- | --- | --- | --- | --- |
| RA-9 | 分档边界 | 49/50/15/14 | 关注/热点/关注/常温 | `boardRadarHelpers.test.ts` tierLabel |
| RA-10 | 环比前 3 日为 0 | last3>0 / 两者为 0 | 新出现 / 「—」 | `trendOf` |
| RA-11 | 锚日落后 | anchor≠global / delayed | 「数据延迟 · 截至 MM/DD」+当日分区带日期 | `isDelayed` + 组件「延迟可辨」 |
| RA-12 | 14 日轴 | 跨月 10-31/11-01/11-02 | 轴以锚日为终点 | `miniAxis` |
| RA-13 | 小矩阵四档 | 0/max/6·8/1·8 | 0/4/3/1 | `laneLevel` |
| RA-14 | 有旧数据刷新失败 | 建版块后重拉失败 | 错误条+旧行并存不白屏 | `BoardRadarView.test.ts` 旧数据失败 |
| RA-15 | 超长标签 | 60 字标签 | title 携带全文不硬裁剪 | 同上超长标签 |
| RA-16 | 一次请求 | 挂载 | 仅 1 次 overview | 同上 RA-8 落组件层 |

实际命令（8A.3 初次，留档事实）：`cd front && pnpm test:unit app/features/tags/components/console/BoardRadarView.test.ts app/features/tags/components/console/boardRadarHelpers.test.ts --maxWorkers=2` → 2 files / 22 tests 全绿（**15 组件 + 7 helper = 22**；初次报告误记 13+9 拆分，总数 22 真实，R-19 更正）；ESLint 实际命令为 `cd front && pnpm exec eslint app/features/tags/components/console/boardRadarHelpers.ts app/features/tags/components/console/boardRadarHelpers.test.ts app/features/tags/components/console/BoardRadarView.test.ts` → exit 0（初次记录省略了前两文件目录，R-19 补全；仓库 eslint.config.js 未含 vue 插件，6 个 .vue 文件被配置忽略、不称 Vue lint 通过，由父线程统一 typecheck/构建覆盖）。

### R-17/R-18/R-19 修复轮（2026-10-26，8A.1–8A.3 重勾依据）

| # | 条件/分支 | 输入 | 期望 | 测试用例名 |
| --- | --- | --- | --- | --- |
| RA-17 | 首次取数失败 | 首次 overview 即失败 | 错误条 + 中性报头「今日雷达」/「总览读取失败」，无业务空态三要素、无「0 个板块值得关注」、统计隐藏 | `BoardRadarView.test.ts` R-17 describe ×1 |
| RA-18 | 成功空数组 | overview 返回 [] | 真实 0：业务空态 + 「0 个板块值得关注」+ 统计显示 | 同上 ×2 |
| RA-19 | raw→客户端接线 | 真实 snake_case raw | camelCase/字符串 ID 读模型；失败透传不归一 | `api/normalizers/boardRadar.test.ts` ×5 + `api/semanticBoards.test.ts` Radar describe ×2 |
| RA-20 | 派生层 | 分档/升温/今日该读/报头/旧数据保留 | composable 层验证（最便宜层） | `useBoardRadarOverview.test.ts` ×9 |
| RA-21 | 新建版块回程 | 创建成功 | 先重拉总览（第 2 次 overview）再进新版块 | `BoardRadarView.test.ts` 冷启动用例（更新） |

实际命令（修复轮）：
- `cd front && pnpm test:unit app/features/tags/components/console/BoardRadarView.test.ts app/features/tags/components/console/boardRadarHelpers.test.ts app/features/tags/composables/useBoardRadarOverview.test.ts app/api/normalizers/boardRadar.test.ts app/api/semanticBoards.test.ts --maxWorkers=2` → 5 files / 47 tests 全绿（17 组件 + 7 helper + 9 composable + 5 normalizer + 9 API 含新增 2 接线）。
- `cd front && pnpm exec eslint app/features/tags/components/console/boardRadarHelpers.ts app/features/tags/components/console/boardRadarHelpers.test.ts app/features/tags/components/console/BoardRadarView.test.ts app/features/tags/composables/useBoardRadarOverview.ts app/features/tags/composables/useBoardRadarOverview.test.ts app/api/normalizers/boardRadar.ts app/api/normalizers/boardRadar.test.ts app/api/semanticBoards.ts app/api/semanticBoards.test.ts` → exit 0、0 error；.vue 文件仍被既有 TS-only eslint 配置忽略（如实记录，不加 lint 工具）。

文案同步（R-19，仅显示文案不改阈值/统计/来源）：workshop-radar delta 「今天要看」→「热点」分档标记、「约 2.7 倍」→「2.7×」、「今天没有明显升温的版块」→「暂未触发升温条件」，与已批准原型一致；分档 ≥50/≥15、升温当日 ≥3 且 ≥近 3 日均值 2 倍、排序与字段来源均未改。

### 类型边界修复（2026-10-26，父统一 typecheck 发现 TS2322）

- 位置：`app/api/normalizers/boardRadar.ts` `mapRadarOverviewResponse` 失败/无数据分支 `return res`（`ApiResponse<RawOverview>` 不可赋给 `ApiResponse<RadarOverview>`，日志 `.pi/subagents/validation/overview-stage/typecheck.log`）。
- 修法：Radar 分支局部判别返回类型 `RadarOverviewResponse`（成功=归一化读模型；失败/无数据=raw 字段透传），两分支显式构造成员，零断言、不改全局 `ApiResponse` 与其它 API；失败分支 error/status/message/pagination/错误体 raw data 逐字段透传；异常「success 无 data」收敛为 success:false（消费方 useBoardRadarOverview 原本就走失败路径，行为不变）。`useBoardRadarApi().getOverview` 返回类型同步为该局部类型。
- 新增白盒：`boardRadar.test.ts` 失败分支 status/raw data 透传、异常 success 无 data 收敛；共 4 接线用例。
- 实际命令：`cd front && pnpm test:unit app/api/normalizers/boardRadar.test.ts app/api/semanticBoards.test.ts --maxWorkers=2` → 2 files / 16 tests 全绿；`pnpm exec eslint app/api/normalizers/boardRadar.ts app/api/normalizers/boardRadar.test.ts app/api/semanticBoards.ts app/api/semanticBoards.test.ts` → exit 0。全量 typecheck 留父线程统一复查（另 7 处 SignalReportView.test.ts possibly undefined 为其他域旧测试，不扩界）。

### 消费者窄化闭环（2026-10-26，父第二次统一 typecheck 后）

- 父日志 `.pi/subagents/validation/overview-stage/typecheck-final.log`：定义处 TS2322 已消，但判别联合在消费者/测试未窄化——本阶段 11 条：`boardRadar.test.ts:170` 无 guard 取 `res.error`、`semanticBoards.test.ts:149-158` 未判 success 直接取 data.boards camel 字段（9 条）、`useBoardRadarOverview.ts:41` else 读 error（`res.success && res.data` 不能排除成功成员）。
- 修法（仅收敛消费者/测试，不改 UI/全局类型/放宽类型/删断言/双重断言）：① composable 改为直接按 `res.success` 分支（成功成员 data 必填，异常 success-无-data 已在 mapper 收敛为失败，行为不变）；② `semanticBoards.test.ts` 接线用例加显式 guard（`if (!res.success) throw`，expect 不构成窄化）后原 camel 字段断言全保留；③ `boardRadar.test.ts` 异常形态用例补 `if (!res.success)` guard 后保留原断言。
- 闭环验证（log 存 `.pi/subagents/validation/overview-stage/`）：`pnpm exec nuxi typecheck`（typecheck-worker-round1.log）→ 全仓 7 条错误、全部 SignalReportView.test.ts 外域旧错，**本阶段 0 诊断**；`pnpm test:unit app/api/normalizers/boardRadar.test.ts app/api/semanticBoards.test.ts app/features/tags/composables/useBoardRadarOverview.test.ts app/features/tags/components/console/BoardRadarView.test.ts app/features/tags/components/console/boardRadarHelpers.test.ts --maxWorkers=2`（unit-worker-type-closure.log）→ 5 files / 49 tests 全绿（真实分拆：normalizer 7 + API 9 + composable 9 + View 17 + helpers 7）；TS ESLint 6 个受影响 TS 文件 exit 0。未跑全量 unit/build（父统一）；外域 7 错未动。

---

## 效果核对

- 触发原因：雷达排序/升温提示依赖真实分区覆盖率；归档 `in_report` 依赖日报分区线索的真实覆盖；矩阵密度依赖 lane-dynamics 聚合。
- 核对方法：本地真库（board 1974）跑 overview 与 articles 端点，对照 `openspec/changes/.../discussion.md §7` 实测基线；用 agent-browser 打开 `/tags` 与 `/tags/1974` 记录行数/密集列。
- 量化结果（2026-10-25 实测）：雷达 `/tags` 12 个活跃版块 → 4 个关注行 + 常温 8 个折叠、5 条今日该读、无升温；375 下列表单列 `scrollWidth=375`、迷你矩阵隐藏。窄屏版块 1974 → 6 条泳道 × 14 格迷你条=84、无矩阵/右轨、日报入口「看 10-24 日报」。
- 量化结果（2026-10-25 产品复审后，版块 1974）：归档默认数据日 2026-10-25 → 13 篇 / 已入 12 / 未入 1 / 弱证据 6；矩阵 5 活跃 + 1 休眠 × 14 列、54 个分区；编排态 14 个连续日期列（画布 896 < 可视 916）；泳道热度按线索数（美伊 14 section / 38 线索）；雷达 12 活跃 = 4 行 + 常温 8、今日该读 5、升温 0；阅读台两栏 225/600、索引 10 项、日期下拉可切日报、正文独立滚动；1100 收为单栏 + 紧凑日期/锚点条；08-28 深链自动扩窗；10-24 有日报版块 10 个。
- 结论：达标交付 / 瓶颈在上游（候选粒度、相似度）——发现偏差先定位上游，不通过调整 UI 阈值掩盖。

## 继承与调整（MODIFIED / REMOVED Requirements）

| 旧 Scenario | 处置 | 旧测试文件 | 动作 |
| --- | --- | --- | --- |
| board-lane-dynamics「默认展示泳道动态区」 | 改语义（卡片墙 → 矩阵） | `lane-dynamics/LaneDynamicsPanel.test.ts`、`LaneDynamicsCard.test.ts` | 旧测试保留至阶段④；新落点 `console/BoardMatrixView.test.ts`，阶段④删旧卡片测试 |
| 「泳道卡片构成」 | 废止（卡片形态退役） | `lane-dynamics/LaneDynamicsCard.test.ts` | 阶段④删除；矩阵/演进带用例继承其断言（格数/事件/态势句） |
| 「发展时间线结构」 | 继承（卡内时间线 → 演进带） | `lane-dynamics/LaneDynamicsCard.test.ts` | 断言迁移到 `console/BoardLaneEvolution.test.ts` |
| 「一句话态势展示」 | 继承（态势句折叠入口） | `lane-dynamics/LaneDynamicsCard.test.ts` | 迁移到 `console/BoardLaneEvolution.test.ts` |
| 「单日多事件」 | 继承（section 抽屉） | `lane-dynamics/LaneDynamicsCard.test.ts` | 迁移到 `console/SectionDrawer.test.ts` |
| 「只读语义」 | 继承 | `lane-dynamics/LaneDynamicsPanel.test.ts` | `console/BoardMatrixView.test.ts` 增加零写请求断言 |
| source-board-hit-rate「显示来源构成 / 切换板块重新拉取 / 空态引导切窗口 / 面板无写动作」 | 改宿主（tab 底部 → 右轨 + 抽屉） | `BoardSourcePanel.test.ts` | 面板组件保留至阶段④；阶段③把宿主断言迁移到 `BoardGovernanceRail/Drawer.test.ts`，`BoardSourcePanel.test.ts` 保持绿灯（回归网） |
| daily-report-system「日报时间线组件（报纸布局）」 | 改语义（fixed 浮层 → 路由阅读台两栏） | `BoardDailyReportTimeline.test.ts` | 迁移宿主断言到 `components/daily-report/DailyReportDeskShell.test.ts`；reader 内部用例（页边注/lifeline/深链）原样保留在 `BoardDailyReportTimeline.test.ts` |
| 「进入完整话题生命线」 | 改语义（切 tab → 回矩阵选中泳道） | `BoardDailyReportTimeline.test.ts`、`BoardThreadBrowser.focus.test.ts` | 阶段②/④迁移；`BoardThreadBrowser.focus.test.ts` 保留至组件退役 |
| 「加载超过 30 天」「切换 board 重置状态」 | 继承（不改语义） | `BoardDailyReportTimeline.test.ts` | 照跑（回归网） |
| board-article-api「按 board 查询文章列表 / filtered_tags 按板过滤 / 分页和排序」 | 继承（仅新增字段与过滤，旧语义不变） | `semantic_board_handler_test.go` | 照跑 + 新增 `board_article_handler_test.go` |
| TagsPage「五个平级内容 tab」宿主断言 | 改语义（阶段④ tab 退役） | `TagsPage.test.ts` | 阶段①–③ 照跑（旧页仍渲染）；阶段④随路由切换改断言或随组件退役删除并留痕 |

## 白盒附加

### 分支表 — 路由与选中状态（R）

| # | 条件/分支 | 输入 | 期望 | 测试用例名 |
| --- | --- | --- | --- | --- |
| R-1 | board 深链映射 | `/tags?board=1974` | `router.replace('/tags/1974')`，移除 `board` | `console redirect: board deep link` |
| R-2 | report 深链桥接 | `?board=1974&report=1211&annotation=8` | 阶段①不重定向（旧页承载）；阶段②映射 `/tags/1974/2026-10-23?annotation=8` | `console redirect: report deep link stays` |
| R-3 | board 非法 | `abc` / `0` / `-3` / 缺失 | 不重定向 | `console redirect: invalid board` |
| R-4 | lane 合法 | `?lane=55` 且存在 | `selectedLaneId=55` | `useBoardConsole: lane from query` |
| R-5 | lane 非法 | `9999` / `abc` / 空串 | 回退线索数最多者；并列取 topic_id 最小 | `useBoardConsole: invalid lane fallback` |
| R-6 | 选中写 query | 点 lane B | replace query `lane=B`，保留其它键 | `useBoardConsole: select writes lane` |
| R-7 | 切版块清场 | A → B | 抽屉关、休眠组收起、视图回矩阵、选中默认 | `BoardConsoleShell: reset on board change` |
| R-8 | 面包屑 | 点击 | `navigateTo('/tags')` | `BoardConsoleShell: breadcrumb` |
| R-9 | 视图切换 | 矩阵↔归档 | 选中泳道/窗口保持 | `BoardConsoleShell: view switch keeps state` |
| R-10 | 选中泳道为休眠 | `?lane=` 指向休眠 | 自动展开休眠组并选中 | `BoardMatrixView: dormant lane selection` |

### 分支表 — 矩阵派生（M）

纯函数：`buildDayAxis(endDate, days)` / `laneCellCount` / `splitDormantLanes`。

| # | 条件/分支 | 输入 | 期望 | 测试用例名 |
| --- | --- | --- | --- | --- |
| M-1 | 轴长 | end=2026-10-23, days=14 | 14 个日期，升序，末位=end | `axis: length` |
| M-2 | 跨月 | end=2026-11-03, days=14 | 首日 2026-10-21，连续 | `axis: cross month` |
| M-3 | 今天列 | date==today | 恰 1 列 `isToday` | `axis: today column` |
| M-4 | 格值 | 当日 3 section | 3 | `matrix: cell count` |
| M-5 | 空格 | 当日 0 | 空串 + empty class | `matrix: gap cell` |
| M-6 | 行尾合计 | [1,0,2] | 3 | `matrix: row total` |
| M-7 | 密度分档 | 1/2/3/4/5 | b1/b2/b3/b4/b4 | `matrix: intensity` |
| M-8 | 窗口切换 | 14→7→30→全部 | days=7/14/30/365；轴长一致 | `useBoardConsole: window switch` |
| M-9 | 轴外数据 | 响应含轴外日期 | 不计入格值/合计 | `matrix: out-of-axis ignored` |

### 分支表 — 休眠（MD）

| # | 条件/分支 | 输入 | 期望 | 测试用例名 |
| --- | --- | --- | --- | --- |
| MD-1 | 阈值 0 | 0 section | 休眠 | `dormant: boundary 0` |
| MD-2 | 阈值 2 | 2 section | 休眠（含 2） | `dormant: boundary 2` |
| MD-3 | 阈值 3 | 3 section | 主矩阵 | `dormant: boundary 3` |
| MD-4 | 默认折叠 | 有休眠 | 不渲染行 + 「休眠泳道 N」 | `matrix: dormant collapsed` |
| MD-5 | 展开/收起 | 点折叠条 | 弱化行出现/消失 | `matrix: dormant toggle` |
| MD-6 | 不写状态 | 折叠/展开 | 写请求 0；响应未变异 | `matrix: dormant read-only` |
| MD-7 | 阈值随窗口 | 7 天 2 个 / 14 天 5 个 | 7 天休眠、14 天不休眠 | `dormant: window dependent` |

### 分支表 — 演进带（E）

| # | 条件/分支 | 输入 | 期望 | 测试用例名 |
| --- | --- | --- | --- | --- |
| E-1 | 节点大小 | 1/2/3/5 → mid/mid/big/big；0 空心 | 档位正确 | `evolution: node size` |
| E-2 | 标签阈值 | 2 不显示 / 3 显示 | 阈值方向正确 | `evolution: label threshold` |
| E-3 | 最密集三天 | 计数并列 | 计数降序→日期降序 | `evolution: densest tie` |
| E-4 | 今日事件 | 今天有/无 section | 列出事件 / 提示无分区 | `evolution: today events` |
| E-5 | 无快照 | snapshot=null | 待结算占位 + 时间线照常 | `evolution: no snapshot` |
| E-6 | 长句折叠 | summary>80 字 | 默认折叠 + 可展开 | `evolution: long summary collapse` |

### 分支表 — Section 抽屉（D）

| # | 条件/分支 | 输入 | 期望 | 测试用例名 |
| --- | --- | --- | --- | --- |
| D-1 | 空格 | 点空 cell | 提示，不开抽屉 | `drawer: empty cell` |
| D-2 | 命中格 | 2 个 section | 标题 + 全部 section/事件 | `drawer: content` |
| D-3 | 截断提示 | folded_count=3 | 「另有 3 条未载入」 | `drawer: folded count` |
| D-4 | 日报线索 | section.id 命中 | thread 标题 + 文章数 | `drawer: report clues` |
| D-5 | 关闭 | Esc/遮罩 | 关闭且选中泳道不变 | `drawer: close keeps lane` |
| D-6 | 定位 | 渲染 | Teleport body；fixed；32rem≤92vw | `drawer: overlay anchor` |
| D-7 | 只读 + 动作 | 抽屉内 | 无生命周期写操作；两动作存在（打开日报 / 看归档），无「加入新泳道」 | `drawer: read only` |

### 分支表 — 归档（A）

| # | 条件/分支 | 输入 | 期望 | 测试用例名 |
| --- | --- | --- | --- | --- |
| A-1 | 首条标签归组 | tags=[T1,T2] | T1 组 + `+1` | `archive: group by first tag` |
| A-2 | 未打标 | tags=[] | 「未打标」组 | `archive: untagged group` |
| A-3 | 排序一级 | 跨日期 | 日期分组倒序，同日完整发布时间倒序 | `archive: full timestamp sort` |
| A-4 | 排序二级 | 同含遗漏 | 最新日期降序 | `archive: sort newest` |
| A-5 | 排序三级 | 日期相同 | 篇数降序 | `archive: sort size` |
| A-6 | 筛选后排序 | not_in_report | 顺序规则不变 | `archive: sort stable under filter` |
| A-7 | 统计条 | 20 篇 12 入 | 20/12/8/弱证据数/组数 | `archive: stats` |
| A-8 | 只读 | 任意文章 | 无写动作 | `archive: read only` |

### 分支表 — 雷达（RA）

| # | 条件/分支 | 输入 | 期望 | 测试用例名 |
| --- | --- | --- | --- | --- |
| RA-1 | 排序 | 80/50/20 | 降序；并列 board id 升序 | `radar: sort` |
| RA-2 | 热点边界 | 49/50 | <50 不带标记 | `radar: hot threshold` |
| RA-3 | 常温边界 | 14/15 | <15 折叠 | `radar: cool threshold` |
| RA-4 | 升温 | 8 vs 日均 3 | 2.7 倍 | `radar: warming ratio` |
| RA-5 | 升温阈值 | 今日 2 / 倍数 1.99 | 不列入 | `radar: warming threshold` |
| RA-6 | 无升温 | 无满足 | 固定文案 | `radar: no warming` |
| RA-7 | 全零版块 | 计数 0 | 常温 + 无密度条 + 今天暂无日报 | `radar: zero board` |
| RA-8 | 单请求 | 首次加载 | 只调 overview | `radar: single request` |

### 分支表 — 编排（C）

| # | 条件/分支 | 输入 | 期望 | 测试用例名 |
| --- | --- | --- | --- | --- |
| C-1 | 采纳预填 | 2 个 section | 名称=label，成员=2 | `compose: adopt prefill` |
| C-2 | 预勾上限 | 25 个 section | 前 20 | `compose: adopt cap 20` |
| C-3 | 确认启用 | 1 天/2 天 | disabled+提示 / 可点 | `compose: enable threshold` |
| C-4 | 平均距离无数据 | 无 relation | 「—」 | `compose: avg distance none` |
| C-5 | 平均距离 | 0.1/0.2/0.4 | 0.233 | `compose: avg distance` |
| C-6 | 离群 | 0.35/0.351 | 不计/计 | `compose: outlier boundary` |
| C-7 | 搜索 | 关键词/空白/大小写 | 过滤/不过滤/不敏感 | `compose: search` |
| C-8 | 已中断 | last_seen < today-2 | 折叠组 | `compose: interrupted fold` |
| C-9 | 保存校验 | 空成员/空名/齐备 | 提示+不发请求 / 提示+不发 / 创建 1 次 | `compose: save validation` |
| C-10 | 取消 | Esc/取消 | 不发请求、状态不变 | `compose: cancel` |
| C-11 | 关联推荐 | relations 5 条 | ≤6 连线、距离升序、3 位小数 | `compose: relations` |
| C-12 | 无关联 | 无 relation | 提示 + 无线 | `compose: no relations` |

### 分支表 — 阅读台（P）

| # | 条件/分支 | 输入 | 期望 | 测试用例名 |
| --- | --- | --- | --- | --- |
| P-1 | 两栏 | 1920 | 索引 + 正文全渲染，无右栏/左历史 | `desk: two columns` |
| P-2 | 收起索引 | 1100 | 无索引栏 + 锚点条 | `desk: collapse index` |
| P-3 | 两区 | 900 | 正文 + 右栏 | `desk: two zones` |
| P-4 | 单栏 | 720/375 | 单栏且无横向溢出 | `desk: single column` |
| P-5 | 索引联动 | 点第 3 项 | 滚到第 3 分区 + 右栏强调 | `desk: index jump` |
| P-6 | scroll-spy | 滚到分区 3 | 索引高亮第 3 项（互斥） | `desk: scroll spy` |
| P-7 | Esc 返回 | 按 Esc | 回矩阵 + 恢复选中泳道 | `desk: esc back` |
| P-8 | 深链 | `/tags/1974/2026-10-23?annotation=8` | 打开并定位；失效退回最新 + 提示 | `desk: deep link` |
| P-9 | 空态 | 无日报 | 生成入口 | `desk: empty` |
| P-10 | 切版块 | 队列点另一版块 | 路由切换，days 重置 14 | `desk: switch board` |

### 分支表 — 后端契约（B）

| # | 条件/分支 | 输入 | 期望 | 测试用例名 |
| --- | --- | --- | --- | --- |
| B-1 | overview 正常 | 3 个 active | 3 行，字段齐备 | `TestBoardOverview_HappyPath` |
| B-2 | overview 零值 | 无日报版块 | 0 值 + digest=null + 200 | `TestBoardOverview_ZeroValues` |
| B-3 | overview 超时 | 单版块 >5s | 该行 0 值 + degraded=true | `TestBoardOverview_TimeoutDegrade` |
| B-4 | overview 只读 | 调用一次 | 写探针 0 | `TestBoardOverview_ReadOnly` |
| B-5 | in_report | 6 份日报覆盖 12/20 | 12 true | `TestBoardArticles_InReportWindow` |
| B-6 | 非法报告状态 | bogus | 400，不静默当 all | `TestBoardArticles_InvalidReportStatus` |
| B-7 | weak 过滤 | 三类弱证据 | 仅返回三类 | `TestBoardArticles_WeakFilter` |
| B-8 | 窗口边界 | 窗口外线索 | `in_report=false` | `TestBoardArticles_WindowBoundary` |

### 分支表 — 跨模块不变量（X）

| # | 不变量 | 断言方式 | 测试用例名 |
| --- | --- | --- | --- |
| X-1 | 矩阵/演进带零写操作 | 交互期间写请求计数 0 | `matrix: read-only` |
| X-2 | 右轨失败不阻塞矩阵 | 体检 500 时矩阵仍渲染 | `rail: failure isolated` |
| X-3 | 工坊首屏 | `/tags` 无参数渲染雷达，不渲染旧五 tab | `pages/tags/index.vue` / `BoardRadarView.test.ts` |
| X-4 | 深链不丢参数 | 路由切换保留非本层 query | `console: query preserved` |
| X-5 | 窄屏不破版 | 375 下 scrollWidth≤375 | `ui-verification` 窄屏截面 |

### 边界值清单

| 对象 | 边界 | 判定 |
| --- | --- | --- |
| 窗口天数 | 7 / 14 / 30 / 365 | 轴长等于天数；请求参数一致 |
| 休眠阈值 | 0–2 / 3 | ≤2 休眠；≥3 主矩阵 |
| 密集节点标签 | 2 / 3 | ≥3 显示 |
| 雷达热点 | 49 / 50 | ≥50 热点 |
| 雷达常温 | 14 / 15 | <15 折叠 |
| 升温 | 倍数 1.99 / 2.0；今日 2 / 3 | 倍数 ≥2 且今日 ≥3 |
| 预勾上限 | 19 / 20 / 21 | 最多 20 |
| 确认启用 | 连续 1 / 2 天 | ≥2 可点 |
| 离群 | 0.35 / 0.351 | >0.35 计离群 |
| 阅读台断点 | 1200/1201；1000/999；768/767.98 | 见 P-1..P-4 |
| in_report 窗口 | 第 6 / 第 7 份日报线索 | 仅前 6 份计入 |
| 时间 | Asia/Shanghai 当天 23:59:59 / 次日 00:00:00 | 「今天」归属切换 |

### 不适用划除（留痕）

- 并发/线程安全：本 change 无新增并发写路径（归档/矩阵只读；`in_report` 派生查询无写）；官编排创建沿用既有端点，不声称线程安全。
- 输入变体中的「纯分隔符/特殊字符」：仅编排搜索框与归档筛选涉及文本输入，已并入 C-7/S3-变体 1；其余界面无自由文本输入。
- 时区归一化：矩阵与雷达「今天」按 Asia/Shanghai；不覆盖用户改写系统时区场景（单用户本地部署）。
- board-article-api 的 SQL 性能：不在白盒分支内，由索引 + 窗口上限（30 天）约束；超过 30 天窗口的异步计数属后续 change（design.md Risks）。

### R-20 移动端 CSS 作用域回归修复（2026-10-27，父真实浏览器验收发现）

- 现象：390 视口 `document.scrollWidth=514`（375 暗色同）；`.rd-story` 宽 361 但 computed grid 仍 6 列，`attention-wrap` right≈514。根因：Vue scoped 样式会命中子组件根节点——父 `BoardRadarView.vue` 的 `.rd-grid`（基础 6 列定义）覆盖了 `BoardRadarStory` 根的 ≤767.98 三列定义。
- 修法（仅本页 CSS 作用域，不动 JS/API/字段/主题）：父列定义全部限定 `.rd-labels`（基础 6 列并入 `.rd-labels` 规则、768–1439.98 五列同样改 `.rd-labels`、模板表头去掉 `rd-grid` class）；子 Story 的 `.rd-grid` 三断点定义自包含不变。桌面 ≥1440 表头/行列宽模板一致对齐不变；≥768 渲染零变化。
- 验证方式：CSS 几何无法由单测/typecheck 覆盖，留父线程 generate 后真实 375/390/1440/2560 测量（`ui-verification/overview-stage-2026-10-27/`）。

### 最终 UI 闭环（2026-10-27，父真实验收 + K3 视觉独审后局部修复）

- R-17 补全：首次失败（浏览器 abort 实测）主舞台已中性但阅读右栏仍出现「今天还没有日报/暂未触发升温条件」业务空结论 → `BoardRadarView` 以 `v-if="!firstLoadFailed"` 不渲染 `BoardRadarReadingRail`（有旧 rows 保留、成功空集照旧）；组件用例补右栏否定断言（失败态不渲染/无该文案）+ 成功空集右栏照常 + 旧数据刷新失败右栏保留，旧断言零删除。
- R-21：移除本页 `.radar` 的 `max-width:120rem; margin:0 auto`（2560 不再被压成居中 1920 阅读列），沿 workspace 可用宽度；不改全局 shell、不拉行高。
- 报头「更多」→「全局治理」（对齐已批准原型）；375/390 统计组与按钮自然换行（actions/meta flex-wrap、summary nowrap），不缩字不藏字段。
- Masthead 两处 P2：≤400 主标题 `text-wrap:balance`（消尾部孤「温」）；副注删「。·数据截至」间隔号（改「。数据截至…」，语义不变）。
- 命令：`cd front && pnpm test:unit app/features/tags/components/console/BoardRadarView.test.ts --maxWorkers=2` → 1 file / 18 tests 全绿（17→18，新增旧数据刷新失败右栏保留用例）；`pnpm exec eslint app/features/tags/components/console/BoardRadarView.test.ts` → exit 0。几何/右栏/HTTP envelope 空集模拟留父一次 generate 复验（父此前空集模拟误用无 success/data 的 raw body 被客户端正确判失败，接口不改）。

---

## 故事 S9：矩阵/追踪阶段（8B，2026-10-27 批准后；阶段账本另立）

用户 2026-10-27 批准 scope-cleanup 矩阵/追踪原型（`ui-approval-matrix-stage.md`）并选定观察窗 A 路线「名单稳定」（design D11）。本阶段完整用例账本另立 **`test-cases-matrix-stage.md`**（主链路 S9-1…25、五组变体、继承与调整表、白盒 OX/MX/EV/CH/RD/CA/RS 分支表、真实浏览器计划），本节只做入口索引，不重写上文任何旧账：

- **旧 S1 中被本阶段改语义的条目**（S1-2/3/4/6/9/10/11/18/19、变体 5/12/13、M-4/M-5/M-6、E-3/E-4/E-6、D-1…D-7、MD 组）在阶段账本「继承与调整」表逐行处置；处置前的旧断言在阶段代码落地前仍描述现网行为，不算失效。
- **未被本阶段触及的故事 S2–S8 照旧**；根账 53 pass / 1 fail / 2 未覆盖与 A-ID 未验收责任继续留账，UI 批准不洗绿。
- 阶段合同/组件映射/字段锚/互斥分工见 **`implementation-matrix-stage.md`**（8B.2 产出，本批未改任何产品代码）。
- 2026-10-27复核：OX-1/2纠正为A=sourceDays唯一名单/前端source全轴排序/休眠/候选基准，B365仅补同ID历史，恰7/30/365日也不切base；OX-12…14补A/B分层失败，CA-1…8补365去重及报告14/文章20有界缓存/当前读项稳定。研究入口按用户明确选择承接真实研究工作台，不洗绿旧关系/关键事件。

---

## 故事 S10：日报文章预览的状态同步（2026-10-28 用户反馈修复）

锚 `daily-report-system` §阅读台文章抽屉与站内阅读预览的状态同步。

**现象**：日报阅读台（`/tags/:boardId/:date`）→ 文章抽屉 →「站内阅读」预览弹窗里点收藏毫无反应——`DailyReportDeskShell.vue` 把 `@favorite` / `@article-update` 接成空函数，事件被吞掉，既不落库也不改界面；矩阵页日报时间线预览虽已接线，但星标要等接口返回才翻转，连点会丢更新。

### 主链路

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 阅读台点开文章 → 抽屉「站内阅读」 | 站内阅读预览 | 预览弹窗打开当前文章 | 组件 | `daily-report/DailyReportDeskShell.test.ts` |
| 2 | 预览内点收藏 | 收藏即时生效 | 星标立即变已收藏（不等接口返回） | 组件 | 同上 |
| 3 | 收藏接口返回成功 | 持久化 | 文章更新接口被调用一次，载荷 `{favorite: true}` | 组件（mock api） | 同上 |
| 4 | 收藏接口失败 | 失败回滚并提示 | 星标回滚为点击前状态 + 出现错误提示 | 组件 | 同上 |
| 5 | 请求未回时再点一次 | 连点不丢更新 | 界面按最新一次点击展示；先到的失败响应不回滚新状态 | 纯逻辑 | `features/articles/composables/useArticleFavoriteToggle.test.ts` |
| 6 | 矩阵页日报时间线点文章 → 预览点收藏 | 同一契约 | 同样乐观翻转 + 失败回滚 | 组件 | `composables/useTagsPage` 宿主断言（`useTagsPage` 无独立用例文件，随 `TagsPage.test.ts` 宿主覆盖） |

### 变体走查

| # | 变体 | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 预览未打开文章就点收藏 | 不发请求、不改状态（无当前文章即忽略） | 纯逻辑 | `useArticleFavoriteToggle.test.ts` |
| 2 | 收藏事件的文章 id 与当前预览文章不一致 | 忽略该事件，不误改当前文章 | 纯逻辑 | 同上 |
| 3 | 接口返回 `success: false` | 视同失败：回滚 + 错误提示 | 组件 | `DailyReportDeskShell.test.ts` |
| 4 | 接口抛异常（网络断） | 视同失败：回滚 + 错误提示 | 组件 | 同上 |
| 5 | 连点三次收藏 | 终态为「与点击次数奇偶一致」，且以最后一次点击为准 | 纯逻辑 | `useArticleFavoriteToggle.test.ts` |

### 白盒分支

| ID | 分支 | 期望 |
| --- | --- | --- |
| FA-1 | 乐观更新时写回宿主 | 调用 `apply(next)` 一次，next.favorite = !current.favorite |
| FA-2 | 接口成功 | 不再调用 `apply`（不回滚），无提示 |
| FA-3 | 接口失败 | 仅当宿主当前值仍等于本次乐观值时才回滚（避免覆盖后续点击）；随后 `notify.error` |
| FA-4 | 文章 id 不匹配 | 直接返回，不调用 `apply`、不发请求 |

### S10 补充：弹窗内「全屏」的层级（2026-10-28 用户反馈修复，同一反馈批次）

现象：站内阅读预览弹窗里点「全屏」毫无反应——`ArticleContentView` 的全屏层（Teleport 到 body）硬编码 `z-[100]`，而弹窗遮罩（`AppDialog`）是 `9100` 且整屏覆盖，全屏层被压在遮罩下（实测 `elementFromPoint(视口中心)` 命中的仍是 `.preview-body`，全屏层矩形虽已是整屏但不可见）。

| 步 | 动作 | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 预览弹窗内点「全屏」 | 全屏层覆盖视口且层级 > 遮罩（9200 > 9100） | 组件 | `tags/components/ArticlePreviewModal.test.ts` |
| 2 | 全屏中视口中心命中元素 | 落在全屏层内（非弹窗内容区） | 浏览器 | `ui-verification/desk-preview-favorite-2026-10-28/report.md` |
| 3 | 全屏层取值 | 低于全局 toast（9999）——全屏中失败提示可见 | 组件 | `ArticlePreviewModal.test.ts` |
| 4 | 正文组件层级来源 | 取自 `fullscreenZIndex` prop；不得回迁硬编码 `z-[100]` | 源码锚 | `articles/components/ArticleContentView.test.ts` |
| 5 | 阅读页（非弹窗宿主）全屏 | 层级行为不变（缺省 100） | 组件 + 浏览器 | 同 4（prop 缺省断言）+ 宿主回归用例 |

变体：

| # | 变体 | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 不传 `fullscreenZIndex` | 缺省 100，阅读页原地全屏零变化 | 源码锚 | `ArticleContentView.test.ts` |
| 2 | 宿主传层级低于遮罩 | 组件不自行纠正（层级是宿主职责），但弹窗宿主固定传 9200 | 组件 | `ArticlePreviewModal.test.ts` |
