# Test Cases: slim-lane-snapshot-trend

> 用例先行账本（complex 档）。主链路表串 specs 全 Scenario；⓪ 契约变更表处置旧测试资产；变体走查 + 白盒附加 + 效果核对。

## ⓪ 契约变更了吗——继承与调整表

REMOVED「日报后滚动态势结算」（board-lane-dynamics）与「14 天档内容与降级」（lane-trend-overview）为契约 M（行为重定义），旧测试资产逐行处置：

| 旧测试（资产） | 旧 Scenario | 处置 | 动作 |
|---|---|---|---|
| `service/lane_snapshot_test.go:TestSettleLaneSnapshot_TwoVersionJSONUpsert` | 两版同窗同素材单次生成 | **删除重写** | 换为单版纯文本 upsert（summary=全文截断、detail=空串） |
| `:TestSettleLaneSnapshot_ClampBothVersionsIndependent` | 机械截断保护（两版独立截断） | **改写** | 单版 >200 rune 截断，断言 summary 截断且 detail 恒空 |
| `:TestSettleLaneSnapshot_DetailExactBoundary` | 长版 1000 边界 | **删除** | detail 概念消失 |
| `:TestSettleLaneSnapshot_NonJSONDegrades` | 坏 JSON 降级 | **删除** | 降级分支删除；新增行为记录用例：非纯文本残留（围栏/引号）原样当文本截断入库，不解析 |
| `:TestSettleLaneSnapshot_MissingDetailNotFailure` | 缺 detail 合法 | **删除** | 同上 |
| `:TestSettleLaneSnapshot_MissingSummaryFailsKeepsOldSnapshot` | summary 空失败 | **继承改写** | 空/纯空白（含全角/tab）→ 该泳道结算失败跳过、快照保旧值 |
| `:TestSettleLaneSnapshot_CodeFenceStripped` | 围栏剥离 | **删除** | 纯文本模式不做围栏剥离（见上「原样入库」行为记录用例） |
| `:TestLaneSnapshotSystemPrompt_TwoVersionContractAndMaxTokens` | 两版契约 | **改写** | 断言演进叙事定位（阶段/转折标记/禁止逐日罗列/禁止预测/量级引用/150-200字/纯文本无JSON）+ MaxTokens=500 |
| `:TestBuildLaneSnapshotUserPrompt_MaterialShape` | 素材形态 | **改写** | 保留素材行断言 + 新增量级信号段断言（每日线索量/缺日补零/近7vs上7合计/无报告日口径注记） |
| `:TestSettleLaneSnapshot_LLMErrorKeepsOldSnapshotContinuesSiblings` | 结算失败不阻塞 | **继承改写（机械适配）** | 响应 JSON→纯文本、断言随 detail 恒空适配，失败路径行为不变 |
| `:TestSettleLaneSnapshot_LegacyRowDetailHeals` | 存量自愈 | **改写** | 存量行 detail 有历史值 → 新结算覆盖后 detail 置空、summary 为新态势句 |
| `:TestSettleBoardLaneSnapshots_{WindowAnchorAndUpsertIdempotent,ClampActiveLanes,SilentLaneSkipped,NoReportsNoCalls}` / `:TestSettleLaneSnapshotsSafe_PanicDoesNotAffectReport` | 结算框架 | **继承** | 不动（窗口锚定/上限/跳过/松耦合均未变） |
| `repository/lane_snapshot_repository_test.go:TestGetBoardLaneDynamics_SnapshotDetailLevels` | 携带长版/缺失明示 | **继承** | 读路径不改：存量 detail 值仍透出（无害），前端不消费；测试保留验证存量透出行为 |
| `front/.../LaneTrendOverview.test.ts:FD-1` | 长版就绪 | **改写** | 单版态势句全文渲染 + 汇总截止日（testid `trend-summary`） |
| `:FD-2` | 长版缺失回退+提示 | **删除** | 换为负向用例：detail 非空（存量）不渲染、无「长版待生成」提示 |
| `:FD-3/FD-4/D6×2` | 快照缺失/泳道缺失/错误/加载 | **继承改写（机械适配）** | 断言 testid `trend-detail`→`trend-summary` 适配，行为不变 |
| `:FD-5..FD-15`（月/年档组） | 月/年档 | **继承** | 不动（本 change 不触碰） |

## 主链路表（步 → Scenario → 期望 → 层 → 落点）

| # | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| 1 | 日报成功后结算单版 | board-lane-dynamics·日报完成后结算 | 异步逐 active 泳道 upsert：summary=态势句（截断后）、detail=空串 | Go 单测（mock airouter） | `service/lane_snapshot_test.go` |
| 2 | 构造素材含量级信号 | ·量级信号随素材注入 | user prompt 含「每日线索量 + 近7/上7合计」段，缺日补零，注记无报告日口径；数字来自 repository 计数非 LLM | Go 单测 + PG(testcontainer) | `service/lane_snapshot_test.go` + `repository/lane_snapshot_repository_test.go` |
| 3 | system prompt 定位 | ·演进叙事定位约束 | 含阶段组织要求/事件仅转折标记/显式禁止逐日罗列与未来预测/纯文本输出 | Go 单测 | `service/lane_snapshot_test.go` |
| 4 | 超长输出截断 | ·机械截断保护 | >200 rune 安全截断（rune 边界 199/200/201） | Go 单测 | `service/lane_snapshot_test.go` |
| 5 | 空输出守卫 | ·空输出守卫 | 空/纯空白 → 失败跳过、快照保旧值、下日报日重试 | Go 单测 | `service/lane_snapshot_test.go` |
| 6 | LLM 失败不阻塞 | ·结算失败不阻塞 | 失败记日志、兄弟泳道继续、日报不受影响 | Go 单测（继承） | `service/lane_snapshot_test.go` |
| 7 | 存量行结算覆盖 | ·存量快照兼容 | 历史 detail 保留至覆盖时置空；读取方仅以 summary 为内容源 | Go 单测 + Vitest | `service/lane_snapshot_test.go` + `LaneTrendOverview.test.ts` |
| 8 | 素材/时间线同窗 | ·态势句与时间线同窗 | 窗口锚 MAX(period_date)（继承） | Go 单测（继承） | `service/lane_snapshot_test.go` |
| 9 | 14 天档渲染 | lane-trend-overview·态势句就绪 | summary 全文（pre-line）+ as_of 截止日 | Vitest | `LaneTrendOverview.test.ts` |
| 10 | 存量长版不消费 | ·存量长版不消费 | detail 非空也只渲染 summary，无任何「长版待生成」文案 | Vitest | `LaneTrendOverview.test.ts` |
| 11 | 快照缺失占位 | ·快照缺失 | 「态势待结算」占位、三档切换可用（继承 FD-3） | Vitest | `LaneTrendOverview.test.ts` |

**agent-browser 豁免说明**：本 change 无新交互故事（内容源收敛，交互面零变化），主链路由组件层 Vitest 覆盖；ui-design.md 验收映射保留人工 :5100 目检（部署后）。

## 变体走查（五组，不适用划除留痕）

- **输入**：空串→守卫失败（V5）｜纯空白（全角空格+tab+换行混合）→同空串｜纯分隔符/单 token→按正常文本入库截断（无格式约束，N/A 划除）｜大小写 N/A（中文域）｜特殊字符（围栏/引号残留）→行为记录用例：原样当文本截断入库、不解析｜超长→V4 rune 边界。
- **前置**：素材空集（无 section 泳道）→结算跳过（继承 SilentLaneSkipped）｜单元素（单日单 section 单 thread）→量级段单日一条、对比段按实际天数输出｜重复（同日多 section）→计数求和（ListDailyThreadCounts 聚合）｜越界引用 N/A（无外键引用面）｜部分满足（窗口内仅 2 个报告日）→量级段只列 2 日、缺日补零。
- **时间窗口**：边界两端——as_of 当日计入近 7 天窗（窗首=as_of-6）；sinceDate 含当日（repository 继承）｜空窗口→无报告不结算（继承 NoReports）｜跨窗口（上 7 天全为缺日）→对比段合计 0，不产生负数｜归一化→日期按 period_date 原值，无时区换算（继承）。
- **幂等**：重复执行（同日重跑日报）→结算覆盖重建（继承 UpsertIdempotent）｜部分失败重试→失败泳道下日报日自愈、成功兄弟已落库（继承）｜并发：结算为 detached 串行逐泳道，不声称线程安全（N/A 划除）。
- **可用性（UI 前三必检）**：误输入反馈 N/A（无输入面）｜空态→快照缺失占位（FD-3 继承）｜错误态→请求失败内联错误条+重试（D6 继承）｜加载态→加载提示不显示旧档（D6 继承）｜超长文本→summary 满额 200 rune 全文 pre-line 渲染不截断（FD-1 断言含长文本变体）｜重复提交 N/A（无提交动作）。

## 白盒附加（分支表 + 边界值）

**parseLaneSnapshotOutput（纯文本三步守卫）**

| 分支 | 条件 | 结果 |
|---|---|---|
| P1 | trim 后 len==0（空串/纯空白） | error，调用方跳过该泳道 |
| P2 | trim 后非空 | truncateRunes(200) 返回 |

边界值：0 rune｜1 rune｜199｜200｜201 rune（中文+emoji 代理对混排验证 rune 安全）。

**buildLaneSnapshotUserPrompt（素材 + 量级段）**

| 分支 | 条件 | 结果 |
|---|---|---|
| U1 | row.ThreadTitles 空 | 省略第三段（继承） |
| U2 | ThreadTitles >3 | 取前 3（继承） |
| U3 | 计数映射缺某日 | 该日明示「无报告」（不计入趋势判断） |
| U4 | 近 7 天窗含缺日 | 合计按补零后计 |
| U5 | as_of 距窗口起点 <7 天 | 按实际天数输出对比段，不报错 |

边界值：as_of 当日（窗首）｜as_of-6（窗尾）｜as_of-7（上 7 天窗首）｜全窗单日。

**settleLaneSnapshot 写路径**：detail 恒空串（无论模型输出形态）；summary 空→不 upsert（快照保旧）。

## 效果核对（效果依赖 LLM 行为，真库量化）

- **触发原因**：演进叙事定位依赖模型遵循 prompt，断言外因素。
- **方法**：部署后下个日报日结算完成，SQL 抽 ≥5 条 `topic_lane_snapshots`（覆盖高/中/低活跃泳道），逐条对照判据清单。
- **判据清单**（讨论期用户三轮反馈沉淀）：① 按阶段组织（非逐日罗列——出现「10日…11日…12日」式连续逐日排比即不合格）② 事件仅作转折标记（不展开细节）③ 无未来预测措辞 ④ 量级引用与库内计数一致 ⑤ 长度 150-200 字 ⑥ 含当前落点/局势判断。
- **量化结果落点**：`effect-check.md`（判定 + 不合格样本摘录）。
- **结论规则**：≥1 条不合格→回改 prompt（收紧措辞/增加示例），不放宽判据；全部合格→效果核对通过。
