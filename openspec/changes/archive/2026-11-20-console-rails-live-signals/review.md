# Review 账 · console-rails-live-signals

审查范围：制品（proposal / design / spec delta / ui-design / tasks / ui-verification）+ rail 相关实现 diff。
审查者：`change-review` 只读子线程（2026-11-19），重点查共同轴对齐数学、口径单一性、排序稳定性、零新增请求、降级诚实性、spec↔实现一致性、测试有效性。
处置原则：Medium 全修；Low 修到不影响正确性的程度，纯重构类驳回并写明理由。

## 总评（审查结论）

核心口径全部正确：
- **共同轴重对齐**：offset 方向、`位置 = i - offset`、越界置 null 均已验算通过（G=11-19 / A=11-15 → offset 4，`clue_days[13]` 落共同轴 index 9 ✓）；负 offset 亦正确。
- **口径单一**：右栏「N 条」唯一来源 `lane-dynamics` 当日 `clue_count` → `dayCellMetrics(day).clueCount`，与矩阵日格**同一函数**；全路径无日报 sections thread 数参与数值。
- **不重复标题**：后端 `title = highlights[0].Title` 已确证，模型与组件均不含 title/summary。
- **排序稳定**：`compareLanes` 带显式 `order` 次键构成全序，不依赖引擎稳定性。
- **零新增请求**：左轨复用壳层已有 `getOverview()`，右轨复用已有 `ensureReportDay` 与既有窗口。

---

### R-1 2026-11-19【Medium】右轨泳道色点自造哈希调色板，与全站话题色不一致

- **状态**：已修复
- 问题：全站其它话题色点取后端 `PersistentTopicColor(topic_id)`（经 `GET /daily-reports/:id` 的 `sections[].persistent_topic.color` 下发），本 change 的 `normalizeReportSection` 丢掉了该字段，改用 `nowBrief.ts` 里 5 色哈希表 → 同一话题在右轨与日报阅读台/侧栏/侦探墙显示不同颜色，且 5 色必然撞色。
- 修复：`normalizeReportSection` 增补 `persistentTopicColor`（后端已在下发，零后端改动）；`nowBrief.buildLanes` 改为 `section.persistentTopicColor ?? laneTopicColor(topicId)`；`laneTopicColor` 降级为 fallback 并改名色表为 `LANE_FALLBACK_COLORS` + 注释说明「有后端色 MUST NOT 走这里」。
- 测试：`nowBrief.test.ts` 新增 1 例（后端色优先 / 缺字段回退哈希）。

### R-2 2026-11-19【Medium】flow 参考文档仍写着已被验收否掉的强口径

- **状态**：已修复
- 问题：`docs/reference/flow/semantic-board.md` §版块态势页左右轨 仍写「保证右栏每个「N 条」在矩阵里有对应格」，而 `ui-verification/report.md` 偏差记录已认定该表述过强（topic 1205 为 watch 关联 + 14 天 2 分区 → 矩阵判休眠折叠）。design/ui-design 已改，flow 未同步。
- 修复：flow 该句改为「名单含 watch 关联泳道，以及矩阵判为休眠而折叠在「休眠泳道 N」里的行——其格需展开后才可见；数值与矩阵日格同源同值，不因折叠而从右栏隐藏」。

### R-3 2026-11-19【Medium】矩阵选中列淡底（design D7）已上线但 spec delta 无对应条目

- **状态**：已修复
- 问题：delta 只有 ADDED「版块栏逐日线索条码」与 MODIFIED「治理右轨与治理抽屉」，而 proposal/design 都把「中间舞台选中列整列淡底」列为本 change 改动 → 归档后主 spec 不记录这条已上线行为。
- 修复：delta 增 MODIFIED「矩阵视图（泳道 × 日期）」，补齐原有 4 个 Scenario 并新增 2 条：「选中日整列淡底」（只铺有分区的格、不改色阶与列头）、「换日底色跟随」。`openspec validate --strict` 通过。

### R-4 2026-11-19【Medium】「跟随矩阵选中日」的日期闸门无测试覆盖

- **状态**：已修复
- 问题：闸门只在 `useConsoleRail.ts` 的 computed 里（`reportDay.date === selectedDate`），而 `useConsoleRail` 全仓无测试；`nowBrief.test.ts` 测的纯函数不含闸门。spec 的「跟随选中日」「选中日无日报」两条 Scenario 因此无回归保护——闸门被改成直接用 `reportDay` 时，右栏会把上一日日报挂在今日标题下（spec 明令 MUST NOT）。
- 修复：闸门下沉到 `buildNowBriefModel`（`selectedDate !== null && report.date !== selectedDate` → 按未到处理），`useConsoleRail` 保留原闸门作双保险。
- 测试：`nowBrief.test.ts` 新增 1 例（详情 11-18 / 选中 11-19 → `status='loading'`、无点评无泳道）。

