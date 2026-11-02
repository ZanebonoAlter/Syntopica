# G2 网格与折叠组交付报告（tasks 8C.3）+ 前置小门禁 A（G1-R3 关闭）

> 线程：全新短线程、当前本写域唯一 writer（未 resume G1、未派子线程、未提交/归档/部署）。
> 顺序：**A 先补 G1-R3 真红→绿**（只 `useLaneDayReading.ts` + 其 test）→ A 全绿后才做 **B（G2 网格与组）**。
> 未跑（按 brief 红线）：全量 `pnpm test:unit`、`nuxi typecheck`、`pnpm build`、全树 lint、浏览器/agent-browser、Go/DB/SQL、dev 服务、部署。**不声称**这些通过；Vue 文件亦**未**经 eslint（配置不覆盖 `.vue`）。

## A. G1-R3（生成后 anchor 前移 → 索引需求覆盖 369→365，force 新鲜列表被旧大覆盖挡掉）

### A.1 真红（先写测试，证旧代码失败）

命令（`cd front`，参数不带 `--`）：

```
pnpm test:unit app/features/tags/composables/useLaneDayReading.test.ts --maxWorkers=2 -t 'R3 —'
```

结果：**2 failed | 45 skipped (47)**，失败断言与缺陷一一对应：

| 新增用例 | 红阶段失败断言（原实现实测） |
| --- | --- |
| `anchor 推进后 force：有效 days 不缩到小于当前成功覆盖，新报日立即可见` | `expected [ '2026-10-22' ] to deeply equal [ '2026-10-22', '2026-10-28' ]`——生成后新报日 **不可见**（365 请求回来被 `index.days(369) <= days(365)` 挡掉，返回旧 369 数据） |
| `无成功索引但已有更大在途请求：force 不缩其覆盖，新鲜列表仍可写入` | `expected last "spy" call to have been called with [ 1974, 369 ]` → received `[ 1974, 365 ]`——force 真发 365，把在途 369 的覆盖意图缩掉（池首 2025-10-25 可能被挤出索引） |

复现方式完全真实、无 mock 特判：`harness()` 的 `anchorDate` 本来就是闭包函数，测试用 `h.setAnchor(TODAY)` 把 anchor 从 `2026-10-24`（池首 2025-10-25 → 369 天）推进到 `2026-10-28`（池首 2025-10-29 → 365 天），`TODAY` 固定不变——即真实的「生成完成 → anchor 前移 → 覆盖需求收缩」路径。

### A.2 最小修复（只 1 个 hunk，`requestIndex`）

`front/app/features/tags/composables/useLaneDayReading.ts`：新请求的有效覆盖取三者较大值，仍整体替换单份索引（不合并、不猜测旧数据）：

```ts
const effectiveDays = Math.max(
  days,
  index.value?.days ?? 0,                                        // 当前成功索引覆盖
  shared !== null && shared.epoch === epoch ? shared.days : 0,    // 同板块在途请求覆盖
)
// entry.days / runIndexRequest(...) 都用 effectiveDays
```

- 非 force 路径**行为零变化**：`loadIndex` 只在「索引为空或需求更大」时才进 `requestIndex`，此时 `max` 恒等于请求值；`shared.days >= days` 的在途合乘提前返回也未动。
- force 路径：`369 → max(365, 369, …) = 369`，新鲜列表满足 `index.days <= days` 正常写入，新报日可见且覆盖不缩水。
- 未改接口/缓存契约/选日/normalizer/`.vue`；`indexSeq` 请求序、旧响应「转交最新结果」、`indexLoading` 归属、双 epoch、R1/R2 全部保留。

### A.3 真绿（同一命令）

`pnpm test:unit ...useLaneDayReading.test.ts --maxWorkers=2 -t 'R3 —'` → **2 passed | 45 skipped (47)**。

### A.4 G1 原 6 文件集合（只跑一次）+ 本组 TS eslint + 相邻回归

| 检查 | 命令（`cd front`） | 结果 |
| --- | --- | --- |
| G1 6 文件集合 | `pnpm test:unit app/features/tags/composables/useLaneDayReading.test.ts app/features/tags/composables/laneDayReadingCache.test.ts app/api/normalizers/laneDayReading.test.ts app/features/tags/components/console/dayOtherTopics.test.ts app/features/tags/components/console/laneConsoleView.test.ts app/features/tags/components/console/useConsoleSelection.test.ts --maxWorkers=2` | **6 files / 118 tests passed**（旧 116 + R3 新增 2；不把新例冒旧已覆盖） |
| 本组 12 个 `.ts` eslint | `pnpm exec eslint`（useLaneDayReading(.test)、laneDayReadingCache(.test)、normalizer(.test)、dayOtherTopics(.test)、laneConsoleView(.test)、useConsoleSelection(.test)） | **exit 0**（0 error 0 warning） |
| 相邻回归（只读跑，未改其源） | `pnpm test:unit .../LaneChronicle.test.ts .../BoardConsoleShell.test.ts --maxWorkers=2` | **2 files / 66 tests passed**（与历史同数，未破） |

