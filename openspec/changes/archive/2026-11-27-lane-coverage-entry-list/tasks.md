## 1. 前端 · 覆盖组挪出网格（折叠条 + 列表）

- [x] 1.1 `ObservationMatrixGrid.vue`：删除 `.omx-grid` 内的覆盖组（错误行 / summary / `ObservationMatrixLaneRow v-for`）；在 `.omx-scroll` 之后、`.omx-matrix` 内渲染折叠条 `<button class="omx-group-btn" :aria-expanded="coverageOpen" data-testid="coverage-toggle">`（chevron +「窗口内未进日报 · N」+ `coverage-group-lanes` 名字清单 +「展开查看 / 收起」）；本地 `const coverageOpen = ref(false)`。验证：Grid.test「默认折叠只有条、无行无格」「点条 `aria-expanded` 翻转」。
- [x] 1.2 展开列表：`<ul data-testid="coverage-list">` > `<li>` > `ObservationMatrixLaneRow`（`:cells="[]" :axis="[]" :coverage="true" :coverage-last-seen :rename-lane`，只接 `@edit-lane` / `@archive-lane`）；`coverageEntries` 去掉 `cells`；`coverageSummaryText` 只用「、」连名字。验证：Grid.test「展开后 N 项、每项名 + 最近命中 / 无记录、无 `matrix-cell-*`」。
- [x] 1.3 失败行 `coverage-error-row` + `coverage-retry` 随入口挪到同一位置（`.omx-scroll` 之外）。验证：Grid.test / Matrix.test 失败用例仍绿。
- [x] 1.4 `ObservationMatrixLaneRow.vue`：coverage 文案「最近一次」→「最近命中」（tag、`rowTitle`、`nameCardMeta` 三处）；头注补「`coverage` 分支同时服务矩阵底部列表（无格形态）」。验证：`grep -c "最近一次" front/app/features/tags/components/console/ObservationMatrixLaneRow.vue` → 0。
- [x] 1.5 样式：列表 reset（`list-style: none; margin: 0` + 左缩进与折叠条标题对齐）+ 行间点线分隔 + `.omx-coverage-item :deep(.omx-lane) { position: static; cursor: default }`。验证：lint 无空 style；浏览器截图列表项不横溢、行高与矩阵行一致。

## 2. Review

- [x] 2.1 派 `change-review` 聚焦 review（制品 + diff；**旧能力去向表**：覆盖行的 ⋯ 三项 / 就地改名 / 失败重试 / 空组隐藏 / 名字静态不可选，逐项核对在列表项上仍可达（Scenario「覆盖行可维护」标题不动，内容改为列表项）），意见按 High / Medium / Low 处置后 `review.md` 建账。验证：`bash scripts/harness/review-trace.sh openspec/changes/lane-coverage-entry-list` → PASS。
  - 实际：`zai-coding-cn/glm-5.4` 审出 R-1【High】归档 / 编辑弹层泳道名为空（壳层 `laneLabelOf` 只查 A 档名单）、R-2【Medium】视觉走查未建账、R-3【Medium】标签样式挂在 Grid `:deep`；三条均已处置（R-1 回退话题清单 + 集成用例锁住、R-2 补 report.md、R-3 样式归位），去向表无「遗漏」。

## 3. 测试

- [x] 3.1 重写 `ObservationMatrixGrid.test.ts` 的覆盖组 describe：① 默认折叠只有 `coverage-group-summary`（含「窗口内未进日报」「· 2」+ 两个名字），无 `coverage-list`、无 `matrix-lane-*` / `matrix-cell-*`；② 点 `coverage-toggle` → `coverage-list` 2 项、每项含名 +「最近命中 MM-DD」/「无记录」、`aria-expanded="true"`、不 emit `selectLane`；③ 项 ⋯ → 菜单含「重命名 / 编辑泳道 / 归档泳道」，选「归档泳道」→ `emitted('archiveLane')[0]` = topicId；④ 失败行文案 + 点重试 → `emitted('retryCoverage')`；⑤ 读取中 / 空清单 → 条与列表都不渲染。验证：该文件全绿。
- [x] 3.2 调整 `ObservationMatrix.test.ts` 覆盖组三条用例：展开后才有 `matrix-lane-20`；错误行；空 / 读取中不渲染。验证：该文件全绿。
- [x] 3.3 调整 `BoardConsoleShell.integration.test.ts`：覆盖组用例改为「折叠条有名字 → 点开列表有名 + 最近命中 11-09」；新增「列表项 ⋯ → 重命名 → Enter → `updateTopic` 被调且清单重取」。验证：该文件全绿。
- [x] 3.4 `ObservationMatrixLaneRow.test.ts` 补一条：`coverage` 行头 tag「最近命中 MM-DD」、缺日期「无记录」、`cells=[]` 不渲染任何 `.omx-cell`。验证：该文件全绿。

