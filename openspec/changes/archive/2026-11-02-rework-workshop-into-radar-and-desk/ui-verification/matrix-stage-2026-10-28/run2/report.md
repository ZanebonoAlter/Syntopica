# 矩阵静态新版 S9 第二轮（定向复验）独立 K3/UI 报告

- 验收人：独立 K3/UI 验证线程（非实现作者；commandcode/deepseek/deepseek-v4.1-flash:max，implementer 档，未派子线程）
- 时间：2026-10-28 06:09–07:0x（Asia/Shanghai）；浏览器会话 `matrix-s9r2-6d5795`（agent-browser 0.37.1）
- 入口：`http://localhost:5100`（唯一 origin，未碰 host:80）；**index SHA `b86179cb4318a06ff104dcc72ccbfc7fabe9c2951b4e225d2384c9989551db5c`（实测 200/7457B 一致）**，chunk `/_nuxt/CSxw8iJQ.js`（实测 200/349388B，SHA `c5bbb1dc…` 与构建记录一致）；后端 PID **1061075** 未重启（ELAPSED 06:15:43）
- 结论：**首轮 2 个 Medium（R34/R35）已修复并经本轮真实页面复验；R36/R37/R38/R39 修复达成；未发现新的 High/Medium。S9 综合状态 24 PASS + 1 PARTIAL（第 24 步缓存容量子项），无 FAIL。建议交付本阶段静态验收，无需回退。**

> 首轮报告/截图/guard 原样保留，未覆盖；本报告与证据只写 `run2/`。首轮统计已按父校正为 21 PASS / 2 FAIL（15/25）/ 2 PARTIAL（22/24），不沿用误计 23 PASS。

## 1. 现场与源码准入（实测）

| 项 | 结果 |
| --- | --- |
| index / chunk | `b86179cb…` / `CSxw8iJQ.js`，HTTP 200，SHA 与服务端构建记录一致 |
| 后端 | PID 1061075 存活未重启 |
| 构建日志 | `.pi/subagents/validation/matrix-static-20261028-055414-run2/build.log`，deploy_exit=0，100s |
| 源码指纹 | `source-fingerprint-before.txt` 与 `after.txt` **SHA 相同**（构建期零源码变动） |
| 源码 delta | 按 path→SHA 比较 645→652：**21 改 + 7 新增 + 0 删**，均为 R34–R40 授权（父 preflight 校正；未沿用错误的“111 路径内容变”） |
| 回退包 | `matrix-before-20261028-055425.tar.gz`，SHA `d4bc92e5…`（与 02:55 包相同；仅父可回退） |
| 未做 | 未 build/typecheck/test/dev/SQL 写/生成/收编/激活/删除/保存；未改产品/后端/其它 change/旧证据；未动 tasks/review；8B.3/8B.4 未勾 |

## 2. 安全闸（浏览器层按 method 阻断 + 受控只读延迟）

- 新副本 `run2/tools/cdp-method-guard-delay.mjs`：CDP `Fetch.enable('*', Request)` 挂页面/iframe；**仅 GET/HEAD/OPTIONS 放行**，其余 `failRequest(BlockedByClient)` 并落 JSONL；新增 `--delay-ms/--delay-match` 只在浏览器网络层暂停匹配 GET（受控只读延迟，不碰后端/库）。
- 生效证明：`POST /api/__guard_selftest*` 与 **阅读采集 `POST /api/reading-behavior/track-batch` ×2** 均被阻断（`guard-main2.jsonl`），页面侧 `TypeError`；`GET /api/semantic-boards/overview` 等 200。**不声称采集写入成功，只证明链路有发出意图且未落库。**
- 全程未点生成/收编/激活/删除/保存；未打开会写库的确认按钮。guard 汇总：`guard-main/phase1/slow/slow8/main2` 的 blocked 仅上述自检与 reading-behavior，无其它非 GET 放行。
- 用户意图输入：CDP `Input.dispatchMouseEvent(mouseWheel)` 与 `Input.dispatchTouchEvent`（触摸明确标注为**模拟触摸**，非真机）。第一处 `agent-browser mouse` 非整数坐标会静默不移动（工具限制，已改为整数/CDP 派发，非产品缺陷）。

