## MODIFIED Requirements

### Requirement: 事件指纹结构化产出
标签提取路径 SHALL 为每个 `category='event'` 的标签同时产出结构化指纹：`subject`（主体原文：公司/机构/人物/组织）、`subject_key`（归一化实体键，格式 `类型:规范名`，类型限于 `person` / `company` / `org` / `product` / `other`）、`action`（动作：发布/宣布/涨价/回应/下架等动词短语）、`object`（对象：产品/政策/资产/事项）、`object_type`（对象角色，枚举 `entity` / `value`：`entity` = 被作用的对象（产品/政策/资产/事项），`value` = 事件参数/度量值（数值、金额、比率、时间点））。`occurred_at` MAY 产出，缺失时 SHALL 由该标签关联文章的最早 `pub_date` 代替。指纹 SHALL 与标签在同一次 LLM 调用内产出，SHALL NOT 为生成指纹新增独立调用。

`subject_key` SHALL 由模型在同一次调用内完成别名归并，使同一实体的不同写法（如 `Trump` / 川普 / 特朗普 / Donald Trump）产出同一个键。`subject_key` SHALL NOT 依赖 `topic_tags.aliases` 做归并——实测 person/event 的 aliases 覆盖率为 0%、keyword 仅 7.3%，不足以支撑。

指纹 SHALL 与已有 `topic_tags.metadata.event_keywords` 做一致性校验，校验范围 SHALL 按 `object_type` 区分：

- `subject` SHALL 能在该标签的 `event_keywords`（允许包含关系与别名展开）中找到；不满足即判为「指纹不可信」。**该要求对所有标签一律适用，SHALL NOT 因 `object_type` 而放宽。**
- `object` 的关键词命中要求 SHALL **仅在对象角色为 `entity` 时适用**；对象角色为 `value` 时，`object` 是事件的度量参数而非标签名用词，SHALL NOT 因其不在 `event_keywords` 中而判为不可信。
- 对象角色为 `value` 时，`object` SHALL 通过**确定性必要条件质检**：归一化后 SHALL 以数字开头；不满足 SHALL 视为角色声明不成立、按 `entity` 处理（回到关键词命中要求）。
- 对象角色缺失或取值非法时 SHALL 按 `entity` 处理。
- 校验不通过的标签 SHALL 标记为「指纹不可信」，且 SHALL NOT 阻断标签本身的入库与既有聚类流程。

角色判定 SHALL NOT 使用关键词黑名单、SHALL NOT 使用长度阈值式启发式：角色由模型在同一次调用内给出的结构化枚举决定，确定性语法只承担上述必要条件质检。

#### Scenario: 同一次调用产出指纹
- **WHEN** 某栏目片触发一次融合提取调用并返回 3 个 event 标签
- **THEN** 每个 event 标签 SHALL 携带 subject/subject_key/action/object/object_type；系统 SHALL NOT 为该片发起第二次指纹提取调用

#### Scenario: 指纹与关键词一致
- **WHEN** 标签「阳光电源股价因业绩反转预期及政策利好暴涨」的 `event_keywords` 含「阳光电源」「股价暴涨」，模型输出 subject=阳光电源、object=股价、object_type=entity
- **THEN** 该标签指纹 SHALL 标记为可信

#### Scenario: 指纹校验失败不阻断标签
- **WHEN** 模型输出的 subject 不在 `event_keywords` 中且无法通过别名展开匹配
- **THEN** 该标签 SHALL 照常入库并参与既有聚类，指纹 SHALL 标记为不可信并记录 warning；后续归属判定 SHALL 走降级路径

#### Scenario: 无关键词可校验时降级
- **WHEN** 某 event 标签的 `metadata.event_keywords` 缺失（存量约 0.3%）
- **THEN** 该标签指纹 SHALL 标记为不可信，SHALL NOT 因缺少校验依据而丢弃标签

