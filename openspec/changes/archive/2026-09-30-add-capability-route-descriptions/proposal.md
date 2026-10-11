<!-- complexity: simple -->
<!-- ui-impact: minor -->
<!-- constraint-domains: ai-summary, daily-report, data-enrichment -->

## Why

设置页「能力路由」八条泳道只有干巴巴的能力名（如「新闻总结」「日报润色」），没有任何一处告诉用户这条能力在给哪个功能供血——用户看到「新闻总结」和「日报润色」并排，完全猜不出前者其实是**话题生命线的月/年归档摘要**（泳道趋势「月」「年」档的数据源），与日报毫无关系；泳道趋势面板「近 14 天」档与「月/年」档实际由两条不同能力供血，这一事实在 UI 上不可见。准确的业务用途映射只活在代码注释（`dataenrichment/capability.go`、`topicgraph` 各 LLM 调用点）与 flow 文档里。

## What Changes

- 能力路由泳道行标题下新增一行「供血对象」说明文案（副标题小字），覆盖全部 8 条能力，数据挂在 `routeLabels` 旁的 `routeDescriptions` 表中（`front/app/features/ai/composables/useAIRoutes.ts`）。
- 说明文案与真实调用链对齐（探索阶段已核实）：
  - `summary` → 阅读流文章自动总结
  - `topic_tagging` → 标签提取与语义操作
  - `digest_polish` → 日报生成（聚类/要闻/叙事）+ 泳道动态与泳道趋势「近 14 天」档（`daily_report.lane_snapshot` 等）
  - `embedding` → 向量嵌入
  - `open_notebook` → 日报页边注问答
  - `feed_discovery` → 订阅源发现推荐精排
  - `data_enrichment_news` → 话题生命线周期归档摘要（泳道趋势「月」「年」档；月/年定时任务生成）
  - `data_enrichment_analysis` → 数据增强三角色编排（解读/分析/评审）
- 不改任何路由、后端接口、数据模型——纯前端文案层。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `openspec/specs/ai-capability-routing/spec.md`: 新增需求「能力路由 UI 展示业务用途说明」——每条能力泳道行 SHALL 展示其供血对象说明，文案 SHALL 与该 capability 的真实 LLM 调用方一致（覆盖 digest_polish 与 data_enrichment_news 的泳道趋势分工这一已验证事实）。

## Impact

- 前端：`front/app/features/ai/composables/useAIRoutes.ts`（新增 routeDescriptions 表）、`front/app/features/ai/components/CapabilitySwimlane.vue`（行标题渲染说明文案）、对应单测。
- 后端：无。
- 部署后用户可见变化：设置页能力路由每行多一行灰色小字说明；无需任何手动操作，无数据迁移，无降级影响。
