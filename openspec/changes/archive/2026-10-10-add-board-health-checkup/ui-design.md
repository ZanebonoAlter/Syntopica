<!-- ui-impact: major -->
<!-- ui-approval: approved -->
<!-- ui-prototype: ui-prototype/index.html -->

<!--
  marker 契约（ui-design-gate 机器读取）：
  - ui-impact: major（信息架构变化：BoardCompositionPanel 退役，体检面板吸收构成管理）
  - approval: pending → 待用户在对话中确认原型后改 approved
  - prototype: 形态 A 合并面板四态静态原型
-->

## User Journey

- **入口**：`/tags` 板块页「板块内容」tab（不变，无新路由）。
- **主任务 1（看健康度）**：打开板块 → 面板顶部即见评分徽章/评语/指标 chips；低分版块经通知（LinkType=board）跳转而来，落地先见 tab 顶部警告条 → 点击滚动定位到面板。
- **主任务 2（治理构成）**：在同一面板内完成——看质量表发现弱标签 → 行内移除；header「＋ 添加构成」展开 picker 搜索多选；组合标签区创建/移除挂载；未收编候选行证据合适时一键「收编」。
- **次任务**：评分趋势观察（收编/移除后的效果验证）；泳道动态、来源构成仍在原位继续可用。

## Information Architecture

「板块内容」tab 布局（本 change 后）：

```
┌ tab 内容 ──────────────────────────────────┐
│ [警告条]（仅有快照且低分时渲染，点击滚到面板） │
│ 话题卡片区（现有，不动）                      │
│ ┌───────────────────────────────────────┐ │
│ │ 版块体检治理面板（新，吸收构成管理）        │ │
│ │  header: 评分徽章·上次体检·＋添加构成     │ │
│ │  LLM 评语条                            │ │
│ │  指标 chips                            │ │
│ │  构成标签质量表（行内移除）               │ │
│ │  组合标签区（创建/移除挂载）              │ │
│ │  评分趋势 | 未收编候选（收编快捷动作）     │ │
│ └───────────────────────────────────────┘ │
│ 泳道动态 LaneDynamicsPanel（独立组件，位置不动）│
│ 来源构成 BoardSourcePanel（不动）            │
└─────────────────────────────────────────────┘
```

- **BoardCompositionPanel 退役**：其全部职能（aux 添加/移除、组合创建/挂载移除、picker）平移进治理面板；aux chips 纯展示（label+ref_count）被质量表替代，ref_count 展示废弃（用户决策）。
- 无体检快照时：评分徽章显示「等待首轮体检」占位、质量列「待体检」，构成管理照常可用（面板不因无体检数据瘫痪）。

## Interaction Contract

- **主操作**：
  - 「＋ 添加构成」→ 展开/收起 picker——**直接复用既有 `AuxiliaryLabelPicker` 组件**（自带搜索 + 分页 pageSize=20 + 搜索重置页码；双数据源兼容：edit 模式 `suggestAuxiliariesForBoard` 版块推荐 / create 模式 `suggestAuxiliaries` 手动指定，两条路径均保留，即旧的手动指定添加完全兼容）→ 确认调用现有 addComposition 接口；提示「添加后历史归属不自动回填，可手动触发回填」（沿用现有 notice 语义）。
  - 行内「✕」移除 → confirm 对话框（沿用现有文案「不会自动回填历史数据」）→ 现有 removeFromComposition 接口；组合挂载移除同。
  - 「收编」快捷动作（未收编候选行）→ 直接调用现有 addComposition（即时挂载）→ toast「已收编」+ 刷新构成与候选；**不走 LLM 建议管道**（手动挂载本就是用户特权，证据在眼前一步到位）。
  - 「＋ 创建组合」→ 现有 CompositeCreateDialog（平移挂载）。
- **次操作**：警告条点击滚动定位；趋势为纯展示。
- **长列表防炸（用户补充要求）**：质量表/组合列表/未收编候选均为快照内存数据，统一用折叠式——默认展示 top 10（按样本数/频次降序），尾部「展开全部 (N)」切换；picker 分页由复用组件自带，不自造。
- **反馈方式**：成功 toast / 失败行内 notice + 重试（沿用现有模式）；confirm 对话框文案保持现有语义不变。
- **危险操作语义**：移除/收编均不自动回填历史归属（沿用现有产品语义，文案明示）。

## State Matrix

| 状态 | 触发 | 呈现 | 恢复路径 |
| --- | --- | --- | --- |
| loading | 面板挂载/切板块/体检数据拉取 | 质量区骨架文案；构成基础数据并行加载 | 完成即出 |
| empty（无快照） | 体检从未跑过 | 徽章「等待首轮体检」+ 引导文案；质量列「待体检」；**构成管理照常** | 首轮体检后自动有数据 |
| error（拉取失败） | 体检 API 失败 | 体检区错误文案+重试；构成管理独立加载不受影响 | 重试按钮 |
| error（构成操作失败） | add/remove 接口失败 | 行内 notice + 重试（现有模式） | 重试 |
| success（正常） | 有快照 | 全量：评分/评语/chips/质量表/组合/趋势/候选 | — |
| insufficient（标签级） | 样本 < 最小样本量 | 该行「数据不足」+ 分布列「—」 | 样本积累后自动恢复 |
| 警告条 | 有快照且评分低于告警线 | tab 顶部红条 | 评分回升或处理后消失 |

