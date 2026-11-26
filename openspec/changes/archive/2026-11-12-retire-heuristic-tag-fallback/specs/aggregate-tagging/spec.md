# aggregate-tagging — retire-heuristic-tag-fallback delta

## MODIFIED Requirements

### Requirement: 跨片去重与文章级上限

聚合路径 SHALL 以纯代码方式跨片去重（按 `Slugify(label)`，重复时保留首栏目出现者）并将文章级标签上限设为 15。语义级撞车（不同措辞指同一话题）SHALL 交由既有标签合并建议机制处理，SHALL NOT 在本次提取链路中新增 LLM 仲裁调用。

聚合路径全部片处理完成后标签数为 0（全片失败或全部空产出）时 SHALL 回落 mono 提取路径（单次调用 LLM 提取），SHALL NOT 让聚合型文章以 0 标签结束打标。【2026-11-12 本 change 起生效】回落链路 SHALL NOT 再含 heuristic（规则词）兜底——规则词兜底打标整体退役（`extractor_heuristic.go` 整文件删除、`extractWithHeuristic` 与 `legacyExtractTopics` 两处兜底分支删除）；mono 提取失败时 SHALL 返回错误、文章不写任何标签，打标任务标记失败并进入既有队列退避重试。「不为 0 标签收尾」的口径随之收窄为「不因聚合切片本身的问题而放弃 mono 回落」。

#### Scenario: 同名标签跨片去重

- **WHEN** "工具推荐"片与"正文整理"片产出了同名标签 "Kubernetes"
- **THEN** 该标签只保留一份，归属首栏目出现的片（score 更高）

#### Scenario: 文章级上限截断

- **WHEN** reduce 后候选标签共 17 个
- **THEN** 按片顺序与片内优先级保留前 15 个入库

#### Scenario: 全片失败回落 mono 路径

- **WHEN** 某聚合文章的所有栏目片提取均失败（或全部返回空候选）
- **THEN** 该文章走 mono 提取路径打标（单次调用 LLM 提取，不含 heuristic 兜底），回落原因记录在日志；mono 提取成功则标签数不为 0，mono 也失败则该文章判任务失败、不写任何标签（不产出规则词泛词）
