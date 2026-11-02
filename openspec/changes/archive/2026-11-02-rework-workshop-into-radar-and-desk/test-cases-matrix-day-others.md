# test-cases: 矩阵「当日其他话题」增量（8C.1，锚 `workshop-board-console` + `board-lane-dynamics`）

> 本文件是 2026-10-28 批准增量（tasks 8C）的用例账本，串 `specs/workshop-board-console/spec.md` 与 `specs/board-lane-dynamics/spec.md` 本次新增/修订的 Scenario。既有阶段账本 `test-cases-matrix-stage.md`（S9/五组变体/OX-MX-EV-CH-RD-CA-RS 分支/旧账 53 pass·1 fail·2 未覆盖）与总账 `test-cases.md` **不重写、不洗绿**；本增量改语义/新增的旧条目在下方「继承与调整」逐行处置。总账 S11 入口索引待父登记（本线程写域不含 `test-cases.md`，见 `implementation-matrix-day-others.md` §8 P3）。
> 判据沿用总账四条（一断言一结果 / 负向反向断言 / mock 网络 + `@iconify/vue` / 阈值三点写清方向）；层选择按 `standard/shared/test-design.md`：纯逻辑→函数单测、组件→Vitest、**完整交互故事→agent-browser 端到端（本增量冷启动与成熟不回归各至少一个落点）**。
> 零写路径声明：本增量后端零改动、前端零写操作（日期轴/其他话题组/阅读面板全部只读 GET）；所有「无写」断言以写请求计数 0 / 响应未变异表达。不把离线原型样本、哈希或截图当成产品验收。

## 故事 S11：日报日驱动的日期轴 + 当日其他话题（用户 2026-10-28 已批准）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 进入 0 正式泳道但有日报的版块（246030），默认 14 档 | 日报日驱动的日期轴·零正式泳道保留日期轴 | 矩阵不再用空态替换网格：列头照画，只有报日为可点 button；「0 正式泳道」为行内说明；「当日其他话题」默认展开 | 组件 | `console/ObservationMatrix.test.ts` |
| 2 | 进入成熟版块（1974）默认 14 档 | 当日其他话题折叠组·默认收起 | 正式行/演进/编年不变；「当日其他话题」默认收起且显示真实计数；休眠组独立 | 组件 | 同上 |
| 3 | 点一个有日报的列头 | 日报日驱动的日期轴·列头与日格共用同一选中日 | `selectedDate` 更新为该报日；列头高亮、日格/演进同一份；**不开弹层、不强滚**；所选泳道当日无分区时编年给空态而不回跳 | 组件 | `console/BoardConsoleShell.test.ts` + `console/laneConsoleView.test.ts` |
| 4 | 点无日报的列头 | 同上·报日才可选 | 不可点（非 button）、`title` 说明当日无日报；不改变 `selectedDate` | 组件 | `console/ObservationMatrix.test.ts` |
| 5 | 拖观察范围把所选日排出窗外 | 同上·范围排出回窗内最新报日 | 回新窗内最新报日；无报日则空态；**不跳今天/不跳有 active 泳道日、不改 sourceDays/名单** | 纯逻辑 + 组件 | `console/laneConsoleView.test.ts` + `console/BoardConsoleShell.test.ts` |
| 6 | 展开「当日其他话题」 | 当日其他话题折叠组·成员来自所选日报 | 行 = 所选日报详情 sections 中排除正式/休眠泳道（按 `sectionId` 与 `persistentTopicId` 身份）；行顺序 = `cluster_index` ASC；不含候选池/watch/辅助标签 | 纯逻辑 + 组件 | `console/dayOtherTopics.test.ts` + `console/DayOtherTopicsGroup.test.ts` |
| 7 | 观察组内行 | 同上·只标所选日 | 只有所选日格显示真实线索数（`threads.length`）；其余日期空白非交互（`aria-hidden`、不可聚焦）；不构成跨日泳道 | 组件 | `console/DayOtherTopicsGroup.test.ts` |
| 8 | 悬停/聚焦其他行 | 同上·标题与当时状态 | 两行 clamp + 完整 `title`/`aria-label` 含「完整标题 + 当时状态词 + 点下方读全文」；状态取自 `topic_status_at_report`，不读当前 `topic.status` | 组件 | 同上 |
| 9 | 点其他行/其他格 | 当日其他话题折叠组与就地下读 | `selectedOther` 更新；下方阅读面板换成该分区：完整标题 + 该 section 全量线索；**不弹层/不强滚/不抢焦点**；正式编年不丢（切回泳道即恢复） | 组件 | `console/OtherTopicReading.test.ts` + `console/BoardConsoleShell.test.ts` |
| 10 | 点线索标题 | 与既有工作面同义务·单条真实摘要 | 就地展开 `thread.summary` 逐字；再点另一条时前一条收束；空 summary = 「暂无摘要记录」；标题/摘要不截断 | 组件 | `console/OtherTopicReading.test.ts` |
| 11 | 展开「相关文章」 | 与既有工作面同义务·相关文章按需 | 首次才按 `related_article_ids` 读取（≤20 有界缓存、同文章复用、单项失败局部重试）；文章主点击走壳层既有站内阅读 | 组件 + 纯逻辑 | `console/OtherTopicReading.test.ts` + `composables/useLaneDayReading.test.ts` + `composables/useLaneDayReading.cache.test.ts` |
| 12 | 点「打开 N 日报 / 看归档」 | 既有动作保留 | 阅读台 `/tags/:boardId/:date` / 归档视图带日期；latest 唯一主入口仍在头部，不重复 | 组件 | `console/OtherTopicReading.test.ts` |
| 13 | 刷新/重进而非切板块 | 读取隔离·索引 1 份/详情 14/文章 20 | 报日索引仅 1 次 GET；同报告详情命中共享缓存（泳道编年与其他话题面板同 ID 不重复取）；失败不入成功缓存 | 纯逻辑 | `composables/useLaneDayReading.reportIndex.test.ts` + `composables/useLaneDayReading.reportDay.test.ts` + `composables/useLaneDayReading.cache.test.ts` |
| 14 | 生成新日报完成 | 生成完成刷新继承 | 既有元信息/A 刷新后，报日索引强制刷新：新报日列头立即可选；索引失败只提示陈旧，不阻塞矩阵 | 组件 | `console/BoardConsoleShell.test.ts` |
| 15 | 390 窄屏打开 0 正式泳道版块 | 窄屏同源日期条 | 横向报日条（同 `selectedDate`，选中日滚入视野）+ 其他话题组 + 同一阅读面板；无横向页面溢出；不新增日历/picker | 组件 + 浏览器 | `console/BoardNarrowSummary.test.ts` + `ui-verification/matrix-day-others-<date>/report.md` |

