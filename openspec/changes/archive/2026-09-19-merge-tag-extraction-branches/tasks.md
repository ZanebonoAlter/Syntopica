# Tasks: merge-tag-extraction-branches

> 实现范围见 design.md（D1-D5）；行为契约见 `specs/auxiliary-label/spec.md`（5 个 Scenario）+ `specs/aggregate-tagging/spec.md`（联动表述）。用例故事见 test-cases.md。

## 1. Prompt 与 schema（纯函数，先行）

- [x] 1.1 实现 `buildMergedExtractionPrompt()`：合并 `buildEventPersonPrompt`/`buildKeywordPrompt` 为单份系统提示（共同规则去重、辅助标签规则归 event/person 段、description 规则归 keyword 段，≤1500 字），更新 `TestBuildExtractionSystemPromptLimitsAndOrdersTags` / `TestBuildExtractionSystemPromptIncludesAuxiliaryLabelRules` 断言并新增长度上限断言
- [x] 1.2 实现双数组 `JSONSchema`（`event_person_tags` + `keyword_tags` 顶层），更新 `TestTagExtractionSchemaIncludesAuxiliaryLabelObjects` 覆盖双数组结构
- [x] 1.3 实现双数组响应解析：顶层拆组后复用既有字段级校验（event/person 数组禁 keyword category、keyword 数组必须有 description 无 auxiliary_labels），新增单测覆盖：合法双数组 / 双数组包裹在 `{"tags":…}` 兼容形态 / event 数组元素 category 串扰 / keyword 元素带 auxiliary_labels 四种输入

## 2. ExtractTags 重构（单次调用）

- [x] 2.1 `ExtractTags` 改为单次 `Router.Chat`（operation=tagmanagement.extractor_enhanced、meta.operation=tag_extraction_merged、maxRetries=3 保持），删除双 goroutine 分支调度；`fakeTagChatRouter` 适配单 operation 队列
- [x] 2.2 数组级部分产出路径：event/person 数组空 → 记 branchErrors 保留 keyword 产出（改写 `TestExtractTagsKeepsKeywordBranchWhenEventPersonFails`）；keyword 数组空 → `heuristicKeywordCandidates` 展示兜底（改写 `TestExtractTagsFallsBackToHeuristicKeywordWhenKeywordBranchFails`）
- [x] 2.3 整体失败路径：重试耗尽 → `extractWithHeuristic`（沿用），新增断言 source=heuristic 且错误信息保留（spec Scenario: 单次调用整体失败回退 heuristic）
- [x] 2.4 调用数断言：fake router 记录 Chat 次数，成功路径恰 1 次（spec Scenario: mono 文章提取调用数为 1 的函数级落点）
- [x] 2.5 双数组间去重走既有 `mergeExtractedTags` 不动，`TestMergeExtractedTagsLimitsAndDedupesByCategoryPriority` / `TestMergeExtractedTagsKeepsHigherPriorityDuplicate` 保持原绿零改动（回归护栏）

## 3. 测试

- [x] T1 `bash scripts/harness/change-scope.sh` 判定影响包 → 输出 `internal/tagmanagement`（按映射只跑该包测试）
- [x] T2 `cd backend-go && go test ./internal/tagmanagement/...` → 全绿（test-cases.md 主链路表逐 Scenario 落点全绿 + 既有测试按「继承与调整」表处置完毕）

## 4. 文档

<!-- doc-impact: flow -->
<!-- §12.2：flow/semantic-board.md 若有提取调用结构描述（双分支/两次调用）随本 change 同步为单次调用双数组 -->

- [x] D1 `docs/reference/flow/semantic-board.md`：提取调用结构描述同步（双分支 → 单次调用双数组），「变更溯源」表归档时补行 —— grep 无命中，无同步对象，N/A（溯源补行随归档阶段）
- [x] D2 `docs/reference/flow/ai-summary.md` 变更溯源表补行（打标消耗减半影响 airouter topic_tagging 吞吐画像）——已补 ai-summary.md 溯源行

## 5. 验证

- [x] V1 `openspec validate merge-tag-extraction-branches` → 输出 valid
- [x] V2 `cd backend-go && go vet ./... && go build ./...` → 无告警、构建成功
- [x] V3 `golangci-lint run ./internal/tagmanagement/...` → 无新增告警
- [x] V4 Scenario→测试映射对账：`bash scripts/harness/scenario-trace.sh openspec/changes/merge-tag-extraction-branches` 退出码 0，映射表见 test-cases.md（8/8 齐全）
- [x] V5 部署后观测口径固化（观测属上线次日夜运维动作，不阻断归档）：① `ai_call_logs` 当日 `tagmanagement.extractor_enhanced` 调用数 ≈ 打标文章数（改造前 ≈ 2 倍）；② 失败率与 attempt>1 占比不高于改造前基线；③ 固定 feed 集（含少数派）标签抽查质量持平；异常时回滚点为 `extractMergedCandidates`（revert 6a82b6cd 即回双分支）

| Scenario | 测试文件 |
| --- | --- |
| event/person 数组为空但 keyword 有产出 | backend-go/internal/tagmanagement/service/core/extractor_test.go |
| keyword 数组为空但 event/person 有产出 | backend-go/internal/tagmanagement/service/core/extractor_test.go |
| 双数组间去重 | backend-go/internal/tagmanagement/service/core/extractor_test.go |
| 单次调用整体失败回退 heuristic | backend-go/internal/tagmanagement/service/core/extractor_test.go |
| mono 文章提取调用数为 1 | backend-go/internal/tagmanagement/service/core/extractor_test.go |
| 同名标签跨片去重 | backend-go/internal/tagmanagement/service/core/article_tagger_aggregate_test.go |
| 文章级上限截断 | backend-go/internal/tagmanagement/service/core/article_tagger_aggregate_test.go |
| 全片失败回落 mono 路径 | backend-go/internal/tagmanagement/service/core/article_tagger_aggregate_test.go |
