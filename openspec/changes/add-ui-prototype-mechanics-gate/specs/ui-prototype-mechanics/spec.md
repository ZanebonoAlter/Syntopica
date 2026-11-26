## ADDED Requirements

### Requirement: 原型分类与危险操作确认检查

原型 SHALL 分两类，由 proposal 头部注释声明：

- `<!-- ui-prototype-kind: functional -->`（默认，可省略）——功能原型，进机械检查
- `<!-- ui-prototype-kind: exploratory(<理由>) -->`——视觉探索原型（three.js 首页改版、品牌/动效实验），MUST NOT 进机械检查

未声明 SHALL 按 `functional` 处理。

对 `functional` 类且声明 `ui-impact: major` 且存在 `ui-prototype/` 的 change，系统 SHALL 检查**破坏性操作是否有二次确认**。

这是唯一被机器化的项；热区、焦点、悬停、键盘保留为设计下限，由人核，MUST NOT 声称已机器检查。

检查范围 SHALL 限定 `ui-prototype/` 顶层 `*.html` 与 `skeleton/**/*.html`，MUST NOT 递归扫描其他子目录。

#### Scenario: 破坏性操作无确认

- **WHEN** 功能原型含 `<button>删除记录</button>` 且无任何确认机制
- **THEN** 检查 SHALL 失败并列出 `file:line` 与元素片段

#### Scenario: 有确认机制

- **WHEN** 同一元素带 `data-confirm`，或文件内有 `<dialog>`，或邻近有「确认」按钮
- **THEN** 检查 SHALL 通过

#### Scenario: input 提交按钮的文字通道

- **WHEN** `<input type="submit" value="删除">` 无确认
- **THEN** 检查 SHALL 识别该元素并判失败

#### Scenario: 视觉探索原型

- **WHEN** proposal 声明 `ui-prototype-kind: exploratory(...)`
- **THEN** 检查 SHALL 跳过（exit 0）

#### Scenario: 不适用档位

- **WHEN** `ui-impact` 为 none/minor 或缺失，或无 `.openspec.yaml`，或无 `ui-prototype/`
- **THEN** 检查 SHALL 以 exit 0 放行（fail-open）

#### Scenario: 裁决确定性

- **WHEN** 同一 change 连续多次执行检查
- **THEN** 裁决 SHALL 一致（判定 MUST NOT 走 `set -o pipefail` 下的 `printf | grep -q` 管道——写端会吃 SIGPIPE 导致随机误红）

### Requirement: 破坏性与确认的判定通道

破坏性识别 SHALL 用三条通道（任一命中即认定）：可点元素文本含「删除/清空/重置/移除/彻底/销毁」；元素带 `data-destructive`；或 `<input type=submit|button value="删除…">`。

确认机制 SHALL 满足任一：同一标签有 `data-confirm` 或 `onclick` 含 `confirm(`；文件内有 `<dialog>`；元素邻近 3 行内有「确认」。

#### Scenario: 通道互不替代

- **WHEN** 元素带 `data-destructive` 但遍寻不到任何确认机制
- **THEN** 检查 SHALL 判失败（属性只用于识别，不构成确认）

### Requirement: 检查挂在归档硬拦与自查两处

同一份判定 SHALL 由 `scripts/harness/ui-prototype-check.sh` 单一实现，被 `.pi/extensions/spec-gate.ts` 归档检查与 `scripts/harness/archive-readiness.sh` 共同调用。判定逻辑 MUST NOT 在扩展内二次实现。

#### Scenario: 归档被拦

- **WHEN** 检查失败且作者执行归档
- **THEN** `spec-gate` SHALL 阻断并列出失败项

### Requirement: 显式豁免必须带理由

原型 MAY 通过 `<html>` 或 `<body>` 上的 `data-ui-mechanics-skip="<理由>"` 豁免。理由为空 MUST NOT 生效；写在子元素上 MUST NOT 生效。

#### Scenario: 根元素带理由

- **WHEN** `<html data-ui-mechanics-skip="视觉实验页，无真实数据">`
- **THEN** 检查 SHALL 跳过并回显理由

#### Scenario: 子元素标记无效

- **WHEN** `data-ui-mechanics-skip` 写在 `<div>` 等子元素上
- **THEN** 豁免 MUST NOT 生效

### Requirement: 规范写明两类原型与判定边界

`docs/reference/standard/frontend/prototype-content.md` SHALL 含「原型类别与可操作性下限」节，写明：

- 两类原型的特征与机械检查适用性
- 设计下限（热区 24×24 / 焦点 / 悬停 / 键盘）**人核**
- 危险操作确认是唯一机器检查项，及其双通道判据

#### Scenario: 类别可查

- **WHEN** 设计者不确定自己的原型该不该被查
- **THEN** SHALL 能从规范查到类别定义与声明方式

#### Scenario: 不假装检查了没查的

- **WHEN** 查阅规范关于热区的规定
- **THEN** SHALL 明确写着「人核，不做机器检查」及原因
