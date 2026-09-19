
## 双分支合并的落点与旧测试资产

双分支合并的实现落点与既有资产（2026-09-19）：

1. **改造核心**：`extractor_enhanced.go` `ExtractTags`（:44，双 goroutine 并行分支）+ `extractBranchCandidates`（:121，每分支 maxRetries=3/maxTokens 2048/temp 0.2）→ 改单次 Chat。两份系统提示 `buildEventPersonPrompt`（:292）/`buildKeywordPrompt`（:334）合并；user prompt `buildExtractionUserPrompt`（:369）不变。`mergeExtractedTags`/`resolveCandidate`/`heuristicKeywordCandidates`/`extractWithHeuristic` 兜底纯函数全部复用零改动。

2. **旧测试资产**（extractor_test.go）：`TestExtractTagsKeepsKeywordBranchWhenEventPersonFails`（:302）与 `TestExtractTagsFallsBackToHeuristicKeywordWhenKeywordBranchFails`（:324）需改写为数组级缺失语义（fake router `fakeTagChatRouter` :365 按 operation 分队列，需适配单 operation）；`TestMergeExtractedTags*`（:277/:295）与 parse 族（:61-:270）大多保留；`TestBuildExtractionSystemPrompt*`（:211/:218）与 schema 测试（:233）断言更新。

3. **spec 侧注意**：auxiliary-label 用 REMOVED（旧需求"Tag 提取拆分为…双分支调用"）+ ADDED（新需求"单次调用输出…双数组"）组合——纯 RENAMED+MODIFIED 组合 archive 会报 not found；aggregate-tagging 的"跨片去重与文章级上限"需求含两处"双分支"字样已联动 MODIFIED。

4. **既有漂移留档（勿在本 change 修）**：spec"合并总数≤5、keyword≤3" vs 代码 `article_tagger.go:14 maxArticleTags=6`。

<!-- pinned 2026-09-19T03:44:34Z -->

## 双分支合并改动边界：符号引用面封闭于 extractor_enhanced.go+extractor_test.go

改动边界确认（2026-09-19 实现开工时 grep）：buildEventPersonPrompt/buildKeywordPrompt/eventPersonExtractionSchema/keywordExtractionSchema/parseEventPersonTags/parseKeywordTags/extractBranchCandidates/extractEventPersonCandidates/extractKeywordCandidates/extractionBranchResult 全部只出现在 backend-go/internal/tagmanagement/service/core/extractor_enhanced.go（生产）与 extractor_test.go（测试），无其他包引用——两个文件即可完成改造，aggregate 路径零改动成立。fakeTagChatRouter（extractor_test.go:365）按 metadata["operation"] 键分队列，单 operation 下 map 结构无需改，仅调用点改为 "tag_extraction_merged"。TestParseKeywordTagsRequiresDescriptionAndIgnoresAuxiliaryLabels 语义：keyword 解析对 auxiliary_labels 是静默忽略（非报错），双数组解析复用同语义。

**引用**：backend-go/internal/tagmanagement/service/core/extractor_enhanced.go、backend-go/internal/tagmanagement/service/core/extractor_test.go

<!-- pinned 2026-09-19T03:54:55Z -->