#### Scenario: 参数型对象不再永久不可信
- **WHEN** 标签「人民币兑美元中间价下调」输出 subject=人民币兑美元中间价、subject_key=other:人民币兑美元中间价、action=下调、object=6.7253、object_type=value，而 `event_keywords` 为「人民币」「美元」「中间价」「下调」「汇率」
- **THEN** 该标签指纹 SHALL 标记为可信，SHALL NOT 因 object=6.7253 不在关键词中而永久判为不可信

#### Scenario: 参数型对象声明不成立时回落为实体型
- **WHEN** 某标签输出 object=中间价、object_type=value，但 object 归一化后不以数字开头
- **THEN** 系统 SHALL 按对象角色 `entity` 处理该 object，SHALL NOT 因角色声明而跳过关键词命中要求

#### Scenario: 存量指纹无角色字段时按实体型处理
- **WHEN** 某 event 标签的指纹没有 `object_type` 字段（存量指纹）
- **THEN** 系统 SHALL 按对象角色 `entity` 处理，SHALL NOT 猜测其角色

### Requirement: 事件线数据模型
系统 SHALL 使用 `board_stories` 表承载事件线。每条记录 SHALL 至少包含：`id`、`board_id`（非空）、`lane_id`（可空，首次归属话题）、`title`、`subject`、`subject_key`、`action`、`object`、`object_type`（可空，对象角色 `entity` / `value`，取值来自建线时事件的指纹；为空 SHALL 按 `entity` 处理）、`subject_entity_id`（可空，实体层主体身份；建线时取自事件标签的 `entity_id`，缺失或技术失败写 NULL）、`occurred_at`（可空）、`status`（`active` / `settled` / `merged`）、`first_seen_date`、`last_seen_date`、`thread_count`、`article_count`、`participant_count`、`embedding`（可空，仅用于召回）、`subject_embedding`（可空，仅用于主体归并兜底）、`merged_into`（可空，指向存活事件线）、`origin`（`rule` / `model` / `manual`）、`created_at`、`updated_at`。

`daily_report_threads` SHALL 增加可空列 `story_id`。该列 SHALL NOT 设非空约束，SHALL NOT 建外键级联删除——事件线被合并时事件的归属 SHALL 通过 `merged_into` 解析到存活事件线，而非删除事件。

一个事件 SHALL 至多归属一条事件线。一条事件线 SHALL 聚合同一版块内多个 `period_date` 的事件。

#### Scenario: 事件未归组时字段为空
- **WHEN** 某事件尚未执行归属判定或判定失败
- **THEN** `story_id` SHALL 为 NULL，且该事件在日报、归档、矩阵中照常可见

#### Scenario: 事件线跨天聚合
- **WHEN** 同一事件在 11-05、11-07、11-09 各有 1 条事件且判定为同一事件线
- **THEN** 该事件线的 `thread_count` SHALL 为 3、`first_seen_date` SHALL 为 11-05、`last_seen_date` SHALL 为 11-09，且三条事件的 `story_id` 均指向该事件线

#### Scenario: 建线时缺向量用标题现算
- **WHEN** 事件线索没有向量（`thread.Embedding` 为空），但事件标题非空
- **THEN** 系统 SHALL 用标题现算 embedding 写入 `board_stories.embedding`，SHALL NOT 以 NULL 建线后不再处理；现算失败时 MAY 仍以 NULL 建线并记 warning，SHALL NOT 因此阻断建线

#### Scenario: 存量无向量线可回填
- **WHEN** 运行 `backfill-stories --fill-story-embeddings`
- **THEN** SHALL 只为 `embedding IS NULL` 的事件线补向量，SHALL NOT 覆盖已有向量；`--dry-run` SHALL 只统计条数、不写库

#### Scenario: 被合并的事件线保留地址
- **WHEN** 事件线 A 被判定与事件线 B 为同一事件并执行合并
- **THEN** A 的 `merged_into` SHALL 指向 B、`status` 为 `merged`，A 的事件 SHALL 在读取时解析到 B，SHALL NOT 出现指向已删除记录的死引用

#### Scenario: 事件线记录对象角色
- **WHEN** 某事件线由一个 `object_type=value` 的事件建线
- **THEN** 该事件线的 `object_type` SHALL 记为 `value`，SHALL NOT 记为空或 `entity`；`object_type` 为空的事件线 SHALL 按 `entity` 参与判定

