# 制品自审（2026-11-10）

按用户“主线程直接建立、不再派子线程”要求执行；只审查重写制品与可定位的既有入口，不使用git，不审查未实现代码。本记录不代表实现验收或独立代码review。

### R-1 2026-11-10【High】旧delta残留会继续要求实体接线与阶段归并
- **状态**：已修复
- 处置：旧event-story/story-heat-evidence delta原文留legacy/previous-plan，移出当前specs，仅保留新增related-events；主spec未动。旧探索页头加失效方向警示。

### R-2 2026-11-10【Medium】收敛故事后任务与UI映射仍指向旧编号
- **状态**：已修复
- 处置：design/UI引用改为新10项任务和S1/S2/S3，不保留S4等不存在故事。

### R-3 2026-11-10【Medium】新测试未创建不应冒充已有覆盖
- **状态**：已采纳
- 处置：tasks明确映射为计划路径，scenario-trace存在性门禁尚不能绿；不生成空壳测试，不把自动用例改写成人工豁免。归档与实现验收仍须建立并跑通真实测试。

### R-4 2026-11-10【Medium】显式刷新和旧实验副作用必须如实披露
- **状态**：已修复
- 处置：design说明初版GET只读、索引需显式刷新及partial/not_ready；legacy记录两次真重算后restore、向量/审计/序列未撤回。未声称完全无痕回滚或新事件自动即时索引。

实现后须继续审查实际代码与真实用户链路；本轮不勾选任何新实现任务。

# 实现后代码审查（2026-11-11，tasks 4.1）

独立审查线程执行（brief：/tmp/review-brief-related-events.md）。范围：tasks 1.1–3.1/5.1–5.2/6.1/7.1 的实现 diff 对照 proposal/design(D1–D9)/specs/test-cases/ui-design，R-1..R-4 处置兑现抽查，只读纪律（未跑全量测试/未碰真库写/未跑 git）。结论：**通过**，4 条 Low 意见建账处置，无阻断项。

核对通过项（摘要）：三故事 S1/S2/S3 闭环（10 Scenario 全有真实测试落点，scenario 映射表与文件一致）；无私货（grep+读码：related 包无 subject_key/实体 ID/指纹/blacklist/传递扩展/跨版块推断，DB 写入仅 `related_event_embeddings` upsert 与 `related_event_index_generation` 设置，`topic_tag_relations`/`board_stories`/thread/aux 零写入且有断言）；D3/D4/D5 落实（独立派生缓存表三轴失效；GET 纯读无 airouter 引用；窗口含两端/排自身/mismatch/merged/非 event；阈值后 score 降序+id 升序取 10；ready/partial/not_ready 口径与覆盖数一致；refresh 批≤32/30s/串行/逐项校验/失败不覆盖/空间不符拒写）；ai-logging 红线（operation=`tagmanagement.related_event.embedding` 非空+capability embedding+已在 ai-logging.md 清单登记，不在 embedding 缓存白名单）；两处自报修复核实（PUT matching-config 先全量校验再单事务、NaN/Inf 显式拒绝、多字段整体失败不落库有测试；Esc 经 `[aria-modal]` 探测隔离、组件测试双向断言）；6.1 文档与代码一致（flow 红线 34 新增未删旧约束、api/database/configuration 口径与实现相符，board→tag→article 来源口径与既有 getBoardArticles 同构）；S3 产物核对（305,371=C(782,2)、达标 5,919、样本 50+10、`cache_writes=0`、模型/表示/窗口字段齐）。

抽查命令（白名单）：`go test ./cmd/related-events` → ok；`go test ./internal/tagmanagement/service/related -run 'TestRelatedEventsDoNotPropagateOrMutateIdentity|TestRelatedEventsWindowThresholdAndOrdering|TestBuildRepresentationIsStableAndAuxOptional' -v` → 3 PASS；`pnpm test:unit MatchingConfigDialog.relatedEvents.test.ts --maxWorkers=2` → 10 passed；`pnpm test:unit ArchiveMatchPopover.relatedEvents.test.ts --maxWorkers=2` → 12 passed。

