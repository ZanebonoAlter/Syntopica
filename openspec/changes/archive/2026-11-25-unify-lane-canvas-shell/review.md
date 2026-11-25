# Review 台账 · unify-lane-canvas-shell

审查范围：`proposal.md` / `ui-design.md` / `design.md` / `tasks.md` / `stage.md`、2 份 spec delta、`ui-verification/report.md`，以及三个代码文件的本 change diff + 新增 `useLaneCanvasShell.ts` / `LaneCanvasStickyRail.vue` / `LaneCorrectionCanvas.sticky.test.ts`（外壳抽取逐项对照 HEAD 版核对）。

审查方独立确认：未发现触碰数据口径 / 排序键 / 热度公式 / 写路径（红线 28 / 29 / 37 均未被破坏）；编辑侧 `x=14` 无双重平移；`axisClipPath` 左界 = 视口 `axisX0`；`onWheel` 屏蔽时早退不 `preventDefault` 与编排画布原语义一致。

处置汇总：**修复 10 条（含 1 条部分修复）/ 误报丢弃 1 条 / 记录不修 1 条 / 显式化 1 条**。

### R-1 2026-11-25【Medium】spec 把「平移」列为 SHALL 共用同一实现，与实现不符

- **状态**：已修复（spec Requirement 收窄为「缩放 / 测量 / 滚动 / `Esc` 共用同一实现；平移 SHALL 保持指针分流、4px 阈值、吞 `click` 口径一致，手势状态机可各自持有」）
- **依据**：审查指出 `useLaneCanvasShell` 只收拢缩放/测量/滚动/`Esc`，两画布平移状态机仍是 `panStart+isPanning` 与 `gesture` 两份，真正共用的只有 `isDragBeyond`/`panScroll` 纯函数
- **不合并状态机的理由**：两个画布的手势对象本就不同（一边是节点拖拽、一边是落点分区），强行合并会引入行为回归风险，收益不抵

### R-2 2026-11-25【Medium】spec Scenario「内容 SHALL NOT 重排」与实现相反

- **状态**：已修复（改为「固定列内的图元（标签 / 勾选框）SHALL NOT 因列宽变化重排；日期轴起点随折叠带右移 76px、日期列随之整体平移属既有行为，SHALL NOT 被计为本 change 引入的重排」）
- **依据**：审查指出 `axisX0` 参与 `columnWidth`/`vbWidth`/列中心计算，折叠带一出现全部日期列与节点横向平移 76px
- **定性**：原 spec 措辞写错——把「列内图元不重排」误写成「内容不重排」

### R-3 2026-11-25【Medium】6 条 sticky 用例是白盒探针，不应沉淀为永久业务测试

- **状态**：已修复（删除 `LaneCorrectionCanvas.sticky.test.ts`；验证证据转记 `ui-verification/report.md` §2–§4 + §6 台账）
- **依据**：`standard/shared/test-design.md`「白盒是临时验证……完成后清理，不提交为长期业务测试」+「纯 UI 用现行 UI 验收证据，不为过归档额外写组件测试」
- **保留项**：`LaneCorrectionCanvas.test.ts` 的「手势外壳与编排画布同源」组（2 条）——真实漂移的 Bug 回归，符合「真实 Bug 回归」准入

### R-4 2026-11-25【Medium】tasks 4.4 已勾选，但「改动后」截图未实拍

- **状态**：已修复（tasks 4.4 取消勾选，待构建部署后补拍再回勾；`ui-design.md` §Acceptance 改为指向 report §6 台账并写明补拍计划）
- **依据**：`ui-design.md` §Acceptance 要求横滚前/后对照；report §5 自述改动后产物不可得（本批禁 `pnpm build`）

### R-5 2026-11-25【Low】compose 解构出的 `scrollLeft` 从未使用（死绑定）

