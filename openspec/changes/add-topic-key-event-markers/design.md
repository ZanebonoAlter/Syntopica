## Context

动机与范围见 proposal.md。现有 DailyReportThread 已有标题、摘要、tag_ids、related_article_ids，话题归属经 section 取得；SaveReport 同日重生成会删除旧 thread/section，再插入新行，ID 不稳定。泳道已有 focus 就地展开，信号发现已有按周期/cutoff 装配并冻结的 SignalMaterial。关键代码定位见 explore-findings.md。

## Goals / Non-Goals

**Goals:** 在现有 thread 上增加轻量人工选择层；重建后保留选择；用有界、非阻断 AI 提供建议；只将确认节点作为手动信号发现的关注信息；具体用户行为见 4 份 delta specs 和 ui-design.md。

**Non-Goals:** 不建立通用事件实体/事件自动合并服务，不新增全文检索或 embedding 去重，不把快照当新闻原文，不迁移整套 UI，不扩展研究工具或自动调度研究。

## Decisions

### D1. 独立轻量注记，而不是 thread 布尔字段或事件管理系统

选 `topic_key_event_markers` 注记表；一个条目的一次进展是一条注记，并非跨天事件集合。仅在有建议或用户动作时建行，无建议的普通 thread 不提前占行。

| 主要字段 | 来源/约束 |
| --- | --- |
| id、semantic_board_id、persistent_topic_id | 服务端解析 owner；板块删除沿用删除语义，topic 硬删可置 NULL 保留注记，不阻塞旧删除 API；孤立注记不混入别的泳道 |
| source_key、source_report_id、source_thread_id、source_section_id | source_key 为服务端确定性来源指纹；thread/section 可 NULL，删除用 SET NULL，不用 CASCADE 删除用户选择 |
| source_period_date、topic_label_snapshot | 上海业务日期与当时泳道名；来源日报日期不是事件发生时间 |
| title_snapshot、summary_snapshot、tag_ids_snapshot、article_ids_snapshot | 服务端来源；确认之后不被 AI 更新成新叙事 |
| state | suggested / confirmed / dismissed，DB CHECK；manual mark 直接 confirmed，取消也进入 dismissed，仅表示不再选择 |
| recommendation_reason、recommendation_basis | 有效 AI 输出及校验过的依据；无建议手动标记时为空 |
| last_action、confirmed_at、decided_at、revision | 区分人工 mark/confirm/dismiss/unmark；用于幂等、冲突与时间审计，不对用户暴露复杂状态机 |

同 owner + source_key 唯一；source_key 内容为 board/topic/来源日报日期/规范化标题/排序去重后的 tag_ids 与 article_ids，SHA-256。该指纹只判定“完全相同的来源”，不是事件语义 ID。不用标题相似度移植确认；证据或标题变化即不能自动视为同一来源。确认行内容快照冻结；AI 只允许写新的 suggested 或更新尚未处理且来源相同的建议。

重建先断开即将删除的来源引用，在同事务内完成；确认快照不删。新条目只有完全一致来源指纹时可重新附着指针，其他保留为来源替换快照。AI 推荐时已处理快照仍进入对照上下文，抑制不同措辞的旧事件建议。合并/分裂改变话题归属时，不静默搬运旧人工选择，旧快照留在原确认泳道（归档泳道仍可查）；新条目由用户另选。topic 硬删后无泳道回看入口，不把遗留注记挂到同名新话题。

**方案比较**：thread 布尔字段最短，但重建删除会丢选择；通用事件表能跨天聚合，但扩大范围；独立快照注记是保持用户选择的最小方案。实现时确认 PostgreSQL FK 与现有 GORM 删除顺序，使用一次性 testcontainer 测试，不能在真库试 DDL/DML。

### D2. 简单服务端动作、明确幂等与冲突

拟定端点：
- `GET /api/daily-reports/:report_id/key-events`：当期来源映射及推荐状态（idle/running/done/failed），按作用域返回注记。
- `GET /api/daily-reports/topics/:id/key-events?days=14&state=confirmed`：7/14/30/全部窗口的确认节点（days=0 全部），来源日期倒序/id 倒序。lifeline 可选 key_events 同形状复用读取；topic读端默认/只允许confirmed，不从日报待处理集合装配总览注记。AI推荐状态/理由仅由日报读端展示，总览即使共享缓存已有suggested也按普通来源投影。
- `PUT /api/daily-reports/topics/:id/key-events/threads/:thread_id`：body 仅 `action=mark|dismiss` 与已有注记的 expected_revision；首建/处理有效来源，不接受客户端文案。首次无行可不带 revision，若已有行且要改变状态则要求最新 revision，避免绕过冲突保护。
- `PATCH /api/daily-reports/topics/:id/key-events/:marker_id`：`action=mark|dismiss|unmark`、已读 revision；来源失效的 confirmed 允许 unmark，但不能凭旧来源新建/重新 mark。

