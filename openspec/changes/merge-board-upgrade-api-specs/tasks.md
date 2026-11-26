<!-- stage: implement -->

# Tasks

## 1. 准备

- [x] 1.1 统计：board-upgrade 族 23 req / 74 sc / 439 行；board-api 族 25 req / 70 sc / 430 行
- [x] 1.2 反查创建 change：board-upgrade ← 2026-07-23 board-discovery-expansion；upgrade-expand ← 2026-09-07 split-board-upgrade-directions；time-window ← 2026-09-17 expand-upgrade-days-window；management-api + article-api ← 2026-05-26 board-interaction-overhaul；editing-ui ← 2026-05-29
- [x] 1.3 确认 `board-concept-management` 不参与（40 行全 DEPRECATED + 并发会话有未提交改动）

## 2. 生成合并稿

- [x] 2.1 复用 `scripts/harness/merge-spec-family.sh`
- [x] 2.2 生成两族初稿：board-upgrade 491 行 / board-api 490 行
- [x] 2.3 守恒：ADDED 32 = REMOVED 32；48 = 32 搬移 + 16 保留（board-upgrade 就地扩容）

## 3. 人工校对

- [x] 3.1 `board-upgrade` 23 business / 0 impl
- [x] 3.2 `board-api` 24 business / 1 impl（`API type updates`）
- [x] 3.3 检查跨 spec 重复（升级建议 API 在 upgrade 与 management-api 两侧的落点）
- [ ] 3.4 内容问题登记到 `review.md`，本次不修——已知两项：①红线 6/7（suggestion_hash 幂等、dismiss 14 天冷却）在 spec 里只有「复用现有机制」式弱覆盖，无独立契约条款；②升级建议 API 在两族各有描述（算法侧 vs 接口侧）

## 4. delta 与切换

- [x] 4.0 写 `.openspec.yaml`（`retire_capabilities: true`）
- [x] 4.1 生成 2 个族的 `## ADDED Requirements`
- [x] 4.2 生成 4 个旧 capability 的 `## REMOVED Requirements`
- [ ] 4.3 归档前摘掉 4 个被退役 spec 的 `<!-- domain: -->`；与 8.2 同一提交
- [ ] 4.4 **归档后**断言 4 个目录已自动退役
- [ ] 4.5 **归档后** `openspec validate --specs` → 152 全过（156 − 4 + 1 = 153，见下）

> 修正：156 − 4（退役）+ 1（board-api 新建）= **153**（board-upgrade 就地扩容不新增）

## 5. 验证

- [ ] 5.1 `domain-history.sh semantic-board` → 第 6 段列出指向 4 个旧 capability 的孤儿 delta（实测记数）
- [ ] 5.2 `check-standards.sh` → 153 个 spec marker 全合法
- [ ] 5.3 抽查 6 条来源注释，grep archive 确认属实
- [ ] 5.4 确认 `board-concept-management` 未被触碰（git diff 为空）

## 6. review

- [x] 6.1 派 review（`zai-coding-cn/glm-5.4:max`）
- [x] 6.2 处置 review 发现

## 7. 文档

- [ ] 7.1 `docs/reference/flow/semantic-board.md` 变更溯源表加本 change 一行
- [ ] 7.2 `docs/research/process-arch-rework/issues.md` 回填 ISS-04（第三族）
- [ ] 7.3 旧 capability 名的路径引用：实测 `board-management-api` / `board-upgrade` 在 docs/reference 有 change 名溯源（不需改），检查有无 `openspec/specs/<旧cap>` 路径引用

## 8. 归档

- [ ] 8.1 `archive-readiness.sh merge-board-upgrade-api-specs` 六项全绿
- [ ] 8.2 `openspec archive merge-board-upgrade-api-specs`
- [ ] 8.3 归档后：
  - [ ] 8.3a `board-api` 补 `<!-- domain: semantic-board, api, ui -->`（成员并集正好 3 域，见 design D6）
  - [ ] 8.3b `board-upgrade` **手改 Purpose**为族级文案（就地扩容时 delta Purpose 被忽略）
  - [ ] 8.3c `board-api` 的 Purpose 由 delta 带入，核验非空

## 9. 测试

零代码改动，验证即结构与守恒：

| 验证点 | 命令 | 期望 |
| --- | --- | --- |
| delta 结构 | `openspec validate merge-board-upgrade-api-specs` | valid |
| 全仓 spec | `openspec validate --specs` | 153 passed |
| 条数守恒 | ADDED 32 = REMOVED 32；48 = 32 + 16 | 恒等 |
| Scenario 守恒 | 74 + 70 = 144 | 恒等 |
| REMOVED 标题匹配 | 4 个 delta vs main spec 逐字 | 32/32 |
| 归档退役 | 归档后 spec 目录数 | 153，无空壳 |
| board-concept-management 不动 | `git diff` | 空 |

## 10. 文档

<!-- doc-impact: flow, research -->

- `docs/reference/flow/semantic-board.md`：变更溯源表加一行（7.1）
- `docs/research/process-arch-rework/issues.md`：ISS-04 进度回填（7.2）

## 11. 验收

- [ ] A `board-upgrade` 23 req（16 保留 + 7 新增）、`board-api` 25 req
- [ ] B 每条新搬入的 Requirement 带 `<!-- from: -->`，可 grep archive 验证
- [ ] C 4 个旧 capability 已退役；`board-concept-management` 原样保留
