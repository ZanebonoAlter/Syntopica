# Design: fix-tagging-pollution

## Context

见 proposal.md Why 节。补充调查期实锤事实（详见 `docs/research/minicpm5-local-tagging/explore-findings.md`）：

- 污染链路一（llm 来源泛词）：`extractor_enhanced.go:59-61` 在 LLM 返回空 `keyword_tags` 时调 `heuristicKeywordCandidates(input)`（= `ExtractTopics` 规则匹配）回填候选，且最终结果统一标 `Source: "llm"`（:92）。`ExtractTopics` 将分类名直接作为标签（score 0.65）——「华尔街见闻」「资讯_凤凰网」两个 feed 的分类名均为「新闻」，对应 624 行 llm 来源「新闻」。
- 污染链路二（heuristic 来源）：`article_tagger.go:143` `if err != nil || len(result.Tags) == 0` 将零结果视为失败降级 `legacyExtractTopics`，存量 293 篇 / 484 行。此外 extractor 内部还有一个零候选降级点：`extractor_enhanced.go:67` `len(candidates) == 0` 即调 `extractWithHeuristic`——移除链路一回填后，「两数组均空」场景 candidates 必空、走此分支变成非空 heuristic 结果（err=nil），仅改 tagger 层条件断不了此路径，须一并处理（见 D2）。
- 「Coding」（tag_id 27，llm 来源约 90 行、全部凤凰网文章）的写入路径尚未完全定位：规则模式词（coding/code/编程/开发）在现存 content 中无匹配，且垃圾行先于模型结果落库（毫秒时间戳证明两个写入批次）。本 change 以排查任务收口，根因大概率同属「历史内容状态下的一次抽取」。

## Goals / Non-Goals

**Goals**

- 空 LLM 结果（任一数组为空、或整体为零候选）不再被规则词/启发式结果替换
- heuristic 兜底范围收窄到「调用失败」
- 泛词标签在任何写入路径都无法入库（单一拦截点）
- 存量污染行清理 + 受影响文章重打，量化留痕

**Non-Goals**

- 不改聚合路径（aggregate）的切片提取逻辑本身
- 不改 heuristic 规则匹配器的内容（仅在编排层决定何时调用它）
- 不做标签 UI、不做黑名单配置化（代码常量即可，词表不扩容）
- provider timeout / mimo P2 绑定等纯配置项（管理界面操作，tasks 验证节留痕）

## Decisions

**D1 回填移除点：`extractor_enhanced.go` ExtractTags 内，而非 `heuristicKeywordCandidates` 删除**

移除 :59-61 的 `if len(keywordTags) == 0 { ... keywordTags = heuristicKeywordCandidates(input) }` 回填分支，`heuristicKeywordCandidates` 函数体一并删除（唯一调用方即该处）。备选：保留函数、只在回填处标记来源为 heuristic——否决，因为回填词（分类名「新闻」）本身就不该成为文章标签，不是来源标注问题。

**D2 兜底条件收窄：extractor 与 tagger 两层各一处**

heuristic 降级共三个触发点，两处需改、一处保留：

- `article_tagger.go:143`：`if err != nil || len(result.Tags) == 0` 改为 `if err != nil`。err==nil 且零候选时直接走 `if len(tags) == 0 { return nil }`（现有代码已覆盖），文章保持无标签
- `extractor_enhanced.go:67`：`if len(candidates) == 0 { return te.extractWithHeuristic(...) }` 整支移除，改为原样返回零候选结果（`Tags: []`、`Source: "llm"`、Errors 照记）。不移除的话，「两数组均空」场景在 extractor 内部就变成非空 heuristic 结果（err=nil、Tags 非空），tagger 层的 `len(result.Tags) == 0` 收窄根本等不到触发，链路二在此场景原样存活
- 保留：`extractor_enhanced.go:52` 提取调用 err != nil 的降级（属「调用失败」，符合新契约）

配套细节：`branchErrors` 中 `"keyword extraction failed: empty keyword array"` 措辞随「空=合法结论」调整（不再称 failed）；`article_tagger.go:126-128` aggregate 回落注释「so aggregate articles never end up with zero tags」随新行为更新（回落后 mono 也可能返回零标签）。

