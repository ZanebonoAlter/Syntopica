# test-cases: 矩阵/追踪阶段（8B.2，锚 `workshop-board-console` + `board-lane-dynamics`）

> 本文件是矩阵/追踪阶段（tasks 8B.2–8B.4）的用例账本，串 `specs/workshop-board-console/spec.md` 与 `specs/board-lane-dynamics/spec.md` 本阶段新增/修订的 Scenario。总账 `test-cases.md` 已加阶段入口（S9），S1–S8 与根 53 pass / 1 fail / 2 未覆盖**不重写、不洗绿**；本阶段改语义的旧条目在下方「继承与调整」表逐行处置。
> 判据原则沿用总账头部四条（一断言一结果 / 负向反向断言 / mock 网络 + `@iconify/vue` / 阈值三点写清进位方向）。
> 零写路径声明：本阶段后端零改动、前端无任何写操作（编年/观察窗/右轨全部只读），所有「无写」断言以写请求计数 0 / 响应未变异表达。

## 故事 S9：矩阵观察窗与当日就地下读（用户已批 A 路线「名单稳定」）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 进入 `/tags/:boardId`（默认 14 档） | 矩阵视图·渲染矩阵 | 矩阵默认视图渲染；日格数字 = 同日各主题桶线索 **max**；无行尾总数列；无分区格 = '—' 禁用；有分区 0 线索格 = '0' 可点 | 组件 | `console/BoardMatrixView.test.ts` |
| 2 | 观察日格三指标 | 矩阵视图·格值与断档 | 格 title/aria 含「桶峰值 X 条 · N 个主题桶 · 当日合计 M 条」；无分区格含「无分区」 | 组件 | 同上 |
| 3 | 点有分区的格子 | 当日线索工作面·就地下读 | **不开抽屉**；`selectedDate=该日`，下方编年更新为该日内容；焦点/滚动不被强抢 | 组件 | `console/BoardConsoleShell.test.ts` + `console/LaneChronicle.test.ts` |
| 4 | 点无分区格 | 矩阵视图·断档可读 | 提示「这一天这条泳道没有分区」，不更新编年、不开任何浮层 | 组件 | `console/BoardMatrixView.test.ts` |
| 5 | 观察编年标题目录 | 当日线索工作面·标题默认收束 | 列出该日全部主题桶的**全量线索标题**（经日报详情按 section_id 读取，>5 条可读，页脚「另有 K 条折叠」只在读取降级时出现）；默认全部收束 | 组件 | `console/LaneChronicle.test.ts` |
| 6 | 点一条标题 | 当日线索工作面·单条真实摘要 | 就地展开该 thread `summary` 逐字；再点另一条时前一条收束（同一时刻仅一条展开） | 组件 | 同上 |
| 7 | 点「相关文章 · N 篇」 | 当日线索工作面·相关文章按需 | 展开前才请求 `related_article_ids` 对应文章详情（按id有界缓存，同文章跨thread在未逐出时复用）；列表不携带正文类大字段；点文章走站内阅读（壳层 ArticlePreviewModal） | 组件 | 同上 + `composables/useLaneDayReading.test.ts` |
| 8 | 点「上一/下一有内容日」 | 当日线索工作面·前后导航 | 就地切到相邻有分区日（跳过无分区日），边界给「已到最早/最新有内容日」且按钮禁用；滚动位置保持 | 组件 | `console/LaneChronicle.test.ts` |
| 9 | 观察演进带 | 演进带·点径与合计 | 节点数字 = 当日线索合计，点径 `min(24, 6+√total×5)`；**无「最密集三天」区块**；态势句折叠为「最近结算态势 · as_of（非本视窗总结）」，正文 = 快照逐字；`snapshot=null` = 待结算占位且时间线照常 | 组件 | `console/BoardLaneEvolution.test.ts` |
| 10 | 可见日 >31（选 365 档或拖宽） | 演进带·长窗折叠 | 逐日节点收进「展开逐日索引」折叠；默认渲染泳道趋势图（日期 X 轴 + 线索合计 Y 轴 + 0 基线 + 真实比例刻度） | 组件 | `console/BoardLaneEvolution.test.ts` + `console/LaneTrendChart.test.ts` |
| 11 | 拖 brush 空白处刷选新区间 | 观察窗·拖选不重排 | 观察范围更新为所选 `[start,end]`；**sourceDays/名单/行序/休眠/候选不变**；常态标「名单/排序/休眠按 N 日档」，超档窗区段标「历史内容由 365 日池补充（不改名单）」；B 仅补 A 同 ID 历史 | 纯逻辑 + 组件 | `console/observationRange.test.ts` + `console/ObservationRangeControls.test.ts` |
| 12 | source14 下拖到恰 7/14/30/365 日或拖回档自身区间 | 观察窗·精确档回切 | **不按日期切档**：sourceDays=14、base=A14，名单/行序/休眠/候选逐项不变；不读取其他短档作 base，B 仅补 A 同 ID 历史 | 纯逻辑 | `console/observationRange.test.ts` |
| 13 | 输入 `2026-10-06`~`2026-10-26` 应用 | 观察窗·精确输入 | 视窗/矩阵/演进/编年联动到该区间；URL query 携带 `days/start/end` | 组件 | `console/ObservationRangeControls.test.ts` |
| 14 | 双端手柄聚焦按 ←/→（Shift） | 观察窗·键盘导航 | 起止端逐日（Shift 逐周）调整，焦点保持手柄；aria-valuenow/text 随日期更新 | 组件 | 同上 |
| 15 | 明确切独立快捷档 7/14/30/365（或 URL days） | 观察窗·明确切独立档 | 请求/命中该档缓存后才重算名单/候选、source 全轴 clueTotal→sectionTotal→ID 行序与休眠；范围重置为该档默认区间；选中泳道保持（不在该档响应时按原排序回退，不造成员）；365 档与 B 只一次请求 | 纯逻辑 + 组件 | `composables/useBoardConsole.test.ts` |
| 16 | 观察右轨 | 治理右轨·顺序与结论常驻 | 区块顺序：版块体检 → 构成候选 → 话题追踪 → 来源构成；体检为真实 llm_score/体检日期/评语首句预览 +「最新快照 · 非所选历史评分」；候选 top3 真实名称 + 实际清单数；watch 名称 + active/paused；来源独立 7/30/90 窗 + 贡献条 +「独立统计」标注 | 组件 | `console/BoardGovernanceRail.test.ts` |
| 17 | 右轨点「管理 ↗」（追踪） | 板块追踪管理·纯板块作用域 | 打开 WatchManagePanel 且参数仅 `boardId`（断言不含 topicId/lane）；关注记录的 persistentTopicId 仅作「对应泳道」定位链接 | 组件 | `console/BoardGovernanceRail.test.ts` |
| 18 | 找匹配回填/标签池入口 | 全局配置迁出·唯一总览入口 | 版块页头部无「更多」菜单、右轨无全局六项按钮（反向断言选择器为空）；`/tags` BoardRadarGovernance 六项真实入口及确认/取消仍可达，不删API/功能、不改总览视觉 | 组件 | `console/BoardConsoleShell.test.ts` |
| 19 | 头部「打开日报 · MM/DD」 | 最新日报唯一主入口 | 按钮进 `/tags/:boardId/:date` 阅读台（既有路由）；编年内 selectedDate ≠ 最新报告日时另保留带完整日期的历史日报动作（不误删 latest 之外的入口） | 组件 | `console/BoardConsoleShell.test.ts` + `console/LaneChronicle.test.ts` |
| 20 | 切板块 A→B | 骨架·切版块清场 | 观察范围回档默认、selectedDate 清空、编年收束、休眠收起、选中回默认（既有 R-7 扩展） | 纯逻辑 + 组件 | `composables/useBoardConsole.test.ts`、`console/BoardConsoleShell.test.ts` |
| 21 | 带 `?lane=&days=&start=&end=` 刷新 | 骨架·深链恢复 | 四参数恢复选中泳道/档/观察范围；任一非法（days 非四档、日期格式错、start>end、池外）→ 忽略该项 + 明确提示，其余合法项照常生效，不白屏 | 纯逻辑 + 组件 | `composables/useBoardConsole.test.ts` |
| 22 | 双视口走 1–3、9–11、16 | major UI 验收 | 1440/1920 无页面级纵向滚动、矩阵容器内横滚仅在 >31 列；375/390 无横向溢出（矩阵/右轨不渲染，走窄屏摘要+编年） | 端到端 | `ui-verification/`（8B.4，见 §真实浏览器） |
| 23 | A/B 分别失败并重试 | 观察窗·档池分层失败 | A成功B失败保留A与视窗、历史标未取到；A首次失败B成功为A错误态；旧A刷新失败stale可读，B不冒名单 | 组件 + 纯逻辑 | `composables/useBoardConsole.test.ts`、`console/BoardConsoleShell.test.ts`（OX-12…14） |
| 24 | 读满/超限、切板块/卸载 | 读取边界·365档池请求去重 / 有界缓存与当前阅读稳定 | source≤4档且365一次、列表1份、报告≤14/文章≤20；FIFO、不缓存失败、释放旧作用域；当前阅读/焦点/滚动不消失或回顶 | 纯逻辑 + 组件 | `composables/useBoardConsole.test.ts`、`composables/useLaneDayReading.cache.test.ts`、`console/LaneChronicle.test.ts`（CA-1…8；8C.4S 结构包把 CA 子套件从 `useLaneDayReading.test.ts` 拆到 `.cache.test.ts`） |
| 25 | 顶栏点研究工作台并返回 | 板块入口·研究工作台真实承接 | 进入`/tags/:boardId/research`真实BoardEnrichmentPanel，返回恢复矩阵泳道/日期/范围/阅读与滚动；不改foreign研究页，不称旧关系/关键事件恢复（A-56/R-11…16仍欠账） | 组件 + 端到端 | `console/BoardConsoleShell.test.ts`；人工：8B.4静态实页往返并记录URL/上下文快照 |

