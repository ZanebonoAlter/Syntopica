# 子线程档位按「角色」重分（rework-subagent-roles）

<!-- ui-impact: none -->
<!-- diagrams: none(档位是清单不是流程，画图不增加信息) -->
<!-- constraint-domains: harness -->

## 起因

上一轮 `stage-agent-routing` 把子线程按**开发阶段**切成 6 档（explore/propose/design/implement/verify/archive），假设阶段是**顺序推进**的。用户看过之后指出这个假设不成立：

> explore 没啥问题，原本的 change-review 和 ui-design 提示词我觉得都可以调整。因为我现在开新功能的流程是：1.主会话提需求→2.派发子线程 explore→3.主线程会根据报告快速确认，然后整理需求，开始和我沟通→4.基本确认后主会话开 change，出 datamap→沟通直到确认业务口径大致没问题→在 datamap 基础上新建原型(这一步可以开子线程)，而且按照规范还有数据口径的自检查和 ui 骨架检查→反复调整原型和业务口径 datamap 直到确认（主线程保持和用户沟通，保持上下文纯净，同时 pin 关键信息、调整 change 内容方便后续开发）→确认后补充 task 等制品→主会话压缩或者开新线程接手 change，派发实现子线程干活，每个模块干完就 review

真实流程里有**两个反复循环**，不是一个流水线：

1. **datamap ↔ 原型**（改十几轮，主线程全程和用户对话）
2. **implement ↔ review**（按模块来回，每个模块一轮）

在循环里，`propose` / `design` / `archive` 从不是独立阶段：
- 写 spec / datamap → **主线程自己写**（用户明确：「datamap 本质是一个比较小的 md，我偏向主线程自己写」）
- 设计 → 就是 datamap + 原型反复迭代，落在一个循环里
- 归档 → 机械跑门禁脚本

所以上一轮 6 个阶段档里，只有 `explore` 和 `implement` 是真需求，其余 4 个是我想象出来的环节。

## 改成什么

**4 个角色，5 个档**：

| 档 | 角色 | 模型 | 说明 |
|---|---|---|---|
| `explore` | 探路 | 不变 | 用户确认「没啥问题」 |
| `ui-design` | 原型制作 + 按反馈修改 | zai-coding-cn/glm-5.4 | 每轮改动涉及代码，主线程只做视觉验收 |
| `impl-heavy` | 复杂业务逻辑实现 | 强模型 | 事务/并发/状态机这类，错一次代价大 |
| `impl-light` | 常规改动实现 | 便宜模型 | 有客观验收标准（编译/测试），错了立刻暴露 |
| `change-review` | 通用审查 | 强模型（走降级链） | **一档 + brief 说明审什么**：change 制品 / 代码实现 / UI 验收 |

删掉 `stage-propose` / `stage-design` / `stage-verify` / `stage-archive` / `stage-explore` / `stage-implement`（上一轮新建的 6 个）。

## 三个关键决定

**1. review 一档 + brief 说明，不按领域/深度分档**

用户选：`.pi/agents/review.md` 一档，派发时在 brief 里写清「审前端还是后端、重点看什么」。

好处是最少文件、最灵活；代价是依赖 brief 写清——写漏了就漏审。这个代价用户接受。

**2. review 模型走降级链，由主线程执行切换**

用户给的链：`zai-coding-cn/glm-5.4` → `openai-codex/gpt-6.2-sol` → `commandcode/deepseek/deepseek-v4.1-flash`（用户口头说 `gpt-sol-6.2`，全称见链文件），且「后续可以自己调整」。

**这推翻了上一轮「额度耗尽保持手工换，不做自动 fallback」的决定**，需要记录：pi-subagents 框架本身不会换模型（官方：「Provider errors are returned from that model rather than selecting another one」）。所以降级由**主线程按链执行**——派发返回 429/401 时读错误、换下一家重派。

链存在 `.pi/model-chains.json`，我不读它，主线程读。

**不用扩展自动换**的理由：自动降级会把额度问题藏到整条链都空才报错，那时更难查；而 429 是客观信号，主线程看到就能换，成本是每次多一个动作。

**3. 子线程不读 AGENTS.md 全文**

用户原话：「对于验证子线程来说 AGENTS.md 的读取感觉不是每个都有必要，容易被污染，因为这个主要是主线程的约束」。

AGENTS.md 里 80% 是主线程流程约束（openspec 编排六步、归档门禁、Router 纪律、quota 换供应商），对 review/implement 是纯噪音，且**会误导**——review 子线程读到「主线程必须派 explore 而不是自己读代码」，可能去做流程判断。

改成按档给最小集：
- `explore` / `impl-*`：相关域红线（`constraint-digest --brief --paths`）+ 代码规范对应节
- `change-review`：spec 的 WHEN/THEN + diff + 测试命令，**不给 AGENTS.md**
- `ui-design`：datamap + `standard/frontend/{layout,prototype-content}.md`

## Impact

- `.pi/agents/`：删 6 个 `stage-*`，新增 `impl-heavy.md` / `impl-light.md`，改 `change-review.md` / `ui-design.md` 提示词
- `scripts/harness/stage-agent.sh` + `.smoke.sh`：**删除**（不再按阶段选档）
- `.pi/model-chains.json`：新增（review 降级链）
- `AGENTS.md` / `docs/reference/开发执行规范.md`：阶段映射表 → 角色映射表；补「子线程不给 AGENTS.md 全文」纪律
- `docs/reference/harness/pi-extensions.md`：同步

## 风险

1. **brief 写漏就漏审**：review 一档靠 brief，前面几轮实测过 brief 写不全的后果。缓解：文档给 review brief 模板（必须写：审什么文件、对照哪条 spec、往哪看）。
2. **降级链靠主线程执行**：主线程忙的时候可能忘了换。但 429 会让派发直接失败，失败是显式的，不会静默降级。
3. **`ui-design` 档名不能动**：`ui-design-gate` 按档名判断，改名会破门禁。
