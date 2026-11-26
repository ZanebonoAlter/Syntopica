<!-- ui-impact: none -->
<!-- constraint-domains: semantic-board -->

# 修正两个巨型 spec 的过期条款

## Why

`workshop-board-console`（1120 行 / 33 req）与 `lane-correction-surface`（728 行 / 19 req）是 spec 收敛后剩下的两个巨型。原本打算拆分它们，但审计先发现更严重的问题：**条款会把人带偏**。

审计（两份报告见本目录 `audit-board-console.md` / `audit-lane-correction.md`）结论：

| spec | 有效 | 过期 | 缺写 |
|---|---:|---:|---:|
| workshop-board-console | 26 | 6 | 8 |
| lane-correction-surface | 15 | 4 | 5 |

最危险的两条实测（不是推测）：

1. **`lane-correction-surface:324` 写「40+4=44px 量级」行距，实际代码是 52px**。`MEMBER_BASE_RADIUS` 是 7.5（spec 写 5.5），`LABEL_BOX_PAD=4`，`NODE_BLOCK_H=48`。**44 小于实际占高 48——照 spec 改正好复现 spec 自己在第 312 行警告的「标签框底与下一圆顶相切」缺陷**
2. **三栏宽度只写了宽档 380/300**，代码在 `≤1279.98px` 有 `@media` 覆盖成 240/280。窄档规则 11-02 就进代码，380/300 是 11-17 才定稿——spec 记的一直只有后者

还有一条是上一轮 `merge-lane-ops-specs` 自己造成的：并入归档视图后，`workshop-board-console` R14 仍写「两个同级视图」，而代码已是三个（`'matrix'|'archive'|'events'`），与同文件 R26 自相矛盾。

**为什么先修过期而不是先拆分**：拆一份写着错数值的 spec，只是把错误分散到两个文件。而且拆分是结构手术、修过期是内容修正，两者混在一个 change 里会让守恒验证失效（分不清「搬过去」和「改过」）。

## What Changes

### 必须修（严重 / 中等）

| # | spec | 条款 | 现状 | 修正为 |
|---|---|---|---|---|
| 1 | lane-correction-surface | R14 行距与节点几何 | 5.5 / 34.5 / 38.5 / 44 | 7.5 / `NODE_BLOCK_H=48` / 52，并注明 `LABEL_BOX_PAD` |
| 2 | lane-correction-surface | R1 三栏宽度 | 只写 380/300 | 补窄档 240/280（`≤1279.98px`）与断点时序 |
| 3 | workshop-board-console | R14 视图数量 | 两个同级视图 | 三个（矩阵 / 归档 / 关键事件），与 R26 对齐 |
| 4 | workshop-board-console | R2 图例口径 | 颜色 = 行内相对强度 | 图例实际只有「当日线索数（不含综合稿）· — 无分区 · ? 数据不可用」三态，与 R11「仅一条图例」一致 |
| 5 | workshop-board-console | R16 时间轴步长 | 24px **固定** | `min(24, 可用宽/(节点数-1))` 钳制 |
| 6 | lane-correction-surface | 左栏块数 | 四块（5 处，含 `:161` 验收条款） | 五块（补辅助标签约束块） |

### 顺带修（轻微，同一段改一遍不增加成本）

| # | spec | 条款 | 修正 |
|---|---|---|---|
| 7 | workshop-board-console | R30 排序档 | 删掉不存在的「id 倒序」档（代码只有质量 / 时间） |
| 8 | workshop-board-console | R13 抽屉形态 | 「治理抽屉」→「单级宽工作面」，顶栏入口名「上下文」 |
| 9 | workshop-board-console | R26 归档入口位置 | 入口在编年头、导航块在工具行**上方** |
| 10 | lane-correction-surface | R? 首义来源 | 不是一条三态字段，是 `source: auto\|manual` + `anchor_origin.resolved` 布尔 |

### 缺写不在本 change 范围

审计另发现 13 条「代码有但 spec 没写」（`?view=` URL 键、行头就地改名、修正侧 365 日曲线、`↩加回` 第三写入口、质心池含更早成员、块内生效徽标等）。**本 change 不补**——补缺写是新增契约（ADDED），与「让既有条款追上代码」（MODIFIED）是两件事，混在一起会让守恒验证失效。已登记在 tasks 3.3，另开 change。

### 不做的事

- **不拆分**这两个 spec。拆分的分界（关键事件 10 条 / 归档 8 条 / 画布几何 vs 工作面结构）已在审计报告里给出，等过期修完另开 change——那时守恒验证才干净
- **不动 7 条跨族重复条款**（`高度贴合容器` / `横向吸顶标签列` / `缩放滚动外壳` / `窄屏门限` / `批内相对归一` 与 `workshop-lane-compose` 逐字重复、`首义读端点零写副作用` 与 `lane-ops` 重复）——归「跨族引用清扫」批次
- **不删死代码** `BoardLaneEvolution.vue:215-217`（带「最密集的三天」文案，与 R4 的 MUST NOT 相悖）。代码删除要单独 change，本 change 只在 spec 里记一条说明
- **不改任何代码**：本 change 只让 spec 追上代码，不反向要求代码改

## Impact

- **spec 数量**：146 不变（两个 spec 都保留，改的是条款内容）
- **代码**：零改动
- **风险**：低。所有修正都有 `file:line` 证据；改错的话 `git revert` 干净
- **可验证性**：每条 MODIFIED Requirement 都能对回审计报告的证据行

## 已知残留

- 本 change 不解决「spec 描述人工判断类内容」的根本问题（如「治理信号常驻右轨」这种形态描述，没有像素级锚点可验）。这类条款的防过期机制应靠 flow 文档与代码注释，不靠 spec 复述
- `lane-correction-surface` 的 Purpose 仍是 `TBD - created by archiving change ...`，本 change 一并补上
