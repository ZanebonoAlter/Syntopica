<!-- complexity: complex -->
<!-- ui-impact: minor -->
<!-- constraint-domains: semantic-board, daily-report -->

## Why

宽屏（≥1440px）下 L1 版块态势页（`/tags/:boardId`）的治理右轨里，「当日日报」块被 `bcr-block--grow`（`flex: 1 1 auto`）撑满栏内剩余高度，但块内内容（`RailNowBrief`）是顶部对齐、不撑开——差值就以可见空白的形式留在右轨底部（用户 2560 宽屏实测：右轨「当日日报」块下缘到「话题追踪」块之间空出一大块）。

这块留白原本是给「当日日报」内容变长预留的，但当版块关注的泳道少、当天正式泳道分区也少时，右轨当天几乎没有内容可看——用户反馈「右侧还空了一块，即使没有关注的泳道也没有数据展示」。

## What Changes

- **右轨「当日日报」块内新增「当日其他话题」子块**：渲染矩阵已有的「当日其他话题」视图模型（`OtherTopicsView`）——每行 = 分区标题 + 该分区当日线索数（**非综合稿口径**，与矩阵该组同源同值）+ 生成日报时的归属状态（当时待观察 / 当时已转正 / 当时已归档 / 当次未归属）。填掉右轨底部留白；没有关注泳道时右轨当天仍有动态可看。
- **形态**：作为「当日日报」块内的**第二个子块**，右轨五个顶层区块顺序不变（版块体检 → 当日日报 → 话题追踪 → 构成候选 → 来源构成），既有顺序断言不破。
- **口径分家**：泳道层的「N 条」= lane-dynamics 该泳道当日合计（与矩阵日格同源同值）；新子块的「N 条线索」= 该分区非综合稿 thread 数。两者分属不同子块、各带标题与口径注，**不得并排成同一列**（避免被读成同一量纲）。
- **显示范围**：仅 ≥1440px。该宽度段才有 `--grow` 留白可填；1200–1439.98px 右轨随页面流动，加内容会真实增加页面高度，守「≥768px 视口布局与交互零变化」回归红线。
- **零新增请求**：数据用壳层既有的 `otherTopics` 派生模型（`buildOtherTopics` 单一实现点），右轨不另发请求、不重算排除口径、不实现第二套「哪些分区算其他话题」。
- **可点定位**：点某行 → 选中该分区（复用既有 `selectOther` 选行链），舞台底部阅读槽读它；右轨只发事件，不自己改选择状态。
- **spec 修订**：`workshop-board-console` 的「治理右轨与治理抽屉」Requirement 整条重写为「治理右轨、治理抽屉与当日其他话题子块」——把「候选分区（「其他动态」）SHALL NOT 进右轨」这条硬禁令改为「MAY 以独立子块进右轨，MUST 与泳道层显式分家」，并把对应 Scenario 从禁令改为新规则（openspec 的 MODIFIED 硬校验不允许在改写中丢 Scenario，故按仓库先例走 REMOVED+ADDED 整条重写，附 Reason/Migration 说明）。
- **不改**：矩阵里的「当日其他话题」折叠组（保留，仍是主交互面）、右轨五块顺序、右轨不提供日期选择器的约束、后端接口与数据模型、日报读取链。

### Out of Scope

- 不删、不折叠、不联动矩阵里的「当日其他话题」折叠组（两处并列是本 change 的明示取舍：右轨是常驻信号面，矩阵是可展开交互面）。
- 不给右轨加日期选择器（日期唯一源仍是矩阵选中日）。
- 不改后端任何接口、字段与数据模型（本 change 全程在前端派生层）。
- 不把「当日其他话题」提升为右轨第六个顶层区块（会改区块顺序契约与既有顺序单测）。

## Capabilities

**New Capabilities**：无。

**Modified Capabilities**：

- `workshop-board-console`：整条重写「治理右轨与治理抽屉」→「治理右轨、治理抽屉与当日其他话题子块」——把「候选分区（「其他动态」）SHALL NOT 进右轨」改为允许以独立子块进右轨但必须与泳道层显式分家，并补对应 Scenario；同时在「当日日报」两层表达里补第三层的子块归属。

## Impact

- **前端**：`front/app/features/tags/components/console/`
  - 新增 `RailOtherTopicsBrief.vue`（右轨版「当日其他话题」受控展示）
  - 改 `BoardContextRail.vue`（「当日日报」块内挂新子块 + ≥1440px 显示门 + 定位事件）
  - 改 `railContext.ts` / `useConsoleRail.ts`（`RailContext` / `RailBinding` / `ConsoleRailDeps` 加 `otherTopics` 字段透传）
  - 改 `BoardConsoleShell.vue`（把 `wiring.otherTopics` 接到右轨装配）
  - 改 `useConsoleShellActions.ts` / `BoardConsoleBody.vue`（若选行定位需透传事件）
- **文档**：`docs/reference/flow/semantic-board.md`（右轨顺序描述 + 「candidate 分区 MUST NOT 进右轨」约束句）、`docs/reference/standard/frontend/layout.md`（已登记例外表的增补说明）
- **后端**：零改动（数据全在前端既有派生层）
