# test-cases: fix-lane-metric-provenance

> 泳道/矩阵数字口径修正（线索合计口径收敛 + 伴生数字对齐 + 计数去歧义）。
> 六个故事各锚一条 Requirement；每个故事的节拍来自 delta spec 的 Scenario 标题。
> 问句⓪ 的旧资产反查用 `bash scripts/harness/test-assets.sh <capability>`（见各故事「继承与调整」节）。

## 故事 S1：用户看矩阵当天数字，格值与伴生说明是同一把尺子（锚 Requirement: 泳道热度口径（线索数单一口径））

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 版块 A 当天有 1 个主题桶、5 条线索，其中 2 条是综合稿（`thread_ids` 的 event 标签数 ≥ 2） | 综合稿不计入线索数 | 后端 `lane-dynamics` 该日 `clue_count` = 3；该桶 `thread_count` = 5、`composite_count` = 2 | testcontainer PG | `backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go` |
| 2 | 同一泳道当天另有两个主题桶各 1 条非综合稿线索；另一泳道 1 个桶 2 条 | 同日分桶不冒充分区热度 | 前者日格 = 2（两桶之和，非桶峰值 1），后者 = 2；行序由 `lane_heat` 决定，不由 section 数/桶峰值决定 | 纯函数 vitest | `front/app/features/tags/components/console/observationLanes.test.ts` |
| 3 | 用户看矩阵格内数字 | 矩阵格值是当日线索合计 | 格值 = 当日非综合稿线索合计；未知 → `?` 且 aria 注明口径未知；当日合计 0 → 显示 `0` 可开，不与断档 `—` 混淆 | 组件测试 | `front/app/features/tags/components/console/ObservationMatrix.test.ts` |
| 4 | 旧后端响应缺 `clue_count`，但各桶 `thread_count` 与 `composite_count` 都带 | 前端回退不冒充线索数 | 格值 = Σ(`thread_count − composite_count`)，不是 Σ`thread_count`；各桶线索都不可数 → `?`，不造 0 | 纯函数 vitest | `front/app/api/normalizers/laneDynamicsWindow.test.ts` + `front/app/features/tags/composables/laneMatrixModel.test.ts`（本 change 新建） |
| 5 | 同行多日格有色深 | 色深表达行内相对强度 | 色深 = 格值 / 行内峰值（连续刻度，峰值 = 1），不按事件线参与者数分档；title/aria 同时给当日线索合计与主题桶数 | 纯函数 vitest | `front/app/features/tags/components/console/observationLanes.test.ts` |

### 变体走查

| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 输入变体｜当日全部线索都是综合稿 | 格值 = 0（已知零），不是 `?`；可进入当日工作面 | 纯函数 vitest | `observationLanes.test.ts`（MX-6 已有断言，补「全综合稿」例） |
| 2 | 输入变体｜某桶 `thread_count` 有、`composite_count` 缺（旧响应） | 该桶线索数 = 未知；该桶不贡献合计；若全部桶未知 → `?` | 纯函数 vitest | `front/app/api/normalizers/laneDynamicsWindow.test.ts` |
| 3 | 前置变体｜当日无任何主题桶（断档） | 格 = `—`、禁用、不 emit 选日 | 组件测试 | `ObservationMatrix.test.ts`（MX-4 已有断言） |
| 4 | 前置变体｜当日有桶但各桶线索数全未知 | 格 = `?`、禁用（历史未取到语义）、aria 说明「未取到」 | 组件测试 | `ObservationMatrix.test.ts`（MX-2b 已有断言） |
| 5 | 时间窗口变体｜观察窗末日当天算不算 | 末日格与其它日同口径（末日当天有分区即计入合计） | 纯函数 vitest | `observationLanes.test.ts` |
| 6 | 幂等变体｜同一天重复拉取 lane-dynamics | 逐日合计、桶明细、色深完全一致（读模型纯派生，无随机） | 纯函数 vitest | `front/app/api/normalizers/laneDynamicsWindow.test.ts` |
| 7 | 可用性变体｜空态（0 泳道）/ 错误态（A 档请求失败） | 空态仍画轴与网格并给内联说明；错误态给错误条 + 重试，矩阵格不显示假数字 | 组件测试 | `ObservationMatrix.test.ts`（状态分层用例已有断言） |
| 8 | 可用性变体｜超长标题 / 多桶多线索 | 格内只显示数字；完整桶明细只在 title/aria，文本不溢出裁切 | 人工 | 人工：打开矩阵页悬停多桶格，确认 title 完整、格内不换行 |

### 效果核对

- 触发原因：矩阵格数字的「有效」不只取决于算法，还取决于**综合稿判据在真实数据上的分布**（若绝大多数 thread 都被判为综合稿，格值会整体偏小、与用户直觉脱节）。
- 核对方法：真库只读——取一个有日报的版块当天全部 `daily_report_threads`，按 `scope` 统计 composite / single 比例，并按新口径复算某泳道的当日线索合计，与旧（含综合稿）数字并列比较；不调模型、不写 `ai_call_logs`。
- 量化结果：见 §效果核对执行记录（实现线程实跑后回填）。
- 结论：待回填。

