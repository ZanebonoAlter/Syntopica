# harness-quick-wins

<!-- complexity: complex -->
<!-- 声明依据：成功采样记账为三字段状态机（consecOk/lastWasFail/everGreen），白盒分支表见 test-cases.md -->

> 纯工具链 change（pi 扩展 / lint 脚本 / 文档），无 constraint-domains 声明。
> 数据依据：2026-08-23 → 09-01 事实库复盘（17,779 条事件，见探索结论）。

## Why

10 天事实库实测账目暴露四项"改动小、回报大"的浪费：

1. **pnpm lint 是门禁墙钟黑洞**：`eslint .` 全量扫描均耗 22.3s，1455 次成功运行累计 ≈9 小时，占门禁总墙钟 65%（quality-gate.ts 注释自认"22.3s 黑洞"但命令未加 `--cache`）。
2. **后端门禁三命令串行**：golangci-lint → go vet → go build 顺序 `await`，每回合 ≈5.4s+，三者相互独立可并行。
3. **gate.check ok=1 全量记账写放大**：14,437 条 gate.check 占事件总量 81%，成功运行诊断价值趋零但每条全量入库（10 天 events.db 6.3MB + 4.2MB WAL 未 checkpoint）。
4. **同根因不短路**：build failed（编译不过）期间 golangci-lint / go vet / go test 三条全红同根因，但每回合照样全套跑（实测 top 失败会话时间线：59+37+32 败集中在多波次"build failed 链"，每链 2-5 次转绿）。
5. **steer 对中间态无差别强催**：实测失败主流形态是"大改动中间态的自然红"（diag 逐次演变、2-5 次转绿，非复读机），门禁对"从未绿过"的新代码与"曾绿变红"的回归用同一强语气催修，干扰 TDD 节奏。
6. **扩展文档漂移**：`.pi/extensions/` 现有 8 个扩展，AGENTS.md 与 harness-facts skill 只记载 3 个（spec-gate / entry-gate / test-scope-guard / tool-output-spill / harness-telemetry 未入档），新会话 agent 对这些门禁"不知道存在"，约束体系漏防。

## What Changes

- `front/package.json` lint 脚本启用 eslint 增量缓存（`--cache` + cache 文件 gitignore），quality-gate / pre-push 调用路径同步收益；需兜底 eslint 配置文件变更时缓存失效的已知坑。
- `quality-gate.ts` 后端门禁命令（golangci-lint / go vet / go build）改为并行执行；每条命令仍各记一条 gate.check（记账语义不变）。
- gate.check **成功事件**从全量记账改为采样式记账（失败仍全量 + diag，粘性重跑语义不变）；分母统计语义随采样率调整（payload 标注采样标志，聚合侧可还原）。
- quality-gate **同根因短路**：golangci-lint 报 typechecking error（编译失败）时本轮跳过必然红的 go vet / go test（未执行不记账，符合既有"实际执行才记账"语义）。
- quality-gate **steer 语气分级**：区分"从未绿过"（新代码中间态，轻提示不打断节奏）与"曾绿变红"（回归，保持强催修），失败粘性重跑语义不变。
- AGENTS.md 与 `.agents/skills/harness-facts/SKILL.md` 补齐 8 扩展全景档：各扩展挂点（turn_end / tool_call）、触发条件、软硬门禁属性、fail-open/fail-loud 策略。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `harness-fact-log`: 门禁记账 requirement 中 "ok=true MUST 同样记账（失败率统计需要分母）" 改为成功事件采样式记账（失败全量），分母经采样率还原；其余 requirement（diag 提取、事件绑定、未运行不记账）不变。

## Impact

- `front/package.json`（lint 脚本一行 + cache 文件 gitignore）
- `.pi/extensions/quality-gate.ts`（本机运行数据，gitignored；快照同步 `docs/research/quality-gate.ts`）
- `.pi/extensions/lib/harness-log.ts`（如采样逻辑落在记账层）或 quality-gate 记账调用处（如采样在调用侧）
- `AGENTS.md`（扩展全景档）、`.agents/skills/harness-facts/SKILL.md`（写入方列表更新）
- `openspec/specs/harness-fact-log/spec.md`（delta 同步）
- 风险：① eslint `--cache` 对 flat config 变更的缓存失效语义需验证（必要时 `--cache-strategy content` 或配置变更检测兜底）；② 并行执行使 gate.check 时间戳交错，不影响 spec（spec 未规定顺序）；③ 采样后既有 SQL 配方（按 ok 分组统计失败率）需同步调整口径。
