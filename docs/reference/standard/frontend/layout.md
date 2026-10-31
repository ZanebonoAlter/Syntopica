# 前端布局契约（Layout）

<!--
doc-impact-applies: front/app/pages/, front/app/features/, front/app/components/ui/, front/app/components/dialog/ | section=Requirements
-->

> **权威源**：本文件是页面布局模式、内容宽度与弹窗尺寸档的唯一权威。UI 工作流（ui-impact 分档、ui-design.md 制品、原型审批）见《开发执行规范》§3 与 `openspec/schemas/syntopica-ui`；双主题 Token 见 [theming.md](theming.md)。
> 来源 change：`make-ui-design-first-class`（design D4）。

## 为什么需要布局契约

主题系统统一了颜色，却没统一空间——「居中还是宽屏」「弹窗多宽」此前是每个页面自由发挥（DiscoveryPanel 860px、各弹窗 520/672/680/90vw 随手写），导致宽屏构图不一致、review 无从对齐。把宽度决策收敛为**少而稳定的枚举**，才能在 UI 原型与实现之间稳定传递。

## Requirements

### Requirement: 新界面必须显式选择布局模式

**级别**: MUST

新建或重构页面/面板 SHALL 通过统一 page shell（`AppPageShell`）选择以下四种布局模式之一，不得在业务组件中自写 `max-width` 居中逻辑：

| 模式 | 默认约束 | 典型用途 |
| --- | --- | --- |
| `reader` | 正文最大宽 **760px**，居中 | 长文、报告正文 |
| `contained` | 内容最大宽 **1120px**，居中 | 设置、治理、表单、普通列表 |
| `workspace` | 填满可用宽度 | 看板、时间线、带常驻侧栏工作台 |
| `split` | workspace 基础上显式主从栏（各栏 min/max + 溢出策略） | 列表+详情、编辑器 |

治理/设置类浏览筛选页面默认 `contained`（宽屏居中，不随父容器无限拉宽）；带常驻侧栏或列表-详情联动的页面选 `workspace` 或 `split` 并记录各栏最小宽度。

#### Scenario: 新增治理列表页

- **WHEN** 新增以浏览、筛选和治理为主且无需多栏联动的列表页面
- **THEN** 实现 SHALL 使用 `AppPageShell mode="contained"`，内容在宽屏居中且不超过 1120px

#### Scenario: 需要自由宽度

- **WHEN** 少数特殊工作台确需脱离四模式
- **THEN** change 的 `ui-design.md` Layout Contract 节 MUST 记录理由、目标视口与溢出策略，经 review 允许

### Requirement: 弹窗必须使用统一尺寸档

**级别**: MUST

新弹窗 SHALL 使用 `AppDialog` 的 `size` 属性四档之一，受可视区宽度约束（**92vw 上限**）：

| 档 | 目标宽度 |
| --- | --- |
| `sm` | 420px |
| `md` | 560px |
| `lg` | 760px |
| `xl` | 1040px |

不得在业务组件中另写自由 `width`、重复 overlay/Teleport 或自建弹窗容器；旧 `width` prop 仅为存量兼容，新代码禁止使用。四档放不下时优先拆分内容或改页面，确需例外在 `ui-design.md` 说明。

#### Scenario: 新建弹窗

- **WHEN** 新弹窗可由四档尺寸之一容纳
- **THEN** 实现 MUST 复用 `<AppDialog size="...">`，不传自由 width

### Requirement: 浮层层级契约

**级别**: MUST

浮层层级 SHALL 取自唯一档位表（代码侧 `front/app/components/ui/layout-contract.ts` 的 `Z_INDEX`），组件 SHALL NOT 手写层级数字（存量例外见本节末「存量例外」与「存量迁移」登记表）：

| 档位 | 值 | 用途 |
| --- | --- | --- |
| `dialog` | 1000 | 基础弹窗遮罩（`AppDialog` 默认） |
| `dialogNested` | 1100 | 弹窗内再开的嵌套浮层 |
| `readingDialog` | 9100 | 阅读层弹窗 |
| `readingFullscreen` | 9200 | 弹窗内全屏层 |
| `toast` | 9999 | 全局提示 / 菜单 |

规则：

1. **弹窗内再开的浮层（嵌套浮层）MUST 严格高于宿主弹窗遮罩**——宿主遮罩与嵌套浮层同为 `body` 直接子节点，绘制顺序只由 z-index 决定，层级低于宿主即被整块盖住（用户点了入口完全看不到浮层）。
2. 嵌套浮层默认取 `Z_INDEX.dialogNested`；**宿主弹窗在更高档时**（如阅读层 9100）SHALL 用 `nestedOverlayZ(hostZ)` 抬升并显式传给嵌套浮层——返回值 SHALL 严格大于 hostZ，**且不得落在任何已占档位上**（档位表是唯一占用清单）：裸抬升撞上「弹窗内全屏层」9200 时 SHALL 继续抬升跳过该档（落到 9200 与 `toast` 之间的档间空隙，如 9300）；嵌套浮层 SHALL 不越过全局提示档 `toast`（9999）。
3. 全屏层 SHALL 高于其宿主弹窗遮罩，且低于全局提示层（`Z_INDEX.toast`）。
4. 层级数字 SHALL 由档位表**内联绑定**注入（`:style="{ zIndex }"`），样式表不再写死第二份——避免双源漂移；「浮层组件展示合理性锚」要求的 overlay 定位规则（`position: fixed` + `inset: 0`）仍 SHALL 保留在 `<style>` 中。

