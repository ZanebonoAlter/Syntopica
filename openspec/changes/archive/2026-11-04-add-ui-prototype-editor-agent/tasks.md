# Tasks: add-ui-prototype-editor-agent

## 0. 制品（已完成）

- [x] 0.1 `proposal.md`（complexity: simple / ui-impact: none）+ `design.md`（D1~D9 决策与落点）+ `ui-design.md`（none 档 N/A）
- [x] 0.2 `specs/ui-design-workflow/spec.md`（MODIFIED：实现入口门禁与存量兼容 / UI 门禁裁决进入事实库）
- [x] 0.3 `specs/subagent-harness-coverage/spec.md`（MODIFIED：子线程扩展加载配置——新增原型编辑类定位 + 追认只读类现状）

## 1. 门禁收窄（ui-design-gate）

- [x] 1.1 `.pi/extensions/lib/ui-design-gate.ts`：`UiToolAction` 的 `agent-dispatch` 变体增加 `subagentType` 字段；新增常量 `UI_DISPATCH_ALLOWED_AGENTS`（只读档 `explore`/`plan`/`change-review` + 原型档 `ui-design`）；`UI_REASON_CODES` 增加 `ui-dispatch-allowed`（8 → 9）；`decideUiToolGate` 的 block 分支新增 agent-dispatch 子分支——命中白名单则返回放行（带 `dispatch-allowed` 标记供上层记账），否则维持 block
- [x] 1.2 `.pi/extensions/ui-design-gate.ts`：`classifyTool` 的 `Agent` 分支从 `event.input` 取 `subagent_type` 填入 `subagentType`；`check()` 收到放行结果时记 `policy.decision(action=warn, reasonCode=ui-dispatch-allowed, change=boundChange)`；`edit`/`write` 分支**不改**
- [x] 1.3 核对「放行不改变写侧判定」：`mutation` 越出 change 目录 → block；change 目录内 → allow（两条路径均维持现状）

## 2. 原型编辑档

- [x] 2.1 新建 `.pi/agents/ui-design.md`：frontmatter 声明 `tools: read, grep, find, ls, edit, write, bash` + `load_skills/load_extensions/codemode/mcp` 全开、**不绑 model**；系统提示词限定辖区为 `openspec/changes/<change>/ui-prototype/**` 与 `ui-design.md` 的 Prototype 节，显式列禁区（`front/`、`backend-go/`、其它 change 目录），要求先读 `ui-design.md` 八节 + `docs/reference/standard/frontend/layout.md`，改后跑原型自带验证脚本/双视口截图，交付「改了什么 + 待用户批准的项」
- [x] 2.2 提示词点名使用项目 `frontend-design` skill 的审美标准；明确不使用 `taste-skill`（其首行声明不覆盖 dashboard / product UI）
- [x] 2.3 提示词写明升级门槛（仅①用户拍板分歧②不可逆副作用③红线留痕绕过三类才联系主线程）

## 3. 测试

- [x] 3.1 `.pi/extensions/tests/ui-design-gate.smoke.cjs` 补四组用例：只读档派发在合同未就绪时放行并记 `ui-dispatch-allowed`；原型档派发放行并留痕；写能力档派发仍 block；白名单每一项在 `.pi/agents/` 有同名文件存在（漂移防护）
- [x] 3.2 补一条：原型档在子线程会话内 `edit` `front/` 下文件 → 仍 block（写侧不放宽）
- [x] 3.3 `bash .pi/extensions/tests/run-harness-smoke.sh` 全绿

## 4. 文档

<!-- doc-impact: none(纯 agent 工具链调整：不触 flow/api/database/architecture/standard/configuration/deployment 任一业务域；改动的 harness 机制文档与档位定义在本节显式列出) -->

- [x] 4.1 `docs/reference/harness/pi-extensions.md`：子线程通道矩阵补 `ui-design` 档行；门禁表补 `ui-dispatch-allowed` reasonCode
- [x] 4.2 `docs/reference/开发执行规范.md` §0.6 档位选择表补 `ui-design` 档用途与放行边界
- [x] 4.3 `AGENTS.md` 档位清单补 `ui-design` 档（五档 → 五档含 ui-design，实际为 5 档）
- [x] 4.4 `docs/reference/harness/pi-extensions.md` 子线程派发模型偏好段：原型审美任务建议用强模型（不绑死档位）

## 5. Review

- [x] 5.1 派 `change-review` 档子线程聚焦 review（审查制品 + 实现 diff），意见按 High/Medium/Low 处置后 `review.md` 建账

## 6. 测试

- `node .pi/extensions/tests/ui-design-gate.smoke.cjs` → PASS（放行/拦截/记账/漂移四组新用例 + profile 前提断言与反向漂移断言 + 既有用例全绿；需先跑 `bash .pi/extensions/tests/run-harness-smoke.sh` 产出 esbuild bundle）
- `bash .pi/extensions/tests/run-harness-smoke.sh` → SMOKE OK（全部断言零失败）
- `node .pi/extensions/tests/policy-decision.smoke.cjs` → PASS（记账路径未破坏既有契约；需先跑 `bash .pi/extensions/tests/run-harness-smoke.sh` 产出 esbuild bundle）
- `npx --yes openspec validate --strict add-ui-prototype-editor-agent` → PASS（delta spec 格式合规）

