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

### Requirement: 浮层组件展示合理性锚

**级别**: MUST

自建浮层/弹窗容器（Teleport + overlay，存量未迁 AppDialog 的组件）与新浮层代码 SHALL 在组件单测中携带「展示合理性」机械锚，防止样式重构丢失定位规则：

1. **样式规则锚**：源码 `<style>` 中 overlay 规则存在（`position: fixed` + `inset: 0` + `z-index`）；
2. **Teleport 挂载锚**：不 stub teleport 挂载时 overlay 渲染在 `document.body` 而非组件原地。

模板与落地实例见 skill `ui-verify`「浮层展示锚模板」；验收四维度见《开发执行规范》§5.3。

#### Scenario: 重写样式块

- **WHEN** 浮层组件的 `<style>` 块被重写/精简，定位规则丢失使弹窗退化为流内 div
- **THEN** 机械锚单测 SHALL 失败拦截（而非依赖 lint/单测都无法发现后流入用户界面）

### Requirement: major UI change 双视口验收

**级别**: MUST

声明 `ui-impact: major` 的 change 验收 SHALL 覆盖 **1440×900** 与 **1920×1080** 两档桌面视口（布局符合所选模式、contained 不超 1120px 居中、workspace 使用可用宽度、dialog 不超 92vw、无横向溢出）；触及窄屏降级路径（主工作台/抽屉/单栏切换）的 change 另加 **375×667** 与 **390×844** 窄屏档。验收方式见《开发执行规范》§5.3（opencli 交互断言 + 视觉子代理分流）。

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

核心页面（主工作台/发现/设置/标签/日报）在 375px 宽视口下 SHALL 无横向溢出、交互元素可达；loading/empty/error 各状态同样不破版。存量页面已存在的其他断点值（如 720px/768px 整点）不强制回改，但新代码一律用 767.98px 断点。

### Requirement: UI 验收机械断言与标准环境

**级别**: MUST

UI 验收（尤其窄屏/响应式路径）的通过依据 SHALL 是**机械断言**，不得依赖执行方（模型/子线程）的视觉能力或「目测截图」：

1. **弹层打开态层叠**：每个弹层（溢出菜单/通知面板/抽屉/dialog）打开后用 `elementFromPoint` 断言命中弹层本体——backdrop-filter/transform 均可创建 stacking context 导致 `z-index` 失效，仅 DOM 存在性断言测不出遮挡；
2. **条件渲染任务态**：队列进度 chip、进度条等条件渲染元素**在场时**重走顶栏/布局断言（不能只在无任务「干净态」验收）；
3. **逐交互状态测 scrollWidth**：列表态/阅读态/弹层打开态各自断言 `documentElement.scrollWidth <= 视口宽`，只测首屏不构成验收；
4. **真实长内容验证**：用含连续长单词（如满屏字母/长 URL）与超宽媒体的真实文章验证断词兑底（`overflow-wrap: break-word`），不应用短标题假数据。

验收环境 SHALL 用**静态发布路径**（`NUXT_PUBLIC_API_BASE=/api pnpm generate` → 铺 `backend-go/frontend/` → `:5100` 同源，见 [deployment.md](../deployment.md) §本地裸跑静态托管）：dev server 的 apiBase/HMR/首访引导均为噪声源，不作为验收依据。验收前 SHALL 关闭新手引导类全屏遮罩（项目已默认关闭首访自动启动，仅保留手动入口）。截图作为证据留档；有视觉能力的复核方 SHOULD 抽检截图，缺视觉时不构成阻断（机械断言为准）。

#### Scenario: 弹层遮挡机械检出

- **WHEN** 窄屏下点开「⋯」溢出菜单并用 `elementFromPoint` 探测菜单项中心
- **THEN** 命中结果 SHALL 为菜单项本体而非内容面板元素（stacking context 遮挡在此步暴露，而非流入用户实机）

## 存量迁移

- **不迁移**：存量页面的自由宽度与旧 `width` 弹窗保持现状，仅在主动重构该界面时按本契约收敛（避免大量用户可见变化混入其他 change）。
- 新代码（新页面/新弹窗/重构）一律走本契约；例外必须登记在 change 的 `ui-design.md`。

## 组件速查

| 场景 | 组件/用法 |
| --- | --- |
| 页面骨架 | `<AppPageShell mode="reader\|contained\|workspace\|split">` |
| 弹窗 | `<AppDialog size="sm\|md\|lg\|xl">`（旧 `width` 勿用于新代码） |
| 窄屏导航抽屉 | `<AppSidebarDrawer :open=".." @close="..">`（仅窄屏容器，宽 min(80vw,320px)，来源 mobile-viewport-stage1） |
| 窄屏断点 | `useIsNarrowViewport()`（composables/useMediaQuery.ts，断点 767.98px） |
| 按钮/输入/开关/标题 | `AppButton` / `AppInput` / `AppToggle` / `AppSectionHeader`（见 [theming.md](theming.md)） |
