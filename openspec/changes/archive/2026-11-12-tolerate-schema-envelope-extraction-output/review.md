# Review — tolerate-schema-envelope-extraction-output

2026-11-12；实现与实测由主线程完成，本文件按 review-trace 契约建账。
审查方式：真实数据反证（`ai_call_logs` 形态分布）+ 真库效果核对（20 篇试验批前后对比）。

### R-1 2026-11-12【High】修复是否真的解决了根因，而不只是让解析器更宽容
- **状态**：已修复
- 现象：11-12 有 82%（1168/1423）的 `topic_tagging` 响应是 schema 外壳形态。
- 核对：根因不在解析器「太严格」，而是 **user prompt 末句与下发 schema 自相矛盾**——`buildExtractionUserPrompt` 仍写 merge-tag-extraction-branches 之前的 `{"tags": [标签列表]}`（单数组），而 system prompt 正例与 JSON Schema 都是双数组。4B 模型被两条指令撕扯后吐出「schema 骨架 + 值塞 properties」。
- 处置：双管——① prompt 末句改为指向双数组（根因）；② `unwrapSchemaEnvelope` 容忍外壳（兜底）。实测 20 篇：失败 14→**0**、含 event 标签 1→**18**（5%→90%），证明两层都生效。

### R-2 2026-11-12【Medium】外壳容错会不会把真正畸形的响应当好数据
- **状态**：已修复
- 判定三条件必须同时满足（顶层是对象 ∧ `properties` 是对象 ∧ 含 `type` 或 `required`），缺一不判外壳。
- 解包后目标键仍缺 → 仍走「非空对象缺双键 → 解析错误」的既有守卫（`TestParseMergedExtractionTagsSchemaEnvelopeMissingTargetKeysStillFails` 守住）。
- `TestUnwrapSchemaEnvelopeLeavesNonEnvelopeContentUnchanged` 覆盖六种不该误伤的形态（规范双数组 / `{"tags":...}` wrapper / legacy 裸数组 / 只有 properties 无 schema 特征 / properties 非对象 / 非 JSON）。

### R-3 2026-11-12【Medium】section 路径改写 prompt 会不会引入新矛盾
- **状态**：已修复
- 明确不动 `extractor_section.go:162`——其 `sectionExtractionSchema()` 本就是单数组 `{"tags":[...]}`，那句指令**与 schema 一致**，改了反而制造新的矛盾。
- section 路径的解析兜底经共用 `parseRawTagObjects` 自动获得（`TestParseRawTagObjectsAcceptsSchemaEnvelope` 守住）。

### R-4 2026-11-12【Medium】批量重打会不会清空已有好标签
- **状态**：已修复（执行纪律）
- `tagArticle` 的 Force 分支是**先删旧标签再提取、失败无回滚**。
- 处置：批量重打只投**当前无任何标签**的文章（`NOT EXISTS article_topic_tags`）且 `force_retag=false`；解析器经 20 篇验证前禁止对带标签文章跑 Force。

### R-5 2026-11-12【Low】幂等性
- **状态**：已修复
- `unwrapSchemaEnvelope` 解开后顶层不再有 `type`/`required`，二次调用原样返回（`TestUnwrapSchemaEnvelopeIsIdempotent` 守住）。
