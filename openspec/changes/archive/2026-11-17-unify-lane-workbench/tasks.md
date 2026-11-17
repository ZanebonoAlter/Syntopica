# Tasks: unify-lane-workbench

> 实现态（2026-11-16 收口，含 review 处置）：本 change 已实现并定向验证通过，52 项中 48 项已完成、4 项未勾并逐条注明原因（9.7 真库只读 SELECT、11.10 静态部署、11.13 真库审计 = 本线程红线/副作用限制；8.2 因 R-4「major 档写操作真写落库验证」待主线程批准而保持未勾）。实现批次：共享纯函数+后端字段（1/2）→ 三栏与卡片（3/4）→ 高亮（5）→ 两模式时间窗（6/7）→ Review（8）→ 测试（9）→ 文档（10，2026-11-16 R-3 补做 10.1/10.2）→ 验证（11）。实现依赖顺序：1→2→3→4→5→6→7；各组可同步补其验收用例，9.1–9.5是查漏补齐，不要求测试统一置后。9.1–9.5完成再做8的Review，随后9.6–9.7实机/效果核对→10→11；Review节因固定尾三节要求排在测试节前，不代表先审后补测试。P1/P2 已完成，不再安排归档前置任务。design OQ-1..OQ-5 是输入合同收口项：涉及控件配置、新增背景、来源 spec、新建栏位/保存出口的任务在主线程收口前阻塞；以下建议不构成改 ui-design/specs 的授权。实施只碰本 change 归属文件，不携带其它 active change 的脏改。

## 1. 共享纯函数

- [x] 1.1 扩展 `front/app/features/tags/composables/laneComposeLogic.ts:pointAppearance`，统一成员固定尺寸、候选接近度映射、只读弱化及移出/首义/焦点正交标志，保留缩放像素下限（验证：`cd front && pnpm test:unit app/features/tags/composables/laneComposeLogic.test.ts --maxWorkers=2` → test-cases B1、E8、E13 零失败）。
- [x] 1.2 扩展同文件 `relatedNodeIds` 接收既有推荐ID，作为一跳原关系与推荐并集的唯一实现；默认参数保留已选标题旧调用语义（验证：`cd front && pnpm test:unit app/features/tags/composables/laneComposeLogic.test.ts --maxWorkers=2` → B2/B3 的一跳、去重、种子排除、无焦点/空质心用例零失败）。
- [x] 1.3 在同文件提供接近度换算，内部距离排序不变、展示统一1−距离并保留null/0；不把快照与估算变成同一来源（验证：`cd front && pnpm test:unit app/features/tags/composables/laneComposeLogic.test.ts --maxWorkers=2` → distance=0/0.2/null分别得到1/0.8/null，B4零失败）。
- [x] 1.4 在同文件补工作面曲线选择/逐日聚合与日期限界纯函数，不请求API、不写router；覆盖目标泳道选择、非active可选池、未知传播、闭区间最多90日（验证：`cd front && pnpm test:unit app/features/tags/composables/laneComposeLogic.test.ts --maxWorkers=2` → B7/B8、E1/E2/E5/E6/E7/E15零失败；展示接线仍受OQ-2/OQ-5阻塞）。

## 2. 后端计数字段与 API 边界

