
## 疑似AI判定链路的落点与实测数据事实

change: add-ai-generated-article-detection（specs/design/tasks 已就绪，planning complete）

## 关键代码落点
- **capability 路由按 capability 级解析**（非 operation）：`internal/platform/airouter/store_vendor.go:78` `LoadRouteWithModels(capability)` → `ai_routes WHERE capability=? AND enabled=true`，default 优先、绑定链按 priority 降级。**operation 只进审计日志、不参与模型选择** → 要给判定单独绑小模型只能新增 capability（已定 D5：新增第 7 类 `ai_detection`）。
- **capability 枚举**：`internal/platform/airouter/store.go:20` 六类；`defaultConcurrency` map 在 `airouter/router.go:28`（topic_tagging=3 / digest_polish=2 / embedding=5）。
- **打标队列骨架（判定队列照抄）**：`internal/models/job_queue.go:TagJob`；`internal/tagmanagement/repository/tag_job_queue.go:TagJobQueue`（Enqueue 同 article_id 幂等原地更新 / Claim 乐观锁+租约 / MarkCompleted / MarkFailed）；worker 在 `internal/app/runtime.go:137` 注册。
- **打标短路（判定不能挂在打标下）**：`internal/tagmanagement/service/core/article_tagger.go:151` 已有标签则跳过、零 LLM；:141 跨 feed 同 link 复用直接 return。
- **feed 开关惯例**：开关一律在**入队点**过滤（worker 不读 feed）；`internal/models/feed.go:24-28` 现有 5 个开关（article_summary_enabled / completion_on_refresh / max_completion_retries / firecrawl_enabled / tagging_enabled）。入队点：文章入库 `internal/reader/service/feed_service.go:101`；抓取完成 `internal/admin/scheduler/job_firecrawl.go:processFirecrawlJob`（:232 附近已有 enqueue tag_jobs 的先例）；抓取失败降级 :199。
- **列表投影是手写白名单**：`internal/reader/handler/article_handler.go:26` `articleListColumns` + `internal/models/article.go:87` `ToListDict`（map 不是 struct）→ 加字段必须两处同改。详情走 :52 `ToDict`。
- **文章模型无空余判定字段**：`internal/models/article.go:Article`（ContentForm 枚举可作字段形态先例）。
- **日报线索文章**：`internal/topicgraph/service/daily_report_merge.go:17` `populateThreadArticles` 纯 first-wins 去重、无排序；红线 45 要求保持并集不被改写 → 只能 stable partition 后置。
- **日报候选 tag 质量闸**：`internal/topicgraph/service/daily_report_orchestrator.go:754` `filterTagsByQuality`（kept/weighted 分层 → kept<10 回捞 → kept>30 按 tier→score 截断）→ 疑似占比只能挂在截断排序末位因子。
- **日报线索读路径**：`internal/topicgraph/repository/daily_report_repository.go:430` `GetReportByID`；前端 `front/app/features/tags/components/daily-report/DailyReportTopicSection.vue`（chip 行 :483-513），取数 `front/app/features/tags/composables/useDailyReportReader.ts:54-58`（当前只取 {id,title}）。
- **版块归档视图（用户说的「文章归档」）**：`front/app/features/tags/components/console/BoardArchiveView.vue`（864 行），数据源 `GET /api/semantic-boards/:id/articles`（`front/app/api/semanticBoards.ts:579`），类型 `ArchiveArticle`（`useBoardArchive`）。既有 chip 形态在标签带末尾 + 「入板」chip。
- **文章详情菜单**：`front/app/features/articles/components/ArticleStatusMenu.vue`（`asm-menu-item` 形态 + `asm-menu-error` 内联错误位）；详情标签区 `ArticleContentPreviewPanel.vue:86` `.tag-row`。
- **ai_call_logs**：`internal/models/ai_models.go:155`；写入点全在 airouter 内；截断 `airouter/router.go:24-25`（response_snippet 10000 / prompt 20000 runes）；保留 7 天（`internal/admin/scheduler/job_log_cleanup.go:20`）。
- **ai_settings**：`internal/models/ai_models.go:8`（key unique，value 为 JSON 字符串，数值型存裸字符串）；读写 `internal/platform/aisettings/config_store.go:42/:65`。
- **前端字段映射只允许在 API 模块**：`front/app/api/normalizers/article.ts:79` normalizeArticle；snake_case→camelCase 不得散落到组件层（reading.md 红线 1）。

