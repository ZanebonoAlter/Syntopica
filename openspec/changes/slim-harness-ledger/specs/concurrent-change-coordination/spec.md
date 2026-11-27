# concurrent-change-coordination 规格增量

## MODIFIED Requirements

### Requirement: 文件归属地图落库

harness 层 SHALL 将"归属某 change 的会话累计编辑过的仓库文件路径"聚合为 `edit.map` 事件落事实库：quality-gate 在 turn_end 检出会话内增量路径时，若会话已绑定 change（stage.set 的 boundChange，过渡期双读 mode.set），SHALL 将本回合新增/变化路径合并进该 change 的归属集合（事件 change 列为绑定的 change，payload 含累计路径集合）。合并后集合与该 change 已有快照**逐项全等**时 MUST NOT 追加新快照（未变化不落库，避免同一集合被每回合重复记账）。无绑定 change 的会话路径 SHALL NOT 计入任何 change 归属。同一文件被两个及以上 change 的归属集合同时包含时，concurrency-status 输出中 MUST 将该文件标记为冲突文件。

**工具自管文件剔除（采集侧）**：openspec CLI 的归档移动产物与元数据（`openspec/changes/archive/**`、`openspec/changes/*/.openspec.yaml`）SHALL NOT 计入任何 change 的归属集合——这些路径变化源于工具操作而非 change 的实际工作范围，混入归属集合会污染 doc-impact 对账与并发态势对照。仓库级共享文档（`AGENTS.md` 等）SHALL 保留在归属集合中（并发冲突感知需要），其对 doc-impact 的影响在 verify 消费侧由黑名单过滤（见 doc-impact-gate capability）。

#### Scenario: 编辑路径按绑定 change 聚合

- **WHEN** 会话绑定 change `foo` 且本回合编辑 `backend-go/internal/dataenrichment/a.go`，另一会话绑定 change `bar` 编辑 `backend-go/internal/admin/b.go`
- **THEN** 事实库中 change `foo` 与 `bar` 的归属集合分别含各自文件，互不串扰

#### Scenario: 路径集合未变化时不追加快照

- **WHEN** 会话绑定 change `foo`，本回合编辑的文件全部已在 `foo` 的既有归属集合中
- **THEN** MUST NOT 追加新的 `edit.map` 行；消费方取最新一条仍是完整集合

#### Scenario: 归档移动产物不计入归属

- **WHEN** 会话绑定 change `foo` 时执行 `openspec archive <other-change>`，openspec CLI 移动/改写 `openspec/changes/archive/<other-change>/**`（含 `.openspec.yaml`）
- **THEN** 这些路径不出现在 `foo` 的归属集合中（采集侧剔除），后续 verify 消费时无需再过滤该来源

#### Scenario: 同文件双 change 触碰标记冲突

- **WHEN** change `foo` 与 change `bar` 的归属集合均含 `backend-go/internal/app/router.go`
- **THEN** concurrency-status 输出中该文件带冲突标记

#### Scenario: 无档会话不计入归属

- **WHEN** 未绑定 change 的会话编辑 `front/app/a.vue`
- **THEN** 该路径不出现在任何 change 的归属集合中
