## ADDED Requirements

### Requirement: 子线程模型由配置决定，不写在文档里

子线程的模型选择 SHALL 由配置文件决定：

- `.pi/settings.json` → `subagents.agentOverridesByProvider.<provider>.<agent>`（按 provider 分层）
- `.pi/settings.json` → `subagents.agentOverrides.<agent>`（单档固定）
- `.pi/agents/<agent>.md` frontmatter `model:` / `thinking:`（档位默认）

文档（`AGENTS.md`、`开发执行规范.md`）MUST NOT 硬编码「默认用哪个模型、判断密集用哪个」，SHALL 指向配置文件。

#### Scenario: 换主力订阅不改文件

- **WHEN** 主会话模型切到 `deepseek/deepseek-v4-pro`
- **THEN** `general-purpose` 子线程 SHALL 使用 `agentOverridesByProvider.deepseek` 配置的模型，且无需修改任何文件

#### Scenario: 文档不再写死模型名

- **WHEN** 检索 `AGENTS.md` 与 `开发执行规范.md` 的模型择选段
- **THEN** MUST NOT 出现「默认 `minimax-cn/MiniMax-M3.1-Flash`」这类写死的当前选择，SHALL 只有指向配置文件的说明与「配置项含义」

### Requirement: 主力档跟随 provider，判断密集档固定

`general-purpose`（执行）/ `explore` / `plan` SHALL 按 provider 分层跟随主会话。

`change-review` / `ui-design` SHALL 固定在强模型档（不随 provider 变），理由是「审得动」优先于「审得便宜」。

#### Scenario: review 不跟着换

- **WHEN** 主会话切到 `minimax-cn`
- **THEN** `change-review` 仍 SHALL 使用配置里钉死的强模型

#### Scenario: 执行档跟随

- **WHEN** 主会话切到 `zai-coding-cn`
- **THEN** `general-purpose` SHALL 使用 `agentOverridesByProvider.zai-coding-cn` 的模型

### Requirement: provider/modelId 全称规则保留

派发时若显式指定 `model`，SHALL 用 `provider/modelId` 全称（可带 `:thinking` 后缀）。裸模型名当且仅当该 id 在已配置 provider 中唯一时才允许。

#### Scenario: 禁止 fuzzy 名

- **WHEN** 派发时写 `model: "glm-5.4"`
- **THEN** 该写法 MUST NOT 被采用，SHALL 写成 `zai-coding-cn/glm-5.4`

### Requirement: 额度耗尽时手工切换，不做自动 fallback

额度耗尽（401/429）时的处置 SHALL 是人工换 provider 或换档位重试。

系统 MUST NOT 在 provider 错误后自动选用另一个模型——该行为与 pi-subagents 的既有语义（错误从当前模型返回）一致，且自动降级会掩盖额度问题。

#### Scenario: 429 如实报错

- **WHEN** 子线程派发遇到 429
- **THEN** SHALL 把错误与建议（换哪个 provider）返回给主会话，MUST NOT 静默换模型重派
