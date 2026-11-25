<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

<!--
  minor 复用契约（无原型）。本次改动全部落在既有全屏工作面 BoardComposeOverlay.vue 的两种模式内，
  不新增页面、不新增弹窗、不改布局模式、不改导航：
    - 修正模式（编辑泳道）：三栏 380 / minmax(0,1fr) / 300，改造点在中央 LaneCorrectionCanvas
      与左栏 LaneAnchorScope 的只读展示；
    - 编排模式（新建泳道）：三栏 300 / minmax(0,1fr) / 320，只改画布高度计算与一个缺失的落点态样式。
  摸底证据见 explore-findings.md（4 条 pin：高度链根因、拖拽机制、修正画布现状、anchor 数据模型）。
-->

## Interaction Decisions

| ID | 决策 | 模式 | forbidden | 状态 |
|---|---|---|---|---|
| D1 | 修正画布改节点归属（加入 / 移出 / 加回） | `canvas`（拖拽） | `dialog`, `drawer` | `open` |
| D2 | 候选相关度的分档呈现 | `canvas`（视觉编码） | — | `open` |
| D3 | 首义来源与滚动窗口的查看 | `panel`（左栏只读块） | `dialog`, `drawer` | `open` |
| D4 | 首义的重设 | 沿用既有 `panel`（左栏勾选 + 按钮） | `dialog` | `pinned`（不改） |
| D5 | 拖拽后的失败回执 | `inline-edit`（节点内联错误态） | `dialog` | `open` |
| D6 | 取消聚焦（修正画布） | `canvas`（轻点空白处） | `dialog`, `drawer` | `open` |

- **D1 依据 P1**（操作依赖多个对象的空间关系）：归属变更的本质是「这个节点属于这条泳道吗」，答案只能在成员区与候选区的空间关系上下文中产生。既有实现把它拆成「点节点 → 读浮层卡 → 点按钮」三步，P1 下属于模式降级。定 `canvas` 拖拽，与新建泳道（`workshop-lane-compose` 已交付的拖拽采纳）**同一手势纪律**。
  - **不新增第二个写入口**：拖拽与节点卡按钮、左栏勾选共用同一条写路径（`useLaneCorrection` 的 `add/remove/restore` + `pendingIds`），三者反映同一份成员集合。
- **D2 依据 P5**（需并列比较）：候选之间的接近度差必须能并排看出大小关系才可判断「哪几条真的近」。绝对值换算在阈值附近几乎是常量，改为批内相对归一后仍属同一画布的视觉编码，不引入新控件。
- **D3 依据 P4**（保留泳道上下文前提下查看多条）：首义来源与滚动窗口是「解释匹配为什么发生」的旁证，必须在能同时看见成员与画布的前提下读——放左栏 `panel` 只读块，沿用 `LaneAnchorScope` 既有块头形态（标题 + 计数徽标 + 生效徽标）。
- **D5 依据 P2**（改单个对象且需看见上下文）：一次拖拽失败只影响一个节点，就地标错并回滚，不弹窗打断连续拖拽。
- **D6 依据 P3**（进入与退出对称）：点节点进入聚焦是一步，退出也该是一步且在同一画布上完成——此前只能点顶部「取消聚焦」按钮或再点同一节点。空白轻点 = 「什么都不选」的自然语义，与编排画布节点卡「×」同义，但不用把指针挪到卡上。**平移松手不算**（那是滚动不是选择）；**落在节点上的轻点不算**（聚焦 / 再点切换由节点自己的 click 负责，否则会把「再点同一节点取消」误翻成重新聚焦）；**取消聚焦零写请求**。
- **节点卡标题换行（实机反馈）**：卡片 264px 宽、标题单行省略，长标题认不出是哪条。改完整展示、按需换行（`white-space: normal` + `overflow-wrap: anywhere`），卡高沿用既有「聚焦切换后实测 `offsetHeight`」口径，位置钳制不变。
- **话题带横条移除（实机反馈）**：成员行上方的强调色横条 + 刻度线不承载判断（成员归属已由圆点 + 勾 + 分区标签表达），整组删除；「本泳道话题带 / 泳道名」标签与中性基线保留，与候选区 / 只读参照区「一线 + 左标签」同形。