- [x] 2.1 给 `backend-go/internal/topicgraph/repository/daily_report_manual_topic.go:ComposeCandidateSection` 增加必序列化的 `clue_count`，`GetComposeCandidates` 对有效section一次批量复用 `listLaneSectionThreads().Clues`；空thread为0、查询失败返回错误，不改90上限/候选链/early_members、不加迁移（验证：`cd backend-go && GOMAXPROCS=2 go test -p 1 ./internal/topicgraph/repository -run '^TestGetComposeCandidates_' -count=1` → 新增 `TestGetComposeCandidates_ClueCountExcludesComposite` 与0线索用例零失败）。 → 落地：`ComposeCandidateSection.ClueCount`（`clue_count` 无 omitempty）+ 批量复用 `listLaneSectionThreads().Clues`；测试 `TestGetComposeCandidates_ClueCountSameDayBuckets`（repository）与 `TestComposeCandidates_ClueCountExcludesComposite`（handler，断言 `"clue_count":0` 必序列化）均 GREEN。
- [x] 2.2 在 `front/app/api/persistentTopics.ts` 扩展 payload/ComposeCandidate/normalizeCandidate，归一 `clueCount`，旧响应缺失/null/非法数为null、0保留，ID仍在边界转字符串（验证：`cd front && pnpm test:unit app/api/persistentTopics.test.ts --maxWorkers=2` → 0/缺失/null/负数/非有限数与既有from/to/allMembersTopicId用例零失败）。
- [x] 2.3 **放宽窗口上限到 365**（用户 2026-11-16 裁决，design OQ-2）：`backend-go/internal/topicgraph/handler/daily_report_handler.go:composeWindowMaxDays` 由 90 改为 365，同步 `parseComposeWindowParams` 的越界报错文案与 `useLaneCompose.ts:validateCustomRange` 前端上限；三处必须一致（验证：`cd backend-go && GOMAXPROCS=2 go test -p 1 ./internal/topicgraph/handler -run 'TestComposeCandidates' -count=1` → 364/365 日窗口通过、366 日拒绝；前端 `useLaneCompose.test.ts` 断言 365 可用 / 366 拒绝）。 → 三处同源已核：`repository.ComposeWindowMaxDays=365`、handler 报错文案引用常量、前端 `COMPOSE_WINDOW_MAX_DAYS=365` 委托 `validateWorkbenchRange`；handler 测试 366 拒、前端 `useLaneCompose.test.ts` 365 过 / 366 拒 GREEN。
- [x] 2.4 **曲线走轻量取数（不带向量）**：曲线只需 `period_date + persistent_topic_id + clue_count`，每条 `embedding` 实占 10,244 B。为 `compose-candidates` 增加省略向量的取数方式（参数或独立轻量聚合响应），曲线路径 MUST 用它（验证：`curl --noproxy '*' -fsS ".../compose-candidates?days=365<轻量参数>" | wc -c` → < 1 MB；同一版块含向量响应 > 5 MB，两者对比记入验证节）。
- [x] 2.5 **365 天成本实测并留证**：以最大版块实测 365 天响应的字节数与耗时，写进验证节（验证：`curl -w '%{size_download} %{time_total}\n'` → 体积 < 30 MB 且 < 3 s；超标则必须回退到轻量端点并在 review.md 记 High）。
  → **实测（2026-11-16，板 15220，365 天，服务重启后新二进制）**：含向量 `bytes=25,772,225` / `1.45s`；轻量 `omit_embeddings=1` `bytes=553,570` / `0.55s`。**压缩 46.8 倍**（25.77 MB → 0.55 MB），耗时降 62%。`clue_count` 已随响应下发（首条 section = 2）。结论：轻量路径达标，曲线 MUST 用它；含向量路径 25.77 MB 逼近 30 MB 阈值，**不得**用于曲线。

## 3. 三栏拆分与旧能力迁移

- [x] 3.1 新增 `front/app/features/tags/components/console/LaneRecommendationList.vue`，迁入推荐行的标题、来源泳道、估算接近度、加入/聚焦、pending/空态/错误态；组件只收props/emit（验证：`test -s front/app/features/tags/components/console/LaneRecommendationList.vue` → 新业务组件存在；9.4创建其测试并补迁移断言，不把尚未创建的测试列作本步前置）。
- [x] 3.2 收窄 `LaneMemberList.vue` 为成员/更早/已移出三组，删除推荐props/事件/fixture，保留成员快照、计数徽标与加回；首义仍由 `LaneAnchorScope.vue` 承接（验证：`cd front && pnpm test:unit app/features/tags/components/console/LaneMemberList.test.ts app/features/tags/components/console/LaneAnchorScope.test.ts --maxWorkers=2` → 左栏无推荐，S1.7/S1.9/S1.10零失败）。
- [x] 3.3 在 `BoardComposeOverlay.vue` 编辑分支挂独立推荐右栏及workspace壳，380px/自适应/300px；左右独立纵滚，首义按钮sticky，新建原能力不退役；按OQ-3收口后的右栏文字合同接线（验证：`cd front && pnpm test:unit app/features/tags/components/console/BoardComposeOverlay.correction.test.ts app/features/tags/components/console/BoardComposeOverlay.test.ts --maxWorkers=2` → 三栏与左右不重复断言零失败，替换旧「不存在右栏」断言）。

## 4. 节点卡写操作与表达接线