## 实测数据（只读统计，2026-11-20）
- 文章总数 74292；近 30 天新增 28921（日均 932.7）；今天新增 589。
- **firecrawl_status='completed' 中 87.4%（64371 篇）抓取正文为空**；全文中位长度 0、均值 1071、P90 3677 runes。
- 抓取正文非空的 12.6% 里：500-2000 段 447 篇、2k-5k 段 2944 篇、>5k 段 5909 篇。
- 全文为空文章的 RSS 正文分布：<500 段 53.8%、500-2k 28.5%、2k-5k 12.2%、>5k 4.3%、空 1.2%。
- 近 30 天新增中正文 <500 的占 25791（89%），≥2000 的仅 2974（10.3%）。
- **开 firecrawl 的 10 个 feed 全是长文源**（掘金人工智能 1452篇/30d、智源社区 870、博客园 476、掘金本周最热 148，>2k 占比 90-94%）；快讯类源（华尔街见闻快讯、凤凰网、金融界、V2EX、联合早报）全部未开 firecrawl。→ 判定目标池 ≈ 98 篇/天。
- 日报近 14 天线索文章约 1937 篇：华尔街见闻快讯 583、凤凰网 297、游民星空 276、金融界 199、掘金 143、智源 103、联合早报 97、V2EX 创意 79 / 技术 79、博客园 26 → 快讯类占 78%，可判定（长文）仅约 14%。
- feed 总数 28；开 firecrawl 10 个；开 firecrawl+打标 9 个。

## 已定的产品决策（用户 2026-11-20 拍板）
1. 独立检测队列（detection_jobs），一篇一作业一次调用（不攒批）——「小模型比较菜，输出结果只需要打个分」。
2. 判定输入回退：firecrawl_content → content → description，截断首 3000 + 尾 2000 runes。
3. 只在 firecrawl 完成后判的原始选择被「输入回退」调和 → 三个入队点：抓取完成 / 抓取失败降级 / 未开抓取时入库。
4. 日报降权 = article 级软降权（排序后置）+ tag 级疑似占比作截断次级因子（不删文章、不改候选成员）。
5. 展示 4 处：版块归档行、阅读页列表行、文章详情、日报线索 chip。
6. 误报治理：高置信才标（阈值 ai_settings 在线可调）+ 手动纠正入口（⋯ 菜单两个动作，人工结论优先不被自动覆盖）；不投票、不分档展示。
7. 不回溯存量，只判新增。
8. 新增第 7 类 capability `ai_detection`，独立绑小模型 + 独立并发配额（需更新 ai-summary.md 红线 2）。

## 红线/门禁清单
- `docs/reference/flow/ai-summary.md` 红线 2（capability 固定六类 → 七类，跨域语义变更，须显式登记）
- `docs/reference/flow/reading.md` 约束 9 末句（列表行字段集回归红线 → 须显式纳入疑似 AI 标记）
- `docs/reference/flow/daily-report.md` 红线 45（线索文章并集不可改写，只排序）
- `openspec/specs/article-list-projection/spec.md` 窄投影字段集
- tasks.md 三固定尾节 + `<!-- doc-impact: flow, api, database, standard -->` + complex 档必须有 review 任务行（check-tasks-tail.sh 已通过）

<!-- pinned 2026-11-20T06:17:59Z -->

## 疑似标记的 6 处落点与三个实现坑

实现期「疑似 AI 生成」标记的 6 处落点（原型已按此出稿，见 ui-prototype/README.md）：

1) 阅读页列表行 `ArticleCardView.vue` + `front/app/components/article/ArticleCard.css:327`（`.archived-badge` 在全局 CSS，非 scoped）：角标插在 `<h3 class="row-title">` 内、标题文本之后；`.row-title` 是 -webkit-line-clamp:2，角标随文本流走（长标题时可能落到第二行）；行高固定 100px 不变。

2) 文章详情 `ArticleContentPreviewPanel.vue`（169 行，**无 <style> 块**）：`.tag-row` 定义在全局 `front/app/components/article/ArticleContent.css:159`；`.article-tag` 是 `ArticleTagList.vue` 的 **scoped** 样式 → 不外溢，详情胶囊样式必须在面板内自行声明。胶囊插在 `.tag-row` 内、`<ArticleTagList>` 之后、手动打标按钮之前。

3) ⋯ 菜单 `ArticleStatusMenu.vue`（scoped `.asm-*`）：两个新项加在 `.asm-menu-pop` 内「手动操作」组（`.asm-menu-label`）下、既有三项之后、两处 `.asm-menu-error` 之前；不新增第二个分组标题；失败复用 `.asm-menu-error`（11px，底 color-mix(in srgb, var(--color-error) 8%, transparent)）。

