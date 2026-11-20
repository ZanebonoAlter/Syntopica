# Tasks: topic-lane-auxiliary-label-constraint

> 依赖说明：第 6 组（前端）中的**编辑面接线**阻塞于 `lane-manual-correction-loop`（该 change 尚未开工，0/60 任务）。第 1~5 组（数据模型 / 后端 / API）不依赖它，可先行。第 9 组（效果核对）需在约束绑定后跑一次真实日报生成。

## 1. 数据模型与迁移

- [ ] 1.1 在 `backend-go/internal/platform/database/postgres_migrations.go` 写显式版本迁移建 `board_persistent_topic_constraints`（`id` / `topic_id` FK→`board_persistent_topics` ON DELETE CASCADE / `term` VARCHAR(100) NOT NULL / `term_embedding` **无维度限定 `vector`** / `embedding_model` VARCHAR(100) NOT NULL / `mode` VARCHAR(16) NOT NULL DEFAULT 'required' / `margin` NUMERIC NOT NULL / `max_distance` NUMERIC NULL / `created_at` / `updated_at`），CHECK：`mode IN ('required','preferred')`、`margin BETWEEN 0 AND 1`、`max_distance` 非空时 BETWEEN 0 AND 1，唯一约束 `(topic_id, term)`，索引 `topic_id`；验证：`bash scripts/harness/db-migration-safety.sh` 通过，且 `psql -c "\d board_persistent_topic_constraints"` 结构与本任务一致
- [ ] 1.2 在 `backend-go/internal/topicgraph/repository/daily_report_models.go` 新增 `TopicLaneConstraint` 模型与 `TableName()`，字段与迁移一一对应；验证：`cd backend-go && go build ./...` 通过
- [ ] 1.3 在 `backend-go/internal/topicgraph/repository/daily_report_topic_repository.go` 加约束仓储（按 topic 列出、全量替换且幂等、按 id 删除）；全量替换 SHALL 在单事务内完成，任一条非法 SHALL 整体回滚不部分写入；验证：仓储单测断言「两次相同替换请求后集合相同」与「含非法项时集合保持提交前状态」
- [ ] 1.4 加仓储层 testcontainer PG 测试（repository 禁用 SQLite）；验证：`cd backend-go && go test ./internal/topicgraph/repository/...` 通过

## 2. 后端 · 约束锚向量计算

- [ ] 2.1 在 `backend-go/internal/topicgraph/service/` 新增约束锚文本编码函数：输入用户短词 `term`，输出 `term + " " + "event"`（与 `buildTagEmbeddingText` 的 category 段同构，见 design D3）；验证：函数单测断言输出字符串逐字等于预期
- [ ] 2.2 实现锚向量计算：调 `airouter.Router.Embed`，取返回的模型名作为 `embedding_model` 一并返回；验证：单测用 stub router 断言「传入的输入文本等于 2.1 的编码结果」且「返回的模型名被透传」
- [ ] 2.3 实现失败路径：embedding 报错 / 返回不可解析 / 维度与既有向量不一致时 SHALL 返回错误且 SHALL NOT 落库、SHALL NOT 写入零向量；验证：单测断言错误被返回且仓储无写入调用
- [ ] 2.4 约束向量 SHALL NOT 在 GORM 模型上声明带维度的 `vector(2560)`（与 `unify-vector-column-dimension` 的终态一致）；验证：`grep -rn "vector(2560)" backend-go/internal/topicgraph/` 无新增命中

## 3. 后端 · 约束覆盖判定（纯函数）

- [ ] 3.1 实现相对偏好判定：约束 `i` 被覆盖 ⟺ 存在 tag `t` 使 `d(t, A_i) + margin_i < d(t, A_j)`（∀ 其它约束 `j`）；`max_distance` 非空时同时要求 `d(t, A_i) < max_distance_i`；验证：表驱动单测覆盖「满足」「不满足」「恰好等于 margin（边界不含）」三态
- [ ] 3.2 实现分腿覆盖聚合：多条 required 约束 SHALL 全部被覆盖才算「满足」，不同约束可由不同 tag 分别满足；验证：单测断言「一个 tag 满足以色列、另一个 tag 满足黎巴嫩 → 满足」与「只有以色列侧 tag → 违反」
- [ ] 3.3 实现跨向量空间跳过：tag 的模型标识与锚不一致时跳过，既不计入覆盖也不判违反；验证：单测断言该 tag 不影响判定结果
- [ ] 3.4 实现单约束退化：话题只有一条 required 约束时相对偏好无参照，SHALL 仅按 `max_distance` 判定；`max_distance` 为空时该约束 SHALL 视为无法判定、SHALL NOT 参与拦截；验证：单测覆盖两种退化路径
- [ ] 3.5 实现 `preferred` 约束不参与拦截、仅作加权信号；验证：单测断言 preferred 未被覆盖时整体判定为「满足」
- [ ] 3.6 实现无标签 section 跳过约束检查、SHALL NOT 判违反；验证：单测断言空标签集合返回「满足」
- [ ] 3.7 全部判定逻辑 SHALL 为纯函数（输入：约束列表 + tag 向量集合 + 模型标识；输出：满足/违反/无法判定），SHALL NOT 触碰数据库；验证：单测文件位于 `backend-go/internal/topicgraph/service/` 且不经数据库即可运行（`go test ./internal/topicgraph/service/ -run TestConstraint`）

