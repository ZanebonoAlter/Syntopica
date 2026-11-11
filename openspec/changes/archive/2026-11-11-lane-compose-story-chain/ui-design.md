<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# UI 复用契约

## 入口与变更

- **既有编排态入口不变**：版块态势页「新建泳道」→ `BoardComposeOverlay.vue`（workspace 模式全屏画布）。不新建页面、不加路由、不加弹层、不加多步向导。
- **左侧候选栏从「单一话题链列表」扩为「两条并列维度」**：`LaneComposeCandidates.vue` 在既有候选列表上方增加一条维度切换（segmented control，两项：「话题链」/「事件线链」），列表主体按当前维度渲染。两条链**共用同一个候选列表容器与既有组复选框/成员复选框三态逻辑**，不新增第二套选择态模型。
- **两条链的区分不得只靠颜色**（用户可见的两类线索必须可分辨）：
  - 话题链：组标题 = 话题名 + 既有元信息「N 个事件 · 连接跨度 N 天」；组图标 = 既有话题图标。
  - 事件线链：组标题 = 事件线标题（空标题降级为「事件线 #ID」）+ 元信息「N 个分区 · 跨天 N 天 · 同一事件」；组图标 = **事件线专属图标**（区别于话题图标）+ **文字标签「事件线」**。
  - 画布上的连线同样区分：话题链沿用既有 identity/similarity 线型；事件线链使用**独立线型**（点划线）与**独立节点标记**（节点角标），与既有两条边型在图例中各自列名。颜色只作辅助，不承载唯一信息。
- **降级文案位**：事件线维度不可用时（板未回填事件身份），维度切换项**仍可见但禁用**，旁边给出说明「事件身份未回填，本板暂无事件线链」；部分回填时列表顶部给一行不完整提示「部分事件未回填，事件线链不完整」。MUST NOT 以空列表冒充「该板没有事件线」。
- **相似推荐区不变**：`useInlineCompose` 的「可补充的事件」仍按已选聚合向量质心 cosine 召回，分组、阈值与文案**一字不改**；事件线链不出现在该区，事件线身份不参与该区排序。

## 受影响状态

| 状态 | 候选栏（两条链） | 画布 | 相似推荐 |
|---|---|---|---|
| 成功 | 两条维度各自成组，组标题/图标/文字标签可区分；事件线组显示事件线标题、成员分区数与跨天数 | 事件线链以独立线型连出，节点带事件线角标；与话题链节点可同屏共存 | 保持既有 cosine 召回与分组 |
| 部分回填 | 事件线维度可用但顶部给「部分事件未回填，事件线链不完整」提示 | 已连出的事件线边照常显示，不补造未回填的边 | 不变 |
| 未回填（降级） | 事件线维度项禁用 + 「事件身份未回填，本板暂无事件线链」；话题维度不受影响 | 无事件线边；不画空事件线节点 | 不变 |
| 板内无事件线（已回填但无跨天事件） | 事件线维度可用但空态写「本板暂无可串联的事件线」，**不写**「事件身份未回填」 | 无事件线边 | 不变 |
| 空窗口 / 无候选 | 两条维度各自的既有空态；维度切换在无候选时仍可切换 | 既有空态 | 既有空态 |
| 加载 / 错误 | 沿用既有骨架与错误条；维度切换在加载中禁用 | 既有骨架 | 既有骨架 |
| 旧后端响应（无新字段） | 事件线维度降级为「不可用」并说明服务端未升级，话题维度照常；不得报错 | 既有边照常 | 不变 |

## 复用组件与布局模式

- 编排态沿用既有 `AppPageShell mode="workspace"` 与三栏结构（左候选 / 中画布 / 右栏），**不调整栏宽、不改 layout mode**。
- 维度切换用既有基础控件承载（分段选择形态），复用既有主题 token 与焦点样式，**禁止硬编码新色值**。
- 候选列表复用 `LaneComposeCandidates.vue` 既有组卡片、组复选框、成员复选框三态（unchecked/indeterminate/checked）、事件数/跨度元信息行；事件线组只替换**标题来源与元信息文案**，不复制一套列表实现。
- 画布复用 `LaneComposeCanvas.vue` 既有节点/边渲染与既有 identity/similarity 线型；事件线边**新增第三种线型**并在既有图例中增列一行。
- 事件线链成员沿用既有可勾选域（candidate 话题分区 / 未归属分区 / 已归档来源分区）；**active 泳道分区不入链**，仍以画布只读参照呈现，不触发任何移出确认。
- 可访问性：维度切换可 Tab 到达、Enter/Space 激活；组复选框沿用既有 aria-label；事件线组标题的 aria 文本须包含「事件线」二字，不靠图标单独承载语义。
- 目标视口：沿用既有编排态的 1440×900、1920×1080 桌面双视口；窄屏沿用既有「仅提示宽屏」行为，本 change 不新增窄屏编排能力。

## 验收映射

| 验收点 | 落点 |
|---|---|
| 两条维度并列、组标题/图标/文字标签可区分（非仅颜色） | `front/app/features/tags/components/console/BoardComposeOverlay.test.ts`（编排态候选栏渲染用例） |
| 事件线组整组勾选一次补齐成员、三态与计数同步 | `front/app/features/tags/components/console/BoardComposeOverlay.test.ts`；`front/app/features/tags/composables/useLaneCompose.test.ts`（两链勾选态与去重合并） |
| 未回填降级文案与「不冒充无候选」 | `front/app/features/tags/composables/useLaneCompose.test.ts`；`front/app/features/tags/components/console/BoardComposeOverlay.test.ts` |
| 新响应字段归一化与旧后端降级 | `front/app/api/persistentTopics.test.ts` |
| 相似推荐口径不变（cosine 质心召回、阈值与排序未被事件线污染） | `front/app/features/tags/composables/laneComposeLogic.test.ts`；`front/app/features/tags/composables/useInlineCompose.test.ts` |
| 事件线链成链口径（≥2 分区、跨 ≥3 天、正交分组、存活 id 归一） | `backend-go/internal/topicgraph/repository/compose_chain_test.go`；`backend-go/internal/topicgraph/repository/daily_report_manual_topic_test.go` |
| 画布事件线边独立线型与图例增列 | `front/app/features/tags/components/compose/LaneComposeCanvas.test.ts` |
| 完整 UI 故事（两条链并列 → 按事件一次勾齐跨天材料 → 保存） | 人工：实施后用 agent-browser 访问静态 :5100 走完「新建泳道 → 切到事件线维度 → 整组勾选 → 保存」，证据建账本 change `ui-verification/report.md`；本阶段未执行，不预写通过 |
