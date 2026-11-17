# review.md · fix-lane-row-gap-zoom

- 审查时间：2026-11-17
- 审查档位：`change-review` 只读档
- 审查范围：制品（proposal / ui-design / design / specs / tasks）+ 实现 diff（4 个文件）

## 总评

实现本身（行距 44、分区下移、半径抽取）经独立验算成立，86 条定向用例全绿、`openspec validate --strict` 通过、画布/纯函数改动等价性无问题。代码侧无需返工。

共 **1 High / 4 Medium / 5 Low**，全部在归档前处置完毕（见下表）。R-4 由「记 known issue」升级为**本次一并修复**（同一几何缺陷，分两次修会让用户再撞一次）。

| 编号 | 等级 | 状态 | 处置要点 |
| --- | --- | --- | --- |
| R-1 | High | **已修复** | 「总占高」口径统一为**圆顶→标签底 = 40**（原 34.5 是「圆心→标签底」，两个量混用）；proposal 表格 / design D1 / spec 定义三处同步 |
| R-2 | Medium | **已修复** | tasks 2.1 改为实际公式（`NODE_BLOCK_H + 4`，并注明刻意不依赖 `defaultMemberRadius`）；4.2 改为「7/30 天相等且 ≥ 44」 |
| R-3 | Medium | **已修复** | design D1–D5 每条补「落点 / 取舍」两行（含被否方案与理由） |
| R-4 | Medium | **已采纳并扩大范围** | 三区统一行距 44；候选/已移出共用堆叠池（原先两者首节点坐标重合）；每区高度按自身堆叠需求；新增 Scenario「候选区与只读参照区同样不重叠」+ 用例 4.5 |
| R-5 | Medium | **已修复** | `labels` 注释改为真实行为（只防 label↔label），并把 label↔圆/卡 横向压盖记为 known issue 指向本文件 |
| R-6 | Low | **已修复** | design D3 的废弃数字（31.2 / 最多 6 个）改为 44 / 2 个 |
| R-7 | Low | **已修复** | ui-design 状态表改为「无节点：最小高 360→416」；Layout Contract 改为「viewBox 宽恒定、高随堆叠自适应」 |
| R-8 | Low | **已采纳** | 测试 4.1/4.3 的硬编码 29 抽成 `LABEL_BLOCK_H` 常量；4.3 阈值从「2r+4」（旧 16px 行距也能过）收紧到「2r+LABEL_BLOCK_H+4」 |
| R-9 | Low | **已采纳（保留单常量）** | `membersBottom` 用 `NODE_BLOCK_H` 会多算 5.5px（保守高估，无重叠风险）；不拆两个常量，改为注释写清语义与理由 |
| R-10 | Low | **已修复** | `defaultMemberRadius` docstring 删掉「画布行距也走这里」，改为明确「画布**不使用**本函数」 |

---

### R-1 2026-11-17【High】「节点纵向总占高 34.5」与公式/常量不符（应为 40），D1 算术链不成立

- **状态**：已修复
- **定位**：`proposal.md` §Why 表格；`design.md` D1；`specs/lane-correction-surface/spec.md` Requirement 1 首段
- **问题**：三处口径互斥——proposal 把半径 5.5 与 5、24 相加得 34.5 却称「纵向总占高」（按占高应为 11+5+24=40）；design D1 写 `= 34.5` 但那行算出来是 40，且 `max(16, 34.5+4)` 应为 38.5 而非注释的 44；spec 首段把「总占高」定义为圆心→标签底（34.5），使其 MUST 文字在 38.5 行距下也成立，与自带 Scenario 的「≥44」冲突。
- **依据**：代码 `NODE_BLOCK_H = 11+5+24 = 40`；测试注释 `2r+29` → 11+29=40；后果是后来者照 D1 复核会得 38.5，而 38.5 正好让标签框底压在下一圆顶上，即本 change 要消灭的缺陷。
- **建议**：三处统一为「圆顶→标签底 = 40」；spec 定义改口径；确保 MUST 文字与 Scenario 的 44 同源。
- **处置**：已全部改为 40；spec 增加「口径警示」段，显式写明「圆心→标签底 34.5 不能作行距基准，按它算得 38.5 等于把缺陷换个数字重演」。

