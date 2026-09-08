## MODIFIED Requirements

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

## ADDED Requirements

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
