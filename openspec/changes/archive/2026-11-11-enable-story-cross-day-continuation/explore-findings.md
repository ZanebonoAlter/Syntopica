
# 历史探索记录（2026-11-10 重做后不再作为新方案要求）

> 下文记录旧身份归并实验，含已撤回方向与当时统计。当前唯一有效目标为 proposal/design/specs/related-events：event 标签 embedding 的直接相关，不合并、不依赖 subject_key、不做阶段发酵。纠错与副作用见 legacy-experiments.md；不能根据下文重新接实体层或重算 thread。

## 跨天事件线断裂三根因（DeepSeek 样本实证）

用户问「DeepSeek 开源 DeepInfer 这种跨天大事件为什么没串成一条线」。真库取 DeepSeek 样本（board 1980）验证，「DeepSeek Harness 推出 Creator Mode」散落成 4333(10-31)/4353(11-01)/4378(11-02)/4391(11-03) 四条单日线，指纹几乎全同（product:deepseekharness | 推出 | Creator Mode 功能）。逐关核对判定链，死因共三层：

**A（本轮新证实）：事件线缺 title 向量 → 永久退出召回池**
- `RecallStoryCandidates` 的 SQL 硬条件 `board_stories.embedding IS NOT NULL`（story_repository.go:199），且召回距离只算 `embedding <=> ?`，不用 subject_embedding。
- `createStoryForThread`（story_attribute.go:497）`Embedding: thread.Embedding`——建线线索无向量 → 事件线永久无向量 → 之后所有同指纹跨天事件**永远召回不到它**，只能新建。
- 样本实证：4333/4353/4378 的 `embedding IS NULL`（4391 才有）；全库 910 条活跃线 70 条无向量（7.7%）。
- 修复方向（Change C 需补任务）：为缺向量事件线批量回填 title embedding（title 可由挂载线索 title 派生）；或召回路径把 subject_embedding 距离并入候选（subject 一票否决仍在上层，不破红线 14）。

**B（已证实）：数值型 object → fingerprint_trusted=false → R-15 闸门拒绝合并**
- `DecideAssignment`（story_assign.go:137）：`!input.FingerprintTrusted` → 一律新建（assignReasonMergeGateUntrusted）。
- 根因在 `ValidateEventFingerprint` 要求 object 在标签关键词/aliases 中命中；数值型 object（汇率、人数）永远命不中。

**C（已证实）：跨天 stage 日期基准 = 建线原点日**
- `stageDatesWithinOneDay` 以候选线建线原点日为基准，长链第 2 天即 dateVeto；Change C 放宽 A（改用最近证据日）已覆盖。

**附加发现：subject_key 提取不稳定（同一产品 4 种 key）**
- DeepSeek 家族真库 key：`product:deepseekharness` / `product:deepseek_harness`（下划线差异）/ `person:deepseek_harness_团队` / `other:deepseek_harness_插件机制详解`（类型前缀漂移）。
- 类型前缀不同 → `NormalizeSubjectKey` 无法归一（other/product/person 互斥）→ subject 一票否决直接挡掉本可同线的候选。这不是词表问题，是提取 prompt 对「实体类型 + 规范名」的稳定性问题，Change C 的 D4（object_type 结构化）不覆盖 key 稳定性，需单独评估（可能要在提取 schema 里加 subject_type 枚举 + 归一化规则，或 subject_embedding cos≥0.92 兜底扩大到跨前缀场景）。

**判定链对照（story_assign.go:249 classifyStoryCandidate）**：key 相同 → object matchEntityText ✓ → sameObjectStrict ✓ → action 匹配 → verdictOccurrence（挂载，无日期检查）——即**指纹全同的跨天挂载本有通路**，4353 若能召回 4333 本可挂上；死在召回盲区（A）与信任闸门（B），不是判定规则本身。

<!-- pinned 2026-11-09T03:49:23Z -->

## 实体层落地后本 change 的三处适配缺口（2026-11-10 复核）