| 16 | 在其他话题阅读区看同归属观察带 | 同归属跨日观察带只列本窗真实记录日 | 同可靠topicId仅列本窗真实记录日；>1日有节点、1日只有说明、无身份/未取到不冒0；计数取同ID同日分区峰值，缺失为未知 | 逻辑 + 组件 | `console/OtherTopicReading.test.ts` + `composables/useOtherTopicObservation.test.ts` |
| 17 | 点一个真实跨日节点 | 点观察节点切换同一选中日并读取真实分区 | 原子更新同一selectedDate和稳定report/section，读取目标日原始内容；无分区不借相似标题 | 组件 + 浏览器 | `console/OtherTopicReading.test.ts` + `console/BoardConsoleShell.test.ts` + S11-C |
| 18 | 观察依赖加载/失败后重试，期间切板块 | 跨日观察缺身份或读取失败不借内容 | 当前日报正文不被阻塞；失败可重试，旧ID/板块/晚到响应不串入 | 逻辑 + 组件 | `composables/useOtherTopicObservation.test.ts` + `console/OtherTopicReading.test.ts` |
| 19 | 0休眠时检查组标题 | 零休眠仍保留同级组标题 | 0/本窗无summary仍在其他组之后，不伪造行；两个组独立 | 组件 + 浏览器 | `console/ObservationMatrix.test.ts` + S11-A |
| 20 | 从其他话题打开日报/归档再返回 | 其他话题打开日报后返回恢复原阅读 | 恢复board/report/section/date与原阅读槽，兼容旧泳道返回，不新增date= query | 组件 + 浏览器 | `console/BoardConsoleShell.test.ts` + S11-A/C |

### 完整交互故事（agent-browser 落点，8C.7必跑）

**S11-A 冷启动（卫生健康 246030，真实静态 `:5100`）**：默认 14 档 → 以实测报日索引为准断言列头可点集合（快照期样本为 10/11、12、13、14、16、22、23、24，不得把样本当现场数量）→ 点最新报日（样本为 10/24）→ 组展开且该日行数 = 该日报详情 section 数减去正式/休眠可读 section（0 正式时行数 = 详情 section 数）→ 点首行 → 面板出现完整分区标题与该 section 全量线索标题 → 展开一条 → 摘要逐字可见、再点第二条时第一条收起 → 展开相关文章 → 文章可点开站内阅读（关闭回阅读面板、焦点回落）→ 打开该日报 → 返回矩阵仍停该报日。**0 正式泳道时页面上不得出现「暂无活跃泳道」阻断网格的空态。**
**S11-B 成熟不回归（中东 1974）**：默认 14 档 → 断言「当日其他话题」默认收起、计数可见、正式行/演进/编年与改动前一致 → 展开组、点一个有真实多行其他分区的报日（原型审计样本为 10/23 四条，以实测为准）→ 逐行标题与日报详情 cluster_label 一致、计数 = `threads.length` → 断言这些行不出现在正式/休眠泳道可读集合中（按 ID 比对）→ 点一个有真实 0 行的报日（样本 10/24，以实测为准）→ 组显示「当日分区都已可在泳道读到」而不是加载失败 → 拖范围跨窗 → 名单/行序/休眠逐项不变、sourceDays 不变 → 收起组后休眠组仍独立可开。

### 变体走查（五组固定清单）

| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 输入：`selectedOther.sectionId` 在当日详情不存在/详情已更新 | 中性「当日报告已更新，未找到该分区」+ 刷新；不借别日/别分区、不静默换行 | 组件 | `console/OtherTopicReading.test.ts` |
| 2 | 前置：0 正式行 / 1 正式行 / 全部休眠 | 0 行组默认展开；≥1 行默认收起；休眠归属不影响组默认态；休眠组仍独立 | 纯逻辑 + 组件 | `console/DayOtherTopicsGroup.test.ts` |
| 3 | 前置：所选日报 0 sections | 组「当日分区都已可在正式/休眠泳道读到」（合法 0，不是失败）；阅读槽空提示 | 组件 | 同上 |
| 4 | 前置：section 带 `persistentTopicId` 但泳道当日 `unavailable` | 仍按身份排除（不重复进组）；组说明追加「部分泳道当日内容未取到」；不补 0 | 纯逻辑 | `console/dayOtherTopics.test.ts` |
| 5 | 时间：所选日无日报 / 范围跨月跨年 / 范围排出所选日 | 无报日 = 组与阅读诚实空态（不是 0 条）；跨月跨年轴连续；排出回窗内最新报日或 null（不跳今天） | 纯逻辑 + 组件 | `console/dayOtherTopics.test.ts` + `console/laneConsoleView.test.ts` |
| 6 | 幂等：重复点同一列头/同一行/重复展开同一条 | 不重复请求（缓存命中/同上下文）、展开态幂等、不重复写 URL | 纯逻辑 + 组件 | `console/OtherTopicReading.test.ts` + `composables/useLaneDayReading.reportDay.test.ts` |
| 7 | 并发：切板块/切日后旧报日索引或详情晚到 | 双 epoch（lane/report）各自丢弃晚到响应，新板块不被污染；写请求计数 0 | 纯逻辑 | `composables/useLaneDayReading.reportDay.test.ts`（RDC-2/3/4b）+ `composables/useLaneDayReading.reportIndex.test.ts`（索引请求序） |
| 8 | 可用性：索引加载中 / 索引失败 / 详情失败 + 重试 | 列头不可选 + 明确提示；矩阵/编年/右轨仍可用；重试确有 GET；失败不冒空集/不冒无日报 | 组件 | `console/ObservationMatrix.dayHeaders.test.ts` + `console/BoardConsoleShell.test.ts`（纯逻辑侧 `useLaneDayReading.reportIndex.test.ts`） |
| 9 | 可用性：某分区 0 线索 | 行显示 `0` 可点；面板给合法空列表（0 条 ≠ 读取失败） | 组件 | `console/DayOtherTopicsGroup.test.ts` + `console/OtherTopicReading.test.ts` |
| 10 | 可用性：超长分区标题/超长线索标题 | 组行两行 clamp + 完整 title/aria；面板全文不截、不撑破布局 | 组件 + 浏览器 | `console/DayOtherTopicsGroup.test.ts` + `console/OtherTopicReading.test.ts` |
| 11 | 可用性：窄屏选中报日在长日期条外 | 选中日滚入视野（无固定 sleep），可继续横滑；点其他报日切换同一 `selectedDate` | 组件 + 浏览器 | `console/BoardNarrowSummary.test.ts` |