### Requirement: 事件归属判定顺序
归属判定 SHALL 按固定优先级执行，前一层命中即终止，SHALL NOT 用后一层覆盖前一层的否定结论。判定 SHALL 分**两层**执行，两层的分界是**证据日**：候选事件线的**最近证据日**（其最新事件所在报告的 `period_date`）与本次事件所属报告的 `period_date` 相同即走**层一（同一次发生）**，候选最近证据日更早即走**层二（跨天延续）**；候选最近证据日晚于本次报告日（乱序）SHALL 按层一处理；任一侧证据日缺失 SHALL 按层一处理（不得因证据不足而享受层二的放宽）。

**层一 · 同一次发生**（同一篇日报、同批文章切出的多段线索）SHALL 沿用既有五层顺序，判据**不得放松**：

1. `subject_key` 不同 → SHALL 判为不同事件（新建事件线）；该判定 SHALL NOT 被向量相似度或 `object` 相近推翻。
2. **主体身份否决**：两侧主体身份相同的条件是「`subject_key` 都非空且精确相等（仅允许确定性归一化：大小写、全半角、空白、分隔符）」**或**「两侧 `subject_entity_id` 均非空且相等，**且两侧 `subject` 归一化后精确相等（不得用包含关系）**」；两者都不满足 → SHALL 执行第 1 层否决，SHALL NOT 由 embedding 或字面相似度改写任一侧的显式键或实体 ID。裸 `subject_key`（仅类型前缀、无规范名，如 `other`）SHALL NOT 参与实体越过。实体 ID 相同且名称相容 SHALL 允许越过 `subject_key` 不相等（治键漂移）；`subject_key` 相等 SHALL NOT 因实体 ID 不同而被推翻（实体只做加法、不做减法）；任一侧 `subject_entity_id` 为 NULL 时 SHALL 回退到 `subject_key` 判断。
3. `subject_key`、`object`、`action` 均相同（`object` 允许别名与包含关系）→ SHALL 判为同一次发生，挂到同一事件线。
4. `subject_key`、`object` **严格相等**但 `action` 不同，且两侧事件日期（`occurred_at`，缺失时用日报 `period_date`）相差 ≤ 1 天 → SHALL 判为同一事件线的不同阶段（进展），挂到同一事件线，并 SHALL 在事件上标记 `is_development=true`；`action` 语义方向相反时（如「发布」对「召回/下架/否认」）SHALL NOT 套用本层。任一侧无日期时 SHALL NOT 套用本层。
5. `subject_key` 相同、`object` **严格不相等** → SHALL 判为不同事件（新建事件线）；此判定 SHALL NOT 下放模型。`object` 的宽匹配（包含关系）仅可用于判「不同」，SHALL NOT 用于判「相同」。
6. **合并闸门**：输入事件指纹不可信、或任一侧 `subject_key` 为空 → SHALL 直接新建事件线，SHALL NOT 交模型判定，且 SHALL NOT 合并。
7. 前三层无法判定且**双方指纹均可信**时 → MAY 交模型做三分类（`SAME_OCCURRENCE` / `UNRELATED` / `ROUNDUP`）仅用于综述稿识别；**模型判定 SHALL NOT 产生合并**——`SAME_OCCURRENCE` 与任何阶段结论均不得 attach 到既有事件线。

**层二 · 跨天延续** SHALL 以层一判据为基线，并只允许以下两处放宽，SHALL NOT 放宽层一、SHALL NOT 放宽 subject 一票否决、SHALL NOT 放宽合并闸门：

