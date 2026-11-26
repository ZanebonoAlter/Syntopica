<!-- stage: implement -->

# Tasks

## 1. 准备

- [x] 1.1 统计 4 个 spec 结构：board-diagnostics 22 req / 109 sc / 615 行；board-research 36 req / 122 sc / 769 行
- [x] 1.2 反查创建 change：health-checkup ← 2026-10-10；direction-check ← 2026-05-29；signal-reports ← 2026-09-26；level-analysis ← 2026-08-31
- [x] 1.3 确认 `radar-pipeline-status` 不并入（UI 展示契约，195 行，与后端分析无改动连带）

## 2. 生成合并稿

- [x] 2.1 复用 `scripts/harness/merge-spec-family.sh`（`merge-tag-spec-family` 已验证）
- [x] 2.2 生成两族初稿：board-diagnostics 673 行 / board-research 867 行
- [x] 2.3 守恒核对：ADDED 58 = REMOVED 58 = 原 4 个 spec 的 Requirement 总数

## 3. 人工校对

- [x] 3.1 `board-diagnostics` 20 business / 2 impl，Purpose 与边界复核
- [x] 3.2 `board-research` 36 business / 0 impl
- [x] 3.3 检查跨 spec 重复（方向校验与体检分诊磁铁信号的交叉引用）
- [x] 3.4 内容问题（过时/矛盾）只记录到 `review.md`，本次不修

## 4. delta 与切换

- [x] 4.0 写 `.openspec.yaml`（`retire_capabilities: true`）
- [x] 4.1 生成 2 个族的 `## ADDED Requirements`
- [x] 4.2 生成 4 个旧 capability 的 `## REMOVED Requirements`
- [ ] 4.3 归档前摘掉 4 个被退役 spec 的 `<!-- domain: -->`（openspec 探针实测 `unaccountedContent` 只有 marker 一行，摘掉即充分）；**与 8.2 同一个提交**，摘除不单独 commit
- [ ] 4.4 **归档后**断言 4 个目录已自动退役（`retire_capabilities` 生效条件：无 leftover 内容）
- [ ] 4.5 **归档后** `openspec validate --specs` → 156 全过（158 − 4 + 2）

## 5. 验证

- [ ] 5.1 `domain-history.sh semantic-board` → 第 6 段列出指向 4 个旧 capability 的孤儿 delta
- [ ] 5.2 `check-standards.sh` → 156 个 spec marker 全合法
- [ ] 5.3 抽查 6 条来源注释，grep archive 确认 change 与日期属实
- [ ] 5.4 确认 `radar-pipeline-status` 未被触碰（文件 mtime / git diff 为空）

## 6. review

- [x] 6.1 派 review（`zai-coding-cn/glm-5.4:max`）：守恒、语义零改动、来源注释、business/impl 分类、归档可行性
- [x] 6.2 处置 review 发现

## 7. 文档

- [x] 7.1 `doc-authoring.md` 的族级约定已在上一 change 落地，本次无需改
- [ ] 7.2 `docs/research/process-arch-rework/issues.md` 回填 ISS-04 进度（第二族已合并）
- [ ] 7.3 `docs/reference/flow/semantic-board.md` 变更溯源表加本 change 一行
- [ ] 7.4 旧 capability 名的路径引用：docs/reference 40 余处均为 change 名溯源不需改；**实质路径引用 1 处**，在在途 change 的探索笔记里（非 main spec，不阻断本 change）

## 8. 归档

- [ ] 8.1 `archive-readiness.sh merge-board-diagnostic-specs` 六项全绿
- [ ] 8.2 `openspec archive merge-board-diagnostic-specs`
- [ ] 7.5 **登记后续 change**：修 `tag-matching` 里「（详见 board-direction-check）」死引用 + 盘点 3 条消费方契约（日报排除 direction_mismatch / 文章 API 过滤 / match detail 返回 direction_sim）在 `tag-matching` 与 `board-diagnostics` 两侧的重复落点（R-2，跨族引用要等相邻族都收口后统一扫）
- [ ] 8.3 归档后给 2 个新族补 marker：`board-diagnostics` → `semantic-board`；`board-research` → `semantic-board, data-enrichment`

## 9. 测试

本 change 零代码改动，验证即结构与守恒：

| 验证点 | 命令 | 期望 |
| --- | --- | --- |
| delta 结构 | `openspec validate merge-board-diagnostic-specs` | valid |
| 全仓 spec | `openspec validate --specs` | 156 passed |
| 条数守恒 | ADDED 58 = REMOVED 58 = 原 4 spec 总数 | 恒等 |
| Scenario 守恒 | 109 + 122 = 231 | 恒等 |
| REMOVED 标题匹配 | 4 个 delta 的标题 vs main spec 逐字 | 58/58 |
| 归档退役 | 归档后 spec 目录数 | 156，无空壳 |

## 10. 文档

<!-- doc-impact: flow, research -->

- `docs/reference/flow/semantic-board.md`：变更溯源表加一行（7.3）
- `docs/research/process-arch-rework/issues.md`：ISS-04 进度回填（7.2）

## 11. 验收

- [ ] A 两族各自包含原成员的全部 Requirement（22 / 36）
- [ ] B 每条带 `<!-- from: -->`，可 grep archive 验证
- [ ] C 4 个旧 capability 已被自动退役；`radar-pipeline-status` 原样保留