复核结论：change 主体（两层判定/object_type/信任判据/矩阵日格/回填 CLI）与实体层正交，照原样可实现；但有 3 处必须按现状调整。

【硬缺口 A：召回盲区未进 tasks】explore-findings 写了「Change C 需补任务」，tasks.md 33 条里没有。现状代码 `story_repository.go:199` 召回硬条件 `board_stories.embedding IS NOT NULL`，建线 `story_attribute.go:509` 用 `Embedding: thread.Embedding`（空则 NULL）。真库 2026-11-10：活跃线 1262 条中 111 条 `embedding IS NULL` → 永久召回不到。补法：回填 title embedding + 建线兜底（或召回并入 subject_embedding 距离，subject 一票否决仍在上层）。

【口径过时 B：「本期不建独立实体表」已作废】实体层已落地并默认 shadow（`entity_registry`/`entity_mentions`/`entity_aux_links`），proposal/design 里多处「本期不建独立实体表」需要改口径，否则实现阶段会误判环境。

【边界 C：subject_key 漂移有解但本期不许接】explore-findings 记录 DeepSeek 家族 4 种 key（product:deepseekharness / product:deepseek_harness / person:deepseek_harness_团队 / other:...）→ 类型前缀不同 → subject 一票否决挡掉本可同线候选。实体层已能跨叫法归一（实测：泸州市中级人民法院→四川省泸州市中级人民法院；PAW3955→PAW Next I），但红线 15 / D9 禁止事件线判定路径读实体 ID，且有源码护栏测试。本期跨天对跨叫法/跨前缀仍会漏，接线须单开 change 并先改红线 15。

【数据未跑偏】proposal 写 8,209 指纹 / 790 数字开头 object；2026-11-10 实测 7,910 / 765，量级一致，不需重写分析。活跃线 910 → 1262、无向量 70 → 111，验收基线 §4 需重测。

【关键机制】`last_seen_date` 由 `RecomputeBoardStoryCounts` 按已挂 thread 的 period_date 重算（`story_repository.go:511`），所以「跨天线 0 条」等价于「跨天挂载 0 次」；explore-findings 的判定链对照（指纹全同跨天挂载本有通路、死于召回与信任闸门）成立。

**引用**：backend-go/internal/topicgraph/repository/story_repository.go:RecallStoryCandidates、backend-go/internal/topicgraph/repository/story_repository.go:recomputeBoardStoryCountsWith、backend-go/internal/topicgraph/service/story_attribute.go:createStoryForThread、backend-go/internal/tagmanagement/service/core/article_tagger.go:293、openspec/changes/enable-story-cross-day-continuation/explore-findings.md

<!-- pinned 2026-11-10T05:29:39Z -->

## E1 实体回填工具与 E3 测算口径（可直接复用）

【E0 盘点（只读，2026-11-10）】近 7 天 event 标签 4,485 条；规范化主体名 3,026；同一规范名对应 >1 个 subject_key 的组 138（跨天出现 95）；排除已回填后定向清单 = 135 组 / 278 个 key / 989 条标签。漂移样例：product:影之刃零 vs product:《影之刃零》；person:特朗普 vs person:美国总统特朗普；org:中国证券投资基金业协会 vs org:中基协；org/person/other/company/organization:民主党（5 种前缀）。

【E1 工具（已实现并试点）】`backend-go/cmd/entity-backfill/main.go`（写在 tasks 9.x 下）：
- 选择面：category='event' AND metadata ? 'event_fingerprint' AND NOT (metadata ? 'entity_subject')；可选 --subject-keys 文件、--since/--until、--limit。
- 复用实体层：`entity.DefaultResolver().ResolveMention(MentionRequest{MentionText=subject, SubjectDesc, ContextSnippet=label+文章标题/摘要, SourceTagID, SourceArticleID, SessionID})`；`core.PersistEntitySubject` 写惰性键。
- 写入面严格两个：实体三表 + `topic_tags.metadata.entity_subject`；不碰 story/report/标签业务字段/指纹。
- `--dry-run` 不调模型不写库；`--with-desc` 按规范化主体去重缓存，一主体一次 desc 调用（operation=tagmanagement.entity_subject.desc）。
- 试点：20 条 19.1s，created=14 reused=6 unresolved=0 desc_generated=18 desc_cached=2。
- 全量运行中：`--subject-keys /tmp/drift-keys.txt --with-desc`，日志 /tmp/entity-backfill-e1.log，dispatch 时进度 229/989（227 成功 / 2 unresolved），entity_registry=91、entity_mentions=232。

