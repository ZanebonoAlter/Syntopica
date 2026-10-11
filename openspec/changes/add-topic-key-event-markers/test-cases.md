# test-cases: add-topic-key-event-markers

本文件是实现与验收计划，不是已运行测试报告。规格共49个Scenario，逐条落点见 tasks.md 验证表；新测试文件在实现阶段创建。主链路必须走静态站 :5100 的 opencli，原型浏览器演示不代替真实后端验收。

## 故事 S1: 我亲自选择真正关键的事件（锚 Requirement: 人工选择关键节点、条目标记交互、保存选择与重复操作）

### 主链路（节拍串联）
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 先打开总览，再进入来源日报 | 建议只在日报展示、总览不承担新选择、建议必须确认才生效 | 总览无建议徽标/理由/待确认aria/新挑选动作，但普通来源保留；正确版块/日期/thread的日报才显示推荐与完整选择 | 组件/opencli | BoardThreadBrowser.focus.test.ts、KeyEventMarkerActions.test.ts、DailyReportTopicSection.test.ts |
| 2 | 点击确认，再打开同泳道 | 重复确认只保留一个节点 | 服务端单行confirmed，重复动作幂等 | PG/HTTP/opencli | key_event_markers_pg_test.go、key_event_handler_test.go；人工：opencli走全链 |
| 3 | 不采纳另一条，再改主意手动标 | 不采纳后仍可选择 | 新闻保留，人工动作不需要AI许可 | 组件/PG | DailyReportTopicSection.test.ts、key_event_markers_pg_test.go |
| 4 | 在日报标记无推荐条目 | 无建议也能手动标记 | 直接确认；分析job计数不增加 | HTTP/opencli | key_event_handler_test.go；人工：opencli确认无研究启动 |
| 5 | 刷新，再取消节点 | 取消不删除新闻或报告 | 状态恢复后退出关键高亮，新闻/旧报告不变 | PG/组件/opencli | key_event_markers_pg_test.go、BoardThreadBrowser.focus.test.ts |
| 6 | 连点/制造保存失败 | 写入失败回到原状态、未知状态与重复提交反馈、标记不误展开文章 | 每条一请求，回滚/重读，不展开文章 | 组件/状态 | KeyEventMarkerActions.test.ts、useTopicKeyEvents.test.ts |
| 7 | 在真实形状的一章A/B中仅标A | 一章内标记不连带其他线索 | A confirmed、B不变；父标题/状态/原计数/关系不变，动作不嵌套文章header；summary的t:thread.id锚和原文不变 | 组件/PG/opencli | DailyReportTopicSection.test.ts、key_event_markers_pg_test.go；人工：现有全屏reader走线索故事 |
| 8 | 展开A的多篇依据 | 一线索多篇依据只标一条 | 一条marker对应A，不给每篇另建标记；文章可独立展开，父级不因标记变成事件 | PG/组件 | key_event_markers_pg_test.go、DailyReportTopicSection.test.ts |

### 变体走查
| 组/条目 | 期望答案 | 层/落点 |
| --- | --- | --- |
| 输入：空串/纯空白(全角/tab)/纯分隔符(单/连/首/尾)/未知action | 400，无新注记；客户端不得替用户猜动作 | key_event_handler_test.go |
| 输入：单token action、大小写、特殊字符/emoji、超长 | 合法小写枚举可用，其他400；JSON文本拒绝/忽略且服务端快照不变 | key_event_handler_test.go |
| 前置：无topic、空thread、越界/跨域 | 无归属400，不存在/他域404；不自动建话题 | key_event_handler_test.go |
| 前置：单条/重复、部分字段缺失 | 单条动作可用，重复幂等；旧thread无推荐可手动标记 | PG、DailyReportTopicSection.test.ts |
| 时间窗口 | 写动作不以浏览窗口删除选择；来源日期按上海日归一，视口/日期切换不改人工状态 | useTopicKeyEvents.test.ts |
| 幂等：重复执行、部分失败重试 | 同一来源单行；未知结果重读再幂等重试 | PG/HTTP/composable |
| 并发：确认/取消、AI迟到、重建交叉 | revision不同目标409；AI不覆盖人工；重建与标记串行结果可解释 | PG/service |
| 可用性：误输入反馈/空态/错误态/加载态/超长/重复提交 | 错误就地、原新闻不丢；无确认保留背景且可关闭强调；按钮每条锁定；理由换行；键盘可达 | 组件+opencli |

