# #3 - 前端操作错误反馈缺失：8 个半开环用户故事

## What to build

用户闭环审计发现 8 个用户故事的操作失败时无可见反馈（静默失败或 console-only）。统一修复为 "API 失败时显示 toast 提示" 模式，复用 `refreshMessage` toast 机制。

## 受影响故事及修复方式

### P1 — 手动打标签 WebSocket 断连 spinner 永转
- **Story:** C3 手动打标签
- **File:** `ArticleContentView.vue`
- **Fix:** 为 `manualTaggingLoading` 添加超时保护（如 30s），超时后清除 loading 并显示错误提示
- **Acceptance:** 手动打标签后若 WS 30s 未响应，spinner 停止并显示 "标签生成超时" 错误

### P1 — B1 文章详情加载失败静默
- **Story:** 打开文章时 `getArticle` 失败
- **File:** `FeedLayoutShell.vue` hydrateSelectedArticle
- **Fix:** catch 中设置 `refreshMessage` error toast
- **Acceptance:** 文章详情加载失败时顶部显示红色 toast

### P2 — C11/C12 队列重试失败静默
- **Story:** Embedding 队列 / 合并重算队列重试
- **Files:** `EmbeddingQueuePanel.vue`, `MergeReembeddingQueuePanel.vue`
- **Fix:** `console.error` 替换为 `pushMessage('error', ...)` 或等效 toast
- **Acceptance:** 重试失败时面板显示错误信息

### P2 — F3 阅读偏好更新无反馈
- **Story:** Settings > 阅读偏好 > 点击"更新偏好"
- **File:** 相关 preferences 组件
- **Fix:** triggerUpdate 后显示成功/失败 toast
- **Acceptance:** 更新偏好后显示成功或失败提示

### P3 — C4 关注标签回滚无提示
- **Story:** 点击标签爱心关注/取关，API 失败时静默回滚
- **File:** `ArticleContentView.vue`
- **Fix:** catch 中添加 toast 提示 "操作失败，请重试"
- **Acceptance:** 关注操作失败时显示提示

### P3 — F2 设置页刷新 Feed 失败静默
- **Story:** Settings > 订阅源配置 > 单个 Feed 刷新
- **File:** 相关 settings 组件
- **Fix:** 刷新失败时显示 toast
- **Acceptance:** 刷新失败时显示错误提示

## Acceptance criteria

- [ ] 上述 6 个场景操作失败时均有用户可见的错误提示（toast 或面板内联）
- [ ] `pnpm lint && pnpm exec nuxi typecheck` 通过
- [ ] 手动验证至少 2 个场景的错误反馈可见

## Blocked by

None - can start immediately.
