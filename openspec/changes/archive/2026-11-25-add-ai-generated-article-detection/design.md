## Context

见 `proposal.md` — Why。本节只记录约束本设计的现状事实：

- **判定点在文章处理链末端**：`internal/reader/service/feed_service.go:101`（文章入库）与 `internal/admin/scheduler/job_firecrawl.go:232`（全文抓取完成）是两个既有的内容就绪节点，两者都已存在「按 feed 开关条件入队 tag_jobs」的先例。
- **队列骨架可复用**：`internal/models/job_queue.go:TagJob` 与 `internal/tagmanagement/repository/tag_job_queue.go:TagJobQueue`（Enqueue 幂等更新 / Claim 乐观锁 + 租约）已是本仓标准作业队列范式。
- **AI 路由按 capability 而非 operation 解析**：`internal/platform/airouter/store_vendor.go:78` `LoadRouteWithModels(capability)` 按 `ai_routes.capability` 取路由，operation 只进审计日志。**这意味着「给判定单独绑小模型」只能靠新增 capability 实现**，靠 operation 无法分流。
- **capability 枚举当前六类**：`internal/platform/airouter/store.go:20`（summary / topic_tagging / digest_polish / open_notebook / embedding / feed_discovery）；`docs/reference/flow/ai-summary.md` 红线 2 规定「新增属跨域语义变更」，本次按该流程显式声明。
- **文章实体无空余判定字段**：`internal/models/article.go:Article` 的现有状态字段各有专属语义（`SummaryStatus` / `FirecrawlStatus` / `ContentForm`），判定需要新增列。
- **列表响应是手写窄投影**：`internal/reader/handler/article_handler.go:26` `articleListColumns` 白名单列 + `internal/models/article.go:87` `ToListDict` 手写 map，新增字段必须两处同改。
- **日报线索文章集合受红线约束**：`docs/reference/flow/daily-report.md` 红线 45 要求 `daily_report_threads.related_article_ids` 保持标签对应文章的并集，不被去重逻辑改写；软降级先例见 `front/app/utils/threadFit.ts:26`（离群线索灰显折叠）。
- **数据实测**（只读统计）：近 30 天新增 28921 篇、日均约 932 篇；正文 ≥2000 字符的仅 2974 篇（10.3%），且几乎全部来自已开全文抓取的技术社区源（掘金人工智能、智源社区、博客园）；快讯类源全部未开全文抓取，且占日报线索文章约 78%。
- **列表行字段集是回归红线**：`docs/reference/flow/reading.md` 约束 9 末句「列表行字段集（title/author/时间/已读/收藏/状态图标）不变为回归红线」，加字段须同步改该条文与 `openspec/specs/article-list-projection/spec.md`。

## Goals / Non-Goals

**Goals:**

- 判定链路与标签打标链路彻底解耦：判定失败、超时、模型未绑定均不影响打标。
- 判定覆盖面完全由「订阅源开关」决定，默认零判定、零成本。
- 判定结论可被人在文章详情纠正，且人工结论不被自动结论覆盖。
- 下游只做「标记 + 软降权」，绝不删改日报线索的文章集合与「进日报」口径。

**Non-Goals:**

- 不做存量文章回溯补判（能力上支持手动重判，不做自动全量回填）。
- 不做短文（快讯类）的专门去重/相似度检测——那是另一套问题域，另起 change。
- 不在文章界面暴露判定理由全文、不做判定可信度看板。
- 不改动文章正文取用优先级（展示层仍按 AIContentSummary→FirecrawlContent→Content→Description）。

## Decisions

### D1. 判定走独立作业队列，骨架照抄 tag_jobs

- **结论**：新增 `detection_jobs` 表与 `DetectionJobQueue`，作业以文章为单位；同文章已有未完成作业时原地更新（幂等），不新建行。
- **落点**：`backend-go/internal/models/job_queue.go:TagJob`
- **取舍**：备选是把判定塞进 `tag_jobs` 的 Reason 分支。否决原因：打标作业存在「文章已有标签则跳过、零 LLM 调用」的短路（`internal/tagmanagement/service/core/article_tagger.go:151`），判定挂在打标下会随打标短路一起被跳过，且打标失败会把判定一起拖垮。

### D2. 判定逐篇一次调用，不做批量攒批