### 效果核对
人工主权与状态不依赖模型准确率，用HTTP/PG/浏览器断言；真库只做只读核对及受控验收动作，不批量确认新闻。

## 故事 S2: AI 提醒我有新进展，但不反复推旧报道（锚 Requirement: 有依据的重要进展推荐、日报推荐与历史兼容）

### 主链路（节拍串联）
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 生成并保存有料新日报 | 建议必须确认才生效 | 一次推荐仅存suggested，未转confirmed | service | key_event_recommendation_test.go |
| 2 | 输入同事件转载 | 转载不当新进展 | 对照已有/已处理记录，不推旧事实 | 函数/效果核对 | key_event_recommendation_test.go；人工：真实材料对照 |
| 3 | 输入正式开放/重大结果 | 新的实质进展可再推荐 | 新节点理由说明增量，不覆盖旧确认 | service/效果核对 | key_event_recommendation_test.go；人工：样本审核 |
| 4 | 模型返回非法ID/空理由/跨域证据 | 无依据或跨泳道建议丢弃 | 丢非法单项，合法项继续 | 函数 | key_event_recommendation_test.go |
| 5 | 超时/解析失败 | 推荐失败可手动选择、推荐失败不改变日报终态 | 已保存日报成功、零第二终态；手动入口可用 | service/组件 | key_event_recommendation_test.go、daily_report_notification_test.go |
| 6 | 打开旧日报 | 旧日报可直接手动标记 | 无隐式AI调用，按新读结果显示旧确认 | 组件 | DailyReportTopicSection.test.ts |
| 7 | 人先标记，AI后到 | 迟到推荐不覆盖人工决定 | 已人工处理状态不变 | PG/service | key_event_markers_pg_test.go |

### 变体走查
| 组/条目 | 期望答案 | 落点 |
| --- | --- | --- |
| 输入：理由空串/全角/tab/分隔符；单token；大小写；特殊字符；超长 | 空与无语义理由丢弃；有事实的单token不按长度武断拒绝；文字保留语义不执行；超预算有明确截取而不改事实 | 函数单测 |
| 前置：空候选/单条/重复/越界/部分满足 | 空不调用；单条可建议；重复来源只一条；非法项丢弃不污染合法项 | service/PG |
| 时间：14天边界/空窗口/跨窗口/上海归一 | 对照范围固定且日志可追；窗口外不伪装已检查，确认节点不因窗口变化删除 | service |
| 幂等：同来源重生成/失败重试/并发 | 已处理不再推；页面读取不自动重试；过期generation整批丢弃 | service/PG |
| UI可用性六项 | 建议失败非阻断，空无催促，长理由可读，保存态不锁正文；异常建议不出按钮 | 组件/opencli |

### 效果核对（问句④必做）
- 触发原因：重要性与“新进展/重复报道”依赖真实LLM行为，mock只能测解析和边界。
- 核对方法：实现后只读选取至少20组同泳道材料，含重复转载、普通评论、重要后续和首次重要事件四类；人工逐例给参考判据，再用配置中的实际模型在受控评估入口运行（不重建正式日报、不写正式注记）。记录 ai_call_logs session、入选/剔除及理由。
- 量化结果：待实现，**不填假数字**；报告重复误荐数/样本数、重要进展漏荐数/样本数、非法引用数、空输出数及调用成本；非法引用落库须为0，所有错误有案例。
- 结论：待真实核对。语义质量不凭任意分数宣称达标，逐例给用户判断；效果不合预期先定位事实材料/上下文/模型层，不默默交付。

## 故事 S3: 重建新闻不抹掉我的选择（锚 Requirement: 日报存储、已确认节点的来源语义、作用域与来源保护）

### 主链路（节拍串联）
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 保存并确认来源条目 | 展示不重新编写新闻 | 保存快照/来源日期/决定，AI理由不冒充批注 | PG/组件 | key_event_markers_pg_test.go、KeyEventMarkerActions.test.ts |
| 2 | 同日重生成，旧ID消失 | 重生成保留选择而不移植确认 | 确认及dismissed不删，不相似迁移 | testcontainer PG | key_event_markers_pg_test.go |
| 3 | 原section空、文章部分缺失 | 来源失效仍可回看、无 section 仍能查确认快照 | 快照可读、坏来源不链接，不伪造图关系 | PG/组件 | key_event_markers_pg_test.go、BoardThreadBrowser.focus.test.ts |
| 4 | 试跨板块/无归属/伪造内容 | 越界引用拒绝、无归属及伪造快照拒绝 | 404/400，服务端来源不被篡改 | handler | key_event_handler_test.go |

