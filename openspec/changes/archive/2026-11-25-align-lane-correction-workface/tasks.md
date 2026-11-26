## 1. 后端：首义读端点扩展（anchor_origin / centroid）

- [x] 1.1 扩展 `backend-go/internal/topicgraph/repository/daily_report_anchor_read.go` 的 `TopicAnchorState`：新增 `anchor_origin`（`resolved`/`section_id`/`title`/`period_date`）与 `centroid`（`window_size`/`anchor_weight`/`section_ids`/`members[]`/`degraded`）两段，既有 5 个字段（`topic_id`/`source`/`section_ids`/`override_count`/`effective_from`）**字段名与语义保留**（`override_count` 明确为「首义留痕条数」，见 design D9）。验证：`cd backend-go && go build ./...` 退出码 0，且新增字段的 json tag 与前端 `TopicAnchorState` 类型逐字对齐。
- [x] 1.2 在 `GetTopicAnchorState` 解析首义来源节点：取最新留痕行反解节点集 → 联查该 section 的标题与报告期；`resolved=false` 时来源节点字段 SHALL 返回 `null`（**不推测、不取「最早成员」顶替**）。验证：`cd backend-go && go test ./internal/topicgraph/repository/ -run Anchor` 全绿，其中断言 auto 不可考 → `resolved=false` 且节点为 null。
- [x] 1.3 装配 `centroid` 段：窗口大小取既有配置 `persistent_topic_centroid_window`（默认 30）、α 取既有配置 `persistent_topic_centroid_anchor_weight`、成员集走 2.1 抽出的取数实现、`degraded` 按「可解析向量 < 2 或首义不可用」判定。验证：`cd backend-go && go test ./internal/topicgraph/repository/ -run Anchor` 全绿，含 `degraded` 真/假两态断言。
- [x] 1.4 `backend-go/internal/topicgraph/handler/daily_report_anchor_read_handler.go` 透传新字段，响应**只增不改**。验证：`cd backend-go && go test ./internal/topicgraph/handler/ -run Anchor` 全绿，断言既有 5 个字段名仍出现在响应 JSON 里。
- [x] 1.5 **零写副作用**：读端点路径 SHALL 不写表、不触碰 `centroid`/`embedding`、不置 `centroid_pending_recompute`、不触发重算、不回填快照。验证：`cd backend-go && go test ./internal/topicgraph/repository/ -run Anchor` 全绿，用例断言读前后的 `centroid` 值、`centroid_pending_recompute` 与相关表行数不变。

## 2. 后端：滚动窗口取数单点实现

- [x] 2.1 从 `backend-go/internal/topicgraph/repository/daily_report_topic_repository.go` 的 `ComputeTopicCentroid` **逐字**抽出只读 `ListCentroidWindowSections(topicID, window)`（谓词「归属本话题 + embedding 非空」→ 排序「报告期倒序、同期 id 倒序」→ `LIMIT window`），**不改写 SQL**。验证：diff 审查确认 SQL 字面保留 + `cd backend-go && go build ./...` 退出码 0。
- [x] 2.2 `ComputeTopicCentroid` 改为调用该实现取窗口成员（窗口大小仍取既有配置，默认 30），质心公式、α 混合、退化口径一律不变。验证：`cd backend-go && go test ./internal/topicgraph/...` 全绿（质心相关既有用例逐条绿）。
- [x] 2.3 **等价性用例**：断言「质心计算所用窗口成员集合」与「读端点返回的 `centroid.section_ids`」**逐 id 相等**；并锁定排序与上限（窗口 30 → 只取最近 30 条、同期按 id 倒序）。验证：`cd backend-go && go test ./internal/topicgraph/repository/ -run 'Centroid|Window'` 全绿。

## 3. 后端：手动建泳道首义留痕