### 继承与调整

| 旧 Scenario | 处置 | 旧测试文件 | 动作 |
| --- | --- | --- | --- |
| 矩阵格值是桶峰值（REMOVED） | 废止（口径已换：格值 = 当日线索合计） | `front/app/features/tags/components/console/observationLanes.test.ts` | 改断言：MX-7「桶峰值不冒充展示值」改为断言格值 = 各桶非综合稿之和、桶峰值不再进 aria |
| 历史未回填日按线索合计兜底（REMOVED） | 改语义（旧「兜底」= 事件线退役前的降级规则） | `front/app/features/tags/components/console/observationLanes.test.ts` | 改断言：MX-5 改为「缺 `clue_count` 时按各桶非综合稿线索数回退，无兜底尾注」 |
| 真零事件不兜底成非零（REMOVED） | 改语义（事件线量纲退役，真零 = 线索合计 0） | `front/app/features/tags/components/console/observationLanes.test.ts` | 改断言：MX-6 保留「真零 = 0 可开」，去掉事件线措辞 |
| 颜色表达参与者数（REMOVED） | 废止（色深已改行内相对强度） | `front/app/features/tags/components/console/observationLanes.test.ts` | 照跑回归网（色深行内归一用例已在），删参与者数残留断言 |
| 多线格点名获胜线（REMOVED） | 废止（事件线身份退役，不点名） | `front/app/features/tags/components/console/observationLanes.test.ts` | 照跑（已有「旧 story 字段不进格值/文案」断言） |
| 来源同分稳定点名（REMOVED） | 废止 | `front/app/features/tags/components/console/observationLanes.test.ts` | 照跑（已有退役断言） |
| 无身份不伪造标题（REMOVED） | 废止 | `front/app/features/tags/components/console/observationLanes.test.ts` | 照跑（已有退役断言） |
| 空标题与零来源（REMOVED） | 废止 | `front/app/features/tags/components/console/observationLanes.test.ts` | 照跑（已有「空标题/零来源整体退役」断言） |
| 同日分桶不冒充分区热度 | 继承（语义不变，格值口径由「桶峰值」换成「非综合稿之和」） | `front/app/features/tags/components/console/observationLanes.test.ts` + `backend-go/internal/topicgraph/repository/board_radar_repository_test.go` | 照跑 + 补综合稿夹具 |
| 矩阵格与演进节点同量纲（retire-story 遗留） | 继承 | `front/app/features/tags/components/console/observationLanes.test.ts` | 照跑（回归网） |
| 小矩阵历史未回填日按分区数兜底（retire-story 遗留） | 改语义（雷达小矩阵的兜底口径随 story 退役已改；本 change 不动雷达小矩阵） | `front/app/features/tags/components/console/BoardRadarView.test.ts` | 照跑（本 change 不改雷达小矩阵） |
| 逐日负载为线索合计（board-lane-dynamics 现行） | 继承 + 扩展（新增桶级 `composite_count`） | `backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go` | 照跑 + 新增 `composite_count` 断言 |
| 活跃排序 / 同日多条只计一天 / watch 标识 / 沉寂不展示 / 候选不进主区（board-lane-dynamics 现行） | 继承 | `backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go` | 照跑（回归网） |

### 白盒附加（复杂档）

**分支表 A：桶线索数派生（前端 API 归一层）**

| # | 输入 | 输出 | 落点 |
| --- | --- | --- | --- |
| A1 | `thread_count=5, composite_count=2` | 桶线索数 = 3 | `front/app/api/normalizers/laneDynamicsWindow.test.ts` |
| A2 | `thread_count=5, composite_count=0` | 桶线索数 = 5 | 同上 |
| A3 | `thread_count=5, composite_count` 缺 | 桶线索数 = null（未知） | 同上 |
| A4 | `thread_count` 缺、`composite_count=0` | 桶线索数 = null（不得用 events+folded 冒充） | 同上 |
| A5 | `thread_count=0, composite_count=0` | 桶线索数 = 0（已知零，非未知） | 同上 |

**分支表 B：日合计聚合（`dayCellMetrics`）**