### 变体走查（五组固定清单）

| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 输入：日期空串 / 纯空白 / `2026-10-6`（缺零）/ `2026/10/06` / start>end / 池外日期 | 各给对应明确文案（缺项/格式/顺序/未取数范围），**原视窗保持不变**；不静默截期、不补 0 | 纯逻辑 | `console/observationRange.test.ts` |
| 2 | 输入：观察范围单日（start=end）/ 2 日 / 全池 365 日 | 全部合法：轴长 = 天数；单日也渲染 | 纯逻辑 | 同上 |
| 3 | 输入：days=abc/0/-1/8（非四档） | 归一回默认 14 并提示；不发起 days=8 请求 | 纯逻辑 | `composables/useBoardConsole.test.ts` |
| 4 | 前置：A 名单泳道在观察轴内无分区；B 另有 A 中不存在的泳道 | A 行仍保留，可见区间内全 '—'；B 独有 ID 不进入名单或候选；不造格、不用 B 重排 | 组件 | `console/BoardMatrixView.test.ts` |
| 5 | 前置：`thread_count` 缺省（旧响应） | 日格回退 `events.length+folded_count`；两者皆缺显示 `?`（未知），不拿桶数补数 | 纯逻辑 | `composables/useBoardConsole.test.ts` |
| 6 | 前置：selectedDate 当日报告已被删除 / 报告存在但 section 不在详情 | 编年降级显示 base 响应的 ≤5 标题 +「另有 K 条折叠未读取」+ 重试；不伪造摘要、不借别日 | 组件 | `console/LaneChronicle.test.ts` |
| 7 | 前置：thread summary 为空 / related_article_ids 为空 / 文章详情失败 | 分别显示「暂无摘要记录」（原 data summary 为空的等价说明）/「相关文章列表为空」/ 单篇失败局部错误 + 重试，不阻塞其他条目；loaded 空摘要与未取到报告不得混同 | 组件 | 同上 |
| 8 | 前置：无日报版块 | 空态引导生成（既有 no-reports 不变）；时间视窗控件给「未取数」提示 | 组件 | `console/BoardConsoleShell.test.ts` |
| 9 | 时间：今天无日报（锚点早于今天） | 轴终点 = 数据锚点；「今天」与「数据截止」分别表达（既有 dataAsOf 语义保留） | 纯逻辑 + 组件 | `composables/useBoardConsole.test.ts` |
| 10 | 时间：跨月/跨年观察范围 | 日期连续无重复（buildDayAxis 既有） | 纯逻辑 | 同上 |
| 11 | 幂等：重复点同一格 / 同一标题 / 重复应用相同范围 | 编年不重复请求（缓存命中）；展开态幂等；范围不变不重渲染请求 | 组件 + 纯逻辑 | `useLaneDayReading.test.ts` / `observationRange.test.ts` |
| 12 | 并发：切板块后旧档/旧池响应晚到 | 双通道 epoch 守卫：旧响应丢弃，新版块数据不被污染（写请求计数同时为 0） | 纯逻辑 | `composables/useBoardConsole.test.ts` |
| 13 | 可用性：A 成功 B 失败 / A 首次失败 B 成功 / A 刷新失败 B 成功 | 分别为 A 可读且历史标未取到（不补0、不换名单/强改视窗）/ A 错误态不可用 B 冒成功 / staleNotice 保留旧 A 可读；均可重试、导航/返回可用 | 组件 | `console/BoardConsoleShell.test.ts` |
| 14 | 可用性：编年读取失败 | 编年局部错误 + 重试；矩阵/演进/右轨不受影响 | 组件 | `console/LaneChronicle.test.ts` |
| 15 | 可用性：超长泳道名 / 超长标题 / 超长评语 | ellipsis + title 全文；不撑破行；评语首句预览两行封顶 | 组件 | `BoardMatrixView/LaneChronicle/BoardGovernanceRail.test.ts` |
| 16 | 可用性：重复提交（范围应用连点） | 仅一次状态更新；按钮无重复请求 | 组件 | `ObservationRangeControls.test.ts` |