## 4. 后端 · 闸门接入 lane group

- [ ] 4.1 明确 lane group 的判定输入来源：SHALL 使用 section 的有效 tag 集合，`daily_report_sections.cluster_tag_ids` 实测 92.8% 悬空 SHALL NOT 直接作为输入，改走 `article_topic_tags`（悬空率 0%）经 section→article 关联取得，或在 section 生成时物化当时有效的 tag id 快照；验证：单测断言在 `cluster_tag_ids` 含已删除 tag id 的情况下仍能取到有效 tag 集合
- [ ] 4.2 在 `backend-go/internal/topicgraph/service/daily_report_lane.go:assembleLaneGroups` 之后、`planTopicAssignments` 之前接入约束闸门：lane group 满足其话题的 required 约束 → 正常归属；违例 → 不锚定该话题；验证：单测断言「无约束话题的 lane group 归属与引入约束前逐字一致（零额外 LLM 调用）」
- [ ] 4.3 违例 lane group 补一次 L2 裁决（复用 `buildL2Prompt` 与 `parseL2Response`，候选含各话题的约束状态），最终 `lane_tier` 仍为 `l2_llm`；验证：单测断言违例 group 触发了 LLM 调用且调用次数为 1
- [ ] 4.4 LLM 选定的话题其 required 约束仍未被覆盖时，沿用 `parseL2Response` 既有「switch 到候选集外 → 降级为 new」范式转 L3；验证：单测断言该 group 最终 `lane_tier=l3_new` 且未锚定到违例话题
- [ ] 4.5 LLM 漏答 / JSON 畸形时的既有安全网（keep 最近候选） SHALL NOT 绕过约束闸门；验证：单测断言漏答场景下违例 group 仍不锚定违例话题

## 5. 后端 · 约束管理 API

- [ ] 5.1 新增 `GET /api/daily-reports/topics/:id/constraints`：返回约束列表（`term` / `mode` / `margin` / `max_distance` / `embedding_model` / 时间戳），SHALL NOT 返回 `term_embedding` 原始向量；话题不存在返回 404；验证：handler 测试断言响应体不含向量字段且 404 路径正确
- [ ] 5.2 新增 `PUT /api/daily-reports/topics/:id/constraints`：全量覆盖语义、幂等；请求体含重复 `term` / 非法 `mode` / 越界 `margin` / 越界 `max_distance` 时返回 400 且 SHALL NOT 部分写入；验证：handler 测试覆盖「两次相同请求结果一致」与「含非法项时集合不变」
- [ ] 5.3 新增约束时 SHALL 计算并持久化锚向量（复用第 2 组），计算失败返回错误且不落库；验证：handler 测试断言 embedding 失败时响应为错误码且表内无新增行
- [ ] 5.4 在 `backend-go/internal/app/router.go` 注册两条路由；验证：`cd backend-go && go build ./...` 通过且 `curl` 命中路由返回预期状态码

## 6. 前端 · 辅助标签配置区

- [ ] 6.1 在 `front/app/api/dailyReports.ts` 加约束读写客户端（列表读取 / 全量替换）；验证：`pnpm exec nuxi typecheck` 通过
- [ ] 6.2 实现配置区组件（短词输入 + `required`/`preferred` 二选一 + `margin` 折叠输入 + 删除 + 列表行 + 模型标识展示），状态覆盖空态 / 加载 / 计算中 / 错误可重试 / 成功 / 长文本 / 重复提交；验证：组件 Vitest 测试断言各状态渲染与「重复短词提示」「删除行」交互
- [ ] 6.3 组件 SHALL 复用 `AppInput` / `AppButton` / 既有列表行与徽标样式，SHALL NOT 引入新组件库；验证：组件测试断言复用组件被渲染
- [ ] 6.4 **接线到话题编辑面**（阻塞）：待 `lane-manual-correction-loop` 的编辑面就绪后接入；本任务 SHALL 在该 change 完成前保持未勾选并留痕；验证：接入后 agent-browser 主链路（打开编辑面 → 添加约束 → 看到行与模式徽标 → 删除）在 `ui-verification/report.md` 建账