### 变体走查
| 组/条目 | 期望答案 | 落点 |
| --- | --- | --- |
| 输入：空/空白/分隔符/单token/大小写/特殊字符/超长标题 | 来源有内容则保留原文，指纹仅归一空白不降大小写/剥emoji；客户端标题不参与保存，超长 UI 可读 | 指纹函数/组件 |
| 前置：空/单/重复/越界/部分证据 | 空来源拒绝；部分证据失效只禁用失效引用；唯一约束挡重复 | PG/handler |
| 时间：同日/跨日/时区 | 同一日期完全来源可重附；跨日不机械视为同一来源；来源日上海归一 | PG/函数 |
| 幂等/并发 | 重建事务回滚不半脱钩；重复重建不删除决定；来源锁交错可解释 | PG |
| 可用性六项 | 失效提示可读、取消可用；不会把无码快照显示成可核查原文 | 组件/opencli |

### 继承与调整（⓪）
已运行 `bash scripts/harness/test-assets.sh daily-report-system` 与 `persistent-topic`。早期 persistent-topic 归档无测试映射，不据此声称已覆盖；以现有测试文件补盘。

| 旧 Scenario | 处置 | 旧测试文件 | 动作 |
| --- | --- | --- | --- |
| 保存日报和分组 | 继承+扩展 | backend-go/internal/topicgraph/repository/daily_report_repository_test.go | 旧保存断言保留；新FK/保留快照用新增PG文件，不用SQLite代替 |
| 查询最近7天/超过30天/非正数默认 | 继承 | 同上、handler/daily_report_handler_test.go | 保留范围与排序回归，不将0“全部关键节点”混同日报ListReports默认7天 |
| 查询话题完整生命线/跨命名漂移 | 继承+可选字段 | repository/daily_report_topic_lineage_test.go | 保留真实sections与relations，扩展key_events，不重写旧身份逻辑 |
| focus 就地展开/拖拽防误触/空话题 | 继承+改空态分支 | front/app/features/tags/components/BoardThreadBrowser.focus.test.ts | 原section空仍为空时间轴，但确认快照可显示；其余旧断言不删除 |

### 效果核对
PG真实约束在一次性容器验；不在本地真库执行重建/删除实验。原型失效演示只测试展示，不替代 FK 验证。

## 故事 S4: 我在长泳道上读清关键节点与时间脉络（锚 Requirement: 当前泳道关键节点高亮、泳道节点可读性与就地详情）

### 主链路（节拍串联）
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 进入focus并突出关键 | 高亮保留背景但不混入他泳道 | 人工节点高亮，未确认建议仅其来源新闻作普通背景，无建议标签/理由/挑选动作，日期/坐标不变，他泳道不混入 | 组件/opencli | BoardThreadBrowser.focus.test.ts |
| 2 | 选择节点 | 点击节点读详情但不标记 | 下方原位显示完整内容，选择不确认、不研究 | 组件/opencli | 同上 |
| 3 | 同日多个及超长标题 | 同日标题错开且长文本可读 | 标题/命中区不互盖，完整内容在详情可读 | 组件/opencli | 同上 |
| 4 | 375px查看与点击 | 窄屏长轴局部滚动 | 仅轴滚动，页面不横溢，44px触摸目标与键盘可达 | 组件/opencli | 同上 |
| 5 | 取消最后一条 | 取消最后节点仍保留时间背景 | 当前无确认提示，原动态仍在可点，坐标不变 | 组件/opencli | 同上 |
| 6 | 切泳道，旧请求迟到 | 切换泳道不串状态 | 重置强调，旧数据不覆盖新内容 | composable/组件 | useTopicKeyEvents.test.ts、BoardThreadBrowser.focus.test.ts |

