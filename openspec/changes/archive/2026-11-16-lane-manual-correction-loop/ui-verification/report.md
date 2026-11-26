# UI 验证建账 — lane-manual-correction-loop

> 制品：`openspec/changes/lane-manual-correction-loop/ui-verification/report.md`
> 契约：`ui-design.md` Acceptance A-1~A-12 + `test-cases.md` §5 UI 机械断言
> 证据：截图落 `ui-verification/`（本地留存、不入 git）
> 工具：agent-browser（无头 Chromium，真实后端 :5100 + 真库）
> 执行日期：2026-11-15

## 0. 环境与边界

| 项 | 值 |
|---|---|
| 后端 | :5100（静态托管前端 + API 同端口；`bash scripts/dev/deploy-frontend.sh` 铺盘 + 重启） |
| 前端 dev server | :3000（仅 HMR 用，本次验证走 :5100 静态产物） |
| 数据库 | 真库（Docker PG + pgvector，库 `syntopica`） |
| 浏览器 | agent-browser（Chromium via CDP），会话 `laneui-6d5795ec8cb0` |
| 样本数据 | 版块 1980「生成式 AI 与大模型厂商」（13 个版块中话题最多的一个）；泳道 971「具身智能与世界模型」（manual 来源、8 名成员、跨 11-08~11-14 七天、11-13 一列 3 个成员——触发同列竖向堆叠） |
| 真库写操作边界 | **只做「移除一条 → 看到反馈 → 加回还原」闭环**；首义重设**只走到确认条出现，不点提交**（提交不可逆且污染真库审计流；写路径已有 handler + repository 测试覆盖） |

### 真库写操作边界说明（主线程裁定）

- **允许**：解除一条 section 的泳道归属 → 观察界面反馈 → 加回还原（闭环后归属复原）。
- **不允许**：提交首义重设（`PATCH /api/daily-reports/topics/:id/anchor` 会追加 `topic_anchor_overrides` 留痕行且不可逆）。
  本次只走到「勾选节点 + 确认条」这一步即停，**不点确认**。
- **不允许**：泳道改名提交（写 `board_persistent_topics.label`）、版块改名保存（写 `semantic_labels.label`）——就地编辑只验证到「输入框出现 / Esc 取消 / 零弹窗 / 未发请求」。
- **允许**：归档泳道弹窗只打开并**取消**二次确认（未产生任何写操作）。

### 部署备注

`bash scripts/dev/deploy-frontend.sh` 构建 + 铺盘 + 重启后端成功（3m04s），但脚本自带的 60s 健康检查窗口没赶上本机启动耗时（后端打印「✗ 后端 60s 内没起来」），实测 **1 分钟后 :5100 `/health` 已 200**，dev 栈正常。

## 1. §10 测试结果（机械复核，先于 UI 验证）

| 任务 | 命令 | 结果 |
|---|---|---|
| 10.1 后端 repository 层 | `cd backend-go && go test -count=1 ./internal/topicgraph/...` | ✅ 全绿（`repository` 316.9s / `service` 218.0s / `handler` 139.2s 全 ok） |
| 10.2 handler 层 | 同上（`./internal/topicgraph/handler/...`） | ✅ 全绿（成功路径 + 各类 4xx + 400 参数非法均在 `daily_report_anchor_handler_test.go` / `daily_report_anchor_read_handler_test.go` / `daily_report_assignment_handler_test.go` 覆盖） |
| 10.3 版块缓存失效回归 | `cd backend-go && go test -count=1 ./internal/tagmanagement/...` | ✅ 全绿（`tagmanagement` 44.5s / `handler` 173.5s / `repository` 94.0s / `service/board` 240.8s 等 ok；`models`、`service/auxlabel`、`service/entity` 首轮因 **testcontainers 并行启动超时**失败，`-p 1` 串行复跑分别 8.5s / 64.7s / 41.5s 全绿 → 环境并发噪声，非断言失败） |
| 10.4 前端组件测试 | `cd front && pnpm test:unit <12 个受影响文件> --maxWorkers=2` | ✅ 12 files / 103 tests 全绿（含本批新增的 `boardEditFields.test.ts` 9 例） |
| 10.5 agent-browser 端到端主链路 | 见 §3 | ✅ 落点达成（矩阵行 ⋯ → 修正工作面 → 勾首义节点 → 剔除一条 → 移除反馈 + 已移出（本次）→ 加回还原 → 关闭） |

