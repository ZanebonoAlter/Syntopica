# Design

## D1 为什么删掉「阶段档」

上一轮按开发阶段切 6 档，隐含假设是**阶段顺序推进**：explore → propose → design → implement → verify → archive。

用户看完真实流程后指出这个假设不成立。真实流程里有两个**反复循环**：

```
datamap ↔ 原型        改十几轮，主线程全程和用户对话
implement ↔ review    按模块来回，每个模块一轮
```

在这两个循环里：

- `propose`（写 spec）不是子线程的活——用户明确「datamap 本质是一个比较小的 md，我偏向主线程自己写」
- `design` 不是独立环节——它就是 datamap + 原型反复迭代，落在循环里
- `archive` 不是判断活——机械跑门禁脚本

所以 6 档里只有 `explore` 和 `implement` 是真需求。剩下 4 个是**我按想象中的流程切出来的**，不是他实际的流程。

## D2 为什么 implement 分两档，review 只一档

用户原话：「实现子线程档位可以根据重要程度调整，复杂的业务逻辑可用强模型 implement+review，不太复杂的派简单模型+强模型 review」。

**implement 的模型是变量**（同一角色，按模块复杂度换模型）→ 所以拆成 `impl-heavy` / `impl-light` 两个档，各自钉死模型。这样"选哪档"是个**显式动作**，比"派发时临时想一个 model 名"可靠。

**review 的模型是常量**（审查永远要用强的），但**审的范围是变量**（审制品 / 审代码 / 审 UI / 审前端 / 审后端）。用户选「一档 + brief 说明」。

为什么不按领域/深度分档（我提过的方案 A/B）：

- **档位是死配置，brief 是活信息**。同一份后端代码，这次要审事务边界、下次要审错误处理——分档追不上这个变化，brief 能
- 用户说「允许一次多个实现的 review（都属统一模块，或者前后端分类这种）」——一次派发审多个模块，这靠 brief 列清单最自然；靠分档反而要派两次

代价是**brief 写漏就漏审**。这是真实代价，文档里写清并要求 brief 模板三件事（审什么 / 对照哪条 spec / 往哪看）。

## D3 为什么降级链由主线程执行，不用扩展

上一轮明确决定「不做自动 fallback」，理由是「自动降级会掩盖额度问题」。这一轮用户改成「允许降级 glm-5.4 → gpt-sol-6.2 → deepseek-v4.1-flash」——**决定变了**。

但变的只是「要不要降级」，没变「谁来降」。两种实现：

| 方案 | 机制 | 问题 |
|---|---|---|
| 主线程按链换（选它） | 派发返回 429/401 → 主线程读到错误 → 换链中下一个重派 | 每次多一个动作 |
| 扩展自动换 | 拦 429 → 读链 → 重派 | 会把额度问题藏到整条链都空才报错，那时更难查 |

关键区别：**429 是客观信号，不是我自报的状态**。主线程看到失败就会换，不存在「忘了换」的静默失败——因为派发直接失败了，失败是显式的。

所以链存 `.pi/harness/model-chains.json`，主线程读它，改它即可调整顺序，无代码。

## D4 为什么子线程不给 AGENTS.md 全文

用户原话：「对于验证子线程来说 AGENTS.md 的读取感觉不是每个都有必要，容易被污染，因为这个主要是主线程的约束」。

AGENTS.md 的内容构成（大概比例）：

| 内容 | 占比 | 对 review 子线程 |
|---|---|---|
| 流程编排（openspec 六步、归档门禁、Router 纪律、换供应商） | ~65% | **噪音 + 会误导** |
| 业务红线索引 | ~15% | 有用（但用 `constraint-digest` 取更精确） |
| 技术环境（端口、DB、构建命令） | ~10% | 部分有用 |
| 代码规范指针 | ~10% | 有用（但可直接 read 目标文档） |

**"会误导"比"噪音"严重**：review 子线程读到「主线程必须派 explore 而不是自己读代码」，可能去做流程判断而不是审代码；读到「归档前跑九项门禁」，可能开始跑门禁而不是审实现。

改成按角色给最小集：

```
explore    → 目标文件清单 + 相关域红线
impl-*     → 域红线摘要 + 代码规范节 + tasks 条目 + 验收命令
change-review → 待审对象 + spec 条款 + 验收命令（不给 AGENTS.md）
ui-design  → datamap + frontend/layout.md + frontend/prototype-content.md
```

## 核心逻辑（伪代码）

### 关键数据结构

```
Role = explore | ui-design | impl-heavy | impl-light | change-review
AgentDef = { name, model, thinking, tools[] }        # .pi/agents/<Role>.md
Chain = { <Role>: [modelId, ...] }                   # .pi/harness/model-chains.json
Brief = { 审什么, 对照什么, 往哪看 }                  # 审查档必填三件事
```

### 派发决策流程（主线程执行）

```
选档(task):
    if task.类型 == 探路:                        return "explore"
    if task.类型 == 原型制作或修改:                return "ui-design"
    if task.类型 == 实现:
        if task.涉及 事务/并发/状态机/跨表一致性:
            return "impl-heavy"      # 错了会静默产生错数据
        return "impl-light"          # 错了立刻被发现
    if task.类型 == 审查:                        return "change-review"

派发(role, brief):
    for model in Chain[role]:
        r = agent(role, model=model, brief=brief)
        if r.ok:                return r
        if r.error in (429, 401):  continue      # 额度问题，换下一个
        return r                                  # 其他错误如实返回
    raise "整条链耗尽"                             # 不静默改用别的模型
```

### 失败与边界分支

- **链耗尽** → 如实报错，**不**静默换一个不在链里的模型
- **brief 缺三件事** → 审查档直接问，不猜着审
- **impl-light 干到一半发现是复杂逻辑** → 停下来报告，不硬用简单思路糊过去
- **非额度错误**（500/超时/内容问题）→ 立刻返回，不做链重试（重试也解决不了）
- **`.pi/harness/model-chains.json` 缺某角色** → 用该档 frontmatter 的 `model`（兜底，不崩）

## 成本

review 走链，首选 glm-5.4（zai 订阅内，成本 0）；429 才降到 gpt-6.2-sol（事实库 terra 系中位 $0.14/次）；再降到 deepseek-flash。

impl 分层后：`impl-light` 用 deepseek-v4.1-flash（85 样本中位 $0.56），`impl-heavy` 用 deepseek-v4-pro。相比「一个 implement 档全用强模型」，常规改动这块成本能降一半以上。
