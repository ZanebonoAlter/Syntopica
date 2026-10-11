
## 方向校验相对门 margin=0.15 数据定值依据

margin=0.15 定值依据（2026-10-10 真库全量调研，topic_tag_board_labels 2596 行 / 非composite活挂载+已拦挂载，identity embedding × board embedding cosine）：

1. 绝对水位无区分度（证实绝对阈值失效）：各 match_reason 的 identity cosine 分布高度重叠——direct_hit p50=0.678 vs weighted p50=0.634（差仅0.04）；min 0.45~0.51 / max 0.84~0.89。
2. gap（组内最佳版块 cos − 该挂载 cos）区分度良好（多挂标签 n>1 共 1582 挂载行）：P50=0.025~0.031，P90=0.075~0.137，P95=0.103~0.176，max=0.213~0.272。
3. gap≥0.15 抽样 18/18 全为肉眼可判串味（俄乌事件→美国新闻/AI版块、OpenAI事件→中东/全球科技/美国新闻；本 change 触发案例 OpenAI→中东 gap=0.250 在列）。
4. gap 0.10~0.15 区间开始混杂合法双归属（乌美会谈→美国新闻 0.149、国内大模型厂商→全球科技巨头 0.148、Claude开放大陆→全球科技 0.145）→ margin 不能低于 0.15。
5. 影响面：多挂挂载中 gap≥0.15 共 114 行（direct_hit 5/183=2.7%、hit_rate 28/336=8.3%、max_sim 34/430=7.9%、weighted 47/633=7.4%），与抽样串味比例吻合。
6. 单挂标签（929 行）gap≡0，相对门恒过，绝对阈值继续兜底——相对门只对多挂标签生效。

结论：margin 默认 0.15（ai_settings key semantic_board_match_direction_relative_margin），0 恒可关闭回退纯绝对门。参照集=该标签本次匹配上的全部版块（含将被标 mismatch 的候选），best=组内 identity cos 最大者；composite_hit 豁免（红线：score恒1.0且mismatch恒false）；direct_hit 一并纳入相对门（红线：direct_hit 降级后与间接规则一样强制方向校验）。

<!-- pinned 2026-10-10T16:10:15Z -->
