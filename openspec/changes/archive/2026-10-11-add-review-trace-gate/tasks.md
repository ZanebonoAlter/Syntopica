# add-review-trace-gate Tasks

## 1. 脚本层

- [x] 1.1 新增 `scripts/harness/review-trace.sh`：只读对账 `<change-dir>`——① proposal.md 头部机器可读注释区 `<!-- review: waived -->` → 通过（输出注明 waived）；② `review.md` 含 ≥1 条合法条目（`### R-<N> <YYYY-MM-DD>【High|Medium|Low】<标题>` + 次行 `- **状态**：待处理|已修复|已采纳|已驳回|误报丢弃`）或合法 `- 审毕无发现（审查范围：<说明>）` 声明 → 通过；③ 皆无 / 空壳文件 → 退出码 1 且中文输出两条修复路径指引（验证：POSIX bash 语法 `bash -n` 通过；判据优先级 waived 先于 review.md，两者同存在时输出注明双痕迹但通过）
- [x] 1.2 新增 `scripts/harness/review-trace.smoke.sh`：临时目录 fixture 六例——建账通过（含条目计数输出）/ 无发现声明通过 / waived 通过 / 零痕迹红 / 空壳红（有文件零合法条目）/ 词表外（等级 Critical、状态「搞定」）不合法（验证：`bash scripts/harness/review-trace.smoke.sh` 退出码 0）
- [x] 1.3 `scripts/harness/archive-readiness.sh` 四项硬检查 → 五项：新增 `run_item "⑤ review-trace 痕迹对账" bash "$BIN_DIR/review-trace.sh" "$CHANGE_DIR"`；human-feedback 提醒层 ⑤→⑥ 编号与文案顺延（不阻断语义不变，verify 注释同步）；`scripts/harness/archive-readiness.smoke.sh` 补五项断言（验证：smoke 退出码 0）
- [x] 1.4 `scripts/harness/harness-retro.sh --lifecycle` 新增「review 覆盖」段：痕迹三态（review.md 建账〔含条目数〕/ waived / 皆无——已归档且皆无记缺口）+ review 类派发计数（dispatch 且 change=本 change，type=change-review-glm 或 desc 含 review/审查，desc 含「修复」优先归修复类）+ 改造前无归属时段标注「归属改造前无数据」；`scripts/harness/harness-retro.smoke.sh` 补断言（验证：smoke 退出码 0）

## 2. 扩展层

- [x] 2.1 `.pi/extensions/harness-telemetry.ts` dispatch 落账补归属：`tool_result(Agent)` 分支内新增一次 `detectActiveChange(ctx.cwd)?.name ?? null` 判定，值同时写 `agentChangeBindings`（既有行复用该值，不二次调用）与 `guardedLog({kind:"subagent.dispatch", ..., change})`（验证：`grep -n 'kind: "subagent.dispatch"' -A2` 可见 change 字段；subagent.complete 分支不动）
- [x] 2.2 新增 `.pi/extensions/tests/telemetry-dispatch.smoke.cjs`（仿 telemetry-archive.smoke.cjs mock 管线 + 临时 events.db）：有活跃 change 目录时 dispatch 行 change=目录名 / 无非 archive 目录时 change=null 不阻断落账 / 既有行不可变（追加不改写）（验证：`node .pi/extensions/tests/telemetry-dispatch.smoke.cjs` 退出码 0；跑法经 `run-harness-smoke.sh` esbuild 后调则并入该脚本清单）
- [x] 2.3 `.pi/extensions/spec-gate.ts` gateArchive 挂第五项 block 检查：`runScript(pi, ctx, ["scripts/harness/review-trace.sh", changeDir])` 失败推入 failures；注释「四项」→「五项 block + 检查⑤措辞 + ⑥' UI 验收」；豁免放行文案枚举六项检查同步更新；`.pi/extensions/tests/spec-gate.smoke.cjs` 补断言（零痕迹 fixture 阻断 + waived fixture 放行）（验证：smoke 退出码 0）

## 3. 文档

- [x] 3.1 `docs/reference/开发执行规范.md`：§0.6 步骤 4 产出列补「review.md 建账或 waived 声明」；「review 执行要点」补建账格式与豁免 marker 契约；§11 归档门禁清单四项→五项（同步自查脚本描述）
- [x] 3.2 `docs/reference/standard/shared/doc-authoring.md` 注册点速查补 review.md 格式契约与 `<!-- review: waived -->` marker（与 FB 条目格式并列）
- [x] 3.3 `docs/reference/harness/pi-extensions.md` 扩展全景表 spec-gate 行六项检查同步；harness 事件词汇表 dispatch 补 change 归属语义
- [x] 3.4 `.agents/skills/harness-facts/SKILL.md` dispatch 事件 payload/字段说明补 change 列（改造前可空的历史边界一句）
- [x] 3.5 `openspec/changes/archive/` 零回改；`docs/reference/architecture/map.md` 不动（harness 工具链不在业务地图）——git 验证：archive/ 无改动；map.md 树上脏块属 add-fast-frequency-datasources（研究数据源行），本 change 零触碰

## 4. 测试

