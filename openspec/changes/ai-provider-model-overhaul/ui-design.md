<!-- ui-impact: major -->
<!-- ui-approval: approved -->
<!-- ui-prototype: ui-prototype/index.html -->

<!--
  marker 契约（ui-design-gate 机器读取）：
  - ui-impact 与 proposal 一致（major）
  - major → 八节 + ui-prototype/ 静态原型；approval 由用户浏览器看原型明确确认后才可改 approved；
    信息架构/主流程/布局模式修订后重置 pending
  - 设计沿革：概念 A「路由矩阵连版（报刊风）」已于 2026-09-28 用户否决（隐喻表意错位、
    墨量编码分辨率不足 3<7 档、矩阵 54% 空格、词汇转译成本高），本版为概念 B
    「设置页常规形态」——回归 SettingsWorkspace 既有分区与组件语言，不引入新隐喻；
    概念 A 无争议资产保留：两段式新增弹窗、扇出提示（转常显徽章）、事件「定位」跳转
-->

## User Journey

**入口**：设置页（SettingsWorkspace，sidebar 分组导航不动）→「AI 配置」组两个 section；观测在「运行状态」组的 ai-health tab。

**主任务**：
1. **管理供应商与模型**（「AI 模型」section 重构）：供应商卡片（logo/名称/base_url/健康点/模型数/**承载 N 能力常显徽章**）点击展开模型清单（启用勾选/类型/默认思考档/新·stale 标），卡片操作行刷新列表/测试连接/编辑/删除；新增走两段式弹窗（选预设 → 只输 API Key → 保存自动拉模型清单）。
2. **配能力路由与思考档**（「能力路由」section 升级，沿用既有链式卡片形态）：每能力一条绑定链，链项 = 序号 + 模型@供应商 + 思考档下拉（继承（模型默认）/七档显式文字）+ ↑↓×；hover 模型名提示「另承载 N 能力」；底部 + chips 按 vendor 分组添加。
3. **查降级**（「运行状态」→ ai-health tab 新增观测块）：24h/7d 统计行 + 降级事件流（类型分「结构化降级 / 供应商失败降级」），事件「定位」→ 跳能力路由 section 对应行并脉冲高亮。

**次任务**：编辑供应商（弹窗预填）、删除供应商（级联确认列明影响）、刷新模型列表看 diff（新增标「新」、上游消失灰显 stale）。

## Information Architecture

```
SettingsWorkspace（不动：header + sidebar + content）
├─ AI 配置组
│  ├─ 「AI 模型」ai-providers ──→ 供应商与模型管理（AIProviderManagement 重构）
│  │    ├─ 供应商卡片列表（展开式：卡片头 + 模型清单面板）
│  │    │    卡片头：logo / 名称 / base_url / 健康点 / N 模型 / 承载 N 能力徽章
│  │    │    清单：启用 / 模型名(+新/stale) / 类型(llm|embedding) / 默认思考档
│  │    │    操作：刷新模型列表 / 测试连接 / 编辑 / 删除
│  │    └─ 「+ 新增供应商」→ 两段式弹窗
│  └─ 「能力路由」capability-routes ──→ 链式卡片（AIRouterCapabilityRoutes 升级）
│       每 capability 一个 route-section（badge + 名称 + N 模型计数）：
│       绑定链 = model 卡片序号列表（drag + ↑↓×），链项含思考档下拉；
│       底部 + chips 按 vendor 分组（embedding 行只列 embedding 模型）
└─ 运行状态组 runtime-status（tab 不动）
     └─ ai-health tab：
         ├─ 现有健康快照 + 自动拉起开关（保留不动）
         └─ 新增「降级观测」块：时间窗切换(24h/7d) + 统计行(调用/降级/失败)
             + 事件流（时间/能力/顺位迁移/模型/类型/错误码/定位）
```

- **导航零变化**：sidebar「AI 配置」组仍是两项，深链（section=ai-providers / capability-routes / runtime-status&tab=ai-health）全部沿用，无重定向成本。
- **词汇回归直白**：供应商/模型/能力路由/思考档/覆盖/降级事件——概念 A 的报刊词（供稿/勘误/栏序/墨量/红笔）全部废弃。
- **扇出信息转正**：供应商卡片常显「承载 N 能力」徽章（单点依赖警示），能力路由 hover 提示补反向视角。

## Interaction Contract

**主操作（能力路由链）**：
| 操作 | 流程 | 反馈 |
|---|---|---|
| 添加绑定 | 底部 + chips 点选（按 vendor 分组） | 新链项追加链尾，入场过渡 |
| 调顺序 | 链项 drag 或 ↑/↓ | 即时换位（沿用现有 drag 实现） |
| 调思考档 | 链项下拉：继承（模型默认 N）/关/微/低/中/高/超高/极 | 下拉值即时存；覆盖时链项标「覆盖」小徽章 |
| 移除绑定 | 链项 × | 链项移除，空链显示既有空态引导 |
| 看扇出 | hover 链项模型名 | tooltip「qwythos 另承载 6 个能力」 |
| 保存 | section 头部「保存路由」（沿用现有） | saving 锁 + 成功/失败 toast |

**次操作（供应商卡片）**：点击展开/收起模型清单；清单内勾选启用、类型下拉（仅本地服务可标 embedding）、默认思考档下拉（embedding 为 —）；刷新模型列表（loading 锁，diff 标「新」/stale）；测试连接三态；编辑（两段式弹窗预填）；删除（sm 确认弹窗，级联列明 N 模型 M 绑定）。

**新增供应商（AppDialog lg，两段式）**：①预设目录（DeepSeek/小米 MiMo/ZAI/OpenRouter/Ollama/llama.cpp/自定义）→ ②预设只需输 API Key（base_url 可改；llama.cpp 另有启动命令字段，文案说明后端执行、跨主机留空）；自定义走全量表单；高级折叠（超时/思考方言/结构化输出覆盖）；测试连接三态；保存后自动拉模型清单并展开卡片。

**观测块（ai-health tab）**：时间窗切换改统计行；事件流行含模型名与类型标（结构化降级 / 供应商失败）；「定位」→ 切到能力路由 section、滚动到对应 route-section、行脉冲高亮一次。

**危险操作**：删除供应商（AppConfirmDialog sm=420，级联影响列明，确认按钮 danger 色）；移除绑定（轻操作不确认，可重加）。

## State Matrix

| 状态 | 供应商与模型 | 能力路由 | 降级观测块 |
|---|---|---|---|
| loading | 卡片骨架行 | route-section 骨架 | 统计行+流水骨架 |
| empty | 「还没有供应商」+ 新增引导 | 空链虚线引导（沿用现有空态） | 「窗口内无记录」 |
| error | 列表加载失败内联条 + 重试 | 同左 | 同左 |
| success | 卡片+清单 | 链式卡片 | 统计+事件流 |
| 刷新 loading/失败 | 按钮锁 / toast 保留旧清单 | — | — |
| 测试连接 | 弹窗内联三态 | — | — |
| 误输入 | 弹窗字段级校验 | — | — |
| 超长文本 | 模型名/base_url ellipsis+title | 链项模型名 ellipsis | 能力/错误码 ellipsis |
| 重复提交 | 保存按钮 loading 锁（沿用现有 saving） | 同左 | — |
| 错误恢复 | 失败可重试，无死胡同 | 换位失败回弹 | 保留上次结果 + 提示 |

## Layout Contract

- **layout mode：`workspace`**（SettingsWorkspace 既有：sidebar + content 滚动容器，本 change 不动框架）；content 区沿用现有 padding（24px 28px）。
- **目标视口**：桌面 1440×900 与 1920×1080（major 双视口验收）；窄屏走设置页既有降级（sidebar 抽屉 + 单栏），无新增断点。
- **弹窗**：新增/编辑供应商 `AppDialog size="lg"`（760px，92vw 上限）；删除确认 `AppConfirmDialog`（sm=420 档）。
- **溢出策略**：供应商卡片/链项模型名/base_url ellipsis+title；清单表与链在 content 滚动容器内自然滚动；页面级无横向溢出；观测事件流窄屏下隐藏次要列（模型名保留）。

## Component Reuse

| 用途 | 复用/新增 |
|---|---|
| section 头 | `AppSectionHeader`（icon + 标题 + 描述，三 section 全用） |
| 弹窗/确认 | `AppDialog size="lg"` / `AppConfirmDialog` |
| 按钮/输入/下拉/开关 | `AppButton` / `AppInput` / 既有 ai-select / `AppToggle` |
| 卡片容器 | route-card 既有形态（12px 圆角 + header 分隔线）复用为供应商卡片 |
| 链式排序 | 现有 drag + ↑↓× 交互原样沿用（`AIRouterCapabilityRoutes` 既有实现） |
| logo | pi `provider-icons.svg` symbol 内嵌（实现时补全各厂商，原型仅示意） |
| **改造** | `AIProviderManagement.vue` → 供应商与模型管理；`AIRouterCapabilityRoutes.vue` → 绑定项 provider 升级为 model + 思考档列 |
| **新增** | `VendorCard.vue`（卡片+扇出徽章+展开清单）、`ThinkingLevelSelect.vue`（七档下拉）、`DegradationPanel.vue`（观测块：统计行+事件流+定位） |

**视觉基调**：完全跟随产品既有组件语言（App 系 token、卡片 12px 圆角、13-14px 字阶），无新隐喻、无特殊字体动作；思考档以显式文字呈现（下拉 + 链项小徽章），不用字重/浓淡编码；「覆盖」徽章用 accent-subtle 底色小标签。

## Prototype

`ui-prototype/index.html`——静态可丢弃原型（单文件，fixture 内嵌，不连 API）。打开：浏览器直接打开，横幅可切双主题；顶部视图切换条模拟三个目标界面（真实入口是设置页 sidebar + runtime-status tab，原型内每视图顶部有文字注明）。

- **视图一 供应商与模型**：卡片展开清单、扇出徽章、两段式新增弹窗（选预设→输 Key→高级折叠）、编辑预填、删除级联确认、刷新 diff（新/stale）。
- **视图二 能力路由**：链式卡片（drag 某次序示意 + ↑↓×）、思考档下拉（继承/七档）、覆盖徽章、hover 模型名扇出 tooltip、+ chips 按 vendor 分组、embedding 行过滤。
- **视图三 降级观测**：24h/7d 切换统计、事件流（类型分结构化降级/供应商失败，含模型名）、「定位」跨视图跳转 + 行脉冲。

## Acceptance

- **opencli 主链路断言**（实现后补）：① 新增供应商两段式全流程 → 新卡片+模型清单出现；② 链上添加绑定+调思考档 → 链项与「覆盖」徽章变化；③ hover 模型名 → 扇出 tooltip；④ 观测事件「定位」→ 跳能力路由对应行并高亮；⑤ 删除供应商级联确认文案正确。
- **视觉检查**：1440×900 与 1920×1080 双档截图（workspace 框架不变、dialog 不超 92vw、无横向溢出），证据记入 tasks.md 验证节。
- **与批准原型差异说明**：实现完成后逐项对照；触及信息架构/主流程/布局模式的差异重置 approval 重新审批。
- **与批准原型差异清单**（2026-09-29 实现后逐项对照，均非 IA/主流程/布局级差异，不重置 approval）：
  1. 卡片头「健康点」未实现——原型示意的数据源是路由级健康快照（ai_health_routes），与卡片级 vendor 口径不一致；留待后续 /ai/vendors 附 vendor 健康字段后补。
  2. 模型清单表新增「xh/max」复选列（原型 5 列→实际 6 列，落点 PUT /api/ai/models/:id 的 supports_xhigh_max），支撑思考档支持度钳制。
  3. logo 资产：原型用 pi provider-icons.svg，实现改为 Iconify mdi 预设图标 + 未知供应商首字母回退。
  4. 两处 loading 呈现差异：列表加载/测试连接用 spinner，DegradationPanel 保留真骨架行。
  5. chips 点选增强为 toggle（已在链中再点即移除），原型仅单向追加。
