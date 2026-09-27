# 前端列表消费点反查（task 3.3）

> change: `slim-article-list-payload`｜范围：`GET /api/articles` 列表窄投影后，前端是否仍有消费点依赖列表行携带的 `content` / `description` / `firecrawl_content` / `ai_content_summary`。
> 结论先行：**列表窄投影无遗漏消费点**；唯一受影响的 store 搜索过滤是无 UI 消费者的死路径（见 §1.3）。

## 0. 命令与输出留痕

### 0.1 正文/描述字段反查（改动前基线）

```console
$ cd front && grep -rn '\.content\b\|\.description\b' app --include='*.vue' --include='*.ts' | grep -v test
app/features/settings/components/ReferenceRolePanel.vue:55:  const content = e.content.trim()
app/features/settings/components/ReferenceRolePanel.vue:133:            <span class="rr-len" :class="{ over: charLen(r.content) > 4000 }">
app/features/settings/components/ReferenceRolePanel.vue:134:            {{ charLen(r.content) }}/4000 字符
app/features/settings/components/ReferenceRolePanel.vue:137:          <p class="rr-preview">{{ r.content.slice(0, 120) }}{{ charLen(r.content) > 120 ? '…' : '' }}</p>
app/features/settings/components/ReferenceRolePanel.vue:166:          <span>画像正文（方法论描述，注入 prompt；{{ charLen(editing.content) }}/4000 字符）</span>
app/features/settings/components/ReferenceRolePanel.vue:167:          <textarea v-model="editing.content" rows="12" class="rr-input rr-textarea" placeholder="分析基因条目：每条一个方法论模式，如「概念考古：先问这个词是谁发明的、为谁服务的…」" />
app/features/settings/components/AnalysisMethodPanel.vue:92:    name: m.name, title: m.title || '', summary: m.summary || '', content: m.content,
app/features/settings/components/AnalysisMethodPanel.vue:110:  const content = f.content.trim()
app/features/settings/components/AnalysisMethodPanel.vue:291:          <textarea v-model="form.content" rows="10" class="am-textarea" placeholder="每行或每段一个步骤，例如「概念考古：先问这个词是谁发明的、为谁服务的…」" />
app/features/settings/components/SettingsWorkspace.vue:91:          <p class="settings-header__desc">{{ currentMeta.description }}</p>
app/features/tags/composables/useTagsPage.ts:138:      description: row.description,
app/features/tags/composables/useBoardCRUD.ts:68:    editDescription.value = board.description || ''
app/features/tags/components/CompositeLabelPool.vue:151:          <p v-if="item.description" class="clp-item-desc">{{ item.description }}</p>
app/features/tags/components/AddSemanticBoardDialog.vue:37:    description.value = props.initialData?.description ?? ''
app/features/tags/components/UpgradeSuggestionPanel.vue:341:              <p v-if="row.description" class="usp-item-desc">{{ row.description }}</p>
app/features/tags/components/BoardEnrichmentPanel.vue:241:  narrativeDraft.value = currentContext.value?.content ?? ''
app/features/tags/components/BoardEnrichmentPanel.vue:557:              <div class="seg-b markdown-body" v-html="renderMarkdown(currentContext.content)" />
app/features/discovery/components/CandidateSubscribeDialog.vue:267:              <p v-if="spec.description" class="sub__hint">{{ spec.description }}</p>
app/features/discovery/components/DiscoveryRunCard.vue:55:    <p v-if="item.description" class="run-card__desc">{{ item.description }}</p>
app/features/discovery/components/CandidateLibrary.vue:276:          <p v-if="c.description" class="library__desc">{{ c.description }}</p>
app/features/discovery/components/DiscoveryCard.vue:155:              <p v-if="spec.description" class="discovery-card__desc">{{ spec.description }}</p>
app/features/discovery/components/CandidateEditDialog.vue:88:  const manualDesc = c?.manualMetadata?.description ?? ''
app/features/discovery/components/CandidateEditDialog.vue:91:  description.value = (c?.kind === 'rsshub' ? manualDesc || c?.route?.description || c?.description : c?.description) ?? ''
app/features/articles/composables/useArticleProcessingStatus.ts:117:  return toPlainText(article.description || article.content)
app/features/articles/composables/useArticleContentView.ts:147:    content: mergedArticle.value?.content,
app/features/articles/composables/useArticleContentView.ts:171:    return shouldShowArticleDescription(mergedArticle.value.description, displayContent.value)
app/features/articles/components/ArticleContentPreviewPanel.vue:117:      <div v-html="article?.description" />
app/features/ai/composables/useAIRouterSettings.ts:299:            description: existingRoute?.description, provider_ids: ids,
app/components/dialog/EditCategoryDialog.vue:19:const description = ref(props.category.description)
app/components/dialog/AddFeedDialog.vue:139:                {{ preview.description || '无描述' }}
app/components/common/AppTooltip.vue:35:  if (props.disabled || !props.content) return
app/utils/routeParams.ts:80:          out[rec.name] = typeof rec.description === 'string' ? rec.description : ''
app/utils/routeParams.ts:92:        out[key] = typeof rec.description === 'string' ? rec.description : ''
app/utils/articleContentSource.ts:21:  const originalContent = cleanContent(input.content)
app/stores/articles.ts:76:        a.description.toLowerCase().includes(searchLower)
app/stores/api.ts:134:        description: feed.description || '',
app/api/discovery.ts:213:      description: r.description || '',
app/api/discovery.ts:241:      description: p.description || '',
app/api/discovery.ts:311:      description: input.description,
app/api/discovery.ts:336:    if (patch.description !== undefined) body.description = patch.description
app/api/discovery.ts:406:      description: p.description || '',
app/api/normalizers/article.ts:76:    description: article.description || '',
app/api/normalizers/article.ts:77:    content: article.content || '',
```

