## Purpose

定义**事件线**（`board_stories`）与**事件**（`daily_report_threads`）的身份关系：一条事件线聚合多天的多个事件。目标是把「今天这条话题有几件事、哪件事有几个来源在报」变成可查询的事实，并让跨期去重有稳定的判定依据。身份判定以确定性规则为主、模型只处理指纹缺失或冲突的窄区间；向量只用于召回候选，不参与判定。术语与图谱见 [`../data-map.md`](../data-map.md)。

## ADDED Requirements

### Requirement: 事件指纹结构化产出
标签提取路径 SHALL 为每个 `category='event'` 的标签同时产出结构化指纹：`subject`（主体原文：公司/机构/人物/组织）、`subject_key`（归一化实体键，格式 `类型:规范名`，类型限于 `person` / `company` / `org` / `product` / `other`）、`action`（动作：发布/宣布/涨价/回应/下架等动词短语）、`object`（对象：产品/政策/资产/事项）。`occurred_at` MAY 产出，缺失时 SHALL 由该标签关联文章的最早 `pub_date` 代替。指纹 SHALL 与标签在同一次 LLM 调用内产出，SHALL NOT 为生成指纹新增独立调用。

`subject_key` SHALL 由模型在同一次调用内完成别名归并，使同一实体的不同写法（如 `Trump` / 川普 / 特朗普 / Donald Trump）产出同一个键。`subject_key` SHALL NOT 依赖 `topic_tags.aliases` 做归并——实测 person/event 的 aliases 覆盖率为 0%、keyword 仅 7.3%，不足以支撑。

指纹 SHALL 与已有 `topic_tags.metadata.event_keywords` 做一致性校验：`subject` 与 `object` 均 SHALL 能在该标签的 `event_keywords` 中找到（允许包含关系与别名展开）。校验不通过的标签 SHALL 标记为「指纹不可信」，且 SHALL NOT 阻断标签本身的入库与既有聚类流程。

#### Scenario: 同一次调用产出指纹
- **WHEN** 某栏目片触发一次融合提取调用并返回 3 个 event 标签
- **THEN** 每个 event 标签 SHALL 携带 subject/subject_key/action/object；系统 SHALL NOT 为该片发起第二次指纹提取调用

#### Scenario: 指纹与关键词一致
- **WHEN** 标签「阳光电源股价因业绩反转预期及政策利好暴涨」的 `event_keywords` 含「阳光电源」「股价暴涨」，模型输出 subject=阳光电源、object=股价
- **THEN** 该标签指纹 SHALL 标记为可信

#### Scenario: 指纹校验失败不阻断标签
- **WHEN** 模型输出的 subject 不在 `event_keywords` 中且无法通过别名展开匹配
- **THEN** 该标签 SHALL 照常入库并参与既有聚类，指纹 SHALL 标记为不可信并记录 warning；后续归属判定 SHALL 走降级路径

#### Scenario: 无关键词可校验时降级
- **WHEN** 某 event 标签的 `metadata.event_keywords` 缺失（存量约 0.3%）
- **THEN** 该标签指纹 SHALL 标记为不可信，SHALL NOT 因缺少校验依据而丢弃标签

### Requirement: 主体归一化

`subject_key` SHALL 按固定顺序匹配，前一层命中即终止：① `subject_key` 精确匹配（归一化为小写、下划线分隔）；② `subject_key` embedding 余弦相似度 ≥ 0.92；③ `subject` 字面归一（小写 / NFKC / 去空格 / 去「公司」「集团」等后缀）后精确匹配。三层均未命中的 SHALL 登记为新的主体候选。

`subject_key` embedding SHALL 仅用于兜同义拼写差异，SHALL NOT 用于事件判定。实体归并的相似度阈值 SHALL 显著高于事件召回阈值（0.92 vs 0.5），因为误归并两个不同主体的代价高于漏归并。

主体键 SHALL 存在于事件线上（`board_stories.subject_key`），SHALL NOT 在本期建设独立实体表；`subject` 原文 SHALL 保留用于展示。

#### Scenario: 跨语言别名归并
- **WHEN** 两次提取分别产出 subject=`特朗普` / `Trump`，subject_key 均为 `person:trump`
- **THEN** 两者 SHALL 判为同一主体，SHALL NOT 因原文字面不同而分开