### 变体走查
| 组/条目 | 期望答案 | 落点 |
| --- | --- | --- |
| 输入：日期选项非法/空/空白/分隔符/大小写/特殊字符/超长 | query仅允许数字窗口；400，不误当全部；原内容保留 | handler |
| 前置：0/1/重复/跨域/只快照无section | 无确认仍保留背景；单条可取消；不重复；无section仍可見独立注记覆盖，不伪造图节点 | PG/组件 |
| 时间：当天/下界/前后一步、空窗口、跨窗口、时区 | 来源日在选定自然日范围内，未来不显示；切窗口不丢选择；上海统一 | PG/函数 |
| 幂等：重复切换、错误重试、并发加载 | 突出开关不写业务状态/移动坐标；旧请求不抢新结果 | composable |
| 可用性六项 | 加载局部/无确认仍可读/错可重试/长标题详情可读/无全页溢出/键盘选点与44px触摸入口 | 组件+opencli |

## 故事 S5: 分析关注我选的事，但不把选择当结论（锚 Requirement: 周期材料与固定候选快照）

### 主链路（节拍串联）
| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 手动发现当期信号 | 只冻结合窗人工关注 | owner/cutoff/状态过滤，只读已确认，仍需新闻依据 | service/PG | signal_material_test.go、key_event_markers_pg_test.go |
| 2 | 取消标记再研究旧候选 | 后续撤销不改旧研究、迟些点击仍用原快照 | 原input_snapshot字节不变 | service/PG | signal_research_test.go、signal_repository相关PG资产 |
| 3 | 标记旧事再回顾历史 | 事后选择不冒充当时预测、历史材料隔离 | 本次回顾关注、日期/确认时间可追溯 | service | signal_material_test.go、signal_detect_test.go |
| 4 | 原来源无有效切片 | 失效依据不升级为有效信号 | gap，不注册伪证据、不产无依据候选 | service | signal_material_test.go、signal_detect_test.go |
| 5 | 传非法period或工具晚于cutoff | 周期校验、外部数据边界 | 原400/nojob与取数边界不变 | service/handler | signal_material_test.go、signal_research_test.go、signal_discovery_test.go |

### 变体走查
| 组/条目 | 期望答案 | 落点 |
| --- | --- | --- |
| 输入全部固定项 | 沿用period校验：空白/分隔符/大小写/超长400；合法month/year可用；引用文本不执行 | handler/service |
| 前置空/单/重复/越界/部分满足 | 无标记旧材料兼容；只接受本作用域confirmed；无可核查切片gap，不用快照凑候选 | service/PG |
| 时间两端/空/跨窗/上海 | `[From,To)`；来源事实<=cutoff，未来剔除；后来选择仅用于本次回顾 | service |
| 重复/失败重试/并发 | 每批冻结，失败无半批；取消不改旧快照，发现与研究仍原互斥 | PG/handler |
| 可用性 | 本次无新增分析表单，沿用工作台输入错误/空候选/失败反馈；不新增研究按钮或自动导航 | 既有workbench回归 |

### 继承与调整（⓪）
已运行 `bash scripts/harness/test-assets.sh board-signal-reports`。
| 旧 Scenario | 处置 | 旧测试文件 | 动作 |
| --- | --- | --- | --- |
| 周期校验 | 继承 | backend-go/internal/dataenrichment/service/signal_material_test.go、handler/signal_discovery_test.go | 原month/year及未来400继续跑 |
| 历史材料隔离 | 继承+添加关注字段 | service/signal_material_test.go | 原period/cutoff不放松，新增回顾标签 |
| 迟些点击仍用原快照 | 继承+扩展 | service/signal_research_test.go | 额外断言取消/新标记不改旧候选 |
| 外部数据边界 | 继承 | service/signal_research_test.go | 七源cutoff包装不变，不顺手改变数据源协议 |

### 效果核对
关注是否影响推荐方向需在 S2 真实材料报告中记录“有/无已确认线索”对照；只读已保存AI日志与快照，不批量改人工选择。未达到效果不得用“输入字段存在”替代用户价值。

## 故事 S6: 我在当前版块的原泳道里看清关键节点（锚 Requirement: 版块内泳道边界与既有主题）

