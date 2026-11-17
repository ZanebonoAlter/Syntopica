# Design: unify-lane-workbench

## Context

本设计只承接 `proposal.md` 的 What Changes 与已批准的 `ui-design.md`，不重新设计界面。D-1..D-10 pinned 保持：全屏 workspace、380px / 自适应 / 300px、推荐只在右栏、节点卡显式写操作、移出不二次确认、首义仍由左栏发起、名称就地编辑、两入口共享状态。

前置状态：P1 `lane-manual-correction-loop`、P2 `fix-lane-metric-provenance` 均已于 2026-11-16 归档；主 specs 已有 `lane-correction-surface` 和成员计数/接近度单一口径。proposal 中的未归档描述是历史残留，不再据此安排前置工作。

已核实的实现边界：

| 事实 | 仓库定位 | 设计影响 |
| --- | --- | --- |
| `lane-dynamics?days=365` 有逐泳道逐日线索数；返回的泳道集合并不等于全部可选 section 集合 | `backend-go/internal/topicgraph/repository/lane_snapshot_repository.go:GetBoardLaneDynamics`；`front/app/api/laneDynamicsWindow.ts:useLaneDynamicsWindowApi` | 编辑曲线可直接取目标泳道；不能把非 active 泳道相加冒充新增可选池 |
| channel store 每板最多四档 FIFO，归 `useBoardConsole` 实例持有，并非全局缓存 | `front/app/features/tags/composables/laneDynamicsChannels.ts:createLaneDynamicsChannelStore`；`front/app/features/tags/composables/useBoardConsole.ts:useBoardConsole` | 复用同一缓存实例需要扩展宿主传递链，不是 import 后即可共享 |
| `ComposeCandidateSection` 无线索数字段；候选只返回完成日报中向量可用的 section | `backend-go/internal/topicgraph/repository/daily_report_manual_topic.go:ComposeCandidateSection`、`GetComposeCandidates` | 新增曲线必须先补数据，不能数 section 或文章 |
| `listLaneSectionThreads` 已批量选取 `section_id,title,scope`，`Clues` 排除 composite | `backend-go/internal/topicgraph/repository/lane_snapshot_repository.go:listLaneSectionThreads` | 可复用现成计数，不复制综合稿判据 |
| days 大于 90 在 repository 钳至 90；显式 from/to 超长或起点越过取数界返回 400 | `backend-go/internal/topicgraph/repository/daily_report_manual_topic.go:resolveComposeWindow`；`backend-go/internal/topicgraph/handler/daily_report_handler.go:parseComposeWindowParams`、`composeCandidates` | 不提供 365 档；不能靠发 days=365 得到一年可选池 |
| 时间控件没有档位配置入口，固定引用 7/14/30/365，事件类型也不含 90 | `front/app/features/tags/components/console/ObservationRangeControls.vue`；`front/app/features/tags/components/console/observationRange.ts:SOURCE_TIER_DAYS` | 「原样复用」与泳道档位合同存在真实缺口，见 OQ-1 |
| 修正模式的推荐在成员清单里，画布没有归属动作；新建已有卡片选入、质心推荐与点形函数 | `front/app/features/tags/components/console/LaneMemberList.vue`；`front/app/features/tags/components/console/LaneCorrectionCanvas.vue`；`front/app/features/tags/components/compose/LaneComposeCanvas.vue`；`front/app/features/tags/composables/laneComposeLogic.ts` | 迁移推荐，不另建成员状态机；统一纯函数，保留两套坐标布局 |

本 change 两份 delta 共 **30 个 Scenario 实例 / 29 个不同标题**（仅「查看与选入分离」重名）；相对主 specs 新增 **5** 个 Scenario，不是四个。代码事实直接记在本制品，不额外写 explore-findings，遵守本轮只新增三个文件的辖区。

## Goals

- 两种画布使用同一套节点视觉与接近度方向；焦点、成员、首义三种状态不混用。
- 编辑模式的左栏、节点卡、右栏通过同一 composable 改成员；一次显式动作对应一次既有归属请求。
- 时间窗具备预览/提交分离、失败保留旧范围、请求防串台；模态不改变矩阵深链。
- 新增曲线计数来自真实非综合稿 thread，未知与已知零分开；测试账本同时盘点被替换的旧断言。

## Non-Goals