- **结论**：一次判定 = 一篇文章 = 一次 LLM 调用，输出一个 0-1 置信分。worker 领取一批作业后逐篇处理。
- **落点**：Scenario「每篇文章一次独立判定」
- **取舍**：备选是一次调用判 N 篇以摊薄成本。否决原因：判定结论只需要一个分数（用户口径「小模型比较菜，输出结果只需要打个分」），批量会引入批内判定互相污染（一批全判 AI 或全判不判），而实测判定池仅约 98 篇/天，批量摊薄的收益远小于污染风险。

### D3. 判定输入按「全文 → RSS 正文 → 摘要」回退，截断为首尾样本

- **结论**：取用优先级 firecrawl_content → content → description；取到的文本截断为「首 3000 字符 + 尾 2000 字符」拼接样本再送模型。
- **落点**：Scenario「判定输入的取用优先级」
- **取舍**：备选是只认全文抓取正文。否决原因：实测 87.4% 的文章抓取正文为空（快讯流源普遍不开全文抓取），只认全文会让订阅源开关对它们完全失效，与「feed 级开关逐源人工配置」的设计前提冲突。截断样本而非全文，是为控制单次调用的 token 成本。

### D4. 判定入队挂在三个内容就绪节点，由 feed 开关过滤

- **结论**：统一入队辅助函数在三个节点调用——① 全文抓取成功完成（输入用全文）；② 全文抓取终态失败降级（输入回退 RSS 正文）；③ feed 未开全文抓取时文章入库后（输入用 RSS 正文）。入队前判 `feed.ai_detection_enabled`。
- **落点**：backend-go/internal/admin/scheduler/job_firecrawl.go:processFirecrawlJob
- **取舍**：备选是只在抓取完成节点入队、失败降级不判。否决原因：抓取失败的文章将永远拿不到判定结论，而它们恰恰是长文源里最需要判定的部分（抓取失败多为付费墙/反爬的长文）。

### D5. 新增第七类 capability `ai_detection`，独立绑模型与并发配额

- **结论**：在 capability 枚举中新增 `ai_detection`，在 `ai_routes` 中为它单独绑定小模型并设独立并发上限；判定调用复用该 capability，operation 名 `tagmanagement.ai_detection`。
- **落点**：backend-go/internal/platform/airouter/store.go:Capability
- **取舍**：备选是复用 `topic_tagging`。否决原因：`internal/platform/airouter/store_vendor.go:78` 按 capability 取路由，复用会让判定共用打标的模型（用户要求小模型）与并发配额（默认 3，会与打标抢位）。代价是触及 `docs/reference/flow/ai-summary.md` 红线 2（枚举固定六类）——按该红线「新增属跨域语义变更」的表述，本 change 以显式声明 + 同步更新红线条文的方式处理，不绕过。

### D6. 判定结论落在 articles 新列，不进辅助标签体系

- **结论**：`articles` 新增四个列：置信分（float）、判定状态（未判定/疑似/已确认非 AI）、判定来源（自动/人工）、判定时间。
- **落点**：backend-go/internal/models/article.go:Article
- **取舍**：备选是把「疑似 AI 生成」做成一个辅助标签（走 `article_topic_tags` + `topic_tags`）。否决原因：辅助标签会参与挂版块与命中率统计（`docs/reference/flow/semantic-board.md` 红线 3/4），把一个「来源属性」混进「语义标签」会污染版块匹配口径；且存量补标需要伪造打标边。新增列可被所有消费端直接投影，且不引入假的打标记录。

### D7. 列表投影新增非正文小字段，并同步改回归红线条文

- **结论**：列表窄投影白名单 `articleListColumns` 增加判定状态列，`ToListDict` 增加对应键；同步修改 `docs/reference/flow/reading.md` 约束 9 末句，把「疑似 AI 标记」显式纳入允许的列表行字段集，并更新 `openspec/specs/article-list-projection/spec.md`。
- **落点**：backend-go/internal/reader/handler/article_handler.go:articleListColumns
- **取舍**：备选是只在详情与下游视图标记、列表不动。否决原因：用户日常扫列表决定点不点开，列表无标记就失去了「避免点进无用文章」的主要落点；且该字段为布尔/短枚举，非正文类，不触碰「列表不得携带正文大字段」的原约束。

### D8. 日报降权只做排序后置，线索文章集合与计数不变