- **状态**：误报丢弃（`scrollLeft` 用于节点卡横向钳制区间：`LaneComposeCanvas.vue:1278` `const viewLeft = scrollLeft.value`，是活代码）
- **依据**：`grep -n "scrollLeft\\b"` 复核后确认有消费点，不删

### R-6 2026-11-25【Low】「透明度递减 SHALL 只有一处定义」与实现不符

- **状态**：已修复（抽 `stickyShadowOpacity(band)` 与 `STICKY_SHADOW_BANDS` 同处 `LaneCanvasStickyRail.vue` 导出，rail 右缘阴影与 compose 吸顶带下缘阴影两处调用；spec 同时明列「通栏吸顶带底不属该单元」的边界）
- **依据**：审查指出 compose 吸顶带下缘阴影仍内联 `0.28 - (band - 1) * 0.045`

### R-7 2026-11-25【Low】日期轴吸顶带盖住标签列右缘阴影顶部 36px，与编排画布层序相反

- **状态**：已采纳（记录不修，见 report §4.4③ 与本条说明）
- **理由**：两层都是提档色，视觉上只表现为「阴影从带下缘才开始」；调整重绘顺序需把带底与刻度文字拆成两个组、同一个 `translate` 写两遍，引入的耦合大于收益

### R-8 2026-11-25【Low】Interaction Decisions 用了词表外的 `proposed`，且 D2/D3 未附判据编号

- **状态**：已修复（改标 `open`，两条补 `P1` 判据）
- **依据**：`standard/frontend/interaction.md`「交互决策台账带 pinned / forbidden 语义」，状态词表只有 `pinned` / `open`

### R-9 2026-11-25【Low】日期轴吸顶带三件套在 `ui-design` 受影响状态中未登记

- **状态**：已修复（受影响状态表补一行「纵滚可读性（日期轴带）」）

### R-10 2026-11-25【Low】共用吸顶单元落在 `components/compose/`，`console/` 反向 import 形成跨模式目录耦合

- **状态**：已修复（移至 `front/app/features/tags/components/LaneCanvasStickyRail.vue`，两个画布 import 与 `design.md` D4 落点同步更新）

### R-11 2026-11-25【Low】tasks 7.1/7.2 未勾选，但 flow 文档已写入

- **状态**：已修复（已回勾；`grep -c "吸顶"` = 5、`grep -c "unify-lane-canvas-shell"` = 1 两条验收均满足）

### R-12 2026-11-25【Low】拖拽期屏蔽滚轮时早退不 `preventDefault`，滚轮会落到容器原生滚动

- **状态**：已采纳（在 spec 明写「该屏蔽只针对缩放，SHALL NOT 拦截容器原生滚动（与编排画布既有行为一致）」）
- **理由**：这是编排画布既有语义，本 change 以一致性优先，不改行为

### R-13 2026-11-25【Low】两个 Scenario（两侧缩放语义一致 / 同一能力覆盖两支画布）无跨画布对账断言

- **状态**：已采纳（不新增跨画布对账用例——纯 UI 按 R-3 同一逻辑不入永久测试；改在 `ui-verification/report.md` §6 台账以「共用实现 + 编排画布存量用例全绿」显式记录覆盖理由）

## 主线程补核（审查方自述未做的部分）

- 审查方未独立重跑 `pnpm test:unit` / `pnpm lint` / `nuxi typecheck`。主线程独立重跑：外壳抽取后 3 文件 168 用例绿、第三步后 4 文件 174 用例绿、review 修复后 5 文件 **280 用例绿**、`pnpm lint` 0 error（7 warning 全在未触碰文件）、`nuxi typecheck` exit 0。
- 坐标变换复核：日期轴吸顶组的 `translate(0, S)` 与容器 `scrollTop = S` 相互抵消，故 `attr y = DATE_AXIS_H` 在屏幕上恒为 36；带底取 `y = -S`、`height = S + 36` 恰好覆盖视口 `[0, 36]`。审查方与主线程独立得出同一结论。
