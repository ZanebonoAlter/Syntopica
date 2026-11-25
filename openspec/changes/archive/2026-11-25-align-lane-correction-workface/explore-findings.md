
## 新建画布高度未吃满的根因与修法

**根因**：LaneComposeCanvas.vue:610-612 `canvasHeight` 只由内容算 + 常数地线，完全不读容器高度：
```
canvasHeight = showAssigned ? max(560, activeRowOffsets.cursor+30) : max(460, rowsBottom + CANVAS_BOTTOM_PAD(36))
```
典型 1 条候选链内容高 = ROWS_TOP 32 + (32+2*24+26) + (32+2*24+26) + (32+24+26) = 326 → +36 = 362 → 被抬到 460。视口 ~660 时 460/660 ≈ 70%，与截图吻合；左右栏各自 overflow:auto 所以顶到底。

**高度链其余部分是通的**：BoardComposeOverlay .bco__shell{flex:1;min-height:0} → __workbench{grid,height:100%,min-height:0} → .bco__canvas{min-height:0;overflow:hidden} → LaneComposeCanvas 根 flex 列 → .lcc-canvas__surface{flex:1;min-height:0;overflow:auto} → svg{width:vbWidth*zoom, height:canvasHeight*zoom}。断点只在 SVG 自身高度。

**修法（现成参照）**：编辑画布 LaneCorrectionCanvas.vue:276 `baseCanvasHeight = max(360, containerHeight||360)`、:347-350 `canvasHeight = max(baseCanvasHeight, needed)`，多余高度由 zones(:353-367) 均分。用户已定：**填满容器，余量全部给散料区（floorRow）**，本次泳道行与候选链行位置不变。
**注意反馈环**：vbWidth > viewportWidth 时横向滚动条吃掉 clientHeight；overflowing(:637-641) 由 canvasHeight*zoom > viewport+1 决定，贴合容器后 100% 时吸顶将不触发（语义正确但需复核 pinnedTransform 用例）。containerHeight 已有测量 :166-173 + ResizeObserver :185-188。

**引用**：front/app/features/tags/components/console/LaneComposeCanvas.vue:610、front/app/features/tags/components/console/LaneComposeCanvas.vue:637、front/app/features/tags/components/console/LaneComposeCanvas.vue:166、front/app/features/tags/composables/laneComposeLogic.ts:1028、front/app/features/tags/components/console/LaneCorrectionCanvas.vue:276、front/app/features/tags/components/console/LaneCorrectionCanvas.vue:347

<!-- pinned 2026-11-24T06:14:36Z -->

## 新建画布拖拽机制与可复用清单

**手势**：全部 pointer 事件，无 HTML5 DnD。监听只在滚动容器一处（LaneComposeCanvas.vue:1399-1403）。
- `onSurfacePointerDown`(:838-867) 四分流：isInteractiveTarget(.lcc-card/.lcc-zoom/.lcc-undo/button/input...) 忽略 → `[data-node]` = kind:'node'（只读节点 return）→ `[data-role="row-handle"]` = kind:'row'（ids=该行未采纳成员）→ 其余 kind:'pan'
- 4px 阈值 isDragBeyond(laneComposeLogic.ts:236-242) 才进 dragging + setPointerCapture(:958-967)
- 坐标换算 contentPoint = (client - rect.left + scrollLeft)/zoom(:869-884)
- **落点只判行、不判日期列**：rowAtPointer(:907-917)（第 0 行用吸顶可视区间 band.top + scrollTop/zoom）+ rowAtContent(:892-898)（top ≤ y ≤ top+height）。日期由节点 periodDate 决定 ⇒ 不能换日期、不能拖排序。
- 结算 applyDrop(:977-993)：节点→第 0 行且原非成员 ⇒ emit('toggle-member', id, true)；节点→非第 0 行且原为成员 ⇒ emit('toggle-member', id, false)；行标签→第 0 行 ⇒ emit('adopt-chain', key)。**新建画布拖拽纯本地草稿，不调 API**（save() 才 POST）。
- 视觉：ghost .lcc-drag-ghost(:1906-1916，12px 圆 scale(1.6)，整组 22px 带数量，rAF 节流 :923-936)、本体跟随 draggedTransform(:1169-1173)、落点框 .lcc-canvas__dropzone(:1492-1500，is-drop-target 加深 :2133-2136)、行 hover 提亮 .is-row-hover(:2256-2259)、拖完 250ms 内吞 click(:1345)、Esc 取消(:1034-1042)。
- 已知缺陷：`.is-drop-other`(:1451) 无任何 CSS 规则，拖到非第 0 行无提示。

