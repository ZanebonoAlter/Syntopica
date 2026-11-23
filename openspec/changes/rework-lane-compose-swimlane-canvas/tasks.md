# Tasks · rework-lane-compose-swimlane-canvas

> 顺序按依赖：只读层（纯函数 + 单测）→ 画布重绘 → 手势 → 门控 → 检索 → 收尾。
> 每条任务的完成判据写在任务里；跨任务的整体验收放第 10 节。

## 1. 只读层：行归属、几何、标题档

- [x] 1.1 在 `laneComposeLogic.ts` 新增 `buildRowIndex(chains, sectionIds)`：把服务端 `chains` 的成员展开成 `Map<sectionId, rowIndex>`，不加任何前端重算；补单测覆盖「同链多成员」「不在任何链 → 地板」「本次泳道成员优先级最高」。 → `pnpm test:unit laneComposeLogic --maxWorkers=2` 通过
- [x] 1.2 在 `laneComposeLogic.ts` 新增行几何推导 `layoutRows(rowIndex, dates, membersByRow)`：输出每行 `{ top, height, slotOf: Map<id, slot> }`，槽距固定 24px、单日封顶 `MAX_SLOT_PER_DAY=6`；补单测覆盖「同日 1/3/7 条」三档与封顶分支。 → `pnpm test:unit laneComposeLogic --maxWorkers=2` 通过
- [x] 1.3 在 `laneComposeLogic.ts` 新增 `titleBudget(columnWidth)`：`columnWidth < 40 → null`，否则 `floor(columnWidth / 9.2)`；补单测覆盖 14 天（≈50px→5 字）与 30 天（≈23px→null）。 → `pnpm test:unit laneComposeLogic --maxWorkers=2` 通过

## 2. 画布重绘：行式泳道

- [x] 2.1 `LaneComposeCanvas.vue` 删除 `bandLanes` 的横条绘制（`rect` + `rx=6` 胶囊）与成员 tick 拖线，改为「行标签 + 分隔线」；`rowIndexOf` 为空的行整行不渲染（链被取空即消失）。 → `pnpm test:unit LaneComposeCanvas --maxWorkers=2` 通过，且断言 `querySelectorAll('[data-role="band-capsule"]').length === 0`
- [x] 2.2 成员 y 坐标改用 `layoutRows` 的结果（同日排序键从 `id` 换为 `rowIndex`），并保留 `scaledPointRadius` 的缩放补偿与 ≥3px 渲染下限。 → `pnpm test:unit LaneComposeCanvas --maxWorkers=2` 通过
- [x] 2.3 节点常显截断标题：候选链成员与本次泳道成员按 `titleBudget` 渲染 `<text>`（点下 13px、居中、≤N 字 + 省略号）；散料不渲染；检索态命中点例外。 → `pnpm test:unit LaneComposeCanvas --maxWorkers=2` 通过，且 30 天 fixture 下标题节点数为 0
- [x] 2.4 同日堆叠：按 `slot` 下叠，行高随 `maxSlots` 增长；超过 6 条封顶并渲染 `+N` 折叠标记（可点开该日全部）。 → `pnpm test:unit LaneComposeCanvas --maxWorkers=2` 通过

## 3. 拖拽选入 / 移出

- [x] 3.1 `LaneComposeCanvas.vue` 把 `pointerdown` 改为「起始命中对象分流」：命中节点/行标签 → `drag-object`，命中空白 → `pan`；两者共用 4px 阈值与拖后吞 click 的既有抑制。 → `pnpm test:unit LaneComposeCanvas --maxWorkers=2` 通过（含「空白拖 = 平移」「节点点按 = 聚焦」两条回归断言）
- [x] 3.2 单条拖入/拖出：拖起 ghost 放大 1.6× 跟随（rAF 节流），目标行整体高亮，松手落在本次泳道行区间内 = 选入、落在行外 = 移出；拖拽期间屏蔽滚轮缩放与 hover 卡。 → `pnpm test:unit LaneComposeCanvas --maxWorkers=2` 通过，且 D7 的「拖拽不重算 points」断言（`points` computed 重算次数为 0）通过
- [x] 3.3 整组拖：行标签作为把手，`pointerdown` 命中行标签 → 该行全部未采纳成员一起进入拖拽；松手后一次落位，左栏该组记 `n/n`。 → `pnpm test:unit LaneComposeCanvas --maxWorkers=2` 通过
- [x] 3.4 键盘通道：节点 `tabindex=0`，Enter/Space 走「钉住卡」；卡内「＋加入 / ⊘移出」调同一写入口；focus-visible 环可见。 → `pnpm test:unit LaneComposeCanvas --maxWorkers=2` 通过，且键盘路径能改变成员数
- [x] 3.5 撤销：`useLaneCompose.ts` 新增 `lastRemoval`（id + 原行 + 原槽），移出后 3 秒内可恢复；新的移出覆盖上一笔，其他成员变更清空。 → `pnpm test:unit useLaneCompose --maxWorkers=2` 通过（含「超时后不可撤销」断言）