- **结论**：`populateThreadArticles` 在去重后做稳定分区——非疑似文章保持既有相对顺序在前，疑似文章排至末尾；集合成员与条数不变。
- **落点**：backend-go/internal/topicgraph/service/daily_report_merge.go:populateThreadArticles
- **取舍**：备选是把疑似文章从 `related_article_ids` 中剔除。否决原因：直接违反 `docs/reference/flow/daily-report.md` 红线 45（线索文章须保持标签对应文章的并集，不被去重逻辑改写），且会让「N 篇」计数与实际线索内容脱节。

### D9. 候选标签的疑似占比只做名额竞争排序的次级因子

- **结论**：候选标签携带其名下当天文章的疑似占比；`filterTagsByQuality` 在候选数超上限需截断时，把占比作为排序的次级因子（占比高者靠后）。占比按比例计算，候选成员集合、去重规则、泳道分桶均不变。
- **落点**：backend-go/internal/topicgraph/service/daily_report_orchestrator.go:filterTagsByQuality
- **取舍**：备选是把疑似占比作为标签级降权因子提前介入 lane 分桶与阈值判定。否决原因：标签级降权会连坐同标签下的正常文章，并污染泳道分桶（`docs/reference/flow/daily-report.md` 红线 2/6），影响面远超「降权」本意；只放在截断排序的末位因子，是在不动任何既有判定口径的前提下兑现「进日报概率低」。

### D10. 人工纠正写同一组字段，来源标记为人工，自动判定不覆盖人工结论

- **结论**：文章详情的 ⋯ 菜单提供「标记为疑似 AI 生成」/「取消疑似 AI 标记」；写入同一组判定字段并把来源置为人工。判定作业在执行前检查人工结论：已有人工结论则跳过，除非用户显式触发重新判定。
- **落点**：backend-go/internal/reader/handler/article_handler.go:UpdateArticle
- **取舍**：备选是人工纠正走独立的覆盖表（与自动结论并存、读取时再仲裁）。否决原因：多一份状态源就要多一套仲裁规则，且列表/详情/归档/日报四个消费端都要实现仲裁；直接在同一组字段上用「来源」位表达优先级，读取端零仲裁成本。

### D11. 阈值与判定样本参数走 ai_settings 在线可覆盖

- **结论**：判定阈值（默认 0.7）与判定样本长度（默认首 3000 + 尾 2000 字符）存入 `ai_settings`，在线可调，改后立即对后续判定生效。
- **落点**：backend-go/internal/platform/aisettings/config_store.go
- **取舍**：备选是把阈值写死在代码或数据库迁移里。否决原因：误报率是本功能最大的未知数（见 Risks），必须能在不改代码、不重启的前提下调；同时 `docs/reference/flow/daily-report.md` 红线 7 对日报阈值已有「经 ai_settings 在线覆盖」的同类先例。

### D12. 存量文章不自动回溯判定

- **结论**：判定只作用于进入判定范围时的新文章；开启订阅源开关时不对其历史文章批量回填。
- **落点**：Scenario「存量文章保持未判定」
- **取舍**：备选是开启开关即回溯该订阅源全部历史文章。否决原因：一次性 token 成本不可控，且历史文章普遍无全文正文（87.4% 抓取正文为空），回溯判定的置信度远低于新文章，回溯出来的标记反而会稀释标签可信度。回溯需求若出现，走手动逐篇重判入口。

### D13. 界面只做标记与排序，不提供隐藏或删除

- **结论**：四处展示位（版块归档行、阅读页列表行、文章详情、日报线索 chip）只呈现标记；日报只做展开列表排序后置。列表与归档视图均不新增「隐藏疑似文章」开关或筛选维度；任何界面都不提供「删除该文章 / 移出版块」的联动动作。
- **落点**：front/app/features/tags/components/console/BoardArchiveView.vue
- **取舍**：备选是提供「隐藏疑似 AI 文章」开关。否决原因：文章列表是固定行高虚拟列表，隐藏会引入行高与分页口径的额外分支；且隐藏会让用户失去「这篇被判定过」的可追溯性。降权与标记已覆盖「少点进无用文章」的主诉求。

### D14. 判定字段的命名契约（前后端共用，禁止各端自创）

- **结论**：本次新增字段名在实现期固定如下，后端 JSON 键与前端 camelCase 一一对应，不得由实现者另起名字（`openspec/specs/article-list-projection/spec.md` 已固定列表项字段名为 `ai_detection_status`）：