> 10.3 说明：树莓派 4 核上 `go test` 默认并行跑包，多个 testcontainers 同时拉起 PG 容器会撞 Docker API 启动超时（`context deadline exceeded`，120s）。串行复跑（`-p 1`）三个包全绿，**非本 change 断言失败**。
>
> 10.4 文件清单：`LaneCorrectionCanvas.test.ts` / `LaneMemberList.test.ts` / `LaneAnchorScope.test.ts` / `BoardComposeOverlay.correction.test.ts` / `useLaneCorrection.test.ts` / `ObservationMatrixLaneRow.test.ts` / `BoardRadarStory.test.ts` / `TopicManageDialog.test.ts` / `BoardQuickEditDialog.test.ts` / `LaneArchiveDialog.test.ts` / `boardEditFields.test.ts` / `api/dailyReports.test.ts`。
> `boardEditFields.ts` 原无单测文件，本批补 `boardEditFields.test.ts`（字段表齐备 / 草稿生成 / 校验两分支 / 载荷映射共 9 例）。

## 2. A-1~A-12 结论表

| # | 场景 | 结论 |
|---|---|---|
| A-1 | 从矩阵行 ⋯ 进画布 | **pass** |
| A-2 | 移除节点 | **pass** |
| A-3 | 设定首义范围 | **未覆盖**（勾选与按钮态已验；确认条按契约仅在覆盖人工首义时出现——真库 6 条泳道全为 auto 故未触发，属**契约口径**非缺陷；提交步按**真库写边界**未执行，替代证据：daily_report_anchor_handler_test.go 成功+4xx 全套） |
| A-4 | 画布只读/选入分离 | **pass** |
| A-5 | 窄屏 | **pass** |
| A-6 | 雷达行 ⋯ → 编辑版块 | **未覆盖**（弹窗已验；保存步按写操作边界未执行） |
| A-7 | 加回节点 | **pass** |
| A-8 | 标题就地改名 | **未覆盖**（输入框/Esc 已验；提交步按写操作边界未执行） |
| A-9 | 界面文字纪律 | **pass** |
| A-10 | 左栏超高可达 | **pass** |
| A-11 | 展示字段溯源 | **pass** |
| A-12 | 窗口外首义折叠带 | **未覆盖**（自动首义侧已验；人工首义侧需写库才能触发） |

统计：**pass 8 / fail 0 / 未覆盖 4**。未覆盖两类原因分陈：①**真库写边界**（A-6/A-8/A-12 的提交步：不可逆写操作不碰真库，替代单测证据在正文）；②**契约口径 + 写边界**（A-3：确认条仅在覆盖人工首义时出现，真库泳道全为 auto 未触发；提交步同①）。D-1 压字缺陷已修复（描边光晕，双视口复核，见 §5）——修复前 A-1 相关「节点不重叠压字」一度不达标，修复后 42/42 标题带光晕可读，A-1 维持 pass。

---

### A-1 从矩阵行 ⋯ 进画布【pass】