## 4. 文档

<!-- doc-impact: flow -->

- [x] 4.1 `docs/reference/flow/semantic-board.md`「业务约束与不变量」：该组那句改为「…放进矩阵底部、休眠条之下的**折叠条入口**『窗口内未进日报 · N』（网格之外；名字常显、默认折叠；点击展开列表，项 = 名字 + 最近命中日期 + ⋯，不渲染网格行 / 日期格）」，其余口径（不参与行序 / 休眠 / 右轨、失败降级、空组不渲染）保留。验证：`bash scripts/harness/doc-impact.sh verify openspec/changes/lane-coverage-entry-list` → PASS。
- [x] 4.2 变更溯源：上述 flow 文档按 §12 追加本 change 链接。验证：`grep -n "lane-coverage-entry-list" docs/reference/flow/semantic-board.md` 有命中。

## 5. 验证

| Scenario | 测试文件 |
| --- | --- |
| 新建泳道窗口外仍可见 | `front/app/features/tags/components/console/ObservationMatrixGrid.test.ts`、`BoardConsoleShell.integration.test.ts` |
| 展开与收起 | `front/app/features/tags/components/console/ObservationMatrixGrid.test.ts`、`ObservationMatrix.test.ts` |
| 覆盖行可维护 | `front/app/features/tags/components/console/ObservationMatrixGrid.test.ts`、`ObservationMatrixLaneRow.test.ts`、`BoardConsoleShell.integration.test.ts` |
| 同窗口内泳道不重复 | `front/app/features/tags/components/console/coverageLanes.test.ts`（不动，回归） |
| 清单失败诚实降级 | `front/app/features/tags/components/console/ObservationMatrixGrid.test.ts`、`ObservationMatrix.test.ts` |

- `cd front && pnpm lint` → PASS（0 error；7 warning 全在本次未触及的既有文件）
- `cd front && pnpm exec nuxi typecheck` → PASS（0 error）
- `cd front && pnpm test:unit app/features/tags/components/console/ObservationMatrixGrid.test.ts app/features/tags/components/console/ObservationMatrix.test.ts app/features/tags/components/console/ObservationMatrixLaneRow.test.ts app/features/tags/components/console/BoardConsoleShell.integration.test.ts app/features/tags/components/console/coverageLanes.test.ts --maxWorkers=2` → PASS（2026-11-27 20:0x 复跑：5 files / 107 tests passed）
- `grep -c "最近一次" front/app/features/tags/components/console/ObservationMatrixLaneRow.vue` → 0
- `cd front && pnpm build` → PASS（实际经 `bash scripts/dev/deploy-frontend.sh --no-restart` 内的 `NUXT_PUBLIC_API_BASE=/api pnpm generate`，19:53–19:55 nice 19 + 看门狗限载，nuxt build 成功出 `.output/public`；线上 `index.html` 引用的 `_nuxt/BU7iW8bj.js` 与盘上一致）
- agent-browser 走查：折叠态（1440×900 / 1920×1080）、展开态、⋯ 菜单截图 + 「列表不产生页面级横向溢出」结论，落 `openspec/changes/lane-coverage-entry-list/ui-verification/report.md` → PASS（A-1~A-5 全 pass，四张截图在盘）
- `bash scripts/harness/review-trace.sh openspec/changes/lane-coverage-entry-list` → PASS（建账 3 条）
- `bash scripts/harness/ui-verification-trace.sh openspec/changes/lane-coverage-entry-list` → PASS（5 条合法判定条目 + 组件测试映射在盘）
- `bash scripts/harness/doc-impact.sh verify openspec/changes/lane-coverage-entry-list` → PASS（声明 flow，文件 1 个）
- `bash scripts/harness/check-standards.sh --change lane-coverage-entry-list` → PASS（通过 356 / 失败 0 / 警告 0）
- `bash scripts/harness/check-tasks-tail.sh openspec/changes/lane-coverage-entry-list` → PASS