## 3. 定向复验结果（首轮失败/受影响项）

### 3.1 冷首载 + 手柄热区判端（R37/R40 + 第三轮 usePlotWidth 接线）

**冷首载**（对 `lane-dynamics?days=14` 施加 1500ms 只读延迟，浏览器全新空缓存）：
- t≈0.84s：无 SVG、无 hit 层、无档按钮（bounds/range 仍 null 阶段）；
- t≈3.44s（数据晚到后）：SVG 800.81×88 出现，**真实 hit 层 `brush-hit-start` 24px / `brush-hit-end` 24px 出现且宽非 0**（不是仅合并 bbox 推断）。
- 证据：`metrics/cold-t0.json` / `cold-t1.json`。

**1440 桌面**：
- 14 日（gap=28.6px）：两端各 24px 区；左可见线中点 `elementFromPoint=brush-hit-start`、右线中点命中 SVG 手柄（8px，overlay 因贴绘图区右缘被 clamp，仍走 end 处理）；`+10px` 拖左线 → start `10-14→10-19`（end 不变），`-10px` 拖右线 → end `10-27→10-22`（start 不变）。
- 热区外缘首动：14 日 start 区最左缘（线左 ~12px）+1px → **0 天变化**；7 日合并区左缘 +1px → 0 天，+5px → +2 天（=5px×0.45 日，无跳变），右缘 -1px → -1 天。
- 7 日（gap≈13px <24）合并区宽 37.2px；左线中点 +10px → start `10-21→10-25`，右线中点 -10px → end `10-27→10-22`（各自正确端）。
- 单日（10-20）：24px 合并区；上半 y=270 左拖 → start `10-20→10-15`（end 不变）；下半 y=320 右拖 → end `10-20→10-25`（start 不变）。
- 池首（`2025-10-28..11-03`）：区域夹回绘图区左缘（l=332,w=37）；向外拖 → start 保持 2025-10-28（钳制），向内 +10px → start `→2025-11-02`。
- 取消：拖动中 start 预览到 10-23，**Esc 后恢复 10-14/10-27、反馈清空、随后松手不提交、URL 不变**。
- 键盘：start 手柄 `←` `10-14→10-13`、`Shift+→` +7 日 →10-20，焦点保留、aria 更新（与首轮一致，重验通过）。
- 日期输入精调：`#orc-start/#orc-end` 严格格式，7/14/30/365 日精确窗口均生效（见 3.3）。

**375 / 390**：
- 375 14 日：可见线宽 3.2px，合并区 35.3px（24+gap）；左右线中点分别 → start `10-14→10-26` / end `10-27→10-15`（各自正确端）；外缘 +1px → +2 天（1px≈1.15 日的量化，不是跳到区域边缘对应的 ~14 天）。
- 375 7 日：合并区 29.2px；左中点 +6px → start 到 10-27（钳制），右中点 -6px → end 到 10-21（钳制）。
- 375 单日：上/下半区分别抓到 start（→10-08）/end（→10-27 钳制）。
- 390 14 日/7 日：合并区 35.8/29.4px，左右线中点均各自正确端（10-14→10-25 / 10-27→10-16；7 日两端钳制）。
- **极短区触摸限制（如实）**：单日/2 日在 375 上两线相距 ≈0–2px，24px 合并区内只能靠“最近端/上下半区”分辨，人手触摸无法精确点选某一条线；实际精调路径为日期输入与键盘。非缺陷，属触控物理限制。

**新发现（Low，不阻塞）**：14 日窗在 1440（gap 28.6px）和 1920（gap≈45.6px）会出现两个 24px 区**部分重叠**（1440 重叠 7.4px）；重叠带内 DOM 后者 `hit-end` 在上，实测 x=1112（离 start 线 8px、离 end 线 20px）按下拖左 → 移动的是 **end**，不是最近端。两端可见线中点与合并区（<24px）判端均正确；仅“两线之间 5–8px 重叠带”非最近端。属 R40 “最近端”语义在 24≤gap<48 几何下的边缘效果，建议父决定是否在 R40 收口为“gap<48 时也合并/按最近端”，本轮不改码。