文件 SHA256（before 取自上轮 R1/R2 修复报告并已被父复核一致；after 为本线程收尾实测）：

| 文件 | before | after | 行数 |
| --- | --- | --- | --- |
| `front/app/features/tags/composables/useLaneDayReading.ts` | `f47887ca08406206…` | `4c6e309130d62554…` | 513 → 522 |
| `front/app/features/tags/composables/useLaneDayReading.test.ts` | `c656ec7a36016ada…` | `1bdef5ab51a20141…` | 871 → 917 |

→ **R3 关闭**，tasks 8C.2 已按本报告重勾（保留父 R1/R2 历史与旧 106/116/66 记录，不冒称新覆盖）。

## B. G2 交付（纯受控网格与组）

父后续核验：三源文件after SHA与回包一致，dormant-toggle仅1处(wrapper内为原生button)，已看正式行→其他组→休眠summary的实际DOM。父实际复验R3筛选命令→1 file/2 passed/45 skipped；复验ObservationMatrixGrid+DayOtherTopicsGroup→2 files/21 passed，均exit0。此为父最小重测，非父重跑118/55/93全套；真实几何/主题/焦点仍留G5。

### B.1 写域与 SHA（开工前指纹缺失，不能重构冒充）

本线程未在开工前记录两处既有Vue文件的SHA，当前也无可用于还原该时刻内容的已保存快照，因此不能补造before指纹。目录untracked不妨碍当时直接计算SHA，缺失原因是未采集，不是Git未跟踪就无法采集。父仅复算after三源文件SHA与交付一致，不称已做完整before差分；后续G3必须开工采集授权写域的现文件SHA。**新**文件before=不存在，须在开工确认。

| 文件 | 动作 | after SHA256 | 行数 |
| --- | --- | --- | --- |
| `console/ObservationMatrix.vue` | 改（0 行网格保留/内联空态/报日列头与组接线） | `ad5a43251a31402055120ec9bc73afaf2612fa9ac94587673509a765359316aa` | 330 |
| `console/ObservationMatrixGrid.vue` | 改（列头三态/组顺序/休眠 summary 前移/共用行格视觉） | `b6695bc5f8bca1871277cced8a7aac01cbd20a95f2b078f272684fd03a5a2726` | 646 |
| `console/DayOtherTopicsGroup.vue` | 新建（组 summary + 自身行，纯受控） | `92f040816205e0663171acd9c1fbc5195b4daf18eefc9c2a58eee12957e7b275` | 143 |
| `console/ObservationMatrix.test.ts` | 改（G2 集成/组序/三态/只标所选日） | `a8714644e1b0b539b34a2c59ca9e403a35659bb142f86d6246616d5416fb2dcb` | 587 |
| `console/ObservationMatrixGrid.test.ts` | 新建（网格层列头/组序/旧事件） | `3cbec322243e5770898663741a7152f2093e72cdd5708c64d65e1e3d42d892e3` | 180 |
| `console/DayOtherTopicsGroup.test.ts` | 新建（GRP/OTH-12/状态分流/布局锚） | `fecffd972a4ad068d32a4c9071cda65ea08772d4130d48bb661468c858559c86` | 221 |

未触碰：Shell/Stage/consoleStageTypes/LaneChronicle/BoardNarrowSummary/OtherTopicReading、`api/**`（含 normalizer/dayOtherTopics/laneConsoleView/useConsoleSelection）、后端、`docs/reference/**`、原型、部署文件。

### B.2 新增/保留的接口（G4 直接照此接线）

**`ObservationMatrix`（props 全部可选，旧调用零改动）**

```ts
reportDates?: ReadonlySet<string> | null   // 省略=未接线；null=已接线未就绪/失败；Set=已知报日列头可选
indexLoading?: boolean                     // 列头提示「读取报日…」
indexError?: string | null                 // 陈旧提示；已知报日仍可点（不一刀禁）
otherTopics?: OtherTopicsView | null        // G1 buildOtherTopics 视图（行/计数/状态文案唯一来源）
othersOpen?: boolean                       // 壳层持有；省略=收起（本组件不初始化第二份状态）
selectedOtherSectionId?: string | null      // 阅读槽高亮
```

