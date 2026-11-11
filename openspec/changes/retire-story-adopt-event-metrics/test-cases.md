# test-cases: 退役事件线，改用 event 标签与当天线索热度

> 四个主故事：S1 看矩阵当天的线索数与热度、S2 日报当天同事件聚簇上今日重点、S3 story 停写后一切照旧、S4 调候选权重。异常与边界就地作为变体走查，不另立故事、不另开能力。
> **本文件是规划态**：所有测试文件路径均为计划路径，当前尚未创建、尚未运行；不得把本文件当作已执行的测试结果。
> 旧契约的回归走查见各故事的「继承与调整」表（问句⓪，依据 `bash scripts/harness/test-assets.sh <capability>` 的反查结果）。

## 故事 S1：看版块矩阵当天的线索数与热度（锚 Requirement：泳道当天热度口径、泳道热度口径（2026-10-27 矩阵阶段修订）、矩阵视图（泳道 × 日期）、泳道卡片构成、泳道动态聚合端点）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 打开版块态势页，矩阵渲染 14 天 × N 泳道 | 渲染矩阵；泳道卡片构成 | 行尾无重复总数列；格值为当日线索合计；断档格与零线索格可区分 | 组件 | planned: `front/app/features/tags/components/console/ObservationMatrixGrid.test.ts` |
| 2 | 点开某泳道某日格，看格内数字与行头 aria | 矩阵格值是桶峰值；同日分桶不冒充分区热度；颜色表达参与者数 | 数字 = 该日各桶线索数之和；aria 给「N 条线索 · M 个主题桶」；无颜色分档 | 组件 | planned: `front/app/features/tags/components/console/observationLanes.test.ts` |
| 3 | 看演进带节点与顶部时间视窗曲线 | 矩阵格与演进节点同量纲；曲线与矩阵格同口径；首屏曲线即含历史 | 节点数字 = 当日线索合计（与格值同值）；曲线逐日值 = 当日线索合计之和，不掺事件线数 / 桶峰值 / 文章数 | 组件 | planned: `front/app/features/tags/components/console/BoardLaneEvolution.test.ts`；`front/app/api/normalizers/laneDynamicsWindow.test.ts` |
| 4 | 看泳道行序与版块关注度 | 活跃排序；同日多条只计一天；关注度为泳道热度之和 | 行序 = 泳道当天热度降序 → 全窗线索合计降序 → topic id；关注度 = 各泳道热度之和（一位小数） | 纯函数 + repository（隔离 PG） | planned: `backend-go/internal/topicgraph/repository/lane_heat_repository_test.go`；`backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go` |
| 5 | 读候选泳道在关注度里的权重 | 候选泳道带当天线索数；活跃与候选按不同权重计 | 候选条目带锚点日线索合计；活跃 × 1.0、候选 × 权重 | repository（隔离 PG） | planned: `backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go` |
| 6 | 雷达首屏看行内小矩阵与分档 | 小矩阵按线索数显示；线索数未知不造零；分档可读不靠颜色 | 小矩阵格值 = 当天线索数；未知标「线索数未知」；分档以文字表达 | 组件 | planned: `front/app/features/tags/components/console/BoardRadarView.test.ts`；`front/app/features/tags/components/console/boardRadarHelpers.test.ts` |

### 变体走查（五组固定清单）

- **输入变体**：线索数 0（真零）｜线索数未知（字段缺失，显示 `?` 不造 0）｜泳道名为空 / 纯空白 / 超长（长名 ellipsis + title）｜同一天多个主题桶（合并计数，不取桶峰值）｜同日无分区（断档 `—`，不可点）。**纯分隔符 / 正则元字符 / emoji 泳道名**：不适用——线索数是计数不是文本解析，泳道名只作展示，不进入任何计算，划除留痕。
- **前置变体**：0 正式泳道但有日报（仍渲染网格与日期轴）｜单泳道｜无任何日报（既有空态）｜泳道只有 candidate 无 active｜话题历史数齐全但当天无 section。
- **时间窗口变体**：当天算不算（当天 = Asia/Shanghai 日历日，与 `period_date` 同日界，含当天）｜前一天的线索不计入今天｜跨月 / 跨年边界｜锚点日滞后于自然今天（用各版块自己的锚点日）｜365 池未取到（提示 + 重试，历史日显示 `?` 不补 0）。
- **幂等变体**：同一份日报数据重复计算热度结果一致（确定性函数，任意历史日可回算）｜重建日报后热度按新数据重算｜连续切档 7/14/30/365 不改名单与行序（拖选不重排）｜拖选到历史区间不重排。
- **可用性变体**：矩阵加载态（沿用既有）｜空态（0 泳道有日报 ≠ 空态）｜错误态（池失败可重试，名单与排序不动）｜长泳道名与长主题桶数展示｜重复点击同一格（选中日幂等）｜非法日期输入（明确文案，原视窗不变）｜深链恢复（`?days/start/end` 恢复，非法项忽略并提示）。

