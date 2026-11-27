# subagent-roles

<!-- domain: harness -->

## ADDED Requirements

### Requirement: 子线程档位按角色划分而非开发阶段

系统 SHALL 提供四个子线程角色档：`explore`（探路）、`ui-design`（原型制作与修改）、`impl-heavy` / `impl-light`（实现，按复杂度分两档）、`change-review`（审查，按 brief 分流）。

系统 SHALL NOT 提供按开发阶段命名的档位（`stage-propose` / `stage-design` / `stage-verify` / `stage-archive` / `stage-explore` / `stage-implement`）。

#### Scenario: 五个角色档存在

- **WHEN** 列举 `.pi/agents/` 下的文件
- **THEN** 存在 `explore.md` / `ui-design.md` / `impl-heavy.md` / `impl-light.md` / `change-review.md`
- **AND** 每个文件的 frontmatter 含 `name`（等于文件名去扩展名）与 `description`

#### Scenario: 阶段档已移除

- **WHEN** 列举 `.pi/agents/` 下的文件
- **THEN** 不存在任何 `stage-*.md`

#### Scenario: 实现档按复杂度二选一

- **WHEN** 一个模块需要实现
- **THEN** 涉及事务/并发/状态机的用 `impl-heavy`，其余用 `impl-light`
- **AND** 两档的 `model` 不同（heavy 用更强的模型）

### Requirement: 审查档一档多用途，由 brief 说明审查范围

`change-review` SHALL 是唯一的审查档，其审查范围（change 制品 / 代码实现 / UI 验收）由派发 brief 指定，而非由不同档位固定。

#### Scenario: 审 change 制品

- **WHEN** brief 指明审查 `proposal.md` / `specs/` / `tasks.md`
- **THEN** 该档按 openspec 制品规范逐条对照

#### Scenario: 审代码实现

- **WHEN** brief 指明审查某个模块的实现并给出 spec 条款
- **THEN** 该档逐条对照 WHEN/THEN 与实现，找「写了但没生效」

#### Scenario: 一次审多个实现

- **WHEN** brief 列出多个待审模块（同模块或前后端分开）
- **THEN** 该档可在一次派发中全部审查，逐模块给结论

### Requirement: 审查档模型按降级链切换

系统 SHALL 在 `.pi/model-chains.json` 维护审查档的模型优先顺序链，主线程在派发返回额度类错误（429/401）时 SHALL 按链切换到下一个模型重派。

#### Scenario: 首选模型可用

- **WHEN** 链首模型派发成功
- **THEN** 本次审查使用该模型

#### Scenario: 首选不可用则降级

- **WHEN** 链首模型返回 429 或 401
- **THEN** 主线程改用链中下一个模型重派
- **AND** 向用户说明降级原因

#### Scenario: 整条链耗尽

- **WHEN** 链中所有模型都返回额度错误
- **THEN** 主线程如实报错并停止重派，不静默改用其他模型

#### Scenario: 链可自行调整

- **WHEN** 用户修改 `.pi/model-chains.json` 中 `review` 数组的顺序或内容
- **THEN** 后续派发按新链执行，无需改代码

### Requirement: 子线程默认不注入 AGENTS.md 全文

派发子线程时 SHALL NOT 把 `AGENTS.md` 全文作为必读塞进 brief；SHALL 只给该角色真正需要的最小集。

#### Scenario: 审查档的最小集

- **WHEN** 派发 `change-review`
- **THEN** brief 包含待审对象（spec 条款 / diff / 测试命令）
- **AND** 不包含 `AGENTS.md` 全文，也不要求该档「先自读 AGENTS.md」

#### Scenario: 实现档的最小集

- **WHEN** 派发 `impl-heavy` 或 `impl-light`
- **THEN** brief 包含相关域的红线摘要（`constraint-digest.sh --brief --paths` 的输出原样）
- **AND** 包含该模块的 tasks 条目与验收命令

#### Scenario: 原型档的最小集

- **WHEN** 派发 `ui-design`
- **THEN** brief 包含 datamap 相关节与 `standard/frontend/{layout,prototype-content}.md` 的要求
- **AND** 不包含流程类约束（openspec 编排、归档门禁）

### Requirement: 档位改名受门禁约束

`ui-design` 与 `change-review` 两个档名 SHALL 保持不变，因为 `ui-design-gate` 的白名单常量按档名匹配。

#### Scenario: 改名会破门禁

- **WHEN** 有人把 `ui-design.md` 改名为其他名字
- **THEN** `ui-design-gate` 的 `UI_DISPATCH_ALLOWED_AGENTS` 需要同步修改，否则原型编辑会被阻断
- **AND** 因此本次改动只调整提示词内容，不改文件名