#### Scenario: 同公司不同写法归并
- **WHEN** 两次提取分别产出 subject=`字节跳动` / `字节` / `ByteDance`
- **THEN** 三者 SHALL 归并为同一 `subject_key`（如 `company:bytedance`）

#### Scenario: 不同主体不因向量相近而合并
- **WHEN** 两个 subject 的 embedding 相似度为 0.85（低于 0.92），且字面不同
- **THEN** 系统 SHALL 判为不同主体，SHALL NOT 归并

#### Scenario: 三层均不命中时新建
- **WHEN** 新主体在精确键、embedding、字面三层均未命中
- **THEN** 系统 SHALL 登记新的 subject_key，SHALL NOT 复用相似度最高的已有键

### Requirement: 事件线数据模型
系统 SHALL 使用 `board_stories` 表承载事件线。每条记录 SHALL 至少包含：`id`、`board_id`（非空）、`lane_id`（可空，首次归属话题）、`title`、`subject`、`subject_key`、`action`、`object`、`occurred_at`（可空）、`status`（`active` / `settled` / `merged`）、`first_seen_date`、`last_seen_date`、`thread_count`、`article_count`、`participant_count`、`embedding`（可空，仅用于召回）、`subject_embedding`（可空，仅用于主体归并兜底）、`merged_into`（可空，指向存活事件线）、`origin`（`rule` / `model` / `manual`）、`created_at`、`updated_at`。

`daily_report_threads` SHALL 增加可空列 `story_id`。该列 SHALL NOT 设非空约束，SHALL NOT 建外键级联删除——事件线被合并时事件的归属 SHALL 通过 `merged_into` 解析到存活事件线，而非删除事件。

一个事件 SHALL 至多归属一条事件线。一条事件线 SHALL 聚合同一版块内多个 `period_date` 的事件。

#### Scenario: 事件未归组时字段为空
- **WHEN** 某事件尚未执行归属判定或判定失败
- **THEN** `story_id` SHALL 为 NULL，且该事件在日报、归档、矩阵中照常可见

#### Scenario: 事件线跨天聚合
- **WHEN** 同一事件在 11-05、11-07、11-09 各有 1 条事件且判定为同一事件线
- **THEN** 该事件线的 `thread_count` SHALL 为 3、`first_seen_date` 为 11-05、`last_seen_date` 为 11-09，且三条事件的 `story_id` 均指向该事件线

#### Scenario: 被合并的事件线保留地址
- **WHEN** 事件线 A 被判定与事件线 B 为同一事件并执行合并
- **THEN** A 的 `merged_into` SHALL 指向 B、`status` 为 `merged`，A 的事件 SHALL 在读取时解析到 B，SHALL NOT 出现指向已删除记录的死引用

### Requirement: 事件归属判定顺序
归属判定 SHALL 按固定优先级执行，前一层命中即终止，SHALL NOT 用后一层覆盖前一层的否定结论：

1. `subject_key` 不同 → SHALL 判为不同事件（新建事件线）；该判定 SHALL NOT 被向量相似度或 `object` 相近推翻。
2. **双方 `subject_key` 都非空时必须精确相等**（仅允许确定性归一化：大小写、全半角、空白、分隔符）；不相等即执行第 1 层否决，SHALL NOT 由 embedding 或字面相似度改写任一侧的显式键。
3. `subject_key`、`object`、`action` 均相同（`object` 允许别名与包含关系）→ SHALL 判为同一次发生，挂到同一事件线。
4. `subject_key`、`object` **严格相等**但 `action` 不同，且两侧事件日期（`occurred_at`，缺失时用日报 `period_date`）相差 ≤ 1 天 → SHALL 判为同一事件线的不同阶段（进展），挂到同一事件线，并 SHALL 在事件上标记 `is_development=true`；`action` 语义方向相反时（如「发布」对「召回/下架/否认」）SHALL NOT 套用本层。任一侧无日期时 SHALL NOT 套用本层。
5. `subject_key` 相同、`object` **严格不相等** → SHALL 判为不同事件（新建事件线）；此判定 SHALL NOT 下放模型。`object` 的宽匹配（包含关系）仅可用于判「不同」，SHALL NOT 用于判「相同」。
6. **合并闸门**：输入事件指纹不可信、或任一侧 `subject_key` 为空 → SHALL 直接新建事件线，SHALL NOT 交模型判定，且 SHALL NOT 合并。
7. 前三层无法判定且**双方指纹均可信**时 → MAY 交模型做三分类（`SAME_OCCURRENCE` / `UNRELATED` / `ROUNDUP`）仅用于综述稿识别；**模型判定 SHALL NOT 产生合并**——`SAME_OCCURRENCE` 与任何阶段结论均不得 attach 到既有事件线。

