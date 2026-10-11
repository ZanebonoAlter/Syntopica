# add-topic-key-event-markers 实现任务

本清单尚未执行：propose 制品已完成，前版简化原型验证已执行148条；FB-4实查后真实reader/一章多线索原型待修订，不是当前批准基线。产品代码、迁移、真实AI效果和产品验收均待用户另行启动 apply。新长轴原型属于major，审批pending；须先把原型对齐真实线索载体，再由用户明确确认并更新approval才允许apply。参考 design.md D1-D6、ui-design.md、test-cases.md S1-S6。下列新增测试文件是实现落点，不是已经存在/通过的测试。

## 1. 持久化与人工动作

- [ ] 1.1 新增注记模型、幂等迁移、状态CHECK/来源唯一约束/可空来源FK，确认快照不随重建删除；新增 `backend-go/internal/topicgraph/repository/key_event_markers_pg_test.go`，在一次性PG覆盖确认/不采纳/取消、重建与回滚、topic删除/归档/合并分裂后归属边界，不碰真库实验。
- [ ] 1.2 实现服务端来源指纹、源数据快照、精确重附及作用域校验；同指纹重复只一行、相似内容不得移植确认；上述PG文件断言同日/跨日/证据变更与失效回看。
- [ ] 1.3 实现按来源/注记动作API与查询接口、revision冲突和幂等响应；新增 `backend-go/internal/topicgraph/handler/key_event_handler_test.go`，验证400/404/409、跨域、伪造正文、来源失效取消、人工动作零AI/研究调用。
- [ ] 1.4 扩展日报查询与lifeline可选节点数据，保持旧fields、范围、relations及topic状态；在 `daily_report_repository_test.go`、`daily_report_topic_lineage_test.go` 保留旧场景，新增PG文件校验FK与确认快照。

## 2. 默认推荐与信号关注

- [ ] 2.1 按D3在日报成功终态后安排单次有界推荐，增加独立Operation/状态及过期token防护，确保失败不改日报结果/不发第二终态；新增 `backend-go/internal/topicgraph/service/key_event_recommendation_test.go`，扩展 `handler/daily_report_notification_test.go` 对应失败/迟到/空候选路径。
- [ ] 2.2 实现推荐输入事实/已处理记录对照、合法ID及理由/依据校验、完全重复过滤，AI不写人工状态；上述service测试覆盖test-cases白盒候选/数量/文字预算/超时边界及重要进展、转载fixture。
- [ ] 2.3 在信号发现材料纳入同owner合窗confirmed关注信息，保留cutoff、有效新闻依据和回顾标签，失效切片记gap；扩展 `backend-go/internal/dataenrichment/service/signal_material_test.go`、`signal_detect_test.go` 验证未确认/跨域/窗外剔除与旧payload兼容。
- [ ] 2.4 冻结关注字段到新批次，不改旧候选/结果，不改变七源工具预算或启动规则；扩展 `signal_research_test.go` 及 `repository/signal_report_test.go` 对比后续撤销前后旧快照不变。

## 3. 现有条目交互

