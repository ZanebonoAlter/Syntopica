# Review — simplify-workshop-matrix-copy

审查时间：2026-11-18 17:00（CST）；基线：HEAD `67b810b8`（工作树 dirty，仅取本 change 16 文件）。
审查方式：逐文件 diff 对照 `proposal.md` / `design.md` / `ui-design.md` / delta spec `specs/workshop-board-console/spec.md`，
并全仓 grep 已删文案的残留引用；不重跑测试（主线程已报 205 相关用例 + typecheck + 增量 lint 绿）。

### R-1 2026-11-18【Low】矩阵/右轨文案精简的 16 文件改动逐条对账完成，无阻塞项
- **状态**：已采纳（结论：无 High / Medium 阻塞项；对账明细见下方「与制品的一致性」表）

## 结论

**无 High / Medium 阻塞项，可继续临时目录构建。** 改动全部落在「删重复说明 + 把口径挪到数据/操作旁」，
未动接口、统计、日期计算、状态机与交互；16 个前端文件与 change 声明一致（15 改 + 1 新增测试）。

## 与制品的一致性

| 制品要求 | 实现落点 | 核对 |
| --- | --- | --- |
| 矩阵图例只留一条三态（当日线索数不含综合稿 / `—` 无分区 / `?` 数据不可用） | `ObservationMatrix.vue` `.omx-hint`，`matrix-basis-note` 段整体退役 | ✅ 文案逐字一致；测试断言 `matrix-basis-note` 不存在、头部仅 2 个 span |
| 档位口径不常驻，仅跨出档窗提示「历史仅补数据，泳道名单不变」 | 同文件 `v-if="usesPoolHistory"` 的 `matrix-tier-note` | ✅ 默认不渲染；跨窗沿用既有 `usesPoolHistory` 判定（未改判定逻辑） |
| 日报落后显示「日报截至 YYYY-MM-DD」，保留「查看所选日 ↓」 | 同文件 `matrix-data-asof` / `omx-goto` | ✅ |
| 日期格式只由空输入提示 + 校验承担 | `RangeDateForm.vue` 标签去掉 `· YYYY-MM-DD`，`placeholder` 保留 | ✅ `ObservationRangeControls.test.ts` 断言 placeholder 与两个标签文本 |
| 矩阵默认不另写图注；工作面专用图注保留 | `ObservationRangeControls.vue` `caption` 缺省 null 不渲染；`BoardComposeOverlay` 两处 `:caption` 原样 | ✅ 工作面图注未被删 |
| 轴单位只写一次「线索数」 | `TrendAxes unit-label="线索数"`；`.orc-caption` 默认不渲染 | ✅ 测试断言 `.ta-unit` 文本且全文无「非文章数」 |
| 右轨只留结论，入口短标识 | `BoardContextRail.vue`：「查看候选」、`话题候选 · N`，删两段 `.bcr-note` | ✅ 测试断言 `.bcr-note` 数量 0 与文本不含旧注脚 |
| 「历史归属不自动回填」挪到实际收编处 | `BoardHealthDetail.vue` 候选清单上方 `board-health-candidates-note`，仅候选非空时渲染 | ✅ 新增测试 3 例（有/无候选、非候选页签） |
| 「最新快照」口径归体检详情 | `BoardHealthWorkspaceHead.vue` `bhw-latest-snapshot-hint`；`RailHealthBrief` brief 变体不再显示 | ✅ 详情 2 例测试（有/无快照）；右轨 RS-2 反转断言 |
| 来源只补「独立于时间视窗」 | `RailSourcesBrief.vue` `rail-sources-scope` | ✅ 7/30/90 窗口按钮仍在，未重复档位 |
| 窄屏头不重复档位脚注 | `BoardNarrowSummary.vue` 删 `.bns-scope` | ✅ 新增断言 |
| 失败/重试/无数据/焦点/aria 不变 | 各组件 error/retry/empty 分支未动；`aria-label`/`aria-expanded` 未动 | ✅ 既有用例未改语义 |

grep 全仓（`front/app`、`front/tests`）确认 `matrix-basis-note`、`.bcr-note`、`.orc-sub`、`.bns-scope`
只在测试的**反向断言/注释**中出现，无残留选择器、无 e2e 依赖旧文案。

## Low 级观察（不阻塞发布，建议后续顺手清）

