<!-- complexity: simple -->
<!-- ui-impact: minor -->
<!-- diagrams: none(只是把矩阵底部那组「整行全是 —」的泳道行换成一条折叠条加一个普通列表，没有新接口、没有状态流转、没有跨层调用，画出来跟现在的区别只有一个展开/收起) -->
<!-- constraint-domains: semantic-board, daily-report -->

## Why

矩阵底部「窗口内未进日报」组用**整行网格**表达「这行没数据」：每条泳道占一整行、逐日格全是不可点的「—」，扫矩阵看色块时纯是噪音；同一个信息（泳道名 + 日期）在组标题、行头、行尾标签重复三遍；位置在休眠折叠条之后却默认展开多行，看着像矩阵后面贴了块补丁（2026-11-27 用户反馈「在这里看着太突兀」）。它要解决的事——活跃泳道不能因为窗口内没分区就整条消失、连 ⋯ 都没处点——仍然成立，只是载体太重。

## What Changes

- **覆盖组退出矩阵网格**：`ObservationMatrixGrid` 不再为「窗口内未进日报」的泳道渲染 `ObservationMatrixLaneRow` 与逐日「—」格，矩阵网格只留给窗口内有读数的行。
- **改为折叠条入口 + 列表**（位置不变：休眠条之后、矩阵底部）：
  - 一条与休眠折叠条同款的 summary「窗口内未进日报 · N」+ 泳道名清单（超长截断，title/aria 保留全文），**默认折叠**——泳道名常显在条上，「不以缺席表达沉默」的原意不变；
  - 点击展开一个**普通列表**（不是网格，没有日期格）：每项 = 泳道名 +「最近命中 MM-DD」/「无记录」+ 行尾 ⋯（重命名 / 编辑泳道 / 归档泳道，与矩阵行头同一套菜单与就地改名）。
- **文案改准**：列表项的日期标签由「最近一次」改为「最近命中」——它是话题最近一次被线索命中的日期，不是进日报的日期；原写法与组名「未进日报」读起来自相矛盾（「最近一次 11-26」却「未进日报」）。
- **不变**：数据源 `GET /semantic-boards/:id/topics` 与过滤规则（`coverageLanes.ts`）；空清单整组隐藏；读取中不渲染；读取失败只出一行「窗口外泳道清单未取到 · 重试」；不参与 A 档排序、休眠分组与右轨名单；泳道名不可选中（不触发行选中）；不用危险色。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `workshop-board-console`：Requirement「窗口内未进日报的活跃泳道可见」——展示载体由矩阵网格行（默认展开、逐日「—」格）改为折叠条入口 + 列表（默认折叠、无日期格）；行头 ⋯ 能力、空组隐藏与失败降级不变；列表项日期标签改为「最近命中」。

## Impact

- **前端代码**（`front/app/features/tags/components/console/`）：
  - `ObservationMatrixGrid.vue`：覆盖组模板由「summary + 网格行」改为「折叠条 + 列表」，新增展开态（组件本地状态）；对应样式。
  - `ObservationMatrixLaneRow.vue`：`coverage` / `coverageLastSeen` 分支随网格行用法一并移除（列表项不复用整行；⋯ 菜单与就地改名的复用方式在 design 定）。
  - 绑定链 `consoleStageBinding.ts` / `consoleStageTypes.ts` / `ObservationMatrix.vue` / `BoardConsoleShell.vue` / `useConsoleShellData.ts` / `coverageLanes.ts`：预期零改动（props/emits 不变，展开态为 Grid 本地状态）。
- **测试**：`ObservationMatrixGrid.test.ts`、`ObservationMatrix.test.ts`、`BoardConsoleShell.integration.test.ts` 中覆盖组断言按新载体改写；`ObservationMatrixLaneRow.test.ts` 中 coverage 用例（如有）随分支删除。
- **spec**：`openspec/specs/workshop-board-console/spec.md`（delta：上述 Requirement）。
- **文档**：`docs/reference/flow/semantic-board.md`「业务约束与不变量」中「窗口内未进日报的活跃泳道」一句 + 变更溯源。
- **后端 / API / 数据模型**：零改动。