- **放宽 A（阶段推进的日期基准）**：层一第 4 层的「两侧事件日期相差 ≤ 1 天」在层二改用**候选事件线的最近证据日**与本次报告日比较（窗口仍为 ≤ 1 天），SHALL NOT 使用事件线的建线原点日作为基准。该放宽 SHALL NOT 使任何原先不成立的合并变得不成立（只放宽不收紧）。
- **放宽 B（参数型对象值可变）**：当输入事件与候选事件线的对象角色**同为 `value`** 时，`object` 的值不同 SHALL NOT 构成否决，SHALL 允许挂到同一事件线；当两侧角色不同（`value` 对 `entity`）或任一侧角色缺失时，SHALL 仍按层一第 5 层判为不同事件。`entity` 角色在层二 SHALL 仍要求 `object` 严格相等。
- 层二的阶段推进与放宽 B 的挂载 SHALL 在事件上标记 `is_development=true`。
- 层二 SHALL 额外要求：候选事件线与本次事件属于同一版块、`action` 非语义反向、两侧指纹均可信、两侧主体身份相同（同层一第 2 条）。层二 SHALL NOT 要求两侧存在共同文章或共同来源（跨天本无共同文章，共同来源是弱信号，会误伤换源报道的正常延续）。

向量相似度 SHALL 仅用于召回候选事件线（放宽阈值、取 top N），SHALL NOT 单独作为合并判定依据。

> 本 change 的实现口径：**误合并优先于误拆分**。指纹不可信时宁可不合并（事件线退化为单事件），也不允许模型凭主题相似度合线。该取舍依据 design §9，并经真库七轮实测确认——放开模型合并时跨文样本误合并率 4/6，收紧后 0/1。
>
> 跨天延续沿用同一取舍：层一只放宽两处明确的判据（放宽 A / 放宽 B），其余一律收紧或不变；向量相似度在任何一层都只用于召回。

#### Scenario: 主体不同一票否决
- **WHEN** 新事件 subject_key=`company:a`、object=主力产品、action=涨价；候选事件线 subject_key=`company:b`、object=主力产品、action=涨价；两者向量距离极近、且两侧 `subject_entity_id` 不同或缺失
- **THEN** 系统 SHALL 判为不同事件并新建事件线，SHALL NOT 合并，且判定理由 SHALL 记录为「subject 不同」

#### Scenario: 主体键漂移经实体归并合并
- **WHEN** 新事件 subject_key=`product:deepseek_harness`、`subject_entity_id=42`、subject=`DeepSeek Harness`；候选事件线 subject_key=`product:deepseekharness`、`subject_entity_id=42`、subject=`DeepSeek Harness 插件机制详解`；其余判据满足同一次发生
- **THEN** 系统 SHALL 判为同一主体并合并，SHALL NOT 因 `subject_key` 字面不等而新建事件线，判定理由 SHALL 记录为「实体身份相同」

#### Scenario: 实体相同但名称不相容时否决
- **WHEN** 两侧 `subject_entity_id` 相同（实体层误合并），但 subject 分别为「特朗普」与「梅拉尼娅·特朗普」（归一化后前者包含于后者但**不相等**）
- **THEN** 系统 SHALL NOT 视为同一主体，SHALL 按层一/层二的其他判据处理，且判定理由 SHALL 记录为「实体身份相同但名称不相等」

#### Scenario: 实体 ID 缺失回退 subject_key
- **WHEN** 新事件有 `subject_entity_id`、候选事件线 `subject_entity_id` 为 NULL，两侧 `subject_key` 相同
- **THEN** 系统 SHALL 按 `subject_key` 判为同一主体（既有行为），SHALL NOT 因候选缺实体 ID 而否决合并

#### Scenario: 实体 ID 不同不推翻既有 subject_key 相等
- **WHEN** 两侧 `subject_key` 精确相等，但 `subject_entity_id` 不同（实体层可能误拆）
- **THEN** 系统 SHALL 仍按 `subject_key` 判为同一主体（实体 ID 只做加法），并 SHALL 记录一条 warning 供复核

#### Scenario: 同一次发生合并
- **WHEN** 新事件与候选事件线的 subject_key/action/object 完全一致
- **THEN** 系统 SHALL 挂到该事件线，`thread_count` 递增，SHALL NOT 新建事件线

#### Scenario: 阶段推进标记为进展
- **WHEN** 候选事件线为「A 公司 · 发布 · A 产品」，新事件为「A 公司 · 开售 · A 产品」
- **THEN** 系统 SHALL 挂到同一事件线并标记该事件为进展，SHALL NOT 新建独立事件线