- 不改变 topic 生命周期、候选链资格、质心算法、归属事务、关系重建和首义次日生效规则。
- 不新增「立即重算」、批量移出、泳道合并/拆分/转正/归档、当日线索下读或事件线量纲。
- 不扩 `composeWindowMaxDays`，不新增聚合端点，不用 days<=0 绕过 90 天读取全历史向量。
- 不重写矩阵页时间控件，不引入缓存框架，不扩大 channel store 四档容量。
- 不把新建草稿选入误接为编辑模式 PATCH；新建仍须保存成泳道。输入制品对新建布局/出口的泛化表述须由主线程收口（OQ-4）。
- 不在本轮修改输入制品或实现代码，也不把以下 Open Questions 当成已经获批的需求变更。

## Decisions

### D1. 编辑曲线由泳道工作面单独取数
- **结论**: 选择工作面独立读取，而非共享矩阵的 channel store 实例。composable 经 `useLaneDynamicsWindowApi().loadLaneDynamics(boardId,365)` 读取，一次打开只保留一份会话 payload；切板/关闭用请求序号隔离迟到响应。按 topicId 提取该泳道逐日 `clue_count`，不求板级总和；成功加入/移出/加回后重取，失败保留旧曲线并标记未刷新，重试可恢复。目标泳道未出现在响应里时为未知，不擅自造一条全零曲线；画布与左栏仍可用。
- **落点**: `front/app/api/laneDynamicsWindow.ts:useLaneDynamicsWindowApi`；`front/app/features/tags/composables/useLaneCorrection.ts:useLaneCorrection`；`tasks.md` 6.3。
- **取舍**: 不共享 `useBoardConsole` 私有的四档 FIFO：编辑入口不止矩阵，共享需向雷达/管理入口扩传实例，而且归属写后仍需强制刷新。接受矩阵已取一年时再多一次只读请求，以免扩散宿主依赖；不创建第二套多档缓存。

### D2. 新增曲线选择 (a)：给 ComposeCandidateSection 补 clue_count
- **结论**: 在 `GetComposeCandidates` 已筛出有效 section 后，按这些 ID **一次批量**复用 `listLaneSectionThreads`，将 `.Clues` 写为 `clue_count`（非负整数，0 必须序列化，不加 omitempty）；无 thread 的 section 为已知 0，查询失败使请求失败，不伪造 0。不加表列/迁移/端点。API normalizer 归一为 `clueCount: number | null`，旧响应缺字段、null、非法数字为未知；0 保留。新增工作面另取固定 90 天候选池，按 `period_date` 对非 active 的可选 section 去重聚合，**包含无归属节点**；草稿已选仍属该池，不因勾选而扣曲线。带归属却缺状态、或任一应计项缺计数时，该日为未知，不把已知子集冒充合计。该决定只补计数字段，**没有解决一年可选池来源**（OQ-2）。
- **落点**: `backend-go/internal/topicgraph/repository/daily_report_manual_topic.go:ComposeCandidateSection`、`GetComposeCandidates`；`backend-go/internal/topicgraph/repository/lane_snapshot_repository.go:listLaneSectionThreads`；`front/app/api/persistentTopics.ts:normalizeCandidate`；`tasks.md` 2.1、2.2、7.1。
- **取舍**: 不选 (b)：新端点会增加参数、窗口、向量资格与 active 排除的另一条实现链；本次只缺每条 section 的计数，现成批量投影足够，代价是一次 thread 批量扫描（含现有 title 投影）。不选 (c)：lane-dynamics 只覆盖返回的泳道，遗漏无归属/未返回来源，不能称为「全部可选线索」。不能用 section 数、thread 总数或文章数替代。

### D3. 区分背景参考范围与可编排范围，不提供 365 档
- **结论**: 编辑快捷档仅 7/14/30，新增仅 7/14/30/90，起止表单/刷选仍受闭区间最多 90 天与最新 completed 日报减 89 天的取数下界限制。日期来自服务端 `window`，不以今天补末日。编辑的 365 曲线只作历史参考：标清「本泳道 · 线索数」与可编排日期范围，参考区外的历史不能变成可提交窗口。手势、平移、缩放归一到可编排界内；手输倒置/超长/越界则报错并保留旧范围，不静默截断。不得先送 days=365 再把后端回显 90 冒充一年。新增曲线一年背景没有现成来源：当前只能证明 90 天，不能以另一口径补前 275 天；背景如何呈现待 OQ-2 批准后接线。
- **落点**: `backend-go/internal/topicgraph/handler/daily_report_handler.go:composeWindowMaxDays`、`parseComposeWindowParams`；`front/app/features/tags/composables/useLaneCompose.ts:validateCustomRange`；`tasks.md` 6.1、6.2、7.2、7.3。
- **取舍**: 无备选：ui-design「明确不做」已禁止泳道 365 档，放宽后端既违背此约束也扩大向量载荷。参考轴一年不代表可编排一年；不通过灰色可点 365、隐性截断或补零制造该错觉。控件缺配置能力另列 OQ-1，不用 CSS 隐藏按钮规避。

