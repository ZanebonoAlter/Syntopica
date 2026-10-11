# test-cases（simple 档精简：specs Scenario 即用例，此处只记主链路落点与变体走查）

## 主链路表

| # | 场景 | 落点 | 期望 |
| --- | --- | --- | --- |
| 1 | 情报型不进失效名单（spec「情报型 warn 不进失效名单」） | smoke 新 fixture：concurrent-mixed ×6 ≥ 阈值 5 | 失效名单为空；情报型汇总行 concurrent-mixed=6 |
| 2 | 混合两类分开统计（spec「混合两类时分开统计」） | smoke 新 fixture：+ full-go-test ×6 | 失效名单仅 full-go-test；`soft.warn_total`=12、corrective=6、informative=6，均由 fixture 独立复算 |
| 3 | 未知 reasonCode 缺省纠正型 | smoke fixture：未知词汇 warn ×5 | 进失效名单（最强监督口径） |
| 4 | 阈值只作用于纠正型 | 同 fixture 用 `--warn-threshold 100` 跑 | 失效名单空；情报型汇总行照常输出 |
| 5 | 基线兼容 | 旧语义基线文件 + `--baseline` | 正常读入；新键按无基线处理不误报 |
| 6 | 真库回检 | `--days 7` 真库 | 失效组 3→0；情报型 265 条（239/16/10）与 sqlite 直查一致 |

## 变体走查

- `--change NAME` 过滤下分类统计：既有窗口过滤 SQL 路径复用，分类 CASE 与过滤正交 ✓
- 情报型与纠正型同 reasonCode 前缀不同 policy（如 spec-gate/concurrent-dirty-tree vs quality-gate/concurrent-mixed）：白名单按 reasonCode 匹配即可，policy 无关（两例均情报型）✓
- 白名单空集（未来全删）：退化为现行为（全部按纠正型），无损 ✓
- ③ 段 fail-open（gate-lock-held 等）不受影响：action 过滤不同路径 ✓

## 效果核对

- 下次体检报告 ④ 段：失效组 0（改前 3 全为情报型误报）；「软提醒失效」语义恢复对纠正型（full-go-test / fuzzy-model-resolve / acceptance-wording / ui-impact-missing）的监督力。
- 观察窗口 7 天后 `--baseline` 对比：`soft.warn_total_informative` 作为并发频率趋势键（不再读成催修失效）。