4) 版块归档行 `BoardArchiveView.vue` + `boardArchiveView.css`：紧凑标记追加在 `.ar-row-tags` 内、`.ar-flow-chip`（入板 chip）之后；`.ar-tag` 的 `border: 1px solid` 颜色靠内联 `:style` 注入，复用 `.ar-tag` 类必须同时注入中性色，否则退化成 currentColor。

5) 日报线索 `DailyReportTopicSection.vue:481-512`：`.drm-article` 是 `display:grid; grid-template-columns: 3px auto 1fr` —— **加第 4 个直接子元素会产生隐式列并挤窄标题列**，标记必须嵌在标题 `<span>` 内部。取数 `useDailyReportReader.ts:12` 的 `ArticleTitle` 接口只有 {id,title}，需扩判定字段（字段映射只能在 API 模块，见 reading.md 红线 1）。

6) 订阅源开关 `FeedDetailEditor.vue:269-325`：`.feed-detail__toggles` 内每行 = 16px 图标 + 名称(13px/500) + 说明(11px/muted) + 右侧 `AppToggle`；新行插在「AI 打标签」之后。

三个坑：① `.archived-badge` 用 `--color-text-muted`（10.5px 下对比度仅 3.3:1），新标记必须用 `--color-text-secondary`（≈6.4:1）才达 V1；② `.article-tag` 的背景是硬编码 `rgba(255,255,255,0.72)`，dark 主题不自适应，新胶囊用 `var(--color-bg-elevated)`；③ mdi 图标必须来自本地子集 `front/app/assets/iconify-subset.json`（187 个），`robot-outline` / `check-circle-outline` / `dots-vertical` 均已在子集内，无需重新生成子集。

**引用**：front/app/components/article/ArticleCard.css:327、front/app/features/articles/components/ArticleContentPreviewPanel.vue、front/app/features/articles/components/ArticleStatusMenu.vue、front/app/features/tags/components/console/BoardArchiveView.vue、front/app/features/tags/components/console/boardArchiveView.css、front/app/features/tags/components/daily-report/DailyReportTopicSection.vue:481、front/app/features/tags/composables/useDailyReportReader.ts:12、front/app/features/settings/components/FeedDetailEditor.vue:269

<!-- pinned 2026-11-20T06:47:50Z -->

## 日报线索疑似排序与标记字段的落点

日报侧「疑似标记 + 疑似排后」的落点与口径：

- 排序：`internal/topicgraph/service/daily_report_merge.go:17` `populateThreadArticles` 纯 first-wins 去重、无排序 → 只能在其后做**稳定分区**（非疑似在前、疑似排末尾），集合成员与条数不变（红线：日报线索文章并集不可改写，只排序）。
- 候选 tag 质量闸：`internal/topicgraph/service/daily_report_orchestrator.go:754` `filterTagsByQuality`（kept/weighted 分层 → kept<10 回捞 → kept>30 按 tier→score 截断）→ `ai_suspect_ratio` 只能挂在截断排序的**末位因子**，不得改候选成员与去重口径。
- 读路径投影：`internal/topicgraph/repository/daily_report_repository.go:430` `GetReportByID`（GET /api/daily-reports/:id）；前端 `front/app/features/tags/composables/useDailyReportReader.ts:54-58` 的缓存只回 {id,title}，日报线索 chip 要带标记必须扩这个接口返回。
- 前端 chip 行：`front/app/features/tags/components/daily-report/DailyReportTopicSection.vue:483-513`（展开列表 `.drm-articles` + 折叠态「N 篇」触发器 `.drm-thread__count`；「N 篇」计数不因疑似标记变化）。
- 列表窄投影是手写白名单：`internal/reader/handler/article_handler.go:26` `articleListColumns` + `internal/models/article.go:87` `ToListDict`（返回 map 不是 struct）→ 加判定字段必须这两处同改，且不得引入任何正文类字段。
- 版块文章列表：`GET /api/semantic-boards/:id/articles`（`internal/tagmanagement/handler/board_crud_handler.go:161`），前端 `front/app/api/semanticBoards.ts:579`，类型 `ArchiveArticle`。
- 实测数据（2026-11-20，本地库）：文章总数 74292、近 30 天新增 28921（日均 932.7）；开 firecrawl 的 10 个 feed 全是长文源（掘金人工智能 4112 篇、博客园 1478、少数派 406…），快讯类源全部未开 firecrawl → 判定目标池约 98 篇/天；日报近 14 天线索 1937 篇中快讯类占 78%、可判定（长文）仅约 14%。