【E3 测算口径（只读）】story → `daily_report_threads.tag_ids`（JSON 数组）→ `topic_tags` → `entity_mentions.source_tag_id`，映射时要求 tag 的 `metadata.event_fingerprint.subject_key = board_stories.subject_key`（精确锚定，避免多主体串味）。统计同 board 同 entity_id、不同 `first_seen_date` 的跨天故事对；分「同 subject_key」（现有判据本可合并，可能死于召回/信任闸门）与「不同 subject_key」（实体接线唯一解锁）两档；再取同 action+同 object 的严格子集。

**引用**：backend-go/cmd/entity-backfill/main.go、backend-go/internal/tagmanagement/service/entity/resolver.go:ResolveMention、backend-go/internal/tagmanagement/service/core/tagger.go:PersistEntitySubject、backend-go/internal/topicgraph/repository/story_repository.go:RecallStoryCandidates

<!-- pinned 2026-11-10T06:01:07Z -->

## E1–E3 实测结论：收益、误合并风险与 D14 加固

【E1 结果（2026-11-10，已抽验）】定向回填 1,384 条标签（278 个 key / 135 组漂移），耗时 28m53s：created=233、reused=1151、unresolved=0、failed=0、desc_generated=387、desc_cached=997。实体层 268 registry / 1,431 mentions / 11 aux；带 entity_subject 的 event 标签 1,428（18%）。跨叫法归一确认成立：DeepSeek Harness 4 键→entity 31；特朗普政府 5 键→entity 127；民主党 8 键→entity 96；美国中期选举 7 键→entity 49。

【E3 结果（只读）】上界跨天候选对 178：同 key 93（异 action/object）+25（同 action/object）；不同 key 60（同 action/object = 0）。叠加层二两闸后：同 key 异 action/object 28、不同 key 8、同 key 同 action/object 0。**关键结论：D14 单独上线一对也合不了**（不同 key 且同 action+object = 0），必须与 D3 层二合用才有 8 对；**25 对「同 key 同 action+object」全部被召回盲区挡掉（25/25 至少一侧 embedding IS NULL）→ 解锁靠 D13**。覆盖率：159/1,262 活跃线（12.6%），涉 57 个 entity_id，不同 key 对覆盖 5 个版块（1980/2272/2197/2128/15220），DeepSeek Harness 占 25/60。

【质量风险（实测）】entity 2「特朗普」吞了 `org:美国共和党`、`person:梅拉尼娅特朗普`、`company:特朗普政府`（9 种 key 混一起）→ 实锤过度合并；直接按「entity 相同 → 同主体」接线会把「特朗普」与「美国共和党」事件合并。另：裸 `subject_key='other'`（无规范名）的 143 条标签散落在 91 个 entity 上（碎片化，不是过度合并）；`product:影之刃零` 裂成 9 个 entity。

【D14 加固建议（待用户拍板）】实体相等只做加法不够，需叠加**名称相容护栏**：两侧 subject 归一化后相等或互相包含才允许越过 `subject_key`；`美国共和党内部人士` vs `特朗普` 不满足 → 挡住误合并；`deepseekharness` vs `deepseek_harness`、`特朗普` vs `美国总统特朗普`、`影之刃零` vs `《影之刃零》` 满足 → 保留收益。裸 `other` 键（无规范名）不参与实体越过。