向量相似度 SHALL 仅用于召回候选事件线（放宽阈值、取 top N），SHALL NOT 单独作为合并判定依据。

> 本 change 的实现口径：**误合并优先于误拆分**。指纹不可信时宁可不合并（事件线退化为单事件），也不允许模型凭主题相似度合线。该取舍依据 design §9，并经真库七轮实测确认——放开模型合并时跨文样本误合并率 4/6，收紧后 0/1。

#### Scenario: 主体不同一票否决
- **WHEN** 新事件 subject_key=`company:a`、object=主力产品、action=涨价；候选事件线 subject_key=`company:b`、object=主力产品、action=涨价；两者向量距离极近
- **THEN** 系统 SHALL 判为不同事件并新建事件线，SHALL NOT 合并，且判定理由 SHALL 记录为「subject 不同」

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

### Requirement: 综合稿不建立事件身份
单篇文章包含 2 个及以上 `category='event'` 标签的 SHALL 视为综合稿。综合稿对应的事件 SHALL NOT 建立新事件线、SHALL NOT 增加已有事件线的 `thread_count` 与热度证据；它 MAY 作为「提及」出现在展示层，但 SHALL NOT 参与事件计数。

#### Scenario: 三事件综合稿不建身份
- **WHEN** 某事件的 `tag_ids` 含 3 个 event 标签（综合稿）
- **THEN** 该事件 SHALL 保持 `story_id = NULL` 并标记 `scope=composite`，SHALL NOT 计入任何事件线的 `thread_count`

#### Scenario: 单事件文章正常归组
- **WHEN** 某事件的 `tag_ids` 含 1 个 event 标签
- **THEN** 该事件 SHALL 走正常归属判定，可建立或加入事件线

### Requirement: 人工纠正优先
人工把事件指派到某条事件线、或标记「独立事件」后，该指派 SHALL 覆盖任何自动判定结果，且后续自动判定 SHALL NOT 改写该事件的 `story_id`。人工合并/拆分事件线后，被合并事件线的 `merged_into` 与事件归属 SHALL 保持稳定，SHALL NOT 被下一次回填或自动判定回滚。

#### Scenario: 人工指派不被自动覆盖
- **WHEN** 用户把事件 T 手工从事件线 A 移到事件线 B
- **THEN** 后续自动归属判定 SHALL 跳过 T；T 的 `story_id` 保持 B

#### Scenario: 人工拆分后不回滚
- **WHEN** 用户把事件线 S 中的事件 T 标记为独立事件
- **THEN** T 的 `story_id` SHALL 为 NULL 且 SHALL 记录人工标记；后续回填 SHALL NOT 重新把它挂回 S

### Requirement: 历史回填与降级
系统 SHALL 提供按版块 × 日期升序的存量回填命令，对 `story_id IS NULL` 且非综合稿的事件执行归属判定。回填 SHALL 幂等：重复执行 SHALL NOT 产生重复事件线、SHALL NOT 改写已有 `story_id`、SHALL NOT 重置人工标记。回填失败或未执行的事件 SHALL 在读取路径按「每个非综合稿事件记 1 条独立事件线、参与者按 feed 去重」保守计算，界面 SHALL 明确标注「事件身份未回填」，SHALL NOT 静默按 0 处理。

#### Scenario: 回填幂等
- **WHEN** 回填命令对同一批事件连续执行两次
- **THEN** 第二次 SHALL 跳过已有 `story_id` 的事件，事件线表 SHALL NOT 新增重复记录，计数 SHALL 与首次执行后一致

#### Scenario: 未回填时保守降级
- **WHEN** 某版块存在日报但全部事件的 `story_id` 为 NULL
- **THEN** 矩阵与雷达 SHALL 按非综合稿事件数作事件线数上界、按 feed 去重计算参与者，并显示「事件身份未回填」提示

#### Scenario: 回填不阻塞在线读写
- **WHEN** 回填命令运行期间用户正常访问矩阵与日报
- **THEN** 读取路径 SHALL 不受影响，不得出现锁等待超时或空白数据