### R-5 2026-11-11【Low】experiment CLI --threshold 帮助文本与实现不符（声称读 ai_settings，实际固定回退 0.82）
- **状态**：已修复
- 处置（2026-11-11 处置轮）：`cmd/related-events` 解析期保留「未指定」哨兵，`runExperiment` 经 `resolveThreshold(opts, related.LoadThreshold(db))` 读当前生效值再跑；帮助文本改为「default: current ai_settings related_event_similarity_threshold, else 0.86」；显式负数（非哨兵）显式拒绝；新增 `TestResolveThresholdUsesCurrentSettingOnlyWhenUnset` 覆盖显式/缺省两分支。
- 定位：backend-go/cmd/related-events/main.go:47（flag 描述 "default: ai_settings value, else 0.82"）与 main.go:107-109（`opts.Threshold < 0 → related.DefaultThreshold`，从未读 ai_settings）。
- 影响：S3 实测时 ai_settings 即默认 0.82，记录结果不受影响；但用户在线调过阈值后再跑 `--experiment` 不带 `--threshold`，会用 0.82 而非当前生效值，帮助文本误导操作者。
- 处置建议：run 阶段（已有 db）改为 `related.LoadThreshold(db)` 作为缺省，或把帮助文本改为 "default: 0.82" 并在 configuration.md 同步措辞。

### R-6 2026-11-11【Low】7.1 执行记录组件测试计数誊写不符（21 vs 实际 22）
- **状态**：已修复
- 处置（2026-11-11 处置轮）：tasks.md §7.1 该行改为 `Tests 22 passed (22)` 并注明 22 = MatchingConfigDialog 10 + ArchiveMatchPopover 12。
- 定位：openspec/changes/enable-story-cross-day-continuation/tasks.md §7.1 执行记录「Tests 21 passed (21)」。
- 复核：本轮实测两文件 10+12=22 例全绿，与 ui-design.md「组件测试 22 例（10+12）」一致；其自身「+既有 ArchiveMatchPopover.test.ts → 51 passed」反推亦为 22（51-29）。21 为誊写笔误，不构成勾选虚报。
- 处置建议：把该行改为 22（或注明「以 4.1 复核 22 例为准」）。

### R-7 2026-11-11【Low】ArchiveMatchPopover 相关事件模板段存在永不渲染的重复副本
- **状态**：已修复
- 处置（2026-11-11 处置轮）：删除 article 模式分支内 `v-if="isEventTagCard"` 的 59 行死模板（真身保留在 tag 分支），`ArchiveMatchPopover.relatedEvents.test.ts` 12 例 + 既有 `ArchiveMatchPopover.test.ts` 29 例回归绿。
- 定位：front/app/features/tags/components/console/ArchiveMatchPopover.vue:748-801（article 模式分支内的 `<section v-if="isEventTagCard">`；`isEventTagCard` 要求 `mode==='tag'`，在 article 分支恒 false）；真实渲染副本在 :629（tag 分支）。
- 影响：纯死标记，行为无误（article 模式零请求已被组件测试断言）；但两份文案/状态结构需同步维护，将来改口径易漏改一份。
- 处置建议：删除 article 分支的死代码段，或收敛为共享子组件/加注释声明唯一真身。

### R-8 2026-11-11【Low】阈值边界含 1e-6 容差（score ≥ threshold−ε 计入）
- **状态**：已采纳（留痕观察，无需改动）
- 定位：backend-go/internal/tagmanagement/service/related/representation.go:73-78（boundaryEpsilon）+ query.go 阈值过滤行。
- 判定：注释已说明动机（pgvector float32 存储取整会让「恰好等于阈值」的构造对掉到阈值下 ~1e-7），容差远小于 UI 0.01 步进，不改变任何用户可见决策；spec「cosine>=阈值」在工程容差内成立，白盒测试（恰好 0.82 对）通过。记观察备查。

### 4.1 审查尾记录

- 2026-11-11 处置轮（审查后）：R-5/R-6/R-7 已修复并复跑定点验证；同轮按用户拍板把默认阈值 0.82 → 0.86（全链路代码/前端/制品/reference 同步，历史 S3 实测记录原样留档；存量 `ai_settings` 经 `PUT /api/semantic-boards/matching-config` 正常通道写为 0.86）。R-8 原文未动。

- review-trace 实际命令与结果（2026-11-11 实跑，最终一次，exit=0）：`bash scripts/harness/review-trace.sh openspec/changes/enable-story-cross-day-continuation` → `✓ 通过：review.md 建账 8 条（R-<N>【等级】+ 状态行格式合法）`（R-1..R-4 原文未动；R-5..R-7 状态「待处理」、R-8「已采纳（留痕观察）」，均用脚本契约的封闭状态词表。首轮跑得 4 条系本线程初稿误用词表外状态词「待处置/留痕观察」，按 openspec/specs/review-trace-gate 契约改词后复跑得 8 条。Low 级不阻断，处置责任在后续维护）。
