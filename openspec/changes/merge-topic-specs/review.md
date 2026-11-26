# 审查报告：merge-topic-specs

- **审查者**：MiniMax M3.1 Flash（只读）
- **结论**：需改后进
- **放行判断**：合并判断本身成立且内容零漂移（15/15 逐字一致、守恒恒等），但归档门禁⑤（minor 档验收映射）实测为红、且 tasks 3.4a 引用的 flow 红线原文在仓库中不存在，须先补这两处再归档。

## 发现

### R-1 2026-11-26【High】ui-design.md 标 minor 档但缺验收映射节，归档门禁⑤实测为红
- **状态**：已修复
- **处置**：`ui-impact` 由 `minor` 改为 `none`；ui-design.md 重写为 N/A Reason（说明「搬契约不构成 UI 变更」）。根因是过度声明——本 change 零 UI 行为变更，minor 档的验收证据（测试文件/ui-verification）根本不存在。改后门禁⑤绿
- **位置**：`openspec/changes/merge-topic-specs/ui-design.md`（全文只有「为什么是 minor 不是 none」「复用哪些既有契约」「不做什么」三节）；`scripts/harness/ui-verification-trace.sh` minor 分支；`scripts/harness/archive-readiness.sh` 第 ⑤ 项
- **问题**：minor 档要求 ui-design.md 存在 `## Acceptance`/`## 验收` 节，且节内指名道姓挂「组件测试 + 真实存在的测试文件路径」或「agent-browser/人工 + `ui-verification/report.md` ≥1 条合法条目」。本 change 的 ui-design.md 一个验收节都没有，脚本在 `acc="$(extract_section "$DESIGN" 'Acceptance|验收')"` 取到空串 → `comp_hit=0 / manual_hit=0` → 直接判红。tasks 8.1「archive-readiness.sh 六项全绿」按现状不可达，归档会被 ⑤ 拦下。
- **证据**：
  - `bash scripts/harness/ui-verification-trace.sh openspec/changes/merge-topic-specs` → `✗ 未通过：minor 档验收映射未指名道姓（验收节须映射到 组件测试 或 agent-browser/人工验证 至少其一）`，`EXIT=1`
  - `bash scripts/harness/archive-readiness.sh openspec/changes/merge-topic-specs` → `✗ ⑤ UI 验收痕迹对账（exit 1）`、`✗ 2 项红`（另一红是 ④ review-trace，本报告落盘后即消）
  - 脚本判定逻辑：`scripts/harness/ui-verification-trace.sh:385-420`（minor 分支）+ `:149-155`（`extract_section` 只认 `^## .*(Acceptance|验收)` 标题）
  - 另注：本 change 前三族先例全部用 `none` 档（`archive/2026-11-26-merge-tag-spec-family/proposal.md` 头 `<!-- ui-impact: none -->` + ui-design.md `## N/A Reason`，`merge-board-upgrade-api-specs` 同样在 N/A 里写明「board-editing-ui 的对话框契约原样搬运」仍走 none 档）——本族用 minor 档不是先例，且正是 minor 档触发了这道门禁
- **建议**：二选一。① 保留 minor 档，在 ui-design.md 增 `## 验收` 节并挂已存在的实物测试（均在盘：`front/app/features/tags/components/KeyEventMarkerActions.test.ts`、`front/app/features/tags/components/daily-report/DailyReportWatchIndex.test.ts`、`front/app/features/tags/composables/useBoardKeyEvents.test.ts`、`front/app/features/tags/components/console/KeyEventTimelineView.test.ts`），写明「本 change 零 UI 行为变更，验收等价于既有组件测试全绿」；② 按前三族先例把 proposal.md 头部与 ui-design.md 降为 `<!-- ui-impact: none -->` + `## N/A Reason`（理由：本 change 零代码改动、无新增/删除/重命名控件，标记与物化契约原样搬运）。