| 含义 | 数据库列 / JSON 键 | 前端字段 | 取值 |
| --- | --- | --- | --- |
| 判定状态 | `ai_detection_status` | `aiDetectionStatus` | `unknown`（未判定）/ `suspect`（疑似）/ `not_ai`（已确认非 AI） |
| 置信分 | `ai_detection_score` | `aiDetectionScore` | 0-1 float，未判定为 0 |
| 判定来源 | `ai_detection_source` | `aiDetectionSource` | `llm`（自动判定）/ `manual`（人工标记） |
| 判定时间 | `ai_detection_at` | `aiDetectionAt` | timestamptz |
| 订阅源开关 | `feeds.ai_detection_enabled` | `aiDetectionEnabled` | bool，默认 false |

- **落点**：backend-go/internal/models/article.go:Article + front/app/types/article.ts
- **接口契约（同时固定，避免前后端各写一套）**：人工标记/取消标记复用 `PUT /api/articles/:article_id`，请求体带 `ai_detection_status`（`suspect` / `not_ai` / `unknown`）+ `ai_detection_source`（固定 `manual`）；显式重判走 `POST /api/articles/:article_id/ai-detection/recheck`（入队一条判定作业，绕过「人工结论优先」的跳过）。
- **取舍**：备选是用 `is_ai_generated` 布尔表达。否决原因：布尔表达不了「已确认非 AI」与「未判定」的区分，也表达不了人工来源；四字段组合才能同时支撑「四态呈现」与「人工结论优先」两条契约。

## Risks / Trade-offs

- **[误报率高，短文尤甚]** → 订阅源开关默认关闭且逐源人工决定覆盖面；阈值经 `ai_settings` 在线可调；界面标注为「疑似」而非「确认」；提供人工纠正入口且人工结论优先。快讯类源建议先单源小样本试开、观察后再决定是否保留。
- **[判定能力未绑定模型 → 判定全部静默失败]** → 失败进 `ai_call_logs` 审计并可在设置页 AI 健康/路由面板观测；界面不弹错误（避免噪声）；`ai_detection` 泳道行的供血说明与路由配置在设置页可见。
- **[判定未完成时日报已生成 → 降权未生效]** → 判定与日报两条链路解耦，重跑日报即按最新判定结论排序（日报本身是整份覆盖重建，幂等）。
- **[capability 从六类扩为七类是跨域语义变更]** → 在 `docs/reference/flow/ai-summary.md` 红线 2 处显式登记，并同步更新 `openspec/specs/ai-capability-routing/spec.md`；不靠「复用现有 capability」绕过。
- **[列表行字段集回归红线]** → 同步修改 `docs/reference/flow/reading.md` 约束 9 末句与 `openspec/specs/article-list-projection/spec.md`，让新字段进入白名单而非破例。
- **[新增队列表扩大运维面]** → 作业表与既有 `tag_jobs` / `firecrawl_jobs` 同构，租约与重试上限沿用既有参数口径，不引入新的调度机制。
- **[日报候选排序多一个因子，可能扰动既有排序稳定性]** → 因子只挂在「超上限截断」这一既有分支的末位，前序因子（层级、分数）不变；候选成员集合不变，影响面被限制在竞争名额的边界情形。

## Migration Plan

1. **迁移顺序**：先加 `articles` 四列与 `detection_jobs` 表（幂等迁移，`ALTER TABLE ... ADD COLUMN IF NOT EXISTS` + `CREATE TABLE IF NOT EXISTS`），再上代码；应用启动的 AutoMigrate 作为兜底。
2. **默认零影响**：新列默认值使存量文章判定状态为「未判定」，界面不显示任何标记；`ai_detection_enabled` 默认关闭，无判定作业产生；`ai_detection` capability 未绑定模型前判定作业只会失败审计、不产生标记。
3. **灰度启用**：用户在设置页为 `ai_detection` 绑定模型 → 在目标订阅源开启「AI 生成检测」开关 → 观察判定结果与误报情况 → 必要时调阈值。
4. **回滚策略**：关闭订阅源判定开关即停止新判定；回滚代码后新增列与 `detection_jobs` 表保留（无害），前端随代码回退不再展示标记；已有人工标记结论在回滚后仍在库中，再次上线即恢复。

## Open Questions

- 判定 prompt 的具体措辞与评分标尺（如何让小模型稳定输出 0-1 而非堆砌形容词）属实现期调优项，可在不改动 spec 与任务拆分的前提下于实现阶段用少量真实样本反复校准。
- 判定样本长度的最终数值（当前取首 3000 + 尾 2000 字符）取决于所选小模型的上下文窗口与成本偏好，可在实现期按绑定模型调整，并经 `ai_settings` 在线调整。