- [ ] 3.1 扩展API类型/normalizer并新增 `useTopicKeyEvents`；创建 `front/app/features/tags/composables/useTopicKeyEvents.test.ts`，覆盖每条请求锁、revision、乐观待保存/回滚、未知结果重读、跨入口缓存同步及迟到响应。
- [ ] 3.2 新增轻量无API副作用的 `KeyEventMarkerActions.vue` 与同目录 `.test.ts`，按宿主review/history显式模式：日报显示AI建议/理由与确认/不采纳/手标/取消，总览只显示已确认与取消维护，未确认按原新闻、不渲染建议和新挑选按钮；组件测试断言每种可见/禁用状态、来源语义、键盘操作与长文字，零嵌套按钮。
- [ ] 3.3 日报当日/历史thread接入完整挑选，BoardThreadBrowser仅confirmed注记回看/取消及有效来源日报导航；总览共享缓存的suggested不得泄漏，保留文章展开、跑题警告与页边注锚；保留真实全屏reader/TopicGroup→section→thread结构，在article.drm-thread内以同级动作接入；用同section两条不同thread+单thread多篇依据验证只变被选者、无父级/兄弟联动、t:thread.id锚原文不变。扩展 `DailyReportTopicSection.test.ts`、`useDailyReportReader.test.ts`，点击标记不展开文章，无文章展开能力不误禁有效标记。
- [ ] 3.4 在当前selectedBoardId的原lanes/focus内改善长轴可读性与关键高亮：标题/同日错排/触摸入口/日期定位，突出开关保留普通背景且坐标不变；点击节点进入原位详情，来源失效快照作为独立注记覆盖，不制造section/关系；扩展 `BoardThreadBrowser.focus.test.ts` 覆盖背景坐标、同日标题、section空但markers非空、跨泳道、取消后背景、键盘/拖拽及375px局部滚动。
- [ ] 3.5 保持TagsPage选版块→BoardThreadBrowser.boardId→lanes/focus的边界，全部仍是文章池；扩展 `TagsPage.test.ts`/`BoardThreadBrowser.focus.test.ts` 的版块切换与迟到响应、focus返回、tag_count/section身份/原标题不变用例。复用main.css两主题及五态statusFill；人工：静态:5100 opencli逐状态检查外环不覆盖原状态色、标题与真实计数，双主题/窄屏差异记录ui-acceptance/report.md。不移植原型整页；新增图标仅执行 `cd front && pnpm generate:icons`。

## 4. 测试

- [ ] 4.1 按 test-cases.md 完成49个Scenario主链路与五组变体，执行 `bash scripts/harness/test-assets.sh daily-report-system`、`persistent-topic`、`board-signal-reports` 并复核继承表；新增测试文件存在且用例逐项断言，不以文件存在代表测试通过。
- [ ] 4.2 `bash scripts/harness/change-scope.sh` 机械确认影响包；串行执行 `cd backend-go && GOMAXPROCS=2 go test -p 1 ./internal/topicgraph/repository ./internal/topicgraph/service ./internal/topicgraph/handler ./internal/dataenrichment/service ./internal/dataenrichment/repository ./internal/dataenrichment/handler ./internal/platform/database` → 影响包测试通过，PG层用testcontainer而非SQLite；若实际scope变化则更新本清单，不顺手全量go test。
- [ ] 4.3 `cd front && pnpm test:unit app/features/tags/components/daily-report/DailyReportTopicSection.test.ts app/features/tags/components/BoardThreadBrowser.focus.test.ts app/features/tags/components/KeyEventMarkerActions.test.ts app/features/tags/composables/useTopicKeyEvents.test.ts app/features/tags/composables/useDailyReportReader.test.ts --maxWorkers=2` → 受影响组件/状态测试通过，参数不加额外 `--`。
- [ ] 4.4 人工：在静态托管 :5100 使用opencli走 ui-design.md 主故事（在日报确认→本版块总览无AI建议且已确认高亮→突出模式保持背景→点击原位详情→刷新→取消→背景仍在）及无建议手标、失败回滚、键盘、双主题和375px路径，记录动作后状态断言至 ui-acceptance/report.md；原型不能代替此产品全链路。
- [ ] 4.5 人工：按 test-cases.md S2 对至少20组真实材料做受控模型效果核对，量化重复误荐/进展漏荐/非法引用/成本，保存 ai_call_logs session 与逐例结论至 effect-check.md；不重建真日报或批量写正式选择，效果欠佳先定位再由用户决定。

## 5. 文档

<!-- doc-impact: flow, api, database -->

规划期已跑doc-impact suggest，当前仅change制品、未命中产品代码，所以未自动预勾；根据计划涉及的新API/注记表/三域行为明确选择上述影响域，apply启动重跑suggest+context并对账，不采用自动输出的none。