【工具 bug（已修）】`articleContext` 原 SQL 引用不存在的 `articles.summary`，导致 295 条 ERROR、描述生成上下文退化；已改为 `coalesce(nullif(a.description,''), a.ai_content_summary, '')` + 去 HTML 标签/实体与空白折叠（`plainText`），build + lint 0 issues。E1 已跑完的数据描述质量因此打了折，重跑仅影响 desc 文本，不影响实体 ID。

**引用**：backend-go/cmd/entity-backfill/main.go:articleContext、openspec/changes/enable-story-cross-day-continuation/test-cases.md、openspec/changes/enable-story-cross-day-continuation/tasks.md

<!-- pinned 2026-11-10T06:34:45Z -->

## C1 标定结论：三者独立 T≈0.80 可行、拼短语不占优、实体层可弃

【C1 标定（2026-11-10，qwen3-embedding 2560 维，119 真库配对）】

主体相似度（决定「同主体」判据）：
- 应合并：特朗普/美国总统特朗普 0.9012、特朗普/特朗普总统 0.9129、高市早苗/日本首相高市早苗 0.8828、影之刃零/《影之刃零》0.9380、中基协/中国证券投资基金业协会 0.8184、A股/A股市场 0.9591、苹果/苹果公司 0.8830、日本央行/日本银行 0.9465
- 应否决：特朗普/梅拉尼娅·特朗普 0.6972、特朗普/美国共和党内部人士 0.6629、美国/美国总统特朗普 0.6903、中国/中国证券投资基金业协会 0.5606
- **分不开**：DeepSeek Harness/DeepSeek Harness 团队 0.9079、恒生科技指数/恒生科技指数期货 0.9652（产品相邻项越过一切合理阈值）

真库 119 对（9 正 / 110 负，正样本含 6 对指纹全同 + 3 对人工判定）：
- 主体余弦几乎无区分力：不同 subject_key 对 p50=0.959（多数是同一主体的不同写法）；真正区分靠 object。
- 阈值扫描（min3 = min(subject,action,object)）：T=0.75 → 7/9 正、1/110 误；**T=0.78~0.82 → 7/9 正、0/110 误**；T=0.85 → 6/9 正、0 误。
- 模式2（action+object 拼短语，min(subject, AOP)）：T=0.78 → 8/9 正、**5/110 误**；T=0.82 → 8/9 正、1 误；T=0.85 → 7/9 正、0 误。**同误合并预算下三者独立更优，拼短语不占优。**
- 非 identical 的正样本得分：DeepSeek 推出 Creator Mode ⟷ 发布教程 创造模式 min3=0.665；恒生 上涨 0.96% ⟷ 上涨 超过5% min3=0.829；A股三大股指 ⟷ A股三大指数 min3=0.742。
- 负样本最高分：A股大盘 低开低走 ⟷ 冲高回落 min3=0.773（与正样本 0.742 重叠且方向相反 → 无阈值能分开这一对）。

结论：数据支持「三者独立 + T≈0.80」作为判定，但 9 个正样本里 6 个是指纹全同（trivial），真实改写正样本只有 3 个且与最难负样本重叠 → 结论是**方向可行但证据弱**，需 shadow 期实测；entity 层可弃用（配偶/国家混淆已被主体向量 0.69-0.70 挡住），但 D13（story 向量）是任何 embedding 方案的前提。

**引用**：backend-go/internal/topicgraph/repository/story_repository.go:RecallStoryCandidates、backend-go/internal/topicgraph/service/story_assign.go:classifyStoryCandidate、openspec/changes/enable-story-cross-day-continuation/design.md

<!-- pinned 2026-11-10T07:01:37Z -->

## event 标签 embedding / 辅助标签 / 关系表 可复用点与边界

【结论】新方向「event 标签 embedding 的相关事件连接」可复用的设施与边界（2026-11-10 定向核查）：

