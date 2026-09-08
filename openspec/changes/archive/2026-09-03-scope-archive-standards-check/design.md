## Context

`check-standards.sh` 的 F 段当前无条件遍历所有 active change；spec-gate 调用该脚本时也未传归档目标。因此一个无关 change 的 doc-impact 中间态会阻断目标 change 的归档。

## Goals / Non-Goals

**Goals:**
- 归档门禁只把目标 change 的 doc-impact 作为阻断条件。
- 保持仓库级标准检查和人工全仓巡检的现有覆盖范围。
- 缺失或无效目标必须显式失败，不能退化为静默全仓扫描。

**Non-Goals:**
- 不放宽目标 change 的 doc-impact、Scenario 映射或任务结构门禁。
- 不改变 `--force` / `SPEC_GATE_BYPASS` 语义。
- 不改变 doc-impact 的启发式、声明格式或其他 active change 本身的验收状态。

## Decisions

### D1：`check-standards.sh` 增加可选 `--change <name>`

参数仅收窄 F 段的 active-change 遍历为指定目录；A-E、G-H 仍是仓库级检查。无参数保留全仓行为，供人工例行巡检和现有调用使用。未知参数、缺少值和不存在的 change 都失败，避免拼写错误误放行。

### D2：spec-gate 显式传递已经解析的归档 change

spec-gate 已在归档预检中解析 change 名称与目录，直接把该名称作为 `--change` 参数传给 standards 脚本。这样 F 段与独立 doc-impact、tasks、scenario-trace 检查都围绕同一目标，避免依赖当前档位或工作区的推断。

### D3：用脚本级回归覆盖两种调用形态

为 `check-standards.sh` 的参数解析与 F 段范围增加可脱离完整工作区的 shell smoke/fixture；spec-gate smoke 断言实际命令携带 `--change`。同时保留一条无参数全仓行为断言。

## Risks / Trade-offs

- [范围参数被误用而漏查其他 active change] → 无参数默认不变；归档前的目标门禁仍保留所有仓库级段与目标自身对账。
- [名称解析不一致] → 只接受 change basename，不接受任意目录路径；不存在即失败。
- [脚本调用方兼容性] → 参数可选，现有无参调用不需要修改。

## Migration Plan

1. 实现参数解析与 F 段目标筛选，补脚本 smoke。
2. 修改 spec-gate 传入归档 change，补 smoke。
3. 更新归档规范，运行目标范围、全仓模式和 spec-gate smoke。

回滚时移除参数调用即可恢复当前全仓归档前扫描；不涉及数据迁移或持久化状态。