**引用**：internal/topicgraph/service/daily_report_merge.go:17、internal/topicgraph/service/daily_report_orchestrator.go:754、internal/topicgraph/repository/daily_report_repository.go:430、front/app/features/tags/composables/useDailyReportReader.ts:54、front/app/features/tags/components/daily-report/DailyReportTopicSection.vue:483、internal/reader/handler/article_handler.go:26、internal/models/article.go:87、internal/tagmanagement/handler/board_crud_handler.go:161

<!-- pinned 2026-11-20T06:48:01Z -->

## 判定链路后端地基已落地：字段/表/capability/服务函数签名

tasks 1.1-1.4 + 2.1-2.4 已完成（后端地基），后续线程可直接接线，无需重探。

## 数据模型（已落库契约）
- `models.Article` 新增四字段（design D14 命名）：`AIDetectionStatus string gorm:"size:20;default:unknown" json:"ai_detection_status"`（unknown/suspect/not_ai）、`AIDetectionScore float64 json:"ai_detection_score"`、`AIDetectionSource string json:"ai_detection_source"`（llm/manual，空=尚无来源）、`AIDetectionAt *time.Time json:"ai_detection_at"`。
- **状态/来源常量的权威定义在 `models` 包**：`models.AIDetectionStatusUnknown/Suspect/NotAI`、`models.AIDetectionSourceLLM/Manual`；另有 `models.NormalizeAIDetectionStatus(string)` 把 Go 零值空串归一为 unknown（GORM `default:unknown` 会让新建行 DB 值为 unknown 但内存仍 ""，投影前必须归一）。投影线程（4.1/4.2/4.3）直接用这些常量 + 归一函数，不要自造字面量。
- `models.Feed` 新增 `AIDetectionEnabled bool gorm:"default:false" json:"ai_detection_enabled"`，已加入 `Feed.ToDict`。
- `models.DetectionJob`（表 `detection_jobs`）：ID/ArticleID/Status/Priority/AttemptCount/MaxAttempts/AvailableAt/LeasedAt/LeaseExpiresAt/LastError/**Reason**/CreatedAt/UpdatedAt/Article。复用 `models.JobStatus*` 四态。Reason 取值约定：`firecrawl_completed` / `firecrawl_failed` / `article_ingested` / `manual_recheck`。

## 迁移
- `internal/platform/database/ai_detection_migration.go` 新文件，版本 `20261120_0001`，已登记进 `postgresMigrations()` 的最后一个 append（在 20261117_0002 之后）。
- 幂等：`ADD COLUMN IF NOT EXISTS`（articles 四列 NOT NULL DEFAULT / feeds 一列）+ `CREATE TABLE IF NOT EXISTS detection_jobs` + 5 个 `CREATE INDEX IF NOT EXISTS`；**Up 开头有 postgres 方言守卫**（`db.Name() != "postgres"` 直接 return nil），SQLite 单测零噪声。
- `migrator.go` 的 RunAutoMigrate 静态模型列表已注册 `&models.DetectionJob{}`（AutoMigrate 兜底建表建列）。

## airouter（task 2.1）
- `airouter.CapabilityAIDetection Capability = "ai_detection"`（第 7 类）；`defaultConcurrency[CapabilityAIDetection] = 2`（独立信号量，与 topic_tagging=3 分桶，可用 ai_routes.max_concurrency 覆盖）。
- **新增 `Router.LogLocalFailure(ctx, req ChatRequest, failure LocalFailure)`**：补写「本地判定失败」审计行（`error_code` = route_not_found / no_providers / parse_error，`request_meta.local_failure=true`）。原因：`Router.Chat` 在 LoadRouteWithModels 失败时早退不写任何行，而红线要求「能力未绑定模型→失败并记审计」。`LocalFailure` 结构体字段：Code/Message/ResponseSnippet/RouteName/ProviderName/ModelName。
- `aihealth.coreHealthCapabilities` **刻意不含** ai_detection（健康门禁只认 embedding+topic_tagging，加入会让判定未绑定时文章入库被 fail-closed 拦住）；ai_detection 仍会自动出现在健康快照 entries 里（entries 由 ListRoutesWithModels 动态枚举）。

## 判定服务（task 2.2/2.3/2.4）—— 文件 `internal/reader/service/ai_detection_service.go`
导出符号（均经 `internal/reader/wire.go` 根门面 re-export，供 admin/tagmanagement/topicgraph 域调用）：

