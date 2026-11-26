<!-- complexity: simple -->
<!-- ui-impact: none -->

## Why

`ui-design-gate` 在 UI 合同未就绪时（新 schema `syntopica-ui` 且 `ui-impact` 声明为 major/minor 但 `ui-design.md` 缺失、`ui-prototype/` 缺失、或 `ui-approval` 仍为 `pending`）会**硬阻断一切 Agent 派发**，而不只是阻断 spec 点名的那两类动作。

对照 spec（`openspec/specs/ui-design-workflow/spec.md:75`）：

> 「major change 未获批准时，门禁 MUST 阻止实现工具对项目代码的写入**和实现子线程派发**，同时**允许**继续修改当前 change 的 UI 设计与原型」

spec 拦的是「**实现子线程**派发」与「项目代码写入」两项，同一句还明确「允许继续修改当前 change 的 UI 设计与原型」。当前实现把 agent-dispatch **一律**拦掉，超出了 spec 的字面边界，带来两个实际问题：

1. **死锁**：major 档 change 的原型缺失或未批准时，唯一合规的修复路径是在 change 目录内编辑 `ui-prototype/`（门禁本就放行该路径），但「派一个子线程去做原型调整」这个动作本身被拦死——只能在主会话里手改，无法享受子线程的上下文隔离与专门提示词。spec 明明「允许继续修改 UI 设计与原型」。
2. **误伤**：只读档（`explore`/`plan`/`change-review`）没有写工具，物理上不是「实现子线程」、也不可能产生前端实现，却同样被拦——把「勘察现状」也堵住了。

## What Changes

- 新增 `.pi/agents/ui-design.md` 子线程档：UI 原型设计专用，**允许 edit/write**（可改原型），提示词限定只做原型与规划制品的修改调整；禁区 `front/`、`backend-go/`、其它 change 目录。
- 收窄 `ui-design-gate` 对 `agent-dispatch` 的判定：**只读档 + 原型编辑档放行**，具备写能力的实现档（`general-purpose`）仍被拦——与 spec「阻止实现子线程派发」的边界一致。
- 新增 reasonCode `ui-dispatch-allowed` 记账：放行属「显著裁决」（既非 block 也非合同健康放行），留痕以备回检「合同未就绪期间发生过多少派发」。
- 写侧边界**不放宽**：子线程继承父会话档位，其每次 `edit`/`write` 仍逐个过门禁——改 `front/**` 照样被拦。
- **不含**：不新增 spec 能力（修改 `ui-design-workflow` 与 `subagent-harness-coverage` 两条既有能力）、不改产品前端、不改 `ui-design.md` 八节结构与审批语义（`ui-approval: approved` 仍只能由用户在对话中明确确认后写入）。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `ui-design-workflow`: 补充「Agent 派发的放行边界」——写侧边界才是安全边界；只读档与原型编辑档的派发不再被 UI 合同状态阻断（对齐 spec「阻止**实现**子线程派发」的字面边界），放行按 `ui-dispatch-allowed` 留痕。
- `subagent-harness-coverage`: 新增「原型编辑类 profile」的扩展加载定位（带 `load_extensions: true`，写入仍受 UI 门禁逐个校验），并追认只读探索/计划类项目覆盖档当前已声明加载扩展的现状。

## Impact

- **新增代码**：`.pi/agents/ui-design.md`（档位定义）。
- **改动代码**：`.pi/extensions/lib/ui-design-gate.ts`（放行判定 + reasonCode 白名单）、`.pi/extensions/ui-design-gate.ts`（取 `subagent_type` + 放行记账）。
- **改动测试**：`.pi/extensions/tests/ui-design-gate.smoke.cjs`（补放行/拦截/记账/漂移四组用例）。
- **改动文档**：`docs/reference/harness/pi-extensions.md`（子线程通道矩阵）、`docs/reference/开发执行规范.md`（档位选择表）、`AGENTS.md`（档位清单）。
- **不改动**：`front/`、`backend-go/`、`openspec/specs/` 主 spec（经 delta spec 归档时同步）。
- **部署后用户可见变化**：无产品行为变化。变化在 agent 侧——UI 合同未就绪时，主会话可派只读子线程勘察现状、可派原型编辑档调整原型；写前端实现的拦截行为完全不变。
