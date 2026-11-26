## Purpose

辅助标签是可人工编辑的标签描述层，用于提升版块匹配质量。本族定义它怎么维护——模型与生命周期、层级质量校验、合并建议与合并界面、清理回收、身份保护。

## ADDED Requirements

### Requirement: 辅助标签垃圾回收（GC）

<!-- from: aux-label-gc -->

系统 SHALL 支持扫描并清理无活跃 topic_tag 引用的辅助标签。清理时 SHALL 跳过 protected=true、创建时间在 grace_days 内、或 status 非 active 的标签。清理策略支持四种模式：dry_run（仅预览）、disable（软删除）、delete（硬删除）、recalculate（仅重算 ref_count）。

#### Scenario: GC dry_run 预览
- **WHEN** 用户调用 GC API mode=dry_run, grace_days=1
- **THEN** 系统 SHALL 返回符合条件的标签数量和前 20 个示例列表，不修改任何数据

#### Scenario: GC disable 软删除
- **WHEN** 用户调用 GC API mode=disable, grace_days=1
- **THEN** 系统 SHALL 将符合条件的辅助标签 status 更新为 "disabled"，同时删除 `board_composition` 中引用这些标签的行，并返回实际影响的标签数量

#### Scenario: GC delete 硬删除
- **WHEN** 用户调用 GC API mode=delete, grace_days=1
- **THEN** 系统 SHALL 硬删除符合条件的辅助标签记录（含 embedding 向量，CASCADE 自动清理 board_composition），并返回实际删除的标签数量

#### Scenario: GC recalculate 存量校准
- **WHEN** 用户调用 GC API mode=recalculate
- **THEN** 系统 SHALL 对所有 label_type='auxiliary' 的 semantic_labels 执行 ref_count 全量重算（基于 topic_tag_semantic_labels 中的实际计数），并返回重算的标签数量和修正数量

#### Scenario: 跳过有活跃引用的标签
- **WHEN** auxiliary label "AI" 在 topic_tag_semantic_labels 中有关联行
- **THEN** 该标签 SHALL NOT 被 GC 清理

#### Scenario: 跳过 protected 标签
- **WHEN** auxiliary label "AI" 的 protected=true，即使无任何 topic_tag 引用
- **THEN** 该标签 SHALL NOT 被 GC 清理

#### Scenario: 跳过 grace_days 内新建标签
- **WHEN** auxiliary label "量子计算" 创建于 12 小时前，无任何 topic_tag 引用，grace_days=1
- **THEN** 该标签 SHALL NOT 被 GC 清理

#### Scenario: 清理后减少匹配负载
- **WHEN** 100 个无引用的 auxiliary label 被 GC disable
- **THEN** 后续 `loadActiveAuxiliaryLabels` 和 board 匹配 SHALL 排除这些 disabled 标签

#### Scenario: GC disable 清理 board_composition
- **WHEN** auxiliary label #42 被 GC disable，且 `board_composition` 中存在 board_id=5, auxiliary_label_id=42 的行
- **THEN** 该 `board_composition` 行 SHALL 被删除，board #5 的 composition 页面不再显示该标签

### Requirement: AuxLabelCleanup 定时任务

<!-- from: aux-label-gc -->

系统 SHALL 提供每小时执行一次的 AuxLabelCleanup 定时任务，自动对无活跃引用的辅助标签执行 disable 清理。任务 SHALL 通过现有调度器框架管理，支持查看状态、手动触发、重置统计、修改间隔。

#### Scenario: 定时自动执行
- **WHEN** AuxLabelCleanup 调度器运行中，到达下一次执行时间
- **THEN** 系统 SHALL 执行 mode=disable, grace_days=1 的 GC 清理，记录执行结果到 SchedulerTask 数据库记录

#### Scenario: 手动触发执行
- **WHEN** 用户在 GlobalSettings 定时任务面板点击 "手动执行" AuxLabelCleanup
- **THEN** 系统 SHALL 立即执行一次 GC 清理，返回执行结果

#### Scenario: 调度器状态查询
- **WHEN** 用户打开 GlobalSettings 定时任务面板
- **THEN** AuxLabelCleanup 任务 SHALL 显示名称、状态、执行次数、上次执行时间、执行耗时等统计信息

#### Scenario: 调度器跳过重复执行
- **WHEN** AuxLabelCleanup 正在执行中，到达下一次定时触发时间
- **THEN** 系统 SHALL 跳过本次执行，等待下次周期