依据：矩阵行头 `lane-menu-trigger-971` → 菜单「编辑泳道」→ `lane-correction-left` 与 `lane-correction-canvas` 同时渲染；
`document.elementFromPoint(画布中心)` 命中 **`svg.lcc-svg`**（画布本体，非遮罩）；
`.bco__workbench--correction` 子节点**恰为 2 个**：`bco__left`（宽 **380px**）+ `lcc-center`（1060px @1440 / 1540px @1920）；
页面内 **不存在 `.bco__right`**（`rightColumnExists: false`）；左栏承载全部三块（成员 / 关联推荐 / 首义范围）；
`document.documentElement.scrollWidth == innerWidth`（无横向溢出）；
画布 42 条节点标题全部渲染（`.lcc-label` 42 = `.lcc-node` 42），且带描边光晕（`paint-order: stroke`，D-1 修复，见 §5）。
截图：`02-correction-workface-1440x900.png`、`04-correction-workface-1920x1080.png`；D-1 修复前后对比：`d1-before.png` / `d1-after.png` / `d1-before-1920x1080.png` / `d1-after-1920x1080.png`。
第二入口同样成立：顶栏「板块管理 → 泳道管理」弹窗内点「编辑」→ 进工作面（标题＝Agent 架构重构与标准化协作生态，8 名成员）。

### A-2 移除节点【pass】

依据：左栏取消勾选 `lane-left-check-8174` →
**弹窗数保持 0**（`document.querySelectorAll('.app-dialog').length === 0`，**未弹任何去处选择器**）；
toast = 「**已移除，该内容不再归属任何泳道，仍可再参与候选**；归属立即生效，重心次日日报生效」（同时含「不再归属任何泳道」与「仍可再参与候选」）；
节点从成员清单移出并进入「**已移出（本次）**」（`lane-removed-group` 出现、`lane-removed-8174` 就位），画布该节点出现 ⊘ 标记（`.lcc-cross` ×1）；
成员计数 8 条 → **7 条**（`lane-members-count` 文案同步）；
网络面板：`PATCH /api/daily-reports/sections/8174/assignment` → **200**。
截图：`03-removed-state-1440x900.png`。

### A-3 设定首义范围【未覆盖】

已覆盖：勾选 2 个节点 → 按钮文案变为「**用选中的 2 个节点定首义**」（提交态可用）、画布出现 **2 个虚线首义环**（`.lcc-anchor-ring` ×2）、左栏两个复选框呈勾选态；全过程弹窗数 0。
截图：`08-anchor-selected-1440x900.png`。

未覆盖：**确认条出现 → 提交 → toast 含「次日」**。原因：确认条只在**覆盖已有手动首义**时出现，而真库 6 条泳道首义来源全为自动
（`GET /api/daily-reports/topics/{90,93,94,88,89,971}/anchor` → `source: auto`、`section_ids: []`、`override_count: 0`），
首次人工重设**直接提交** → 提交即写 `topic_anchor_overrides`（不可逆 + 污染真库审计流），按主线程写操作边界**不点提交**；
且「自动首义」场景下点按钮会直接落库，无法停在「确认条出现」这一步。
替代证据（单测）：`useLaneCorrection.test.ts:186`「已有首义时先出画布内确认条（不叠弹窗）」、`:173`「提交选定节点 → 来源变「人工（选定的 N 个节点）」+ toast 含「次日日报生效」」。

### A-4 画布只读/选入分离【pass】

依据：点画布节点 `lane-node-8174` → **成员数 8 → 8 不变**（取消勾选才改变）、弹窗数 0；
节点卡 `lane-node-card` 出现并跟随：标题「Claude深夜无人值守自行采购机械臂并调配颜料作画」+「2026-11-08 · **接近度 0.89**」+「**本泳道成员**」+「非首义来源节点」（状态结论，无教学语）。

### A-5 窄屏【pass】

依据：视口 375×667 → `lane-correction-narrow` 出现，文案**恰两句**：「泳道修正需要宽屏或横屏」/「请在宽屏或横屏上打开。」；
`lane-correction-left` 与 `lane-correction-canvas` **均不渲染**（不画残缺两栏），`.bco__workbench--correction` 不存在；
`documentElement.scrollWidth 375 == innerWidth 375`（**无横向溢出**）。
轻量弹窗仍可用：`md` 档「编辑版块」在 375 宽下宽 **345px**（92vw 上限）、left 15 / right 360 **完全落在视口内**，无横向溢出，保存按钮可见。
截图：`05-narrow-375x667.png`、`07-board-quick-edit-375x667.png`。