**可整段搬（算法层，只依赖 layout + emit）**：contentPointOf(:869-879)、rowAtPointer/rowAtContent(:892-917)、applyDrop(:977-993)、resetGesture(:995-1006)；CSS 前缀同是 `lcc-`，.lcc-drag-ghost* / .lcc-canvas__dropzone / .is-drop-target 可直接共享。
**不能搬**：行模型 rowOrder+buildRowIndex(:255-259,:351-355) 建立在「候选链=行」，编辑画布是固定三区；emit 语义（编辑画布每条即时落库 setSectionAssignment + pendingIds 防重入，不是草稿 Set）；吸顶依赖「本次泳道恒第 0 行」。

**引用**：front/app/features/tags/components/console/LaneComposeCanvas.vue:838、front/app/features/tags/components/console/LaneComposeCanvas.vue:977、front/app/features/tags/components/console/LaneComposeCanvas.vue:907、front/app/features/tags/composables/laneComposeLogic.ts:236

<!-- pinned 2026-11-24T06:14:36Z -->

## 编辑画布（修正工作面）现状：坐标系、图例错位、无拖拽

**结构**：BoardComposeOverlay.vue mode=correction → grid `380px minmax(0,1fr) 300px`(:809-816)。左 bco__left = LaneMemberList(:457-467) + LaneAnchorScope(:468-481)；中 LaneCorrectionCanvas(:483-511)；右 bco__right-correction = LaneRecommendationList(:512-517)。（代码里无 bco__center / bco__right 类名。）高度链通、图例与缩放条是 absolute 叠在中心栏底部（.lcc-legend :1343 / .lcc-zoom :1387）。

**坐标系 = 日期列 × 横向带区**（不是行）：日期轴(:749)、话题带 BAND_Y=44(:116,:769)、成员基线 MEMBER_Y=86(:118)、候选区、只读参照区 zones(:326-367)、左缘「更早」折叠带 LEFT_PAD=150 FOLD_W=68（上限 LANE_FOLD_MAX=6）。节点=圆点+两行标题，节点卡是绝对浮层 .lcc-card(:932/:1268)。

**底部图例 5 项**（:971-978）：成员（实心+勾）/ 候选大小=接近度 / 虚线=首义 / ⊘已移出 / 相关。**两处语义错位**：①「虚线=首义」只画未提交的 anchorSelection(:473)，**已生效首义成员反而没环**；②「相关」仅在 focusId 存在时出现。

**相关度肉眼不可辨的根因**：appearanceOf(:462-478) 走绝对值口径 1-distance/threshold(laneComposeLogic.ts:512-518)，且**不传** proximityOverride/recommendRange/strengthRange → 半径只有 3.6-4.6px、填色 12-67%（laneComposeLogic.ts:575 注释点名「修正画布逐位不变」）。新建画布走 relativeProximities 批内归一 + recommendRange{5.9,7.4} + strengthRange{18,80} + proximityOverride(LaneComposeCanvas.vue:724,:740-745)。
**抬尺寸的前置链**：recommendCap = min(cap*0.72, memberBase-0.5)，correction memberRadius=5.5(:122) ⇒ 候选上限被压到 5.0；抬 memberRadius 会牵动 NODE_BLOCK_H/nodeRowGap/membersBottom/zones(:308,:340,:347,:353)，必须同批改。相对归一只能用于 estimate 桶（成员是 snapshot 且不编码接近度）。

**交互现状**：只有「空白处 pointer 拖 = 平移」(:235-265)，**无任何节点拖拽**；节点写入口 = 节点卡三按钮(:932-964) emit add/remove/restore。缩放 = 滚轮(:200)+右下「− 91% + 重置」(:981-986)，resetZoom(:220) 只回 zoom=1 且滚动归零，不动成员/选中/首义。

