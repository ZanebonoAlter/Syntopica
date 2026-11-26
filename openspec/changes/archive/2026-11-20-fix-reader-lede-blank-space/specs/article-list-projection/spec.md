# Delta Spec：article-list-projection

## MODIFIED Requirements

### Requirement: excerpt 承载列表层导语

列表响应每项 SHALL 提供 `excerpt`：由 `description`（为空时回退 `content`）按**文章摘要的同一套清洗口径**（还原实体 → 按 HTML 解析 → 去脚本与样式 → 剔除隐藏节点子树 → 去标签取文本 → 折叠空白）得到的纯文本，长度 ≤ 200 字符；源为空、源无实质内容（无字母/数字）或**导语与正文重复**时 SHALL 返回空字符串（字段存在但不省略）。截断 MUST NOT 追加省略号（列表导语保持既有形态）。

「与正文重复」MUST 按阅读页既有去重 guard 的判据判定（正文归一化后与导语完全相同，或导语归一化后 ≥ 40 字符且被正文包含），且判据 MUST 是 guard 规则的**子集**——即 MUST NOT 抑制 guard 本会渲染的导语（否则 92% 文章会在点选时出现「首帧导语闪现→详情返回后消失」，实测 `description == content` 占非归档文章 92%）。

当导语来自**正文兜底**（`description` 无实质文本、导语取自 `content`）且该文章无 Firecrawl 正文（阅读页展示的正文就是 `content`）时，导语 SHALL 为空字符串——它与展示正文必然重复，guard 的相等/包含规则会隐藏它（实测此类文章 198 条，其中带 Firecrawl 正文的 0 条）。

清洗口径 MUST 与文章摘要（`articles.description`）同源同实现（`internal/platform/htmltext`），仅长度上限与「无实质内容即空串」这一既有地板不同——两处 MUST 用同一组固定样本断言逐字相同的清洗结果。

#### Scenario: 长 HTML 源被截断

- **WHEN** 某文章 `description` 为 50 KB 的 HTML 片段
- **THEN** `excerpt` SHALL ≤ 200 字符、不含 HTML 标签

#### Scenario: 空源返回空串

- **WHEN** 某文章 `description` 与 `content` 均为空
- **THEN** `excerpt` SHALL 为 `""`，字段 SHALL 仍存在于响应项中

#### Scenario: 与正文重复的导语不下发

- **WHEN** 某文章的导语源与正文重复（`description` 与 `content` 为同一份文本，或导语归一化后 ≥ 40 字符且被归一化正文包含）
- **THEN** `excerpt` SHALL 为 `""`（阅读页去重 guard 本就会隐藏该导语，下发只会造成首帧导语闪现）
- **AND** 导语与正文不重复（含短导语 < 40 字符）时 `excerpt` SHALL 照常返回

#### Scenario: 正文兜底导语不下发

- **WHEN** 某文章 `description` 无实质文本（导语取自 `content`）且该文章无 Firecrawl 正文
- **THEN** `excerpt` SHALL 为 `""`（阅读页展示的正文就是 `content`，导语必然与之重复）
- **AND** 该文章有 Firecrawl 正文时 `excerpt` SHALL 照常返回（展示正文非 `content`，导语可能不重复）

#### Scenario: 整体隐藏的源其导语为空串

- **WHEN** 某文章 `description` 是整篇文章 HTML 且整体位于声明隐藏的根节点内（清洗口径剔除隐藏子树）
- **THEN** `excerpt` SHALL 为 `""`（不可见内容不构成导语），且 MUST NOT 出现「清洗后残留的可见碎片」之外的任何内容
