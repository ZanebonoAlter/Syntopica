# 独立评审：add-question-driven-news-research

**结论：有条件通过。** 架构与制品方向正确（协议隔离、safefetch 串联、冻结快照、answer 结构校验、旧路径零变化均有实现与测试支撑；`go build`/`go vet`/`TestQuestion*` 全绿、`openspec validate --strict` 退出 0），但存在 3 处 High 级规格违例——先读门漏拦七源调用、compose 提示词完全未告知 answer 输出契约、本地空正文可被当作已读证据——均属局部修复，不需整体返工。修复并补 policy 级测试后可放行。

> 评审范围说明：工作区混有前置 change（redesign-enrichment-research-workbench 等）的脏文件。schema3 受控块/兼容文本、disposition/visibility/view 筛选、related_topics、`getBoardAnalysisStatus` 路由修正等归属前置 change，本报告不对其判定；只在与本 change 逻辑交织处核对。

## High（必须修）

### H1 先读门未拦截七源 call_tool，违反「先核前提再补证」
- **位置**：backend-go/internal/dataenrichment/service/signal_research.go:634-641（`CheckCall`）
- **问题**：`CheckCall` 仅当 `p.q.authorizesTool(toolName)`（即 read_source/web_search/fetch_page）时调用 `q.gateTool`；question-v1 材料下模型先 `call_tool yahoo_chart/eia_wpsr_table1` 等七源调用会落入 `else if !isSignalSourceTool` 分支直接放行执行。`RunLoopAction`（:706）只拦 calculate。而 runtime 的 `gateTool`（signal_question.go:237-244）明确设计为"外搜/数值源/calculate 一律拦截"。spec Scenario「先核前提再补证」要求"有冻结正文且模型先请求外搜、**数值**或 calculate → 不执行"。现有测试 `TestQuestionReadFirst` 只测了 `rt.gateTool("yahoo_chart")` 纯函数，未测 policy 路径，恰好漏过此洞。
- **建议**：`CheckCall` 中当 `p.q != nil` 时对七源工具也统一过 `gateTool`（例如在白名单判定通过后、question 检查外再调一次门）；补一条 loop 级测试：question-v1 材料 + 未读来源时先发七源 call_tool，断言 `BlockedReason=question_read_first` 且工具执行次数为 0。
- **处置**：已修：CheckCall 改 switch 三分支，question-v1 下白名单七源统一过先读门（read_source 恒放行、legacy p.q==nil 零变化）；补 policy 级测试 TestQuestionReadFirstGatesSevenSourcesAtPolicy（七源 Blocked=question_read_first / 非法工具仍白名单拦 / 读后放行 / legacy 不受影响）。

### H2 compose 系统提示词未声明 answer 输出契约，question-v1 成文首轮必失败
- **位置**：backend-go/internal/dataenrichment/service/signal_compose.go:255-322（`signalComposeSystemPrompt`）；对照校验 :1048-1051
- **问题**：question-v1 下 `validateSignalReportDraft` 强制 `draft.Answer != nil`（缺失即回注"缺 answer 字段"），但系统提示词的输出 JSON 模板、结构纪律、引用纪律均未提及 `answer` 字段（`[[source:]]` 引用格式也只出现在用户消息"已读文字证据"行）。真实模型首轮输出必缺 answer，三次成文机会固定烧掉一次，只能靠回注文本自我纠正；模型若不遵回注则整次研究在 compose 阶段失败。design D6 明确"由明确 prompt"。mock 测试（`scQuestionDraft` 自带 answer）掩盖了该断链。
- **建议**：在系统提示词（或按 `view.contract` 条件追加的段落）中加入 answer 契约：三枚举语义、reason 非空≤400、partial/unresolved 必填 unanswered（≤5 项各≤300）、计入 3000 可见预算，并在输出 JSON 模板补 `"answer":{...}`；同时把 `[[source:ID]]` 写进引用纪律。
- **处置**：已修（与本轮 E1 真实样本独立发现同一缺口，互为印证）：signalComposeAnswerPrompt 按 contract 增补（answer 必填、unanswered 是字符串数组、三枚举语义），signalComposeSystemPromptOf 路由；TestQuestionComposePromptCarriesAnswerContract 钉住旧协议逐字不变；E1 真实研究修复后两连通过（报告 22/23）；后半句也已闭环——[[source:]] 引用格式补进 question-v1 增补段的引用纪律（成文是独立 LLM 调用，不能只靠研究侧 prompt），测试同步断言。

