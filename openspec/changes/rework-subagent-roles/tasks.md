# Tasks

## 1. 删掉上一轮的阶段档

- [x] 1.1 删 `.pi/agents/stage-{explore,propose,design,implement,verify,archive}.md`（六个）
- [x] 1.2 删 `scripts/harness/stage-agent.sh` + `stage-agent.smoke.sh`（不再按阶段选档）

## 2. 新建实现档

- [x] 2.1 `impl-heavy.md`：复杂业务逻辑（事务/并发/状态机），deepseek-v4-pro / max
- [x] 2.2 `impl-light.md`：常规改动，deepseek-v4.1-flash / high
- [x] 2.3 两档正文都写明判断标准，且 `impl-light` 要能「发现其实是复杂逻辑就停下报告」

## 3. 改造既有档提示词

- [x] 3.1 `change-review.md` → 通用审查档：四类审查（制品/代码/UI/跨模块）+ brief 三件事 + 报告格式
- [x] 3.2 `change-review.md` 加 `write`（要写 review.md，上一轮 stage-verify 就栽在这）
- [x] 3.3 `ui-design.md` → 原型制作与修改档：工作循环、四态、占位诚实、危险操作确认、双视口
- [x] 3.4 两者档名都不改（`ui-design-gate` 白名单按名匹配）

## 4. 降级链

- [x] 4.1 `.pi/harness/model-chains.json`：按角色分链（review / impl-heavy / impl-light / explore / ui-design）
- [x] 4.2 review 链 = glm-5.4 → gpt-6.2-sol → deepseek-v4.1-flash
- [x] 4.3 链中所有模型 id 经 `pi --list-models` 校验存在

## 5. 配置同步

- [x] 5.1 `.pi/settings.json` 的 `agentOverridesByProvider` 改为角色档（五套 provider）
- [x] 5.2 `ui-design-gate.ts` 白名单：去掉 stage-*，加 impl-heavy / impl-light

## 6. 文档

- [x] 6.1 `AGENTS.md` 阶段档段 → 角色档表 + 降级链 + 不给 AGENTS.md 纪律
- [x] 6.2 `开发执行规范.md` §主线程纪律 阶段映射表 → 角色映射表
- [x] 6.3 `pi-extensions.md` 同步

## 7. review

- [x] 7.1 派 `change-review` 审本 change（它自己写了 review.md —— 上一轮 stage-verify 栽在无 write，这档已修）
- [x] 7.2 处置 9 条（2 High：R-1 白名单反转主 spec 已回退、R-2 走链必须显式传 model 已写死）

## 8. 归档

- [x] 8.1 九项门禁全绿
- [ ] 8.2 执行归档命令
- [ ] 8.3 `openspec validate --specs` 全过

## 9. 测试

| 验证点 | 方法 | 期望 |
| --- | --- | --- |
| 五档注册 | `discoverAgents(cwd,'both',provider)` | explore/ui-design/impl-heavy/impl-light/change-review 均有 model |
| 阶段档已删 | `ls .pi/agents/stage-*` | 无匹配 |
| provider 跟随 | 分别传 commandcode / zai-coding-cn | impl-heavy 分别是 deepseek-v4-pro / glm-5.4 |
| 降级链可读 | `python3 -c "json.load(...)"` | 五个角色链齐全 |
| 链中模型有效 | `pi --list-models` 比对 | 无无效 id |
| 白名单无漂移 | `run-harness-smoke.sh` E5/E6 | 全绿 |
| AGENTS.md 不注入 | 派 impl-light 问它有没有读 | 回答没读 |
| 旧档回归 | 派 explore / change-review | 正常完成 |

## 10. 文档

<!-- doc-impact: harness=AGENTS.md 与开发执行规范 的子线程档位段改写; dev=模型链配置约定 -->

- `AGENTS.md`（6.1）
- `docs/reference/开发执行规范.md`（6.2）
- `docs/reference/harness/pi-extensions.md`（6.3）

## 11. 验证

- [x] 11.1 §9 八项已过（注册 / provider 跟随 / 链有效 / 白名单 / 不读 AGENTS.md）
- [x] 11.2 harness smoke 77 项全绿（唯一红项 `spec-gate-smoke:block-target-only-names` 已登记台账，非本 change 引起）

## 12. 验收

- [x] A 五个角色档覆盖用户的真实流程（explore / 原型 / 实现两档 / 审查）
- [x] B 审查范围靠 brief 不靠分档（本轮 review 一次审了 9 项，覆盖制品+配置+文档+实测）
- [x] C 链已配好并校验模型 id；降级动作由主线程执行（本轮未撞 429，逻辑见文档表）
- [x] D 已实测：派 `impl-light` 问它是否读了 AGENTS.md，回答「没有读」并给出理由；七个档正文均已写明
- [x] E **implement → review 循环走通一次**：本 change 自身的配置改动 → 派 `change-review` → 9 条发现 → 修复 → 复检全绿（同模块实现后立即派审查，brief 给 spec 条款；这是流程的高频循环，必须验一次而不是只看文档）
- [x] F `impl-*` 不在 `UI_DISPATCH_ALLOWED_AGENTS`（grep 确认为 0；白名单恢复 4 项）（它们有写能力，按 `ui-design-workflow` spec 应被拦）
