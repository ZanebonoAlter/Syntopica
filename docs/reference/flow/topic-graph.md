# 话题图谱流程（Topic Graph）

<!-- doc-impact-applies: backend-go/internal/topicgraph/, backend-go/internal/tagmanagement/, front/app/features/tags/components/daily-report/, front/app/features/tags/components/TopicGraphMergeDialog.vue | section=业务约束与不变量 -->
> 大功能：PersistentTopic 日报归属、展示，关系双轨。
> 跨端。互补：`flow/daily-report.md`（叙事生成驱动话题）、`architecture/backend.md`。

## 需求说明

PersistentTopic（持久话题）解决「跨天追踪同一话题演进」的问题。每天生成的日报 section 是一次性的叙事单元，但用户关心的 AI / 新能源 / 中美竞争等**话题是跨天存续、逐日累积**的。话题图谱把每天的 section 锚定到持久话题上，让用户：

- 在话题泳道里看到同一主题**跨天的连续叙事**（identity 轨），而不是每天孤立的一团。
- 在时间线上看到话题的**生命周期状态**——emerging / continuing / split / merge / ending（similarity 轨）。
- 通过 candidate→active 的渐进确认机制，避免噪声 / 瞬态事件污染持久话题集合；任何归档都由用户显式操作触发。

> 话题聚类消费的 event 标签来自标签提取 / 去重 / 层级管线（tagmanagement domain，详见 `flow/semantic-board.md`）；标签质量直接影响聚类纯度。本 flow 聚焦 PersistentTopic 的归属、生命周期与关系双轨。
>
> **边界（related-events，enable-story-cross-day-continuation）**：图上的「相关事件」是**读侧推荐**（event 标签按名称/描述/有效 aux 的语义相似度），不参与本 flow 的任何判定——不读 `subject_key`/实体 ID/指纹信任/thread 向量，不建事件线、不挂 thread、不改 `board_stories`/`story_signals`/热度/`display_tier`，也不做传递聚类。事件线身份判定顺序与红线 12–16 逐字不变。

## 链路设计

### 日报 section 归属与话题启用

```mermaid
flowchart TD
  SAVE[日报 section 保存] --> CONFIRM{双重确认归属已有 topic}
  CONFIRM -->|一致| BELONG[归属已有 topic]
  CONFIRM -->|不一致| CAND[创建 candidate]
  CAND --> THRESH{consecutive_hits 达阈值}
  THRESH -->|是| QUALIFY[仅获人工确认资格<br/>仍保持 candidate]
  THRESH -->|否| WAIT[继续累计]
  QUALIFY --> PATCH[话题管理 PATCH status=active 转正]
  PATCH --> ACTIVE[只有 active topic 进入<br/>独立持久泳道]
```

双重确认锚定（AND-gate，详见 `flow/daily-report.md` §2）是归属的第一道门：语义门（section 标题向量 ↔ 话题首义向量余弦距离 ≤ 0.30）+ LLM 门（Step3 聚类的 `matched_topic_id` 指向同一话题），两道都过才 anchor_hit 到已有 topic；否则 auto_new 开新 candidate。

### 事件标签结构化指纹与主体归一化（add-event-story-heat）

event 标签（本身就是完整主谓宾句）自 add-event-story-heat 起携带**结构化指纹**，作为事件线（`board_stories`，见 `flow/daily-report.md` §事件线归属与跨期去重）的身份判据：

| 字段 | 语义 | 备注 |
| ---- | ---- | ---- |
| `subject` | 主体原文（公司/机构/人物/组织），展示用 | 可能是「特朗普」也可能是「Trump」 |
| `subject_key` | 归一化实体键，格式 `类型:规范名` | 类型 ∈ person/company/org/product/other；模型同次调用内完成别名归并（Trump/川普/特朗普 → `person:trump`，字节跳动/ByteDance → `company:bytedance`） |
| `action` | 动词短语（发布/宣布/涨价/回应/下架…） | 阶段判定与反向动作表（发布↔召回/下架/否认）的输入 |
| `object` | 对象（产品/政策/资产/事项） | |
| `occurred_at` | 发生日期（可空） | 缺失时由关联文章最早 `pub_date` 代替 |

产出位置：`tagmanagement/service/core/extractor_section.go` 的**融合提取调用**内（`extractor_enhanced.go` 同步），**零新增 LLM 调用**（同一次语义理解的副产品，token 增量约 30-50）；`keyword`/`person` 标签不产出。存 `topic_tags.metadata.event_fingerprint`（存量标签不回填，只在新提取时写入）。

**一致性校验（`fingerprint_trusted`）**：用已有 `metadata.event_keywords`（覆盖 99.7%）做纯字符串比对——`subject` 与 `object` 均须能在关键词集中找到（允许包含关系与 `topic_tags.aliases` 展开 + lower/NFKC 归一）；不通过或关键词缺失 → `fingerprint_trusted=false`（存 `topic_tags.metadata`）并记 warning，**不阻断标签入库与聚类**，归属判定走降级路径（预筛后候选交 LLM 四分类）。实现：`topicgraph/service/story_fingerprint.go`。

