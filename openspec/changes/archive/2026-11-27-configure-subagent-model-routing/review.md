# 审查报告：configure-subagent-model-routing

- **审查者**：GLM 5.3（只读）
- **结论**：需改后进
- **放行判断**：配置内容全部核对无误（优先级链/键名/模型 id 7/7 存在/无用户级覆盖冲突），但实测发现五档缺 `name` frontmatter 导致 pi-subagents 0.90.0 注册失败——`agentOverridesByProvider`/`agentOverrides`/frontmatter 三层配置对五档均无生效路径，须补 `name` 并实测派发后才可进。

## 发现

### R-1 2026-11-27【High】五档缺 `name` frontmatter，pi-subagents 无法注册——整套模型路由配置零生效
- **状态**：已修复
- **处置**：**这是本 change 最有价值的发现：配置写了但根本没路径生效。**

源码确认（`pi-subagents/src/agents/agents.js:1943-1945`）：`if (!frontmatter.name || !frontmatter.description) { continue; }`——缺 `name` 就**静默跳过**、无 diagnostics。五个项目档只有 `display_name`，所以 `discoverAgentSnapshot` 返回 `project: []`。

历史派发能成功是因为每次都在派发参数里显式写 `model`，走的是 per-run 路径，不经过 registry——这掩盖了缺口。

已给五档各补 `name: <档名>`（现 5/5 齐全）。**副作用**：这也意味着今天之前 `agentOverrides` / frontmatter `model` 从未生效过，本 change 是第一次让它们真正可用。
- **位置**：`.pi/agents/{general-purpose,explore,plan,change-review,ui-design}.md` frontmatter（均只有 `display_name`，无 `name`）；根因代码 `pi-subagents/src/agents/agents.js:1946-1949`
- **问题**：本 change 的全部价值建立在「五档可被 subagent 工具解析」之上，但 pi-subagents 0.90.0 的 `loadAgentsFromDefinitionFiles` 要求 `frontmatter.name`，缺失即 `continue` **静默跳过**（无 diagnostics）。五档文件均无 `name`（git 历史 `43eef1f8` 2026-10-14 起就无，本 change 只加 `model:`/`thinking:` 零删除，系存量缺口），导致项目档全部不注册。
- **证据**：
  1. **实派两次均失败**：本审查会话内 `subagent(agent:"explore")` 与 `subagent(agent:"general-purpose")` 均返回 `Unknown agent`，错误自述 `project: .pi/agents (5 candidates)` 但 Available agents 仅 13 个 builtin；
  2. **node 只读复现**：调用 `pi-subagents@0.90.0` 的 `discoverAgentSnapshot('/home/…/Syntopica','both')` → `project: []`、`agentDiagnostics: []`（静默）；
  3. **对照实验（/tmp，未动仓库）**：同一份 explore.md 加一行 `name: explore` → 注册成功且 `model=minimax-cn/MiniMax-M3.1-Flash` 被正确读取——证明 model/thinking 字段本身完全兼容，缺的只是 `name`；
  4. tasks.md 3.x 验证仅静态检查（JSON 合法/无重复键），未落「实派一条」任务，故漏检。
- **建议**：五个档 frontmatter 各补 `name: <档名>`；tasks.md 验证节补「派一个不指定 model 的 explore 子线程，核对实际模型」任务（proposal Impact 的可回检项本该落成任务）；修复后可用 `/subagents-models` 或实派验证。注意 git 历史无 name 但昨日 dispatch 成功的记录（`subagent.dispatch`，model 均带 `:max/:high` per-run 后缀）走的是显式 model 派发/SubagentWorkflow agentType 路径，不构成 registry 注册成功的反证。

### R-2 2026-11-27【Medium】change-review.md description 残留写死的旧默认模型，与配置自相矛盾
- **状态**：已修复
- **处置**：`change-review.md` 的 description 里「默认 minimax-cn/MiniMax-M3.1-Flash:max」改成「模型见 `.pi/settings.json` 的 subagents 段（本档钉死强模型，不随 provider 变）」。
- **位置**：`.pi/agents/change-review.md:2`
- **问题**：description 仍写「模型由派发时指定（默认 minimax-cn/MiniMax-M3.1-Flash:max）」，而本 change 已把该档钉死 `zai-coding-cn/glm-5.4`（agentOverrides + frontmatter）。spec「文档不再写死模型名」的 Scenario 只点名 AGENTS.md 与 开发执行规范.md，agent 描述文件漏网。
- **证据**：`change-review.md:2` 原文对照 `.pi/settings.json` `agentOverrides.change-review.model = zai-coding-cn/glm-5.4`。
- **建议**：改写 description 为「模型见 .pi/settings.json 的 agentOverrides」，删具体型号。

