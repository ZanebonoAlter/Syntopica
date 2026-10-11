## Context

设置页能力路由泳道行的标题数据来自 `front/app/features/ai/composables/useAIRoutes.ts` 的 `routeLabels`（渲染白名单 + 文案表），行渲染在 `CapabilitySwimlane.vue` 的 `swimlane__title` 区。说明文案所需的「capability → 真实调用方」映射已在探索阶段核实（见 proposal.md 与 specs delta），后端无需任何改动。

## Goals / Non-Goals

**Goals:**
- 每条泳道行标题下出现一行「供血对象」小字，文案与真实调用链一致。
- 文案表与 `routeLabels` 同源同文件维护，新增能力时两表同处一目了然。

**Non-Goals:**
- 不改「新闻总结」「日报润色」等能力名（改名牵连 health 展示/文档/记忆成本，用户已选 A 放弃 B）。
- 不改泳道交互、绑定保存、健康徽章等任何既有行为。
- 不补 flow 文档之外的新文档（doc-impact 收尾时按 suggest 对账即可）。

## Decisions

1. **文案表放 `useAIRoutes.ts`，紧邻 `routeLabels`**（`routeDescriptions: Record<string, string>`）。
   - 备选：塞进 `routeLabels` 值里用分隔符拼接 → 否决，污染渲染白名单语义、测试断言变脆。
   - 备选：后端下发描述 → 否决，纯前端文案层，加接口是过度设计。
2. **渲染在 `CapabilitySwimlane.vue` 行标题下方，可选 prop**（`description?: string`），空/undefined 时不输出该行。
   - 这同时实现 spec 的「未收录能力不渲染空说明行」scenario：宿主 `AIRouterCapabilityRoutes.vue` 从 `routeDescriptions[capability]` 取值传入，缺省即不渲染。
   - 备选：直接在泳道循环里内联 span → 否决，说明是行头固有信息，归泳道组件所有，宿主只传数据。
3. **文案口径**：一句话直供血对象，避免行话。重点两条按 spec 原文口径：`digest_polish` = 日报生成 + 泳道趋势近14天档；`data_enrichment_news` = 话题生命线月/年归档（泳道趋势月/年档），明示与日报无关。

## Risks / Trade-offs

- [窄视口文案截断] → 已决策维持现状（单行省略号 + hover title）：设置页整体移动端适配本就是既有欠账，desc 不单独适配，待将来设置页适配 change 一并处理；关键锚词（「14 天」vs「月」「年」）在窄屏可能被截入省略号属已知取舍，桌面端 hover title 兜底（2026-09-30 用户确认）。
- [文案漂移：后端调用方变了文案没跟上] → spec 已把两条关键分工写成 SHALL 断言场景；测试断言 8 条非空 + 关键文案含锚词（如「月」「年」归档）。
- [行头变高影响密集布局] → 说明为单行 sm 小字，泳道行高自适应，settings workspace 布局余量充足；组件测试覆盖渲染即可。
- [未来新增能力忘补文案] → 缺省不渲染（有优雅降级），且 `routeDescriptions` 与 `routeLabels` 并排可维护。

## Migration Plan

纯前端文案，随常规部署生效；无数据迁移、无配置要求、回滚即还原旧构建。

## Open Questions

（无）