### 3.2 深滚动返回 + 慢详情 settled + pending 用户意图取消（R34）

- **1440 正常链**：stage 先经真实 wheel 滚到 371（max=371）；打开日报→返回；t=0.5s 仍 0（pending），t=2.0s **371**、t=4.0s 仍 371；URL 四参恢复。
- **1440 慢链（`/api/daily-reports/` 延迟 4000ms，只读）**：返回后 26 次 0.5s 采样——前 17 次 `st=0, loading=true`（**pending 期间没有被提前钳制/没有中途 302 之类**），第 18 次 `loading=false` 同刻 `st=291`（saved=291, max=291）；截图 `png/s9r2-1440-slow-pending.png` / `s9r2-1440-slow-restored.png`，时间线 `metrics/slow-timeline.jsonl`。
- **pending 用户意图取消（8s 延迟，独立三次往返）**：
  - wheel：pending 中真实 wheel → st=150（`loading=true`）；delayed 详情到达、`loading=false` 后 **st 仍 150**（不强拉 291）；
  - keydown：pending 中 ArrowDown → st=40；settled 后 **仍 40**；
  - touch（CDP 触摸模拟）：touchStart/Move/End → st=105；settled 后 **仍 105**。
  - 截图：`png/s9r2-1440-slow-cancel-wheel.png` / `s9r2-1440-slow-cancel-touch.png`。
- **1280 window**：saved=471（编年按钮点击前 scrollY）；返回 pending=0；delayed settled 后 **471**（doc max=471）。
- **390 window**：saved=687；pending=0；settled 后 **687**（doc max=767）。截图 `png/s9r2-390-slow-restored.png`。
- **研究工作台往返（1440 深滚动）**：saved=468 → 返回后 t=0.8s 已 468（settled 快）、t=2.8s 仍 468；URL `?days=14&start&end&lane=1204`，名单/档/编年一致。
- 结论：R34 的“settled 后才恢复/有界重试/用户意图取消”在真实深链、慢链、三种真实输入下均成立；pending 期间可观测地不提前钳制（帧级“不耗布局帧”由 `BoardConsoleShell.test.ts:812 R34：阅读 pending 不耗布局帧…` 单测覆盖，本轮未做帧计数）。

### 3.3 缓存切档（R35）与范围语义

- 365 首次（网络）：URL `?days=365`，默认 `2025-10-28..2026-10-27`，365 列/2190 格，1 次 `days=365`。
- 14（缓存）：URL `?days=14`，默认 `10-14..10-27`，14 列，0 请求。
- **复现首轮 F2 路径**：14 档 + 自定义 `10-15..10-16` → 点「近 1 年」（该档已缓存）→ URL `?days=365`（无旧 start/end）、输入 `2025-10-28..2026-10-27`、**365 列/2190 格**、脚注 365、0 请求。首轮“2 列旧 range 回灌”不再出现。
- 反向 365→14（缓存）→ 默认 14 + 14 列。
- 外部四参 `?lane=1844&days=30&start=2026-09-28&end=2026-10-27` 全新加载：全恢复（30 档/范围/lane/编年）。
- 自由窗（source=30 档）恰 7 日/365 日/30 日：sourceDays 始终 30、列数 7/365/30、URL 保留 `days=30&start&end`、365 日窗有池补充脚注；无档请求。
- 证据：`metrics/tier-and-geom-run2.json`。

### 3.4 列几何与宽窄布局回归（R36）

