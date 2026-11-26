# Design

## D1 为什么放弃 marker 改判文件

`add-ui-stage-accounting` 的设计是「agent 在 `ui-design.md` 写 `<!-- ui-stage: l0|skeleton|interaction|data|approved -->`，门禁按 stage 判越域」，观察期内只记账。

实测结果否定了这个前提：

| 事件 | 次数 |
|---|---:|
| `ui-stage-missing` | 38 |
| `ui-stage-scope-out` | 0 |

38 次写入里没有一次申报阶段，所以越域判定**从未执行**。这不是"观察期数据不够"，是**手段选错了**：让执行者自己申报约束自己的状态，它只要不申报就自由了。

新判据全部基于**磁盘事实**：文件在不在。agent 无法通过"不申报"绕过——它想让 `index.html` 存在，就得先让 `skeleton/` 存在。

## D2 依赖链的每一环为什么是这一环

| 环 | 判据 | 为什么是它 |
|---|---|---|
| L-1 伪代码 | `design.md` 有 `## 核心逻辑（伪代码）` | 业务口径的唯一载体。原型改不动它，所以业务问题必须回到这里 |
| L0 八节 | `ui-design.md` 八节齐全 | 既有 `evaluateUiContract` 已保证（pass 前提），依赖链**不重复查** |
| L1 骨架 | `ui-prototype/skeleton/` 至少 1 文件 | 最便宜的一步（空壳），成本低到没有拒绝理由；跳过它意味着直接进高成本步骤 |
| L2 原型 | `ui-prototype/index.html` 存在 | 写 `fixture.json` 之前必须先有能跑的原型，否则数据注入无宿主 |
| L3 数据 | `fixture.json` | 最后一环，不设后继检查 |

**为什么 L0 不在依赖链里重复查**：`evaluateUiContract` 的 major 分支已经 `missingSections(uiDesignText, "major")` 查八节，缺则 block。重复查会产生两个码描述同一件事，反而让错误信息混乱。

## D3 检查点放在 approved 而不是 pending

`ui-approval` 从 pending 变 approved 是「界面契约定了」的时刻。

若在 pending 期就要求伪代码，会阻碍正常的探索路径：原型做着做着发现业务问题，本来就该改图，这时逼它先有伪代码是反的。

若在 approved 之后才查，业务错误已经在批准时被确认了，检查失去意义。

所以在 **approved 那一刻**查：此时业务口径必须已经存在。这和"用户批的是界面契约"不冲突——批的是界面，但界面背后的业务逻辑得先站得住。

## D4 只对 major 强制

`ui-impact` 三档的语义：

| 档 | 依赖链 | 理由 |
|---|---|---|
| none | 不查 | 无界面 |
| minor | 不查 | 只改一个按钮位置，逼它出骨架是浪费 |
| major | 全链 | 全流程最重的一档，骨架与数据的顺序有价值 |

这条与 `ui-impact` 的既有语义一致：三档本来就是对"这个改动有多重"的判断。

## D5 出口设计：`ui-skip` 为什么不破坏约束

`<!-- ui-skip: skeleton(纯样式微调，无新增区块) -->` 让某些 change 跳过骨架环。

会不会变成万能逃生口？两条防线：

1. **理由必须非空**（实测：`ui-skip: skeleton()` 不豁免）
2. **它是一种显式声明**，会出现在 `ui-design.md` 里，review 时能看见

对比原来的 marker 方案：marker 可以**不写**（隐形逃逸），`ui-skip` 必须**写出来**（显式逃逸）。显式的逃逸能被审计，隐形的不能。

## D6 为什么这次敢用 block

`add-ui-stage-accounting` 说「观察期数据达标后才转阻断」，而这一轮直接上 block。理由是判据换了：

- 旧判据（marker 越域）的数据是 0 次命中——没法据此判断误伤率
- 新判据（文件存在性）是**确定性事实**，不存在"判不准"的情况
- 且已有 `ui-skip` 出口兜底

风险确认为零误伤：仓库里唯一的 major 档 active change（`add-home-pipeline-status-page`）**没有 `.openspec.yaml`**，走 legacy 分支不进入依赖链。

## 核心逻辑（伪代码）

### 关键数据结构

```
UiContractInput = {
  proposalText:    string | null   // 取 ui-impact
  uiDesignText:    string | null   // 八节 + 可选 ui-skip
  designMd:        string | null   // 伪代码节
  changeDirFiles:  string[]        // 磁盘快照（写入前）
}

PseudocodeProbe = 'ok' | 'na-empty' | 'missing'
ChainVerdict    = null | { reasonCode, detail }   // null = 允许
```

### 主流程

```
decideUiToolGate(input):
    if stageFamily != implementation or boundChange == null: allow
    verdict = evaluateUiContract(input)

    if verdict.kind == legacy: 按旧逻辑（front/ 写入 warn-once，其余 allow）
    if verdict.kind == block:  按工具动作分流（other allow / mutation 在 change 内 allow / 否则 block）
    if verdict.kind == pass:
        if tool.kind == mutation and targetPath 在 change 内:
            rel   = 去掉 openspec/changes/<chg>/ 前缀
            chain = checkPrototypeChain(input, rel)
            if chain != null: block(ui-chain-out-of-order, chain)
        allow

evaluateUiContract(input):        # 既有逻辑 + 一处新增
    ... none / minor 分支不变 ...
    ... major 八节 + 原型路径校验不变 ...
    if approval == pending: block(ui-approval-pending)     # 既有，排在新检查之前
    if approval == approved:                                # ← 新增
        probe = readPseudocodeSection(input.designMd)
        if probe == missing:   block(ui-pseudocode-missing)
        if probe == na-empty:  block(ui-pseudocode-missing)
    pass

checkPrototypeChain(input, rel):
    if parseUiImpact(proposalText) != major: return null        # minor/none 不查
    if rel 不在 ui-prototype/ 下:            return null        # 非原型路径不查

    if rel 不是 skeleton/ 开头 and not hasSkipMarker(uiDesign, 'skeleton'):
        if changeDirFiles 里没有任何 skeleton/ 文件:
            return "缺 L1 骨架……"

    if rel == ui-prototype/fixture.json and not hasSkipMarker(uiDesign, 'data'):
        if changeDirFiles 里没有 ui-prototype/index.html:
            return "缺 L2 可点原型……"

    return null

readPseudocodeSection(designMd):
    if designMd == null:                     return missing
    定位 '## 核心逻辑（伪代码）'，截到下一个 '## '
    if 节内容为空:                           return missing
    if 节以 N/A 开头: 理由 ≥ 4 字 ? ok : na-empty
    return ok
```

### 失败与边界分支

- **`design.md` 不存在**：`readPseudocodeSection(null)` → `missing` → block。major approved 的 change 必须能解释自己的逻辑
- **`N/A：` 空理由**：判 `na-empty`。4 字下限只挡空壳（`N/A：` 后什么都没有），不判理由质量
- **`ui-skip` 理由为空**：`hasSkipMarker` 返回 false → 不豁免。实测 `<!-- ui-skip: skeleton() -->` 仍被 block
- **写 `fixture.json` 但连骨架都没有**：报「缺 L1 骨架」而不是「缺 L2」——检查顺序从前往后，先报最前面的缺口
- **`changeDirFiles` 是写入前快照**：因此写 `skeleton/a.html` 本身不需要 `skeleton/` 已存在（否则第一块骨架永远写不进去）
- **contract 已 block 时**：不进依赖链。既有设计「修当前 change 目录内的文件」优先放行——原型越域不该盖过"合同还没定"这个更前置的问题
