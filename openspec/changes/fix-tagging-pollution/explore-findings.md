
## 审查发现：heuristic 降级三触发点与清理 SQL 缺口

审查制品时核实的代码事实（2026-09-25）：

**① heuristic 降级触发点全景（共 3 处，design D1/D2 只覆盖 2 处）**：
- extractor_enhanced.go:52 `extractWithHeuristic(input, err)`——extractMergedCandidates 返回 err 时触发，符合新 Requirement「仅调用失败」，保留；
- extractor_enhanced.go:67 `extractWithHeuristic(input, errors.New("no candidates extracted"))`——**candidates==0 时触发，D1/D2/tasks 均未覆盖**。D1 移除 keyword 回填后，「两数组均空」场景 candidates 必空 → 走此分支返回非空 heuristic 标签（err=nil）→ article_tagger 收到非零 Tags 照常写库 source=heuristic。链路二在此场景断不了根，spec Requirement 1 Scenario 2（零候选不产生规则词候选）会红。需在 tasks 2.1 扩展：该分支改为返回零候选结果（Tags=[], Source="llm"）；
- article_tagger.go:143（design 写 142）`err != nil || len(result.Tags) == 0` → legacyExtractTopics——D2 收窄对象。

**② 文章标签写入路径共 4 条，persistArticleTags 只覆盖 3 条**：
mono / aggregate（回落 mono）/ heuristic 兜底均经 article_tagger.go:164 persistArticleTags（filterGenericLabels 接入点 ✓）；
**第 4 条：reuseTagsFromSiblingArticle（article_tagger.go:263）**——跨 feed 同 link 文章直接 createArticleTopicTagLink 复制 sibling 关联行（source=reuse），不经 persistArticleTags，黑名单拦截不到。D5 清理后 sibling 源头已净属数据层兜底，非拦截层覆盖；spec「所有写入路径 SHALL 拦截」表述与拦截点不匹配，需补 reuse 过滤或显式划除。

**③ D5 清理 SQL 词表与 proposal/tasks 预期数矛盾**：proposal/test-cases/tasks 4.1 的 llm 泛词清理预期含「Coding」约 90 行（合计≈804），但 D5 第二条 DELETE 词表（新闻/论坛/要闻/快讯/文章/内容/技术/发展）删不到 Coding（tag_id 27，D3 明确不进黑名单）。执行时 4.1 的「偏差超 ±5% 停下核对」会卡住（804 vs ~714，偏差 11%）。需 D5 补一条按 label='Coding' 或 topic_tag_id=27 删 llm 行的 DELETE，或把预期数改 ~714 并在 proposal 划除 Coding。

**④ 其他**：heuristicKeywordCandidates 确实唯一调用方为回填处（定义 extractor_enhanced.go:149）；aggregate 路径无独立 heuristic 降级；article_tagger.go:126-128 注释「so aggregate articles never end up with zero tags」在 D2 后语义失效需更新；branchErrors 中 "keyword extraction failed: empty keyword array" 措辞在空=合法结论后应调整；testcontainer PG 基建在 tagmanagement 包内已存在（edge_gc_test.go 等）；行号小偏差：design 引 :61-68 实为 59-61、:142 实为 143。

<!-- pinned 2026-09-25T12:43:43Z -->
