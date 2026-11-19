## MODIFIED Requirements

### Requirement: 归档门禁集成（spec-gate 第五项 block 检查）

`openspec archive` 前置门禁 SHALL 在其他适用的 block 检查之外执行：`bash scripts/harness/review-trace.sh <changeDir>` 退出码 0。该项与其他检查各自独立判定、任一失败即阻断归档，失败输出与修复指引并入既有 reason 格式；豁免放行文案（`--force` / `SPEC_GATE_BYPASS=1`）SHALL 同步枚举新增检查项。逃生口沿用既有机制不新增；存量 in-flight change 不豁免。

#### Scenario: 归档时零痕迹阻断

- **WHEN** agent 执行 `openspec archive <change>` 且其他适用检查通过而 review-trace.sh 退出码非 0
- **THEN** 归档被阻断，reason 含缺痕迹说明与 review.md 建账 / waived 声明两条修复路径

#### Scenario: 豁免放行留痕

- **WHEN** review-trace.sh 退出码非 0 但以 `--force` 或 `SPEC_GATE_BYPASS=1` 归档
- **THEN** 归档放行，warning 文案枚举含 review 痕迹检查未执行的留痕说明

#### Scenario: 自查脚本同口径聚合

- **WHEN** 运行 `scripts/harness/archive-readiness.sh <change>` 自查
- **THEN** 输出含 review 痕迹检查项（硬检查语义，见 archive-readiness capability），判定实现与门禁同一脚本事实源