**主体归一化三层**（`story_subject_normalize.go` 的 `ResolveSubjectKey`，前层命中即终止）：① `subject_key` 精确匹配（lower + 下划线归一）→ ② `subject_key` embedding cos ≥ 0.92（`subject_embedding` 存 `board_stories`，仅为已建立事件线算一次）→ ③ `subject` 字面归一（lower/NFKC/去空格/去「公司」「集团」后缀）精确匹配；全不命中登记新主体候选。**不用 `topic_tags.aliases` 归并**——实测 person/event 的 aliases 覆盖率 0%、keyword 仅 7.3%，现有标签体系没有实体层；embedding 阈值 0.92（含）远高于事件召回 0.5，因为误归并两个主体的代价高于漏归并。

### 候选生命周期窗口（全人工归档）

candidate 与 active 的归档**完全由用户在话题管理界面手动操作**。`planLifecycle` 仅更新命中计数：当天有 section 归属则 `consecutive_hits += 1`、`hit_count += 1`、`last_seen_date = 当天`；无归属则 `consecutive_hits` 归零。**不自动变更任何 status**；任何 status → archived 的转换只能由显式的用户操作触发。

`persistent_topic_candidate_decay_window`（默认 7 天）在此处仅用于 ClusterTags prompt 的卫生过滤——决定哪些 candidate 注入 LLM prompt，**不触发任何状态变更**。

### 候选展示门槛（observing → 可见 candidate）

`consecutive_hits < upgrade_threshold`（默认 3）的 candidate（"observing"）在话题管理 UI（`GET /api/semantic-boards/:id/topics`）中**隐藏**，但对用户不可见；它们仍持久化于数据库并参与可锚定话题集合（保证跨天命中能累积）。当 `consecutive_hits` 达到 `upgrade_threshold` 后，candidate 自动在管理 UI 可见（无需额外操作）。

### `auto_new` 创建门槛

当天 section 无法归属到已有 topic 时，系统自动创建 `status=candidate`、`consecutive_hits=1` 的 PersistentTopic。

### 一次性清理迁移

本 change 附带一次性迁移 `20260628_0001`：幂等删除 `status=candidate AND consecutive_hits < upgrade_threshold` 的历史 candidate，采用 `DeleteTopic` 语义——unlink 关联 section（置 NULL 含 `topic_status_at_report`），硬删 candidate，按 board 重建 relations。

### 可锚定话题选择器

ClusterTags 注入与双重确认归属共享同一个选择器（`ListAnchorableTopicsByBoard`）：全部 active 无条件入选，窗口内 candidate 按 `last_seen_date DESC, hit_count DESC, id ASC` 排序最多取 `persistent_topic_candidate_prompt_limit`（默认 20）条。窗口外 / 被截断的 candidate 两侧一致排除。

### 关系双轨

```text
similarity（匈牙利算法）
  → 时间线连线
  → emerging / continuing / split / merge / ending 状态

identity（同 persistent_topic）
  → 话题泳道连续性
  → 不参与时间线状态
```

| 轨 | 算法 | 作用 |
|----|------|------|
| similarity | 匈牙利算法 | 时间线连线、生命周期状态（emerging/continuing/split/merge/ending） |
| identity | 同 persistent_topic | 话题泳道连续性，不参与时间线状态 |

### 概念 bootstrap

```text
概念 bootstrap: 连通分量聚类 → 最小簇过滤 → LLM 命名概念
```

从话题相似度构建连通分量，过滤掉过小簇，再用 LLM 为每个概念簇命名。

### 话题运维操作（重命名 / 状态 / 合并 / 分裂 / 硬删）

话题管理 UI（`GET /api/semantic-boards/:id/topics`）不仅展示全量话题，还通过下列端点把 candidate→active→archived 的状态流转与归并/拆分交给用户显式操作（`backend-go/internal/topicgraph/handler/daily_report_handler.go`）：

| 端点 | 语义 |
| ---- | ---- |
| `PATCH /api/daily-reports/topics/:id` | `{label?, status?}`：重命名（label）与/或改状态（status 仅 `active`/`archived`，即转正/重激活/归档），两字段皆可省略以单独变更。**不止「转正」一种语义**。 |
| `DELETE /api/daily-reports/topics/:id` | 硬删话题：清空其 section 的归属（section 本身保留），不可逆；可逆路径是 PATCH status=archived。 |
| `POST /api/daily-reports/topics/:id/merge` | `{source_topic_ids:[]}`：把若干源话题的 section 全部划归 `:id`，源话题归档。 |
| `POST /api/daily-reports/topics/:id/split` | `{section_ids:[], label}`：把指定 section 从 `:id` 拆出，新建一个话题承载。 |

> 注：旧文档把 PATCH 仅描述为「active 转正」过窄——实际 PATCH 是 rename + status(active|archived) 的组合运维端点，且 merge/split/DELETE 同属话题级治理操作（均不自动触发，需用户显式调用）。

### 前端：演进报告与标签合并

