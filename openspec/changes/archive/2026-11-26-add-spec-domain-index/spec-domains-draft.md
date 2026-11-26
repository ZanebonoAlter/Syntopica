# spec 域归属草表（v2，带 Purpose）

共 177 个。**domain 列第一个是主域**，正事看第一个。

判定口径：先看 PURPOSE 定主域，Purpose 缺省（TBD / `(无)`）才靠 Requirement 首句，并自问「坏掉后用户在界面上失去哪个功能」。置信：`h` = Purpose 明确指向单一主域且 Requirement 不矛盾；`m` = 可属 2-3 域但主域清楚，或 Purpose 缺省而 Requirement 主域清楚；`l` = Purpose 缺省或横切且主域存疑；`?` = 判不出。**Purpose 缺省的一律不给 h。**

| capability | 行数 | domain | 置信 | 依据（≤22 字） |
| --- | --- | --- | --- | --- |
| agent-guide-slim | 75 | dev | h | AGENTS 引导文档精简形态 |
| agent-quota-gate | 99 | harness | m | 派发前拦截 Agent 工具调用 |
| aggregate-tagging | 196 | semantic-board, ai-summary | m | 聚合文切片打标，摘要出形态标记 |
| ai-capability-routing | 148 | ai-summary | m | AI 能力路由配额；词表无平台域 |
| ai-generated-article-detection | 149 | content-enrichment | m | 正文疑似 AI 判定并标记文章 |
| ai-health-reprobe | 98 | ai-summary | m | Purpose 缺省；模型健康重探 |
| ai-logging | 115 | ai-summary | m | Purpose 缺省；AI 调用日志表 |
| ai-model-health | 183 | ai-summary | m | Purpose 缺省；本地模型拉起 |
| ai-route-observability | 58 | ai-summary | m | AI 路由降级观测；无平台域 |
| ai-thinking-levels | 89 | ai-summary | m | 思考档七档，按 AI 场景覆盖 |
| ai-vendor-model | 119 | ai-summary | m | AI 供应商模型注册表，无平台域 |
| analysis-method-library | 99 | data-enrichment | h | Purpose=全局分析方法卡库 |
| analysis-pause-control | 207 | scheduler | m | Purpose 缺省；分析类调度总闸 |
| api-docs | 37 | dev | m | API 文档准确性维护规范（Purpose 缺省，按内容判；R-6 降 m） |
| architecture-docs | 132 | dev | m | 架构文档准确性维护规范（Purpose 缺省，按内容判；R-6 降 m） |
| archive-directory | 26 | dev | m | Purpose 缺省；docs 归档目录 |
| archive-readiness | 44 | harness, dev | m | 归档就绪自查脚本，对齐门禁 |
| article-bulk-update | 25 | api, reading | m | 批量操作 scope 接口契约 |
| article-content-crawling | 105 | content-enrichment | m | Purpose 缺省；中立抓取契约 |
| article-description-sanitization | 145 | content-enrichment | h | RSS 摘要纯文本化入库 |
| article-list-projection | 87 | api, reading | m | 列表接口传输契约，不含正文 |
| article-preview-dialog | 25 | ui, reading | m | 预览弹窗内容区确定高度 |
| article-reference-integrity | 124 | db, daily-report | m | 删文章同步维护 jsonb 引用 |
| article-retention | 113 | content-enrichment, db | m | Purpose 缺省；超限文章归档 |
| article-search | 155 | reading | m | 站内混合搜索与独立搜索页 |
| article-tag-watch-toggle | 55 | reading, semantic-board | m | 文章页切换标签关注状态 |
| aux-label-gc | 94 | semantic-board, scheduler | m | 辅助标签 GC，含定时清理 |
| auxiliary-label-description-repair | 82 | semantic-board | m | Purpose 缺省；辅助标签描述修复 |
| auxiliary-label | 228 | semantic-board | h | Purpose=辅助标签提取入库治理 |
| backend-package-boundaries | 59 | dev | h | 后端包依赖边界，lint 强制 |
| bipartite-relation-matching | 139 | topic-graph | h | Section 跨天关系二分图匹配 |
| board-affinity-metadata | 34 | semantic-board | h | 簇与版块亲和度元数据 |
| board-article-api | 146 | semantic-board, api | m | 版块文章列表独立 API |
| board-concept-management | 37 | semantic-board | h | 旧版块概念管理，已废弃 |
| board-direction-check | 207 | semantic-board | h | 标签匹配版块的方向性校验 |
| board-editing-ui | 68 | semantic-board, ui | m | 版块编辑弹窗与标签展示 |
| board-health-checkup | 402 | semantic-board | h | 版块体检统计与评分 |
| board-lane-dynamics | 245 | semantic-board, daily-report | m | 泳道动态视图，日报后结算 |
| board-level-analysis | 379 | semantic-board, data-enrichment | m | 版块为对象的简报与调查 |
| board-management-api | 208 | semantic-board, api | m | 版块与辅助标签 CRUD API |
| board-match-auto-trigger | 24 | semantic-board | h | embedding 后自动触发匹配 |
| board-match-cache | 146 | semantic-board | h | 版块匹配的内存缓存层 |
| board-signal-reports | 384 | semantic-board, data-enrichment | h | 按作用对象定主域（用户 2026-11-26 拍板） |
| board-upgrade-expand | 87 | semantic-board | h | 单版块扩充候选与挂载建议 |
| board-upgrade | 276 | semantic-board | h | Purpose=辅助标签聚类升级版块 |
| branding | 37 | ui | h | 品牌一致性契约 |
| case-first-testing | 29 | dev | m | Purpose 缺省；用例先行准入 |
| change-diagram-convention | 55 | dev | h | change 制品图表达规范 |
| change-scope-gate | 118 | harness, dev | m | Purpose 缺省；范围→验证脚本 |
| client-poll-budget | 83 | api, ui | m | 常驻轮询合并入口与退避 |
| compose-firecrawl | 47 | dev, content-enrichment | m | Purpose 缺省；自部署抓取栈 |
| composite-label | 95 | semantic-board | h | 组合标签实体全生命周期 |
| concurrent-change-coordination | 95 | harness | m | 事实库文件归属地图与并发（Purpose 缺省，按内容判；R-6 降 m） |
| configuration-docs | 21 | dev | m | 配置文档准确性维护规范（Purpose 缺省，按内容判；R-6 降 m） |
| constraint-injection | 547 | harness | m | 约束注入通道，system+steer |
| cross-board-relation-discovery | 109 | semantic-board, topic-graph | m | 跨版块关系候选发现 |
| daily-report-margin-notes | 240 | daily-report | h | 日报阅读层划选批注问答 |
| daily-report-match-fallback | 36 | daily-report, semantic-board | m | 日报收集时补算板块归属 |
| daily-report-system | 1414 | daily-report, semantic-board | h | Purpose=日报生成编排主域 |
| data-enrichment | 681 | data-enrichment, semantic-board, topic-graph | m | 持久话题认知分析，板块配置 |
| database-docs | 59 | dev | m | 数据库文档准确性维护规范（Purpose 缺省，按内容判；R-6 降 m） |
| db-migration-execution | 71 | db | m | Purpose 缺省；迁移脱离外层事务 |
| db-migration-safety | 64 | db | m | Purpose 缺省；破坏性迁移开关 |
| demo-seed-sync | 66 | dev, scheduler | m | demo 每周同步与脱敏断言 |
| deployment-init | 144 | dev | m | Purpose 缺省；部署唯一入口 |
| design-decision-landing | 73 | dev | h | design 决策须带可解析落点 |
| desktop-shell | 102 | ui | h | 桌面壳窗口装载与恢复 |
| detective-wall-camera | 124 | topic-graph, ui | m | 话题墙 3D 运镜相机系统 |
| detective-wall-interaction | 190 | topic-graph, ui | m | 话题墙交互与 BFS 生命线 |
| detective-wall-scene | 359 | topic-graph, ui | m | 话题墙 Three.js 场景渲染 |
| dev-api-networking | 71 | dev | h | 开发期端口与直连契约 |
| dev-image-reachability | 59 | dev | h | 测试镜像加速与容器回收 |
| dev-process-guard | 202 | harness | m | Purpose 缺省；扩展清理泄漏进程 |
| development-docs | 81 | dev | m | 开发文档准确性维护规范（Purpose 缺省，按内容判；R-6 降 m） |
| disaster-recovery | 94 | db | h | 备份档恢复契约与演练 |
| doc-impact-gate | 289 | harness, dev | m | Purpose 缺省；文档影响归档对账 |
| docs-milestone-structure | 71 | dev | m | Purpose 缺省；里程碑目录命名 |
| docs-reference-layer | 130 | dev | m | reference 目录为单一真相源 |
| embedding-cache | 80 | ai-summary, db | m | Purpose 缺省；向量结果落库缓存 |
| embedding-dimension-contract | 86 | db, ai-summary | m | 向量维度是运行时属性非 schema |
| entity-subject-resolution | 176 | topic-graph | h | 全局实体身份层与主体归一 |
| event-story | 29 | topic-graph | h | 事件线与事件的身份关系 |
| feed-candidate-catalog | 115 | discovery | h | RSS 候选源目录，非订阅表 |
| feed-delete-referential-integrity | 77 | discovery, db | m | 删订阅源须清理 NO ACTION 引用 |
| feed-discovery | 206 | discovery | m | Purpose 缺省；RSSHub 路由参数 |
| feed-icon-management | 187 | discovery | h | Feed 图标来源状态机 |
| feed-settings-ui | 267 | discovery, ui | m | Purpose 缺省；feed 卡片管线开关 |
| feed-summary-trigger | 85 | ai-summary, discovery | m | feed 级总结触发双层开关 |
| feed-tagging-control | 55 | discovery | m | Purpose 缺省；feed 级打标开关 |
| frontend-interaction-contract | 168 | ui | h | 交互模式词表与文字职责 |
| frontend-test-env | 34 | dev | h | vitest 运行环境确定性 |
| gate-concurrency-control | 84 | harness | h | 门禁命令全局互斥与限核 |
| gate-failure-reporting | 122 | harness | h | 门禁失败归属与注入收敛 |
| gate-interop-health | 75 | harness | h | interop 探测与门禁短路 |
| gate-status-query | 34 | harness | h | 门禁状态只读查询入口 |
| gate-toolchain-health | 61 | harness | h | 工具链探测与门禁短路 |
| graceful-shutdown-hardening | 62 | dev | m | Purpose 缺省；后端关停顺序 |
| harness-fact-log | 459 | harness | h | harness 层事件事实账本 |
| harness-retro-loop | 402 | harness | h | 账本失败聚类复盘报告 |
| http-response-compression | 49 | api | h | HTTP 文本响应协商压缩 |
| image-proxy | 73 | api, reading | m | 外链图片统一后端代理 |
| lane-correction-surface | 716 | semantic-board | m | Purpose 缺省；泳道修正工作面 |
| lane-heat-metric | 101 | semantic-board | h | 泳道当天热度量纲 |
| lane-manual-correction | 315 | semantic-board, topic-graph | m | 人工重设持久话题首义向量 |
| lane-trend-overview | 105 | daily-report, semantic-board | m | 日报视图泳道趋势区块 |
| lint-zero-debt | 21 | dev, harness | m | lint 零债务门禁契约 |
| log-cleanup | 76 | db, scheduler | m | scheduler 定时清理日志表 |
| markdown-mermaid-render | 57 | ui | m | mermaid 围栏块客户端渲染 |
| match-detail-ondemand | 46 | semantic-board, api | m | 匹配详情按需计算 API |
| match-score-visualization | 98 | semantic-board, ui | m | tag chip 匹配分颜色编码 |
| mobile-viewport | 63 | ui | h | 窄视口布局降级契约 |
| notification-center | 99 | daily-report, ui | m | 日报终态落库通知与铃铛 |
| otel-business-tracing | 251 | ai-summary | m | LLM 调用追踪横切；见人判；词表无可观测性域（已定，R-6） |
| outbound-proxy-failover | 97 | content-enrichment, discovery | m | 出站代理熔断，作用于抓取链路 |
| persistent-topic | 714 | topic-graph, semantic-board, daily-report | m | 版块与 section 间持久话题层 |
| pg-scheduled-backup | 85 | db, scheduler | m | PG 关键表每日定时备份 |
| public-read-only-demo | 84 | dev | h | 一键只读 demo 的 compose 栈 |
| radar-pipeline-status | 192 | semantic-board, daily-report | m | 雷达页管线状态块与手动日报 |
| reading-article-pane | 204 | reading | h | 阅读页文章主体阅读列版式 |
| reading-list-panel | 111 | reading | h | 阅读页扫描-选择列表面板 |
| refresh-parallelization | 67 | discovery | m | feed refresh 并发限流并行 |
| related-events | 119 | topic-graph, semantic-board | m | 事件标签的相关事件推荐 |
| remote-push-deploy | 55 | dev | h | 远程一键推送部署 |
| research-data-sources | 274 | research-data-sources | h | 外部研究数据源目录与取数 |
| research-retention | 35 | dev | m | 调研文档落点规范（非业务） |
| review-trace-gate | 135 | harness, dev | m | review 痕迹归档门禁脚本 |
| rss-article-dedup | 105 | discovery, db | m | RSS 入库判重与唯一性兜底 |
| same-origin-deployment | 65 | dev | h | 同源部署形态契约 |
| scheduler-accuracy | 79 | scheduler, daily-report | m | next_run 真实化与日报墙钟 |
| scheduler-observability | 99 | scheduler | m | 调度器执行结果持久化可见 |
| section-content-embedding | 56 | topic-graph, daily-report | m | 日报 section 向量内容化生成 |
| section-lifecycle | 469 | topic-graph | h | Section 状态推导与 DAG 时间线 |
| section-relations | 235 | daily-report, topic-graph | h | 主域 daily-report：DailyReportSection 是日报的数据结构，改坏伤日报阅读；代码在 topicgraph 包故含次域（R-2 人工裁定） |
| semantic-label-model | 113 | semantic-board, db | m | 辅助标签与版块统一数据模型 |
| sequence-drift-self-heal | 74 | db | h | 主键自增序列漂移自愈 |
| settings-workspace | 235 | ui | h | 独立设置工作区导航布局 |
| source-board-hit-rate | 94 | semantic-board, discovery | m | 订阅源命中板块率反查聚合 |
| spa-loading-ux | 119 | ui | h | SPA 首屏加载体验契约 |
| sql-safety-guard | 98 | harness, db | m | 扩展执行前硬拦截破坏性 SQL |
| standard-spec-format | 30 | dev | m | Purpose 缺省；规范文档结构 |
| structured-output-adaptation | 72 | ai-summary | m | vendor 结构化输出档位降级 |
| subagent-harness-coverage | 63 | harness | m | 子线程扩展加载配置矩阵 |
| tag-embedding-management | 226 | semantic-board, db | m | 标签 embedding 记录生命周期 |
| tag-hierarchy-quality | 67 | semantic-board, scheduler | m | 标签 slug/抽象层级质量清理 |
| tag-match-identity-protection | 34 | semantic-board | m | 高相似匹配不自动合并标签 |
| tag-merge-suggestions | 107 | semantic-board, topic-graph | m | 相似标签合并建议与扫描 |
| tag-merge-ui | 45 | semantic-board, ui | m | TagsPage 标签合并入口 |
| tag-queue-progress-chip | 53 | semantic-board, ui | m | header 常驻标签队列进度芯片 |
| tag-queue-scheduling | 50 | semantic-board, scheduler | m | 打标队列新任务优先消费 |
| tag-to-board-matching | 247 | semantic-board | h | 标签经辅助标签匹配版块规则 |
| tagging-domain | 459 | semantic-board, topic-graph | m | 打标域包结构，含版块与实体 |
| test-case-design | 123 | dev | h | 测试用例设计规范权威源 |
| test-db-fixture | 44 | dev, db | m | 测试库建库与迁移链自洽 |
| test-debt-patrol | 91 | dev, harness | m | 测试欠账台账与巡检 |
| test-infrastructure | 275 | dev | h | Go 测试工具包与分层约定 |
| theme-system | 314 | ui | h | 三层 token 与双主题切换 |
| thread-lineage | 26 | topic-graph | h | Thread 血统已退役 |
| thread-related-events | 142 | topic-graph, daily-report | m | 线索相关事件读模型 |
| thread-storage | 79 | daily-report, db | m | 日报 thread 独立存储表 |
| tool-output-spill | 90 | harness | h | 超大工具输出强制 spill |
| topic-graph-display-controls | 59 | topic-graph, ui | m | 主题图显示设置与节点拖拽 |
| topic-graph-highlight-bfs | 81 | topic-graph, ui | m | 图高亮共享有界 BFS |
| topic-key-event-markers | 165 | topic-graph, semantic-board | m | 泳道内人工选定关键事件 |
| topic-watch | 139 | semantic-board, daily-report, topic-graph | m | 三域横跨，主域按作用对象=版块（已定，R-6） |
| ui-design-workflow | 212 | ui | h | UI 影响分档与原型审批 |
| ui-icon-localization | 38 | ui | m | 本地图标子集产物 |
| ui-verification-trace-gate | 88 | harness, ui | m | UI 验收痕迹归档门禁脚本 |
| unified-dialog | 310 | ui | h | 统一对话框外壳 AppDialog |
| unified-form-controls | 320 | ui | h | 统一表单原子组件族 |
| upgrade-candidate-time-window | 67 | semantic-board | h | 升级候选按文章活动时间过滤 |
| user-onboarding | 165 | ui | h | 首次引导 tour 与空态引导 |
| watch-materialized-topic | 199 | daily-report, topic-graph, semantic-board | m | 日报生成时物化关注为 section |
| web-search-provider | 83 | data-enrichment, daily-report | m | 统一网页搜索入口供问答调研 |
| workshop-article-archive | 243 | semantic-board | h | 版块文章归档视图与证据 |
| workshop-board-console | 859 | semantic-board | h | 版块态势页泳道×日期矩阵 |
| workshop-lane-compose | 565 | semantic-board, topic-graph | m | 新建泳道编排画布 |
| workshop-radar | 216 | semantic-board | h | 工坊首屏跨版块雷达 |

