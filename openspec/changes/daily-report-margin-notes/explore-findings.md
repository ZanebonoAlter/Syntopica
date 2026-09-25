
## 日报页边注关键代码事实

daily-report-margin-notes change 的关键代码事实（探索阶段确认）：

1. 冲突面：`DailyReportTopicSection.vue` 中 thread 标题+摘要在 `<button class="drm-thread__header" @click="toggleThread">` 内（点击=展开/收起相关文章溯源，`:disabled="!thread.related_article_ids?.length"`）；topic 收展在 `<button class="drm-topic__header" @click="toggleTopic">`（active zone 附带 emit ensureLifeline）。头条 lead 摘要是纯 `<p>`（DailyReportMasthead.vue，无按钮，无冲突）。解法：capture 阶段 selection guard（mouseup 有效选区 ≥2 字符→同一点击序列吞掉）+ mark.mn-highlight 挂自身 click stopPropagation 跳卡 + header 不开放批注。
2. 布局：`BoardDailyReportTimeline.vue` `.drm-layout` 现为 `grid-template-columns: 14rem minmax(0,1fr)`，gap clamp(2rem,3vw,2.75rem)，<1100px 塌单列。新第三列页边注栏 clamp(15rem,17vw,17rem)，sticky + 内部滚动。
3. 相关文章链路：`toggleThread` 展开 thread 时 `emit('ensureArticles', thread.related_article_ids)`，related_article_ids 反查豁免 archived（daily-report 约束 10），QA 引用 chips 必须走同一 ensureArticles/文章预览通道。
4. airouter：`CapabilityOpenNotebook = "open_notebook"`（store.go:25，2026-03-12 commit 847f6784 引入）与并发默认值 2（router.go:31）存在，全后端零调用方；收编为首调用方，不新增枚举。ai-capability-routing spec「能力与业务用途绑定」需补第五条绑定（delta spec 已写）。
5. thread 模型：`topicgraph/repository/daily_report_models.go` DailyReportThread{ID,ReportID,SectionID,Title,Summary,TagIDs,Confidence,RelatedArticleIDs JSON,Embedding vector,FitDistance *float64}；日报 period 生成后不可变（backfill 整份覆盖重建除外）→ 批注锚定 = quoted_text + report/section/thread id + 偏移线索即可，无需 Range API。
6. 主题 token：main.css 浅色主题 --color-accent #d94a4a 系、--color-border-strong rgba(26,26,26,.25)、正文 Noto Serif SC；报纸双线框 3px double。
7. 基础组件：AppButton/AppInput/AppDialog(sm=420)/AppSidebarDrawer 可复用；新组件 SelectionAskBubble/MarginNotesRail/MarginNoteCard。
8. **联网扩充（SearXNG，2026-09-24）**：详见 pin「SearXNG 接入页边注问答的现状与先例」与本文件上方 design D7；关键点——本地 `localhost:8889` JSON API 实测可用；dataenrichment `WebSearcher` 接口已有（Noop/Bocha）但仅服务 agent 循环；`dataenrichment/service/lifeline_renderer.go` 预留 import topicgraph 方向 → 客户端落 `internal/platform/searxng` 独立包避免循环引用；配置对齐 `bocha_config`（`aisettings/config_store.go`，DB>env>config.yaml>空）；数据增强红线 10「只用原始网页结果」与 SearXNG 天然对齐；migration 追加函数在 `platform/database/postgres_migrations.go`（Go 函数+append 注册）；前端设置 section 先例 `SettingsWorkspace.vue:40`（`{ key: 'bocha', label: '博查搜索' … }`）。

<!-- pinned 2026-09-21T14:59:10Z；2026-09-24 追加联网扩充事实 -->

## SearXNG 接入页边注问答的现状与先例

## 联网搜索扩充（SearXNG）关键代码事实

### SearXNG 本机实例（2026-09-24 实测）
- `http://localhost:8889/search?q=<query>&format=json` 返回 200，JSON 结构：`{query, results[], answers[], corrections[], infoboxes[], suggestions[], unresponsive_engines[]}`；results 项含 `title/url/content(摘要)/engine/score/publishedDate`，中文查询「央行逆回购」返回 39 条。
- 本地零 API 成本，无 key；延迟约 1-3s（聚合多引擎）。

### 现有 WebSearcher 抽象（dataenrichment 域）
- `backend-go/internal/dataenrichment/service/web_search.go`：`WebSearcher interface { Search(ctx, query) ([]WebSearchResult{Title,URL,Snippet}, error) }`。
- 已有实现：`NoopWebSearcher`（默认，报错降级）、`BochaWebSearcher`（读 `bocha_config`，DB(ui)>env>config.yaml 动态现读，summary:false 拒 AI 摘要防幻觉，只保留非空 url 供 evidence_chain 可点）。
- 消费方：agent 循环 `web_search` 工具（`tool_registry.go:220`，RegistryOption `WithWebSearcher`），wiring 在 `dataenrichment/wire.go:92`。**页边注 QA 不走 agent 循环，是单次 airouter 调用，不直接受益。**
- SearXNG 尚无 Go 实现。

### 页边注 QA 现状（本 change 已实现，未归档）
- `backend-go/internal/topicgraph/service/margin_notes_qa.go`：`AskMarginNote` = 上下文组装（quoted_text+thread+前3篇文章摘录≤300 runes）→ 单次 airouter `open_notebook`（operation `daily_report.margin_note_qa`，JSONMode+JSONSchema）→ `{answer, cited_article_ids[], terms[]}` → cited 白名单（集外剔除，空=`PureModelKnowledge`）→ QA 落库 → terms 失败隔离 upsert（D6）。
- `marginNoteChatFn` 可替换钩子（测试 stub 用），prompt 由 `buildMarginNoteQAPrompt` 组装（文件后半）。
- 表 `annotation_qas`：`cited_article_ids jsonb / extracted_terms jsonb / operation / provider / model(空)`；回修轮（tasks §9）已把提问响应改为按 GET 同形状返回 `data.qa`（terms 带 `{term,is_new}`）。
- migration 已跑过（2.1 完成），加列须追加新 migration。

### 配置面先例
- `platform/aisettings/config_store.go`：`bocha_config` 键模式（key+endpoint+enabled，Load/Save，动态现读即时生效）。SearXNG 配置应对齐此模式（如 `searxng_config: {endpoint, enabled}`）。

### 待决策（用户）
1. 触发策略：每次必搜失败静默降级 / UI 开关 / LLM 两阶段
2. SearXNG 实现落点：通用 `WebSearcher` 实现（惠及 dataenrichment agent 循环）vs 页边注私有
3. 网络来源结构化沉淀（annotation_qas 加 cited_web_sources jsonb + 卡片 chips）与否

<!-- pinned 2026-09-24 web-search expansion exploration -->

<!-- pinned 2026-09-24T13:55:09Z -->