话题图谱前端除 candidate→active 的生命周期操作外，还承载两个与「演进/合并」相关的组件（`front/app/features/tags/components/`）：

- **演进报告 `EvolutionReport.vue`**：由 `BoardEnrichmentPanel.vue` 在板块详情「数据增强」面板的「演进报告」子区块挂载。它消费 boardEnrichment 的 `result` 详情 + `reviews`（`api/boardEnrichment.ts` 的 `EvolutionAnalysis` 复合对象），以报刊式 editorial 呈现话题的演进定位（强化/转折/扩散）、跨泳道引用与证据清单。属「B 分析认知」数据富化循环的展示层（详见 `flow/data-enrichment.md`）。
- **标签合并对话框 `TopicGraphMergeDialog.vue`**：presentational 组件，提供「搜索目标标签 → 把当前标签的所有文章迁移到目标标签」的交互（emit `doMerge`）。注意它是**标签级**合并 UI（区别于上表的**话题级** merge 端点）；当前该组件未被任何视图引用（预留/orphan）。

### 前端：泳道编排态（新建泳道）

版块矩阵顶栏「新建泳道」进入全屏编排态（`front/app/features/tags/components/console/BoardComposeOverlay.vue` + `useInlineCompose.ts`），只在 ≥768px 可用；窄屏仅提示「新建泳道需要宽屏或横屏」。

- **画布**：14 个连续日期列 × 现有 active 泳道行 + 待确认/未分类行；已归属 section 圆点为「已归属（只读）」，只能聚焦看关系，不能勾选或保存进新泳道（MUST NOT 把已有泳道 section 移出）。
- **候选**：侧栏 `ComposeSidebar` 按 candidate 话题与语义搜索列出候选；「采纳」只预填名称 + 勾选待确认 section，预勾上限 20，并按日期升序取前 20。
- **关系**：点画布圆点显示 top-6 相似 section 连线与侧栏距离（3 位小数）；无 relation 时明确「暂时没有关联 section，画布不画线」。
- **质量指标**：成员数、平均距离（无已知关系显示「—」）、离群数随勾选即时刷新；保存前必须同时有名称与成员，重复点保存只创建一次。
- **退出**：取消 / Esc 关闭且不落库；保存成功退出并回矩阵选中新泳道。组合建议/候选采纳仍走既有 compose 建议确认事务，不在本画布直接建话题。

## 业务约束与不变量

> 本节是 constraint-injection extension 的注入数据源：apply 改 `internal/topicgraph/` 或 `internal/tagmanagement/` 代码前会自动注入 system prompt，必须遵守。

