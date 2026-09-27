# markdown-mermaid-render Specification

## Purpose
markdown 宿主内 mermaid 围栏块的客户端渲染能力契约：` ```mermaid ` 代码块必须呈现为图而非源代码，失败时无损降级，主题双适配，并覆盖共享 markdown 渲染器的全部调用宿主。

## Requirements

### Requirement: mermaid 围栏块渲染为图

markdown 块级渲染产物中的 mermaid 代码块 MUST 被渲染为 mermaid 图（SVG），MUST NOT 以源代码形态终态呈现。mermaid 代码块包括：带 `language-mermaid` 标注的代码块，以及无语言标注、内容首词为 mermaid 图类型（flowchart / graph TD / sequenceDiagram 等）的裸代码块（firecrawl 抓取常丢失语言标记）。图 MUST 在源码块原位置替换（无布局跳动、无骨架屏占位）；替换完成前 MUST 保留源码块展示。

#### Scenario: 合法 mermaid 块变为图

- **WHEN** 用户打开内容含合法 ` ```mermaid ` 围栏块的文章
- **THEN** 该块呈现为 SVG 图，原源码 `<pre>` 不再可见，图位于原文位置

#### Scenario: 无 mermaid 块零行为

- **WHEN** 文章内容不含任何 mermaid 围栏块
- **THEN** 渲染行为与现状完全一致，且不下载 mermaid 渲染资源（按需加载，无额外 chunk 请求）

#### Scenario: 裸围栏 mermaid 块识别

- **WHEN** 文章正文存在无语言标注、内容以 mermaid 图类型首词（如 flowchart）开头的代码块
- **THEN** 该块渲染为图；带其它语言标注的普通代码块不受影响

### Requirement: 渲染失败无损降级

mermaid 图渲染失败（图语法无效或渲染资源加载失败）时 MUST 保留原源码块完整可读，并在其下方以一行轻量提示（warning 色）告知失败；MUST NOT 白屏、MUST NOT 吞没原文内容、MUST NOT 阻断同宿主内其他 mermaid 块的渲染。

#### Scenario: 图语法无效

- **WHEN** 某 mermaid 块内容不合法（LLM 产出的常见情形）导致渲染抛错
- **THEN** 该块保留源码展示，下方出现一行失败提示，其余合法 mermaid 块正常成图

#### Scenario: 渲染资源加载失败

- **WHEN** mermaid 渲染资源动态加载失败（如异常离线）
- **THEN** 全部 mermaid 块降级为源码 + 失败提示，页面其余功能不受影响

### Requirement: 主题双适配与切换重绘

图渲染 MUST 跟随应用主题令牌（editorial/dark）：editorial 主题下用浅色图主题、dark 主题下用深色图主题。应用主题切换时，已渲染的图 MUST 重绘为目标主题配色（选定方案 C 恒定深底容器时豁免；本 change 已选定 B，重绘为必选行为）。

#### Scenario: 主题切换重绘

- **WHEN** 用户在已渲染 mermaid 图的页面切换 editorial ↔ dark 主题
- **THEN** 已渲染图按新主题重绘，文字与线条在新背景下可读

### Requirement: 共享渲染器全部宿主覆盖

mermaid 渲染行为 MUST 覆盖共享 markdown 渲染器的全部块级调用宿主：阅读页 AI 整理稿、阅读页 firecrawl 正文，以及 QA 面板、因果分析报告、版块富化面板三个 LLM 产出宿主。各宿主行为一致，MUST NOT 出现仅阅读页生效的面板差异。

#### Scenario: 面板宿主同规则渲染

- **WHEN** 用户在 QA 面板（或因果分析/富化面板/候选编辑弹窗）获得含合法 mermaid 块的 LLM 回答
- **THEN** 该块同样渲染为图，行为与阅读页一致
