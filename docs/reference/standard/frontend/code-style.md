# 前端代码规范（Code Style）

> **权威源**：本文件是前端代码规范的唯一权威。`front/AGENTS.md` 的 Anti-Patterns 与架构约定的红线要点深链指向本文件。
> 互补：`architecture/frontend.md` 描述 Feature 划分与骨架定位。

## 1. 通用约定

- 组件使用 `<script setup lang="ts">`（Composition API）
- 组件名 PascalCase；composable 名 camelCase + `use` 前缀
- 导入顺序：Vue/Nuxt → 第三方 → 内部 → type-only；使用 `import type` 导入纯类型
- 使用 `~` alias 引用 app 根路径
- 文件 UTF-8 编码；保持与周围代码一致的分号风格（大部分前端文件不使用分号）
- UI 维持 editorial/magazine 风格（详见 [theming.md](./theming.md)）

## 2. 目录归属规则

| 代码类型 | 归属位置 | 说明 |
|---------|---------|------|
| 业务组件 | `features/*/components/` | Feature 私有，其他 feature 不得直接 import |
| 业务 composable | `features/*/composables/` | Feature 私有 |
| 跨 feature 共享组件 | `features/*/public.ts` facade | 或上移到 `components/` |
| 全局 composable | `composables/` | 如 `useEventStream`、`useNotify` |
| HTTP 调用 | `api/` | **唯一 HTTP 边界**，通过 `useXxxApi()` 使用 |
| 数据 normalizer | `api/normalizers/` | 共享 DTO 转换；feature 内的仅限内部使用 |
| 状态管理 | `stores/` | 仅放需跨组件共享的状态 |
| 领域类型 | `types/` | API 响应类型、Store 状态类型 |
| 纯工具函数 | `utils/` | 常量、格式化、通用逻辑 |
| 页面 | `pages/` | Nuxt 路由入口，只做挂载，不放业务逻辑 |

## 3. API 层归一化

查询构建、响应解包、camelCase 转换**各只有一个权威 Module**：

```typescript
// ✅ 正确：使用统一工具
import { buildQueryString, camelizeKeys, mapApiResponse, unwrapResponse } from '~/utils/api-helpers'

// ❌ 错误：手写 URLSearchParams 或重复实现
const params = new URLSearchParams()
params.set('page', '1')
```

**字段映射（snake_case → camelCase 改名 + 枚举归一）只允许发生在 `api/normalizers/`**：实体正常化函数（如 `normalizeArticle` / `normalizeFeed`）负责把后端键改写成领域字段名，并对接口新增的枚举位做白名单归一（非法值返回 `undefined` 而不是原样透传）；组件、composable、store 的渲染层 MUST NOT 直接读 `payload.xxx_yyy` 或自建一份映射（组件写了映射，接口改名时就只会在 UI 静默空掉）。

```typescript
// ✅ 正确：normalizer 集中映射（后端 `ai_detection_status` → 前端 `aiDetectionStatus`）
// front/app/api/normalizers/article.ts
type AiDetectionStatus = 'unknown' | 'suspect' | 'not_ai'
const AI_DETECTION_STATUSES: AiDetectionStatus[] = ['unknown', 'suspect', 'not_ai']
const normalizeAiDetectionStatus = (v?: string): AiDetectionStatus | undefined =>
  AI_DETECTION_STATUSES.includes(v as AiDetectionStatus) ? (v as AiDetectionStatus) : undefined

aiDetectionStatus: normalizeAiDetectionStatus(payload?.ai_detection_status),

// ❌ 错误：组件里自建映射
const suspect = article.ai_detection_status === 'suspect'
```

API 模块中的 `as unknown as ApiResponse<T>` 应替换为 `mapApiResponse<T>()`。

## 4. Store 规则

**写操作必须持久化到后端（store-integrity）：**

```typescript
// ✅ 正确：调用 API + 乐观更新 + 回滚
async function toggleFavorite(articleId: string) {
  const prev = article.favorite
  article.favorite = !prev  // 乐观更新
  const { success } = await articlesApi.toggleFavorite(articleId)
  if (!success) {
    article.favorite = prev  // 回滚
    notify.error('收藏失败')
  }
}

// ❌ 错误：只改本地状态
article.favorite = !article.favorite
```

**Store 间数据同步通过小 Interface：**

```typescript
// ✅ 正确：通过方法同步
feedsStore.adjustUnreadCount(feedId, -1)

// ❌ 错误：直接遍历别的 store
apiStore.feeds.find(f => f.id === feedId).unread_count--
```