#### Scenario: 反向动作不走阶段规则
- **WHEN** 候选事件线为「A 公司 · 发布 · A 产品」，新事件为「A 公司 · 召回 · A 产品」
- **THEN** 系统 SHALL NOT 直接判为进展，SHALL 走模型判定；模型不可用时 SHALL 新建事件线

#### Scenario: 同主体不同对象分开
- **WHEN** 候选事件线为「A 公司 · 裁员 · 研发部门」，新事件为「A 公司 · 涨价 · 主力产品」
- **THEN** 系统 SHALL 判为不同事件并新建事件线

#### Scenario: 指纹缺失走模型且保守处理
- **WHEN** 新事件指纹不可信且候选事件线向量距离处于模糊区间
- **THEN** 系统 MAY 请求模型四分类；模型返回 `UNRELATED` 或 `ROUNDUP`、或置信度低于阈值、或调用失败时，SHALL 按不同事件新建事件线，SHALL NOT 默认合并

#### Scenario: 同日多段线索仍按严格判据分开
- **WHEN** 同一篇日报切出的两条线索主体相同、`object` 严格不相等（其中一条为 `value` 角色）
- **THEN** 系统 SHALL 判为不同事件并新建事件线，SHALL NOT 因参数型放宽而把两条线索并进同一条线

#### Scenario: 跨天延续按参数型对象值可变挂载
- **WHEN** 候选事件线为「人民币兑美元中间价 · 下调 · 6.7253」（`object_type=value`，最近证据日 11-03），本次事件为「人民币兑美元中间价 · 下调 · 6.7120」（`object_type=value`，报告日 11-04）
- **THEN** 系统 SHALL 挂到该事件线、标记该事件为进展，SHALL NOT 因 object 值不同而新建事件线

#### Scenario: 跨天延续不适用实体型对象
- **WHEN** 候选事件线为「A 公司 · 收购 · B 公司」（`object_type=entity`，最近证据日 11-03），本次事件为「A 公司 · 收购 · C 公司」（`object_type=entity`，报告日 11-04）
- **THEN** 系统 SHALL 判为不同事件并新建事件线，SHALL NOT 合并

#### Scenario: 跨天延续要求角色一致
- **WHEN** 候选事件线对象角色为 `value`、本次事件对象角色为 `entity`，其余判据相同
- **THEN** 系统 SHALL 判为不同事件并新建事件线，SHALL NOT 把两侧角色不同的对象并进同一条线

#### Scenario: 跨天延续以最近证据日判定相邻
- **WHEN** 候选事件线最近证据日为 11-02、本次报告日为 11-04（相差 2 天），其余判据满足层二
- **THEN** 系统 SHALL NOT 按放宽 A 把本次事件挂到该事件线，SHALL 新建事件线

#### Scenario: 证据日缺失按层一处理
- **WHEN** 候选事件线没有可用证据日（其最近证据的报告日缺失），其余判据满足层二
- **THEN** 系统 SHALL 按层一处理该候选，SHALL NOT 享受层二的参数型放宽与日期基准放宽

#### Scenario: 跨天延续不放松反向动作
- **WHEN** 候选事件线为「A 公司 · 发布 · A 产品」（最近证据日 11-03），本次事件为「A 公司 · 召回 · A 产品」（报告日 11-04）
- **THEN** 系统 SHALL NOT 合并，SHALL 走模型判定；模型不可用时 SHALL 新建事件线

## ADDED Requirements

### Requirement: 跨天延续的合并证据与可复核标记
跨天延续的挂载 SHALL 留下可复核的痕迹，SHALL NOT 只体现在计数上：