### A-6 雷达行 ⋯ → 编辑版块【未覆盖】

已覆盖：雷达行尾 ⋯（`radar-menu-trigger-1980`）→ 菜单「编辑版块」→ 弹窗**宽 560px**（`style="--dialog-w: 560px"`，md 档）；
`document.elementFromPoint(弹窗中心)` 命中**弹窗本体内部元素**（`TEXTAREA.bqed-textarea`，非遮罩）；
四字段齐备（`bqed-label` / `bqed-description` / `bqed-display_order` / `bqed-protected`）+ `bqed-save`；
点行尾 ⋯ **未触发行跳转**（URL 仍为 `http://localhost:5100/tags`）；无横向溢出。
截图：`06-board-quick-edit-1440x900.png`。

未覆盖：改名称 → 保存 → toast + 雷达行更新（保存会写真库 `semantic_labels.label`）。
替代证据（单测）：`BoardQuickEditDialog.test.ts`「保存：载荷含名称/描述/排序/受保护，成功后提示并上报 saved」。

### A-7 加回节点【pass】

依据：「已移出（本次）」点 `lane-restore-8174` → toast = 「**已加回**」；
成员计数回到 **8 条**、「已移出（本次）」分组消失、画布 ⊘ 标记消失；**全程弹窗数 0**（未叠任何选择器）；
网络面板第二条 `PATCH .../sections/8174/assignment` → 200；
真库复原核对（psql 只读 SELECT）：`persistent_topic_id = 971` 复原，`lane_tier = l1_direct` 保留，`topic_match_confidence = manual`（加回口径），
`topic_match_distance = 0.1267`（实算距离），`manual_assignment_action = attach`（留痕）。

### A-8 标题就地改名【未覆盖】

已覆盖：点标题 `lane-correction-name` → **就地变输入框** `lane-correction-name-input`（预填当前名「具身智能与世界模型」），**全程弹窗数 0**（不弹对话框）；
改写为「临时改名测试」→ 按 **Esc** → 输入框消失、标题复原为「具身智能与世界模型」；
网络面板该轮只有 GET（`/anchor`、`/lifeline`），**无 PATCH** → 未写库。

未覆盖：改写 Enter → 标题更新 + toast「泳道名已更新」（提交会写真库 `board_persistent_topics.label`）。
替代证据（单测）：`BoardComposeOverlay.correction.test.ts:119`「点标题变输入框 → Enter 保存 → toast「泳道名已更新」，全程零弹窗」、`:140`「Esc 取消就地编辑（不写库）」。

### A-9 界面文字纪律【pass】

依据：工作面全文（1517 字）扫描禁用表述 → **命中 0**（细圈 / 实心 + 勾 / 不是置信度 / 点节点 = / 重心 = / 滚动均值 / 每天重算 / 草稿 / 画布只做展示 / 请在左栏 / 要在左栏 / 接近程度（）；
图例**恰 4 项**且每项 ≤8 字（成员 / 大小=接近度 / 虚线=首义 / ⊘已移出）；
页脚只有**两枚结论型徽标**：「归属 · 立即生效」「次日日报生效」；
节点卡只有状态结论（标题 / 日期 · 接近度 / 本泳道成员 / 非首义来源节点），无教学语。

### A-10 左栏超高可达【pass】

依据：切「近30天」窗口 → 成员 19 条；**块内清单独立滚动**（成员块 scrollHeight 570 > clientHeight 190，滚动到 380）；
「用选中的 N 个节点定首义」按钮**始终完整落在左栏内**（top 601 / bottom 631 ⊂ 左栏 88..868）且在视口内 → 块内滚动不裁按钮；
左栏 `overflow-y: auto` 且 scrollHeight == clientHeight（781）→ **不存在「报告无溢出却滚不到」的假可达状态**。