| 步 | 用户动作 | 来源 Scenario | 期望断言 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 左侧选择AI版块，进入话题总览 | 总览只展示选中版块泳道 | 仅AI泳道/section/注记；原位详情注明版块与泳道；无住房节点 | 组件/opencli | TagsPage.test.ts、BoardThreadBrowser.focus.test.ts |
| 2 | 点本版块泳道进入focus再返回 | 总览只展示选中版块泳道 | focus只该泳道，返回仍同一版块多行，selectedBoardId不变 | 组件/opencli | BoardThreadBrowser.focus.test.ts |
| 3 | 在途读/人工写未完时切住房版块 | 切换版块迟到结果不串入 | 旧读不显示/结束新读取，旧合法写只存旧owner，提示不串，新版块选择/强调重置 | composable/组件/opencli | useTopicKeyEvents.test.ts、TagsPage.test.ts |
| 4 | 确认/取消并切浅深主题 | 双主题高亮不覆盖原状态语义 | main.css的editorial/dark生效；原status/状态色、cluster_label、section ID、动态数/tag_count不变；仅注记层更新 | 组件/opencli | BoardThreadBrowser.focus.test.ts；ui-acceptance/report.md |
| 5 | 侧栏选择全部 | 全部入口不汇总所有泳道 | 仍是文章池，不能给全局话题轴或混合注记 | 组件/opencli | TagsPage.test.ts |

| 五组变体 | 明确答案 | 层 |
| --- | --- | --- |
| 输入：空串/全角或tab空白/纯分隔符/单token/大小写/特殊字符/超长 | 本故事无自由输入（空/空白/分隔符/大小写不适用）；引用ID非法/越owner拒绝；单泳道正常；超长版块/泳道名可读且不撑破 | 函数/组件/opencli |
| 前置：空集/单元素/重复/越界引用/部分满足 | 空板块局部空态；单泳道可focus/返回；重复注记不重复计真实section；不在本板块的topic过滤/拒绝；部分section失效仅降级该快照 | PG/组件 |
| 时间窗口：两端/空/跨窗/归一化 | 沿来源日报日期边界；无确认背景保留；换窗不换owner；同日顺序稳定，key不改变坐标或日数 | 函数/组件 |
| 幂等：重复/失败重试/并发 | 重选同板块不写业务；失败恢复且可重试；owner+generation隔离迟到读写，不声明任何无测试的线程安全 | composable/组件/opencli |
| 可用性：误输入/空/错/加载/超长/重复提交 | 无输入误填不适用；空/错/加载均有本owner说明；加载禁未知写；长标题/固定标签不遮目标；条目在途互斥，双主题文字可读 | 组件/opencli |

继承与调整：TagsPage.test.ts现有版块切换/组合入口、BoardThreadBrowser.focus.test.ts的focus/返回/拖拽/空态断言保留，补注记层，不删旧契约；新增展示源锚在本Requirement，不以jsdom模拟色值冒充真实主题验收。原型computed-style检查只验证原型，产品主题与完整故事须静态:5100 opencli或显式人工豁免。

效果核对：本故事是确定性版块隔离/主题复用，不依赖LLM（真实材料效果沿S2）；用版块成员ID集合、owner、源字段前后快照及真实浏览器computed style断言，不能凭截图相似宣称完成产品。

### FB-4 真实阅读器与多线索补充（S1/S4）

- 事实基线：现行全屏reader的TopicGroup.sections→section.threads，标题/摘要/相关篇数按thread呈现；旧简化原型148断言不验证此形状。原型须改后另做机械验收，不宣称本轮已过新用例。
- 输入/前置：刻意令section.cluster_label、规范topic label、thread A/B标题不同；A有2篇、B有1篇，共享一section ID。仅A变confirmed，B不联动；marker source ID取A.thread.id而非section.id，快照来源A。无归属仍按原400/说明，不自动建题。
- 空/单/多/重复/部分满足：空threads保留原空态（确认快照另读）；单条可选；同section不同事实的N条独立；完全相同事实的重复源沿原source_key幂等；部分文章失效保留可用依据并如实提示，不逐篇标、不认证真实性。
- 时间/重建：同topic同日可有多个section，每章多thread；只按确切来源重附，不靠同section/相似标题迁移。取消A不取消B或改历史报告；失效A快照沿已有维护入口。
- 可用性/允许性：新增按钮与原header同级；点击标记不展开、点击篇数不标记、兄弟展开状态不变；无文章header.disabled不误禁有效标记；保留fit_distance警告、t:thread.id页边注与文本选择。长线索/多依据、loading/error/重复写按原五组。
- 白盒：按thread/marker索引而非sectionHasKey；不同thread同section复合分支、多marker同section的总览独立注记、来源为空/正常/失效、0/1/N文章。纯投影/索引用unit、允许性/锚/动作用组件、SQL身份用PG testcontainer；完整现有reader操作用静态:5100 opencli，不以截图或单次模拟代替。
- 效果：此处确定性层级无需新增LLM量化；S2真材料要求保留，标记不改变既有推荐频次/锚定/生命周期。

