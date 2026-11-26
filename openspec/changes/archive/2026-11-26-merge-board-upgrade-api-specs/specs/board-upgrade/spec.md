## Purpose

辅助标签聚类升级为 SemanticBoard 的完整管线：候选收集 → 时间窗过滤 → 预聚类压缩 → LLM 单一决策空间判定（compose|skip）→ 用户确认执行。两个方向共用一套建议机制：**创建**新版块与**扩充**既有版块（扩充锁定单版块 + 相似/共现双路召回 + 版块画像二分类）。建议按 `suggestion_hash` 幂等，dismiss 后冷却期内不重生。本族只规定算法与业务判定；端点契约与前端面板见 `board-api`。

## ADDED Requirements

### Requirement: 扩充方向锁定单版块

<!-- from: board-upgrade-expand (2026-09-07 split-board-upgrade-directions) -->

版块扩充方向 SHALL 一次只针对一个用户选定的目标版块运行：用户从活跃版块下拉中单选目标后触发生成，该轮产生的所有扩充建议（单标签 merge 与组合 compose）的 target SHALL 恒等于该锁定版块。系统 SHALL NOT 在一轮生成中跨多个版块产扩充建议。

#### Scenario: 锁定版块生成
- **WHEN** 用户选定版块「美债」触发生成（扩充×单标签）
- **THEN** 该轮所有 merge_into_existing 建议 target_board_id SHALL 等于「美债」的 ID

#### Scenario: 目标版块被禁用后确认失败
- **WHEN** 建议生成后目标版块被禁用（status≠active），用户确认该建议
- **THEN** 确认 SHALL 失败并提示目标版块不可用，建议保持 pending

### Requirement: 扩充候选召回

<!-- from: board-upgrade-expand (2026-09-07 split-board-upgrade-directions) -->

版块扩充方向 SHALL 按来源召回与目标版块相关的候选，召回按以下优先级：

- **成员携带路（member-derived，主路）**：目标版块存在体检快照且含未收编候选时，候选 SHALL 来自体检快照的未收编候选清单（事实标签画像高频 aux），按候选清单频次排序取前 N 送 LLM 裁决。
- **全库召回（回退路）**：目标版块无体检快照或快照未含未收编候选时，回退为原有双路召回：
  - **单标签路**：候选为 active 且未挂载进目标版块的辅助标签，召回条件为（1）与目标版块 embedding 相似度达阈值（配置化），或（2）与目标版块现有构成标签在 co-tag 窗口内共现频次达标（配置化）。两种来源 SHALL 去重合并。
  - **组合路**：候选为满足 compose 共现门槛的标签对/三元组，且至少一个组件满足上述单标签召回条件（相似或与版块构成共现）。

生成请求携带 days>0 时 SHALL 按时间窗收紧召回：member-derived 路候选额外要求近 days 天文章中出现过；相似路候选额外要求近 days 天文章中出现过；共现路（含组合路）co-tag 窗口 cutoff 取 days 窗口与全局配置窗口中更严者。days=0 或未携带时召回 SHALL 与无时间窗行为完全一致。

召回结果 SHALL 按相关性排序后送 LLM 裁决；召回为空时 SHALL 返回空建议列表（正常结果，非错误）。扩充建议的确认执行保持用户逐条勾选子集语义不变。

#### Scenario: member-derived 候选被召回
- **WHEN** 目标版块「AI 前沿」体检快照含未收编候选「算力」（12 个成员 tags 携带），用户手动触发生成扩充建议
- **THEN** 「算力」SHALL 进入候选列表送 LLM 裁决，且不触发全库相似/共现扫描

#### Scenario: 无体检快照回退全库召回
- **WHEN** 目标版块「美债」从未跑过体检（无快照），用户手动触发生成扩充建议
- **THEN** 召回 SHALL 回退为原有相似路 + 共现路双路召回，行为与既有完全一致

#### Scenario: 相似标签被召回
- **WHEN** 目标版块「美债」扩充走回退路，aux「美债拍卖」与版块 embedding 相似度达阈值且未挂载
- **THEN** 「美债拍卖」SHALL 进入候选列表送 LLM 裁决

#### Scenario: 共现标签被召回
- **WHEN** 目标版块「美债」扩充走回退路，aux「国债期货」未挂载进「美债」，但与「美债」构成标签「美国国债」在窗口内共现达标
- **THEN** 「国债期货」SHALL 进入候选列表送 LLM 裁决

