# 重排前后

> 事实来源：`docs/reference/flow/semantic-board.md` 的实测行数与块清单（2026-11-26）。

## 1. 从「按 change 切片」到「按能力组织」

这张图回答：**重排前后读者找一条规则的路径差在哪**。它不表达具体块的搬迁顺序。

```mermaid
flowchart TB
    subgraph 现在
        A1["链路设计 448 行"] --> A2["17 块，标题带 change 名"]
        A2 --> A3["查「标签现在什么规矩」<br/>要翻 5 块：<br/>辅助标签 / 组合标签 /<br/>延迟 embedding / 合并预览 /<br/>watched tags"]
    end
    subgraph 重排后
        B1["链路设计 6 族"] --> B2["一、标签体系"]
        B1 --> B3["二、版块匹配"]
        B1 --> B4["三、升级建议"]
        B1 --> B5["四、泳道与工作面"]
        B1 --> B6["五、版块体检"]
        B1 --> B7["六、版块治理"]
        B2 --> B8["查标签：只看一族<br/>上半「当前规则」"]
    end

    style A3 fill:#f8d7da
    style B8 fill:#d4edda
```

**图不表达的约束**：红线节（48 行 / 39 条）与变更溯源表（76 行）完全不动——它们本来就是干净的。

## 2. 每族内的两层

这张图回答：**同一个族里「当前规则」和「决策记录」怎么放**。它不表达具体条目。

```mermaid
flowchart TD
    A["### 一、标签体系"] --> B["#### 当前规则<br/>现在什么规矩<br/>+ 一张流程图"]
    A --> C["#### 决策记录<br/>规矩怎么来的、踩过什么坑<br/>（change 名 + 日期）"]
    B --> D["查规则：只看上半"]
    C --> E["考古：翻到下段"]

    style B fill:#d4edda
    style C fill:#e7f5ff
```

**图不表达的约束**：决策记录必须保留——台账抽样确认有些坑（两阶段事务边界导致 `idle in transaction` 4–40s、L2 命中改 Label 形成合并黑洞）**spec 里永远没有**。

## 3. 为什么这次只改一个域

这张图回答：**改动范围与验证节奏**。它不表达各域行数。

```mermaid
flowchart LR
    A["10 个 flow 文档<br/>都按 change 堆"] --> B["先改最胖的<br/>semantic-board 448 行"]
    B --> C{"方法验证通过?<br/>行数对账 / 术语不丢 /<br/>人读一遍能找到规则"}
    C -- 是 --> D["再推广到<br/>daily-report 322 行<br/>data-enrichment 254 行…"]
    C -- 否 --> E["改方法再试"]

    style B fill:#fff3cd
    style D fill:#d4edda
```

**图不表达的约束**：ai-summary（49 行链路）与 research-data-sources（20 行）这类小文档不该动——重排的成本大于收益，判据是「链路设计 >150 行且块数 ≥10」。
