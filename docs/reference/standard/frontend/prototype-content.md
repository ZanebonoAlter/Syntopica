# 原型内容质量（Prototype Content）

<!--
doc-impact-applies: front/app/features/, front/app/pages/, front/app/components/ui/, front/app/components/dialog/, front/app/components/common/, front/app/composables/ | section=Requirements
-->

> **权威源**：本文件是「原型里放什么数据、状态文案写什么、视觉与动效的下限在哪」的唯一权威。
> 互补：[interaction.md](interaction.md)（交互模式选择判据 / 界面文字职责 / 原型目录预算）、[theming.md](theming.md)（颜色与间距取值）、[layout.md](layout.md)（布局模式与弹窗尺寸档）、[loading-experience.md](loading-experience.md)（SPA 首屏与路由加载）。
> 来源 change：`add-prototype-slop-filter`。本文件由该 change 从 `interaction.md` 拆出，以保住 `interaction.md` 的 ≤180 行预算。

## 为什么需要这一层

`interaction.md` 管的是「用什么形态交互、界面上允许多少字」。但原型还有一类缺陷它管不到：**填进去的东西是假的**。

实测证据：`add-home-pipeline-status-page` 的原型里 `fake` 命中 163 处、`示例` 249 处。现有 `Data Provenance` 只堵住「字段没有来源」，没堵「来源有了、数字是编的」——而编造的"真实感"数据比视觉缺陷更危险，因为它让评审**误以为该设计已有数据支撑**，从而批准一个后端根本喂不出的界面。

## Requirements

### Requirement: 占位数据与伪造内容必须区分

界面与原型中的每处数据 SHALL 属于下列两档之一：

| 档 | 判定 | 判定依据 |
|---|---|---|
| **诚实占位** | 允许 | 该值读起来像**对字段的说明**（`Your Name`、`email@example.com`、`[REAL DATA]`、`—`、`示例数据`） |
| **伪造内容** | 禁止 | 该值读起来像**一个具体的真实的人 / 公司 / 事件**（`John Doe`、`johndoe@example.com`、`Acme Inc.`、编造的活动流） |

**判定依据是「读起来像什么」，不是「字符串长什么样」**：`email@example.com` 与 `johndoe@example.com` 字面只差 local part，但前者是对字段的说明、后者伪装成一个具体的人。

诚实占位的字面白名单是**封闭**的，只有五个：`Your Name`、`email@example.com`、`[REAL DATA]`、`—`、`示例数据`。需要别的写法时先扩充本白名单，SHALL NOT 临时发明。

此外 SHALL NOT 出现：无真实对照期的百分比变化（`+12% this week`）、无来源的统计数字、`lorem ipsum`。

**标注载体与剂量**：整表或整卡全为占位时，MUST 在**表注或列头标一次**（如列头写字段名 + 表注写「示例数据」），SHALL NOT 逐格标。该区域 MUST 同时在本 change 的 `## Data Provenance` 表占一行，`source` 填 `none` + `needs-approval`——**「诚实占位」只免除「必须换成真数」，不免除审批**。

#### Scenario: 模板假名被驳回
- **WHEN** 原型表格里出现 `John Doe` / `johndoe@example.com` / `Acme Inc.`
- **THEN** 判定为伪造内容，MUST 改为白名单占位或真实值

#### Scenario: 无对照期的涨幅被驳回
- **WHEN** 卡片展示 `+12% this week` 而没有任何真实对照期数据
- **THEN** 判定为伪造内容；MUST 删除，或改为有真实对照期与来源的数值

#### Scenario: 诚实占位被接受
- **WHEN** 原型空表单用 `Your Name` / `email@example.com`，或某处写 `[REAL DATA]`，且该区域在 provenance 表有 `none` + `needs-approval` 行
- **THEN** 判定合格：SHALL NOT 被要求填真数，等用户批准 `needs-approval` 后即可进实现

### Requirement: 空态、加载、错误态必须给原因与下一步

