# test-cases: daily-report-section-title-decouple

> 交付账本：把 delta specs 的 Scenario 串成完整故事。层选择依据 standard/shared/test-design.md 问句③——本 change 为纯服务端逻辑（无 SQL/迁移/HTTP 契约/前端变更），最便宜层 = 函数/服务单测。

## 故事 S1: 用户在日报里看到板块标题反映当天实际内容，而不是话题创建时的旧事件名（锚 Requirement: section 展示标题内容化）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| 1 | threads LLM 按 cluster 标签事实产出顶层 `section_title`（prompt 指令 + schema 字段 + 事实锚/不复述话题名约束） | 标题遵守事实锚约束 | prompt 含指令；schema 含字段；响应可解析出标题 | 函数单测 | `service/daily_report_llm_test.go`（新增 TestBuildThreadsPrompt_RequestsSectionTitle / TestParseThreadsResponse_SectionTitle） |
| 2 | orchestrator 构建 section 时标题走兜底链：LLM 标题 → 首条 thread title → 话题 label → GroupName | 命中既有话题的 section 标题反映当天内容 | `cluster_label` = 当日标题，非话题 label | 服务单测 | `service/daily_report_orchestrator_test.go`（新增 TestResolveClusterLabel_*） |
| 3 | 模型漏字段 / 返回空 / LLM 失败时逐级降级，最终话题名保底 | 标题生成失败时降级兜底 | 每级降级取值正确，永不出空标题 | 服务单测 | 同上（变体走查 V1-V4） |
| 4 | L3 新建话题 section 标题沿用分组名，lane_tier=l3_new 不变 | L3 新话题标题行为不变 | `cluster_label` = GroupName，归属字段不变 | 服务单测 | 同上 |
| 5 | 归属字段照常写入 | 话题归属字段不受标题影响 | persistent_topic_id / lane_tier / topic_match_* 与标题来源正交 | 服务单测 | 同上（断言既有字段写入不受新路径影响） |

### 变体走查

| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| V1 | 输入：响应含 `section_title` 正常串 | 取 LLM 标题 | 服务单测 | daily_report_orchestrator_test.go |
| V2 | 输入：响应缺 `section_title` 字段（模型漏给） | 降级首条 thread title | 服务单测 | 同上 |
| V3 | 输入：`{"threads":[]}` 空数组 + 无 section_title | 越过 thread 层，降级话题 label（命中时）或 GroupName（L3） | 服务单测 | 同上 |
| V4 | 输入：section_title 为纯空白串（"  "） | 视为无效，走降级链（不 Trim 后留白标题） | 服务单测 | 同上 |
| V5 | 输入：section_title 超长（>500 runes） | 截断或按现行 label 长度约束落库（DB varchar 限制内不炸） | 服务单测 | 同上（断言长度不超列约束） |
| P1 | 前置：cluster 无 tags（空聚） | GenerateClusterThreads 现有短路返回 nil 行为不变，标题落话题 label/GroupName | 服务单测 | 现有测试回归 |
| P2 | 前置：threads 走 synthesizeFallbackThreads（LLM 空 threads 兜底） | 合成 thread 的 title（top tag label 转录）作为标题来源之一 | 服务单测 | 同上 |
| I1 | 幂等：同日重跑日报（覆盖式重建） | 标题随重建重新生成，无重复/残留逻辑（SaveReport upsert 既有行为，标题不引入新状态） | 服务单测 | 现有 SaveReport 相关回归 |
| W1 | watch_keyword / watch_sentence section | 走同一兜底链（无命中话题，落在 thread/GroupName 级），lane_tier 标记不变 | 服务单测 | lane 管线现有测试回归 |
| U1 | 可用性：标题质量（是否复述话题名） | prompt 明令禁止；单测断言 prompt 含「不得复述」指令字样；线上质量人工观察 | 函数单测+人工 | prompt 断言单测；效果核对见下 |

### 效果核对（问句④：效果依赖 LLM 行为）

- 触发原因：标题质量与"不复述话题名"依赖模型遵循度，单测无法证真。
- 核对方法：实现合入后取 board 2128（钉子户案发板）手动触发下一期日报生成，抽查 L1/L2 命中 section 的 cluster_label 与 topic label 是否解耦。
- 量化结果：留待实现后回填（预期：命中 topic 935/38 的 section 标题 ≠ 话题 label）。
- 结论：待回填。

### 继承与调整（问句⓪）

本 change delta 为纯 ADDED Requirements（spec 层无 MODIFIED/REMOVED）。代码行为变更点（orchestrator 标题覆盖逻辑删除）经查无旧测试直接断言"cluster_label = topic label"（orchestrator/lane/embed_text 现有测试均不覆盖该取值），无继承处置行。

| 旧 Scenario | 处置 | 旧测试文件 | 动作 |
| （无——无旧断言资产） | | | |

### 白盒附加（兜底链分支表）

分支：`resolveClusterLabel(threadsResp, threads, matchedTopic, groupName)`（实现时或为内联逻辑，分支语义如下）

| 分支 | 条件 | 输出 |
| B1 | threads 响应 section_title 非空白 | section_title（Trim 后） |
| B2 | B1 落空 && threads[0] 存在且 title 非空白 | threads[0].title |
| B3 | B2 落空 && matchedTopic 存在且 label 非空白 | topic.label |
| B4 | 其余 | cluster.GroupName |

边界值清单：section_title = 正常串 / 空串 / 纯空白 / 缺失字段 / 超长；threads = 空 / 首条 title 空白；matchedTopic = nil（L3/watch_keyword）/ label 空串；GroupName = 空串（最终兜底返回空 → spec 不允许，落库前断言非空）。

不适用划除：~~时间窗口组~~（无窗口语义）；~~并发组~~（未声称线程安全，单报告生成串行）；~~误输入反馈/加载态等 UI 组~~（无前端变更）。

## 故事 S2: 老日报不被动过，时间线靠话题归属串起每天的板块（锚 Requirement: 历史与跨天可读性边界）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| 1 | 变更部署后读取变更前的历史 section | 历史数据不回刷 | 旧 cluster_label 原样保留（无迁移/回刷代码，语义保证） | 人工 | 人工：部署后抽查 board 2128 历史日报标题未变 |
| 2 | 同话题连续多天 section 标题各异，时间线归并 | 时间线跨天串联不依赖标题一致 | 前端按 persistent_topic_id 归并（既有行为，无代码变更） | 人工 | 人工：时间线视图中同话题多日 section 归并为一链 |

### 变体走查

| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| V1 | 前置：单元素（话题仅 1 天 section） | 归并链长度 1，展示正常 | 人工 | 时间线抽查 |
| V2 | 幂等：变更前后各生成的 section 混在同一话题链 | 混合标题（旧话题名 + 新内容标题）均正常归并 | 人工 | 时间线抽查 |
| （其余五组） | 划除：无用户输入面 | | | |

### 效果核对

- 不触发：无依赖断言外因素的可量化效果（归并为既有前端行为）。