### R-2 2026-11-26【Medium】tasks 3.4a 引用的 flow 红线原文在仓库中不存在，两处「受控例外」其实不与 flow 打架
- **状态**：已修复
- **处置**：**我的错误**：brief 里那 5 条「红线」是手写的，未走 `constraint-digest.sh`，其中「不得出现无主 section」在仓库中不存在（`grep -c 无主` = 0）。已取真实红线（第 1 条是「planLifecycle 只更新命中计数、不得自动变更 status」），spec 的两处 NULL/unassigned 例外与它不冲突。tasks 3.4a 标误报并撤回，同时记下派发纪律：后续 brief 一律用脚本输出原样粘贴
- **位置**：`openspec/changes/merge-topic-specs/tasks.md` 3.4a；`docs/reference/flow/topic-graph.md`（业务约束与不变量节）；`openspec/specs/persistent-topic/spec.md:62`、`:449`
- **问题**：3.4a 断言「`docs/reference/flow/topic-graph.md` 写『topic 身份以 PersistentTopic 为准，DailyReportSection 必须 1:N 归属，不得出现无主 section』」，但该句在 flow 目录里**查不到**；flow 的现行红线并不禁止未归属 section。结论：既不是 spec 的问题，也不是 flow 红线写得太绝对，而是**引用错了出处**——若照这条「假红线」去改 spec（例如删除两处例外），反而会造成行为回归。
- **证据**：
  - `grep -rn '无主\|1:N\|以 PersistentTopic 为准' docs/reference/flow/` → 0 命中；`grep -rn '单成员' docs/` → 0 命中
  - flow 现行口径允许未归属：`docs/reference/flow/topic-graph.md` 红线 6「section 话题快照字段在 SaveReport 同一事务写入，不得随后续话题状态变化回填：…（快照值 = 当时 PersistentTopic 的 candidate|active；**未归属写 NULL**）」；`docs/reference/flow/daily-report.md` 红线 5 同款
  - DB 文档同口径：`docs/reference/database/tables/_conventions.md:209`「`daily_report_sections.persistent_topic_id` → `board_persistent_topics(id` — **可空**，无 OnDelete」；`docs/reference/database/tables/daily-report-watch.md:134` 明写 `topic_match_confidence` 的 `unmatched` 态 = 「section 无 embedding，无法分桶」
  - 两处「例外」确实存在（评审题给的定位成立）：`openspec/specs/persistent-topic/spec.md:62`「embedding 为空时 confidence=unmatched、lane_tier 允许为空、persistent_topic_id 允许为 NULL（仅此例外）」+ 场景 `:130-133`；`:449`「成员数为 1 的孤立 cluster … SHALL NOT 建话题，其 section SHALL 保持 `persistent_topic_id IS NULL`（unassigned）」+ 场景 `:455-459`、`:467-471`
  - 「强制 1:N」这一绝对表述的实际出处不是 flow，而是 spec 自己的 Purpose（`persistent-topic` Purpose「section 强制 1:N 归属」）与 DB 文档散文句 `daily-report-watch.md:107`「一个板块 N 个话题，每个 section 归属一个话题（1:N）」
- **建议**：把 3.4a 改写为「引用错误：flow 无该红线句；现行 flow 红线 6 / daily-report 红线 5 与 DB 文档均明确允许未归属写 NULL，spec 两处例外与现行口径一致，**不得据此删除例外**」；另起一条 Low 级登记，把 `daily-report-watch.md:107` 的散文句「每个 section 归属一个话题（1:N）」与 spec/DB 的「可空 + unmatched」口径对齐（属存量文案，另案处理）。

### R-3 2026-11-26【Medium】分类 #8「历史数据回刷」判 impl 不当：它是可重复调用的回刷端点契约，含长期顺序不变量
- **状态**：已修复
- **处置**：#8「历史数据回刷」改判 business：它是可重复调用的回刷端点（不是一次性脚本），且「在 SaveReport 之后、RebuildBoardRelations 之前」是长期顺序不变量。重新生成后本族 0 impl
- **位置**：`openspec/specs/persistent-topic/spec.md:308-325`（= 分类序号 8，见 `spec-classification.txt`）；判定口径见 `docs/reference/standard/shared/doc-authoring.md`「合并时发现施工记录（包结构/命名/一次性迁移/纯测试要求）…加 `<!-- kind: impl -->`」
- **问题**：该条写的是「系统 SHALL 提供 `BackfillPersistentTopics(boardID)`」这一**对外可调用的端点契约**，且含一条长期有效的顺序不变量「回刷 SHALL 在 SaveReport 之后、RebuildBoardRelations 之前执行（关系重建需要 topic_id 写身份边）」——这不是一次性迁移的施工步骤，而是「任何时候调用都要满足」的运行契约（分类方自己在 3.4b 也提了这句）。按 doc-authoring 的 impl 定义（包结构 / 命名 / 一次性迁移 / 纯测试要求），本条更接近 business。
- **证据**：
  - `openspec/specs/persistent-topic/spec.md:308-325`：`系统 SHALL 提供 BackfillPersistentTopics(boardID)…回刷 SHALL 回填所有历史 section 的 persistent_topic_id。回刷 SHALL 在 SaveReport 之后、RebuildBoardRelations 之前执行` + 场景「回刷历史 section 归属」「回刷后关系含身份边」
  - 该能力是常驻 handler 入口（`docs/reference/flow/topic-graph.md` 代码入口节：`handler/daily_report_handler.go` 列有 `POST … backfill-topics`），不是一次性迁移脚本
  - 误判的副作用不是排版问题：doc-authoring 明确「impl 段**不删**——删契约要单独提 change」，反过来说，标成 impl 等于把这条端点契约与顺序不变量标记为「可被后续 change 删掉的施工记录」，后续清理族 spec 时容易连带删掉顺序不变量
