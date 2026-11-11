# 用例故事 — 可验证的重点与矩阵颜色

**故事**：用户看到一格有两条事件线，能从提示知道颜色说的是哪一条；打开日报，看到来源依据和昨日重点延续，再从重点直接抵达当前正文第一条对应事件。用户不应被旧数据、归并或生成失败引到假位置。

**测试单元**：Requirement 用户故事；spec Scenario 为断言片段。以下均为待实现/待执行用例，不表示当前实现已具备能力。

## 1. 功能：主链路节拍

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| 1 | 读取同日两线矩阵 | 多线格点名获胜线 / 来源同分稳定点名 | 数字2、颜色4，id/title/max 同源，同分id最小 | PG仓储 + Vitest | `backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go`；`front/app/features/tags/components/console/observationLanes.test.ts` |
| 2 | 打开矩阵听读/悬停 | 图例可见 / 颜色表达参与者数 | title/aria 点名；图例「当天被报道最多…」 | Vitest | `front/app/features/tags/components/console/ObservationMatrix.test.ts`；`ObservationMatrixGrid.test.ts`（同目录） |
| 3 | 生成本期报告 | 并行生成成功 / 完整流水线执行 | C×K并行→SaveReport→归属→去重→候选→A→最终标题，LLM不在事务 | Go编排 fake | `backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go`；`daily_report_watch_integration_test.go`（同目录） |
| 4 | 按48h热度取8条 | 候选按热度而非桶数截断 / 来源窗口与半衰期 | H原值→P→id，窗口两端明确，0也可入选 | PG仓储 | `backend-go/internal/topicgraph/repository/story_heat_test.go` |
| 5 | LLM从候选写文案 | 只可从候选事实写文案 / 少量候选与合法空返回 | 集外身份拒绝、tag事实白名单，1条/[]合法 | Go fake LLM | `backend-go/internal/topicgraph/service/daily_report_llm_test.go` |
| 6 | 查看今日重点依据 | 重点快照与旧数据兼容 | 新项来源/热度快照，旧项不造数；第一条仍为title/summary | PG + Vitest | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go`；`front/app/features/tags/components/daily-report/DailyReportMasthead.test.ts` |
| 7 | 比较昨日同线 | 昨日同线再入选 / 合并与坏引用 | 精确昨日、存活id同线→进展，不改线程is_development | PG + Vitest | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go`；`front/app/features/tags/components/daily-report/DailyReportMasthead.test.ts` |
| 8 | 点击或键盘激活重点 | 点击第一条可见正文线程 | 展开首条正文容器，滚动、焦点和2s高亮；不是最小id | Vitest | `front/app/features/tags/components/DailyReportReader.test.ts`；`front/app/features/tags/components/daily-report/DailyReportTopicSection.test.ts` |
| 9 | 走完可见故事 | 重复定位与减弱动画 / 失去目标与旧重点 | 单高亮、切报清理、缺目标诚实降级，无详情页 | 完整交互 | 人工：agent-browser 静态 :5100 检查实际滚动位置/activeElement/DOM高亮，记录 `ui-verification/report.md` |
| 10 | 故障后仍能读报告 | 归属失败标签回退仍出报告 / 归属超时不迟到写入 | 标签模型有效时重点非空/无story；60s取消，不迟到写 | Go fake依赖+隔离PG | `backend-go/internal/topicgraph/service/daily_report_watch_integration_test.go`；`story_attribute_test.go`（同目录） |

任务验证节包含所有 delta Scenario→真实测试文件的逐行映射，继承项也须回归，不以主链路替代完整清单。

## 2. 继承与调整（⓪ 契约变更）

已运行 `bash scripts/harness/test-assets.sh workshop-board-console` 与 `bash scripts/harness/test-assets.sh daily-report-system` 反查（分别323/790行输出），以下按本 change 实际触及的 Requirement 处置，其余不改。