## 需要人判的（l 或 ?）

| capability | 暂判 | 纠结原因 |
| --- | --- | --- |
| ~~board-signal-reports~~ **已定** | semantic-board, data-enrichment | 口径=**按作用对象**（用户 2026-11-26 拍板）：它们伤的是版块页，不是调研能力本身。同类 `board-level-analysis` 已同步改主域。 |
| ~~otel-business-tracing~~ **已定** | ai-summary | 裁决（2026-11-26）：**维持 ai-summary**。它是横切可观测性（LLM span / AICallLog / 打标链路 span），词表没有可观测性域；挂在 AI 调用层是最近似选择。坏掉只影响排障不影响界面功能，诚实记入附注即可。 |
| ~~topic-watch~~ **已定** | semantic-board, daily-report, topic-graph | 裁决（2026-11-26）：**主域 semantic-board**，按用户已拍的口径「按作用对象判」——用户是在版块下声明追踪。三域全挂，查询任一域都能命中。 |

**附注（判 m、不用逐个复核，但同根因，一句话定）**

- **AI 平台类 9 个**（ai-capability-routing / ai-model-health / ai-health-reprobe / ai-logging / ai-route-observability / ai-thinking-levels / ai-vendor-model / structured-output-adaptation / embedding-cache）：都是 AI 调用层自身（路由、供应商、思考档、调用日志、Embed 缓存），不是摘要业务。**已拍板（2026-11-26）：维持挂 `ai-summary`**——词表不扩，语义按 `constraints-index` 域表的「AI/摘要」理解。
- **标签池 10 个**（aggregate-tagging / tag-embedding-management / tag-hierarchy-quality / tag-match-identity-protection / tag-merge-suggestions / tag-merge-ui / tag-queue-progress-chip / tag-queue-scheduling / tag-to-board-matching / tagging-domain）：词表两边定义都没写「标签本体」（event / person tag）。按项目结构挂 semantic-board——打标域包内含版块匹配、辅助标签、体检服务，`/tags` 标签页就是工坊首屏；事件身份为主的相关事件取 topic-graph 主域。aggregate-tagging 保留 ai-summary 次域是因为它有一条 Requirement 直接约束摘要生成调用（产出 content_form），不是因为它用 LLM。

## 自查统计

h=66 m=111 l=0 ?=0；单域=105 多域=72；

- 合计 177，与 `openspec/specs/` 目录数一致；行数取自 `/tmp/spec-deep.txt`。
- 域名全部落在 15 词表内，单行最多 3 域（3 域的 4 个：data-enrichment / persistent-topic / topic-watch / watch-materialized-topic）。
- 读全文 9 个（限额 10）：archive-directory、research-retention、analysis-pause-control、data-enrichment、otel-business-tracing、tagging-domain、concurrent-change-coordination、tag-hierarchy-quality、board-signal-reports。