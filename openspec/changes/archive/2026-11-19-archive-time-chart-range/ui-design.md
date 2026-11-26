<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# UI 设计契约：归档时间导航块

本 change 不新增页面 / 路由 / 导航层级，只在既有归档视图（`/tags/:boardId` → 顶栏「文章归档」）内把日期筛选从「快捷 chip + 裸日期输入」换成「一条带轴折线 + 一条全历史导航条」，布局模式与页面骨架不变。依据 `docs/reference/standard/frontend/interaction.md` 模式词表：主图是 `panel` 内的 `canvas` 语义（依赖日期轴上多个位置的相对关系 → 区域框选与拖动），导航条同属该 `canvas`，不引入新模式。

## 入口与入口变更

- 入口不变：版块控制台顶栏视图 seg「文章归档」（`BoardConsoleHeader.vue`，`data-testid` 保持），URL 无新增键。
- 归档视图内的变化：
  - 工具行（`ar-toolbar`）**移除**「数据日 / 近 7 天 / 近 14 天 / 近 30 天」chip 与「从 / 到」两个日期输入；保留组内排序、全部/未纳入日报/弱证据筛选、⚙筛选、ⓘ匹配流程。
  - 工具行**下方**新增时间导航块（`data-testid="archive-time-nav"`），自上而下三段：日期表单（复用 `RangeDateForm`：从 / 到 + 校验反馈）→ 90 天视窗主折线（`TrendAxes` 外置轴 + 选区高亮 + 双端手柄）→ 全历史导航条（`data-testid="archive-history-strip"`，含「最早日」「今天」两端标签与可拖动视窗窗口）。
  - deck 行（范围 / 篇数 / 已进日报比例 / 未进 / 弱证据）不变，仍显示当前范围的起止与计数。
- 默认状态：日期 = 日历今天（Asia/Shanghai）单日；视窗 = 以今天为右端的 90 天；从矩阵历史格子「看归档」带入 `initialDate` 时，日期 = 该日、视窗 = 以该日为右端（不足 90 天时向前补到最早有文章日为止）。

## 受影响状态

| 状态 | 触发 | 界面表现 |
| --- | --- | --- |
| loading（曲线） | 首次进归档 / 切版块，`article-daily-counts` 请求中 | 主图反馈行显示「时间范围加载中…」（组件按标题生成）、导航条置禁用态；日期表单与工具行照常可用，列表照旧走自己的 loading；曲线失败不得阻塞列表 |
| empty（曲线） | 该版块从无命中文章（`earliest_date` 为空） | 主图不画 0 基线（`series` 传 null，走「时间范围趋势未取到」分支）并在反馈行给「本版块还没有命中文章，暂无时间曲线」；导航条只留两端标签；日期表单与列表按今天单日正常请求 |
| error（曲线） | 计数端点失败 | 导航块上方右对齐的错误行「时间曲线加载失败」+「重试」；**列表与 deck 不受影响**（range 仍由今天兜底），重试只重拉计数 |
| error（列表） | 既有行为不变 | 导航块保持可用（可换日期重试），列表区既有错误条 + 重试 |
| success | 计数与列表就绪 | 主图折线 + 选区高亮 + 手柄 + 两端日期刻度；导航条显示全历史轮廓与视窗窗口位置；每天 0 篇的日子按基线 0 绘制（已知 0，不桥接、不省略） |
| 手势中（预览） | 拖空白 / 拖选区 / 拖手柄 | 主图选区实时跟随并显示「MM-DD — MM-DD · 松开应用」反馈；**列表不重拉**，deck 数字保持上次提交值 |
| 导航条拖动中 | 拖视窗窗口 / 点导航条 | 视窗与已提交区间一起平移（区间宽度不变），主图与日期表单实时跟随；松手/键盘步进提交一次并重拉列表；Esc / pointercancel 回到拖动前 |
| 窄屏（<768px） | 既有窄屏断点 | 导航块整块默认折叠为一行摘要（「时间范围 · N 天 · MM-DD~MM-DD」，取**已提交区间**，与 deck 起止一致）；点开展开；折叠偏好独立持久化，不与矩阵页共享 |

## 复用组件与布局模式

