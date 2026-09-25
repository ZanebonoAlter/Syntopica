<!-- complexity: complex -->
<!-- ui-impact: major -->
<!-- constraint-domains: daily-report, ai-summary, data-enrichment -->

## Why

读日报遇到不懂的术语或段落（如「央行逆回购」）时，用户只能跳出系统去搜索：问答不留在报告里、同一个词反复查、系统也始终不知道用户懂什么/不懂什么。Syntopica 作为单用户知识系统，只有"信息输入侧"（抓取、聚类、日报），缺一条"学习侧"的回流——问过的东西应该沉淀下来，成为系统的知识资产。

## What Changes

第一期（本 change 范围）落地「报纸页边注 + 问答沉淀 + 术语派生」三件套，钉死在日报阅读层：

- **划段批注**：日报正文选中文本 → 浮出「问一问」气泡 → 点击落锚（正文高亮 + 右侧页边注栏出现锚点链接）。批注锚点独立存在，可先标记后提问，可挂多轮追问。
- **页边注问答**：在批注锚点处输入问题 → AI 回答 = 通用概念解释 + 该 thread 关联的当天文章上下文（RAG），答案附引用文章（可点开查证，无引用时明示"纯模型知识"）。每次调用走 airouter 并落审计。
- **沉淀三表**：`report_annotations`（锚点）/ `annotation_qas`（问答轮，含 `cited_article_ids`）/ `term_notes`（术语库）。每次 QA 由 LLM 顺手抽取涉及的术语自动入术语库（零边际成本），P1 只攒数据+批注卡片内最小展示，不做独立术语库页面。
- **全局批注管理页**：设置工作台新增「页边注」分区——跨报告列出全部批注（版块/日期/期号、划词、术语 chips、问答折叠），支持版块与关键词筛选、跳转原日报定位、删除（与日报内删除同源连带问答）。
- **联网搜索补强（2026-09-24 扩充，用户决策）**：部分问题（时效性事实、模型知识盲区）单靠当天文章 + 模型自身知识无法可靠回答。提问时系统先调本地 SearXNG（`http://localhost:8889`，零 API 成本）拿原始网页结果作为参考上下文并入同一次 LLM 调用，回答可引用网络来源（title+url 结构化沉淀、白名单防编造链接），卡片展示可点外链 chips；搜索失败/未配置静默降级为现状行为。SearXNG 客户端落 `internal/platform/searxng` 通用包（未来 agent 循环可复用），配置对齐 `bocha_config` 模式。
- **收编 open_notebook 能力槽**：airouter 预留的 `open_notebook` capability（2026-03-12 引入至今零调用方）成为问答的首个调用方，不新增 capability 枚举。
- **窄视口降级**：<1100px 布局塌单列时批注栏降级为浮层/内联形态（双视口验收）。

**Non-goals（后续期，数据模型已预留但功能不点亮）**：解释从已知术语讲起、热词/盲区雷达（hit_count 视图）、术语库独立浏览页（管理页仅展示术语 chips，非术语库页面）、问答信号反哺 discovery 偏好、日报生成难度感知。

## Capabilities

### New Capabilities

- `daily-report-margin-notes`: 日报阅读层的划段批注、页边注问答（通用+当天文章 RAG）、批注/问答/术语三表沉淀与术语派生。

### Modified Capabilities

- `ai-capability-routing`: 「能力与业务用途绑定」补第五条绑定——`open_notebook` SHALL 驱动日报页边注问答；该 capability 从预留槽转为实际调用方。

## Impact

- **前端**：`front/app/features/tags/`——`BoardDailyReportTimeline.vue` 的 `drm-layout` 由两列改三列（右侧页边注栏）、`daily-report/` 组件群新增批注/问答卡片组件、划词气泡交互、窄视口降级；`useDailyReportReader.ts` 扩展批注状态；`front/app/features/settings/`——`SettingsWorkspace` sections 注册「页边注」分区 + 新增 `SettingsSectionMarginNotes` 管理页。
- **后端**：`backend-go/internal/topicgraph/`（daily_report 域）新增 annotations/qa/term 模型 + repository + API（批注 CRUD、问答提交、引用与术语返回）；新增 QA service：组装 prompt（划中文本 + 问题 + thread 相关文章摘录 + SearXNG 联网结果参考块）→ airouter `open_notebook` → 解析回答/引用/术语抽取；新增 `internal/platform/searxng` 通用客户端（独立包，避免 topicgraph↔dataenrichment 潜在循环引用，未来 dataenrichment `WebSearcher` 可薄适配复用）；`aisettings` 新增 `searxng_config` 键（endpoint+enabled，DB(ui)>env>config.yaml>空=禁用）。
- **数据库**：三张新表 migration + 追加 migration `annotation_qas` 加 `cited_web_sources jsonb NOT NULL DEFAULT '[]'`（网络来源结构化沉淀，`[{title,url}]` 保序去重）；`term_notes.embedding` 为 vector 列（后续相似词归并/话题挂接预留）。
- **AI 成本**：每次提问 1 次 LLM 调用不变（联网搜索是本机 SearXNG 零 API 成本、只是 prompt 变长），无预计算、无后台批处理；结果按 annotation+question 缓存不重复调用；SearXNG 延迟 +1~3s（失败静默降级不阻塞）。
- **前端（联网增补）**：批注卡与管理页问答轮新增「网络来源」chips（外链新窗口）；「纯模型知识」标注口径改为本地引用与网络来源均为空；设置页新增「SearXNG 搜索」分区（对齐「博查搜索」section 模式）。
- **锚定可靠性依赖**：日报 period 生成后不可变（backfill 除外），批注锚定采用 quoted_text + thread/section id + 偏移线索，无需通用网页批注的 Range 花活。
