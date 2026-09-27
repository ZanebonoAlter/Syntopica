## Context

动机见 proposal.md，行为边界见两个 delta specs，交互基线为已批准 ui-design.md 与含手动查询入口的原型。现有发现逻辑位于 `backend-go/internal/admin/service/`，目录通过 RSSHub 同步生成，偏好使用行为/seed 向量，推荐引用路由。改造必须兼容现有订阅、参数字典与历史状态，不建设公共目录服务。

代码核对（未连接实库验证DDL）：模型实际位于 `backend-go/internal/models/discovery.go`，由 `platform/database/migrator.go:RunAutoMigrate` 注册。现有向量列是不限维vector；PreferenceVector的board_id+source普通唯一索引不保证NULL全局桶唯一。FeedRecommendation现有hash为全表唯一，不是仅pending唯一，迁移必须显式处理索引而非只加新约束。

现有 `admin/service/recommendation_service.go:Ask` 在插入后自行重建无数据库ID的即时卡片，且忽略部分写入错误；`AcceptRecommendation` 直接写Feed再标accepted，没有RSS验证或整体事务。`reader/handler/feed_handler.go:CreateFeed` 以原始URL精确去重；Feed.URL为唯一varchar(500)。现有RSS解析器没有请求context/体积限制，通用HTTP客户端没有SSRF防护。上述均是本次要修复的缺口，不是已具备的安全复用点。

目录同步 `catalog_sync_service.go:SyncAll` 的抓取失败可能返回空成功，content_hash不含url/example；现有画像/目录任务没有PauseAware包装。D6/D7/D9实施时须一并纠正这些路径，避免只修新入口而旧同步仍绕过契约。

本文件以下新增表、端点、配置及数值均为本次设计，不声称已经实现。实现前以迁移测试核对数据库实际约束；已有共享文件上的其他 change 改动不得覆盖。

## Goals / Non-Goals

**Goals**：独立表达候选目录、推荐结果、用户排除和真实订阅；以有界多路候选保证方向；所有写入可重试且无隐式订阅副作用；失效与数据更新可观察。

**Non-Goals**：不重做标签生成，不用 LLM 判定长期兴趣，不输出 AI 编造的目录介绍，不在第一版提供云端目录发布；原型固定示例不进入生产。新种子列表替代 discovery 的旧 seed 合并使用方式，行为画像计算继续保留现有按版块行为加权标签质心，避免改动上游算法。

## Decisions

### D1. 统一候选实体，保留 RSSHub 上游表

新增 `feed_candidates`，不向 `rsshub_routes` 塞伪路由。字段：`id`、可移植 `stable_key`、`kind=rsshub|rss`、可空且唯一 `route_id`、可空 `feed_url`、`canonical_key`、`manual_metadata` JSON、`recommendation_enabled`、`access_scope=public|private_pending|private_allowed`、`revision`、时间戳。manual metadata保存人工名称/描述/语言/地区；RSSHub原始资料仍以原表为准。有效字段按人工非空覆盖→上游→缺省计算，清空覆盖表示恢复上游。

RSSHub stable_key由namespace和路由模板构成，原生RSS以规范化地址的SHA-256构成，不导出本机ID。规范化只处理scheme/host大小写、默认端口及fragment，保留path大小写、query顺序与参数，不擅自去掉token或合并不同有效地址。拒绝URL userinfo，要求http/https；带query的敏感性采用保守导出策略，不能靠简单关键词保证安全。

同一直接地址可对应RSSHub和手动条目：保存时检查规范地址，已有可直接解析的候选则报告重复；必须填参的模板不与未实例化原生URL强行合并。订阅时以实际解析URL再次去重。后台迁移冲突保留旧关联并合并推荐资格，不直接删除历史。

选择独立实体而非拓展路由伪装：参数字典、上游diff语义仍只属于RSSHub，手动RSS不需要虚构namespace或文档地址。

### D2. 将推荐身份、展示归属与长期排除分开

新增 `candidate_preferences`：candidate_id唯一、excluded_at、snoozed_until、updated_at。它是跨qa/refresh的排除权威；目录enabled与此正交。已订阅以实际feed URL检查，必要参数模板同时遵循已有路由accepted约束，不借机扩大“同模板多个实例”的订阅能力。

推荐增加candidate_id、last_selected_at、expires_at及可审计状态。推荐hash以候选稳定身份和展示桶为基础，不含source；全局桶使用明确哨兵参与唯一键，不能依赖NULL普通唯一约束。相同hash的pending行唯一，更新理由/时间而非插入。迁移先审计重复并建立pending部分唯一索引，再移除旧hash全表唯一约束；历史行允许同hash多条但不得有多个pending。跨来源的源级排除由candidate_preferences保证，避免只靠board相关hash留下漏口。