状态约定：
- `useApiStore` 是主数据源，其余 store 从中派生
- `useFeedsStore` / `useArticlesStore` 只做派生视图
- 禁止新增 `syncToLocalStores()` 一类的副本同步逻辑
- 禁止 Store 间循环依赖（articlesStore 不 import apiStore 的 mutation 方法）

## 5. 事件流

**所有全局实时事件通过 `useEventStream()` 订阅，禁止自建 WebSocket/EventSource：**

```typescript
import { useEventStream } from '~/composables/useEventStream'
import { EVENT_TYPES } from '~/utils/eventTypes'

const stream = useEventStream()
let off: (() => void) | undefined
onMounted(() => { off = stream.on(EVENT_TYPES.TAG_COMPLETED, (data) => { /* ... */ }) })
onUnmounted(() => { off?.() })  // 必须在 onUnmounted 中 unsubscribe
```

事件类型常量使用 `utils/eventTypes.ts` 的定义；专用 SSE 长任务（scan/evaluate）在对应 API module 中作为命名 Adapter 暴露。

## 6. 错误通知

**写操作失败时由执行方（Store/Composable）通知，底层 API 不弹 toast：**

```typescript
// ✅ 正确：Store 层通知
async function deleteBoard(id: string) {
  const { success } = await boardsApi.deleteBoard(id)
  if (!success) notify.error('删除失败')
}

// ❌ 错误：api/semanticBoards.ts 内调用 notify.error()
```

使用 `useNotify()` 的 `success/error/warn`；同一次失败只由一个责任层通知，避免重复 toast。

## 7. Feature 间共享

跨 feature 引用**必须**通过 facade 或共享层：

```typescript
// ✅ 正确：facade / 共享 normalizer
import { ArticleContentView } from '~/features/articles/public'
import { normalizeArticle } from '~/api/normalizers/article'

// ❌ 错误：深 import 另一 feature 内部
import ArticleContentView from '~/features/articles/components/ArticleContentView.vue'
```

新建 feature 对外暴露能力时创建 `features/<name>/public.ts`。

## 8. 大组件拆分

单文件超过 **500 行 / ~15KB** 时应拆分：

1. 按**内聚行为**拆 composable，不是按行数机械拆
2. 每个 composable 以小 Interface 暴露一组高内聚行为
3. 不能形成巨型 composable（>500 行需继续拆）
4. 子组件通过 props 接收数据，不直接依赖父级 composable 内部状态

## 9. 数据映射约定

- 后端数字 ID 在 API/store 边界转为 `string`
- `snake_case → camelCase` 集中在 API 或 store 层，**不在组件里做**
- 字段重命名直接在类型和 store 映射层切换，不在组件里做兼容
- API 返回值统一 `ApiResponse<T>` 包装

## 10. Anti-Patterns（硬禁）

- ❌ 组件内直接调 API / 调 WebSocket / 用 `any` / Options API / `@ts-ignore`
- ❌ 不调 API 直接改 Store 状态
- ❌ Store 间循环依赖
- ❌ 跨 feature 深 import
- ❌ 重复通知（同一失败多层 toast）
- ❌ 组件内手写 echarts / 手写 CSS、SVG 图表（见 §11 图表约定）

## 11. 图表约定（echarts）

**新增数据图表统一走 ECharts 封装，禁止绕开封装手搓：**

- ✅ 统一走 `useEcharts` composable（init/resize/dispose 生命周期）+ `chart-options.ts` 的 option **纯函数**（组件只负责挂载/传参/事件桥接，option 可单测）
- ❌ 禁止组件内直接手写 `echarts.init` / `setOption` / `dispose` 或直接操作 echarts 实例
- ❌ 禁止再手写 CSS/SVG 图表（div 小棍、手算坐标 polyline 等）
- 例外：特殊场景不受此限——3D（侦探墙 three.js）、关系 DAG（`BoardThreadBrowser`）等非数据图表
- 图表颜色从 CSS 变量读取（`readPalette`）+ `watch(useTheme().theme)` 重建 option，禁止硬编码色板

> 选型与封装细节见 [`architecture/frontend.md`](../../architecture/frontend.md) §图表库选型（首个落地场景 topic-landscape）。

## 资料来源

收敛自原 `front/AGENTS.md`（Frontend-Specific Conventions / Anti-Patterns / 架构约定 §1–§8）与 `development.md` §前端目录/状态/数据映射约定。