> 事故记述（2026-10-30 用户实机报障）：版块治理工作面（`AppDialog size="xl"`，遮罩 1000）内点「创建并挂载」打开的组合标签创建弹窗，遮罩层级在 CSS 里写死 **60**，被宿主遮罩连同 backdrop blur 整块盖住——按钮点了毫无反应。根因是层级没有契约、随手写数字，修复方式是引入本节档位表并让嵌套浮层取 `dialogNested` 档。

**存量例外**：本契约对**新增与改造**的浮层立即生效；已入库的存量组件仍有手写层级数字（登记表见「存量迁移」节）——**保持现状不迁移**（避免大量用户可见变化混入其它 change），但**主动重构该组件时 SHALL 一并收敛到档位表**。

#### Scenario: 弹窗内嵌套浮层高于宿主遮罩

- **WHEN** 用户在弹窗内打开一个嵌套浮层（治理工作面内打开组合标签创建弹窗）
- **THEN** 嵌套浮层的 z-index SHALL 严格大于宿主弹窗遮罩的 z-index，渲染在宿主之上，不被宿主遮罩遮挡

#### Scenario: 嵌套浮层档位可按宿主抬升

- **WHEN** 宿主弹窗使用高于基础档的层级（如阅读层弹窗 9100）
- **THEN** `nestedOverlayZ(hostZ)` SHALL 返回严格大于 hostZ 的层级，供宿主显式传给嵌套浮层
- **AND** 返回值 SHALL 不等于任何其它已占档位（如 `readingFullscreen` 9200），且 SHALL 不超过 `Z_INDEX.toast`

#### Scenario: 层级档位单一事实源

- **WHEN** 新增或修改任一浮层组件
- **THEN** 其层级 SHALL 取自 `Z_INDEX` 档位表（组件内不得手写层级数字），且 `AppDialog` 默认层级 SHALL 等于 `Z_INDEX.dialog`

### Requirement: 浮层组件展示合理性锚

**级别**: MUST

自建浮层/弹窗容器（Teleport + overlay，存量未迁 AppDialog 的组件）与新浮层代码 SHALL 在组件单测中携带「展示合理性」机械锚，防止样式重构丢失定位规则：

1. **样式规则锚**：源码 `<style>` 中 overlay 规则存在（`position: fixed` + `inset: 0`；层级按「浮层层级契约」由档位表内联绑定注入时，`<style>` 不再写死 `z-index`）；
2. **Teleport 挂载锚**：不 stub teleport 挂载时 overlay 渲染在 `document.body` 而非组件原地。

模板与落地实例见 skill `agent-browser`「浮层展示锚模板」；验收四维度见《开发执行规范》§5.3。

#### Scenario: 重写样式块

- **WHEN** 浮层组件的 `<style>` 块被重写/精简，定位规则丢失使弹窗退化为流内 div
- **THEN** 机械锚单测 SHALL 失败拦截（而非依赖 lint/单测都无法发现后流入用户界面）

### Requirement: major UI change 双视口验收

**级别**: MUST

声明 `ui-impact: major` 的 change 验收 SHALL 覆盖 **1440×900** 与 **1920×1080** 两档桌面视口（布局符合所选模式、contained 不超 1120px 居中、workspace 使用可用宽度、dialog 不超 92vw、无横向溢出）；触及窄屏降级路径（主工作台/抽屉/单栏切换）的 change 另加 **375×667** 与 **390×844** 窄屏档。验收方式见《开发执行规范》§5.3（agent-browser 交互断言 + 视觉子代理分流；证据以 `ui-verification/report.md` 建账，归档由 `scripts/harness/ui-verification-trace.sh` 实物对账）。

#### Scenario: 宽屏构图验收

- **WHEN** major UI change 完成实现进入验收
- **THEN** 两档视口截图/检查证据 SHALL 记录在 tasks.md 验证节，宽屏内容被无限拉长视为阻断项（除非合同选择 workspace 并说明用途）

### Requirement: 窄视口断点与宽屏零变化

**级别**: MUST

窄视口降级统一以 **`@media (max-width: 767.98px)`** 为断点（与 Tailwind md 对齐），JS 侧统一读 `useIsNarrowViewport()`（`front/app/composables/useMediaQuery.ts`），不得另立断点值。窄屏降级改动 SHALL 全部落在媒体块/窄屏分支内：**≥768px 视口的布局与交互零变化**是回归红线（新状态如 viewMode 不得出现在宽屏渲染路径）。

#### Scenario: 断点跨越状态保持

- **WHEN** 用户在窄屏阅读态把窗口放宽到 ≥768px
- **THEN** 布局恢复宽屏三栏，当前选中文章保持；缩回窄屏时回到列表态