## Data Provenance

| 数据项 | source | 说明 |
|---|---|---|
| 成员集合 / 候选集合 / 只读参照 | `api GET /semantic-boards/:id/persistent-topics/compose-candidates`（`all_members_topic_id`） | 既有端点，本 change 不改请求与归一化 |
| 成员接近度（快照） | `api` 的 `topic_match_distance` → `1 - d` | 既有口径，**不参与相对归一** |
| 候选接近度（估算） | `派生` 质心距离，前端 `centroidRecommendations` | 既有口径，本 change 只改**显示映射**（相对归一），不改算法；沿用「估算」标注 |
| 相关度分档区间 | `派生` 批内 min-max 归一 | 本 change 新增显示层派生，无后端字段 |
| 首义来源节点 | `api GET /api/daily-reports/topics/:id/anchor` 的 `anchor_origin` | 本 change 扩展字段；`resolved=false` 时显示「创建时写入 · 来源不可考」，**不得编造节点** |
| 参与滚动计算的节点（N/30） | 同上 `centroid.section_ids` / `centroid.members[]` | 本 change 新增字段 |
| 滚动窗口大小与 α | 同上 `centroid.window_size` / `centroid.anchor_weight` | 新增字段，只读展示 |
| 退化态（<2 可解析向量 / 首义不可用） | 同上 `centroid.degraded` | 新增字段；为真时该组文案改为「质心当前为退化口径（窗口向量不足或首义不可用）」——**两种退化分支的落点不同**（向量不足→退回首义向量；首义不可用→退化为纯滚动均值），文案 SHALL NOT 写死成其中一种 |
| 生效时机徽标（立即生效 / 次日日报生效） | `none` | 既有静态结论文案，本 change 不改口径 |
| 拖拽落点合法性 | `派生` 目标分区与节点当前归属 | 前端派生，无后端字段 |

## 入口与入口变更

**不新增入口。** 三种能力都随既有入口进入：

| 入口 | 现有接线 | 本 change 变化 |
|---|---|---|
| `E2` L1 矩阵行头「⋯」→「编辑泳道」 | `BoardConsoleShell.vue` → `BoardComposeOverlay mode="correction"` | 无变化 |
| `E3` 泳道管理 →「编辑泳道」 | `TopicManageDialog.vue` → 同上 | 无变化 |
| `E1` 顶栏「新建泳道」 | `BoardConsoleHeader.vue` → `BoardComposeOverlay mode="compose"` | 无变化 |

**唯一新增的交互面**（都在既有容器内，不新增容器）：

1. 中央画布节点变为可拖拽（`LaneCorrectionCanvas.vue`）——触发元素是既有圆点本体；不改节点卡按钮的既有位置与语义。
2. 左栏「首义范围」块内新增两块只读内容（`LaneAnchorScope.vue`）：首义来源一行（或「来源不可考」）、可折叠「参与滚动计算的节点（N/30）」。
3. 左栏「首义范围」块之后的「辅助标签」面板属于活跃 change `topic-lane-auxiliary-label-constraint`，本 change **不触碰**该块，只在其之前完成自己的插入。

**禁止**：任何把首义来源/滚动窗口做成弹窗、抽屉、或悬停 tooltip 的实现；任何把拖拽结果做成「待应用」草稿再确认的实现（与「归属变更立即生效」口径冲突）。

## 受影响状态

拖拽与首义展示引入的状态，逐个交代（未列出的状态沿用现状）：