### D4. 时间窗归模态本地，预览不取数、提交成功才切换
- **结论**: 新建与编辑各持有本地已提交范围、控件草稿、手势预览、请求序号和错误；`preview` 只更新选区/反馈，`commit` 校验后才取 candidates，松手一次请求。候选成功后原子替换三栏数据与服务端回显范围；失败恢复旧范围并保留草稿/旧清单。新增沿用 `switchWindow` 的有草稿确认（确认且新取数成功才清草稿）；编辑不把已落库操作当待保存草稿。不得 import 矩阵的 router/query 写回路径；打开、拖动、提交、失败、关闭前后 `days/start/end` 保持逐字相同，关闭也不再 restoreQuery 覆盖矩阵。
- **落点**: `front/app/features/tags/composables/useLaneCompose.ts:switchWindow`；`front/app/features/tags/composables/useLaneCorrection.ts:setWindow`；`front/app/features/tags/components/console/BoardComposeOverlay.vue`；`tasks.md` 6.2、7.2。
- **取舍**: 不复用矩阵 `useObservationWindow` 的 URL 提交流程，也不复用其「档位决定名单、观察窗仅裁显示」语义；泳道候选/画布/推荐必须同一个实际候选窗口。选择受控 UI 复用、业务状态隔离，而不是第二份路由状态。

### D5. 推荐迁为独立业务组件，左栏只留已选侧
- **结论**: 新建 `console/LaneRecommendationList.vue`，从 `LaneMemberList.vue` 迁走推荐行、来源泳道名、估算标注、加入/聚焦事件、pending/empty/error 展示；新组件只接 props/emit，不调 API。`LaneMemberList` 只保留成员/更早/本次已移出，`LaneAnchorScope` 仍承接首义。Overlay 编辑模式接三栏，380px / `minmax(0,1fr)` / 300px，两侧独立纵滚、首义按钮 sticky。原 `LaneMemberList.test.ts` 的推荐三条与「估算 vs 快照」的推荐半边迁到新组件测试；原文件保留快照、计数、更早、移出/加回，并新增左栏无推荐断言；Overlay 旧「不存在右栏」改三栏且不重复。
- **落点**: `front/app/features/tags/components/console/LaneMemberList.vue`；`front/app/features/tags/components/console/BoardComposeOverlay.vue`；`front/app/features/tags/components/console/LaneMemberList.test.ts`；`front/app/features/tags/components/console/BoardComposeOverlay.correction.test.ts`；`tasks.md` 3.1、3.2、3.3、9.4。
- **取舍**: 不在左右各渲染一遍推荐，不把首义移到右栏，不引入新基础组件；迁移职责与断言，而非删掉旧测试让其消失。新建现有候选链/检视能力不因编辑右栏迁移被退役，其栏位称呼冲突见 OQ-4。

### D6. relatedNodeIds 是唯一相关集合实现入口
- **结论**: 扩展现有 `relatedNodeIds`，以原关系、种子 ID 集与可选的「已算出的推荐 ID」计算 **1 跳原关系 ∪ 同一推荐集合**，排除种子本身、去重，不做传递闭包/新边。聚焦高亮的种子是 `{focusId}`，不是全部成员；推荐集合仍由 `centroidRecommendations` 和两模式原有可选资格过滤产生，焦点不参与质心。编辑质心继续含有效更早成员。两画布都经此入口；既有新建「已选成员的相关标题」也调用此函数但仍传已选集，不把标题故事悄悄改成焦点故事。无焦点恢复默认透明度，空质心只保留可证明的一跳关系；active 可因原关系被强调但仍只读。
- **落点**: `front/app/features/tags/composables/laneComposeLogic.ts:relatedNodeIds`、`centroidRecommendations`；`front/app/features/tags/components/compose/LaneComposeCanvas.vue:relatedIds`；`front/app/features/tags/composables/useLaneCorrection.ts:recommendations`；`tasks.md` 1.2、5.1、5.2。
- **取舍**: 不在两画布各写一遍 union/阈值判断；不以点击焦点向量替代成员均值、不把高亮当自动勾选；已有标题入口与新焦点入口共享实现但保留不同种子，避免破坏旧标题可达性。