### A-11 展示字段溯源【pass】

依据：界面内「疑似串味 / 成员偏多 / 疑似 / 偏多」等无来源装饰徽标 **命中 0**；
展示字段逐项可溯源：泳道名（`board_persistent_topics.label`）· 成员计数（窗口内归属 section 数，与清单同源）·
成员行日期/标题（`period_date` / `cluster_label`）· 首义来源（`topic_anchor_overrides` 有无）· 接近度（成员取 `1 − topic_match_distance`，候选取成员向量均值实时算）·
节点状态（归属态 + 本次会话移除集）· 生效时机徽标（契约语义）。

### A-12 窗口外首义折叠带【未覆盖】

已覆盖（**自动首义侧**）：首义来源 = auto → 画布**无** `lane-fold-band`、左栏**无**「更早」分组（`lane-anchor-earlier-group` 不存在）→ 符合「自动首义 SHALL NOT 出现折叠带、不虚构节点列表」。

未覆盖（**人工首义侧**）：折叠带渲染 / 虚线分隔 + 节点数 / 首义虚线圆点非比例堆叠 / ≤6 +「+M」/ 带内可点可勾 / 选择池 = 窗口内节点 + 当前首义集。
原因：需要真库存在「人工首义且含窗口外节点」的记录才会触发，造这条记录必须提交首义重设（写 `topic_anchor_overrides`，不可逆 + 污染审计流）→ 按写操作边界不执行。
替代证据（单测）：`LaneCorrectionCanvas.test.ts:109`（折叠带渲染）/ `:119`（带内可勾选）/ `:125`（≤6 +「+M」）/ `:134`（无窗外节点时不出现）；
`useLaneCorrection.test.ts:212`（窗外节点进「更早」分组）/ `:224`（更早的其它历史节点不在选择池内）。

## 3. 端到端主链路落点（10.5）

链路：**矩阵行 ⋯ → 修正工作面 → 勾首义节点 →（确认条：真库首义为自动，提交即写库，故止步于勾选态）→ 剔除一条 → 移除反馈 + 已移出（本次）→ 加回还原 → 关闭**。

| 步 | 动作 | 状态断言 | 证据 |
|---|---|---|---|
| 1 | 矩阵页打开 `/tags/1980` | 6 条泳道行头带 `⋯` 入口 | `01-matrix-1440x900.png` |
| 2 | 行头 `⋯` → 「编辑泳道」 | 工作面打开；两栏（380px 左栏 + 画布）；无右栏；`elementFromPoint` 命中画布 | `02-correction-workface-1440x900.png` |
| 3 | 左栏「首义范围」勾 2 个节点 | 按钮文案「用选中的 2 个节点定首义」；画布 2 个首义虚线环；弹窗数 0 | `08-anchor-selected-1440x900.png` |
| 4 | 取消勾选成员 `8174`（剔除一条） | toast「已移除，该内容不再归属任何泳道，仍可再参与候选…」；**弹窗数 0**；成员 8→7；节点进「已移出（本次）」；画布 ⊘ 标记 | `03-removed-state-1440x900.png` |
| 5 | 「↩ 加回来」 | toast「已加回」；成员回到 8；分组消失；⊘ 消失；弹窗数 0；PATCH 200；真库 `persistent_topic_id=971` 复原 | 同上（状态断言） |
| 6 | 「← 返回」关闭 | 工作面消失，回到矩阵页 | `09-closed-back-to-matrix-1440x900.png` |

双视口：`04-correction-workface-1920x1080.png`（左栏恒 380px、画布随容器放大到 1540px、1:1 不拉伸、无横向溢出）、
`10-radar-1920x1080.png`（雷达页 1920 宽无横向溢出）。

## 4. 与批准原型的差异说明（§8.3）

