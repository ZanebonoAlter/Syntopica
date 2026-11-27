# Design

## D1 为什么按阶段而不按角色

旧五档（explore / plan / general-purpose / change-review / ui-design）回答的是「这个活是什么性质」，不回答「现在走到哪一步」。后果是**权限与模型只能按性质估**，估不准：

- `general-purpose` 一个档同时承担 propose（写制品）和 implement（改代码），权限取并集 = 最宽
- verify 要最强模型、implement 要中档，靠人记

按阶段分之后，**每个档的权限和模型都有唯一解**——因为阶段本身就限定了「该干什么、不该干什么」。

## D2 为什么跨供应商是常态

用户原话：「主会话 commandcode 的 deepseek 在控制，但 codex 的 gpt 在干活」。

这不是特殊用法，而是**成本与能力的自然分工**：

- 控制（读 brief、决定派谁、看结果）→ 中档模型够用，量大
- 判断（审查、设计、需求拆解）→ 必须强模型，错了不报错只悄悄漏
- 实现 → 有客观验收标准（编译/测试），中档错不了

三者对模型的要求不同，硬绑一家等于某一段必然吃亏。所以**按阶段钉死**，允许一轮里横跨三家。

## D3 为什么 `--cheap` 不改路由（review R-3 修正）

初版 `--cheap` 把 explore/propose/verify 路由到 `general-purpose`。审查指出这是反的：

> 降本开关 = 权限最宽，回到 change 要解决的问题

`general-purpose` 的 tools 是 write + edit + bash + mcp 全开，而 `stage-verify` 是只读。用「降本」当理由把只读审查档换成全权限档，等于用安全性换钱——而且是在用户看不见的地方换。

改成：**只输出建议文本，不改路由**。要降本请改 frontmatter 的 `model` 一行，权限不动。

## D4 为什么「只读」要说实话（review R-6）

`stage-explore` 是真只读——它的 tools 里**没有 bash**，四个工具（read/grep/find/ls）都写不了文件，这是唯一做得到「无法写」的档。

但 `stage-verify` / `stage-archive` / `stage-design` 有 `bash`，而 `bash` 能 `echo > file`。所以它们的「只读」是**纪律**不是**能力**。

初版文档写「只读」让人以为工具层拦住了，这是误导。改法：档位正文里明说「bash 本身能写文件，你有写权限，靠纪律不动」。既有的 `change-review` 档同理。

**为什么不把 bash 也去掉**：verify 必须跑门禁脚本（`archive-readiness.sh` / `check-standards.sh`），archive 必须跑 `openspec` 命令。去掉 bash 这些阶段就废了。这是真实的取舍，不是疏忽。

## 核心逻辑（伪代码）

### 关键数据结构

```
Stage = explore|propose|design|implement|verify|archive
StageMarker = "<!-- stage: <Stage> -->"        # stage.md 头部，恰好 1 条
AgentDef = { name, model, thinking, tools[] }   # .pi/agents/stage-<Stage>.md
```

### 主流程

```
取档(change):
    f = openspec/changes/<change>/stage.md
    if 文件不存在:            return "general-purpose"   # legacy
    markers = 全文匹配 /<!--\s*stage\s*:\s*(\w+)\s*-->/
    if len(markers) == 0:     return "general-purpose"   # 缺失
    if len(markers) > 1:      return "general-purpose"   # 多条即非法（与 parseStageMarker 对齐）
    s = lower(markers[0])
    if s not in Stage:        return "general-purpose"   # 非法取值
    return "stage-" + s
```

### 失败与边界分支

- **任何读取失败** → 兜底 + `exit 0`（fail-open）：路由问题不该挡住开发
- **`--cheap`** → 路由不变，只在 `--explain` 文本里附一句降本建议
- **无空格 marker**（`<!--stage:x-->`）→ 接受（与权威解析器一致）
- **`stage : x`（冒号前空格）** → 接受
- **正文里提到 `<!-- stage: x -->` 字样** → 若只有一条则仍算命中；这是与 `parseStageMarker` 的已知共同边界，不额外处理
- **档位未注册**（frontmatter 缺 `name`）→ pi-subagents 静默跳过，派发报 `Unknown agent`。smoke 里有 `name` 漂移防护

## 成本（review R-9 修正）

初版写「gpt-6.1-terra 约 $4.89/次会话」，审查查事实库无法复现该数字。真实数据（`.pi/harness/events.db` 的 `session.rollup`，按子会话取末次快照）：

| 模型 | 样本 | 中位 | p90 | 均值 | 最大 |
|---|---|---|---|---|---|
| gpt-6.1-terra | 52 | **$0.14** | $1.55 | $1.11 | $37.54（跑飞会话） |
| commandcode deepseek-v4.1-flash | 85 | **$0.56** | $2.61 | — | — |

典型 change 约 10 次派发：按中位约 **$2.5**，按均值约 **$10**，尾部风险是单个跑飞会话。

所以成本结论应是：**中位可接受，尾部要防**。防尾部的机制是 `stage-verify` brief 里限定工具调用范围（既有纪律），不是换便宜模型。
