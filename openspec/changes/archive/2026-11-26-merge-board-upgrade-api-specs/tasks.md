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
- [ ] 3.4 内容问题登记（本次不修，见 review.md）：
  - [ ] 3.4a **跨族重复 4 组**（R-3）：升级建议**触发/生成**（board-api 写了「执行聚类 + LLM 判断流程」，越界进算法）、**确认执行**（DTO Scenario vs ConfirmSuggestion API）、**回填**（回填触发 API vs 手动回填队列，同一组 all/unassigned/board 模式）、**升级建议面板**（3 条各落两族）
  - [ ] 3.4b **两条矛盾端点路径**（R-4）：`/upgrade/suggest` 与 `/upgrade-suggest` 并存，代码只有后者——原 spec 既有矛盾，合并后同族更显眼，应另立 change 修
  - [ ] 3.4c **红线 6/7 弱覆盖**（R-6）：只有 compose 场景半句「复用现有机制」，未覆盖 create_new / merge_into_existing，未写 14 天默认值与冷却起算点
  - [ ] 3.4d **两条 Requirement 逐字重复 Scenario**（R-11）：升级建议 API 与 DTO
  - [ ] 3.4e **两条一次性迁移记录**（R-12）：watch 清理 / discover_new 置 dismissed

## 4. delta 与切换

- [x] 4.0 写 `.openspec.yaml`（`retire_capabilities: true`）
- [x] 4.1 生成 2 个族的 `## ADDED Requirements`
- [x] 4.2 生成 4 个旧 capability 的 `## REMOVED Requirements`
- [x] 4.3 归档前摘掉 **5 个**被退役 spec 的 `<!-- domain: -->`（upgrade-expand / time-window / management-api / article-api / editing-ui；board-upgrade 就地扩容不退役）；与 8.2 同一提交
- [x] 4.4 **归档后**断言 5 个目录已自动退役
- [x] 4.5 **归档后** `openspec validate --specs` → **152** 全过（156 − 5 退役 + 1 新建 = 152）

> 修正：156 − 5（退役）+ 1（board-api 新建）= **152**（board-upgrade 就地扩容不新增）

## 5. 验证

- [x] 5.1 `domain-history.sh semantic-board` → 第 6 段列出指向 5 个旧 capability 的孤儿 delta（实测记数）
- [x] 5.2 `check-standards.sh` → 152 个 spec marker 全合法
- [x] 5.3 抽查 6 条来源注释，grep archive 确认属实
- [x] 5.4 确认 `board-concept-management` 未被触碰（git diff 为空）

## 6. review

- [x] 6.1 派 review（`zai-coding-cn/glm-5.4:max`）
- [x] 6.2 处置 review 发现

## 7. 文档

- [x] 7.1 `docs/reference/flow/semantic-board.md` 变更溯源表加本 change 一行
- [x] 7.2 `docs/research/process-arch-rework/issues.md` 回填 ISS-04（第三族）
- [x] 7.3 旧 capability 名引用扫描：路径引用 0 处；**能力名引用 3 处**（`board-diagnostics:222` → `board-upgrade-expand`；`board-concept-management:5,26` → `board-management-api`）
- [ ] 7.4 **登记「跨族引用清扫」change**（R-5 + 上族遗留）：统一修所有族合并后残留的旧 capability 名引用——本 change 的 3 处、上族的 `tag-matching`「详见 board-direction-check」1 处；改 main spec 需 MODIFIED delta，且会打破各族的守恒账，故集中一批做
- [ ] 7.5 **登记「升级建议跨族收口」change**（R-3）：把 `board-api` 里越界的生成侧描述降为指针（只留端点/DTO/面板行为），把 `board-upgrade` 的「手动回填队列」并入接口族；顺带修 R-4 的两条矛盾端点路径（`/upgrade/suggest` vs `/upgrade-suggest`）、R-11 的重复 Scenario、R-6 的红线 6/7 独立条款

## 8. 归档

- [x] 8.1 `archive-readiness.sh merge-board-upgrade-api-specs` 六项全绿
- [x] 8.2 `openspec archive merge-board-upgrade-api-specs`
- [ ] 8.3 归档后：
  - [x] 8.3a `board-api` 补 `<!-- domain: semantic-board, api, ui -->`（成员并集正好 3 域，见 design D6）
  - [x] 8.3b `board-upgrade` **手改 Purpose**为族级文案（就地扩容时 delta Purpose 被忽略）
  - [x] 8.3c `board-api` 的 Purpose 由 delta 带入，核验非空

## 9. 测试

零代码改动，验证即结构与守恒：

| 验证点 | 命令 | 期望 |
| --- | --- | --- |
| delta 结构 | `openspec validate merge-board-upgrade-api-specs` | valid |
| 全仓 spec | `openspec validate --specs` | 152 passed |
| 条数守恒 | ADDED 32 = REMOVED 32；48 = 32 + 16 | 恒等 |
| Scenario 守恒 | 74 + 70 = 144 | 恒等 |
| REMOVED 标题匹配 | 5 个 delta vs main spec 逐字 | 32/32 |
| 归档退役 | 归档后 spec 目录数 | 152，无空壳 |
| board-concept-management 不动 | `git diff` | 空 |

## 10. 文档

<!-- doc-impact: flow, research -->

- `docs/reference/flow/semantic-board.md`：变更溯源表加一行（7.1）
- `docs/research/process-arch-rework/issues.md`：ISS-04 进度回填（7.2）

## 11. 验收

- [x] A `board-upgrade` 23 req（16 保留 + 7 新增）、`board-api` 25 req
- [x] B 每条新搬入的 Requirement 带 `<!-- from: -->`，可 grep archive 验证
- [x] C 5 个旧 capability 已退役；`board-upgrade` 与 `board-concept-management` 原样保留
