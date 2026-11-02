# Design: daily-report-lead-direct-preview

## Context

现状链路：`DailyReportTopicSection`（线索文章点击）/ `TopicDetectiveWall` → `emit('openArticle', articleId)` → `DailyReportReader` 转发 → `DailyReportDeskShell` 置 `articleId` → `DeskArticleDrawer`（AppDrawer 60vw）打开并 `getArticle(id)` 拉详情 → 用户点「站内阅读」→ `openPreview(article)` → `ArticlePreviewModal`。shell 已挂载预览弹窗实例并接好收藏乐观更新（`handlePreviewFavorite` / `handlePreviewArticleUpdate`，daily-report flow 约束 32）。同仓库既有直达先例：`pages/search.vue` 的 `openPreview`（getArticle → normalizeArticle → 弹窗 + loading 态）、`useTagsPage.openArticlePreview`（矩阵页）。

## Goals / Non-Goals

- **Goals**：线索文章点击直达预览弹窗；补齐加载态与失败反馈；移除抽屉组件与死代码；测试与 flow 文档同步。
- **Non-Goals**：不改 `ArticlePreviewModal` 内部（全屏层级/iframe 高度等既有契约）；不改 `openArticle` 事件签名与线索文章标题列表的缓存/重试机制；不动矩阵页/搜索页各自的预览入口。

## Decisions

### D1. 直达接线放 shell，取数模式照抄 search 页
- **结论**：`DailyReportDeskShell` 的 `@open-article` 处理器改为 async：`getArticle(id)` → `normalizeArticle` → 置 `previewArticle`/`previewOpen`；新增 `loadingPreviewArticle` ref 传给弹窗既有 prop。
- **落点**: `front/app/features/tags/components/daily-report/DailyReportDeskShell.vue:openPreview`
- **取舍**：备选一「让 `DailyReportReader`/`TopicSection` 直接带文章对象 emit」——事件链三层都要改签名、侦探墙出口也要跟改，影响面大；备选二「抽屉保留但自动直通弹窗」——死 UI 留着误导。search 页模式只动 shell 一个文件，事件契约（数字 id）零改动。

### D2. 失败反馈用 useNotify，不搬抽屉内嵌重试
- **结论**：详情请求失败 → `notify.warn('文章加载失败…')`、`previewOpen` 保持/复位为关，不实现弹窗内错误重试。
- **落点**：delta spec「详情加载失败可见反馈」Scenario
- **取舍**：备选「弹窗内嵌错误态 + 重试按钮」——要在 ArticlePreviewModal 加新状态通道，违反「不动弹窗内部」边界；notify 是全局既有反馈通道（shell 已 import），成本最低。用户重点一下标题即等价重试。

### D3. 删除 DeskArticleDrawer 组件而非留置
- **结论**：删 `DeskArticleDrawer.vue` 文件及 shell 内 import/渲染/`articleId` ref；`AppDrawer` 组件本体不动（其他宿主仍在用）。
- **落点**: `front/app/features/tags/components/daily-report/DeskArticleDrawer.vue`
- **取舍**：备选「保留组件备用」——唯一使用方消失后即死代码，lint/后续检索都是噪声；git 历史可随时找回。

### D4. delta 基线取 rework delta 的 desk 版全文，归档顺序靠声明约束
- **结论**：本 change 的 REMOVED/MODIFIED 以 `rework-workshop-into-radar-and-desk` delta 中的需求文本为基线（主 spec 尚无这些需求）；proposal/specs 头部注明「rework 先归档、本 change 后归档」。
- **落点**: `openspec/changes/daily-report-lead-direct-preview/specs/daily-report-system/spec.md`（头部注释）
- **取舍**：备选「改主 spec 现状文本」——会把 rework 未归档的 desk 版描述（三栏/日期重置 14 天等）整体回滚成矩阵旧文，归档时互相覆盖；声明顺序是并发 change 下唯一不丢内容的做法（concurrent-change-coordination 的归属地图只管文件冲突，不管 spec 同步顺序）。

## Risks / Trade-offs

- [弹窗比抽屉重（90vw×85vh vs 60vw 侧栏）] → 用户诉求即「直接站内阅读」，信息密度是预期收益而非回归；Esc/遮罩点击关闭链路沿用 AppDialog 既有行为，组件测试断言关闭可达。
- [详情请求慢时先见空弹窗] → loadingPreviewArticle 传弹窗既有 loading 通道，加载态可见；见 delta Scenario「详情加载中呈现加载态」。
- [归档顺序被打破（rework 长期不归档）] → 本 change tasks 归档节写明前置检查；若届时仍未归档，先人工同步 rework delta 到主 spec 再归档本 change。

## Migration Plan

纯前端、无数据迁移。部署 = 前端构建铺静态托管（`bash scripts/dev/deploy-frontend.sh`）。回滚 = revert 单个 commit 重建组件文件即可。

## Open Questions

（无）
