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

声明 `ui-impact: major` 的 change 验收 SHALL 覆盖 **1440×900** 与 **1920×1080** 两档桌面视口（布局符合所选模式、contained 不超 1120px 居中、workspace 使用可用宽度、dialog 不超 92vw、无横向溢出）；产品明确支持窄屏时另加窄屏视口。验收方式见《开发执行规范》§5.3（opencli 交互断言 + 视觉子代理分流）。

#### Scenario: 宽屏构图验收

- **WHEN** major UI change 完成实现进入验收
- **THEN** 两档视口截图/检查证据 SHALL 记录在 tasks.md 验证节，宽屏内容被无限拉长视为阻断项（除非合同选择 workspace 并说明用途）

## 存量迁移

- **不迁移**：存量页面的自由宽度与旧 `width` 弹窗保持现状，仅在主动重构该界面时按本契约收敛（避免大量用户可见变化混入其他 change）。
- 新代码（新页面/新弹窗/重构）一律走本契约；例外必须登记在 change 的 `ui-design.md`。

## 组件速查

| 场景 | 组件/用法 |
| --- | --- |
| 页面骨架 | `<AppPageShell mode="reader\|contained\|workspace\|split">` |
| 弹窗 | `<AppDialog size="sm\|md\|lg\|xl">`（旧 `width` 勿用于新代码） |
| 按钮/输入/开关/标题 | `AppButton` / `AppInput` / `AppToggle` / `AppSectionHeader`（见 [theming.md](theming.md)） |