- [x] 3.1 `backend-go/internal/topicgraph/repository/daily_report_manual_topic.go` 的 `CreateManualTopic`：在**同一事务内**、归属改写之后、`RebuildBoardRelations` 之前追加一行 `topic_anchor_overrides`（`source=selected_sections`、`input_summary` = **实际参与聚合的** section id 列表），使读端点返回 `source=manual` 且可反解首义节点集。验证：`cd backend-go && go test ./internal/topicgraph/repository/ -run ManualTopic` 全绿，断言留痕行存在且 id 列表 = 参与聚合的 section。
- [x] 3.2 **事务性**：留痕写入失败 SHALL 回滚整个建话题事务（话题创建与归属改写一并回滚），SHALL NOT 留下「有话题、无留痕」的半成品。验证：`cd backend-go && go test ./internal/topicgraph/repository/ -run ManualTopic` 全绿，含注入留痕失败的用例（断言无话题、无归属改写）。
- [x] 3.3 **历史不回填**：SHALL NOT 新增任何针对存量手动话题的补写路径或迁移脚本；存量话题 SHALL 继续按 `auto` 读为「来源不可考」。验证：`grep -rn 'topic_anchor_overrides' backend-go/internal/topicgraph/repository/ | grep -i backfill` 无命中 + 用例断言存量手动话题读出 `source=auto`。
- [x] 3.4 **不新增来源枚举**：留痕写入模式沿用既有 `selected_sections`，SHALL NOT 新增枚举常量。验证：`grep -rn 'selected_sections' backend-go/internal/topicgraph/` 的取值集合未新增。

## 4. 前端：修正画布拖拽改归属

- [x] 4.1 `front/app/features/tags/components/console/LaneCorrectionCanvas.vue`：在既有 `onSurfacePointerDown` 上分流——命中可拖节点（**排除**只读参照区与左缘「更早」折叠带节点）→ 进入拖拽候选；位移 > 4px（复用 `front/app/features/tags/composables/laneComposeLogic.ts:isDragBeyond`）才起拖；未过阈值仍算点击（只改选中态）。不可拖节点光标为 `default`、按下不启动拖拽。验证：`cd front && pnpm test:unit LaneCorrectionCanvas --maxWorkers=2` 退出码 0。
- [x] 4.2 **落点按分区判定**：实现「成员区 / 候选区（含已移出）/ 只读参照区」命中测试；拖入成员区 = 加入（无归属）或加回（已移出），拖出成员区 = 移出，拖到只读参照区 = 不可落点（不发起任何写操作）。**X 轴是日期事实，SHALL NOT 参与落点判定**。验证：`cd front && pnpm test:unit LaneCorrectionCanvas --maxWorkers=2` 退出码 0，覆盖三类落点各一例。
- [x] 4.3 **写路径复用**：`front/app/features/tags/composables/useLaneCorrection.ts` 的 `addMember`/`removeMember`/`restoreMember`（与节点卡按钮、左栏勾选同一条路径、同一份成员集合）；落库期间该节点进既有 `pendingIds`（半透明 + 不可再次拖拽）。验证：`cd front && pnpm test:unit useLaneCorrection --maxWorkers=2` 退出码 0。
- [x] 4.4 **失败乐观回滚 + 节点内联标错**：写请求失败时节点立即回原分区、成员集合复原；错误以节点内联形态呈现（红描边 + `title`/`aria-live`），**不弹窗**、不中断后续连续拖拽。验证：`cd front && pnpm test:unit LaneCorrectionCanvas --maxWorkers=2` 退出码 0，含失败回滚与「失败后可继续拖下一条」两例。
- [x] 4.5 手势纪律：拖拽期间抑制文本选中、指针捕获只在判定为拖拽那一刻启用、拖完 250ms 内吞 click、Esc / `pointercancel` 取消回原位且**不发任何写请求**。验证：`cd front && pnpm test:unit LaneCorrectionCanvas --maxWorkers=2` 退出码 0，含「取消不落库」一例。
- [x] 4.6 落点视觉：成员区高亮 / 不可落点区弱化 + ghost 跟随（复用既有 `.lcc-drag-ghost*` 与 `.lcc-canvas__dropzone` 样式语言）。验证：`cd front && pnpm test:unit LaneCorrectionCanvas --maxWorkers=2` 退出码 0。

## 5. 前端：候选分档、尺寸图例与首义标记

