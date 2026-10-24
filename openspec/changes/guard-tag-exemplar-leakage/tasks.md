# Tasks: guard-tag-exemplar-leakage

## 1. 护栏核心

- [x] 1.1 确认 `Slugify` 中文行为；若中文剥成空串，按 design D3 回退原词等值比较
- [x] 1.2 新增 `mergedPromptExemplarLabels` / `sectionPromptExemplarLabels` 常量集合与 `guardExemplarLeakage(tags, visibleText, exemplarSlugs)` 函数（主标签丢弃 + aux 逐项过滤 + 观测尾注）
- [x] 1.3 防漂移单测：断言两个集合每个词原文出现在对应 prompt 文本中

## 2. 挂点接入

- [x] 2.1 mono 路径：`extractMergedCandidates` 返回前过滤，可见文本 = Title+Summary，丢弃记入 ExtractionResult errors
- [x] 2.2 section 路径：`extractTagsFromSection` 解析成功后过滤，可见文本 = Title+sectionTitle+sectionText，丢弃记 warn 日志
- [x] 2.3 旧测试 fixture 处置：正文无提及却断言示例词挂载的旧测试，按 design D5 正文补提及（清单见 test-cases.md 继承与调整表）

## 3. 存量清理工具

- [x] 3.1 `cmd/admin` 新增 `tagguard-cleanup` 子命令（按 fix-event-tag-anchor 先例，非独立 cmd 目录）：判定逻辑与护栏同源，默认 dry-run，`--apply` 事务删除 + tag_count 重算 + `CleanupOrphanedTags`
- [x] 3.2 真库 dry-run 核对清单 = 摸底预期（实测 28 行：PostgreSQL 26 + 某国央行上调基准利率 2 + kubernetes 1，比摸底多 3 条为后台新打标 + event 占位词标签）
- [x] 3.3 执行 `--apply` 后复核：PostgreSQL 关联 36→11、标签本体在、占位词标签孤儿清理、重跑 dry-run leaked=0（幂等）

## 4. 测试

<!-- test-coverage: guard 单测 + 两条路径行为测试 + 防漂移 + 清理幂等 -->

- [x] T1 guardExemplarLeakage 函数单测：丢弃/保留/非示例词/大小写变体/aux 过滤/空切片（对应 specs Scenario 1-6）
- [x] T2 mono 路径行为测试（fake router）：无提及丢弃 + 有提及保留
- [x] T3 section 路径行为测试（fake router）：栏目片无提及丢弃
- [x] T4 防漂移测试（Scenario 7）
- [x] T5 清理工具判定函数单测 + dry-run/apply 幂等（Scenario 8-10：真库 dry-run 清单核对 + apply 后重跑 leaked=0）
- [x] T6 受影响包既有测试全绿（`go test ./internal/tagmanagement/service/core/` 含 testcontainer 完整跑 10.7s ok；fixture 处置见 test-cases.md）

## 5. 文档

<!-- doc-impact: flow -->

（更新对象：docs/reference/flow/semantic-board.md 变更溯源节）

- [x] D1 `docs/reference/flow/semantic-board.md` 变更溯源节补记：示例词泄漏护栏上线 + 存量清理结果（36→11）
- [x] D2 归档前 `bash scripts/harness/doc-impact.sh verify` + `check-standards.sh` 过（本 change verify 通过；check-standards 仅余 2 个他人 active change 的 doc-impact 欠账，与本 change 无关）

## 6. 验证

- [x] V1 `cd backend-go && golangci-lint run ./internal/tagmanagement/... ./cmd/admin/...` → 0 issue
- [x] V2 `cd backend-go && go vet ./internal/tagmanagement/... ./cmd/admin/` → 无告警
- [x] V3 `cd backend-go && go test ./internal/tagmanagement/service/core/ -run 'Guard|Exemplar|Leak'` → 全 PASS（11 个护栏测试）
- [x] V4 `cd backend-go && go build ./...` → 编译通过
- [x] V5 真库复核（BEGIN/ROLLBACK 只读）：`SELECT count(*) FROM article_topic_tags WHERE topic_tag_id=1556` → 11 ✓
- [x] V6 抽查 416 期文章：`SELECT topic_tag_id FROM article_topic_tags WHERE article_id=153877` → 不含 1556 ✓