新增 `discovery_runs`（request_key唯一、kind、query、状态、started/finished_at、错误码）与 `discovery_run_items`（run_id/candidate_id唯一、推荐引用、排序、实际召回来源、理由快照）。查询结果通过run读取，不覆盖个性化列表，也不因共享pending推荐行造成查询结果丢失。运行结果引用已经共享的推荐身份，不另造qa幂等池。响应时重查排除/订阅状态，过时页面不能绕过资格。

网络与LLM调用在事务外；发布前短事务复查资格，原子写入run结果、pending更新及成功查询的兴趣条目。失败只落运行错误，不写种子或新卡。并发提交相同request_key返回同一运行，重试失败运行复用key；用户重新主动提问可生成新key。一次个性化刷新只允许运行一轮，避免旧请求覆盖新结果。

### D3. 独立兴趣列表与明确的衰减预算

新增 `discovery_interest_entries`：id、run_id唯一、query_text、board_id可空、embedding、embedding_config_id/model/dim、created_at、legacy_ref、status。embedding存储遵循当前pgvector兼容方案，不固定为探索时2560维。失败查询不写；成功零结果也写一条完整兴趣记录。重算行为仅更新behavior，旧seed不再参与新召回，不改写成伪造的原始问答。

默认参数（服务端配置持久化、统一校验，前端第一版不另增未批准设置面板）：

| 参数 | 默认 | 合法范围 |
| --- | --- | --- |
| interest_window_days | 30 | 1–365 |
| interest_max_entries | 5 | 1–20 |
| interest_half_life_days | 7 | 1–90且不超过window |
| behavior_maturity_articles | 20 | 1–1000 |
| seed_candidate_budget | 4 | 0–16 |
| seed_match_similarity | 0.78 | 0–1 |
| seed_match_margin | 0.03 | 0–1 |

归属只比较有兼容向量的board，最高相似度达到阈值且领先第二名达到margin才归属，否则NULL；单版块只检验绝对阈值。比较用cosine similarity=1-distance，禁止混淆距离。此数值是保守起点，不声称已校准；不匹配不损失当前查询结果。

参与集合取窗口内最近K条，时间相同以ID稳定排序。条目强度 `w=2^(-ageDays/halfLife) × (1-min(N/20,1))`，N是对应版块最近30天有有效阅读行为的去重文章数，未匹配条目用全局N，分母实际取配置。age达到window即排除；系统时钟异常使age<0时按0处理。整个seed预算 `floor(seed_candidate_budget × max(w))`，不能因条数增长越过上限；按w比例最大余数法分配、稳定ID破同分。未用份额不额外奖励其他seed。成熟度达到阈值后该范围seed份额为0，保留记录显示已衰减；新的手动查询仍按本次表达完整找源，不受历史seed预算限制。

拒绝替代方案：单行EMA会丢多主题结构；按每条固定top-N会让提问数无限增加推荐份额；LLM兴趣判断无可靠真值。

### D4. 有保底的双路召回与严格精排

默认每个有效版块基础路8个、行为路8个；seed总预算见D3。每次精排batch不跨版块争抢基础路名额，批次最多20候选（基础8+行为8+受限seed），问答直接以查询向量取20个。全局行为无匹配版块时单独批次，不伪造board。目录资格过滤在截取top-N前应用：gone/broken、disabled、已订阅、冷却/排除及既有accepted路由排除，unknown保留。多路同候选去重并保留来源集合，欠额不需要凑满。

版块identity和行为向量分开检索，不混向量。基础路有候选就进入精排，但LLM仍可全不选。精排通过已有独立feed_discovery能力，不新增枚举；上下文含名称/版块摘要、实际行为标签摘要、查询（若有）及有效候选介绍，标签摘要只是证据，不声明为长期兴趣。候选介绍视为不可信数据，提示词与结构化字段隔离，不能执行其中指令。

响应采用严格JSON集合，校验标识属于输入集合、理由非空且长度上限、去重；任何未知ID/协议异常整批失败。全部批次成功才原子发布本轮；单批失败保留原轮，避免一半新一半旧。空数组合法。配置缺失也记运行失败码，不能依赖仅真实provider调用才存在的AI日志；真实调用继续遵守Operation/Capability和审计日志要求。

### D5. 推荐时效与用户选择