不适用划除（变体固定清单）：输入组「纯分隔符/特殊字符/大小写」——本阶段唯一自由文本输入是 `YYYY-MM-DD` 日期（固定格式校验，已含非法格式分支），泳道搜索属编排态不在本阶段；故划除。

### 效果核对

- 触发原因：日格桶峰值/演进合计/编年全量标题的真实性依赖 `thread_count` 覆盖率与日报详情完整性（数据上游）。
- 核对方法：8B.4 用真实库板块（如 1974）在静态 `:5100` 走步 1–8、11–13：对同一日抽格，格值 = `SELECT max(thread 计数)`；点开编年标题数 = 详情端点该 section threads 数；拖选后行序与拖前一致（名单稳定）。
- 量化结果：8B.4 回填（预期：格值/标题数与 GET 原始响应逐一对上；≥1 个多桶日与 ≥1 个 0 线索日实页可辨）。
- 结论：达标交付 / 瓶颈在上游（候选粒度、打标覆盖）——偏差先定位上游，不调 UI 阈值掩盖。

## 继承与调整（问句⓪：本阶段契约 M/R 资产逐行处置）

旧资产反查：`bash scripts/harness/test-assets.sh board-lane-dynamics`（主 specs 8 Requirements/24 Scenarios + archive 三 change 映射）；`workshop-board-console` 无主 spec（本 change 新增能力，delta 即合同）。`source-board-hit-rate`/`daily-report-system` 等其余六 spec 只读继承，无 M/R 新增。

