# Aggregate Tagging

## Purpose

聚合型文章（科技周刊、排行榜合集等）的打标机制：摘要产出内容形态标记（content_form）、栏目级切片 map-reduce 打标路径、跨片去重与 score 分层。

## Requirements

### Requirement: 摘要生成产出内容形态标记

系统的摘要生成调用（`summarizeContent`）SHALL 在 system prompt 中要求模型将文章判定为单主题（`mono`）或聚合型（`aggregate`），并以摘要第一行 HTML 注释 `<!-- form: mono -->` 或 `<!-- form: aggregate -->` 输出该判定。

系统 SHALL 在持久化 AIContentSummary 前解析并剥离该注释行，将标记值存入 `articles.content_form` 列；剥离后的摘要正文 SHALL NOT 包含任何形态注释残留。

#### Scenario: 聚合型文章被正确标记

- **WHEN** 一篇内容为多栏目合集（如科技周刊：AI 缓存专题 + 科技动态 + 工具推荐 + 言论）的文章完成摘要生成
- **THEN** 该文章 `content_form = 'aggregate'`，且 AIContentSummary 首行不是 HTML 注释

#### Scenario: 单主题文章被正确标记

- **WHEN** 一篇通篇围绕单一主题的文章（如一篇 .NET 教程，即使有 20+ 个章节小标题）完成摘要生成
- **THEN** 该文章 `content_form = 'mono'`

#### Scenario: 模型未输出标记时安全降级

- **WHEN** 模型返回的摘要不含合法的形态注释行
- **THEN** `content_form` 保持为空，摘要原文正常入库（注释解析失败不影响摘要存储）

#### Scenario: 存量文章不回填

- **WHEN** change 合并前已入库的文章被读取
- **THEN** 其 `content_form` 为空，不触发任何回填任务

### Requirement: 摘要按栏目切片

系统 SHALL 提供纯代码切片器，输入聚合型文章的 markdown 摘要，输出 4-8 个栏目级文本片：按 `## ` 标题切分，跳过"导读"片（其内容是正文概括，map 后无增量），短栏目（<300 runes）向后并入相邻片；`##` 切完超过 8 片时对超长栏目按 `### ` 细分；仍超过 8 片时从尾部合并相邻片压回 8。切片器 SHALL NOT 发起任何 LLM 调用。

#### Scenario: 标准周刊摘要被切成栏目片

- **WHEN** 输入含"导读 + 正文整理（专题）+ 科技动态 + 工具 + 言论"5 个 `##` 栏目的周刊摘要
- **THEN** 输出 4 片（导读被跳过），每片携带栏目标题上下文与正文

#### Scenario: 短栏目被合并

- **WHEN** 某个 `##` 栏目正文仅 100 runes
- **THEN** 该栏目与其后相邻栏目合并为一片

#### Scenario: 无标题结构时安全回落

- **WHEN** 输入摘要不含任何 `## ` 标题
- **THEN** 聚合路径放弃切片，回落到 mono 路径处理该文章

### Requirement: 逐片融合提取标签

聚合路径 SHALL 对每个栏目片发起恰好 1 次 LLM 调用，使用 event/person/keyword 三分类融合的单一 prompt（每片产出上限 4 个标签候选），SHALL NOT 对单篇文章发起超过 8 次提取调用。

单片调用失败时 SHALL 记录 warning 并跳过该片继续处理其余片，SHALL NOT 因单片失败丢弃整篇或整篇重试。

单个标签的 auxiliary_labels 校验失败或 keyword description 缺失 SHALL 降级处理（丢弃校验不过的部分、保留其余标签），SHALL NOT 因单标签问题让整片候选报废；每次降级 SHALL 记录 warning（含标签名与原因）。JSON 整体解析失败 SHALL 维持原有重试语义（重试 3 次后跳过该片）。

#### Scenario: 每片一次调用产出候选

- **WHEN** 切片器产出 5 个栏目片
- **THEN** 发起 5 次融合 prompt 提取调用，每次调用的输入仅含对应片文本

#### Scenario: 单片失败不影响其余片

- **WHEN** 5 片中第 3 片的 LLM 调用重试耗尽后仍失败
- **THEN** 第 1、2、4、5 片的标签候选正常进入 reduce 阶段，失败信息记录在日志

#### Scenario: 单标签 aux 校验失败降级保留

- **WHEN** 某片返回 3 个标签，其中 1 个 event 标签的 auxiliary_labels 数量为 2（不满足 3-5 条）
- **THEN** 该 event 标签仍入库（无 aux 锚点），同片其余 2 个标签不受影响，降级记录在 warning 日志

#### Scenario: keyword 缺 description 单标签跳过

- **WHEN** 某片返回的 keyword 标签 description 为空
- **THEN** 该 keyword 标签被跳过并记录 warning，同片其余标签正常进入 reduce 阶段

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

### Requirement: 聚合路径 score 按栏目位置分层

聚合路径产出的标签 score SHALL 取所在片的位置层级：首个正文栏目 0.9、中间栏目 0.7、尾部栏目 0.5。mono 路径产出的标签 score 维持一律 0.7 不变。

#### Scenario: 主专题标签获得高 score

- **WHEN** 周刊首个正文栏目（本期主打专题）的标签入库
- **THEN** 其 score 为 0.9

#### Scenario: 尾部栏目标签低 score

- **WHEN** "言论/图片"类尾部栏目的标签入库
- **THEN** 其 score 为 0.5

### Requirement: 聚合标签复用既有入库链路

聚合路径产出的每个标签 SHALL 复用既有的 `findOrCreateTag`、auxiliary labels 挂载、`createArticleTopicTagLink` 事务链接与 event 标签 embedding 入队链路，SHALL NOT 另建入库通道。

#### Scenario: event 标签照常进入 embedding 队列

- **WHEN** 聚合路径产出 6 个 event 标签入库
- **THEN** 6 个标签均被 enqueue embedding，与 mono 路径行为一致

#### Scenario: 标签命中已有话题

- **WHEN** 聚合片产出的标签 slug 与库中既有 topic_tag 一致
- **THEN** 复用既有标签实体并累加引用，不新建重复标签

### Requirement: 融合提取 JSON 尾逗号容错

融合与 mono 提取共用的 JSON 标签解析入口（`parseRawTagObjects`）SHALL 在反序列化前对输入做无损修复：剥除对象与数组末元素后的多余逗号；修复 SHALL NOT 影响合法 JSON 的解析结果，SHALL NOT 修改字符串字面量内部内容。

#### Scenario: 尾逗号 JSON 被修复解析

- **WHEN** 模型返回的 tags JSON 含 `"label": "X",}` 形式的尾逗号
- **THEN** 修复后解析成功，标签 "X" 正常产出，不触发重试

#### Scenario: 合法 JSON 不受影响

- **WHEN** 模型返回标准合法 JSON
- **THEN** 解析行为与修复前完全一致

#### Scenario: 字符串内逗号结构不误伤

- **WHEN** 标签 description 字符串字面量内容含 `,}` 字符序列
- **THEN** 该字符串内容保持原样，不被修复函数改动