- **建议**：把序号 8 改判 business（`spec-classification.txt` 第 8 行 `8 impl` → `8 business`，并同步 tasks 3.1 的「25 business / 1 impl」→ 26/0）；若坚持保留 impl 归类，则必须把「SaveReport 之后 / RebuildBoardRelations 之前」这句顺序不变量拆成独立的 business 条目再归 impl。

### R-4 2026-11-26【Medium】分类 #12「回刷重建持久化话题（单成员不建）」归 business 与 #8 同族同性质却口径不一
- **状态**：已修复
- **处置**：#12 与 #8 同性质口径已统一——两者都是 business；#8 改判后不再存在「同族同性质口径不一」
- **位置**：`openspec/specs/persistent-topic/spec.md:445-471`（= 分类序号 12）；对照 `openspec/specs/persistent-topic/spec.md:308-325`（= 序号 8）
- **问题**：#12 的主体同样是回刷程序的行为契约（complete-link 聚类、规模门槛 ≥2、回刷事务末尾触发 `RebuildBoardRelations`、「回刷 SHALL NOT 调用任何 LLM」），只有「单成员不建 → section 保持 unassigned → 留给日报路径 candidate 涌现」这一条是跨日持续语义。#8 判 impl、#12 判 business，两者同属「一次性回刷程序」却在同一族里分到两段，分类边界不自洽。
- **证据**：
  - `:445-471` 原文：`BackfillPersistentTopics 为历史未归属 section 重建持久化话题时，SHALL 采用 complete-link 聚类…`、`**规模门槛**：只有成员数 ≥ 2 的 cluster SHALL 建话题…`、`回刷 SHALL NOT 调用任何 LLM（纯聚类 + 向量运算）。回刷事务末尾 SHALL 触发 RebuildBoardRelations`
  - `spec-classification.txt` 第 8 行 `8 impl`、第 12 行 `12 business`
- **建议**：与 R-3 一并统一口径（建议 #8 / #12 同判，或明确写「#12 的『单成员不建』是持续不变量、其余为 impl 子句」并在 tasks 3.x 记判定理由）；无论选哪种，都要在 tasks 里留一句判定理由，避免下一族沿用时再次分叉。

### R-5 2026-11-26【Medium】就地扩容下 impl 标记无处落地 + D5「两段式」不成立，分类结论在制品里不可核
- **状态**：已修复
- **处置**：design D5 补明：**就地扩容族不具备两段式的落地条件**（原有条目留在原位，只有新增条目进 delta，impl 若在保留条目里既不会被重排也不会带标记）。本次因分类 39/0 无 impl 故不触发，但口径已写清
- **位置**：`openspec/specs/persistent-topic/spec.md`（19 条无一有 `<!-- kind: impl -->`）；`scripts/harness/merge-spec-family.sh`（`--kind-file` 解析与 ADDED 分支）；`openspec/changes/merge-topic-specs/spec-classification.txt`；design D4 第 2 条 vs D5 第一句
- **问题**：① `merge-spec-family.sh` 只在 **ADDED（搬入的 15 条）** 分支写 `<!-- kind: impl -->`，族自身既有条目走不到该分支；而 `persistent-topic` 现状 19 条 `grep impl` 零命中 → 归档后族 spec 里一条 impl 标记都不会有，分类结果在制品里不可见；② D5 声明「business 在前、impl 在后」，但就地扩容不改既有条目顺序（`历史数据回刷` 固定在第 8/26 位），两段式在本次事实上不可能成立，design D4 第 2 条与 D5 未调和；③ `spec-classification.txt` 只有 39 行「序号 business|impl」，不与 requirement 标题绑定，读者无法从制品本身判断哪条是 impl（我只能靠脚本的「族 → cap → Requirement」生成序反推：1-19 = persistent-topic 既有 19 条、20-26 = key-event-markers、27-31 = topic-watch 既有 5 条、32-39 = watch-materialized-topic；据此 #8 = 历史数据回刷、#12 = 回刷重建持久化话题、#14 = PersistentTopic 质心表示、#25 = 条目标记交互、#33 = 一句话轨辅助标签检索、#39 = 物化板块当日标题，与题面描述逐条吻合——反推正确，但制品本身不自解释）。
- **证据**：
  - `grep -rn 'kind: impl' openspec/specs/persistent-topic/spec.md openspec/specs/topic-watch/spec.md` → 0 命中（对照：`openspec/specs/board-api/spec.md`、`tagging/spec.md` 等 4 个 spec 有 impl 标记）
  - `scripts/harness/merge-spec-family.sh`：`if kind == "impl": lines.append("<!-- kind: impl -->")` 只在生成**整族**文本与 **ADDED** 分支出现，就地扩容族的原有条目不进这两条路径
  - `openspec/changes/merge-topic-specs/spec-classification.txt` 全文 39 行，每行仅「N business|impl」，无标题映射
  - design D4 第 2 条「原有条目留在原位、新增条目追加末尾」；D5「沿用已验证的两段式：business 在前（保持来源顺序）、impl 在后」——本次两者不可兼得
