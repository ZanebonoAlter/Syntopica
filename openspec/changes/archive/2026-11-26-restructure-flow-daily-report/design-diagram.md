# 重排结构

> 事实来源：`docs/reference/flow/daily-report.md` 实测（555 行 / 链路 322 行 / 14 块），
> 以及 `openspec/specs/daily-report-{system,interface,ops}` 的 Requirement 分布。

## 1. 从「后端/前端」到「生成 → 阅读 → 运维」

这张图回答：**为什么族顺序不是原文的前后端**。它不表达每块的具体内容。

```mermaid
flowchart TB
    subgraph 现在
        A1["9 块「后端：…」"] --> A2["技术层分组"]
        A3["4 块「前端：…」"] --> A2
        A2 --> A4["查「今日重点怎么选题」<br/>要记得它叫 story highlights"]
    end
    subgraph 重排后
        B1["一、生成与匹配<br/>daily-report-system"] --> B4["查选题：族一内"]
        B2["二、对外契约与阅读<br/>daily-report-interface"] --> B5["查阅读台行为：族二内"]
        B3["三、运维与补档<br/>daily-report-ops"] --> B6["查补填为什么不能覆盖：族三内"]
    end

    style A4 fill:#f8d7da
    style B4 fill:#d4edda
    style B5 fill:#d4edda
    style B6 fill:#d4edda
```

**图不表达的约束**：族与 spec 一一对应（3 族对 3 个 spec），不是「按感觉分三堆」。

## 2. 每族两层的分界线

这张图回答：**一段话该放「当前规则」还是「决策记录」**。它不表达具体条目。

```mermaid
flowchart TD
    A["读到一段话"] --> B{"换一种做法会不会错?"}
    B -- 会错 --> C["当前规则<br/>（含「不得做 X」类禁令）"]
    B -- 不会错，只是解释为什么 --> D["决策记录<br/>（change 名 + 日期 + 坑）"]
    C --> E["例：慢 I/O MUST 在事务外"]
    C --> F["例：补填不得覆盖已有报告"]
    D --> G["例：历史实现曾导致 idle in transaction 4–40s"]
    D --> H["例：主标签路径曾因重算 embedding 形成合并黑洞"]

    style C fill:#d4edda
    style D fill:#e7f5ff
```

**图不表达的约束**：这是上一轮 semantic-board 的 review 校准出来的判据——第一版我用「有没有 change 名」判，结果把「慢 I/O MUST 事务外」这类现行约束误下沉了。

## 3. 对账四条防线

这张图回答：**怎么证明 322 行没丢东西**。它不表达具体数字。

```mermaid
flowchart LR
    A["搬迁 322 行"] --> B["① 正文行零丢失<br/>原文非空行（标题除外）都在"]
    A --> C["② 术语 grep 计数<br/>20 个高频词前后一致"]
    A --> D["③ 红线注入逐字相同<br/>73 条一条不差"]
    A --> E["④ 四节逐字相同<br/>红线/变更溯源/代码入口/需求说明"]
    B & C & D & E --> F["可归档"]

    style B fill:#fff3cd
    style F fill:#d4edda
```

**图不表达的约束**：**行数不是判据**——重排必然新增结构行（3 族标题 + 6 层标题 + 族说明），总行数一定涨。上一轮我把「非空白行数相等」写进 spec，是错的。
