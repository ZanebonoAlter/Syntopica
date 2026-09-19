# Tasks: long-form-sampled-tagging

> 实现范围见 design.md（采样收敛在 `buildArticleSummary`，新写 `sampling_splitter.go`）；行为契约见 `specs/tagging-domain/spec.md` 六个 Scenario。

## 1. 采样切分器（新文件，不依赖现有 article_tagger）

- [x] 1.1 实现 `stripNoise`：删除 `![alt](url)` 图片语法、`[text](url)` → `text`，单测覆盖多图/多链接/嵌套/无链接正文四种输入，断言 URL 不残留且文字保留
- [x] 1.2 实现 `splitByHeadings`：认 `#`~`######` 标题行切段、无标题时按空行聚合，单测覆盖 h2 文集/仅 h3/无标题叙事/标题后紧跟标题四种输入，断言段数与标题归属
- [x] 1.3 实现 `truncateAtSentenceBoundary(text, n)`：句界回退窗口 50 runes（句末标点 `。！？\n`），窗口内无句界硬切，单测覆盖句中切点/正好句末/无标点列表文本三种
- [x] 1.4 实现 `sampleSections(sections, budget=4000)`：标题全保 + 均分预算（保底 120/段）+ 段数超限退化为"其他栏目：标题清单"行，单测覆盖 15 段周刊/30 段超限/8 段常规三种，断言输出总长 ≤ 预算 × 1.05 且全部标题在场
- [x] 1.5 实现 `sampleHeadMidTail(body, budget=4000)`：头 2000/中 1000/尾 1000 + `……` 分隔，单测断言三段来源区间互不重叠且带省略标记

## 2. 接入 buildArticleSummary 与真实样本验证

- [x] 2.1 重构 `article_tagger.go` 的 `buildArticleSummary`：剥噪声 → ≤4000 原样返回 → 按段数分流 sampleSections / sampleHeadMidTail，`maxSummaryRunesForTagging` 常量保留为预算语义
- [x] 2.2 真实样本端到端单测（数据固化为测试夹具）：少数派周刊（`##` 多栏目）、无标题叙事长文、短 RSS 摘要三类各一篇，断言分别命中文集采样/头中尾/原样通过三条路径，输出符合 spec Scenario 1/2/3
- [x] 2.3 回归断言：短文（≤4000）输出与剥噪声后正文逐字节一致、不含省略标记（spec Scenario: 短输入原样通过）
- [x] 2.4 存量文章路径一致性测试：content_form 为空的文章打标输入构造走同一采样函数（spec Scenario: 存量文章走 mono 路径）

## 3. 测试

- [x] T1 `bash scripts/harness/change-scope.sh` 判定影响包 → 输出 `internal/tagmanagement`（按映射只跑该包测试）
- [x] T2 `cd backend-go && go test ./internal/tagmanagement/...` → 全绿（新增单测 + 既有 tagger 测试零回归）

## 4. 文档

<!-- doc-impact: semantic-board -->
<!-- §12.2：flow/semantic-board.md「代码入口」节的打标输入构造描述随本 change 更新（4000 掐头 → 分段采样） -->

- [x] D1 `docs/reference/flow/semantic-board.md` 业务约束/代码入口节：补"mono 打标输入为预算 4000 runes 的分段采样（文集型均匀采样/叙事型头中尾/先剥 markdown 噪声）"一条
- [ ] D2 本 change 归档时按 §12 在 flow 文档「变更溯源」表补行（日期/摘要/归档位置）

## 5. 验证

- [x] V1 `openspec validate long-form-sampled-tagging` → 输出 valid
- [x] V2 `cd backend-go && go vet ./... && go build ./...` → 无告警、构建成功
- [x] V3 `golangci-lint run ./internal/tagmanagement/...` → 无新增告警
- [x] V4 Scenario→测试映射对账：6 个 delta Scenario 逐条注明落点（见下表），`bash scripts/harness/scenario-trace.sh openspec/changes/long-form-sampled-tagging` 退出码 0
- [ ] V5 部署后观测（人工，上线次日夜）：`ai_call_logs` 中 `tagmanagement.extractor_enhanced` 当日 prompt 长度 p90 ≤ 8k 字符（改造前基线 ~10.5k）；少数派/阮一峰 feed 当日新文章标签覆盖栏目数 ≥ 改造前

| Scenario | 测试文件 |
| --- | --- |
| 短输入原样通过 | backend-go/internal/tagmanagement/service/core/sampling_splitter_test.go |
| 单主题长摘要截断 | backend-go/internal/tagmanagement/service/core/sampling_splitter_test.go |
| 叙事型长文头中尾采样 | backend-go/internal/tagmanagement/service/core/sampling_path_test.go |
| 段数过多退化为标题清单 | backend-go/internal/tagmanagement/service/core/sampling_splitter_test.go |
| 图片与链接噪声不占预算 | backend-go/internal/tagmanagement/service/core/sampling_splitter_test.go |
| 存量文章走 mono 路径 | backend-go/internal/tagmanagement/service/core/sampling_path_test.go |