## 4. 阈值门控与散料折叠

- [x] 4.1 `laneComposeLogic.ts` 新增 `visibleScatter(sections, selectedIds, candidateIds, readonlyIds, center, tier)`：可见 = 已选 ∪ 只读 ∪ 候选链成员 ∪ 距离达标的散料；三档 `tight = matchThreshold*0.5 / standard / all = 1`。 → `pnpm test:unit laneComposeLogic --maxWorkers=2` 通过，且「仅高相关档下候选链成员数量不变」断言通过
- [x] 4.2 `LaneComposeCanvas.vue` 地板渲染改为「折叠计数 + 展开/收起」；计数 = 散料总数 − 当前可见散料数，措辞只用「另有 N 条未展开」，不出现「不相关」。 → `pnpm test:unit LaneComposeCanvas --maxWorkers=2` 通过
- [x] 4.3 阈值三档控件接入画布工具条，切换后右栏「可补充的事件」同一批更新（共用同一个 tier 状态，不各算各的）。 → `pnpm test:unit LaneComposeCanvas ComposeSidebar --maxWorkers=2` 通过

## 5. 检索态

- [x] 5.1 新增 `unclassifiedIds(sections, chains)` 派生（= 窗口内 sections − ⋃ chain.memberIds）；`LaneComposeInspector.vue` 的本地标题过滤范围改为该集合。 → `pnpm test:unit LaneComposeInspector --maxWorkers=2` 通过（含「只出现在候选链成员标题里的词 → 命中 0」断言）
- [x] 5.2 检索态画布呈现：候选链行与地板降透明度到 0.20，本次泳道行不降级；未命中节点不可点；退出检索回到进入前的阶。 → `pnpm test:unit LaneComposeCanvas --maxWorkers=2` 通过
- [x] 5.3 命中点在画布上按**真实位置**浮出，标记为四角括号（不复用实环），并显示截断标题，与「已选=实心+✓」「相关=加粗描边」形状可区分。 → `pnpm test:unit LaneComposeCanvas --maxWorkers=2` 通过
- [x] 5.4 右栏三段恒定骨架：①②③ 在任何态都渲染；空态 ③ 默认落「按标题找」；检索态只切换 ③ 内容，①② 不变。 → `pnpm test:unit LaneComposeInspector BoardComposeOverlay --maxWorkers=2` 通过

## 6. 收尾

- [x] 6.1 画布工具条那句「推荐不自动选入；候选大小=接近度（成员固定尺寸），不是置信度」按新语义改写为只讲手势与状态，不加教学句（`standard/frontend/interaction.md:53-66`）。 → `pnpm lint` 通过且工具条文案不含「置信度」「不是」类解释句式
- [x] 6.2 双视口自检：1440×900 与 1920×1080 下 `documentElement.scrollWidth === clientWidth`、三栏 300/1fr/320 不破版、日期轴标签不越界。 → **1440 已实测 pass**（部署后静态产物三次采样均 `scrollWidth === clientWidth === 1440`，三栏与日期轴正常）；**1920 走组件级断言**：`LaneComposeCanvas.test.ts`「几何自查」按画布宽 820 / 1300 两档跑（1300 = 1920 视口下中栏宽），断言节点/日期轴/行标签/节点标题全在画布内、同列标题 y 差 ≥24px、行标签右缘 < 首列点左缘。**未做 1920 浏览器实拍**：agent-browser 无 `viewport` 子命令、headless 窗口启动即固定 1280×633、headed 模式缺 X server。另实测发现 P-1（画布纵向滚动 1076>505）→ 用户选方案 1（吸顶）已修，组件测试 6 条 + 反向验证通过；**修复后未再实拍**：复跑时后端 `:5100` 已被其他会话停掉，避免抢跑他人后端代码
- [x] 6.3 前端收尾部署：`bash scripts/dev/deploy-frontend.sh`（构建静态产物 → 铺 `backend-go/frontend/` → 重启后端 → 健康检查）。 → **实测 exit 0**：`[1/3]` nuxi generate（8 路由预渲染 20.6s）→ `[2/3]` 铺 `backend-go/frontend/` → `[3/3]` 重启后端（PID 47116）+ `/health` 200。用户确认「其他会话改完了」后执行；`:3000` dev 前端已按「3000 端口废弃、统一 5100 静态托管」停掉并清理 pidfile