- **建议**：① 归档后在 `persistent-topic` 的「历史数据回刷」标题下补 `<!-- kind: impl -->`（并把该动作写进 tasks 8.3 的手改清单，与 Purpose 手改同提交）；② 在 design D4 里显式写「就地扩容族豁免两段式排序，以 impl 标记区分」，把 tasks 3.1 的「25 business / 1 impl」表述改成「分类结果（排序不适用）」，避免读起来像已完成排序；③ `spec-classification.txt` 增一列「Requirement 标题」映射表（或在文件头写明生成序规则），使分类可独立复核。

### R-6 2026-11-26【Medium】旧 capability 名清扫不完整：8.3b 只修 Purpose，同族正文仍有 3 处自指，另有 daily-report-system 2 处悬空指针
- **状态**：已采纳
- **处置**：tasks 7.3 记下实测结果：`topic-watch` 正文 3 处自指 + `daily-report-system` 2 处悬空指针，全部登记到 7.4 的「跨族引用清扫」批次（改 main spec 需 MODIFIED delta，会破守恒）
- **位置**：`openspec/specs/topic-watch/spec.md:7、9、17、118`；`openspec/specs/daily-report-system/spec.md:237、642`；tasks 7.3 / 7.4 / 8.3b
- **问题**：合并后 `watch-materialized-topic` 这个 capability 名不复存在，但 `topic-watch` 自己的**正文**里仍写着「见 watch-materialized-topic 能力」——这会变成「族内自指」（读者在 `topic-watch` 里被指向 `topic-watch`），语义比跨族悬空更迷惑；tasks 8.3b 只写了「手改 Purpose 并去掉自指」，没覆盖正文两处。另有 `daily-report-system` 两处引用同一批退役 capability，归档后成为悬空指针。
- **证据**：
  - `openspec/specs/topic-watch/spec.md:17`「sentence_topic 关注 SHALL 持久化其专属持久话题的关联（关联建立于首次物化时，**见 watch-materialized-topic 能力**）」
  - `openspec/specs/topic-watch/spec.md:118`「删除 sentence_topic 关注：…（**联动细节见 watch-materialized-topic 能力**）」
  - `openspec/specs/topic-watch/spec.md:9`（基线来源说明里的 `watch-materialized-topic（2026-08-25）`，历史溯源语，可保留但建议注明「已并入本族」）
  - `openspec/specs/daily-report-system/spec.md:237`「…**Watch 物化追加**（keyword_topic / sentence_topic 物化轨按 **watch-materialized-topic 能力**产出追加 section…）」、`:642`「watch_sentence section SHALL 归属其关注的专属持久话题（**见 watch-materialized-topic 能力**）」
  - 参照先例：上一族把这类指针当本 change 内的活干（`archive/2026-11-26-merge-tag-spec-family/tasks.md` 7.4「修旧 capability 名的跨文件引用…另列为本 change 内的仅改指针 MODIFIED」）；本族则把跨族指针整体推给「跨族引用清扫」change（tasks 7.4），族内自指却没被 8.3b 覆盖
- **建议**：8.3b 扩写为「手改 `topic-watch` 的 Purpose（用 delta 的 `## Purpose` 原文，见 R-8）**并**把正文 `:17`/`:118` 的「见 watch-materialized-topic 能力」改为族内指向（如「见本族『一句话轨持久话题联动』」「见本族『删除关注联动』」）」；`daily-report-system` 两处在 tasks 7.3 记录确切行号并按 7.4 登记到跨族引用清扫 change（不必在本 change 改 `daily-report-system`，但要留痕）。