### D7. 节点表达收进 laneComposeLogic.ts 纯函数
- **结论**: 以现有 `pointAppearance` 为单一入口给出成员/候选/只读的基础半径、填充强度与相关/弱化标志；成员基准半径不读接近度，候选按距离映射且小于成员，只读更弱更小，普通可见点渲染半径不低于 3px。已移出划除、首义虚线、焦点环是正交标志，不相互顶掉。两画布删除自己的半径/透明度公式，共享 `scaledPointRadius` 的缩放下限；坐标、日期布局、折叠带、卡片锚点继续归各画布。显示数值统一 `proximity = 1 − distance`，保留内部距离排序；成员快照与候选估算保持不同名形，null 不显示数字，distance=0 显示 1.00。
- **落点**: `front/app/features/tags/composables/laneComposeLogic.ts:pointAppearance`、`scaledPointRadius`、`cosineFor`；`front/app/features/tags/components/console/LaneCorrectionCanvas.vue:radiusOf`；`front/app/features/tags/components/compose/LaneComposeCanvas.vue:appearances`；`front/app/features/tags/components/compose/LaneComposeInspector.vue`；`tasks.md` 1.1、1.3、4.3。
- **取舍**: 不保留两套画布公式，也不合并两套空间布局为巨型组件。P2 的两把尺子是来源不同，不是方向不同；不继续展示新建卡片/检视栏的「距离」。ui-design provenance 的 `1−cos` 表述与方向相反，按主 spec 的 `1−余弦距离` 实现，并把原文修订交主线程（OQ-4）。

### D8. 节点卡与列表只派发既有 composable 动作
- **结论**: 修正卡片 emit 加入/移出/加回意图，Overlay 与右栏统一调用 `useLaneCorrection` 的 `addMember/removeMember/restoreMember`；不在组件里再存成员集合。点节点/Enter 仅 focus；显式按钮 stop propagation。其它 active 与归属资格未知节点无加入入口；pending 同时禁用卡片及列表的同一节点，composable 防重复请求。成功一次更新派生清单/计数，失败不改事实集合（若展示乐观态则撤回）、保留卡片并报行内错误。移出不弹确认，首义只由左栏发起，Esc 取消焦点不解除归属。
- **落点**: `front/app/features/tags/composables/useLaneCorrection.ts:addMember`、`removeMember`、`restoreMember`；`front/app/features/tags/components/console/LaneCorrectionCanvas.vue`；`front/app/features/tags/components/console/BoardComposeOverlay.vue`；`tasks.md` 4.1、4.2。
- **取舍**: 不新增写 API，不在画布里直接请求，不以 UI 乐观变化代替持久化；不提供移动目标选择器、不自动跨 active 改挂。新建 `toggleMember` 仍只改草稿，不接这些 PATCH。

## Risks

| 风险 | 应对 / 可判定验收 |
| --- | --- |
| 控件复用、曲线历史范围与输入合同不相容 | OQ-1/OQ-2 未收口前停止对应接线，不复制控件、不改 backend 上限、不假装一年可选池 |
| 新字段传了 0 却被 normalizer 吞掉，或部分未知被当总量 | `persistentTopics.test.ts` 断言 0/null/缺失；曲线聚合用例断言一项未知使该日未知、全零仍是零 |
| 新建质心/编辑质心来源不同被误「统一」 | 只统一共享运算/表达，不改变有效已选集；编辑保留更早成员入质心，窗口内 0 成员仍可推荐 |
| 推荐迁移漏掉旧能力 | `test-cases.md` 逐行继承表 + Review 的旧能力去向表；右栏含来源/估算/加入/聚焦/pending/empty，左栏不再含推荐 |
| 迟到候选或曲线请求覆盖新窗口/板块 | 请求序号 + 会话作用域；失败保旧，关闭/切板丢弃迟到响应，不承诺跨线程缓存安全 |
| 365 曲线与手势范围看起来都是一年 | 单列参考范围和可编排范围；表单拒绝越界，手势限界；不让返回 90 却高亮 365 档 |
| 加入一个节点后重算质心导致其它推荐也变化 | 最小验收 fixture 只放一个推荐，故可断言右栏 −1；真实数据只断言该节点退出推荐，**不**要求推荐总长永远精确 −1 |
| 三栏挤占画布、操作按钮被裁 | 两桌面档量 380/300px、中心 clientWidth>0 与 SVG 1:1；侧栏纵滚、sticky 按钮矩形在可视区；<768px 不渲染工作台 |
| 改窗口触发未经批准的语义变更 | 新建有草稿先确认，编辑保留已落库事实；「取消」冲突见 OQ-3，不实现撤销已落库操作 |

