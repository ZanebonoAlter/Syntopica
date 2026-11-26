# Review: add-dispatch-constraint-digest

- **审查方式**：`change-review` 只读子线程（模型 `minimax-cn/MiniMax-M3.1-Flash:max`），审查制品（proposal / design / specs / tasks / explore-findings）+ `git diff` 内实现，并实跑只读门禁
- **审查日期**：2026-11-04
- **结论**：修完可归档（H1 必须修，M1–M4 建议处理；L 级择机）
- **实跑核验**：`openspec validate --strict` ✅ / `constraint-digest.smoke.sh` 37-0 ✅ / `telemetry-dispatch.smoke.cjs` 10-10 ✅ / `harness-retro.smoke.sh` 112-0 ✅ / `run-smoke.sh` 仅 2 项预存在红（已登记 test-patrol）/ `check-standards.sh` 260-0 ✅ / `doc-impact verify` ✅ / `scenario-trace` 8-8 ✅；另有 bundle 失败模拟（exit 1 无静默降级）、`extractRedlines` 迁移前后 diff 逐字节一致、脚本输出 → `parseConstraintBrief` 格式契约实测

### R-1 2026-11-04【High】脚本输出格式与 telemetry 解析契约不对齐，按「原样粘贴」必然记 absent

- **状态**：已修复（新增 `--brief` 模式：单块 `## 约束（红线，源 …）` + 每域 `### <域>（源 …）` 子标题 + 固定兜底行；smoke 加端到端断言，用真实 `parseConstraintBrief` 对账）
- 问题：脚本输出 `### <域>（源 …）`，`parseConstraintBrief` 只认 `^## 约束`，而规范三处要求「原样粘贴」→ 实测原样粘进 brief 得 `absent`。失败无声、不阻断，会系统性压低「派发带域比例」，使设计 D7 的 ≥70% 判据得出反向结论（误判规范引导不成立 → 错误地另开插件兜底 change）。
- 定位：`scripts/harness/constraint-digest.sh`（输出标题行）vs `.pi/extensions/harness-telemetry.ts`（`parseConstraintBrief`）vs `specs/constraint-injection/spec.md` / `开发执行规范.md` §0.6 步骤 3
- 处置：见 tasks 2.4；规范模板随 `--brief` 真实输出形态统一（tasks 8.2）

### R-2 2026-11-04【Medium】esbuild 硬依赖阻断「未命中回退指引」三态

- **状态**：已修复（bundle 改惰性构建：标签扫描与三态判定不依赖 esbuild；域块降级为「红线不可用」+ 细节层指引，未命中说明照常打印）
- 问题：`build_redlines_bundle` 在 `--paths` 三态判定之前无条件调用，esbuild 不可用时连「未命中 → 回退 `--domains`」这条最该给的离线兜底都出不来。
- 定位：`scripts/harness/constraint-digest.sh`
- 处置：见 tasks 2.5

### R-3 2026-11-04【Medium】指标名「派发带域比例」与实际口径不符，域正则未校验目录前缀

- **状态**：已修复（收域正则收紧为 `docs/reference/flow/([a-z0-9-]+)\.md`，要求目录前缀；`present + []` 保留为「格式异常」态；SKILL ⑦G 明确口径）
- 问题：分子是 `constraintDigest=present`（有无 `## 约束` 段），不是「有没有域」——`present + domains=[]` 也计入「带域」；正则只要路径含 `flow/<name>.md` 就收域，不校验 `docs/reference/flow/` 前缀、不与 flow basename 白名单交叉校验，与本 change 亲手确立的「域唯一真相源」自相矛盾。
- 定位：`.pi/extensions/harness-telemetry.ts`、`scripts/harness/harness-retro.sh`、`.agents/skills/harness-retro/SKILL.md`
- 处置：见 tasks 3.4 与 5.3

### R-4 2026-11-04【Medium】spec delta 验收示例路径在本仓不存在

- **状态**：已修复（WHEN 改为 `backend-go/internal/admin/scheduler/job_daily_report.go`，THEN 改为命中 `daily-report` + `scheduler` 两域，与 tasks 9.2 对齐）
- 问题：原示例 `backend-go/internal/domain/daily_report` 无任何 `doc-impact-applies` 信号命中，照文档复现会得到与 THEN 相反的结论；该 Scenario 归档后会成为长期契约。
- 定位：`specs/constraint-injection/spec.md`

### R-5 2026-11-04【Medium】归档就绪缺口：review.md 未建账 + tasks.md 全未勾选