### R-2 2026-11-17【Medium】tasks.md 2.1 / 4.2 与实现、spec 相反

- **状态**：已修复
- **定位**：`tasks.md` 2.1、4.2
- **问题**：2.1 写「`max(16, 2 * defaultMemberRadius(columnWidth) + 4)`」——正是 design D2 否决的按半径推导；4.2 写「7 天的 cy 差更大」——与 spec Scenario、ui-design 映射、实际断言（`expect(gap7).toBe(gap30)`）全相反。
- **依据**：`memberRowGap` 实际不含 `columnWidth`；测试断言 7/30 相等。
- **建议**：改成实际公式与新语义。
- **处置**：2.1 改为 `NODE_BLOCK_H + 4` 并注明「刻意不依赖 defaultMemberRadius」；4.2 改为「相等且 ≥44」。

### R-3 2026-11-17【Medium】design D1–D5 缺「落点 / 取舍」两行

- **状态**：已修复
- **定位**：`design.md` §Decisions
- **问题**：主 spec `design-decision-landing` 要求每条 Decision 有「落点」行与「取舍」行；D1–D5 均缺。
- **依据**：`grep -n "落点\|取舍\|备选" design.md` 仅命中 D5 标题。
- **建议**：每条补两行，无代码落点写「无代码落点：<spec 落点>」。
- **处置**：D1–D5 各补「落点 / 取舍」，取舍均写出被否方案与理由。

### R-4 2026-11-17【Medium】候选/已移出（22px）与只读（12px）行仍存在同类标签压盖

- **状态**：已采纳（本次一并修复，范围扩大）
- **定位**：`LaneCorrectionCanvas.vue` `points` / `zones`；scope 见 tasks 2.2、proposal §What Changes
- **问题**：同一「标签挂在圆下方」几何在这两区未变：候选 r≤5、占高 ≈39 对行距 22；只读 r≈2.6、占高 ≈34 对行距 12。用户原始抱怨「间隔卡死」在这两区依旧成立，而制品读起来像已整体解决。
- **依据**：`zone1Top + slot*22` / `zone2Top + slot*12` 未改；`labels` 由四个集合共同生成。
- **建议**（原）：不动代码，在 known issue 留痕。
- **处置（升级为修复）**：三区统一行距 44；`zoneHeights` 每区按自身同日最大堆叠数计算；`canvasHeight` 累加；**额外发现并修复**——候选与已移出原先各自从 0 计数、起点同为 `zone1Top`，同日两者首节点坐标重合，现共用同一堆叠池；spec 扩为「同一区、同一日期的节点不得重叠」并新增 Scenario；补用例 4.5。

### R-5 2026-11-17【Medium】横向压盖是独立问题，但现有避让机制弱于制品声称

- **状态**：已修复（注释纠正 + known issue 留痕；代码未改，属独立问题）
- **定位**：`LaneCorrectionCanvas.vue` `labels` computed 注释；`design.md` 原「横向另有宁缺勿叠兜底」论证
- **问题**：`labels` 只跟**已放标签**比，不与节点圆比、不与节点卡比，注释却写「与已放标题**或节点卡**冲突则跳过」——描述的行为不存在；design 曾以此论证「不必压半径」，属乐观。
- **判定**：**独立问题，不属本 change 范围**（本 change Requirement 已限定「同一区」垂直距，横向跨日期压盖不违反该 Requirement），但用户截图现象真实可见，不得默默放过。
- **处置**：① 注释改为真实行为，并把 label↔圆/卡 横向压盖标为 known issue 指向本文件；② design 的乐观论证已随 D1 重写移除；③ 后续 change 承接横向避让（`estimateTextWidth` 对中文不低估，无需改）。

### R-6 2026-11-17【Low】design D3 残留旧推导数字（31.2 / 「最多 6 个成员」）

- **状态**：已修复
- **处置**：改为「改动后行距 44 → 只放得下 2 个」，并修正改动前的 slot 估算。

### R-7 2026-11-17【Low】ui-design 两处状态描述被实现推翻

- **状态**：已修复
- **处置**：状态表改为「无节点：画布最小高 360→416（容器不足时纵向滚动）」；Layout Contract 改为「`viewBox` **宽**恒等于容器 `clientWidth`，**高**随堆叠自适应（0/1/4 成员 → 416/456/588）」。