- 事件线 SHALL 以 `first_seen_date < last_seen_date` 标识「跨天事件线」；该标识 SHALL 可由事件线记录直接读出，SHALL NOT 需要另建标记表。
- 一次跨天延续挂载 SHALL 表现为：该事件所在报告的 `period_date` 晚于所属事件线的 `first_seen_date`。系统 SHALL NOT 把同日多段线索（同一篇日报切出的多条线索）记为跨天延续。
- 经层二放宽（阶段推进 / 参数型值变化）挂载的事件 SHALL 标记 `is_development=true`，表示该事件线在后续日期的延续（该事件线的一个新阶段）；层二的「指纹全同」挂载维持同一次发生语义、SHALL NOT 标记 `is_development`（保持跨期重述检测可用）。
- 判定理由 SHALL 以稳定枚举进入日志（区分同一次发生 / 同日阶段推进 / 跨天延续 / 参数型跨天延续 / 各类否决），SHALL NOT 只写自由文本。
- 人工纠正 SHALL 继续优先：`story_manual=true` 的事件 SHALL NOT 被任何自动判定改写，跨天延续 SHALL NOT 覆盖人工标记。

#### Scenario: 跨天事件线可被识别
- **WHEN** 某事件线聚合了 11-03 与 11-04 两个报告日的事件
- **THEN** 该事件线 SHALL 满足 `first_seen_date < last_seen_date`，SHALL 可被按此条件直接查出

#### Scenario: 同日多段线索不算跨天延续
- **WHEN** 同一篇日报切出的两条线索被挂到同一条事件线
- **THEN** 该事件线的 `first_seen_date` SHALL 等于 `last_seen_date`，SHALL NOT 被计为跨天延续

#### Scenario: 跨天延续标记为进展
- **WHEN** 某事件经层二放宽（阶段推进或参数型值变化）挂到已有事件线
- **THEN** 该事件的 `is_development` SHALL 为真，SHALL NOT 被跨期去重降级为快讯

#### Scenario: 人工标记不被跨天延续覆盖
- **WHEN** 某事件已被人工标记为独立事件（`story_manual=true`）
- **THEN** 跨天延续判定 SHALL 跳过该事件，SHALL NOT 改写其 `story_id`

### Requirement: 跨天延续的存量重算与快照回滚
系统 SHALL 提供**显式开关**的存量身份重算能力，SHALL NOT 自动改写历史事件身份：

- 默认（无开关）时，既有事件身份 SHALL 保持原样，历史事件线 SHALL NOT 被自动合并或改写；跨天延续只对开关生效之后新归属的事件与显式重算范围内的数据生效。
- 显式重算 SHALL 只处理指定版块与日期范围内、`story_manual=false` 的事件；人工标记的事件与人工创建的事件线 SHALL NOT 被重算改写。
- 重算 SHALL 按版块 × 日期升序执行，SHALL 幂等：重复执行 SHALL NOT 产生重复事件线、SHALL NOT 改写人工标记。
- 写操作前 SHALL 支持导出快照（事件 → 事件线归属映射），SHALL 支持用该快照还原；快照还原 SHALL 只影响事件身份，SHALL NOT 触及日报正文、文章与标签。
- 预览模式 SHALL 产出可核对的统计（处理线程数、新建线数、挂载数、合并数、跳过数），并 SHALL 能列出跨天延续的样本对（供人工判定是否同一事件），SHALL NOT 写库。

#### Scenario: 默认不改写历史
- **WHEN** 不带重算开关执行存量回填
- **THEN** 既有事件的 `story_id` 与既有事件线 SHALL 保持不变，SHALL NOT 自动跨天合并历史事件线

#### Scenario: 显式重算只动非人工事件
- **WHEN** 对某版块某日期范围执行显式重算，范围内含 `story_manual=true` 的事件与 `origin='manual'` 的事件线
- **THEN** 人工标记的事件归属与人工事件线 SHALL 保持不变，SHALL NOT 被重算改写

#### Scenario: 重算幂等
- **WHEN** 对同一范围连续执行两次显式重算
- **THEN** 第二次 SHALL NOT 新增重复事件线，事件归属与计数 SHALL 与首次执行后一致

#### Scenario: 快照可还原身份
- **WHEN** 写操作前导出快照，随后执行重算，再用该快照还原
- **THEN** 事件 → 事件线的归属 SHALL 回到快照记录的状态，SHALL NOT 触及日报正文与文章