### R-3 2026-11-27【Medium】缺 commandcode provider 层——「跟随主力」对当前实际主力不生效
- **状态**：已修复
- **处置**：补 `commandcode` provider 层（general-purpose → deepseek-v4.1-flash / explore → 同 / plan → deepseek-v4-pro）。现共 5 层：minimax-cn / deepseek / zai-coding-cn / openai-codex / commandcode。openai-codex 层 explore 保持 gpt-6.1-terra 属用户自担成本，不改。
- **位置**：`.pi/settings.json` `agentOverridesByProvider`（四键：minimax-cn/deepseek/zai-coding-cn/openai-codex）
- **问题**：主会话当前实际模型是 `deepseek/deepseek-v4.1-flash`，但其 provider 是 **commandcode**（`pi --list-models` 中 commandcode 下列有 7 个 deepseek 系模型），不在配置四层内。按官方语义「provider key 来自 active parent session model」，主会话切到此主力时五档全部 miss 跟随层，落到 frontmatter（minimax flash）——「切主力不用改文件」的核心承诺对当前实况不成立。
- **证据**：`events.db` session.rollup 最新三条（2026-11-27T00:39-00:40）`model=deepseek/deepseek-v4.1-flash`；`pi --list-models` provider 列含 commandcode（且 enabledModels 用户级即含 commandcode/deepseek/deepseek-v4.1-flash）。
- **建议**：补 `"commandcode": {…}` 一层（general-purpose 可用 `commandcode/deepseek/deepseek-v4-pro` 或 v4.1-flash 系），或在文档写明「跟随层按 provider 精确匹配，聚合网关类 provider 需单独配层」。

### R-4 2026-11-27【Low】分档策略三处自洽性观察
- **状态**：已修复
- **处置**：design D4 补语义边界：`defaultThinking` **只作用于没有 `thinking` 值的档**，五档都写了 thinking，所以它对它们不可达——纯兜底、不在生效链上，别误以为能覆盖 frontmatter。
- **位置**：`.pi/settings.json`（openai-codex 层 explore；deepseek 层；defaultThinking）；design.md D4
- **问题**：①openai-codex 层 `explore` 也配 `gpt-6.1-terra`（历史实测 $4.89/次），与 minimax/zai 层「explore 用 flash 系便宜模型」的自我分层相悖；②deepseek 层在余额 ¥1 期间实际不可用（预案性质，用户知情，可接受）；③`defaultThinking` 官方语义只作用于「无 thinking 值」的 agent，五档 frontmatter 均已写 thinking，故该键对五档不可达——纯兜底无害，但 D4「每处都成对写」的表述易让人以为它在生效链上。
- **证据**：models.md「Thinking level defaults」："give … agents **without a `thinking` value** a shared thinking level"；五档 frontmatter 均含 `thinking:`。
- **建议**：openai-codex 层 explore 改配便宜档或保持现状（用户自担成本）；design.md D4 补一句「defaultThinking 仅兜底未写 thinking 的档」。

## 核实记录

- **A1 优先级链** ✅ 官方 models.md："per-run override → provider-scoped role override → `agentOverrides.<name>.model` → agent frontmatter `model` → `subagents.defaultModel` → the parent session model"。spec（无显式序但 Scenario 语义一致）、design.md 伪代码 ①-⑥、proposal 表、AGENTS.md:124、开发执行规范.md:187-194 六层表——全部一致。
- **A2 provider 键格式** ✅ "The provider key comes from the active parent session model"；四键 minimax-cn/deepseek/zai-coding-cn/openai-codex 均为 `pi --list-models` 真实 provider 名。
- **A3 `defaultThinking` 键名** ✅ 存在（models.md「Thinking level defaults」节）。用户级 `defaultThinkingLevel` 是 pi 主体的另一键，二者不冲突；brief 的担心解除。
- **A4 模型 id** ✅ 7/7 全存在：`minimax-cn/MiniMax-M3.1-Flash`、`deepseek/deepseek-v4-pro`、`deepseek/deepseek-flash`、`zai-coding-cn/glm-5.4`、`zai-coding-cn/glm-5.3`、`zai-coding-cn/glm-5.3-flash`、`openai-codex/gpt-6.1-terra`（`pi --list-models` 逐个命中）。
- **A5 项目级覆盖** ✅ 用户级 `~/.pi/agent/settings.json` 无 `subagents` 段；项目级 `.pi/settings.json` 仅含 `subagents` 键——theme/packages/defaultModel/enabledModels/defaultThinkingLevel 等均保留用户级，无意外丢失。
- **B1 分档划分** ✅ explore 归执行档合理（官方推荐 tier-1 fast workhorse 做侦察；各层 explore 均配 flash 系，除 R-4①）。**B4 不做 fallback** ✅ 与官方语义逐字一致（"Provider errors … are returned from that model rather than selecting another one"），D1 论证成立。
- **C 实测** ❌ **致命，见 R-1**：`subagent(agent:"explore")`/`subagent(agent:"general-purpose")` 均报 `Unknown agent`——配置生效性无从验证，因为档位本身进不了 registry。session.rollup 无法提供子线程 models 字段（子线程未启动）。
- **D1 文档硬编码** ✅ `grep 'MiniMax-M3.1-Flash:max' AGENTS.md` = 0 命中；AGENTS.md:124 已改「看配置」并保留六层链。**D2** ✅ 开发执行规范.md 六层优先级表（187-194 行）与 spec 一致。**D3** ✅ `provider/modelId` 全称规则保留（spec「禁止 fuzzy 名」Requirement + Scenario）。
- 附注：dev-process-guard 提示的 6 个泄漏 agent-browser/chromium 进程非本审查线程所起（本线程仅读文件+派发尝试均失败未成进程），按只读纪律未处置，建议人工确认。