| 旧条目（test-cases.md 总账） | 处置 | 旧测试文件 | 动作 |
| --- | --- | --- | --- |
| S1-2「6 行×14 列、行尾线索总数」 | 改语义（格值=桶峰值；行尾总数删除） | `console/BoardMatrixView.test.ts` | 改断言：格值 peak、无行尾列；轴长断言保留 |
| S1-3「断档空格不渲染 0」 | 改语义（无分区='—' 禁用；有分区 0='0' 可开） | 同上 | 改断言；「断档可读」义务保留（aria 含无分区） |
| S1-4/6/11 + D-1..D-7（SectionDrawer 族） | 改语义（抽屉退役于日格路径→当日编年就地下读） | `console/SectionDrawer.test.ts` | 断言迁 `console/LaneChronicle.test.ts`（空格提示/全部事件/打开日报/看归档/只读零写/Esc=关闭保持 义务全继承）；SectionDrawer.test.ts 保留照跑至 8.4（组件未删、另有挂载路径则绿） |
| S1-9/10 + E-1..E-6（演进带） | 改语义（E-3 最密集三天废止；E-4 今日事件块废止→编年默认最近内容日；态势句标注改「最近结算态势·as_of」） | `console/BoardLaneEvolution.test.ts` | 改断言：删 densest/today-events 用例（留痕），E-1/E-2/E-5 保留并更新点径/标签阈值口径，E-6 改折叠标注 |
| S1-12/13 + MD-1..MD-7（休眠） | 继承（阈值/只读/随窗不变；依据改为 base 全窗） | `console/BoardMatrixView.test.ts`、`composables/useBoardConsole.test.ts` | 照跑 + 增「裁切不重算休眠」负向断言 |
| S1-14 + R-4..R-6（lane 深链） | 继承 | `composables/useBoardConsole.test.ts` | 照跑；新增 days/start/end 参数用例 |
| S1-18 + S7-4（更多菜单/全局入口） | 改语义（版块页更多菜单与右轨全局六项退役；唯一入口总览） | `console/BoardConsoleShell.test.ts`、`console/BoardGovernanceRail.test.ts` | 改断言：反向断言菜单不存在 + 总览入口可达；A-36/A-57 旧故事标「改语义」不洗绿 |
| S1-16/17/19（右轨体检/失败隔离） | 继承 + 顺序与预览补充 | `console/BoardGovernanceRail.test.ts`、`BoardHealthPanel.test.ts` | 照跑 + 顺序/首句预览/最新快照标注用例 |
| S1-变体5/12（窗口切换/非法 days） | 改语义（四档独立响应 + 双通道；非法归一不变） | `composables/useBoardConsole.test.ts` | 保留归一断言；A 唯一名单基准、B 仅补同 ID 历史，365 去重；标明 source 档，不按日期推导档位 |
| S1-变体13（切版块竞态） | 继承（扩双通道 epoch） | 同上 | 扩展：档+池两路晚到响应均丢弃 |
| M-4/M-5/M-6/M-9（格值/空格/行尾/轴外） | 改语义/继承 | `console/BoardMatrixView.test.ts` | M-4 改 peak、M-6 删除留痕、M-5/M-9 改口径后照跑 |
| D-4（抽屉日报线索按 section.id 命中） | 继承（逻辑迁 useLaneDayReading） | `console/SectionDrawer.test.ts` | 断言迁 `composables/useLaneDayReading.test.ts` |
| board-lane-dynamics 主 spec「默认展示泳道动态区/只读语义/一句话态势展示/发展时间线结构/单日多事件/沉寂不展示/候选不进主区/单请求聚合/无态势快照降级/长版缺失明示」 | 继承（delta 本阶段修订措辞：日格/节点口径 + 单日多事件补「完整线索经日报详情读取」；沉寂/候选/单请求/降级语义不变） | `lane-dynamics/LaneDynamicsPanel.test.ts`、`LaneDynamicsCard.test.ts`、`backend lane_snapshot_repository_test.go` | 照跑（回归网，保留至阶段④）；后端零改动，Go 侧不因本阶段重跑 |
| S5/S6/S2/S3/S4/S8（阅读台/来源/归档/编排/总览） | 不动（不在本阶段扩批） | 各自测试文件 | 照跑；本阶段不新增其用例 |
| 根账 53 pass / 1 fail / 2 未覆盖 | 不洗绿 | — | 留账不动；本阶段修复轮只新增/修改上表所列文件 |

