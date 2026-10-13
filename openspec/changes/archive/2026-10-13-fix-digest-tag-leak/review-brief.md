# 审查 brief：fix-digest-tag-leak（全量增量，commit f7793374，已提交）

只读审查。你只输出报告，禁止修改任何文件、禁止执行改变状态的命令（允许 read/grep/find/ls 与只读验证如 `go test ./internal/tagmanagement/service/board -run Direction` / `git show`；测试只跑下列指定的两包 `-run` 精确匹配，不跑全量；优先 grep 与精读）。

## 一、审查对象（本 change 辖区，已提交为 f7793374）

**背景**：2026-10-10 俄乌版块日报 digest 串味（OpenAI 标签挂进俄乌版块、挤掉相关柴油标签）。两个根因：①方向校验绝对阈值 0.5 在新闻域无区分度（正确归属 0.81 vs 串味 0.60 双双过线）；②去重 tiebreak 只比文章数+ID，同文章双事件标签纯抽签。本 change 补：方向校验相对门（gap ≥ margin 判 mismatch）+ 去重三级 tiebreak（文章数 > 版块相关性 cosine > ID）。

**改动清单**（审查只看这些，`git show f7793374 -- <path>` 看增量）：

| 层 | 文件 |
| --- | --- |
| 方向校验 | `backend-go/internal/tagmanagement/service/board/semantic_board_matching.go`（`applyDirectionRelativeGate` + `DirectionRelativeMargin` 配置 + ai_settings key `semantic_board_match_direction_relative_margin`） |
| 方向校验测试 | 同包 `semantic_board_matching_config_test.go`（margin 五子用例）、`semantic_board_matching_unit_test.go`（`TestEvaluateSemanticBoardMatches_DirectionRelativeGate`）、`semantic_board_matching_test.go`（direct_hit 既有用例回归） |
| 去重 | `backend-go/internal/topicgraph/service/daily_report_dedup.go`（`DedupRelevance` + `tagCosine` + `loadDedupRelevance` + 三级 tiebreak） |
| 去重接线 | `backend-go/internal/topicgraph/service/daily_report_orchestrator.go`（collect 后加载 embedding 传入去重，失败降级） |
| 去重测试 | `daily_report_dedup_test.go`（9 用例） |
| repository | `backend-go/internal/topicgraph/repository/daily_report_topic_repository.go`（`ListTagIdentityEmbeddings` + `GetBoardEmbedding`）+ `daily_report_dedup_embeddings_test.go` |
| 文档 | `docs/reference/flow/semantic-board.md`（红线 4 双门表述）、`docs/reference/flow/daily-report.md`（Step2 三级 tiebreak 表述） |

契约权威源：delta spec `specs/board-direction-check/spec.md`（9 Scenario）+ `specs/daily-report-system/spec.md`（5 Scenario）；design 权威源 `design.md`；用例账本 `test-cases.md`。

## 二、审查维度

1. **相对门实现 vs delta spec**（精读 `applyDirectionRelativeGate`，约 L486-531）：
   - 参照集：bestCos 是否取「同轮全部匹配成功版块（composite_hit 除外）」的 identity cosine 最大者；无 cosine 可算时是否整体跳过（不误标）。
   - composite_hit 豁免是否双侧贯彻（参照集收集与最终标记两处循环都 skip）；`direction_mismatch` 只 OR 不清（不覆盖绝对门已标的 true）。
   - direct_hit 挂载是否同样进相对门（与 spec「direct_hit 纳入双门」一致）；gap==margin 等值边界是否用 `>=`（spec Scenario「gap 0.21 >= 0.15」与 design「等值边界」约定）。
   - margin ≤ 0 关闭（`DirectionRelativeMargin <= 0` return）；tag embedding 空 return；单挂标签 gap≡0 自然通过（数学上是否必然，不用单列分支）。
   - 配置链：默认 0.15、ai_settings key 拼写、非法值 fallback（`parseDirectionRelativeMargin`）与既有 threshold 解析语义一致。
2. **去重三级 tiebreak vs delta spec**（精读 `daily_report_dedup.go` 全文）：
   - 优先级顺序：article_count 短路（不同文章数时不算 cosine）→ identity×board cosine 高者存活 → cosine 相等/任一缺失回退 ID 小者。spec 明确「cosine 相同或任一 embedding 缺失时，id 更小者存活」。
   - `tagCosine` 对全零向量、缺失 map 项的处理是否稳健（全零是否按缺失回退）；`DedupRelevance` 为 nil 时是否完全等价旧行为（向后兼容红线）。
   - 去重只动内存列表不改 `topic_tags` 原始数据（spec 最后一个 Scenario）。
   - `loadDedupRelevance` 失败路径：log warn + 返回 nil/降级，orchestrator 不中断（与 design「加载失败降级现行为且管线不中断」一致）。