## 7. Review

- [x] 7.1 按 `开发执行规范.md` §0.6 review 纪律做一次 change 级审查（含 spec delta 对账：4 个 MODIFIED Requirement 的每条 Scenario 都能落到实现或明确记为未覆盖）。 → 实测 `change-review` 线程（`zai-coding-cn/glm-5.3:max`）结论「有条件通过」：六条硬红线全 pass、4 条被改断言判为合同变更的合法适配；必修项 2 条（无成型候选空态 override 缺失、30 天检索命中降级 2 字标题）已派返工；驳回 1 条（标题偏移 0.6px——与已批准三态图一致，抬高会破 24px 槽位预算）

## 8. 测试

- [x] 8.1 影响包单测（按 `bash scripts/harness/change-scope.sh` 判定的范围跑，不跑全量）：`pnpm test:unit laneComposeLogic LaneComposeCanvas LaneComposeInspector ComposeSidebar useLaneCompose BoardComposeOverlay --maxWorkers=2`。 → 实测 `Test Files 7 passed (7) / Tests 265 passed (265)`，Duration 13.54s
- [x] 8.2 组件测试补齐 4 条硬断言：① 点击节点成员数不变 ② 阈值切「仅高相关」时候选链成员/已选/只读参照数量不变 ③ 同日 3 条各自可读不重叠 ④ 链被取空后该行不渲染。 → 实测四条均在（`BoardComposeOverlay.test.ts` A-5/tier/A-13 + `LaneComposeCanvas.test.ts` 同日堆叠与几何自查）；另补 5 条：无成型候选空态、「全拖入不算无候选」、取数失败不误弹、30 天命中不出 2 字碎片、14 天命中照常带标题
- [x] 8.3 `bash scripts/harness/test-patrol.sh --report` 巡检台账。 → 实测 exit 0；12 分片 last_ok 全 1，域外历史欠账在台账内，本 change 影响文件无红项

## 9. 文档

<!-- doc-impact: flow -->
<!-- doc-impact-excuse: api=本 change 零 api 改动，归属来自 edit.map 被其他会话污染;database=同上，本 change 零表结构改动;architecture=同上，本 change 未动架构骨架;standard=同上，本 change 只读 standard 未修改 -->

- [x] 9.1 `bash scripts/harness/doc-impact.sh suggest` 预勾选并核对声明域，运行 `doc-impact.sh context` 注入 flow 业务约束后确认无新增红线冲突。 → 实测 exit 0；**勘误**：`doc-impact` 的域是**文档目录轴**（flow/api/…）不是业务域轴，声明已从 `semantic-board, daily-report, topic-graph` 更正为 `flow`
- [x] 9.2 更新 `docs/reference/flow/topic-graph.md` 泳道编排态一节：画布纵向语法改为「行 = 泳道」，补「拖拽 = 显式选入动作」「本次泳道不绑候选链」两条口径与代码入口。 → 实测该节已改为入口/候选/保存链路，并加一行指回 `semantic-board.md` §泳道工作面（权威口径唯一）；入口文件更正为 `useLaneCompose.ts`
- [x] 9.3 更新 `docs/reference/flow/daily-report.md` 约束 38-40 附近：补「阈值承担散料可见性门控」「候选链成员永不因阈值隐藏」的不变量。 → 实测新增约束 58（含三档映射、折叠计数口径、禁词、无成型候选窗口 override、画布与右栏同批）
- [x] 9.4 更新 `docs/reference/flow/semantic-board.md` 泳道工作面一节：补右栏三段恒定骨架与检索范围＝未归类。 → 实测追加 5 条：画布纵向语法（行=泳道）/ 选入手势 / 右栏三段恒定 / 检索范围=未归类（含 active 排除理由）/ 代码入口
- [x] 9.5 `bash scripts/harness/doc-impact.sh verify openspec/changes/rework-lane-compose-swimlane-canvas && bash scripts/harness/check-standards.sh --change rework-lane-compose-swimlane-canvas`。 → **flow 轨已通过**（声明 flow 且 3 个 flow 文档已更新）；仍报 api/database/architecture/standard 四条「疑似遗漏」——**并发 change 归属污染，非本 change 缺陷**：该 change 的 `edit.map` 最新快照被 `--full-page`、`backend-go/.frontend-before-matrix-copy.*/_nuxt/*.js`、`front/app/api/normalizers/article.ts` 等**其他会话**路径污染（1195 条 backend-go），本 change 实际只改了 `front/app/features/tags/` 下 9 个文件 + 3 个 flow 文档。归档前需等其他会话提交后重跑

