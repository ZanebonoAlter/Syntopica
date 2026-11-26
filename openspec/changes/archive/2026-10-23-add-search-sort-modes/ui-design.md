<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# UI Design: add-search-sort-modes

## 入口与入口变更（复用契约）

- 入口不变：侧边栏「文章搜索」→ `/search` 独立页。
- 变更：搜索页过滤行（现为「含归档 / LLM 扩展 / 结果摘要」）新增分段切换 **「最新优先」｜「相关度优先」**，默认选中「最新优先」。视觉复用 `BoardTimelinePanel.vue` 既有 `.tags-sort-toggle` 双按钮分段形态（clock/star 图标 + active 用 `--color-accent` / `--color-accent-subtle`），不新造布局模式或组件。
- 控件在所有查询状态可见；首查 `loading` 期间按钮 disabled；追加加载（loadMore）期间可点击，点击会以新排序重查并作废进行中的追加（seq 丢弃旧响应）。

## 受影响状态

| 状态 | 表现 | 恢复 |
| --- | --- | --- |
| initial（未搜索） | 排序控件可见，默认「最新优先」高亮；切换只改本地状态、不请求 | 输入并提交后按当前排序请求 |
| loading（首查） | 排序控件 disabled；结果区骨架屏照旧 | 响应落地后解禁 |
| success | 结果按所选排序展示；切换即按最后查询重查（首查加载中不重入） | — |
| empty（无结果） | 排序控件仍可见；切换触发重查（可能同样空） | 扩展引导照旧 |
| error | 排序控件可见；重试沿用当前排序 | 重试按钮 |
| 语义降级 | 排序控件不受影响；「相关度优先」退化为关键词相关度（RRF 语义分量缺失） | 下次查询自动重试语义 |

## 复用组件与布局模式

- `AppPageShell mode="contained"`（≤1120px 居中）不变；过滤行 flex-wrap 窄屏折行不变。
- 复用 `AppButton` / `AppInput` / `AppToggle` 既有 token 与 disabled 语义；分段按钮为页面内既有模式同构实例（不新增 UI 基础组件）。
- 双主题零特判：只用 `--color-accent*`、`--color-text-muted`、`--color-border-medium` 语义 token。

## 验收映射（指名道姓）

- 组件测试映射（实文件）：
  - `front/app/features/search/composables/useSearch.test.ts`：默认 `sortBy='date'`；请求携带 `sortBy`；切换排序触发重查并写入分页指纹；loadMore 沿用当前排序。
  - `front/app/pages/search.test.ts`：默认「最新优先」active；点「相关度优先」→ 以 `sortBy:'relevance'` 重查；loading 时按钮 disabled；empty/error 下控件仍在。
- 视觉/交互抽查（agent-browser 人工验证）：`ui-verification/report.md` 记录 1440×900 桌面 + 375×667 窄屏截图（editorial 主题；dark 主题走查一次），确认过滤行不横向溢出、active 态对比度正常。