备选：零结果时外层重试一次——否决，extractor 内部已有 3 次重试预算，外层重试是对「模型判空」的不信任重掷，浪费且引入方差。

**D3 泛词拦截点：`persistArticleTags` 入口单一过滤器 + reuse 查询过滤**

新增 `filterGenericLabels(tags []TopicTag) []TopicTag`，在 `persistArticleTags` 入口调用——mono / aggregate / heuristic 三条路径的汇聚点（比在 resolveCandidate/mergeExtractedTags 各自拦截更不易漏）。第四条写入路径 `reuseTagsFromSiblingArticle`（article_tagger.go:263）直接复制 sibling 关联行、不经 persistArticleTags，须在其 siblingLinks 查询处 join topic_tags 排除黑名单 label——否则拦截只覆盖 3/4 条路径，存量清理后看似干净实为「数据层碰巧兜住」，未来任何新漏入的泛词行会被 reuse 持续复制扩散。黑名单为包内常量切片：`新闻、论坛、要闻、快讯、文章、内容、技术、发展`，**完整匹配**（Slugify 后比较），不做子串匹配——「GLM Coding Plan」等正常标签不受影响；「Coding」不进黑名单（编程文章的合法主题词），其治理靠 D1/D2 断根 + D4 排查 + D5 定向清理。

**D4 「Coding」写入路径排查**

tasks 中列为独立排查任务：对比受污染凤凰网文章的 `updated_at`/content 变更时间线与标签落库时间，确认是否「初版内容含模式词 → 抽取 → content 被爬虫替换」的时序；若证实为时序问题，D1/D2 已断根，无需额外代码；若发现独立写入路径，升级为独立修复（必要时追加 change）。

**D5 存量清理 SQL（含安全红线流程）**

```sql
-- ① 备份（bash 侧执行）
-- pg_dump -t article_topic_tags > backup_att_<date>.sql
-- ② count 留底（三项各自留数：heuristic=484 / llm 黑名单泛词≈714 / llm Coding≈90）
SELECT count(*) FROM article_topic_tags WHERE source='heuristic'
UNION ALL SELECT count(*) FROM article_topic_tags att JOIN topic_tags tt ON tt.id=att.topic_tag_id
  WHERE att.source='llm' AND tt.label IN ('新闻','论坛','要闻','快讯','文章','内容','技术','发展')
UNION ALL SELECT count(*) FROM article_topic_tags
  WHERE source='llm' AND topic_tag_id = 27;  -- 「Coding」，存量全部为凤凰网垃圾行
-- ③ BEGIN 内预览 → 确认后 COMMIT
BEGIN;
DELETE FROM article_topic_tags WHERE source='heuristic';
DELETE FROM article_topic_tags att USING topic_tags tt
  WHERE att.topic_tag_id = tt.id AND att.source='llm'
    AND tt.label IN ('新闻','论坛','要闻','快讯','文章','内容','技术','发展');
DELETE FROM article_topic_tags WHERE source='llm' AND topic_tag_id = 27;  -- Coding 不入黑名单（编程文章合法主题词），仅按存量垃圾定向清除
-- ROLLBACK;  -- 预览后回滚，确认数字再真跑
```

受影响文章（清理后零标签者）由既有 tag 队列周期自动重打（队列以无标签文章为输入），无需专门脚本；期望规模约 300-400 篇 × 2s/篇。

## Risks / Trade-offs

- **快讯文章无标签状态增多**：内容稀薄的文章将诚实无标签（此前被「新闻」假填充）。语义版块匹配以标签为输入，短期命中率可能略降；换来辅助标签池不再被稀释，长期匹配质量提升。可接受，属产品意图（宁缺毋滥）。
- **黑名单误伤风险**：理论上存在标题恰为「技术」的合法文章标签——完整匹配 + 词表封闭（不扩容）使风险可控；若未来误伤，走 tag 层改名而非扩黑名单。
- **清理 SQL 触碰真库**：严格执行 备份 → count 留底 → BEGIN 预览 → COMMIT 流程（standard/backend/testing.md 红线），DELETE 前数字须与本 design 记录的规模吻合（484 / 约 714+90）。

