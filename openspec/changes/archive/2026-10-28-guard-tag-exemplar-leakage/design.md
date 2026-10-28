# Design: guard-tag-exemplar-leakage

## D1. 护栏语义：输出必须有「模型可见输入」中的依据

护栏判定不是「文章全文有没有提」，而是「**模型实际看到的文本**里有没有提」：

- mono 路径可见输入 = `ExtractionInput.Title + Summary`（Summary 已是采样后的提取输入）
- section 路径可见输入 = `ExtractionInput.Title + sectionTitle + sectionText`（模型只看到栏目片）

模型没看到的词却被输出，无论全文是否提及，都属于「无依据输出」（照抄示例或先验幻觉），丢弃均合理。这使护栏无需回读文章原始全文，保持在 extractor 层内闭合。

## D2. 挂点：两条 LLM 出口的解析后收口

不挂在纯解析函数（`parseMergedExtractionTags` / `parseSectionTags` 是无副作用的纯函数，改签名牵连纯解析测试），也不挂在编排层（heuristic 兜底、跨 feed 复用不经 LLM 出口，无需覆盖）。收口点：

- `extractMergedCandidates` 返回前对两组 tags 过滤（mono 路径，extractor_enhanced.go）
- `extractTagsFromSection` 的 `parseSectionTags` 成功后、return 前过滤（aggregate 路径，extractor_section.go）

两处调用同一个 `guardExemplarLeakage(tags []ExtractedTag, visibleText string, exemplarSlugs map[string]struct{}) []ExtractedTag`。被丢弃的 label 记入结果观测通道（mono 记 `Errors` 尾注 / section 记 warn 日志），不静默。

## D3. 示例词集合：两份 prompt 各自常量 + 防漂移测试

`buildMergedExtractionPrompt` 与 `buildSectionExtractionPrompt` 是两份独立 prompt，各自提取常量集合（标签 label 维度，alias/description 示例词不算）：

- merged：`PostgreSQL`、`Transformer架构`、`央行禁止比特币交易`、`某国央行上调基准利率`、`Sam Altman`、`央行`、`基准利率`、`利率上调`
- section：上表并 `Kubernetes`、`李飞飞`、`苹果WWDC 2024发布会`

防漂移：单测逐词断言「集合词在对应 prompt 文本中原文出现」。prompt 改示例词而忘改集合 → 测试红；集合加 prompt 里没有的词 → 同红。中文词如「央行」是真实高频词，护栏不误伤正文确有提及的（D1 的 Contains 检查放行），无提及时丢弃亦合理。

**匹配规则**：`Slugify(label) == Slugify(exemplar)`（两侧同函数归一，覆盖大小写变体如 `postgres`）。实现前须确认 `Slugify` 对中文的行为：若中文被剥成空串，则空串不入集合（跳过），中文示例词回退为 label 原词大小写不敏感等值比较——避免空串集合项误伤一切被剥空的标签。

**auxiliary_labels 过滤**：主标签保留时，其 aux 项按同规则逐项过滤（aux label 命中示例词且可见文本无提及 → 剔除该项）；主标签被丢弃时 aux 全部随之丢弃。

## D4. 存量清理：一次性幂等工具，不动 prompt（用户决策）

`backend-go/cmd/tagguard-cleanup/main.go`：

- 判定复用护栏同源逻辑：`article_topic_tags` 关联的 topic_tag label ∈ 示例词集合，且文章 `title+content+firecrawl_content` 拼接中 0 次出现（大小写不敏感）→ 待删
- 默认 `--dry-run` 打印清单（article_id / tag_id / label / 标题摘要）；`--apply` 才执行删除，删后调用既有 `CleanupOrphanedTags` 清孤儿
- 幂等：重复 `--apply` 第二次 dry-run 清单为空
- 预期规模 ~25 行（全部为 PostgreSQL 误挂）；11 行真实提及关联与 PostgreSQL 标签本体保留
- 不修改 prompt 示例词（用户决策：换词是换汤不换药，护栏是结构性兜底）

## D5. 旧测试处置原则

fixture 用了示例词而文章正文无提及的旧测试，断言的恰是泄漏行为，按新契约处置：正文 fixture 补上对应提及（保留原断言语义，顺带覆盖护栏放行分支），而非删断言。清单见 test-cases.md「继承与调整」表。