**双口径红线**：成员 proximity = 1-topic_match_distance（快照），候选 = 1-质心距离（estimate，标「估算」）(useLaneCorrection.ts:88-100,:274-278)。两者 MUST NOT 同名同形（fix-lane-metric-provenance）。

**引用**：front/app/features/tags/components/console/LaneCorrectionCanvas.vue:462、front/app/features/tags/components/console/LaneCorrectionCanvas.vue:473、front/app/features/tags/components/console/LaneCorrectionCanvas.vue:971、front/app/features/tags/components/console/BoardComposeOverlay.vue:809、front/app/features/tags/composables/laneComposeLogic.ts:575

<!-- pinned 2026-11-24T06:14:36Z -->

## 首义（anchor）数据模型、滚动质心与读 API 缺口

**概念纠正**：代码里「首义/anchor」不是「一组话题」，而是每条泳道（persistent topic）的一条向量：`board_persistent_topics.embedding`（首义，daily_report_models.go:98）+ `centroid`（=α·首义+(1-α)·滚动均值，:111）+ `centroid_pending_recompute`(:123)。话题级集合概念是 ListAnchorableTopicsByBoard（聚类/归属候选池），不可混谈。界面「窗口内·11·全部183」的 183 = 该泳道成员 section 总数（useLaneCorrection.ts:221-222），不是话题数。

**auto vs manual**：GetTopicAnchorState(daily_report_anchor_read.go:53) 只数 topic_anchor_overrides 行数——0=auto、≥1=manual，节点集从最新行 input_summary 反解(:84,:93)。常量 TopicAnchorSourceAuto/Manual(:21-26)。阈值判定在前端 useLaneCorrection.ts:198。文案 '自动（创建话题时写入）' 在 :339-343，渲染 LaneAnchorScope.vue:107-108。

**三种创建期首义来源**（topic-graph.md:87）：auto_new 候选=首条命中 section 向量原样(daily_report_assignment.go:179-183)；**手动建泳道=选定 section 的均值(daily_report_manual_topic.go:635, Source=manual:637) 但没写 override 留痕** ← 所以手动建的泳道也读成「自动」(模型缺口)；watch 物化=检索句向量(topic_watch_repository.go:457-458)。

**滚动计算 = ComputeTopicCentroid**(daily_report_topic_repository.go:959)：内联 SQL(:962-979) 取该 topic 归属 section `WHERE persistent_topic_id=? AND embedding IS NOT NULL ORDER BY period_date DESC, s.id DESC LIMIT window`；window=persistent_topic_centroid_window 默认 30(:85, 建表 :1704)；<2 可解析向量→退回首义(:992-994)；混合 blendCentroidWithAnchor α=persistent_topic_centroid_anchor_weight 默认 0.5(:88,:234-239,:911)；写回 UpdateCentroidOnSectionChange(:1049)，触发=日报保存后(:1404-1408)/rebucket(service/daily_report_rebucket.go:111)，范围=当日 touched ∪ pending。**持久化列，不是每次重算**。匹配侧 service/daily_report_lane.go:116 topicAnchorVec（centroid 优先）→ :140 BucketTagsByCentroid 分 L1/L2/L3。

**读 API 缺口**：GET /api/daily-reports/topics/:id/anchor（注册 handler/daily_report_handler.go:75，handler daily_report_anchor_read_handler.go:32）只返回 TopicAnchorState{topic_id,source,section_ids,override_count,effective_from}(daily_report_anchor_read.go:32-45)，**不含**解析出的首义来源 section、参与滚动的 section 列表/条数/权重/窗口/α/degraded。
**可复用**：GetTopicAnchorState、GetTopicLifeline(daily_report_repository.go:1231)、LoadPersistentTopicConfig、ListAnchorableTopicsByBoard(:320)、GetComposeCandidates(daily_report_manual_topic.go 附近, early_members:202)。
**须新写**：ListCentroidWindowSections(topicID, window)——把 ComputeTopicCentroid 内联 SQL(:962-979) 抽出来，两份口径共用一份实现，避免漂移。
**红线**：auto 首义无节点集不得虚构（api/daily-reports.md:342 契约）；不可考时显式 resolved:false + null，不得拿「最早成员」冒充；读接口零写副作用、不回填快照。

