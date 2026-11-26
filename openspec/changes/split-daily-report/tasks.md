<!-- stage: implement -->

# Tasks

## 1. 准备

- [x] 1.1 统计 41 条按功能分 6 组（A 生成 15/470 行、B 接口 4/217、C 前端 7/285、D 调度 9/260、E 选题 5/159、F 分桶 1/58）
- [x] 1.2 反查创建 change（daily-report-system ← 2026-05-26 board-interaction-overhaul 等）
- [x] 1.3 确认切法：A+E+F 留原位，B+C+D(-2) 去 interface/ops；match-fallback 并入 **daily-report-system 自身**（不新建 capability）；radar 并入 interface

## 2. 生成拆分稿

- [x] 2.1 写 `split-plan.tsv`（族名 / 源 cap / Requirement 标题，`*` = 全取）——cap 级合并脚本粒度不够，需按标题子集切分；**生成器已入库** `scripts/harness/split-spec-family.sh`（R-5）
- [x] 2.2 生成器按 plan 抽 ADDED、按「被取走的条目」写 REMOVED（未取走的不出现在 REMOVED 里）
- [x] 2.3 守恒：ADDED 27 = REMOVED 27；27 + 21（留原位）= 48 = 原 3 个 spec 总和
- [x] 2.4 Scenario 守恒：159（搬走）+ 99（留原位）= 258 = 原 3 spec 总和

## 3. 人工校对

- [x] 3.1 分类 26 business / 1 impl（`定时任务复用`）
- [ ] 3.2 `daily-report` 2 条新增（match-fallback）+ 21 条原位，Purpose 需手改
- [ ] 3.3 跨 spec 重复检查：`daily-report-match-fallback` 的「排除 direction_mismatch」与 `tag-matching` / `board-diagnostics` 的方向校验条款交叉
- [ ] 3.4 内容问题登记（本次不修）

## 4. delta 与切换

- [x] 4.0 写 `.openspec.yaml`（`retire_capabilities: true`）
- [x] 4.1 生成 3 个族的 `## ADDED Requirements`
- [x] 4.2 生成 3 个源 cap 的 `## REMOVED Requirements`（daily-report-system 20 条、match-fallback 2、radar 5）
- [ ] 4.3 归档前摘掉 **2 个**整体退役 spec 的 `<!-- domain: -->`（match-fallback、radar，实测 `unaccountedContent` 仅剩 marker 一行）；**daily-report-system 不摘**
- [ ] 4.4 **归档后**断言 2 个目录已退役、`daily-report-system` 仍在
- [ ] 4.5 **归档后** `openspec validate --specs` → 151 全过（150 − 2 + 2 新建 + 1 保留 = 151）

## 5. 验证

- [ ] 5.1 `domain-history.sh daily-report` → 第 6 段列出 2 个旧 capability 的孤儿 delta
- [ ] 5.2 `check-standards.sh` → 全部 spec marker 合法
- [ ] 5.3 抽查 6 条来源注释，grep archive 确认属实
- [ ] 5.4 确认 `daily-report-system` 归档后只剩 21 条、99 sc

## 6. review

- [x] 6.1 派 review（`minimax-cn/MiniMax-M3.1-Flash:max`）
- [x] 6.2 处置 review 发现（12 条）

## 7. 文档

- [ ] 7.1 `docs/reference/flow/daily-report.md` 变更溯源表加本 change 一行
- [ ] 7.2 `docs/research/process-arch-rework/issues.md` 回填 ISS-04（第五族）
- [ ] 7.3 旧 capability 名引用扫描（`daily-report-match-fallback`、`radar-pipeline-status`）
- [ ] 7.4 登记到「跨族引用清扫」batch（若需改 main spec 才能修）

## 8. 归档

- [ ] 8.1 `archive-readiness.sh split-daily-report` 六项全绿
- [ ] 8.2 `openspec archive split-daily-report`
- [ ] 8.3 归档后：
  - [ ] 8.3a `daily-report-system` 手改 Purpose；**marker 不用补**——openspec 2.1.1 实测预演（`buildUpdatedSpec`）显示它是「纯删除 + 追加」，原 marker 保留、顺序不变、新增 0 行（R-4 更正了原先「会丢」的错误说法）
  - [ ] 8.3b `daily-report-interface` 补 `<!-- domain: daily-report, ui -->`
  - [ ] 8.3c `daily-report-ops` 补 `<!-- domain: daily-report -->`

## 9. 测试

零代码改动，验证即结构与守恒：

| 验证点 | 命令 | 期望 |
| --- | --- | --- |
| delta 结构 | `openspec validate split-daily-report` | valid |
| 全仓 spec | `openspec validate --specs` | 151 passed |
| 条数守恒 | ADDED 27 = REMOVED 27；27 + 21 = 48 | 恒等 |
| Scenario 守恒 | 159 + 99 = 258 | 恒等 |
| REMOVED 标题匹配 | 3 个 delta vs main spec 逐字 | 27/27 |
| 归档退役 | 归档后目录数 | 2 个退役，daily-report-system 仍在 |
| 未触碰项 | `git diff` on data-enrichment | 空 |

## 10. 文档

<!-- doc-impact: flow, research -->

- `docs/reference/flow/daily-report.md`：变更溯源表加一行（7.1）
- `docs/research/process-arch-rework/issues.md`：ISS-04 进度回填（7.2）

## 11. 验收

- [ ] A `daily-report-system` 23 req（21 原位 + 2 新增）、`daily-report-interface` 18 req、`daily-report-ops` 7 req
- [ ] B 27 条搬移的 Requirement 带 `<!-- from: -->`，可 grep archive 验证
- [ ] C 2 个旧 capability 已退役；`daily-report-system` 保留 21 条 99 sc
