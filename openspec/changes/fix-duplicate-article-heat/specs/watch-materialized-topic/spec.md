## MODIFIED Requirements

### Requirement: 关键字轨物化生成

系统 SHALL 在日报生成时，对 board 下每个 `status=active` 且 `type=keyword_topic` 的关注标记执行物化：扫描**报告周期日发布窗口**内的全部未归档文章，按该关注的关键字表达式（复用既有 DNF 语义：`|` 分隔 OR 组、空白分隔 AND 词、大小写不敏感字面匹配）匹配。扫描的文本层 SHALL 为严格版：文章标题 + 摘要类字段择优（AI 内容摘要 > 正文抓取 > 原文 > 描述），SHALL NOT 匹配正文全文。

「报告周期日」SHALL 绑定该份日报的 `period_date`（北京时间自然日 00:00–24:00），SHALL NOT 由执行时刻（`time.Now()`）推导：当天按时生成、补档重建历史日报、手动按指定日期重建三条路径 SHALL 得到同一发布窗口。调用方 SHALL 显式传入报告日期；实现 SHALL NOT 保留「日期缺失/零值时退化为今天」的缺省路径。

命中文章 SHALL 经 **LLM 文章级裁决**（默认开启，可配置关闭）过滤：裁决以单次批量调用打包该关注当日全部命中文章（标题 + 摘要截断），判定每篇文章是否贴合该关注的追踪意图；仅裁决通过的文章聚合为一条 section（标题遵循「物化板块当日标题」requirement 的规则，兜底为关键字的固定派生名，如「关键字『harness』相关话题」）。裁决标准 SHALL 为**贴合追踪意图**（意图限定词 + 实质因果链：与追踪主题有直接因果关联的事件算贴合，仅平行存在或顺带提及不算），SHALL NOT 是字面关键词重复。每篇通过文章对应一条 thread（标题 + 文本层摘要 + 裁决置信度），thread 的 `confidence` SHALL 承载该篇的裁决置信度。

召回层（扫描、匹配、thread 文本组装）SHALL 保持零 AI；裁决层 SHALL 为单次（或少量分批）批量调用，SHALL NOT 对单篇文章单独发起请求。AI 返回的文章标识 SHALL 经合法候选集过滤（幻觉防护）。

裁决关闭或该关注当日命中文章为 0 时，行为与裁决前的机械聚合一致（关闭时全量聚合，0 命中无 section）。

当天无命中文章（或裁决后无通过文章）时，该关注 SHALL NOT 产出 section（自然消失，非报错）。

#### Scenario: 含关键字文章聚合为固定话题

- **GIVEN** board #5 存在 active 的 keyword_topic 关注，表达式为 `harness`
- **WHEN** 当天有 5 篇未归档文章的标题或摘要含 "harness"，其中 3 篇以 harness 为主题、2 篇仅顺带提及
- **THEN** 该关注 SHALL 产出 1 条 `lane_tier=watch_keyword` 的 section，只含裁决通过的 3 篇，每篇 1 条 thread

#### Scenario: 漏网文章可被捞回

- **GIVEN** 某篇文章未被任何 event tag 命中、也不属于任何聚类
- **WHEN** 其标题含关键字表达式全部词项且经裁决贴合追踪意图
- **THEN** 该文章 SHALL 出现在关键字物化 section 中

#### Scenario: 裁决批量单次请求

- **WHEN** 某关注当日命中 18 篇候选文章
- **THEN** 裁决 SHALL 以批量方式调用（单次或少量分批），SHALL NOT 对每篇文章单独发起请求

#### Scenario: 无命中不产空 section

- **WHEN** 当天没有任何文章命中关键字表达式，或全部候选被裁决剔除
- **THEN** 该关注 SHALL NOT 产出 section，SHALL NOT 以空 section 占位

#### Scenario: 裁决关闭回退机械聚合

- **GIVEN** 裁决开关经配置关闭
- **WHEN** 当天有 5 篇文章命中关键字表达式
- **THEN** 5 篇全量聚合进 section，全程零 AI 调用，thread confidence SHALL 保持默认值

#### Scenario: 补档扫描绑定报告日而非执行时刻

- **GIVEN** 系统在 D 日补档重建 D-3 日的日报，D-3 当日有 2 篇含关键字的文章，D 日另有 7 篇含关键字的文章
- **WHEN** 关键字物化轨执行
- **THEN** 该 section SHALL 只含 D-3 当日的 2 篇文章
- **AND** D 日的 7 篇文章 SHALL NOT 出现在 D-3 的报告中

#### Scenario: 手动按指定日期重建同口径

- **WHEN** 用户以 `POST /api/daily-reports/generate` 指定日期 D-1 重建
- **THEN** 关键字物化轨的扫描窗口 SHALL 为 D-1 自然日，与 D-1 当天按时生成的结果一致
