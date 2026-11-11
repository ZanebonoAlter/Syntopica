<!-- complexity: complex -->
<!-- ui-impact: minor -->
<!-- constraint-domains: semantic-board, topic-graph, daily-report, ai-summary -->

# 基于 embedding 的相关事件（重做）

## Why

用户最终决定：「我觉得可以按embedding的来做相关事件就行，没必要做到什么阶段发酵」「可以直接重做change」。

旧方案试图判断同一事件、跨天阶段和发酵，再把日报 thread 归并到事件线，导致三重耦合：叙事单元被当成事实身份；subject_key 要求模型预先完成实体归一；实体注册表会把错误身份固化。本次放弃这些目标，回答更简单的问题：**读到一个事件时，还有哪些事件值得一起看？**

thread 是日报叙事单元；event 标签是事件描述，不预设其已经代表全局唯一事实；相关性不是同一事实身份。名称不同的主体可出现在相关事件里，无须先归一。

目录名保留以兼容已有引用，不表示继续实施跨天合并。旧版原文在 `legacy/previous-plan/`，已落地实验及纠错见 `legacy-experiments.md`。旧探索记录只作历史，不是本方案的规范。

## What Changes

- 以 `topic_tags.category=event` 的标签为比较单位，使用名称、描述、已有辅助标签的确定性文本计算 embedding。
- 在同版块内返回余弦相似度达到阈值的**直接相关事件**。不依赖 thread 向量、watch 生成方式、story_id、subject_key、实体 ID 或指纹信任标志。
- 默认阈值 **0.86**（2026-11-10 S3 实测后由起始值 0.82 上调，历史实测数据原样保留），通过 `ai_settings.related_event_similarity_threshold` 在线覆盖，前端既有「匹配规则」弹窗提供配置。
- 不存事件合并结果、不传递扩散、不聚成身份簇；相关事件各自保留标签、文章和身份。
- 初版为同版块近 30 天候选、最多 10 条结果；这是设计默认，不是用户指定的已标定参数。时间按标签创建时间筛选，不宣称事件发生日期。
- 在归档文章的既有 event 标签卡中增加按需展开的「相关事件」，沿用文章预览，不创建独立页面、主面板或阶段图。
- 用 AI 版块历史 event 标签做只读相关性试验，验证新表示与阈值；不等一两周、不回放 thread 身份、不预填效果。

## Capabilities

### New Capabilities

- `related-events`：事件语义表示、直接相关查询、独立阈值配置、既有标签卡展示、历史效果核对与身份隔离。

### Modified Capabilities

无。撤回旧 `event-story` / `story-heat-evidence` 未实施 delta，不以 REMOVED 删除现行主规范。D13、旧 CLI 与实体层关停已落地事实单独留档；本轮不借重做制品删除它们。

## Impact

- 后端：在 tagmanagement 域增加独立相关事件服务、缓存向量仓储、查询 handler、索引刷新 CLI；沿用 airouter 的 embedding 路由及调用审计。
- 数据：计划独立 `related_event_embeddings` 派生缓存表，**不建实体身份表、不建合并关系表**。不复用 `topic_tag_relations`，避免污染现有父子/后代查询。
- 前端：复用 `MatchingConfigDialog.vue`、`ArchiveMatchPopover.vue` 和 `ArticlePreviewModal.vue`，UI minor 合同见 `ui-design.md`。
- 不修改旧事件归属、指纹提取、辅助标签归并、话题生命周期、日报正文、热度与升温口径；旧 subject_key 仍可供旧功能使用。
- 文档待实现时更新 flow / api / database / configuration；本轮不把新方案写成已经生效的生产红线。

## 部署后影响与需要的操作

- **本轮仅改制品，产品行为不变**，不实现、不部署、不写数据库。
- 将来实现后，event 标签卡增加相关事件列表，匹配规则可调阈值；不承诺同一事件、阶段推进或发酵。
- 初次启用由执行方显式刷新相关事件向量索引，用户无需清理标签、重生成日报或执行身份回填；历史效果试验先在 AI 版块只读运行。
- 旧向量、实体数据、旧 story_id 保留。新缓存未就绪时明确提示，而非把缺数据显示为「没有相关事件」；关闭新入口不会影响既有阅读与旧事件线。

## 当前状态

已实现并部署（2026-11-11，tasks 1.1–3.1 / 5.1–5.2 / 6.1 / 7.1 完成；4.1 代码审查由独立审查线程负责）：`related-events` 能力（确定性文本 + 隔离派生缓存 + 显式刷新 CLI + 同版块只读端点 + 匹配规则字段 + 归档 event 卡按需展开）、S3 历史实测（AI 版块 1980：782 标签 / 305,371 对 / ≥0.82 共 5,919 对；分层样本 50 对明确相关 34，0.86+ 区间 12/12 → 0.82 偏宽松）、reference 文档与三视口浏览器验收齐备。

**0.86 是实测后的默认值，仍不是标定保证**：S3 在起始值 0.82 下实测偏宽松（明确相关 68%），用户据此拍板把默认值上调为 0.86（同一分层样本 ≥0.86 带 12/12）；本实现不预填准确率，历史 0.82 数据原样留档。刷新索引的实际操作与"部署后影响/用户需执行的操作"见 tasks.md 7.1 与 `ui-verification/report.md`。