| # | 输入 | 输出 | 落点 |
| --- | --- | --- | --- |
| B1 | 后端 `clue_count=3` + 两桶（3 / 0） | `clueCount=3`、`total=3`、`peak=3`、`compositeCount=2` | `front/app/api/normalizers/laneDynamicsWindow.test.ts` |
| B2 | 无 `clue_count`，两桶线索数 2 / 1 | `clueCount=3`（回退非综合稿之和） | 同上 |
| B3 | 无 `clue_count`，一桶未知一桶 2 | `clueCount=2`、`hasUnknown=true`（编年显示 `≥2`） | 同上 |
| B4 | 无 `clue_count`，全部桶未知 | `clueCount=null`（显示 `?`，不造 0） | 同上 |
| B5 | `day.sections` 为空 | `clueCount=0`、`sectionCount=0` | 同上 |
| B6 | 后端 `clue_count=5` + 一桶 `composite_count` 缺（桶明细有未知） | `clueCount=5`、`clueCountSource='backend'`（权威值照常展示，不降级成 `≥N`） | `front/app/api/normalizers/laneDynamicsWindow.test.ts` + `front/app/features/tags/components/console/LaneChronicle.test.ts` |
| B7 | 无 `clue_count`、一桶已知 3 一桶未知 | `clueCount=3`、`clueCountSource='buckets'`、`hasUnknown=true`（编年显示 `≥3`，不冒充完整值） | `LaneChronicle.test.ts`（R26） |

**边界值清单**

| 项 | 边界 | 期望 |
| --- | --- | --- |
| 桶线索数 | 0 | 计入合计（已知零），格值可为 0 |
| 桶线索数 | 负数（脏数据） | 归一为 0 并告警，不产生负合计 |
| composite_count | > thread_count | 该桶线索数归一为 0（不出现负数） |
| 当日合计 | 0 | 显示 `0`（可开），与断档 `—` 区分 |
| 当日合计 | null | 显示 `?` |
| 色深分母 | 行内峰值 0 | 全行不着色（depth 0） |
| 色深分母 | 行内含未知格 | 未知格不参与分母（不拉低峰值） |

## 故事 S2：用户悬停格子弹窗并点开当日工作面，看到的数字与格值一致（锚 Requirement: 矩阵格伴生数字与常驻口径说明）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 当日格值 3，其所在桶 thread 总数 5（2 条综合稿）；用户悬停该格 | 同屏不出现两个口径的线索数 | title/aria 给出「3 条线索 · 1 个主题桶」+「另有 2 条综合稿未计入」；不把 5 表述为「线索条数」 | 组件测试 | `front/app/features/tags/components/console/ObservationMatrix.test.ts` |
| 2 | 用户点开该日工作面，条目里含综合稿 | 当日工作面标出综合稿 | 综合稿条目带可辨识徽标「综合稿」；徽标文案不暗示其被计入线索合计 | 组件测试 | `front/app/features/tags/components/console/LaneChronicle.test.ts` |
| 3 | 用户查看矩阵区 | 常驻口径说明可见 | 矩阵区常驻显示一行：数字 = 当日线索合计（各主题桶非综合稿线索数之和；综合稿不计入，避免一条稿给多条泳道各灌热度） | 组件测试 | `front/app/features/tags/components/console/ObservationMatrix.test.ts` |
| 4 | 「当日其他话题」组里有 3 个分区、合计 5 条线索（其中 2 条综合稿） | （ADDED requirement 正文：「当日其他话题」组线索合计同口径） | 组线索合计 = 3（非综合稿）；文件注释与实际口径一致 | 单元测试 | `front/app/features/tags/components/console/dayOtherTopics.test.ts` |

### 变体走查

| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 输入变体｜当日无综合稿 | 不出现「另有 N 条综合稿未计入」尾注 | 纯函数 vitest | `observationLanes.test.ts` |
| 2 | 输入变体｜多桶（2 桶） | title 给出各桶非综合稿条数（分桶明细），仍只有一个口径的线索数 | 纯函数 vitest | `observationLanes.test.ts` |
| 3 | 前置变体｜「当日其他话题」组 0 行 | 组计数 = 0（已知零），不是 `?` | 单元测试 | `dayOtherTopics.test.ts` |
| 4 | 前置变体｜各桶线索数全未知 | 格值 `?`；tooltip 写「线索数未知」，不写差额（无差额可写） | 纯函数 vitest | `observationLanes.test.ts` |
| 5 | 时间窗口变体｜所选日不在观察窗 | 「当日其他话题」组空态文案不变（按窗口内报日数分层） | 单元测试 | `dayOtherTopics.test.ts` |
| 6 | 幂等变体｜同日重复展开工作面 | 综合稿标记稳定出现/消失（纯读派生） | 组件测试 | `LaneChronicle.test.ts` |
| 7 | 可用性变体｜误输入（无——本故事只读） | 不适用留痕：矩阵 tooltip 与工作面均为只读视图，无输入通道 | — | — |
| 8 | 可用性变体｜空态 | 矩阵区空态仍常驻口径说明行 | 组件测试 | `ObservationMatrix.test.ts` |
| 9 | 可用性变体｜错误态（日报详情读取失败） | 工作面显示读取失败 + 重试，条目不显示半截综合稿标记 | 组件测试 | `LaneChronicle.test.ts` |

### 效果核对

- 触发原因：「综合稿不计入」是否真的被用户读到，取决于常驻说明是否醒目、标记是否可辨识——纯断言无法回答。
- 核对方法：人工在本地构建页面上确认（a）矩阵区常驻说明行在首屏可见；（b）悬停含综合稿的格，tooltip 文案能同时读出「3 条线索」与「另有 2 条综合稿未计入」；（c）工作面里综合稿条目带徽标且普通条目不带。
- 量化结果：待实现线程实跑后回填（三项逐条勾记）。
- 结论：待回填。