## 白盒附加（复杂档：分支表 + 边界值 + 不适用划除）

### 分支表 — 观察窗（OX）

纯函数落 `console/observationRange.ts`（validate/观察轴派生，不从日期选择base），状态机落 `useBoardConsole`。A=source档唯一基准、B=365同ID历史补充；排序与休眠只用A全轴。

| # | 条件/分支 | 输入 | 期望 | 测试用例名 |
| --- | --- | --- | --- | --- |
| OX-1 | 恰好档宽也不切档 | sourceDays=14，拖至恰 7/14/30/365 日（含其他档自身区间） | sourceDays=14、base=A14；名单/行序/休眠/候选逐项不变；不把已缓存其他档提为 base | `range: exact widths keep source14` |
| OX-2 | 任意跨 source 窗观察 | A14 名单含 x；B 含 x 历史与独有 y；观察跨出14日且非四档宽 | 仅补 x 历史，y 不进入名单/候选；base=A14；常态标 source14、历史区段标 B 补内容；不重排/重分组 | `range: pool history only for source ids` |
| OX-3 | 校验四类 | 空串 / 非法格式 / start>end / 池外 | 各自文案 + 原范围不变 | `range: validate messages` |
| OX-4 | 池边界 | start=池首 / end=池尾 / 各越界一日 | 边界内合法、越界拒绝 | `range: pool bounds` |
| OX-5 | 平移/缩放 | 视窗平移至池首/池尾；缩放 0.5×/2× | 结果钳制在池内、宽度≥1 日 | `range: shift clamp` / `range: zoom clamp` |
| OX-6 | 键盘 | ←/→/Shift+←/→/Home/End 于起/止端 | 逐日/逐周/到端点；不越过对端 | `range: keyboard bounds` |
| OX-7 | 唯一可换基准的动作 | source14 自由观察后明确点7档 / 明确 URL days=7 | A7 成功才按其 source 全轴重算名单/行序/休眠/候选，range 重置为7档默认；日期动作不能走此分支 | `range: explicit tier changes source` |
| OX-8 | 365 档即池 | sourceDays=365 | 只发一次 lane-dynamics 请求，档/池共用响应 | `range: 365 shares pool request` |
| OX-9 | URL 恢复 | 合法四参数 / 任一非法 | 全恢复 / 忽略非法项 + 提示，其余生效 | `range: url restore partial` |
| OX-10 | 竞态 | 切板块后档/池旧响应晚到 | 双通道 epoch 丢弃，新板块数据不被旧响应覆盖 | `console: dual-channel epoch` |
| OX-11 | 前端 source 全轴排序 | A 中两行 clueTotal 相同、sectionTotal 不同；再设两值均相同但 ID 不同；B/观察轴给出相反热度顺序 | 仍按 A 全轴 clueTotal 降序→sectionTotal 降序→ID 升序；≤2 分区休眠按 A 计算；候选不被 B 替换；拖选前后逐项相等，写请求0 | `range: source axis order and dormancy` |
| OX-12 | 仅 B 失败 | A14 成功，B365 失败，观察跨出 A 窗 | A 保持可读；历史未取到不补0；source/名单/行序/休眠/候选/观察范围不变，B 可重试 | `console: pool failure keeps source readable` |
| OX-13 | 仅 A 首次失败 | A14 首次失败、B365 成功 | A 错误态 + 重试；不展示 B 名单冒充 A14 成功 | `console: first source failure is not pool success` |
| OX-14 | A 刷新失败 | 旧 A14 成功快照存在，刷新失败、B 成功 | staleNotice + 旧 A 可读，原行序/分组/候选保持；失败不覆盖成功缓存 | `console: stale source survives refresh failure` |

### 分支表 — 矩阵格（MX）

