# Review · unify-inner-scrollbar-style

> 审查方式：`change-review` 子线程只读审查（2026-11-18），对象 = change 制品 + 12 个改动文件的 diff。
> 意见按 §0.6 review 执行要点分类；处置状态五值封闭。
> 已核实无问题项见文末「审毕无发现（审查范围）」。

### R-1 2026-11-18【High】tasks.md 的 Scenario→测试文件映射表格式违反 scenario-trace 契约，归档门禁硬阻断

- **状态**：已修复
- **事实**：`bash scripts/harness/scenario-trace.sh openspec/changes/unify-inner-scrollbar-style` → exit 1，8 项失败（`映射文件不存在`）。根因是单元格写成散文（`` `path`（token 与声明结构）+ 人工截图 ``），脚本按空白/逗号切词后每个词都当路径校验；脚本也要求表格保持两列。
- **处置**：映射表改为纯仓库相对路径（4 个 Scenario 全指向 `front/app/assets/css/scrollbar-style.test.ts`），散文移到 ui-design / test-cases。复跑 `scenario-trace.sh` → `✓ 通过：4 个 Scenario 映射齐全（自动测试 4 / 人工留痕 0）`。

### R-2 2026-11-18【Medium】"两者都是继承属性"是错的：`scrollbar-width` 不继承

- **状态**：已修复
- **事实**：CSS Scrollbars L1 定义 `scrollbar-color` Inherited: yes、`scrollbar-width` Inherited: no。主线程在部署态 Chromium 152 复测：父级 `scrollbar-width: thin` → 子级 computed `auto`；父级 `scrollbar-color: red blue` → 子级继承同值。原注释/design D4/proposal/delta spec 都按"两者都继承"写，导致 D4 的否决理由（"放全局会把页面级条改细"）不成立。
- **处置**：`main.css` 注释重写为「`scrollbar-color` 继承、`scrollbar-width` 不继承，每个容器必须自己挂类」；design Context/D1/D4、proposal Why/Impact、delta spec、test-cases 全部同步纠正；D4 否决理由改为「放 `html` 会让颜色继承到全站（含页面级杂志条改色）并让全局伪元素失效，而宽度不继承根本解决不了子容器」。

### R-3 2026-11-18【Medium】`hover 取 --color-text-muted` 在 Chromium ≥121 不可实现

- **状态**：已修复
- **事实**：`scrollbar-color` 语法只有 `auto | <color>{2}`，无 hover 变体；而声明标准属性后 Chromium ≥121 忽略同元素 `::-webkit-scrollbar`。故 `.scroll-thin::-webkit-scrollbar-thumb:hover` 只在老 Chromium 兜底通道生效。另 `--color-text-muted` 在 editorial 是 `#8a8a8a`（不透明），"hover 提到 0.35"只对 dark 成立。
- **处置**：保留该伪元素规则但明确标注"仅老 Chromium 兜底"；delta spec 把 hover 从 Requirement 与验收点里删除并写明"SHALL NOT 作为现代浏览器可见行为"；design D3、ui-design（hover 行改为"不作承诺"）、test-cases（hover 行改为不作验收点）同步。

### R-4 2026-11-18【Medium】delta spec 写成仓库级 SHALL，但仍有 3 处内部滚动区不在统一通道上

- **状态**：已修复
- **事实**：全仓核对后「只写 thin 不写颜色」的漏网为 0，但仍有 3 处只走伪元素通道：`.virtual-list`（硬编码 `rgba(26,26,26,0.1)`，dark 下几乎不可见）、`.sidebar-content`（`--color-border-medium` + 0.375rem）、`.content-scroll`（硬编码，且**全仓零使用点**）。
- **处置**：`.virtual-list`、`.sidebar-content` 一并迁到 `.scroll-thin` 并删除各自 4 段伪元素规则（顺带修掉 dark 下不可见的缺陷）；`.content-scroll` 作为死代码整块删除（删前 `grep -rn content-scroll front backend-go docs scripts` 零使用点）。proposal Impact 的文件清单（12 个）与 tasks 2.8/2.9/1.2 同步。

### R-5 2026-11-18【Medium】回归测试留有假绿口子

- **状态**：已修复
- **事实**：原测试只断言"模板里出现过 `scroll-thin`"与"文件不含 `scrollbar-width`"——在 scoped 里加硬编码 `scrollbar-color` 仍全绿；类挂在相邻 wrapper 上也全绿。
- **处置**：重写为 26 例：① 按静态 class 属性做 token 精确比对（排除 `:class`/`v-bind:class`，也避免 `rd-stage` 命中 `rd-stage-wrapper` 这类 `\b` 假绿），逐个断言 11 个容器"带该 class 的元素**本身**也带 `scroll-thin`"；② 迁移过的文件整文件不含 `scrollbar-`（含硬编码色值），`AppSidebar.css` 同；③ 补机械锚：`main.css` 不含 `content-scroll`、页面级杂志条段未被波及。反向验证两次（加硬编码色值 / 改挂 wrapper）均变红，还原后 26 passed。

### R-6 2026-11-18【Low】proposal 的"用户可见变化"含一个当前不渲染的界面

- **状态**：已采纳
- **事实**：`DailyReportDeskIndex`（`.ddi`）全仓无引用点（文本检索无 import、无动态组件名；`DailyReportReader.test.ts` 反断言 `desk-index` 不存在）。
- **处置**：proposal Impact 与 ui-design 受影响状态表均注明「该组件当前无挂载点，本轮只做样式收敛，将来复活时自动生效」；test-cases 变体走查新增该前置。

### R-7 2026-11-18【Low】tasks.md 6.2 的期望结果（typecheck exit 0）在当前树上不可达

- **状态**：已采纳
- **事实**：实现期 typecheck 曾报 `ObservationRangeControls.vue(320,10) TS2322`，属其它 change 的瞬时脏状态（该文件 `git diff` 为空，与本 change 无关）。
- **处置**：6.2 改为「exit 0；若 exit 1 且红点仅该处，记录后继续」，最终复跑已确认 exit 0 全绿。

---

## 审毕无发现（审查范围）

- 「只写 thin 不写颜色」漏网容器：全仓 0 个（`DailyReportMiniLifeline` 成对声明，属刻意豁免）。
- `.scroll-thin` 与 Tailwind 层序：`main.css` 顶层规则无 `@layer`，优先于 Tailwind v4 utility，不会被压掉。
- scoped 覆盖：迁移后 7 个 `.vue` 的 scoped 样式内已无 `scrollbar-*`，不存在同元素双通道冲突。
- 迁移前已存在的继承副作用：`.drm-notes-drawer` 原本就从 `.drm-reader` 继承 `scrollbar-color`，本次无新增回归。
- 测试正则与路径：`cssBlock` 对五个选择器均命中正确块；`it.each` 相对路径解析正确。
- 门禁形态：tasks.md 三固定尾节 + `<!-- doc-impact: none(...) -->` 声明合理（`standard/frontend/theming.md` 无滚动条章节）。
- 未验证项（已在 6.6 以 computed style 覆盖）：headless Chromium 走 overlay 滚动条，像素级截图看不到条，故不依赖截图判定。
