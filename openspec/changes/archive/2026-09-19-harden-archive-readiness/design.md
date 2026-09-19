# Design — harden-archive-readiness

## Context

spec-gate 归档门禁四项 block 检查中三项已是独立脚本（doc-impact.sh verify / check-standards.sh / scenario-trace.sh），检查③（tasks.md 尾三节 + doc-impact 声明标记）是 spec-gate.ts 内联 TS 逻辑（`checkTasksMd`）。门禁 block reason 已携带脚本输出尾部 20 行，信息不缺，缺的是 archive 时刻之前的同口径自查入口（见 proposal.md - Why）。

## Goals / Non-Goals

**Goals**

- 归档前一次看清四项全貌的自查入口，与门禁判定同一事实源
- 门禁 block 文案引导「自查全绿再归档」，替代试探式逐项重试
- `m7.block_recur_groups` 5 → ≤2（7 天观察窗）

**Non-Goals**

- 不改四项检查的判定逻辑/阈值/豁免机制（`--force` / `SPEC_GATE_BYPASS=1` 原样）
- 不把 readiness 做成新门禁：它不被 spec-gate 消费、不产生 policy.decision、无阻断语义
- 不搬检查④'（UI 验收证据）与检查⑤/⑤'（warn 级）进自查

## Decisions

### D1：检查③抽出 `scripts/harness/check-tasks-tail.sh`，单一事实源

尾三节判定从 spec-gate.ts 内联函数抽为独立 bash 脚本，spec-gate.ts 改为 `runScript(["scripts/harness/check-tasks-tail.sh", changeDir])`——四项检查全部脚本化，readiness 聚合调用同一批脚本，口径天然一致。

- 备选 A：readiness.sh 内 bash 复刻尾三节判定 → 拒绝，TS/bash 双实现必然漂移，违背 spec「同一事实源」
- 备选 B：readiness 只聚合三个现成脚本 → 拒绝，四项口径不齐，「自查全绿」就不可信
- bash 保真要点：TS `^##\s+\d+\.\s*<节>`（m flag）→ POSIX ERE `^##[[:space:]]+[0-9]+\.[[:space:]]*<节>`；`<!-- doc-impact:` 标记用 `grep -F` 固定串；缺失清单文案与 TS 版逐字一致
- spec-gate.ts 同步删除 `checkTasksMd` / `TAIL_SECTIONS` / `DOC_IMPACT_MARKER`（无其它引用）

### D2：`archive-readiness.sh` 是纯消费者

逐项顺序执行四个脚本、按各自退出码裁决、汇总输出；不包装判定、不改写输出（透传尾部 20 行，与门禁 `runScript` 的 tail 口径一致）。参数：change 名（字符集同 `extractChangeName`：`[A-Za-z0-9][A-Za-z0-9._-]*`），容忍 `openspec/changes/` 前缀（剥离）。退出码：0 全绿 / 1 有红项 / 2 用法错误或 change 目录不存在。

### D3：readiness 明示覆盖边界

UI 验收证据（检查④'）是 TS 纯函数 + 快照读取（`lib/ui-design-gate`），搬 bash 工程量与收益不匹配；readiness 输出末尾固定一行「UI 验收证据项不在自查范围（仅归档门禁校验）」，不静默省略。对 ui-impact:none change 无实际影响（N/A 恒过）。

### D4：block 文案最小增量

`buildBlockReason` 「修复指引」列表前插一行：「先跑 `bash scripts/harness/archive-readiness.sh <name>` 自查全绿再重试归档（一次看清全部缺口，勿补一项就试一次）」。既有各行指引与豁免说明原样保留。

## Risks / Trade-offs

- [尾三节 grep 语义迁移偏差（如 `### 2.x 测试` 子节误命中）] → `check-tasks-tail.smoke.sh` 覆盖正/负/畸形 case（含从 TS smoke 迁移的判定样本）；spec-gate.smoke.cjs 既有 case 回归行为不变
- [readiness 多跑一遍脚本增加归档前耗时] → 四脚本均为只读秒级，且省下的是 N 次门禁重试的完整四项执行
- [readiness「全绿」后归档仍被 ④' 拦（minor/major UI change）] → D3 明示边界；实测 UI 证据 block 以用户审批等待为主（ui-approval-pending），非重试噪声
- [agent 不看指引照旧试探] → 文案 + §11 规范双通道；7 天后 `m7.block_recur_groups` 回检，不降再议强干预（如 block 后冷却提示）

## Migration Plan

1. 新增 `check-tasks-tail.sh` + `archive-readiness.sh`（独立可用，零破坏）
2. spec-gate.ts 切检查③到新脚本 + 文案增强（四项判定结果不变）
3. smoke 与文档同步
4. 回滚：git revert 单提交即可，无数据/状态迁移