```go
const AIDetectionOperation = "tagmanagement.ai_detection"
const AIDetectionInputFirecrawl/Content/Description = "firecrawl"/"content"/"description"

type AIDetectionService struct{ router AIDetectionChatRouter; config AIDetectionConfigProvider }
func NewAIDetectionService() *AIDetectionService
func NewAIDetectionServiceWithRouter(router AIDetectionChatRouter, config AIDetectionConfigProvider) *AIDetectionService

// 入口 1：只判定不落库
func (s *AIDetectionService) DetectArticle(ctx context.Context, article *models.Article, force bool) (*AIDetectionResult, error)
// 入口 2：判定 + 落库（worker 直接用这个）
func (s *AIDetectionService) DetectAndPersist(ctx context.Context, article *models.Article, force bool) (*AIDetectionResult, error)

type AIDetectionResult struct{ Score float64; Status string; Source string; InputSource string; Threshold float64; DetectedAt time.Time }

// 人工结论优先检查（task 3.4 判定执行前用它跳过）
func HasManualAIDetection(article *models.Article) bool
// 输入回退：firecrawl_content → content → description（返回文本+来源标记，全空返回 "","")
func SelectAIDetectionInput(article *models.Article) (text string, source string)
// 截断样本（按 rune 切，首 head + 尾 tail）
func BuildAIDetectionSample(text string, head, tail int) string
// 置信分解析（容错：整段→首个 0-1 数字；失败返回 (0,false)）
func ParseAIDetectionScore(content string) (float64, bool)
func ApplyAIDetectionResult(article *models.Article, result *AIDetectionResult)
func PersistAIDetectionResult(article *models.Article, result *AIDetectionResult) error

var ErrAIDetectionManualOverride   // 人工结论优先，本次跳过（worker 按「非失败」处理，errors.Is 判定）
var ErrAIDetectionNoInput          // 无可判定文本 / 样本长度配成 0
```

调用形态：`airouter.ChatRequest{Operation: AIDetectionOperation, Capability: airouter.CapabilityAIDetection, Messages: [system=判定器, user=标题+样本], Temperature: &0.0, Metadata:{article_id, feed_id, input_source, sample_runes}}`；**不开 JSONMode**（要求模型输出单行纯数字）。

## ai_settings（task 2.3）
`internal/platform/aisettings/config_store.go` 新增：
```go
type AIDetectionConfig struct{ Threshold float64; SampleHead int; SampleTail int }
func DefaultAIDetectionConfig() AIDetectionConfig            // 0.7 / 3000 / 2000
func LoadAIDetectionConfig() (AIDetectionConfig, error)      // 每次现读，改后立即生效
func SaveAIDetectionConfig(cfg AIDetectionConfig) error      // 校验 (0,1] 与非负长度
```
键名：`ai_detection_threshold`（裸字符串 "0.7"）/ `ai_detection_sample_head` / `ai_detection_sample_tail`（裸字符串整数）。键缺失或值非法 → 回落默认 + Warnf；DB 错误 → 返回 error（服务侧再降级到默认）。
在线改阈值的 HTTP 入口：`POST /api/ai/settings`（`admin/handler/ai_handler.go` 的 `SaveSettings` → `applyAIDetectionSettings`，接受 JSON 数字或数字字符串）；读侧 `GetSettings` 会自动透出这三个键（它遍历全部 ai_settings）。

## 失败语义（三条）
1. 路由缺失/无可用绑定 → Chat 失败 + `LogLocalFailure` 补审计，文章判定状态不改写。
2. HTTP 成功但解析不出 0-1 → 补一条 `error_code=parse_error` 审计行（Router 已写过一行成功行），判定失败。
3. 样本长度为 0 → `ErrAIDetectionNoInput`，不发空样本请求。

## 测试落点
- `internal/reader/service/ai_detection_service_test.go`：四种输入路径、长文截断、置信分解析、阈值边界（=阈值即疑似）、人工结论优先+force 覆盖、阈值在线调整、审计成功/失败两路径、解析失败保持未判定、落库四字段、新文章默认 unknown。
- `internal/platform/airouter/ai_detection_capability_test.go`：capability 字符串与独立并发配额、信号量分桶、判定走自己的路由、max_concurrency 覆盖、LogLocalFailure 写审计行 + 截断 10000 runes。

**引用**：backend-go/internal/reader/service/ai_detection_service.go、backend-go/internal/reader/wire.go、backend-go/internal/models/article.go、backend-go/internal/models/feed.go、backend-go/internal/models/job_queue.go、backend-go/internal/platform/airouter/store.go、backend-go/internal/platform/airouter/router.go、backend-go/internal/platform/aisettings/config_store.go、backend-go/internal/platform/database/ai_detection_migration.go、backend-go/internal/platform/database/migrator.go、backend-go/internal/admin/handler/ai_handler.go

<!-- pinned 2026-11-20T07:54:27Z -->