不适用划除（固定清单）：输入组「空串/纯空白/纯分隔符/大小写/特殊字符」——本增量唯一自由文本输入是既有 `YYYY-MM-DD` 范围表单（已在 OX-3 覆盖），其他话题组/阅读面板无文本输入；故划除。并发写/线程安全——零写路径，仅读通道 epoch（变体 7）。

### G4a 控制器层落点（8C.5A，不替换上述组件/浏览器落点）

> 本小节只登记**控制器/作用域层**证据，供 G4b（8C.5B）接线时对照；上面的组件（G2/G3）与 agent-browser（S11-A/B/C）落点全部保留、不因本包改写或视为已验。选行/列头/观察节点细则在同包 `otherTopicSelection.select.test.ts`。

| 主链路步/变体 | 控制器层落点 | 证据 |
| --- | --- | --- |
| 3/5（选列头、范围排出回窗内报日） | `useConsoleReading.test.ts`（`bindSelectedDate` 同日不重复/跨日清显示）、`otherTopicSelection.test.ts`（`selectDayHeader` 只改同一 `selectedDate`，轴外/非报日忽略） | 控制器层绿；纯模型 RTY 仍在 `laneConsoleView.test.ts` |
| 9（其他行/格选择、单一阅读槽） | `otherTopicSelection.test.ts`（`selectOther` 稳定 sectionId/reportId/topicId、缺 section 不清选择）、`useConsoleReturnContext.test.ts`（other 恢复） | 控制器层绿；`OtherTopicReading.test.ts` 组件层不变 |
| 13（索引 1 份/详情共享/失败不入成功缓存） | `useConsoleReading.test.ts`（创建即取索引 1 次、锚点晚到覆盖扩大、force 重试确实重取） | 缓存细节仍归 G1 五份子套件 |
| 14（生成完成刷新 + 索引 force） | `useConsoleRefresh.test.ts`（meta→A→after 顺序/防重/after 失败不破刷新）、`useConsoleReading.test.ts`（无选择 invalidate 零 GET / 有选择重读 / 异步切日不回填） | 控制器层绿；Shell 接线归 G4b |
| 17（观察节点原子切日） | `otherTopicSelection.test.ts`（`selectObservedDay` 原子写 + 排队 date watcher 不清新选择 + 轴外/非法 ID guard） | 控制器层绿；节点 DOM 与终态归 G3 |
| 20（其他话题打开日报后返回恢复原阅读） | `useConsoleReturnContext.test.ts`（旧六字段兼容、other 恢复不污染四参、显式 lane 优先/同 lane 往返、0 正式/allLanes 空恢复、同日多 section 精确） | 控制器层绿；真实浏览器往返归 S11-A/C |
| 变体 1（详情缺原 section） | `otherTopicSelection.test.ts`（选择保留）、`useConsoleReading.test.ts`（`sectionFound=false` 且 `applied` 标识） | 中性文案仍由 `OtherTopicReading` 组件测试承担 |
| 变体 6（幂等重复点/展开） | `otherTopicSelection.test.ts`（重复选同一行保持同一身份且不改日期、非 ready 不制造假展开）、`useConsoleReading.test.ts`（同日重复赋值不重复取详情） | 不重复请求断言基于受控 fetcher/状态计数 |
| 变体 7（并发切板块/切日后晚到） | `useConsoleReading.test.ts`（retry 在途切板块 `applied=false` + 旧详情不发布；refresh 在途切日按最新日重读）、`otherTopicSelection.test.ts`（切泳道 sync 清场） | 双 epoch 本体仍归 G1 `useLaneDayReading.reportDay.test.ts` |
| 变体 8（索引加载/失败/重试） | `useConsoleReading.test.ts`（失败只标 indexError、保留旧索引、force 重试恢复）、`otherTopicSelection.test.ts`（index-error 状态可重试） | 列头提示 DOM 归 G2 `ObservationMatrix.dayHeaders.test.ts` |

### G4b 壳层集成落点（8C.5B/8C.5，真实组件链）

> 本小节只登记**壳层集成层**证据（真实 `Shell → Body → Stage → ObservationMatrix/Grid/DayOtherTopicsGroup / BoardNarrowSummary → OtherTopicReading`）；G2/G3 组件层与 G4a 控制器层落点全部保留，S11-A/B/C 浏览器故事仍归 8C.7。详细报告 `ui-verification/matrix-day-others-g4b.md`。