### R-8 2026-11-17【Low】新增测试阈值与实现常量脱钩

- **状态**：已采纳
- **处置**：4.1/4.3 的硬编码 `29` 抽为 `LABEL_BLOCK_H = 5 + 2*12` 并注明须与组件常量同步；4.3 阈值收紧为 `2r + LABEL_BLOCK_H + 4`（原 `2r+4` 在旧的 16px 行距下也能通过）。

### R-9 2026-11-17【Low】`NODE_BLOCK_H` 一常量两义，`membersBottom` 多算 5.5px

- **状态**：已采纳（保留单常量，改为注释说明）
- **理由**：多算 5.5px 是**保守高估**，不会造成重叠；拆成两个近似常量反而增加漂移面。已加注释写清「语义是圆顶→标签底，加在圆心上会多算半个圆，保留是为让行距与区高同源」。

### R-10 2026-11-17【Low】`defaultMemberRadius` docstring 与 design D2/实现相反

- **状态**：已修复
- **处置**：删掉「与画布行距计算都走这里」，改为「`pointAppearance` 默认路径的单一公式源；修正画布走 `memberRadius` 覆盖，**不使用**本函数。把画布行距改回按本函数推导会得出 16px，正是本 change 要修的缺陷」。

---

## 审毕无发现（审查范围内）

- 审毕无发现（依赖链无环：`canvasHeight ← baseCanvasHeight + membersBottom + zoneHeights`，`zones ← membersBottom + canvasHeight + zoneHeights`，`points ← zones + nodeRowGap`；`.lcc-surface` 是滚动容器，`measure()` 读 `clientHeight` 不受内容高度影响，无 measure↔computed 反馈环）
- 审毕无发现（`defaultMemberRadius` 抽取行为等价：0…1200px 以 0.25 步长逐点比对**0 处不一致**；`flags.memberRadius` 覆盖分支未动）
- 审毕无发现（`labels` 的 `y1`/`y2` 都用 `LABEL_TOP_GAP`，避让框与渲染位置同源，无第二处硬编码 `+5`）
- 审毕无发现（未误改：`components/compose/LaneComposeCanvas.vue` 无 diff；`scaledPointRadius` 未被触碰；`MEMBER_BASE_RADIUS` 仅一处声明）
- 审毕无发现（事实论断「半径恒 5.5」：`appearanceOf` 无条件传 `memberRadius: MEMBER_BASE_RADIUS`，与列宽无关）
- 审毕无发现（spec 结构：`openspec validate --strict` exit 0；无同名 Requirement 冲突；Scenario 与 tasks 映射表一致）
- 审毕无发现（测试证据真实：读 `cy`/`r`/`width`/`height` 真实 DOM 属性，无快照、无内部变量）

## 审查方独立验算（记录）

```
MEMBER_BASE_RADIUS = 5.5 (直径 11)
LABEL_TOP_GAP = 5, LABEL_MAX_LINES = 2, LABEL_LINE_H = 12 → 标签 24
NODE_BLOCK_H = 11 + 5 + 24 = 40          ← 圆顶→标签底
memberRowGap = max(16, 40 + 4) = 44

自圆心起算：圆1底 +5.5 → 标签起 +10.5 → 标签墨迹底 ≈ +35
          圆2顶 = +44 − 5.5 = +38.5 → 净空 ≈ 3.4px ✓
标签碰撞框高 = 2×12+4 = 28，框底 = +38.5（与下一圆顶相切 0px，标称 4px 被框的内边距吃掉）
```

## 处置后复验

| 命令 | 结果 |
| --- | --- |
| `pnpm test:unit laneComposeLogic.test.ts LaneCorrectionCanvas.test.ts --maxWorkers=2` | **87 passed**（logic 57 / canvas 30，含新增 4.5） |
| `pnpm exec nuxi typecheck` | exit 0（仅 Nuxt 打印的 DEP0205 deprecation 噪音） |
| `pnpm exec eslint laneComposeLogic.ts` | 0 error |
| `bash scripts/harness/scenario-trace.sh <change>` | 5 Scenario 映射齐全 |
