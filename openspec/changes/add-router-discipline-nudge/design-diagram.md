# 决策图

> 事实来源：本 change 的 `proposal.md`、`design.md` D1–D4，以及 `session.rollup` 的实测成本。

## 1. 提醒与豁免的优先级

这张图回答：**一次工具调用里，提醒为什么会被触发或静音**。它不表达阈值常量的来源。

```mermaid
flowchart TD
    A["主线程 tool_call"] --> B{"本会话有豁免文件<br/>且理由非空?"}
    B -- 有 --> S["静音：不提醒<br/>（首次读到记 bypass 事件）"]
    B -- 无 --> C{"工具是 read 或 edit?"}
    C -- 否 --> S2["不介入"]
    C -- read --> D{"目标 > 200 行?"}
    D -- 是 --> R1["steer 一次：该派 explore"]
    D -- 否 --> S2
    C -- edit --> E{"同文件连续 edit ≥ 3?"}
    E -- 是 --> R2["steer 一次：该派 general-purpose"]
    E -- 否 --> S2

    style R1 fill:#fff3cd
    style R2 fill:#fff3cd
    style S fill:#d4edda
```

**图不表达的约束**：提醒绝不阻断工具执行；每会话每类信号只提醒一次。

## 2. 为什么是这两个信号

这张图回答：**为什么不判「任务该不该派」而判行为**。它不表达具体成本数字。

```mermaid
flowchart LR
    A["任务该不该派出去?"] -->|主观判断| B["不可观测<br/>每次都吵架"]
    A -->|拆成行为事实| C["读了多大文件<br/>同一文件改了几次"]
    C --> D["可观测、可计数<br/>阈值可调"]
    D --> E["提醒只在信号出现时发<br/>低噪声"]

    style B fill:#f8d7da
    style E fill:#d4edda
```

**图不表达的约束**：阈值（200 行 / 3 次）是经验值，改常量不必走 change；shell 通道（`cat`/`sed` 读大文件）绕得过提醒，这是明确接受的边界。

## 3. 豁免为什么必须带理由

这张图回答：**隐形跳过与显形跳过的差别**。它不表达文件格式。

```mermaid
stateDiagram-v2
    [*] --> 无豁免
    无豁免 --> 有豁免: --excuse "理由"
    有豁免 --> 有豁免: 提醒静音 + 理由进事实库
    有豁免 --> 无豁免: --clear
    无豁免 --> 阻断: --excuse ""（空理由）→ exit 2

    note right of 有豁免
        理由写在 .pi/tmp/router-discipline/<session>.json
        bypass 事件带 detail=理由，可事后抽查
    end note
```

**图不表达的约束**：豁免是 per-session 的，新会话自动恢复提醒——不写就用不上，也就不会变成永久静音。
