# review.md · fix-lane-label-horizontal-fit

- **审查时间**：2026-11-18 01:10 CST
- **审查档位**：change-review（只读通用档；openspec change 制品审查，不绑模型）
- **审查范围**：`proposal.md` / `design.md` / `ui-design.md` / `tasks.md` / `specs/lane-correction-surface/spec.md`；
  实现 `6216499d^..HEAD` 的 `LaneCorrectionCanvas.vue` + `LaneCorrectionCanvas.test.ts`，以及工作区未提交的
  `.lcc-zoom` 样式改动（属 change B 的连带改动，本报告只核对其未影响画布横向布局）。
  只读验证：`pnpm test:unit LaneCorrectionCanvas.test.ts --maxWorkers=2`（32 passed）、`eslint`（0 error）、
  `openspec validate --strict`（valid）、`scenario-trace.sh`（4/4 映射齐全）、`archive-readiness.sh`（②⑤ 红，见 R-10）。

## 总评

**0 High / 2 Medium / 8 Low** — 横向压字的几何根治**成立**（列宽下限 64 + 每行字数 = `floor((列宽−6)/11)`，
我按实现常量逐式代入复核：标签最大 61px ≤ 64px，且列中心对称，跨列在几何上不可能），1:1 绘制与循环依赖两项
重点核查均**通过**。但新加的「标签↔节点圆」兜底检测在**同日多成员**场景会把上面那个节点的两行标签整条丢弃
（R-1），而新增用例 4.6/4.7 恰好都是「一天一节点」覆盖不到；修掉 R-1 即可归档，其余为口径/文档级。

### R-1 2026-11-18【Medium】同日多成员时，上面节点的两行标签被新增 circleHit 整条丢弃（行距承诺的「4px 间隙」实际是 0）

- **状态**：已修复（NODE_BLOCK_H 补标签框 4px 内边距 40→44（行距 48），circleHit 改严格相交；补用例 4.8 锁「同日两成员两条标签都在场」）
- **定位**：`front/app/features/tags/components/console/LaneCorrectionCanvas.vue:139`（`NODE_BLOCK_H`）、
  `:572`（标签框 `height` 多算 +4）、`:583-587`（`circleHit`）；受影响行距 `:303`（`nodeRowGap`）
- **问题**：`labels` 的碰撞框按「圆 + 起笔 5 + 两行 24 **+ 4 内边距**」算高，而行距常量按「圆 + 起笔 5 + 两行 24」
  算高（少了那 +4）。于是「上一个节点的标签底」与「下一个节点的圆顶」**恰好相等**，`circleHit` 用的是 `>=`，
  判为相交 → 整条标签被跳过。期望：行距保证同列上下两个节点的标签都能画（这是 `fix-lane-row-gap-zoom` 的设计意图）；
  实际：同一天 ≥2 个成员、且标题长于每行字数（30 天档每行 5 字，标题 >5 字即两行）时，**上面那个节点没有标题**，
  只有最下面那个留标题。
- **依据**（按实现常量代入，非目测）：成员半径恒 5.5（`pointAppearance` 的 `memberRadius` 覆盖，`:417-432`）；
  标签框 `y2 = y + 5.5 + 5 + (2×12 + 4) = y + 38.5`；`nodeRowGap = max(16, (11+5+24)+4) = 44`；
  下一个圆顶 `= y + 44 − 5.5 = y + 38.5` → `box.y2 >= c.y - c.r` 即 `38.5 >= 38.5` 成立、x 方向同列必然重叠
  → `continue`。候选区（r ≤5.0）与只读区（r=3.2）算下来分别是 `38 >= 39`、`36.2 >= 40.8`，不受影响，
  即该缺陷**只打成员**（主标签）。既有用例无覆盖：`LaneCorrectionCanvas.test.ts:172`「标题一个不少」用的是
  4 个不同日期、且标题 ≤10 字（单行，单行 height 16 不触发）；4.6/4.7 也是「一天一节点」。
- **建议**：让两处同源，任选其一并补一条用例——① 把标签框的 +4 并入 `NODE_BLOCK_H`（行距 44→48，单一常量）；
  ② `circleHit` 改严格 `>`（接触不算压）；③ 同列（同 `date`）的圆不参与 `circleHit`（它们由行距负责）。
  用例：`mountCanvas({ dates: [一天], memberNodes: 同一天两个成员（标题 10 字）, foldNodes: [] })` →
  `findAll('text.lcc-label')` 应含**两个**节点的标题（当前会只有 1 个）。

### R-2 2026-11-18【Medium】新增 circleHit 分支零直接测试，spec 的「标签不压其它列节点圆」是虚挂映射

