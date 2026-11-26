# 决策图

> 事实来源：本 change 的 `proposal.md`（marker 三态与门禁范围）+ `design.md` D3/D6 + `scripts/harness/archive-readiness.sh` 现有七项结构。

## 1. 归档门禁如何判 diagrams

这张图回答：**归档时在一个 change 上看出什么问题会判红**。它不表达 CLI 参数与错误文案（那些随实现变）。

```mermaid
flowchart TD
    A[change 归档] --> B{有 .openspec.yaml?}
    B -- 无（legacy / 已归档补档） --> OK[跳过，不判]
    B -- 有 --> C{proposal.md 头部有 diagrams marker?}
    C -- 无 --> RED1[判红：写 full 或 none 并给理由]
    C -- 有 --> D{值是 full 还是 none?}
    D -- 其他值 --> RED2[判红：二值之外不认]
    D -- none --> E{括号内理由非空?}
    E -- 为空 --> RED3[判红：none 必须写理由]
    E -- 非空 --> OK
    D -- full --> F{design-diagram.md 存在?}
    F -- 无 --> RED4[判红：full 档缺图文件]
    F -- 有 --> G{含至少一块 mermaid 代码块?}
    G -- 无 --> RED5[判红：文件里没有图]
    G -- 有 --> OK

    style OK fill:#d4edda
    style RED1 fill:#f8d7da
    style RED2 fill:#f8d7da
    style RED3 fill:#f8d7da
    style RED4 fill:#f8d7da
    style RED5 fill:#f8d7da
```

**图不表达的约束**：不判图的类型选得对不对、不判 mermaid 语法、不判 `none` 的理由是否成立。这三件归 review 与看图的人（design D3）。

## 2. 一张图从产生到被看见

这张图回答：**为什么「落盘」不等于「做完了」**。它不表达 mermaid 渲染器的内部实现。

```mermaid
sequenceDiagram
    autonumber
    participant Agent as 写 change 的人（agent）
    participant File as design-diagram.md
    participant UI as 对话界面（pi-web）
    participant Human as 用户

    Note over Agent: 改动的 change 涉及结构/分支/时序
    Agent->>Agent: 对照五类判据决定画哪几类
    Agent->>File: 写入 mermaid 源码块（配问题行 + 约束行）
    Agent->>UI: 同一回合对话里贴出同样的 mermaid 块
    UI->>UI: 渲染成图（pi-web 自带 mermaid）
    UI-->>Human: 用户当场看到图
    Human-->>Agent: 「这里分支不对」/「少了一条失败路径」
    Agent->>File: 修正
    Note over File,Human: 归档门禁只确认「文件在 + 有图块」<br/>「看得见」由这条链路保证，不是门禁能查的
```

**图不表达的约束**：渲染失败（语法错）只会在对话里暴露，门禁查不到；这条链路依赖 agent 主动贴图，规范里写成 SHALL 也仍可能被跳过——那是 review 的事。

## 3. 伪代码节的位置与本 change 的关系

这张图回答：**伪代码与图的分工**。它不表达具体字段与函数名。

```mermaid
flowchart LR
    P[proposal.md<br/>Why / What / 不做] --> D[design.md<br/>决策 + 取舍 + 伪代码]
    D --> DD[design-diagram.md<br/>结构 / 分支 / 时序]
    D --> T[tasks.md<br/>可执行清单]
    DD -.引用指针.-> L0[ui-design.md L0 契约]
    subgraph 给谁看
        DD --> H1[用户：看图做判断]
        D --> H2[实现者：看伪代码解题]
    end
```

**图不表达的约束**：`ui-design.md` 只写指针不复制图（design D5），复制会漂。