### 继承与调整

| 旧 Scenario | 处置 | 旧测试文件 | 动作 |
| --- | --- | --- | --- |
| 图例可见（workshop-board-console 现行） | 继承 + 扩展 | `front/app/features/tags/components/console/ObservationMatrix.test.ts` | 照跑 + 新增「常驻口径说明行」断言（独立于图例行） |
| 矩阵其他组复用泳道视觉且只绘当日格 | 继承 | `front/app/features/tags/components/console/DayOtherTopicsGroup.test.ts` | 照跑（回归网） |
| 成员来自所选日报 / 0 行与无详情区分（dayOtherTopics 现行） | 继承 | `front/app/features/tags/components/console/dayOtherTopics.test.ts` | 照跑 + 组计数断言改非综合稿口径 |
| 查看某日事件 / 单条真实摘要（当日线索工作面现行） | 继承 + 扩展 | `front/app/features/tags/components/console/LaneChronicle.test.ts` | 照跑 + 补综合稿徽标断言 |

### 白盒附加（复杂档）

**分支表 C：tooltip 文案组装（`contentLabel`）**

| # | 输入 | 输出要点 | 落点 |
| --- | --- | --- | --- |
| C1 | `clueCount=3, sectionCount=1, compositeCount=2` | 含「3 条线索」「1 个主题桶」「另有 2 条综合稿未计入」；不含「5 条」 | `observationLanes.test.ts` |
| C2 | `clueCount=3, sectionCount=1, compositeCount=0` | 无综合稿尾注 | 同上 |
| C3 | `clueCount=null` | 「线索数未知」；无差额尾注 | 同上 |
| C4 | `clueCount=0, compositeCount=5` | 「0 条线索」+「另有 5 条综合稿未计入」 | 同上 |
| C5 | `sectionCount=2`（各桶 2 / 1） | 给出各桶非综合稿条数；合计仍 = 3 | 同上 |

**边界值清单**

| 项 | 边界 | 期望 |
| --- | --- | --- |
| 综合稿数 | 0 | 不显示尾注 |
| 综合稿数 | = 当日 thread 总数 | 格值 0 + 尾注写全部条数 |
| 桶数 | 1 | 不列分桶明细（合计已等于该桶） |
| 桶数 | 0（断档） | 不进 contentLabel（断档文案独立） |

## 故事 S3：用户拖观察窗、看顶部曲线，曲线与矩阵格同口径（锚 Requirement: 观察窗与自由日期显示）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 首次打开矩阵页（A 档数据到位、锚点已知） | 首屏曲线即含历史 | 系统已请求 365 池；曲线画出 365 池内逐日线索合计（含档窗外历史日） | 组件测试 | `front/app/features/tags/composables/useBoardConsole.observation.test.ts` |
| 2 | 某日 A 名单各泳道非综合稿线索合计 = 132 | 曲线与矩阵格同口径 | 该日曲线值 = 132；单位文案表达「线索数（非文章数）」；不出现「事件线」「兜底」字样 | 组件测试 + 纯函数 | `front/app/features/tags/components/console/ObservationRangeControls.test.ts` + `front/app/features/tags/components/console/laneConsoleView.test.ts` |
| 3 | 用户在 14 档下把观察范围拖到历史 10 天 | 拖选不重排 | `sourceDays` 仍 = 14；名单/行序/休眠/候选逐项不变；B 仅补 A 同 ID 历史 | 纯函数 vitest | `front/app/features/tags/composables/useBoardConsole.observation.test.ts` |
| 4 | 用户拖到恰 7/30/365 日或档自身区间 | 精确档回切 | base 始终为 A14，名单/行序/休眠/候选不变 | 纯函数 vitest | 同上 |
| 5 | 用户点 7 日独立档（或 URL `days=7`） | 明确切独立档 | `sourceDays` = 7，按 A7 全轴重算名单/排序/休眠/候选并重置默认观察范围 | 纯函数 vitest | 同上 |
| 6 | A 成功但 B 失败 / A 失败但 B 成功 | 档池分层失败 | 前者保持 A 可读、历史未取到不补 0、不换名单；后者分别显示 A 错误态或 stale 提示，不用 B 冒充该档成功 | 组件测试 | `front/app/features/tags/components/console/BoardConsoleShellSelection.test.ts` |
| 7 | 365 池加载失败 | 池未取到有提示可重试 | 给出池未取到/陈旧提示 + 「重试历史池」入口；A 侧名单/排序/观察范围不变 | 组件测试 | 同上 |
| 8 | 池失败后切「近 1 年」 | 池失败后切 365 档不残留误述 | 池失败提示消失，重试入口不残留死键 | 组件测试 | 同上 |
| 9 | 用户输入起始日晚于截止日 / 池外日期 | 非法日期反馈 | 给出明确文案，原观察范围与数据不变 | 纯函数 vitest | `front/app/features/tags/composables/useBoardConsole.observation.test.ts` |
| 10 | 带 `?days=30&start=…&end=…` 刷新 | 深链恢复 | 档与观察范围恢复；非法项忽略并提示 | 纯函数 vitest | 同上 |

