# Tasks: fix-tagging-pollution

> 按开发执行规范 §2 用例先行：test-cases.md 与单测先于实现代码。

## 1. 测试先行

- [x] 1.1 在 `extractor_enhanced_test.go` 新增用例：LLM 返回 event + 空 keyword 数组 → 结果仅 event、来源 llm、无分类词候选（对应 spec「快讯类文章 keyword 为空时保持 LLM 原样」）；以及两数组均空 → 返回零候选（Tags=[]、Source=llm）、不触发 heuristic 降级（对应 spec「空摘要文章两个数组均为空」，当前 :67 降级会红）；先跑确认红
- [x] 1.2 在 `article_tagger_test.go` 新增用例：融合提取成功但零候选 → `tagArticle` 不写入任何 `article_topic_tags` 行（testcontainer PG）；先跑确认红
- [x] 1.3 在 `article_tagger_test.go` 新增用例：泛词黑名单拦截（构造 label=「新闻」候选 → persist 零行）与不误伤（「GLM Coding Plan」正常入库）；reuse 复制路径过滤（sibling 带「新闻」关联 → 新文章不被复制该行，其余照常，对应 spec「跨 feed 复用不复制泛词标签」）；先跑确认红

## 2. 代码实现

- [x] 2.1 `extractor_enhanced.go`：移除空 keyword 数组时的 `heuristicKeywordCandidates` 回填分支（:59-61）及函数体（唯一调用方）；同时移除 `len(candidates) == 0` 的 `extractWithHeuristic` 降级（:67），改为原样返回零候选结果（Tags=[]、Source="llm"、Errors 照记），仅保留 :52 调用失败降级；`branchErrors` 中空数组的 "failed" 措辞同步调整
- [x] 2.2 `article_tagger.go`：兜底条件 `err != nil || len(result.Tags) == 0`（:143）收窄为 `err != nil`；同步更新 :126-128 aggregate 回落注释（「never end up with zero tags」在新行为下不再成立）
- [x] 2.3 新增 `filterGenericLabels`（黑名单常量：新闻/论坛/要闻/快讯/文章/内容/技术/发展，Slugify 后完整匹配），接入 `persistArticleTags` 入口（覆盖 mono/aggregate/heuristic 三路径）；`reuseTagsFromSiblingArticle` 的 siblingLinks 查询 join topic_tags 排除黑名单 label（第四条写入路径，不经 persist 入口）
- [x] 2.4 跑 1.x 用例确认全绿；如存量用例断言旧契约（回填/零结果降级），按 test-cases.md ⓪ 表改写

## 3. 排查任务

- [x] 3.1 定位凤凰网文章 llm 来源「Coding」（tag_id 27）写入路径：对比受污染文章 content `updated_at` 与标签落库时间线，验证「初版内容含模式词被抽取 → content 被爬虫替换」假设；结论与证据记入本 change design.md 附录；若发现独立写入路径，升级单独修复

## 4. 存量数据清理（红线流程：备份 → count 留底 → BEGIN 预览 → COMMIT）

- [x] 4.1 `pg_dump -t article_topic_tags` 备份落盘；执行 design D5 留底 count（预期分项：heuristic=484、llm 黑名单泛词≈714、llm Coding(tag_id 27)≈90），数字与本 design 记录偏差超过 ±5% 时停下核对
- [x] 4.2 BEGIN 内执行 D5 两条 DELETE 并 count 验证 → COMMIT 真删；二次执行确认 count=0
- [x] 4.3 确认 tag 队列对零标签文章的重打已被触发（观察 `tag_queue` 状态 / scheduler 周期），等待重打消化（约 300-400 篇 × ~2s）<!-- 实际情况修正：队列无零标签自动发现机制（proposal 假设不成立）；用户决策改用 retag-today 全量重跑今日文章（443 篇 force_retag 入队），历史零标签文章中仅 36 篇未归档但用户选择不单独补队；重启后新落库 237 行 llm 均干净，heuristic/泛词/Coding 新增 = 0（修复生效铁证） -->
- [ ] 4.4 量化核对（test-cases.md 效果核对）：抽样 ≥30 篇重打文章，黑名单词命中数 = 0；48h 新落库标签黑名单命中 = 0 <!-- 观察窗口项：重打消化中（556 任务 ~20min）；24h 抽样与 48h 新增观测需后续会话留痕 -->

## 5. 配置项（管理界面操作，非代码）

- [x] 5.1 供应商 `qwen` 的 `timeout_seconds` 由 3000 调整为 300（当前 50 分钟超时会钉死 tag worker）<!-- 经管理 API PUT /api/ai/providers/1 完成（全字段保真回写），复核 timeout=300 -->
- [x] 5.2 `topic_tagging` 路由绑定 `mimo-v2.6-pro` 为 P2 兜底（当前单绑定，断供即落 heuristic）<!-- 经管理 API PUT /api/ai/routes/topic_tagging 完成，复核绑定 qwen(P1)+mimo-v2.6-pro(P2) -->

## 文档

<!-- doc-impact: flow -->
- [x] 6.1 `docs/reference/flow/semantic-board.md`：辅助标签入库节补充空结果语义（LLM 判空 = 合法结论，不再有泛词回填进入辅助标签池）；若 3.1 排查发现新写入路径，同步链路描述

## 测试

- [x] 7.1 受影响包测试全绿：`go test ./internal/tagmanagement/...`（含 1.x 新用例与改写后的存量用例）
- [x] 7.2 `golangci-lint run ./internal/tagmanagement/...` 零新增告警

## 验证

- [x] 8.1 `bash scripts/harness/change-scope.sh` 确认影响范围仅 tagmanagement 包
- [x] 8.2 `go test ./internal/tagmanagement/service/core/ -run 'TestTag|TestExtract' -count=1` 全绿（期望：ok）
- [x] 8.3 `bash scripts/harness/doc-impact.sh verify` 通过（期望：语义版块 flow 文档对账无缺口）
- [x] 8.4 `bash scripts/harness/check-standards.sh` 通过（期望：F/G 段无违例）
- [ ] 8.5 真库核对（4.4 留痕）：黑名单词新命中 = 0（期望：查询返回 0）