可复用：
1) 标签向量表 `topic_tag_embeddings`（backend-go/internal/tagmanagement/models/tag_models.go:22-29）：embedding_type 三值 `identity`/`semantic`/`event_keyword`（core/embedding.go:25-27）。文本组装唯一实现 `buildTagEmbeddingText`（core/embedding.go:700-722）：identity = label+aliases+category（**不含 description**）；semantic = label+". "+description+aliases+category（**含 description**）；event_keyword = 逐关键词单行（core/embedding.go:191-212 getEventKeywords，读 metadata.event_keywords）。
2) 辅助标签：`semantic_labels`（models/semantic_label.go：Label/Description/Aliases/Embedding/MergeEmbedding，label_type=auxiliary），与标签的关联表 `topic_tag_semantic_labels`（tag_models.go:118-126）。辅助标签自身有两条向量：merge 向量（仅 label，用于 L2 归并）+ storage 向量（"label: description"）。L1/L2/L3 去重唯一实现 `auxlabel/auxiliary_label_service.go:270-320`。
3) 相关连接表先例：`daily_report_section_relations`（repository/daily_report_models.go:52-59，from/to + distance + relation_type + (from,to) 唯一），写入 `RebuildBoardRelations`（repository/daily_report_matching.go:298，SaveReport 内按版块全量重建）。
4) 在线阈值：`ai_settings`（models/ai_models.go:8-15）+ 既有读法 `loadAuxLabelDedupeSim`（auxlabel/auxiliary_label_service.go:981+）。
5) 参数暴露 API/UI：`GET/PUT /api/semantic-boards/matching-config`（handler/board_crud_handler.go:138-139），键白名单 `isSemanticBoardConfigKey`（同文件 ~993）、取值校验 `validateSemanticBoardConfigValue`（默认分支限定 0..1 浮点）；前端 `front/app/features/tags/components/MatchingConfigDialog.vue`（AppDialog 740px「参数配置」）＋ `front/app/api/semanticBoards.ts` 的 `MatchingConfig` 接口/get+update ＋ `useTagsPage.ts:164-177 handleOpenMatchingConfig` ＋ 入口 `console/BoardGovernanceRail.vue:162`「匹配规则」→ `console/BoardRadarGovernance.vue:82`。

边界（不得写成 aux 归并或 tag 合并）：
- 辅助标签 L2 命中**只 addAlias**，不得改 Label / 重算 MergeEmbedding（flow/semantic-board.md 红线 1-2）→ 新连接不得写进 `semantic_labels.aliases` / merge_embedding。
- 事件线身份判定路径**不得读实体 ID**、向量不得单独作合并依据（flow/topic-graph.md 红线 14-15）→ 新连接只读、只做「相关」边，不得回写 `board_stories` 身份/`merged_into`。
- `topic_tag_relations`（models/topic_tag_relation.go）虽是现成关系表且 relation_type 枚举含 'related'，但**生产无写入方**，且 reader 侧 `collectDescendantTagIDs`（backend-go/internal/reader/handler/article_handler.go:360-380）与 watched_tags_service.go:72 都**不按 relation_type 过滤**地遍历父子关系 → 往里写 'related' 行会污染后代展开，**不可复用**。
- `semantic_labels`/aux 向量在版块匹配里是**逐对 cosine**（service/board/semantic_board_matching.go:349-520 evaluateSemanticBoardMatches），不是「标签自身融合向量」→ 「label+description+辅助标签」的**融合向量是全新表示**，必须新建并**重新标定阈值**（不可套 C1 的 min3 结论）。

【引用】backend-go/internal/tagmanagement/service/core/embedding.go:25-27,191-212,700-722；backend-go/internal/tagmanagement/service/auxlabel/auxiliary_label_service.go:270-320,981+；backend-go/internal/tagmanagement/models/tag_models.go:22-29,118-126；backend-go/internal/models/semantic_label.go；backend-go/internal/topicgraph/repository/daily_report_models.go:52-59；backend-go/internal/topicgraph/repository/daily_report_matching.go:298；backend-go/internal/tagmanagement/handler/board_crud_handler.go:138-139,~993,1000-1019；backend-go/internal/reader/handler/article_handler.go:360-380；front/app/features/tags/components/MatchingConfigDialog.vue；front/app/api/semanticBoards.ts；front/app/features/tags/composables/useTagsPage.ts:164-177；front/app/features/tags/components/console/BoardGovernanceRail.vue:162；front/app/features/tags/components/console/BoardRadarGovernance.vue:82