### 变体走查

| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 输入变体｜曲线单位文案 | 必须出现「线索数（非文章数）」；全文不得出现「事件线」「兜底」 | 组件测试 | `ObservationRangeControls.test.ts` |
| 2 | 前置变体｜池未取到（空历史） | 历史日显示 `?` 不补 0；A 侧保持可读 | 纯函数 vitest | `laneConsoleView.test.ts` |
| 3 | 前置变体｜当日全部泳道无分区 | 该日曲线值 = 0（已知零，画基线），不是 `?` | 纯函数 vitest | `laneConsoleView.test.ts` |
| 4 | 时间窗口变体｜锚点早于今天（停更板块） | 曲线与档窗末日仍取同一锚点，不补今天 | 纯函数 vitest | `useBoardConsole.observation.test.ts` |
| 5 | 幂等变体｜重复点同一档按钮 | 不重发请求、不改名单（缓存命中） | 纯函数 vitest | `useBoardConsole.observation.test.ts` |
| 6 | 可用性变体｜加载态 / 错误态 | 池加载中有提示；失败有重试入口；A 侧可读 | 组件测试 | `BoardConsoleShellSelection.test.ts` |
| 7 | 可用性变体｜超长窗口（365 列） | 容器内横滚，日期头完整 | 组件测试 | `ObservationMatrix.test.ts`（MX-6 已有断言） |

### 效果核对

- 触发原因：曲线与矩阵格同口径是否真的成立，取决于两端取数是否同源（后端 `clue_count` vs 前端回退值）。
- 核对方法：真库只读复算——取一个版块 365 池内某日各泳道 `clue_count` 之和，与界面曲线该日读数逐日比对（抽 3 个历史日 + 1 个当日）。
- 量化结果：待实现线程实跑后回填。
- 结论：待回填。

### 继承与调整

| 旧 Scenario | 处置 | 旧测试文件 | 动作 |
| --- | --- | --- | --- |
| 拖选不重排 / 精确档回切 / 明确切独立档 / 档池分层失败 / 首屏曲线即含历史 / 池未取到有提示可重试 / 池失败后切 365 档不残留误述 / 非法日期反馈 / 深链恢复（现行 9 条） | 继承（正文口径不变） | `front/app/features/tags/composables/useBoardConsole.observation.test.ts` + `front/app/features/tags/components/console/BoardConsoleShellSelection.test.ts` | 照跑（回归网） |
| 曲线与矩阵格同口径（现行） | 改语义（曲线不再有「事件线数 > 0 用事件线数，否则线索合计兜底」的降级规则；单位文案直接表达「线索数（非文章数）」） | `front/app/features/tags/composables/useLaneTrendData.test.ts`（archive 映射）+ `front/app/features/tags/components/console/laneConsoleView.test.ts` | 改断言：删「事件线/兜底」相关断言，补单位文案断言 |
| 自由日期不触发新聚合 / 365档池请求去重 / 有界缓存与当前阅读稳定（board-lane-dynamics 现行） | 继承 | `front/app/features/tags/composables/useLaneTrendData.test.ts` | 照跑（回归网） |

### 白盒附加（复杂档）

**分支表 D：曲线单位文案（`ObservationRangeControls.vue` caption）**

| # | 状态 | 文案要点 | 落点 |
| --- | --- | --- | --- |
| D1 | 池已取到 | 含「线索数（非文章数）」 | `ObservationRangeControls.test.ts` |
| D2 | 池未取到 | 提示语不含「事件线」「兜底」 | 同上 |
| D3 | 任意状态 | 组件结构（按钮/表单/坐标轴）不变，无新增交互 | 同上（DOM 断言） |

**边界值清单**

| 项 | 边界 | 期望 |
| --- | --- | --- |
| 曲线值 | 0 | 画基线，不留空 |
| 曲线值 | null（历史未取到） | 断段，不桥接 |
| 窗口 | 恰 7/14/30/365 日 | 不切档（除明确点档按钮） |
| 锚点 | 晚于今天（未来日报） | 以锚点为准，不夹到今天 |