HTTP 经项目既有 success/data/error/message 信封；非法输入/动作400、越界或不存在404、不同状态的旧 revision409并返回最新状态。相同目标状态重复提交直接返回已保存结果。DB 事务 + 唯一约束 + 行锁保证来源单行；重建与动作共享来源事务校验，重建先提交时对旧 thread 的新操作404，动作先提交时随后保留其快照。晚到 AI 保存不能更新 confirmed/dismissed。

### D3. 推荐是一次有界 best-effort，不是另一条分析链

新日报 SaveReport 成功并完成既有终态后，派发独立、脱离 HTTP request 的短时推荐；不加入 SignalDiscovery 的202/409分析任务，不延长日报域闸或发第二条终态。每板块最多一个推荐任务，重建产生新 generation token，使旧任务保存时可识别为过期；token 不匹配整批丢弃。重启不自动恢复付费调用，failed/running 存量读时如实降级，下一次新日报仍会默认推荐。

复用 airouter，独立 Operation `daily_report.key_event_recommend`、CapabilityDigestPolish、同日报 session 派生子 session；单次 Chat、不联网、不启工具循环、不自动重试，90秒上限，串行执行。候选最多40条（按日报顺序、稳定ID），无有效候选不调AI；超限有明确截取说明。用户打开页面只读，不触发推荐。

输入分为：当前候选 thread 的原始 tag 事实与文章依据；泳道近期14天事实指纹；最新已确认及已不采纳/取消节点各最多20条。总文字预算12000 runes，有界裁剪并留 input 日志说明。历史人工节点仅作“用户已选择/已处理事件”的对照，不能拿历史 AI 叙事当作新的事实来源。用户处理记录只抑制同事件同进展，不抑制后续实质进展。

输出只有 source_thread_id、reason、new_progress、evidence_refs；理由为空/纯空白、ID越界、无当期有效依据丢弃，不信模型自报 owner、状态、重要性分。Prompt 强调稀疏选择、相对于泳道的意义，已有事件说明实质新增变化。先机械过滤完全重复来源，再让模型比较进展；语义推荐质量仍需真实材料验收（test-cases.md S2），不是算法能绝对保证的事件识别。

### D4. 来源快照用于回看，信号证据依旧严格

SignalLaneMaterial 增加可选 confirmed_key_events；装配只读注记，不调用推荐。按来源日报日期筛选 `[From,To)` 且日期不晚于 cutoff，同 board/topic，保留 snapshot/confirmed_at；用户后来标记旧事标作 current_retrospective_attention，不能声称历史当时可得。

注记只作 attention，不注册新的“关键事件证据 ID”。有效证据仍由本次 section 白名单解析：原引用已删除时，可对指纹严格一致的存续来源映射；不一致或无有效 section 记 gap，不让仅存的快照独立制造候选。关注字段与材料同批写入 input_snapshot；研究完全读冻结版本，不查询新标记、不修改旧报告。无标记保持旧 payload 可读（字段 optional），不回刷旧结果。

### D5. UI 与共享状态

FB-4只读实查当前日报：BoardDailyReportTimeline卡片→全屏drm-reader，报纸封面/头条/highlights/目录/话题正文/页边注；DailyReportTopicSection按TopicGroup→sections→threads渲染，实际同section含5条线索。既定数据粒度本来就是thread，不能因旧原型1:1fixture把父section当线索。新增动作宿主是article.drm-thread，与button.drm-thread__header同级（header点展开本thread文章），原summary的data-mn-anchor=t:thread.id、跑题提示保留；无文章展开能力不能误禁有效标记。

同section的A/B独立，来源动作使用thread.id，快照取thread.title/summary/tag_ids/related_article_ids，topic owner通过section验证。总览confirmed注记以marker/thread身份区分：同一真实section可旁挂多条确认线索，不能用sectionHasKey染整章、替换原cluster_label或把多个thread伪装成多个section。真实section集合/计数/关系不变。

