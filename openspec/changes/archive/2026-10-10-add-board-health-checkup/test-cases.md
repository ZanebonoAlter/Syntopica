# test-cases.md — add-board-health-checkup

> 故事主线（版块体检日常运转）：日报完成 → 体检自动跑 → 快照落库（LLM 评分）→ 低分触发通知 → 用户点通知跳板块看体检卡（评分/评语/指标/趋势）→ 手动触发扩充生成 → member-derived 候选进 LLM 裁决 → pending 列表勾选子集确认执行。

## 主链路表

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 日报 job 成功完成 | 体检在日报后自动执行（board-health-checkup） | RunCheckup 被同步调用 | 单测(mock) | scheduler job_daily_report_test.go |
| 2 | 体检统计版块级指标 | 体检统计与标签贴合度归因（版块级部分） | matched/reason_dist/ghost/vector_missing 齐全（无单 aux 占比——已退役） | 函数单测 | boardhealth metrics_test.go |
| 3 | 标签级贴合度分布 | 构成标签贴合度分布全覆盖 + cosine 重算 + 规则分不作输入 | 每构成标签对全部有向量挂载 tags 的分布；规则分未进样本 | 函数单测 | boardhealth labels_test.go |
| 3b | 低分线板块内相对分位 | 低分线按板块内相对分位 | low_sim_ratio 按板块内 P25 线计算，非固定 0.5 | 函数单测 | boardhealth labels_test.go |
| 4 | 样本不足标签 | 样本不足显示数据不足 | insufficient=true 分布字段 null（含构成标签自身缺 embedding） | 函数单测 | 同上 |
| 5 | 事实画像 diff | 高频未收编标签进入候选清单 | 候选含频次、排除已挂载/disabled | testcontainer PG | boardhealth repo_test.go |
| 6 | LLM 综合评分 | LLM 产出评分与评语 | score+comment+notify 落快照 | 单测(mock LLM) | boardhealth llm_test.go |
| 7 | LLM 失败降级 | LLM 失败降级不丢统计 | 统计照常、score=NULL、轮次继续 | 单测(mock) | 同上 |
| 8 | 快照落库 | 每轮落一条快照 | 新增一行，历史不变 | testcontainer PG | repo_test.go |
| 9 | 异常通知 | 异常且裁决值得则通知 | notification 行 LinkType=board | 单测 | boardhealth notify_test.go |
| 10 | 裁决不值得/冷却 | 裁决不值得则静默 + 冷却期内不重复 | 无新通知行 | 单测 | 同上 |
| 11 | 体检轮次编排 | 体检轮次单版块失败隔离 | 失败版块记日志，兄弟继续 | 函数单测 | boardhealth run_test.go |
| 12 | 体检不改匹配数据 | 体检不改匹配数据 | 匹配表无写入 | 单测 | 同上 |
| 13 | 查询体检数据 | 查询最新快照 + 趋势序列 | latest+trend 升序 | handler 测试 | board handler_test.go |
| 14 | 无快照空态/404 | 无快照返回空态 / 版块不存在 404 | 空态非错误；404 | handler 测试 | 同上 |
| 14b | 面板无快照降级 | （新增 scenario 同源：未收编候选可一键收编 + ui 无快照态） | 徽章「等待首轮体检」+ 质量列「待体检」+ 构成管理照常 | 组件测试 | BoardHealthPanel.test.ts |
| 14c | 候选一键收编 | 未收编候选可一键收编（board-health-checkup） | 调既有 addComposition + toast + 刷新，不走 LLM 管道 | 组件测试 | BoardHealthPanel.test.ts |
| 14d | 构成管理平移（含手动指定路径） | ui-design Interaction Contract（picker/移除/组合） | 添加（版块推荐+手动指定双路径）/移除/组合创建与原行为等价 | 组件测试 | BoardHealthPanel.test.ts |
| 14e | 长列表折叠 | ui-design Interaction Contract 长列表防炸 | 默认 top 10 + 展开全部切换 | 组件测试 | BoardHealthPanel.test.ts |
| 14f | 双主题两档渲染 | theming.md 硬约束 + ui-design Acceptance | editorial/dark 两套主题下全量渲染正常，无硬编码色 | 人工（双主题切换视觉检查） | 人工（9.5 步骤内补双主题切换核验） |
| 15 | 手动扩充生成 | member-derived 候选被召回 | 快照候选进 LLM，不走全库扫描 | 函数单测 | board semantic_board_expand_test.go |
| 16 | 无快照回退 | 无体检快照回退全库召回 | 双路召回原行为 | 函数单测 | 同上（既有用例改造，见⓪） |
| 17 | 扩充确认勾选子集 | 确认执行保持逐条勾选 | 仅勾选行挂载 | 既有测试 | 既有 confirm 用例（不动） |

