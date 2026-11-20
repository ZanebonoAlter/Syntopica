
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

## 外部参考 site-to-map 环境层实现事实（MIT，非本项目实现）

外部参考仓库 https://github.com/dofy/site-to-map（调研版本 `641432a381160d6b914001984cec8205e6f96e18`；MIT，Copyright (c) 2026 Seven；本地浅克隆 /tmp/s2m，仅作参考，未装入仓库/未安装 skill）。以下是「环境丰富但不杂乱」的源码证据，均属外部实现、非本项目现状：

1) 地形台地化 + 分面调色：engine/map-engine.js:753-760 `terraceStep=.3`，`terrace()` 把高度量化到 0.3 台阶并在 0.6~0.9 段用 sstep 平滑；`H()`(762-770) 按 `farness` 在台地/原始高度插值，道路 1.2u 内压平到 `h0-.02`，河道 `r.w+1.2` 内压到 -0.85。面分类 ~770-790：每三角形按高度/坡度落 9 类（sandWet/sand/cut/stone/slate/snow/meadow/grass/olivegrass），`slope>0.3` 且近处→`cut`（削坡断面 kraft 色），每面乘 jitter 0.93~1.05。纸感来自「每面一色+微抖动」，无贴图。
2) 树=规则否定的抖动网格+实例化：`blocked()` 1199-1209 给出避让半径（田 .5 / 城内半径 / 街道 .4 / 城墙带 ±.35 / 外路 .75 / 河 w+.45 / 建筑半径）；网格 .55 抖动，`forestDens>.56` 保留率 .8 否则 .05；5 种树冠 mergeGeometries 合一几何 + InstancedMesh 冠/干（10 drawcall）+ 逐实例 tone 调色（1182-1245）。
3) 道路是「有厚度的纸带」：846-891，半宽 .42、侧壁下探 .22（kraft 色）、圆头端帽压接缝、进城镇高度接 `city.y-.02`（sstep 0~1.4）；左边缘每 9 个采样点一个里程碑(.1×.18×.06)；过水处加桩+双线栏杆+桥墩。
4) 地块与镇界：每楼一块 `irregular(7,.7,.92,seed)` 厚 .04 薄板（paper/road 交替、随机旋转，1108-1110）；街道从广场 `bend()` 放射到每楼（横向偏移≤22%），镇内 .4 / 入口 .56，路灯只挂隔一条街的外侧（w/2+.1，1028-1042）；镇墙/树篱 46 段折线在 .98 半径，遇街口留缺口（<.85 跳过），每 2 段垛口、每 7 段塔（1044-1057）。
5) 建筑细节分层：`roofPrism` 出檐 .06/.1（360-365）；`panes()` 真盒子四面窗（亮玻璃 .3 / 暗 .4 / 木框 .3），`windows()`/`timber()` 纯墨线（345-387）；每楼必带门 .15×.26、烟囱概率 .6；`annex()` 13 种附属件（hut/tower/stall/barrels/crates/stairs/bench/well/lantern/cart，388-430）——附属件是建筑功能外延，不是装饰堆。
6) 观感靠一个后处理 shader 而非更多模型：`post`(276-320) 深度拉普拉斯+法线差出墨线（edgeK .82），按焦距算 CoC 做 16 采样螺旋景深，雾按距离抑线宽；灯光 hemi 1.75 + 平行光 2.1、3072² 阴影图、`shadowMap.autoUpdate=false`（246, 330-337）；全体 MeshLambertMaterial{flatShading:true}（257-259），墨线两档 .45/.85。

**引用**：外部 /tmp/s2m/skills/site-to-map/engine/map-engine.js:753-770、外部 /tmp/s2m/skills/site-to-map/engine/map-engine.js:1182-1245、外部 /tmp/s2m/skills/site-to-map/engine/map-engine.js:846-891、外部 /tmp/s2m/skills/site-to-map/engine/map-engine.js:1002-1110、外部 /tmp/s2m/skills/site-to-map/engine/map-engine.js:345-430、外部 /tmp/s2m/skills/site-to-map/engine/map-engine.js:276-337

<!-- pinned 2026-11-13T09:41:25Z -->

## city-workflow 现有环境层空白点与可复用钩子（03 总览基线内）

原型未被 CodeGraph 索引（索引语言仅 go/js/ts/vue/yaml，无 html），下列为直接读 openspec/changes/add-home-pipeline-status-page/ui-prototype/city-workflow.html（2421 行，SHA 324ec5ce…）所得。