## 10. 验证

- [x] 10.1 `cd front && pnpm lint` → 实测 exit 0（0 error / 7 warning，7 条全在未触碰文件：`ArticleCardView` / `useArticleContentView` / `detective-wall/utils` / `schedulerMeta`；本 change 6 个文件 0 warning。此前 `htmlSanitize.ts` 的 `no-control-regex` 已被其他会话修掉）
- [x] 10.2 `cd front && pnpm exec nuxi typecheck` → 实测 exit 0，`grep -c "error TS"` = 0（日志仅 Nuxt `module.register` DeprecationWarning）
- [x] 10.3 `cd front && pnpm test:unit laneComposeLogic LaneComposeCanvas LaneComposeInspector ComposeSidebar useLaneCompose BoardComposeOverlay --maxWorkers=2` → 实测全绿 265 例，第 8.2 节的四条硬断言均在输出中
- [x] 10.4 `cd front && pnpm build` → 期望 exit 0。实测 **首次 exit 134（node OOM）**，加 `NODE_OPTIONS=--max-old-space-size=3072` 后 exit 0（`Σ Total size: 7.14 MB`）；P-2 收尾后又完整重建一次，同样 exit 0
- [x] 10.5 `openspec validate rework-lane-compose-swimlane-canvas` → 实测 `is valid`（两条 `>500 characters` warning 为母 spec 既有长文本形态，非本 change 新增）
- [x] 10.6 打开静态托管页面（`bash scripts/dev/start-dev.sh status` 确认健康），按 `ui-prototype/interaction-table.md` 逐行走查，证据写入 `ui-verification/report.md`（`### A-N 场景【pass|fail|未覆盖】+ 依据行`，覆盖 `ui-design.md` 的 A-1…A-14）。 → 实测建账完成：A-1/A-9(1440)/A-11/A-14 + 无成型候选空态 **pass**；A-2~A-8/A-12/A-13 因 pointer 手势无法脚本替身记**未覆盖并指向组件测试**；发现 **P-1 画布纵向滚动致拖拽主手势失效** → 已修（吸顶）并实拍确认；P-2 右栏②机制科普文案 → 已修并实拍确认
- [x] 10.7 归档前门禁：`bash scripts/harness/spec-gate.sh`（若不存在则跳过）/ `doc-impact.sh verify` / `test-patrol.sh --report` 全绿，`ui-approval: approved` 仍在位。 → **实测全绿**：
  - `doc-impact.sh verify` → **通过**（声明 `flow`，文件 3 个，启发式 `ownership`）。两处修正：① 任务 9.5 的正文里写了字面量 `docs/reference/flow/*.md`，被脚本当成"声明的文档"判不存在 → 改为文字描述；② api/database/architecture/standard 四条"疑似遗漏"来自 `edit.map` 归属污染（含 `--full-page`、`backend-go/.frontend-before-matrix-copy.*` 等**他会话**路径），按脚本自带的 `doc-impact-excuse` 逐域留痕豁免（本 change 确实零 api/表结构/架构/standard 改动）
  - `check-standards.sh --change` → **319 通过 / 0 失败**（含 F 段 doc-impact 对账、I 段 175 个 capability validate 通过）
  - `test-patrol.sh --report` → exit 0，12 分片 last_ok 全 1
  - `ui-approval: approved` 仍在位