**引用**：backend-go/internal/tagmanagement/service/core/embedding.go、backend-go/internal/tagmanagement/service/auxlabel/auxiliary_label_service.go、backend-go/internal/tagmanagement/models/tag_models.go、backend-go/internal/models/semantic_label.go、backend-go/internal/topicgraph/repository/daily_report_models.go、backend-go/internal/tagmanagement/handler/board_crud_handler.go、backend-go/internal/reader/handler/article_handler.go、front/app/features/tags/components/MatchingConfigDialog.vue、front/app/api/semanticBoards.ts、front/app/features/tags/composables/useTagsPage.ts

<!-- pinned 2026-11-10T10:15:45Z -->

## 旧 event-story / story-heat-evidence delta 的撤回方式（不误 REMOVE 主 spec）

【结论】彻底重做现有 change 时，旧 delta 的撤回方式（2026-11-10 核对 openspec 1.13.2 应用语义 + 磁盘主 spec 实况）：

磁盘实况：主 spec 已存在但**未被 git 跟踪**（`openspec/specs/event-story/spec.md`、`openspec/specs/story-heat-evidence/spec.md` 均是 untracked）——delta 以磁盘版为准。
- event-story 主 spec 现有 7 条 Requirement：事件指纹结构化产出 / 主体归一化 / 事件线数据模型 / 事件归属判定顺序 / 综合稿不建立事件身份 / 人工纠正优先 / 历史回填与降级。事件线数据模型仅 3 个 Scenario（事件未归组时字段为空 / 事件线跨天聚合 / 被合并的事件线保留地址）。
- story-heat-evidence 主 spec「热度消费口径一致」仅 3 个 Scenario（矩阵数字为事件线数 / 无事件但有分区 / 雷达排序与热度一致）。

openspec 应用语义（@fission-ai/openspec dist/core/specs-apply.js）：
1) **MODIFIED 必须包含主 spec 该 Requirement 的全部 Scenario**，少一个 → archive 直接报错「current spec contains scenario(s) not present in the modified block」（diffScenarioNames）。所以重写 delta 时，对仍要改的 Requirement，必须把主 spec 原文整块抄进来再改。
2) **REMOVED 用错会误伤主 spec**：对不存在于主 spec 的名字 REMOVE 只 warn「treating it as already removed」（除非大小写/空格近似同名 → 直接报错）。→ 本 change 的旧 ADDED（「跨天延续的合并证据与可复核标记」「跨天延续的存量重算与快照回滚」）从未 archive 进主 spec，**撤回方式是直接从 delta 文件里删掉这些块，不写 REMOVED**；写 REMOVED 只会产生噪音，且一旦名字与主 spec 近似就会炸。
3) MODIFIED 名字不存在于主 spec → 报错；ADDED 名字已存在 → 报错。→ 换方向时优先「删块」而非「REMOVE」。

据此的重做建议（制品层）：
- event-story delta 收敛为：MODIFIED「事件线数据模型」= 主 spec 3 个 Scenario **＋ 已落地的 D13 两个 Scenario**（建线时缺向量用标题现算 / 存量无向量线可回填，代码已在 story_attribute.go:554+ 与 cmd/backfill-stories/fill_*_embeddings.go）＋（可选）ADDED「事件身份重算与快照回滚（实验工具）」描述已存在的 --reattribute/--snapshot/--restore；**删掉** 事件指纹结构化产出 / 事件归属判定顺序 的 MODIFIED 块（不再做 object_type、实体接线、两层判定），主 spec 原文保持不动。
- story-heat-evidence delta **整体删除**（其两个新增 Scenario 都是跨天延续语义，前提消失；「当日窗口参与者数」（D8）也无存在理由——无跨天合并则无历史日格污染），并从 proposal 的 Modified Capabilities 中移除该能力。
- 新方向的能力载体建议**新建 capability**（如 `event-related-connection`）：新能力 delta 必须带 `## Purpose`（≥50 字符，否则 `openspec validate --strict` 报过短）。

