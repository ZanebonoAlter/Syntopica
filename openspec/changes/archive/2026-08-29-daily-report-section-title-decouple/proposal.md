<!-- constraint-domains: daily-report -->

## Why

日报 section 一旦挂上持久话题，展示标题（`cluster_label`）被强制改写为话题 label（`daily_report_orchestrator.go` 构建 section 时 `clusterLabel = topic.Label`）。话题 label 冻结在创建时的事件名上，导致后续每天的新内容顶着过期旧标题展示——实测 board 2128 的「日本首相高市早苗宣布不于7月释放石油储备」（topic 935，7-25 建）连续两周顶着 8 月的抗议/自辩/预算等全新内容；「日本5月核心通胀率持稳」（topic 38，6-20 建）同理。L1 门禁（candidate-topic-l2-gate）只管挂载路径管不到标题层，钉子户无法自愈。

## What Changes

- section 展示标题改为**按当天实际内容新拟**（复用既有 threads LLM 调用顺带产出，遵守日报文案生成事实锚约束），不再复用所挂话题的 label。
- 话题退为纯归属锚点：`persistent_topic_id` / `lane_tier` / `topic_match_confidence` / `topic_match_distance` 语义不变，跨天演进链路不受影响。
- L3 新话题（GroupName 本就是当天 LLM 命名）与 watch 物化 section 统一走同一标题来源，行为不变或对齐。
- 标题生成失败时的兜底链：LLM 当日标题 → 当日代表 thread title →（旧兜底）topic label / GroupName，故障时降级回旧行为，不出空白标题。
- section 内容化 embedding（`buildSectionEmbedText`）的兜底文本随标题一并内容化，质心随实际内容漂移（现有设计意图）不受阻。
- L2 裁决 prompt 中"近期 section 框架名"信号自动随内容化标题变得鲜活，无 prompt 结构变更需求；标题生成属文案产出，`promptVersion` 相应升级。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `daily-report-system`: 新增 Requirement「section 展示标题内容化」——规定 `cluster_label` 的来源为当天内容派生（LLM 事实锚产出 + 兜底链），话题 label 仅作归属与兜底信号，不再作为展示标题的默认值。

## Impact

- 代码：`backend-go/internal/topicgraph/service/daily_report_orchestrator.go`（section 构建处标题解析）、`daily_report_llm.go`（threads 产出加 section 标题）、`daily_report_models.go`（如需区分存储当日标题）。前端仅展示既有字段，无接口变更。
- 数据：`daily_report_sections.cluster_label` 语义从"话题名复读"变为"当日内容标题"；历史数据不回刷（旧 section 保留旧标题，自然分界）。
- 风险：标题质量依赖 LLM；同话题跨天标题不同属预期（时间线靠 topic 归属串联），需在验收场景中固化。
