# Tasks

> 白盒用例（分支表/边界值）见 `test-cases.md`，任务描述里的 A/B/C/D 编号指向该文件的用例行。
> 断言判据由本文件给出，子线程只做机械展开，不得改判据。

## 1. 纯函数与单测（用例先行）

- [x] 1.1 在 `.pi/extensions/tests/failure-classify.smoke.cjs` 先补 `extractFailurePaths()` 用例（`test-cases.md` B1-B8，含 `internal\admin\wire.go:97:1:` 反斜杠形态与 go vet 包锚点）；跑 `bash .pi/extensions/tests/run-harness-smoke.sh` 确认新用例红（函数尚不存在）。验证：新用例确实失败且失败原因是「函数不存在/未导出」
- [x] 1.2 在 `.pi/extensions/lib/failure-classify.ts` 实现 `extractFailurePaths(output): string[]`（双锚点：文件路径形态 + `# / FAIL <module>/<pkg>` 包锚点转目录前缀；`\` 归一为 `/`；去重；解析不出返回 `[]`；不做猜测式推断），重跑 1.1 至全绿。验证：`run-harness-smoke.sh` 中 failure-classify 段全绿
- [x] 1.3 把归属判定实现为纯函数（建议落 `lib/failure-classify.ts`：`isForeignFailure({paths, mine, foreign})`，语义 = `paths.length > 0 && paths ∩ mine = ∅ && paths ⊆ foreign`，包锚点按前缀匹配），并在同一 smoke 文件补 `test-cases.md` C1-C8 真值表用例。验证：真值表 8 行逐行断言通过（含 C6「未归属新脏文件不判外部」的保守分支）

## 2. quality-gate 接线

- [x] 2.1 `.pi/extensions/quality-gate.ts` 新增会话启动基线 `baselinePaths`（session_start 以当时脏文件集初始化一次，**turn_end 不覆盖**；session_start/session_shutdown 与 `snapshot`/`stickyFailures` 同点位重置；仅 owner 会话生效）。验证：会话内两个回合后 `baselinePaths` 仍等于启动时集合（smoke 用 mock git 返回新增文件断言）
- [x] 2.2 实现失败指纹状态机与报告分级（`test-cases.md` A1-A10）：`failureReports: Map<cmd,{diag,rounds}>`；首次/指纹变化 → 完整块；同指纹 → 单行 `⟳`；rounds ≥ 3 → 附加「未修」；转绿（曾处于失败段）→ 单行 `✓` 并清条目；会话边界清零。**`stickyFailures` 语义保持不变**。验证：quality-gate.behavior.smoke.cjs 新增 A 组行为用例全绿，且既有粘性用例不回归
- [x] 2.3 实现并发外部归因（`test-cases.md` C1-C11）：`mine = 本会话累计触发集 ∪ 本 change 归属`——**累计触发集 = 新增 module 变量 `accumulatedTriggerPaths`（`Set<string>`，每个跑过门禁的回合并入当回合 trigger，与 `baselinePaths` 同点位重置；不得用 edit.map 顶替，bash 编辑不进 edit.map）**；本 change 归属经 `lib/edit-map.ts` 新增只读查询 `latestEditMapByChange(cwd)`（按 `id in (select max(id) … group by change)` 取每个 change 最新快照）；`foreign = baselinePaths ∪ 其他 change 归属集合`。**mine/foreign 集合（含 sqlite 异步查询）须在门禁命令执行前预取完成再供 `gateLog` 使用（gateLog 是同步回调，内部不得发起异步查询）**。命中外部 → 不进 `stickyFailures`、不进 `[回归]/[中间态]` 分级（改一行 `[外部]` 文案，**同指纹会话内至多一行，后续回合静默，指纹变化重新输出**）、追加 `logPolicyDecision({action:"warn", reasonCode:"foreign-breakage", target:<cmd>})`、`gate.check` 照记失败事实。验证：smoke 断言「外部命令下回合纯对话不重跑 + 账本新增一条 foreign-breakage + 无 [回归] 前缀 + 同指纹第 2 编辑回合不重发提示行（C11）」
- [x] 2.4 外部归因的 fail-open 兜底：无绑定 change / 无 `edit.map` 记录 / 事实库不可用 / 无 git 时，判定退化为会话基线单信号，且任何异常不得影响门禁既有流程（异常吞掉 + 视同本会话失败）。验证：smoke 构造「库不可用」场景断言门禁仍正常执行、失败照旧进 sticky

## 3. suggest 取数口径

- [x] 3.1 `scripts/harness/doc-impact.sh` 新增 change 名三源解析（`--change` → `PI_SESSION_ID` 查最新 `mode.set.boundChange` → 空）与「全部 change 最新归属集合」只读查询（沿用 `ownership_paths()` 的 `sqlite3`/库存在性守卫与单引号转义，失败返回空）。验证：`doc-impact.smoke.sh` 新增 D1/D2/D3/D6/D8 用例（含特殊字符 change 名不产生 SQL 语法错误）
- [x] 3.2 `cmd_suggest()` 改为归属优先 + 三桶分列输出（本 change / 其他 change / 无归属，各桶上限 20 行 + 「另有 N 个」；回退轨显式标注「全树 diff，可能含其他会话/其他 change 的改动」；预勾选只吃本 change 桶；退出码恒 0）。验证：`test-cases.md` D4/D5/D7 用例通过，且 verify 子命令行为零变化（既有 smoke 用例不回归）

## 4. 测试

- [x] 4.1 在 `.pi/extensions/tests/quality-gate.behavior.smoke.cjs` 补行为级场景：① 首次失败完整块 → 同指纹第 2/3 回合单行（第 3 回合带「未修」）② 转绿收尾行 ③ 会话边界重置指纹（新会话同 diag 仍出完整块）④ 粘性在纯对话回合仍重跑 ⑤ 失败路径落在启动基线 → `[外部]`、不进粘性、无 `[回归]`、账本一条 foreign-breakage ⑥ 混合路径/解析不出 → 现状 ⑦ 外部同指纹第 2 个编辑回合不重发提示行（C11）。验证：`bash .pi/extensions/tests/run-harness-smoke.sh` 全绿且断言数增加
- [x] 4.2 在 `scripts/harness/doc-impact.smoke.sh` 补 suggest 用例（D1-D8；fixture 用临时 git 仓库 + 临时 `events.db` 造 `edit.map`/`mode.set` 行）。验证：`bash scripts/harness/doc-impact.smoke.sh` 退出码 0
- [x] 4.3 全量 harness smoke 回归。验证：`bash .pi/extensions/tests/run-harness-smoke.sh` 与 `bash .pi/extensions/tests/run-smoke.sh` 无新增红
- [x] 4.4 实机演练：在真实并发窗口（另一会话改动其 change 文件时）观察本会话 steer——外部失败是否标 `[外部]`、重复失败是否降为单行、上下文体积是否下降；留痕实测样本。验证：本节写入实测样本（若无法在窗口内复现并发，则以 4.1 smoke 覆盖为准并写明跳过理由）
  - 跳过理由（apply 期留痕）：本会话 pi 扩展模块在会话启动时已加载旧版 quality-gate（TS 改动不热重载），本会话 turn_end 无法体现新行为；并发窗口真实存在（同期 harden-subagent-constraint-channel 会话活跃，真树上其归属文件 8 个）但无新逻辑可观察。行为验证由 4.1 smoke 覆盖（场景 L/M/N/O/P 用真 tmp events.db + mock git 全链路断言）；实机效果留后续新会话实战观察（本 change 挂 active 状态，后续会话的 events.db 可回检 foreign-breakage 计数与同指纹重复注入下降）。

## 5. 文档

<!-- doc-impact: none(harness 机制文档不在 doc-impact 七域；pi-extensions.md 与开发执行规范属 harness 机制/流程文档，随本 change 就地同步) -->

- [x] 5.1 `docs/reference/harness/pi-extensions.md` quality-gate 节补「失败报告收敛与并发归因」小节（指纹递变规则 / 外部判定三向信号 / foreign-breakage 记账 / 保守边界），并在 spec-gate 行附近说明与归档前 `concurrent-dirty-tree` warn 的分工（一个在 turn_end、一个在归档）。验证：文档节存在且与实现一致（逐条对照 spec Scenario）
- [x] 5.2 `docs/reference/开发执行规范.md` §0.6 第 1 步与 §11 相关表述同步 suggest 新口径（归属轨优先 + 三桶 + 回退标注；`--change` 可选参数）。验证：§0.6/§11 中 suggest 描述与实际输出一致，无残留「全树预勾选」旧措辞
- [x] 5.3 把本 change 的数据证据与现场样本（67% 并发同频、91% 重复注入、2026-09-18 06:08 的 19 连击）落到 `openspec/changes/attribute-concurrent-gate-noise/explore-findings.md`（持久化给实现/评审阶段读）。验证：文件含三个数字与现场样本的复现 SQL

## 6. 验证

- [x] V1 `openspec validate attribute-concurrent-gate-noise` → 输出 valid
- [x] V2 `bash .pi/extensions/tests/run-harness-smoke.sh` → 全绿（含新增 A/B/C 组用例）
- [x] V3 `bash scripts/harness/doc-impact.smoke.sh` → 退出码 0（含新增 D 组用例）
- [x] V4 `bash scripts/harness/doc-impact.sh verify openspec/changes/attribute-concurrent-gate-noise` + `bash scripts/harness/scenario-trace.sh openspec/changes/attribute-concurrent-gate-noise` → 均退出码 0
- [x] V5 `bash scripts/harness/check-standards.sh --change attribute-concurrent-gate-noise` → 本 change 相关段零失败（并发窗口下 E 段既有欠账另行说明）
- [x] V6 `git status --short` → 本 change 变更仅含 `scripts/harness/doc-impact.sh`、`.pi/extensions/quality-gate.ts`、`.pi/extensions/lib/failure-classify.ts`、`.pi/extensions/lib/edit-map.ts`、三个 smoke 文件、`docs/reference/harness/pi-extensions.md`、`docs/reference/开发执行规范.md`、`openspec/changes/attribute-concurrent-gate-noise/`；其他脏文件为无关 change 的并发改动，不纳入本 change commit

### Scenario → 测试文件映射

| Scenario | 测试文件 |
| --- | --- |
| 首次失败输出完整块 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 同指纹持续只输出单行摘要 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 连续三回合未变化附加未修标记 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 失败转绿输出收尾一行 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 粘性重跑语义不变 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 失败路径全落在会话启动基线时判外部 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 失败路径含本会话触发文件时维持代码失败语义 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 路径不可解析时保守按本会话失败处理 | .pi/extensions/tests/failure-classify.smoke.cjs, .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 归属其他 active change 的文件被识别为外部 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 外部失败不进粘性且记 warn 归因 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 同指纹外部失败不重复提示 | .pi/extensions/tests/quality-gate.behavior.smoke.cjs |
| 归属集合存在时预勾选只看归属轨 | scripts/harness/doc-impact.smoke.sh |
| 归属集合为空时全树回退且显式标注 | scripts/harness/doc-impact.smoke.sh |
| 其他 change 归属的脏文件分列不计入预勾选 | scripts/harness/doc-impact.smoke.sh |
| change 名解析优先级 | scripts/harness/doc-impact.smoke.sh |
| sqlite3 不可用时回退且不报错 | scripts/harness/doc-impact.smoke.sh |
