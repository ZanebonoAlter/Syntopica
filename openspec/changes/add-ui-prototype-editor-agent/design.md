# Design: add-ui-prototype-editor-agent

## Context

`ui-design-gate` 的当前实现（`lib/ui-design-gate.ts` 的 `decideUiToolGate`）在合同 block 态下按工具动作分流：

```
other（read/bash/grep）              → allow
mutation 且目标在 change 目录内       → allow
mutation 且越出 change 目录           → block
Agent 派发                            → block   ← 本 change 收窄这一条
```

spec（`openspec/specs/ui-design-workflow/spec.md:75`）的原文是：

> 「major change 未获批准时，门禁 MUST 阻止实现工具对项目代码的写入和**实现子线程派发**，同时**允许**继续修改当前 change 的 UI 设计与原型」

因此 spec 拦的是两项：**项目代码写入** 与 **实现子线程派发**；而「修改当前 change 的 UI 设计与原型」被明确允许。当前实现把 agent-dispatch 一律拦掉，落差有两处：

- 只读档（`explore`/`plan`/`change-review`）无写工具，物理上不是「实现子线程」，拦它无依据；
- 原型编辑档服务的正是 spec 明确允许的「修改 UI 设计与原型」路径，拦派发反而把唯一合规的修复路径堵死（原型目录内的写入本就放行）。

所以本 change 是**把实现拉回 spec 的字面边界**，不是放松约束。

关键安全前提（已核实）：子线程继承父会话档位——`constraint-injection.ts` 在子线程 `session_start` 时执行 `inheritFromParent[History]` 并记 `mode.set source=inherit`（写在**子线程自己的 sessionId** 下），因此 `ui-design-gate` 在子线程会话里 `readLatestMode(cwd, 子线程sessionId)` 能取到 `mode=implementation` 与 `boundChange`。**子线程的每次 `edit`/`write` 仍逐个过同一门禁**。

另一处约束定位：`openspec/specs/subagent-harness-coverage/spec.md:13` 把项目侧 profile 分为「实现类」（SHALL 声明 `load_extensions: true`）与「只读探索/计划类」（SHALL NOT 声明加载扩展，保持轻量）两类——原型编辑类在现行分类里无处安放，且只读类那条与 2026-10-13 上线的项目覆盖档（已声明 `load_extensions: true`）存在既存漂移。本 change 的 delta spec 一并处理这两处。

## Decisions

### D1 — 放行判定放在纯函数层，与既有分支同层

- **落点**：`.pi/extensions/lib/ui-design-gate.ts` 的 `decideUiToolGate`
- **取舍**：不把判定塞进扩展层的 `classifyTool`（那里只负责识别动作类型，判定归纯函数是本仓既有分层）；扩展层只负责把 `subagent_type` 透传进 `UiToolAction`。代价是纯函数入参多一个字段。
- **理由**：判定逻辑保持可直测（`lib` 层白盒用例），扩展层只做 I/O 与记账。

### D2 — 放行集合按**档位语义**划分（显式常量白名单），不按工具集动态判定

> **白名单划分依据是档位语义而非工具集**——`change-review` 带 shell，不满足「无写工具」的严格定义，但其用途限定为审查（只读通用档），故仍在放行集合内；`ui-design` 带 `edit`/`write`，其写入边界由写侧门禁逐个兜住（D4）。

- **落点**：`lib/ui-design-gate.ts` 新增常量 `UI_DISPATCH_ALLOWED_AGENTS`（只读档 + 原型编辑档）
- **取舍**：不用「读档位文件的 tools 字段」动态判定（每次 tool_call 读盘太重，且档位文件缺失时行为不可预期），改用显式常量白名单；代价是档位改名/新增需同步两处。
- **理由**：门禁热路径必须零 I/O；白名单是「有界、可审计」的选择。
- **漂移防护**：smoke 用例断言白名单每一项在 `.pi/agents/` 有同名文件存在，并断言具备写能力的实现档（`general-purpose`）**不在**白名单内——后者是「放行集合不越界到写能力档」的可执行断言；另补**反向**断言——`.pi/agents/` 中 `tools` 不含 `edit`/`write` 的 profile 必须都在白名单内（新增只读档忘记同步白名单会被误拦），失败时报出待同步的档名。

### D3 — 只读档在**任何**合同状态下放行；原型编辑档在合同 block 态放行

- **落点**：`decideUiToolGate` 的 block 分支新增 agent-dispatch 子分支
- **取舍**：不区分「合同 block 原因是否属于原型族」（`ui-prototype-missing` / `ui-approval-pending` / `ui-design-missing`）——只读档与原型档的放行都不依赖具体原因，判定更简单、更少漏网。
- **理由**：这两个档的写入边界已被 D4 的写侧门禁兜住，无需在派发层再按原因细分。

### D4 — 写侧边界不放宽：子线程写入仍逐个过门禁