**适用范围**：原型与产品界面同口径。**例外**：SPA 首屏加载与路由切换以 [loading-experience.md](loading-experience.md) 为准（该文件已是那一段的唯一权威，本节不重复要求）。

四态齐全只是及格线。每种态 MUST 同时说清「为什么」与「下一步」：

| 态 | MUST 说清 | SHALL NOT 出现 |
|---|---|---|
| 空 | 为什么空 + 填满它的那一个动作 | 「暂无数据」+ 装饰插图 |
| 加载 | 正在加载什么 | 裸 spinner（SPA 首屏/路由加载除外，见上） |
| 错误 | 什么失败 + 用户能做的下一步 | 「出错了」「系统异常」 |
| 无权限 | 权限缺失本身 | 与「空」「首次」共用同一句文案 |

「首次使用」「筛选后为空」「无权限」SHALL 视为三种不同的空态，MUST NOT 共用同一套文案与同一张图。

#### Scenario: 无名空态被驳回
- **WHEN** 原型某列表的空态只写「No data available」或「暂无数据」
- **THEN** 判定不合格，MUST 补「为什么空」与「填满它的动作」

#### Scenario: 三种空态不得共用
- **WHEN** 首次、筛选为空、无权限三个场景复用同一空态文案
- **THEN** 判定不合格，MUST 拆成三套

### Requirement: 视觉与动效的最低否决项

原型 MUST 在**绘制阶段**即满足下列六条，SHALL NOT 留到交付前补——这些决定在原型里画错，实现就照抄：

| # | 项 | 下限 | 怎么核 |
|---|---|---|---|
| V1 | 文字对比度 | 正文 ≥4.5:1；大字（≥24px 或 ≥19px 粗体）≥3:1 | 取原型源文件里的色值计算；结果写进 `ui-verification/report.md` 判定条目 |
| V2 | 非文字对比 | 图形、边框、焦点环 ≥3:1 | 同上 |
| V3 | 焦点可见 | Tab 到任何可交互元素必须看得见 focus；SHALL NOT 画成 `outline: none` | 可点 HTML：直接看 CSS；状态图形态：见下方载体分流 |
| V4 | 键盘可达 | 主链路 Tab / Enter / Esc 走通；画布类 MUST 回答「Esc 做什么」 | 同上 |
| V5 | 动效 | MUST 响应 `prefers-reduced-motion`；SHALL NOT 无限循环脉冲；>5s 动效 MUST 提供暂停控制 | 同上 |
| V6 | 剂量与装饰 | glass ≤2 处、glow ≤2 处、同页 radius ≤3 档（按出现的不重复取值数计）、阴影只用于需要脱离页面平面浮起的浮层；SHALL NOT 用装饰性 emoji、无含义彩色左条、无状态含义的装饰点 | 数一遍即可 |

**载体分流（V3/V4/V5）**：可点 HTML 原型直接核 CSS 与 DOM。**画布类等采用「状态图 + 操作表」形态的原型没有可点 HTML**，此时 V3–V5 MUST 在 `interaction-table.md` 末尾以一节「可访问与动效声明」承接，四行必须齐全：

| 声明项 | 要写什么 |
|---|---|
| 焦点顺序 | Tab 依次经过哪几个区域（用区域名） |
| 焦点样式 | 焦点环的颜色/粗细，或「复用全局 token `--color-accent`」 |
| Esc 行为 | 按 Esc 发生什么（取消当前拖动 / 关闭节点卡 / 无动作及理由） |
| 减动降级 | `prefers-reduced-motion` 下哪些动效被替换为瞬时切换 |

缺任一行的，判定 V3–V5 不完整。

**三段式裁决**：颜色取值与间距刻度 → [theming.md](theming.md)；可读性与可操作性下限 → 本节 V1–V6 + EC-1–EC-4；**发现 token 组合达不到 V1/V2 时，原型 MUST 换用达标 token；token 本身不达标则报 `theming.md` 修订，SHALL NOT 在原型里私自调色绕过。**

> `—` 作为数据占位符属「诚实占位」档，不受任何文案禁令约束。