### R-7 2026-11-26【Medium】D3 预判的未来拆分点与合并后实际内容不匹配
- **状态**：已修复
- **处置**：design D3 更正拆分点：不是「话题模型 vs 泳道呈现」，而是「话题身份与归属 vs 话题在界面上的呈现」——后者约 11 条（泳道节点 4 + 关键事件标记 7）
- **位置**：design D3 末段「若后续再增长，拆分点是『话题模型 vs 泳道呈现』：前者是数据/生命周期/API，后者是节点高亮/可读性/计数口径」
- **问题**：合并进来的 7 条里只有「条目标记交互」是 UI 契约，而它长在**日报阅读器的 thread 行**（左列标题/摘要、右列 AI 理由、动作组行底右对齐、720px 单列回退），不是「泳道呈现」；真正新增的异质性是「关键事件标记域」——它横跨日报阅读器（标记交互）+ 话题总览（confirmed 高亮）+ 版块态势页（`GET /api/semantic-boards/:id/key-events` 批量读 + 关键事件时间线）。按 D3 现措辞去拆，会把「条目标记交互」错归到「泳道呈现」一侧，留下真正该拆的边界没拆。
- **证据**：
  - 搬入的 7 条标题：人工选择关键节点 / 有依据的重要进展推荐 / 保存选择与重复操作 / 作用域与来源保护 / 已确认节点的来源语义 / **条目标记交互** / 版块级关键事件批量读
  - 「条目标记交互」原文（delta `specs/persistent-topic/spec.md`）：「新增日报动作 SHALL 在现有全屏日报阅读器的 thread 行中作为文章展开按钮的同级控件…线索行呈现 SHALL 采用左右等宽两列分区…窄屏（≤720px）SHALL 回退单列」——语境是日报阅读器，不是泳道
  - 「版块级关键事件批量读」原文：「供版块态势页一次取全观察窗内的事件」——消费面是版块态势页
  - 反向证据（支持合并而非另立）：`persistent-topic` 的「话题生命线 API」正文已内嵌关键事件节点读侧：「响应 SHALL 向后兼容地提供可选已确认关键节点列表…节点 SHALL 不占用真实 section ID」，即标记族的读端本就长在 persistent-topic 的 API 里
- **建议**：把 D3 的拆分点改写为「话题模型 + 泳道呈现 vs 关键事件标记域（日报阅读器 + 版块态势页）」；D3 的行数结论（885 < 1200 预警线）本身仍成立，但拆分边界要按实际内容改写，否则下一族沿用时会拆错刀。

### R-8 2026-11-26【Medium】tasks 3.3「跨 spec 重复」检查未完成，合并后同一族内出现 3 处同义重述
- **状态**：已采纳
- **处置**：tasks 3.3 登记 3 处同义重述（节点交互、并排展示、watch 隔离各一次），不修（改正文破守恒）
- **位置**：`openspec/changes/merge-topic-specs/tasks.md` 3.3（仍为 `[ ]`）；`openspec/specs/persistent-topic/spec.md:599-652`（「当前泳道关键节点高亮」「泳道节点可读性与就地详情」「版块内泳道边界与既有主题」）× 搬入的「条目标记交互」
- **问题**：合并把「总览只读 confirmed、建议不外泄、失效快照不伪造 section、不写回原 section 标题与计数」这组规则的三处重述放进了同一个文件，且没有登记。上一族的做法是「重复的合并为一条并在来源注释里列两个来源」（`archive/2026-11-26-merge-tag-spec-family/tasks.md` 3.4），本族 tasks 3.4 又定「不改 spec 内容」→ 结果是合并文件里同一规则出现三遍，后续读者会误当成三条独立规则去改。
- **证据**：三处重述——
  - 搬入的「条目标记交互」：「话题总览（包括lanes、focus和就地详情）SHALL NOT 展示未确认建议、理由或上述挑选按钮…只有confirmed注记能新增关键强调」
  - 既有「当前泳道关键节点高亮」：「未确认AI建议只在日报展示，总览不得出现建议徽标、理由或挑选动作…同一section有多条已确认thread时 MAY 以节点旁的星标数量入口紧凑展示…无真实来源的快照 SHALL 用同轴独立快照标识而非伪造section」
  - 既有「泳道节点可读性与就地详情」：「点击/键盘选择 SHALL 展示对应条目原位详情而非直接确认…总览不提供新确认/不采纳，已有确认可取消维护」
  - 既有「版块内泳道边界与既有主题」：「关键文字来自thread/确认快照并作为独立注记，不能回写原section标题…真实section ID、关系和动态计数 SHALL 保留；失效快照 SHALL 不伪造section」
- **建议**：本轮至少把 3.3 做掉并按 3.4 的「只登记不修」口径写进 review/登记条目（记录「同义重述 3 处，建议后续合并为一条 + 来源注释列两个来源」）；本 change 不改文本亦可，但必须留痕，否则「跨 spec 重复检查」这一项等于白写。

