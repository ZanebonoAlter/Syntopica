## Purpose

关注标记（watch）从只读命中提示升级出两条"物化"轨：keyword_topic 把当天含关键字的文章聚合为固定名称话题 section，sentence_topic 用一句话向量检索当天相关辅助标签、聚合为可持续延续的持久话题 section。物化 section 与常规聚类 section 并排出现在日报中。

## Requirements

### Requirement: 关键字轨物化生成

系统 SHALL 在日报生成时，对 board 下每个 `status=active` 且 `type=keyword_topic` 的关注标记执行物化：扫描当天发布窗口内的全部未归档文章，按该关注的关键字表达式（复用既有 DNF 语义：`|` 分隔 OR 组、空白分隔 AND 词、大小写不敏感字面匹配）匹配。扫描的文本层 SHALL 为严格版：文章标题 + 摘要类字段择优（AI 内容摘要 > 正文抓取 > 原文 > 描述），SHALL NOT 匹配正文全文。

命中文章 SHALL 经 **LLM 文章级裁决**（默认开启，可配置关闭）过滤：裁决以单次批量调用打包该关注当日全部命中文章（标题 + 摘要截断），判定每篇文章是否贴合该关注的追踪意图；仅裁决通过的文章聚合为一条 section（标题遵循「物化板块当日标题」requirement 的规则，兜底为关键字的固定派生名，如「关键字『harness』相关话题」）。裁决标准 SHALL 为**贴合追踪意图**（意图限定词 + 实质因果链：与追踪主题有直接因果关联的事件算贴合，仅平行存在或顺带提及不算），SHALL NOT 是字面关键词重复。每篇通过文章对应一条 thread（标题 + 文本层摘要 + 裁决置信度），thread 的 `confidence` SHALL 承载该篇的裁决置信度。

召回层（扫描、匹配、thread 文本组装）SHALL 保持零 AI；裁决层 SHALL 为单次（或少量分批）批量调用，SHALL NOT 对单篇文章单独发起请求。AI 返回的文章标识 SHALL 经合法候选集过滤（幻觉防护）。

裁决关闭或该关注当日命中文章为 0 时，行为与裁决前的机械聚合一致（关闭时全量聚合，0 命中无 section）。

当天无命中文章（或裁决后无通过文章）时，该关注 SHALL NOT 产出 section（自然消失，非报错）。

#### Scenario: 含关键字文章聚合为固定话题

- **GIVEN** board #5 存在 active 的 keyword_topic 关注，表达式为 `harness`
- **WHEN** 当天有 5 篇未归档文章的标题或摘要含 "harness"，其中 3 篇以 harness 为主题、2 篇仅顺带提及
- **THEN** 当期日报 SHALL 产出一条固定名称 section，包含 3 条通过裁决的 thread（confidence 为各篇裁决置信度），2 篇顺带提及的文章 SHALL NOT 进入板块

#### Scenario: 漏网文章可被捞回

- **GIVEN** 某篇文章未被任何 event tag 命中、也不属于任何聚类
- **WHEN** 其标题含关键字表达式全部词项且经裁决贴合追踪意图
- **THEN** 该文章 SHALL 出现在关键字物化 section 中

#### Scenario: 裁决批量单次请求

- **WHEN** 某关注当日命中 18 篇候选文章
- **THEN** 裁决 SHALL 以批量方式调用（单次或少量分批），SHALL NOT 对每篇文章单独发起请求

#### Scenario: 无命中不产空 section

- **WHEN** 当天没有任何文章命中关键字表达式，或全部候选被裁决剔除
- **THEN** 当期日报 SHALL NOT 出现该关注的物化 section

#### Scenario: 裁决关闭回退机械聚合

- **GIVEN** 裁决开关经配置关闭
- **WHEN** 当天有 5 篇文章命中关键字表达式
- **THEN** 5 篇全量聚合进 section，全程零 AI 调用，thread confidence SHALL 保持默认值

### Requirement: 一句话轨辅助标签检索

系统 SHALL 在关注创建时对 sentence_topic 的检索句生成一次 embedding 并缓存于关注记录；缓存缺失时（创建时生成失败、或检索句被更新后失效），下次日报生成 SHALL 惰性补算并回写。

日报生成时，系统 SHALL 用缓存的检索句向量在 board 绑定的辅助标签池（BoardComposition 关联的 SemanticLabel）内做余弦相似检索，取相似度不低于阈值（可配置）的 top-K 辅助标签为命中集；命中集经标签-tag 关联解析出 event tag，再限定为当天发布窗口内有文章的 tag，其文章并集构成**候选文章集**。