事件：**保留** `selectLane(topicId)` / `selectDate({topicId,date})` / `toggleDormant` / `retry` / `gotoSelected` / `generate`；**新增** `selectDayHeader(date: string)` / `toggleOthers()` / `selectOther({sectionId: string, date: string, topicId?: string | null})`。

**`ObservationMatrixGrid`**：同样的新 props/事件镜像（`reportDates`/`indexLoading`/`indexError`/`otherTopics`/`othersOpen`/`selectedOtherSectionId` + `selectDayHeader`/`toggleOthers`/`selectOther`），旧事件不变。

**`DayOtherTopicsGroup`**：props `axis: string[]`、`selectedDate: string | null`、`view: OtherTopicsView | null`（必填可空）、`expanded?: boolean`、`selectedSectionId?: string | null`；事件 `toggleOthers` / `selectOther({sectionId,date,topicId?})`。组件零网络、零日期自建、零身份排除/计数/状态词重算（只读 `view.rows/count/statusText/note` 与 G1 导出常量）。

**testid 交接**：`matrix-dayhead-<date>`（报日为 `BUTTON`，非报日为 `DIV`）、`matrix-report-index-hint`、`other-topics-group`、`other-topics-toggle`（wrapper div，内层原生 button 带 `aria-expanded`）、`other-topics-count`、`other-topics-note`、`other-topics-extra`、`other-topics-note-row`、`other-topic-row-<sectionId>`、`other-topic-cell-<sectionId>-<date>`；**保留** `dormant-toggle`（wrapper div + 内层 button，位置前移、选择器不破）、`matrix-lane-*`、`matrix-cell-*`、`matrix-grid`、`matrix-scroll`、`matrix-empty-no-lanes`、`matrix-empty-no-reports`、`matrix-empty-generate`。

### B.3 可见 DOM 变化（逐条对批准稿）

1. **0 正式泳道不再整块空态挡网格**：`emptyState='no-lanes'` 时轴/网格照画（0 行），前面只多一行内联说明「**0 正式泳道** · 可展开/已展开当日其他话题」；`no-reports` 同样降为内联说明并**保留**「去生成第一份日报」按钮。初次 loading/A 错误仍走原骨架/错误分支，不会误列「0 正式泳道」。
2. **报日列头可点**：只有 `reportDates` 内的日期渲染为原生 button（`aria-pressed`=是否所选日，`title/aria-label`=「<日期> · 选这一天」），点它 `selectDayHeader(date)`；非报日为 `DIV`（不可聚焦，`title` 说明「该日没有日报」；索引未就绪时改为「报日索引未就绪，暂不可选」，陈旧时「可能已陈旧」——**不冒无日报**）。≤31 列仍两行 MM/DD、>31 列仍舞台内横滚 + 单行短标签。
3. **组顺序**（DOM 与视觉一致）：正式活跃行 → 「当日其他话题」summary → 其自身行 → 「休眠泳道」summary → 休眠行。两个 summary 都是 `grid-column: 1 / -1` 的纵向全宽条、原生 button + `aria-expanded`，互不串联。
4. **休眠 summary 前移且恒在**：0 休眠仍渲染「休眠泳道 · 0 + 本窗无」并禁用展开、不造行；有休眠时文案沿用既有「展开查看/收起（它们仍挂着追踪，只是这段时间没动）」。
   - ⚠️ 可见文案变化：summary 不再罗列休眠泳道名（冻结稿 `04-mature-1023-expanded-1440.png` 的 `休眠泳道 · 1` 只有计数与箭头）；展开后行名照旧可见。
5. **其他行**：与正式行同高同列（行高 2.9rem，标题两行 `-webkit-line-clamp: 2`，完整标题 + 当时状态词 + 「点下方读全文」在 `title`/`aria-label`）；**只有所选日列**有计数格（`0` 合法可点），其余日期为 `DIV aria-hidden` 空白（不可聚焦、不画跨日泳道）。选行/选格发同一 `selectOther`。组 summary 计数只在 `ready` 出现（0 合法），非 ready 一律无数字、只给状态文案；`view.note`（部分泳道当日未取到）作为附加说明同排展示。
6. **视觉语言单一来源**：正式行/其他行/两个 summary 的样式写在 `ObservationMatrixGrid.vue` 同一批规则里（`.omx-lane, .omx-grid :deep(.omx-other-lane)` …），避免「两套密度/配色」漂移；`DayOtherTopicsGroup` 自身只有 `.otg { display: contents }`（参与父网格）。未新增图标/依赖/新卡片墙/刊头，沿用现 editorial/dark token。

