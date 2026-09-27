# Test Cases: merge-tag-extraction-branches

## ⓪ 继承与调整（契约 M：双分支 → 单次调用双数组）

旧资产反查：`bash scripts/harness/test-assets.sh auxiliary-label`（主 spec 10 Requirements / 18 Scenarios；历史 change 无映射表，测试落点以 grep `internal/tagmanagement/service/core/extractor_test.go` 为准）。

| 旧 Scenario | 处置 | 旧测试 | 动作 |
| --- | --- | --- | --- |
| event/person 分支失败但 keyword 分支成功 | 重写语义（调用级失败 → 数组级缺失） | `TestExtractTagsKeepsKeywordBranchWhenEventPersonFails`（extractor_test.go:302） | 改写：fake 由"event 分支连续 err"改为"单次调用成功但 event 数组空"，断言不变（保 keyword、不触发 heuristic） |
| keyword 分支失败但 event/person 分支成功 | 重写语义（同上） | `TestExtractTagsFallsBackToHeuristicKeywordWhenKeywordBranchFails`（:324） | 改写：fake 改"成功但 keyword 数组空"，断言 event 产出 + heuristic keyword 兜底 + 不入辅助池语义不变 |
| 双分支合并去重 | 保留（数组间去重，规则不变） | `TestMergeExtractedTagsLimitsAndDedupesByCategoryPriority`（:277）/ `KeepsHigherPriorityDuplicate`（:295） | 零改动，作回归护栏 |
| （解析族无独立 Scenario，属实现细节） | 入口重写、字段校验保留 | `TestParseExtractedTags*` 系列（:61-:270） | 入口函数换双数组解析（任务 1.3），字段级断言迁移复用；`TestParseKeywordTagsRequiresDescriptionAndIgnoresAuxiliaryLabels` 原样保留 |
| （系统提示/schema 无独立 Scenario） | 随实现更新 | `TestBuildExtractionSystemPrompt*`（:211/:218）/ `TestTagExtractionSchemaIncludesAuxiliaryLabelObjects`（:233） | 断言更新（任务 1.1/1.2） |

aggregate-tagging 侧：`全片失败回落 mono 路径` Scenario 行为不变（回落调用的 `ExtractTags` 签名未变），既有 aggregate 回落测试零改动。

## 主链路表（一个 mono 文章的提取故事）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 构造 ExtractionInput，发起提取 | mono 文章提取调用数为 1 | Chat 恰 1 次，meta.operation=tag_extraction_merged | 函数单测 | 任务 2.1/2.4 |
| 2 | LLM 返回合法双数组 | 双数组间去重 | event 1 个 + keyword 1 个同名 slug → 保留 person，总数/keyword≤3 规则生效 | 函数单测 | 任务 2.5（既有测试） |
| 3 | 返回但 event 数组空 | event/person 数组为空但 keyword 有产出 | 保 keyword、branchErrors 记缺失、不 heuristic | 函数单测 | 任务 2.2 |
| 4 | 返回但 keyword 数组空 | keyword 数组为空但 event/person 有产出 | 保 event/person、heuristic keyword 展示兜底不入辅助池 | 函数单测 | 任务 2.2 |
| 5 | 调用重试耗尽仍失败 | 单次调用整体失败回退 heuristic | source=heuristic、错误保留 | 函数单测 | 任务 2.3 |
| 6 | 线上一晚打标 | mono 文章提取调用数为 1 | ai_call_logs 提取调用数 ≈ 文章数、失败率不升 | 真库量化 | V5 |

## 变体走查（五组固定清单）

- **输入（解析层）**：空串/纯空白 → 整体失败路径（步 5 覆盖）；纯分隔符（`{}`/`{"tags":[]}`）→ 数组空路径（步 3/4 覆盖兼容包裹形态，任务 1.3）；单 token（仅 keyword 一个标签）→ 正常部分产出；大小写（JSON key 大小写）→ json.Unmarshal 默认大小写不敏感，接受；特殊字符（未转义引号）→ 既有 `fixBrokenJSON`/`TestParseExtractedTagsWithUnescapedQuotes` 继续生效，1.3 双数组入口同样过该清洗；超长输出 → maxTokens 截断即解析失败 → 重试路径（步 5）。
- **前置**：数组空集（步 3/4）；单元素（步 2 退化）；重复（步 2 去重）；越界引用（alias 指向不存在 tag——提取层不涉引用，划除）；部分满足（某 event 元素 category 串扰 → 该元素拒收，任务 1.3 第 3 种输入）。
- **时间窗口**：提取无窗口语义，划除。
- **幂等**：重复执行（同篇文章重复打标 → already-tagged 守卫上游拦截，既有测试覆盖）；部分失败重试（maxRetries=3 内换 attempt，fake 队列既有模式）；并发：不声称线程安全新增点，划除。
- **可用性（UI）**：无 UI 改动，前三项划除。

## 效果核对

效果依赖 LLM 行为（解析遵从率/质量持平），真库量化见 tasks.md V5：① 调用数减半对账 ② attempt>1 占比与失败率基线对比 ③ 固定 feed 标签人工抽查一周。