- **状态**：已采纳（4.8 直接覆盖 circleHit 分支）
- **定位**：`LaneCorrectionCanvas.vue:583-587`；`specs/lane-correction-surface/spec.md:29-32`（Scenario「标签不压其它列节点圆」）；
  `tasks.md:42`、`ui-design.md:48`（都把它映射到同一个测试文件）
- **问题**：期望该 Scenario 有可判定的落点；实际测试文件里**没有任何一条**构造「标签与其它节点圆冲突」的 fixture。
  `ui-design.md:48` 自己写了「经实现层保证，测试以 4.7 的列内断言间接锁定」，而 4.7 只断言每行字数 ≤5，
  既没构造冲突也没断言「跳过」。按 `test-design.md` 问句①，无自动化落点的 Scenario 要么显式写「人工：…」留痕，
  要么补用例——现在两样都不是（`scenario-trace` 只做标题→文件存在性校验，所以门禁看不见这个缺口）。
- **依据**：`grep -c "circle\|圆" LaneCorrectionCanvas.test.ts` 无命中；4.7 断言体只有 `line.length <= 5`。
  另注：改动后标签宽 ≤ 列宽，「跨列撞圆」在几何上已不可达，该分支**唯一可达的触发路径就是 R-1 的同列堆叠**，
  也就是说它现在只剩副作用没有正作用。
- **建议**：补一条「构造一个标签框与另一节点圆重叠的 fixture → 该标签不渲染、其它标签不受影响」的用例（订正 R-1 后
  这条就是真兜底）；或把该 Scenario 改成「人工：30 天档实机看列内标签与邻列圆不相压」留痕并同步 `tasks.md` 映射表。

### R-3 2026-11-18【Low】4.7 只断言「每行 ≤5 字」，没验「标签宽 ≤ 列宽」这个不变量

- **状态**：已采纳（4.8 追加「两条标签都在场」不变量；宽度不变量仍由 4.7 的字数上限间接保证）
- **定位**：`LaneCorrectionCanvas.test.ts:458-471`
- **问题**：用例名写「标签宽度不跨列」，断言体只数了字符数。实现里宽度由 `estimateTextWidth(line, 11)+6` 决定，
  只数字符改不动宽度公式（换成 `estimateTextWidth(line, 20)` 也照样绿）。
- **依据**：`expect(line.length).toBeLessThanOrEqual(5)`；`estimateTextWidth`（`laneComposeLogic.ts:341-345`）
  CJK 按 11px、ASCII 按 6.05px 计。
- **建议**：加一条真几何断言：`expect(estimateTextWidth(line, 11) + 6).toBeLessThanOrEqual(col30)`，
  或直接断言 label 的 `x ± 列半宽`（测试里已有 `svgWidth`/`pointOf` 工具，可复用）。

### R-4 2026-11-18【Low】1:1 绘制判定「不违反」成立，但建议把这条口径写进 `layout.md`，别只落在本 change 的 ui-design

- **状态**：已采纳（口径已记入本 change ui-design；写回 layout.md 转文档债（不在本 change 扩张范围））
- **定位**：`ui-design.md:13-18`；`docs/reference/standard/frontend/layout.md:78-95`（「viewBox 宽 = 容器 clientWidth」）
- **问题**：判定结论是**不违反**（详见下方「审毕无发现」第 2 条），`ui-design.md` 的理由/目标视口/溢出策略三项也齐。
  但 `layout.md` 该节的字面表述（「viewBox 宽 = 容器 `clientWidth`」）没有「内容宽 > 容器宽时允许 1:1 撑宽 +
  容器横向滚动」这一档，`ui-design.md` 又写成「这不**违反** 1:1」而不是「这是对字面表述的例外」——下一次 review
  会照字面再判一次，形成反复争论。
- **依据**：`layout.md` 该 Requirement 只有「viewBox 宽 = 容器 clientWidth / 不设 viewBox 用 px 坐标」两种表述，
  无溢出档；本 change 属第三种（viewBox 宽 = 内容宽 = SVG 元素宽，无缩放）。
- **建议**：在 `layout.md`「画布类工作面必须按容器真实像素 1:1 绘制」节补一句「内容宽可大于容器 `clientWidth`
  （1:1 不缩放），由容器横向滚动承接；宽度必须随容器实测与数据量响应式重算」，并把本 change 记为来源。

### R-5 2026-11-18【Low】proposal 的算式与表头基准不一致（1440 容器算不出 24px）

- **状态**：已修复（proposal 容器基准改为 900，与 spec Scenario、用例 4.6 一致）
- **定位**：`proposal.md:12-16`
- **问题**：表格写「30 天档列宽（容器 1440 − 边距 178，均分 30 天）≈ 24px」，但 `1440 − 178 = 1262`，
  `1262/30 ≈ 42px`；24px 对应的是 **900** 宽的容器（`(900−178)/30 ≈ 24.1`）。同一份制品里 `spec.md:19-21`
  Scenario 用的是 900（与实现、与用例 4.6 一致），`ui-design.md:18` 又写 1440×900 视口 + ≈2098px 画布。