| # | 条件/分支 | 输入 | 期望 | 测试用例名 |
| --- | --- | --- | --- | --- |
| MX-1 | 桶峰值 | 同日 3 桶线索 3/7/1 | 格值 7（非 11、非桶数 3） | `matrix: peak of buckets` |
| MX-2 | thread_count 缺省 | events 2 + folded 1 / 皆缺 | 回退 3 / 显示 `?` 且不参与误报 | `matrix: peak fallback unknown` |
| MX-3 | 零线索 | 有分区、全桶 0 | 格值 '0'、可点击开编年（零线索合法） | `matrix: zero clue cell openable` |
| MX-4 | 断档 | 无分区 | '—'、禁用、aria「无分区」 | `matrix: gap dash disabled` |
| MX-5 | 行尾总数 | 任意 | 无行尾合计列/读数（DOM 反向断言） | `matrix: no row total` |
| MX-6 | 列数 | ≤31 / >31 | 单屏网格 / 容器内横滚（min-width 随列数），页面不横滚 | `matrix: wide window inner scroll` |
| MX-7 | 轴外数据 | base 含可见区间外日期 | 不计入格值/编年；行序仍用全窗口值 | `matrix: out-of-view ignored for cells` |
| MX-8 | 今天列 | date=today / 锚点早于 today | 今天列高亮仅一处；锚点日单独「数据截止」提示 | `matrix: today vs anchor` |

### 分支表 — 演进带（EV）

| # | 条件/分支 | 输入 | 期望 | 测试用例名 |
| --- | --- | --- | --- | --- |
| EV-1 | 点径公式 | total 0/1/2/3/4/16/≥36 | 6/11.0/13.1/14.7/16.0/26→24（封顶）/24 | `evolution: dot size formula` |
| EV-2 | 节点数字 | total N | 显示 N（日合计，非桶峰值） | `evolution: node count is total` |
| EV-3 | 标签阈值 | total 2/3；可见日 31/32 | ≥3 且 ≤31 日显示首线索标题；否则不显示 | `evolution: label threshold with width` |
| EV-4 | 最密三日 | 任意数据 | 区块不存在（DOM 反向断言） | `evolution: no densest block` |
| EV-5 | 态势句 | snapshot 有/无；summary>80 字 | 折叠「最近结算态势 · as_of（非本视窗总结）」+ 可展开逐字 / 待结算占位 + 时间线照常 | `evolution: narrative latest only` |
| EV-6 | 长窗 | 可见日 >31 | LaneTrendChart 渲染（轴刻度 = max/⌊max/2⌋/0，X 轴首/中/末日期）+ 折叠逐日索引 | `evolution: trend chart axes` |
| EV-7 | 空心节点 | 无分区日 | 空心禁用、无标签、不造事件 | `evolution: hollow node` |

### 分支表 — 当日编年（CH）

| # | 条件/分支 | 输入 | 期望 | 测试用例名 |
| --- | --- | --- | --- | --- |
| CH-1 | 默认选日 | 进入/切泳道 | selectedDate = 所选泳道可见区间内最后有分区日 | `chronicle: default latest content day` |
| CH-2 | join 命中 | section_id 在详情 | 全量 threads（>5 条可读）；标题与详情逐字一致 | `chronicle: full threads by section id` |
| CH-3 | join 未命中 | 报告存在但 section 缺失 | 降级 base ≤5 标题 +「另有 K 条折叠未读取」+ 重试；不借相似标题/别日 | `chronicle: degrade to returned titles` |
| CH-4 | 当日无报告 | 列表无该 period_date | 同 CH-3 降级 + 说明 | `chronicle: no report that day` |
| CH-5 | 摘要展开互斥 | 连点两条 | 仅后一条展开 | `chronicle: single expansion` |
| CH-6 | 相关文章 | ids 空 / N 篇 / 单篇失败 | 空文案 / 按需 N 次缓存读取 / 局部错误 + 重试不阻塞 | `chronicle: related articles on demand` |
| CH-7 | 首尾导航 | 最早/最新有内容日 | 按钮「已到…」禁用；中间日就地切换保持滚动 | `chronicle: content day nav bounds` |
| CH-8 | 历史日报动作 | selectedDate ≠ 最新报告日 | 保留带完整日期的「打开 N 日报」；latest 主入口仍在头部且唯一 | `chronicle: historical report action kept` |
| CH-9 | 三指标 caption | 桶峰值/合计/桶数 | 三值并列且口径各注（非同名混称） | `chronicle: metrics caption` |
| CH-10 | 只读 | 全交互 | 写请求计数 0 | `chronicle: read-only` |

### 分支表 — 读取层（RD，`useLaneDayReading`）