## 7. 用例设计文档（complex 档必备）

- [ ] 7.1 写 `openspec/changes/topic-lane-auxiliary-label-constraint/test-cases.md` 的主链路表（步 / 动作 / 来源 Scenario / 期望 / 层 / 落点），串起「绑定约束 → 违例降级 → 纠正归属」三条故事；验证：文档存在且每个引用的 Scenario 名能在本 change 的 delta spec 中匹配到
- [ ] 7.2 补「变体走查」五组固定清单（输入 / 前置 / 时间窗口 / 幂等 / 可用性），每组有明确答案，不适用项划除留痕；验证：五组齐全且无「待定」
- [ ] 7.3 补「⓪ 改契约了吗」表：跑 `bash scripts/harness/test-assets.sh persistent-topic` 反查旧资产，逐条处置旧 Scenario（继承与调整 / 替换 / 退役）；验证：表中每条旧资产都有处置结论
- [ ] 7.4 补「效果核对」节（真库量化）：绑定「以色列」+「黎巴嫩」两条 required 约束到 id=2 后跑一次日报生成，核对「2026-11-12 之前 32 条纯以色列内政 section 在新约束下是否被拦下」，记录触发原因 / 方法 / 量化结果 / 结论；并据此标定 `margin` 最终取值；验证：核对结论与 margin 取值写入文档
- [ ] 7.5 补「白盒附加」节：分支表（覆盖判定三分支 / 跨空间跳过 / 单约束退化 / preferred 不拦截 / 无标签跳过 / LLM 漏答兜底）+ 边界值清单（margin=0 边界、max_distance 边界、约束数 0/1/2）+ 不适用划除留痕；验证：文档存在

## 8. 文档更新

- [ ] 8.1 更新 `docs/reference/flow/topic-graph.md` 的「业务约束与不变量」节，新增约束准入红线（相对偏好判定、跨空间跳过、不参与生命周期）；验证：`grep -n "约束" docs/reference/flow/topic-graph.md` 命中新增红线句
- [ ] 8.2 更新 `docs/reference/flow/daily-report.md` 的归属链路描述（lane group 级约束闸门与违例降级）；验证：文中能检索到约束闸门描述
- [ ] 8.3 更新 `docs/reference/api/daily-reports.md`，补两条约束端点（路径 / 方法 / 请求体 / 响应体 / 错误码）；验证：端点条目存在且字段与实现一致
- [ ] 8.4 更新 `docs/reference/database/tables/daily-report-watch.md`，在 §9.5（`board_persistent_topics`）之后新增 `board_persistent_topic_constraints` 表条目与字段；验证：表文档含新表结构
- [ ] 8.5 archive 后按 §12.2 补 `docs/reference/flow/topic-graph.md` 与 `daily-report.md` 的「变更溯源」链接；验证：归档后链接可达

## 9. Review

- [ ] 9.1 派子线程聚焦 review（审查制品 + diff），意见按 High/Medium/Low 处置后 `review.md` 建账；验证：`bash scripts/harness/review-trace.sh openspec/changes/topic-lane-auxiliary-label-constraint` 不报零痕迹，High 条目状态均非「待处理」

## 10. 测试

- [ ] 10.1 后端影响包测试：`cd backend-go && go test ./internal/topicgraph/... ./internal/tagmanagement/...`（按 `bash scripts/harness/change-scope.sh` 判定的影响包收敛，不跑全量 `./...`）
- [ ] 10.2 前端组件测试：`cd front && pnpm test:unit <受影响测试文件> --maxWorkers=2`（参数不带 `--`）
- [ ] 10.3 门禁：`cd backend-go && golangci-lint run ./... && go vet ./... && go build ./...`；`cd front && pnpm lint && pnpm exec nuxi typecheck`

## 11. 文档

<!-- doc-impact: flow, api, database -->

- [ ] `docs/reference/flow/topic-graph.md`
- [ ] `docs/reference/flow/daily-report.md`
- [ ] `docs/reference/api/daily-reports.md`
- [ ] `docs/reference/database/tables/daily-report-watch.md`

## 12. 验证