以下 6 条为**主线程已裁定的既定差异**，本次逐条**照录并核实**（均非信息架构 / 主流程 / 布局模式级差异，**不重置 `ui-approval`**）：

| # | 既定差异 | 核实结果 |
|---|---|---|
| ① | 顶栏无保存按钮（逐条即时落库） | ✅ 修正工作面顶栏只有「← 返回 / 泳道名（可就地编辑）/ 日期窗口 / 取消聚焦」，**无保存按钮**；写操作逐条即时落库（移除 = `PATCH .../assignment`、加回 = 同端点相反载荷、首义 = `PATCH .../anchor`，各一次） |
| ② | 移除 toast 带生效尾巴 | ✅ 实测 toast =「已移除，该内容不再归属任何泳道，仍可再参与候选；**归属立即生效，重心次日日报生效**」 |
| ③ | 窄屏提示只两句 | ✅ 375×667 下窄屏文案恰两句：「泳道修正需要宽屏或横屏」/「请在宽屏或横屏上打开。」 |
| ④ | 画布同列按列堆叠 | ✅ 同日期多节点在**同一列纵向堆叠**（11-13 一列 3 个成员，y = MEMBER_Y + slot×16） |
| ⑤ | 补 GET anchor 读端点 | ✅ `GET /api/daily-reports/topics/:id/anchor` 已注册并被工作面调用（network 面板多次 200，返回 `source/section_ids/override_count/effective_from`） |
| ⑥ | 选择池不含本次已移出节点 | ✅ 选择池 = 窗口内本泳道节点 + 当前首义集；已移出节点不在池内。**本批另修**：移除后首义选择集未同步剔除（计数偏大，见 §5 D-2），修后实测移除 1 个 → 按钮由「用选中的 2 个节点定首义」变「用选中的 1 个节点定首义」 |

**结论**：未发现信息架构 / 主流程 / 布局模式级差异 → **`ui-approval` 维持 `approved`，无需重新请用户确认**。

## 5. 缺陷清单

| # | 级别 | 标题 | 处理 |
|---|---|---|---|
| D-1 | 中 | 画布节点标记（成员勾号圆点）压住邻近节点标题文字 | **已修**（主线程裁定「标题描边光晕」，代码 + 单测 + 双视口 e2e 复核） |
| D-2 | 小 | 移除节点后首义范围选择集未剔除 → 按钮计数偏大 | **已修**（代码 + 单测 + e2e 复核） |

### D-1（已修）：画布节点标记压字

现象：1440×900 与 1920×1080 两档视口下，画布**成员勾号圆点（红底白勾）**会压在同一日**下一行成员**的节点标题文字上，
被压处的字符呈「灰字叠红底」，可读性下降。ui-design「1440×900 视口验收要点」明写「中央画布**节点不重叠压字**」→ 该条不达标。

量化：画布 42 个节点标题中 **22 个**与某个节点标记圆发生几何重叠（in-page `getBBox()` × 节点圆包围盒比对）。

成因：标题落位（`LaneCorrectionCanvas.vue` 的 `labels` computed）只把**其它标题盒**放进冲突表（`placed`），
不把**节点标记圆**当障碍；同一日期的多个成员按 16px 行距竖向堆叠时，上方节点的标题盒（起点 = 节点 y + r + 5，高约 28px）
必然覆盖下方成员的标记圆。
批准原型是同一套算法（`ui-prototype/index.html:1606` 只做标题间冲突判定），但其 fixture 每列只有 1 个成员，故未暴露该现象——**本条不是实现相对原型的偏差**。

修法三选一（均属画布布局级改动，初版按「不自处置」纪律上报）：
① 标题落位把节点标记圆也纳入冲突表（宁缺勿叠）——代价：约一半标题会被隐去；
② 给标题加描边光晕（`paint-order: stroke`）保可读——代价：光晕会反过来遮住勾号；
③ 加大同列成员竖向间距 / 成员标题改侧排——代价：成员区高度与画布分区需重排。