| 视口 | 30 列 | 31 列 | 32 列 | 365 列 | 页面横滚 |
| --- | --- | --- | --- | --- | --- |
| 1440 | `sw=cw=845`、非 scrollable、两行日期、无裁切 | `sw=cw=845`、非 scrollable | `sw=1390>cw=845`、scrollable、单行短日期、sticky | `sw=13851>cw=845`、scrollable、sticky | 无（1440/1440） |
| 1920 | `sw=cw=1277` | `sw=cw=1277` | `sw=1390>cw=1277`、scrollable | `sw=13851>cw=1277` | 无（1920/1920） |

- 日期两行 `MM`/`DD`（`.omx-day-month` 09 / `.omx-day-day` 28，y 489.1/500.7，10.56px），完整日期在 title/aria；**看图确认两行日期与格内数字可读、无硬裁**（`png/s9r2-1440-30col.png`）；数值 `clippedCount=0`（cell span `scrollWidth<=clientWidth`）。
- 默认布局（scroll=0，双主题）：1440/1920/2560/375/390 全部 `root 16px`、无横向溢出；1440/1920/2560 无页面纵向滚动（stage 是滚动宿主），左右轨 272/255、272/303、272/303（登记范围内），2560 主舞台填满 1916.8（无 120rem 上限）；375/390 `sw=cw`、矩阵/右轨不渲染、窄屏摘要 349.4/364.4、header 171.9 折行但完整；**头部「刷新」在所有视口可见且 elementFromPoint 命中**。
- 证据：`metrics/layout-{1440,1920,2560,375,390}-{editorial,dark}.json`。

### 3.5 站内原文语义/焦点（R38）与业务大小写（R39）

- matrix 打开原文（148014）：`.app-dialog` `role=dialog`、`aria-modal=true`、`aria-label=站内原文阅读`、`tabindex=-1`，**初始焦点在对话框本体**；宽 1296×758（=90vw 存量兼容，未重绘）；正文来自 API `content` **逐字完整**（687 字，head/tail 都在，无截图/导语中间层）；`mark/.search-highlight/.hl = 0`、`iframe = 0`；打开时 stage scroll 430 不变。
- Esc 关闭：焦点回 `lane-chronicle-article-148014`、scroll 430 不变；✕ 关闭：同样回焦点、scroll 382（当时值）不变。
- 大小写：`.lch-lane-label`（`Agent 架构重构与标准化协作生态`）computed `text-transform: none`，title 保留全文；`.lch-kicker-system`（`当日线索`）computed `uppercase`、letter-spacing 1.4784px（系统 kicker 保留）。看图 `png/s9r2-1440-chronicle-kicker.png` 确认 “Agent” 未被大写。

## 4. 25 步综合映射（本轮实页 / 继承首轮 / 便宜层 / 未验分栏）

> **重要**：最后一列链接的单测/用例是**实现者按 implementation-matrix-stage.md §13 实际跑绿的记录**（R34–R39 批 9 文件/181 tests；第三轮 3 文件/69 tests），**不是本轮浏览器执行**；本轮没有复跑这些命令。首轮证据原样保留。