1. **`BoardNarrowSummary` 的 `sourceDays` prop 成了死参数**：模板不再引用，父级 `BoardConsoleStage` 仍传。
   可下轮连同 `:source-days` 绑定一起删（本 change 保持最小改动，未擅动）。
2. **注释文案漂移**：`ObservationMatrix.vue` prop 注释仍写「提示数据截止」、
   `useBoardConsole.ts:122` 注释与 `ObservationMatrix.test.ts` 头注释/用例名仍是「数据截止」，
   与界面新文案「日报截至」不同步（仅注释，不影响运行）。
3. **`RailHealthBrief` 的 `variant === 'full'` 分支当前无生产挂载点**（右轨固定 brief；full 变体随
   `BoardWideWorkface` 改用 `BoardContextRail variant="full"` 后已成死路径）。本 change 只是把该分支的
   「最新快照」保留为 full 专属，未引入新死代码；是否整体退役 variant 留待后续。
4. **`ObservationMatrix.test.ts` 的 `findAll('.omx-head > span')).toHaveLength(2)` 对后续新增头部 span 较脆**，
   但正好表达「只剩 kicker + 图例」，可接受。
5. **信息减量属批准范围**：右轨细粒度口径（`Asia/Shanghai · 含起止日`、入口上的「人工激活」、
   「高频共现 ≠ 必须收编」）从常驻位删除，语义分别由校验/空提示、详情页激活按钮、
   `BoardHealthDetail` 收编处承接。delta spec 只要求「分列标清 / 不自动转正」，行为未变，
   判定为文案减量而非约束丢失。

## 发布包（已构建，待主线程复核发布）

- 基线：`git archive HEAD front`（HEAD `67b810b8`，**非 worktree**）解到
  `/home/zanebono/tmp/syntopica-matrix-copy-20261118-170119/`，仅 overlay 上表 16 个文件
  （`cmp` 校验与工作树逐字节一致；`diff` 确认除新增 `BoardHealthDetail.test.ts` 外文件集合 = HEAD）。
- 依赖：`front/node_modules` 软链仓库本机依赖；`NUXT_PUBLIC_API_BASE=/api NODE_OPTIONS=--max-old-space-size=3584 pnpm generate`，
  日志 `generate.log`（exit 0，8 路由 prerender 完成）。
- **产物目录（绝对路径）**：`/home/zanebono/tmp/syntopica-matrix-copy-20261118-170119/front/.output/public`
  （47MB / 1175 文件 / 100 js chunk；`index.html` 入口 `_nuxt/5BCchMxs.js`，同源 `apiBase:"/api"` 与旧产物一致）。
- **chunk 命中证据**：新文案全部命中 `_nuxt/Duew5KT2.js`（421,563B，`data-testid:"matrix-legend"` 等
  console 组件同包；由入口 `_nuxt/5BCchMxs.js` 静态引用，`tags/index.html` 亦指向该入口）：
  - `当日线索数（不含综合稿） · — 无分区 · ? 数据不可用`
  - `历史仅补数据，泳道名单不变` / `日报截至 "+dataAsOf`
  - `独立于时间视窗`
  - `收编只挂到本版块构成；历史归属不自动回填。`
  - `评分与评语取最新体检快照，不随观察日期回溯为历史评分。`
- **旧产物对照（只读比对 `backend-go/frontend`）**：旧 console chunk `_nuxt/DTU7ocn3.js`（422,533B）
  命中全部旧文案，新产物中已不存在；新 chunk `Duew5KT2.js` 旧产物中亦不存在。
  被删 CSS 类 `.omx-basis` / `.orc-sub` / `.bns-scope` / `.bcr-note` 只存在于旧 `index.DaOY6n4j.css`，
  新产物无命中；保留类 `.omx-hint` / `.bcr-hint` / `.orc-caption` / `.bns-head` 迁到新 `index.PEmlUGed.css` 仍在。
  旧文案串（`数字 = 当日线索合计` / `Asia/Shanghai · 含起止日` / `独立统计，不随时间视窗` /
  `名单/排序/休眠按` / `高频共现 ≠ 必须收编`）新产物 0 命中。
- **未发布**：`backend-go/frontend` mtime 仍为 15:15（未覆写）、未重启（`:5100` PID 8027 未动）、未 commit、未 archive。
  说明：新产物与 15:15 旧部署另有 97 个 chunk 哈希差异，属 HEAD（14:16 提交）与当时脏树部署的既有漂移，
  非本 change 16 文件引入；本 change 的增量证据以上述新旧 console chunk/CSS 命中为准。