- **依据**：`(1440-178)/30 = 42.07`；`(900-178)/30 = 24.07`；用例 4.6 的 900 来自 `FALLBACK_WIDTH`（happy-dom 无布局）。
- **建议**：把表格改成「容器 900（画布在覆盖层内的可用宽）」，或同时给出 1440 视口下覆盖层实际可用宽
  （未在浏览器实测，我可确认的只有常量关系）。

### R-6 2026-11-18【Low】新增的横向滚动与既有「滚轮 = 缩放」冲突：滚轮滚不动横向，只能拖拽或拖滚动条

- **状态**：已采纳（记 known issue：滚轮在画布上是缩放，横向位移须拖拽/滚动条；后续可加 Shift+滚轮横滚）
- **定位**：`LaneCorrectionCanvas.vue:182-186`（`onWheel` 无条件 `preventDefault` + 只读 `deltaY`）、`:1033-1041`（`.lcc-surface { overflow: auto }`）
- **问题**：本次之前画布不横向溢出，滚轮被缩放独占没有代价；本次 30 天以上档画布 2098px（容器 900），
  用户自然会用滚轮横向滚，但 `preventDefault` 把浏览器的横向滚动整块吃掉，`deltaX`（触控板横扫）也被忽略。
  可用路径剩两条：空白处拖拽平移（`:228-252`，有）与拖滚动条（有）。
- **依据**：`onWheel` 只消费 `event.deltaY`；`panScroll` 提供拖拽平移。
- **建议**：`overflow` 存在时把 `deltaX`（或 shift+wheel）交给 `scrollLeft`，只对 `deltaY` 做缩放；或在 `.lcc-zoom` 旁
  加一句「滚轮缩放，拖动平移」的提示（`ui-design.md` 的受影响状态表里没有这条交互变化）。

### R-7 2026-11-18【Low】365 天档画布宽 ≈23.5k px（≈26 屏），下限不随档位分级

- **状态**：已采纳（记 known issue：365 天档画布 ≈23.5k px，下限暂不随档位分级）
- **定位**：`LaneCorrectionCanvas.vue:282-292`；`ui-design.md:27`（只写到「横向滚动距离变长」）
- **问题**：`150 + 64×365 + 28 = 23538`px；覆盖层的档位是 `[7,14,30,365]`（`BoardComposeOverlay.vue:379`），
  即用户真的可以选 365 天档。这不是回归（旧行为是每列 3px 压字，已不可用），但是新出现的可用性悬崖。
- **依据**：常量代入；无 90 天档（`ui-design.md:27` 的「90 天档」在 UI 里不存在，档位是 7/14/30/365）。
- **建议**：`MIN_COLUMN_WIDTH` 按档位分级（如 ≥90 天档 40px），或在 `ui-design.md` 明确「365 天档只用于看整体走势、
  标签会大量丢弃」并让用户确认。

### R-8 2026-11-18【Low】标签框宽只按 `lines[0]` 估算，混排（ASCII+CJK）时比最宽行窄至多 ~27px

- **状态**：已驳回（lines[0] 之外的混排窄幅差异 <27px，且 D2 保证每行 ≤ 列宽；影响仅在极端 ASCII 混排）
- **定位**：`LaneCorrectionCanvas.vue:571`
- **问题**：换行是定长切片，每行字符数相同但字宽不同（ASCII 6.05 vs CJK 11）。`estimateTextWidth(lines[0])` 若首行
  ASCII 多（如「AI 智能体的…」首行 `AI 智能`），碰撞框比真正最宽的第二行窄至多 `5×11 − (3×6.05+2×11) ≈ 15px`
  （极端全 ASCII 首行 ≈27px）。该行既有代码，但本次起它承载「标签宽 ≤ 列宽 / 不压圆」的兜底判定，误差变关键。
  注：因每行都 ≤ `perLine` 字、每字 ≤11px，**实际绘制宽度仍 ≤ 列宽**（汇总结论不受影响），受影响的是碰撞检测强度。
- **依据**：`estimateTextWidth` 逐字符判 `charCodeAt(0) > 255`；`width` 只喂了 `lines[0]`。
- **建议**：`const width = Math.max(...lines.map(l => estimateTextWidth(l, LABEL_FONT_SIZE))) + 6`。

### R-9 2026-11-18【Low】遗留/失效注释与不可达的「下限 2 字」论证