### Requirement: 手动 GC API

<!-- from: aux-label-gc -->

系统 SHALL 提供 `POST /api/auxiliary-labels/gc` 端点，允许手动触发 GC 操作，支持指定 mode（dry_run / disable / delete / recalculate）和 grace_days 参数。

#### Scenario: 手动 dry_run
- **WHEN** POST /api/auxiliary-labels/gc body={"mode":"dry_run","grace_days":1}
- **THEN** 返回 eligible_count 和 preview 列表，不修改数据

#### Scenario: 手动 disable
- **WHEN** POST /api/auxiliary-labels/gc body={"mode":"disable","grace_days":1}
- **THEN** 符合条件的标签 status 设为 "disabled"，关联 board_composition 行删除，返回 affected_count

#### Scenario: 手动 delete
- **WHEN** POST /api/auxiliary-labels/gc body={"mode":"delete","grace_days":1}
- **THEN** 符合条件的标签被硬删除，返回 affected_count

#### Scenario: 手动 recalculate
- **WHEN** POST /api/auxiliary-labels/gc body={"mode":"recalculate"}
- **THEN** 所有 auxiliary label 的 ref_count 被重算，返回 total_count 和 corrected_count

#### Scenario: 参数校验
- **WHEN** mode 不为 dry_run / disable / delete / recalculate 之一
- **THEN** 返回 400 错误

#### Scenario: grace_days 默认值
- **WHEN** 请求未提供 grace_days
- **THEN** 系统 SHALL 使用默认值 1

#### Scenario: recalculate 忽略 grace_days
- **WHEN** mode=recalculate
- **THEN** 系统 SHALL 忽略 grace_days 和 protected/status 过滤条件，对所有 auxiliary label 执行 ref_count 重算

### Requirement: 共享描述修复须人工确认

<!-- from: auxiliary-label-description-repair (2026-11-06 board-health-triage-advice) -->


系统 SHALL 提供从体检问题项进入的辅助标签描述修复能力：展示实时原描述、建议文案、可编辑新描述、直接引用版块及全局共享影响。描述须 trim 后非空、不仅重复 label、不超过 500 字符。只有用户明确确认才可创建修复任务；只查看建议/打开页面/运行体检 MUST NOT 创建任务。disabled、非 auxiliary 或不存在的标签 MUST 被拒绝。

#### Scenario: 核对并确认共享影响
- **WHEN** 用户确认将「上海」描述从商业场所表述改为城市定义
- **THEN** 系统 SHALL 在提交前声明是共享标签、使用它的版块均受影响、匹配更新覆盖全部活跃话题标签，用户取消时零写入

#### Scenario: 无效描述与禁用标签
- **WHEN** 用户提交空白、超过 500 字符、仅重复标签名的描述，或标签已 disabled/非 auxiliary
- **THEN** 系统 SHALL 返回明确错误且不修改描述、向量或匹配，不将任务显示为成功

### Requirement: 描述与 storage 向量原子更新

<!-- from: auxiliary-label-description-repair (2026-11-06 board-health-triage-advice) -->


系统 SHALL 使用最终确认的新描述生成 storage embedding（label + ': ' + description），生成成功后在事务中校验来源版本、类型与状态并原子保存 description+embedding。MUST NOT 清空旧向量后复用 NULL 回填；MUST NOT 修改 label、aliases、merge_embedding、组合标签向量。生成/保存失败时旧描述和旧向量均保持。并发修改冲突 MUST 明确拒绝并引导重新核对，不能覆盖较新描述。

#### Scenario: 生成失败保持旧数据
- **WHEN** 修复任务生成新向量失败
- **THEN** 原描述与原 storage/merge 向量 SHALL 保持，任务记录失败原因并可重试

#### Scenario: 修改说明不改变标签身份
- **WHEN** 新描述与 storage 向量提交成功
- **THEN** label/aliases/merge_embedding SHALL 原值不变，description 与 storage 向量均为对应的新版本，不出现半更新

#### Scenario: 来源版本已变化
- **WHEN** 用户核对后、事务保存前标签已被其他操作修改或禁用
- **THEN** 系统 SHALL 拒绝覆盖，记录冲突或不可用状态；界面保留草稿并要求加载最新版本重新核对

### Requirement: 修复任务持久且幂等

<!-- from: auxiliary-label-description-repair (2026-11-06 board-health-triage-advice) -->


