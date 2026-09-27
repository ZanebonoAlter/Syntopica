# analysis-pause-control Specification

## Purpose
TBD - created by archiving change pause-analysis. Update Purpose after archive.
## Requirements
### Requirement: 全局分析暂停开关

系统 SHALL 提供全局"分析暂停"**用户开关**（`analysis_paused`，true=用户主动暂停），其值 SHALL 持久化到数据库，服务重启后 SHALL 保持用户上次设置不变。开关的读取（`GET /api/analysis/pause`）与切换（`POST /api/analysis/pause`）API、前端按钮行为 SHALL NOT 因 AI 模型健康状态而改变。

分析类任务是否**实际运行** SHALL 由「有效暂停态」决定：

> 有效暂停 = 用户暂停（analysis_paused） **或** NOT 健康（由 ai-model-health capability 定义）。

当 `analysis_paused=false`（用户未暂停）但模型未就绪（NOT 健康）时，分析类任务 SHALL 同样不 lease、不消费队列；但用户开关状态 SHALL 保持 false 不被改动（前端以健康提示告知，而非翻转开关）。当健康就绪后（且用户未暂停），分析 SHALL 自动恢复运行，无需用户再次手动启动。

暂停生效范围（分析类）、入库与维护类不受影响、优雅停、恢复后自动续跑、总闸与分闸共存等既有语义 SHALL 保持不变，仅"是否实际运行"增加健康维度。

#### Scenario: 用户开关默认不暂停

- **WHEN** 系统首次部署，未设置过暂停标志
- **THEN** analysis_paused 为 false（用户意图=运行）

#### Scenario: 用户开关跨重启保持

- **WHEN** 用户触发暂停后服务重启
- **THEN** 重启后 analysis_paused 仍为 true（用户意图不变）

#### Scenario: 健康未就绪时开关不变但任务不跑

- **GIVEN** analysis_paused=false（用户未暂停），模型未就绪（NOT 健康）
- **WHEN** content_completion 调度器触发 tick
- **THEN** 该 tick SHALL 不 lease 任何任务（有效暂停），且 GET /api/analysis/pause SHALL 仍返回 paused=false

#### Scenario: 健康就绪后自动恢复运行

- **GIVEN** analysis_paused=false，此前因 NOT 健康而暂停运行
- **WHEN** 启动健康检测完成且判定健康
- **THEN** 分析类任务 SHALL 恢复 lease（无需用户点启动）

#### Scenario: 用户手动暂停优先级不变

- **WHEN** analysis_paused=true（用户主动暂停），即便模型健康
- **THEN** 有效暂停 SHALL 为 true，分析类任务 SHALL 不运行

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
### Requirement: 优雅停——不强杀在跑任务
暂停 SHALL 只阻断新的 tick/lease，当前正在执行（已 lease）的任务 SHALL 自然执行完成，系统 SHALL NOT 强制中断在跑任务。

#### Scenario: 暂停时在跑批处理跑完
- **WHEN** firecrawl 调度器已 lease 一批任务正在抓取，此时用户触发暂停
- **THEN** 当前这批抓取任务执行完成，后续 tick 不再 lease 新任务

### Requirement: 恢复后自动续跑
当 analysis_paused 从 true 切回 false 时，暂停期间堆积的 pending 队列任务 SHALL 在后续 tick/lease 周期按 tag-queue-scheduling 定义的消费顺序（新任务优先，priority 可插队）自动消化，无需手动干预。

#### Scenario: 恢复后消化堆积任务
- **WHEN** 暂停期间 tag_jobs 堆积了 50 条 pending，用户触发恢复
- **THEN** TagQueue 在后续周期按新任务优先顺序逐步处理这 50 条，无需手动操作
### Requirement: 总闸与分闸共存
全局分析暂停（总闸）与 feed.tagging_enabled（分闸）SHALL 共存。当全局暂停生效时，即使 feed.tagging_enabled 为 true，该 feed 的文章 SHALL NOT 进入 tag 处理；全局恢复后，分闸重新生效。

#### Scenario: 总闸关时分闸无效
- **WHEN** analysis_paused 为 true 且某 feed 的 tagging_enabled 为 true
- **THEN** 该 feed 的新文章入库但不进入 tag 队列处理

#### Scenario: 总闸恢复时分闸重新生效
- **WHEN** analysis_paused 从 true 切回 false 且某 feed 的 tagging_enabled 为 true
- **THEN** 该 feed 的文章按 feed-tagging-control 既有规则进入 tag 处理

### Requirement: 暂停控制 API
系统 SHALL 提供 GET /api/analysis/pause 返回当前暂停状态（含 paused 布尔与 paused_at 时间），SHALL 提供 POST /api/analysis/pause 接受 { paused: bool } 切换状态并返回新状态。

