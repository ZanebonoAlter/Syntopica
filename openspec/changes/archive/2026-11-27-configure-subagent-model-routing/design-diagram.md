# 模型路由

> 事实来源：pi-subagents 官方文档 `docs/models.md` 的优先级声明，与本仓库
> `.pi/settings.json` / `.pi/agents/*.md` 的实际配置。

## 1. 六层优先级

这张图回答：**一次派发到底用哪个模型**。它不表达具体的模型名（那些在配置里）。

```mermaid
flowchart TD
    A["派发子线程"] --> B{"① 派发时指定 model?"}
    B -- 是 --> Z["用它"]
    B -- 否 --> C{"② agentOverridesByProvider<br/>[主会话provider][档] 有?"}
    C -- 有 --> Z
    C -- 否 --> D{"③ agentOverrides[档] 有?"}
    D -- 有 --> Z
    D -- 否 --> E{"④ frontmatter model 有?"}
    E -- 有 --> Z
    E -- 否 --> F{"⑤ defaultModel 有?"}
    F -- 有 --> Z
    F -- 否 --> G["⑥ 父会话模型"]

    style Z fill:#d4edda
    style G fill:#fff3cd
```

**图不表达的约束**：`thinking` 是同级独立字段，不拼进 model 串；provider 错误（401/429）如实抛回，**不换模型重派**。

## 2. 两类档位

这张图回答：**为什么有的档跟随 provider、有的钉死**。它不表达具体模型。

```mermaid
flowchart LR
    A["子线程档"] --> B["执行类"]
    A --> C["审查类"]
    B --> D["general-purpose<br/>explore<br/>plan"]
    C --> E["change-review<br/>ui-design"]
    D --> F["跟随 provider 主力"]
    C --> G["钉死强模型"]
    F --> H["订阅变了<br/>不改任何文件"]
    G --> I["审得动优先<br/>审得便宜其次"]

    style H fill:#d4edda
    style I fill:#e7f5ff
```

**图不表达的约束**：执行档产出有客观标准（编译过不过、测试绿不绿），便宜模型错了立刻暴露；审查档产出没有客观标准，便宜模型错了不报错、只悄悄漏。

## 3. 换订阅时改什么

这张图回答：**三种情况下分别动哪里**。它不表达 JSON 结构。

```mermaid
flowchart TD
    A["订阅变了"] --> B{"情况"}
    B -->|"主力移到别家<br/>（minimax → deepseek）"| C["什么都不用改<br/>切主会话 provider 即可<br/>（agentOverridesByProvider 已预置）"]
    B -->|"某家额度耗尽"| D["派发时临时指定 model 全称<br/>或等窗口重置"]
    B -->|"想让 review 更省钱"| E["改 .pi/settings.json 的<br/>agentOverrides.change-review"]
    B -->|"界面上点选"| F["改 .pi/agents/&lt;档&gt;.md<br/>但优先级低于 JSON"]

    style C fill:#d4edda
    style D fill:#fff3cd
```

**图不表达的约束**：预置了四套 provider 层（minimax-cn / deepseek / zai-coding-cn / openai-codex），新增 provider 需要自己往 JSON 里加一层。
