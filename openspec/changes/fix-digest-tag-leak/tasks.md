# Tasks — fix-digest-tag-leak

## 1. 方向校验相对门（tagmanagement/service/board）

- [x] 1.1 `LoadConfig` 新增 `DirectionRelativeMargin`（ai_settings key `semantic_board_match_direction_relative_margin`，默认 0.15，≤0 视为关闭），补 margin 三配置用例（0.18/0/缺省），验证 `go test ./internal/tagmanagement/service/board -run TestLoadConfig` 通过
- [x] 1.2 `evaluateSemanticBoardMatches` 实现相对门：参照集=同轮全部匹配候选（composite_hit 除外）的 identity cosine 最大者，gap ≥ margin 时 `direction_mismatch=true`；composite_hit 保持豁免、direct_hit 纳入双门（对齐现行语义），按 test-cases.md 故事 A 表补齐 8 步用例（含 gap==margin 等值边界），验证 `go test ./internal/tagmanagement/service/board -run 'TestEvaluateSemanticBoardMatches_Direction'` 通过
- [x] 1.3 旧用例回归：`TestEvaluateSemanticBoardMatches_DirectionCheck` 绝对门断言全部保留不变，验证 `go test ./internal/tagmanagement/service/board` 通过

## 2. 去重 tiebreak 版块相关性（topicgraph/service）

- [x] 2.1 topicgraph repository 新增批量加载标签 identity embedding（`topic_tag_embeddings` WHERE embedding_type='identity'）与版块 embedding（`semantic_labels` WHERE label_type='board'）查询，验证对应 repository 单测通过
- [x] 2.2 `DeduplicateTags`/`shouldReplace` 扩三级 tiebreak（文章数 > identity×版块 cosine > id），签名接入相关性上下文；按 test-cases.md 故事 B 表补齐 6 步用例，改造 `TestDeduplicateTags_SingleArticleOverlap` 为三级 tiebreak 用例，验证 `go test ./internal/topicgraph/service -run TestDeduplicate` 通过
- [x] 2.3 orchestrator 接线：collect 后加载 embedding 传入去重，加载失败降级现行为且管线不中断，验证 `go test ./internal/topicgraph/service` 通过

## 3. 测试

- [x] 3.1 按 test-cases.md 主链路两故事全量执行：`cd backend-go && go test ./internal/tagmanagement/service/board ./internal/topicgraph/service`（只跑影响包），期望全绿
- [x] 3.2 白盒附加边界值核对：gap==margin 标记、cos 差精确相等回退 ID 两用例落地，验证对应 `-run` 精确命中执行
- [x] 3.3 效果核对（真库量化，呼应 test-design ④）：实现完成后对触发案例标签（204328）手动 rematch，SQL 验证 2165 俄乌挂载 direction_mismatch 翻转为 true、1980 生成式 AI 保持 false；期望多挂 mismatch 增量 ~7% 量级

## 4. 文档

<!-- doc-impact: semantic-board, daily-report -->

- [x] 4.1 `docs/reference/flow/semantic-board.md` 业务约束节：方向校验描述补「绝对+相对双门、margin=0.15 可配、composite_hit 豁免、direct_hit 同样校验」，grep 校验新表述存在
- [x] 4.2 `docs/reference/flow/daily-report.md` 业务约束节：去重描述补「三级 tiebreak（文章数>版块相关性>ID，embedding 缺失回退）」，grep 校验新表述存在

## 5. 验证

- [x] 5.1 `cd backend-go && golangci-lint run ./internal/tagmanagement/... ./internal/topicgraph/...`，期望 0 issue
- [x] 5.2 `cd backend-go && go vet ./internal/tagmanagement/... ./internal/topicgraph/...`，期望无输出
- [x] 5.3 `cd backend-go && go build ./...`，期望编译成功
- [x] 5.4 `bash scripts/harness/change-scope.sh` 机械判定影响包与执行命令一致，期望命中 tagmanagement/board 与 topicgraph/service
