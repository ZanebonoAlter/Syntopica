# subagent-harness-coverage 规格增量

## MODIFIED Requirements

### Requirement: 子线程门禁降载

quality-gate SHALL 感知子线程会话（父子关系可证：会话文件 header 带 `parentSession` 或 fork 路径可解），子线程的 turn_end 门禁 SHALL 降载执行：

- 子线程 turn_end MUST NOT 执行与主会话同额度的重命令（golangci-lint / go test / pnpm 全家桶等），默认整体跳过；
- 降载裁决 MUST 向事实库记 `policy.decision`（policy=quality-gate、action=bypass、reasonCode=child-session），MUST NOT 静默跳过；记账粒度 SHALL 为**每会话至多一条**——该会话首个降载回合写入，后续回合 MUST NOT 重复写；跨 `/reload` 与进程重启的判重 MUST 以事实库既有记录为准，MUST NOT 只依赖进程内状态；
- 主会话的 turn_end 门禁行为 MUST 保持现状（不受本降载逻辑影响）；
- 子线程代码改动的质量欠账由既有机制兜底：共享工作树的增量门禁由主会话 turn_end 承担，交付验收由 §0.6 步骤4/5 收口。

#### Scenario: 子线程 turn_end 降载并记账

- **WHEN** 带扩展子线程完成一个 turn 触发 turn_end，该会话父子关系可证，且事实库中该会话尚无 child-session 记账
- **THEN** 本轮 lint/test 等重命令不执行，回合正常结束
- **AND** 事实库记录一条 `policy.decision`（policy=quality-gate、action=bypass、reasonCode=child-session）

#### Scenario: 同会话后续回合不重复记账

- **WHEN** 同一子线程会话已有 child-session 记账后继续完成更多 turn
- **THEN** 后续 turn_end 仍整体降载（重命令不执行），但 MUST NOT 追加新的 child-session 记账行

#### Scenario: reload 后不重复记账

- **WHEN** 子线程会话期间执行 `/reload`（扩展模块状态重建）后再次触发 turn_end
- **THEN** 判重仍以事实库既有记录为准，MUST NOT 追加第二条 child-session 记账

#### Scenario: 主会话门禁行为不变

- **WHEN** 主会话（无父子关系）turn_end 触发
- **THEN** 门禁命令照常执行，MUST NOT 出现 child-session 降载记账