### FB-3 跨入口补充（S1/S4，共享五组变体沿原表）

- 空/单/重复建议、非法/失效来源、仅取消过的节点：日报如实显示其有效态；总览都不产生未确认注记，既有section/计数不变。
- 无输入时的空白/大小写变体不适用；日期/thread导航必须取服务端来源，不允许客户端造标题/越owner/伪造失效快照的日报锚。
- 日报来源日期窗两端/历史窗可选；总览查询默认只confirmed；非confirmed query参数按契约400。测试落点key_event_handler_test.go（真实SQL用PG testcontainer）。
- 缓存已有suggested→总览只投影普通来源；日报确认→总览更新confirmed；取消→原来源保留；失效快照取消→注记消失、不伪造section。
- 切入口或切日报日期的迟到读取/合法在途写按owner+generation隔离；旧写不抢新视图；未知状态/外部失败保留新闻、禁未知写，不触发AI重试。
- 最便宜层：模式/投影用unit/组件，确认写库用PG/handler；完整“总览普通新闻→来源日报挑选→回总览高亮→刷新→取消”须产品静态:5100 opencli；原型不代替。
- 效果核对：此处是确定性入口分工，不需要新LLM评测；S2真实材料推荐效果要求保留。总览text/aria/按钮负断言与原section集合/颜色/坐标正断言同时存在，不能只断言节点数。

### S6 白盒补充

| 分支/边界 | 断言 | 落点 |
| --- | --- | --- |
| selectedBoardId=null / 有效ID / 外部topic | null不挂全局轴，有效仅本owner，外部引用不挂入 | TagsPage.test.ts、useTopicKeyEvents.test.ts |
| currentOwner/currentGeneration ≠ 请求快照 | 旧读取不抢内容，合法人工写只保存发起owner | useTopicKeyEvents.test.ts |
| source section / detached marker | section身份/状态/标题不变；detached无假section ID/原文 | key_event_markers_pg_test.go、BoardThreadBrowser.focus.test.ts |
| editorial / dark / 767/768边界 | 语义Token来自唯一源，强调不替代状态色，固定标签不挡选中目标 | 组件/opencli |

## 白盒附加（复杂档，全故事共享）

| 分支 | 判据与边界 | 断言 |
| --- | --- | --- |
| suggested → confirmed/dismissed | 只有显式人工动作 | AI无法转confirmed |
| none/dismissed → confirmed | 有有效来源、无AI门槛 | 手动成功；来源失效拒绝新mark |
| confirmed → dismissed | 允许来源失效、同owner | 可取消，不删快照/新闻/旧报告 |
| 重复目标/旧revision不同目标 | 同目标返回原状态；不同目标409 | DB单行，客户端重读 |
| AI输出合法/部分非法/全非法/超时 | 逐项校验、失败非阻断 | 合法建议保存；失败不改日报终态 |
| 任务token同/不同、人工先/AI先 | 保存前复验token、只更新suggested | 过期不写，人工主权保持 |
| 重建指纹完全相同/改变 | 只有完全相同来源可重附 | 相似title不能移植确认 |
| SQL事务失败/重启 | 选择先落库、FK不级联注记 | 无半脱钩、刷新可恢复 |
| focus sections空/markers非空 | 两种数据独立 | 不被section空态隐藏 |
| owner错/事实窗外/无切片 | 过滤/gap/404按层 | 不绕过证据白名单 |
| 长度/数量边界 | 候选0/1/40/41；历史19/20/21；文字11999/12000/12001；超时前后 | 有界可追溯、超限说明、不捏造截取事实 |
| 日期边界 | 上海日00:00、窗口下界、To前后、cutoff当日与未来、闰日 | 日归一一致，半开边界不漏/不跨 |

不适用划除：不做外部网页/工具循环，故联网搜索失败与工具结果截断不属推荐故事；不新增自由文本表单，输入变体主要覆盖API枚举与AI理由；不实现跨泳道事件合并，不以同名话题归并为测项。视觉截图不委派子代理（本次只做原型，用户未要求独立视觉评审），采用浏览器机械断言与父线程截图检查；实现阶段按规范安排视觉复核。
