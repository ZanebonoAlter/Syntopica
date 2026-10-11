<!-- complexity: simple -->
<!-- ui-impact: none -->

## Why

harness 体检的双入口存在覆盖盲区（2026-10-11 用户体检时发现）：全局报告（`--days N`）七段全部由 events.db 事件聚合驱动，而人工反馈（human-feedback.md，文件为源、随 change 归档永久保存）与漏采信号只在 `--lifecycle` 单 change 体检卡第五段可见——定期体检（全局模式）默认永远看不到这一面。人工反馈的采集纪律靠「高返工波次 × 零反馈条数 = 漏采信号」判定，该判定需要的两侧数据（编辑波次在全局 C 组已有、反馈条数在文件侧）只有全局报告能把它们乘起来跨 change 巡视。当前全库仅 3 份 human-feedback.md，且没人主动跑 `--lifecycle` 逐个 change 检查——反馈面事实上处于「建了账没人看」状态。

## What Changes

- **全局报告 ⑦ 效能看板新增子组 E「生命周期面」**（与 D 测试欠账巡检同级的文件/台账面数据源）：
  - **反馈覆盖**：窗口内活跃 + 归档 change 的 human-feedback.md 覆盖情况（有账 N / 无账 M / 未处置条数）
  - **归因分布聚合**：窗口内全部反馈条目的归因五类（frontend/backend/test-gap/fact-check/process）跨 change 聚合计数——高发层直接映射规范整理去向（skill 既有解读口径）
  - **漏采信号**：窗口内编辑波次 ≥3 session（复用 C 组口径）× 反馈 0 条的 change 列表——多轮返工却没人建反馈账 = 采集纪律松了
- **change 清单口径**：活跃（changes/ 目录）+ 窗口内归档（archive/ 目录日期前缀落在窗口内）；反馈解析复用 lifecycle 模式 parse_feedback 的词表与降级语义（文件缺席=无记录不算异常、格式异常单列不炸段）。
- **SKILL.md 同步**：harness-retro skill 的七段表加 ⑦E 行与漏采信号解读。
- 零写入原则不变：报告全程只读 events.db 与文件系统。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `harness-retro-loop`: 全局报告效能看板新增 ⑦E 生命周期面子组（delta spec 见 specs/）。

## Impact

- 代码：`scripts/harness/harness-retro.sh`（全局模式 ⑦ 段渲染 + E 子组）、`scripts/harness/harness-retro.smoke.sh`（fixture 断言）、`.agents/skills/harness-retro/SKILL.md`（gitignored 本地同步）。
- 影响包/命令：仅 harness 脚本与 smoke；后端前端零触碰。
- 部署影响：无——纯本地工具链，用户无感；下次跑 `harness-retro.sh --days N` 输出多一个子组。
