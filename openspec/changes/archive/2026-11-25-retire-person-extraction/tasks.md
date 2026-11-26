## 1. prompt 收敛为 event + keyword

- [x] 1.1 **两条路径同步**：`buildMergedExtractionPrompt()`（单篇）与 `buildSectionExtractionPrompt()`（聚合）都去掉 person 提取要求——章节标题「【event/person 数组】」→「【event 数组】」或首句「三类标签」→「两类标签」，删除 person 定义行与示例（`Sam Altman`/`李飞飞`）、`每个 event/person 标签` 措辞、`person 可留空` 尾巴、`person/keyword 不输出` 中的 person，以及正反例中的 person 提法；`event_person_tags` 字段名与 event 的全部约束原样保留。验证：两条 prompt 函数体内 grep `person` 零命中（merged 仅剩正例 JSON 里的 `event_person_tags` 字段名）。
- [x] 1.2 同步 `extractMergedCandidates` / `buildMergedExtractionPrompt` / `buildSectionExtractionPrompt` 的文档注释（不再写双类/三类），并说明保留 `event_person_tags` 字段名是为兼容；两个 JSON Schema 的 `category` / `description` / `auxiliary_labels` 描述字符串同步（它们会随结构化输出请求发给模型）。验证：`cd backend-go && go build ./...` 退出码 0。

## 2. 单测

- [x] 2.1 新增 `TestMergedExtractionPromptExcludesPerson`（断言 merged prompt 不含人物提取要求）；`TestSectionExtractionPromptIncludesFusedRules` 的 `require.Contains(prompt, "person（人物）")` 反转为 `require.NotContains`；两条 off-prompt golden 首句同步。验证：`cd backend-go && go test ./internal/tagmanagement/service/core -run 'TestMergedExtractionPromptExcludesPerson|TestSectionExtractionPromptIncludesFusedRules|TestSectionExtraction_OffPromptAndSchemaUnchanged|TestEnhancedExtraction_OffPromptAndSchemaUnchanged' -count=1` PASS。
- [x] 2.2 既有解析测试保持绿（含 mock 返回 person 的用例，钉住 design D2 兼容性）。验证：`cd backend-go && go test -p 1 -count=1 ./internal/tagmanagement/service/core` 全 PASS。

## 3. 文档
<!-- doc-impact: flow -->

- [x] 3.1 `docs/reference/flow/content-enrichment.md`「业务约束与不变量」补一条红线句：打标提取类别为 event + keyword，person 不再作为提取目标，解析层仍兼容偶发返回。验证：`grep -n 'person' docs/reference/flow/content-enrichment.md` 命中且位于「业务约束与不变量」节内。
- [x] 3.2 归档前跑 `bash scripts/harness/doc-impact.sh verify openspec/changes/retire-person-extraction`，期望「通过（声明: flow）」。

## 4. 测试

- `cd backend-go && go test -p 1 -count=1 ./internal/tagmanagement/service/core`
- `cd backend-go && go vet ./internal/tagmanagement/service/core/...`

## 5. 验证

- [x] 5.1 静态检查：`cd backend-go && golangci-lint run ./internal/tagmanagement/service/core/... && go vet ./internal/tagmanagement/service/core/...`，期望 0 issues。
- [x] 5.2 构建：`cd backend-go && go build ./...`，期望退出码 0。
- [x] 5.3 prompt 实物核对：`cd backend-go && grep -A3 '【event 数组】' internal/tagmanagement/service/core/extractor_enhanced.go`，期望只见 event 规则、无 person 定义。

## 6. Review

- [x] 6.1 派子线程聚焦 review（审查制品 + diff），意见按 High/Medium/Low 处置后 review.md 建账（主线程执行）。