#### Scenario: 已挂载标签不召回
- **WHEN** aux「美联储」已在目标版块构成中
- **THEN**「美联储」SHALL NOT 出现在扩充候选里（member-derived 候选清单生成时已排除，回退路召回同样排除）

#### Scenario: 无关标签不被召回
- **WHEN** 扩充走回退路，aux「日本央行」与目标版块「美债」相似度未达阈值、且与版块构成标签无达标共现
- **THEN**「日本央行」SHALL NOT 进入候选（省 LLM 成本）

#### Scenario: 时间窗收紧 member-derived 召回
- **WHEN** 扩充请求携带 days=7，member-derived 候选「算力」的关联文章均在 7 天前
- **THEN**「算力」SHALL NOT 进入候选

#### Scenario: 时间窗收紧相似路召回
- **WHEN** 扩充请求携带 days=7（走回退路），aux「美债拍卖」相似度达标但其关联文章均在 7 天前
- **THEN**「美债拍卖」SHALL NOT 进入候选

#### Scenario: 时间窗收紧共现窗口
- **WHEN** 扩充请求携带 days=3，全局 CoTagWindowDays=30
- **THEN** 共现统计 SHALL 只计入最近 3 天的同文章共现

#### Scenario: 组合路候选相关性过滤
- **WHEN** 回退路组合路生成时，共现对「美国国债 × 收益率」中「美国国债」已在目标版块构成中（视为相关）
- **THEN** 该共现对 SHALL 进入候选；而与版块无任何相关组件的共现对 SHALL 被过滤

#### Scenario: 召回为空
- **WHEN** 目标版块无任何相关候选（member-derived 清单为空且回退路也无候选）
- **THEN** 系统 SHALL 返回空建议列表，不报错

#### Scenario: 确认执行保持逐条勾选
- **WHEN** 扩充建议列表展示多行候选，用户只勾选其中 2 个标签确认执行
- **THEN** 系统 SHALL 只执行勾选的 2 个标签的挂载，未勾选行不受影响

### Requirement: 版块画像上下文与二分类裁决

<!-- from: board-upgrade-expand (2026-09-07 split-board-upgrade-directions) -->

版块扩充方向的 LLM prompt SHALL 聚焦目标版块完整画像：版块名称与描述、现有构成标签列表（含组合标签）、近期内容标题（该版块近期匹配的 section 标题，数量配置化）。LLM 任务 SHALL 为二分类：每个候选「属于该版块」（单标签路 → merge_into_existing；组合路 → compose 携带目标）或「不属于」（skip）。LLM SHALL NOT 被要求在版块间比较或指定目标。

#### Scenario: 版块画像进 prompt
- **WHEN** 扩充「美债」版块触发 LLM 裁决
- **THEN** prompt SHALL 包含「美债」的描述、构成标签列表与近期内容标题，以及候选列表

#### Scenario: 二分类裁决
- **WHEN** 候选「美联储利率」与「日本央行殖利率」同轮送裁，LLM 判定前者属于「美债」后者不属于
- **THEN** 系统 SHALL 仅对「美联储利率」产出 merge 建议，「日本央行殖利率」skip 不落库

### Requirement: 候选按文章活动时间过滤

<!-- from: upgrade-candidate-time-window (2026-09-17 expand-upgrade-days-window) -->

系统 SHALL 在收集升级候选时，支持按文章活动时间（articles.created_at）过滤。只收集在指定时间窗口内的文章中，通过 article_topic_tags → topic_tag_semantic_labels 关联出现的候选辅助标签。时间过滤 SHALL 与 ref_count 阈值同时生效（双重过滤）。

#### Scenario: 默认只收集今天的候选
- **WHEN** 用户点击"升级建议"且未指定时间窗口
- **THEN** 系统 SHALL 只收集今天（now() - interval '1 day'）文章中出现的、满足 ref_count ≥ 5 且未归入已有 board 的辅助标签

#### Scenario: 用户选择最近7天
- **WHEN** 用户在时间窗口选择器中选择"最近7天"
- **THEN** 系统 SHALL 只收集最近7天文章中出现的候选辅助标签