| 旧 Scenario / 契约 | 处置 | 旧测试 | 动作 |
|---|---|---|---|
| 颜色表达参与者数、图例可见 | 改写 | `front/app/features/tags/components/console/ObservationMatrixGrid.test.ts`、`ObservationMatrix.test.ts`（同目录） | 不再只断言最热N来源，新增同线标题/id与新图例 |
| 同日分桶不冒充分区热度、矩阵格值是桶峰值（旧名称） | 保持数值，补提示 | `front/app/features/tags/components/console/observationLanes.test.ts` | 保留合并计数、零值/断档；不按旧名称恢复峰值 |
| 渲染矩阵/断档可读/零线索可开 | 保持 | `front/app/features/tags/components/console/ObservationMatrix.test.ts` | 数值、行序、点击范围不变 |
| 并行生成成功 | 改写 | `backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go` | 从A+C×K并行改为C×K并行、A在归属/去重后 |
| 完整流水线执行/生成失败 | 改写 | `backend-go/internal/topicgraph/service/daily_report_watch_integration_test.go` | 补归属预算/标签回退/最终写；仅不可恢复正文失败整报失败 |
| Thread输出不含status、分桶/物化失败不阻断 | 保持 | `backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go` | 仅叙事生成C×K与既有降级，不新增线程LLM字段 |
| 日报数据模型、正常highlights落库 | 扩充 | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go`、`backend-go/internal/topicgraph/service/daily_report_llm_test.go` | story与tag并存，服务端快照；旧tag-only不造数据 |
| highlights null/空数组、合法[]、非法响应一次重试 | 保持 | `backend-go/internal/topicgraph/service/daily_report_llm_test.go`、`daily_report_orchestrator_test.go`（同目录）；`front/app/features/tags/components/daily-report/DailyReportMasthead.test.ts` | 标签/故事两模式均保持，归属失败不自动变无重点，LLM失败仍允许[] |
| 跨期重复降级、未归组仍展示 | 保持 | `backend-go/internal/topicgraph/service/daily_report_story_dedup_test.go` | 7天/0.6不变，不因昨日重点徽标恢复flash，降级不减热度 |
| 人工纠正优先、综合稿不建立身份 | 保持 | `backend-go/internal/topicgraph/service/story_attribute_test.go` | 同步调用也跳过人工，不将composite纳入故事候选 |
| 刊头前三条/首条标题摘要/空态 | 扩充 | `front/app/features/tags/components/daily-report/DailyReportMasthead.test.ts`、`dailyReportMagazine.test.ts`（同目录） | 保持前三条与lead选择，只加来源、进展与有落点按钮 |

### 展示字段来源盘点（⑤）

| 字段 | 来源语义 | spec锚点 |
|---|---|---|
| 格值/颜色/title/aria/图例 | 保持计数，获胜线与max同源 | MODIFIED 泳道热度口径 / 矩阵视图 |
| 重点title/reason/tag_ids/story_id | 候选事实LLM白名单；标签回退story为空 | MODIFIED 日报数据模型；ADDED 事件线热度先筛重点候选 |
| 报告title/summary | 首条有效重点；空时既有兜底 | ADDED 重点快照与旧数据兼容 Scenario |
| 来源数/热度/版本/截止时刻/不完整 | 服务端生成时快照，不信模型 | ADDED 事件线热度先筛重点候选 / 重点依据与正文定位 |
| 进展 | 精确昨日重点存活身份交集 | ADDED 昨日重点延续徽标 |
| 定位可用性/目标 | 当前report正文DOM第一条thread | ADDED 重点依据与正文定位 |

## 3. 边界：五组变体走查

### 3.1 输入

| 变体 | 答案 |
|---|---|
| 空串 / 纯空白（全角/tab） | title/reason trim后空为非法；最热线标题空则#ID；不以空白标题换身份 |
| 纯分隔符（单/连/首尾） | 标题不参与身份判定；story_id须正整数，非数字/字符串分隔符输入拒绝；不按标题分词造归属 |
| 单token | 事实标题可短，不强凑字数，身份仍用id |
| 大小写 | 标题大小写不影响身份，story_id数值精确比较 |
| 特殊字符/引号/HTML | Vue文本转义，不用v-html；后端参数化查询，不拼SQL |
| 超长标题/理由 | prompt已有长度要求，UI换行和title/aria可读，不撑宽网格/页面；拒绝空白，不按截断文本匹配 |
| 缺数字 vs 0 | 缺snapshot省略，真实0显示0；不把0当无数据 |

### 3.2 前置

| 变体 | 答案 |
|---|---|
| 空集 | 无事件线走标签模式；标签模型[]合法，保留可见空态 |
| 单元素 | 可选1条，不为凑2–3编造 |
| 重复 | 归并后候选去重；LLM重复故事保首条；同来源多报道只计一次 |
| 越界引用 | 集外/跨board story拒收；坏merge不标进展不定位；其他故事tag剔除后空拒收 |
| 部分满足 | 部分归属失败整轮选择标签模式，已成功归属保留；只读候选不改人工或正文 |
| 全composite / 人工拆分 | 无故事候选，标签回退不造id；正文照常 |
| 仅flash/hidden | 不可定位重点候选；signals/H不变，不恢复正文凑候选 |
| 来源同分/标题缺失 | 存活id最小胜出；空标题用事件线#ID |

### 3.3 时间窗口

| 变体 | 答案 |
|---|---|
| 两端 | story窗口含t-48h及t，48h贡献0.25，t贡献1，t之后排除；24h贡献0.5 |
| 空窗口 | H=0/P=0仍可候选（有正文身份）；无正文则标签回退 |
| 跨窗口/跨月跨年 | 昨日按北京时间日历-1，不能用最近一次有报日；48h按绝对时差 |
| 归一化 | 固定Asia/Shanghai，不依赖宿主TZ；历史生成用报日截止，不用time.Now |
| 昨日无/旧重点 | 无story集合→false，即使前日/prev_report_id匹配也不标 |
| 合并发生在生成后 | 读取双方一跳存活id比较；快照数值不变；坏链保守false |
| 高亮2s | fake timer检查1999ms仍亮、2000ms清理；重复点击重新计时 |
| 归属60s | fake deadline检查临界前可完成、达到预算取消且无迟到提交 |

### 3.4 幂等

| 变体 | 答案 |
|---|---|
| 重复执行 | 固定证据/截止/模型响应时两次重建同一report收敛；signals不重复、候选无累计、重点无残留 |
| 部分失败重试 | 归属部分写入保留，取消后不继续提交；本轮标签模式，后续正常重建重新算，不后台偷偷改标题 |
| 并发 | 沿用既有同板同日生成闸；模拟旧轮最终写与新SaveReport交错，updated_at修订不符不得覆盖；不新增跨板全局锁 |
| UI重复激活 | 最多单目标高亮；切report/卸载清理计时器/焦点引用，无旧报回跳 |

### 3.5 可用性

| 变体 | 答案 |
|---|---|
| 误输入反馈 | 无用户输入表单，不适用；键盘激活定位目标不存在有轻提示 |
| 空态 | highlights=[]/null/缺字段均显示本期暂无重点；矩阵断档与0不混淆 |
| 错误态 | 保持现有错误条/重试；失目标不跳假位置、不新增事件详情请求 |
| 加载态 | 沿用既有骨架，不新增悬空导航；终态在最终重点写后 |
| 超长文本 | 三桌面/窄屏测试标题换行、badge来源行不溢出，aria可读 |
| 重复提交 | 无写按钮不适用；重复定位重置单计时器 |
| 划选与批注 | 选中文字不定位，不抢初始焦点，不破坏既有批注锚点 |
| reduced-motion | 不平滑滚动，焦点轮廓仍可见，高亮不只依赖颜色 |

## 4. 效果核对（实施后执行，不伪填结果）

**触发原因**：事件线回填覆盖、LLM是否真正只选候选及同步时延不能靠纯函数测试代表。**方法**：在静态 :5100 配合只读API抽样，至少选一个多线格与一个昨日同线重点；优先复查 board1974 的既有样本（用户提供29有数据日/13多线日，不声称本阶段重测）。记录：多线格总数、可点名格数/比例；新故事模式重点总数、候选内story比例（目标100%）、有正文落点比例（目标100%）；旧重点不造来源/进展；归属耗时与超时样本数、回退原因。通过fake故障验证60s不迟到写，真实统计不触发写库重建。输出样本日期、计数、分母与结论入 `ui-verification/report.md`，当前结果均未采集。若覆盖不足，报告真实降级比例，不掩盖为零热度。

## 5. 白盒附加（complex）

### 5.1 分支表

| 分支条件 | 路径 / 断言 | 最便宜层 |
|---|---|---|
| 归属成功且body候选>0 | 只调用故事模式一次，最终写顺序在归属/去重后 | Go fake编排 |
| 归属错误/超时/去重错误/候选读取错误 | 只调用标签模式；有效fake响应重点非空；保留正文；取消不迟到写 | Go fake+隔离PG |
| 归属成功但无可定位故事 | 标签模式，不伪造id | Go fake |
| LLM成功/非法/两次失败/合法[] | 合法采用；非法仅重试一次；两次失败[]；合法[]不重试 | Go fake LLM |
| 集外id、tag跨故事、空tag、空白文案 | 非法语义；不能悄然扩大候选范围 | Go解析单测 |
| 重复候选/返回故事 | 候选按存活id去重，返回首条唯一，最多3条 | Go单测 |
| 昨日精确命中/缺日/仅前日/旧重点 | true / false / false / false | PG |
| A→B/坏引用/跨board/多跳循环 | 合法一跳相等；其余false，不改库内人工id | PG |
| thread两条渲染分支/折叠 | 两处皆有稳定锚点，按DOM首次，展开后nextTick定位 | Vitest |
| 无身份/无目标/点击时消失 | 静态 / 禁用+文案 / 原地轻提示 | Vitest |
| 新旧两轮最终写交错 | 旧修订条件更新0行，不盖新报告 | 隔离PG |
| display_tier改变 | 候选资格可变，H/P/signals不变，旧去重判据不改 | Go去重+PG热度 |

### 5.2 边界值清单

- 候选条数0/1/2/3/8/9；返回0/1/3/4；第9条不进prompt、超过3条不展示/不存额外项。
- H=0、H原值接近但四舍五入相同、H完全同/P不同、H/P同/id不同；来源0/1/4，标题空/空白/长文本。
- 时间t-48h-1ns/t-48h/t-24h/t/t+1ns；北京时间跨年昨日；60s与2s边界用fake clock，不真实等待60s。
- story_id=null/0/负数/字符串/候选正数/集外正数，tag空/部分合法/全非法；JSON null/数组/对象/schema回抄。

### 5.3 不适用留痕

无新DDL/数据迁移、无新表单输入、无新路由/弹层、无新线程安全算法、无热度阈值标定：相应用例不新增；现有仓储测试坚持隔离PG，不拿SQLite代替。复杂逻辑mock原型留待批准实施前执行，本阶段只写制品。