| Scenario | 测试文件 |
| --- | --- |
| 添加约束写入完整行 | `backend-go/internal/topicgraph/repository/daily_report_topic_repository_test.go` |
| 模式取值受约束 | `backend-go/internal/topicgraph/repository/daily_report_topic_repository_test.go` |
| margin 与 max_distance 越界受约束 | `backend-go/internal/topicgraph/repository/daily_report_topic_repository_test.go` |
| 同一话题内短词重复被拒 | `backend-go/internal/topicgraph/repository/daily_report_topic_repository_test.go` |
| 话题删除级联清理约束 | `backend-go/internal/topicgraph/repository/daily_report_topic_repository_test.go` |
| 短词计算锚向量并记录模型标识 | `backend-go/internal/topicgraph/service/daily_report_constraint_test.go` |
| 不要求用户补充描述 | `backend-go/internal/topicgraph/service/daily_report_constraint_test.go` |
| 计算失败不落库 | `backend-go/internal/topicgraph/service/daily_report_constraint_test.go` |
| 双侧 required 被不同标签分腿覆盖 | `backend-go/internal/topicgraph/service/daily_report_constraint_test.go` |
| 单侧 required 未覆盖判违反 | `backend-go/internal/topicgraph/service/daily_report_constraint_test.go` |
| 纯单侧内容不因相对偏好误判为双侧 | `backend-go/internal/topicgraph/service/daily_report_constraint_test.go` |
| preferred 未覆盖不拦截 | `backend-go/internal/topicgraph/service/daily_report_constraint_test.go` |
| 跨向量空间的标签被跳过 | `backend-go/internal/topicgraph/service/daily_report_constraint_test.go` |
| 无标签 section 跳过约束检查 | `backend-go/internal/topicgraph/service/daily_report_constraint_test.go` |
| 单个标签不得独自满足多条 required 约束 | `backend-go/internal/topicgraph/service/daily_report_constraint_test.go` |
| 单约束话题按绝对上限判定 | `backend-go/internal/topicgraph/service/daily_report_constraint_test.go` |
| 满足约束保持 L1 直挂 | `backend-go/internal/topicgraph/service/daily_report_lane_test.go` |
| 违反约束的 L1 直挂降级 L2 | `backend-go/internal/topicgraph/service/daily_report_lane_test.go` |
| L2 裁决选中约束不满足的话题时降级 | `backend-go/internal/topicgraph/service/daily_report_lane_test.go` |
| 无约束话题行为不变 | `backend-go/internal/topicgraph/service/daily_report_lane_test.go` |
| 读取约束列表 | `backend-go/internal/topicgraph/handler/daily_report_handler_test.go` |
| 批量替换幂等 | `backend-go/internal/topicgraph/handler/daily_report_handler_test.go` |
| 非法载荷不部分写入 | `backend-go/internal/topicgraph/handler/daily_report_handler_test.go` |
| 话题不存在返回 404 | `backend-go/internal/topicgraph/handler/daily_report_handler_test.go` |
| 约束违例不减少命中计数 | `backend-go/internal/topicgraph/repository/daily_report_topic_repository_test.go` |
| 增删约束不改变话题状态 | `backend-go/internal/topicgraph/repository/daily_report_topic_repository_test.go` |
| required 约束未覆盖时不得直挂 | `backend-go/internal/topicgraph/service/daily_report_lane_test.go` |
| 约束满足时 L1 直挂不变 | `backend-go/internal/topicgraph/service/daily_report_lane_test.go` |
| 无约束话题的 L1 直挂不变 | `backend-go/internal/topicgraph/service/daily_report_lane_test.go` |

- `cd backend-go && go build ./...` → PASS（编译通过）
- `cd backend-go && go test ./internal/topicgraph/... -run TestConstraint` → PASS（约束判定纯函数与闸门单测全绿）
- `cd backend-go && go test ./internal/topicgraph/repository/... -run TestTopicLaneConstraint` → PASS（迁移与仓储单测全绿）
- `cd backend-go && golangci-lint run ./...` → PASS（零告警）
- `cd backend-go && go vet ./...` → PASS
- `cd front && pnpm lint` → PASS
- `cd front && pnpm exec nuxi typecheck` → PASS
- `cd front && pnpm test:unit front/app/features/tags/components/... --maxWorkers=2` → PASS（配置区组件测试全绿）
- `bash scripts/harness/doc-impact.sh verify openspec/changes/topic-lane-auxiliary-label-constraint` → PASS（域注释与 checkbox 两层对账通过）
- `bash scripts/harness/check-standards.sh --change topic-lane-auxiliary-label-constraint` → PASS
- `bash scripts/harness/check-tasks-tail.sh openspec/changes/topic-lane-auxiliary-label-constraint` → PASS（尾节结构合规，含 Review 节）