| 主链路步/变体 | 壳层集成落点（新增） | 证据 |
| --- | --- | --- |
| 3/5（列头选日与格/组共一 `selectedDate`） | `BoardConsoleShell.integration.test.ts`（0 正式泳道列头 `button`/非报日 `div`；列头点击后 `ObservationMatrix.props('selectedDate')`；窄屏 `narrow-dayhead-*` 同源） | 真实组件链绿；纯模型 RTY 仍在 `laneConsoleView.test.ts` |
| 9（其他组行/格 → 单一阅读槽） | 同上（成熟板块默认收起 + 计数 → 展开 → 点行 → `OtherTopicReading` 逐字标题/摘要；旧编年/演进不在；泳道高亮抑制；点行零额外详情 GET） | 真实组件链绿；`OtherTopicReading.test.ts` 组件细节不变 |
| 12/16（清场：点同一已选泳道/普通格） | 同上（同 laneKey 再点也清其他选择；普通格同理） | 真实组件链绿 |
| 17（观察带节点原子切日） | 同上（同归属两记录日 → 点另一节点 → 同一 `selectedDate` + 真实 report/section → 阅读槽换分区） | 真实组件链绿；G3 终态 hook 未改 |
| 14（生成完成刷新 + observer 懒失效 + lane 编年重建） | 同上（生成完成 → force 索引/当日报告 → 编年出现刷新后标题；无其他选择观察零 GET） | 真实组件链绿；`useConsoleRefresh.test.ts` 控制器语义不变 |
| 变体 8（索引/详情失败 + 重试） | 同上（详情失败 → Stage `matrix-report-chain-notice` → 重试：force 索引先于 force 详情，`invocationCallOrder` 断言；缺分区「报告已更新」→ 刷新 → 内容回来） | 真实组件链绿 |
| 20（其他话题打开日报后返回 / 内部归档同实例返回） | 同上（内部归档 → 矩阵：其他阅读槽/报日/滚动恢复）；路由往返四参/显式 URL 优先仍由 `BoardConsoleShellNavigation.test.ts` + `useConsoleReturnContext.test.ts` 承担 | 组件层绿；S11-A/C 浏览器往返仍归 8C.7 |

旧 38 例壳层资产拆为 4 个子套件（骨架 10 / 唯一选中日+档 7 / 入口·管理·路由 16 / 版块·归档·失败 5；`fullName+status` 多重集与拆前一致），新增 9 例集成另计，不与旧 38 相加冒充覆盖。

### 效果核对

- 触发原因：其他话题行的「是否真属于所选日报」与「计数是否真实」依赖日报详情覆盖率与 A/B 泳道解析；状态词真实性依赖 `topic_status_at_report` 覆盖率（旧数据可能 NULL）。
- 核对方法（8C.5 真库只读 GET，静态 `:5100`）：对同一板块同一日抽 1 个有正式泳道的板块（1974）与 1 个 0 正式泳道板块（246030）：(a) 组行集合 = 详情 sections 减去「正式+休眠行当日可读 section id 并集」；(b) 行计数 = 该 section `threads.length`；(c) `topic_status_at_report` 有值行状态词与字段一致，NULL 行显示「当时状态未记录/当次未归属」；(d) 正式泳道格值与编年与改动前逐项一致；(e) 全程无写请求。
- 量化结果：8C.5 回填（预期：行集合/计数/状态与 GET 原始响应逐一对上；≥1 个真实 0 行报日与 ≥1 个真实多行报日可辨）。
- 结论：达标交付 / 瓶颈在上游（日报分区完整性、历史状态字段覆盖）——偏差先定位上游，不调 UI 阈值掩盖，也不回填历史。

### 展示字段盘点（改数据结构时必填；完整表见 `implementation-matrix-day-others.md` §3）

| 用户可见字段 | 来源语义 | spec 锚 | 无锚处置 |
| --- | --- | --- | --- |
| 组计数 / 行计数 | 所选日报详情 sections 过滤后的行数 / section `threads.length`（F4） | workshop-board-console「当日其他话题折叠组」 | 已有锚 |
| 行标题 / 阅读分区标题 | 详情 `cluster_label` 逐字（当日原题） | 同上 | 已有锚 |
| 行状态文本 | `topic_status_at_report`（active/candidate/null）→「当时已转正/当时待观察/当时状态未记录/当次未归属」 | 同上 + F4 字段注释 | 已有锚；当前 `topic.status` 禁用 |
| 行徽标 | 不新增（正式行「追踪」/休眠标签保持）；其他行不冒充泳道/候选管理 | 同上 | 已有锚（负向约束） |
| 日期 | 报日索引 `period_date` ∩ 可见轴；`anchorDate`/`today` 分列 | 「日报日驱动的日期轴」 | 已有锚 |
| 摘要 | 详情 `thread.summary` 逐字；空 = 「暂无摘要记录」 | 「当日其他话题折叠组与就地下读」 | 已有锚 |
| 相关文章 / 原文 | `related_article_ids` → `GET /articles/:id` 按需；站内阅读走既有预览 | board-lane-dynamics「矩阵观察窗读取边界」 | 已有锚 |

## 继承与调整（问句⓪：旧契约 M/R 资产逐行处置）

旧资产反查命令与结果见 §实际命令。`workshop-board-console` 无主 spec（本 change 新增能力，delta 即合同，`test-assets.sh` 报无匹配）；`board-lane-dynamics` 主 spec 8 Requirements/25 Scenarios + archive 4 change（含 3 份 test-cases）；`daily-report-system` 只读继承（本次不改 reader/端点）。