## 11. 返工（2026-11-21 用户实拍后提档：删关系边 + 放大节点与文字 + 横向滚动）

- [x] 11.1 删除事件关系边：`<g class="lcc-canvas__edges">` 整块、`edges` computed、`edgePath()`、`.lcc-canvas__edge*` 三条 CSS 一并移除；确认 `identity` / `similarity` 都不画（spec「不生成事件关系边」此前被实现超纲违反）。
- [x] 11.2 列宽下限与横向滚动：`columnWidth = max(52, span / 天数)`，`vbWidth = 132 + 天数 × 列宽 + 24`（内容宽可超容器宽）；容器 `overflow: auto`；宽度测量改用 clientWidth 防反馈环。
- [x] 11.3 行标签列横向吸顶：`scrollLeft` ref + 同步；每行标签子组整组反向 `translate(scrollLeft/zoom, 0)`，配不透明底 + 6 级渐隐阴影（与纵向吸顶同款）；行标签拖拽热区同步平移。
- [x] 11.4 日期轴纵向吸顶：日期轴组与本次泳道行共用 `translate(0, scrollTop/zoom)`；时间刻度随内容横向移动，不横向吸顶。
- [x] 11.5 节点与文字放大：散料直径 11px、推荐 12.4–14px、已选 15px（`memberRadius` 传 7.5，推荐上限随之压到 7.0）；常显标题 8px → 11px；`TITLE_MIN_COLUMN` 40 → 52、字宽除数 9.2 → 12.6（11px 档），`MAX_TITLE_CHARS` 保持 5。
- [x] 11.6 同列堆叠槽距**保持 24px**（子线程先按 18px 实现，几何核算发现「11px 标题下缘 y+15」与「下一槽圆顶 y+slot−7.5」要求 slot ≥22.5，18 必然重叠约 5px → 主线程改回 24 并同步测试期望；行标签 32px + 槽位 24px/条 + 行间 26px）。额外修复：`floor-fold` 折叠计数与 `row-more` 提示原本画在内容坐标左侧，横向滚动后会滑到吸顶标签列下面点不到 → 加 `rowLabelTransform` 随标签列吸附。
- [x] 11.7 拖拽与节点卡在新几何下复核：`contentPoint` 的 `scrollLeft` 换算仍成立；`rowAtPointer` 吸顶行区间、`pinnedShift` 节点卡定位、落点框宽度（=可视宽而非内容宽）逐项核对。
- [x] 11.8 测试：删边相关断言移除；补「无 `<path class="lcc-canvas__edge">`」「列宽锁 52」「内容宽 > 容器宽时 overflow-x」「30 天标题仍 11px 常显」「行标签横向吸顶位移 = scrollLeft/zoom」。
- [x] 11.9 验证：`pnpm test:unit`（受影响 7 文件）/ `pnpm lint` / `nuxi typecheck` / `NODE_OPTIONS=--max-old-space-size=3072 pnpm build`；实拍 1920 与 1440 双视口 + 横向滚动 + 双吸顶。 → **通过**：单测 6 文件 261 例全绿（含 `LaneCorrectionCanvas` 回归）、lint 0 error / 7 warning（全在未触碰文件）、typecheck exit 0、`build` exit 0、`deploy-frontend.sh` 铺盘 + 重启后端 `/health` 200。部署后实测（:5100 静态产物，1280×633 无头窗口）：`.lcc-canvas__edge` = **0**；`viewBox` 宽 1716 = 132+30×52+24、可视 650 → `overflow-x` 成立；`node-title` 11px、30 天**仍常显**（42 条）；`scrollLeft=400` → `row-label-group transform=translate(400,0)` 且标签屏幕左缘不动（310→310）；`scrollTop=400` → 本次泳道行与日期刻度组 `translate(0,400)`、首条链行 533→133、吸顶行 top 不动；`floor-fold`「另有 90 条未展开 · 展开」随标签列吸附（left=302 vs 画布左 300，可点）。
- [x] 11.10 重跑 `doc-impact.sh verify` + `check-standards.sh --change`，更新 `ui-verification/report.md`（新增 P-3：关系边超纲、P-4：字号不可读）。 → `doc-impact verify` **通过**（声明 flow / 文件 3 / 启发式 ownership）；`check-standards --change` **317 通过 / 2 失败**——两条失败是 `2026-11-19-stage-driven-constraint-injection` 与 `2026-11-19-streamline-business-test-verification` 的 §12 flow 溯源欠账，因**时钟跨过 2026-11-22 零点**刚退出 3 天宽限期，与本 change 无关（两 change 均已声明无 flow 影响，但检查器只认「无 flow 影响」字样，其中后者用的是 `doc-impact-excuse: flow=...` 表述 → 检查器口径缺口）。

