## 1. 文案表与泳道渲染

- [x] 1.1 `front/app/features/ai/composables/useAIRoutes.ts` 新增 `routeDescriptions: Record<string, string>`（紧邻 `routeLabels`），8 条能力文案按 specs delta 口径填写；`digest_polish` 涵盖日报+泳道趋势近14天档，`data_enrichment_news` 指向话题生命线月/年归档（泳道趋势月/年档）且不暗示供血日报
- [x] 1.2 `CapabilitySwimlane.vue` 行标题区（`swimlane__title` 下方）新增可选 `description` prop，渲染为单行 sm 次级文本；description 为空/undefined 时不输出该小字行（复用主题既有 token，不新写自由宽度）
- [x] 1.3 `AIRouterCapabilityRoutes.vue` 泳道循环从 `routeDescriptions[capability]` 取值传入 `CapabilitySwimlane`，缺省不传

## 2. 测试

- [x] 2.1 `useAIRoutes.test.ts` 补断言：`routeDescriptions` 覆盖 `capabilityOrder` 全部 8 条能力且每条非空；`routeDescriptions.data_enrichment_news` 含「月」「年」归档锚词且不含「日报」，`routeDescriptions.digest_polish` 含「日报」与「14」锚词
- [x] 2.2 `AIRouterCapabilityRoutes.test.ts` 补渲染断言：8 条泳道行均渲染出对应说明文案；`routeDescriptions` 缺某能力时该行不渲染空说明占位（可临时用未收录 capability 的组件级直渲验证或 mock 移除条目）
- [x] 2.3 运行受影响测试：`cd front && pnpm test:unit useAIRoutes.test.ts AIRouterCapabilityRoutes.test.ts --maxWorkers=2`（注意：参数不带 `--`）→ 期望全部通过

## 3. 文档

<!-- doc-impact: flow -->
- [x] 3.1 `docs/reference/flow/ai-summary.md`「配置可视」或「能力路由」条目补一句：设置页泳道行展示供血对象说明（含 digest_polish / data_enrichment_news 与泳道趋势三档的分工口径）
- [x] 3.2 归档前跑 `bash scripts/harness/doc-impact.sh verify` → 期望无未对账文档缺口；`bash scripts/harness/check-standards.sh` → 期望通过

## 4. 验证

- [x] 4.1 `cd front && pnpm lint` → 期望 0 error
- [x] 4.2 `cd front && pnpm exec nuxi typecheck` → 期望 0 error
- [x] 4.3 `cd front && pnpm test:unit useAIRoutes.test.ts AIRouterCapabilityRoutes.test.ts --maxWorkers=2` → 期望全部通过
- [x] 4.4 人工：构建并静态托管后打开设置页 AI → 能力路由（`bash scripts/dev/deploy-frontend.sh`），逐行确认 8 条泳道行均有供血小字；重点核对「新闻总结」行显示泳道趋势月/年归档、「日报润色」行显示日报+近14天档
- [x] 4.5 `openspec validate --change "add-capability-route-descriptions" --strict` → 期望通过

### Scenario → 测试落点映射

| Scenario | 测试文件 |
| --- | --- |
| 八条能力均有供血说明 | front/app/features/ai/composables/useAIRoutes.test.ts, front/app/features/ai/components/AIRouterCapabilityRoutes.test.ts |
| 新闻总结说明与真实供血一致 | front/app/features/ai/composables/useAIRoutes.test.ts |
| 日报润色说明覆盖日报与近14天档分工 | front/app/features/ai/composables/useAIRoutes.test.ts, front/app/features/ai/components/AIRouterCapabilityRoutes.test.ts |
| 未收录能力不渲染空说明行 | front/app/features/ai/components/AIRouterCapabilityRoutes.test.ts |
