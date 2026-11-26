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

### Requirement: 交互模式选择判据先于布局定稿

**级别**: MUST

布局模式（本文件：`reader` / `contained` / `workspace` / `split`）只决定**容器宽度与栏数**，不决定操作以什么形态发生。同一页面可以是 `split` 布局 + `inline-edit` 交互。交互模式的词表、选择判据与禁止组合见 [interaction.md](interaction.md)（`page` / `panel` / `drawer` / `dialog` / `inline-edit` / `canvas` / `command`），本文件不重复判据。

`ui-design.md` 的 `## Interaction Decisions` 台账 MUST 先于 Layout Contract 定稿——布局是为已选定的交互模式提供容器，反序会诱导「先挑一个好写的容器、再往里塞操作」。视觉与动效的下限（对比度、焦点可见、`prefers-reduced-motion`、glass/glow 剂量）见 [interaction.md](interaction.md)。

#### Scenario: 单字段修改被塞进弹窗

- **WHEN** 某操作只需改一个名称，`ui-design.md` 却把它定为 `dialog`
- **THEN** review SHALL 依 `interaction.md` 判据 P2 要求改为 `inline-edit`，布局随之调整（不因此更换布局模式）

#### Scenario: 用弹窗尺寸档反推交互模式

- **WHEN** 设计理由是「这个内容 `md` 档装得下」
- **THEN** SHALL 判定为理由无效——尺寸档解决的是尺寸，不构成选弹窗的理由

### Requirement: 画布类工作面必须按容器真实像素 1:1 绘制

**级别**: MUST

> 本节约束的是**渲染保真**（选定了画布之后要画对），SHALL NOT 被读作「推荐使用画布」；是否该用画布见 [interaction.md](interaction.md) 判据 P1。

以图形画布为主体的工作面（点线图、关系图、编排画布）SHALL 按**容器真实像素 1:1** 绘制：SVG 坐标系与 CSS 像素 1:1（viewBox 宽 = 容器 `clientWidth`，或干脆不设 viewBox 而用 px 坐标），SHALL NOT 在挂载/开页那刻算死一个宽度后随视口变化被缩放或裁边；窗口尺寸变化 SHALL 重排重绘。

图面上的文字、编号与横条 SHALL NOT 越出画布边界；标签冲突时 SHALL 宁缺勿叠（放不下就不画，不得压字）。点的尺寸 SHALL 以**像素**为下限（普通点 ≥3px 半径），并 SHALL 把「已选」与「相近程度」用**两套独立视觉**表达（实心 + 勾号 vs 大小/深浅），大小 SHALL NOT 同时表达选中。

#### Scenario: 窗口变化后画布比例

- **WHEN** 用户拉宽/拉窄窗口
- **THEN** 画布 SHALL 按新容器宽度重排重绘，比例仍为 1:1，且无任何元素越出边界

#### Scenario: 图面标签放不下

- **WHEN** 两个标题位置冲突或超出画布
- **THEN** 实现 SHALL 跳过冲突标题（可悬停看全文），SHALL NOT 互相压字或溢出画布

### Requirement: 弹窗必须使用统一尺寸档

**级别**: MUST

> 本节只解决「弹窗该多宽」。**是否该用弹窗**不在本节判定，见 [interaction.md](interaction.md) 判据 P1–P5——尺寸档装得下不构成使用弹窗的理由。

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
| `readingTooltip` | 9099 | 阅读层内的行内预览浮层（高于阅读台遮罩 `.drm-overlay` 9000、低于阅读层弹窗 9100；replace-lane-lifeline-with-topic-events） |
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

自建浮层/弹窗容器（Teleport + overlay，存量未迁 AppDialog 的组件）与新浮层代码 SHALL 在组件单测中携带「展示合理性」机械锚。这是 2026-09-07 浮层丢失定位、退化为流内布局的回归守卫：

1. **样式规则锚**：源码 `<style>` 中 overlay 规则存在（`position: fixed` + `inset: 0`；层级按「浮层层级契约」由档位表内联绑定注入时，`<style>` 不再写死 `z-index`）；
2. **Teleport 挂载锚**：不 stub teleport 挂载时 overlay 渲染在 `document.body` 而非组件原地。