#### Scenario: 读取暂停状态
- **WHEN** 调用 GET /api/analysis/pause
- **THEN** 返回 { paused: <bool>, paused_at: <time|null> }

#### Scenario: 切换为暂停
- **WHEN** 调用 POST /api/analysis/pause 且 body 为 { paused: true }
- **THEN** 系统写入暂停标志，返回 { paused: true, paused_at: <now> }，后续分析类任务停止 lease

### Requirement: 前端顶部栏暂停开关
首页顶部栏 SHALL 提供二态开关按钮：运行态显示"暂停"动作图标并可点击触发暂停；暂停态以醒目高亮显示"恢复"动作图标并可点击触发恢复。切换时 SHALL 通过全局通知提示用户。暂停态 SHALL 经轮询维持。

#### Scenario: 运行态点击暂停
- **WHEN** 当前 analysis_paused 为 false，用户点击顶部栏暂停按钮
- **THEN** 按钮切换为暂停态高亮，弹出"分析已暂停"通知

#### Scenario: 暂停态点击恢复
- **WHEN** 当前 analysis_paused 为 true，用户点击顶部栏恢复按钮
- **THEN** 按钮切换回运行态，弹出"分析已恢复"通知

### Requirement: 浏览器 favicon 暂停态标识
当 analysis_paused 为 true 时，浏览器标签页 favicon SHALL 切换为带暂停标识（⏸ 角标）的图标；恢复时 SHALL 切回默认 favicon。

#### Scenario: 暂停时 favicon 变化
- **WHEN** analysis_paused 切为 true
- **THEN** 浏览器 tab favicon 显示带 ⏸ 角标的图标

#### Scenario: 恢复时 favicon 复原
- **WHEN** analysis_paused 切回 false
- **THEN** 浏览器 tab favicon 恢复为默认 favicon.png

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
### Requirement: 前端健康未就绪提示

前端 SHALL 在「用户意图为运行（analysis_paused=false）但 AI 模型未就绪（NOT 健康）」时，通过通知中心展示健康未就绪提示：通知铃铛进入警示态（警示图标/配色，与未读数角标正交叠加），且通知面板列表顶部展示置顶系统状态条，告知用户 LLM/Embedding 未连通、分析暂停运行，并提供「重新检测」与跳转至模型配置页的入口。该提示 SHALL NOT 修改或禁用既有的暂停/启动按钮，SHALL NOT 在页面顶部悬浮 banner 形式展示。设置页 SHALL 提供「AI 健康状态」面板，展示各路由主 provider 的可达性明细、是否被后端拉起、上次检测时间，以及 `auto_start_models` 总开关。

#### Scenario: 意图运行但不健康时展示 banner

- **GIVEN** analysis_paused=false，ai_healthy=false
- **WHEN** 用户打开任意页面
- **THEN** 通知铃铛 SHALL 进入警示态，通知面板列表顶部 SHALL 显示「AI 模型未就绪（LLM/Embedding 未连通），分析暂停运行」置顶条（提示条随通知中心展示，不再悬浮页面顶部），含「重新检测」按钮与跳转设置页入口，且暂停/启动按钮 SHALL 保持可用不被禁用

#### Scenario: 健康恢复后警示消失

- **GIVEN** 通知中心处于 AI 未就绪警示态
- **WHEN** ai_healthy 变为 true（探活自愈或手动重探通过）
- **THEN** 铃铛 SHALL 回归普通态，面板置顶条 SHALL 消失（状态驱动，无需用户交互清除）

#### Scenario: 用户主动暂停时不展示健康 banner

- **GIVEN** analysis_paused=true（用户主动暂停）
- **WHEN** 模型亦 NOT 健康
- **THEN** 铃铛 SHALL NOT 进入健康警示态，面板 SHALL NOT 显示该健康未就绪置顶条（用户已主动暂停，无需再提示健康）

#### Scenario: 设置页展示健康面板与总开关

- **WHEN** 用户打开设置页
- **THEN** SHALL 见「AI 健康状态」面板（各路由主 provider 通断 + 是否后端拉起 + 上次检测时间）与 auto_start_models 开关

#### Scenario: 顶部栏常驻健康指示

- **WHEN** 顶部栏渲染
- **THEN** SHALL 常驻显示当前 AI 健康状态（健康/不健康二态，如 mdi:heart-pulse 绿/红），点击跳 AI 健康设置 section；与通知中心的健康未就绪警示（铃铛警示 + 面板置顶条）并存，二者 SHALL NOT 互斥

#### Scenario: 面板空列表时置顶条仍可见

- **GIVEN** analysis_paused=false，ai_healthy=false，通知列表为空
- **WHEN** 用户打开通知面板
- **THEN** 置顶系统状态条 SHALL 显示在空列表占位之上（系统状态与通知列表内容正交）