- **状态**：已修复（本文件即建账；tasks 已按实测结果勾选，预存在红与台账登记写入 1.3/7.2）
- 问题：`review-trace.sh` 实测为红（零痕迹），归档门禁 ⑤ 会阻断；tasks 全 `[ ]` 与「已实现并实测」现状不符。
- 定位：`openspec/changes/add-dispatch-constraint-digest/`、`tasks.md`

### R-6 2026-11-04【Low】`scan_tag_docs` 的 section 列为死列（子 shell 赋值丢失且取值早于解析）

- **状态**：已修复（删死列或注明 section 未消费，见 tasks 2.6）
- 问题：`section="$TAG_SECTION"` 恒空（赋值发生在 process substitution 子 shell，父 shell 拿不到；且取值在 `parse_tag_value` 之前）。当前无下游消费，无行为影响；但读起来像「解析出了 section」，未来依赖会静默拿到空值。
- 定位：`scripts/harness/constraint-digest.sh`

### R-7 2026-11-04【Low】tasks 2.1 声明读两个阈值，实现只读 `minSectionBytes`

- **状态**：已修复（tasks 2.1 改为只声明 `minSectionBytes`，并注明 `digestDocThreshold` 不适用——脚本只取节内红线，不做整文速览）
- 定位：`tasks.md` 2.1

### R-8 2026-11-04【Low】`--help` 硬编码域数量「（10 个）」

- **状态**：已修复（改为动态输出/去数字，与「唯一真相源在文件系统」口径一致，见 tasks 2.6）
- 定位：`scripts/harness/constraint-digest.sh`

### R-9 2026-11-04【Low】脚本输出正文形态与 brief 模板正文形态不同（编号+加粗 vs 无编号短句）

- **状态**：已修复（规范模板改为直接引用 `--brief` 真实输出形态，避免「模板手写示意 ≠ 实际输出」的第二处契约漂移）
- 定位：`docs/reference/开发执行规范.md` §0.6 步骤 3、`tasks.md` 8.2

### R-10 2026-11-04【Low】路径矩阵缺 design D3 两个边界 fixture（标签在 15 行外 / 头部两条标签行）

- **状态**：已修复（smoke 增 2 个临时 fixture：第 16 行标签→不命中、头部两条标签→只取首条，见 tasks 2.7）
- 问题：仓内所有标签都在第 3–6 行且无文档有两条合法标签行，这两个语义只有代码目视一致，无自动化回归保护。
- 定位：`scripts/harness/constraint-digest.smoke.sh`

### R-11 2026-11-04【Low】节提取语义在仓内有三份实现（extension / 脚本 awk / smoke 对账器）

- **状态**：已驳回（三份实现经逐行核对一致；smoke 对账器基于独立实现反而能捕获 bash 侧漂移，改造成本高于收益。作为已知边界记录，若 extension 的 `extractSection` 语义变更，需同步脚本 awk 与 smoke 对账器）
- 问题：三者当前语义一致（heading `trimEnd` 全等、段止于 `/^##\s+/`、整段 trim），但 smoke 的对账基准是自带的第三份实现，extension 改节语义时 smoke 不会报警。
- 定位：`.pi/extensions/constraint-injection.ts` `extractSection`、`scripts/harness/constraint-digest.sh`、`constraint-digest.smoke.sh`

### R-12 2026-11-04【Low】tasks 3.1「既有 17 个字段」与实际不符

- **状态**：已修复（改为「15 个固定字段 + 可选 `failure`」）
- 定位：`tasks.md` 3.1

### R-13 2026-11-04【Low】`npx -y esbuild` 首次运行需联网拉包，离线直接失败

- **状态**：已修复（优先本仓已装 esbuild：`command -v esbuild` / `node_modules/.bin/esbuild` / `npx --no-install`，都不可用才回落 `npx -y`；见 tasks 2.6）
- 定位：`scripts/harness/constraint-digest.sh`

### R-14 2026-11-04【Low】retro G 段样本未区分主线程派发与子线程二次派发

- **状态**：已修复（SKILL ⑦G 增「混池提示」：子线程再派发会稀释主线程带域比例，看比例须同时看 present 绝对数）
- 定位：`scripts/harness/harness-retro.sh`、`.agents/skills/harness-retro/SKILL.md`

## 复核

- 修复后复跑：`constraint-digest.smoke.sh`、`telemetry-dispatch.smoke.cjs`、`harness-retro.smoke.sh`、`run-smoke.sh`、`run-harness-smoke.sh`、`openspec validate --strict`、`scenario-trace.sh`、`doc-impact.sh verify`、`check-standards.sh --change`、`review-trace.sh` 全绿（`run-smoke.sh` 仅 2 项已登记预存在红）
