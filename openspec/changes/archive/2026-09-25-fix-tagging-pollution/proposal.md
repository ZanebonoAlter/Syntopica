<!-- complexity: simple -->
<!-- ui-impact: none -->
<!-- constraint-domains: semantic-board -->

## Why

打标签链路存在两类「把空结果当失败」的代码路径（extractor 提取层与 tagger 编排层共三个代码点），导致规则匹配产生的泛词标签（新闻/论坛/Coding…）以 `llm` 或 `heuristic` 来源污染文章标签：LLM 合法返回空 keyword 数组时被 `heuristicKeywordCandidates` 用分类名/规则词回填（`extractor_enhanced.go:59-61`，已产生 624 行 llm 来源「新闻」、约 90 行「Coding」，全部与文章内容无关）；LLM 整体空结果时降级 heuristic 兜底（extractor 内部零候选降级 + `article_tagger.go:143` 零标签降级两处，存量 293 篇 / 484 行 heuristic 垃圾）。切换本地 Qwen3.5-4B 主力后链路已稳定（当日 373/375 成功），残余污染全部来自这两处代码行为，现在修复成本最低。

## What Changes

- **移除空 keyword 数组时的规则回填**：`extractor_enhanced.go` 中 `heuristicKeywordCandidates` 不再在 LLM 返回空 keyword_tags 时注入分类名/规则词；空 keyword 是「宁缺毋滥」提示词下的合法结论，原样保留
- **收紧 heuristic 兜底触发条件**：extractor 提取层与 tagger 编排层各一处——编排层 `article_tagger.go` 仅 `err != nil`（transport/HTTP 失败）触发 heuristic 兜底，`len(result.Tags) == 0` 不再触发；提取层 `extractor_enhanced.go` 零候选时原样返回空结果（Source=llm）、不再内部降级 heuristic。文章保持无标签状态等待后续重打
- **新增泛词标签黑名单**：在持久化入口（`persistArticleTags`）拦截「新闻/论坛/要闻/快讯/文章/内容/技术/发展」等泛词，无论来源（llm/heuristic/回填）一律丢弃；跨 feed 标签复用（reuse）直接复制 sibling 关联行、不经持久化入口，其查询同滤黑名单
- **存量数据清理**：删除 484 行 `source='heuristic'` 标签关联 + llm 来源的泛词标签关联（约 714 行「新闻」+约 90 行「Coding」+少量「论坛」），受影响文章重新进入打标队列由本地主力重打
- **排查任务**：定位凤凰网文章 llm 来源「Coding」标签的确切写入路径（模型响应已证实不含该标签，疑似文章初版内容被抽取后 content 被替换导致旧标签残留）
- 纯配置项（qwen provider timeout_seconds 3000→300、topic_tagging 绑定 mimo P2 兜底）走管理界面操作，不在本 change 代码范围内，仅记录于 tasks 验证节

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `tagging-domain`: 抽取编排行为变更——(1) LLM 空 keyword 数组不再触发规则词回填，空结果是合法结论；(2) heuristic 兜底仅限调用失败（err != nil），空结果文章保持无标签；(3) 泛词黑名单在候选解析层拦截，任何来源的泛词标签不得入库

## Impact

- 后端：`backend-go/internal/tagmanagement/service/core/`（extractor_enhanced.go、article_tagger.go，可能触及 extractor_heuristic.go 的函数保留/删除）
- 数据：`article_topic_tags` 删除约 1288 行污染关联（含备份与 count 留底流程）；受影响文章（约 300+ 篇）由 tag 队列自动重打
- 行为变化：内容稀薄的快讯类文章将出现「无标签」状态（此前被泛词假标签填充）；语义版块辅助标签池不再被「新闻」类泛词稀释，匹配质量预期提升
- 无 API/前端/依赖变化
