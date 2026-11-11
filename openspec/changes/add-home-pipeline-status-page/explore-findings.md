
## v64 场地重构影响面清单

## v64 场地重构（布局/道路/装饰）影响面清单

### 要改的位置（行号为改动前）
- `index.html:539-550` SITES 坐标块（9 站 x/z）
- `L551-556` ROADS（9 条边）
- `L561-568` ZONES（6 个分区矩形）
- `L569` GROUND {w:40,d:23}
- `L597` LEVELS（map 100 / isle 34 / focus 10 / inspect 4）
- `L618` FIT_BOX {map:{x:62,z:44}, isle:{x:21.5,z:13}}
- `L812-814` decorCount / decorPts / decorMeshes 声明
- `L1099-1100` `buildDecor()` 调用 + `window.__DECOR_COUNT`
- `L1104-1220` `buildDecor()` 函数整体（23 类装饰：煤堆/传送带/烟囱/龙门吊/跨海栈桥/缓冲车挡/装卸月台/集装箱垛/冷却塔/变电所/道岔/信号灯/罐车/筒仓/成品圆筒/装货月台/货车/水塔/围墙大门/路灯/储罐/小车/中央大道）+ `__DECOR_PTS` / `__DECOR_AUDIT`
- `L1243-1256` `addRoadPath` 的 `bends` 表（3 处折弯）+ `clip()` 用 NODE_GEOM 原始 foot
- `L1390-1396` buildSites 的 `buildingScale` 白名单 + `plinth()` foot
- `L1420` `arcR` 用 NG.foot
- `L1044/1051` 底板 BoxGeometry(GROUND.w,.5,GROUND.d)
- `L1090-1091` 点阵网格按 GROUND 铺
- `L914` / `L959` buildIslandSurroundings 用 GROUND.w/2, d/2 算外圈与岸线
- `L22856-22862` zoneLabels 用 ZONES 取锚点
- `L1252-1254` clip：`foot[0]*1.725/2` 是「半 foot × 1.725」的留白系数

### foot 消费点（改缩放必须同步）
`siteFootLong` L602-603 / `clip()` L1252-1254 / `plinth` L1396 / `arcR` L1420 / `buildPartsStation` 台基 L21524（组内局部，随组缩放，无需改）/ tag 车间 L22524、L22559（tag 缩放不变 1.5）

### 关键结论
- 精修站全部 `buildingScale=1`，scene 里站体尺寸 = parts.json 的 foot。改 1.25 要同时改 `plinth`/`arcR`/`clip` 三处 foot，否则路会切进台基。
- `siteFootLong` 的基准 2.2 是 feed 的 foot；若所有精修站统一 ×1.25，比值不变 → **相机自适应逻辑不用改**（tag/fetch 的 1.5 非均匀缩放被 clamp 吸收）。
- `buildDecor()` 删掉后 `window.__DECOR_AUDIT` / `__DECOR_PTS` / `__DECOR_COUNT` 全部消失，需检查验证脚本是否引用（v57 前用过）。

<!-- pinned 2026-11-09T09:00:56Z -->

## 微缩世界讨论：真实业务不是九站环线

只读核对结论（2026-11-09）：现原型 ROADS 的视觉环线不可当业务因果链。向量完成触发 per-tag 版块匹配，日报以版块/当天 event 标签/已有话题质心生成；board_stories 在 SaveReport 后归属，并非“事件归并后才提供日报正文素材”。搜索是查询读取，不应表现为返工生产线。发现的候选/推荐仅用户 accept（必要时填参验证）后创建 feed，候选池更新不等于已订阅；不存在 report→discovery 的生产依赖。动画数据边界：processing 是在处理的文章/标签任务数，不是员工人数；当前静态 data.json 不支持真实逐条运输或持续 searching，现有原型按存量非零恒流会误导。后续应区分真实状态/真实事件、明确标识的工序演示、无数据的未知态；统一资讯实体需保持来源身份与文章→标签/聚合的关联，不把所有派生对象当同一原件。用户本轮仅要求讨论，尚未批准新布局或实现。

**引用**：backend-go/internal/domain/topicgraph/service/daily_report_orchestrator.go、docs/reference/flow/daily-report.md、docs/reference/flow/discovery.md、docs/reference/flow/reading.md、docs/reference/flow/semantic-board.md、openspec/changes/add-home-pipeline-status-page/ui-prototype/index.html:ROADS

<!-- pinned 2026-11-10T02:20:59Z -->