1. **planLifecycle 只更新命中计数、不得自动变更任何 status，归档只能由用户显式操作触发**：`planLifecycle`（`repository/daily_report_assignment.go`）只更新命中计数——当天有 section 归属则 `consecutive_hits += 1`、`hit_count += 1`、`last_seen_date = 当天`；无归属则 `consecutive_hits` 归零。**不自动变更任何 status**；任何 status → archived 的转换只能由用户在话题管理界面的显式操作触发。
2. **candidate 连续命中达 `upgrade_threshold`（默认 3）仅获人工激活资格、需手动转 active，未达标 UI 隐藏但持续累积**：`consecutive_hits` 达阈值的 candidate 仅**获得人工激活资格**（`CanActivate = Status==candidate && HitCount >= UpgradeThreshold`），仍保持 candidate，需用户 PATCH active 才转 active。同时该阈值是**话题管理 UI 的可见门槛**——未达阈值的 "observing" candidate 在 `GET /api/semantic-boards/:id/topics` 中隐藏（仍持久化、参与可锚定集合，保证跨天累积），达阈后自动可见。
3. **当天 section 无法归属已有话题时，auto_new 自动创建 candidate 新话题并种命中计数 1**：当天 section 无法归属到已有 topic 时，系统自动创建 `status=candidate`、`consecutive_hits=1`、`hit_count=1` 的新 PersistentTopic（首义向量 = 该 section 标题向量）。
4. **`persistent_topic_candidate_decay_window`（默认 7 天）仅过滤 prompt 注入候选，不得触发任何状态变更或归档**：决定哪些 candidate 注入 ClusterTags prompt，**不触发任何状态变更 / 归档**。
5. **ClusterTags 注入与双重确认归属必须共享同一选择器 ListAnchorableTopicsByBoard，消除单边锚定**：ClusterTags 注入（Step3）与双重确认归属共享同一个选择器 `ListAnchorableTopicsByBoard`——全部 active 无条件入选；窗口内 candidate 按 `last_seen_date DESC, hit_count DESC, id ASC` 排序最多取 `persistent_topic_candidate_prompt_limit`（默认 20）条。窗口外 / 被截断的 candidate 两侧一致排除，消除单边锚定的隐式 bug。
6. **section 话题快照字段在 SaveReport 同一事务写入，不得随后续话题状态变化回填**：每条 section 的 `topic_status_at_report`、`persistent_topic_id`、`topic_match_distance`、`topic_match_confidence` 在 `SaveReport` 同一事务内写入（快照值 = 当时 PersistentTopic 的 candidate|active；未归属写 NULL）。历史快照不随后续 topic 状态变化回填。
7. **用户手动建话题直接落 active，跳过 candidate 阶段与连续命中门禁，归属置信标记 manual**（`source=manual`，详见 `flow/daily-report.md` §2）：用户主动建 active topic 跳过 candidate 阶段与连续命中门禁，`topic_match_confidence='manual'`（人工归属第四态，非算法三态）；下一期日报起与自动 active topic 一样参与 lane 锚定。**sentence_topic 关注的专属话题同属 manual 主权声明家族**（watch-materialized-topic）：首次物化时由系统代建（`Embedding=Centroid=检索句向量`，命中计数种 0、当期 lifecycle 推到 1），与手动建话题一视同仁参与锚定；删除该关注需显式确认并联动归档（`confirm_archive_topic=true`，走 UpdateTopic status=archived 软归档，历史 section 快照不变）。
8. **同一 link 的跨 feed 文章副本必须复用已有打标结果（`source='reuse'`、score 沿用、不超单篇标签上限），只有无可复用标签时才允许调 AI**：`tagArticle`（非 Force）在本副本已打标检查**之前**查同 link 其它副本的 `article_topic_tags`（`article_topic_tags JOIN articles ON link`）：命中则按 score 降序把本副本缺失的标签补齐（已存在的 `(article_id, topic_tag_id)` 不重复写，`idx_article_topic_tags_link` 兑底）、`source='reuse'`、score 沿用、**总数不超过 `maxArticleTags`（6）**，补入即返回且**不调 AI**；本副本已满上限则落到已有的“已打标跳过”分支（也不调 AI）；无同 link 副本标签才走原 AI 提取路径。`RetagArticle`（Force，用户手动重打）**明确不复用**、不受补空间限制，仍走完整 AI 提取 + `limitArticleTags`。副本标签因此向同 link 的完整集合收敛（部分标签副本会被补齐），该路径写完后同步重算 `articles.tag_count`。
9. **打标队列消费必须新任务优先（night-window-alignment）**：`tag_jobs` lease 顺序 = `priority DESC, created_at DESC`（priority 可插队；退避由 `available_at <= now` WHERE 保证，**不把 available_at 作排序键**——Enqueue 时 available_at=created_at，升序排序会复活 FIFO）。陈年 pending 不另做业务侧 TTL 淘汰（原方案于 2026-09-24 验收期用户决策取消）：旧任务几乎不被 lease 到，不挤占分析窗口，随文章 7 天归档 GC 自然失去下游价值。
10. **话题关键事件注记是人工主权层，不参与话题生命周期状态机，话题生命线回看只透出 confirmed 注记（add-topic-key-event-markers）**：`topic_key_event_markers` 只挂 confirmed/dismissed/suggested 三态注记（AI 只写 suggested），不推进 candidate/active/archived 任何状态（与 `planLifecycle` 互不相干）；`GET /daily-reports/topics/:id/lifeline` 与话题 key-events 读端只透 confirmed——建议永不进总览/生命线回看；注记与话题/section 的关系由来源指纹维护（重建重挂/失效快照见 `flow/daily-report.md` 约束 30/31），注记的存在不得反写 section 归属或关系计算。
12. **event 标签结构化指纹必须与标签同一次 LLM 调用产出，零新增调用；指纹存 `topic_tags.metadata.event_fingerprint`，`fingerprint_trusted` 一致性校验失败只降级不阻断标签入库**（add-event-story-heat）：每个 `category='event'` 标签同次调用产出 `subject`/`subject_key`/`action`/`object`（`occurred_at` 可空，缺失时由关联文章最早 pub_date 代替）；`keyword`/`person` 标签不产出。校验用已有 `metadata.event_keywords`（覆盖 99.7%）+ `topic_tags.aliases` 展开（纯字符串比对）：`subject` 与 `object` 均须能在关键词集中找到（允许包含与别名展开）；不通过或关键词缺失 → `metadata.fingerprint_trusted=false` 并记 warning，标签照常入库照常聚类，归属判定走降级路径（见 `flow/daily-report.md` §事件线归属）。指纹解析/校验唯一实现 `topicgraph/service/story_fingerprint.go`，提取侧 schema 在 `tagmanagement/service/core/extractor_section.go`（同步 `extractor_enhanced.go`）。
13. **`subject_key` 归一化不依赖 `topic_tags.aliases`（实测 person/event aliases 覆盖 0%、keyword 仅 7.3%），主判据是模型同次调用产出的 `类型:规范名` 键（person/company/org/product/other）；主体归一化三层顺序固定：① subject_key 精确匹配 → ② subject_key embedding cos ≥ 0.92 → ③ subject 字面归一（lower/NFKC/去空格/去「公司」「集团」后缀），前层命中即终止，全不命中登记新主体**（add-event-story-heat）：`subject_key` 由模型在同一次调用内完成别名归并（Trump/川普/特朗普 → `person:trump`）；embedding 只兑同义拼写差异（阈值 0.92 远高于事件召回 0.5，含 0.92/不含 0.919），MUST NOT 用于事件判定；`subject_embedding` 存 `board_stories`（仅为已建立事件线算一次，不逐篇算）；独立实体身份层见红线 15（截至 2026-11-10 默认 shadow 运行，尚不参与事件线判定）。实现 `topicgraph/service/story_subject_normalize.go`（`ResolveSubjectKey`）。
14. **事件归属判定顺序固定且 subject 一票否决不可推翻：subject_key 预筛先于一切层（含指纹不可信路径）；向量相似度只用于召回候选（dist ≤0.5 top10），MUST NOT 单独作为合并判定依据**（add-event-story-heat）：五层判定与综合稿/人工保护/合并链的完整契约见 `flow/daily-report.md` §事件线归属与跨期去重（约束 42-45）；本域只强调：话题（persistent_topic）不是事件身份——同日一话题可拆 1-6 桶，事件线身份（board_stories）与话题归属正交，lane_id 仅记录首次归属用于展示分组。