| 步 | 本轮 run2 实页复验 | 继承首轮真实 PASS | 关联便宜层证据（已跑绿记录/用例名） | 仍未验/失败 |
| --- | --- | --- | --- | --- |
| 1 矩阵默认/峰值/无行尾/断档 | ✅ 1980 10-27 格 aria「峰值3·3桶·合计7」，gap disabled+title | ✅ 无行尾列/零线索语义 | `ObservationMatrix.test.ts:110 MX-1`、`:146 MX-4`、`:157 MX-5` | 真库无「有分区 0 线索」格样本（`MX-3` 单测覆盖） |
| 2 三指标 aria/title | ✅ 14 日/30 日格 aria+title 逐字 | ✅ | `ObservationMatrix.test.ts:110` | — |
| 3 点有分区格就地下读 | ✅ 1980 10-27 格 → `?lane=90`、编年、无浮层、scroll 0、焦点在格 | ✅ 1974 10-16 同断言 | `ObservationMatrix.test.ts:259/274`、`BoardConsoleShell.test.ts:522` | — |
| 4 点无分区格 | 未重跑 | ✅ 首轮 lane5 10-21 无变化/无浮层 | `ObservationMatrix.test.ts:146` | — |
| 5 编年 >5 条全量/收束 | 未重跑 | ✅ 首轮 report1033 7 条 | `LaneChronicle.test.ts:162` | — |
| 6 单条摘要/互斥 | ✅ 14173 summary 打开（内容与 API 一致） | ✅ 首轮互斥/切条收束 | `LaneChronicle.test.ts:183` | — |
| 7 相关文章按需/站内原文 | ✅ 3 篇、正文逐字、失败重试未重跑 | ✅ 首轮单篇失败隔离+重试 | `LaneChronicle.test.ts:207/221/239`（本轮 3 篇真实） | — |
| 8 前后有内容日/边界/滚动 | 未重跑 | ✅ 首轮（200→200、最新/最早禁用 title） | `LaneChronicle.test.ts:250/263` | — |
| 9 演进点径/合计/无最密三日/态势 | 未重跑 | ✅ 首轮（点径公式/200 字逐字/无 densest） | `ObservationLaneEvolution.test.ts:91/139/152` | 真库无 snapshot=null（`:167` 单测覆盖） |
| 10 >31 日折叠 | 本轮 365 窗只验几何（趋势/索引未重验） | ✅ 首轮趋势轴 28/14/0 + 逐日索引 | `ObservationLaneEvolution.test.ts:174` | — |
| 11 刷选/平移不重排 | ✅ 空白拖选 commit（URL 恰一次）、选区平移 +14 日宽不变、预览 URL 不变 | ✅ 首轮 B 仅补同 ID/行序不变 | `ObservationRangeControls.test.ts:97/114`、`ObservationMatrix.test.ts:202/220` | — |
| 12 恰 7/14/30/365 不切档 | ✅（source=30：7/30/365 列、tier 30、无档请求） | ✅ 首轮 source14 同断言 | `ObservationRangeControls.test.ts:323`、`ObservationMatrix.test.ts:202` | — |
| 13 日期输入 + URL + 联动 | ✅ 10-16 单日/30 日/21 日；URL start/end | ✅ 首轮 10-06..10-26 联动 | `ObservationRangeControls.test.ts:62/84` | — |
| 14 键盘手柄 | ✅ ←/Shift+→/Home/End；焦点保持；aria 更新 | ✅ 首轮 | `ObservationRangeControls.test.ts:165/179` | — |
| 15 明确切独立档 | ✅ **首轮 FAIL 修复**：缓存 365/14 均默认范围、轴、URL 一致，0 旧 range 回灌 | — | `BoardConsoleShell.test.ts:605 R35 deferred 导航` | 真库无法观测“router 提交前”瞬间（单测覆盖） |
| 16 右轨顺序/体检/候选/来源 | 未重跑 | ✅ 首轮（82=llm_score、7/30/90 独立窗） | `BoardGovernanceRail`/`BoardContextRail` 既有测试（首轮已逐项断言） | — |
| 17 追踪管理仅板块 | 未重跑 | ✅ 首轮 | `BoardConsoleShell.test.ts:982` | — |
| 18 全局入口只在总览 | 未重跑 | ✅ 首轮（反向 0 入口、六项可达、取消无写） | `BoardConsoleShell.test.ts:686` | — |
| 19 最新/历史日报入口 | ✅ header「打开日报」→ `/tags/1974/2026-10-27`（本轮两次往返） | ✅ 首轮历史日动作 | `BoardConsoleShell.test.ts:385`、`LaneChronicle.test.ts:276` | — |
| 20 切板块清场 | ✅ 1974→1980：URL 清参、14 档默认、名单 6 条、health 78、无残留 | ✅ 首轮 | `BoardConsoleShell.test.ts:1012` | — |
| 21 四参深链/非法项 | ✅ 合法四参全恢复；非法 days/start>end/池外未重跑 | ✅ 首轮四类非法提示 | `useBoardConsole.observation.test.ts`（OX-9） | — |
| 22 双视口/窄屏/列几何 | ✅ **首轮 PARTIAL 修复**：1440/1920 30/31 无内滚、32/365 内滚+sticky、页面不横滚；375/390 无横溢 | — | `ObservationMatrix.test.ts:163/178` | — |
| 23 A/B 分层失败/重试 | 未重跑 | ✅ 首轮（B 失败横幅+独立重试、375 可达） | `BoardConsoleShell.test.ts:570 R29` | A 首错/A 刷新失败仅单测（`useBoardConsole.test.ts:301/311`） |
| 24 读取边界/缓存 | ✅ 切档缓存 0 请求；报告详情/文章命中不重发（首轮 4 连点 1 次详情） | ✅ 首轮列表 1 份/详情/文章缓存 | `useLaneDayReading.test.ts:355 CA-3`、`:365 CA-4`、`:375 CA-5`、`:418 CA-8`；晚到 `:174/187`，epoch `useBoardConsole.observation.test.ts:258 OX-10` | **报告 14/20 容量与 FIFO 逐出未在真实浏览器验**（单测覆盖）→ 本步 PARTIAL |
| 25 研究工作台往返/恢复 | ✅ **首轮 FAIL 修复**：1440 深滚动 468→468、URL 四参+lane、档/名单/编年一致；报告/研究同机制 | ✅ 首轮研究工作台真实页与 URL 恢复 | `BoardConsoleShell.test.ts:812/864`（R34 pending/cancel） | 真机触摸未验（CDP 模拟） |

