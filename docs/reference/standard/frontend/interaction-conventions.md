# 前端交互约定（Interaction Conventions）

> **权威源**：本文件是前端"交互友好性 / 可观测性展示分层"约定的唯一权威。`front/AGENTS.md` 速查深链指向本文件。
> 互补：[theming.md](./theming.md)（视觉 token）、[code-style.md](./code-style.md)（代码结构）。
>
> 这些约定源自真实评审踩坑，非凭空设计——每条都附案例。

## 1. 状态标记必须在左侧醒目位，不得埋在右侧 meta

**规则**：用一个图标/色点标记某条目的状态（离群/警告/降级等）时，图标必须放在**条目标题的左侧同一行**（标题文字前），且带 `aria-label`。**禁止**把状态图标塞进条目右侧的 meta 区（与计数、箭头挤在一起）。

**理由**：左侧是视觉阅读起点（F 型扫描第一落点），状态一眼可见；右侧 meta 聚集了计数、折叠箭头等多个元素，状态图标会被淹没到"根本看不见"。

| 位置 | 可见性 | 取舍 |
|------|--------|------|
| 标题左侧（标题文字前） | ✅ 强 | **采用** |
| 标题右侧 meta 区 | ❌ 几乎不可见 | **禁止** |

**DOM 结构**：用 inline-flex 容器（如 `.drm-thread__title`）包裹 `图标 + <strong>标题</strong>`，图标在 `<strong>` 之前的 DOM 顺序即视觉左位。

**案例**：thread-fit 软降级初版把"可能跑题"alert 图标放在 thread 右侧 meta（与"X 篇"计数、折叠箭头并列），评审反馈"在最右边太隐秘根本看不见"。改为标题左侧后状态一目了然。见 `DailyReportTopicSection.vue` `.drm-thread__flag`。

## 2. 状态说明行不得伪装成动作

**规则**：section/列表底部的"统计说明"行（如"另有 N 条可能跑题的线索"）若只是**状态说明**（告诉用户这里有什么），必须渲染成**不可交互的 `<p>`**（无 hover 高亮、无 cursor pointer、无焦点环）。**禁止**把它做成 `<button>` 伪装成可点击动作——除非它真的绑定了一个有意义的批量动作。

**理由**：一个看起来可点、但点了"没意义"的按钮，比没有按钮更糟——用户会期待它做点什么，结果失望/困惑。状态说明就该长得像状态说明。

**判定**：
- 状态说明（仅告知"这里有多少离群/异常"）→ `<p>`，不可点。
- 真批量动作（点了有明确、有用的效果，如"展开所有详情"）→ `<button>`，且文案是动词开头（"展开"/"收起"）。

**案例**：thread-fit 软降级初版的"另有 N 条可能跑题的线索"是 `<button @click="toggleAllDemoted">`，但点击效果是"展开这些 thread 的关联文章"——而展开文章对"提示跑题"这个信号**毫无意义**（用户要看文章照常点单条 thread）。评审判定"展开关联文章没有意义"，改为 `<p>` 纯状态说明，删掉批量 toggle。见 `DailyReportTopicSection.vue` `.drm-thread__hint`。

## 3. 可观测性信号的展示分层

承接 observability 系列（System 1 tag↔板块 / System 2 section↔话题 / System 3 thread↔section）的展示哲学，**正文极轻、分数只进探究区**：

- **正文层**：只用"形态"传达信号（降级样式、图标、折叠态），**不出现任何数字**。thread 标题、section 头部都不写分数。
- **探究层**（hover/展开后才可见）：才显示贴合度数值（`toFixed(2)`）+ 中文标签（"贴合"/"可能跑题"）。

**理由**：正文堆满数字会变成仪表盘，破坏阅读流；分数是给"想深究"的人看的，收进探究区不打扰默认阅读。

**案例**：thread-fit 的 `fit_distance` 数值只在 `.drm-thread__fit-probe` 里，而 probe 在 thread 展开后（`.drm-articles` 内）才渲染。thread 标题正文从不出现数字。见 `DailyReportTopicSection.vue` + `app/utils/threadFit.ts`。

## 4. 历史数据的安全降级

任何依赖"新字段"的展示逻辑，**缺字段时必须降级为正常态**，不得报错、不得误判：

- 字段 `undefined`/`null`/`NaN`/负值 → 按"无信号"处理，渲染成与正常条目一致。
- 降级（灰显/折叠/警告标记）只在**字段有效且超阈值**时触发，默认状态永远是正常。

**案例**：历史 thread 无 `fit_distance`（本 change 上线前生成的），`isThreadFitDemoted(undefined) === false`，按正常 thread 渲染。见 `app/utils/threadFit.ts`。

## 关联

- 可观测性三系列的业务设计 → [daily-report.md](../../flow/daily-report.md) §可观测性
- 验证这些交互（opencli 按需验证 / 视觉子代理验证；major UI 对照已批准 ui-design.md 基线 + 双视口）→ `.agents/skills/ui-verify/`、[ui-navigation.md](../../architecture/ui-navigation.md)、[layout.md](layout.md)