15. **全局实体身份层已存在但本期不参与事件线身份判定：事件线判定路径（`ResolveSubjectKey` / `subjectPreFilter`）不读取实体 ID，`subject_key` 三层与一票否决、人工归属（story_manual）、组合标签（不产实体提及/事件线归属）、热度口径全部逐字不变**（normalize-event-subject-entities）：实体主体归一是**与事件线身份并行的另一套身份**——`entity_registry`/`entity_mentions`/`entity_aux_links` 三表承载全局稳定实体 ID，以影子模式（`entity_subject.mode=shadow`，默认 `off`）落地（影子态下：写实体层三表 + 审计 + metadata.entity_subject 惰性键，标签/事件线/日报/话题/界面字段零变化）：第一轮同次调用额外产出稳定短身份说明（`subject_desc`，落 `topic_tags.metadata.entity_subject` 独立惰性键、**不进 `event_fingerprint`**）；候选=名向量 ∪ 名+短说明向量 ∪ 已登记叫法命中，按 entity_id 去重；二轮纯选择题（capability=`topic_tagging`、operation=`tagmanagement.entity_subject.resolve`）只输出合法实体 ID 或 none，**相关≠同一实体**（仅相关/隶属/上下游/产品-厂商/机构-成员判不同）；非法 ID/不可解析/服务失败/超时=技术失败（不降级 none、不新建、最多 2 次尝试、耗尽后未归一、不阻断标签入库）；合法 none 在单短事务内原子登记（身份行+证据行+aux 溯源行）；同来源同提及重放幂等（幂等键=（来源标签,提及键）非提及文本）；跨进程单一全局会话级咨询锁（独占独立连接）串行化「闸内重读最新池→二判→原子登记」，闸外旧 none 作废、等待/总时长/模型调用均有界、finally 释放归还连接。默认 `off`（2026-11-10 决策：影子实验后不接线，避免实体注册表成为只增不减的黑洞；`shadow` 仅为实验态，需显式配置开启）；显式 `off` 则第一轮提示与 schema 与调用数逐字不变、零二轮调用、零写入、不写 entity_subject 键；`active` 本期显式报错拒绝（不静默按 shadow 执行）。实体身份层可整层删除而不影响任何既有功能；「实体 ID 接入事件线判定」的接线规则属后续 change，本期不写为现行规则。

16. **事件线建线必须留下召回向量，缺向量只记 warning 不阻断建线；存量无向量线由显式回填补齐**（enable-story-cross-day-continuation D13）：`createStoryForThread` 在 `thread.Embedding` 为空且事件 `title` 非空时，用标题现算一次 embedding 写入 `board_stories.embedding`（operation `topicgraph.story_title_embedding`、`CapabilityEmbedding`、30s 超时）；**现算失败只记 warning、不阻断建线**（事件线仍以 `embedding = NULL` 建，与上线前行为一致）。**召回口径不变**：仍只算 `embedding <=> ?`、`dist ≤ 0.5` top10、`WHERE board_stories.embedding IS NOT NULL`（不把 `subject_embedding` 并入召回、不引入新召回面）；存量补齐只走显式 CLI `cmd/backfill-stories --fill-story-embeddings`——**只补 `embedding IS NULL` 的行、只写 `embedding` 一列**（不碰 `first_seen_date`/`last_seen_date`/`thread_count`/`participant_count` 等任何其它列）、`--dry-run` 只统计条数且不发模型调用、逐条失败不中断并汇总失败数、可重复执行（补过的行不再被处理）。**线程侧向量缺失同样由 `--fill-thread-embeddings` 补齐**（`daily_report_threads.embedding`，只写 `embedding` 一列、`--dry-run` 只统计；线程向量是召回的**实际输入**——`recallCandidates` 拿 `thread.Embedding` 查 `board_stories.embedding <=> ?`，缺向量只能走文章重叠兜底）；watch 物化线程（`lane_tier` 前缀 `watch_`）按设计无向量（其事件线归属走「只比文章集合」的降级路径），默认排除，需显式 `--include-watch-threads` 才会被改动。

## 代码入口

