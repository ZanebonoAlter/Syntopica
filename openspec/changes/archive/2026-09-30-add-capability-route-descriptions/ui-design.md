<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## 入口与入口变更

- 入口：设置页 →「AI」→「能力路由」section（`front/app/features/ai/components/AIRouterCapabilityRoutes.vue`，泳道行 `CapabilitySwimlane.vue`）。
- 入口不变：无新页面、无新导航、无交互流程变化。唯一变化是每条泳道行标题（能力名）下方新增一行「供血对象」说明小字。

## 受影响状态

- **success**：泳道行标题区渲染说明文案（`routeDescriptions[capability]`，缺省回落不渲染该行——8 条能力全覆盖，理论不缺省）。
- **loading**：initialLoading 分支走既有骨架/占位，说明文案随行一起出现，无新增状态。
- **error**：加载失败分支不变（`loadError` 已有内联错误条），说明文案不存在于错误态，无影响。
- **empty**：模型池空/泳道空不改变行标题区结构，说明文案照常渲染（它挂在行头，不依赖绑定卡）。

## 复用组件与布局模式

- 复用 `CapabilitySwimlane.vue` 行标题区（`swimlane__title`）既有结构，说明文案作为标题下的一行小字（次级文本色 / sm 尺寸，沿用现有主题 token，不新写自由宽度）。
- 不新组件、不改布局模式：仍为设置页 workspace 布局内的既有 section，泳道行高轻微自适应增加。
- 文案表挂在 `useAIRoutes.ts` 的 `routeLabels` 旁（`routeDescriptions: Record<string, string>`），与渲染白名单同源维护。

## 验收映射

- 组件测试（Vitest）：`AIRouterCapabilityRoutes` / `useAIRoutes` 相关测试断言 8 条能力均输出非空说明文案，且泳道行渲染出对应文案。
- 人工验证：打开设置页 AI → 能力路由，逐行可见供血对象小字；重点核对「新闻总结」行显示泳道趋势月/年归档、「日报润色」行显示日报+近14天档分工。