## 变体走查（五组固定清单）

| 组 | 变体 | 答案 |
| --- | --- | --- |
| 输入 | trend=0 / 91 / 非整数 / 负数 | 白名单外一律 400（端点校验，handler 测试覆盖 0 与 91 边界） |
| 输入 | 空串/纯空白/分隔符/单token/大小写/特殊字符/超长 | 不适用（无自由文本输入端点；LLM prompt 数据为服务端聚合，删划） |
| 前置 | 空集：版块零挂载 tags | 版块级指标全 0、labels 空、候选空——正常快照落库（run_test 覆盖） |
| 前置 | 单元素：版块仅 1 构成标签 | 正常统计（挂载样本不足则 insufficient） |
| 前置 | 重复：同日两轮体检（手动补日报） | 两条快照并存，趋势原样展示（spec 已定不去重） |
| 前置 | 越界引用：版块不存在/已删 | 端点 404；体检轮次按活跃版块遍历天然不含 |
| 前置 | 部分满足：部分构成标签样本不足 | 逐标签独立 insufficient 标记，互不影响 |
| 时间窗口 | 冷却期边界：恰第 3 天 | `< cooldown_days` 严格小于判定，第 3 天当天可再通知（notify_test 边界用例） |
| 时间窗口 | 趋势跨窗口/空窗口 | trend 按条数不按日期窗口；空窗口=只有 1 条时 trend 长度 1 |
| 时间窗口 | days>0 主路过滤边界 | 近 days 天引用 EXACT 边界沿用既有相似路实现（days 当天含，同现有口径） |
| 幂等 | 体检重复执行 | 每轮新增快照（设计如此，无幂等诉求） |
| 幂等 | 扩充生成重复触发 | suggestion_hash 幂等既有管道兜底（不动，既有测试覆盖） |
| 幂等 | 通知重复裁决 | 冷却查询拦截（步 10） |
| 并发 | 声称线程安全？ | 否——单 job 顺序执行，划除 |
| 可用性(UI) | 空态（无快照） | 「等待首轮体检」引导文案（BoardHealthPanel empty 态断言） |
| 可用性(UI) | 错误态+重试 | 错误文案+重试按钮（同 BoardSourcePanel 模式） |
| 可用性(UI) | 加载态 | 骨架文案（loading 态断言） |
| 可用性(UI) | 超长文本（评语） | 评语区 CSS ellipsis 不撑爆面板（组件测试断言 class 存在） |
| 可用性(UI) | 超长列表（标签一多） | 质量表/组合/候选默认 top 10/5 + 「展开全部 (N)」切换；picker 分页由复用组件自带（组件测试断言展开切换与默认截断） |
| 可用性(UI) | 误输入/重复提交 | 收编/移除/添加均有 confirm 或按钮态钡防重复；picker 确认钡可防连点（组件测试断言 disabled 态） |

## ⓪ 继承与调整

### 口径修正（2026-10-08，标签级归因改贴合度分布）

board-health-checkup 为本 change ADDED 能力（主 specs 无历史资产，test-assets.sh 不适用）；旧资产 = 本 change 已实现的 boardhealth 测试，按新契约逐行处置：

| 旧 Scenario | 处置 | 旧测试 | 动作 |
| --- | --- | --- | --- |
| 单 aux tag 匹配行归因到唯一标签 | 删（新口径不筛样本） | labels/metrics_test 归因用例 | 重写为贴合度全覆用例 |
| 多 aux tag 不进入标签级归因 | 删（新口径多 aux 也进样本） | 同上 | 删断言，补多 aux tag 计入用例 |
| 单 aux 占比指标 | 删（指标退役） | metrics_test 单 aux 占比断言 + 前端 chip 断言 | 删 |
| cosine 重算进入分布（tag×board） | 改写（tag×aux） | labels_test | 改输入夹具与断言 |
| 向量缺失跳过且计数 | 保留（语义兼容，主语从 tag×board 改 tag×aux） | 同上 | 断言微调 |
| 样本不足 insufficient | 保留（语义不变：样本<门槛） | 同上 | 保留，补构成标签缺 embedding 分支 |
| 低分线（固定 0.5） | 改（板块内 P25 相对线） | labels_test low_sim 用例 | 重写断言 |

