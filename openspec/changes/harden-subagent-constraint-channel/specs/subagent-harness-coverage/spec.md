# subagent-harness-coverage Specification (Delta)

## ADDED Requirements

### Requirement: 子线程扩展加载配置

仓库 SHALL 在 `.pi/agents/` 维护项目侧 agent profile 声明，实现子线程的扩展加载配置 SHALL 全部落于仓库侧文件（MUST NOT 依赖修改 pi-web 包产物）：

- **实现类 profile**（供 §0.6 步骤3 派发实现/验证任务使用）SHALL 声明 `load_extensions: true`，使子线程加载 `.pi/extensions/` ambient 扩展，harness 约束（注入与记账）对子线程可达；
- **只读探索/计划类 profile**（explore/plan 等价物）SHALL NOT 声明加载扩展，保持轻量（现状行为不变）；
- pi-subagents 通道的前台派发 agent 定义（如存在）SHALL 以 frontmatter `extensions:` 显式挂载所需扩展；后台派发默认加载 ambient 扩展，SHALL NOT 被 `extensions: []` 覆盖。

profile 字段名 MUST 遵循 pi-web 的解析白名单（snake_case：`load_extensions` / `load_skills` / `tools` / `model` 等）；字段名漂移导致配置失效 SHALL 在本 change 的验证节以实测暴露而非静默。

#### Scenario: 实现档带扩展派发后约束可达

- **WHEN** 用实现类 profile（`load_extensions: true`）经 pi-web Agent 工具派发子线程，父会话已绑定活跃 change X
- **THEN** 子线程会话加载 constraint-injection 扩展，注入带 X 的完整约束块（档位基础块 + X 声明的域）
- **AND** 事实库出现该子会话的 `session.start`、`constraint.inject` 与 `mode.set source=inherit` 事件

#### Scenario: 只读探索档保持轻量

- **WHEN** 用未声明 `load_extensions` 的探索类 profile 派发子线程
- **THEN** 子线程不加载任何 `.pi/extensions/` 扩展（现状行为），零 harness 事件产生
- **AND** 该盲区为已知限制，由编排层兜底（派发任务文本带红线摘要）

### Requirement: 子线程门禁降载

quality-gate SHALL 感知子线程会话（父子关系可证：会话文件 header 带 `parentSession` 或 fork 路径可解），子线程的 turn_end 门禁 SHALL 降载执行：

- 子线程 turn_end MUST NOT 执行与主会话同额度的重命令（golangci-lint / go test / pnpm 全家桶等），默认整体跳过；
- 降载裁决 MUST 向事实库记 `policy.decision`（policy=quality-gate、action=bypass、reasonCode=child-session），MUST NOT 静默跳过；
- 主会话的 turn_end 门禁行为 MUST 保持现状（不受本降载逻辑影响）；
- 子线程代码改动的质量欠账由既有机制兜底：共享工作树的增量门禁由主会话 turn_end 承担，交付验收由 §0.6 步骤4/5 收口。

#### Scenario: 子线程 turn_end 降载并记账

- **WHEN** 带扩展子线程完成一个 turn 触发 turn_end，该会话父子关系可证
- **THEN** 本轮 lint/test 等重命令不执行，回合正常结束
- **AND** 事实库记录一条 `policy.decision`（policy=quality-gate、action=bypass、reasonCode=child-session）

#### Scenario: 主会话门禁行为不变

- **WHEN** 主会话（无父子关系）turn_end 触发
- **THEN** 门禁命令照常执行，MUST NOT 出现 child-session 降载记账

### Requirement: 通道行为矩阵文档化

`docs/reference/harness/pi-extensions.md` SHALL 记录子线程通道行为矩阵，覆盖四象限：pi-web × 前台/后台、pi-subagents × 前台/后台，每格注明「扩展是否加载 / 约束是否可达 / 门禁行为」，并登记已知限制（pi-web 子线程 `noContextFiles` 恒真——AGENTS.md 不自动加载，靠派发任务文本兜底；pi-subagents 前台默认盲区）。矩阵 MUST 附本 change 的实测依据（事件库/转录证据）。

#### Scenario: 矩阵文档可检索

- **WHEN** 在 `docs/reference/harness/pi-extensions.md` 中检索「子线程」通道矩阵
- **THEN** 存在四象限矩阵小节，含 pi-web 与 pi-subagents 两通道前后台行为与已知限制条目