## Migration Plan

1. 不做数据库迁移、数据回填或重建日报。后端先发布增量 JSON 字段 `sections[].clue_count`，保留既有端点/状态码/窗口/候选链/early_members 语义；旧前端忽略新字段。
2. 前端按共享纯函数 → 字段归一化 → 右栏迁移 → 卡片写操作 → 高亮 → 时间窗接线顺序实施。OQ-1/OQ-2/OQ-3/OQ-4/OQ-5 由主线程按下表收口后再执行受影响任务；没有隐含授权扩窗或改新建保存方式。
3. 受影响 Vitest 与 Go 包定向验证，真实库效果核对只 SELECT；agent-browser 在静态 `:5100` 走两模式故事，出桌面双视口/双主题及窄屏禁用证据，落 `ui-verification/report.md`。本制品阶段不跑这些实现验收、不声称已通过。
4. 前端实现验证通过后由主线程错峰执行 `bash scripts/dev/deploy-frontend.sh`；不与 Vitest、build、浏览器验收并行。制品提交本身不部署。
5. 回滚前端可恢复旧工作面，后端新增响应字段可保留或回退；已经提交的成员/首义/改名事实不会因 UI 回滚撤销，不安排自动反向写库。

**部署后影响与需要的操作**：实现后编辑泳道推荐移到右栏，节点卡可显式加入/移出，顶部增加时间视窗；点节点仍只是查看。无需用户重新生成数据或执行 SQL。旧后端没有 `clue_count` 时新增曲线显示未知，不补 0；编辑曲线失败不影响既有成员/首义可读。

## Open Questions

这些是**输入合同收口项**，不是交给实现线程猜的产品决定；本轮不改 proposal/ui-design/specs。标记「阻塞」的任务不得按建议自行开工。

| ID | 冲突 / 缺口与证据 | 建议与责任 / 阻塞范围 |
| --- | --- | --- |
| OQ-1 | ui-design 要 `ObservationRangeControls` 原样复用、不改本体；实际组件硬编码 7/14/30/365，无法给新增 90、隐藏 365、区分一年背景和 90 天可选界 | 请主线程/用户批准**最小向后兼容配置扩展**（泳道档位/可选界/最大跨度，矩阵默认不变），或修订复用合同；未批准不改组件，不用克隆/CSS 隐藏。阻塞 6.1、6.4、7.3 的控件接线 |
| OQ-2 | ui-design 同时要求背景 365 池、compose-candidates 来源与 <=90 取数；D2(a) 补计数并不能产生前 275 天真实可选池。days<=0 全历史虽然存在，但会绕过成本边界，不能视作允许一年 | 建议将「365 历史背景」限定编辑；新增只展示真实 90 池。若坚持新增真实 365，须另批准不含向量的聚合能力/范围变更，不偷偷退回方案(c)。主线程/用户二选一；阻塞 7.4 与新增背景最终验收 |
| OQ-3 | correction delta 的动作表与候选口径还写「左栏/同栏关联推荐」，与同文右栏专属冲突；workshop delta「取消」说任一模式均不保存，与编辑即时写入、返回保留冲突 | 请主线程修输入文字：推荐仅右栏；新建取消弃草稿，编辑返回不撤销已提交事实。设计不发明编辑全局撤销；阻塞相应 Scenario 最终判定，不阻塞共享函数/字段工作 |
| OQ-4 | workshop delta 新建写「左候选/右检视、保存新泳道」；ui-design D-2/出口及新增空态原型泛称「左已选/右待选、无保存、全部即时」，provenance 又把候选接近度写 `1−cos` | 请主线程明确 D-2/无保存的模式适用范围，避免丢候选链/历史带入/命名保存。方向按已存在主 spec `persistent-topic` 的 `1−距离` 不变；公式文字可直接纠正。未明确前不重排新建栏位、不改其保存协议；阻塞新建布局/出口最终对账 |
| OQ-5 | 新增用户可见「可选线索数（不含 active）」的来源、泳道曲线/本地窗口合同只在 proposal/ui-design，当前两份 delta 未为它声明 Requirement；主 spec 的板级曲线不能替代新增可选池来源锚 | 按 test-design 问句⑤，主线程须补该展示来源/范围的 spec 锚再实现曲线；本轮只能在 test-cases 字段盘点标缺口，不能把 UI 设计表冒充 spec。阻塞 6.3/7.1/7.4 的展示接线与归档，不阻塞 D2 数据字段本身 |