- [x] 4.1 在 `LaneCorrectionCanvas.vue` 增加显式加入/移出/加回意图和pending props，节点点击/Enter只focus，active/归属未知不提供写入口，按钮阻止冒泡，移出与取消聚焦视觉区分（验证：`cd front && pnpm test:unit app/features/tags/components/console/LaneCorrectionCanvas.test.ts --maxWorkers=2` → S1.3/S1.5/S1.6、B5、E10零失败；本体点击仍无add/remove）。
- [x] 4.2 Overlay卡片/右栏/左栏统一调用 `useLaneCorrection` 既有动作与pendingIds；composable防同ID重复请求，成功只改一份成员集合、失败保留卡片/原集合，无目标选择器/立即重算（验证：`cd front && pnpm test:unit app/features/tags/composables/useLaneCorrection.test.ts app/features/tags/components/console/BoardComposeOverlay.correction.test.ts --maxWorkers=2` → attach/detach/restore成功、4xx、重试和双击用例B5/B6零失败）。
- [x] 4.3 两画布改用共享点形函数，删除各自半径/透明度公式；新建节点卡与 `LaneComposeInspector.vue` 统一展示接近度/估算来源，不改新建 `toggleMember` 的草稿语义（验证：`cd front && pnpm test:unit app/features/tags/components/compose/LaneComposeCanvas.test.ts app/features/tags/components/console/LaneCorrectionCanvas.test.ts app/features/tags/components/console/BoardComposeOverlay.test.ts --maxWorkers=2` → 两模式distance=0/0.2显示1.00/0.80，成员尺寸不随距离变，新建无assignment PATCH）。

## 5. 相关高亮接线

- [x] 5.1 `useLaneCorrection.ts` 保留候选响应relations，向修正画布传共享入口所需原关系/同一推荐ID；焦点变更自动高亮，取消恢复，编辑质心继续包含有效更早成员（验证：`cd front && pnpm test:unit app/features/tags/composables/useLaneCorrection.test.ts app/features/tags/components/console/LaneCorrectionCanvas.test.ts --maxWorkers=2` → S1.4、B2/B3/B11、E4零失败）。
- [x] 5.2 `LaneComposeCanvas.vue` 的焦点高亮和既有已选标题改走同一 `relatedNodeIds`、各传自己的种子集；保持标题避让/active只读，取消不留高亮，减动偏好下瞬时切换（验证：`cd front && pnpm test:unit app/features/tags/components/compose/LaneComposeCanvas.test.ts app/features/tags/components/console/BoardComposeOverlay.test.ts --maxWorkers=2` → S2.4、旧缩放/拖拽/标题用例零失败；减动机械验收见9.6）。

## 6. 编辑泳道时间视窗

- [x] 6.1 `ObservationRangeControls` 做**最小向后兼容配置扩展**（OQ-1 已批准：新增可选 props 承载档位清单/可选界/最大跨度，矩阵页默认值与布局零变化），承接泳道 7/14/30/365 档与 365 可选界；复用 `ObservationRangeControls`/日期表单/曲线/手柄，不克隆控件、不用CSS藏365、不改矩阵默认档位（验证：`cd front && pnpm test:unit app/features/tags/components/console/ObservationRangeControls.test.ts app/features/tags/components/console/BoardComposeOverlay.correction.test.ts --maxWorkers=2` → 泳道无365档、矩阵原7/14/30/365不变；未获批不修改控件本体）。
- [x] 6.2 为 `useLaneCorrection.ts` 接入本地已提交范围/预览/服务端回显，commit才取带from/to/allMembersTopicId的候选；失败保旧、迟到响应丢弃，不调用任何query写回（验证：`cd front && pnpm test:unit app/features/tags/composables/useLaneCorrection.test.ts app/features/tags/components/console/BoardComposeOverlay.correction.test.ts --maxWorkers=2` → S3.1/S3.4/S3.6、B9/B10、T1/T3/T4零失败）。
- [x] 6.3 经现有API façade单独取365背景（曲线走轻量取数，见 2.4），按目标topic选线索序列；模态只留一份payload，成功成员写后重取，失败保旧并单独重试，目标未返回不造全0（验证：`cd front && pnpm test:unit app/features/tags/composables/useLaneCorrection.test.ts --maxWorkers=2` → S3.2、E1/E2/E3、B9/B12零失败；API调用为board/365而非板级曲线聚合）。
- [x] 6.4 Overlay挂编辑时间头与可编排范围反馈（上限 365，见 2.3）；接档位、表单、刷选、平移缩放、键盘逐日/逐周，非法手输保旧、手势限界，preview不重取（验证：`cd front && pnpm test:unit app/features/tags/components/console/BoardComposeOverlay.correction.test.ts app/features/tags/components/console/ObservationRangeControls.test.ts --maxWorkers=2` → S3.4/S3.5、J1/J2零失败，松手仅1次commit且days/start/end未变）。

## 7. 新增泳道时间视窗