| # | 条件/分支 | 输入 | 期望 | 测试用例名 |
| --- | --- | --- | --- | --- |
| RD-1 | 列表缓存 | 同板块两次不同日（该板块列表未失效） | `getBoardDailyReports` 仅1次（days=365列表1份），切板块/卸载失效后允许重读 | `reading: report list cached` |
| RD-2 | 详情缓存 | 同报告两日来回切（未被逐出） | `getDailyReportDetail` 每报告仅 1 次；逐出后重读允许再请求（见 CA） | `reading: report detail cached` |
| RD-3 | 文章缓存 | 同文章跨 thread 展开（未被逐出） | `getArticle` 仅 1 次；逐出后重读允许再请求（见 CA） | `reading: article cached` |
| RD-4 | epoch | 切板块/切日在途请求 | 晚到响应丢弃，不渲染旧板块内容 | `reading: epoch guard` |
| RD-5 | 失败 | 列表/详情/文章各失败路径 | 分层错误 + 重试；互不污染缓存成功项 | `reading: failure isolation` |

### 分支表 — 有界缓存（CA，`useBoardConsole.test.ts` / `useLaneDayReading.cache.test.ts`）

| # | 条件/分支 | 输入 | 期望 | 测试用例名 |
| --- | --- | --- | --- | --- |
| CA-1 | 空缓存 | 当前板块首次进入，各缓存0项 | source/报告/文章按需读取，不预取全年详情；报告列表索引至多1份 | `cache: empty loads on demand` |
| CA-2 | 四档与365去重 | 同板块7→14→30→365再观察全年，重复请求在途365 | source成功缓存至多4档；365的档/池共用一次请求及成功项，不生成第五项 | `cache: four tiers share pool` |
| CA-3 | 报告上限 | 0→13→14份成功详情 | 缓存依次0/13/14，无提前逐出；命中不请求 | `cache: report capacity fourteen` |
| CA-4 | 报告超1 | 14份后读第15份不同报告 | Map FIFO逐出最先入项，仍14份；被逐出项再次读取允许GET；失败不入成功缓存 | `cache: report fifteenth evicts first` |
| CA-5 | 文章上限与超1 | 0→19→20→21篇不同文章 | 0/19/20/20份；第21篇成功后FIFO逐出最先入项，缓存命中复用 | `cache: article twentieth and twenty-first` |
| CA-6 | 失败不占位 | 列表/source/报告/文章首次或刷新失败后重试 | 失败无成功项、不逐出成功项；旧成功项保留可读；重试确实发GET | `cache: failures never enter success map` |
| CA-7 | 生命周期释放 | A板块缓存非空→B板块→卸载；A在途响应晚到 | 旧板块四档/列表/报告/文章全部失效；B列表独立1份；卸载释放；晚到响应不回填 | `cache: board switch and unmount release` |
| CA-8 | 当前阅读项稳定 | 当前展开报告/摘要/文章为FIFO首项，随后成功写入第15报告/第21文章将其逐出 | 缓存仍≤14/20；正在读的内容/展开态/选日/泳道/焦点/滚动不消失或回顶；显示态独立持有，不以Map有无决定渲染 | `cache: eviction keeps current reading stable` |

### 分支表 — 右轨（RS）

| # | 条件/分支 | 输入 | 期望 | 测试用例名 |
| --- | --- | --- | --- | --- |
| RS-1 | 区块顺序 | 挂载 | DOM 顺序：体检→构成候选→话题追踪→来源 | `rail: block order` |
| RS-2 | 评语首句 | 长 llm_comment | 首句两行预览 + 完整依据入口；标注「最新快照 · 非所选历史评分」 | `rail: health excerpt latest only` |
| RS-3 | 候选 top3 | 候选 0/2/5 | 0=空文案；2 全列；5 列 3 + 实际清单数 | `rail: candidates top3` |
| RS-4 | 追踪管理作用域 | 点管理 | 参数仅 boardId；persistentTopicId 仅定位链接 | `rail: watch board scope` |
| RS-5 | 来源窗 | 7/30/90 切换 + 未取数档 | 各档独立请求；无数据档显示未取数不补 0；「独立统计」标注在场 | `rail: source windows independent` |
| RS-6 | 全局入口 | 任意 | 右轨无新建板块/扩充/回填/规则/合并/标签池按钮（反向断言） | `rail: no global actions` |
| RS-7 | 失败隔离 | 体检/来源 500 | 局部错误 + 重试；矩阵照常（既有 X-2 继承） | `rail: failure isolated` |

### 边界值清单