> 上表为**改动前基线**。本 change 触碰的 3 个文件（`useArticleContentView.ts` +3 行、`ArticleContentPreviewPanel.vue` +2 行、`normalizers/article.ts` +5 行）改动后行号有位移，命中集合不变。

### 0.2 `fetchArticles` 反查

```console
$ cd front && grep -rn 'fetchArticles\b' app --include='*.vue' --include='*.ts' | grep -v test
app/features/feeds/composables/useAutoRefresh.ts:36:        const { useArticlesStore } = await import('~/stores/articles'); await useArticlesStore().fetchArticles({ per_page: 10000 })
app/stores/articles.ts:30:  async function fetchArticles(filters?: ArticleFilters) {
app/stores/articles.ts:258:    fetchArticles,
```

### 0.3 store 搜索过滤消费者反查

```console
$ cd front && grep -rn 'filteredArticles\|updateFilters(\|filters\.value\.search' app --include='*.vue' --include='*.ts' | grep -v test
app/stores/articles.ts:59:  const filteredArticles = computed(() => {
app/stores/articles.ts:72:    if (filters.value.search) {
app/stores/articles.ts:73:      const searchLower = filters.value.search.toLowerCase()
app/stores/articles.ts:219:  function updateFilters(newFilters: Partial<FilterState>) {
app/stores/articles.ts:253:    filteredArticles,
```

（`resetFilters` / store `filters` 的其他外部读写同样只命中 `stores/articles.ts` 自身；`useArticlePagination.ts:57` 的 `...filters.value` 是分页 composable 自己的本地 filters，与 store 无关。）

### 0.4 `useRefreshPolling` 的 `per_page: 10000` 留痕（task 4.5）

```console
$ grep -n 'per_page: 10000' front/app/features/feeds/composables/useRefreshPolling.ts
22:        await apiStore.fetchFeeds({ per_page: 10000 })
24:        await apiStore.fetchFeeds({ uncategorized: true, per_page: 10000 })
31:          per_page: 10000,
34:        await apiStore.fetchFeeds({ per_page: 10000 })
```

## 1. 命中逐类判定

### 1.1 `ArticleCardView.vue`（列表行）— 不受影响

该文件**不在** §0.1 命中里（不读 `.content`/`.description`）。实测它实际读取的 `Article` 字段：

- 直接读取：`id`、`feedId`、`title`、`pubDate`、`author`、`read`、`favorite`、`imageUrl`、`firecrawlError`、`completionError`、`tagCount`；
- 经 `getArticlePipelineState` / `getFirecrawlStatusMeta` / `getSummaryStatusMeta` 读取：`firecrawlStatus`、`summaryStatus`、`firecrawlCrawledAt`、`summaryGeneratedAt`；
- 自身不读 `link`（click 把整个 article 上抛父层），但 `link` 也在窄投影列清单内。