## Open Questions

- 无（D4 为排查任务而非未决设计问题）。

## 附录 A：D4 排查结论——「Coding」写入路径已完全定位（2026-09-25）

**结论：无独立写入路径，「初版内容被替换」假设证伪；污染源就是 D1 移除的回填链路本身，且今天仍在活跃产出（最新一行 2026-09-25 20:11，后端未重启前）。D1 已断根，无需额外修复。**

证据链（全部只读查询 + 本地纯函数复现）：

1. **现场抓个正着**：文章 2939（白宫国宴）的打标调用 ai_call_logs id=2025273（20:11:36.670，qwythos，success，latency 2117ms）。响应原文 `keyword_tags: []`，不含 Coding；prompt 原文也不含任何模式词。但 10ms 后（20:11:36.680）即写入 Coding(source=llm) 行，6.7 秒后同批写入两个 event 标签与「新闻」（时间差 = 首个候选缓存命中 vs 后续 embedding TagMatch 耗时）。
2. **旧链路本地复现**：用该次调用的精确输入（标题/摘要/来源/分类）跑旧版 ExtractTopics → 产出恰好 `[Coding(0.90), 新闻(0.65)]`；回填→mergeExtractedTags→dedupeTagsWithCategory 后的顺序 `[Coding, 中美元首会晤, 新闻, 白宫国宴]` 与实际落库顺序完全一致。
3. **模式词命中位置**：`coding` 是子串匹配（strings.Count），命中的是凤凰网正文 HTML 里惰性加载图片属性 `de**coding**="async"`（img 标签）。mono 采样只剥 markdown 格式图片/链接噪声，凤凰网给的是原始 HTML，img 标签整体存活进打标输入（prompt 原文含 `<figure>/<img>` 实锤）。
4. **系统性验证**：89 篇 llm-Coding 文章中 73 篇现存内容含 `decoding`、81 篇命中任一模式词（coding/code/编程/开发）；对照组华尔街见闻 461 篇 llm-新闻 文章零命中 decoding（解释了为何只有凤凰网中招：其正文是原始 HTML）。所有 8 篇抽样文章签名一致：`Coding + 新闻 + 事件标签`，全部 source=llm。
5. **tag 27 本体**：2026-06-14 创建的合法编程标签（历史编程文章用），被此次污染错挂到政治/社会新闻上。

残留风险：D1 移除回填后此链路已死；D5 第三条 DELETE 定向清除 89 行存量（tag_id=27、llm、现存凤凰网垃圾行）。

## 附录 B：D5 执行留痕与队列假设修正（2026-09-25）

- 清理实制：备份 `backups/article_topic_tags_pre_fix_tagging_pollution_20260925_2121.sql`（22234 行，与表总行数一致）；留底 count heuristic=484 / llm 泛词=708（新闻 639+论坛 69）/ llm Coding=89，与预期偏差均 <±5%；BEGIN 预览验证后用户确认 COMMIT，DELETE 484+708+89=1281 行，复检三类残留均 0，表余 20953 行；同事务重算 1003 篇受影响文章的 `articles.tag_count`。
- 受影响 1003 篇中 711 篇保有剩余合法标签无需重打；292 篇清后零标签，其中 256 篇已归档（重打无价值），仅 36 篇未归档。
- **队列假设修正**：proposal/design「队列以无标签文章为输入周期自动重打」不成立——tag_jobs 仅两个入队源（文章入库时 EnqueueAsync + 手动 retag-today），无零标签自动发现。用户决策：临时补队的 746 条全量零标签任务撤回，改用 `POST /api/tag-queue/retag-today` 全量重跑今日文章（443 篇 force_retag）；历史零标签文章保持零标签（宁缺毋滥的合法状态）。
- 修复生效铁证：后端重启（换新代码）+ 清理后，重打期间新落库 237 行 llm 标签全部干净，heuristic/泛词/Coding 新增均为 0。