### R-9 2026-11-26【Low】D2 对 `related-events` 的排除理由不完整：该 capability 4 条里有 1 条是日报生成期契约
- **状态**：已修复
- **处置**：design D2 补：related-events 的 4 条里有 1 条是「日报同事件聚簇」，排除理由改为「主体是阅读页检索，只 1 条涉及日报且是读数不写数」
- **位置**：design D2；`openspec/specs/related-events/spec.md:69-121`
- **问题**：D2 把 related-events 概括为「事件检索，消费者在阅读页」，但它 4 条 Requirement 中第 4 条「日报同事件聚簇」是**日报生成管线**的契约（当天同版块 event 标签聚簇 = 今日重点候选源、批量 embedding 调用、`ai_call_logs` 审计、不落库）。排除结论仍成立（该条完全不触碰 PersistentTopic / section 归属 / 泳道），但理由陈述与实物不符，且顺带暴露它自己的 domain marker（`topic-graph, semantic-board`）没覆盖它实际依赖的 daily-report 红线。
- **证据**：`openspec/specs/related-events/spec.md` 4 条标题 = 查看相关事件 / 在线调整相关阈值 / 历史样本验证相关性 / **日报同事件聚簇**；`grep -c '^### Requirement:' → 4`；「日报同事件聚簇」原文含「日报生成时，系统 SHALL 在**同一版块当天**的 event 标签范围内做同事件聚簇，作为『今日重点』的候选源」；其 marker 为 `<!-- domain: topic-graph, semantic-board -->`（未含 daily-report）
- **建议**：D2 的理由改成「4 条里 3 条读侧推荐 + 1 条日报聚簇，均不触碰话题建模，故不并入话题族」；related-events 的 marker 缺 daily-report 属存量问题，另案登记即可，不在本 change 处理。

### R-10 2026-11-26【Low】ui-design.md「复用哪些既有契约」表 4 行里 2 行出处对不上，且漏了一个消费面
- **状态**：已采纳
- **处置**：ui-design.md 重写为 none 档 N/A，原「复用既有契约」表不再适用（本 change 无 UI 变更）
- **位置**：`openspec/changes/merge-topic-specs/ui-design.md`「复用哪些既有契约」表
- **问题**：① 表第 3 行把「日报 section 栏位（常规聚类 / 物化 watch 并排）」挂在「物化板块当日标题」名下，但「并排/栏位」口径实际住在「物化 section 管线边界」（`lane_tier` 标记来源、不参与同日 section 合并、不参与关系计算、report 聚合计数不重算）与 flow `daily-report.md` Step7.5，「物化板块当日标题」只管 `cluster_label` 的生成与兜底；② 表第 4 行「物化失败时的**降级展示**」对应的「物化失败降级」写的是管线降级（召回失败→跳过该关注；裁决失败→回退召回全量 + confidence 默认值 + 记日志），不是展示层口径；③ 表未列搬入的「版块级关键事件批量读」的消费面（版块态势页「关键事件」视图）。
- **证据**：表 4 行分别写「① 泳道节点点击 → 就地详情 ← persistent-topic 泳道节点可读性与就地详情（对得上）② 关键节点标记的视觉态（已标记/未标记）← 条目标记交互（条款存在，但「已标记/未标记视觉态」不是该条的术语，该条讲的是动作组/状态徽标/左右两列/720px 折叠）③ 日报 section 栏位 ← 物化板块当日标题（出处错位）④ 物化失败时的降级展示 ← 物化失败降级（语义漂移））；实际条款见 `openspec/changes/merge-topic-specs/specs/topic-watch/spec.md`（物化 section 管线边界 / 物化失败降级 / 物化板块当日标题）与 `openspec/specs/persistent-topic/spec.md:102-133`（条目标记交互）
- **建议**：校正 ②③④ 的出处表述并补「版块级关键事件批量读 ← 版块态势页关键事件视图」一行；若按 R-1 走 (b) 降 none 档，则整表替换为 N/A 理由（与前三族一致）。

### R-11 2026-11-26【Low】proposal/design 引用的行数比实物各多 1 行，验收对账口径要统一
- **状态**：已修复
- **处置**：行数口径统一为「来源 spec 之和」，tasks/design 已注明；delta 实测行数见 merge 脚本 stderr
- **位置**：proposal.md「Why」表与「不做的事」；design D2/D3；tasks 1.1
- **问题**：proposal 写 persistent-topic 717 / topic-watch 142 / topic-key-event-markers 168 / watch-materialized-topic 202 / related-events 122，仓库实物依次是 716 / 141 / 167 / 201 / 121（各差 1 行）。不影响 4→2 的判断，但 tasks 9 的验收表与 1.1 的统计会被逐行对账。
- **证据**：`wc -l openspec/specs/{persistent-topic,topic-watch,topic-key-event-markers,watch-materialized-topic,related-events}/spec.md` → 716 / 141 / 167 / 201 / 121
- **建议**：tasks 1.1/9 与 proposal 数字按实物改（或统一注明计数口径含尾行/不含尾行）。