### H3 本地空正文来源可冻结、可开先读门、可作 explained 证据
- **位置**：backend-go/internal/dataenrichment/service/signal_sources.go:284-306（`FreezeSignalQuestionSources` 无内容可用性检查）；signal_question.go:250-280（`readSource` 无检查）
- **问题**：`pickSignalArticleContent`（firecrawl>content）可为空串；冻结时空文本照样入 `Sources`/brief 白名单。`readSource` 成功返回空正文即置 `readOK`（解锁先读门）并入 ledger `sources`——空文来源可支撑 `validateSignalAnswerEvidence` 的 explained 证据门。spec ADDED「来源截取与空正文」明确"**本地**/网页正文……为空……不能标已读正文"；网页侧有 `signalUsableWebText` 把关，本地侧没有。
- **建议**：冻结阶段对空正文（trim 后）记 gap、不入 brief 白名单；或至少 `readSource` 对空文本返回 typed 缺口（不开 readOK、不 addSourceRecord、记 gap），保证空文既不开门也不可引用。
- **处置**：已修：freeze 侧空正文（TrimSpace 后）不冻结、记 gap；readSource 侧空文本返回 typed 错误——不开 readOK、不入 ledger sources（证据门不可能被空文满足）。

## Medium（应修）

### M1 webSearch 账本在执行前先记一条假 "ok"，失败路径出现 ok+error 双记录
- **位置**：backend-go/internal/dataenrichment/service/signal_question.go:316-320（`q.searchesUsed++` 后立即 `recordTextCall(..., "ok", "")`）
- **问题**：未配置/搜索失败分支随后又记一条 `"error"`，text_calls 里同一调用出现"成功"与"失败"两行，违反"错误不冒充成功"（D6、tasks 3.3）与 spec「网页预算与配置失败」的"失败有类型和原因"。另外 `recordTextCall` 的 `kind` 传空串，与结构注释 `query|source_id|url` 不符。
- **建议**：把 "ok" 记录移到搜索成功返回之后；kind 补 `"query"`（fetch_page 同理核对）。补断言：失败搜索的 text_calls 恰好一条且 status=error。
- **处置**：已修：ok 记录移到搜索成功返回之后；未配置分支记 rejected（含原因），失败记 error——失败路径不再出现双行；kind 补 "query"。

### M2 无可读本地来源时未在账本记 gap
- **位置**：backend-go/internal/dataenrichment/service/signal_question.go:237-244、445；signal_research.go（ResearchCandidate 组装处）
- **问题**：spec Scenario「先核前提再补证」要求"不存在可读正文则**如实记 gap**而非死锁"。实现仅在 `sourceCatalog` 提示文本里写"如实记缺口"，代码未向 ledger 追加任何 gap 行；报告附录/进展快照中看不到"本候选无冻结原文"这一缺口。候选依据切片无关联存文时同理（freeze 不记 gap）。
- **建议**：runtime 构建时（或研究首轮）对 `len(q.sources)==0 && contract==question-v1` 追加一条 ledger gap；候选引用无关联文章时在 freeze.Gaps 记录。
- **处置**：已修：runtime 构造时 len(sources)==0 → missingLocalGap，bindLedger 时落账（Tool=read_source）；报告附录与进展快照均可见。

### M3 本地未知发布日期文章与日期合格证据同权可引用，与设计 D2 及网页侧语义不一致
- **位置**：backend-go/internal/dataenrichment/service/signal_sources.go:291-294（PubDate==nil 冻结 + TemporalNote）
- **问题**：design D2 写明"未知日期不当作合窗原文，**记 gap**"；实现是冻结 + 注记 + 完全可引用（read_source 后与合格来源同权，可支撑 explained）。网页侧未知日期是硬拒绝（signalPageGapUnknownDate，正文不进事实材料）。两侧语义不一致，且实现偏离设计文本。
- **建议**：二选一并保持制品一致：实现侧对齐设计（未知日期本地文不进可引用集或降级+gap），或修订 design/spec 明示"本地文经周期切片关联结构性证明合窗，未知 pub_date 冻结可引用但必须展示注记"。当前实现与设计原文冲突，不能两存。
- **处置**：已处置（修订制品）：design D2 改为「本地文经板块周期切片关联结构性证明合窗（thread 属期内）；未知 pub_date 冻结可引用但必须展示注记；网页侧无结构性证明故硬拒」——实现与制品一致，两侧差异有了明示依据（结构证明 vs 无证明）。