- **后端生命周期与归属**：`backend-go/internal/topicgraph/repository/daily_report_assignment.go`（`planLifecycle` 命中计数、`planTopicAssignments` 双重确认锚定）、`repository/daily_report_topic_repository.go`（status 状态机 `candidate|active|archived`、可锚定选择器 `ListAnchorableTopicsByBoard`）、`repository/daily_report_manual_topic.go`（手动建泳道）。
- **后端事件指纹与主体归一化（add-event-story-heat）**：`backend-go/internal/topicgraph/service/story_fingerprint.go`（`ParseEventFingerprint` 指纹解析 + `ValidateEventFingerprint` 一致性校验，产出 `fingerprint_trusted`）、`story_subject_normalize.go`（`ResolveSubjectKey` 三层归一化）；提取侧 schema `backend-go/internal/tagmanagement/service/core/extractor_section.go`（+`extractor_enhanced.go` 同步）；事件线归属/去重/热度的完整入口见 `flow/daily-report.md` §事件线归属与跨期去重。
- **后端关系双轨 / 聚类**：`backend-go/internal/topicgraph/service/daily_report_cluster.go`（ClusterTags 聚类 + 概念 bootstrap）、`service/daily_report_orchestrator.go`（`GenerateDailyReport` 管线、`RebuildBoardRelations` 关系双轨）、`service/daily_report_merge.go`。
- **后端 handler**：`backend-go/internal/topicgraph/handler/topic_watch_handler.go`（话题 watch）、`handler/daily_report_handler.go`（话题管理列表 + PATCH rename/status、DELETE 硬删、POST merge/split、手动建泳道、backfill-embeddings/relations/topics）。
- **标签提取 / 去重 / 层级**（话题聚类的输入源）：`backend-go/internal/tagmanagement/`（`service/` 抽象标签层级与合并、`handler/` 标签管理、`repository/` 持久化）；文章打标执行点 `service/core/article_tagger.go`（`tagArticle` / `RetagArticle` / `reuseTagsFromSiblingArticle` 跨 feed 复用）。
- **关键事件注记（回看侧）**：`backend-go/internal/topicgraph/repository/key_event_markers.go`（注记模型与人工动作，状态机与指纹详见 `flow/daily-report.md` §关键事件注记）、`handler/key_event_handler.go`（话题读端 confirmed + 注记维护端点）、`GetTopicLifeline`（生命线 `key_events` 字段装配）；前端 `BoardThreadBrowser.vue`（lanes ★n / 焦点合并轴 + 失效快照同轴注记 + 突出开关只弱化透明度不改坐标背景）、`useTopicKeyEvents.ts`（共享缓存单例）。
- **前端**：`front/app/features/tags/components/daily-report/`（SectionLifecyclePanel、话题锚定展示）、`front/app/features/tags/components/BoardThreadBrowser.vue`（话题总览泳道）、`front/tests/e2e/topic-graph.spec.ts`。

## 变更溯源