### R-12 2026-11-26【Low】归族判断的边界说明有两处需要收紧口径（related-events / data-enrichment）
- **状态**：已修复
- **处置**：design D2 收紧口径：data-enrichment 的排除理由不是「域声明不同」（结果非理由），而是「改它不需要理解话题模型，反之亦然」
- **位置**：design D1 / D2；proposal「判据」段
- **问题**：① D1 说 watch 组「同一个 watch 实体、同一条日报管线、同一批 API」——前两项成立，**「同一条日报管线」不成立**：提示轨判定在「日报生成流程末尾（section 持久化与 persistent_topic 归属完成之后）」，物化轨在日报落库前的 Step7.5 追加 section，两者在流水线的**不同点位**（这一点也正是红线 4「并排展示」的实现前提）。② D2 对 data-enrichment 的排除成立（29 条全是 agent loop / 分层汇总 / 见解分级 / 报告追问 / 素材新鲜度门，域声明 `data-enrichment` 打头，与话题建模不同层），但它对 key-event-markers 有**真实读取依赖**（`flow/data-enrichment.md` 红线 36：confirmed 注记以 `SignalKeyEventAttention` 并入 `SignalLaneMaterial.ConfirmedKeyEvents`，只作 LLM 关注提示、永不写入证据链）——这条依赖决定了标记族必须留在 topic-graph 侧被 daily-report/semantic-board 共同约束，值得在 D2 补一句，避免日后有人按「data-enrichment 也用关键事件」而反向并族。
- **证据**：`openspec/specs/topic-watch/spec.md`「关注标记 AI 命中判定」原文「日报生成流程末尾（section 持久化与 persistent_topic 归属完成之后），系统 SHALL 对该 board 下所有 `status=active` 且 `type=label` 的关注标记执行 AI 命中判定」；`docs/reference/flow/daily-report.md:76` Step7.5「Watch 物化追加…同日合并之后、落库之前」；`docs/reference/flow/data-enrichment.md:310` 红线 36（`collectConfirmedKeyEvents` 只作 attention hint）
- **建议**：D1 把「同一条日报管线」改为「同一份日报产物、流水线上的两个点位」；D2 补一句 data-enrichment 对 confirmed 注记是**只读消费**（attention hint）而非建模依赖，据此维持不并。

## 核实记录

以下为本轮独立复核（只读命令），结论与题面「已机器验证通过」清单一致或补充如下：