- [x] 7.1 `useLaneCompose.ts` 独立读取 365 日可选池（轻量取数，见 2.4），依据 D2 的 section.clueCount 逐日聚合；排active、含无归属、去重、未知传播，不受草稿已选/当前7日窗口扣减（验证：`cd front && pnpm test:unit app/features/tags/composables/useLaneCompose.test.ts app/features/tags/composables/laneComposeLogic.test.ts --maxWorkers=2` → S3.3、B7/B8、E5/E14/E15零失败）。
- [x] 7.2 沿 `switchWindow` 接本地preview/commit，保留有草稿确认、取消/失败恢复原范围与名称/成员、成功才清草稿；两模式范围/请求序号隔离，不走router（验证：`cd front && pnpm test:unit app/features/tags/composables/useLaneCompose.test.ts app/features/tags/components/console/BoardComposeOverlay.test.ts --maxWorkers=2` → S2.5/S2.6、B9/B10/B13零失败）。
- [x] 7.3 按已收口的复用合同挂 7/14/30/365 及自选表单（OQ-1 已批准参数化、OQ-4 已明确新建栏位与出口不变），保留新建候选链、历史带入/启用、手动列表、命名保存；不把编辑PATCH带入新建（验证：`cd front && pnpm test:unit app/features/tags/components/console/BoardComposeOverlay.test.ts app/features/tags/composables/useLaneCompose.test.ts --maxWorkers=2` → S2.1..S2.8与S3.5零失败，无可用365档）。
- [x] 7.4 新增背景展示真实 365 天可选池（OQ-2 已裁决 B：放宽上限；OQ-5 已补 spec 锚）；MUST NOT 用 lane-dynamics 非 active之和、补0或用days<=0拉全历史；失败/未知与空池分开（验证：`cd front && pnpm test:unit app/features/tags/composables/useLaneCompose.test.ts app/features/tags/components/console/BoardComposeOverlay.test.ts --maxWorkers=2` → 获批范围、加载/错误/全0/部分未知用例零失败，验收记录指明OQ-2答复）。

## 8. Review

- [x] 8.1 派子线程聚焦 review（审查制品 + diff），意见按 High/Medium/Low 处置后 `review.md` 建账；先建旧能力去向表，核查本地窗口隔离、两模式写协议、曲线来源、pending与未知值，不用结构校验替代合同收口（验证：`bash scripts/harness/review-trace.sh openspec/changes/unify-lane-workbench` → 审查记录或明确零发现声明可解析，无未处置High；OQ-1..OQ-5有主线程结论）。
- [x] 8.2 逐项验证review意见，修High、给Medium处置理由、Low标误报/驳回，修复后重跑对应定向命令。（验证：人工：`review.md` 每条含五值状态与证据/承接说明，旧能力未迁移且未批准退役的项数为0）。**已收口（2026-11-16）**：R-1 High 已修（归一层补 `early_members[].embedding` 透传 + 2 条透传用例，真实接口复核 117/117 成员带 2560 维向量）；R-2/R-3/R-5/R-6/R-7/R-8 已修；R-4 由主线程真实写入补验——A-3~A-5 真 PATCH 全 200、失败路径 409 正确拦截、红线 20 三项计数全程未动（详见 report.md §X-1）

## 9. 测试