### OQ 收口记录（2026-11-16，主线程）

| ID | 处置 |
| --- | --- |
| OQ-1 | **已批准**：`ObservationRangeControls` 做**最小向后兼容配置扩展**（新增可选 props：档位清单 / 可选界 / 最大跨度；矩阵页默认值与布局零变化，不改既有交互）。写入 ui-design「Component Reuse」与「与批准原型的已知差异」。解除 6.1 / 6.4 / 7.3 阻塞。 |
| OQ-2 | **待用户拍板**（见下）。7.4 与新增背景曲线最终验收仍阻塞至定案。 |
| OQ-3 | **已修输入文字**：`lane-correction-surface` 三处「左栏/同栏关联推荐」改为右栏；「取消」Scenario 拆为「新建模式丢弃草稿 / 编辑模式返回不撤销已落库事实」。解除对应 Scenario 阻塞。 |
| OQ-4 | **已修 ui-design**：D-2 明确三栏只适用编辑泳道、新建泳道栏位与「起名并创建」出口不变；「明确不做」补一条；候选接近度公式由 `1 − cos` 纠正为「`距离 = 1 − cos`，`接近度 = 1 − 距离 = cos`」。 |
| OQ-5 | **已补 spec 锚**：`lane-correction-surface` 新增 Requirement「新增泳道的时间视窗曲线口径」（含「只计可选池」与「可编排范围超限明确区分」两个 Scenario）。解除 6.3 / 7.1 / 7.4 的展示接线阻塞。 |

proposal 的 P1/P2 未完成描述与「后端仅扩 365 才改」条件均为过时文字：本轮新 brief 已明确两项完成，并明确要求评估 D2(a)；按本轮契约编写 D2，交主线程同步说明即可，不再设归档前置任务。

---

### OQ-2 定案（2026-11-16，用户裁决 = 放宽后端到 365）

**用户选择**：放宽后端上限到 365 天，新增泳道可看一年真实可选池。

**实测成本（真库只读 SELECT，2026-11-16）**：

| 口径 | 90 天 | 365 天 | 增幅 |
| --- | --- | --- | --- |
| 最大版块（15220）section 数（有 embedding） | 620 | 1,035 | +67% |
| 该版块有数据天数 | 87 | 141 | +62% |
| 响应体积（含每条 10,244 B 向量） | ≈ 6.2 MB | ≈ 10.4 MB | +4.2 MB |
| 全库跨版块 section 数（对照） | 4,223 | 6,543 | +55% |

**结论**：放宽到 365 **可行**——现行 90 天实现已经是 6 MB 量级，365 天只多 4 MB，不是数量级跃升（数据本身只有约一年，且早期稀疏）。

**实现必须注意的两点**（写进任务）：
1. **曲线数据走轻量路径**：曲线只需要 `period_date + persistent_topic_id + clue_count`，**不需要 `embedding`**。每条向量 10,244 B，跳过它可把 365 天响应从 ≈10 MB 压到 ≈200 KB 量级（约 50 倍）。实现时 SHALL 给曲线路径提供「不带向量」的取数方式（新增轻量参数或独立聚合端点），MUST NOT 为画一条曲线拉全量向量。
2. **放宽范围要同步三处**：`composeWindowMaxDays`（后端常量）、`parseComposeWindowParams` 的校验文案、前端自选起止的上限校验。三处不一致会出现「界面允许、后端静默截断」。

**被否选项**：A（新增只画 90 天真实池）——用户要一年视野；C（用非 active 泳道线索和）——漏无归属节点、口径系统性偏低。

**遗留**：全库 6,543 条 section 若未来数据量增长到数年，365 天窗口需重新评估；建议在任务里加一条「实测 365 天响应体积与耗时，超过 30 MB 或 3 s 需回退到轻量端点」。
