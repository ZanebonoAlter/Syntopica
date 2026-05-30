## Context

Global Settings 的"订阅源配置"tab 是用户管理 feed 级行为的唯一入口。当前每个 feed 卡片只有 3 个控制：刷新间隔、最大文章数、AI 摘要 toggle。但实际数据处理管线有 4 条独立路径（Firecrawl → Tag → Summary → 补全），用户无法逐条控制。同时分类文件夹不可折叠，feed 多时页面极长。

当前 Feed 模型已有 `firecrawl_enabled`、`article_summary_enabled`、`completion_on_refresh` 字段，但缺少 `tagging_enabled`。前端全局设置没有暴露 firecrawl 和补全的 per-feed 控制。

### 当前处理管线

```
Article 入库
  → enqueueArticleProcessing()
    → Firecrawl 队列 (if firecrawl_enabled)
      → 完成后 → Tag 队列 (无条件)
      → 完成后 → Content Completion (if article_summary_enabled)
    → Tag 队列 (直通, if !firecrawl_enabled)
```

关键缺口：Tag 队列没有 feed 级开关，始终会入队。

## Goals / Non-Goals

**Goals:**

- Feed 级控制全部 4 条处理管线：Firecrawl、Tag、AI 摘要、内容补全
- 分类标题可折叠，默认展开，状态记忆在页面生命周期内
- 最大文章数"无限制"语义正确（`0` = 不限制，不是 9999）
- 后端 `enqueueArticleProcessing` 和 Firecrawl 回调都尊重 `tagging_enabled`

**Non-Goals:**

- 不做文章级的前置规则过滤（Infinitum 那套黑名单/评分制，留到后续）
- 不做过滤后的复核机制（个人工具不需要）
- 不做 feed 噪声率统计和自动降级
- 不重新设计 Global Settings 整体布局（只改"订阅源配置"tab 内部）

## Decisions

### D1: `tagging_enabled` 默认 true

新增 `tagging_enabled bool gorm:"default:true"`，现有 feed 默认开启打标签，不影响已有行为。关掉后该 feed 的文章跳过 tag 队列和后续 tag 相关的叙事/图谱分析。

### D2: `max_articles = 0` 表示无限制

后端 `CleanupOldArticles` 检查 `maxArticles <= 0` 时直接 return。前端选项改为 `{ label: '无限制', value: 0 }`。`9999` 值保留兼容但不再作为"无限制"语义。

### D3: Firecrawl 回调检查 tagging_enabled

Firecrawl 完成后当前直接入 tag 队列。改为先读 feed 的 `tagging_enabled`，关闭则跳过。这避免了"关了 tag 但 firecrawl 完成后又自动触发 tag"的问题。

### D4: 折叠状态不持久化

用 `ref<Record<string, boolean>>` 管理，存在组件生命周期内，刷新页面后恢复默认展开。不需要 localStorage 或后端存储，因为用户不会频繁调整设置。

### D5: Feed 卡片管线 toggle 的 UI 布局

4 个 toggle 垂直排列在卡片底部，每个用一行，图标 + 标签 + toggle switch。顺序为 Firecrawl → 打标签 → AI 摘要 → 内容补全，与处理管线执行顺序一致。

## Risks / Trade-offs

- [已有 feed 的 tagging_enabled 默认 true] → 无迁移风险，gorm auto-migrate 加列即可
- [max_articles=0 语义变更] → 已有 feed 的 max_articles 值不变（还是 100），只有用户手动选择"无限制"时才设为 0
- [折叠不持久化] → 可接受的 trade-off，设置页不是高频操作页面