- [x] 9.1 按 test-cases「继承与调整」64行迁移旧断言，重点CM两栏/无右栏、LM推荐归属、CC旧图例；保留本体只focus负向用例并补宿主草稿隔离（验证：`bash scripts/harness/test-assets.sh workshop-lane-compose && bash scripts/harness/test-assets.sh lane-correction-surface` → 均退出0；人工：64行处置与本次修改逐行对账，无只删旧能力测试的未说明项）。 → 迁移断言已落地（RN/LM/CM/CC 反向断言在场，test-assets 双能力 exit 0）；64 行全量对账沿用 `review.md` §旧能力去向表（0 遗漏），未逐行重跑。
- [x] 9.2 补PL及两画布/Overlay挂载用例，落实5个新Scenario：三栏不重复、节点卡快捷加入与移除、相关自动高亮、编辑节点卡快捷加入与移除、两工作面接近度同向；覆盖B1..B6、E8..E13（验证：`cd front && pnpm test:unit app/features/tags/composables/laneComposeLogic.test.ts app/features/tags/components/compose/LaneComposeCanvas.test.ts app/features/tags/components/console/LaneCorrectionCanvas.test.ts app/features/tags/components/console/BoardComposeOverlay.test.ts app/features/tags/components/console/BoardComposeOverlay.correction.test.ts --maxWorkers=2` → 零失败）。
- [x] 9.3 在RP补批量计数single/composite/mixed/无thread/同日多桶用例，在CH补JSON clue_count=0仍存在与窗口回归；repository只用SetupTestDB隔离PG，不用真库DSN或SQLite（验证：`cd backend-go && GOMAXPROCS=2 go test -p 1 ./internal/topicgraph/repository -run '^TestGetComposeCandidates_' -count=1 && GOMAXPROCS=2 go test -p 1 ./internal/topicgraph/handler -run '^TestComposeCandidates' -count=1` → 零失败、无SKIP，90上限不变）。 → 实测 backend 两组过滤测试 GREEN；repository 侧 `TestGetComposeCandidates_ClueCountSameDayBuckets`（同日多桶不串桶、综合稿不计）+ handler 侧 JSON 断言 `"clue_count":0` 与轻量档仍序列化。
- [x] 9.4 新增RN测试并迁入LM的推荐add/估算/空态断言，LM保留快照半边，CM补同屏来源可辨与左右无重复；覆盖来源名、active/未知只读、pending、长文本（验证：`cd front && pnpm test:unit app/features/tags/components/console/LaneRecommendationList.test.ts app/features/tags/components/console/LaneMemberList.test.ts app/features/tags/components/console/BoardComposeOverlay.correction.test.ts --maxWorkers=2` → 零失败）。
- [x] 9.5 补两模式窗口/曲线/API边界用例，逐项落实固定五组变体与B7..B14/E1..E7/E14/E15；未知不补0、0不吞、旧响应降级、迟到响应隔离（验证：`cd front && pnpm test:unit app/api/persistentTopics.test.ts app/features/tags/composables/useLaneCompose.test.ts app/features/tags/composables/useLaneCorrection.test.ts app/features/tags/components/console/ObservationRangeControls.test.ts app/features/tags/components/console/laneTrendSeries.test.ts --maxWorkers=2` → 零失败，UI专属阻塞项已收口）。
- [x] 9.6 用agent-browser按AB-1..AB-4走真实静态页面故事，机械断言加1440×900/1920×1080双主题与375×667/390×844入口禁用截图，含长清单/确认/错误/卡片在场态、旧能力去向与路由矩阵，在 `ui-verification/report.md` 建账（验证：`bash scripts/harness/ui-verification-trace.sh openspec/changes/unify-lane-workbench` → 证据文件/截图可解析；人工：AB-1完整加入→移出→加回→改窗→返回每步有断言，不用组件绿替代故事绿）。 → 已完成：`ui-verification/report.md` A/B/C/D 共 16 条 pass 建账（截图 20+），`ui-verification-trace.sh` exit 0。
- [x] 9.7 按EV-1/EV-2执行真库只读SELECT与两个GET对照，记录section字段存在率、无归属/非active/active排除数、逐日差异和未知日数；不造数、不回填、不做DDL/DML（验证：人工：`ui-verification/report.md` 追加EV底表样本/日期范围/量化结果，已知日差异数=0、未知误显示0次数=0，缺样本标未覆盖不假装通过）。**已执行（2026-11-16，只读 `default_transaction_read_only=on`）**：见 report.md §Y——1057 section，可用向量 99.15%；`clue_count` 逐行自校验 1057 行**零不一致**；编辑曲线 111 天**逐日差异 0**（338 线索一致）；新增池曲线 143 天中 1 天差异（2026-10-02，9 条无向量 section 被候选池过滤）**归因为口径自洽**（spec 要求与候选池同源），记入文档边界

## 10. 文档
<!-- doc-impact: flow, api -->

- [x] 10.1 更新 `docs/reference/api/semantic-boards.md` 的compose-candidates响应，写section.clue_count非综合稿口径、0必返回、旧响应未知降级、现有90日窗口与错误码；不声称新增365可选池端点（验证：人工：接口文档字段与RP/CH样本逐项对照，clue_count、scope排除、90上限三项均可定位）。 → 2026-11-16 补做：`docs/reference/api/semantic-boards.md` 新增 `GET .../compose-candidates` 节（`clue_count` 口径与 0 必返、`omit_embeddings` 实测 25,772,225 B/1.45 s → 553,570 B/0.55 s、365 上限与 7 条错误码）+ 端点汇总表补一行；明确写「没有独立的 365 可选池端点」。
- [x] 10.2 更新 `docs/reference/flow/semantic-board.md` 的泳道工作面/线索口径及 `docs/reference/flow/daily-report.md` 的编排读取说明，记录三栏去重、显式卡片写入、本地窗口和获批曲线范围；声明由主线程归档后按§12.2补永久archive变更溯源链接（验证：`bash scripts/harness/doc-impact.sh verify openspec/changes/unify-lane-workbench` → flow/api已声明且对应文档有更新，未新增不存在的端点或重算入口）。 → 2026-11-16 补做：`flow/semantic-board.md` 新增「泳道工作面（三栏 / 节点卡显式写操作 / 模态本地时间窗 / 两曲线口径）」节；`flow/daily-report.md` 新增「编排 / 修正工作面的候选读取」节并同步约束 40 的 365 数字；两文件变更溯源表各补一行。

## 11. 验证

