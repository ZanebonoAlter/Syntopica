# Test Cases — tolerate-schema-envelope-extraction-output

> 测试单元 = 一个 Requirement 的用户故事。spec Scenario 是断言片段，本文件串成完整故事。

## 0. 旧资产继承与调整（⓪ 改契约了）

| 旧资产 | 处置 | 动作 |
| --- | --- | --- |
| `extractor_test.go` 的 `TestParseMergedExtractionTagsAcceptsCanonicalDoubleArrays` | **保留** | 规范形态行为不变，加固断言 |
| `extractor_test.go` 的 `TestParseMergedExtractionTagsAcceptsWrappedDoubleArrays`（`{"tags":...}` 包装） | **保留** | 既有容忍形态不受影响 |
| `extractor_section.go` 的 section 解析用例 | **保留** | section 路径经 `parseRawTagObjects` 自动获得兜底 |
| prompt 断言（若有断言末句文本） | **改** | 末句文案变了 |

## 1. 主链路表

| 节拍 | 动作 | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 模型返回 schema 外壳（双数组在 `properties` 下） | 解析成功，拿到 event/person + keyword 两组标签 | 纯逻辑单测 | `extractor_test.go` |
| 2 | 模型返回 schema 外壳（单数组 `tags` 在 `properties` 下，section 路径） | 解析成功 | 纯逻辑单测 | `extractor_test.go` |
| 3 | 模型返回外壳但 `properties` 内双数组皆空 | 走**合法零结果**（不是解析错误） | 纯逻辑单测 | `extractor_test.go` |
| 4 | 模型返回外壳但 `properties` 内既无目标键 | **解析错误**（触发重试），不得当合法结果 | 纯逻辑单测 | `extractor_test.go` |
| 5 | 模型返回正常形态 | 行为**逐字不变** | 纯逻辑单测 | 既有用例加固 |
| 6 | prompt 末句指令 | 不含 `{"tags": [标签列表]}`；含两个真实键名 | 纯逻辑单测 | `extractor_enhanced_test.go` |
| 7 | 真库效果 | 外壳形态响应被解析成功、拿到 event 标签 | 真库量化核对 | tasks 9 |

## 2. 变体走查

### 2.1 输入
- 空串 / 纯空白 → 不是外壳，原样返回，后续解析报错（既有行为）
- 纯分隔符 → 同上
- 单 token → 同上
- 大小写：外壳判定看键名 `properties`/`type`/`required`——**大小写敏感**（schema 关键字是固定小写）；若模型输出 `Properties`，不视为外壳 → 走既有解析路径报错（保守，不误解）
- 特殊字符 / 超长 → 由 `jsonutil.SanitizeLLMJSON` 既有路径处理

### 2.2 前置
- 空集 → `properties` 为空对象 → 解开后是 `{}` → merged 路径：非空对象缺两键 → 解析错误（既有守卫）
- 单元素 → `properties` 内只有一个目标键 → 解开后按既有 canonical 分支处理（缺的那侧视为空数组）
- 重复 / 越界引用 / 部分满足 → 不适用

### 2.3 时间窗口 / 幂等 / 并发
- 时间窗口：不适用
- 幂等：`unwrapSchemaEnvelope` 幂等（解一次后顶层无 `type`/`required`，再解原样返回）
- 并发：纯函数，无共享状态

### 2.4 可用性（UI 必检前三）
- 误输入反馈 / 空态 / 错误态 / 加载态 / 超长文本 / 重复提交：**不适用**（无界面改动，ui-impact: none）

## 3. 效果核对

| 核对项 | 方法 | 量化结果 | 结论 |
| --- | --- | --- | --- |
| 外壳形态解析成功率 | 真库投一批重打任务，观察 `tag_jobs` 失败率与 `ai_call_logs` 形态分布 | 待 tasks 9 填 | 待填 |
| 标签语义未变 | 对比修复前后同批文章的标签产出 | 待填 | 待填 |

## 4. 白盒附加（分支表）

| 函数 | 分支 | 边界值 | 期望 |
| --- | --- | --- | --- |
| `unwrapSchemaEnvelope` | 规范双数组 | `{"event_person_tags":[],"keyword_tags":[]}` | 原样返回（无 `type`/`required`） |
| 同上 | 外壳 + 双数组 | `{"type":"object","properties":{...},"required":[...]}` | 返回 `properties` 的 JSON 文本 |
| 同上 | 外壳 + 空 `properties` | `{"type":"object","properties":{}}` | 返回 `{}` |
| 同上 | `properties` 不是对象 | `{"type":"object","properties":"x"}` | 原样返回（不误解） |
| 同上 | 有 `properties` 无 `type`/`required` | `{"properties":{...}}` | 原样返回（缺 schema 特征，不误解） |
| 同上 | 非 JSON | `not json` | 原样返回 |
| `unmarshalMergedExtraction` | 外壳解开后缺两键 | `{"type":"object","properties":{"x":1}}` | 解析错误（触发重试） |
| `parseRawTagObjects` | 外壳解开后是 `{"tags":[...]}` | section 形态 | 正常解析 |

## 5. 实测结果（2026-11-12，20 篇试验批 Force=false）

| 指标 | 修复前 | 修复后 |
| --- | --- | --- |
| 任务失败 | 14 / 20 | **0 / 20** |
| 拿到标签 | 6 / 20 | **19 / 20** |
| 含 event 标签 | **1 / 20（5%）** | **18 / 20（90%）** |

失败原因从 `merged extraction object has neither event_person_tags nor keyword_tags keys` 变为无失败。结论：schema 外壳容错 + prompt 指令对齐**双双生效**；外壳形态已被解析器吃掉，剩余 1 篇未拿到 event 是模型正常零产出（合法结论）。