- [x] 5.1 候选桶改**批内相对归一**：复用 `front/app/features/tags/composables/laneComposeLogic.ts:relativeProximities` 与 `pointAppearance` 的 `proximityOverride`/`recommendRange`/`strengthRange`；**成员与只读参照不传 override**（尺寸固定，接近度恒为快照原值）。验证：`cd front && pnpm test:unit LaneCorrectionCanvas --maxWorkers=2` 退出码 0，断言相对归一只作用于候选桶、成员尺寸不变。
- [x] 5.2 成员基准半径与布局链**同批**调整（`MEMBER_BASE_RADIUS` / `NODE_BLOCK_H` / `nodeRowGap` / `membersBottom` / `zones`）；行距基准仍按「圆顶→标签底 + 4px」推导，SHALL NOT 改用固定常量。验证：`cd front && pnpm test:unit LaneCorrectionCanvas --maxWorkers=2` 退出码 0，同日不重叠断言仍绿。
- [x] 5.3 画布底部图例改**尺寸图例**（小圆—大圆），SHALL NOT 继续用单一固定圆表示「大小=接近度」。验证：`cd front && pnpm test:unit LaneCorrectionCanvas --maxWorkers=2` 退出码 0。
- [x] 5.4 **首义标记语义修正**：已生效首义成员与待提交勾选各一种**可区分**标记（前者当前无标记，是既有错位）。验证：`cd front && pnpm test:unit LaneCorrectionCanvas --maxWorkers=2` 退出码 0，含两种标记不混淆一例。
- [x] 5.5 修正画布高度贴合容器的契约复核（既有 `baseCanvasHeight`/`canvasHeight`/`zones` 口径）：内容少时填满容器、余量归候选区与只读参照区、**成员区不因富余高度下移**。验证：`cd front && pnpm test:unit LaneCorrectionCanvas --maxWorkers=2` 退出码 0，含「内容少时无空白带」「成员区位置不随富余下移」两例。
- [x] 5.6 **节点卡标题完整可读**（实机反馈）：`.lcc-card__title` 去单行省略，完整展示、按需换行（`white-space: normal` + `overflow-wrap: anywhere`），卡高沿用聚焦切换后实测口径。验证：`cd front && pnpm test:unit LaneCorrectionCanvas --maxWorkers=2` 退出码 0，含「标题样式允许换行且无省略号」一例。
- [x] 5.7 **话题带强调色横条移除**（实机反馈）：删 `.lcc-band-bar` rect 与 `.lcc-band-tick` 刻度线（模板 + 样式），标签与中性基线保留，布局常量不动。验证：同上，含「画布无 `.lcc-band-bar` / `.lcc-band-tick`」一例。
- [x] 5.8 **点空白画布取消聚焦**（实机反馈）：新增 `clear-focus` emit——聚焦在场且轻点落在空白处（未平移、未落在节点 / 节点卡 / 缩放条 / 折叠带勾选框）才抛；`BoardComposeOverlay.vue` 接 `correction.clearFocus()`。验证：`cd front && pnpm test:unit LaneCorrectionCanvas BoardComposeOverlay --maxWorkers=2` 退出码 0，含「空白轻点抛 / 平移松手不抛 / 落在节点上的轻点不抛 / 宿主接线卡收起且零写请求」四例。

## 6. 前端：首义来源与滚动窗口只读展示

- [x] 6.1 `front/app/api/dailyReports.ts`：`TopicAnchorState` 类型扩展（新增 `anchor_origin` / `centroid`），既有字段不动；新字段缺失时前端 SHALL 有兜底（后端未升级不崩）。验证：`cd front && pnpm exec nuxi typecheck` 退出码 0 + `cd front && pnpm test:unit dailyReports --maxWorkers=2` 退出码 0。
- [x] 6.2 `front/app/features/tags/composables/useLaneCorrection.ts`：读回并派生 `anchorOrigin` / `centroid` 状态（loading / error / empty 三态）；读失败 SHALL **不阻断**画布拖拽、节点卡按钮与左栏勾选等写入口。验证：`cd front && pnpm test:unit useLaneCorrection --maxWorkers=2` 退出码 0。
- [x] 6.3 `front/app/features/tags/components/console/LaneAnchorScope.vue`：新增首义来源一行（可考 → 标题 + 日期；不可考 → 「创建时写入 · 来源不可考」，不显示任何节点）与可折叠「参与滚动计算的节点（N/30）」组（默认收起）。验证：`cd front && pnpm test:unit LaneAnchorScope --maxWorkers=2` 退出码 0。
- [x] 6.4 **命名隔离**：滚动窗口计数用「滚动窗口 N」/「参与滚动计算的节点（N/30）」，与既有「窗口内 N · 全部 M」**分开命名**（后者是成员总数）；`degraded` 为真时该组文案为**中性退化口径**（「质心当前为退化口径（窗口向量不足或首义不可用）」），SHALL NOT 写死两种分支之一。验证：`cd front && pnpm test:unit LaneAnchorScope --maxWorkers=2` 退出码 0，含两把尺子不同名一例 + 「文案不含『退回首义向量』」一例。
- [x] 6.5 首义来源/滚动窗口块 SHALL NOT 触发任何写动作（读端点零写副作用 + 前端不主动写）。验证：`cd front && pnpm test:unit LaneAnchorScope useLaneCorrection --maxWorkers=2` 退出码 0。