现状（环境层几乎为零，故「周遭细节缺失」成立）：
- 地面只有三块平面色：`addDistrictPaving()`(1793-1797) 用 `flatRect(...,0.014, DISTRICT_PAVE[id])`；`addStreets()`(1798-1814) 街道 y=.028 / 广场 .03 的 `flatRect` + `lineLoop` 墨线（街口 opacity .50、广场 3 条横线 .14、院落 2 条 .12）。没有厚度、侧壁、地块板。
- 接地靠假阴影：`shadowTexture()`/`contactShadow()`(732-764) 是 canvas 贴图矩形（opacity .20，偏移 -0.30/-0.46），`renderer.shadowMap.enabled = false`（~1862）。
- 场景常量：`GROUND = {x0:-40,x1:40,z0:-27,z1:27,thickness:0.7}`(356)；1 布局单位 ≈ 1/2.4 层楼高(349-352)；ECS 级 `ELEV=0.85`(684，≈48.7°)；`THREE.OrthographicCamera`(1841)；相机由 `solveCam()`(1323-1331) 按 extent 求 `viewH`，px/u = S.H/CAM.viewH(1904)。
- 墨线：`inkEdges()`(784-793) 每 mesh 一套 EdgesGeometry(25°) 线材质 opacity .26；`lineLoop`/`rectLineXY` 各自新建材质——环境件增多会线性涨 drawcall，需实例化/合并。
- 已有可复用探针：`__CITY_STATE()`（含 viewH / pxPerUnit / 工位 AABB / 投影框重叠）、`__CITY_VISIBLE('station'|'worker'|'card')` 遮挡感知像素差、`__CITY_GROUND_POINT()`、`__CITY_PICK/HIT_AT`。环境层的「不遮 03」判据可直接用前两个，无需新造。
- 03 已认可基线：南墙开间 x[-17.2,-3.1] 永久敞开、工位 AABB x[-15.91,-3.68] y[0.30,2.90] z[-13.28,-10.79]、整组 scale 2.1；屋脊/山墙/北墙保留。视线来自 +z 南侧，配景是否遮挡必须依据总览/近景的真实投影与 `__CITY_VISIBLE` 差分实测，保住1440下工位宽201px/人高22px/卡宽20px的可见基线；前方0.4~1.2u、限高0.35u只是未验证的调研设想，不作为既定约束。
- 街道语义保护（主线程核正）：当前 `city-workflow.html:addStreets()` 的九条街巷是静态通行铺装，不是数据流；没有调研初稿所称的1.15宽流带/弧长方向三角，那是与旧版表达混淆。环境只能强化街道、院落等空间组织，不凭通路连接制造业务因果，不加暗示任务运输的箭头/流动短划。

**引用**：openspec/changes/add-home-pipeline-status-page/ui-prototype/city-workflow.html:356、openspec/changes/add-home-pipeline-status-page/ui-prototype/city-workflow.html:1793-1814、openspec/changes/add-home-pipeline-status-page/ui-prototype/city-workflow.html:732-793、openspec/changes/add-home-pipeline-status-page/ui-prototype/city-workflow.html:1323-1331、openspec/changes/add-home-pipeline-status-page/ui-prototype/city-workflow.html:1841、openspec/changes/add-home-pipeline-status-page/ui-prototype/city-workflow.html:1904

<!-- pinned 2026-11-13T09:41:25Z -->

## 环境参考复核：当前街道是通行铺装，装饰遮挡以现有探针实测

主线程定点复核city-workflow.html:1791-1815：addDistrictPaving/addStreets只生成分区平面、九条街巷/广场/院落铺装与静态墨线，没有调研初稿所称1.15宽数据流带或弧长方向三角，不能混用旧index.html的ROADS表达。街道连接不是业务因果。环境候选可复用__CITY_VISIBLE('station'|'worker'|'card')的遮挡感知像素差，保护1440下已认可的201px工位宽/22px人物高/20px卡宽；未实测的0.35u配景限高不能升级成约束。外部参考site-to-map已核对版本641432a381160d6b914001984cec8205e6f96e18，MIT Copyright(c)2026 Seven；参考其分层/避让/实例化方法，不照抄不同尺度参数，不执行外部skill。用户已认可03总览可见工序；本轮仅调研周边环境和后续高楼精修，尚未创建或批准city-workflow-env候选。

**引用**：openspec/changes/add-home-pipeline-status-page/ui-prototype/city-workflow.html:addStreets、openspec/changes/add-home-pipeline-status-page/ui-prototype/city-workflow.html:__CITY_VISIBLE、https://github.com/dofy/site-to-map/blob/641432a381160d6b914001984cec8205e6f96e18/skills/site-to-map/engine/map-engine.js

<!-- pinned 2026-11-13T09:57:44Z -->

## 03建筑＋小院是独立单模块候选，体量与工位平移尚未获城市布局批准