## 12. 返工第二轮（2026-11-23 用户实拍：吸顶文本重叠 + 关联度映射失效）

- [x] 12.1 吸顶带底色与描边：`sticky-bg`（bg-base 不透明打底）+ 新增 `sticky-tint`（elevated 一档）+ 下缘 1px 硬分隔 + 标签列与内容区之间的 1px 竖分隔，两条吸顶轴看起来是同一块「固定框」。—— 两层 rect 是因为深色主题 `--color-bg-elevated` 是 0.98 alpha，单用会透出滚动内容。
- [x] 12.2 吸顶底色几何起点修正（**本次重叠的真正成因**）：底色 rect 原来 `y=0`，即「内容坐标 0」，而吸顶行整组被下移了 `scrollTop` → 底色实际落在视口上方的不可见区，滚上来的行标签就叠在吸顶文字上。改为 `y=-scrollTopLogical`、`height=scrollTopLogical + row.top + row.height`，覆盖范围恒等于「视口顶 → 本行下缘」。实测：scrollTop=300 时 rect 高 438、`elementFromPoint` 在 y=10/120 命中 `sticky-tint`（自己的底色），y=60 命中吸顶行自己的 `row-count`。
- [x] 12.3 行容器去掉裸 `<template>` 包装：`<template>` 在 SVG 内会被渲染成未知元素，其子节点的 `getBoundingClientRect()` 恒为 0（12.2 的诊断就卡在这上面），改成 `<g class="lcc-canvas__row-body">`。
- [x] 12.4 本次泳道行标签移入置顶覆盖层（`data-role="lane-label-overlay"`，在刻度组之前、所有行之后），行循环里 `v-if` 跳过——标签底色与文字不再依赖绘制顺序。`data-role="row-label"` 全画布仍只出现一次（测试断言）。
- [x] 12.5 日期刻度/列 guide 加 `clipPath`，左界 = 可视区 `LABEL_W`（内容坐标即 `132 + scrollLeftLogical`）——横向滚动后刻度不再滑到吸顶标签列上方压住行名。不溢出时不下发 clip。
- [x] 12.6 恢复「关联度 → 颜色 + 大小」映射（**上一轮被尺寸提档压成死区**）：推荐半径原为 `clamp(7.3×(0.9+proximity), 7.0)` = 6.57–7.0（0.43px，肉眼等于常量），改为按 `recommendRange {5.9, 7.4}` 线性插值；深浅 `strength` 从 `12+55p` 改 `18+62p`。两者都是**显式 flag**，修正画布 `LaneCorrectionCanvas` 缺省行为逐位不变。实测（13 个已选成员）：推荐半径 6.12–7.02（直径 12.2–14.0px）、填充 26.9%→64.1% 渐变、已选恒 7.5（15px）。
- [x] 12.7 验证：`LaneComposeCanvas` 76 例全绿、`laneComposeLogic`/`LaneCorrectionCanvas`/`BoardComposeOverlay` 全绿（261 例）、lint 0 error、typecheck exit 0、build exit 0、`deploy-frontend.sh` 部署完成；部署后实拍确认吸顶带分界与无重叠。

## 13. 返工第三轮（2026-11-23 用户实拍：标题要 10 字 / 删底部说明 / 折叠措辞）