**综合**：**24 PASS + 1 PARTIAL（24） + 0 FAIL**（首轮 21/2/2 → 本轮修复 15/25/22，24 因容量子项保持 PARTIAL）。

**仍未验（真库缺样本或本轮禁触发，不用假数据补绿）**：
1. 报告 14/20 缓存容量与 FIFO 逐出、当前阅读项稳定性（CA-3/4/5/8 单测已跑绿，本轮未造 15 份报告/21 篇文章）。
2. 切板块/切日旧响应**真实晚到**（本轮未构造真实延迟竞态；OX-10/`useLaneDayReading:174/187` 单测覆盖）。
3. thread_count 缺省、空 summary（实现有「暂无摘要记录」空态文案但无用例断言，且与用例「原始摘要为空」措辞不同，Low）、空 related（单测 `:239` 覆盖，真库无样本）。
4. 报告被删 / section 不在详情 / 无日报板块空态（单测覆盖，真库无样本）。
5. A 首错/A 刷新失败 stale（单测覆盖，本轮未模拟）。
6. snapshot=null 待结算占位（单测 `:167`，真库 12 板块全有快照）。
7. 「有分区 0 线索」格（单测 `MX-3`，真库无样本）。
8. 真实生成 DB 提交与 WS 先后（本轮禁触发生成；不计入 S9）。
9. 物理真机触摸（仅 CDP 触摸模拟；375/390 的 24px 热区与判端已在 headless 指针/触摸模拟下验证）。
10. 首轮已知旧债：根 53 pass/1 fail/2 未覆盖、旧投诉、7 条 SignalReport 类型债——不因本轮改变。

## 5. 截图与看图结论（run2，均本轮新图）