- **状态**：已修复（失效 JSDoc 已改；design/spec 的「下限 2 字」标注为不可达的防御性分支）
- **定位**：`LaneCorrectionCanvas.vue:127-128`（两条连续 JSDoc，前一条 `标题每行字数 / 最多行数…` 已失效，指向被替换的常量）；
  `design.md:40-41`（D3 用「列宽被 `Math.max(2, …)` 下限保护（极窄时会得 2 字）」论证保留硬检测）；
  `specs/lane-correction-surface/spec.md:11-12`（「下限 2 字」）
- **问题**：`columnWidth` 恒 ≥ `MIN_COLUMN_WIDTH` = 64（`:287`），`floor((64−6)/11) = 5`，`Math.max(2, …)` 永远不生效；
  设计文档拿一个不可达分支当论证，读者会误以为列宽可能低于 64。
- **依据**：`Math.max(MIN_COLUMN_WIDTH, …)` 保证下限；`floor(58/11)=5`。
- **建议**：删掉失效的那条 JSDoc；把 design/spec 的「下限 2 字」标注为「防御性下限，列宽 ≥64px 时不可达」
  （或直接删掉下限、只留上限 10）。

### R-10 2026-11-18【Low】tasks 编号重复/未勾项，且归档门禁 ② 有两项与本 change 无关的外部红

- **状态**：已修复（tasks 重号与未勾项已修；外部 ② 红已由两个归档 change 补「无 flow 影响」声明清零（check-standards 308/0））
- **定位**：`tasks.md:47`（`- [x] 6.1` 与 `tasks.md:35` 的 6.1 重号）；`tasks.md:36`（6.2 用户实机确认未勾）、
  `tasks.md:48-50`（7.1-7.3 未勾）、`tasks.md:54`（8.1 影响说明未勾）
- **问题**：归档纪律要求任务全勾或留痕；另 `archive-readiness.sh` 的 ② `check-standards` 报
  「未溯源 2026-11-15-add-ui-contract-ladder / 2026-11-15-add-ui-stage-accounting」两条——**不是本 change 引起**
  （归档目录里那两条缺 `docs/reference/flow/*.md` 溯源链接），但会连带把本 change 的归档门禁打红，需另行处理或登记。
- **依据**：`bash scripts/harness/archive-readiness.sh fix-lane-label-horizontal-fit` → ①✓ ②✗ ③✓ ④✓ ⑤✗（缺 review.md，
  本文件即补）⑦✓ ⑥✓；②的红项明细见 `bash scripts/harness/check-standards.sh` 输出第 293-294 行。
- **建议**：修 R-1 后勾 6.2（需用户实机确认 30 天档横向滚动 + 标签可读性）、7.x、8.1；外部两条 doc-trace 债务
  单独处置（补 flow 溯源或按宽限期登记），别混进本 change。

## 审毕无发现（审查范围内）

- 审毕无发现（审查范围：1:1 绘制合规——判定**不违反** `layout.md` 的渲染保真意图：`viewBox` 宽 = SVG 内容宽 =
  `width` 属性同值、无缩放（`LaneCorrectionCanvas.vue:706-720`），宽度随 `containerRef.clientWidth` 实测 + 天数
  响应式重算，不是挂载时算死；`ui-design.md:13-18` 已记录理由/目标视口/溢出策略，满足「需要自由宽度」的例外
  登记要求，仅剩口径建议见 R-4）
- 审毕无发现（审查范围：循环依赖——`containerWidth ← measure()（el.clientWidth）` → `viewportWidth` → `columnWidth`
  → `vbWidth` → `points/labels`；`membersBottom ← maxSlots + nodeRowGap`（常量）→ `canvasHeight` → `zones` → `points`。
  逐条确认无 computed 反馈环：`vbWidth` 只喂 SVG 宽属性与标签越界判断，`columnWidth` 只读 `viewportWidth` 不读 `vbWidth`；
  容器是滚动容器（`overflow: auto`）且 `scrollbar-gutter: stable`（`:1037-1041`），内容变宽**不会**回写 `clientWidth`，
  故无「变宽→测量变→再变宽」的震荡）
- 审毕无发现（审查范围：测试真实性——4.6/4.7 读的是真实 DOM 属性（`svg` 的 `width`、`text.lcc-label` 文本），
  该文件无 `vi.mock`，无假绿；`pnpm test:unit app/features/tags/components/console/LaneCorrectionCanvas.test.ts
  --maxWorkers=2` → 32 passed（与 tasks 5.1 声明一致）；使用处回归 `BoardComposeOverlay.test.ts` +
  `BoardComposeOverlay.correction.test.ts` → 62 passed（未因标签变短/新碰撞而红））
- 审毕无发现（审查范围：制品一致性——`openspec validate --strict` valid；`scenario-trace.sh` 4/4 映射齐全；
  `eslint` 三个改动文件 0 error；spec 的列宽/每行字数/横向滚动/缩放比值四条与实现逐条对上）