1. **内容零漂移（自建脚本复核，15/15 逐字一致）**：从 `openspec/specs/{topic-key-event-markers,watch-materialized-topic}/spec.md` 抽 Requirement 块，与本 change delta 的 ADDED 块做「去 `<!-- from: -->` 注释 + 折叠空行」归一后逐字比对 → `ADDED requirements compared: 15 | text mismatches: []`（唯一差异是注释位留下的空行，非语义）。
2. **Scenario 守恒**：搬入侧 key-event-markers 28 + watch-materialized-topic 25 = 53（与 delta 内计数一致）；保留侧 persistent-topic 92 + topic-watch 17 = 109；53 + 109 = 162 = 原 4 个 spec 总数。REMOVED 侧 7 + 8 = 15 标题与各自 main spec 标题集合**完全相等**（脚本断言通过）。
3. **openspec 校验**：`openspec validate --specs` → `Totals: 152 passed, 0 failed`；`openspec validate merge-topic-specs` → valid（仅继承性 WARNING：5 条「Requirement text is very long >500 chars」，与上一族同口径，不阻断归档门禁）。
4. **归档机制（读 openspec 2.1.1 源码核实）**：`dist/core/specs-apply.js:248-263` 明确「delta Purpose 只为不存在的 spec 播种；已存在 spec 的 Purpose 忽略并告警」→ design D4 第 1 条与 tasks 8.3a/8.3b 的「手改 Purpose」**成立且必要**。
5. **tasks 4.3 的必要性（读 openspec 源码核实）**：`dist/core/specs-apply.js:642-700` 的「合并无法命名的内容」审计把 `## Requirements` 之前、既非标题又非 Purpose 的行计为 leftover；而 `dist/core/archive.js:1508-1560` 规定 `retire_capabilities: true` 时 leftover 非空即拒绝退役并报错「the spec holds content the merge cannot safely account for」。被退役的两个 spec 头部第一行正是 `<!-- domain: ... -->`（marker）→ **摘 marker 是退役的前置条件**，且两个 spec 除 marker/标题/Purpose 外无其他 `###` 杂散内容（`grep '^#\{1,4\}'` 复核），故 4.3 是必要且充分的。
6. **两族 marker 与域无丢失**：persistent-topic 保留 `topic-graph, semantic-board, daily-report`、topic-watch 保留 `semantic-board, daily-report, topic-graph`，各 3 域（词表上限 3，J 段合规）；被退役两个 spec 的域集合分别为 {topic-graph, semantic-board}、{daily-report, topic-graph, semantic-board}，均已被两族的 marker 覆盖，无域丢失。marker 位于 `parts.before`，归档重建时逐字保留（8.3c 只需核验，不需改写）。
7. **分类序号可反推且与题面吻合**：由 `merge-spec-family.sh` 的生成序（族 → cap → Requirement）反推 `spec-classification.txt` 的序号映射：1-19 = persistent-topic 既有 19 条、20-26 = key-event-markers 7 条、27-31 = topic-watch 既有 5 条、32-39 = watch-materialized-topic 8 条；据此 #8=历史数据回刷、#12=回刷重建持久化话题（单成员不建）、#14=PersistentTopic 质心表示、#25=条目标记交互、#33=一句话轨辅助标签检索、#39=物化板块当日标题——与题面描述逐条吻合，分类文件本身机器可读（脚本 `--kind-file` 格式 `<序号> <business|impl>`）但不含标题映射。
8. **归族耦合证据（支持 4→2）**：① `topic-watch` 的「关注标记实体模型」把 `type` 四值（label/keyword/keyword_topic/sentence_topic）与 `embedding_cache` 写在**同一张表** `board_topic_watches`；② 「关注标记管理 API」的 DELETE 分支已承载 sentence_topic 的「显式确认 + 归档专属话题」联动；③ `persistent-topic` 的「话题生命线 API」正文直接返回「可选已确认关键节点列表」——标记族读端本就内嵌在话题族 API 里。
9. **红线 3 未被合并违反**：「关注标记与持久话题隔离」原文对全部 type 约束「命中提示记录」通道（命中 SHALL NOT 改 `persistent_topic_id`、SHALL NOT 触发 `consecutive_hits`/生命周期），并明写「keyword_topic / sentence_topic 物化轨 SHALL NOT 产生命中提示记录」「sentence_topic 专属话题的生命周期推进 SHALL 仅由物化 section 的归属驱动」——即隔离限定在「命中提示」通道，物化轨走的是归属通道（与 flow 红线 7「sentence_topic 专属话题属 manual 主权家族」一致）。delta 的 `## Purpose` 也保持了「提示轨…不改任何 section 归属与话题生命周期」的限定词，合并后不会把「隔离」误读成覆盖物化轨。
10. **delta Purpose 与脚本 PURPOSE 字典一致（可机械抄回）**：脚本 `git diff` 显示本轮新增 `persistent-topic` / `topic-watch` 两条 PURPOSE 文案与 tasks 2.1「补了 PURPOSE 缺失即报错」一致；程序化比对「delta 的 `## Purpose` 正文 == 脚本 PURPOSE 字典值」→ 两族均 `True`（212 / 200 字符），故 8.3a/8.3b 可直接抄 delta 原文（建议在 tasks 里写明「抄 delta 的 `## Purpose` 整段」，避免主线程另行改写措辞）。
11. **门禁现状（实测）**：`bash scripts/harness/archive-readiness.sh openspec/changes/merge-topic-specs` → ①doc-impact ✓ ②check-standards ✓ ③check-tasks-tail ✓ ④review-trace ✗（本报告落盘后消）⑤UI 验收痕迹对账 ✗（R-1）⑥约束文档读取对账 ✓ ⑦human-feedback ✓。`check-standards.sh` J 段对「缺 domain marker」只 WARN 不阻断，故 tasks 4.3 先摘 marker 不会撞门禁。
12. **未触碰项**：`git status --porcelain` 显示工作树中的 `openspec/specs/{board-article-api,board-editing-ui,board-management-api,board-upgrade-expand,upgrade-candidate-time-window}/spec.md` 删除与 `scripts/harness/merge-spec-family.sh` 修改**均为本 change 之外的既存脏状态**（前三族合并的遗留），`related-events` / `data-enrichment` 两个 spec 在工作树中未被改动 ✓。