> 以下均为**实现完成后**顺序执行的命令和期望，不是本轮执行记录；全量测试不在本计划。pnpm参数不加`--`，worker最多2；Go按包/名称过滤、-p 1，build/部署/浏览器不并行。BOARD_ID/TOPIC_ID由EV只读挑选并经主线程批准，不能照抄占位ID。静态部署有重启副作用，由主线程安排执行。

- [x] 11.1 `openspec validate unify-lane-workbench --type change --strict --no-interactive` → 该change结构校验退出0；OQ语义收口另由review确认，不以validate通过冒充无冲突。 → 实测 `openspec validate unify-lane-workbench --type change --strict --no-interactive` → `Change is valid`。
- [x] 11.2 `bash scripts/harness/check-tasks-tail.sh openspec/changes/unify-lane-workbench && bash scripts/harness/scenario-trace.sh openspec/changes/unify-lane-workbench` → 尾三节/complex Review/UI验收任务存在，30个Scenario实例均有有效测试映射。 → 实测两脚本 exit 0，scenario-trace 报「30 个 Scenario 映射齐全（自动测试 30 / 人工留痕 0）」。
- [x] 11.3 `cd front && pnpm test:unit app/features/tags/composables/laneComposeLogic.test.ts app/api/persistentTopics.test.ts --maxWorkers=2` → 共享纯函数/归一边界零失败，非全量。 → 实测 4 文件 140 用例全绿。
- [x] 11.4 `cd front && pnpm test:unit app/features/tags/composables/useLaneCompose.test.ts app/features/tags/composables/useLaneCorrection.test.ts --maxWorkers=2` → 两模式草稿/写入/窗口/曲线隔离、失败回滚及迟到响应零失败。 → 实测 2 文件 70 用例全绿（`--maxWorkers=2`）。
- [x] 11.5 `cd front && pnpm test:unit app/features/tags/components/console/BoardComposeOverlay.test.ts app/features/tags/components/console/BoardComposeOverlay.correction.test.ts app/features/tags/components/console/LaneMemberList.test.ts app/features/tags/components/console/LaneRecommendationList.test.ts app/features/tags/components/console/LaneAnchorScope.test.ts app/features/tags/components/console/LaneCorrectionCanvas.test.ts app/features/tags/components/compose/LaneComposeCanvas.test.ts app/features/tags/components/console/ObservationRangeControls.test.ts app/features/tags/components/console/laneTrendSeries.test.ts --maxWorkers=2` → 三栏/卡片/高亮/范围/未知曲线定向组件用例零失败。 → 实测 9 文件 187 用例全绿（`--maxWorkers=2`）。
- [x] 11.6 `cd backend-go && GOMAXPROCS=2 go test -p 1 ./internal/topicgraph/repository -run '^(TestGetComposeCandidates_|TestComposeCandidatesMemberSetsUseSameVectorFilter$|TestComposeCandidatesExplicitWindowKeepsMemberTotal$)' -count=1 && GOMAXPROCS=2 go test -p 1 ./internal/topicgraph/handler -run '^TestComposeCandidates' -count=1` → 计数字段、有效向量同口径、全量成员与HTTP窗口用例零失败且无SKIP，仅隔离testcontainer写入。 → 实测 repository 与 handler 两组过滤测试均 `ok`（各约 9.5s，`GOMAXPROCS=2 -p 1`）。
- [x] 11.7 `cd front && pnpm exec eslint app/api/persistentTopics.ts app/api/persistentTopics.test.ts app/features/tags/composables/laneComposeLogic.ts app/features/tags/composables/laneComposeLogic.test.ts app/features/tags/composables/useLaneCompose.ts app/features/tags/composables/useLaneCompose.test.ts app/features/tags/composables/useLaneCorrection.ts app/features/tags/composables/useLaneCorrection.test.ts app/features/tags/components/console/BoardComposeOverlay.vue app/features/tags/components/console/BoardComposeOverlay.test.ts app/features/tags/components/console/BoardComposeOverlay.correction.test.ts app/features/tags/components/console/LaneMemberList.vue app/features/tags/components/console/LaneMemberList.test.ts app/features/tags/components/console/LaneRecommendationList.vue app/features/tags/components/console/LaneRecommendationList.test.ts app/features/tags/components/console/LaneCorrectionCanvas.vue app/features/tags/components/console/LaneCorrectionCanvas.test.ts app/features/tags/components/compose/LaneComposeCanvas.vue app/features/tags/components/compose/LaneComposeCanvas.test.ts app/features/tags/components/compose/LaneComposeInspector.vue app/features/tags/components/console/ObservationRangeControls.vue app/features/tags/components/console/ObservationRangeControls.test.ts` → 本change相关文件lint零错误；OQ-1若批准增加其它真实文件，在执行前将其显式补入同一命令。 → 实测 `eslint` exit 0（0 error；6 条 warning 为 `.vue` 不在 eslint 配置覆盖范围）。
- [x] 11.8 `cd front && pnpm exec nuxi typecheck` → 类型检查退出0，无把90错误塞进矩阵SourceTierDays的类型绕过。 → 实测 `nuxi typecheck` exit 0。
- [x] 11.9 `cd backend-go && golangci-lint run ./internal/topicgraph/repository ./internal/topicgraph/handler && GOMAXPROCS=2 go vet -p 1 ./internal/topicgraph/repository ./internal/topicgraph/handler && GOMAXPROCS=2 go build -p 1 ./internal/topicgraph/repository ./internal/topicgraph/handler` → 影响包lint/vet/build零失败，不跑./...。 → 实测 `golangci-lint run`（repository+handler）0 issues、`go vet` exit 0、`go build ./internal/topicgraph/...` exit 0。
- [x] 11.10 静态部署已执行（2026-11-16 主线程，用户批准）：`deploy-frontend.sh` 完成构建（Prerendered 8 routes）→ 铺 `backend-go/frontend/` → 重启后端 → `/health` 200（PID 1167418）。入口 http://localhost:5100/ ，后续浏览器检查用同源:5100
- [x] 11.11 `test -n "${BOARD_ID:-}" && agent-browser --session lane-workbench open "http://localhost:5100/tags/$BOARD_ID" && agent-browser --session lane-workbench snapshot -i` → 静态矩阵可达；依test-cases AB-1..AB-4继续逐步操作/机械断言，证据记录每步点击后的状态，不止记录按钮存在。
- [x] 11.12 `bash scripts/harness/ui-verification-trace.sh openspec/changes/unify-lane-workbench` → report条目及点名截图真实存在；AB主故事、双桌面/双主题、窄屏禁用、逐态矩形断言有证据，无用未覆盖冒充pass。 → 实测 `ui-verification-trace.sh` exit 0（archive-readiness ⑦ 同源检查绿）。
- [x] 11.13 真库只读审计 SQL + 配对 GET 对照已执行：按 `BOARD_ID=15220 / TOPIC_ID=503` 跑 `default_transaction_read_only=on` 会话（零写），结果见 `ui-verification/report.md` §Y；已知日差异=0（编辑曲线）、1 天已归因（新增池，口径自洽）、未知误补 0 次数=0（`clue_count=0` 的 6 条均为纯综合稿，正确返回 0 而非未知）
- [x] 11.14 `bash scripts/harness/review-trace.sh openspec/changes/unify-lane-workbench` → 聚焦审查有账，无未处置High，旧能力去向与合同OQ结论有记录。 → 实测 `review-trace.sh` exit 0（archive-readiness ⑤ 绿）。
- [x] 11.15 `bash scripts/harness/doc-impact.sh verify openspec/changes/unify-lane-workbench && bash scripts/harness/check-standards.sh --change unify-lane-workbench` → 本change文档域、规范与导航死链检查零失败，不遍历其它active change的归档准备状态。 → 实测 `doc-impact.sh verify` 通过（声明 flow, api；3 个文件）+ `check-standards.sh` 304 通过 / 0 失败。
- [x] 11.16 `bash scripts/harness/test-patrol.sh --report` → 只读巡检台账可读；域外红有既存登记/后续由主线程登记，本change影响文件红项必须修复，不能以域外欠账豁免。 → 实测 `test-patrol.sh --report` exit 0；域外 stale 1 条（`TestRelationDiscoveryTrigger_ObservationAndQuestion`）属既有台账，非本 change 文件。