### Requirement: 原型类别与可操作性下限

原型分**两类**，规则与检查口径不同——混为一谈会让视觉探索被功能开发的规则误伤：

| 类别 | 特征 | 产物形态 | 机械检查 |
|---|---|---|---|
| **功能原型** | 状态/交互/组件，用于驱动功能开发 | `index.html` + `skeleton/` + `fixture.json` | 适用（见下方 WC） |
| **视觉探索原型** | three.js 首页改版、品牌/动效实验 | 多版本实验页、渲染截图、导出资源，产物量天然巨大（现存某 change 顶层 20 个平级 HTML、整目录 504 文件/98M） | **不适用**——状态不闭环、多版本并存是常态 |

类别在 proposal 头部声明：

```html
<!-- ui-prototype-kind: functional -->                    默认值，可省略
<!-- ui-prototype-kind: exploratory(理由，大白话) -->      视觉探索，跳过原型机械检查
```

**未声明按 functional 处理**（失败方向安全：功能原型该被查）。

#### 设计下限（人核）

下列四条是**设计下限**，由设计者与 review 人眼核，**不做机器检查**——静态扫 CSS 只能看「字样在不在」，实测连 `outline: none` 都能过关、而用浏览器默认焦点环的反被判红，属于形式主义：

| 项 | 下限 |
|---|---|
| 热区 | 可点元素 ≥ **24×24**（WCAG 2.2 AA；桌面鼠标场景，不照搬移动端 44px） |
| 焦点 | Tab 到任何可交互元素看得见 focus（同 V3） |
| 悬停 | 可点元素有 hover 反馈（画布类按载体分流豁免） |
| 键盘 | 主链路 Tab / Enter / Esc 通（同 V4） |

#### WC 危险操作确认（唯一机器检查项）

**破坏性操作 MUST 有二次确认。** 这是唯一被机器化的项，理由：它不依赖 CSS 解析，是「有没有这个机制」的事实判断。

- 机器判据：`scripts/harness/ui-prototype-check.sh`（`spec-gate` 检查⑨ + archive-readiness ⑨）
- 破坏性识别（双通道，任一命中）：可点元素文本含 删除/清空/重置/移除/彻底/销毁；或带 `data-destructive`；或 `<input type=submit|button value="删除…">`
- 确认机制（满足任一）：同一标签有 `data-confirm` 或 `onclick` 含 `confirm(`；文件内有 `<dialog>`；元素邻近 3 行内有「确认」
- 豁免：`<html>` 或 `<body>` 上 `data-ui-mechanics-skip="<理由>"`——**理由非空才生效**，子元素上的标记无效（否则等于一键全免）

#### Scenario: 原型画成无焦点环
- **WHEN** 可点 HTML 原型把按钮/链接的 focus 样式写成 `outline: none` 且无替代焦点样式
- **THEN** 判定违反 V3，MUST 补可见焦点样式

#### Scenario: 画布形态缺可访问声明
- **WHEN** 画布类原型的 `interaction-table.md` 末尾没有「可访问与动效声明」四行
- **THEN** 判定 V3–V5 不完整，MUST 补齐后才可提交审批

#### Scenario: 画布动效忽略减动偏好
- **WHEN** 「减动降级」行未声明，或声明内容为「无降级」而无理由，而原型描述含持续脉冲动效
- **THEN** 判定违反 V5，MUST 声明降级方案或移除循环动效

#### Scenario: 装饰性元素被驳回
- **WHEN** 原型在标题旁点缀无状态含义的彩色圆点，或给每张卡片左侧加纯装饰色条
- **THEN** 判定违反 V6，MUST 移除或赋予真实状态含义

## 与 interaction.md 的边界

`interaction.md` 决定**用什么形态**（7 个交互模式 + 判据 P1–P5）、**界面允许写多少字**、**原型目录受什么体积预算约束**。本文件决定**填进去的内容真不真**、**状态怎么说话**、**视觉与动效的底线在哪**。一个原型可以「形态选对了、字数没超、但数据全是编的」——那就是本文件要拦的。