默认recommendation_ttl_days=14（1–365），snooze_days=30（1–365），UTC时间、边界使用now>=截止即到期。成功再入选刷新last_selected_at及expires_at；未再入选但未到期可继续作为旧推荐显示原更新时间，不冒称本轮新结果。手动查询展示该run结果，成功空结果不混入旧卡。过期卡退出默认页，历史可见；资格可以再次恢复。失败不刷新时间，也不人为延期，已经自然到期的卡仍进入历史。

UI的不再推荐写源级排除并将相关pending退出；恢复只恢复资格，不立即生成新卡。并发旧run发布前再次查排除。取消订阅不在本change自动恢复accepted路由资格，沿用现有约束。

### D6. 目录向量升级采用版本化生成指纹

新增候选向量存储，以(candidate_id, embedding_config_id, model, dim)定位；文本指纹包括有效元数据与生成器版本（当前 v2）。清洗Markdown/提示块标记但保留正文，规范空白，再按rune限制：网站/名称各80、分类/语言/地区合计80、说明257，四段合计≤500 rune——对齐 embedding 供应商 512 token 物理上限（CJK≈1rune≈1token，E2E 实测 794 token 超限报错后收紧）；拒绝按字节截断中文。没有资料保留未知，不用LLM补全。

同步/人工编辑只标dirty；worker小批处理，生成时携带候选revision和指纹，写入前复查二者以防旧请求覆盖新资料。失败保留兼容旧向量并记录重试状态。不同模型不能使用旧向量兜底；模型切换完成覆盖前阻断混算轮次。既有route_embeddings作为迁移输入/旧系统回滚资料，不进行双写的长期维护。

### D7. 可用性和网络访问安全

可用性按实际resolved URL、实例和参数组合保存，不把一个填参实例的失败提升为整个模板broken。字段含last_attempt_at、last_success_at、last_error_code、consecutive_failures、next_check_at及状态；端点变化使旧检查失效。默认检查周期7天，单次超时10秒、响应上限2MiB、重定向最多3次。只认可可解析RSS/Atom；未填必需参数记录requires_parameters，不发请求。

连续3次失败且跨度至少24小时才将短暂网络/5xx/解析失败升级broken；明确410可直接broken，429按Retry-After有界推迟、不计确定失效；成功清失败计数，不解除用户disabled/excluded。初始unknown与暂时失败不硬过滤。

public默认禁止私网、loopback、link-local、multicast、云元数据地址；private_pending在任何自动任务前都被阻断。用户明确授权自建端点后，只放行该来源解析后的指定host/port范围，不是全局关闭SSRF。每次重定向与DNS解析均检查并绑定连接目标，防DNS rebinding；环境HTTP代理不得绕过同样的限制。无权探测返回脱敏错误。不得增加抓取工具或引入浏览器作为RSS检查替代。

### D8. 导入导出是受控配置操作

第一版JSON格式 `format=syntopica-candidate-catalog, version=1, entries=[]`，携带stable_key、kind、路由标识或URL、人工字段、推荐开关；不含数据库ID、向量、检查结果、画像、订阅状态或访问授权。导入文件<=2MiB、最多1000条、字段限长（名称200、URL500字符（与既有Feed.URL兼容）、说明4000runes），服务端和客户端都校验。拒绝未知版本和非法类型。

预览无DB写入和网络；给出有效/重复/冲突/无效。确认请求带预览指纹及本地revision，冲突默认跳过，不支持静默覆盖；确认期间记录变化返回需重新预览。逐条事务应用有效项，唯一键使重试幂等；返回逐项结果，部分失败明确不是全部成功。导入私有或不能安全分类的地址为private_pending，所有后台任务遵守授权门槛。

默认导出仅public且无query/userinfo的条目，私有、含query或标为敏感的整条排除并说明数量；保守排除可少导但不能假称可识别任意token。第一版不提供“连凭据一起导出”。跨安装导入RSSHub条目时以namespace/path绑定本地上游，未同步的条目保留未解析状态不捏造技术参数，用户同步后再启用。

### D9. API、前端和调度集成边界

保留现有发现入口及接口语义，新增候选库资源 `/api/discovery/candidates` 的分页GET/POST/PATCH、import/preview、import/confirm、export、check、access确认；兴趣列表与run详情使用 `/api/discovery/interests`、`/api/discovery/runs/:id`。这是拟定资源路径，实现时映射现有路由group避免重复前缀。推荐响应新增candidate和run上下文，RSSHub仍返回route.param_options，原生不伪造route；旧路由accept参数兼容迁移后的candidate引用。

