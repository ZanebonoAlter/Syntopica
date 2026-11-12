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

- [x] 12. 留底：`article_topic_tags` 总 18533 / heuristic 1727；`topic_tags` 总 13888 / heuristic 28；受影响文章 1142 篇；board 1980 挂载行 944；`backups/pg-key-*.dump`（2026-11-12 04:00）已确认可用
- [x] 13. **事务演练**（`BEGIN; …; ROLLBACK;`）跑通；发现并修掉两处执行期问题（见 design D3.1）
- [x] 14. 正式执行（`BEGIN; …; COMMIT;`）：`DELETE 1727` 文章标签关联 / `DELETE 14` 零引用标签 / `UPDATE 14` source→llm / `DELETE 12` repair_items 悬挂 / `DELETE 0` embedding_queues / `DELETE 21` merge_suggestions / `UPDATE 1142` + `UPDATE 0` tag_count → **COMMIT**
- [x] 15. 验证：`source='heuristic'` 文章标签行 **0**、标签 **0**；`topic_tags` 13882（原 13888，-14 实删 +8 重打新增）；board 1980 挂载 944；**Coding 关联文章 722 → 1**、**AI Agent 85 → 16**（余下为 LLM 真实产出）；`articles.tag_count` 全库不一致 1231 → **1157**（等于存量值，本次受影响的 74 行已修正，未越界动存量）
- [ ] 16. 批量重打 —— **阻塞：等 `tolerate-schema-envelope-extraction-output`（另案）先修解析器**。20 篇试验批实测：模型 82% 返回 schema 外壳形态（`{"type":"object","properties":{...}}`），解析器不认 → 任务失败重试 3 次后仍失败 → 拿不到 event 标签 → 重打无产出。试验批已置 failed 暂停，待解析器修复后统一重打
- [ ] 17. 重打后核对（随 16 一并做）：board 1980 挂载标签不再含泛词；日报文章数恢复；抽样确认拿到真 event 标签

## 部署与归档

- [ ] 18. 重启后端 + 部署前端静态托管（前端零改动，仅确认不影响）；`/health` 200
- [ ] 19. `bash scripts/harness/doc-impact.sh verify openspec/changes/retire-heuristic-tag-fallback` + `check-standards.sh`
- [ ] 20. 派 review 子线程审查制品 + diff，意见按 High/Medium/Low 处置后 `review.md` 建账
- [ ] 21. 归档门禁六项自查全绿 → `openspec archive retire-heuristic-tag-fallback` → 补 `docs/reference/flow/semantic-board.md` 变更溯源行
- [ ] 22. 汇报部署后影响 + 需要的用户操作 + 旧数据降级路径

## 测试

- [ ] 23. 按 `test-cases.md` 逐条核对：主链路表 + 变体走查五组 + 效果核对 + 白盒附加
- [ ] 24. 队列重试链路验证：注入一次提取失败 → 观察任务从 failed→pending（退避）→ 重领 → 终态 failed