### 效果核对

- 触发原因：新关注度量纲的分档阈值（热点 / 关注）依赖真实分布，测试断言证明不了「阈值合理」。
- 核对方法：在真库（Docker `syntopica-postgres`，只读 SELECT）分布上实测 P80 / P20 并回写 `boardRadarHelpers.ts` 常量，留痕样本、日期、分位点（见 tasks 4.1）。
- 量化结果（2026-11-11 实测）：样本 = 近 14 个报告日内全部版块 × 日的关注度正值 **154 个 / 13 个版块**（库内最新报告日 2026-11-10）；**P20 = 2.3、P80 = 14.5**（一位小数），P50 = 8.5、P90 = 17.9、min 0.5 / max 24.5、均值 8.79。30 日窗复算（321 个样本 / 14 个版块）P20 = 2.0、P80 = 14.0（口径稳定）。写入值：`RADAR_HOT_ATTENTION = 14.5`、`RADAR_WARM_ATTENTION = 2.3`。
- 结论：**已实测并落常量**，旧值 3.3 / 0.7（lane_heat 量纲）与 50 / 15（分区量纲）均不得沿用（4.2 已落可执行断言）。当日（2026-11-10）按新阈值 0 个版块达热点、11 个进「关注」档——这是「当天没有版块落在近 14 日分布前 20%」的真实结论，不是漏算。

### 继承与调整（问句⓪）

| 旧 Scenario | 处置 | 旧测试文件 | 动作 |
| --- | --- | --- | --- |
| 矩阵格值是桶峰值 | 改语义 | `front/app/features/tags/components/console/ObservationMatrixGrid.test.ts` | 断言改为当日线索合计；文件名保留 |
| 同日分桶不冒充分区热度 | 继承 | `front/app/features/tags/components/console/ObservationMatrixGrid.test.ts` | 照跑（回归网），断言口径随新量纲更新 |
| 未回填时保守降级 | 废止 | `front/app/features/tags/components/console/ObservationMatrix.test.ts`；`backend-go/internal/topicgraph/repository/story_heat_test.go` | 「事件身份未回填」三态退役；删除该断言并留痕 |
| 无事件但有分区 | 改语义 | `front/app/features/tags/components/console/ObservationMatrix.test.ts` | 改为「零线索仍显示 0 且可点进当日工作面」 |
| 矩阵数字为事件线数 | 废止 | `backend-go/internal/topicgraph/service/lane_snapshot_test.go`；`front/app/features/tags/components/console/ObservationMatrixGrid.test.ts` | 改断言为当日线索合计 |
| 雷达排序与热度一致 | 改语义 | `backend-go/internal/topicgraph/repository/board_radar_repository_test.go`；`front/app/features/tags/components/console/BoardRadarView.test.ts` | 改断言为泳道当天热度之和 |
| 半衰期衰减 / 窗口外不计入 / 话题热度为事件线热度之和 / 桶数变化不影响热度 / 事件线合并后热度合并 / 历史重算一致 / 落后源标记后重算 | 废止 | `backend-go/internal/topicgraph/repository/story_heat_test.go` | 故事热度公式退役；文件删除或改写为新热度用例（新文件 `lane_heat_repository_test.go`） |
| 同一 feed 多条不叠加 / 不同 feed 各计一次 | 改语义 | `backend-go/internal/topicgraph/repository/story_heat_test.go` | 改为「内容身份去重口径」用例（同一内容多 feed 只算 1 篇） |
| 小矩阵历史未回填日按分区数兜底 | 废止 | `front/app/features/tags/components/console/BoardRadarView.test.ts` | 兜底层退役；改为「线索数未知不造零」 |
| 升温判定 / 无升温 | 废止 | `front/app/features/tags/components/console/BoardRadarView.test.ts` | 升温整档退场；删除断言并留痕 |

