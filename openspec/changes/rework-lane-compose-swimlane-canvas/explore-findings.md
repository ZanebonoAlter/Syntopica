
## LaneComposeCanvas 现状几何与视觉编码（改造前基线）

画布 = SVG，坐标 1:1 于容器像素（`vbWidth = clientWidth`，resize 重排）。

**数据单位**：一个点 = 一个 section（日报事件片段），字段 `id/reportId/periodDate/clusterLabel/embedding/persistentTopicId/persistentTopic{id,label,status}/clueCount`（`front/app/api/persistentTopics.ts:29-46`）。"已入 active 泳道的节点"= `persistentTopic.status==='active'`，默认隐藏（`showAssigned` 才画）。

**坐标（改造重点）**：
- x = 日期列：`leftPad(40或150) + (dateIndex+0.5)*columnWidth`，`columnWidth = span/dates.length`（`LaneComposeCanvas.vue:333-355`）。
- y = **同日内槽位**：`poolTop + k*rowGap`，k = 该 section 在同日按 `(periodDate asc, id asc)` 排序的序号；`rowGap = max(36, min(110, 360/(maxSlots-1)))`（`:339-350`, `:390-407`）。**y 无语义**。
- 关系边 = 二次贝塞尔（`edgePath` `:652-658`），identity 实线、similarity 虚线 4 4（`:1055-1065`）。
- 日期竖 guide 虚线 2 6（`:809-825`）。

**大小/深浅 = 接近度**（仅当有质心）：`pointAppearance` `laneComposeLogic.ts:414-451`；`proximity = clamp(1-distance/threshold)`；已选点尺寸固定 `memberBase`、`recommendCap < memberBase`（推荐半径恒小于已选）；`--proximity-fill = 12+55*proximity`。无质心时全部走 ordinary 分支 → 视觉休眠。

**视觉态**：普通空心（fill=bg-base, stroke=muted 1.5）/ 推荐（accent 描边 + 深浅填充）/ 已选（实心 accent + 白✓）/ 焦点（focus-ring r+4）/ 相关（accent 2）/ 弱化（opacity .35）。

**编号**：`numberById`（`:548-556`）按 `periodDate desc, id desc` 生成**全局序号**，两位补零；显示条件 = 属于候选链/已选/焦点，或 `dates<=14 && 总数<=20`；`dates>45` 全不显示。**编号与左栏候选组、右栏列表无任何映射**（死编号）。

**候选带**：`bandLanes` 取成员数降序前 6（`MAX_BAND_LANES`，`:317-330`）；行 `BAND_TOP(20) + i*24`，横条 `r=6, h=12`；成员用 accent 细虚线 tick（0.8px, opacity .35）从带底连到点上方；>6 条时底部写「另有 N 个候选话题在左侧候选栏」。标题避让四位置，冲突则放弃不画（宁缺勿叠）。

**只读参照区**：分隔线下方，每个已有泳道一行，成员按 10px 密排梯子（`:373-375`）。

**交互现状**：hover 出节点卡（HTML 浮层，宽固定 300px 常量参与定位）；点击 = 聚焦钉住（`is-pinned`），再点取消；卡内「＋选入 / 移出」按钮才是选入通道；空白处按住拖 = 平移（4px 阈值）；滚轮缩放 0.5–3×（步进 1.1，光标锚点）；缩放补偿 `scaledPointRadius` 保证渲染半径 ≥3px。命中区独立透明圆 `r=max(7, r+4)`。

**引用**：front/app/features/tags/components/compose/LaneComposeCanvas.vue、front/app/features/tags/composables/laneComposeLogic.ts、front/app/api/persistentTopics.ts

<!-- pinned 2026-11-20T06:58:55Z -->

## 推荐/候选链口径与门禁红线（改造须遵守）

**推荐与候选链口径**
- 候选链 = 同 `persistent_topic_id` 分组内沿**原 relations**（identity + similarity）取无向连通分量；合格 = 节点 ≥2 **且** 首末 UTC 日差 ≥3；分量各自成链、不因相似边合并；`key = {topic_id}:{最小 section id}`；排序 `last_date desc → span_days desc → topic_id asc`（`docs/reference/flow/daily-report.md:364` 约束 38，服务端 `backend-go/internal/topicgraph/repository/compose_chain.go` 为唯一口径，前端只渲染）。
- 质心 = **当前全部有效已选成员向量均值**（非首成员/非焦点/非 candidate embedding）；阈值 `match_threshold` 默认 0.3；画布与右栏「可补充的事件」**必须同一批**（约束 39 + `spec.md:141`）。
- 红线："推荐 SHALL 只是距离提示、不构成准入或置信度"；"SHALL NOT 自动勾选"；"推荐点半径 SHALL NOT 大于已选点半径"；"已选只由实心＋✓表达，大小只表达接近程度"。
- **距离是前端算的**：`laneComposeLogic.centroidOf / cosineFor / centroidRecommendations`（:276-314）。→ 阈值档位纯前端，无需后端改动。
- `matchThreshold` 由 API 下发的 pool 响应带入（`useLaneCompose.ts`/`useInlineCompose.ts:121`，默认 0.3），当前**无 UI 可调**。

**"按标题找"现状（很弱）**：纯前端本地过滤（`LaneComposeInspector.vue:49`），范围 = 窗口内已加载 sections，**只匹配标题**，只影响右栏列表，画布无感知，不调 AI、不自动勾选。母 spec 要求该入口 SHALL 收在右栏 tab 内、不在多处重复入口（`spec.md:168`）。

**交互红线**：`spec.md:8`「查看与选入分离」——点击 SHALL 只改选中态，SHALL NOT 由点画布本体触发选入；显式选入动作 = 勾选框 或 节点卡「＋加入」/「⊘移出」。修正模式下「取消勾选 = 直接解除归属」为破坏性动作（不弹选择器、不指定去处）。`standard/frontend/interaction.md:74-84` D1：泳道内容增删改入口 = `canvas`（forbidden: dialog/drawer），状态 `pinned`，用户已定。

**引用**：openspec/specs/workshop-lane-compose/spec.md、docs/reference/standard/frontend/interaction.md、docs/reference/flow/daily-report.md、front/app/features/tags/components/compose/LaneComposeInspector.vue、front/app/features/tags/composables/useLaneCompose.ts

<!-- pinned 2026-11-20T06:59:14Z -->