## 7. 前端：编排画布高度贴合容器与不可落点视觉

- [x] 7.1 `front/app/features/tags/components/compose/LaneComposeCanvas.vue`：`canvasHeight` 由「只按内容算 + 460/560 地线」改为 **`max(容器实测高度, 内容所需高度)`**，富余高度**全部**给底部散料区，**本次泳道行与候选链行位置不变**。验证：`cd front && pnpm test:unit BoardComposeOverlay --maxWorkers=2` 退出码 0，含「单泳道时画布填满高度」「行位置不变」两例。
- [x] 7.2 **反馈环与吸顶语义**：容器高度用实测 `clientHeight`（横向滚动条占位后的值），高度计算 SHALL **不反向依赖「是否溢出」**（溢出判据只用于吸顶）；内容**恰好贴合容器**（不溢出）时第 0 行 SHALL **不吸顶**。验证：`cd front && pnpm test:unit BoardComposeOverlay --maxWorkers=2` 退出码 0，含「刚好贴合不吸顶」「超出才吸顶」两例。
- [x] 7.3 补 **`.is-drop-other`** 样式（拖到非目标行的「不可落点」视觉，与落点高亮明确不同）——该 class 当前绑定但**全仓无任何 CSS 规则**；修正画布的分区高亮复用同族 `.lcc-` 样式，不另起类名。验证：`cd front && pnpm test:unit BoardComposeOverlay --maxWorkers=2` 退出码 0，断言「不可落点」态有实际样式。
- [x] 7.4 接线：`front/app/features/tags/components/console/BoardComposeOverlay.vue` 把新增交互面接进编排/修正两种模式（拖拽、落点视觉、相对归一、首义只读块）。验证：`cd front && pnpm test:unit BoardComposeOverlay --maxWorkers=2` 退出码 0 + `cd front && pnpm exec nuxi typecheck` 退出码 0。

## 8. 测试

- [x] 8.1 **后端 API 主线**：anchor 读端点的 API 主线用例（新增字段齐备、既有字段不变、auto 不可考 → `resolved=false` + 节点 null、`degraded` 两态），留在 `backend-go/internal/topicgraph/handler/` 与 `repository/` 的永久用例里。验证：`cd backend-go && go test ./internal/topicgraph/...` 退出码 0。
- [x] 8.2 **后端真实回归**：手动建泳道首义留痕的事务性用例（含「留痕失败 → 整体回滚」）保留为永久回归用例，防止未来把留痕挪出事务。验证：`cd backend-go && go test ./internal/topicgraph/repository/ -run ManualTopic` 退出码 0。
- [x] 8.3 **前端组件测试**：`front/app/features/tags/components/console/LaneCorrectionCanvas.test.ts` 覆盖拖拽三态（加入 / 移出 / 加回）、`pendingIds` 防重入、失败回滚与内联标错、相对归一只作用于候选桶、尺寸图例、两种首义标记、不可拖节点。验证：`cd front && pnpm test:unit LaneCorrectionCanvas --maxWorkers=2` 退出码 0。
- [x] 8.4 **前端组件测试**：`front/app/features/tags/components/console/LaneAnchorScope.test.ts` 覆盖首义来源三态（可考 / 不可考 / 读失败）、滚动窗口计数与成员计数的口径隔离与命名隔离、`degraded` 文案。验证：`cd front && pnpm test:unit LaneAnchorScope --maxWorkers=2` 退出码 0。
- [x] 8.5 **前端 composable 测试**：`front/app/features/tags/composables/useLaneCorrection.test.ts` 覆盖 `anchor_origin` / `centroid` 的归一与派生（含 degraded、读失败不阻断写入口）。验证：`cd front && pnpm test:unit useLaneCorrection --maxWorkers=2` 退出码 0。
- [x] 8.6 **前端回归**：`front/app/features/tags/components/compose/LaneComposeCanvas.test.ts` 覆盖编排画布高度贴合容器、余量给散料区、不溢出时不吸顶、「不可落点」态有实际样式。验证：`cd front && pnpm test:unit LaneComposeCanvas --maxWorkers=2` 退出码 0。
- [x] 8.7 **测试准入自查**：永久业务测试只留 API 主线与真实缺陷回归；白盒探针/分支分析用完即弃，不提交（`docs/reference/standard/shared/test-design.md`）。验证：diff 内无临时探针文件残留、无 `.only` / `skip` 遗留。

