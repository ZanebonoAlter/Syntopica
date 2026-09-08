<!-- constraint-domains: data-enrichment, semantic-board -->

## Why

board-level-deep-analysis 端到端验证暴露质量问题，根因是**素材断供**：系统已积累的历史新闻记忆（month/year 背景记忆，67 个泳道在库）在分析链路上无人消费——态势卡装配器只读 week 档（生产库 97% 泳道 week 缺失，`lifeline_weekly` 定时任务从未执行过），降级产物是「标题×篇数」的零信息指纹；D9 新鲜度门花 LLM 成本补齐的 month 档同样不被读取；agent 下钻工具 `get_lane_detail` 也只查 daily_report sections，整条链路没有任何路径能到达背景记忆。结果是分析产出「方法论骨架完整、事实血肉空泛」，观感即"纯按模板输出"。同时前端 `BoardEnrichmentPanel` 把信息架构重排做成了新旧堆叠（双下拉控制同一状态、QA 面板藏在折叠区、单 tab 导航），交互混乱。

## What Changes

- **态势卡素材接入历史记忆（P0）**：`laneFactsDigest` 取材链从「week → section 指纹 → description」扩为「week → **month** → section 指纹 → description」——month 档压缩摘要作为 week 缺失时的主要事实源（生产库 month 覆盖 67 泳道），D9 补齐的成果真正被消费。
- **下钻工具读背景记忆（P0）**：`get_lane_detail` 输出附带该泳道的 lifeline 背景记忆（month/year 归档行，受字符预算约束），agent 下钻不再只能拿到「标题时间线」。
- **section 指纹降级提质（P0）**：最终降级路径的 section 指纹携带实质内容（section 摘要/事实指纹），不再输出「泳道名 (N篇)」的同义反复。
- **前端收口（P1）**：`BoardEnrichmentPanel` 去掉顶部旧话题选择条（泳道选择收敛到聚焦分析折叠区单一下拉）、QA 追问面板挂到版块报告侧、移除单 tab「新闻背景」导航的 tab 栏形态（保留新闻背景入口）。
- **测试以生产形态为准**：本 change 的用例 fixture 必须「month 在库、week 缺失」的生产形态起步（正是上一轮测试盲区：自造 week 数据测分支，绿灯掩盖断供）。

### 明确不做（本期边界）

- 不改模型路由配置（用户已手动配好 deepseek-v4-flash 挂 `data_enrichment_analysis`）。
- 不做 week 档首份全量补建（维持 M4.7 防首跑爆量护栏——month 回退后 week 缺失不再是断供点；week 覆盖率交还给 `lifeline_weekly` 定时任务）。
- 不动三角色编排结构、参考角色库、evidence schema（board-level-deep-analysis 主体行为保留）。
- 不做分析产出质量的 LLM 评测体系（素材修复后先观察实际产出再议）。

## Capabilities

### Modified Capabilities

- `board-level-analysis`：态势卡素材取材链扩 month 档回退（含卡片详略/质量信号对 month 源的适配）、`get_lane_detail` 附带背景记忆、section 指纹降级内容提质——素材装配相关 requirement 变更。注：该 capability 主 spec 随 board-level-deep-analysis（未归档）首次落库，本 change delta 在其之上修订。
- `data-enrichment`：单泳道下钻工具（`get_lane_detail`）的素材可见范围变化 + lifeline 读取边界声明。

## Impact

- **后端**：`internal/dataenrichment/service/situation_cards.go`（取材链）、`tool_registry.go` + `production_wiring.go`（get_lane_detail 渲染源）、可能的 `lifeline_context.go` 读取扩展；对应单测 + DB 集成测试以生产形态 fixture 重构。
- **前端**：`front/app/features/tags/components/BoardEnrichmentPanel.vue` 信息架构收口（+ 组件测试适配）。
- **数据**：无 schema 变更、无迁移——只改读取路径。
- **LLM 成本**：态势卡注入内容从零信息指纹变为 month 摘要截断（单卡 ~120 rune 预算不变），interpret/analyze prompt 体积可控。
- **归档顺序**：依赖 board-level-deep-analysis 先归档（`board-level-analysis` 主 spec 落库），本 change 随后归档。
