# aggregate-tagging Delta

## MODIFIED Requirements

### Requirement: 跨片去重与文章级上限

聚合路径 SHALL 以纯代码方式跨片去重（按 `Slugify(label)`，重复时保留首栏目出现者）并将文章级标签上限设为 15。语义级撞车（不同措辞指同一话题）SHALL 交由既有标签合并建议机制处理，SHALL NOT 在本次提取链路中新增 LLM 仲裁调用。

聚合路径全部片处理完成后标签数为 0（全片失败或全部空产出）时 SHALL 回落 mono 提取路径（单次调用 LLM 提取，含 heuristic 兜底），SHALL NOT 让聚合型文章以 0 标签结束打标。

#### Scenario: 同名标签跨片去重

- **WHEN** "工具推荐"片与"正文整理"片产出了同名标签 "Kubernetes"
- **THEN** 该标签只保留一份，归属首栏目出现的片（score 更高）

#### Scenario: 文章级上限截断

- **WHEN** reduce 后候选标签共 17 个
- **THEN** 按片顺序与片内优先级保留前 15 个入库

#### Scenario: 全片失败回落 mono 路径

- **WHEN** 某聚合文章的所有栏目片提取均失败（或全部返回空候选）
- **THEN** 该文章走 mono 提取路径打标（单次调用 LLM 提取，含 heuristic 兜底），回落原因记录在日志，最终标签数不为 0（除非 mono 路径同样无产出）