| 状态 | 触发 | 表现 | 恢复路径 |
|---|---|---|---|
| **loading（首义来源）** | anchor 读接口请求中 | 首义来源一行显示骨架/「读取中」，滚动窗口折叠组不可展开 | 请求结束自动消失 |
| **loading（拖拽落库）** | 松手后单据写入中 | 节点进入 `pendingIds`：半透明 + 不可再次拖（防重入），成员区/候选区计数暂不跳变 | 响应到达后更新 |
| **empty（首义来源不可考）** | `anchor_origin.resolved=false` | 显示「创建时写入 · 来源不可考」，**不显示任何节点**，附一句「用下方的节点定一次首义即可留痕」 | 用户重设首义后转 success |
| **empty（滚动窗口不足）** | `centroid.section_count < 2` 或 `degraded=true` | 折叠组显示「参与滚动的节点不足，质心当前为退化口径（窗口向量不足或首义不可用）」，不画空列表骨架 | 次日日报产生新归属后自动恢复 |
| **error（首义读接口失败）** | 请求失败 | 首义来源一行显示「读取失败 · 重试」，不影响画布与成员操作（**只读块失败不得阻断写入口**） | 点重试 |
| **error（拖拽落库失败）** | 写接口 4xx/5xx | 节点立刻回到原位（乐观回滚），节点内联标错（红描边 + `title`/`aria-live` 文案），**不弹窗**；连续拖拽不中断 | 再次拖拽重试 |
| **success（拖拽落库）** | 写入成功 | 节点归入新分区、计数更新、徽标按既有「归属 · 立即生效」口径 | — |
| **取消聚焦（空白轻点）** | 聚焦在场时轻点画布空白处（未平移、未落在节点 / 节点卡 / 缩放条 / 折叠带勾选框） | 节点卡收起、相关高亮与弱化恢复默认、成员数不变、零写请求 | 再点任一节点重新聚焦 |
| **disabled（不可拖节点）** | 只读参照区节点、「更早」折叠带节点 | 光标 `default` 而非 `grab`；按下不启动拖拽（与平移手势区分）；悬停仍可点出节点卡 | — |
| **empty（无候选）** | 候选区为空 | 沿用现状文案，不新增空态 | — |

## 复用组件与布局模式

**布局模式**：沿用 `workspace`（全屏 `AppPageShell mode="workspace"`，`position:fixed; inset:0`），栏宽沿用既有常量——修正 380 / `minmax(0,1fr)` / 300；编排 300 / `minmax(0,1fr)` / 320；窄屏（<1280）回退 240/1fr/280 与 380→既有窄屏拦截。**本 change 不引入任何自由宽度、不新增 dialog 尺寸档**。

**复用（零新增组件）**

| 复用对象 | 位置 | 用途 |
|---|---|---|
| `LaneCorrectionCanvas.vue` | 既有 | 拖拽、落点视觉、相对归一、图例、首义标记的宿主 |
| `LaneAnchorScope.vue` | 既有 | 首义来源与滚动窗口的展示宿主（新增两块只读内容） |
| `laneComposeLogic.ts` 纯函数 | 既有 | `relativeProximities` / `recommendRange` / `strengthRange` / `pointAppearance` / `isDragBeyond` / `contentPointOf` / `applyDrop` / `resetGesture` |
| `.lcc-` 手势视觉层 CSS | `LaneComposeCanvas.vue` | `.lcc-drag-ghost*` / `.lcc-canvas__dropzone` / `.is-drop-target` 共享（两画布前缀同族） |
| 节点卡浮层 `.lcc-card` | 既有 | 标题由单行省略改为完整换行展示；拖拽期间保持既有跟随/隐藏规则不变 |
| `clear-focus` 事件名 | `LaneComposeCanvas.vue` 既有同名 emit | 修正画布新增同名 emit，宿主统一接 `clearFocus()` |
| 顶部「取消聚焦」按钮 | 既有 | 保留（键盘 / 精确操作的兜底入口），不因空白轻点而移除 |
| 生效时机徽标 | 既有 | 「归属 · 立即生效」「次日重算」口径不改 |

**新增（最小面）**

| 新增 | 位置 | 理由 |
|---|---|---|
| 修正画布的落点分区高亮（成员区 highlight / 不可落点区 dimmed） | `LaneCorrectionCanvas.vue` | 复用新建画布的 dropzone 视觉语言，但目标是**分区**不是行 |
| 尺寸图例（小圆—大圆） | `LaneCorrectionCanvas.vue` 底部图例 | 相对归一后单一固定圆无法表达档位 |
| 已生效首义标记（区别于待提交勾选） | `LaneCorrectionCanvas.vue` | 修既有图例语义错位 |

