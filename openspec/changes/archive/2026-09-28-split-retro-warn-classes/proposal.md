<!-- complexity: simple -->
<!-- ui-impact: none -->

## Why

2026-09-27 定期体检发现 harness-retro ④ 段「软提醒失效」连续输出**全是误报**：本周 3/3 失效组（`concurrent-mixed` 239 次 / `concurrent-dirty-tree` 16 次 / `foreign-breakage` 10 次）**全部是情报型 warn**——它们是 quality-gate 失败归属三态化的记账副产物（告知"这个红不全是你弄的，修自己那份就行"，源码注释明说 mixed"进粘性不催修"），本来就不期待 agent 的服从动作。数据佐证：239 次 concurrent-mixed 中 45% 来自 `decouple-backend-domains` 单个 session（跨域重构结构性撞车），54% 集中在 09-26 一天，与 gate-lock-held 194 条 Top session 完全重合——都是"多 change 并发"同一事实的投影。

后果：④ 段标题写「提醒被反复无视」，把归属情报误读成"催了 239 次没用"，既制造假信号（用户据此可能误升 block，把并发状态的客观归属当违规罚），又淹没对**真纠正型提醒**（`full-go-test` / `fuzzy-model-resolve` / `acceptance-wording` / `ui-impact-missing` 等，期待行为改变）的监督力。

根因：`harness-retro.sh` 的 soft 聚合（`warns` CTE → `soft` json_object）对**所有** `policy+reasonCode` 的 warn 一视同仁，没有语义分类。选型讨论（2026-09-28，用户拍板）：升 block 会打断合法的跨 change 并行工作流；改 quality-gate 触发降频有 overfit 风险（107 次/单 session 是跨域重构的结构性特征，常态小 change 并行下现有行为合理）；**改 retro 报告口径**（本 change）——指标对齐后"239 次无视"的假信号自然消失，并发频率另由情报型计数趋势观察。

## What Changes

- `scripts/harness/harness-retro.sh`：
  - 新增情报型 reasonCode 白名单常量（`concurrent-mixed` / `concurrent-dirty-tree` / `foreign-breakage`——语义=归属/状态告知，不期待服从动作；与 harness-facts reasonCode 词汇表同步演进）；
  - soft 聚合按白名单拆分类：`soft.items`（失效名单）只含纠正型组；情报型 warn 单列汇总（分组计数一行，不进失效判定、不受 `--warn-threshold` 影响）；
  - ④ 段渲染改双层：标题注明「纠正型」，失效名单下方加一行「情报型 warn（归属/状态告知，不计失效）」汇总；
  - metrics 键：`soft.stale_groups` 语义收窄为纠正型失效组数（键名不变，保基线连续），新增 `soft.warn_total_corrective` / `soft.warn_total_informative` / `soft.informative_groups`；`soft.warn_total` 保留为全量总和（兼容旧基线）；usage 文案同步。
- `scripts/harness/harness-retro.smoke.sh`：A1 断言同步新标题；新增 fixture 场景——同库混入情报型（≥阈值）与纠正型（≥阈值）warn，断言纠正型进失效名单、情报型只计数不进名单；既有 ④ 段阈值边界/低于阈值断言回归。
- `.agents/skills/harness-retro/SKILL.md`：④ 段解读表（看什么/信号/改进方向按两型分别表述）+ 改进项判据的指标名列表补新键。
- `docs/reference/harness/pi-extensions.md`：变更溯源表加一行（不动既有历史叙述）。

不改动：quality-gate 等扩展的记账行为与 reasonCode 词汇（纯消费侧口径调整）；③ 段 fail-open 口径；基线快照结构（仅新增键，旧基线读入不报错、比对按既有缺键处理）。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `harness-retro-loop`：「软提醒失效判定」Requirement 增加分类 SHALL——统计与失效名单仅覆盖纠正型 warn，情报型（白名单 reasonCode）单列计数不进失效判定；「报告分段与可回检指标」Requirement 的 ④ 段描述同步（④ 软提醒失效（纠正型）+ 情报型单列）。

## Impact

- **代码**：仅 `scripts/harness/harness-retro.sh` / `harness-retro.smoke.sh` 与两份文档；不触碰 `.pi/extensions/`、不写 events.db（报告只读边界不变）。
- **部署后影响**：下次起 retro 报告 ④ 段失效名单预期从 3 组降为 0（本周数据全为情报型），`concurrent-mixed` 等改以情报型计数趋势呈现；并发混跑的**记账与 [并发] 提示行为完全不变**（只是不再被报告误读为"催修无效"）。
- **需要的手动操作**：无。体检基线快照 `.pi/harness/retro-baseline.json`（09-27 已落）含旧语义键，首次用 `--baseline` 比对时新键按"无基线"提示，属预期。
- **旧数据降级**：无——账本不重写，口径变化只影响报告呈现与 metrics 键语义（`soft.stale_groups` 变小是口径修正不是指标恶化，回检时以本 change 归档日为语义分界）。