### R-5 2026-11-19【Low】左轨行 title 在「无锚点」时误写「本次未取到」

- **状态**：已修复
- 问题：后端在「版块无泳道锚定」时给全 0 轴 + 空 `anchor_date`（注释：仍是完整零值行）；前端把 `anchorDate === null` 与「本次未取到」压成同一文案，而行内渲染的是真实 0 的空槽，文案与形态矛盾。
- 修复：`railRowTitle` 分层——无锚点写「该版块无日报锚点」，「本次未取到」只留给真正没取到的情况。
- 测试：`railClueAxis.test.ts` 在同一条用例中断言 title 含「该版块无日报锚点」且不含「本次未取到」。

### R-6 2026-11-19【Low】缺 `anchor_date` 时兜底 `offset = 0`，等于断言值属于共同轴

- **状态**：已修复
- 问题：无锚点就无法确定 `clue_days[i]` 是哪天，兜底 `offset = 0` 是静默假设「这些值属于共同轴」。当前后端恰只在全 0 轴时出现，但那是不变量依赖，后端一旦给「有值但无锚点」就会**静默错日期且不降级**。
- 修复：`usable` 增加 `anchorDate !== null` 条件，无锚点 ⇒ 整行未取到（宁可空，不可错位）。
- 测试：`railClueAxis.test.ts` 新增 1 例（14 个真值 + 无锚点 ⇒ 全 null）。

### R-7 2026-11-19【Low】`numArrayOrNull` 会 filter 掉非数值项，改变下标↔日期对应

- **状态**：已修复
- 问题：`normalizers/boardRadar.ts` 的 `numArrayOrNull` 用 `filter` 会缩短数组；本 change 是首个依赖 `clue_days` **下标↔日期**对应的消费方，一旦轴里出现 null 槽，后续下标整体前移 → 静默错日期且仍显示为有值。
- 修复：**不改共享 normalizer**（它还有其它消费方，改语义影响面大于收益），改为在 `railClueAxis` 侧加长度断言：`clueDays.length !== RAIL_AXIS_DAYS` ⇒ 整行未取到。这样无论上游怎么归一，错位都不会被渲染成有值。
- 测试：`railClueAxis.test.ts` 新增 1 例（长度 3 ⇒ 全 null），原「不拿别的版块数据顶上」用例的 fixture 改为 14 长度以保持原意。

### R-8 2026-11-19【Low】`mergeLaneDynamicsLanes` 放在 `nowBrief.ts`，职责与文件名不匹配

- **状态**：已驳回
- 理由：该函数是 A 档 + 365 池的通用合并，被壳层用于喂矩阵与右轨整套泳道清单，确实不属于「当日日报读模型」。但把它搬到 `observationLanes.ts` 或新文件会改动矩阵读链的 import 关系（跨模块重构），与本 change「只改左右轨呈现」的边界不符，且不影响任何正确性。**延后到独立重构**：本 change 内以「归属备注」在函数头注释标注其真实用途，避免后续误用。

### R-9 2026-11-19【Low】spec 说「每个 active 分区一条」，实现按 topicId 去重只留首个

- **状态**：已修复
- 问题：同一泳道当日可能被拆成多个主题桶分区，实现只出第一行（用该分区首条 thread 标题作代表句）；实现语义比 spec 字面更自洽，但 spec 与代码不一致。
- 修复：spec 泳道层措辞改为「**每个正式泳道一行**（同一泳道当日若被拆成多个主题桶分区，取首个 `topic_status_at_report = active` 的分区作代表，不重复列出该泳道）」，并顺带写明色点取后端稳定色。

### R-10 2026-11-19【Low】`ui-design.md` Layout Contract 的条码宽度已过期

- **状态**：已修复
- 问题：文档写「14 × 6px ≈ 97px / 名称 ≈ 139px」，验收后微调已改为 5px、名称实测 144px。
- 修复：同步为 5px / 83px / 144px，并注明首版 6px 致名称截断、验收后收窄。

---

## 处置后复测

| 项目 | 结果 |
| --- | --- |
| `pnpm test:unit`（railClueAxis / nowBrief / BoardRail / BoardContextRail / ObservationMatrixLaneRow） | **5 files / 73 tests 全通过**（新增 4 例：R-1 色点优先、R-4 日期闸门、R-6 无锚点、R-7 长度不符） |
| `openspec validate console-rails-live-signals --strict` | valid |
| 未处置 | R-8（已驳回，延后独立重构），其余 9 条全部修复 |

## 审查未覆盖范围（如实记录）

- `pnpm build` / 全量 `pnpm test:unit` 未由审查者执行（按 brief 禁止长命令），由主会话在验收环节跑过。
- 工作树中其它会话的文件（`board_crud_handler.go`、`DailyReportDesk*`、`AppSidebar*`、`theming.md` 等）未审——不属本 change。
