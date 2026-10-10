# Design: fix-event-tag-time-anchor

## Context

见 proposal.md 与 docs/research/event-tag-time-anchor/explore-findings.md。三处 prompt（`tagger.go generateTagDescription` event 分支、`extractor_section.go buildSectionExtractionPrompt`、`extractor_enhanced.go buildMergedExtractionPrompt`）以"伊朗袭击以色列"为示例/正例，自带"2024年4月"历史时间锚；description 生成 LLM 照抄示例而非 articleContext 前缀中已有的 `[日期: …]`。存量 `topic_tags` 中约 176 个 event 标签时间锚早于创建时间 ≥1 年，其中确有病态样本。

## Goals / Non-Goals

**Goals**
- 新建 event 标签的 description 时间锚与触发文章语境一致（治根）
- 存量病态标签 description 可列出、可确认、可重生成（还债）

**Non-Goals**
- 不改日报 threads 生成逻辑与事实锚约束（源头修复后下游自然正确）
- 不做"复用时自动检测/刷新 description"常驻机制（并发与 LLM 成本权衡，另行评估）
- 不重建历史日报线索
- 不区分"事件类 vs 事件实例"的数据模型改造

## Decisions

### D1: 示例替换策略——中性虚构事件，而非换一个真实事件

三处 prompt 的示例改用时间中性或虚构组合（如 `"央行上调基准利率" → "某国央行于文章发布当期上调基准利率，具体时间以文章为准"` 风格的正例说明约束本身）。理由：任何真实事件示例都自带先验知识时间锚，模型可能再次照抄；示例的职责是示范格式与纪律，不是示范事实。
*备选*：换成另一真实事件（如"苹果WWDC 2024"）——否决，问题会以新形式复发。

### D2: 时间锚约束加在 generateTagDescription 的 prompt 里，利用已有 `[日期: …]` 前缀

articleContext 构造（`persistArticleTags`）已带 `[日期: 发布日期] 标题. 摘要≤800字` 前缀，无需改传参。在 event 分支 prompt 的 Description requirements 中追加：
- 事件发生时间 MUST 以 Context 中的 `[日期: …]` 为参照，描述当日或相对时间语境；
- MUST NOT 引入 Context 中不存在的时间信息（含模型记忆中的历史事件时间）；
- 若标签名与某历史事件同名但 Context 讲的是当日内容，以 Context 为准。
同时给 user prompt 结构保持不变（无接口变化）。

### D3: 存量修复做成一次性 CLI 子命令（admin 已有 cmd 基础），扫描→清单→确认→重生成

- **扫描**：SQL 筛 event 标签，Go 侧正则提取 description 中最早年份，与 `created_at` 年份差 ≥1 → 候选清单（打印 ID/label/created_at/description 预览）。正则提年份在 Go 侧做而非 SQL，便于单测。
- **确认**：默认 dry-run 只出清单；`--apply --ids 1,2,3` 显式指定 ID 才重生成。不做"自动修复全部候选"——176 个里混有合法历史参照，误伤代价是改写正确描述。
- **重生成**：复用 `generateTagDescription` 的调用路径（同 prompt、同 airouter 通道），articleContext 取该标签最早挂载文章的发布日期 + 标题 + 摘要（与 `persistArticleTags` 构造口径一致）。
- **幂等**：重生成后 description 时间锚对齐，再跑扫描不再命中；无其它副作用。
*备选*：后台任务/管理页 UI——否决，一次性还债不值得常驻界面。

### D4: 病态判定的最小语义，不做 NLP 级"事件主体 vs 背景参照"分类

扫描器只负责"时间锚早于创建 ≥1 年"的机械筛选并输出清单，"是否病态"由人工看清单确认（`--ids` 显式圈选）。理由：自动区分"创2013年以来新高"（合法）与"2024年4月伊朗袭击"（病态）需要语义理解，引入 LLM 判定会新增一类不可靠环节；单用户系统人工确认成本可接受。

## Risks / Trade-offs

- [LLM 仍可能无视约束带出时间] → 约束写进 system prompt + 示例同步中性化（双管）；重生成走同一约束，修复结果抽样人工核验（test-cases 效果核对节）。
- [重生成后 description 可能更笼统（少了时间细节）] → 可接受：描述的职责是"解释标签指代什么"，时间锚宁缺勿错；prompt 要求"以文章日期为参照"而非"删除所有时间"。
- [存量修复误改合法标签] → dry-run 默认 + 显式 ID 圈选 + 非 ID 命中不触碰；修复前清单留档。
- [首挂文章本身是旧闻转载（如"路某某再审"）] → 重生成结果仍锚定该文章语境，与"文章发布日"一致——这是数据口径（标签服务于挂它的文章），不追求客观事实时间。

## Migration Plan

1. 合入 prompt 修改（无部署顺序要求）。
2. 跑扫描 dry-run 出清单 → 人工圈选病态 ID → `--apply --ids …` 重生成。
3. 回滚：prompt 改动 revert 即可；已重生成的 description 如需恢复，修复脚本执行前导出原 description 备份（清单落 CSV）。

## Open Questions

（无）