### Requirement: 主工作台窄屏降级（抽屉 + 单栏切换）

**级别**: MUST

主工作台（FeedLayoutShell）窄屏下：常驻侧栏隐藏，经顶栏汉堡入口以 `AppSidebarDrawer` 抽屉呈现（宽 `min(80vw, 320px)`，遮罩/Esc/选中即关）；文章列表与内容互斥单栏（`viewMode: 'list' | 'reading'`，仅窄屏分支读取），点文章进阅读态、返回回列表态并恢复列表滚动位置（超出已加载范围先补一页，仍不足落顶）。高度单位用 `100dvh`（`@supports` 回退 `100vh`）。

#### Scenario: 抽屉选中節选

- **WHEN** 用户在抽屉内点选订阅源/标签
- **THEN** 筛选生效、抽屉关闭、界面回到列表态

### Requirement: 核心页面 375px 不破版基线

**级别**: MUST

核心页面（主工作台/发现/设置/标签/日报）在 375px 宽视口下 SHALL 无横向溢出、交互元素可达；loading/empty/error 各状态同样不破版。存量页面已存在的其他断点值（如 720px/768px 整点）不强制回改，但新代码一律用 767.98px 断点。验收的机械断言清单与标准环境见 [`standard/shared/test-design.md`](../shared/test-design.md)「UI 验收机械断言」节（通用，不限窄屏）。

## 存量迁移

- **不迁移**：存量页面的自由宽度与旧 `width` 弹窗保持现状，仅在主动重构该界面时按本契约收敛（避免大量用户可见变化混入其他 change）。
- 新代码（新页面/新弹窗/重构）一律走本契约；例外必须登记在 change 的 `ui-design.md`。

- **不迁移（层级同款）**：存量浮层组件里手写的层级数字保持现状，仅在主动重构该组件时收敛到 `Z_INDEX` 档位表；新建或改造的浮层 MUST 取档位表，不得手写层级数字。遗留清单见下表。
- 新代码（新页面/新弹窗/重构）一律走本契约；例外必须登记在 change 的 `ui-design.md`。

### 遗留手写层级登记表

| 组件 | 手写层级值 | 收敛时机 |
| --- | --- | --- |
| `MarginNotesRail.vue` | 9100 | 主动重构该批注侧栏时改取 `Z_INDEX.readingDialog` |
| `ArticlePreviewModal.vue` | 9100 / 9200（本地常量） | 主动重构该预览弹窗时改取 `Z_INDEX.readingDialog` / `Z_INDEX.readingFullscreen` |
| `AppTooltip.vue` | 9999 | 主动重构该 tooltip 时改取 `Z_INDEX.toast` |
| `FeedActionMenu.vue` | 9999 | 主动重构该操作菜单时改取 `Z_INDEX.toast` |
| `NotifyContainer.vue` | 9999（Tailwind `z-[9999]`） | 主动重构该通知容器时改取 `Z_INDEX.toast` |
| `AppSidebarDrawer.vue` | 1000 / 1001 | 主动重构该抽屉时改取 `Z_INDEX.dialog`（遮罩）与嵌套浮层档 |
| `DateAssociationMarker.vue` | 1000 | 主动重构该标记时收敛到档位表（`Z_INDEX.dialog`） |
| `AppDrawer.vue` | 60 / 61 | 主动重构该抽屉时改取 `Z_INDEX.dialog`（遮罩）与嵌套浮层档 |
| `UpgradeSuggestionPanel.vue` | 100 | 主动重构该面板时改取 `Z_INDEX.dialogNested` |
| `TagMergePreview.vue` | 78 | 主动重构该预览时改取 `Z_INDEX.dialogNested` |
| `BoardRadarGovernance.vue` | 60 | 主动重构该治理面时收敛到档位表（浮层取 `Z_INDEX.dialogNested`） |

> 登记表只记「组件 → 手写层级值 → 收敛时机」，不要求逐行改造；新写浮层一律不得再进表。

## 组件速查

| 场景 | 组件/用法 |
| --- | --- |
| 页面骨架 | `<AppPageShell mode="reader\|contained\|workspace\|split">` |
| 弹窗 | `<AppDialog size="sm\|md\|lg\|xl">`（旧 `width` 勿用于新代码） |
| 浮层层级 | `Z_INDEX` 档位表（`~/components/ui/layout-contract`）；嵌套浮层默认 `dialogNested`，宿主更高档时 `nestedOverlayZ(hostZ)` 抬升（跳过已占档位、不越过 `toast`） |
| 窄屏导航抽屉 | `<AppSidebarDrawer :open=".." @close="..">`（仅窄屏容器，宽 min(80vw,320px)，来源 mobile-viewport-stage1） |
| 窄屏断点 | `useIsNarrowViewport()`（composables/useMediaQuery.ts，断点 767.98px） |
| 按钮/输入/开关/标题 | `AppButton` / `AppInput` / `AppToggle` / `AppSectionHeader`（见 [theming.md](theming.md)） |
