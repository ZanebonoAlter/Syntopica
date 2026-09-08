# test-cases: watch-materialize-llm-adjudication

## 故事 S1: 用户的追踪板块只聚合真正贴合意图的文章（锚 Requirement: 关键字轨物化生成 / 一句话轨辅助标签检索，均 MODIFIED）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| 1 | board 存在 keyword_topic 关注，当天 DNF 命中 5 篇（3 主题 2 顺带） | 含关键字文章聚合为固定话题 | 板块含 3 条通过裁决 thread，2 顺带被剔 | 函数单测（mock chat） | `watch_materialize_keyword_test.go`（改断言） |
| 2 | thread 逐条带 confidence=裁决置信度 | 同上 | confidence 非 1.0 默认、与 verdict 一致 | 函数单测 | 同上 |
| 3 | sentence 关注检索命中标签→文章并集 12 篇，其中 3 篇纯平行战报 | 检索命中并物化 / 意图限定词参与裁决 | 板块剔除 3 篇平行战报 | 函数单测（mock chat） | `watch_materialize_sentence_test.go`（改断言） |
| 4 | 候选「沙特谴责伊朗袭击其船只」（美伊行动波及） | 因果链判定 | 保留（因果链成立） | 函数单测 | `watch_materialize_sentence_test.go` |
| 5 | 无 event tag 的文章标题含全部词项且裁决贴合 | 漏网文章可被捞回 | 出现在板块 | 函数单测 | `watch_materialize_keyword_test.go`（改断言） |
| 6 | 裁决单次批量打包候选（18 篇一次送） | 裁决批量单次请求 | mock chat 断言调用次数=1 | 函数单测 | `watch_materialize_adjudicate_test.go`（新） |
| 7 | 连库生成一期日报（含裁决） | 检索命中并物化 | section/threads 落库、confidence 持久化 | testcontainer PG | `watch_materialize_integration_test.go`（扩展） |

### 变体走查

| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| 1 | 输入·候选 summary 为空（title-only） | 裁决输入退化为标题，正常裁决 | 单测 | adjudicate_test |
| 2 | 输入·summary 超长 | 截 200 runes 送裁，不爆 prompt | 单测 | adjudicate_test |
| 3 | 输入·特殊字符（正则元字符/emoji）在标题/摘要 | 原样送 LLM，无正则语义 | 单测 | adjudicate_test |
| 4 | 前置·召回 0 候选 | 不调 AI、不产 section（现状） | 单测 | keyword/sentence_test（照跑） |
| 5 | 前置·单候选 | 正常裁决 1 篇 | 单测 | adjudicate_test |
| 6 | 前置·全剔（所有 related=false） | 不产 section；sentence 话题自然衰减 | 单测 | keyword/sentence_test（补） |
| 7 | 前置·候选超 limit（40+） | 按 id 截断送裁 + Warn；关开关时不截断 | 单测 | adjudicate_test |
| 8 | 时间窗口 | 不适用——裁决层不涉时间窗（召回层既有测试覆盖），划除 |
| 9 | 幂等·同日重跑日报 | SaveReport 覆盖重建，裁决随管线整体重跑，无残留半状态 | 集成 | integration_test（既有覆盖语义补 confidence 字段断言） |
| 10 | 可用性·AI 失败（见 S3） | 降级全量板块 | 单测+集成 | 见 S3 |
| 11 | 可用性·空态=全剔无板块 | 日报其余部分正常，无空板块 | 单测 | 变体 6 |
| 12 | 可用性·超长文本展示 | 标题超长按 200 runes 截断（cluster_label 列宽） | 单测 | 标题两态断言（S4） |

