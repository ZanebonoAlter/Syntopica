<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# 事件线退役的 UI 复用合同

> 本次只替换既有面板里的**口径与内容**，不新建页面 / 不新建独立面板 / 不引入新交互流程：雷达首屏、版块态势矩阵、日报阅读台刊头三处沿用现有结构与既有交互模式，故 ui-impact=minor，approval=not-required，prototype=none。

## 入口与入口变更

| 入口 | 现状 | 变更后 | 触发的实际状态 |
| --- | --- | --- | --- |
| 雷达首屏行内小矩阵（`BoardRadarStory.vue` `laneCells`） | 格值 = 事件线数（缺失时按分区数兜底），色阶按行内最大值四档 | 格值 = 当天该泳道线索数；**取消色阶通道**，格内数字 + title/aria 承载全部信息 | 无灰度态；旧后端缺该字段时按「线索合计」显示并在 aria 标注「线索数未知」，不显示空白冒充 0 |
| 雷达首屏右栏（`BoardRadarReadingRail.vue`） | 「今日该读」跨版块事件线清单（上限 8，行内「新起 / 升温 N×」徽标）+「跨天延续建设中」脚注 | 换成「当天线索最多的泳道」清单：每条 = 版块名 + 泳道名 + 当天线索数；徽标与脚注整段退场 | 清单为空时显示既有空态占位；某版块数据延迟时该版块条目缺席并标注「本次清单不完整」 |
| 雷达首屏行头（`BoardRadarStory.vue` `todayCount` / `anchorText`） | 「N 条事件线」 | 「N 条线索」 | 无 |
| 雷达分档徽标（`boardRadarHelpers.ts` `tierLabel`） | 热点 / 关注 / 常温（阈值 3.3 / 0.7） | 分档名不变，阈值按新关注度量纲的真库分布重标定 | 阈值未重标定前不得上线新量纲分档（实现阶段由 tasks 的重标定任务把关） |
| 版块态势矩阵（`observationLanes.ts` `matrixCellView` / `ObservationMatrixLaneRow.vue`） | 格值 = 事件线数（缺失时按线索合计兜底）+ 颜色 = 最热事件线来源数分档；行头 aria 含「N 个事件线 · 最热 N 个来源 · 未归组 N 个事件 · N 个主题桶」 | 格值 = 当天该泳道线索合计；**取消颜色通道**；行头 aria 改为「N 条线索 · N 个主题桶」 | 线索全未知时按既有未知态显示 `?`，不造 0 |
| 演进带（`BoardLaneEvolution.vue`） | 节点数字 = 当日线索合计，文案「N 条事件线」 | 节点数字不变，文案改「N 条线索」 | 无 |
| 顶部时间视窗曲线（`laneDynamicsWindow.ts` `dayCellMetrics` 的 `displayCount`） | 事件线数 > 0 用事件线数，否则线索合计兜底 | 逐日值 = 当日线索合计（纯 thread 口径，无兜底层） | 线索全未知时显示 `?`，历史池缺失时沿用既有「历史池未取到」提示与重试入口 |
| 日报刊头「今日重点」（`DailyReportMasthead.vue` `onLocate`） | 重点项带 `story_id`，可定位到正文首条对应 thread | 重点项带聚簇 id，定位到该聚簇在正文中的首条 thread；`?story=` 深链参数退场（解析旧链接时忽略并提示，不报错） | 无 `story_id` / 无聚簇落点的重点保持静态文案；本期正文无对应 thread 时原地提示「本期正文暂无对应内容」，不空跳转 |
| 日报重点依据文案（`dailyReportMagazine.ts` `highlightEvidence`） | 「N 个来源」+ 48h 热度 + 截止时刻 + 不完整标记 | 「N 篇来源」（内容身份去重后的文章数）+ 数据截至日 + 研究时点 | 快照字段缺失时省略数字，不伪造 0 |

不动的入口：既有「匹配规则」配置弹窗（新增 `lane_heat_candidate_weight` 字段，沿用同一弹窗的既有表单控件与保存链路）、版本管理 / 归档 / 编排 / 治理右轨、站内阅读预览。

## 受影响状态