任务 SHALL 持久记录请求幂等键、标签及来源版本、用户确认的新描述、执行阶段、保存版本、匹配范围与进度、失败原因。相同幂等键重试 MUST 返回同一任务；同标签未结束任务不能并发覆盖。页面关闭不取消已确认任务，worker SHALL 绑定应用生命周期而非提交请求的 context。服务重启后须可查询并恢复失租的未结束阶段；明确失败态 SHALL 等待用户重试，不自动无限重跑。不得凭内存 job 或组件本地状态宣称可恢复。

#### Scenario: 双击与请求重试不重复执行
- **WHEN** 同一提交因双击/超时使用相同幂等键重发
- **THEN** 系统 SHALL 返回同一任务，不重复重嵌、不重复提交描述

#### Scenario: 离开或服务重启后再进入
- **WHEN** 用户关闭页面或服务重启后查询已确认的任务
- **THEN** 系统 SHALL 返回持久阶段与结果；已保存的描述不再次覆盖，未完成匹配可继续或明确重试

#### Scenario: 运行中与完成后的重试幂等
- **WHEN** 用户重复重试已运行或已完成的任务
- **THEN** 系统 SHALL 返回202及同一任务当前状态，不再排队、不启动重复worker、不修改范围/计数/时间；conflict/unavailable则明确409要求重新核对，不借retry修改原描述请求或来源版本

#### Scenario: 失败等待明确重试
- **WHEN** 已确认任务进入embedding_failed、save_failed或matching_failed
- **THEN** 后台 SHALL 不自动无限重跑；用户retry只重新排入同一任务，已提交描述只续匹配，旧失败清空后展示新尝试

### Requirement: 修复后更新匹配并独立重试

<!-- from: auxiliary-label-description-repair (2026-11-06 board-health-triage-advice) -->


描述与新向量提交后 SHALL 失效 board 匹配缓存，并用既有匹配引擎更新任务范围内全部活跃 topic_tags 的版块匹配，包含当前未归属任何版块的标签；任务范围在执行起点固定并可恢复，后续新标签沿既有实时匹配路径处理。MUST NOT 仅枚举现有匹配行或只处理当前版块，MUST NOT 改匹配算法或重嵌全部文章。任务 SHALL 返回冻结总数 total、真实成功 processed、失败 failed、不再适用 not_applicable 与未完成 remaining；所有计数由服务端提供，四类之和 SHALL 等于 total。成功指实际调用匹配引擎且正常返回（包括合法零命中）；未调用、报错或失效跳过 MUST NOT 冒充成功。冻结后被删除、禁用或合并的范围项 SHALL 持久记录为不再适用，total 保持不变，不永久卡死任务。部分失败不得标全完成。匹配失败后仅重试失败或未完成匹配，不重嵌/再次保存描述。

#### Scenario: 未归属标签也进入更新范围
- **WHEN** 修复时某活跃 topic_tag 在匹配表没有任何行
- **THEN** 该标签 SHALL 仍被纳入本次匹配范围并由既有匹配引擎处理

#### Scenario: 匹配失败只重试匹配
- **WHEN** 描述和向量已保存，但部分 tag 匹配失败
- **THEN** 任务 SHALL 标部分完成并返回计数；用户重试只继续未完成匹配，embedding 调用次数不再增加

#### Scenario: 冻结后标签不再适用
- **WHEN** 冻结范围中的标签在处理前已删除、禁用或合并
- **THEN** 系统 SHALL 不调用该项匹配，持久记录不再适用，not_applicable 增加而 processed/failed 不增加，total 不缩小；其余适用项仍正常执行

#### Scenario: 成功执行可合法零命中
- **WHEN** 既有匹配引擎对适用标签成功执行并返回空命中结果
- **THEN** 系统 SHALL 按成功执行计入 processed，不伪造版块命中，不将正常空结果误报失败

### Requirement: 更新完成与效果验证分离

<!-- from: auxiliary-label-description-repair (2026-11-06 board-health-triage-advice) -->


界面 SHALL 展示生成向量、原子保存、匹配更新三个真实阶段及可执行恢复动作。仅在冻结范围所有仍适用项均匹配成功、其余项均有明确不再适用记录且无失败/未完成项后才显示「更新完成，效果待下一轮体检验证」，并引导处理下一项。旧体检快照、既有日报正文、历史评分 MUST 保持，不因修复被覆盖；不自动触发额外体检，不把复制、已排队、描述已保存或部分完成当成效果已改善。