**主线程裁定：采用 ②（标题描边光晕），2026-11-15 落地。**

改动（仅 1 个组件 + 1 条单测）：
`LaneCorrectionCanvas.vue` → 标题 `<text class="lcc-label">` 加 `paint-order="stroke" stroke-width="3" stroke-linejoin="round"`，
CSS `.lcc-label` 加 `stroke: var(--color-bg-base)`（= 画布底色，随双主题翻转：editorial `#faf7f2` / dark `#080c12`）。
标题**一个不少**（42 条仍旧 42 条）、`labels` 落位算法与节点坐标**一行未动**。

复验（agent-browser，重新构建部署后独立跑）：
- **标题数不降**：`.lcc-label` 42 条（修前 42 条），与 `.lcc-node` 42 个一一对应——无标题被隐去；
- **几何重叠数不变**：1440×900 与 1920×1080 均 **22/42**（本方案接受重叠，只保可读，不是避让）；
- **可读性恢复（量化）**：重叠处字形紧邻背景由「标记圆红 `#d94a4a`」变「画布底色 `#faf7f2`」，
  文字 `#8a8a8a` 对紧邻背景的对比度 **1.21:1 → 3.23:1**（后者 = 标题在画布任何未被压处的常态对比度，即标记圆不再造成额外损失）；
- **属性落盘**：`paint-order: stroke` / `stroke-width: 3px` / `stroke: rgb(250,247,242)`，dark 主题自动变 `rgb(8,12,18)`（实测白字压橙红标记仍可读）；
- 单测：`LaneCorrectionCanvas.test.ts`「标题一个不少，且带描边光晕」——断言 4 条标题 + 三个属性齐备。

**接受的代价**（主线程预先知悉）：文字描边会把其下方的标记圆切出缺口，被压处勾号圆只剩残弧（标记仍是可点区域 `lcc-hit`，功能不受影响）。

截图（前后对比，同裁剪区域可直接叠图看）：
`d1-before.png` / `d1-after.png`（1440×900 全幅）、`d1-before-detail.png` / `d1-after-detail.png`（最差重叠处 5× 放大）、
`d1-before-1920x1080.png` / `d1-after-1920x1080.png`（1920×1080 全幅）。
> 1920 的「修前」为同构建上移除描边属性后截取（几何与真修前一致：布局算法未动、重叠数同为 22/42），1440 的「修前」是修前构建的真截图。

### D-2（已修）：移除节点后首义范围选择集未剔除

现象：先在「首义范围」勾选 2 个节点、随后移除其中 1 个 → 「用选中的 **2** 个节点定首义」计数**未随之变 1**（选择集仍含已移出节点）；
该节点已不在选择池内，提交会带上**非本话题节点**并被后端 4xx 拒绝。

修复（最小改动）：`front/app/features/tags/composables/useLaneCorrection.ts` → `removeMember` 成功分支同步从 `anchorSelection` 剔除该 id
（与既有契约注释「已被移出（不再属本泳道）的节点也不在池内」对齐）；
补单测 `useLaneCorrection.test.ts`「移除节点 → 首义范围选择集同步剔除（按钮计数不偏大，不带非本话题节点）」；
e2e 复核（重新部署后实测）：移除 8174 后按钮文案由「用选中的 2 个节点定首义」→ 「用选中的 1 个节点定首义」，勾选数 2 → 1。

### 真库残留（如实记录，均为契约内行为）

「移除 → 加回还原」闭环后，section 8174 的行状态为：`topic_match_confidence` 由 `anchor_hit` 变 `manual`（加回口径：manual 置信 + 实算距离）、
`topic_status_at_report` 由 `active` 变 NULL（加回口径：快照列置 NULL）、`manual_assignment_at` 有值且 `manual_assignment_action = attach`（归属变更留痕）；
topic 971 的 `centroid_pending_recompute = true`（归属变更按契约置位，**不立即重算质心**，由次日日报生成时纳入刷新范围并清标记）。
下一次日报生成会按契约刷新质心并把快照列写回。