## 故事 S2：日报当天同事件聚簇，产出「今日重点」（锚 Requirement：日报同事件聚簇、当天同事件聚簇先筛重点候选、重点依据与正文定位、日报数据模型）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 日报生成，取当天命中的 event 标签 | 同版块当天同事件聚成一簇；综合稿不进聚簇 | 候选集 = 当天命中的 event 标签（去重）；`scope=composite` 的 thread 不进簇、不计篇数 | service（stub embedder） | planned: `backend-go/internal/topicgraph/service/daily_report_event_cluster_test.go` |
| 2 | 批量取向量并在内存聚簇 | 批量调用并留审计；聚簇不落库不改索引 | 向量批量取得（不逐标签调用）；每次调用带非空 operation 且写 `ai_call_logs`；`related_event_embeddings` 与业务表零写入 | service + 隔离 PG | planned: `backend-go/internal/topicgraph/service/daily_report_event_cluster_test.go` |
| 3 | 聚簇结果作今日重点候选 | 候选按簇内文章数截断；同分与精度不反转 | 候选 = 簇 Top 8（篇数降序 → 标签数降序 → 代表标签 id 升序），不按 section 数或泳道热度替代 | service | planned: `backend-go/internal/topicgraph/service/daily_report_event_cluster_test.go` |
| 4 | LLM 从候选里选至多 3 条 | 只可从候选事实写文案；少量候选与合法空返回；无聚簇走标签回退 | 集外 `cluster_id` / 越界 `tag_ids` 判非法并同参重试一次；合法 `[]` 不重试；零候选走标签回退且 `cluster_id` 为空 | service | planned: `backend-go/internal/topicgraph/service/daily_report_highlights_cluster_test.go` |
| 5 | 读刊头「今日重点」与其依据 | 重点快照与旧数据兼容；展示筛选不改变热度 | 服务端快照给出篇数 / 聚簇口径 / 数据截至日 / 研究时点；模型不产出数字；缺字段省略不伪造 0 | 组件 | planned: `front/app/features/tags/components/daily-report/DailyReportMasthead.cluster.test.ts` |
| 6 | 点重点定位正文 | 点击第一条可见正文线程；深链参数定位与失去目标；旧 story 深链被忽略不报错 | 按聚簇定位本期正文首条 thread 并高亮 2s；无落点原地提示；旧 `?story=` 忽略并提示、不报错 | 组件 | planned: `front/app/features/tags/components/daily-report/DailyReportMasthead.cluster.test.ts`；`front/app/features/tags/components/DailyReportReader.test.ts` |

### 变体走查

- **输入变体**：当天 0 个 event 标签（零候选 → 标签回退）｜1 个标签（单元素簇，篇数 1）｜标签描述为空（表示退化为名称 + 辅助标签）｜辅助标签缺失（允许降级）｜超过单批上限的标签数（按批拆分）｜超长文本（超预算显式失败，不静默截断）。
- **前置变体**：全部 thread 都是综合稿（零候选）｜embedding 调用失败 / 超时 / 无可用路由（降级链）｜当天标签的 `topic_tag_embeddings` 缺失（语义向量兜底层不完整）｜`related_event_similarity_threshold` 存量为坏值（回退默认 0.86）。
- **时间窗口变体**：聚簇只含当天（昨天的标签不进今天的簇）｜23:30 兜底路径（不等 embedding 队列，向量可能缺 → 降级标注）｜跨天日报重建（聚簇按新数据重算，不残留旧簇）。
- **幂等变体**：同一天重复生成日报（聚簇结果按当前数据重算）｜重建日报（不残留旧聚簇快照）｜同一批标签不重复计算向量。
- **可用性变体**：聚簇降级（界面注明「本次按主体近似，非向量聚簇」）｜零候选 → 刊头既有空态占位（不整块消失）｜重点无落点（禁用定位并提示）｜长标题 / 长依据展示｜重复点击定位（最多一个高亮）｜reduced-motion（不平滑滚动）。