#### Scenario: 结果不伪造健康改善
- **WHEN** 修复任务成功结束
- **THEN** 用户 SHALL 看到匹配更新完成与下一步处理入口，同时知道效果待下一轮体检验证；旧评分和旧快照不变

### Requirement: Slug generation normalizes whitespace

<!-- from: tag-hierarchy-quality (2026-05-10 fix-tag-hierarchy-inflation) -->

The `Slugify` function SHALL collapse consecutive whitespace characters into a single space before applying any other transformations, so that labels differing only in whitespace (e.g., "DeepSeek首轮融资" and "DeepSeek 首轮融资") produce the same slug.

#### Scenario: Labels with space versus no-space produce same slug
- **WHEN** `Slugify("DeepSeek首轮融资")` and `Slugify("DeepSeek 首轮融资")` are called
- **THEN** both SHALL return `deepseek 首轮融资`

#### Scenario: Multiple consecutive spaces collapse to single space
- **WHEN** `Slugify("foo   bar")` is called
- **THEN** the result SHALL be `foo bar`

#### Scenario: Leading and trailing whitespace trimmed
- **WHEN** `Slugify("  hello world  ")` is called
- **THEN** the result SHALL be `hello world` (spaces preserved between words, punctuation removed)

### Requirement: Abstract tag creation requires minimum information gain

<!-- from: tag-hierarchy-quality (2026-05-10 fix-tag-hierarchy-inflation) -->

Before creating a new abstract tag, the system SHALL verify that:
1. At least 2 distinct child tags are proposed
2. No pair of proposed child tags shares more than 70% of their associated articles (Jaccard similarity)
3. The resulting tree SHALL have a leaf-to-depth ratio of at least 1.5

If any of these checks fail, the abstract tag creation SHALL be rejected and logged.

#### Scenario: Too few children rejected
- **WHEN** LLM proposes an abstract tag with only 1 candidate child
- **THEN** the system SHALL reject creation and log a warning

#### Scenario: High article overlap rejected
- **WHEN** two candidate child tags share 80% of their associated articles
- **THEN** the system SHALL reject abstract creation and log a warning suggesting merge instead

#### Scenario: Acceptable abstract passes all checks
- **WHEN** LLM proposes an abstract with 3 children whose maximum pairwise Jaccard is 0.3 and the resulting tree leaf-to-depth ratio is 2.0
- **THEN** the system SHALL proceed with abstract tag creation

### Requirement: Existing whitespace-variant duplicate tags are cleaned up

<!-- from: tag-hierarchy-quality (2026-05-10 fix-tag-hierarchy-inflation) -->

A scheduled cleanup task SHALL detect active tags whose slugs would be identical after whitespace normalization (`\s+` → single space) and merge them, migrating article associations, embeddings, and hierarchy relations to the surviving tag.

#### Scenario: Whitespace variant pair merged
- **WHEN** cleanup runs and finds two active tags "DeepSeek首轮融资" (slug: `deepseek首轮融资`) and "DeepSeek 首轮融资" (slug: `deepseek 首轮融资`) that are the same after normalization
- **THEN** the tag with fewer article associations SHALL be merged into the other (marking it `status=merged`, `merged_into_id` set), all article_topic_tags SHALL be migrated, and duplicate article-tag pairs SHALL be skipped

### Requirement: Degenerate abstract trees are flattened

<!-- from: tag-hierarchy-quality (2026-05-10 fix-tag-hierarchy-inflation) -->

A scheduled cleanup task SHALL detect abstract tag chains where the leaf-to-depth ratio falls below 1.5 and flatten them by removing intermediate abstract nodes, relinking children to the nearest ancestor that provides meaningful grouping.

#### Scenario: Four-level chain with 5 leaves flattened
- **WHEN** cleanup encounters A→B→C→D with D's children being 5 leaf tags
- **THEN** intermediate nodes B and C SHALL be deactivated and their children linked directly to A

### Requirement: Multi-parent assignments are prevented

<!-- from: tag-hierarchy-quality (2026-05-10 fix-tag-hierarchy-inflation) -->

When linking a child tag to a new abstract parent via `linkAbstractParentChild()`, if the child already has an active abstract parent and the new parent's article set overlaps with the existing parent's article set (Jaccard > 0.3), the second parent assignment SHALL be rejected.

#### Scenario: Second parent rejected due to overlap
- **WHEN** tag X has parent A covering articles {1,2,3} and a new parent B covering articles {1,2,4} is proposed (Jaccard = 0.5)
- **THEN** the system SHALL reject assigning B as parent and log the conflict

