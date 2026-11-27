<!-- ui-impact: none -->
<!-- diagrams: full(重排前后是两种文档结构，纯文字描述"分了几族、每族怎么分层"读三遍也对不上；画出来一眼看出信息有没有丢) -->
<!-- constraint-domains: semantic-board -->

# 链路设计按功能族重排：从「按 change 切片」到「按能力组织」

## Why

`docs/reference/flow/semantic-board.md` 646 行，链路设计占 **448 行（69%）**，里面是 **17 个按 change 追加的块**，标题直接带 change 名：

```
### 辅助标签入库（L1/L2/L3）
### SemanticBoard 匹配（add-composite-labels 后五级优先）
### 组合标签（composite label，add-composite-labels）
### 升级建议生命周期（split-board-upgrade-directions 四格矩阵）   ← 90 行
### 泳道动态（board-lane-dynamics，取代已退役的话题态势版图）
### 版块态势页左右轨（console-rails-live-signals：…）
### 关键事件视图（add-key-event-matrix-view：…）
### 泳道工作面（unify-lane-workbench：…）
### 版块体检（add-board-health-checkup，…）
### 版块体检三病分诊与共享描述修复（board-health-triage-advice）
### Event 标签延迟 embedding                                      ← 9 行
…
```

三个具体毛病：

1. **按 change 追加而非按功能族** → 没有入口。想查「标签匹配现在什么规矩」，要在 `辅助标签`、`组合标签`、`SemanticBoard 匹配`、`Event 延迟 embedding`、`标签合并预览` 五块里各翻一段
2. **信息密度极不均** → 90 行的 `升级建议生命周期` 与 9 行的 `Event 标签延迟 embedding` 并排
3. **当前规则与历史决策混在一起** → 读完分不清哪条还有效（`泳道动态` 的标题里就写着「取代已退役的话题态势版图」）

### 台账里有一处判断需要更正

台账（ISS-14）说「链路设计装的是 ADR——为什么这么设计 + 踩过什么坑，spec 里永远没有」。**实际抽样看，17 块里大部分是「当前规则描述」**，历史决策只占少数。

所以这不是「ADR 被埋在文档里」，而是**规则按 change 切片存放**。这改变了做法：不是「把 ADR 挖出来」，而是**先把规则按族归位，再把散落的历史决策降到同族的决策记录层**。

## What Changes

### 1. 链路设计按 6 个功能族重组（与 spec 族对齐）

| 族 | 对应 spec | 合并哪些块 |
|---|---|---|
| **一、标签体系** | `tagging` | 辅助标签入库 / 组合标签 / Event 延迟 embedding / 标签合并预览 / watched tags（**5 块 → 1 族**） |
| **二、版块匹配** | `tag-matching` | SemanticBoard 匹配（五级优先） |
| **三、升级建议** | `board-upgrade` | 升级建议生命周期（90 行，拆现状 + 决策） |
| **四、泳道与工作面** | `lane-ops` + `workshop-board-console` | 泳道动态 / 左右轨 / 关键事件视图 / 泳道工作面 |
| **五、版块体检** | `board-diagnostics` + `board-research` | 体检 / 三病分诊 |
| **六、版块治理** | `board-api` + `lane-correction-surface` | 管理回填 / 来源构成 / 治理面板 / 相关事件 |

### 2. 每族内上下分层

```
### 一、标签体系

#### 当前规则                     ← 现在什么规矩（必读）
（合并后的规则描述 + 一张 mermaid 流程图）

#### 决策记录                     ← 规矩怎么来的、踩过什么坑（考古时看）
- 2026-xx-xx add-composite-labels：为什么是五级优先而不是三级
- …
```

查规则只看上半，考古看下半。

### 3. 红线节原样搬

`## 业务约束与不变量` **48 行装 39 条红线，是全文最干净的部分**——节名不能改（`constraint-injection` 按节名硬编码抓红线句），内容与编号顺序原样保留，一个字不动。

### 4. 变更溯源表保留

76 行的 change → 变化索引有检索价值，不动。

## 不做的事

- **不删任何内容**：448 行的信息完整搬进新结构，只调整位置和层次。搬迁前后做非空白行数与关键术语对账
- **不动别的 flow 文档**：这次只做 semantic-board（你选的「先只改一个」）。daily-report（322 行）、data-enrichment（254 行）等验证方法后再论
- **不改任何代码**：纯文档重排
- **不加新的规范条款**：只重组已有内容，不借机写新规矩
- **不合并到 spec**：链路设计里的族是「阅读入口」，不是 spec 的别名——spec 是契约，flow 是地图

## Impact

- **改动**：`docs/reference/flow/semantic-board.md`（链路设计节 448 行重排；其余四节基本不动）
- **零代码 / 零前端 / 零后端**
- **链接安全**：`grep -rn 'semantic-board\.md#'` 零命中——**没有跨文档锚点引用**，重排不会断链
- **风险**：448 行搬迁过程中丢内容。用「搬迁前备份 + 非空白行数对账 + 关键术语 grep 对账 + 人工读一遍」四道防线

## 已知残留

- **daily-report 等 9 个 flow 文档仍是按 change 堆的**：本轮只治最胖的一个，验证方法后再推广
- **族名与 spec 名不完全一致**（如「泳道与工作面」对应两个 spec）：族是阅读单元，spec 是契约单元，允许 1:N
- **`升级建议生命周期` 的 90 行**可能要拆得更细（四格矩阵 / 冷却期 / 幂等），具体在实现时看内容定