### 效果核对

- 触发原因：聚簇质量取决于向量表示与阈值，方法断言证明不了「同事件聚得对不对」。
- 核对方法：取真库一个版块的当天 event 标签，按 S2 口径（related-event 表示向量 + cosine ≥ 0.86 单链 union-find，代表 = 最小标签 id，篇数 = 内容身份去重）复算，人工审核簇对「这两条是否值得一起看」，并记录聚类覆盖率（当天标签进入簇的比例）与簇内篇数分布。
- 量化结果（2026-11-11 实测，只读）：样本版块 **15220 / 2026-11-10**，当天 event 标签 **17 个**（非综合稿 thread 命中，去重），17/17 都有向量。按阈值 0.86 复算：**1 个簇（3 个标签）+ 14 个单例**，成簇标签覆盖率 **3/17 = 17.6%**；簇内篇数分布 = `{3 篇: 1 簇}`（该簇 3 条内容身份，无重复副本）。簇样本：`267178 英伟达启动对黄仁勋出轨调查并遭中方驳斥间谍论` × `267275 特朗普称黄仁勋为高素质人才并回应英伟达指控` × `267597 英伟达 CEO 黄仁勋风波未阻 AI 交易回暖`，两两相似度 0.8714 / 0.8613（另一对未达阈值）。第二样本版块 **2197 / 2026-11-10**：16 个标签全有向量、≥0.86 的标签对 **0** 个 → 无成簇。
- **口径如实标注**：本次复算走的是降级链第 2 级「**语义向量兜底**」（`topic_tag_embeddings.semantic`）——主路径（related-event 表示向量）需要调 embedding 模型，会写 `ai_call_logs`，与「真库只允许 SELECT」的红线冲突，故不执行；因此本次结果不能冒充「向量聚簇」口径。
- 初判：15220 那个簇的三条确实是同一事件的不同侧面（指控 → 当事方回应 → 市场反应），**值得一起看**，聚簇方向正确；单链把「市场反应」并入同簇可接受（重点只作阅读入口，不做事实判定）。覆盖率 17.6% 偏低，说明 0.86 阈值「宁可少聚不乱聚」——多数日子重点候选会退化为单例簇 Top 8，这与 D13「候选 = 聚簇 Top 8」一致，但若长期覆盖率都这么低，重点会退化成条数榜，留作 review（6.1）关注点。
- 结论：**样本已留档用户可复核**（版块 15220 / 2197，日期 2026-11-10，阈值 0.86，语义向量兜底口径）；聚类质量结论 = 有簇时方向正确、覆盖率偏低、无假阳簇样本。

### 继承与调整（问句⓪）