新原型ui-prototype/module-03-workshop.html只含一栋03与10件小院配景，使用__MOD_STATE/__MOD_PICK/__MOD_HIT_AT/__MOD_VISIBLE等探针。新壳体占地20×5.2，双坡檐6.2/脊7.26；原城市03仍26×10，旧8页完全保留，尚未把新占地嵌回城中。工位保持已认可2.1倍均匀与原10.2s节拍，仅整组原点从(-10.3,.3,-11.55)平移到(-.6,.3,-1.10)。主线程实际投影点(969,618)射线命中03，near→返回仍播放、暂停转俯视冻结；单scene/renderer、sceneChildren6/listen20/rafMax1。HTTP/file账本71/71，资产入口32项通过（31旧文件与139节点manifest）。用户现已认可03方向，并授权其余八栋按“建筑＋周边”各做一遍后统一审阅，不再逐栋等待批准（见module-batch-plan.md与根故事S8）。整城布局/新占地仍未获整合批准；S6环境页被指出不协调，仅保留对照，不能用它当风格基线，也不能自动按新占地改原城市。

**引用**：openspec/changes/add-home-pipeline-status-page/ui-prototype/module-03-workshop.html、openspec/changes/add-home-pipeline-status-page/ui-prototype/module-03-workshop-verification.md、openspec/changes/add-home-pipeline-status-page/test-cases.md:故事 S7

<!-- pinned 2026-11-14T08:47:29Z -->

## 单栋批次共享保护与原始运行记录入口

清理后的保护唯一入口为 ui-prototype/module-batch-assets.json + module-batch-assets-verify.py：schemaVersion=1，activeSources为路径→SHA字典，requiredActiveSources为必需路径；transition保留原776清单/stamp摘要与416授权退役路径，360只指迁移当时未变，不是当前776/776。已核验至08时活动源52项；新模块仅追加自身已完成源码，fixture.json按modules['NN']改自身键，协调md/图片/运行账本不永久冻结。共享module-batch-runs.jsonl已首次导入现存08的8份完整运行对象，字段recordType=import-existing-run、observedAtUtc、sourceFile、sourceFileSha256、run；这不是执行当时写入，更不是恢复误覆盖的旧55b…HTTP40/40。08当前ac48fab…两路径48/48原始对象均在。后续完整run需先追加日志再更新latest，避免新旧版本同名覆盖失证。

**引用**：openspec/changes/add-home-pipeline-status-page/ui-prototype/module-batch-assets.json、openspec/changes/add-home-pipeline-status-page/ui-prototype/module-batch-assets-verify.py、openspec/changes/add-home-pipeline-status-page/ui-prototype/fixture.json、openspec/changes/add-home-pipeline-status-page/ui-prototype/module-batch-runs.jsonl

<!-- pinned 2026-11-15T12:52:13Z -->

## 九栋审阅样板与城市原型是两条独立制品线

