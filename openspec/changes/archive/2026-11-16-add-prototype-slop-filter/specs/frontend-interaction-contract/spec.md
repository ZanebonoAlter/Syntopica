# frontend-interaction-contract — 原型内容底线 delta

## ADDED Requirements

### Requirement: 占位数据与伪造内容必须区分

界面与原型中的每处数据 SHALL 属于「诚实占位」或「伪造内容」两档之一，判定依据是**该值读起来像什么**而非其字符串形态：

- **诚实占位**（允许）：该值读起来像**对字段的说明**。字面白名单封闭，仅五个：`Your Name`、`email@example.com`、`[REAL DATA]`、`—`、`示例数据`；扩充白名单须先改本条；
- **伪造内容**（禁止）：该值读起来像**一个具体的真实的人 / 公司 / 事件**，如 `John Doe`、`johndoe@example.com`、`Acme Inc.`、编造的活动流。

`email@example.com` 与 `johndoe@example.com` 字面只差 local part，前者是对字段的说明，后者伪装成一个具体的人——这是本条的判定样例。

SHALL NOT 出现：无真实对照期的百分比变化（`+12% this week`）、无来源的统计数字、`lorem ipsum`。

整表或整卡全为占位时，占位标记 MUST 在表注或列头标**一次**（SHALL NOT 逐格标），且该区域 MUST 在 `## Data Provenance` 表占一行、`source` 填 `none` + `needs-approval`。**「诚实占位」只免除「必须换成真数」，不免除审批。**

#### Scenario: 模板假名被驳回
- **WHEN** 原型表格里出现 `John Doe` / `johndoe@example.com` / `Acme Inc.`
- **THEN** 判定为伪造内容，MUST 改为白名单占位或真实值

#### Scenario: 无对照期的涨幅被驳回
- **WHEN** 卡片展示 `+12% this week` 而没有真实对照期数据
- **THEN** 判定为伪造内容；MUST 删除，或改为有真实对照期与来源的数值

#### Scenario: 诚实占位被接受
- **WHEN** 原型空表单用 `Your Name` / `email@example.com`，或某处写 `[REAL DATA]`，且该区域在 provenance 表有 `none` + `needs-approval` 行
- **THEN** 判定合格，SHALL NOT 被要求填真数；用户批准 `needs-approval` 后即可进实现

### Requirement: 空态、加载、错误态必须给原因与下一步

本节与产品界面同口径；**SPA 首屏加载与路由切换以 `loading-experience.md` 为准**，本节不重复要求。

四态齐全只是及格线。每种态 MUST 同时说清「为什么」与「下一步」：

| 态 | MUST 说清 | SHALL NOT 出现 |
|---|---|---|
| 空 | 为什么空 + 填满它的那一个动作 | 「暂无数据」+ 装饰插图 |
| 加载 | 正在加载什么 | 裸 spinner（SPA 首屏/路由加载除外） |
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

原型 MUST 在绘制阶段即满足六条，SHALL NOT 留到交付前补：

| # | 项 | 下限 | 怎么核 |
|---|---|---|---|
| V1 | 文字对比度 | 正文 ≥4.5:1；大字（≥24px 或 ≥19px 粗体）≥3:1 | 取源文件色值计算，结果写进 `ui-verification/report.md` 判定条目 |
| V2 | 非文字对比 | 图形、边框、焦点环 ≥3:1 | 同上 |
| V3 | 焦点可见 | Tab 到任何可交互元素必须看得见 focus；SHALL NOT 画成 `outline: none` | 可点 HTML 看 CSS；状态图形态见载体分流 |
| V4 | 键盘可达 | 主链路 Tab / Enter / Esc 走通；画布类 MUST 回答「Esc 做什么」 | 同上 |
| V5 | 动效 | MUST 响应 `prefers-reduced-motion`；SHALL NOT 无限循环脉冲；>5s 动效 MUST 提供暂停控制 | 同上 |
| V6 | 剂量与装饰 | glass ≤2 处、glow ≤2 处、同页 radius ≤3 档（按不重复取值数计）、阴影只用于需脱离页面平面浮起的浮层；SHALL NOT 用装饰性 emoji、无含义彩色左条、无状态含义装饰点 | 数一遍即可 |

**载体分流**：画布类等采用「状态图 + 操作表」形态的原型没有可点 HTML，V3–V5 MUST 由 `interaction-table.md` 末尾一节「可访问与动效声明」承接，四行必须齐全：`焦点顺序`（Tab 依次经过的区域名）、`焦点样式`（颜色/粗细或复用 token）、`Esc 行为`、`减动降级`（`prefers-reduced-motion` 下哪些动效改为瞬时切换）。缺任一行判定 V3–V5 不完整。

**三段式裁决**：颜色取值与间距刻度以 `theming.md` 为准；可读性与可操作性下限以本节为准；token 组合达不到 V1/V2 时，原型 MUST 换用达标 token，token 本身不达标则报 `theming.md` 修订，SHALL NOT 在原型里私自调色绕过。

#### Scenario: 原型画成无焦点环
- **WHEN** 可点 HTML 原型把按钮/链接的 focus 样式写成 `outline: none` 且无替代焦点样式
- **THEN** 判定违反 V3，MUST 补可见焦点样式

#### Scenario: 画布形态缺可访问声明
- **WHEN** 画布类原型的 `interaction-table.md` 末尾没有「可访问与动效声明」四行
- **THEN** 判定 V3–V5 不完整，MUST 补齐后才可提交审批

#### Scenario: 画布动效忽略减动偏好
- **WHEN** 「减动降级」行未声明，或声明为「无降级」而无理由，而原型描述含持续脉冲动效
- **THEN** 判定违反 V5，MUST 声明降级方案或移除循环动效

#### Scenario: 装饰性元素被驳回
- **WHEN** 原型在标题旁点缀无状态含义的彩色圆点，或给每张卡片左侧加纯装饰色条
- **THEN** 判定违反 V6，MUST 移除或赋予真实状态含义