模板与落地实例见 skill `agent-browser`「浮层展示锚模板」；验收四维度见《开发执行规范》§5.3。

#### Scenario: 重写样式块

- **WHEN** 浮层组件的 `<style>` 块被重写/精简，定位规则丢失使弹窗退化为流内 div
- **THEN** 机械锚单测 SHALL 失败拦截（而非依赖 lint 判定布局）

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

### Requirement: UI 复审的全系统与产品任务覆盖

**级别**: MUST

UI 复审 MUST NOT 只覆盖本次 change 新增组件/原型对照；涉及共享壳、导航或任一核心路由时，验收 SHALL 增加「全系统回归」一档：

1. **路由矩阵**：至少覆盖 `/`、`/search`、`/discovery`、`/settings`、`/tags`、`/tags/:boardId`、`/tags/:boardId/:date`、`/tags/:boardId/research`；每路由在 **editorial 与 dark** 两主题下检查，桌面 1440×900 必做，触窄屏的再加 375×667；
2. **产品任务走查**：按真实任务而非控件清单走一遍（例：只看今天的归档文章、从列表选每天事件、返回主页、冷启动新建版块）；旧版存在的功能 SHALL 先建「旧→新功能对照表」，缺失项要么迁移、要么在报告里显式判定为退役；
3. **暗色不是附加项**：新增独立路由/全屏页 MUST 提供自己的主题底色（或由全局 `html/body` 承接），不得依赖浏览器默认白底；复审证据须包含暗色截图；
4. **结论分流**：`pass/fail/未覆盖` 只描述证据，不替代产品判断；发现「功能没了/没法完成任务」时按 fail 处理，不得用「符合原型」放行。

#### Scenario: 共享壳改动后的复审
- **WHEN** change 改动 `/tags` 及其子路由或全局导航
- **THEN** `ui-verification/report.md` SHALL 包含全系统路由矩阵与至少 3 条产品任务走查，且含 dark 截图
- **AND** 若旧功能未迁移且未显式判定退役，归档 SHALL 被 review 阻断

## 已登记例外（固定视口栅格）

「需要自由宽度」条款之外的**固定视口栅格**（无页面级纵向滚动、仅舞台内滚动）属于登记制例外，逐条登记于此；未登记的页面一律按四模式处理。

| change | 界面 | 理由 | 目标视口 | 溢出策略 | 回落规则 |
| --- | --- | --- | --- | --- | --- |
| `rework-workshop-into-radar-and-desk`（原始登记） | L0 `/tags` 雷达、L1 `/tags/:boardId` 版块态势（L0 右栏底部新增常驻管线状态块，`add-radar-pipeline-status` 增补；L1 右轨「当日日报」块内新增「当日其他话题」子块，`add-other-topics-to-context-rail` 增补） | 观看便利：4K 本地屏一次看全（三栏常驻 + 矩阵不靠滚动寻找），非挂墙数据大屏 | ≥1440×900 / 1920×1080 / 4K（150% 缩放 → 2560×1440 CSS px） | 页面本身 `overflow: hidden`；只有矩阵舞台/归档列表/编排画布内部滚动；版块栏与治理右轨各自独立滚动；L0 右栏改「上滚下驻」两段容器——上段内容区（今日该读 / 今天可能升温）独立滚动、下段管线状态块常驻右栏底部不随之滚动（`add-radar-pipeline-status` 增补） | <1440px 回落 `workspace` + 页面滚动；<768px 按窄屏线性化：L0 雷达单列（隐藏迷你泳道矩阵）、L1 只渲染版块摘要（泳道列表 + 每行 14 天迷你条 + 最新日报入口，矩阵/右轨不渲染）、L3 阅读台单栏（索引收为顶部锚点条），编排入口提示「新建泳道需要宽屏或横屏」 |

登记后如信息架构或布局模式再变（例如新增第五区），MUST 回到该 change 的 `ui-design.md` 重置 `ui-approval` 并更新本表。

