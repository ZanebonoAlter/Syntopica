# 决策图

> 事实来源：本 change 的 `proposal.md` 与 `design.md` D1–D5，以及 `events.db` 的 38:0 实测。

## 1. 为什么 marker 方案必然失效

这张图回答：**同一批写入在两种判据下的结局差异**。它不表达 `checkStageScope` 的内部正则。

```mermaid
flowchart LR
    W["agent 写<br/>ui-prototype/index.html"] --> M{"ui-design.md<br/>有 ui-stage marker?"}
    M -- 无（实测 38 次） --> P1["无法判定阶段<br/>→ 放行 + 一条 warn"]
    M -- 有（实测 0 次越域） --> P2["按 stage 判越域<br/>→ 正常拦截"]
    P1 --> R1["结果：分层规则形同虚设"]
    P2 --> R2["结果：规则生效"]

    style P1 fill:#f8d7da
    style R1 fill:#f8d7da
    style P2 fill:#d4edda
    style R2 fill:#d4edda
```

**图不表达的约束**：marker 缺失时只 warn 不 block 是 `add-ui-stage-accounting` 的观察期设计，不是 bug；错在把「观察期只记账」当成了长期方案。

## 2. 新判据：查文件而不是查申报

这张图回答：**依赖链上每个断点判什么、报什么码**。它不表达 `ui-skip` 的匹配正则。

```mermaid
stateDiagram-v2
    [*] --> 业务未定
    业务未定 --> 业务已定: design.md 有伪代码节<br/>或 N/A 带理由
    业务未定 --> 阻断1: 缺 → block ui-pseudocode-missing
    业务已定 --> 契约未定
    契约未定 --> 契约已定: ui-design.md 八节齐全
    契约未定 --> 阻断2: 缺 → block ui-design-missing
    契约已定 --> 骨架未出
    骨架未出 --> 骨架已出: ui-prototype/skeleton/ 有文件<br/>或 ui-skip: skeleton(理由)
    骨架未出 --> 阻断3: 写非 skeleton 文件 → block ui-chain-out-of-order
    骨架已出 --> 可写原型: ui-prototype/index.html 可写
    可写原型 --> 可灌数据: fixture.json 可写（要求 index.html 已在）
    可写原型 --> 阻断4: 直接写 fixture → block ui-chain-out-of-order

    style 阻断1 fill:#f8d7da
    style 阻断2 fill:#f8d7da
    style 阻断3 fill:#f8d7da
    style 阻断4 fill:#f8d7da
```

**图不表达的约束**：只有 `ui-impact: major` 走完整条链；minor 只过「契约已定」前的两步（八节检查由既有 `evaluateUiContract` 负责）。

## 3. 伪代码节探测的三种结果

这张图回答：**`readPseudocodeSection` 怎么判「有/没有/敷衍」**。它不表达 Markdown 解析细节。

```mermaid
flowchart TD
    A["design.md"] --> B{"有 ## 核心逻辑（伪代码）标题?"}
    B -- 无 --> M["missing → block"]
    B -- 有 --> C{"节内有实质内容?"}
    C -- 空 --> M
    C -- 有 --> D{"开头是 N/A?"}
    D -- 否 --> OK["ok → pass"]
    D -- 是 --> E{"理由 ≥ 4 字?"}
    E -- 否 --> NA["na-empty → block"]
    E -- 是 --> OK
```

**图不表达的约束**：「≥ 4 字」只是挡住 `N/A：` 这种空壳，不判断理由是否成立——理由成立性归 review。
