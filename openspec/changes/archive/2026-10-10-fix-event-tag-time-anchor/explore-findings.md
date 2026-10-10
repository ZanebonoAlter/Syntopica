
## event 标签时间锚修复：代码结构与 CLI 用法

【prompt 治根】tagger.go generateTagDescription 重构为三段：buildEventTagDescriptionPrompt（event 专用 prompt，时间锚纪律三句：锚定 Context [日期: …]、禁 Context 外时间、同名历史事件以 Context 为准；示例全虚构中性"某国央行上调基准利率"/"科技公司数据中心投产"）+ requestTagDescription（重试 3 次 LLM 调用，返回 desc+keywords）+ saveTagDescription（500 rune 截断 + event_keywords metadata + re-embedding 入队，幂等 UPDATE）。extractor_section.go / extractor_enhanced.go 输出正例同步换中性事件"某国央行上调基准利率"（merged prompt 有 1500 rune 预算测试，描述词已精简"货币当局"/"货币政策利率"/"提高利率的举措"）。

【存量修复 CLI】cmd/admin fix-event-tag-anchor（子命令式，config.LoadConfig("./configs") + database.InitDB + repository.InitRepository 初始化链）：dry-run 默认扫描+清单+备份 CSV（写 CSV 必须在 stdout 被 head 管道截断前完成——go run 管道 SIGPIPE 会杀进程导致 CSV 没写，跑时输出重定向到文件）；--apply --ids 显式圈选，非候选 ID 整批拒绝（SelectEventAnchorCandidatesByIDs 返回 error）。

【核心逻辑】internal/tagmanagement/service/core/event_tag_anchor.go：anchorYearPattern=\b20\d{2}\b（只匹配 2000-2099 半角，1999/全角/v2024/12024 均不匹配——test-cases 白盒定案）；FilterEventAnchorCandidates（createdYear-anchorYear≥1 命中）；RegenerateEventTagDescription 复用 requestTagDescription+saveTagDescription，articleContext 取首挂文章（article_topic_tags 按 created_at ASC, id ASC 第一行）经 buildArticleContext（从 persistArticleTags 抽出的共用构造：[日期: 发布日期] 标题. 摘要≤800字）。

【测试】event_tag_anchor_test.go：纯函数（年份提取 11 用例/筛选边界/白名单/复扫幂等）+ testutil PG + httptest fake LLM server（复用 metadata_test.go setupTopicExtractionTestDB + vendor/model/route 注册模式）。坑：GORM First(&struct) 复用带主键的 struct 会叠加旧 ID 条件导致 record not found——reload 必须用独立变量。

【真库执行结果 2026-10-09】候选 321（全量史，多为 6-14 导入批合法旧事件）；圈选 4 个病态（183730/197750/194440/200389）apply 4/4 成功，复扫 321→317；新描述全部锚定"文章报道当期"；备份 CSV /tmp/event-tag-anchor-backup-apply.csv。

<!-- pinned 2026-10-09T15:32:22Z -->
