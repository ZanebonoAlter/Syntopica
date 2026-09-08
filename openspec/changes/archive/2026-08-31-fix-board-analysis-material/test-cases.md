# 白盒用例 · fix-board-analysis-material

> 复杂档白盒用例（开发执行规范 §2）。断言判据由主线程定（design D1–D5）；黑盒 Scenario 在 specs/*/spec.md。

## ⓪ 继承与调整（test-design 四问之⓪：改契约了吗）

本 change MODIFIED 两条 requirement（态势卡素材装配、探索 agent 工具集），反查旧测试资产：

| 旧 Scenario | 处置 | 旧测试 | 动作 |
|---|---|---|---|
| 态势卡：泳道多时素材仍可控 | 保留（未变） | situation_cards_test.go（M1.5） | 不动 |
| M1.1 week lifeline 优先 | 保留（week 优先级不变） | situation_cards_test.go | 不动，另加生产形态对照 |
| M1.2/M1.3 无 lifeline 降级指纹 | **契约变化**：指纹必须带实质内容 | situation_cards_test.go | 改造：断言指纹含 thread 标题 |
| M6.x get_lane_detail（原有场景） | 保留 + 新增档案段 | lifeline_renderer_test.go | 补空归档实现后回归 |

## M1 取材链矩阵（tasks 1.1）

facts_source 判定（lanes 的 lifeline 状态 × 降级链）：

| # | 条件 | 期望 facts_source | 判据 |
|---|------|------|------|
| M1.1 | week 在库（生产形态对照：**month 在、week 缺**） | `lifeline_month` | month 最新 2 期压缩拼接；full 卡 120 rune 截断 |
| M1.2 | week + month 均在库 | `lifeline_week` | week 优先级不变（回归） |
| M1.3 | 仅 month，且 month 内容为空串 | 走指纹 | 空串=无内容（同旧 week 语义） |
| M1.4 | 无任何 lifeline、section 有 threads | `section_fingerprint` | 指纹=`[日期] 标题1 \| 标题2 (N篇)`，**不含纯 cluster_label 同义反复** |
| M1.5 | 无 lifeline、section 无 threads（titles 空） | `section_fingerprint` | 退回 cluster_label（不比现状差），不报错 |
| M1.6 | 无 lifeline、无 section、有 description | `description` | 现状回归 |
| M1.7 | 全无 | `none` | 卡片仍产出（命中统计+质量信号），M1.7 语义 |
| M1.8 | month 在库 + 稀疏历史 ≥2 | brief 卡 | 降级详略规则不受 facts_source 影响 |
| M1.9 | 密度信号 | 有 month/week 归档加权 | 公式固化：density = 篇数分 + month(+2)/week(+2) 可用性分 |

## M2 指纹提质（tasks 1.2）

| # | 条件 | 期望 |
|---|------|------|
| M2.1 | 单 section 3+ 条 threads | 取前 3 条标题拼接 |
| M2.2 | 多 section | 各 section 独立 `[日期] …` 段拼接，整体 120 rune 截断 |
| M2.3 | thread 标题超长 | rune 截断不撕裂多字节字符（沿用 truncateRunes） |

## M3 get_lane_detail 档案段（tasks 2.1/2.2）

| # | 条件 | 期望 | 判据 |
|---|------|------|------|
| M3.1 | month 2 期 + year 1 期在库 | 渲染 `## 历史背景记忆（月/年档案）`，每期带粒度+period+as_of | 段落在「逐日演进」与「section 关联关系」之间 |
| M3.2 | 归档总量 > 4000 rune | 截断（month 新→旧、year 兜底） | 末尾含 `[档案截断]` 标注；总长 ≤ 预算+标注 |
| M3.3 | 无任何归档行 | 段落输出 `（无背景记忆归档）` | 不静默省略段落 |
| M3.4 | 既有消费方（单泳道 QA 等 fake reader） | fake 返回空切片 → 渲染走 M3.3 路径 | 既有测试零破坏（仅补接口方法） |
| M3.5 | 归档行选取 | month 最新 2 期 + year 最新 1 期 | period DESC 排序断言 |

## M4 前端收口（tasks 3.1）

| # | 条件 | 期望 | 判据 |
|---|------|------|------|
| M4.1 | 页面渲染 | 泳道下拉仅聚焦区一处 | `selectedTopicId` 单一绑定点；顶栏旧 toolbar 不存在 |
| M4.2 | 新闻背景入口 | 折叠 section 形态，非单 tab 栏 | subtabs 导航组件移除；展开后周期筛选/翻历史可用 |
| M4.3 | 刷新入口 | 版块分析区头部按钮 | 原 toolbar 刷新功能保留 |
| M4.4 | 聚焦分析/QA | 折叠区展开即达 | 行为与改版前一致 |

## M5 效果核对（tasks 4.2，真库量化）

- 触发：生产库跑 `AssembleSituationCardsForTest`（当前活跃板块）
- 量化：facts_source 分布——预期 `lifeline_month` ≥80%（67/69 泳道 month 在库）
- 不达标：标记素材缺口，回流 lifeline 补齐议题（不在本 change 扩 scope）

## 测试落点对照

| 模块 | 测试文件 | 层 |
|------|----------|------|
| M1/M2 | `backend-go/internal/dataenrichment/service/situation_cards_test.go`（改造） | 单测（sqlite→既有测试库形态） |
| M3 | `lifeline_renderer_test.go` + 生产 wiring 测试 | 单测 |
| M4 | `front/app/features/tags/components/BoardEnrichmentPanel.test.ts`（如无则新建轻量断言） | unit |
| M5 | 临时核对入口（不落测试资产，结果记 tasks.md） | 真库 |

## M7 分析触发异步化（8.x 追加）

| ID | 场景 | 变体/边界 | 层 | 锚点 | 验收措辞 |
| --- | --- | --- | --- | --- | --- |
| M7.1 | 触发后客户端断连，分析照常完成 | 父 ctx 取消不传播进后台 fn | 后端单测（runner） | analysis_runner_test.go | 父取消后 job 仍 finished、result_id 正确、无 error |
| M7.2 | 同目标在跑时重复触发 | 不同目标不受影响；跑完后可再触发 | 后端单测（runner+handler） | analysis_runner_test.go / board_enrichment_handler_test.go | 第二次 ErrAlreadyRunning/409；跑完后再触发 200 |
| M7.3 | 分析 panic / 卡死超时 | panic 变 error 状态；30min 强杀 | 后端单测（runner） | analysis_runner_test.go | 进程不崩、状态可见 error / 超期 finished |
| M7.4 | 状态轮询链路 | 从未触发=空闲；完成带 result_id；失败带 error | 后端 handler 测试 | board_enrichment_handler_test.go | status 响应字段齐、详情接口可拿 sectors |
| M7.5 | 未开启板块同步拒绝 | not enabled → 400（异步化后语义保留） | 后端 handler 测试 | board_enrichment_handler_test.go | 预检失败立即 400，不启动后台 job |
