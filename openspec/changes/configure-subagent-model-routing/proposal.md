<!-- ui-impact: none -->
<!-- diagrams: full(配置的三层优先级与"provider 自动跟随"是两条规则叠在一起，纯文字读两遍还算不准哪个赢；画出来一眼看到"主会话切 deepseek → 子线程跟着换") -->
<!-- constraint-domains: dev -->

# 子线程用哪个模型，改成可配置的

## Why

你的情况（原话）：

> 我的订阅经常变，有时候 minimax，有时候 deepseek 做主力，review 会用能力强的（因为直接干活额度不够）

现在的做法把模型**硬编码在三处**：`AGENTS.md` 的「子线程模型偏好」段、`开发执行规范.md` §供应商与模型选择、以及每次派发时任务文本里手写的 `model` 参数。

代价今天实际付了三次：

| 时间 | 情况 | 结果 |
|---|---|---|
| 派 review | GLM 5h 窗口剩 8% | 阻断，换 MiniMax |
| 换 MiniMax | Token Plan 到上限（429） | 阻断，再换 DeepSeek |
| DeepSeek | 余额 ¥1 | 勉强 |

每次换都要改文档、改派发参数，而且**下一轮又忘了改回来**。

## What Changes

### 1. 把模型配置从文档搬进配置文件

pi-subagents 本来就支持，只是我们没用：

| 位置 | 作用 | 优先级 |
|---|---|---|
| 派发时 `model` 参数 | 单次覆盖 | 最高 |
| `.pi/settings.json` → `subagents.agentOverridesByProvider.<provider>.<agent>` | **按 provider 分层**，主会话切哪家就跟哪家 | 高 |
| `.pi/settings.json` → `subagents.agentOverrides.<agent>` | 单档固定 | 中 |
| `.pi/agents/<agent>.md` frontmatter `model:` / `thinking:` | 档位默认（pi-web 界面可点选） | 低 |
| `subagents.defaultModel` | 兜底 | 更低 |
| 父会话模型 | 最终兜底 | 最低 |

### 2. 主力档按 provider 自动跟随

```json
"agentOverridesByProvider": {
  "minimax-cn": { "general-purpose": { "model": "minimax-cn/MiniMax-M3.1-Flash", "thinking": "max" } },
  "deepseek":   { "general-purpose": { "model": "deepseek/deepseek-v4-pro",  "thinking": "high" } },
  "zai-coding-cn": { "general-purpose": { "model": "zai-coding-cn/glm-5.3",  "thinking": "max" } }
}
```

**主会话切到哪家 provider，干活的子线程自动用那家的主力模型——不改任何文件。**

### 3. 判断密集档固定用强模型

`change-review` 与 `ui-design` 单独钉在强模型上（`agentOverrides`），不跟随 provider——因为「审得动」比「审得便宜」重要。

### 4. 文档改成指向配置，不再硬编码

`AGENTS.md`「子线程模型偏好」段与 `开发执行规范.md` §供应商与模型选择：删掉写死的「默认 MiniMax / 判断密集 glm」，改为「见 `.pi/settings.json` 的 `subagents` 段」+ 保留 `provider/modelId` 全称规则（那条与配置无关，仍然有效）。

## 不做的事

- **不做自动 fallback 链**：pi-subagents 文档明确写「Provider errors … are returned from that model rather than selecting another one」——要自动降级得自己写扩展拦截 429 再重派。额度耗尽目前是**手工换**（你选的），一次 30 秒，比维护一条 fallback 状态机划算
- **不改 `provider/modelId` 全称规则**：fuzzy 名会按字母序落到错误供应商，这条纪律与配置无关，保留
- **不动 agents 的 tools/codemode 等字段**：本 change 只碰模型
- **不做「今天用哪家」的显式标记**：provider 自动跟随已经覆盖；再加标记就多一步确认

## Impact

- **新增**：`.pi/settings.json`（项目级，之前没有这个文件）
- **改动**：`.pi/agents/{general-purpose,explore,plan,change-review,ui-design}.md` 各加 `model` / `thinking` frontmatter
- **文档**：`AGENTS.md` 模型偏好段、`开发执行规范.md` §供应商与模型选择
- **零代码 / 零前端 / 零后端**
- **风险**：`.pi/settings.json` 是项目配置，会覆盖用户级同名键——需要确认不影响其他项目设置
- **可回检**：派一个子线程，看 `session.rollup` 里出现的是不是配置的模型

## 已知残留

- **pi-web 界面能否编辑 `agentOverrides` 未知**：`/api/subagents/profiles` 路由支持 `model` 字段，但它写的是 frontmatter 还是 settings.json 未验证。两者并用后，界面改 frontmatter 也有效（只是优先级低于 settings.json）
- **额度耗尽的换法依赖我记得改回**：配置化降低了一半成本（只改一处），但「换回来」仍是人肉
