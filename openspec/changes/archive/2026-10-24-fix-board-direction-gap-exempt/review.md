# Review：fix-board-direction-gap-exempt

> 步骤 4 review 意见建账。审查人：change-review-glm（zai-coding-cn/glm-5.3）子线程，2026-10-22；范围：五制品 + 工作树 diff + flow 红线 4 一致性。

### R-1 2026-10-22【Medium】skip 用例未测到声称机制（M1）
- **状态**：已修复
- 原 `gap exemption also skips relative gate` 用例：默认配置（margin 0.10 < relative 0.15）下 skip 是数学死分支，B 实由绝对门拦截、A 的 skip 分支未被触发覆盖。已重写为 margin 0.20 > relative 0.15 构型（A 差值 0.16 豁免带内、同轮 gap ≥ 0.15）真正钉住 skip，注释与断言消息同步更正（`semantic_board_matching_unit_test.go:TestGapExemption/gap exemption also skips relative gate (margin > relative margin)`）。

### R-2 2026-10-22【Medium】floor 收回豁免后相对门仍 skip（M2，spec 一致性）
- **状态**：已修复
- `applyDirectionRelativeGate` 跳过集合不带 floor 条件，被 floor 收回豁免的挂载只回退绝对门，与 spec Scenario「回退双门判定」不符（默认配置布尔 OR 掩盖，margin>relative 且 floor>threshold 时可观测）。处置选代码侧修复：skip 判定补与 `directionMismatch` 闭包同式的 floor 条件（`semantic_board_matching.go:applyDirectionRelativeGate`），并补针对性用例 `floor-revoked mount falls back to relative gate (review M2)`（margin 0.20 / floor 0.65 / gap 0.16 构型）。design D1 已同步留痕。

### R-3 2026-10-22【Medium】工作树 diff 混叠 fix-board-ghost-false-positive 未提交代码（M3）
- **状态**：已采纳
- HEAD 上无 ghost change 代码（`git show HEAD | grep computeGlobalBestBoards` 为空），本 change diff 与其叠加，增量不可单独切出。处置：**归档提交时按 hunk 拆两 commit**（ghost 先、本 change 后，与 design D4 归档顺序一致；`git add -p` 按 hunk 划分——floor 相关键/测试属 ghost，margin 相关键/测试属本 change，`semantic_board_matching.go` 中改名 hunk 属本 change）。本条为建账留痕（先例：fix-digest-tag-leak review M1），不阻塞 apply 阶段。

### R-4 2026-10-22【Low】config 测试文件未过 gofmt（L1）
- **状态**：已修复
- `TestConfigDirectionFloor` 循环内缩进错位（ghost change 未提交 hunk 引入）。已 `gofmt -w` 归位，格式修复跨 hunk 无害。

### R-5 2026-10-22【Low】custom margin Scenario 行为半边无端到端断言（L2）
- **状态**：已修复
- 补 `custom margin widens band end-to-end`（margin 0.15 下差值 0.12 豁免、默认 0.10 会标）。

### R-6 2026-10-22【Low】spec「保持现值」措辞与实现语义有隙（L3/L4/L6）
- **状态**：已修复
- delta spec ADDED Requirement：措辞改「拒绝并保持默认 0.10」（与 Scenario 断言及 margin 类 parse 继承语义一致）；补「有效带宽 = max(margin, 1e-9)」（亚噪声区间归一）；补「DB-only 键，不经 matching-config API 白名单」与热生效 TTL 时滞说明（design D2 同步）。

### R-7 2026-10-22【Low】白盒表手法/覆盖未同步 + 1.0 上界无断言（L5）
- **状态**：已修复
- test-cases.md 白盒分支表改注 0.10001/0.09999 两侧锁定法（浮点方向不可控）；`TestConfigDirectionGapExemptMargin` 补 `upper bound 1.0 accepted`（闭区间接受）。

### R-8 2026-10-22【Low】test-cases.md 效果核对表未同步实测（L7）
- **状态**：已修复
- 效果核对表补「实测」列：日本 1 ✅ / 美国 48 ✅ / 陈旧 0 ✅ / 全库 229（+63 相对门非冤案，引 tasks 4.1 留痕）；日报恢复收录待 4.2。

- 审毕无发现补充：High 级零发现（审查范围：五制品一致性 + 判定管道代码 + 测试资产 + flow 红线 4，核实记录见审查报告）。