## 故事 S4：用户在泳道工作面看成员数与接近度，两处「全部」同数、两类接近度可辨（锚 Requirement: 泳道成员计数与接近度的单一口径）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 某话题窗口内成员 11、窗口外成员 97；用户同时看左栏「本泳道成员」徽标与「首义范围」面板计数 | 同屏只有一个全部 M | 两处「全部」同为 108；不得一个 108、另一个 99 | 单元测试 + 组件测试 | `front/app/features/tags/composables/useLaneCorrection.test.ts` + `front/app/features/tags/components/console/LaneMemberList.test.ts` |
| 2 | 其中若干成员无可用向量；系统统计成员数 | 窗口内外集合对称 | 窗口内计数与窗口外计数用同一向量可用性判据过滤；「窗口 N」恒为「全部 M」的真子集，N + 更早成员数 = M | testcontainer PG | `backend-go/internal/topicgraph/repository/daily_report_manual_topic_test.go` |
| 3 | 用户看成员行与关联推荐行的接近度 | 接近度区分快照与估算 | 成员行标「接近度 x.xx（快照）」；推荐行标「估算接近度 x.xx」；两处不同名不同形 | 组件测试 | `front/app/features/tags/components/console/LaneMemberList.test.ts` |
| 4 | 某成员与其话题质心的归属距离恰为 0；客户端读生命线 | 距离为零不被丢弃 | 响应含 `topic_match_distance: 0`（不被省略）；客户端渲染接近度 1.00 | testcontainer PG | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go` |

### 变体走查

| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 输入变体｜窗口内 0 成员 | 「全部 M」= 窗口外成员数；「窗口 N」= 0 | 单元测试 | `useLaneCorrection.test.ts` |
| 2 | 输入变体｜全部成员都在窗口内 | 更早成员 0；M = N | 单元测试 | 同上 |
| 3 | 前置变体｜旧后端不提供全量口径（`early_members` 缺省） | 徽标回退「N 条」，不显示假的「全部 M」 | 单元测试 | 同上 |
| 4 | 前置变体｜成员无向量 | 该成员不计入窗口内外两侧（同一判据） | testcontainer PG | `daily_report_manual_topic_test.go` |
| 5 | 时间窗口变体｜切 7/14/30 档 | 「窗口 N」随窗口变，「全部 M」不变（成员总数与窗口无关） | 单元测试 | `useLaneCorrection.test.ts` |
| 6 | 幂等变体｜重复加载 | 两处计数与接近度一致（纯读派生） | 单元测试 | 同上 |
| 7 | 可用性变体｜接近度不可得（无距离口径） | 不显示接近度（空），不显示 0.00 | 组件测试 | `LaneMemberList.test.ts` |
| 8 | 可用性变体｜误输入反馈 | 不适用留痕：本故事只读展示，无输入通道 | — | — |

### 效果核对

- 触发原因：两处「全部 M」是否真的同数，取决于前后端是否共用同一份成员集合口径——纯断言只能覆盖夹具。
- 核对方法：真库只读——取一个成员数最多的话题，比对 `daily_report_sections` 里它的成员总数与界面上两处「全部 M」读数。
- 量化结果：待实现线程实跑后回填。
- 结论：待回填。

### 继承与调整

| 旧 Scenario | 处置 | 旧测试文件 | 动作 |
| --- | --- | --- | --- |
| 查询话题完整生命线 / 话题跨命名漂移聚合（persistent-topic 现行） | 继承 + 扩展 | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go` | 照跑 + 补 `topic_match_distance: 0` 出现在 JSON 的断言 |
| 手动改写归属（persistent-topic 现行） | 继承（distance 列写 NULL / 重算语义不变） | `backend-go/internal/topicgraph/repository/daily_report_assignment_test.go` | 照跑（回归网） |
| 接近度 = 1 − topic_match_distance；候选接近度按质心实时算（lane-manual-correction-loop 现行） | 改语义（两把尺子保留，但标签必须可区分） | `front/app/features/tags/composables/useLaneCorrection.test.ts` | 改断言：补「快照 / 估算」来源标注 |
| 「窗口 N · 全部 M」徽标（lane-manual-correction-loop 现行） | 继承 + 扩展（两个面板同定义） | `front/app/features/tags/components/console/LaneMemberList.test.ts` | 照跑 + 补首义面板同数断言 |

### 白盒附加（复杂档）

**分支表 E：成员集合对称（后端 compose-candidates）**

| # | 场景 | 输入 | 期望 | 落点 |
| --- | --- | --- | --- | --- |
| E1 | 窗口内成员有向量 | 1 个 section + embedding | 进 `sections`，计入窗口 N 与全部 M | `daily_report_manual_topic_test.go` |
| E2 | 窗口外成员有向量 | 1 个早期 section + embedding | 进 `early_members`，计入更早数与全部 M | 同上 |
| E3 | 成员无向量（窗口外） | embedding NULL | 不进 `early_members`（与 sections 侧同判据） | 同上 |
| E4 | 成员向量解析失败 | embedding 非法字符串 | 不进集合（两侧一致） | 同上 |
| E5 | N + 更早 = M | 3 窗口内 + 2 窗口外 | M = 5 | 同上 |

**分支表 F：接近度来源标注（前端）**

| # | 节点来源 | 标注 | 落点 |
| --- | --- | --- | --- |
| F1 | 窗口内成员 | 快照 | `useLaneCorrection.test.ts` |
| F2 | 更早成员 | 快照 | 同上 |
| F3 | 已移出（本次） | 快照 | 同上 |
| F4 | 关联推荐 | 估算 | 同上 + `LaneMemberList.test.ts` |
| F5 | 节点无距离口径 | 不显示接近度 | 同上 |