## 9. 文档

<!-- doc-impact: api, database, flow -->
<!-- doc-impact-excuse: architecture=本 change 不动架构骨架；standard=只读 standard 未修改；configuration=无配置项变更（窗口 30 / α 0.5 均沿用既有）；deployment=无部署流程变更 -->

- [x] 9.1 `docs/reference/api/daily-reports.md`：anchor 读端点新增 `anchor_origin` / `centroid` 字段与 `override_count` 口径说明（首义留痕条数）。验证：`grep -n "anchor_origin" docs/reference/api/daily-reports.md` 有命中。
- [x] 9.2 `docs/reference/flow/topic-graph.md`：手动建泳道首义留痕（同一事务、不新增来源枚举）、历史不回填、滚动窗口取数单点实现。验证：`grep -n "ListCentroidWindowSections\|首义留痕" docs/reference/flow/topic-graph.md` 有命中。
- [x] 9.3 `docs/reference/flow/daily-report.md`：修正画布拖拽改归属的即时落库与失败回滚口径、首义来源三态（`resolved=false` 不虚构）。验证：`grep -n "resolved" docs/reference/flow/daily-report.md` 有命中。
- [x] 9.4 `docs/reference/database/tables/daily-report-watch.md`：`topic_anchor_overrides` 的写入来源补「手动建泳道（建话题时写入）」。验证：`grep -n "topic_anchor_overrides" docs/reference/database/tables/daily-report-watch.md` 有命中。
- [ ] 9.5 归档后按 `docs/reference/开发执行规范.md` §12.2 在 flow 文档的「变更溯源」表补本 change 链接。验证：归档后 `grep -rn "align-lane-correction-workface" docs/reference/flow/` 有命中。

## 10. 验证

- [x] 10.1 **后端门禁**：`cd backend-go && golangci-lint run ./... && go vet ./... && go test ./internal/topicgraph/... && go build ./...`，期望全部退出码 0。
- [x] 10.2 **前端门禁**：`cd front && pnpm lint && pnpm exec nuxi typecheck`，期望 0 error。
- [x] 10.3 **前端单测（按文件，参数不带 `--`）**：`cd front && pnpm test:unit LaneCorrectionCanvas LaneAnchorScope useLaneCorrection BoardComposeOverlay LaneComposeCanvas --maxWorkers=2`，期望全绿。
- [x] 10.4 **制品与文档门禁**：`openspec validate align-lane-correction-workface`（期望退出码 0）、`bash scripts/harness/check-tasks-tail.sh openspec/changes/align-lane-correction-workface`（期望「尾三节与 doc-impact 声明标记齐全」）、`bash scripts/harness/doc-impact.sh verify openspec/changes/align-lane-correction-workface/`（期望退出码 0）。
- [x] 10.5 **实机验收（agent-browser，双视口）**：1440×900 与 1920×1080 各走一遍——① 拖候选进成员区（计数 +1、刷新后仍在）、② 拖成员出成员区（计数 −1、可加回）、③ 候选圆点大小差异肉眼可辨且图例一致、④ 首义来源三态与滚动窗口计数、⑤ 编排画布填满高度且下方无空白带、⑥ 拖到非目标行出现「不可落点」态；结论建账 `ui-verification/report.md`（minor 档：验收映射指名道姓 + report.md ≥1 条判定条目）。验证：`bash scripts/harness/ui-verification-trace.sh openspec/changes/align-lane-correction-workface` 退出码 0。
- [x] 10.6 **部署后影响与操作说明**：完工汇报必须含「部署后用户可见行为变化」「需要用户手动执行的操作」「旧数据如何降级」三节（AGENTS.md 强制）。验证：汇报文本三节齐备，且「旧数据降级」明确写出手动泳道显示「创建时写入 · 来源不可考」、自动话题同理。
- [x] 10.7 **实机复验三处补丁**（agent-browser）：编辑泳道 → 聚焦长标题节点（卡内标题全文换行）→ 成员行上方无强调色横条 → 轻点旁边空白画布（卡收起、成员数不变）→ 再点节点重新聚焦；截图建账 `ui-verification/round3-*.png`，结论追加 `ui-verification/report.md`。验证：report.md 新增判定条目且与截图文件名对应。

## 11. Review

- [x] 11.1 派子线程聚焦 review（审查制品 + diff），意见按 High/Medium/Low 处置后 review.md 建账
