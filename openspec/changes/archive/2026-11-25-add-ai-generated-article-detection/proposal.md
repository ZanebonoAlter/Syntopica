<!-- complexity: complex -->
<!-- ui-impact: minor -->
<!-- constraint-domains: ai-summary, content-enrichment, reading, daily-report, semantic-board -->

## Why

每日入库文章里混有 AI 撰写/洗稿的内容，现在只能靠人工点开才发现，浪费阅读时间；同时这类内容挤占日报线索名额，让日报的"值得读"密度下降。

实测规模：近 30 天新增 28921 篇、日均约 932 篇；其中正文 ≥500 字符的仅 3130 篇（10.8%），且长文几乎全部来自已开启全文抓取的源（掘金人工智能、智源社区、博客园），正是 AI 洗稿最泛滥的技术社区长文。日报线索近 14 天约 1937 篇中，长文类占约 14%。

判定可由小模型单次调用完成（输出一个 0-1 置信分），在长文池上日均约 98 篇，增量成本可忽略；但**判定覆盖面必须由人按订阅源显式控制**——快讯流源（华尔街见闻快讯、凤凰网资讯等，占日报文章 78%）文本过短，文风判定误报率高，若无开关闸门会造成大面积误标、标签迅速失去可信度。

## What Changes

- **新增 AI 生成检测链路**（新能力 `ai-generated-article-detection`）：
  - 新增 `detection_jobs` 队列（骨架对齐 `tag_jobs`：一 job 一 article、乐观锁 claim、租约、重试幂等），独立于打标队列，判定失败不影响打标。
  - 新增第 7 类 AI capability `ai_detection`（现为六类：summary/topic_tagging/digest_polish/open_notebook/embedding/feed_discovery），在 `ai_routes` 独立绑定**小模型**与独立并发配额，不与打标抢 `topic_tagging` 的并发位。
  - `articles` 新增判定字段：置信分、状态（unknown/suspect/not_ai）、来源（llm/manual）、判定时间。
  - **feed 级开关** `feeds.ai_detection_enabled`（默认关闭），逐订阅源人工开启；判定输入按 `firecrawl_content` → `content` → `description` 优先级取用（全文抓取成功用全文，否则回退 RSS 内容）。
  - **阈值在线可调**（`ai_settings`，默认 0.7），高于阈值才标记"疑似 AI 生成"。
  - **人工纠正入口**：文章详情 ⋯ 菜单提供「标记为疑似 AI 生成」/「取消标记」；人工结论优先，后续自动重判定不覆盖（除非用户主动触发重判）。
- **下游展示与降权**（4 处，均在既有视图内加标记，不引入独立页面或弹窗）：
  - 版块态势页「归档」视图文章行：标签带末尾追加「疑似 AI 生成」chip。
  - 阅读页列表行：标题旁追加「疑似 AI 生成」徽标。
  - 文章详情标签区：追加「疑似 AI 生成」静态 chip。
  - 日报线索文章 chip：追加「疑似 AI 生成」标记，且疑似文章在展开列表中排至末尾（软降权）。
- **日报降权**：日报候选的 tag 按其下当天文章的疑似 AI 占比（`ai_suspect_ratio`）排序，占比高者在名额竞争中靠后；仅影响竞争排序，不改变线索文章集合。
- **不硬删任何文章**：日报线索的文章集合与「进日报」判定口径保持不变，降权一律以排序后置 + 打标表达。

## Capabilities

### New Capabilities

- `ai-generated-article-detection`: AI 生成疑似度判定链路——独立检测队列、`ai_detection` capability 路由与并发配额、文章级判定字段、feed 级开关与判定输入优先级、置信阈值在线覆盖、人工纠正与人工结论优先。

### Modified Capabilities

- `ai-capability-routing`: capability 枚举由六类扩为七类（新增 `ai_detection`），新 capability 独立绑定模型与并发配额，纳入路由配置与降级观测口径。
- `article-list-projection`: 列表窄投影新增文章级 AI 疑似标记字段（非正文类），并同步「列表行字段集不变」的回归红线条文。
- `reading-list-panel`: 阅读页列表行新增「疑似 AI 生成」徽标（复用既有徽标形态，不新增交互模式）。
- `reading-article-pane`: 文章详情标签区新增「疑似 AI 生成」chip，⋯ 菜单新增标记/取消标记动作。
- `workshop-article-archive`: 版块归档视图文章行新增「疑似 AI 生成」chip。
- `board-article-api`: 版块文章列表响应新增文章级 AI 疑似标记字段。
- `daily-report-system`: 日报线索文章新增疑似标记展示与展开列表排序后置；候选 tag 新增疑似占比作为名额竞争的次级排序因子。
- `feed-settings-ui`: 订阅源详情新增「AI 生成检测」toggle，与既有处理管线 toggle 同层、即时保存。

## Impact

- **后端**：`internal/models`（Article/Feed 字段 + `DetectionJob` 模型）、`internal/platform/database`（迁移登记）、`internal/reader/handler`（列表投影列与详情 DTO）、`internal/tagmanagement`（队列骨架/worker）、`internal/platform/airouter`（capability 枚举与路由）、`internal/topicgraph/service`（日报降权与候选占比）、`internal/admin/scheduler`（firecrawl 完成后的判定入队点）。
- **前端**：`front/app/types` 与 `front/app/api/normalizers`（字段映射）、`features/articles/components`（列表行/详情 chip 与菜单动作）、`features/tags/components/console/BoardArchiveView.vue`（归档行 chip）、`features/tags/components/daily-report`（线索 chip）、`features/settings/components/FeedDetailEditor.vue`（toggle）、`front/app/stores/api.ts`（feed 字段映射）。
- **AI 调用**：新增 `operation=tagmanagement.ai_detection`，沿用 `ai_call_logs` 审计与截断约束；用户需在设置页为 `ai_detection` capability 绑定模型（未绑定时判定作业失败并计入失败审计，不静默跳过）。
- **红线触及**：`ai-summary.md` 红线 2（capability 固定六类 → 七类，属跨域语义变更）、`reading.md` 约束 9（列表行字段集回归红线）、`daily-report.md` 红线 45（线索文章集合不变，仅排序）。
- **存量数据**：不回溯补判；存量文章判定状态保持 unknown，界面不显示标记。
