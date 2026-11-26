# 判定流程

> 事实来源：本 change 的 `proposal.md`「What Changes」、`design.md` D1–D4，以及
> `docs/reference/standard/frontend/prototype-content.md` 的载体分流条款。

## 1. 一次检查会走哪条路

这张图回答：**什么情况下放行、什么情况下会红**。它不表达正则细节。

```mermaid
flowchart TD
    A["归档 change"] --> B{"有 .openspec.yaml?"}
    B -- 否 --> Z0["放行（legacy）"]
    B -- 是 --> C{"ui-impact: major?"}
    C -- 否 --> Z1["放行（none/minor 不查）"]
    C -- 是 --> D{"ui-prototype-kind:<br/>exploratory?"}
    D -- 是 --> Z2["放行（视觉探索原型不查）"]
    D -- 否 --> E{"有 ui-prototype/ ?"}
    E -- 否 --> Z3["放行（无原型可查）"]
    E -- 是 --> F{"根元素带<br/>skip 且有理由?"}
    F -- 是 --> Z4["放行 + 回显理由"]
    F -- 否 --> G["只扫顶层 *.html<br/>与 skeleton/**"]
    G --> H{"有破坏性操作?"}
    H -- 没有 --> P["绿"]
    H -- 有 --> I{"该元素或其邻近<br/>有确认机制?"}
    I -- 有 --> P
    I -- 没有 --> R["红：列出 file:line<br/>+ 元素片段"]

    style Z0 fill:#e7f5ff
    style Z1 fill:#e7f5ff
    style Z2 fill:#e7f5ff
    style Z3 fill:#e7f5ff
    style Z4 fill:#e7f5ff
    style R fill:#f8d7da
    style P fill:#d4edda
```

**图不表达的约束**：热区/焦点/悬停**不在**这条流程里——它们是设计下限，人核，不做机器检查。

## 2. 为什么只机器化危险操作确认

这张图回答：**四个候选项各自能不能被可信地静态判定**。它不表达具体正则。

```mermaid
flowchart LR
    A["候选项"] --> B["热区尺寸"]
    A --> C["焦点 / 悬停"]
    A --> D["危险操作确认"]
    B --> B1["静态只能看「有没有声明」<br/>一处 padding 全过<br/>→ 形式主义，撤回"]
    C --> C1["静态只能看「字样在不在」<br/>outline:none 也过关<br/>→ 形式主义，撤回"]
    D --> D1["查「有没有确认机制」<br/>不依赖 CSS 解析<br/>→ 可判，保留"]

    style B1 fill:#f8d7da
    style C1 fill:#f8d7da
    style D1 fill:#d4edda
```

**图不表达的约束**：撤回的三项在规范里保留为**人核的设计下限**（热区 24×24 等），并明写「不做机器检查」——不假装查了没查的。

## 3. 门禁挂在两处而不是一处

这张图回答：**为什么同一份判定要跑两次**。它不表达脚本内部结构。

```mermaid
flowchart LR
    subgraph 自查
        A1["archive-readiness.sh<br/>第⑨项"] --> A2["一次看清全部缺口"]
    end
    subgraph 硬拦
        B1["spec-gate.ts<br/>检查⑨"] --> B2["真的阻断归档"]
    end
    A2 -.同一脚本.-> C["ui-prototype-check.sh"]
    B2 -.同一脚本.-> C

    style B2 fill:#d4edda
```

**图不表达的约束**：ISS-01 踩过——门禁只在自查脚本里等于没有（`archive-readiness`
是给作者自己看的，真正的阻断点在 `spec-gate`）。两处都挂、判定逻辑只在脚本里一份。