- [x] 4.1 运行 `bash scripts/harness/review-trace.smoke.sh` → 退出码 0（六 fixture 全断言）
- [x] 4.2 运行 `bash scripts/harness/archive-readiness.smoke.sh` → 退出码 0（五项聚合 + ⑥ 提醒层无回归）
- [x] 4.3 运行 `bash scripts/harness/harness-retro.smoke.sh` → 退出码 0（lifecycle 新段无回归）
- [x] 4.4 运行 `node .pi/extensions/tests/telemetry-dispatch.smoke.cjs` → 退出码 0
- [x] 4.5 运行 `node .pi/extensions/tests/spec-gate.smoke.cjs` → 退出码 0（第五项集成无回归）
- [x] 4.6 运行 `node .pi/extensions/tests/session-rollup.smoke.cjs` 与 `telemetry-archive.smoke.cjs` → 退出码 0（harness-telemetry 邻近分支无回归）

## 5. 文档一致性

- [x] 5.1 三处编号一致（人工：spec-gate.ts 注释 / archive-readiness.sh 输出 / pi-extensions.md 全景表）：block 级五项 + 检查⑤措辞 + ⑥' UI 验收，无相互矛盾
- [x] 5.2 `grep -rn 'review: waived' docs/ scripts/ .pi/` 命中处语义一致（proposal.md 头部机器可读注释唯一豁免形态）

## 6. 验证

| Scenario | 测试文件 |
| --- | --- |
| review.md 建账通过 | scripts/harness/review-trace.smoke.sh |
| 审毕无发现声明通过 | scripts/harness/review-trace.smoke.sh |
| waived 豁免通过 | scripts/harness/review-trace.smoke.sh |
| 零痕迹阻断 | scripts/harness/review-trace.smoke.sh |
| 空壳文件阻断 | scripts/harness/review-trace.smoke.sh |
| 标准条目解析 | scripts/harness/review-trace.smoke.sh |
| 词表外等级不合法 | scripts/harness/review-trace.smoke.sh |
| 状态词表外不合法 | scripts/harness/review-trace.smoke.sh |
| 纯文档 change 豁免 | scripts/harness/review-trace.smoke.sh |
| 声明位置不识别 | scripts/harness/review-trace.smoke.sh |
| 归档时零痕迹阻断 | .pi/extensions/tests/spec-gate.smoke.cjs |
| 豁免放行留痕 | .pi/extensions/tests/spec-gate.smoke.cjs |
| 自查脚本同口径聚合 | scripts/harness/archive-readiness.smoke.sh |
| 全绿通过 | scripts/harness/archive-readiness.smoke.sh |
| 多项红一次性汇总 | scripts/harness/archive-readiness.smoke.sh |
| review 痕迹红项聚合 | scripts/harness/archive-readiness.smoke.sh |
| 建账 change 覆盖段展示 | scripts/harness/harness-retro.smoke.sh |
| 归属改造前不误报 | scripts/harness/harness-retro.smoke.sh |
| dispatch 携带 change 归属 | .pi/extensions/tests/telemetry-dispatch.smoke.cjs |
| 无活跃 change 时 dispatch 归属可空 | .pi/extensions/tests/telemetry-dispatch.smoke.cjs |
| 事件追加不可变 | .pi/extensions/tests/telemetry-dispatch.smoke.cjs |
| TTL 分级清扫 | .pi/extensions/tests/harness-log.smoke.cjs |
| spill.write 事件随词汇扩展落库 | .pi/extensions/tests/harness-log.smoke.cjs |
| subagent.complete 随词汇扩展落库 | .pi/extensions/tests/harness-log.smoke.cjs |
| policy.decision 随词汇扩展落库 | .pi/extensions/tests/harness-log.smoke.cjs |
| edit.map 随词汇扩展落库 | .pi/extensions/tests/harness-log.smoke.cjs |
| session.rollup 随词汇扩展落库 | .pi/extensions/tests/harness-log.smoke.cjs |
| change.archive 随词汇扩展落库 | .pi/extensions/tests/harness-log.smoke.cjs |
| change 不存在 | scripts/harness/archive-readiness.smoke.sh |
| 只读安全 | scripts/harness/archive-readiness.smoke.sh |

<!-- doc-impact: standard, harness（业务域白名单零触碰；docs/reference 四处更新：doc-authoring.md→standard 域、pi-extensions.md→harness 域、开发执行规范.md 属执行规范本体随流程段落更新、SKILL.md 为 skill 副本非 docs 域） -->

- [x] 6.1 `bash scripts/harness/review-trace.sh openspec/changes/add-review-trace-gate` → 退出码 0（本 change 自身建账：review.md 留痕，格式吃自家狗粮）
- [x] 6.2 `openspec validate add-review-trace-gate` → `is valid`
- [x] 6.3 既有门禁无回归：`bash scripts/harness/archive-readiness.sh <任一 in-flight change>` → 输出五项且仅新增项可能红（其余四项判定逻辑零改动）
- [x] 6.4 落地后回检（人工，下一会话）：`bash scripts/harness/harness-retro.sh --lifecycle <后续任一 change>` → review 覆盖段展示三态且新 dispatch 事件带 change 归属
  - 2026-10-11 回检完成：`--lifecycle fix-board-match-cross-domain` 六段 review 覆盖段展示三态（皆无/已归档缺口形态）；`node .pi/extensions/tests/telemetry-dispatch.smoke.cjs` 4 case 全过（change 列归属/追加不改写/bindings 同源）；真实派发自然样本待未来产生（改动落盘后暂无 dispatch 发生）
