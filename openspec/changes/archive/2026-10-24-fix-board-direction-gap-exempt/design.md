# Design: fix-board-direction-gap-exempt

## Context

`computeGlobalBestBoards`（`semantic_board_matching.go:510`）当前以 `bestCos-cos < 1e-9` 判等构建豁免集合，只覆盖并列最优。真库实证（proposal Why）表明窄带分布下真家常排 2-5 名，豁免够不着。本 change 在既有三层判定骨架内做**最小侵入扩围**：豁免集合的判等阈值从常量 1e-9 换成可配置 margin（默认 0.10）。

绝对门/相对门/floor/composite_hit 豁免/embedding 缺失语义全部不动。消费方（boardhealth ghost_matches、日报 537 行过滤、前端默认隐藏）零代码改动，随标记分布变化自动受益。

## Goals / Non-Goals

**Goals**
- 豁免集合覆盖「与全库最优差值 < margin」的挂载，清除窄带冤案（实证：全库 ghost 553→166，日本新闻 21→1）。
- margin 可配置可关闭（≤0 回退现行为），默认 0.10。
- 一次性重算存量标记（含 82 个无 identity embedding 的陈旧标记与时间漂移标记）。

**Non-Goals**
- 不动挂载判定管道（hit_rate/max_sim/weighted 门槛）——另立 change。
- 不动板 embedding 生成文本源（板 description 空导致的语义重叠另议）。
- 不动 boardhealth 评分口径与 LLM prompt。
- 不自动卸载任何挂载行（mismatch 仍只标记）。

## Decisions

### D1. 豁免集合判等阈值参数化（改 computeGlobalBestBoards 而非新增后处理）
- **结论**: `computeGlobalBestBoards` 增配 `margin` 参数，集合条件改为 `bestCos-cos < max(margin, 1e-9)`；函数更名 `computeGapExemptBoards` 以反映语义；`directionMismatch` 闭包与 `applyDirectionRelativeGate` 的跳过集合同步换用新函数结果。margin ≤ 0 时 `max(0, 1e-9)=1e-9`，行为与现版逐位一致。review M2 修复：`applyDirectionRelativeGate` 的跳过判定补与闭包同式的 floor 条件（被 floor 收回豁免的挂载回退双门，spec「回退双门判定」字面兑现）。
- **落点**: `backend-go/internal/tagmanagement/service/board/semantic_board_matching.go:computeGapExemptBoards`
- **取舍**: 备选是在 directionMismatch 闭包内单独算差值做二次豁免——被否：豁免逻辑会散落两处（闭包 + 相对门跳过集合），且 computeGlobalBestBoards 的「全库参照」职责正好承载；单点改函数签名最小且可测。

### D2. 配置键与默认值
- **结论**: 新键 `semantic_board_match_direction_gap_exempt_margin`，默认 0.10，≤0 关闭；走 `LoadConfig` 既有 switch 白名单 + margin 类 parse 语义（负→0、>1/非法拒绝保持默认）——完全复刻 `semantic_board_match_direction_floor` 的落地模式，且同 floor/relative_margin 一样为 **DB-only 键**（不进 matching-config API 白名单；热生效依赖 config 缓存 TTL 或任意 config API 写触发的失效，回滚时注意 ≤5min 时滞）。
- **落点**: `backend-go/internal/tagmanagement/service/board/semantic_board_matching.go:LoadConfig`
- **取舍**: 备选默认 0.08/0.15——0.08 日本新闻同样达 1 但美国新闻残留 70（vs 46）；0.15 漏放风险实测增大（「李在明」类小 gap 真串味）；0.10 为实证甜点（proposal Why 扫描表）。

### D3. 存量重算走既有 rematch-all，不做定向清洗
- **结论**: 部署后手动触发既有 rematch-all 全量重算（3448 tag，上个 change 4.1 同口径），陈旧标记（82 个无 embedding）随全删全写语义自然清除；不写 cmd/admin 定向工具。
- **落点**: `backend-go/internal/tagmanagement/handler/board_match_handler.go:rematchAll`
- **取舍**: 备选 cmd/admin 定向 UPDATE mismatch 行——被否：replaceTopicTagBoardLabels 本就是全删全写，rematch-all 一条路已覆盖且免新工具；直接 SQL UPDATE 绕过判定管道有口径漂移风险。

### D4. 与 fix-board-ghost-false-positive 的归档衔接
- **结论**: 该 change 未归档（4.2 快照核对、6.5 归档检查未完成）。本 change 部署重算后，其 4.2 验收（ghost 回落个位数）才有达成的数据条件——两 change 按序归档：fix-board-ghost-false-positive 先行（4.2 留痕注明实际达成依赖本 change 重算），本 change 随后；delta spec 已按「三层 + gap 扩围」最终形态书写，顺序合并无冲突。
- **落点**: 无代码落点：openspec change 目录归档顺序，因为 纯流程衔接无代码产物
- **取舍**: 备选把本 change 并回 fix-board-ghost-false-positive——被否：该 change 已过 review 阶段且实证口径独立，并入会污染其账本。

## Risks / Trade-offs

- [margin 过大漏放小 gap 真串味] → 默认 0.10 锚定实证；配置键可下调；「李在明」类个案（gap 0.066）接受为代价（真家板也会以弱分挂它，体检标签级分布仍能暴露）。
- [科技巨头板残留 ~96 个标记（gap>0.10 的次优挂载）评分改善有限] → 明确为 Non-Goal（构成治理另立 change）；本 change 后该板 ghost 21%→与生成式AI重叠主导，属语义真实。
- [磁铁板本身（中国国内新闻）的挂载构成漂移不在治理范围] → 同上，方向校验只管「挂错没收」，不管「该挂没挂」。
- [两未归档 change 同 Requirement 的 delta 合并顺序] → D4 定序；归档前 `openspec archive --dry-run` 验证合并结果。

## Migration Plan

1. 合并代码（判定扩围 + 配置键）→ 2. 部署重启（LoadConfig 白名单自动带默认 0.10，无需预置键）→ 3. 手动 rematch-all 全量重算 → 4. SQL 核对（ghost 分布 vs 实证预期 166±10%）→ 5. 次日体检快照与日报核对（4.2 口径）。
回滚：ai_settings 置 `semantic_board_match_direction_gap_exempt_margin=0` 热关闭回退现行为，再 rematch-all 即恢复（无需回滚代码）。

## Open Questions

（无）