【引用】openspec/specs/event-story/spec.md；openspec/specs/story-heat-evidence/spec.md；openspec/changes/enable-story-cross-day-continuation/specs/event-story/spec.md；openspec/changes/enable-story-cross-day-continuation/specs/story-heat-evidence/spec.md；/home/zanebono/.nvm/versions/node/v26.8.2/lib/node_modules/@fission-ai/openspec/dist/core/specs-apply.js:146-460；backend-go/internal/topicgraph/service/story_attribute.go:129-178,554-560；backend-go/cmd/backfill-stories/

**引用**：openspec/specs/event-story/spec.md、openspec/specs/story-heat-evidence/spec.md、openspec/changes/enable-story-cross-day-continuation/specs/event-story/spec.md、openspec/changes/enable-story-cross-day-continuation/specs/story-heat-evidence/spec.md、backend-go/internal/topicgraph/service/story_attribute.go

<!-- pinned 2026-11-10T10:15:46Z -->

## UI 落点建议与遗留实验（D13/回填工具/实体 off）处置

【结论】UI 落点 + 遗留实验处置（2026-11-10）：

A. UI 沿用既有界面（不杜撰位置）：
- 阈值配置 → 复用 **tags console 治理抽屉「匹配规则」参数配置弹窗**：`front/app/features/tags/components/MatchingConfigDialog.vue`（AppDialog，存量 `width="740px"`，标题「参数配置」）；两个入口：`console/BoardGovernanceRail.vue:162`「匹配规则」→ `console/BoardRadarGovernance.vue:82 openAdvanced('matching')`；`console/BoardRadarMasthead.vue:76 data-testid="radar-more-matching"`。数据流：`useTagsPage.ts:164-177 handleOpenMatchingConfig/handleSaveMatchingConfig` → `front/app/api/semanticBoards.ts` `getMatchingConfig/updateMatchingConfig` + `MatchingConfig` 接口 → 后端 `GET/PUT /api/semantic-boards/matching-config`（handler/board_crud_handler.go:138-139），键白名单 `isSemanticBoardConfigKey`（同文件 ~993）、取值校验 `validateSemanticBoardConfigValue`（默认分支限定 0..1 浮点）、`getAllConfigs`（~979-991）汇总输出；持久化在 `ai_settings`（models/ai_models.go:8-15）。
- 新阈值（如相关事件连接 θ=0.82）需**新增 ai_settings 键 + 白名单项 + 校验项 + 前端 MatchingConfig 字段 + 弹窗内一个 rule-block**（沿用既有 AppInput + 段落结构），属既有页面结构内加字段 → **ui-impact: minor**（ui-design.md 需从 N/A 改为 minor 合同：入口、loading/empty/error/success 状态、组件复用、验收映射指名组件测试文件；approval=not-required、prototype=none）。
- **若**还要在雷达右栏/阅读台新增「相关事件」展示区 → 新增展示面板/信息架构 → **major**，建议**另开 change**，本 change 只做连接计算 + 阈值配置（数据层产物供后续消费）。

