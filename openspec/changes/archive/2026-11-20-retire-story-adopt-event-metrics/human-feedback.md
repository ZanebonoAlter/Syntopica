# 用户反馈：退役事件线，改用 event 标签与当天线索热度

> 本 change 的产品方向由用户于 2026-11-11 拍板。下文保留用户原话痕迹与逐条处置；制品中的口径一律按此处拍板结果写死，实现阶段不得自行改回。

### FB-1 2026-11-11【产品|fact-check】story 这一层产品设计上不可取
- **状态**：已消化
- 用户：「story 这一层太依赖模型能力和修剪了，从产品设计上不可取，雷达/热度/日报用到的 story 我打算换成现在的 event 标签以及其包含的辅助标签」。
- 处置：本 change 的立项理由。雷达矩阵、泳道热度、雷达关注度、日报「今日重点」四处度量口径全部换成 event 标签与当天线索几何；story 在线生产停写、旧数据只读保留。理由与实测证据见 proposal.md 的 Why（引归档 change 的 explore-findings 三根因 / 25 对召回盲区 / 138 组 subject_key 漂移）。

### FB-2 2026-11-11【产品|fact-check】矩阵日格取当日线索合计，不用桶峰值
- **状态**：已消化
- 用户：矩阵日格口径拍板为「每泳道当天 thread 总数」，与演进带 / 趋势图统一口径，不用桶峰值。
- 处置：矩阵日格 = 该泳道该日各主题桶线索数之和；演进节点与顶部时间视窗曲线同量纲；不再有「事件线为 0 时按线索合计兜底」这层降级。口径见 design D4 与 `workshop-board-console` delta。

### FB-3 2026-11-11【产品|fact-check】日报同事件聚簇取方案 D
- **状态**：已消化
- 用户：拍板方案 D——日报生成时内联现算当天 event 标签的 related-event-v1 向量 + 内存聚簇，不落库、不做身份、不跨天；0.86 阈值沿用且可调；每板新增一批 embedding 调用，带 operation 进 `ai_call_logs` 审计；先例取归档 change 的 experiment 模式。
- 处置：见 design D11 / D12 / D16 与 `related-events` delta「日报同事件聚簇」。表示与阈值复用 `related.BuildRepresentation` 与 `related_event_similarity_threshold`，不另立阈值；降级链为「语义向量兜底 → 主体近似兜底」并逐级标注口径。

### FB-4 2026-11-11【产品|fact-check】雷达右栏换成当天线索最多的泳道
- **状态**：已消化
- 用户：「今天可能升温」整档退场，换成「当天线索最多的泳道」清单。
- 处置：右栏条目单位由事件线改为泳道，跨版块按当天线索数降序、上限 8，每条带版块名 + 泳道名 + 当天线索数；「升温 N×」「新起」行内徽标与「跨天延续事件线建设中」脚注一并退场。见 design D10 与 `workshop-radar` delta「当天线索最多的泳道清单」。

### FB-5 2026-11-11【产品|fact-check】story 处置 = 停写 + 切读端
- **状态**：已消化
- 用户：归属判定停写入口加开关、旧数据只读保留、界面不再透出事件线概念。
- 处置：见 design D1 / D2 / D3。开关落 `ai_settings`（可回滚灰度），代码与表都不删；`cmd/backfill-stories` 写模式直接拒绝以堵死旁路；`board_stories` / `story_signals` / `daily_report_threads.story_id` 只读保留、不迁移。`lane-compose-story-chain` 已由用户归档，与本 change 无冲突，proposal.md 已提一句。

### FB-6 2026-11-11【产品|fact-check】热度公式只表达当天
- **状态**：已消化
- 用户：热度公式拍板为「关注（active）泳道当天线索数 ×1.0 + 候选（candidate）泳道当天线索数 ×0.5」，权重键 `ai_settings.lane_heat_candidate_weight` 默认 0.5、域 [0,1]、可在线调；只表达当天热度，废半衰期。
- 处置：见 design D6 / D7 / D8 与 `lane-heat-metric` delta。候选权重只进热度，SHALL NOT 参与话题生命周期判定（topic-graph 红线 1–4 不动）；新热度是确定性函数、任意历史日可回算，不落历史表。

### FB-7 2026-11-11【产品|fact-check】矩阵颜色通道退役
- **状态**：已消化
- 用户：矩阵颜色通道退役，数值 + aria 承载全部信息（符合现行红线「颜色不得是唯一载体」）。
- 处置：见 design D5。矩阵日格与雷达行内小矩阵都不再渲染颜色分档，`laneLevel` / `laneDegradedLevel` 退场；矩阵口径说明改为「数字 = 当日线索合计」。
- **修订（2026-11-11 用户看版后，见 FB-15）**：颜色回归，但换成线索口径——行内归一的连续深浅，不再是旧事件线来源数四档；数值 + `title` / `aria` 仍为唯一信息承载。