## 故事 S2: 裁决 AI 挂掉或被关掉时，板块保底可用（锚 Requirement: 物化失败降级，MODIFIED）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| 1 | 裁决 chat 返回 error（召回 12 篇） | 裁决失败回退召回全量 | 板块含全 12 篇、confidence=1.0、Warn 日志 | 函数单测 | keyword/sentence_test（补降级分支） |
| 2 | 响应坏 JSON / verdicts 全幻觉 | 同上 | 同上回退 | 函数单测 | adjudicate_test |
| 3 | `watch_materialize_llm_filter_enabled=false` | 裁决关闭回退机械聚合 | 全量聚合、全程零 AI 调用（mock chat 断言 0 次）、confidence 默认 | 函数单测 | keyword/sentence_test（补开关分支） |
| 4 | sentence 检索本身失败（召回层） | 单轨失败不阻断 | 该 watch 跳过、日报正常完成（现状继承） | 集成 | integration_test（照跑） |
| 5 | 连库跑降级三态端到端 | — | 三态落库正确 | testcontainer PG | integration_test（扩展） |

### 变体走查

| # | 变体 | 期望答案 | 层 | 落点 |
| 1 | 误输入·ai_settings 键值非法（非数字/超界） | 忽略用默认值，不 panic | 单测 | config 单测（LoadWatchMaterializeConfig） |
| 2 | 空态·无物化关注 | 管线零调用 | 单测 | 既有早退（照跑） |
| 3 | 错误态·依赖挂了用户看到什么 | 降级全量板块（同旧版表现），非空板块非报错 | 单测+人工 | S2 步 1 |

## 故事 S3: 物化板块有当日贴合标题且追踪源可见（锚 Requirement: 物化板块当日标题，ADDED）

### 主链路

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| 1 | 裁决通过 3 篇（油价/债市主题） | LLM 生成当日标题 | cluster_label=贴合标题（事实锚），watch 名经装饰字段透出 | 单测 | keyword/sentence_test 标题断言 |
| 2 | LLM 未返回 section_title / 裁决降级 | 标题兜底链 | cluster_label=watch 名 | 单测 | 同上两态 |
| 3 | 日报详情 API 读路径 | section 序列化带 watch_label | `watch_*` 板块有值、常规板块空、不入库（transient） | handler/repository 测试 | `daily_report_repository_test.go`（补） |
| 4 | 前端板块渲染 | badge 装饰 | 「关键字物化板块 · harness」两态渲染 | Vitest 组件测试 | `SectionWatchBadge.test.ts`（改） |
| 5 | 历史板块查看 | 历史板块不回刷 | 旧标题原样 | 集成+人工 | integration_test（既有数据断言）+ 人工抽查 |

### 变体走查

| # | 变体 | 期望答案 | 层 | 落点 |
| 1 | 输入·LLM 标题超 200 runes | 截断落库 | 单测 | 标题断言 |
| 2 | 输入·LLM 标题为空串/纯空白 | 走兜底 watch 名 | 单测 | 两态断言 |
| 3 | 可用性·无 watch 名可解析（理论不达：物化板块必有归属 watch） | 兜底链最末=固定派生名 | 单测 | badge 可选 prop 空态 |

## 故事 S4: 物化轨关注不再出现在命中提示（锚 Requirement: 关注标记 AI 命中判定 MODIFIED + 物化轨不参与命中提示 既有）

### 主链路

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| 1 | board 下 label/keyword/keyword_topic/sentence_topic 四类 active 关注并存，日报生成完成 | 物化轨关注不进入判定 | 物化两类零 `topic_watch_hits`；label 走 AI、keyword 走文本（行为不变） | 函数单测（mock chat） | `daily_report_watch_test.go`（补分流三路断言） |
| 2 | 存量违规 hits 清理执行 | — | 物化轨 watch 的 hits 归零、label/keyword 历史不动 | testcontainer PG | 迁移/清理集成测试 |
| 3 | 前端时间线/详情索引展示 | 物化 watch 无重复预告 | 只剩合法轨命中 | 人工 | tasks 4.2 人工核对 |

### 变体走查

| # | 变体 | 期望答案 | 层 | 落点 |
| 1 | 前置·仅物化轨关注无 label/keyword | AI 组零调用、整体早退 | 单测 | daily_report_watch_test |
| 2 | 幂等·清理 SQL 重跑两次 | 第二次 0 行受影响 | 集成 | 清理测试 |