| 旧条目（来源账本） | 处置 | 旧测试文件 | 动作 |
| --- | --- | --- | --- |
| workshop-board-console「矩阵视图」Scenario 渲染矩阵 | 改语义（新增：0 正式泳道仍渲染日期轴；报日列头可选） | `console/ObservationMatrix.test.ts`、`console/ObservationMatrix.dayHeaders.test.ts` | 改断言：`emptyState='no-lanes'` 仍渲染 `matrix-grid` + 组；原有 6×14/今天列断言保留 |
| 同上「断档可读」 | 继承（正式格不变） | 同上 | 照跑 |
| 同上「零线索可开」 | 继承 | 同上 | 照跑 |
| 「当日线索工作面」查看某日事件 | 继承（lane 路径不改）+ 新增其他话题路径 | `console/LaneChronicle.test.ts`、`console/OtherTopicReading.test.ts` | 照跑旧断言；新路径用 `OtherTopicReading.test.ts` 覆盖同一义务（全量标题/逐字摘要） |
| 同上「空格提示」 | 继承（仅正式格） | `console/ObservationMatrix.test.ts`、`console/ObservationMatrix.groups.test.ts` | 照跑；其他行无空格的语义由「只在所选日标数」覆盖 |
| 同上「单条真实摘要」 | 继承 + 复用同一 `LaneChronicleThread` | `console/LaneChronicle.test.ts`、`console/OtherTopicReading.test.ts` | 照跑；新面板断言互斥展开与逐字摘要 |
| 同上「相关文章按需」 | 继承（≤20 缓存/单项重试同一 store） | `console/LaneChronicle.test.ts`、`composables/useLaneDayReading.cache.test.ts` | 照跑；新增同文章跨面板（编年↔其他话题）只 1 次 GET |
| 同上「完整读取失败降级」 | **改语义**：其他话题=报告详情优先，没有聚合标题可降级 → 失败=错误态+重试（lane 编年的聚合降级义务不变） | `console/LaneChronicle.test.ts`、`console/OtherTopicReading.test.ts` | lane 断言照跑；新面板断言「失败不伪造标题/不借别日」 |
| 「选中泳道演进带」切换泳道 | 继承 + 补充：切泳道保留当前报日；泳道当日无分区给空态不回跳 | `console/BoardConsoleShell.test.ts`、`console/laneConsoleView.test.ts` | 改/补断言 |
| 「观察窗与自由日期显示」拖选不重排/精确档回切/明确切独立档/档池分层失败/非法日期反馈/深链恢复 | 继承（本增量不改名单/范围/URL 键） | `composables/useBoardConsole.observation.test.ts`、`console/observationRange.test.ts` | 照跑；新增「范围排出所选日不改 sourceDays」断言 |
| 「休眠泳道折叠」折叠/展开休眠 | 改语义（summary 从网格底部前移到自身行前；其他组在前、休眠在后；两 toggle 默认态互不串联） | `console/ObservationMatrix.groups.test.ts`、`console/dayOtherTopics.test.ts` | 补顺序/独立性/summary 位置断言；阈值/只读/全窗口径照旧 |
| 「板块入口作用域」无泳道仍有日报入口 | 改语义（空态不再阻断网格，可直接选日读分区；latest 主入口仍唯一） | `console/BoardConsoleShell.test.ts` | 改 `console-no-lanes-fallback` 断言：仍在且含 latest 入口说明，但网格/组同时在场 |
| board-lane-dynamics「矩阵观察窗读取边界」 | 继承 + 追加（报日索引读列表端点、其他分区读日报详情、0 泳道仍读报日、全部只读） | `composables/useLaneDayReading.reportIndex.test.ts`、`composables/useLaneDayReading.reportDay.test.ts`、`console/ObservationMatrix.dayHeaders.test.ts` | 照跑缓存/稳定 ID/零写断言；新增索引共享/双 epoch 断言 |
| board-lane-dynamics「泳道动态视图入口」只读语义 | 继承 | `lane-dynamics/LaneDynamicsPanel.test.ts` | 照跑（旧面板回归网，保留至阶段④） |
| daily-report-system「按持久话题状态分区」/「展开 thread 文章并复用缓存」 | 继承（阅读台/reader 零改动；本次只读消费同一详情字段） | `daily-report/DailyReportTopicSection.test.ts`、`DailyReportDeskShell` 相关 | 照跑；不新增 reader 用例 |
| 矩阵阶段资产 OX-1/2/7/8/11/12/13/14、MX-1..8、EV-1..7、CH-1/2/5/6/7/8/9/10、RD-1..5、CA-1..8、RS-1..7 | 继承（无口径改动） | 见 `test-cases-matrix-stage.md` 各落点 | 照跑；本增量只新增 RDY/OTH/ORD/GRP/RDC/NAR 组 |
| CH-1「默认选日=最后有分区日」 | 改语义（按泳道优先，报日兜底；范围排出按窗内最新报日，见 RDY） | `console/laneConsoleView.test.ts` | 改/补断言：无泳道 → 报日；切泳道保留报日；排出不跳今天 |
| 根账 53 pass / 1 fail / 2 未覆盖、A-61 等未验收故事 | 不洗绿 | — | 留账不动；本增量通过不改变其状态 |

## 白盒附加（复杂档：分支表 + 边界值 + 不适用划除）

### 分支表 — 日期轴/选中日（RDY，`laneConsoleView.ts` + `useConsoleSelection.ts`）

| # | 条件/分支 | 输入 | 期望 | 测试用例名 |
| --- | --- | --- | --- | --- |
| RDY-1 | 零泳道初始化 | 0 正式行、报日索引含窗内 8 天 | selectedDate = 窗内最新报日；列头仅报日可点 | `reconcile: zero lanes pick latest report day` |
| RDY-2 | 有泳道初始化 | 选中泳道窗内最后内容日 10/22、窗内最新报日 10/24 | selectedDate = 10/22（既有 CH-1 不变） | `reconcile: lane content day before report fallback` |
| RDY-3 | 保留用户报日 | current=10/24（报日，泳道当日无分区） | 保留 10/24（不搬回内容日） | `reconcile: keep report day without lane content` |
| RDY-4 | 保留内容日 | current 在轴内、是内容日但索引暂未含（lookback 边界） | 保留（不因索引缺项搬走） | `reconcile: keep content day outside index` |
| RDY-5 | 范围排出 | current=10/12；新窗 10/15—10/24；报日 10/16/22/23/24 | 回10/24（窗内最新，与冻结稿一致，不回10/16） | `reconcile: latest report day after range cut` |
| RDY-6 | 排出后无报日 | 新窗内无报日 | null（空态）；不改 sourceDays、不跳今天/active 日 | `reconcile: no report day yields null` |
| RDY-7 | 索引未就绪 | 首帧 `reportDates` 空、无泳道 | selectedDate 暂 null；索引到达后 RDY-1 | `reconcile: waits for report index` |
| RDY-8 | 切板块 | 有选中日/其他行 | 复位 null → 重新按 RDY-1/2；其他行选择清空 | `reconcile: board switch resets` |
| RDY-9 | 报日集合含窗内 vs 窗外 | 索引含窗外日期 | 只按可见轴裁切；窗外不作为回退候选 | `reconcile: report days clipped to axis` |
| RDY-10 | 切泳道 | current=报日；新泳道当日无分区 | 保留报日；编年空态不回跳 | `reconcile: lane switch keeps report day` |

### 分支表 — 其他话题组模型（OTH，`dayOtherTopics.ts`）