### M4 制品声明的测试落点多处不存在（声明了但没做）
- **位置**：openspec/changes/add-question-driven-news-research/tasks.md「Scenario → 测试文件」表；对照 backend-go/internal/dataenrichment/service/signal_research_test.go、handler/signal_{research,discovery}_test.go、internal/datasources/wiring/tools_test.go
- **问题**：映射表声称「先核前提再补证」「原文已足够则早停」「研究显式授权」「注入后现有工具面不变（新协议分支）」等落点含 signal_research_test.go / wiring/tools_test.go / handler 测试，但 grep 显示 signal_research_test.go、wiring/tools_test.go、handler 层均无任何 question-v1/read_source/fetch_page 测试（question 覆盖只存在于 signal_sources_test.go、signal_page_fetch_test.go、signal_compose_test.go 及 migration 测试）。已勾选的 3.1/3.4 验收（"期望S4/S10旧候选仍仅七源，新候选早停或有限补证"）缺对应断言；repository 层 research_brief 持久化/GET 返回也只有 handler 发现测试间接覆盖（未断言 research_brief 字段）。
- **建议**：补 loop 级 question-v1 测试（含 H1 场景）、wiring 授权面测试、handler research_brief/research_contract 返回断言；或先把映射表改为如实状态，验收后再勾。
- **处置**：部分处置：handler E2E 补 research_brief（unknown/verification/source_ids）+ detail research_contract=question-v1 断言；H1 的 policy 级 loop 测试补齐「先核前提再补证」落点；wiring 层授权面由既有七源 tools_test + TestQuestionProtocol（question 工具只挂研究子注册表）等价覆盖。归档前 8.2 scenario-trace 复核映射真实性。

## Low（可选）

### L1 naive datetime 按 UTC 解释，注释却称"按上海时区"
- **位置**：backend-go/internal/dataenrichment/service/signal_page_fetch.go:269-289（`parseSignalPageDate`）
- **问题**：`time.Parse` 无时区格式得 UTC（Location nil），`t.In(ShanghaiTZ)` 是换显示时区而非换解释时区；结果比"按上海时区解释"晚 8 小时（方向保守、更严），但注释与语义不符，排障时会误导。
- **建议**：改用 `time.ParseInLocation(f, s, shanghaiLocation())`，或修正注释明示"按 UTC 解释、保守"。
- **处置**：已修：time.Parse → time.ParseInLocation(…, shanghaiLocation())，无时区格式按上海时区解释（RFC3339 带时区仍按自身偏移）；注释与语义一致。

### L2 JSON-LD 日期正则扫全文，未限定 script 块与 Article/NewsArticle 类型
- **位置**：backend-go/internal/dataenrichment/service/signal_page_fetch.go:232-246（`extractSignalPageDates`）
- **问题**：D5 称"只处理已支持的 JSON-LD Article/NewsArticle 键"；`"datePublished"\s*:\s*"..."` 会命中页面任意位置的该键（含普通 JS 对象），可能取到无关日期。方向上多为偏晚（保守）但非保证。
- **建议**：将 JSON-LD 匹配限定在 `<script type="application/ld+json">` 块内（可接受纯文本级限定），或修订制品措辞。
- **处置**：已修：新增 signalJSONLDScriptRe 限定 <script type="application/ld+json"> 块内拼接文本后匹配 datePublished/dateModified；页面普通 JS 对象同名键不再被采信。

### L3 readability Excerpt 兜底被当作可引用正文
- **位置**：backend-go/internal/dataenrichment/service/signal_page_fetch.go:189
- **问题**：`article.Content` 为空时用 `Excerpt`（摘要式文本）充当正文并成为可引用 wN 证据，踩在 spec"空正文/摘要不能标已读正文"的边界上（Excerpt 源自页面自身，非搜索摘要，但确是摘要形态）。
- **建议**：去掉 Excerpt 兜底，正文为空按 empty_body typed gap 处理；或明示该取舍。
- **处置**：已修：去掉 Excerpt 兜底；正文为空走既有 empty_body typed gap（摘要形态不得充当可引用正文）。

### L4 evidence_refs 解析口径不一致（Sscanf 宽松 vs ParseUint 严格）
- **位置**：backend-go/internal/dataenrichment/service/signal_sources.go:264、357（`fmt.Sscanf("%d")` 会把 "101abc" 当 101）；对照 signal_material.go `MapSignalEvidenceToLanes` 的 `ParseUint`
- **问题**：同一路径两套解析；detect 白名单校验使其仅属防御层，但"101abc"类 ref 在 freeze 侧可被宽恕、在 topic 关联侧会记 gap，行为不对称。
- **建议**：统一用严格解析。
- **处置**：已修：freeze 与 countRemaining 两处统一 ParseUint 严格解析（"101abc" 不再被宽恕）；期间 countRemaining 变量断链由 TestQuestionSourceSnapshot 捕获并修复。