**引用**：backend-go/internal/topicgraph/repository/daily_report_anchor_read.go:53、backend-go/internal/topicgraph/repository/daily_report_topic_repository.go:959、backend-go/internal/topicgraph/repository/daily_report_manual_topic.go:635、backend-go/internal/topicgraph/handler/daily_report_handler.go:71、front/app/features/tags/components/console/LaneAnchorScope.vue:107、front/app/features/tags/composables/useLaneCorrection.ts:198

<!-- pinned 2026-11-24T06:14:36Z -->

## 修正画布三处补丁落点：节点卡标题 / 话题带横条 / 空白轻点取消聚焦

文件：front/app/features/tags/components/console/LaneCorrectionCanvas.vue（修正工作面中央画布）。

1. 节点卡：模板 `.lcc-card`（`data-testid="lane-node-card"`，`CARD_WIDTH=264`，`position:absolute` 挂在 `.lcc-surface` 滚动容器内），标题 `<strong class="lcc-card__title">`。卡高由 `cardSize` 在 `watch(() => props.focusId)` 的微任务里实测 `cardEl.offsetHeight`（首帧用默认 120）。原样式 `white-space:nowrap + text-overflow:ellipsis` 截断 → 改 `white-space:normal + overflow-wrap:anywhere + line-height:1.45`。卡体原 `pointer-events:none`（穿透到画布）→ 改 `auto`，否则点卡会落到画布空白被当成取消聚焦。

2. 话题带：`<g class="lcc-band">`（BAND_Y=44）含 `.lcc-band-title`「本泳道话题带」、`.lcc-band-sub` 泳道名、`.lcc-track` 中性基线（`--color-border-medium`，与 `.lcc-zone` 同色）；已删 `.lcc-band-bar`（accent 圆角横条 12px）与 `.lcc-band-tick`（成员刻度虚线）模板 + CSS。布局常量不动。

3. 取消聚焦：`focusId` 是 prop，节点 click → `emit('focus', id)`；宿主 `useLaneCorrection.setFocus` 是**切换**语义（同 id 再点 = null），`clearFocus()` 置 null。新增 emit `'clear-focus': []`（与编排画布 LaneComposeCanvas 同名事件），在 `onSurfacePointerUp` 末尾：`if (!wasPanning && !panStart.onNode && props.focusId) emit('clear-focus')`。`panStart.onNode` 在 `onSurfacePointerDown` 记录 `Boolean(nodeEl)`；不可拖节点（只读参照/折叠带）按下会退化到 panStart（R-6），所以必须靠该 flag 排除，否则 pointerup 先清空、随后节点 click 的 setFocus 切换会把「再点同一节点取消」误翻成重新聚焦。`onSurfacePointerDown` 开头的 `closest('.lcc-card, .lcc-zoom, button, .lcc-fold-check')` 早退已覆盖卡/缩放条/勾选框。宿主接线：BoardComposeOverlay.vue `<LaneCorrectionCanvas @clear-focus="correction.clearFocus()">`。

测试：LaneCorrectionCanvas.test.ts（`pressBlank` 助手：pointerdown/up 直接派发到 `[data-testid="lane-correction-canvas"]`；样式断言用 readFileSync 读 SFC 源码锁 `.lcc-card__title` 规则）、BoardComposeOverlay.correction.test.ts（本地 `pointer()` 助手）。

**引用**：front/app/features/tags/components/console/LaneCorrectionCanvas.vue:onSurfacePointerUp、front/app/features/tags/components/console/LaneCorrectionCanvas.vue:onSurfacePointerDown、front/app/features/tags/components/console/BoardComposeOverlay.vue:LaneCorrectionCanvas、front/app/features/tags/composables/useLaneCorrection.ts:setFocus

<!-- pinned 2026-11-24T15:57:49Z -->
