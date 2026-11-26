<!-- stage: implement -->

# Tasks

## 1. 准备

- [x] 1.1 统计 9 个 spec：3 个内聚（877/728/572），6 个碎片（318/248/108/104/219/246）
- [x] 1.2 反查创建 change（lane-manual-correction ← 2026-11-16；board-lane-dynamics ← 2026-09-11；lane-trend-overview ← 2026-09-22；lane-heat-metric ← 2026-11-20；workshop-radar + article-archive ← 2026-11-02 同一个 change）
- [x] 1.3 确认：lane-ops 新建（4 并 1）+ article-archive 并入 board-console（就地扩容）；radar 保持独立（R-5 更正）；3 个内聚 spec 不动

## 2. 生成合并稿

- [x] 2.1 复用 `merge-spec-family.sh`，补两族 PURPOSE 文案
- [x] 2.2 生成：lane-ops 797 行（23 req）/ workshop-board-console 就地扩容 +8 req
- [x] 2.3 守恒：ADDED 38 = REMOVED 38；Scenario 168 = 168；38 条逐字与原 spec 一致（0 差异）

## 3. 人工校对

- [x] 3.1 分类 38 business / 0 impl（本族无施工记录）
- [x] 3.2 跨族引用检查：`lane-heat-metric` 被 `workshop-board-console` 引用（「泳道热度口径」条），合并后指向 lane-ops 不影响
- [ ] 3.3 内容问题登记（本次不修）：`lane-correction-surface` 的 Purpose 仍是 `TBD - created by archiving change ...`（历史遗留）

## 4. delta 与切换

- [x] 4.0 写 `.openspec.yaml`（`retire_capabilities: true`）
- [x] 4.1 生成 2 个族的 `## ADDED Requirements`
- [x] 4.2 生成 6 个旧 capability 的 `## REMOVED Requirements`
- [ ] 4.3a 归档前摘掉 **5 个**被退役 spec 的 `<!-- domain: -->`（lane-manual-correction / board-lane-dynamics / lane-trend-overview / lane-heat-metric / workshop-article-archive）
- [ ] 4.3b 清掉 `lane-manual-correction` 的 **20 行 markdown 表格**（R-1 实测：表格行属 openspec 的「不可点名内容」，会导致归档中止两次；表格内容已逐字保留在 lane-ops 的 ADDED 块里，`grep '字段 | 含义'` 可验）。**不要删空 spec**——`counts.removed=0` 会让 openspec 走 write 而非 retire
- [ ] 4.4 **归档后**断言 6 个目录已退役
- [ ] 4.5 **归档后** `openspec validate --specs` → 146 全过（150 − 5 退役 + 1 新建）

## 5. 验证

- [ ] 5.1 `domain-history.sh semantic-board` → 第 6 段列出 6 个旧 capability 的孤儿 delta
- [ ] 5.2 `check-standards.sh` → 146 个 spec marker 全合法
- [ ] 5.3 抽查 6 条来源注释，grep archive 确认属实
- [ ] 5.4 确认 lane-correction-surface / workshop-lane-compose / workshop-radar 未被触碰

## 6. review

- [ ] 6.1 派 review（`zai-coding-cn/glm-5.4:max`）
- [ ] 6.2 处置 review 发现

## 7. 文档

- [ ] 7.1 `docs/reference/flow/semantic-board.md` 变更溯源表加本 change 一行
- [ ] 7.2 `docs/research/process-arch-rework/issues.md` 回填 ISS-04（第六族，泳道组收口）
- [ ] 7.3 旧 capability 名引用扫描（6 个）
- [ ] 7.4 登记到「跨族引用清扫」batch（若需改 main spec 才能修）

## 8. 归档

- [ ] 8.1 `archive-readiness.sh merge-lane-ops-specs` 六项全绿
- [ ] 8.2 `openspec archive merge-lane-ops-specs`
- [ ] 8.3 归档后补 marker（R-2 更正）：
  - [ ] 8.3a `lane-ops` → `<!-- domain: semantic-board, daily-report, topic-graph -->`（3 域，topic-graph 不可省——lane-manual-correction 原 marker 含它，且族内有 topic-graph 红线 23/24/25）
  - [ ] 8.3b `workshop-board-console` 保留原 marker `semantic-board`（就地扩容，归档后实测确认是否保留）

## 9. 测试

零代码改动，验证即结构与守恒：

| 验证点 | 命令 | 期望 |
| --- | --- | --- |
| delta 结构 | `openspec validate merge-lane-ops-specs` | valid |
| 全仓 spec | `openspec validate --specs` | 146 passed |
| 条数守恒 | ADDED 38 = REMOVED 38 | 恒等 |
| Scenario 守恒 | 168 = 168 | 恒等 |
| REMOVED 标题匹配 | 6 个 delta vs main spec 逐字 | 38/38 |
| 归档退役 | 归档后目录数 | 146，无空壳 |
| 未触碰项 | `git diff` on 3 个内聚 spec | 空 |

## 10. 文档

<!-- doc-impact: flow, research -->

- `docs/reference/flow/semantic-board.md`：变更溯源表加一行（7.1）
- `docs/research/process-arch-rework/issues.md`：ISS-04 进度回填（7.2）

## 11. 验收

- [ ] A `lane-ops` 23 req、`workshop-board-console` 33 req（25 原有 + 8 新增）
- [ ] B 38 条 Requirement 带 `<!-- from: -->`，可 grep archive 验证
- [ ] C 6 个旧 capability 已退役；3 个内聚 spec 原样保留