## Layout Contract

- **layout mode**：/tags 页既有 contained（1120）布局内，无新 layout mode、无自由宽度。
- **目标视口**：桌面 1440×900、1920×1080（现有验收基线）；面板内部 two-col（趋势|候选）在窄容器下 flex-wrap 折行为单列（min-width 300px）。
- **无 dialog 变更**：确认移除沿用浏览器原生 confirm（现有行为）；创建组合沿用现有 CompositeCreateDialog（尺寸档不变）。
- 组件级面板样式完全对齐既有 bsp-panel 面板语言（border-subtle 圆角 10 + bg-elevated），主题变量同名引用。

## Component Reuse

| 复用 | 来源 | 说明 |
| --- | --- | --- |
| AppButton / AppInput / AppSectionHeader | 统一组件（theming.md 必复用清单） | header「＋ 添加构成」用 AppButton；picker 搜索框用 AppInput；区块标题（构成标签/组合标签/趋势/候选）用 AppSectionHeader |
| 面板容器/header/表格/chips 模式 | BoardSourcePanel | 同款 bsp-* 结构语言（新 bhp-* 前缀） |
| 评分色档语义 | FeedSourceQualityPill | 好/中/差三档色 + subtle 背景（Layer 2 token 派生，不硬编码） |
| picker（搜索+分页+双模式） | AuxiliaryLabelPicker（原样复用） | 版块推荐 + 手动指定双路径兼容；分页搜索自带 |
| CompositeCreateDialog | 现有组件（平移挂载） | 零修改 |
| addComposition / removeFromComposition | 现有 API client | 收编快捷动作同接口 |
| Icon（@iconify/vue） | 全局 | mdi:heart-pulse 等 |

**双主题硬约束（theming.md）**：所有颜色仅引用 Layer 2 语义 token（`--color-*`），禁用 `--raw-*` 与硬编码色值；评分色档/警告条/徽章样式均从语义 token 派生；根元素 `data-theme="editorial|dark"` 切换下面板两套主题均可用。**Observability 分层哲学定位**：体检治理面板属探究区（同日报 section 徽章先例），分数/均值/占比等数字展示合理；警告条接近正文区，措辞偏形态化（「体检发现质量预警」优先于裸数字，数字点击进入面板后呈现）。

新组件：`BoardHealthPanel.vue`（+ `.test.ts`）承担全部治理面板职能；`LaneDynamicsPanel`、`BoardSourcePanel` 挂载点迁至 TagsPage 直接渲染（BoardCompositionPanel 删除）。

## Prototype

静态原型 `ui-prototype/index.html`（vanilla HTML/CSS + 原型自用状态切换器，不连 API）：四态演示——健康（78 分全量布局）、健康·添加展开（picker 交互形态）、需关注（52 分红档 + 顶部警告条）、无快照（「等待首轮体检」+ 构成管理照常）。收编按钮带 toast 交互演示。样式引用 main.css 同名主题变量（含深色 media query）。mock 数据为讨论中的「AI 前沿」示例。

## Acceptance

- opencli 主链路断言（实现后补）：板块内容 tab → 面板渲染（评分徽章/质量表/组合区/候选）→ 行内移除 confirm → picker 添加（含手动指定路径）→ 候选收编 toast → 无快照版块构成管理可用。
- 1440×900 / 1920×1080 两档视觉检查：面板与邻居（泳道动态/来源构成）视觉一致，two-col 折行正常。
- **双主题两档视觉检查**：`data-theme="editorial"` 与 `data-theme="dark"` 下面板全量渲染正常（评分色档/警告条/徽章/表格在两套主题下均从语义 token 正确派生，无硬编码色残留；原型含主题切换按钮可预览两套效果）。
- 组件测试：七状态矩阵逐态断言 + 移除/添加/收编三动作交互断言（见 test-cases.md）。
- 与批准原型的差异说明（用户批准后补充，不重置审批）：①「＋ 添加构成」实现为直接复用既有 AuxiliaryLabelPicker（原型中简化 chips 示意），其搜索/分页/双模式（版块推荐 + 手动指定）原样保留；② 长列表防炸：质量表/组合列表/未收编候选默认折叠 top 10 + 「展开全部 (N)」（原型中未体现，属于交互细节补充非信息架构变更）；③ 双主题前置合规：原型主题机制由 `prefers-color-scheme` 修正为项目实际的 `data-theme` 切换（原型内含主题切换演示按钮），Component Reuse 补统一组件登记与 Layer 2 token 硬约束；④ 指标 chips 删「单 aux 占比」条（2026-10-08 口径修正：该指标随旧归因口径退役，chips 布局不变，内容减一项非信息架构变更，不重置审批）。