| 变量 | 边界值 | 期望 | 测试用例名 |
| --- | --- | --- | --- |
| 档天数 | 7 / 14 / 30 / 365（6/8 归一） | 四档各自请求与轴长；非四档归一 14 | `range: tier normalization` |
| 观察宽度 | 1 / 2 / 365 / 366（越界） | 合法渲染 / 合法 / 全池 / 拒绝 | OX-4 |
| 桶峰值 | 0 / 1 / 2 / 5 / 大值 | '0' 可开；>0 显示数值；未知 '?' | MX-2/3 |
| 日合计 | 0 / 1 / 4 / 36 | 点径 6 / 11 / 16 / 24（封顶） | EV-1 |
| 可见日数 | 30 / 31 / 32 | 网格 / 网格（31 含） / 趋势图+折叠 | EV-3/EV-6 |
| thread_count | 缺省 / 0 / 正常 | 回退 / '0' / 数值 | MX-2 |
| folded_count | 0 / N | 无提示 / 「另有 N 条折叠」（仅降级态） | CH-3 |
| 池边界 | 池首/池尾 ±1 日 | 含端点；越界拒绝 | OX-4 |
| 报告详情缓存 | 0 / 13 / 14 / 15份 | 按需 / 未满 / 恰满 / FIFO逐出至14；当前读项保持 | CA-1/3/4/8 |
| 文章详情缓存 | 0 / 19 / 20 / 21份 | 按需 / 未满 / 恰满 / FIFO逐出至20；当前读项保持 | CA-1/5/8 |
| source缓存/列表索引 | 0 / 4档；0 / 1份 | 仅当前板块，365共用；切板块/卸载释放 | CA-1/2/7 |
| 时区 | 23:59:59 / 00:00:00（Asia/Shanghai） | 「今天」归属切换（既有口径） | `matrix: today vs anchor` |

### 不适用划除（留痕）

- 并发写/线程安全：本阶段零写路径（观察窗/编年/右轨全只读），不声称线程安全；竞态仅测读通道 epoch（OX-10/RD-4）。
- SQL/迁移/repository 测试：后端零改动、零迁移，无新 SQL；不触 testcontainer（⑤b 对位：前端纯逻辑用例不提 DB）。
- 输入组的纯分隔符/特殊字符/大小写：唯一自由文本为固定格式日期，已在 OX-3 覆盖非法格式；划除理由见变体节。
- LLM/外部依赖失败路径：本阶段不调 AI、不触发生成/回填（红线），无新增外部写依赖；读取失败按 RD-5/CH 覆盖。

## 真实浏览器主故事与手动豁免计划（8B.4）

- 环境：静态发布 `:5100`（`NUXT_PUBLIC_API_BASE=/api pnpm generate` 铺盘，标准环境），真实库板块 1974 + 一个无活跃泳道有日报版块（如 50413/246030）。
- 视口：1440×900、1920×1080、2560×1440（4K150% 等效）、375×667、390×844；editorial + dark 双主题。
- 主故事（每条含点击后到达状态断言）：
  1. 进 1974 → 14 档默认 → 点多桶日格 → 编年全量标题 + 单条摘要 + 相关文章 → 收起回触发点（滚动/焦点保持）。
  2. source14 拖 brush 选 10/06–10/26，再拖至恰7/30/365日及14档自身区间 → sourceDays仍14，名单/前端source全轴行序/休眠/候选逐项不变，B仅补A同ID历史；最后明确点7档才换基准。
  3. 输入池外日期 → 明确文案 + 原视窗不变。
  4. 365 档 → 趋势图 + 折叠逐日 → 矩阵容器内横滚、页面无横滚。
  5. 右轨顺序/首句预览/来源7/30/90 → 总览六项全局治理逐一可达并保留确认/取消流程（不执行业务写）→ 返回矩阵；点「研究工作台」进真实`/tags/:boardId/research`（BoardEnrichmentPanel），返回保留泳道/日期/范围/阅读/滚动，不以此判旧关系/关键事件已恢复。
  6. 无活跃泳道版块 → 已有日报仍可读（既有主故事不回退）。
  7. 窄屏 375/390 → 泳道摘要 + 下方编年 + 无横向溢出（scrollWidth ≤ 视口）。
- 机械断言：弹层（若有）elementFromPoint 命中本体；ellipsis 截断 title 可达；长内容（≥1 超长标题/评语）在场态验证。
- 手动豁免留痕项：真机触摸手势（headless 合成指针代验，标注代验）、暗色全图视觉舒适度（k3 截图抽检）。
- 证据落 `ui-verification/matrix-stage-<date>/report.md`；A-ID 对账沿总账，不新增「通过」结论给未验区域（A-56/A-59 等仍留账）。

## 实际命令（8B.3 各批执行时回填真实输出；本批未跑任何前端命令）

- 计划命令形态：`cd front && pnpm test:unit app/features/tags/composables/useBoardConsole.test.ts app/features/tags/components/console/observationRange.test.ts --maxWorkers=2`（按组扩展文件列表；参数不带 `--`）；`pnpm exec eslint <受影响 TS 文件>`；.vue 文件由父统一 typecheck/构建覆盖（仓库 eslint 配置不含 vue 插件，如实记录）。
