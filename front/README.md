# Frontend

前端基于 Nuxt 4、Vue 3、TypeScript 和 Pinia。

## 开发命令

```bash
pnpm install
pnpm dev
pnpm build
pnpm exec nuxi typecheck
```

## 当前入口

- 应用壳：`front/app/app.vue`
- 首页：`front/app/pages/index.vue`
- Digest 页面：`front/app/pages/digest/index.vue`
- 主布局实现：`front/app/features/shell/components/FeedLayoutShell.vue`

## 架构文档

- 前端架构：`docs/reference/architecture/frontend.md`
- 组件分工：`docs/reference/architecture/frontend-components.md`
- 功能说明：`docs/reference/frontend-features.md`
- 数据流：`docs/reference/architecture/data-flow.md`
- 开发流程：`docs/reference/development.md`

## 当前目录原则

前端现在以 feature 组织为主，核心规则是：

- `api/` 成为唯一接口边界
- `features/` 收拢业务代码
- `stores/api.ts` 作为后端数据源
- `stores/feeds.ts`、`stores/articles.ts` 只暴露衍生视图，不再手工同步副本
- `components/` 只放通用组件和弹窗
- 业务实现统一从 `features/` 进入，不再兼容旧入口