3. **repository 加载查询正确性**：`ListTagIdentityEmbeddings` 是否 `embedding_type = 'identity'`、`GetBoardEmbedding` 是否取 `label_type='board'` 的 embedding；空入参是否短路（不生成 `IN ()` 非法 SQL）；返回 map 语义与调用方解析一致（vector 文本 → []float64 的解析职责在谁、错误如何传播）。
4. **orchestrator 接线**：加载时机在 collect 后、去重前；boardID 来源正确；失败仅 Warn 不阻断；与既有「load tag embeddings for bucket」调用是否复用同一批数据（重复加载仅提示性意见）。
5. **测试覆盖对账**（对照 delta spec 全部 14 个 Requirement-scenario 与 test-cases.md 两故事表）：
   - board-direction-check 9 Scenario：hit_rate pass/fail、weighted fail、embedding unavailable、direct_hit、relative gate 拦截（0.81/0.60/margin 0.15）、legal dual-mount（0.82/0.70）、single-board、composite_hit——逐一确认有用例函数与断言名对应。
   - DirectionRelativeMargin 3 Scenario（custom 0.18 / zero 关闭 / no row 默认）→ config test 子用例。
   - daily-report 5 Scenario：集合相同合并、单文章合并、双事件保留贴近版块者（0.60 vs 0.80 故事）、embedding 缺失回退 ID、不碰原始数据——逐一确认。
   - 白盒边界：gap==margin 等值、cos 精确相等回退 ID（tasks 3.2 声称落地）是否真有用例。
6. **旧用例回归**：`TestEvaluateSemanticBoardMatches_DirectionCheck` 绝对门断言是否全部保留（tasks 1.3 声称不动）；`git show f7793374 -- backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go` 中该函数是否零改动。
7. **grep/只读验证**（至少）：
   - `cd backend-go && go test ./internal/tagmanagement/service/board -run 'TestEvaluateSemanticBoardMatches_Direction|TestLoadConfig' -v` → 全 PASS。
   - `cd backend-go && go test ./internal/topicgraph/service -run 'TestDeduplicate' -v` → 全 PASS。
   - `grep -n "semantic_board_match_direction_relative_margin" backend-go/internal/tagmanagement/service/board/semantic_board_matching.go` → 默认值与 LoadConfig/parse 三处链路齐全。
   - `grep -n "三级 tiebreak\|相对门" docs/reference/flow/daily-report.md docs/reference/flow/semantic-board.md` → 文档表述与实现一致（margin 默认 0.15、回退语义）。
8. **制品自洽**：tasks.md 已全勾；proposal「明确不做」两事项（事件句级作用域、L3 拆散异质组）是否真未在本 diff 偷做；spec RENAMED requirement（FROM non-direct_hit TO matched boards）与 MODIFIED 正文一致。

## 三、输出要求（三段式，中文）

1. **总评**：实现是否达成 proposal/design 意图、spec 双门与三级 tiebreak 是否忠实落地、可否进入门禁收口（一句话结论 + 理由）。
2. **问题清单**：按 High/Medium/Low 分级，每条给 `file:line` + 复现/依据 + 建议修法；没有就明确写「无」。
3. **核实记录**：实际跑过的只读命令与退出码、读过的关键文件清单（防幻觉）。

## 已知且已裁决、**不要**当问题报的事项

- 事件标签辅助标签仍为标签级作用域、L3 聚类不拆散异质组——proposal「明确不做」，评估为不稳定留观察。
- 存量已挂载标签不回刷（direction_mismatch 只对新匹配生效；tasks 3.3 的真库 rematch 验证是事后人工动作，不在本 diff 辖区）。
- `daily_report_tracing_test.go` 的 2 行改动为签名联动（DeduplicateTags 新参数），非行为变更。
- 工作树含其他 active change 脏文件（`add-topic-key-event-markers/**`、`dataenrichment/**`、`front/**` 等）——超出本 change 辖区，一律不评。
- 本 change 目录尚未归档、tasks.md 门禁修复（review 节/scenario 映射表/doc-impact 声明）正在主线程补——不是审查对象。
