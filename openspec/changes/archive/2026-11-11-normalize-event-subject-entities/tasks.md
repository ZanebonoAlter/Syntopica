# tasks: 主体实体归一（全局稳定实体 ID）

> 交付颗粒：4 个核心交付项 + Review，尾三节按规范收尾。**T1–T4 与文档节已实现并勾选；T5 review 待独立 change-review 线程；§5 人工真实模型评测未授权——保持未勾选、不建 `evaluation-report.md`**。
> 只跑本次改动影响的包（树莓派红线），不跑全量 `go test ./...`；仓储层只用隔离 PostgreSQL / testcontainer，不用 SQLite、不写真库。
> 故事划分见 `test-cases.md`（S1 首次出现 / S2 跨叫法复现 / S3 相关但不同 / S4 并发与重放 / S5 真实效果且不影响现网）；每个交付项标注覆盖的故事。

## 1. 核心交付

- [x] T1 **实体存储与证据层**（覆盖 S1/S2）：新增 `entity_registry` / `entity_mentions` / `entity_aux_links` 三表与仓储层——稳定实体 ID、规范名不唯一、**无类型列**、两套检索向量（无维度限定 `vector` + 行内记 model/版本/维度）、已登记叫法与提及原文分列存储、辅助标签溯源只记「与本次提及文本规范化相等的那一行」且 aux 被 GC 时只掉溯源行不损毁身份；迁移体放独立文件 `entity_subject_migration.go`，`postgres_migrations.go` 只加一行注册，无历史回填。验证：`cd backend-go && go test ./internal/tagmanagement/service/entity -run 'EntityIdentity|EntityRepository|EntityCandidate'` 通过（T1 的身份/仓储/候选用例落在 service/entity 包；`internal/tagmanagement/repository` 与 `models` 两包内没有 Entity 用例，原写法会「no tests to run」空跑通过）。
- [x] T2 **完整解析链**（覆盖 S1/S2/S3）：第一轮（仅影子态）同次产出主体原文与稳定短身份说明并落 `topic_tags.metadata.entity_subject` 独立惰性键；候选 = 名向量 ∪ 名+短说明向量 ∪ 已登记叫法命中，按 `entity_id` 去重（同名多实体保留各自选项）；第二轮只输出合法实体 ID 或 `none`（非法 ID / 不可解析 / 服务失败 / 超时 = 技术失败，不降级 none、不新建、不半写、最多重试 2 次）；`none` 在单短事务内原子登记统一 ID；同来源同提及重放复用决策。验证：`cd backend-go && go test ./internal/tagmanagement/service/core ./internal/tagmanagement/service/entity` 通过。
- [x] T3 **并发闸门与影子接入、既有判定零改动**（覆盖 S4/S5）：PostgreSQL 会话级咨询锁（独占独立连接）串行化「最新池检索 → 二判 → 原子登记」，第一轮在闸外、事务只包写库段、取得后重读最新池再二判（最新合法 `none` 仍可新建，**不承诺锁保证全局不重**）、取消/超时/异常释放并归还连接、有界等待且无无界重试；模式 `off`（默认，2026-11-10 决策：影子实验后不接线，避免实体注册表只增不减成为黑洞；第一轮提示与调用数逐字不变、零新增调用与写入）/ `shadow`（实验态，需显式配置开启；只写实体层 + 正常 AI 审计 + `entity_subject` 惰性键，其余数据不变）/ `active`（显式拒绝）；事件线身份判定不读实体 ID，`subject_key` 三层与一票否决、人工归属、composite、热度零改动，存量不回填。验证：`cd backend-go && go test ./internal/tagmanagement/service/entity ./internal/topicgraph/service -run 'Entity|Subject|Assign|Manual|Composite|Heat'` 通过。
- [x] T4 **效果评测入口**（覆盖 S5）：`cmd/entity-resolve-eval` —— 真实第一轮产出（影子路径持久化结果或现场重跑）、运行前冻结样本/快照/分组/模型版本/预算/评分规则、盲标金标（运行器不读、评分器事后 join、争议项预列报数）、预算按真实阶段四项机械列出并由评测专用 transport 在**发送前**拦截（无硬预算能力则预先阻断）、评测不写标签业务表、评分按固定总 N + 覆盖率 + 技术失败数 + 空键「缺身份」/无决策「未归一」单列 + 同一可比子集算误合/漏合/重复身份组率、零分母报「不适用」、未达标不启用生产。验证：`cd backend-go && go test ./cmd/entity-resolve-eval` 通过。

## 2. Review

- [x] T5 派只读子线程做 review（审查制品 + diff），重点核对：闸门是否只包「重读→二判→登记」且事务不跨模型；候选三路合并且「名字命中≠身份判定」；关闭态第一轮逐字不变；影子写入是否严格三处；评分口径是否可机械复算；意见按 High/Medium/Low 处置后在本 change 建 `review.md`。验证：`bash scripts/harness/review-trace.sh openspec/changes/normalize-event-subject-entities` 退出码 0。

## 3. 测试

- `cd backend-go && go test ./internal/tagmanagement/service/core ./internal/tagmanagement/service/entity ./internal/tagmanagement/repository ./internal/tagmanagement/models ./internal/topicgraph/service ./cmd/entity-resolve-eval` → 通过（仓储层只用隔离 PostgreSQL / testcontainer，不用 SQLite、不写真库）。
- `cd backend-go && golangci-lint run ./... && go vet ./... && go build ./...` → 全绿。
- 人工：按 `test-cases.md` §4 记录真实数字（固定总 N、覆盖率、技术失败数、误合 / 漏合 / 重复身份组率 / 重复新建率、统一 ID 稳定性、闸门等待与总解析时长、各阶段调用数与用量），**不预填结果、不写库**；结论写入 `evaluation-report.md`。