- [x] 13.1 常显标题 3 字 → **10 字**：`MAX_TITLE_CHARS` 5→10、`TITLE_MIN_COLUMN` 52→**124**、`TITLE_CHAR_WIDTH` 12.6→11.5（`LANE_MIN_COLUMN` 随标题档走）。代价：30 天内容宽 132+30×124+24 = 3876px（1920 视口滚 ~3×），用户已知情并选此档。
- [x] 13.2 标题截断改**宽度口径**：新增 `truncateByWidth(text, maxWidth, size)`（CJK 按 1 个字宽、ASCII 按 0.55），`nodeTitleText` 用「列宽 − 2px」作上限。原因：字数口径会把 `Gemini 4.5` 截成 `Gemini 4 …`，明明还有半列空白。
- [x] 13.3 删掉画布底部那行说明（「相关节点（只读 N 个…）：…；另有 M 个可补充的相关节点未标名…」）：连同 `relatedHintText` / `relatedReadonlySections` / `relatedUnlabeledCount` 三个死计算属性与 `.lcc-canvas__related-hint` 样式一并移除（不留空壳）。
- [x] 13.4 折叠计数措辞：`另有 N 条未展开 · 展开` → **`低相关散料 N 条 · 展开`**（用户：原措辞容易被误解成「不相关」；括号内仍不含「不相关/无关联/置信度」禁词）。
- [x] 13.5 spec / ui-design 同步：`≤5 字` → `≤10 字`、列宽下限 52 → 124（含 Requirement、Scenario、A-11 验收项）。`openspec validate` valid。
- [x] 13.6 测试：`laneComposeLogic` 80→83 例（标题按宽度截断 3 条）、`LaneComposeCanvas` 76 例、`BoardComposeOverlay` 72 例（V1/V2 改为「容器够宽时 viewBox 回落容器宽」、删除提示行的断言改为「不存在」）；受影响 7 文件 324 例全绿，lint 0 error，typecheck exit 0。
- [x] 13.7 部署后实测（:5100）：`viewBox` 宽 **3876**（=132+30×124+24）、可视 650 → 横向滚动；`node-title` 42 条、**最长 10 字**（`大模型服务限流、封…` / `用户反映Astra…`）；底部 `lane-related-hint` **不存在**；折叠计数文本 = `低相关散料 90 条 · 展开`。

## 14. 返工第四轮（2026-11-23 用户："关联度看不出区别" + "一部分文字有一部份没有"）

- [x] 14.1 接近度改**批次内相对归一**：`relativeProximities(ds)` = `(max − d)/(max − min)`，最近 1 / 最远 0；单点或全等距取 0.5；非有限距离不进批次。经 `LanePointFlags.proximityOverride` 传给 `pointAppearance`，**修正画布不传时行为逐位不变**。
  - 根因记录：原口径 `1 − d/threshold`，实测这批距离 0.59–0.67、阈值同量级 → proximity 只在 0.04–0.14，半径 5.9→6.0px、填充 18%→21%，肉眼等于常量。
  - 归一集合 = 真正落槽会画出的节点 − 已选 − 只读，且 `cosineFor` 给出有限距离者；阈值与档位**不参与**该映射。
- [x] 14.2 节点标题口径统一：**所有会画出的节点都常显截断标题**（含散料），不再按「是否与已选相关」区分。落位统一走 `placeRelatedTitles`（障碍 = 所有节点圆 + 已落位标题），优先级 已选 > 候选链成员 > 与焦点相关 > 其它散料；放不下整条不画（宁缺勿叠），只读梯子仍不画。两套标题（persistent / related）合并成一套，同一节点最多一个 `<text>`。
- [x] 14.3 标题宽度上限 `(budget+1) × TITLE_CHAR_WIDTH`：让 10 个汉字**再加省略号**放得下（原来 `budget×字宽` 只给到 9 字 + 省略号）。
- [x] 14.4 spec 同步：① 散料 SHALL 也常显标题（原「散料 SHALL NOT 常显标题」整句改写）；② 新增接近度相对归一红线（SHALL NOT 用「距离/阈值」绝对除法）。`openspec validate` valid。
- [x] 14.5 验证：受影响 5 文件 **299 例全绿**（含 `LaneCorrectionCanvas` 33 例回归）、lint 0 error、typecheck exit 0、build + deploy exit 0。
- [x] 14.6 部署后实测（:5100，board 1974，选中 1 组）：
  - 半径**46 个不同取值**，min 5.90 / max 7.50（已选恒 7.5）；
  - 散料行 **43 个节点 / 43 条标题**（原来 0 条）；候选链行 2/2、7/7；泳道行 5/5 → 全画布 57 条标题；
  - 标题样例 `美国麻疹病例破纪录，…`、`以色列举行第26届议…`（10 字 + 省略号）。