| 状态 | 可见反馈与恢复 |
| --- | --- |
| 矩阵加载 | 沿用既有加载态；矩阵与右上时间视窗共用同一聚合响应，不新增二次请求 |
| 矩阵空态 | 0 正式泳道但有日报时仍渲染网格与日期轴（既有行为不变），不因口径切换改成空态 |
| 矩阵格线索全未知 | 显示 `?` 并在 title/aria 标注「线索数未知」，不显示空白或 0 |
| 矩阵格真零 | 显示 0 且可点进当日工作面（零线索是合法结果），与断档格「—」可区分 |
| 雷达右栏清单为空 | 渲染既有空态占位（说明当天没有可排的泳道线索），不渲染空白 |
| 雷达右栏部分版块延迟 | 缺席版块条目不补 0；清单下方标注「本次清单不完整」并列出缺席版块 |
| 聚簇降级（日报） | 语义向量不可得 → 按主体近似兜底，日报重点依据处注明「本次按主体近似，非向量聚簇」；两者都不可得 → 今日重点降级为空数组并走既有空态占位，日报主体照常落库 |
| 聚簇零候选 | 今日重点候选为空时走既有合法空返回，不重试、不编造 |
| 旧深链 `?story=` | 解析后忽略并提示「该定位方式已下线」，不报错、不跳转、不静默定位到别的内容 |
| 加载 / 错误态 | 雷达总览首次取数失败沿用既有错误态（不展示业务空态）；刷新失败保留旧值 |
| 重复提交 | 配置保存按钮沿用既有禁用与防重逻辑 |

## 复用组件与布局模式

- 不新建页面，沿用既有 console workspace（`BoardConsoleShell` / `BoardRadarView`）与日报阅读台两栏结构；不引入新布局模式，也不改既有栏宽与断档规则。
- 雷达右栏沿用既有清单行结构（版块名 + 计数 + 可选徽标位）；本次**移除**徽标渲染分支，保留行结构与空态组件。
- 矩阵格沿用既有格渲染与 `.b1..b4` 色阶类名；本次停用色阶类名的赋值（颜色通道退役），保留数字样式与 `title` / `aria`。
- 刊头「今日重点」沿用既有刊头边框语言与要闻条目行；定位按钮沿用既有按钮与高亮计时器（2s、reduced-motion 不平滑滚动）。
- 配置字段沿用既有「匹配规则」弹窗的 `AppDialog`（`size="lg"`，760px、92vw 上限）与既有输入控件、错误提示与保存链路，不新造弹层、不自由设宽度。
- 日报重点依据文案沿用既有可读提示样式；长标题 ellipsis + 完整 title。

## 验收映射

| 故事 | 机械断言 | 证据落点（实现后产生，本轮为计划路径） |
| --- | --- | --- |
| S1 看矩阵线索数与当天热度 | 格值 = 当天线索合计；演进节点同值；行序按新热度降序；颜色通道不渲染；aria 含「N 条线索」；线索全未知显示 `?` | planned: `front/app/features/tags/components/console/observationLanes.metrics.test.ts`；`front/app/features/tags/components/console/BoardLaneEvolution.test.ts`；人工：浏览器实链路留痕（实现阶段写 `ui-verification/report.md`） |
| S2 日报同事件聚簇上今日重点 | 候选 = 当天聚簇 Top 8；簇内文章数按内容身份去重；LLM 只能选候选白名单内条目；聚簇降级时注明口径；零候选走合法空返回 | planned: `backend-go/internal/topicgraph/service/daily_report_event_cluster_test.go`；`front/app/features/tags/components/daily-report/DailyReportMasthead.cluster.test.ts` |
| S3 story 停写后一切照旧 | 硬停写（无开关，review R-1）下不再产生 `board_stories` / `story_signals` / `story_id`；旧数据只读可查；CLI 写模式被拒；雷达 / 日报 / 矩阵读端正常 | planned: `backend-go/internal/topicgraph/service/daily_report_story_highlights_test.go`；`backend-go/cmd/backfill-stories/main_test.go` |
| S4 调热度权重 | 保存候选权重后重开仍为保存值；非法值（越界 / 空 / 非有限数）拒绝且不覆盖；新值立即影响泳道热度而不影响话题状态 | planned: `front/app/features/tags/components/MatchingConfigDialog.laneHeat.test.ts`；`backend-go/internal/tagmanagement/handler/board_crud_handler_test.go` |
| 布局与三视口 | 1440×900、1920×1080、390×844 下矩阵与雷达右栏无横溢；长泳道名 ellipsis + title；刊头重点定位命中当前浮层 | 人工：实现阶段按 `ui-verification/report.md` 建账（A-N 场景【pass\|fail\|未覆盖】+ 依据行）；本轮尚未执行，不得当作已完成 |

本合同签署时证据尚未产生：以上「planned」路径均为实现阶段计划文件，当前不存在；不得把合同本身当已完成测试。