## 7. 文档

<!-- doc-impact: none(理由同「## 4. 文档」节首行) -->

- [x] `docs/reference/harness/pi-extensions.md` → 通道矩阵 + reasonCode 表已同步
- [x] `docs/reference/开发执行规范.md` → 档位选择表已同步
- [x] `AGENTS.md` → 档位清单已同步

## 8. 验证

| Scenario | 测试文件 |
| --- | --- |
| 只读档派发在合同未就绪时放行 | .pi/extensions/tests/ui-design-gate.smoke.cjs |
| 原型编辑档派发在合同未就绪时放行 | .pi/extensions/tests/ui-design-gate.smoke.cjs |
| 写能力档派发仍被阻断 | .pi/extensions/tests/ui-design-gate.smoke.cjs |
| 放行不改变写侧判定 | .pi/extensions/tests/ui-design-gate.smoke.cjs |
| major pending 放行只读档与原型编辑档派发 | .pi/extensions/tests/ui-design-gate.smoke.cjs |
| 原型编辑档写入越界仍被阻止 | .pi/extensions/tests/ui-design-gate.smoke.cjs |
| major pending 阻止实现写入 | .pi/extensions/tests/ui-design-gate.smoke.cjs |
| 完整规划正常放行 | .pi/extensions/tests/ui-design-gate.smoke.cjs |
| 旧 change 不被突然卡死 | .pi/extensions/tests/ui-design-gate.smoke.cjs |
| pending 审批阻断被记录 | .pi/extensions/tests/ui-design-gate.smoke.cjs |
| 存量提示被记录 | .pi/extensions/tests/ui-design-gate.smoke.cjs |
| 合同未就绪时的放行派发被记录 | .pi/extensions/tests/ui-design-gate.smoke.cjs |
| 健康放行零记录 | .pi/extensions/tests/ui-design-gate.smoke.cjs |
| 实现档带扩展派发后约束可达 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 只读探索档保持轻量 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 原型编辑档带扩展派发后写入受门禁校验 | .pi/extensions/tests/ui-design-gate.smoke.cjs |

- `node .pi/extensions/tests/ui-design-gate.smoke.cjs` → PASS（§E 用例全绿：E1~E4 + E5/E5b 白名单正向漂移防护 + E5c 原型档 profile 前提断言 + E6 反向漂移防护；需先跑 `bash .pi/extensions/tests/run-harness-smoke.sh` 产出 esbuild bundle）
- `node .pi/extensions/tests/constraint-injection.smoke.cjs` → 2 项红（`semantic-board.md`/`reading.md` 的 `doc-impact-applies` 标签路径漂移，既存问题，与本 change 无关，已登记 test-patrol 台账），其余全绿；`bash .pi/extensions/tests/run-smoke.sh` 整体退出码 1 属预期
- `bash .pi/extensions/tests/run-harness-smoke.sh` → SMOKE OK（零失败）
- `bash scripts/harness/scenario-trace.sh openspec/changes/add-ui-prototype-editor-agent` → 退出码 0（12 个 Scenario 映射齐全）
- `bash scripts/harness/review-trace.sh openspec/changes/add-ui-prototype-editor-agent` → 退出码 0（review.md 已建账 11 条）
- `bash scripts/harness/doc-impact.sh verify openspec/changes/add-ui-prototype-editor-agent` → 退出码 0
- `bash scripts/harness/check-standards.sh --change add-ui-prototype-editor-agent` → 退出码 0
- `bash scripts/harness/check-tasks-tail.sh openspec/changes/add-ui-prototype-editor-agent` → 退出码 0
- `npx --yes openspec validate --strict add-ui-prototype-editor-agent` → PASS
- `bash scripts/harness/archive-readiness.sh openspec/changes/add-ui-prototype-editor-agent` → 退出码 0（六项全绿）

## 9. Review 处置（2026-11-04，11 条全修）

- [x] 9.1 R-1 写侧边界措辞限定到结构化写工具（两份 delta spec + AGENTS.md + 开发执行规范.md + pi-extensions.md）
- [x] 9.2 R-2 放行理由改为可核验事实陈述（delta spec + lib 注释 + design D2）
- [x] 9.3 R-3 本 change 的 ui-design.md 补 `<!-- ui-prototype: none -->` marker
- [x] 9.4 R-4 §8 验证记录改为如实（constraint-injection 2 项红 + review-trace 建账条数）
- [x] 9.5 R-5 补原型编辑档 profile 前提断言（`load_extensions: true` + tools 含 `edit`/`write`）
- [x] 9.6 R-6 design D8 机制指正（`load_skills: true` 生效键）
- [x] 9.7 R-7 §3.3 runner 路径改正
- [x] 9.8 R-8 §6 单文件命令补前置 bundle 产注
- [x] 9.9 R-9 design 补 worktree 隔离边界
- [x] 9.10 R-10 pi-extensions.md 矩阵 ui-design 行补 turn_end 降载
- [x] 9.11 R-11 白名单漂移反向断言（失败报待同步档名）