| # | 条件/分支 | 输入 | 期望 | 测试用例名 |
| --- | --- | --- | --- | --- |
| OTH-1 | 身份排除（sectionId） | 详情 section id ∈ 正式行当日 sections | 不进组 | `others: exclude readable lane section ids` |
| OTH-2 | 身份排除（topic） | `persistentTopicId` ∈ A 名单、当日泳道 gap | 不进组（防重复归类） | `others: exclude by persistent topic identity` |
| OTH-3 | 候选池隔离 | `candidates` 有 5 条但日报无对应分区 | 组 0 行；不拿候选凑 | `others: candidates never enter` |
| OTH-4 | 行序 | 详情 cluster_index 乱序输入 | 按 cluster_index ASC（不按计数/名称重排） | `others: report cluster order` |
| OTH-5 | 计数口径 | section 有 3 threads，lane 聚合 thread_count 不同 | 行计数 3（详情 `threads.length`） | `others: count from detail threads` |
| OTH-6 | 状态词 | active/candidate/archived/null+有 topic/无 topic | 当时已转正/当时待观察/当时已归档/当时状态未记录/当次未归属 | `others: status text from snapshot field` |
| OTH-7 | 大小写/未知枚举 | `topic_status_at_report='ACTIVE'`/`'paused'` | 视为未知 → 当时状态未记录（不猜） | `others: unknown status degrades` |
| OTH-8 | 0 行 | 详情全部被排除 | ready + 0 行 + 「都已可在泳道读到」 | `others: all sections covered` |
| OTH-9 | 无详情 | 详情 error/no-report | 不给计数/不给 0 行；给状态文案 | `others: no detail states` |
| OTH-10 | 部分泳道 unavailable | 1 条 A 行 unavailable | 组照常 + 备注句；不把该泳道分区补进组 | `others: unresolved lane note` |
| OTH-11 | 重复/同题 section | 同日两个 section 同名不同 id | 两行各自保留（按 id），不合并 | `others: same label distinct sections` |
| OTH-12 | 只标所选日 | 行渲染到多列 | 只有 selectedDate 列有值；其余 `aria-hidden`/不可聚焦 | `others: only selected day cell` |

### 分支表 — 其他话题阅读面板（ORD，`OtherTopicReading.vue`）

| # | 条件/分支 | 输入 | 期望 | 测试用例名 |
| --- | --- | --- | --- | --- |
| ORD-1 | 正常 | 详情命中、3 条线索 | 分区标题逐字 + 3 条标题；默认收束 | `other reading: renders full threads`（`console/OtherTopicReading.test.ts`） |
| ORD-2 | 单条展开互斥 | 连点两条 | 仅后一条展开；摘要逐字 | `other reading: single expansion` |
| ORD-3 | 空摘要 / 空线索 | summary='' / threads=[] | 「暂无摘要记录」/ 合法空列表 | `other reading: empty summary and empty threads` |
| ORD-4 | section 已更新 | selectedOther.sectionId 不在详情 | 中性「报告已更新，未找到该分区」+ 刷新；不借别区 | `other reading: missing section` |
| ORD-5 | 相关文章 | ids 空/2 篇/单篇失败 | 空文案 / 按需 2 次缓存读取 / 局部错误+重试不阻塞 | `other reading: related on demand` |
| ORD-6 | 与编年共享缓存 | 同一文章先在编年展开再在其他面板展开 | `getArticle` 仅 1 次（未逐出） | `other reading: shares article cache` |
| ORD-7 | 动作 | 打开日报/看归档 | emit 完整日期；latest 主入口不重复 | `other reading: date actions` |
| ORD-8 | 只读 | 全交互 | 写请求计数 0 | `other reading: read-only` |
| ORD-9 | 受控重试意图（M1） | 错误态点重试 / 缺分区点刷新 | 只 emit `retryReport({date,sectionId})`；组件不调用 store 直接读旧 reportId（同日重建须父先 force 索引） | `M1 失败态点重试只发 retryReport` / `M1 缺分区点刷新同样只发意图` |
| ORD-10 | 重试 payload 上下文 | 壳层切到另一分区后再点刷新 | payload 用当前稳定 date/sectionId，不带旧 sectionId/旧 reportId | `M1 payload 用当前稳定上下文` |
| ORD-11 | 终态信号时序（M2） | 初始 cache-ready 挂载前 / 强刷在途 / 同 context 刷新 / 卸载或换上下文 | 挂载+DOM 更新+报告终态+观察真实终态后才发；刷新在途不发、落定再发一次；旧排队不发；观察失败/无 ID/相关文章未展开不挡 | `M2 初始 cache-ready` / `M2 强刷在途` / `M2 同 context 刷新` / `M2 观察失败…` / `M2 卸载后旧排队` |

### 分支表 — 折叠组（GRP，`DayOtherTopicsGroup.vue` + `ObservationMatrixGrid.vue`）

| # | 条件/分支 | 输入 | 期望 | 测试用例名 |
| --- | --- | --- | --- | --- |
| GRP-1 | 顺序 | 有正式行+两组 | DOM：正式行 → 其他 summary+行 → 休眠 summary+行；两组全宽；休眠 summary 在自身行前 | `group: order and full width` |
| GRP-2 | 默认态 | 0/1 正式行 | 0 → 展开；≥1 → 收起；只初始化一次 | `group: default open for zero lanes` |
| GRP-3 | 独立性 | 先开其他再开休眠/反之 | 两 toggle 互不串联；键盘可聚焦、重渲染焦点保持 | `group: toggles independent` |
| GRP-4 | 折叠 | 收起其他组 | 组行消失；休眠组状态不变；计数仍在 summary | `group: collapse hides own rows` |
| GRP-5 | 空渲染 | 0 行 + 索引失败/无报日 | summary 与状态文案在场（不整块消失） | `group: empty states visible` |
| GRP-6 | 行密度 | 长标题 | 与正式行同高两行 clamp；title/aria 全量 | `group: row density and clamp` |

### 分支表 — 读取层共享/隔离（RDC，`useLaneDayReading.ts`）