### board-upgrade-expand MODIFIED（原有）

| 旧 Scenario | 处置 | 旧测试 | 动作 |
| --- | --- | --- | --- |
| 相似标签被召回 | 保留（限定回退路语义） | semantic_board_expand_test.go | 改造：夹具保证目标版块无体检快照（走回退路）后原断言保持 |
| 共现标签被召回 | 保留（回退路） | 同上 | 同上 |
| 已挂载标签不召回 | 保留（双路语义不变） | 同上 | 保留原断言 |
| 无关标签不被召回 | 保留（回退路） | 同上 | 随夹具改造自动归入回退路 |
| 时间窗收紧相似路召回 | 保留（回退路） | 同上 | 随夹具改造保留 |
| 时间窗收紧共现窗口 | 保留（回退路） | 同上 | 保留 |
| 组合路候选相关性过滤 | 保留（本 change 不改组合路） | 同上 | 不动 |
| 召回为空 | 扩展（主路空且回退空） | 同上 | 改造：补主路候选为空+回退为空的组合分支 |
| 版块画像进 prompt / 二分类裁决 | 保留（LLM 管道零改动） | semantic_board_upgrade_test.go | 不动 |
| 锁定版块生成 / 目标版块被禁用后确认失败 | 保留 | handler/expand tests | 不动 |
| （新增）member-derived 候选被召回 / 时间窗收紧 member-derived | 新增 | semantic_board_expand_test.go | 新用例（步 15 + days 过滤变体） |

（口径修正新增 Scenario 的落点见主链路步 3b 与白盒附加「贴合」行。）

## 效果核对

- **触发原因**：贴合度口径修正后体检管线产出是否恢复判别力（旧单 aux 口径实测：标签级大面积「数据不足」，板块 3030 仅 6/96 标签有样本）。
- **方法**：部署后首轮体检（10.x 任务落地后重跑日报触发）跑完，SQL 聚合 `board_health_snapshots` 首日新口径数据。
- **量化结果**：记录（a）有分布的构成标签占比（目标 ≥90%，旧口径实测 ~6%）；（b）板块内 cosine_mean 极差（大板块目标 ≥0.1，实测预验 0.19~0.21）；（c）llm_score NULL 占比（LLM 降级率）；（d）未收编候选非空版块数 / 总版块数。
- **结论**：有分布占比仍低 → 查向量覆盖率；极差全板块趋近 0 → 小板块为主属预期；降级率异常高 → 查 airouter 路由；候选全空 → 查画像 diff 查询。写入归档前会话记录。
- **实测结果（2026-10-07 21:33 轮，已达标）**：（a）有分布标签占比 11/12 板块 100%（目标 ≥90%），唯一例外刚果疫情挂载量 1 < min=5 全 insufficient 属正确行为；（b）大板块极差：3030 为 0.544~0.730（≥0.1 ✓）；（c）llm_score 55~82 无 NULL（降级率 0）；（d）未收编候选非空 11/12。

## 白盒附加（RunCheckup 与召回分支）

| 分支 | 输入 | 期望 |
| --- | --- | --- |
| 轮次：零活跃版块 | 版块表空 | 空轮次成功返回（无快照） |
| 轮次：单版块统计失败 | 版块 A 查询 err | 记日志继续 B，轮次成功 |
| 轮次：LLM 全失败 | mock LLM 全 err | 全部快照 score=NULL，轮次成功，零通知 |
| 评分：边界阈值 | score 恰=alert_threshold | 不通知（严格小于） |
| 归因：samples 恰=min_sample_size | 默认 5 时 samples=5 | 达标出分布（≥ 判定）；samples=4 insufficient |
| 贴合：多 aux tag 也进样本 | 某 tag 挂 3 个 aux 且有向量 | 计入全部构成标签的贴合分布（不筛选） |
| 贴合：构成标签缺自身 embedding | 某 aux embedding 为空 | 该标签 insufficient，不计入 vector_missing 干扰 |
| 低分线：板块内 P25 | 全部标签 mean 落在同一簇（小板块） | P25 线自适应簇内位置，不误报全板块低分 |
| cosine：向量全缺失 | 版块挂载 tags 全无 identity 向量 | vector_missing=matched 数，labels 全 insufficient |
| 召回：主路候选含已挂载 | 快照候选与构成重叠 | 保险滤后剔除 |
| 召回：days=0 | 手动生成不带 days | 主路候选不过滤（与既有口径一致） |
