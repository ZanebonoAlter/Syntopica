
## 泳道态势总结瘦身：现状结构与改造要点 <!-- pin:785553ea -->

讨论结论（2026-10 与用户对齐）：14 天档泳道态势总结砍长版、收敛为 150-200 字单版态势句，定位=大致形势+已发生走向（不预测未来），自由成段。事件明细职责完全交给逐日事件时间线。月/年档（enrichment contexts 归档摘要）不动。

关键代码事实：
1. 生成端 backend-go/internal/topicgraph/service/lane_snapshot.go（246行）：
   - laneSnapshotSystemPrompt()：要求 JSON {"summary"≤200字, "detail"≤1000字}成段叙述，两版同素材
   - 解析 parseLaneSnapshotOutput：JSON 有效→两字段各自 truncateRunes 截断（laneSnapshotMaxRunes=200 / laneSnapshotDetailMaxRunes=1000）；JSON 坏→整段截 200 作 summary、detail 空、degraded=true（不重试，下个日报日自愈）；summary 空→error 跳过
   - LLM 调用：operation=daily_report.lane_snapshot，Capability=CapabilityDigestPolish，温度 0，MaxTokens 1536
   - 素材 buildLaneSnapshotUserPrompt：每行「YYYY-MM-DD｜section 标题｜前3条 thread 标题」（LaneSnapshotThreadTitlesPerSection=3，repository 层已截，builder 兜底再截）
2. 素材查询：repository/lane_snapshot_repository.go ListLaneSnapshotMaterial，row=section 级（一天可能多行）
3. 产物表 topic_lane_snapshots：每泳道一行 upsert，rolling_summary + rolling_detail（可空，空串=缺失）+ as_of_date=最新报告期；存量快照 detail 空=合法，随下个日报日结算覆盖自愈、不回填
4. 前端 front/app/features/tags/components/daily-report/LaneTrendOverview.vue（458行）：三档 tab（14d/month/year）；14d 档主视觉优先渲染 detail（summaryText = detail || summary），detailMissing 时降级短版+「长版随下次日报结算生成」提示；14d 档附逐日事件就地展开（默认收起，承担事件明细职责，保持不动）；月/年档走 enrichment contexts 按需拉取（不动）
5. 受影响测试：backend lane_snapshot_test.go；front LaneTrendOverview.test.ts（长版/短版降级用例需改）
6. 长度上限演化史：overview-lane-dynamics(09-11) 定 100/500 → relax-lane-snapshot-length-caps(09-22) 因硬切烂尾放宽至 200/1000；烂尾病因是"要求成段叙述但上限紧"，定位改为形势提炼后字数需求自然下降，不会复发
7. 消费方：板块内容 tab 泳道动态卡片只渲染短版（不受影响）；日报阅读视图泳道展开体顶部 LaneTrendOverview（主改造点）

改造要点（对齐结论）：prompt 改单版态势句（150-200字、自由成段、明令禁止逐日罗列、只描述已发生走向不预测）；素材追加后端算好的量级信号段（每日线索量 + 近7天/上个7天对比）；输出改纯文本（解析只剩 trim+截断+空值守卫，JSON 降级分支删除）；MaxTokens 1536→~400；结算 detail 固定写空串（列保留不删migration）；前端删长版优先/降级提示链，主视觉直接渲染态势句；ui-impact: minor。

<!-- pinned 2026-10-24T13:45:12Z -->

## 态势句定位判据与最终决策（演进叙事版） <!-- pin:3417984c -->

用户三轮样例反馈试出的定位判据（比结论本身更重要，prompt 设计必须遵守）：
1. 带事件罗列的态势句 → 判"和 summary 重叠"（summary 已做定性+点事件，无增量）
2. 纯抽象格局（零事件引用）→ 判"太笼统，还不如 summary"（悬空无信息量）
3. 演进叙事版（阶段+驱动+转折）→ 认可

最终定位（取代 2026-10-24 前一条 finding 中的"纯形势句"表述）：
- 新态势句 = 演进叙事，~200 字：阶段划分（带大致日期锚）→ 每阶段驱动/转折（事件只作转折标记出现，不展开、不罗列）→ 当前落点与局势判断（"首次受挫""转守为攻"这类只有串时间线才写得出的词）→ 量级佐证（近7天 vs 上个7天、走势）
- 认可的示例（美伊博弈泳道真实数据手写）："美伊博弈近 14 天走了三个阶段：上半程（约至 15 日）美方动武讨论层层加码，局势滑向对抗边缘；中段（16-19 日）第三方密集入场——俄方转达构想、中方呼吁外交解决、美方转入选举前暂停，对抗势头首次受挫；近一周伊朗转守为攻，一边主动斡旋也门、一边以强硬动作持续加压，博弈进入「边谈边压」的双轨并行期。报道量尾段连续走高（10-23 起），对抗烈度未见回落。"
- prompt 关键约束：按阶段组织而非按日期罗列；事件仅可作转折标记（带日期锚点出）；必须使用量级信号；只描述已发生走向不预测；自由成段；150-200 字
- 其余决策不变：砍长版短版归一（全泳道只此一段）；纯文本输出（弃 JSON 两字段）；素材加后端算好的量级信号段（每日线索量+7天对比）；MaxTokens 1536→~500；前端删降级链主视觉直渲染；rolling_detail 列保留写空自愈；逐日事件展开不动（事件明细职责）；月/年档不动；ui-impact: minor

<!-- pinned 2026-10-24T14:20:56Z -->