- [ ] 5.1 更新 `docs/reference/flow/daily-report.md`、`docs/reference/flow/topic-graph.md`、`docs/reference/flow/data-enrichment.md` 的链路及人工主权/重建保留/关注非结论边界；归档后按§12.2各补archive永久链接，人工：链接可打开。
- [ ] 5.2 更新 `docs/reference/api/` 中日报/话题接口对应文档，补读写端点、400/404/409、optional字段及禁伪造来源；人工：curl/handler响应与文档字段逐项一致。
- [ ] 5.3 在 `docs/reference/database/tables/` 登记注记表、状态/唯一/FK删除语义及注册索引；先读 doc-authoring.md，再修改实际文档。人工：与一次性PG迁移测试约束一致。
- [ ] 5.4 人工：部署影响说明与用户操作提示跟随交付；旧数据默认无建议、不回刷、不自动研究，迁移无需用户手动清理；更新reference反映实际实现而非提前写“已上线”。

## 6. 验证

以下是实现后门禁，不是本次已执行结果。新增测试路径须在实现期创建；scenario-trace、doc-impact、review-trace只在实际实现与文档更新后才应全绿。

- [ ] 6.1 `openspec validate add-topic-key-event-markers --strict` → delta规格校验退出码0。
- [ ] 6.2 `cd backend-go && golangci-lint run ./... && go vet ./internal/topicgraph/... ./internal/dataenrichment/... ./internal/platform/database && go build ./...` → lint/vet/build零错误；不和前端build/浏览器并行。
- [ ] 6.3 `cd front && pnpm lint && pnpm exec nuxi typecheck && pnpm build` → lint/typecheck/build零错误；单测先串行执行4.3，不与build并行。
- [ ] 6.4 `bash scripts/harness/scenario-trace.sh openspec/changes/add-topic-key-event-markers` → 49个Scenario全部映射且文件真实存在。
- [ ] 6.5 `bash scripts/harness/doc-impact.sh verify openspec/changes/add-topic-key-event-markers && bash scripts/harness/check-standards.sh --change add-topic-key-event-markers` → 声明文档更新、标准与链接对账退出码0。
- [ ] 6.6 人工：完成聚焦review并按规范将发现/处置记录在review.md；再执行 `bash scripts/harness/review-trace.sh openspec/changes/add-topic-key-event-markers` → 审查痕迹对账退出码0，不在规划期虚构审查。
- [ ] 6.7 `bash scripts/harness/test-patrol.sh --report` → 确认欠账；非本change红按规范register留context，本change红先修，不以“别人的问题”跳过登记。
- [ ] 6.8 `bash scripts/dev/deploy-frontend.sh` → 实现验收通过后构建静态产物、铺盘、重启后端与健康检查成功；本次只propose/原型，不执行部署。

### Scenario → 测试文件映射

下列路径均为仓库根相对路径。新增key_event/KeyEventMarker/useTopicKeyEvents测试仍待实现；归档前须真实存在并通过。完整交互故事另有4.4的opencli人工留痕落点。