| 文件 | 真实状态 | 看图判断 |
| --- | --- | --- |
| `s9r2-1440-editorial-default.png` | 1980 14 日、scroll 0 | 单行头部+刷新；轨 272/255；矩阵 14 列两行日期可读；无溢出；与批准基准同构 |
| `s9r2-1440-dark-default.png` | 同上深色 | token 生效、层级清晰、无裁切 |
| `s9r2-1440-30col.png` | 30 列（无内滚） | **两行 MM/DD 10.56px 实际可读、格值不裁**；30 列单屏，页面无横滚（R36 达成） |
| `s9r2-1440-365col.png` | 365 列 | 容器内横滚 + sticky 标签；页面不横滚 |
| `s9r2-1920-editorial-default.png` / `s9r2-1920-dark-default.png` | 1920 14 日 | 主舞台填宽、轨 272/303、单行头部、无溢出 |
| `s9r2-2560-editorial-default.png` / `s9r2-2560-dark-default.png` | 2560 14 日 | 填满 2560（无 120rem 上限），构图稳定 |
| `s9r2-375-editorial-default.png` / `s9r2-375-dark-default.png` | 375 双主题 | 头部折行完整、刷新可达、无横溢；刷选区在右缘 |
| `s9r2-390-editorial-default.png` / `s9r2-390-dark-default.png` | 390 双主题 | 窄屏摘要「87 线索 · 16 分区」与 API 一致；无矩阵/右轨 |
| `s9r2-1440-chronicle-kicker.png` | 1980 编年 | 「当日线索 · Agent 架构重构…」原大小写，非 AGENT；系统 kicker 大写 |
| `s9r2-1440-slow-pending.png` / `-slow-restored.png` | 慢链 pending / settled | pending 停在顶部，settled 后回到深滚动；无中途强拉 |
| `s9r2-1440-slow-cancel-wheel.png` / `-cancel-touch.png` | pending 用户取消 | 取消后停在用户位置，详情回来不拉回 |
| `s9r2-390-slow-restored.png` | 390 window 恢复 | 回 687（深滚动） |

## 6. 新发现与首轮 Medium 关闭

| 编号 | 严重度 | 状态 | 说明/证据 |
| --- | --- | --- | --- |
| R34（首轮 F1） | Medium | **已修复并实页复验** | 1440 371→371、慢链 pending 0→settled 291、wheel/keydown/touch 取消均不强拉；1280 471、390 687、research 468 |
| R35（首轮 F2） | Medium | **已修复并实页复验** | 缓存 365/14 切档默认输入/轴/URL 一致，0 旧 range；外部四参/自由窗回归 |
| R36（首轮 F3） | Low/合同 | **已修复并实页复验** | 1440/1920 30/31 `sw=cw`、32/365 内滚+sticky、页面不横滚；两行日期可读 |
| R37/R40（首轮 F4） | Low | **已修复并实页复验** | 冷载 24px 真实 hit 层；1440/375/390 7/14 线中点各自正确端；外缘无跳变；单日/池端点/取消/键盘通过 |
| R38（首轮 F5） | Low | **已修复并实页复验** | role/aria-modal/label/tabindex+初始焦点在 dialog；Esc/✕ 回触发点且不强滚；正文逐字；90vw/高亮/iframe 未重绘 |
| R39（首轮 F6） | Low | **已修复并实页复验** | 业务 label computed `text-transform:none`；系统 kicker 仍 uppercase |
| N1（新，Low） | Low | 观察/待父裁决 | 24≤gap<48 时两个 24px 命中区部分重叠，重叠带（1440 14 日 7.4px）按 DOM 顺序归 end 而非最近端；两端中点与 <24px 合并区判端正确 |
| N2（新，Info） | Info | 如实说明 | 375 下 1px≈1.15 日，外缘首动 1px 可变化 1–2 天（量化噪声，非跳变）；1–2 日极短区触摸无法精确点选单线，精调依赖输入/键盘 |
| N3（新，Low） | Low | 留账 | 空 summary 实现文案「暂无摘要记录」，与用例「原始摘要为空」措辞不同且无用例断言（真库无样本） |

## 7. 交付判断

- **S9 本阶段：可交付**（静态新版，只读验收）。25 步 24 PASS + 1 PARTIAL（第 24 步缓存容量未做真实浏览器大样本，已有单测固定）；首轮两个 Medium 已修复并真实复验，无新的 High/Medium，无需回退。
- 建议父在验收记录中保留：N1 重叠带（可选 R40 收口）、N2 触摸限制说明、N3 文案/用例差异；以及第 24 步的真实浏览器容量子项仍未验。
- 未验旧债保持：根 53/1/2、旧投诉、7 条 SignalReport 类型债；真实生成 DB/WS 时序、物理真机触摸仍单列未验；8B.3/8B.4 与主 review/tasks 未动。