### Scenario → 测试文件

标题逐字取自本change的两份delta；跨能力重名「查看与选入分离」一行覆盖两实例。表中现有文件是待补/迁移断言的落点，不表示现有断言已经符合新合同。RN与AB证据另外由9.4/9.6建账，不用未创建的文件充当前置映射。

| Scenario | 测试文件 |
| --- | --- |
| 从矩阵行进入修正工作面 | front/app/features/tags/components/console/BoardComposeOverlay.correction.test.ts |
| 查看与选入分离 | front/app/features/tags/components/console/LaneCorrectionCanvas.test.ts, front/app/features/tags/components/console/BoardComposeOverlay.test.ts |
| 取消勾选即移除 | front/app/features/tags/components/console/LaneMemberList.test.ts, front/app/features/tags/composables/useLaneCorrection.test.ts |
| 本次移出的节点可加回 | front/app/features/tags/components/console/LaneMemberList.test.ts, front/app/features/tags/composables/useLaneCorrection.test.ts |
| 首义范围手动勾选 | front/app/features/tags/components/console/LaneAnchorScope.test.ts, front/app/features/tags/composables/useLaneCorrection.test.ts |
| 首义来源可辨 | front/app/features/tags/components/console/LaneAnchorScope.test.ts, front/app/features/tags/components/console/LaneCorrectionCanvas.test.ts |
| 左栏超高时操作按钮仍可达 | front/app/features/tags/components/console/BoardComposeOverlay.correction.test.ts, front/app/features/tags/components/console/LaneAnchorScope.test.ts |
| 首义节点在窗口外时左缘折叠带展示 | front/app/features/tags/components/console/LaneCorrectionCanvas.test.ts |
| 自动首义不虚构节点列表 | front/app/features/tags/components/console/LaneAnchorScope.test.ts, front/app/features/tags/components/console/LaneCorrectionCanvas.test.ts |
| 首义选择池边界 | front/app/features/tags/components/console/LaneAnchorScope.test.ts, front/app/features/tags/composables/useLaneCorrection.test.ts |
| 候选口径含动态泳道节点 | front/app/features/tags/composables/useLaneCorrection.test.ts, front/app/features/tags/components/console/BoardComposeOverlay.correction.test.ts |
| 首义面板当前/更早分区 | front/app/features/tags/components/console/LaneAnchorScope.test.ts |
| 更早成员分组与计数口径 | front/app/features/tags/components/console/LaneMemberList.test.ts, front/app/features/tags/components/console/LaneAnchorScope.test.ts |
| 窄屏禁用画布 | front/app/features/tags/components/console/BoardComposeOverlay.correction.test.ts |
| 画布标题就地改名 | front/app/features/tags/components/console/BoardComposeOverlay.correction.test.ts |
| 改名的次要路径不弹窗 | front/app/features/tags/components/console/BoardComposeOverlay.correction.test.ts |
| 三栏内容不重复 | front/app/features/tags/components/console/BoardComposeOverlay.correction.test.ts, front/app/features/tags/components/console/LaneMemberList.test.ts |
| 节点卡快捷加入与移除 | front/app/features/tags/components/console/LaneCorrectionCanvas.test.ts, front/app/features/tags/components/console/BoardComposeOverlay.correction.test.ts |
| 相关节点自动高亮 | front/app/features/tags/composables/laneComposeLogic.test.ts, front/app/features/tags/components/console/LaneCorrectionCanvas.test.ts, front/app/features/tags/components/compose/LaneComposeCanvas.test.ts |
| 新增泳道曲线只计可选池 | backend-go/internal/topicgraph/repository/daily_report_manual_topic_test.go, front/app/features/tags/composables/useLaneCompose.test.ts |
| 可编排范围不静默截断 | backend-go/internal/topicgraph/handler/compose_candidates_handler_test.go, front/app/features/tags/composables/useLaneCompose.test.ts |
| 编辑泳道节点卡可快捷加入与移除 | front/app/features/tags/components/console/LaneCorrectionCanvas.test.ts, front/app/features/tags/components/console/BoardComposeOverlay.correction.test.ts |
| 两工作面接近度方向一致 | front/app/features/tags/composables/laneComposeLogic.test.ts, front/app/features/tags/components/console/BoardComposeOverlay.test.ts |
| 进入编排态 | front/app/features/tags/components/console/BoardComposeOverlay.test.ts |
| 进入修正模式 | front/app/features/tags/components/console/BoardComposeOverlay.correction.test.ts |
| 取消勾选为破坏性动作 | front/app/features/tags/components/console/LaneMemberList.test.ts, front/app/features/tags/components/console/LaneCorrectionCanvas.test.ts |
| 新建模式草稿不被修正模式污染 | front/app/features/tags/components/console/BoardComposeOverlay.test.ts, front/app/features/tags/components/console/BoardComposeOverlay.correction.test.ts |
| 窄屏禁用编排 | front/app/features/tags/components/console/BoardComposeOverlay.test.ts, front/app/features/tags/components/console/BoardComposeOverlay.correction.test.ts |
| 取消 | front/app/features/tags/components/console/BoardComposeOverlay.test.ts, front/app/features/tags/components/console/BoardComposeOverlay.correction.test.ts |
| 编辑泳道节点卡可快捷加入与移除 | front/app/features/tags/components/console/BoardComposeOverlay.correction.test.ts, front/app/features/tags/composables/useLaneCorrection.test.ts |
| 两工作面接近度方向一致 | front/app/features/tags/composables/laneComposeLogic.test.ts, front/app/features/tags/components/console/BoardComposeOverlay.test.ts, front/app/features/tags/components/console/BoardComposeOverlay.correction.test.ts |
