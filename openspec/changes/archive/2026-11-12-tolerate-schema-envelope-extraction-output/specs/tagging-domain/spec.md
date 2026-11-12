# tagging-domain — tolerate-schema-envelope-extraction-output delta

## ADDED Requirements

### Requirement: 提取响应形态容错与 prompt 指令一致性

打标提取的响应解析 SHALL 容忍模型因 JSON-schema 引导而吐出的 **schema 外壳形态**——即顶层为 `{"type":"object","properties":{...},"required":[...]}`、而数组值被塞在 `properties` 下的响应。解析器 SHALL 识别该外壳并取 `properties` 内的对象继续按既有校验链处理（event/person 与 keyword 双数组、section 路径的单 `tags` 数组各按其既有形态解析）。

外壳判定 SHALL 同时满足三条，缺一不判为外壳（避免误解正常响应）：
1. 顶层是 JSON 对象；
2. 含 `properties` 键且其值是 JSON 对象；
3. 含 `type` 或 `required` 键（schema 特征）。

解包 SHALL 是幂等的：解开一次后顶层不再具备 schema 特征，再解 SHALL 原样返回。

**不放松畸形判定**：外壳解开后若目标键仍缺失（非空对象缺 `event_person_tags` 与 `keyword_tags` 双键，或非数组形态），SHALL 报解析错误触发既有重试，**MUST NOT** 把坏数据当作合法结果或合法零候选。

提取路径的 **user prompt 输出格式指令 SHALL 与下发给模型的 JSON Schema 一致**：merged 提取路径（双数组 `event_person_tags` / `keyword_tags`）的 prompt SHALL 指向双数组形态，MUST NOT 再指向已废弃的单数组 `{"tags": [...]}` 形态；section/aggregate 提取路径（单数组 `tags`）的 prompt 维持单数组指令不变。

#### Scenario: 双数组被包在 schema 外壳内
- **WHEN** 模型响应为 `{"type":"object","properties":{"event_person_tags":[...],"keyword_tags":[...]},"required":[...]}`
- **THEN** 解析器 SHALL 解开外壳并按规范双数组形态处理，返回 event/person 与 keyword 两组标签

#### Scenario: 单数组被包在 schema 外壳内
- **WHEN** section 路径的模型响应为 `{"type":"object","properties":{"tags":[...]},"required":["tags"]}`
- **THEN** 解析器 SHALL 解开外壳并按单数组形态处理

#### Scenario: 外壳内目标键缺失仍判解析错误
- **WHEN** 模型响应为 `{"type":"object","properties":{"other":[1]},"required":["other"]}`
- **THEN** 解包后仍缺目标键，解析 SHALL 返回错误触发重试，MUST NOT 视为合法零候选

#### Scenario: 非外壳响应不被误解
- **WHEN** 模型响应为不含 schema 特征的普通对象或数组
- **THEN** 解析器 SHALL 原样处理，行为与既有形态完全一致

#### Scenario: 提取 prompt 输出指令与 schema 一致
- **WHEN** merged 提取路径构造 user prompt
- **THEN** 末句输出格式指令 SHALL 指向 `event_person_tags` 与 `keyword_tags` 双数组，MUST NOT 出现已废弃的 `{"tags": [标签列表]}` 单数组写法

#### Scenario: section 提取 prompt 指令保持单数组
- **WHEN** section/aggregate 提取路径构造 user prompt
- **THEN** 末句输出格式指令 SHALL 维持 `{"tags": [...]}` 单数组（其 schema 本就是单数组），MUST NOT 被改成双数组