| # | 条件/分支 | 输入 | 期望 | 测试用例名 |
| --- | --- | --- | --- | --- |
| RDC-1 | 索引共享 | 壳层启动 + 编年首读同板块 | 列表端点仅 1 次；索引 1 份 | `read: index shared by axis and chronicle` |
| RDC-2 | 详情共享 | 同日同报告：泳道编年与其他话题 | 详情仅 1 次 GET（同一缓存键） | `read: report detail shared across surfaces` |
| RDC-3 | 双 epoch | 切板块时 lane 与 report 均在途 | 两路晚到都被丢弃；互不取消正常读取 | `read: independent epochs` |
| RDC-4 | 报告优先失败 | 索引成功、详情失败 | 组/面板错误态 + 重试；失败不入成功缓存；泳道编年不受阻 | `read: report day failure isolated` |
| RDC-4b | 跨日错配 | 切日后旧报日详情晚到 | 卡片按 `reportDay.date === selectedDate` gate 丢弃；新日期标题下不挂旧日 sections | `read: report day display gate` |
| RDC-5 | 上限继承 | 14 报告 / 20 文章 FIFO | 逐出后当前显示对象不消失/不回顶（CA-8 继承） | `read: bounded caches keep current reading` |
| RDC-6 | 生成刷新 | `ensureReportIndex(force)` | 重新拉列表一次；失败保留旧索引标陈旧 | `read: report index force refresh` |

### 分支表 — 窄屏（NAR，`BoardNarrowSummary.vue`）

| # | 条件/分支 | 输入 | 期望 | 测试用例名 |
| --- | --- | --- | --- | --- |
| NAR-1 | 报日条 | 轴 14 天含 8 报日 | 只报日可点；其余非交互；选中日只滚日期条自身 `scrollLeft`（nextTick 测量后 clamp，无固定 sleep；不 `scrollIntoView`、不改页面纵向滚动） | `narrow: report day strip` |
| NAR-2 | 0 正式泳道 | 0 行 + 有报日 | 报日条 + 其他组（默认展开）+ 阅读槽；不再只有空态 | `narrow: cold start readable` |
| NAR-3 | 迷你条口径不变 | 活跃/休眠行 | 迷你条仍分区数、未知不补 0 | `narrow: mini bar unchanged` |
| NAR-4 | 无溢出 | 375/390 | 页面无横向溢出（报日条自己横滑） | 浏览器（S11-A 窄屏）+ 组件断言 |

### 边界值清单

| 变量 | 边界值 | 期望 | 测试用例名 |
| --- | --- | --- | --- |
| 正式行数 | 0 / 1 / 全部休眠 | 组默认态与顺序 | GRP-2 |
| 报日数（窗内） | 0 / 1 / 8 / 全窗 | 空态 / 单报日 / 正常 / 全可点 | RDY-1/6、OTH-8 |
| 其他组行数 | 0 / 1 / N | 0 行合法文案 / 单行 / 顺序与计数 | OTH-8/OTH-4 |
| section threads | 0 / 1 / 5 / 大值 | 0 可点 / 正常 / 全量（不折叠成 5） | OTH-5、ORD-3 |
| 状态枚举 | active / candidate / archived / null / 未知大小写 | 5 类文案 | OTH-6/7 |
| 索引 lookback | 365 / 369（停更差额） | 池首报日不漏 | 既有 RDY「停更 lookback」+ RDC-1 |
| 报告详情缓存 | 14 / 15 | 恰满 / FIFO 逐出 | RDC-5 |
| 文章缓存 | 20 / 21 | 恰满 / FIFO 逐出 | RDC-5 |
| 日期条长度 | 7 / 14 / 31 / 365 | 桌面网格/横滚；窄屏条横滑 | GRP-1、NAR-1/4 |
| 时区 | 23:59:59 / 00:00:00（Asia/Shanghai） | 「今天」归属（既有口径） | 继承 |

### 分支表 — 获批跨日观察与返回（OBV）

| # | 输入/动作 | 期望 | 落点 |
| --- | --- | --- | --- |
| OBV-1 | 同ID窗内2日、窗外1日、其他ID同标题 | 仅窗内同ID2个节点，不借同标题或窗外记录 | G3纯模型/组件测试 |
| OBV-2 | 无topicId / 0日 / 1日 / 多日 | 分别缺身份/无记录/只说明/画节点，不补0 | G3纯模型/组件测试 |
| OBV-3 | 同日记录数值2、4；或2、null | 峰值4而非6；有缺失则未知不冒0 | G3纯模型测试 |
| OBV-4 | 点另一报日节点，同日多分区 | 原子日期+稳定目标选择，详情核对ID/原序；不误读别条或相似标题 | G4壳组件测试 |
| OBV-5 | 依赖loading/error/retry、切板块后旧响应晚到 | 当前日报仍可读，失败可重试，不串旧ID/板块 | G3依赖/组件测试 |
| OBV-6 | 打开日报/归档后返回、缺分区、旧lane载体 | 恢复其他阅读身份；缺失诚实提示；旧返回不回归 | G4壳组件测试 |
| OBV-7 | 已有数据强刷在途 | `view.status='ready'` 且 `refreshing=true` | 不作本轮终态；终态后归 false；观察强刷不挡补正文 | `强刷在途：…refreshing=true` |
| OBV-8 | 生成后懒失效 | 调 `invalidate()`（选中/未选中其他行） | 不发 GET、丢成功缓存、在途旧响应作废；下次 `ensureObservation` 才重读 | `invalidate 懒失效` / `invalidate 使在途旧响应失效` |

**S11-C（agent-browser）**：真实卫生健康样本选有可靠ID且本窗多记录日的其他分区→核观察带日期/峰值来自section-timeline→点另一节点→核同一selectedDate、真实目标report/section与逐字摘要→打开日报并返回仍是该分区/日期。若现场没有多记录日样本，明确登记真实样本不足并用只读路由夹具补组件/交互证据，不冒真库通过；不得删除故事。

### 不适用划除（留痕）

- SQL/迁移/repository：后端零改动、零迁移，无新 SQL；不触 testcontainer，不跑 Go 包测试。
- LLM/AI/生成：本增量不调 AI、不触发生成/激活/归档/回填；生成完成只验证「刷新索引」的读取行为。
- 并发写/线程安全：零写路径，仅读通道双 epoch（RDC-3），不声称线程安全。
- 输入组自由文本：唯一自由文本是既有日期范围表单（OX-3 覆盖），故划除。
- 跨日同topicId观察带**不得划除**：父已按冻结稿裁决纳入G3，见S11-C/OBV分支与对应spec；当前topic属性不是历史快照，同归属不称同一事件/正式泳道。
- 独立日历/日期 picker/跨日建题/关系：未获批准，不测不实现。

## 真实浏览器主故事与手动豁免计划（8C.7）