### FB-8 2026-11-11【产品|fact-check】关注度分档阈值必须重标定
- **状态**：已消化
- 用户：新口径落地后用真库分布实测重标雷达关注度分档阈值（现 `boardRadarHelpers.ts` 的 3.3 / 0.7），不预设数值。
- 处置：见 design D9 与 tasks 4.1 / 4.2。规格只要求「按新分布标定并留痕」，不写死数值；重标定前不得对外宣称新量纲的热点 / 关注分档已定型；并加一条断言禁止沿用旧 3.3 / 0.7 与更早的 50 / 15。

### FB-9 2026-11-11【产品|fact-check】簇内文章数按内容身份去重
- **状态**：已消化
- 用户：聚簇的簇内文章数按文章内容身份去重（复用 `heatParticipantKey` 口径，吸取 `fix-duplicate-article-heat` 的教训）。
- 处置：见 `lane-heat-metric` delta「线索计数的内容身份去重口径」。去重键 `COALESCE(NULLIF(content_key,''), participant_key)`，同一篇内容被多 feed 抓取只算 1 篇；该口径是所有对外计数的唯一实现点，`story-heat-evidence` 的「参与者身份与去重」退役时由它继承。

### FB-10 2026-11-11【产品|fact-check】回填 CLI 写模式直接拒绝
- **状态**：已消化
- 用户：`cmd/backfill-stories` 的写模式（`--reattribute` / `--restore` / `--rebuild-signals` 等）直接拒绝，只读 / 实验模式保留。
- 处置：见 design D3 与 tasks 1.2。拒绝时非零退出并打印「事件线已退役」说明，CLI 文件不删（便于回滚）。

### FB-11 2026-11-11【产品|fact-check】scope 综合稿判定保留
- **状态**：已消化
- 用户：`scope` 综合稿判定保留（不依赖 story）。
- 处置：见 design D15。`scope` 仍由 `tag_ids` 中 event 标签数推导（≥2 = composite），composite 的 thread 不进任何当天聚簇、不计入簇内文章数。

### FB-12 2026-11-11【流程|process】停写开关走 ai_settings 布尔键
- **状态**：已消化
- 用户：停写开关用 `ai_settings` 布尔键（可回滚灰度），不删代码不删表。
- 处置：见 design D2。键名 `story_attribution_enabled`，缺项回退默认（保持兼容）、坏值告警回退；回滚 = 把键改回 true，不需要数据修复。
- **修订（2026-11-11 review R-1，用户拍板）**：该键整体删除，停写改为硬停写（编译期约束），回滚改 `git revert`；见 design D2「决策修订」。

### FB-13 2026-11-11【产品|fact-check】今日重点仍调 LLM，候选源换聚簇
- **状态**：已消化
- 用户：今日重点（highlights）仍调 LLM，但候选源从事件线换成聚簇簇；prompt 重写、禁编造。
- 处置：见 design D13。候选 = 聚簇 Top 8（簇内文章数降序 → 簇内 event 标签数降序 → 代表标签 id 升序），模型返回的 `cluster_id` 与 `tag_ids` 必须在白名单内，非法同参重试一次、合法空不重试；篇数 / 聚簇口径 / 数据截至日 / 研究时点由服务端补快照，模型不产出数字。

### FB-14 2026-11-11【流程|process】任务与用例聚焦主故事
- **状态**：已消化
- 用户：任务与测试用例不宜过多发散，主要集中在主要用户故事。
- 处置：test-cases.md 只保留四个主故事（S1 看矩阵线索数与当天热度、S2 日报同事件聚簇上今日重点、S3 story 停写后一切照旧、S4 调热度权重），异常与边界就地作为变体走查，不另立故事；tasks.md 按依赖排序成 8 节，不为每个分支另开任务。

### FB-15 2026-11-11【产品|frontend】矩阵色深按当日线索数回归（看版后修订）
- **状态**：已消化
- 用户：「矩阵色块按当日线索数连续深浅着色」——纯数字版看不出相对多少，颜色要回来，但口径换成线索数，不是旧的事件线来源数。
- 处置：见 tasks 3.5。矩阵日格与雷达行内小矩阵共用 `clueDepth`：色深 = 该格当日线索合计 ÷ 本行可见窗最大值（行内归一、线性插值；行内最大 = 最深、0 = 底色、跨行不归一），`clueCount` 为 null 不着色；数值 + `title` / `aria` 仍为唯一信息承载，红色线「颜色不得是唯一载体」不破。顶部曲线与演进节点不着色；旧 `laneLevel` / `laneDegradedLevel` 四档不恢复。
