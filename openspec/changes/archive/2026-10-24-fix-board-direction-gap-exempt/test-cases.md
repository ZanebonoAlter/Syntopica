# Test Cases: fix-board-direction-gap-exempt

> 层选择：判定纯逻辑 → 函数单测（`semantic_board_matching_unit_test.go` 扩展）；效果 → 真库量化核对（⑤d）。无 UI（ui-impact none）。复杂档白盒附加见文末。

## ⓪ 继承与调整（改契约：MODIFIED「Direction check for non-direct_hit matches」豁免条件）

`bash scripts/harness/test-assets.sh board-direction-check` 反查（2026-10-21）：

| 旧 Scenario（主 spec 22 个） | 处置 | 旧测试 | 动作 |
| --- | --- | --- | --- |
| hit_rate match passes/fails direction check | 保留（双门语义不变） | semantic_board_matching_unit_test.go | 不动，新增豁免分支用例 |
| weighted match fails direction check | 保留 | semantic_board_matching_unit_test.go | 不动 |
| tag or board embedding unavailable | 保留（缺失跳过语义不变） | semantic_board_matching_unit_test.go | 不动 |
| direct_hit match | 保留（同校验语义不变） | 两文件 | 不动 |
| relative gate 三连（multi-mount leak / dual-mount within margin / single-board unaffected） | 保留（相对门不动） | semantic_board_matching_unit_test.go | 不动 |
| composite_hit exempt | 保留 | semantic_board_matching_unit_test.go | 不动 |
| 数据加载/持久化/日报过滤/API 过滤/direction_sim/三个配置 Requirement | 保留（零改动） | 各自文件 | 不动 |
| **（fix-board-ghost-false-positive delta 未归档资产）global-best 三连 + floor 三组** | **继承并调整**：全库最优（gap=0）恒在新豁免集合内，行为不变 | semantic_board_matching_unit_test.go | 保留断言；margin≤0 回退用例需覆盖「gap<1e-9 豁免、gap≥1e-9 不豁免」旧口径 |

## 1. 主链路故事表（豁免扩围判定）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | tag embedding 就绪，挂载板全库 cosine 差 0.066 < margin 0.10 | near-best mount within gap margin exempted | direction_mismatch=false | 函数单测 | semantic_board_matching_unit_test.go |
| 2 | 挂载板差值 0.214 ≥ 0.10 且 cos 0.597 < 0.6 | mount beyond gap margin still flagged | direction_mismatch=true | 函数单测 | 同上 |
| 3 | 挂载板即全库最优（差值 0） | rank-1 mount always within gap margin | false（与 margin 无关） | 函数单测 | 同上 |
| 4 | margin 置 0，非并列最优低 cos 挂载 | gap margin disabled falls back to tie-only exemption | 回退旧行为 true | 函数单测 | 同上 |
| 5 | floor>0 且 cos<floor 且 gap<margin | floor revokes gap exemption | 豁免被收回，双门判定 | 函数单测 | 同上 |
| 6 | 配置键 0.15 / 未设置 / 非法 1.5 | custom margin applied / default / invalid rejected | 0.15 生效 / 默认 0.10 / 拒绝保持 | 函数单测 | semantic_board_matching_config_test.go |

## 2. 变体走查（五组）

- **输入**：margin=0（关闭）、负值（同 0）、极大 1.0（全豁免——合法但荒谬，越界 1.0 拒绝？→ 设计取 >1.0 拒绝，1.0 为合法上界）；embedding 空向量（跳过）——均有答案（步 4 + 越界用例 + 缺失用例继承）。
- **前置**：单板挂载（gap≡0 恒豁免）；全库仅 1 板（best=mounted，恒豁免）；并列最优 2 板（差值 0 双豁免）；板 embedding 缺失（不入参照系）——并列/缺失继承旧用例，单板/孤板新增。
- **时间窗口**：不适用（无时间语义），划除。
- **幂等**：rematch-all 重算两次标记一致（纯函数判定）；replaceTopicTagBoardLabels 全删全写幂等继承——重算幂等新增一条真库核对（⑤d 步 3）。
- **可用性(UI)**：不适用（ui-impact none），划除。

## 3. 效果核对（真库量化，2026-10-21 已锚定基线）

| 指标 | 重算前 | 预期（gap10 实证） | 实测（2026-10-22，tasks 4.1 留痕） | 核对方式 |
| --- | --- | --- | --- | --- |
| 全库 active mismatch | 682（含 82 陈旧） | ~166±10% | 229（+63 为相对门拦下的高 cos 次优挂载，非冤案，见 4.1） | `SELECT count(*) FROM topic_tag_board_labels WHERE direction_mismatch;` |
| 日本新闻(2128) ghost | 25 | 1 | **1** ✅ | `SELECT metrics->'board'->>'ghost_matches' FROM board_health_snapshots WHERE semantic_board_id=2128 ORDER BY checked_at DESC LIMIT 1;` 次日快照 ≤5 |
| 美国新闻(2197) | 216 | ~46 | 48（±10% 内）✅ | 同上量级 |
| 无 embedding 陈旧标记 | 82 | 0 | **0** ✅ | mismatch 行 join topic_tag_embeddings(identity) 无空 |
| 次日日报 | 日本新闻漏高市早苗类 | 恢复收录 | ✅ 10-22 event_tag_count 8（含高市早苗）/ 10-23 5（含央行），事故期对照 10-20 仅 1（tasks 4.2 留痕） | 人工：日报页抽查 |

## 4. 展示字段盘点

不适用（不改数据结构、无新用户可见字段；direction_mismatch 字段语义不变），划除留痕。

## 5. 白盒附加（分支表）

| 分支 | 边界值 | 用例 |
| --- | --- | --- |
| `bestCos-cos < max(margin,1e-9)` | 差值≈margin（浮点方向不可控，用 0.10001/0.09999 两侧锁定严格 <） | exempt_band_boundary 白盒 |
| margin≤0 回退 | max(0,1e-9)=1e-9 | margin_zero_falls_back |
| floor 收回豁免（闭包） | floor=cos 边界 | floor_at_or_below（ghost change 继承） |
| floor 收回豁免（相对门 skip 同步，review M2） | margin>relative 且 floor>threshold 构型（默认配置不可观测） | floor-revoked mount falls back to relative gate |
| margin 上界 1.0 | 闭区间接受（review L5） | upper bound 1.0 accepted |
| tag embedding 空 | 早退 false | 继承 |
| 挂载板无 embedding | 跳门 false | 继承 |
| 相对门跳过集合=豁免集合（且 floor 放行） | 默认配置数学死分支，margin>relative 时承重（review M1） | gap exemption also skips relative gate (margin > relative margin) |