B. 遗留实验标法（不回滚生产代码/数据）：
1) **D13 召回兜底已落地**：建线时标题现算 `backend-go/internal/topicgraph/service/story_attribute.go:129-178,554-560`；回填 CLI `backend-go/cmd/backfill-stories/fill_story_embeddings.go`、`fill_thread_embeddings.go`（--fill-story-embeddings / --fill-thread-embeddings）。它服务的是**同日归属召回**（story_repository.go RecallStoryCandidates），不因新方向作废 → 制品中标「已落地、继续生效、非跨天专属」；规格以 ADDED「事件线召回可用性（已落地）」入 delta，措辞不含跨天语义。flow/topic-graph.md 红线 16 已记录该行为。
2) **跨天身份重算工具**（--reattribute/--snapshot/--restore，tasks 4.1/4.2 已实现；实现 `internal/topicgraph/service/story_reattribute.go`，CLI `cmd/backfill-stories/main.go:134-136`）：随「跨天合并」方向作废而**保留代码、保留 CLI** → delta 里以窄化 ADDED requirement 记录其存在与边界（不改写既有判定、不自动触发），并在 design 里标 legacy 实验工具；不删 CLI、不删已回填数据（board_stories.embedding 已回填 1262 条、threads 1131 条）。
3) **实体层**：`entity_subject.mode` 默认 `off`（configs/config.yaml:66-67；mode.go:44-71，`active` 显式拒绝），代码与三表（entity_registry/entity_mentions/entity_aux_links）保留、可切 shadow；新方向**不读实体 ID**（flow/topic-graph.md 红线 15 不变），制品标「保留为实验态、不接线」。
4) 纪律：制品只**记录**遗留状态，**不回滚**代码/表/列/已回填数据；撤销动作若需要，另开 change。

【引用】front/app/features/tags/components/MatchingConfigDialog.vue；front/app/features/tags/components/console/BoardGovernanceRail.vue:162；front/app/features/tags/components/console/BoardRadarGovernance.vue:82；front/app/features/tags/components/console/BoardRadarMasthead.vue:76；front/app/features/tags/composables/useTagsPage.ts:164-177；front/app/api/semanticBoards.ts；backend-go/internal/tagmanagement/handler/board_crud_handler.go:138-139,979-1019；backend-go/internal/models/ai_models.go:8-15；backend-go/internal/topicgraph/service/story_attribute.go:129-178,554-560；backend-go/internal/topicgraph/service/story_reattribute.go；backend-go/cmd/backfill-stories/main.go:100-176；backend-go/internal/tagmanagement/service/entity/mode.go:44-71；backend-go/configs/config.yaml:60-68；docs/reference/flow/topic-graph.md 红线 15-16

**引用**：front/app/features/tags/components/MatchingConfigDialog.vue、front/app/features/tags/composables/useTagsPage.ts、backend-go/internal/tagmanagement/handler/board_crud_handler.go、backend-go/internal/topicgraph/service/story_attribute.go、backend-go/internal/topicgraph/service/story_reattribute.go、backend-go/cmd/backfill-stories/main.go

<!-- pinned 2026-11-10T10:16:49Z -->

## 当前有效方案：event标签embedding直接相关（取代旧身份归并）

2026-11-10 用户授权重做change并明确主线程直接写、任务/用例聚焦主故事。当前唯一有效delta为specs/related-events/spec.md；旧event-story与story-heat-evidence delta已撤回，原文留legacy/previous-plan，不改主spec。三个故事：S1查看相关事件、S2调0.82起始阈值、S3 AI历史只读相关性核对。event标签label+description+semantic_labels关联的aux标签生成向量；不依赖subject_key/实体ID/thread向量/指纹trusted，不做身份合并、传递聚类、阶段或发酵。计划独立派生向量缓存，GET只读，显式刷新，既有MatchingConfigDialog和ArchiveMatchPopover复用，UI minor。0.82新文本表示未实测，旧C1与31/65线对不可作效果证明。旧代码/向量/实体off/CLI保留，legacy-experiments.md明确两次真重算后restore及未撤销副作用。共10个新任务和10个Scenario全部未实施。openspec strict、tasks-tail、review-trace、feedback verify通过；scenario-trace因5个新测试文件未建立而红（10场景12条不存在），不得造空测试或假称覆盖。用户本轮仅授权规划，不apply。

**引用**：openspec/changes/enable-story-cross-day-continuation/proposal.md、openspec/changes/enable-story-cross-day-continuation/design.md、openspec/changes/enable-story-cross-day-continuation/specs/related-events/spec.md、openspec/changes/enable-story-cross-day-continuation/tasks.md、openspec/changes/enable-story-cross-day-continuation/test-cases.md、openspec/changes/enable-story-cross-day-continuation/legacy-experiments.md

<!-- pinned 2026-11-10T12:21:17Z -->
