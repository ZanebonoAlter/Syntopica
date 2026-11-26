<!-- stage: implement -->

# Tasks

## 1. 准备

- [x] 1.1 统计 23 个 spec 的 Requirement/Scenario/行数分布，确认三族映射（tagging 55req/1042行、auxiliary-label 38req/678行、tag-matching 55req/1051行）
- [x] 1.2 从 archive 反查每个 capability 的首个 change（17 个可查，6 个零引用：aux-label-gc、tag-merge-suggestions、tag-merge-ui、tag-match-identity-protection、board-affinity-metadata、board-match-auto-trigger）
- [x] 1.3 确认分层依据（`backend-go/internal/` import 图：tagmanagement 被 reader/topicgraph/discovery/dataenrichment 四包 import）

## 2. 生成合并稿

- [x] 2.1 写 `scripts/harness/merge-spec-family.sh`：按映射抽 Requirement 块（含 Scenario）、插入来源注释、按 D4 规则分组输出到临时目录
- [x] 2.2 生成 3 个族的初稿（`/tmp/spec-merge/{tagging,auxiliary-label,tag-matching}.md`）
- [x] 2.3 逐族核对：Requirement 条数守恒（55/38/55）、Scenario 条数守恒（150/95/170）、无内容丢失

## 3. 人工校对与重组

- [x] 3.1 `tagging`：确认 business 段 41 条在前、impl 段 14 条在后（D4 两段式），Purpose 已由脚本写入，人工过一遍措辞
- [x] 3.2 `auxiliary-label`：确认就地扩容后 23 条 ADDED 是 business(22)/impl(1) 两段，Purpose 措辞过一遍
- [x] 3.3 `tag-matching`：确认 business 38 / impl 17 两段，Purpose 措辞过一遍
- [x] 3.4 检查跨 spec 重复条款（同一规矩在两处出现），重复的合并为一条并在来源注释里列两个来源
- [x] 3.5 发现的内容问题（过时、矛盾、不可测）只记录到 `review.md`，本次不修

## 4. delta 与切换

- [x] 4.0 写 `.openspec.yaml`（`retire_capabilities: true`）——**缺它会整次中止归档**（R-1）

- [x] 4.1 生成 `specs/{tagging,auxiliary-label,tag-matching}/spec.md` 的 `## ADDED Requirements`
- [x] 4.2 生成 23 个旧 capability 的 `## REMOVED Requirements`（脚本从现有 spec 抽标题，逐条带删除原因）
- [x] 4.3 **归档后断言** 22 个 `openspec/specs/<cap>/` 目录已被 openspec 自动退役删除（凭 `.openspec.yaml` 的 `retire_capabilities: true`，不留空壳；R-1 更正了原先「先归档再手删空壳」的错误流程）
- [x] 4.4 归档后补 marker 与 Purpose：
  - [x] 4.4a `tagging` → `<!-- domain: semantic-board, ui -->`（成员并集 7 域装不下，取舍见 design D9）
  - [x] 4.4b `tag-matching` → `<!-- domain: semantic-board, topic-graph -->`
  - [x] 4.4c `auxiliary-label` 保留原 marker，但**手改 Purpose**为族级文案（delta 的 Purpose 对已存在 spec 会被忽略，R-5）
  - [x] 4.4d `tagging` / `tag-matching` 的新 main spec 标题按存量风格改为 `<capability> Specification`（R-14）
- [x] 4.5 `openspec validate --specs` → 158 全过（178 - 22 + 2）

## 5. 验证

- [x] 5.1 `bash scripts/harness/domain-history.sh semantic-board` → 第 6 段列出「仍能经其它 capability 命中本域」的孤儿 delta（实测 15 个 change）；另有 21 个只碰过被删 capability 的 change 会彻底不可见（R-7，见 design D6）
- [x] 5.2 `bash scripts/harness/check-standards.sh` → J 段 3 个新 capability 有合法 marker，0 失败
- [x] 5.3 抽查 5 条 Requirement 的来源注释，grep archive 确认 change 名与日期属实
- [x] 5.4 确认代码零改动：`git status` 里 `backend-go/` 与 `front/` 无本 change 文件

## 6. review

- [x] 6.1 派 review（判断密集型，用 `zai-coding-cn/glm-5.4`）：核对条数守恒、来源注释真实性、有无语义改动
- [x] 6.2 处置 review 发现（High 必修，Medium/Low 记状态）

## 8. 归档

- [x] 8.1 `bash scripts/harness/archive-readiness.sh merge-tag-spec-family` 六项全绿
- [x] 8.2 `openspec archive merge-tag-spec-family`（归档验证：REMOVED 3/3 已实测有效，按标题匹配，Reason 文本不干扰）
- [x] 8.3 归档后跑 `openspec validate --specs` 确认 158 个 capability 无空壳残留

## 9. 测试

本 change **零代码改动**，不产生新的测试用例。「测试」即结构与守恒验证：

| 验证点 | 命令 / 手段 | 期望 |
| --- | --- | --- |
| delta 结构合法 | `openspec validate merge-tag-spec-family` | valid |
| 全仓 spec 合法 | `openspec validate --specs` | 158 passed |
| 条数守恒 | ADDED 133 + 保留 15 = 148 = 原 23 spec 总数；REMOVED 133 | 恒等 |
| Scenario 守恒 | ADDED 381 + auxiliary-label 原位 34 = 415 | 恒等 |
| REMOVED 标题可匹配 | 22 个 delta 的标题 vs 各自 main spec 逐字对比 | 133/133 命中 |
| 语义零改动 | review 全量逐字比对 133/133 | 一致 |
| 归档退役生效 | `openspec archive` 后 `ls openspec/specs/` 计数 | 158，无空壳 |

## 10. 文档

<!-- doc-impact: standard, flow -->

- [x] 7.1 `docs/reference/standard/shared/doc-authoring.md` 补「族级 capability 命名与来源注释」约定
- [ ] 7.2 `docs/research/process-arch-rework/issues.md` 回填 ISS-04 进度（首族已合并，其余 29 个待办）
- [x] 7.3 本 change 无 flow 影响不成立（semantic-board 流程文档需补一句指向新族），在 `docs/reference/flow/semantic-board.md` 变更溯源表加一行
- [x] 7.4 修旧 capability 名的跨文件引用（R-8）：`docs/reference/api/feeds.md:55`（source-board-hit-rate）、`api/semantic-boards.md:128,153`、`flow/semantic-board.md:500,522`、`flow/reading.md:212` → 改指新族；另有 4 个 main spec 的指针（board-concept-management ×3、board-health-checkup:312、analysis-pause-control:87、board-editing-ui:17）列为本 change 内的仅改指针 MODIFIED
- [x] 7.5 归档后跑 `domain-history.sh` 复核孤儿损失口径，把「已退役 capability → 族」别名表登记为后续 change（R-7）

## 11. 验收

- [x] A 三个族级 capability 各自包含原成员的全部 Requirement，条数守恒（55/38/55）
- [x] B 每条 Requirement 带 `<!-- from: ... -->` 来源注释，可 grep archive 验证
- [x] C 22 个旧 capability 在 `openspec/specs/` 下被自动退役；其 delta 中「仍能经其它 capability 命中本域」的 15 个 change 落第 6 段，21 个会彻底不可见（R-7 已知损失，用别名表后续补）

## 备注

- `openspec validate --strict` 的 22 条 WARNING（Requirement 超 500 字符 / 缺 SHALL）全部继承自原 spec，本 change 不改语义故不修；归档门禁不含 strict，不阻断（R-11）