- 布局模式：**不变** —— 页面仍是 `AppPageShell mode="workspace"`（`front/app/pages/tags/[boardId]/index.vue`），归档视图仍在控制台主舞台列内，宽度由舞台决定，不写自由宽度、不新增 fixed 定位层。
- 主图复用 `ObservationRangeControls.vue`（矩阵页同一组件）：刷选状态机 `rangeBrush.ts`、几何 `brushCurve.ts`、轴 `TrendAxes.vue`、命中层 `RangeHandleHits.vue`、日期表单 `RangeDateForm.vue`、测宽 `usePlotWidth.ts` 全部沿用，**不复制手势代码**。本 change 只给它加**缺省即现状**的可选项：
  - `unitLabel`（缺省 `线索数`）→ 归档传 `篇`；
  - `title`（缺省 `时间视窗`）→ 归档传 `时间范围`；
  - `collapseStorageKey`（缺省 `syntopica.orc.collapsed`）→ 归档传 `syntopica.archive.time.collapsed`；
  - `footer` 具名插槽（缺省不渲染任何 DOM）→ 归档注入导航条；
  - 无曲线提示文案改为按 `title` 生成（矩阵仍读作「时间视窗趋势未取到」，归档读作「时间范围趋势未取到」，不再出现「365 日池」字样）；
  - `tiers` 容器加 `v-if="tiers.length"`，归档传 `[]` 时不留空的无障碍分组。
- 导航条新增 `ArchiveHistoryStrip.vue`：受控纯 UI（`earliest` / `latest` / 计数 / `viewport` → emit `viewport-preview` / `viewport-commit` / `viewport-cancel`），内部只做像素↔日期换算与拖动，不请求 API。视窗平移时**已提交区间同步平移**（保持相对位置），因此主图永远不会收到「区间在视窗之外」的态（`ObservationRangeControls` 按下标高亮，负下标会画歪）。
- 数据派生新增 `archiveTimeSeries.ts` 纯函数（稀疏 `days[{date,count}]` + `earliest_date` → 视窗切片序列 / 导航条分桶 / 缺日补 0），与 `laneTrendSeries.ts` 的「null 不补 0」约定**不同**：归档图口径是「整段历史都已查询」，缺口即已知 0，可以画基线；该差异在图注与代码注释里写明，避免两套口径混淆。
- 基础组件复用：日期输入复用 `RangeDateForm`（内部 `AppInput`），按钮复用既有 `ar-*` 样式类，浮层不新增。

## 验收映射

| 验收点 | 证据载体（组件测试） |
| --- | --- |
| 默认日期 = 日历今天；`initialDate` 优先；`asOf` 不再作默认 | `front/app/features/tags/components/console/BoardArchiveView.test.ts`（改「默认只查数据锚点当天」用例） |
| 快捷 chip 已移除、工具行不再有日期输入 | `front/app/features/tags/components/console/BoardArchiveView.test.ts`（`archive-preset-1` 不存在、`archive-start-date` 不再渲染） |
| 主图提交 range 才重拉列表（预览不重拉） | `front/app/features/tags/components/console/BoardArchiveView.test.ts`（stub 主图 emit commit / preview，断言请求次数与 `start_date`/`end_date`） |
| 导航条拖动平移视窗、区间同步平移、Esc 取消 | `front/app/features/tags/components/console/ArchiveHistoryStrip.test.ts` |
| 稀疏计数 → 视窗切片 / 导航条分桶 / 缺日补 0 | `front/app/features/tags/components/console/archiveTimeSeries.test.ts` |
| `ObservationRangeControls` 新参数缺省时矩阵行为不变（单位仍「线索数」、标题仍「时间视窗」、存储键不变） | `front/app/features/tags/components/console/ObservationRangeControls.test.ts`（补缺省用例） |
| 曲线口径 = 全版块命中文章数，不随筛选变化 | `front/app/features/tags/components/console/BoardArchiveView.test.ts`（切筛选后计数请求参数与 `dailyCounts` 不变） |
| 曲线加载失败不阻塞列表、可单独重试 | `front/app/features/tags/components/console/BoardArchiveView.test.ts` |
| 后端聚合口径与列表成员一致、稀疏返回、空版块 | `backend-go/internal/tagmanagement/handler/board_article_handler_test.go` |
| 主链路走查（在主对话用手工点检记录于 tasks 勾选备注）：切到归档 → 见今天 1 天 → 拖选 5 天 → 列表/deck 同步；导航条拖到去年后主图与日期表单跟随 | 由上述组件测试逐条覆盖可机械断言的部分，视觉/手感部分在实现完成后手工点检并在汇报中说明（minor 档不建 `ui-verification/`） |

> 说明：minor 档不产出 `ui-prototype/`，`ui-approval: not-required`。任何改变信息架构或主流程的修订（例如把导航块提到页面级顶栏）MUST 回到本文件按 major 重新评审。