**移除（实机反馈）**：话题带强调色横条 `.lcc-band-bar` 与刻度线 `.lcc-band-tick`（`LaneCorrectionCanvas.vue` 模板 + 样式一并删）。

## 验收映射

**组件测试（指名到既有/新增文件）**

| 验的是 | 文件 |
|---|---|
| 拖拽改写归属路径（add / remove / restore 三态，含 pendingIds 防重入与失败回滚） | `front/app/features/tags/components/console/LaneCorrectionCanvas.test.ts` |
| 相关度相对归一只作用于候选桶、成员尺寸不变；尺寸图例与视觉一致 | `front/app/features/tags/components/console/LaneCorrectionCanvas.test.ts` |
| 已生效首义标记 vs 待提交勾选标记不混淆 | `front/app/features/tags/components/console/LaneCorrectionCanvas.test.ts` |
| 不可拖节点（只读参照 / 折叠带）不启动拖拽 | `front/app/features/tags/components/console/LaneCorrectionCanvas.test.ts` |
| 点空白取消聚焦：聚焦在场才抛 `clear-focus`；平移松手 / 落在节点上的轻点 / 落在节点卡上不抛 | `front/app/features/tags/components/console/LaneCorrectionCanvas.test.ts` + `front/app/features/tags/components/console/BoardComposeOverlay.correction.test.ts`（宿主接线：卡收起、零写请求） |
| 节点卡标题完整展示（样式允许换行、无省略号）；话题带无强调色横条与刻度线 | `front/app/features/tags/components/console/LaneCorrectionCanvas.test.ts` |
| 首义来源三态（resolved / 不可考 / 读取失败）与滚动窗口计数口径隔离 | `front/app/features/tags/components/console/LaneAnchorScope.test.ts` |
| `anchor_origin` / `centroid` 归一与派生（含 degraded） | `front/app/features/tags/composables/useLaneCorrection.test.ts` |
| 编排画布高度贴合容器 + 余量给散料区 + 不溢出时吸顶语义 | `front/app/features/tags/components/console/BoardComposeOverlay.test.ts` |
| 编排画布「不可落点」态有实际样式 | `front/app/features/tags/components/console/BoardComposeOverlay.test.ts` |

**端到端验证（agent-browser，本 change 建账）**

- 在 `ui-verification/report.md` 建账，双视口（1440×900 与 1920×1080）各走一遍：
  1. E2 进修正工作面 → 把 1 个候选拖进成员区 → 松手后计数 +1、节点落点正确、刷新后仍在；
  2. 把 1 个成员拖出成员区 → 计数 -1，左栏「已移出（本次）」出现该节点，「加回来」可恢复；
  3. 观察候选圆点大小差异在默认缩放下可辨；图例与视觉一致；
  4. 首义范围块：人工重设过的泳道显示来源节点；未重设的显示「来源不可考」；滚动窗口计数与「窗口内 N · 全部 M」不同名；
  5. 新建泳道：单候选链时画布填满高度，下方无空白带；拖动节点到非成员区时出现「不可落点」态；
  6. 编辑泳道（实机反馈补丁）：聚焦长标题节点 → 卡内标题全文换行可读；成员行上方无强调色横条；轻点旁边空白画布 → 节点卡收起（成员数不变），再点节点可重新聚焦。
- 失败项回填 `human-feedback.md` 与 review.md。

## 明确不做

- 不做「拖拽改日期」与「拖拽排序」：修正画布的 X 轴是日期事实（节点 `periodDate`），拖动不得改写它。
- 不做编辑画布的草稿/应用/撤销栈：归属变更即时落库，与既有口径一致。
- 不做首义来源/滚动窗口的弹窗、抽屉、悬停 tooltip 形态。
- 不把首义或滚动窗口画到单个节点上：它们是**泳道级**事实，画到节点会误导（成员节点只标「是否首义成员」）。
- 不为算法自动话题虚构首义节点列表（`resolved=false` 是最终形态，不是占位）。
- 不改布局模式、栏宽、导航、入口、dialog 尺寸档。
- 不触碰左栏「辅助标签」面板（`topic-lane-auxiliary-label-constraint` 的领域）。
