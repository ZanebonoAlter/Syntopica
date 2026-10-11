# Design — add-retro-lifecycle-rollup

simple 档，记关键决策与取舍。

## 决策

| # | 决策 | 理由 |
| --- | --- | --- |
| D1 | 挂 ⑦ 效能看板新子组 **E 生命周期面**，不独立成第八段 | ⑦ 本来就是杂烩看板（A-D 子组各有独立数据源），D 组「测试欠账巡检」同为先例（台账/文件面）；独立段会把「七段怎么读」的 skill 表整套重排，收益不成比例 |
| D2 | change 清单 = 全部活跃（changes/ 非 archive）+ 窗口内归档（archive 日期前缀 ≥ 窗口 since） | 漏采信号主要猎物是**活跃** change（还来得及补账）；窗口外归档 change 的反馈账与本次窗口无关，不进分母防稀释 |
| D3 | 反馈解析逻辑与 lifecycle 模式 `parse_feedback` 同词表同降级（文件缺席=无记录、格式异常单列） | 两处口径漂移会让「全局计数」与「单卡明细」对不上账；实现上把解析函数提为两模式共用（lifecycle 已是 python 内嵌函数，全局模式同文件直接复用） |
| D4 | 漏采信号阈值：编辑波次 ≥3 session（与 C 组「跨 session 编辑波次」Top 清单同源同阈值）× 反馈 0 条 | 复用 C 组既有口径，不引入新阈值参数；「≥3」正是 C 组清单的入选线 |
| D5 | 归因分布聚合只报计数不报比率 | 分母（各 change 条数）随建账纪律波动，比率会误导；skill 既有解读口径（高发层→规范整理去向）只需要计数排序 |
| D6 | 不动主 spec「效能看板指标」requirement 的 D 组漂移（实现=测试欠账巡检、spec=健康度沿用） | 夹带 MODIFIED 要全量复制几百行 requirement，本 change 保持最小；漂移记档留独立 change 收敛 |

## 实现要点

- `harness-retro.sh` 全局模式渲染段（⑦ 之后）加 E 子组：change 清单扫描（bash 侧 glob）→ 逐目录调 `parse_feedback`（python 侧既有函数上移共用）→ 波次数据取 C 组已算好的 per-file × per-session 集合按 change 归属（change 名→其 edit.map 归属文件的波次，复用 events 已有数据）。
- edit.map 归属：events 表 change 列 + 归属地图（doc-impact.sh 头注释的 edit.map 机制）；C 组波次是文件维度，需换算为 change 维度（文件的归属 change 的 session 波次）。
- smoke：harness-retro.smoke.sh 造 fixture change（有账+无账+高波次无账=漏采三态）断言输出。

## 风险

- 低：纯输出面新增，零写入不变；json 模式同步补 `m7.lifecycle.*` 键供基线比对。
- 波次换算 change 归属时无归属文件（change 列空的编辑）不进任何 change 的波次计数——与 C 组口径一致。
