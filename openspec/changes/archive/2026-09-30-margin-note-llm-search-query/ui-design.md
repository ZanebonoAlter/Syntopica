<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A Reason

本 change 翻案 D7 搜索策略为纯后端改造：LLM 生成搜索 query、多 query 并行搜索合并、失败回退锚文本。回答契约 `{answer, cited_article_ids[], web_sources[], terms[]}` 与 `cited_web_sources` 结构不变，前端卡片 chips、管理页展示、SearXNG 设置分区全部不变（proposal ui-impact: none）。无任何用户可见界面变化，无需 UI 设计制品。
