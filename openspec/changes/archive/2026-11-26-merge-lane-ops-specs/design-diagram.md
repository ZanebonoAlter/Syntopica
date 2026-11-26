# 决策图（补档样板）

> 补档说明：本文件按 §12「只增不改」范畴补于 2026-11-26（change `add-diagram-and-pseudocode-layer`），
> proposal / design / specs / tasks 保持归档时原样。事实来源：本 change 的 `proposal.md` 与 `design.md` D1–D7。

## 1. spec 族的层级结构

这张图回答：**一个族级 capability 里面装的是什么**。它不表达字段类型与 naming 约定。

```mermaid
erDiagram
    SPEC_FAMILY ||--|{ CAPABILITY : "吸收 1..n 个旧 capability"
    CAPABILITY ||--|{ REQUIREMENT : "含"
    REQUIREMENT ||--|{ SCENARIO : "含"
    REQUIREMENT ||--o{ FROM_NOTE : "带 from 溯源注"
    SPEC_FAMILY ||--|| DOMAIN_MARKER : "标 1..3 个业务域"

    SPEC_FAMILY {
        string name "族名 = 目标 capability 名"
        int    total_lines
    }
    CAPABILITY {
        string name
    }
    REQUIREMENT {
        string title "逐字搬运，零语义变更"
        string kind "business / impl"
    }
    SCENARIO {
        string when
        string then
    }
    FROM_NOTE {
        string source_change
    }
    DOMAIN_MARKER {
        string domains "按改这族必读哪些红线取舍"
    }
```

**图不表达的约束**：Requirement 条数与 Scenario 条数必须守恒（ADDED = REMOVED）；`impl` 类条款排在族尾但不改变内容。

## 2. 一次族合并的执行顺序

这张图回答：**从计划到归档要经过哪几步、哪一步可能返工**。它不表达具体命令与脚本参数。

```mermaid
sequenceDiagram
    autonumber
    participant Plan as 计划（plan.tsv）
    participant Script as merge-spec-family.sh
    participant Rev as review
    participant OS as openspec archive

    Plan->>Script: 族名 + 源 capability + 标题（* = 全取）
    Script->>Script: 抽取 Requirement → 按 kind 重排 → 写 PURPOSE
    Script-->>Plan: 新族 ADDED + 旧 capability REMOVED
    Plan->>Plan: 守恒验证（条数 / Scenario / 逐字比对）
    Plan->>Rev: 送审
    Rev-->>Plan: 发现（含「归档会中止」这类硬阻断）
    Note over Plan: 返工：改指针、补 marker、清表格
    Plan->>OS: 摘旧 spec 的 domain marker 后归档
    OS-->>Plan: +N -N，新族落盘
    Plan->>Plan: 归档后实测 marker 是否保留（预测不算）
```

**图不表达的约束**：归档前必须清掉旧 spec 里的 markdown 表格与首段后散文段（openspec 判为「不可点名内容」），否则 retire 拒绝删除。这条只在 `lane-ops` 那轮实测到。

## 3. 伪代码示意（族合并的守恒断言）

这张图回答：**「零语义变更」在代码上如何被验证**。它不表达真实 shell 实现。

```mermaid
flowchart LR
    A[收集 ADDED 的 Requirement 文本] --> B{逐条与源 spec 比对}
    B -- 不一致 --> X[报错退出，不写盘]
    B -- 一致 --> C[统计 Scenario 总数]
    C --> D{ADDED 场景数 == 源场景数?}
    D -- 否 --> X
    D -- 是 --> E[生成 delta]
```

**图不表达的约束**：比对要**规范化空白后再比**（source spec 里可能有软换行），但对内容必须逐字。
