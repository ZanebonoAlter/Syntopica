# analysis-pause-control Delta

## MODIFIED Requirements

### Requirement: 暂停生效范围——分析类
当 analysis_paused 为 true 时，分析类调度任务的 JobFunc SHALL 在每次 tick 自检并直接返回、不 lease 新任务，覆盖：content_completion、daily_report、board_upgrade_suggest、lifeline_weekly、lifeline_monthly、lifeline_yearly、tag_quality_score。tag worker 池（TagQueue、EmbeddingQueueWorker、MergeReembeddingQueueWorker）SHALL 不消费各自队列。

firecrawl（正文抓取）SHALL NOT 属于分析类管制（见「入库与维护类不受暂停影响」）；其抓取完成的下游影响 SHALL 仅限于落库 firecrawl 状态位与入队后续任务，下游 LLM 消费方（tag worker、content_completion）在暂停期间 SHALL NOT 被触发发起任何 LLM 调用。

#### Scenario: 暂停时调度任务不 lease
- **WHEN** analysis_paused 为 true 且 content_completion 调度器触发 tick
- **THEN** 该 tick 不从队列 lease 任何任务，直接返回 skipped 结果

#### Scenario: 暂停时 tag worker 不消费
- **WHEN** analysis_paused 为 true
- **THEN** TagQueue 不 lease tag_jobs，EmbeddingQueueWorker 不 lease embedding_queues

#### Scenario: 暂停时 firecrawl 的下游不触发 LLM
- **GIVEN** analysis_paused 为 true，某文章 firecrawl 抓取完成且所属 feed tagging_enabled=true
- **WHEN** 抓取完成事件处理
- **THEN** 该文章 tag_jobs 照常入队（pending 等待），SHALL NOT 因此发起任何 LLM 调用；恢复后由 worker 按消费顺序消化

### Requirement: 入库与维护类不受暂停影响
当 analysis_paused 为 true 时，auto_refresh（RSS 入库）、firecrawl（正文抓取，纯抓取算力、零 LLM 调用）及维护类调度（log_cleanup、aux_label_cleanup、blocked_article_recovery、rsshub_catalog_sync、preference_profile_update）SHALL 继续正常运行。

#### Scenario: 暂停时 RSS 继续入库
- **WHEN** analysis_paused 为 true 且 auto_refresh 触发
- **THEN** RSS feed 照常刷新，新文章照常写入 articles 表

#### Scenario: 暂停时日志清理继续
- **WHEN** analysis_paused 为 true 且 log_cleanup 触发
- **THEN** log_cleanup 照常清理过期日志

#### Scenario: 暂停时 firecrawl 照常抓取
- **GIVEN** analysis_paused 为 true（含模型 NOT 健康的常态白天），存在 firecrawl_status=pending 的文章
- **WHEN** firecrawl 调度器触发 tick
- **THEN** 照常抓取正文并写回 firecrawl 状态字段；SHALL NOT 因暂停跳过

### Requirement: 健康门硬执行

当 AI 模型未就绪（NOT 健康，见 ai-model-health）时，所有分析类调度任务的 JobFunc SHALL 在每次 tick 自检 `analysispause.IsPaused()`（= 用户暂停 || NOT 健康）并直接返回、不 lease 新任务；tag worker 池（TagQueue、EmbeddingQueueWorker、MergeReembeddingQueueWorker）SHALL 不消费各自队列。firecrawl 因零 LLM 调用 SHALL NOT 受健康门管制。`IsPaused()` 在健康快照未就绪（启动竞态）时 SHALL 视 NOT 健康返回 true（保守，分析不跑）。手动切换暂停/恢复（POST /api/analysis/pause）SHALL NOT 因健康状态被拒绝——开关仅表达用户意图，实际是否运行由健康门在 worker 侧裁定。

#### Scenario: 模型未就绪时调度任务不 lease

- **GIVEN** analysis_paused=false（用户未暂停），模型 NOT 健康
- **WHEN** content_completion / daily_report 等分析类调度器触发 tick
- **THEN** 各 tick SHALL 不 lease 任务，直接返回（与用户主动暂停表现一致）

#### Scenario: 模型未就绪时 firecrawl 照常抓取

- **GIVEN** 模型 NOT 健康（如本地 LLM 主机未开机）
- **WHEN** firecrawl 调度器触发 tick
- **THEN** firecrawl SHALL 照常抓取（不受健康门管制），其下游 LLM 任务仍不消费

#### Scenario: 模型未就绪时 tag worker 不消费

- **WHEN** 模型 NOT 健康
- **THEN** TagQueue SHALL 不 lease tag_jobs，EmbeddingQueueWorker SHALL 不 lease embedding_queues

#### Scenario: 启动竞态期视为不健康

- **WHEN** 后端刚启动、健康快照未就绪（首次检测未完成）
- **THEN** IsPaused() SHALL 返回 true，分析类任务 SHALL 不 lease

#### Scenario: 手动启动不被健康状态拒绝

- **GIVEN** 模型 NOT 健康，analysis_paused=true
- **WHEN** 用户点击启动（POST /api/analysis/pause { paused:false }）
- **THEN** 系统 SHALL 接受请求、写入 analysis_paused=false（用户意图），SHALL NOT 返回错误；分析类任务是否实际运行仍由健康门在 worker 侧裁定

#### Scenario: 恢复时重新探活

- **WHEN** 用户点击恢复（POST /api/analysis/pause { paused:false }）
- **THEN** 系统 SHALL 在更新用户开关后异步触发一次 RunStartupProbe，使健康门能自愈（启动探活曾失败、或模型后来才就绪时，点恢复即重新评估）；pause=true 时 SHALL NOT 触发。响应仍只反映用户意图

### Requirement: 恢复后自动续跑
当 analysis_paused 从 true 切回 false 时，暂停期间堆积的 pending 队列任务 SHALL 在后续 tick/lease 周期按 tag-queue-scheduling 定义的消费顺序（新任务优先，priority 可插队）自动消化，无需手动干预。

#### Scenario: 恢复后消化堆积任务
- **WHEN** 暂停期间 tag_jobs 堆积了 50 条 pending，用户触发恢复
- **THEN** TagQueue 在后续周期按新任务优先顺序逐步处理这 50 条，无需手动操作