候选文章集 SHALL 经 **LLM 文章级裁决**（默认开启，可配置关闭）过滤：裁决以单次批量调用打包候选文章（标题 + 摘要截断），判定每篇是否贴合检索句的追踪意图（含意图限定词与实质因果链）；仅通过文章聚合为一条 section。thread 的 `confidence` SHALL 承载该篇的裁决置信度。AI 返回的文章标识 SHALL 经合法候选集过滤（幻觉防护）。

命中集为空、解析后当天无文章、或裁决后无通过文章时，该关注 SHALL NOT 产出 section。

#### Scenario: 检索命中并物化

- **GIVEN** board #5 存在 active 的 sentence_topic 关注（话题名「AI 编程工具进展」，检索句已缓存向量），辅助标签池中「AI 编程」标签向量与检索句余弦相似度超过阈值
- **WHEN** 「AI 编程」标签关联的 event tag 当天命中 4 篇文章，其中 3 篇经裁决贴合检索句意图
- **THEN** 当期日报 SHALL 产出该关注的 section，包含 3 条通过裁决的 thread（confidence 为各篇裁决置信度）

#### Scenario: 阈值过滤

- **WHEN** 辅助标签池中与检索句相似度最高的标签仍低于阈值
- **THEN** 该关注当期 SHALL NOT 产出 section

#### Scenario: 意图限定词参与裁决

- **GIVEN** sentence_topic 关注「美伊形势对市场影响」
- **WHEN** 候选文章含「美军打击伊朗，美油盘中涨近6%」（市场影响）与「伊朗婚礼现场遭袭」（人道新闻，无市场关联）
- **THEN** 前者 SHALL 通过裁决，后者 SHALL 被剔除

#### Scenario: 因果链判定

- **GIVEN** sentence_topic 关注「美伊形势对市场影响」
- **WHEN** 候选文章「沙特谴责伊朗在霍尔木兹海峡袭击其船只」为美伊军事行动的直接波及
- **THEN** 该文章 SHALL 通过裁决（实质因果链成立，虽未直接提及市场）

#### Scenario: 检索句更新后缓存失效

- **WHEN** 用户修改该关注的检索句
- **THEN** 系统 SHALL 使向量缓存失效；下次日报生成时用新检索句惰性补算并回写缓存

### Requirement: 一句话轨持久话题联动

sentence_topic 关注 SHALL 拥有一个专属持久话题（`source=manual`、`status=active`，首次物化时创建，label 取关注的话题名），其物化 section SHALL 归属该话题。该话题 SHALL 与普通手动话题一视同仁地参与后续日报的聚类锚定与自动归属，其生命周期（命中计数、连续命中、可见性）SHALL 遵循持久话题能力的既有规则推进，SHALL NOT 获得特殊阈值或豁免。

当期未产出物化 section 时，该话题 SHALL 按既有规则记为当日未命中（自然衰减，与普通话题一致）。

#### Scenario: 物化 section 推进话题延续

- **GIVEN** sentence_topic 关注的专属话题 T 已连续 2 天有物化 section
- **WHEN** 第 3 天物化 section 再次归属 T
- **THEN** T 的连续命中计数 SHALL 按持久话题既有规则 +1

#### Scenario: 物化话题作为聚类锚

- **GIVEN** sentence_topic 关注的专属话题 T 处于 active
- **WHEN** 后续日报聚类中某 event tag 的语义与 T 高度相近
- **THEN** 该 tag SHALL 按既有 lane 归属规则正常参与对 T 的归属判定，SHALL NOT 因 T 源自 watch 而被排除或放宽

#### Scenario: 无物化日自然衰减

- **WHEN** 某日检索句无命中、未产出物化 section
- **THEN** 专属话题 SHALL 按既有规则记当日未命中，SHALL NOT 保持虚假的连续命中

### Requirement: 物化 section 管线边界

物化 section SHALL 以 `lane_tier=watch_keyword`（关键字轨）或 `watch_sentence`（一句话轨）标记来源。物化 section SHALL NOT 参与同日 section 合并，SHALL NOT 参与 section 关系计算。section 自身的 article_count SHALL 如实反映其文章数；report 级聚合计数（article_count / event_tag_count / cluster_count）SHALL 保持常规聚类口径，SHALL NOT 因物化 section 重算。

#### Scenario: 不参与同日合并