- 环境：静态发布 `:5100`（`bash scripts/dev/deploy-frontend.sh` 或既有 generate 铺盘），后端零改动不重启；真实库板块 246030（0 正式泳道、有日报）+ 1974（成熟对照）。
- 视口：1440×900、1920×1080、2560×1440、375×667、390×844；editorial + dark双主题（按阶段风险分层留证，未跑视口不得称已验）。
- 主故事：S11-A / S11-B / S11-C（上方完整交互故事），每条含点击后到达状态断言与「无写」核对。
- 机械断言：列头可点集合与报日集合一致；组行集合与详情差值一致；行计数与 `threads.length` 一致；折叠后行不在 DOM；两 toggle 独立；长标题 ellipsis 有完整 title/aria；面板摘要逐字、无 clamp；窄屏 `scrollWidth ≤ 视口`；弹层（若触发站内阅读）elementFromPoint 命中本体，关闭后焦点回到触发点。
- 手动豁免留痕项：真机触摸横滑报日条（headless 合成指针代验，标注代验）；暗色全图视觉舒适度（k3 截图抽检）。
- 证据落 `ui-verification/matrix-day-others-<date>/report.md`；A-ID 对账沿总账，不新增「通过」结论给未验区域（A-61 等仍留账）。

## 实际命令（本批 8C.1 只跑轻量文档验证；未跑任何产品命令）

- 8C G3（2026-10-29，Linux 本机串行，参数不带 `--`）：`cd front && pnpm test:unit app/api/otherTopicObservation.test.ts app/features/tags/composables/useOtherTopicObservation.test.ts app/features/tags/components/console/OtherTopicReading.test.ts app/features/tags/components/console/BoardNarrowSummary.test.ts --maxWorkers=2` → **4 files / 38 tests passed**（API 8 / hook 10 / 阅读组件 8 / 窄屏 12，含旧 4 例）；相邻回归 `BoardConsoleShell.test.ts` → **1 file / 38 tests passed**；本组 6 个 `.ts` `pnpm exec eslint` → exit 0（Vue 文件不被 eslint 配置覆盖，未声称 Vue lint）。观察带固定 `days=0`（端点全部历史；正数会被后端静默钳到 90 天，覆盖不了 365 池）的证据与未验边界见 `ui-verification/matrix-day-others-g3.md`。
- 8C G3 收口批（2026-10-29 全新短线程）：同一目标集合重跑 → **4 files / 49 tests passed**（API 8 / hook 13 / 阅读组件 16 / 窄屏 12；旧 38 例中 API 原「显式 days 透传」1 例按新契约（不开放窗口 days）替换为窗口守卫、其余保留并新增 11 例：ORD-9/10/11 + OBV-7/8 与 M2 余例，不与旧 38 相加当总数）；相邻回归 `BoardConsoleShell.test.ts` → **1 file / 38 tests passed**；本组 7 个 `.ts` eslint exit 0；S1 拆分后 source 全部 <500 行/<15KB。M1/M2 先红后绿（组件首轮 8 failed | 8 passed，hook 首轮 6 failed | 7 passed，API 首轮 1 failed | 7 passed），窄屏原 12 例未改作结构回归。G4 顺序（`retryReport` → force 索引 → force 当日）、真实几何/触屏与 typecheck/build/浏览器/全量/Go 未跑；证据见 `ui-verification/matrix-day-others-g3-closure.md`。
- 8C.4S 结构规范包（2026-10-29 全新短线程，纯结构，证据 `ui-verification/matrix-day-others-structure.md`）：重构前 **10 files / 189 tests passed**（G1 六文件 118、G2 三文件 55、G3 阅读 16）→ 拆分后同行为集合 **18 files / 189 tests passed**，`fullName + status` 多重集 SHA 前后一致（缺例 0 / 新增 0 / skip 0 / todo 0）；本组 19 个 `.ts` `pnpm exec eslint` exit 0。新落点：lane 主路 `useLaneDayReading.test.ts`、索引/覆盖/请求序 `useLaneDayReading.reportIndex.test.ts`、报告优先 `useLaneDayReading.reportDay.test.ts`、有界缓存 `useLaneDayReading.cache.test.ts`、相关文章晚到 `useLaneDayReading.relatedArticles.test.ts`；矩阵主路 `ObservationMatrix.test.ts`、列头 `ObservationMatrix.dayHeaders.test.ts`、组序/其他行 `ObservationMatrix.groups.test.ts`；阅读 `OtherTopicReading.test.ts`（ORD）、观察带 `OtherTopicReading.observation.test.ts`、M1/M2 `OtherTopicReading.signals.test.ts`；fixture 在三个非 `*.test.ts` harness。未跑全量/typecheck/build/浏览器/Go/部署，真实几何仍留 G5。
- 以下为 8C.1 文档批历史记录：
- `openspec validate rework-workshop-into-radar-and-desk --strict` → `Change '...' is valid`（exit 0）
- `bash scripts/harness/check-tasks-tail.sh openspec/changes/rework-workshop-into-radar-and-desk` → `✓ tasks.md 尾三节与 doc-impact 声明标记齐全`（exit 0）
- `bash scripts/harness/test-assets.sh board-lane-dynamics` → 主 spec 8 Requirements/25 Scenarios；archive 4 change（`2026-09-11-overview-lane-dynamics`、`2026-09-22-lane-trend-overview`、`2026-09-22-relax-lane-snapshot-length-caps`、`2026-10-28-add-intra-lane-theme-bucketing`），其中 3 份含完整 test-cases；Scenario→测试文件映射已按上表取用
- `bash scripts/harness/test-assets.sh workshop-board-console` → `✗ capability 无匹配`：主 specs 与 archive 均无（本 change 新增能力，delta 即合同）
- `bash scripts/harness/test-assets.sh daily-report-system` → 主 spec 27 Requirements/115 Scenarios；archive 15 change（含 9 份 test-cases，共 208 行映射）；本次不改 reader/端点，因此不追加 delta，只读继承「按持久话题状态分区」「展开 thread 文章并复用缓存」旧测试
- 实现批命令形态（逐组）：`cd front && pnpm test:unit <受影响文件...> --maxWorkers=2`（参数不带 `--`）+ `pnpm exec eslint <受影响 .ts>`；typecheck/build 由父集中跑一次，不与测试并行。
