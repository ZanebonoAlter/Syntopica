<!-- complexity: simple -->
<!-- ui-impact: none -->
<!-- constraint-domains: semantic-board -->

## Why

mono 打标的双分支提取（event/person + keyword 两次独立 LLM 调用）把同一篇 user prompt（实测 p50 ~3.9k 字符、长文采样前 p90 ~10.5k）原样送两遍，prefill 成本翻倍；本地 qwen 并发 2 的瓶颈下，实测 3 天 5674 次调用 × 平均 4.4s，打标是晚间批处理的主要消耗之一。单次调用输出双数组即可覆盖两分支语义，调用次数减半、prefill 减半。

## What Changes

- `ExtractTags` 的双分支并行调用合并为**单次 LLM 调用**：一次输出含 event/person 标签数组（每个带 3-5 辅助标签）与 keyword 标签数组（带 description、无 auxiliary_labels）的 JSON，两份系统提示合并去重为一份。
- 失败语义随之简化：单次调用（含既有 maxRetries=3）整体失败 → heuristic keyword 兜底（与现状"双分支均失败"路径一致）；调用成功但某数组为空/被校验清空 → 正常部分产出，不触发 heuristic（对应现状单分支成功语义的连续演化）。
- 不变项：合并去重规则（同 slug 按 person > event > keyword 优先级）、标签数量上限、keyword 直入辅助池、辅助标签三级入库（L1/L2/L3）、event/person 辅助标签数量校验——全部沿用既有契约与入库代码路径。
- 调用数可观测：`ai_call_logs` 中 `tagmanagement.extractor_enhanced` 的每篇 mono 文章调用数由 2 降为 1（operation 名保留以利前后对比，request_meta 标注合并形态）。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `auxiliary-label`: "Tag 提取拆分为 event/person 与 keyword 双分支调用"需求变更为"单次调用双数组输出"——两个独立 LLM 调用合并为一次（schema 约束双数组），部分失败语义改为"数组级缺失"；数量上限、去重优先级、keyword 直入与三级入库契约不变。
- `aggregate-tagging`: "跨片去重与文章级上限"需求中"回落 mono 提取路径（双分支 LLM 提取…）"的路径描述随 mono 路径结构变更同步为单次调用表述（回落行为本身不变）。

## Impact

- 代码：`backend-go/internal/tagmanagement/service/core/extractor_enhanced.go`（`ExtractTags` 主流程、两份 prompt 合并、schema 双数组、解析函数）及其单测；aggregate 回落路径（`article_tagger_aggregate.go`）零改动（其回落调用的就是 `ExtractTags`）。
- 与 `long-form-sampled-tagging` 正交且叠加：采样缩短每次输入，合并减少调用遍数；两 change 先后 apply 均不冲突。
- 可观测：打标 LLM 调用次数减半（~950 篇/晚：1890 → 950 次提取调用），晚间批处理打标段墙钟预期压缩 ~35-45%；解析失败率需观察（输出结构变复杂，重试即重复 prefill）。
- 部署后影响：无需用户手动操作；行为变化仅"标签提取的调用组织方式"，标签产出质量预期持平（上线后对同 feed 前后对比验证）。