**登记项的增补口径**：表内 `change` 列记录**原始登记**该例外的 change；后续 change 若在同一界面上增补实现、且不新增分区、不改四种布局模式枚举，SHALL 在对应单元格内标注增补来源 change 名 + 在本表下方追加一条增补说明，SHALL NOT 为同一界面另起一行重复登记（重复登记会被读成两条互相独立的例外）。**`add-radar-pipeline-status` 的增补**：L0 `/tags` 雷达右栏底部新增常驻管线状态块——右栏改「上滚下驻」两段容器（上段内容区独立滚动 + 下段状态块常驻底部），**不新增第五区、不改布局模式枚举**，页面仍为三栏常驻，故仍属本条固定视口栅格例外、不触发上段的 `ui-approval` 重置条件；窄屏（<768px）该状态块随页面流落到雷达单列下方、板块清单单列（来源：`add-radar-pipeline-status` 的 `ui-design.md` 与 `ui-verification/report.md`）。

**`add-other-topics-to-context-rail` 的增补**：L1 `/tags/:boardId` 右轨「当日日报」块**内部**新增「当日其他话题」子块（当日未入正式/休眠泳道的分区：标题 + 该分区线索数 + 归属状态词）——子块挂在**既有顶层区块内**，右轨仍是版块体检 / 当日日报 / 话题追踪 / 构成候选 / 来源构成五块，**不新增第五区**，**四档布局模式枚举不变**、页面仍为三栏常驻 + `overflow: hidden`，故仍属本条固定视口栅格例外、不触发上段的 `ui-approval` 重置条件；子块默认隐藏、仅 `@media (min-width: 1440px)` 显示（1200–1439.98px 右轨随页面流动，出该子块会改变该视口下的页面高度，故该视口不出现），窄于 1440px 一律不渲染该子块（来源：`add-other-topics-to-context-rail` 的 `ui-design.md`）。

**泳道矩阵标签列宽 + 名称看全（2026-11-26 直接修复，用户当场要求不开 change）**：L1 矩阵第一列 MUST 按「先给日格下限、余量归名字」分配，不得再写死上限——`grid-template-columns: minmax(7rem, max(10rem, min(19rem, calc(100% - var(--matrix-cols, 14) * 2.6rem))))`：日格按 2.6rem 保底（两位数 + `MM/DD` 短格式的下限），余量全给标签列，再用 7rem / 19rem 卡上下界（约 20 个汉字）；>31 天档容器是 `min-width: max-content`（宽不定，百分比/calc 会退化成 auto），只给固定 `minmax(9rem, 13rem)`。名称仍被省略号截断时 SHALL 用悬停/聚焦卡片补全名与本行指标（`useNameHoverCard`：只对真被截断的文本出卡、不挂原生 `title`、Teleport + `fixed` 免被矩阵滚动容器裁剪、层级取 `Z_INDEX.toast`）；左栏版块名（栏宽仍为固定 17rem）同款浮层。实测 1920 / 1600 全泳道名零截断、1440 标签列 262px；再收窄就会退回「日格挤名字」（旧 `minmax(7rem, 10rem)` 在 1920 屏上标签列只有 160px、日格 70px 只放 1–3 位数）。

## 存量迁移

- **不迁移**：存量页面的自由宽度与旧 `width` 弹窗保持现状，仅在主动重构该界面时按本契约收敛（避免大量用户可见变化混入其他 change）。
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
| 右侧详情/治理抽屉 | `<AppDrawer :open=".." width="32rem" label=".." @close="..">`（Teleport + 遮罩 + Esc + body 滚动锁；32rem，窄屏 100vw；来源 rework-workshop-into-radar-and-desk） |
| 右栏常驻底部状态块 | 「上滚下驻」两段容器（上段内容区独立滚动 + 下段状态块常驻右栏底部、不随之滚动）：`front/app/features/tags/components/console/BoardRadarStatusBlock.vue` + `BoardRadarView.vue` 右栏（L0 `/tags` 雷达专用模式；来源 add-radar-pipeline-status；登记例外见 §已登记例外） |
| 窄屏断点 | `useIsNarrowViewport()`（composables/useMediaQuery.ts，断点 767.98px） |
| 按钮/输入/开关/标题 | `AppButton` / `AppInput` / `AppToggle` / `AppSectionHeader`（见 [theming.md](theming.md)） |
