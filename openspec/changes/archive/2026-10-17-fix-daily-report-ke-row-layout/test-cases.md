# test-cases — fix-daily-report-ke-row-layout

> 单元：`topic-key-event-markers`「条目标记交互」Requirement（MODIFIED）——线索行呈现分区与动作组位置。本 change 为纯前端样式摆放修复，故事层=日报页内一条线索行的标记交互全流程。

## ⓪ 契约变更：继承与调整

`bash scripts/harness/test-assets.sh topic-key-event-markers` 反查旧资产，受本 change 影响的旧行逐行处置：

| 旧 Scenario | 处置 | 旧测试 | 动作 |
| --- | --- | --- | --- |
| 条目标记交互·正文呈现句 | MODIFIED（左右等宽+动作右下+归属子组件） | KeyEventMarkerActions.test.ts「splits detail…」 | 断言继承（结构分离仍成立），用例注释改写新分区；新增源码守卫用例 |
| 建议只在日报展示 | 不变（回归） | KeyEventMarkerActions.test.ts overview 用例 | 跑绿即可 |
| 标记不误展开文章 | 不变（回归） | DailyReportTopicSection.test.ts | 跑绿即可 |
| 未知状态与重复提交反馈 | 不变（回归） | KeyEventMarkerActions.test.ts busy/error 用例 | 跑绿即可 |
| 其余 5 Requirement / 17 Scenario | 不变 | 后端 PG/service/handler 测试 | 不触碰（本 change 零后端改动），不重跑 |

## 主链路（故事节拍）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 打开日报，线索行带未确认 AI 建议（理由+依据非空） | 建议行左右等宽且动作右下 | 理由块与标题列各占 50%，带分隔线，长文本折行不截断 | 组件 + e2e | KeyEventMarkerActions.test.ts + ui-verification/report.md A-1 |
| 2 | 观察动作组位置 | 同上 | AI建议徽标+确认为关键+不采纳横排、行底右对齐，不压理由文本 | 组件 + e2e | 同上 + A-2 |
| 3 | 点击「确认为关键」 | 标记不误展开文章（继承） | 单步标记生效，文章不展开，行内状态变已标记 | e2e | A-3（复用既有组件回归兜底） |
| 4 | 摆放规则来源检查 | 摆放规则不因scope断链失效 | 摆放声明在子组件 scoped 样式；宿主无 `> .ke-` 死规则 | 源码守卫 | KeyEventMarkerActions.test.ts 守卫用例 |

## 变体走查（五组清单，适用性留痕）

- 输入组：超长理由（英文长串/无空格长串）→ `overflow-wrap: anywhere` 折行不撑破右列【组件断言 pre-line+anywhere 保留，e2e A-4 超长理由目视】；其余（空串/分隔符/大小写）→ 文本渲染域，与本 change 无关，划除。
- 前置组：无标记/confirmed/dismissed/error 各前置 → 动作组一律底行右对齐、位置跨状态一致【e2e A-5】；空集（hasDetail=false）→ 不渲染 detail，动作组仍 r2 底行【组件断言 detail 不存在+actions 存在】；越界引用/部分满足 → 后端域，划除。
- 时间窗口组：全部划除（无时间语义）。
- 幂等组：重复点击 → busy 互斥禁用（既有用例回归）；并发 → 未声称线程安全，划除。
- 可用性组（UI 必检前三）：误输入反馈 → N/A（无输入框）；空态 → hasDetail=false 行高不虚增【组件断言】；错误态 → error 文案随 detail 右列显示、保留重试【既有用例回归】；加载态 → busy 标签位置随动作组（横排右下）【e2e A-5 附检】；超长文本 → 输入组已覆盖；重复提交 → 幂等组已覆盖。

## 效果核对

不适用：本 change 不含 AI/数据覆盖率依赖的效果（纯 CSS 摆放），无量化核对项。

## 白盒附加

不适用（simple 档、纯样式；无算法/状态机/解析/协议逻辑）。
