<!-- complexity: simple -->
<!-- ui-impact: none -->

## Why

`openspec validate --specs` 存量 13 个 capability 失败（101 项中 13 红），全部为历史格式债：缺 Purpose/Requirements 节骨架、Requirement 缺 Scenario、delta 头残留、设计文档型 spec 无 requirement 结构。存量红噪声会淹没新引入的真问题（本次归档 sync 时验证已受干扰），且当前无任何门禁拦截主 spec 写坏。

## What Changes

- **A 组骨架修复（3）**：branding 加 title/Purpose/Requirements 头（内容已含 Scenario）；lint-zero-debt 的 `## ADDED Requirements` delta 头换主 spec 结构；thread-lineage（DEPRECATED 占位）补 Requirements 节含退役声明 requirement。
- **B 组混合修复（1）**：theme-system 加 title/Purpose，Requirements 头插到首个 Requirement 前（已有 6R/9S 保留；theming.md 是权威源，spec 只补结构不动内容）。
- **C 组加壳（3）**：settings-workspace / unified-dialog / unified-form-controls（纯设计文档、0 requirement、全部零外部引用的孤儿）加 title/Purpose + 提炼 1~2 条核心 requirement（带 Scenario，锚定已有实现行为），原设计内容节保留为自由节。
- **D 组补 Scenario（6）**：board-concept-management（含两条 DEPRECATED requirement 处置）/ board-management-api / match-detail-ondemand / detective-wall×3——为缺 Scenario 的 Requirement 逐条补 `#### Scenario`（WHEN/THEN 从代码现状写，不发明新行为）。
- **防增量**：check-standards.sh 新增 I 段——`openspec validate --specs` 全量零失败（存量清零后转为真护栏；归档门禁 ② 挂 check-standards，新坏主 spec 会被 block）。

## Capabilities

### New Capabilities

（无——纯格式修复，`.openspec.yaml` 已声明 `skip_specs: true`，不产生 delta spec）

### Modified Capabilities

（无——修复的是主 specs 的格式完整性，不改变任何行为契约语义）

## Impact

- `openspec/specs/`：13 个 spec.md 格式修复（零外部引用链破坏，已核实 5 个 A/C 类孤儿无引用）
- `scripts/check-standards.sh`：+I 段（validate --specs 校验）
- 验收：`openspec validate --specs` 101/101 全绿；check-standards 含 I 段零失败