分页默认30、上限100，query限500runes。错误响应统一可识别code（validation/conflict/configuration/unavailable/forbidden），不回传凭据/私有完整URL。候选订阅抽出可供普通建源与推荐accept共享的服务层边界，不从service反调handler。新安全fetch获取有界响应后调用解析能力；成功验证后在短事务内按规范URL去重/创建Feed并标accepted，提交后沿用原有后续处理入口，不另起抓文章流程。RSSHub填参后最终URL也必须不超过500字符，超长就地拒绝且不写Feed。旧accept也必须接入此路径，不能继续直写绕过验证；原型缺失的真实导入文件、下载和后端错误在产品实现补齐，不复制演示fixture。

前端仍在features/discovery边界拆分推荐/候选库/兴趣记录；AppPageShell contained、统一dialog，遵循批准原型，不新增参数配置管理页。手动查询保留输入与独立run视图，refresh只刷新个性化，按钮与请求状态分开。

后台通过scheduler.Registry注册目录检查和推荐历史维护任务，不在handler维护第二清单。单job不并发；触发接口按accepted判成败。目录embedding属分析类遵守analysis_paused，历史过期和纯HTTP检查属维护类不受分析暂停影响；目录同步原有职责不扩大到自动订阅。初始全量向量回补限批，不能阻塞每次页面请求。

## Risks / Trade-offs

- [8+8和0.78等默认值尚未实测校准] → 在隔离fixture上比较至少日本新闻、AI、开发工具版块的基础候选入选、重复率与零结果，不把单案例推广为全局证明；参数可调，不以增加LLM掩盖数据缺失。
- [查询次数相同但行为成熟度达到阈值后seed不再参与刷新] → 兴趣记录显示衰减原因，当前手动查询不受该限制；避免用户以为搜索功能失效。
- [严格整轮发布提高失败概率] → 配置校验前置、批次有界、可观测错误与重试；不将失败静默降级成低质量推荐。
- [内网访问与可分享配置存在张力] → 私网授权本地化，默认保守导出，重定向逐跳验证，拒绝自动授权。
- [新增统一实体迁移复杂] → 保留旧route关联、分步回填、幂等索引与隔离DB测试，不直接替换或删除旧表。
- [向量重建费用] → 仅dirty条目、限批重试，暴露待更新/失败计数；无需重复构建未变条目。

## Migration Plan

1. **发布前**：列出候选/推荐/seed计数与异常归属审计，备份相关表；只在隔离数据库验证迁移，不执行生产清理。记录现有模型与能力配置，明确一次重嵌成本。
2. **扩展schema**：创建新表和nullable关联，不删除旧列。按稳定键回填RSSHub候选，旧推荐回填candidate_id，冲突保存映射和审计。重新执行只补缺失项。
3. **旧状态**：accepted及真实feed保留；dismissed沿用旧时间推导剩余冷却，不转换永久排除。旧pending进入legacy历史，不伪装成新精排结果，下一轮成功后展示新结果。旧seed不知原句的不编造，标legacy inactive并保留旧引用；明确告诉用户旧兴趣未自动迁移，可重新查询表达。
4. **向量准备**：先建立新候选向量，只有同配置兼容条目可参与；全部不可用时给初始化状态，不偷偷混旧模型。新介绍指纹不同的一次重建由后台完成，无需用户手动SQL。
5. **切换**：内部`discovery_v2_enabled`开关默认关闭，schema/backfill成功后启用服务和新UI。初始化失败保持旧入口，不开启一半新写路径；上线后按可观测检查确认。
6. **回滚**：关闭v2入口并停止v2 worker，保留新增表和手动候选，新目录仅暂不可见；不drop。回滚到旧推荐引擎前须将candidate_preferences的冷却/排除投影至旧路由资格，旧版本无法表达长期排除时禁用旧推荐发布而非绕过用户选择。真实feeds始终保留。不得宣称完全无损降级到旧功能集。
7. **用户操作与部署影响**：已有订阅和文章不变；旧待处理卡进入历史，旧不可靠seed不再参与；推荐可能暂空，后台会重建向量。用户仅需按提示检查feed_discovery配置及对旧兴趣重新提问，私有源需显式确认访问；不要求清库。旧数据不自动删除，后续清理另议。

后续tasks需包含复杂档test-cases故事账本：并发幂等、截止时间两端、NULL桶、参数回归、AI未知ID、DNS/重定向越界、重复导入、迁移重跑与回滚用户排除保护。规划校验通过不等于这些测试已通过。