| 旧 Scenario | 处置 | 旧测试文件 | 动作 |
| --- | --- | --- | --- |
| 重述降级为快讯 / 新来源判为进展 / 当事方新动作判为进展 / 未归组事件不参与去重 / 降级不改变热量 / 读取路径只回正文线程 | 废止 | `backend-go/internal/topicgraph/service/daily_report_dedup_test.go` | 跨期去重整档退役；文件删除或改写为聚簇用例并留痕 |
| 候选按热度而非桶数截断 / 同分与精度不反转 / 只可从候选事实写文案 / 无事件线走标签回退 / 少量候选与合法空返回 / 重点快照与旧数据兼容 | 改语义 | `backend-go/internal/topicgraph/service/daily_report_story_highlights_test.go` | 断言改为聚簇候选（篇数排序、白名单为 `cluster_id`、快照字段换口径） |
| 昨日同线再入选 / 缺昨日不借前日 / 合并与坏引用 | 废止 | 同上文件（重点相关断言） | 昨日延续徽标整档退役；删除断言并留痕 |
| 点击第一条可见正文线程 / 失去目标与旧重点 / 重复定位与减弱动画 / 展示筛选不改变热度 / 深链参数定位与失去目标 | 改语义 | `front/app/features/tags/components/daily-report/DailyReportMasthead.test.ts`；`front/app/features/tags/components/DailyReportReader.test.ts` | 定位目标由 `story_id` 改 `cluster_id`；深链参数改 `?cluster=` |
| 事件落库时 scope 由事件标签数推导 / 事件未归组不影响日报 / 创建日报记录 / 日报记录唯一性 / 并发落库同板同日不炸唯一约束 / 日报关联昨日报告 / 线程存储在独立表中 / 兄弟桶共享聚类序号 / 未分桶 section 取默认桶序号 / highlights 落库恒为数组 / 历史 null highlights 读出归一为数组 | 继承 | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go` | 照跑（回归网）；其中「事件未归组不影响日报」改写为「`story_id` 恒 NULL 仍照常展示」 |

## 故事 S3：story 停写后，雷达 / 矩阵 / 日报一切照旧（锚 Requirement：日报数据模型、新日报不写事件身份、旧事件身份只读保留）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 关停写开关，生成一期日报 | 新日报不写事件身份 | 所有 thread 的 `story_id` 为 NULL、`story_manual` 为 false、`display_tier` 为 `body`；不新建 / 合并 / 改写 `board_stories` 与 `story_signals` | service + 隔离 PG | planned: `backend-go/internal/topicgraph/service/daily_report_story_highlights_test.go` |
| 2 | 读旧日报 | 旧事件身份只读保留 | 历史 `story_id` 原样保留、不被清空；对外响应不透出事件线身份字段 | repository（隔离 PG） | planned: `backend-go/internal/topicgraph/repository/daily_report_repository_test.go` |
| 3 | 跑 `cmd/backfill-stories` 的写模式 | 回填幂等（旧）已废止；本步验锁写 | `--reattribute` / `--restore` / `--rebuild-signals` / `--backfill-content-keys` / `--fill-story-embeddings` / `--fill-thread-embeddings` 直接拒绝、非零退出并打印退役说明；只读 / `--dry-run` 仍可用 | CLI 单测 | planned: `backend-go/cmd/backfill-stories/main_test.go` |
| 4 | 打开雷达 / 矩阵 / 日报 | 打开工坊首屏；从雷达进入版块；一次请求渲染雷达 | 三处读端正常；单次总览请求仍给全部版块行与泳道清单；无事件线概念的字段与文案残留 | 组件 + 浏览器 | planned: `front/app/features/tags/components/console/BoardRadarView.test.ts`；人工：浏览器实链路留痕（实现阶段写 `ui-verification/report.md`） |

### 变体走查

- **输入变体**：开关值为 `true`（默认，行为不变：仍写 story）｜`false`（停写）｜缺项（回退默认）｜坏值（告警回退默认，不阻断日报）｜大小写 / 空白变体（解析为布尔）。
- **前置变体**：当期无日报（不产生 story 写入也无聚簇）｜日报生成失败（不产生 story）｜重建日报（不产生 story，且不残留旧聚簇快照）。
- **时间窗口变体**：停写开关当天切换（当日剩余日报走新行为）｜跨天切换（次日日报行为一致）。
- **幂等变体**：同一天重复生成日报（不产生新 story 行）｜开关反复开合（幂等，不产生脏数据）。
- **可用性变体**：停写开关误关时的用户可见后果（雷达 / 矩阵 / 日报读端照常，只是口径变了）｜CLI 拒绝时的退出码与文案（明确说明「事件线已退役」而非崩溃）｜历史日报打开（`story_id` 不透出但正文照常）。

### 效果核对

- 触发原因：停写的完整性不能只看开关本身——「还有没有别的写入口」。
- 核对方法：全仓 grep 写路径（`CreateBoardStory` / `AttachThreadToBoardStory` / `MergeBoardStories` / `RecomputeSignalsForStory` / `RecomputeBoardStoryCounts`）的调用点，逐个确认只剩测试调用或已被开关拦住。
- 量化结果（2026-11-11 执行 `grep -rn ... backend-go/internal backend-go/cmd --include=*.go`，命中 100 行）：按文件归类——① **测试文件**（`story_repository_test.go` / `story_signals_test.go` / `story_signals_content_key_test.go` / `story_heat_test.go` / `story_attribute_test.go`）只做仓储单测；② **定义处**（`story_repository.go` / `story_signals_repository.go`）无调用；③ **唯一在线入口** `service/story_attribute.go` 的实现在 `daily_report_story_highlights.go:boundedReportAttribution` 内，开关关闭即提前返回（1.1 已测三态）；④ `service/story_reattribute.go` 只被 CLI `--reattribute` 调用，CLI 写模式在 `main.go:171` 前置拒绝；⑤ `cmd/backfill-stories/*.go` 的写路径同上被拒；⑥ **一条遗留维护路径**：`repository/daily_report_repository.go:406`（`SaveReport` 提交后对「本次重建删掉了 thread 的旧事件线」重算 `story_signals` 与计数）。判定：它不是建线/挂线/合并入口（无 `CreateBoardStory`/`AttachThreadToBoardStory`/`MergeBoardStories` 能力），开关关闭后新报不再写 `story_id`，重跑时 `affectedStoryIDs` 为空 → 自然 no-op；只有「重跑开关上线前生成的历史报」这一情形会刷新旧线派生计数（不清空 `story_id`、不新建身份）。
- 结论：**不存在第二条在线写入口**；遗留的 `SaveReport` 派生计数重算属历史数据维护，不作新建线，列为 review（6.1）关注点如实留痕。

### 继承与调整（问句⓪）

| 旧 Scenario | 处置 | 旧测试文件 | 动作 |
| --- | --- | --- | --- |
| 事件线跨天聚合 / 被合并的事件线保留地址 / 主体不同一票否决 / 同一次发生合并 / 阶段推进标记为进展 / 同主体不同对象分开 / 指纹缺失走模型且保守处理 | 废止 | `backend-go/internal/topicgraph/service/story_assign_test.go` | 归属判定退役；文件删除或改写为「停写后不产生事件线」用例并留痕 |
| 三事件综合稿不建身份 / 单事件文章正常归组 | 改语义 | `backend-go/internal/topicgraph/service/story_assign_test.go` | 改为「综合稿不进聚簇；`scope` 推导仍生效」 |
| 人工指派不被自动覆盖 / 人工拆分后不回滚 | 废止 | 同上文件 | 人工纠正随事件线退役；`story_manual` 变只读历史列 |
| 回填幂等 / 回填不阻塞在线读写 | 改语义 | `backend-go/internal/topicgraph/service/story_assign_test.go`；`backend-go/cmd/backfill-stories/main_test.go` | 改为「写模式被拒 / 只读模式可用」 |
| 事件未归组时字段为空 | 改语义 | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go` | 改为「新日报 `story_id` 恒 NULL」 |
| 同一次调用产出指纹 / 指纹与关键词一致 / 指纹校验失败不阻断标签 / 无关键词可校验时降级 | 继承 | `backend-go/internal/tagmanagement/service/core/extractor_section_test.go` | 照跑（回归网）：指纹产出契约本次不动 |
| 默认关闭态逐字不变 / 影子态写入限定三处 / 事件线身份判定不读实体 ID | 改语义 | `backend-go/internal/tagmanagement/service/entity/entity_mode_test.go`（原映射到 `backend-go/internal/topicgraph/service/story_subject_normalize_test.go`） | 实体层行为一字不动，只改措辞：事件线退役后「不读实体 ID」这条断言改挂在实体层的影子模式用例上（story 侧断言随 `story_subject_normalize_test.go` 一并废止） |

## 故事 S4：调候选泳道权重（锚 Requirement：泳道当天热度口径、版块关注度与分档）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 在「匹配规则」里保存候选权重 | 候选权重在线调整后立即生效 | 权重落 `ai_settings`，重开仍为保存值；保存后新请求用新值 | handler + 组件 | planned: `backend-go/internal/tagmanagement/handler/matching_config_test.go`；`front/app/features/tags/components/MatchingConfigDialog.laneHeat.test.ts` |
| 2 | 回看候选泳道热度与版块关注度 | 活跃与候选按不同权重计；关注度为泳道热度之和 | 候选热度按新权重重算；活跃泳道热度不变；关注度随之变化 | 纯函数 + repository | planned: `backend-go/internal/topicgraph/repository/lane_heat_repository_test.go` |
| 3 | 输入非法权重 | 坏权重存量回退默认 | 越界 / 空 / 非有限数被拒且不覆盖旧值；坏存量值告警回退 0.5，界面照常渲染 | handler + 组件 | planned: `backend-go/internal/tagmanagement/handler/matching_config_test.go` |
| 4 | 检查话题状态机 | 候选权重不影响话题状态机 | 命中计数、连续命中资格、转正门槛不受权重影响；候选话题不因权重为 0 被隐藏 / 归档 / 转正 | repository（隔离 PG） | planned: `backend-go/internal/topicgraph/repository/lane_heat_repository_test.go` |

### 变体走查

- **输入变体**：0｜1｜0.5（默认）｜1（边界内）｜−0.1｜1.1｜空串｜纯空白｜`NaN` / `Infinity`｜超长输入｜保留多位小数（如 0.33）。
- **前置变体**：键缺项（回退 0.5）｜键存在但值非法（告警回退）｜同键多次保存（幂等）｜多字段 PUT（整体失败不部分落库）。
- **时间窗口变体**：权重变更当天生效（立即）｜跨天读取（沿用新值）｜历史日回算（按当前权重，权重是配置不是历史事实）。
- **幂等变体**：重复保存同值结果不变｜读取不产生写入｜并发保存以最后一次为准。
- **可用性变体**：字段非法时错误提示就近展示且保留输入｜保存失败保留输入并提示重试｜长尾数值展示不截断｜未加载完成不提交默认值覆盖旧配置。

### 效果核对

- 触发原因：默认 0.5 是产品口味，不是技术常量。
- 核对方法：用真库各版块最新锚点日的候选 / 活跃线索结构，按权重 0 / 0.5 / 1 复算版块关注度，看排序变化是否合理（是否让候选话题喧宾夺主）。
- 量化结果（2026-11-11 实测，只读，最新锚点日 2026-11-10，关注度 / active 线索 / candidate 线索）：全球科技巨头动态 `9.0 / 13.5 / 18.0`（9 / 9）、中国国内新闻 `9.0 / 12.5 / 16.0`（9 / 7）、美国新闻 `5.0 / 10.5 / 16.0`（5 / 11）、生成式 AI `9.0 / 9.0 / 9.0`（9 / 0）、开发者工具 `5.0 / 8.5 / 12.0`（5 / 7）、中东地缘 `7.0 / 7.5 / 8.0`（7 / 1）、游戏版块 `0.0 / 4.5 / 9.0`（0 / 9）、台湾新闻 `4.0 / 4.0 / 4.0`（4 / 0）。
- 结论：权重 0.5 时「游戏版块」（纯候选，9 条线索）为 4.5，仍低于「中东地缘」（7 active / 1 candidate）的 7.5；权重 1 时游戏版块升到 9.0 反超中东（8.0）——即 **1.0 会让纯候选版块喧宾夺主**；权重 0 时游戏版块归零、且出现三个版块并列 9.0（排序区分度下降）。**默认 0.5 合理，维持不变**（改默认须用户拍板）。

### 继承与调整（问句⓪）

| 旧 Scenario | 处置 | 旧测试文件 | 动作 |
| --- | --- | --- | --- |
| 阈值保存后新查询生效 / 非法阈值不覆盖配置（related-events 旧契约） | 继承 | `front/app/features/tags/components/MatchingConfigDialog.relatedEvents.test.ts`；`backend-go/internal/tagmanagement/handler/related_events_test.go` | 照跑（回归网）：相关事件阈值与本次新增的候选权重键相互独立，不得互相覆盖 |
| 排除不可见证据 / 同 feed 多条不叠加 / 不同 feed 各计一次 | 改语义 | `backend-go/internal/topicgraph/repository/story_heat_test.go` | 「排除不可见证据」随事件线热度退役；其余两条改为内容身份去重口径用例 |

## 白盒附加（complex 档：分支表 / 边界值 / 不适用划除留痕）

> 断言判据由本文件定（口径来自 delta specs）；机械枚举不派子线程，本轮由规划线程直接列出。

### 分支表 A：新热度纯函数

| 输入组合 | 期望输出 | 层 |
| --- | --- | --- |
| active 线索 6 / candidate 线索 4 / 权重 0.5 | 6.0 与 2.0 | 函数单测 |
| active 线索 0 / candidate 线索 0 | 0 与 0（不是未知） | 函数单测 |
| active 线索 0 / candidate 线索 4 / 权重 0 | 0 与 0 | 函数单测 |
| candidate 线索 4 / 权重 1 | 4.0（与 active 同权） | 函数单测 |
| 综合稿 thread 3 条 + 普通 thread 2 条 | 线索数 2（综合稿不计） | 函数单测 |
| 同一天两条 section 合计 7（3+4） | 7（合并，不取峰值） | 函数单测 |
| 权重存量越界 / 非有限数 | 回退 0.5 + 告警 | 函数单测 |

### 分支表 B：日报聚簇

| 分支 | 期望 | 层 |
| --- | --- | --- |
| 两标签 cosine = 阈值 − ε / = 阈值 / = 阈值 + ε | 不成簇 / 成簇（边界含）/ 成簇 | service（stub embedder） |
| 两标签 cosine 完全相同（同分） | 按代表标签 id 升序稳定破同分 | service |
| 候选集为空 / 单元素 / 超过单批上限 | 零候选走标签回退 / 单簇 / 按批拆分且不重复计算 | service |
| embedding 调用失败 / 超时 / 无路由 | 降级到语义向量兜底 → 主体近似兜底，并标注口径 | service |
| 语义向量兜底层向量缺失 | 继续降到主体近似，不静默当成向量聚簇 | service |
| 同一文章被 3 个 feed 抓取 | 簇内篇数 +1（内容身份去重） | service |
| 同文章链接为空 | 按 feed 粒度计 1 篇，不丢弃 | service |
| thread 为 composite | 不进簇、不计篇数 | service |
| 聚簇总预算超限 | 截断到前 N 个标签并标注截断，不阻断日报 | service |
| LLM 返回集外 `cluster_id` / 越界 `tag_ids` / 空白标题 | 判非法 → 同参重试一次 → 仍非法则 `[]` | service |
| LLM 返回合法 `[]` | 不重试，直接落空数组 | service |

### 分支表 C：停写开关与 CLI 锁写

| 分支 | 期望 | 层 |
| --- | --- | --- |
| 开关缺项 | 回退默认（写 story），行为不变 | service |
| 开关 `false` | 不产生 `board_stories` / `story_signals` / `story_id` | service + 隔离 PG |
| 开关值非法 | 告警回退默认，不阻断日报 | service |
| CLI `--reattribute` / `--restore` / `--rebuild-signals` / `--backfill-content-keys` / `--fill-story-embeddings` / `--fill-thread-embeddings` | 全部拒绝、非零退出、打印退役说明 | CLI |
| CLI `--dry-run` / 只读模式 | 仍可运行、零写入 | CLI |
| 同日重建日报 | 不残留旧聚簇快照、不写 story | service + 隔离 PG |

### 边界值清单

- 阈值：0.86 − 1e-6 / 0.86 / 0.86 + 1e-6（聚簇成簇边界含阈值）。
- 权重：0 / 0.0001 / 0.5 / 0.9999 / 1（域内边界）/ −0.0001 / 1.0001 / `NaN` / `+Inf`（域外）。
- 清单上限：候选泳道 7 / 8 / 9 条（第 9 名不展示）。
- 重点候选：簇 7 / 8 / 9 个（第 9 个不进候选）；重点 0 / 1 / 3 / 4 条（最多 3 条）。
- 线索数：0 / 1 / 未知（`?`）/ 断档（`—`）。
- 时间：当天、前一天、次日；跨月首日 / 跨年首日；23:29:59 与 23:30:01（日报兜底时刻前后）。

### 不适用划除留痕

- **状态机循环（≥3 状态）**：本 change 不引入新状态机；停写开关是布尔闸、聚类是单向计算、热度是纯函数——无循环状态，划除。
- **多模块交互协议**：本 change 不新增对外协议（`GET /api/semantic-boards/overview` 与 `GET /semantic-boards/:id/lane-dynamics` 的既有路径不变，仅字段语义变更）——协议本身未改，划除。
- **并发**：本 change 不声称任何新写入路径线程安全；聚簇与热度均为读路径计算，不引入并发写——并发变体划除（除 CLI 拒绝路径外）。
- **纯函数 × SQLite**：本 change 无纯函数用例依赖 SQLite；repository 层用例一律走 testcontainer PG（禁 SQLite），划除。