## 4. 文档

<!-- doc-impact: flow, database, configuration -->

- [x] 4.1 实现阶段更新 `docs/reference/flow/topic-graph.md`（红线 13/14：实体身份层存在但本期不参与事件线身份判定、判定路径不读实体 ID）、`docs/reference/flow/semantic-board.md`（辅助标签只作资料与叫法证据、GC 不损毁实体身份、auto alias 不构成同体证据）、`docs/reference/flow/ai-summary.md`（二判走 `topic_tagging` + 语义化 operation、技术失败不得降级 none、每次尝试写审计）、`docs/reference/flow/daily-report.md`（本期不参与事件线归属、人工归属不覆盖、composite 不产实体提及）。
- [x] 4.2 新增 `docs/reference/database/tables/entity-subjects.md` 并在 `docs/reference/database/_index.md` 登记 3 张新表（唯一约束、FK 与级联方向、**无类型列**、向量列无维度限定 + 行内记 model/版本/维度）；`docs/reference/configuration.md` 补三模式配置（默认 `off`，`active` 本期显式拒绝）；归档后按《开发执行规范》§12.2 补四处 flow 的变更溯源链接。

## 5. 验证

- `openspec validate normalize-event-subject-entities --strict` → 输出 valid。
- `bash scripts/harness/scenario-trace.sh openspec/changes/normalize-event-subject-entities` → Scenario 映射齐全（退出码 0；实现阶段测试文件落地后才可能为 0，制品阶段为预期的红）。
- `bash scripts/harness/check-tasks-tail.sh openspec/changes/normalize-event-subject-entities` → 尾三节与 doc-impact 声明齐全（退出码 0）。
- `bash scripts/harness/review-trace.sh openspec/changes/normalize-event-subject-entities` → 退出码 0（review 条目均已处置）。
- `bash scripts/harness/feedback.sh verify openspec/changes/normalize-event-subject-entities` → 退出码 0（条目均已消化）。
- `bash scripts/harness/change-scope.sh` → 机械判定影响包；**只测影响包，禁止顺手全量**。
- 人工：真实模型评测需**单独授权**后执行（上一轮 25 次 Chat 授权已消耗完，不得复用或自动扩额）；**未授权则不得勾选、不得以 fixture 冒充效果结论**。

| Scenario | 测试文件 |
|---|---|
| 实体身份与名称证据按层持久化 | backend-go/internal/tagmanagement/service/entity/entity_identity_test.go |
| 辅助标签被清理后实体身份仍然有效 | backend-go/internal/tagmanagement/service/entity/entity_repository_test.go |
| 同名不同实体可并存 | backend-go/internal/tagmanagement/service/entity/entity_identity_test.go |
| 新主体首次出现走完抽取到原子登记 | backend-go/internal/tagmanagement/service/entity/entity_resolution_test.go |
| 稳定短身份说明为空回退仅名检索 | backend-go/internal/tagmanagement/service/entity/entity_retrieval_test.go |
| 合法 none 与技术失败是不同结论 | backend-go/internal/tagmanagement/service/entity/entity_resolution_test.go |
| 后续跨叫法跨板块再次出现合为一个 ID 选项 | backend-go/internal/tagmanagement/service/entity/entity_candidate_test.go |
| 相关但不同的候选不被强合 | backend-go/internal/tagmanagement/service/entity/entity_candidate_test.go |
| 同来源同提及重放幂等 | backend-go/internal/tagmanagement/service/entity/entity_resolution_test.go |
| 跨来源并发在闸内重读最新候选再二判 | backend-go/internal/tagmanagement/service/entity/entity_gate_test.go |
| 取消超时异常释放闸门与连接 | backend-go/internal/tagmanagement/service/entity/entity_gate_test.go |
| 默认关闭态逐字不变 | backend-go/internal/tagmanagement/service/entity/entity_mode_test.go |
| 影子态写入限定三处 | backend-go/internal/tagmanagement/service/entity/entity_mode_test.go |
| 事件线身份判定不读实体 ID | backend-go/internal/topicgraph/service/story_subject_normalize_test.go |
| 评测协议与金标在运行前冻结并隔离 | backend-go/cmd/entity-resolve-eval/main_test.go |
| 预算超限在发请求前被拦截 | backend-go/cmd/entity-resolve-eval/main_test.go |
| 评分按固定总 N 与覆盖率并单列缺身份与未归一 | backend-go/cmd/entity-resolve-eval/main_test.go |
| 未达标不启用生产 | 人工：见 evaluation-report.md 的「结论」节与 T4 的未勾选状态 |
| 跨语言别名归并 | backend-go/internal/topicgraph/service/story_subject_normalize_test.go |
| 同公司不同写法归并 | backend-go/internal/topicgraph/service/story_subject_normalize_test.go |
| 不同主体不因向量相近而合并 | backend-go/internal/topicgraph/service/story_subject_normalize_test.go |
| 三层均不命中时新建 | backend-go/internal/topicgraph/service/story_subject_normalize_test.go |
| 影子态不改变事件线身份判定 | backend-go/internal/topicgraph/service/story_subject_normalize_test.go |
| 影子态同一次调用产出主体原文与稳定短身份说明 | backend-go/internal/tagmanagement/service/core/extractor_section_test.go |
| 关闭态第一轮提示与字段输出不变 | backend-go/internal/tagmanagement/service/core/extractor_section_test.go |
