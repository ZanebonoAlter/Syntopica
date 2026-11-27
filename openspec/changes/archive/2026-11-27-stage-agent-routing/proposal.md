# 按开发阶段配 agent（stage-agent-routing）

<!-- ui-impact: none -->
<!-- diagrams: none(配置迁移 + 一张映射表就够，画图不增加信息) -->
<!-- constraint-domains: harness -->

## 为什么要改

用户原话：

> 我实际可能 commandcode 的 deepseek 在主会话控制，但是 codex 的 gpt 在干活，跨供应商，而且按照我的开发流程，我觉得可能每个阶段都要考虑配对应的 agent，而不是现在的通用 agent

现有 5 个档（explore / plan / general-purpose / change-review / ui-design）是按**角色**分的，不含任何开发阶段信息。后果：

1. **跨供应商表达不了**：同一轮里「控制用 deepseek、干活用 gpt」无法描述，只能每次派发手写 `model`。
2. **阶段与权限不挂钩**：`explore` 只读是对的，但 `propose`（写制品）和 `implement`（改代码）用的是同一个 `general-purpose`，权限一样宽。
3. **阶段与模型不挂钩**：verify 阶段需要最强模型审，implement 用中档即可——现在靠人记。

## 改成什么

**6 个阶段档，各自钉死模型与工具权限**，派发时按 change 的 `stage.md` marker 自动选：

| 阶段 | agent | 模型 | 工具 | 为什么 |
|---|---|---|---|---|
| explore | `stage-explore` | openai-codex/gpt-6.1-terra | 只读 | 探路不需要写权限 |
| propose | `stage-propose` | openai-codex/gpt-6.1-terra | 读 + 写制品 + bash（仅 openspec 自检） | 需求拆解是判断活 |
| design | `stage-design` | zai-coding-cn/glm-5.4 | 读 + 写 + 画图 | 设计要强模型，伪代码/图是它的产出 |
| implement | `stage-implement` | commandcode/deepseek/deepseek-v4.1-flash | 全工具 | 有客观验收标准，中档足够 |
| verify | `stage-verify` | openai-codex/gpt-6.1-terra | 只读 + bash + write（仅 review.md） | 审查是判断活，错了不报错 |
| archive | `stage-archive` | zai-coding-cn/glm-5.4 | 读 + bash（纪律只读） | 跑门禁对账，要能读日志 |

**跨供应商成为常态**：控制（主会话）用哪家和干活（子线程）用哪家解耦——上表里 6 个阶段横跨 3 家。

## 怎么保证「自动选」

不靠我记，靠脚本：

```bash
bash scripts/harness/stage-agent.sh <change>   # 输出该派的 agent 名
```

它读 `openspec/changes/<change>/stage.md` 的 `<!-- stage: xxx -->`，映射到 `stage-<xxx>`；无 marker / legacy change 时输出 `general-purpose`（旧兜底）。派发前跑一次，agent 名照抄。

## 旧 5 档怎么办

**保留**，但降级为「无阶段任务的通用档」（一句话问答、跨 change 的杂活）。文档写清：

- change 流程内 → 用 `stage-*`
- change 流程外 / 没有 stage.md → 用旧四档 + `ui-design`

不删的理由：`.pi/extensions/ui-design-gate.ts` 按 `ui-design` 档名判断；`archive-readiness` / review 流程的 brief 模板都还引用 `change-review`。改名牵连面大于收益。

## Impact

- `.pi/agents/stage-{explore,propose,design,implement,verify,archive}.md` 新建 6 个
- `scripts/harness/stage-agent.sh` 新建（+ smoke）
- `.pi/settings.json`：6 个新档不配 provider 层（钉死），保留旧档的 provider 层
- `docs/reference/开发执行规范.md` §主线程纪律 + `AGENTS.md` 子线程段：写清阶段→档映射与脚本用法
- `docs/reference/harness/pi-extensions.md` 子线程通道矩阵补一列

## 风险

1. **成本**：explore/propose/verify 三档用 `gpt-6.1-terra`。事实库真实分布（`session.rollup` 末次快照，52 个样本）：中位 **$0.14** / p90 $1.55 / 均值 $1.11 / 最大 $37.54（跑飞会话）；对照 commandcode deepseek-v4.1-flash（85 样本）中位 $0.56。典型 change ≈10 次派发 → 按中位约 $2.5。**中位可接受，尾部要防**——防尾部靠 brief 限定工具调用范围，不靠换便宜模型。降本改 frontmatter `model` 一行（`--cheap` 只给建议不改路由，见 design D3）。
2. **档多维护重**：6 个文件，换供应商要改 6 处。这是用户明确选择的代价（「完全可控、一眼看懂」）。
3. **stage.md 缺失**：legacy change 落到 `general-purpose`，行为与今天一致，不会崩。
