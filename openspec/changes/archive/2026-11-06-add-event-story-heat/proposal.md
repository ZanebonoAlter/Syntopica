<!-- complexity: complex -->
<!-- ui-impact: minor -->
<!-- constraint-domains: daily-report, semantic-board, topic-graph -->
<!-- 域声明理由：编辑路径 backend-go/internal/topicgraph/（→ daily-report）、backend-go/internal/tagmanagement/service/core/（→ semantic-board / topic-graph）、front/app/features/tags/（→ semantic-board）命中 doc-impact-applies 标签 -->

> 设计图谱与代码落点见 [`data-map.md`](data-map.md)；术语以那里的第 0 节为准。

## Why

「热度」现在不是一个指标，是**四个互不可比的计数**，而且全部从日报派生：

| 视图 | 当前口径 | 位置 |
|---|---|---|
| 矩阵日格 | 该话题当日各分区内事件数的**最大值**（桶峰值） | `workshop-board-console/spec.md:22` |
| 演进节点 / 话题趋势 | 同日各分区事件数**之和** | 同上 |
| 行序 | `source` 全窗 `clueTotal` → `sectionTotal` → id | 同上 |
| 雷达关注度 | 版块当日 `section` 数 ×2 + 近3日 + 净增 | `board_radar_repository.go:211-214` |
| 雷达「今天可能升温」 | 今日 `section` 数 ≥ 3 且 ≥ 近3日均值 2 倍 | `useBoardRadarOverview.ts:58-65` |

四个口径带来三个结构性缺陷：

1. **计数单位随编辑策略漂移**。事件是 LLM 按 `k = clamp(round(lines/2.5), 2, 6)` 现场拆出来的，同一批内容换个 `k`，格值就变；雷达关注度更是直接用 `section` 数——`lane_snapshot_repository.go:487-490` 已明确禁止这种排序（「按 section 数排序会奖励被拆得更多的内容」），但雷达正在这么做。
2. **没有参与者去重**。一个 feed 当天发 20 条 = 20；20 家不同源报同一件事也 = 20。矩阵刷热度零成本。
3. **事件身份缺位**。`daily_report_threads` 每天重新生成、无任何跨天字段（`daily_report_models.go:187-209`），`board_persistent_topics` 是**话题**不是事件（同日一话题可拆多桶即证）。于是「这条话题今天有几件事、哪件事有几个来源在报」这个问题，数据上答不出来；跨期去重也无从下手（现有跨天机制是分区相似度匹配，连的是「相似日桶」而非「同一事件」）。

**术语现状先说清**（避免后文歧义）：代码 `daily_report_threads` 一直叫「线索」，但它就是业务上的**事件**（一行关联多篇文章）；`board_persistent_topics` 是**话题**；`section` 是当天临时拆的**分区**，只是展示容器，不是身份。本 change 新增的 `board_stories` 叫**事件线**（一条线贯穿多天的多个事件）。完整术语表见 `data-map.md` §0。

**为什么现在做**：热度要从「日报派生」改成「原始信号派生」，必须先有事件身份；而事件身份所需的原料已经全部在库里——`topic_tags.category='event'` 10,077 个（标签本身就是完整主谓宾句子，平均 20.3 字）、`metadata.event_keywords` 覆盖 99.7%、标签 embedding 覆盖 99.9%、事件 embedding 已有数据、pgvector `<=>` 已在 `daily_report_matching.go:142` 使用。缺的只是把它们连起来的两张表、一段判定逻辑，以及一个稳定的实体键。

## What Changes

- **标签提取产出结构化指纹**：`tagmanagement/service/core/extractor_section.go` 的融合提取调用在同一 schema 内多输出 `subject / subject_key / action / object / occurred_at`（**零新增调用**，token 增量约 30–50）。`subject_key` 是归一化实体键（如 `person:trump`、`company:bytedance`），由模型在同一次调用里完成别名归并（Trump / 川普 / Donald Trump → `person:trump`）。指纹用已有 `metadata.event_keywords` 做一致性校验（`subject` 与 `object` 必须能在关键词里找到），校验失败标记不可信并按现有路径降级。
- **新增 `board_stories`（事件线）**：一条事件线聚合多天的多个事件，承载 `subject / subject_key / action / object / occurred_at / first_seen_date / last_seen_date / thread_count / participant_count / merged_into`。事件线挂在版块下，可选关联首次归属话题；`daily_report_threads` 增加 `story_id` 列（可空）。
- **事件归属判定（确定性优先）**：向量只做召回（放宽阈值），判定顺序为「`subject_key` 不同 → 新建」；`subject_key + action + object` 同 → 同一次发生；`subject_key + object` 同、`action` 不同 → 同一事件线的不同阶段（进展），但反向动作（发布 vs 召回/下架/否认）例外；指纹缺失或不一致（约 5%）才交 LLM 四分类。综合稿（单篇含 ≥2 个 event 标签，实测约 18%）不建事件身份，只做提及。
- **subject 归一化三层**：`subject_key` 精确匹配 → `subject_key` embedding（cos ≥ 0.92）→ `subject` 字面归一。主判据是模型产出的 `subject_key`；embedding 只兜同义拼写差异。**不用 `topic_tags.aliases`**——实测 person/event 的 aliases 覆盖率为 0%，keyword 仅 7.3%。
- **新增 `story_signals`（热度证据）**：一行 = 一个参与者（第一版 `feed:<id>`）在信源时间（`articles.pub_date`）讨论一条事件线。身份（事件归属）与热度（signals）分离，去重只作用于身份层。
- **热度改为确定性公式**：`story_heat = Σ 0.5^(Δt/24h)`（48h 窗口、每个 participant 只计一次）；`lane_heat = Σ story_heat`（半衰期 72h）。**热度从原始信号计算，不从日报派生**，因此改 prompt、改去重规则、改分桶策略都不影响热度曲线，且历史可回算。
- **矩阵与雷达换口径**：矩阵日格改为「当日该话题的事件线数」，颜色深浅表达「当日最热事件线的参与者数」；行序与雷达关注度改为 `lane_heat`；「今天可能升温」改为 `story_heat` 环比。**不扩展界面结构（无新路由、无新弹层、无新交互步骤）**，只换既有格子的数值语义并补一条图例。
- **跨期去重（展示层）**：日报生成时对每个事件查 `story_id`；该事件线最近 7 天已进过正文时，按「文章集合重叠 + 是否有新来源/新动作」判定重述或进展；重述降级为快讯，**不删除事件、不删 signals、不减热度**。
- **历史回填**：按版块 × 日期升序对存量事件执行归属判定，幂等可重跑；失败事件保持 `story_id = NULL`，界面降级为「当日独立事件」，不阻塞现有功能。