## 6. 遗留风险

- **D-1 已修但仍接受重叠**：标题与标记圆几何重叠在真实数据密度下照旧存在（1440/1920 均 22/42），修的是**可读性**（对比度回到 3.23:1 常态）；若后续要求「零重叠」，需走布局级改法（标记圆纳入冲突表 / 加大同列间距），属另一 change 范围。副作用：被压处标记圆（勾号圆）只剩残弧，可点热区不变。
- **人工首义侧未做 e2e**：A-3 提交步 / A-12 折叠带侧均需写库才能触发，本批按写操作边界未执行；若后续要出人工首义的界面证据，需主线程另行授权一次写操作（或在预置库上做）。
- **真库残留**：见 §5 末段，归属快照列与质心待重算标记会在下一次日报生成时按契约自愈。

---

## 方案 A 增量验证（全量分层加载，2026-11-15 追加）

### D6 更早成员分组与计数口径【pass】

- 依据：LaneMemberList.test.ts / LaneAnchorScope.test.ts 组件层断言全绿（徽标「窗口 N · 全部 M」、更早成员按月分组渲染、节点卡可开、选择池含窗口外成员、候选不入池、旧后端兼容回退）；后端 handler 测试 D14-D17（传参返回窗口外成员升序 / 不传参字段缺席 / 话题不属板块 404 / 参数非法 400）。原型 v5.4 agent-browser 实测（徽标/分组/分堆/节点卡/折叠带专属不变）。真库泳道成员均在窗口内，产品运行时分组为空态（空分组不渲染），故 e2e 层以组件+原型双覆盖替代，如实记录此边界。
- 视觉参照：原型 v5.4（ui-prototype/index.html，已 agent-browser 实测）；产品运行时真库无窗口外成员，分组为空态（空分组不渲染），以组件测试为证。

---

## D7 候选口径放开 + 首义面板分区（2026-11-15 追加）

### D7-a 候选区/关联推荐不再为空【pass】

- 依据：真库实测 board 1974 / topic 5（近 30 天窗口）164 条 section 中「无归属」0 条 —— 原候选口径（只取无归属节点）必然空。改为「无归属 ∪ 属 candidate（动态）泳道」后，关联推荐 0 → 14 条；画布候选区同步出现节点。属 active（已转正）泳道的节点仍只作只读参照（画布「只读参照」区空心圈）。
- 组件层：useLaneCorrection.test.ts 断言候选集构成（103 无归属 + 104 candidate 进候选 / 105 active 只读）与来源泳道名；后端 daily_report_assignment_test.go 断言改挂（candidate → 本泳道：留痕记来源与目标、两侧泳道均置待重算）与拒绝（active 来源 → 4xx、归属不变）。
- 截图：10-lane-candidate-and-anchor-groups.png（画布候选区有节点、关联推荐行带「来自 X」来源标注）。

### D7-b 首义范围当前/更早分区【pass】

- 依据：实测修正工作面左栏「首义范围」徽标为「窗口内 7 · 全部 98」，「更早成员 · 91」独立折叠分组（默认折叠，展开后逐条可勾选，日期如实，最早 06-15）；此前两者平铺同列、无计数，用户看不出哪些能选到窗口外。
- 组件层：LaneAnchorScope.test.ts 断言默认折叠态、展开后可见、窗口内分组默认展开、徽标口径。
- 截图：10-lane-candidate-and-anchor-groups.png（首义范围面板两分组 + 徽标）。

### 已知边界（非本 change）

- features/tags/components/console/LaneTrendChart.test.ts 2 例红（文案「事件线数」→「线索数」期望未同步，属 retire-story-adopt-event-metrics 口径变更遗留）——已登记 test-patrol 台账（manual/open），本次改动未触碰该文件。