| Scenario | 测试文件 |
| --- | --- |
| 一章内标记不连带其他线索 | front/app/features/tags/components/daily-report/DailyReportTopicSection.test.ts |
| 一线索多篇依据只标一条 | backend-go/internal/topicgraph/repository/key_event_markers_pg_test.go |
| 无建议也能手动标记 | backend-go/internal/topicgraph/handler/key_event_handler_test.go |
| 建议必须确认才生效 | front/app/features/tags/components/daily-report/DailyReportTopicSection.test.ts |
| 不采纳后仍可选择 | front/app/features/tags/components/daily-report/DailyReportTopicSection.test.ts |
| 取消不删除新闻或报告 | backend-go/internal/topicgraph/repository/key_event_markers_pg_test.go |
| 转载不当新进展 | backend-go/internal/topicgraph/service/key_event_recommendation_test.go |
| 新的实质进展可再推荐 | backend-go/internal/topicgraph/service/key_event_recommendation_test.go |
| 无依据或跨泳道建议丢弃 | backend-go/internal/topicgraph/service/key_event_recommendation_test.go |
| 推荐失败可手动选择 | backend-go/internal/topicgraph/service/key_event_recommendation_test.go |
| 重复确认只保留一个节点 | backend-go/internal/topicgraph/repository/key_event_markers_pg_test.go |
| 迟到推荐不覆盖人工决定 | backend-go/internal/topicgraph/repository/key_event_markers_pg_test.go |
| 写入失败回到原状态 | front/app/features/tags/composables/useTopicKeyEvents.test.ts |
| 越界引用拒绝 | backend-go/internal/topicgraph/handler/key_event_handler_test.go |
| 无归属及伪造快照拒绝 | backend-go/internal/topicgraph/handler/key_event_handler_test.go |
| 展示不重新编写新闻 | front/app/features/tags/components/KeyEventMarkerActions.test.ts |
| 来源失效仍可回看 | front/app/features/tags/components/BoardThreadBrowser.focus.test.ts |
| 标记不误展开文章 | front/app/features/tags/components/daily-report/DailyReportTopicSection.test.ts |
| 未知状态与重复提交反馈 | front/app/features/tags/components/KeyEventMarkerActions.test.ts |
| 建议只在日报展示 | front/app/features/tags/components/BoardThreadBrowser.focus.test.ts |
| 总览不承担新选择 | front/app/features/tags/components/KeyEventMarkerActions.test.ts |
| 保存日报和分组 | backend-go/internal/topicgraph/repository/daily_report_repository_test.go |
| 查询最近 7 天日报 | backend-go/internal/topicgraph/repository/daily_report_repository_test.go |
| 查询超过 30 天日报 | backend-go/internal/topicgraph/repository/daily_report_repository_test.go |
| 非正数天数使用默认值 | backend-go/internal/topicgraph/repository/daily_report_repository_test.go |
| 重生成保留选择而不移植确认 | backend-go/internal/topicgraph/repository/key_event_markers_pg_test.go |
| 旧日报可直接手动标记 | front/app/features/tags/components/daily-report/DailyReportTopicSection.test.ts |
| 推荐失败不改变日报终态 | backend-go/internal/topicgraph/handler/daily_report_notification_test.go |
| 查询话题完整生命线 | backend-go/internal/topicgraph/repository/daily_report_topic_lineage_test.go |
| 话题跨命名漂移聚合 | backend-go/internal/topicgraph/repository/daily_report_topic_lineage_test.go |
| 无 section 仍能查确认快照 | backend-go/internal/topicgraph/repository/key_event_markers_pg_test.go |
| 高亮保留背景但不混入他泳道 | front/app/features/tags/components/BoardThreadBrowser.focus.test.ts |
| 取消最后节点仍保留时间背景 | front/app/features/tags/components/BoardThreadBrowser.focus.test.ts |
| 切换泳道不串状态 | front/app/features/tags/composables/useTopicKeyEvents.test.ts |
| 点击节点读详情但不标记 | front/app/features/tags/components/BoardThreadBrowser.focus.test.ts |
| 同日标题错开且长文本可读 | front/app/features/tags/components/BoardThreadBrowser.focus.test.ts |
| 窄屏长轴局部滚动 | front/app/features/tags/components/BoardThreadBrowser.focus.test.ts |
| 总览只展示选中版块泳道 | front/app/features/tags/components/TagsPage.test.ts |
| 切换版块迟到结果不串入 | front/app/features/tags/composables/useTopicKeyEvents.test.ts |
| 双主题高亮不覆盖原状态语义 | front/app/features/tags/components/BoardThreadBrowser.focus.test.ts |
| 全部入口不汇总所有泳道 | front/app/features/tags/components/TagsPage.test.ts |
| 周期校验 | backend-go/internal/dataenrichment/service/signal_material_test.go |
| 历史材料隔离 | backend-go/internal/dataenrichment/service/signal_material_test.go |
| 迟些点击仍用原快照 | backend-go/internal/dataenrichment/service/signal_research_test.go |
| 外部数据边界 | backend-go/internal/dataenrichment/service/signal_research_test.go |
| 只冻结合窗人工关注 | backend-go/internal/dataenrichment/service/signal_material_test.go |
| 后续撤销不改旧研究 | backend-go/internal/dataenrichment/service/signal_research_test.go |
| 失效依据不升级为有效信号 | backend-go/internal/dataenrichment/service/signal_detect_test.go |
| 事后选择不冒充当时预测 | backend-go/internal/dataenrichment/service/signal_material_test.go |