**BREAKING（内部契约）**：`aggregate-tagging` 的融合提取 schema、`daily-report-system` 的事件数据模型、`workshop-board-console` 的矩阵格值定义、`workshop-radar` 的关注度定义均发生行为级变更，旧前端若按旧口径解读格值会误读，需同批次发布。

**不含**：不改话题分桶阈值（0.18/0.30）、不改日报分区/话题的生成算法、不改归档口径（`in_report` 仍由 `related_article_ids` 派生）、不引入事件线级 LLM 综述、不做热度历史曲线页面、不接社媒/讨论帖信号（`kind='signal'` 预留但本期只写 `editorial`）、不建独立实体表（`subject_key` 先存在 `board_stories` 上）。

## Capabilities

### New Capabilities

- `event-story`：事件线与事件身份——结构化指纹（含 `subject_key`）、归属判定（subject 一票否决 / 阶段识别 / 综合稿不建身份）、主体归一化三层、合并与别名、人工纠正优先、历史回填。
- `story-heat-evidence`：热度证据与确定性热度——signal 数据模型（参与者 + 信源时间）、热度公式与窗口、事件线/话题两级热度、落后源重算、与日报/去重的解耦边界。

### Modified Capabilities

- `aggregate-tagging`：融合提取标签新增结构化指纹字段（含 `subject_key`）与一致性校验。
- `daily-report-system`：事件数据模型增加 `story_id`/`scope`/`is_development`；日报生成增加基于事件线身份的跨期去重（重述降级快讯）。
- `workshop-board-console`：矩阵日格与行序口径从「桶峰值 / 事件合计」改为「事件线数 / `lane_heat`」。
- `workshop-radar`：关注度与「今天可能升温」口径从「分区数」改为 `lane_heat` / `story_heat`。

## Impact

- **改动代码**：
  - `backend-go/internal/tagmanagement/service/core/extractor_section.go` + `extractor_enhanced.go`：提取 schema 与解析新增五字段（含 `subject_key`）。
  - `backend-go/internal/topicgraph/repository/`：新增 `board_stories`、`story_signals` 模型与迁移；`daily_report_models.go` 的事件增加 `story_id`/`scope`/`is_development`。
  - `backend-go/internal/topicgraph/service/`：新增事件线归属 job、主体归一化、回填命令；`daily_report_orchestrator.go` 接入跨期去重。
  - `backend-go/internal/topicgraph/repository/board_radar_repository.go`：关注度改 `lane_heat`。
  - `backend-go/internal/topicgraph/repository/lane_snapshot_repository.go`：矩阵响应补充事件线数与参与者数。
  - `front/app/features/tags/`：矩阵格值渲染、颜色通道与图例；雷达关注度展示；文案「线索」统一改「事件」。
- **数据影响**：新增两张表 + 三列；存量数据不回写正文，只回填 `story_id` 与 signals。事件归属与热度均可重算，不产生不可逆变更。
- **部署后用户可见变化**：
  - 矩阵日格数字**含义改变**（从「分区内事件峰值」变成「事件线数」），同日多分区不再让数字变大；格子新增颜色深浅表达「最热事件线有几个来源在报」，界面补一条图例说明。
  - 雷达关注度排序可能变化（不再奖励分区拆得多的版块），「今天可能升温」的判据从分区数改为事件线热度。
  - 日报正文可能出现「同一事件前几期已报过」的条目被降级到快讯，正文条目数可能减少。
  - 界面文案中「线索」统一改为「事件」。
- **需要用户手动执行的操作**：
  - 部署后执行一次事件线回填命令（提供 CLI 与幂等保证；未执行时界面降级为「当日独立事件」，不报错）。
  - 回填完成后核对雷达分档阈值（按 `lane_heat` 分布标定，不沿用旧 50/15）。
  - **旧口径的历史热度不会自动转换**，因为此前热度未落历史表。
- **旧数据降级**：无 `story_id` 的事件照常展示、照常计入日报，只在「事件线数」「热度」两处按「每个非综合稿事件记为 1 条独立事件线、参与者按 feed 去重」保守计算，不阻塞任何现有功能。
- **执行窗口**：回填需遍历存量事件并做向量召回，建议低峰执行；不影响在线读写（只 UPDATE `story_id` 与 INSERT signals）。
