<!-- ui-impact: none -->
<!-- diagrams: full(新增一个归档门禁判定流程 + 图的产出与观看链路，两者都是分支/时序结构，纯文字说不清) -->
<!-- constraint-domains: dev -->

# 决策制品图形化：图改成默认动作，伪代码进 design.md

## Why

用户的诊断原话：

> 原型这块其实 md 我都基本不看，我最希望的是图片表达，涉及到数据库的需要 er 图，用户故事需要顺序图，复杂业务流转的需要状态图，大型 srs 的需要用例图，按架构和用户故事顺序拆解功能设计，这个就是《伪代码》沟通。

现状机制是「`design-diagram.md` 可选 + 只在判据成立时画 + 不设门禁」。三处都指向同一个结果：**没人画**。

更关键的一条是在实施本 change 时发现的：原本打算「渲染成 PNG，agent 在对话里贴出来」，但**当前 pi-web 界面的工具结果不渲染图片**——用户看不到。改用 mermaid 代码块直接进对话后确认可行（`pi-web` 自带 `mermaid ^11.16.1`，对话里的 ` ```mermaid ` 块会渲染成图）。

这说明一件事：**「图」的价值取决于它出现在用户真正看的地方**。落在 md 文件里、渲染成 PNG 存盘，都不算「看到」。

## What Changes

### 1. 判据反转：默认画，`none` 才需理由

去掉「只在判据成立时画」。改为：

- change 根有 `<!-- diagrams: full(白话理由) -->` → 必须有 `design-diagram.md` 且至少一块 mermaid
- `<!-- diagrams: none(白话理由) -->` → 理由必填，写清为什么这个 change 不需要图（「只改措辞」「无表结构变更」「无状态机」「单层调用」）
- marker 缺失 → 归档门禁红

理由的**正确性**不判（那是人和图的事），只判**有没有**。

### 2. 四类图（补 ER 图与用例图）

| 你要的 | mermaid 写法 | 现状 |
|---|---|---|
| 数据库 | `erDiagram` | **本次新增** |
| 用户故事 | `sequenceDiagram` | 已有 |
| 业务流转 | `stateDiagram-v2` | 已有 |
| 大型 SRS | `flowchart` + subgraph（左角色 / 右用例） | **本次新增**（mermaid 无原生用例图） |
| 功能路径 | `flowchart` | 已有 |

### 3. 图必须在对话里呈现

写进规范：**写完图 SHALL 在对话中贴出 mermaid 源码块**，不能只落盘。理由：用户的观看现场是对话，不是文件。落盘是为了归档可查，贴出来才是给他看。

### 4. 伪代码成为 `design.md` 的强制节

新增 `## 核心逻辑（伪代码）` 节，写明三件事：关键数据结构、主流程控制流、失败/边界分支。粒度到**函数签名 + 关键分支**为止，不抄实现。

同样按「默认要有，`none` 写理由」处理：纯文档 / 配置项 / 命名调整这类无逻辑变化的 change 写理由跳过。

### 5. 门禁从「不设」改为「查形式」

只查三件眼睛不可靠的事：

1. marker 存在且二值合法
2. `none` 有非空理由
3. `full` 时有 `design-diagram.md` 且含至少一块 ` ```mermaid `

**不查**图画得对不对、伪代码写得准不准——那归人和图。

## 不做的事

- **不做渲染脚本**：pi-web 直接渲染 mermaid，不需要 chromium → PNG → trim 那条链（实测可行但没必要）
- **不把图做成 schema 制品**：仍是 change 根的非 schema 文件，不进 DAG
- **不动 `ui-design.md` 的八节结构**：`design-diagram.md` 与 `ui-design.md` 的关系只加一句指针约定（L0 契约引用图时写指针，不复制）
- **不强制四类都画**：一个 change 画哪几类由它自己的内容决定，`diagrams: full` 只要求「有图」，不要求「四类齐」

## Impact

- **代码**：零
- **制品**：`change-diagram-convention` spec 3 条 MODIFIED + 3 条 ADDED；`开发执行规范.md` §13 重写 + §11 门禁表加一行；`doc-authoring.md` 补伪代码节约定
- **门禁**：`archive-readiness` 增加一项（marker 与图/理由一致性）
- **向后兼容**：已归档 change 不追溯（无 marker = legacy，不阻断）；只有新 change 要求 marker
- **样板**：给一个已归档 change 补 `design-diagram.md` 做样子（只增不改）

## 已知残留

- `diagrams: none` 的理由质量无法机器判。若某天发现大量「理由」是敷衍（「改动较小」），那是 review 该管的事，不是加更多门禁
- 用例图只能靠 flowchart 模拟，表达力弱于真正的 UML 用例图。这个项目单用户无 auth，用例图的实际需求低