#### Scenario: Second parent accepted for non-overlapping sets
- **WHEN** tag X has parent A covering articles {1,2} and a new parent B covering articles {3,4,5} is proposed (Jaccard = 0)
- **THEN** the system SHALL accept B as an additional parent

### Requirement: tag_merge_suggestions 表

<!-- from: tag-merge-suggestions -->


系统 SHALL 维护 `tag_merge_suggestions` 表，记录可能需要合并的相似标签对。

#### Schema
- `id`: PK, auto-increment
- `new_tag_id`: FK → topic_tags，被提取的新标签
- `existing_tag_id`: FK → topic_tags，相似的已有标签
- `new_label`: text，新标签的 label
- `existing_label`: text，已有标签的 label
- `category`: text，标签类别
- `similarity`: float，embedding 相似度 (0-1)
- `status`: text，枚举 `pending` / `merged` / `dismissed`，默认 `pending`
- `source`: text，来源 `incremental` / `full_scan`
- `created_at`: timestamp
- `updated_at`: timestamp

#### Constraints
- UNIQUE (`new_tag_id`, `existing_tag_id`)
- INDEX (`status`, `similarity` DESC)

### Requirement: 增量记录合并建议

<!-- from: tag-merge-suggestions -->


`findOrCreateTag` 创建新 tag 时，如果 `TagMatch` 返回 candidates，SHALL 将候选对写入 `tag_merge_suggestions`。

#### Scenario: 新标签有相似候选
- **WHEN** `findOrCreateTag` 的 `TagMatch` 返回 `candidates`
- **AND** 新 tag 创建成功（获得 new_tag_id）
- **THEN** 对每个 candidate 写入一条 `tag_merge_suggestion`（status=pending, source=incremental）
- **AND** 以 `(new_tag_id, candidate_tag_id)` 为唯一键，已存在则 skip

#### Scenario: 新标签无相似候选
- **WHEN** `TagMatch` 返回 `no_match` 或 `exact`
- **THEN** 不写入 suggestion

### Requirement: 异步全量扫描

<!-- from: tag-merge-suggestions -->


用户可手动触发全量扫描，遍历所有 active tag，每个 tag 调用 `FindSimilarTags` 查找相似对，结果写入 `tag_merge_suggestions`。

#### Scenario: 触发全量扫描
- **WHEN** 收到 `POST /merge-preview/scan` 请求
- **THEN** 后端在 goroutine 中启动扫描，立即返回 202
- **AND** 遍历所有 active tag，每个 tag 调用 `FindSimilarTags(tag, category, 10, 'semantic')`
- **AND** 相似度 ≥ LowSimilarity 的结果写入 suggestion（source=full_scan）
- **AND** 已存在的对 skip，不重复写入

#### Scenario: 并发保护
- **WHEN** 已有一个扫描任务在运行
- **AND** 收到新的 `POST /merge-preview/scan` 请求
- **THEN** 返回 409 "scan already in progress"

### Requirement: SSE 进度推送

<!-- from: tag-merge-suggestions -->


全量扫描通过 `GET /merge-preview/scan/stream`（SSE）实时推送进度。

#### Message Format
```json
{
  "status": "scanning",
  "total": 590,
  "scanned": 342,
  "current_category": "keyword",
  "new_suggestions": 23
}
```

扫描完成后发送最终消息：
```json
{
  "status": "done",
  "total": 590,
  "new_suggestions": 47
}
```

#### Scenario: 客户端连接 SSE
- **WHEN** 前端创建 `EventSource('/api/topic-tags/merge-preview/scan/stream')`
- **THEN** 收到 Content-Type: text/event-stream 响应
- **AND** 扫描进行中持续收到 progress 事件
- **AND** 扫描完成后收到 done 事件并关闭

### Requirement: 查询合并建议

<!-- from: tag-merge-suggestions -->


`GET /merge-preview` 改为查询 `tag_merge_suggestions` 表。

#### Scenario: 查询 pending 建议
- **WHEN** 前端调用 `GET /merge-preview`
- **THEN** 返回 `tag_merge_suggestions` 中 status=pending 的记录
- **AND** 按 similarity DESC 排序
- **AND** 附带 source/new_tag 的文章数量统计

### Requirement: 合并后标记 suggestion

<!-- from: tag-merge-suggestions -->


合并操作完成后，SHALL 将相关的 suggestion 标记为 merged。