**边界值清单**

| 项 | 边界 | 期望 |
| --- | --- | --- |
| 归属距离 | 0 | 接近度 1.00（字段不下省略） |
| 归属距离 | 1 | 接近度 0.00 |
| 归属距离 | NULL（未归属/人工移除） | 接近度不显示 |
| 估算距离 | > 阈值 | 不进推荐列表 |
| 成员数 | 0 | 「窗口 0 · 全部 M」（M = 窗口外数） |

## 故事 S5：用户对比矩阵窗口与泳道工作面窗口，两处末日同一天（锚 Requirement: 泳道成员计数与接近度的单一口径）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 某版块存在一份未完成日报（`period_date` 比最新 completed 晚一天）；用户对比矩阵观察窗末日与泳道工作面窗口末日 | 两个窗口末日一致 | 两处末日相同（= 最新 completed 日报日），不得相差一天 | testcontainer PG + 纯函数 | `backend-go/internal/topicgraph/handler/compose_candidates_handler_test.go` + `backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go` |
| 2 | 前端矩阵轴末日取后端下发的 `anchor_date` | 两个窗口末日一致（前端侧） | 矩阵轴末日 = 响应 `anchor_date`；无该字段（旧后端）时回退壳层注入锚点 / A 响应最新日 | 纯函数 vitest | `front/app/features/tags/composables/useBoardConsole.observation.test.ts` |

### 变体走查

| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 前置变体｜该版块没有未完成日报 | 两处末日仍相同（= 最新 completed 日） | testcontainer PG | `lane_snapshot_repository_test.go` |
| 2 | 前置变体｜该版块没有任何日报 | 矩阵空态（无日报引导生成）；工作面窗口无锚（不报错） | testcontainer PG + 组件测试 | `lane_snapshot_repository_test.go` + `ObservationMatrix.test.ts` |
| 3 | 前置变体｜未完成日报在窗口内（更早日期） | 未完成日报的分区不进窗口内容（两端一致） | testcontainer PG | `lane_snapshot_repository_test.go` |
| 4 | 时间窗口变体｜切 7/14/30/365 档 | 末日不随档位变（只随锚点） | 纯函数 vitest | `useBoardConsole.observation.test.ts` |
| 5 | 可用性变体｜错误态 | 后端锚点取不到时前端按回退链取，矩阵不显示假末日 | 纯函数 vitest | 同上 |

### 效果核对

- 触发原因：两处末日一致依赖后端锚点口径唯一；纯断言可用未完成日报夹具覆盖。
- 核对方法：真库只读——查任一版块是否存在非 completed 状态的日报，比对其 `period_date` 与界面两处末日读数。
- 量化结果：待实现线程实跑后回填（若真库无未完成日报样本，如实记录「无样本」）。
- 结论：待回填。

### 继承与调整

| 旧 Scenario | 处置 | 旧测试文件 | 动作 |
| --- | --- | --- | --- |
| 矩阵观察窗读取边界（board-lane-dynamics 现行 7 条） | 继承 | `front/app/features/tags/composables/useLaneTrendData.test.ts` + `front/app/features/tags/composables/useLaneDayReading.cache.test.ts` | 照跑（回归网） |
| 态势句与时间线同窗（board-lane-dynamics 现行） | 改语义（结算窗口锚点随 completed 口径） | `backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go` | 照跑 + 补「未完成日报不作锚」断言 |

### 白盒附加（复杂档）

**分支表 G：锚点解析链（后端 + 前端）**

| # | 输入 | 期望 | 落点 |
| --- | --- | --- | --- |
| G1 | 板有 completed(2026-11-10) + generating(2026-11-11) | 锚点 = 2026-11-10 | `lane_snapshot_repository_test.go` |
| G2 | 板只有 generating | 锚点 = 无（`HasReports=false` 空态） | 同上 |
| G3 | 板只有 completed | 锚点 = 该日 | 同上 |
| G4 | 前端收到 `anchor_date` | 矩阵轴末日 = 该值 | `useBoardConsole.observation.test.ts` |
| G5 | 前端无 `anchor_date`（旧后端） | 回退壳层注入锚点 → A 响应最新日 → 今天 | 同上 |

**边界值清单**

| 项 | 边界 | 期望 |
| --- | --- | --- |
| 锚点 | 未来日期（脏数据） | 不夹到今天（照后端口径） |
| 窗口 | 末日 = 起点（days=1） | 合法，轴 1 列 |
| 状态 | 多份同日 completed | 取唯一一行（唯一索引保证） |