当前审阅入口 ui-prototype/modules-review.html 展示 module-01…09 独立样板截图并跳转对应单栋页面；城市原型 ui-prototype/index.html 使用 nodes/*/parts.json 的城市几何，两者同编号并非同一栋，样板未并回城市。03 城市几何为手工生成，未使用 nodes/03 parts.json。单栋 03/06/08/09 有工序演示；01/02/04/05/07 为静态造型。07 独立展厅仅有概览/近景/俯视瞬切，暂无工序动画；城市版 07 则是另一套圆柱仓。上述为代码与现有审阅截图核对，不代表用户认可。

**引用**：openspec/changes/add-home-pipeline-status-page/ui-prototype/modules-review.html、openspec/changes/add-home-pipeline-status-page/ui-prototype/node-decisions.md、openspec/changes/add-home-pipeline-status-page/ui-prototype/module-07-board-hall.html、openspec/changes/add-home-pipeline-status-page/ui-prototype/index.html

<!-- pinned 2026-11-16T03:26:34Z -->

## 首页3D可视化真实目标 + 历史无Blender（glTF为未启用空插座）

【Blender 结论】历史上从未用 Blender。全仓 grep -ril 'blender'（排除 node_modules/.git/vendor）仅命中 6 文件，全是产品自己抓回的 AI 新闻实体（demo/seed/seed.sql 的 'Blender 开源3D创作软件'/'BlenderMCP'/'Blender Vulkan后端'、backend-go/.tmp-entity-resolution-eval/data/aux_pool.tsv），change 目录 0 命中。

【最接近 Blender 的历史遗留 = glTF 接口，但从未启用】ui-prototype/node-decisions.md「全局 → 模型接口」行：`SITE_MODELS` 挂载点 + model-spec.md：glTF `{id}.glb`，材质槽 body/roof/detail/emit，单栋三角面数 ≤8k，加载失败警告并回退盒体。代码里留了插座：ui-prototype/index.html:342 `<script src="vendor3/GLTFLoader.js">`、:349 `const SITE_MODELS = {feed:null,...,discovery:null}`（全 null）、:1473 `if (SITE_MODELS[s.id]) loadSiteModel(...)`、:25870 `loadSiteModel()`。change 目录内 0 个 .glb/.gltf/.blend 文件 → 空插座。

【实际建模管线（v58 定型，至今唯一在跑）】真实数据 → 造型决策 → parts 列表 → Anatomy 等距 SVG 定型 → 人工评审 → parts.json 内联 → three.js 转换 → 检视档验收。工具是代码：ui-prototype/nodes/<id>/build.mjs 引 /home/zanebono/.agents/skills/anatomy/kit/iso-kit.mjs（3D 常量 → fitProjection → 等距 SVG 线稿）；parts.json 是唯一几何真源（8 站，79–105 件/站）。渲染栈 vendored three.min.js 正交投影（v3 起取代 p5；vendor/p5.min.js 为 v2 遗留已不引用）。

【3D 可视化真实目标】首页 `/` 由三栏信息流改为「脉络地图」：只读、9 站 6 区 9 边、四档变焦、双主题、巨幅地图（平台只是被红圈标出的一块地）、站点即门 + 两处钻取。状态由纯函数 deriveHomeMap()/diagnose() 推导（不落库不存 flag），9 站状态互不排斥，诊断条 12 级优先级。数据红线：缺失显示「—」不填 0 不估算；覆盖率 ≠ 健康率；边只表达运转/堵住/停，不表达吞吐速率；小人 = processing 篇数，1 箱 ≈ 100 篇（1084 → 11 箱）。见 proposal.md、ui-design.md L5-248、L134-216。

【小城市布局真实目标】三层：① 制图语言优先，平台外乡道是制图符号、数据流只有 9 条 ROADS、街道是通行铺装不是数据流（explore-findings.md 2026-11-09/13 核对）；② 空间组织（街巷/院落/门廊/台基 + 纸艺微缩材质线重）；③ 被否定的「工序传送带」解读（v57 用户评「其他都是堆方块了」；08 样板「人在办公写作，不是一条传送机器」）。两条独立制品线：整城 index.html + nodes/<id>/parts.json vs 单栋 module-NN-*.html，同编号不是同一栋，样板未并回城市，城市布局至今未获批（ui-approval: pending）。

【现状】2026-11-16 统一审阅入口 modules-review.html 完成（HTTP 64/64、file 36/36、57 活动源），九栋候选待用户统一风格意见；ui-approval 仍 pending，未实现、未部署。历史操作账本：.pi/harness/events.db 共 15088 条（2026-11-04→11-16），subagent.dispatch 160、gate.check 362、mode.set 45、pin.write 8。

**引用**：openspec/changes/add-home-pipeline-status-page/proposal.md、openspec/changes/add-home-pipeline-status-page/ui-design.md:349、openspec/changes/add-home-pipeline-status-page/ui-design.md:647、openspec/changes/add-home-pipeline-status-page/ui-design.md:708、openspec/changes/add-home-pipeline-status-page/human-feedback.md、openspec/changes/add-home-pipeline-status-page/module-batch-plan.md、openspec/changes/add-home-pipeline-status-page/explore-findings.md、openspec/changes/add-home-pipeline-status-page/ui-prototype/node-decisions.md:24、openspec/changes/add-home-pipeline-status-page/ui-prototype/index.html:349、openspec/changes/add-home-pipeline-status-page/ui-prototype/index.html:25870、openspec/changes/add-home-pipeline-status-page/ui-prototype/nodes/01-feed/build.mjs:1

<!-- pinned 2026-11-16T03:55:51Z -->

## 城市白模基线与已认可工序复用边界

用户认可 07 修复版风格，并授权先做统一尺度城市白模，再按 08/09、01/02/04/05、03/06 顺序逐栋改造，最后统一审阅；这不等于批准整城布局或正式 UI 合同。旧 `ui-prototype/space-skeleton.html` 的 STATIONS 表提供九站地块与道路参考（底板 80×54），不是新城市最终定案。`city-workflow.html` 只把 03 工序接到旧城市；03/06/08/09 各自现有工序封装在独立 HTML，并非直接可导入组件，需提取工位、时间轴和动作身份；09 人工接受前不得自动登记。07 修复 GLB 含地台、街道和远景配景，不能将其当作仅有建筑的独立件直接重复拼入城市。

**引用**：openspec/changes/add-home-pipeline-status-page/ui-prototype/space-skeleton.html、openspec/changes/add-home-pipeline-status-page/ui-prototype/city-workflow.html、openspec/changes/add-home-pipeline-status-page/ui-prototype/blender-pilot-07/preview.html、openspec/changes/add-home-pipeline-status-page/ui-prototype/module-09-consultation.html

<!-- pinned 2026-11-17T01:06:08Z -->