#### Scenario: 用户选择全部
- **WHEN** 用户选择"全部"（days=0）
- **THEN** 系统 SHALL 不按时间过滤，收集所有满足 ref_count ≥ 5 且未归入已有 board 的辅助标签（行为与原系统相同）

#### Scenario: 时间过滤与引用门槛双重过滤
- **WHEN** 辅助标签 "华为"（ref_count=30）在今天的文章中出现，且未归入已有 board
- **THEN** 系统 SHALL 将其收集为候选（同时满足时间窗口和 ref_count 门槛）
- **WHEN** 辅助标签 "某冷门词"（ref_count=3）在今天的文章中出现
- **THEN** 系统 SHALL NOT 将其收集为候选（ref_count < 5，即使满足时间条件）

### Requirement: 升级建议 API 支持时间窗口参数

<!-- from: upgrade-candidate-time-window (2026-09-17 expand-upgrade-days-window) -->

`POST /api/semantic-boards/upgrade-suggest` SHALL 接受可选查询参数 `days`（int，默认 1）。`days > 0` 表示按最近 N 天的文章活动时间过滤候选；`days = 0` 表示不过滤。

#### Scenario: API 调用带 days 参数
- **WHEN** 前端调用 `POST /api/semantic-boards/upgrade-suggest?days=3`
- **THEN** 系统 SHALL 收集最近3天文章中出现的候选辅助标签，后续聚类和 LLM 判断流程不变

### Requirement: 前端时间窗口选择器

<!-- from: upgrade-candidate-time-window (2026-09-17 expand-upgrade-days-window) -->

系统 SHALL 在升级建议弹窗的生成参数区提供「候选时间窗」下拉选择器，选项为：今天（days=1，默认）、最近3天（days=3）、最近7天（days=7）、最近30天（days=30）、全部（days=0）。该选择器 SHALL 对全部四格方向组合（创建×单标签 / 创建×组合 / 扩充×单标签 / 扩充×组合）可用，选择后 SHALL 以查询参数形式随生成请求传递。

#### Scenario: 扩充方向显示时间窗选择器
- **WHEN** 用户在升级建议弹窗选择「版块扩充」方向
- **THEN** 「候选时间窗」下拉 SHALL 保持可见可选，与创建方向一致

#### Scenario: 用户切换时间窗口
- **WHEN** 用户从下拉选择器中选择"最近7天"并点击"生成建议"（任意方向组合）
- **THEN** 前端 SHALL 调用生成 API 并携带 days=7 查询参数

#### Scenario: 组合标签方向不受时间窗影响
- **WHEN** 用户选择「创建×组合」或「扩充×组合」并携带 days
- **THEN** 请求 SHALL 正常发出（组合路候选以共现门槛为准，时间窗仅作用于共现窗口收紧，无独立过滤语义）

### Requirement: 扩充候选的时间窗过滤

<!-- from: upgrade-candidate-time-window (2026-09-17 expand-upgrade-days-window) -->

版块扩充方向生成建议时，SHALL 支持可选查询参数 days（非负整数，默认按现状不过滤或前端默认值），并按以下规则作用于候选召回：

- **相似路**：与目标版块 embedding 相似达标的候选，SHALL 额外要求在 days 窗口内的文章中出现过（通过标签-文章关联判定）；days=0 时 SHALL NOT 施加该过滤。
- **共现路**：co-tag 窗口 cutoff SHALL 取「days 窗口」与「全局 CoTagWindowDays 配置窗口」中更严（更近）者；days=0 时 SHALL 使用全局配置窗口（与现状一致）。
- **组合路**：共现窗口收紧规则与共现路相同。

#### Scenario: days 收紧相似路
- **WHEN** 用户扩充版块「美债」且选择 days=7，aux「美债拍卖」相似度达标但近 7 天无文章引用
- **THEN**「美债拍卖」SHALL NOT 进入扩充候选

#### Scenario: days 收紧共现窗口
- **WHEN** 全局 CoTagWindowDays=30，用户选择 days=7
- **THEN** 共现统计窗口 SHALL 为最近 7 天（更严者生效）；选择 days=0 时 SHALL 保持 30 天窗口不变

#### Scenario: days=0 等于现状
- **WHEN** 用户扩充任一版块且选择"全部"（days=0）
- **THEN** 召回行为 SHALL 与本 change 之前完全一致（相似路全库、共现路用全局配置窗口）