复用现有 API/composable/文章缓存；新增 `useTopicKeyEvents` 和无副作用 `KeyEventMarkerActions`，由宿主发起读写。每条目请求锁、revision 与查询 generation 防响应串板块。乐观状态显式待保存，失败回滚；未知结果先重读。详情、历史与focus同时失效/更新，不只改某一组件本地布尔值。

现有TagsPage按selectedBoardId挂BoardThreadBrowser；总览仅该版块lanes，聚焦是既有focus，全部仍是文章池。两个版块不能共用一条全局话题轴。原lanes/focus以可读长轴呈现，confirmed独立注记高亮、普通section/关系保留，未确认建议不进入总览注记层；对应新闻仍是普通节点，不带推荐徽标/理由/待确认aria文案。突出开关只改变强调，不删除节点或移动日期坐标；同日节点错开、标题可读，点击后复用原就地详情，不创建新的viewMode。section空但确认快照仍在时以独立注记覆盖层回看，不伪造section/关系。日报承接确认/不采纳/新手标，动作与展开按钮同级，保留跑题警告和页边注锚；总览详情仅新闻/confirmed回看与来源日报导航，允许已确认取消维护（包括失效快照），不开放未定挑选。原日报日期选择继续复用；跳转须定位正确版块、来源日报日期/有效thread，不伪造快照锚；轴滚动仅在局部容器内。版块总览装配只能以当前board_id与本次返回的topic集合为白名单读取注记，快照与真实section分开；不做全局标记流或无界全topic扫描。版块切换清理局部请求generation/选择，合法在途人工写仍存原owner，不向新owner显示旧提示。保持原cluster_label、statusFill五态及tag_count来源；theme仅用现有main.css的editorial/dark，关键外环不替换状态色。UI细节见major/pending契约与FB-2/FB-3/FB-4。

### D6. 原型验证范围

FB-4发现当前原型日报页仍是扁平演示、fixture一thread=一section，不体现真实reader及一章多线索，不能作为新版日报接入/批准基线。此前148断言只证该简化模型。本轮只读探索/澄清制品，未修改原型代码；下一版须保留真实阅读器结构，用不同section标题与A/B thread标题、多篇依据验证独立标记后再请求用户审批。major/pending不变。

根据用户要求，在 ui-prototype/index.html 制作独立静态原型：日报→泳道的既有上下文、推荐与人工操作、重复报道/新进展示例、局部失败回滚、失效来源、空态、双主题及窄屏。使用明确标为演示的 fixture 与专属 localStorage；不取真库、不调API、不宣称fixture为真实新闻。不以原型本地保存代替 PostgreSQL 持久化/AI效果测试。

用户反馈拒绝列表式回看，改为泳道长轴高亮并提升可读性，UI按major重新审批，approval=pending；必须看过新原型并明确确认后才可实现。复杂逻辑的方案比较与风险已在 D1-D4 记录，白盒边界见 test-cases.md。原型可丢弃，不把其 HTML/JS 搬进 Nuxt。

## Risks / Trade-offs

- [LLM可能漏荐或误荐] → 人工直接标记始终可用；不强凑数量；真实样本量化、逐例说明，不以mock假装效果达标。
- [有界上下文遗漏很久以前的事件] → 不承诺全历史语义去重；完全来源重复机械拦截，旧处理节点对照及人工不采纳兜底；达到数量痛点后再扩展检索。
- [来源重建导致脱钩] → 快照保留、只严格指纹重附，不盲目搬运；可读性与可核查性分开。
- [新表/FK影响删除] → PG集成覆盖日报重建、topic硬删/归档/合并分裂；不更改既有生命周期主权。
- [模型建议增加成本] → 有界候选、单次无工具、90秒best-effort；日志留成本，不自动历史补跑。
- [数据增强被用户关注诱导只求证] → Prompt明确关注不等于结论，保留既有新闻依据门和反证边界。

## Migration Plan

实现期新增幂等迁移及模型（本次不执行）；先后端部署再前端，附加字段可选、无标记为空，无需重建日报。旧新闻可人工标记，不批量付费回刷；既有汇总、候选、报告不变。回滚前端/停用推荐即可，保留新表中的人工选择不删；若回滚到完全不支持来源FK的旧后端，保留快照并在一次性PG验删除兼容后才切换，不以DROP表“回滚”。上线及前端静态部署遵循既有脚本。

## Open Questions

无阻断性未决项。未来跨泳道、多事件聚合与治理界面另开 change，不作为本次实现前置。推荐候选/上下文预算可据真实材料效果调整，但不能改变人工主权、材料边界或引入自动研究。