### B.4 测试与命令（Linux 本机串行，参数不带 `--`）

| 检查 | 命令（`cd front`） | 结果 |
| --- | --- | --- |
| G2 目标（本组三文件） | `pnpm test:unit app/features/tags/components/console/ObservationMatrix.test.ts app/features/tags/components/console/ObservationMatrixGrid.test.ts app/features/tags/components/console/DayOtherTopicsGroup.test.ts --maxWorkers=2` | **3 files / 55 tests passed**（matrix 34 / grid 9 / group 12） |
| 直接消费者回归（不改其源） | 上述 + `app/features/tags/components/console/BoardConsoleShell.test.ts --maxWorkers=2` | **4 files / 93 tests passed**（shell 38 全绿） |
| 本组 `.ts` eslint | `pnpm exec eslint ObservationMatrix.test.ts ObservationMatrixGrid.test.ts DayOtherTopicsGroup.test.ts` | **exit 0** |

用例覆盖（对应 `test-cases-matrix-day-others.md`）：0 正式泳道保 14 连续列 + 内联说明 + 组在场（S11-1/RDY-1）；报日列头三态（未接线/null/Set）+ `selectDayHeader` + 非报日不 emit（S11-4、变体 8）；陈旧索引已知报日仍可点；组顺序正式→其他→休眠 + 休眠 summary 在自身行前（GRP-1）；两 toggle 独立 + 受控不自持状态（GRP-3/GRP-2 的受控侧）；折叠行离 DOM 且计数仍在（GRP-4）；0 休眠 0/本窗无 + 禁用（S11-19）；非 ready 不给计数/状态文案/禁用（GRP-5、变体 3/5/8）；只所选日计数 + 0 可点 + 其余 `aria-hidden`（OTH-12、变体 9）；行 title/aria 含状态词与「点下方读全文」+ 两行 clamp 样式锚（GRP-6、变体 10）；长窗 365 列 summary 仍在 grid 内；旧事件 `selectLane/selectDate/toggleDormant` 保留。

### B.5 未验 / 风险（交 G4、G5 与父）

1. **typecheck / build 未跑**（brief 禁止，父 8C.6 集中跑）：`.vue` 只经 vitest 的 SFC 编译（模板/脚本可编译），**不等于** `nuxi typecheck` 通过；新增 props/emit 的 TS 类型（`ReadonlySet<string> | null`、`{sectionId,date,topicId?}`）请父在 8C.6 复核。
2. **真实像素/主题/横滚/键盘**：jsdom 结构测试不验证几何——两行 clamp 实际高度、全宽 summary 在 365 列横滚下的表现、editorial+dark 双主题、`elementFromPoint` 命中与焦点稳定性，均留 G5（`ui-verification/matrix-day-others-<date>/report.md`）。
3. **默认展开态归 G4**：本组件不初始化 `othersOpen`（0 正式行默认展开、成熟默认收起、只初始化一次、切板块复位），G4 必须按首份 A 响应写入；当前未接线时组按 `no-date`（「先选择一个有日报的日期」）诚实降级——G4 接线前若先在壳里看到该组，属预期中间态。
4. **`:deep` 耦合**：其他行/组的样式由网格以 `:deep()` 供给（列对齐/密度必须与正式行同源）。若日后把 `DayOtherTopicsGroup` 移出网格使用，会失去行样式——迁移时必须同时搬样式。
5. **其他行计数格的分档底色**用本地 1 行 `bucketOf()`（阈值与 `observationLanes.matrixCellView` 的桶峰值分档一致，仅视觉）；不参与任何计数/状态口径。
6. 其余话题的「当时状态词」只取 G1 `statusText`，本组不做映射；旧数据 NULL 降级文案由 G1 负责。
7. 未做 G3（`OtherTopicReading`/跨日观察带/窄屏报日条）与 G4（壳层接线），未删改 G3 合同。

## C. tasks 勾选

- 8C.2（G1 数据与模型）：本报告 §A 关闭 R3 后重勾，保留父 R1/R2 与旧 106/116/66 历史。
- 8C.3（G2 网格与组）：源码 + 目标测试 + TS eslint 绿，按 §B 勾选；**不代表** UI 验收通过（G5 未跑）。