**结论**：窄投影（id/feed_id/title/link/image_url/pub_date/author/read/favorite/summary_status/created_at + tag_count 子查询）覆盖以上全部字段 → 列表行渲染不受影响。

### 1.2 `ArticleContentPreviewPanel.vue` / `useArticleContentView.ts` — 详情路径，仅首帧导语改走 excerpt

- `ArticleContentPreviewPanel.vue:117`（基线）导语 `v-html="article?.description"`：详情返回后 `description` 仍完整；**详情未就绪的首帧**改为回退列表 `excerpt`（纯文本 `v-text` 渲染）。
- `useArticleContentView.ts:147`（`contentSources` 读 `mergedArticle.content`）：首帧列表行 `content` 为 `''` → `getArticleContentSources` 判无可用正文源 → `displayContent` 为空，走既有空正文兜底；详情返回覆盖后恢复正常（两段式流程不变，design D3/D7 已声明首帧正文为空可接受、详情返回后 MUST 非空）。
- `useArticleContentView.ts:171`（`showDescription` guard）：输入改为 `ledeText`（详情 `description` 优先 → 列表 `excerpt` 回退），guard 判定逻辑本身未改。
- `app/utils/articleContentSource.ts:21`：`input.content` 是上述调用链的入参，属同一路径。

**结论**：唯一行为变化是首帧导语来源（`excerpt`）；正文/完整导语仍由详情接口提供。

### 1.3 `app/stores/articles.ts:76` 搜索过滤读 `a.description.toLowerCase()` — 无 UI 消费者的死路径，无用户可见影响

- §0.3 证据：`filteredArticles` 全仓只在 store 内定义（:59）与导出（:253），**零外部引用**；`updateFilters(` 只在 store 内定义（:219），零调用点；`filters.value.search` 只在 store 内读取（:72-73），无写入点（`resetFilters` 也无外部调用）。
- store 的真实外部消费者只有：`AppSidebarView.vue`（`markAllAsRead`、`favoriteCount`）、`FeedLayoutShell.vue`（`fetchArticlesStats`、`markAllAsRead`）、`useArticleContentView.ts`（`articles` 里按 id 找行做状态同步，见 §1.6）、`useAutoRefresh.ts`（`fetchArticles`，见 §0.2）。
- 主列表页（`FeedLayoutShell`）的文章数据来自 `useArticlePagination`（直连 API），**不经过 store.articles**；搜索/筛选由后端参数承担。

**结论**：投影后 `description` 为空串会让 store 内 `filteredArticles` 的文本搜索失效，但该 computed 与 `updateFilters`/`resetFilters` 均无任何 UI 消费者（死代码），**无用户可见影响**；本 change 不做额外改动（清理死代码不属本 change 范围）。

### 1.4 `useArticleProcessingStatus.getSummaryPreview` — 死代码，无影响

全仓（`front/app`、`front/tests`、仓库根 grep `*.ts|*.vue|*.md`）只有定义处命中：

```console
$ grep -rn 'getSummaryPreview' front/app front/tests
front/app/features/articles/composables/useArticleProcessingStatus.ts:113:export function getSummaryPreview(article: Article): string {
```

**结论**：无消费者。即使投影后 `aiContentSummary`/`description`/`content` 缺失（返回空串），也不影响任何界面。

### 1.5 `useRefreshPolling.ts` 的 `fetchFeeds({ per_page: 10000 })` — 保持不动

该调用打的是 `/api/feeds`（响应仅 14.6 KB，非本 change 痛点），不读文章正文类字段。§0.4 输出留痕；本 change 不修改该文件（task 4.5）。

### 1.6 `useArticleContentView.syncCurrentArticle` 读 `useArticlesStore().articles` — 不崩、值为空