- **落点**：`.pi/extensions/ui-design-gate.ts`（`classifyTool` 对 `edit`/`write` 的分支**不改**）
- **取舍**：不放宽任何 `mutation` 判定；原型档改 `front/**` 照样被拦，改其它 change 目录也照样被拦。
- **理由**：放行的是「派发」这个动作，不是「写入」。原型档改 `openspec/changes/<boundChange>/ui-prototype/**` 落在 change 目录内 → 门禁本就放行。
- **已知边界（如实记录，不在本 change 扩权）**：bash 类工具（`other`）不受门禁约束，理论上可用 shell 重定向写 `front/`——这是**既有边界**（父会话同样具备该能力），本 change 不扩大也不缩小它。
- **已知边界二（如实记录，不在本 change 扩权）**：pi-web `Agent` 工具支持 `isolation: "worktree"`，worktree 自 HEAD 构建——未提交的 change 目录不存在 → `.openspec.yaml` 读不到 → `schema=null` → 合同判 `legacy` → 写侧降级为「`front/` mutation 每会话/change warn 一次、不阻断」。本 change 放行了 `ui-design` 档派发，故「派发放行 + worktree 隔离 → 写侧零执行」成为新可达组合；规避路径是派发原型档时不带 worktree 隔离（或先把 change 制品提交进 HEAD），本 change 不扩权处理。

### D5 — 放行按 `action=warn` + 新 reasonCode `ui-dispatch-allowed` 记账

- **落点**：`lib/ui-design-gate.ts` 的 `UI_REASON_CODES`（8 → 9）+ `decideUiToolGate` 返回值新增 `dispatch-allowed` 动作；扩展层据此记账
- **取舍**：不新增 `policy.decision` 的 `action` 枚举值（spec 限定 `block|warn|bypass|fail-open`），复用 `warn`；不加新 action 值以免动 `harness-fact-log` 的枚举契约。
- **理由**：放行既不是阻断也不是「合同健康放行」（后者 MUST 零记录），留痕才能事后回检「合同未就绪期间派了多少次、派的是什么档」。
- **配套**：`ui-design-workflow` spec 的 reasonCode 枚举同步加第 9 个值（delta spec MODIFIED）。

### D6 — 放行只对 pi-web `Agent` 工具生效

- **落点**：扩展层 `classifyTool`（`toolName === "Agent"` 分支）
- **取舍**：不覆盖 pi-subagents 的 `subagent` 工具名——本仓当前全部派发走 pi-web `Agent` 工具，pi-subagents 通道的档位语义不同（自带 scout/worker/reviewer），贸然放行会扩大面。
- **理由**：与既有门禁的 `toolName` 判定方式一致（spec-gate 同样只认 `bash`）。

### D7 — 原型编辑档不绑模型，模型派发时指定

- **落点**：`.pi/agents/ui-design.md` 的 frontmatter（不写 `model`）
- **取舍**：不给该档固定默认模型（原型审美质量对模型敏感，固定会锁死），改为在 description 中提示「原型审美任务建议用强模型」。与 `change-review` 档一致。
- **理由**：模型选择属派发时决策，档案位定义不该替用户钉死。

### D8 — 原型档挂项目 `frontend-design` skill，不挂 `taste-skill`

- **落点**：`.pi/agents/ui-design.md` 的 `load_skills: true`（**生效键**；与既有四档一致，全开）——`skills: true` 为与既有档同形的惰性键（pi-web 侧解析 `loadSkills: v(e?.load_skills ?? e?.skills, false)`，布尔 `true` 落 `undefined`，实际不生效）
- **取舍**：不用白名单锁死 skill 集合——`frontend-design`（`.agents/skills/`，项目内）适用于产品 UI；`taste-skill`（`~/.pi/agent/skills/`）首行明确写「Not dashboards, not data tables, not multi-step product UI」，**不适用 Syntopica**，故不引导使用。
- **理由**：档位提示词点名用 `frontend-design` 的审美标准，避免 AI 味；不主动推荐不适用的 skill。

### D9 — 原型档提示词锁定四件事：辖区、方法、验证、交付

- **落点**：`.pi/agents/ui-design.md` 的系统提示词
- **取舍**：提示词写死「只改 `openspec/changes/<change>/ui-prototype/**` 与 `ui-design.md` 的 Prototype 节」，并显式列禁区（`front/`、`backend-go/`、其它 change 目录）；不写「可自行决定改哪」这类开放授权。
- **理由**：提示词是门禁之下的第二道防线——门禁管路径，提示词管意图。

## Risks

| 风险 | 缓解 |
| --- | --- |
| 白名单漂移（档位改名后门禁仍放行/误拦） | smoke 用例断言白名单项与 `.pi/agents/` 文件一一对应 |
| 放行被当成「门禁失效」 | 新 reasonCode 留痕 + spec 明确「写侧边界才是安全边界」 |
| 原型档越界改前端 | 门禁写侧逐个拦（D4）+ 提示词禁区（D9）双重防线 |
| reasonCode 白名单变动遗漏 spec | delta spec MODIFIED 同步 + 归档门禁 `openspec validate --strict` |

## Migration

无数据迁移。既有 change 的行为变化：major/minor 档合同未就绪时，主会话从「派发一律被拒」变为「只读档与原型档可派」。历史 `policy.decision` 事件不受影响。