| 日期 | 变更 | 摘要 | 归档位置 |
|------|------|------|----------|
| 2026-11-11 | normalize-event-subject-entities | 全局实体身份层影子落地（`entity_registry`/`entity_mentions`/`entity_aux_links` 三表 + 三路候选检索→二判→原子登记/新建全链 + 会话级咨询锁闸门 + 有界评测 CLI；默认 `off`/`shadow`，`active` 显式拒绝）：事件线身份判定不读实体 ID，`subject_key` 三层与一票否决逐字不变，存量不迁移不回填 | [`openspec/changes/archive/2026-11-11-normalize-event-subject-entities`](../../../openspec/changes/archive/2026-11-11-normalize-event-subject-entities) |
| 2026-11-11 | lane-compose-story-chain | **设计废弃（未实施）**：泳道编排新增事件线链（跨天同事件选材维度）经讨论与数据验证后决定放弃，0/24 任务归档；delta spec 未同步，本 flow 主 spec 与行为不变 | [`openspec/changes/archive/2026-11-11-lane-compose-story-chain`](../../../openspec/changes/archive/2026-11-11-lane-compose-story-chain) |
| 2026-11-11 | enable-story-cross-day-continuation | 新增 `related-events`（读侧推荐）：event 标签语义相似度相关推荐，明确**不进**事件线判定/聚合；本 flow 的身份判定顺序、指纹、`subject_key` 三层、热度口径均逐字不变 | [`openspec/changes/archive/2026-11-11-enable-story-cross-day-continuation`](../../../openspec/changes/archive/2026-11-11-enable-story-cross-day-continuation) |
| 2026-11-06 | enable-unbound-jit-injection | 约束标签辖区补全：补后端漏网目录与前端业务区信号，修 5 条孤儿标签；未激活档编辑路径 JIT 注入见 change（本 flow 无业务行为变更） | [`openspec/changes/archive/2026-11-06-enable-unbound-jit-injection`](../../../openspec/changes/archive/2026-11-06-enable-unbound-jit-injection) |
| 2026-11-03 | fix-embedding-model-column-width | `topic_tag_embeddings.model` 由 `VARCHAR(50)` 加宽到 `VARCHAR(100)`（与 `ai_models.name` 对齐）：本地 llama.cpp / GGUF embedding 模型以文件绝对路径注册（61 字符）超限 → `SQLSTATE 22001` → 标签向量落库被拒、embedding 队列批量失败（689 条）；建立「存 embedding 模型名列宽 MUST NOT 窄于模型注册表」不变式（数据驱动测试）；连带恢复被 GORM AutoMigrate 顺带 `DROP NOT NULL` 的 `model` 列约束（迁移 `20261103_0002`） | [`openspec/changes/archive/2026-11-03-fix-embedding-model-column-width`](../../../openspec/changes/archive/2026-11-03-fix-embedding-model-column-width) |
| 2026-10-24 | add-intra-lane-theme-bucketing | 同一话题同一天可有多条 section（泳道内主题分桶/手动编排/watch 物化）：`planLifecycle` 命中按日去重、质心按 section 逐条均权计入（约束 11） | [`openspec/changes/archive/2026-10-28-add-intra-lane-theme-bucketing`](../../../openspec/changes/archive/2026-10-28-add-intra-lane-theme-bucketing) |
| 2026-10-12 | add-topic-key-event-markers | 话题关键事件注记（人工主权层）：生命线/总览回看只透 confirmed（约束 10）、lanes ★n 徽标与焦点视图合并轴/失效快照注记；注记不参与生命周期状态机 | [`openspec/changes/add-topic-key-event-markers`](../../../openspec/changes/add-topic-key-event-markers) |
| 2026-07-05 | manual-topic-lane | 手动建泳道：用户主动建 active topic（`source=manual`），次期接入 AND-gate；新增 `board_persistent_topics.source` 列 + `topic_match_confidence=manual` 第四态 | [`openspec/changes/archive/2026-07-05-manual-topic-lane`](../../../openspec/changes/archive/2026-07-05-manual-topic-lane) |
| 2026-05-31 | section-lifecycle-ui | Section 获得独立生命周期（status + prev_section_id），BoardThreadBrowser 从 thread 粒度改为 section 粒度话题总览 | [`openspec/changes/archive/2026-05-31-section-lifecycle-ui`](../../../openspec/changes/archive/2026-05-31-section-lifecycle-ui) |
| 2026-08-01 | inline-compose-lane | 就地编排新建泳道：composeMode 叠加态（不切 viewMode）+ unassigned 主战场勾选 + 贴合度实时分层 + active 淡显可勾走移出 + 聚类质量单卡 + 候选侧边栏（语义搜索/相似推荐/已中断折叠）+ 废弃 ComposePanel；CandidateTopicBrief 补 status 对齐 lanes 移出口径 | [`openspec/changes/archive/2026-08-01-inline-compose-lane`](../../../openspec/changes/archive/2026-08-01-inline-compose-lane) |
| 2026-08-22 | analysis-remediation | 分析报告整改四线：W1 存储清理（topic_tag_embeddings 孤儿 3.07GB 清除 + FK CASCADE 防复发、semantic_labels disabled 行向量置 NULL）；W2 pi-sessions 并行编排；W3 openspec 指令副本收敛；W4 规范可观测（spec-gate 归档门禁强制化） | [`openspec/changes/archive/2026-08-22-analysis-remediation`](../../../openspec/changes/archive/2026-08-22-analysis-remediation) |
| 2026-08-25 | watch-materialized-topic | watch 物化轨与持久话题联动：sentence_topic 专属 manual active 话题（代建/锚定/删除确认归档）；keyword_topic 物化板块不建话题；物化边界约束见 `flow/daily-report.md` §16 | [`openspec/changes/archive/2026-08-25-watch-materialized-topic`](../../../openspec/changes/archive/2026-08-25-watch-materialized-topic) |
| 2026-09-04 | constraint-declaration-redline | 约束节红线句格式化：本域「业务约束与不变量」节每条约束改写为首行加粗自含红线句 + 细节跟后（语义不变），declaration 注入降为红线层（上线后实测 bytes 降约 60%），细节层经关键词/JIT 全节注入按需补全；本域为格式改写，无业务行为变更 | [`openspec/changes/archive/2026-09-04-constraint-declaration-redline`](../../../openspec/changes/archive/2026-09-04-constraint-declaration-redline) |
| 2026-09-17 | dedupe-rss-articles | 跨 feed 同 link 副本打标复用：`tagArticle` 在已打标检查前查同 link 兄弟副本的 `article_topic_tags`，命中则按 score 降序补齐缺失标签（`source='reuse'`、score 沿用、不超 `maxArticleTags`）且不调 AI；Force 重打不复用；同 feed 判重与快讯 upsert 见 `flow/content-enrichment.md` | [`archive/2026-09-17-dedupe-rss-articles`](../../../openspec/changes/archive/2026-09-17-dedupe-rss-articles) |
| 2026-09-22 | lane-trend-overview | 泳道态势结算产物扩展为长短两版（同窗同素材单次 LLM 出 JSON，解析降级不重试不阻塞日报；存量长版不回填）：`topic_lane_snapshots` 加 `rolling_detail` 长版列、lane-dynamics 聚合 snapshot 增 `detail` 可空字段；日报阅读视图泳道展开体顶部新增「泳道趋势」区（14 天长版/月/年三档），结算口径详见 `flow/daily-report.md` §泳道态势结算 | [`openspec/changes/archive/2026-09-22-lane-trend-overview`](../../../openspec/changes/archive/2026-09-22-lane-trend-overview) |
| 2026-09-23 | night-window-alignment | 打标队列消费顺序改新任务优先（priority DESC, created_at DESC，退避由 WHERE 保证）；原方案含过期任务 TTL 淘汰（terminal skipped），验收期用户决策取消（既有 7 天归档 GC 已兜底，未合入）； | [`openspec/changes/archive/2026-09-24-night-window-alignment`](../../../openspec/changes/archive/2026-09-24-night-window-alignment) |
| 2026-10-15 | add-topic-key-event-markers | 话题生命线可选节点扩展：`ListTopicKeyEventMarkers` 按 topic/窗口返回 confirmed 与快照失效注记（`daily_report_topics` 反查归属），总览泳道长轴高亮 confirmed 背景、取消纠错不删背景、来源日报导航；不改变话题归属/生命周期/identity-similarity 关系语义 | [archive/2026-10-15-add-topic-key-event-markers](../../../openspec/changes/archive/2026-10-15-add-topic-key-event-markers) |
| 2026-11-05 | redesign-enrichment-research-workbench | 话题新闻背景管理入口落到话题总览：BoardThreadBrowser focus 头部新增「新闻背景」按钮（AppDialog lg 承载 TopicNewsBackground，话题+粒度+周期独立读写）；不改话题筛选/关键事件/生命周期行为，contexts API 可选 semantic_board_id 归属校验（不传零差异） | [openspec/changes/archive/2026-11-05-redesign-enrichment-research-workbench](../../../openspec/changes/archive/2026-11-05-redesign-enrichment-research-workbench) |
| 2026-10-13 | add-article-search | 站内搜索只读消费标签向量与边：语义召回路径对 `topic_tag_embeddings`（identity 型）做 cosine 近邻、经 `article_topic_tags` 召回文章取 max cosine，零写入、不触发打标/向量任务、不影响话题生命周期；搜索行为契约见 `flow/reading.md` §站内搜索与约束 #10-#12 | [`openspec/changes/add-article-search`](../../../openspec/changes/add-article-search) |
11. **同一话题同一天可以有多条 section（泳道内主题分桶/手动编排/watch 物化）：命中计数按「日」去重、质心按 section 逐条计入（add-intra-lane-theme-bucketing）**：一篇日报内同一 topic 拥有多条 section（兄弟桶共享 `persistent_topic_id`）时，`planLifecycle` 的 `hit_count` / `consecutive_hits` SHALL 只按该篇日报是否命中 +1（实现上 `hitTopicIDs` 是集合，与 section 条数无关），SHALL NOT 按条数累加；`ComputeTopicCentroid` SHALL 按 `persistent_topic_id` 的最近 `centroid_window` 条 section 逐条均权计入，SHALL NOT 对同日同话题的多条 section 做特殊去重（拆得越细质心越纯，是正向副作用）。`topic_status_at_report` 仍是报告期快照，不随后续状态回填。
| 2026-11-02 | rework-workshop-into-radar-and-desk | 工坊重构为雷达+版块态势台：`/tags` 首屏改今日雷达（单次聚合端点 overview），五 tab 退役（矩阵/归档视图/编排/阅读台承接，8.4 物理清理旧宿主），新建泳道编排工作面（服务端候选链口径+质心推荐+画布缩放平移），阅读台路由化 `/tags/:boardId/:date` + 旧版三栏恢复 + 页边注/收藏/Esc 语义回归 | [archive/2026-11-02-rework-workshop-into-radar-and-desk](../../../openspec/changes/archive/2026-11-02-rework-workshop-into-radar-and-desk) |
| 2026-11-06 | add-event-story-heat | 事件线身份与热度证据分离：新增 `board_stories`（跨天事件线，带 `subject/subject_key/action/object/occurred_at` 指纹与 `participant_count`）与 `story_signals`（每篇证据按 `pub_date` 优先计时）；日报线程加 `story_id/scope/is_development/story_manual/display_tier` 五列。热度改为从 signals 派生（story 48h/24h、lane 14d/72h 独立衰减，`heat-v1-48h-hl24h`），矩阵格值=事件线数、颜色=最热事件线来源数（4 档）、行序=lane_heat→story_count→id、雷达关注度=Σ lane_heat（P80/P20 标定 3.3/0.7）。合并闸门最终收紧为「显式 subject_key 优先、对象严格相等、指纹不可信一律不合并、LLM 不参与合并」；新增 `cmd/backfill-stories`（含 `--rebuild-signals`）与 `cmd/backfill-fingerprints` 两个运维入口。 | [archive/2026-11-06-add-event-story-heat](../../../openspec/changes/archive/2026-11-06-add-event-story-heat) |
