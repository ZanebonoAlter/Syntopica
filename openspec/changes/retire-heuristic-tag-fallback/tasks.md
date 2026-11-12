# Tasks — retire-heuristic-tag-fallback

<!-- doc-impact: flow, database -->

## 代码删除

- [ ] 1. `internal/tagmanagement/service/core/extractor_heuristic.go` **整文件删除**（`ExtractTopics` / `topicRules` / `extractFeedEntity` / `slugify` / `productTokenPattern` / `heuristicPunctPattern`——已核实无他处调用方）
- [ ] 2. `extractor_enhanced.go`：删 `extractWithHeuristic`（:256-281）；`ExtractTags` 失败分支改为 `return nil, err`；删 `ExtractionResult.Source` 字段（:38 + :82/:107 两处构造）
- [ ] 3. `article_tagger.go`：编排层失败分支返回错误（不再 `legacyExtractTopics` / 不再 `source="heuristic"`）；`source = result.Source` → `source = "llm"`
- [ ] 4. `tagger.go`：删 `legacyExtractTopics`（:40-56）
- [ ] 5. 后端测试同步：`extractor_test.go` / `extractor_enhanced_test.go` / `article_tagger_test.go` / `sampling_path_test.go` / `hard_merge_test.go` / `extractor_generic_subject_test.go`（断言从「降级 heuristic」改为「返错 + 不写标签」）
- [ ] 6. 后端门禁：`go build ./...` / `go vet ./...` / `golangci-lint run ./...` / `go test -short ./internal/tagmanagement/...`

## 规范与文档

- [ ] 7. `openspec/specs/tagging-domain/spec.md` delta：REMOVED「heuristic 兜底仅限提取调用失败」+ ADDED「提取失败不降级，直接判任务失败」+ MODIFIED「泛词标签黑名单拦截」路径清单 + MODIFIED「LLM 空 keyword 结果不触发规则回填」补「无任何规则词回填路径」
- [ ] 8. `openspec/specs/auxiliary-label/spec.md` delta：MODIFIED「Tag 提取单次调用输出 event/person 与 keyword 双数组」两条 heuristic 条款作废
- [ ] 9. `docs/reference/flow/semantic-board.md`：顶部红线段 + 红线 19 改为「不降级，失败即失败交队列重试」
- [ ] 10. `docs/reference/flow/scheduler.md`：健康门理由改为「文章无标签入库，不污染下游」
- [ ] 11. `docs/reference/database/tables/topic-tags.md`：`source` 枚举去 `heuristic`

## 存量清理与重打（真库，红线：事务包裹 + 先留底）

- [ ] 12. 留底：`count(*)` 记录 `article_topic_tags`（heuristic 1724 行）/ `topic_tags`（heuristic 28 个）/ 引用它们的挂载行与向量行；确认 `backups/pg-key-*.dump` 备份路径可用
- [ ] 13. **事务演练**：`BEGIN; …; ROLLBACK;` 内执行清理语句，核对影响行数与 FK 行为（`topic_tag_board_labels` / `topic_tag_embeddings` / `article_topic_tags` / `topic_tags` 的删除顺序）
- [ ] 14. 正式执行：`BEGIN; …; COMMIT;` 事务内清理 + `articles.tag_count` 重算
- [ ] 15. 验证：真库只读核对 `source='heuristic'` 归零、board 1980 上 `Coding` / `AI Agent` 挂载消失、挂载总行数变化可解释
- [ ] 16. 批量重打：对受影响文章走 `POST /api/tag-jobs/retag-today`（Force=true 不复用 sibling 标签）；分批跑并观察成功率与失败率
- [ ] 17. 重打后核对：board 1980 挂载标签不再含泛词；日报文章数恢复；抽样确认拿到真 event 标签

## 部署与归档

- [ ] 18. 重启后端 + 部署前端静态托管（前端零改动，仅确认不影响）；`/health` 200
- [ ] 19. `bash scripts/harness/doc-impact.sh verify openspec/changes/retire-heuristic-tag-fallback` + `check-standards.sh`
- [ ] 20. 派 review 子线程审查制品 + diff，意见按 High/Medium/Low 处置后 `review.md` 建账
- [ ] 21. 归档门禁六项自查全绿 → `openspec archive retire-heuristic-tag-fallback` → 补 `docs/reference/flow/semantic-board.md` 变更溯源行
- [ ] 22. 汇报部署后影响 + 需要的用户操作 + 旧数据降级路径

## 测试

- [ ] 23. 按 `test-cases.md` 逐条核对：主链路表 + 变体走查五组 + 效果核对 + 白盒附加
- [ ] 24. 队列重试链路验证：注入一次提取失败 → 观察任务从 failed→pending（退避）→ 重领 → 终态 failed