- **GIVEN** 关键字物化 section 与某常规 section 语义高度相似
- **WHEN** 同日合并步骤执行
- **THEN** 物化 section SHALL NOT 被合并或改写

#### Scenario: 计数不重复

- **GIVEN** 某文章既在常规 section A 又在关键字物化 section W 中
- **WHEN** 日报保存
- **THEN** report 级 article_count SHALL 保持聚类口径不重复累计，section A 与 W 各自的 article_count SHALL 各自如实

### Requirement: 物化失败降级

任一物化轨在日报生成中失败 SHALL 分层降级，SHALL NOT 阻断日报生成与保存，SHALL NOT 使日报状态置为失败：

- **召回失败**（检索失败、数据解析失败等）：跳过该关注的当期物化并记录日志。
- **裁决失败**（AI 调用失败、响应解析失败）：SHALL 回退该关注当日的召回全量结果（关键词机械聚合 / 标签并集聚合），thread confidence SHALL 保持默认值，并记录日志。

#### Scenario: 单轨失败不阻断

- **WHEN** sentence_topic 检索调用失败
- **THEN** 该关注当期跳过，其余物化轨与日报流水线 SHALL 正常完成，日报 status SHALL 正常完成

#### Scenario: 裁决失败回退召回全量

- **GIVEN** sentence_topic 关注当日召回 12 篇候选文章
- **WHEN** 裁决 AI 调用失败
- **THEN** 当期该关注的 section SHALL 包含全部 12 篇（与裁决前行为一致），thread confidence 为默认值，降级 SHALL 记录日志


### Requirement: 删除关注联动

删除 keyword_topic 关注 SHALL 仅停止后续物化，历史物化 section SHALL 保留原样。删除 sentence_topic 关注 SHALL 要求用户显式确认，确认后归档其专属持久话题（历史物化 section 保留，归属不变），SHALL NOT 静默归档。

#### Scenario: 删除关键字轨保留历史

- **WHEN** 用户删除 keyword_topic 关注
- **THEN** 后续日报不再产出该物化 section，已保存日报中的历史物化 section SHALL 保留

#### Scenario: 删除一句话轨确认归档

- **WHEN** 用户删除 sentence_topic 关注并确认
- **THEN** 其专属持久话题 SHALL 被归档（archived），历史物化 section 保留且归属不变

### Requirement: 物化轨不参与命中提示

命中提示判定 SHALL 跳过全部物化轨关注（keyword_topic / sentence_topic SHALL NOT 产生命中记录），物化 section SHALL NOT 被提示轨扫描命中。既有 label / keyword 提示轨行为 SHALL 保持不变。

#### Scenario: 物化轨无命中记录

- **WHEN** 日报生成完成，board 下存在 keyword_topic 关注
- **THEN** 该关注 SHALL NOT 产生任何命中提示记录

### Requirement: 物化板块当日标题

物化板块的展示标题（`cluster_label`）SHALL 由 LLM 基于板块内当日通过裁决的文章事实生成贴合标题，SHALL 遵守事实锚约束（仅基于板块内文章实际内容，SHALL NOT 编造事件、数字或因果）。兜底链 SHALL 为：LLM 当日标题 → 关注名。标题生成 SHALL 与文章裁决共用或紧邻同一调用批次，SHALL NOT 为标题单独增加每日期内的额外调用轮次（裁决失败时标题同步走兜底链）。

section 序列化 SHALL 透出关注名装饰字段（transient，不入库），前端 SHALL 以既有物化板块徽标展示关注名，使用户在 LLM 标题之外始终可识别追踪源；系统 SHALL 维护物化 section 与其归属关注的关联（新物化 section 持久化关联，历史物化 section 按固定名称/标题回退解析）以支撑装饰字段读取。历史物化板块的标题 SHALL NOT 回刷。

#### Scenario: LLM 生成当日标题

- **GIVEN** sentence_topic 关注「美伊形势对市场影响」，当日板块聚焦原油与债市冲击
- **WHEN** 当日标题生成
- **THEN** `cluster_label` SHALL 为贴合当日内容的标题（如「美伊冲突升级推高油价、债市承压」），关注名 SHALL 经装饰字段透出

#### Scenario: 标题兜底链

- **WHEN** 当日裁决失败（降级回退全量）或 LLM 未返回可用标题
- **THEN** `cluster_label` SHALL 取关注名，板块与装饰展示 SHALL 正常

#### Scenario: 历史板块不回刷

- **WHEN** 本能力上线后查看历史日报
- **THEN** 历史物化板块标题 SHALL 保持原样，SHALL NOT 被重新生成
