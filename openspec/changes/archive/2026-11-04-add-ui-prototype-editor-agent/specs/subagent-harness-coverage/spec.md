## MODIFIED Requirements

### Requirement: 子线程扩展加载配置

仓库 SHALL 在 `.pi/agents/` 维护项目侧 agent profile 声明，子线程的扩展加载配置 SHALL 全部落于仓库侧文件（MUST NOT 依赖修改 pi-web 包产物）：

- **实现类 profile**（供 §0.6 步骤3 派发实现/验证任务使用）SHALL 声明 `load_extensions: true`，使子线程加载 `.pi/extensions/` ambient 扩展，harness 约束（注入与记账）对子线程可达；
- **原型编辑类 profile**（仅在当前 change 的 `ui-prototype/**` 与 UI 规划制品内写入）SHALL 声明 `load_extensions: true`，使子线程受 UI 门禁的写入侧校验逐个裁决；其经 `edit`/`write` 工具的写入 MUST NOT 越出当前 change 目录；shell 类通道不在本门禁覆盖范围（既有边界，见 design D4）；
- **只读探索/计划类 profile**（explore/plan 等价物）MAY 声明加载扩展——项目覆盖档自 2026-10-13 起已声明 `load_extensions: true`（约束注入对只读档同样有价值，且实测 `<skills>` 节与约束注入块可达），保持现状不变；
- pi-subagents 通道的前台派发 agent 定义（如存在）SHALL 以 frontmatter `extensions:` 显式挂载所需扩展；后台派发默认加载 ambient 扩展，SHALL NOT 被 `extensions: []` 覆盖。

profile 字段名 MUST 遵循 pi-web 的解析白名单（snake_case：`load_extensions` / `load_skills` / `tools` / `model` 等）；字段名漂移导致配置失效 SHALL 在本 change 的验证节以实测暴露而非静默。

#### Scenario: 实现档带扩展派发后约束可达

- **WHEN** 用实现类 profile（`load_extensions: true`）经 pi-web Agent 工具派发子线程，父会话已绑定活跃 change X
- **THEN** 子线程会话加载 constraint-injection 扩展，注入带 X 的完整约束块（档位基础块 + X 声明的域）
- **AND** 事实库出现该子会话的 `session.start`、`constraint.inject` 与 `mode.set source=inherit` 事件

#### Scenario: 只读探索档保持轻量

- **WHEN** 用探索类 profile 派发子线程
- **THEN** 该 profile 的扩展加载配置以仓库侧文件为准；未声明 `load_extensions` 的探索类 profile 不加载任何 `.pi/extensions/` 扩展，零 harness 事件产生
- **AND** 未加载扩展的盲区由编排层兜底（派发任务文本带红线摘要）

#### Scenario: 原型编辑档带扩展派发后写入受门禁校验

- **WHEN** 用原型编辑类 profile（`load_extensions: true`）派发子线程，父会话绑定 UI 合同未就绪的 change X
- **THEN** 子线程加载 UI 门禁扩展，其写入目标越出 `openspec/changes/X/` 时被阻止并记 block