- 代码：按 `props.article.id` 在 store 列表找行，仅 `Object.assign` 处理状态字段（`summaryStatus`/`completionAttempts`/`completionError`/`summaryGeneratedAt`/`aiContentSummary`/`firecrawlContent`/`firecrawlStatus`/`firecrawlError`、标签结果、read/favorite）。
- 投影后 store 行里的 `description`/`content` 为空串、`firecrawlContent`/`aiContentSummary` 为 `undefined`：`find` 与 `Object.assign` 都不读这些字段，**不抛错**；被同步进去的字段以传入值为准。
- 该 store 列表无其他正文类字段消费者（§1.3），因此空值不外溢。

**结论**：行为安全，无需改动。

### 1.7 非 Article 类型命中（§0.1 其余全部行）— 与列表投影无关

以下命中均属其他领域模型或通用字段，不消费 `/api/articles` 列表行：

| 文件:行 | 类型/用途 |
| --- | --- |
| `settings/ReferenceRolePanel.vue` 55/133/134/137/166/167 | 分析基因（reference role）表单字段 |
| `settings/AnalysisMethodPanel.vue` 92/110/291 | 分析方法表单字段 |
| `settings/SettingsWorkspace.vue:91` | 设置分区 meta 描述 |
| `tags/useTagsPage.ts:138` | 升级建议/标签页行模型 |
| `tags/useBoardCRUD.ts:68`、`AddSemanticBoardDialog.vue:37` | SemanticBoard 描述 |
| `tags/CompositeLabelPool.vue:151` | 复合标签描述 |
| `tags/UpgradeSuggestionPanel.vue:341` | 升级建议行描述 |
| `tags/BoardEnrichmentPanel.vue:241/557` | 版块增强 context 正文 |
| `discovery/*`（5 处） | 候选源/spec 描述 |
| `ai/useAIRouterSettings.ts:299` | AI 路由描述 |
| `dialog/EditCategoryDialog.vue:19` | Category 描述 |
| `dialog/AddFeedDialog.vue:139` | Feed 预览描述 |
| `common/AppTooltip.vue:35` | 组件 props `content` |
| `utils/routeParams.ts:80/92` | 路由 record 通用字段 |
| `stores/api.ts:134` | Feed normalizer |
| `api/discovery.ts`（5 处） | 候选源/spec 描述 |
| `api/normalizers/article.ts:76/77` | 本 change 修改点：normalizer 自身兜底 |

## 2. 判定与偏离

### 2.1 `Article.description` / `Article.content` 保持必填 `string`（对 tasks.md 3.1 字面表述的有意收窄）

tasks.md 3.1 写「`ArticlePayload`/`Article` 类型的 `content`/`description`/`firecrawl_content` 转为可选」，实际实现按本子线程 brief 收窄为：

- `ArticlePayload`（API 入参）：`description?` / `content?` / `firecrawl_content?` / 新增 `excerpt?` —— 后端列表投影后这些键**不存在**，入参必须可选；
- `Article`（normalizer 出参）：`description: string` / `content: string` 保持必填，新增 `excerpt?: string`。

理由：`normalizeArticle` 已用 `|| ''` 兜底（现在补上 `excerpt: article.excerpt || ''`），出参类型上这两个字段**永远存在**（列表为空串、详情为完整值）。若改成 `description?: string`，全仓多处 `a.description.xxx` / `article.content.xxx` 消费点（`ArticleContentPreviewPanel.vue`、`useArticleContentView.ts`、`useArticleProcessingStatus.ts`、`stores/articles.ts` 等，含本 change 辖区外文件）会因 TS 可选链检查连锁改动，**零行为收益**（值仍是空串/完整值，运行时语义不变），只增加回归面。`firecrawlContent` 出参保持 `string | undefined`（原样，未收紧也未放松）。

### 2.2 `excerpt` 在 `Article` 上为可选

`normalizeArticle` 始终写入 `excerpt: article.excerpt || ''`，但类型上保留 `?`：详情接口不返回 `excerpt`（旧 bundle 兼容窗口/存量调用点构造 `Article` 字面量时无需补齐），且 guard 侧用 `article.excerpt?.trim()` 容错，与 `firecrawlContent?` 的既有风格一致。

### 2.3 未处理项

无。§1.1–1.6 覆盖 brief 要求核查的全部条目；§1.7 逐行归类为非 Article 命中；未发现「依赖列表正文/描述的活跃 UI 消费点」。