#### Scenario: 合并后标记
- **WHEN** `POST /merge-with-name` 完成合并（source→target）
- **THEN** 更新 `tag_merge_suggestions` 中 `new_tag_id=source` 或 `existing_tag_id=source` 的 pending 记录为 merged
- **AND** 同时更新涉及 target 的 pending 记录为 merged（因为 target 可能被重命名）

### Requirement: TagsPage 提供标签合并入口

<!-- from: tag-merge-ui -->


TagsPage 左侧栏操作按钮区 SHALL 包含"标签合并"按钮，点击后显示 `TagMergePreview` 组件。

#### Scenario: 点击标签合并按钮
- **WHEN** 用户在 TagsPage 点击左侧栏"标签合并"按钮
- **THEN** 弹出 TagMergePreview 面板
- **AND** 自动从 `tag_merge_suggestions` 表加载 pending 状态的候选对
- **AND** 用户可逐个确认合并、跳过、交换合并方向、编辑合并后名称

#### Scenario: 合并完成
- **WHEN** 用户完成合并操作并关闭面板
- **THEN** TagsPage 刷新标签数据以反映合并结果
- **AND** 已合并的 suggestion 被标记为 `merged` 状态

#### Scenario: 忽略建议
- **WHEN** 用户点击某个候选对的"忽略"按钮
- **THEN** 该 suggestion 被标记为 `dismissed` 状态
- **AND** 不再出现在候选列表中

### Requirement: 异步全量扫描 + SSE 进度展示

<!-- from: tag-merge-ui -->


用户可手动触发全量扫描，发现老标签之间的相似对。扫描异步执行，进度通过 SSE 实时推送到前端。

#### Scenario: 触发全量扫描
- **WHEN** 用户点击"全量扫描"按钮
- **THEN** 后端启动异步扫描任务（同时只允许一个任务运行）
- **AND** 前端通过 SSE 接收进度更新

#### Scenario: SSE 进度推送
- **WHEN** 全量扫描正在运行
- **THEN** 前端实时展示：进度条（scanned/total）、当前处理的类别、已发现的新建议数
- **AND** 扫描完成后自动刷新候选列表

#### Scenario: 扫描已完成时再次触发
- **WHEN** 用户在扫描进行中再次点击"全量扫描"
- **THEN** 返回错误提示"扫描正在进行中"

### Requirement: Embedding 高相似度匹配降级为 candidates

<!-- from: tag-match-identity-protection -->


`TagMatch` 方法中，当 embedding 余弦相似度达到 `HighSimilarity` 阈值时，SHALL 返回 `MatchType: "candidates"`（而非 `"exact"`），将匹配结果作为候选列表返回。`findOrCreateTag` 对 `candidates` 的已有行为是 fall through 到创建新 tag。

#### Scenario: embedding 高相似度不再返回 exact
- **WHEN** 一个标签通过 embedding 搜索匹配到已有标签，cosine 相似度为 0.96（≥ HighSimilarity 0.97）
- **THEN** `TagMatch` 返回 `MatchType: "candidates"`，`ExistingTag` 为 nil，`Candidates` 包含匹配结果
- **AND** `findOrCreateTag` 创建新的独立 tag

#### Scenario: slug 精确匹配仍返回 exact
- **WHEN** `TagMatch` 通过 slug+category 在数据库中找到完全匹配的记录
- **THEN** 返回 `MatchType: "exact"`，`ExistingTag` 为匹配到的标签
- **AND** `findOrCreateTag` 正常合并（更新 label/slug 等）

#### Scenario: alias 精确匹配仍返回 exact
- **WHEN** `TagMatch` 通过 alias 字段匹配到已有标签
- **THEN** 返回 `MatchType: "exact"`，`ExistingTag` 为匹配到的标签
- **AND** `findOrCreateTag` 正常合并

### Requirement: 删除 keyword 类别阈值覆盖

<!-- from: tag-match-identity-protection -->


`CategoryThresholdOverrides` map SHALL NOT 包含 `"keyword"` 键。所有类别的 embedding 匹配统一使用 `DefaultThresholds`（`HighSimilarity: 0.97, LowSimilarity: 0.78`）。

#### Scenario: keyword 类别使用默认阈值
- **WHEN** `ThresholdsForCategory("keyword")` 被调用
- **THEN** 返回 `DefaultThresholds`（`HighSimilarity: 0.97, LowSimilarity: 0.78`）