## 继承与调整（问句⓪，本 change 含 MODIFIED Requirements 必填）

| 旧 Scenario | 处置 | 旧测试文件 | 动作 |
| --- | --- | --- | --- |
| watch-materialized-topic: 含关键字文章聚合为固定话题 | 改语义（+裁决过滤与置信度） | `service/watch_materialize_keyword_test.go` | 改断言：3/5 通过、confidence、零 AI→单次 AI |
| watch-materialized-topic: 漏网文章可被捞回 | 改语义（裁决贴合才算捞回） | `watch_materialize_keyword_test.go` + `repository/topic_watch_repository_scan_test.go` | 前者改断言；后者照跑（扫描层不动） |
| watch-materialized-topic: 无命中不产空 section | 继承+扩展（全剔同语义） | `service/watch_materialize_integration_test.go` | 照跑 + 补全剔变体 |
| watch-materialized-topic: 检索命中并物化 | 改语义（并集→候选→裁决） | `service/watch_materialize_sentence_test.go` | 改断言 |
| watch-materialized-topic: 阈值过滤 | 继承（召回层不动） | `watch_materialize_sentence_test.go` + `repository/topic_watch_repository_sentence_test.go` | 照跑 |
| watch-materialized-topic: 检索句更新后缓存失效 | 继承（召回层不动） | `repository/topic_watch_repository_test.go` | 照跑 |
| watch-materialized-topic: 单轨失败不阻断 | 改语义（分层降级：召回跳过/裁决回退） | `service/watch_materialize_integration_test.go` | 照跑 + 补裁决失败回退分支 |
| topic-watch: 命中记录 / 不走双重确认 / 批量单次请求 | 继承（label 轨行为不变，仅适用范围收紧） | `service/daily_report_watch_test.go` | 照跑 + 补物化轨跳过断言 |

## 白盒附加（复杂档）

**裁决函数（adjudicateWatchArticles）分支表**

| 分支 | 输入 | 期望 |
| 正常·部分通过 | 5 候选 3 related=true | kept=3 + 各篇 confidence + title |
| 正常·全剔 | 全 related=false | kept 空 → 调用方不产 section |
| 幻觉 | verdict.article_id 不在候选集 | 该条丢弃，其余有效 |
| 坏 JSON | chat 返回非 JSON | error → 调用方降级全量 |
| 调用失败 | chat 返回 error | error → 降级全量 |
| 标题缺失 | section_title 空/空白 | 兜底 watch 名（调用方） |
| 超限 | 候选 > limit(40) | 按 id 截断 + Warn |
| 开关关闭 | enabled=false | 不调用 chat，直接返回全量标记 |

**边界值**：候选数 0 / 1 / 39 / 40 / 41；summary 长度 0 / 200 / 201 runes；confidence 0 / 0.5 / 1；标题长度 0 / 200 / 201 runes。

**分流（evaluateWatchHitsWithChat）分支表**

| type | 落组 |
| label | AI 判定组 |
| keyword | 文本匹配组 |
| keyword_topic / sentence_topic | 跳过（零 hits） |
| 无 active watch / 无可判 sections | 早退（既有） |

**不适用划除留痕**：时间窗口变体（裁决层不涉窗口，召回层既有覆盖）；并发变体（裁决在日报生成单线程管线内，无线程安全声称）。

## 效果核对（问句④）

- 触发原因：裁决质量依赖 LLM 实际行为，测试全 mock chat——绿灯 ≠ 裁决有效。
- 核对方法：tasks 4.2——本地连库生成一期真实日报，读裁决日志（候选数/通过数/剔除率/标题），人工通读三 watch 板块比对贴合度（对照 explore-findings 库内实证基线：美伊 09-02 期约 50% 跑偏）。
- 量化结果：剔除率落档 + 跑偏文章（纯平行战报/泛 AI 内容）占比目测下降。
- 结论：达标交付 ｜ 需调 prompt（版本化迭代）｜ 关开关回退。