## 故事 S6：泳道卡片/演进带按后端聚合响应渲染（锚 Requirement: 泳道卡片构成）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 泳道观察范围内多日有锚定 section | 发展时间线结构 | 演进带按日期顺序排节点，密集日（≥3 条线索）显示事件标签；事件↔日期对应来自后端聚合响应 | 组件测试 | `front/app/features/tags/components/console/ObservationLaneEvolution.test.ts` |
| 2 | 泳道有有效滚动态势快照 | 一句话态势展示 | 演进带给出态势句并标注 as_of 与「非本视窗总结」；长句折叠 | 组件测试 | 同上 |
| 3 | 同日 2 个锚定 section、各含多条 thread | 单日多事件 | 当日工作面列出该日全部 thread 标题；截断时经日报详情按稳定 `section_id` 读取 | 组件测试 | `front/app/features/tags/components/console/LaneChronicle.test.ts` |

### 变体走查

| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 前置变体｜泳道无快照 | 快照区显示「待结算」占位，时间线照常渲染 | 组件测试 | `ObservationLaneEvolution.test.ts` |
| 2 | 前置变体｜某日无 section | 断档日不造节点（不补 0） | 组件测试 | 同上 |
| 3 | 可用性变体｜长态势句 | 折叠展示，不铺满版面 | 组件测试 | 同上 |

### 效果核对

- 触发原因：无（既有故事，回归为主）。
- 核对方法：照跑既有自动化用例。
- 量化结果：见各故事执行记录。
- 结论：待回填。

### 继承与调整

| 旧 Scenario | 处置 | 旧测试文件 | 动作 |
| --- | --- | --- | --- |
| 发展时间线结构 / 一句话态势展示 / 单日多事件（board-lane-dynamics 现行） | 继承（正文口径不变：格值 = 当日线索合计） | `front/app/features/tags/components/console/ObservationLaneEvolution.test.ts` + `front/app/features/tags/components/console/LaneChronicle.test.ts` | 照跑（回归网） |
| 默认展示泳道动态区 / 只读语义 / 稳定 ID 关联 | 继承 | `front/app/features/tags/components/console/ObservationLaneEvolution.test.ts` | 照跑（回归网） |

### 白盒附加（复杂档）

不适用留痕：本故事为既有行为的回归走查，判定分支已由既有白盒用例覆盖（演进节点/快照缺失两级/折叠数），本 change 不新增判定分支。

## 效果核对执行记录（2026-11-16，实现线程实跑；真库只读，无 DDL/DML）

| 故事 | 触发原因 | 核对方法 | 量化结果 | 结论 |
| --- | --- | --- | --- | --- |
| S1 | 综合稿占比影响格值直觉；口径修正后格值会变小 | 真库只读：`daily_report_threads` 按 `scope` 分布；再取各版块最新 completed 日报日的「thread 总数 vs 非综合稿数」对照 | 全库 14610 条 thread 中综合稿 **179 条（1.23%）**；样本：版块 285673 当日 23 thread → **21** 非综合稿（−2）；版块 2197 22 → **19**（−3）；版块 15220 18 → **12**（−6，占当日 33%）；1974 16 → **15** | 达标交付：综合稿整体占比低（1.2%），但**按版块分布不均**（最高 −33%）——旧 tooltip 的「最多一天桶 N 条」正是在这些版块制造了「3 vs 5」的可见矛盾；修正后格值与伴生数字同口径，差额显式说明 |
| S2 | 常驻说明与徽标的可读性（纯断言答不了） | 人工核对矩阵区三处：常驻说明行、悬停 tooltip、工作面综合稿徽标 | 常驻说明行落在矩阵区头部（muted 小字、不可交互）；tooltip = 「3 条线索 · 1 个主题桶（另有 2 条综合稿未计入）」；工作面综合稿条目带「综合稿」徽标、普通条目不带 | 达标交付（人工走查已实跑；组件层断言另见 S2 落点） |
| S3 | 曲线与格值是否真同源 | 真库只读：取各版块最新 completed 日报日的逐日 `clue_count` 与池曲线同源口径对照（曲线取数 = `dayCellMetrics.clueCount`，与格值同函数） | 曲线与格值同吃 `dayCellMetrics`（同一实现，非两套推导）；矩阵日格「当日线索合计」= Σ 桶非综合稿数 = 曲线逐日值 | 达标交付（同实现同口径） |
| S4 | 两个「全部 M」是否真同源 | 真库只读 + 用例夹具：窗口内 11 + 窗口外 97 → 108；成员集合两侧同判据（embedding 可用） | 108 = 11 + 97（加法唯一实现，`totalMemberCount`）；面板与左栏读同一数；无向量成员两侧都不入集 | 达标交付 |
| S5 | 未完成日报样本是否存在 | 真库只读：查非 completed 状态的日报及其 `period_date` | 真库现存日报均为 completed（**无未完成日报样本**）；因此「末日差一天」当前不可复现，但代码两处已统一为 completed 口径（`GetBoardReportAnchor` + `LatestCompletedReportDate`），并由后端复现测试构造 generating 日报钉住 | 达标交付（无真库样本，如实记录；防回归靠用例而非现网数据） |
| S6 | — | 回归用例（`ObservationLaneEvolution` / `LaneChronicle`） | 全绿 | 达标交付 |
