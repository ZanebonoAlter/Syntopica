# reading-article-pane Specification (Delta)

## ADDED Requirements

### Requirement: mermaid 图以细分隔线轻容器呈现

阅读列内渲染完成的 mermaid 图 MUST 以「上下 hairline 细分隔线 + 小字图题」的轻容器呈现（用户 2026-09-19 原型审批选定方案 B）：上下线使用 `--color-border-subtle` 级令牌，图题位于图下方、小号次要文字色。容器 MUST NOT 使用边框+投影卡片形态（去卡片化红线）；图 MUST 在列宽内等比缩放（`max-width: 100%`），超宽图容器 MUST 以横向滚动兜底、MUST NOT 撑破 840px 列版式。

#### Scenario: 常规图在阅读列内呈现

- **WHEN** 宽屏（≥1440px）阅读含 mermaid 图的文章
- **THEN** 图位于阅读列内、上下以 hairline 细线与正文分区，图题小字在图下方，无卡片边框与投影

#### Scenario: 超宽图横向滚动兜底

- **WHEN** 文章含超宽 mermaid 图，等比缩放至列宽后不可读
- **THEN** 容器在列宽内横向可滚动，阅读列版式不被撑破
