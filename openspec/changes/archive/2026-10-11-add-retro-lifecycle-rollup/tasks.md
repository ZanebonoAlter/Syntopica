# Tasks — add-retro-lifecycle-rollup

- [x] T1 `scripts/harness/harness-retro.sh` 全局模式实现 E 子组：change 清单扫描（活跃 + 窗口内归档）→ 复用 parse_feedback 逐目录解析 → 反馈覆盖 / 归因分布聚合 / 漏采信号（波次 ≥3 session × 零反馈）三项输出；json 模式补 `m7.lifecycle_*` 键
  - 验收：`bash scripts/harness/harness-retro.sh --days 7` ⑦ 段后输出 E 生命周期面子组且全程零写入
- [x] T2 `scripts/harness/harness-retro.smoke.sh` 加 fixture 断言：有账/无账混合计数、归因聚合、漏采三态（进名单/低波次不进/有账不进）、全空降级、格式异常单列、窗口边界归档目录计入/排除、连跑两次输出一致
  - 验收：`bash scripts/harness/harness-retro.smoke.sh` 102/102 全过（M1-M8 新增 + 既有 K/L 系列无回归）
- [x] T3 `.agents/skills/harness-retro/SKILL.md` 七段表同步 ⑦E 行 + 漏采信号解读（本地 gitignored 文件照改）
  - 验收：文件存在且含「E 生命周期面」与漏采信号条目
- [x] T4 真库一致性抽查：`--days 7` 的 E 子组数值与 `--lifecycle <某有账 change>` 第五段手工汇总一致（抽查 1 个）
  - 验收：`bash scripts/harness/harness-retro.sh --days 7` 的计数 ≥ 逐卡汇总抽查值且归因分布吻合

## 1. 测试

- `bash scripts/harness/harness-retro.smoke.sh` 全过（新增 case + 既有无回归）
- `bash scripts/harness/harness-retro.sh --days 7` 正常出报告（E 子组可见，抽查一致性）

## 2. 文档

<!-- doc-impact: harness（pi-extensions.md 的 harness 全景描述若提及效能看板子组构成需同步；SKILL.md 为本地 skill 非 docs 域） -->

- `docs/reference/harness/pi-extensions.md`：检查 harness-retro 效能看板子组描述是否需补 E 组（若文中未罗列子组明细则零改动）。

## 3. 验证

| Scenario | 测试文件 |
| --- | --- |
| 反馈覆盖计数 | scripts/harness/harness-retro.smoke.sh |
| 归因分布聚合 | scripts/harness/harness-retro.smoke.sh |
| 漏采信号 | scripts/harness/harness-retro.smoke.sh |
| 反馈文件格式异常不炸段 | scripts/harness/harness-retro.smoke.sh |
| 零反馈数据降级 | scripts/harness/harness-retro.smoke.sh |
| 全程只读 | 人工：跑 `--days 7` 后 `git status` 干净区无新增文件 + sqlite readonly 打开语义沿用既有（报告头显示 readonly） |
