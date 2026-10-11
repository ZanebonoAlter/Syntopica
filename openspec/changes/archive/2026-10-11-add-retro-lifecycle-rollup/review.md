# Review — add-retro-lifecycle-rollup

### R-1 2026-10-11【Medium】自审：CHG_PY 反馈解析与 lifecycle 模式 parse_feedback 的词表同步
- **状态**：已修复（审毕确认）

自审范围：`harness-retro.sh` 新增 CHG 装配块 / chg_wave CTE / lifecycle metrics / E 渲染段 / m7.lifecycle_* 键，`harness-retro.smoke.sh` M 段，SKILL.md ⑦ 行，pi-extensions.md 同步。

- **词表同步（design D3）**：CHG_PY 内嵌解析与 `--lifecycle` 模式 `parse_feedback` 逐词对照——FB_STATUSES 五值、归因五类正则、状态行正则、文件缺席/无条目降级、bad 单列语义一致；全局侧未实现 types 字段（产品/流程拆分单卡才有意义，全局聚合用不上）属有意省略非漂移。
- **波次口径**：chg_wave 基于 emap_flat（win 窗口内 edit.map 事件 DISTINCT session），与 C 组 wave CTE 同源；change 列为空的事件不进任何 change 波次（WHERE chg != ''）与 C 组口径一致。
- **SQL 注入面**：CHG_LIT 经 sed 单引号转义（ARCH_LIT 同款）后进 json_each(${CHG_LIT})——fixture 文件名含单引号不会破坏 SQL（smoke 未覆盖单引号目录名，属可接受残留，ARCH_PY 同边界）。
- **渲染降级**：lc 空 / layers 空 / miss 空三态均有显式输出行，不静默省略（smoke M8 覆盖）；坏条目 bad_n>0 才输出格式异常行。
- **幂等与只读**：smoke M7 覆盖（除窗口时间戳行外两次输出一致）；全程零写入（库 readonly + 目录仅读，C1 先例语义沿用）。

- 审毕无发现（审查范围：上述全部 diff + spec delta 六 Scenario 与实现逐条对照）