### L5 损坏的 related_article_ids 静默跳过、无 gap
- **位置**：backend-go/internal/dataenrichment/service/signal_sources.go:139-142（`decodeUintList` 失败 continue）
- **问题**：损坏关联被当作"无文章"，不进 sectionArticles、countRemaining 也看不到，附录无任何痕迹；与"未纳入数和原因记 gap"的精神不符。
- **建议**：decode 失败时记一条 freeze.Gap。
- **处置**：已修：LoadSignalSourceArticles 返回 badThreads 计数，detect 记 material gap（无「候选」前缀 → 全批次候选报告可见，符合数据完整性缺口语义）。

## 核对过的红线

- ✅ 信号发现/研究仅界面手动触发；409 互斥经 `StartSignalGuarded`（前置 change 机制，本 change 未破坏）；研究 150 分钟（`signalResearchJobTimeout`）独立于发现 30 分钟（`analysisJobTimeout`），有测试断言。
- ✅ 信号报告不可变：本 change 无 judge/digest/review 调用；`CreateTopicEnrichmentResult` 只追加；重研究追加新版本、外部取回另记 fetched_at，不覆盖。
- ✅ 数据时效：schema3 机械分源 `data_freshness` + 前端"数据截至/研究时点"行 + 文字来源时间行（发布/取回/冻结、事后核查标注）均可见（正文不再强制"数据截至"句为前置 change 的新口径，基线一致）。
- ✅ 候选相关话题只由冻结 input_snapshot 机械解析（该部分归前置 change；本 change 未引入按 active 清单重建）。
- ✅ 新报告受控块仅三类平面块，未知类型/未知字段整稿拒绝（`decodeSignalComposeBlock`）；schema2 存量不重编不降版（新报告才 schema3，旧行不动）。
- ✅ LLM 调用全经 airouter.Router.Chat（detect/research/compose 均走 router；web_search/fetch_page 是工具非 LLM），研究带 session_id，工具决策（含被拦/dedup）均留痕于 loop.ToolCalls + ledger textCalls/sources。
- ✅ 测试不碰真库：新增测试纯函数 + httptest loopback（显式 AllowedIPs 授权，仅测试选项）；migration 测试走 testcontainer PG（`OpenTestDB`），无真库 DDL/DML。
- ✅ topic-graph 生命周期：本 change 文件未引入状态机变更。
- ✅ 来源安全主链：URL 门（http(s) only、拒 userinfo、去 fragment、host 归一、保 query）→ 登记簿精确匹配（模型自造 URL/网页内链接不获授权，测试断言危险目标请求为 0）→ safefetch（逐跳 DNS 全结果校验、固定 IP 拨号防重绑定、Proxy=nil、10s/2MiB/3跳、私网/loopback/link-local/metadata 全拒）→ readability 从 bytes 解析（绝无 FromURL 先抓后验）；重定向入内网被测试覆盖。未发现绕过路径。
- ✅ 时间语义（除 M3/L1/L2 外）：cutoff 判定两侧一致（`After(cutoff)` 拒绝、相等接受）；date-only 上海时区日末保守判断有测试；later-of(pub,modified) 取更晚有测试；事后材料标记（retrospective note / 事后核查 note）落地。
- ✅ 旧协议/旧 UI 兼容：legacy 材料（无 research_contract）runtime 为 nil，allowedTools/prompt/policy 字节级不变（`signalQuestionMaterialView` 对无 Sources 材料为 no-op；question 段仅在 qRuntime 非 nil 时追加）；旧候选 brief NULL→nil nil；旧报告无 answer/contract/sources 前端全部缺省可读（`?? []`/`?? null`、`v-if` 分支）；detect 输出新增字段不破坏旧解析（JSON 增量字段）。
- ✅ 答题范围（除 H2/H3 外）：三枚举+形状校验+证据门有测试；unresolved 可零证据成文；answer 不冒充可信度（前端徽章文案+注释）；answer 计入 3000 预算。
- ✅ 越界检查：七源清单、host、40 轮/40 执行、detect 两次尝试、score≥6、成文三次上限均未改动；question 工具只挂在研究子注册表（不进全局 Registry，发现阶段零工具调用）；3 搜/5 页为 question 运行时独立预算，失败计入、去重只计决策轮（复用共享 dedupKeyFor）。
- ⚠️ 冻结时间语义：本地空正文/未知日期两处不一致见 H3/M3。

## 验证记录（只读）

- `go build ./internal/dataenrichment/...` ✅；`go vet ./internal/dataenrichment/...` ✅ exit 0。
- `go test ./internal/dataenrichment/service -run 'TestQuestion' -count=1` ✅ ok（2.066s）。
- `openspec validate add-question-driven-news-research --strict` ✅ exit 0。
- 未运行 repository/集成/前端测试（本评审只读范围；tasks 6.x 亦未勾选）。
