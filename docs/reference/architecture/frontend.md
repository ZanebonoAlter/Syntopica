# 前端架构

## 技术栈

- Nuxt 4.2.2
- Vue 3.5
- TypeScript
- Pinia
- Tailwind CSS v4
- Iconify
- Day.js
- marked
- motion-v
- Vitest

## 入口与路由

- 应用壳入口：`front/app/app.vue`
- 主阅读页：`front/app/pages/index.vue`
- Digest 总览：`front/app/pages/digest/index.vue`
- Digest 单视图：`front/app/pages/digest/[id].vue`
- Topic Graph：`front/app/pages/topics.vue`

`app.vue` 只做一件事：启动时调用 `apiStore.initialize()`，先拉分类、订阅源和文章，再渲染页面。

## 当前目录分层

```text
front/
├─ app/
│  ├─ api/                 # 唯一 HTTP 边界
│  ├─ assets/css/          # 全局主题与基础样式
│  ├─ components/          # 通用组件与对话框
│  ├─ composables/         # 少量跨 feature 的通用能力
│  ├─ features/            # 业务实现主体
│  ├─ pages/               # Nuxt 路由入口
│  ├─ plugins/             # Nuxt 插件
│  ├─ stores/              # Pinia store
│  ├─ types/               # 领域类型
│  ├─ utils/               # 常量和纯工具函数
│  └─ app.vue
├─ nuxt.config.ts
└─ package.json
```

## feature 划分

- `features/shell`：主壳、顶部栏、侧栏、文章列表栏
- `features/articles`：正文阅读、内容补全状态、Firecrawl 全文、AI 整理稿
- `features/summaries`：AI 总结列表、队列进度、WebSocket 实时更新
- `features/digest`：日报、周报、详情页、配置抽屉
- `features/feeds`：自动刷新和刷新轮询
- `features/preferences`：阅读行为埋点与偏好相关逻辑
- `features/topic-graph`：主题图谱、热点标签、话题详情、analysis、timeline、标签层级（TagHierarchy）、标签合并预览、叙事面板

这套结构已经替代旧的“业务都堆在 `components/`”的方式。新功能优先进入 `features/*`。

## 数据层约定

### API 层

`front/app/api/*` 是唯一 HTTP 边界。

- `client.ts` 统一封装 `fetch`
- 各领域模块只关心自己的接口
- query 参数统一走 `buildQueryParams()`
- 文件上传和下载也在这里收口

当前已落地的模块包括：

- `categories.ts`
- `feeds.ts`
- `articles.ts`
- `summaries.ts`
- `digest.ts`
- `opml.ts`
- `reading_behavior.ts`
- `firecrawl.ts`
- `scheduler.ts`
- `aiAdmin.ts`
- `topicGraph.ts`
- `abstractTags.ts`
- `embeddingConfig.ts`
- `embeddingQueue.ts`
- `mergeReembeddingQueue.ts`
- `tagMergePreview.ts`
- `watchedTags.ts`

### Store 层

`useApiStore()` 是前端数据主源。

- 持有 `categories`、`feeds`、`allFeeds`、`articles`
- 负责后端返回值到前端字段的映射，例如 `article_summary_enabled -> articleSummaryEnabled`、`summary_status -> summaryStatus`
- 负责增删改后重新拉取必要数据
- 负责文章已读、收藏、批量已读等更新

`useFeedsStore()` 和 `useArticlesStore()` 现在是派生视图层。

- `feedsStore` 基于 `apiStore` 暴露分类分组、未读数等计算结果
- `articlesStore` 基于 `apiStore` 暴露筛选、排序、当前文章等视图状态
- 不再维护本地副本
- 不再依赖手动 `syncToLocalStores()`

`usePreferencesStore()` 独立负责阅读偏好和统计。

## 数据映射规则

- 后端字段以 `snake_case` 为主
- 前端 store 和组件内部统一用 `camelCase`
- ID 在前端统一存成 `string`
- 数字 ID 与字符串 ID 的转换只应发生在 API 边界或 store 映射层
- 文章处理相关字段统一使用 `articleSummaryEnabled`、`summaryStatus`、`summaryGeneratedAt`

## 页面骨架

主阅读页由 `FeedLayoutShell.vue` 组织为三栏：

- 左栏：分类、订阅源、快捷入口
- 中栏：文章列表或 AI 总结列表
- 右栏：文章正文或 AI 总结详情

Digest 页面走独立路由和独立视觉壳，不复用主阅读页的三栏壳。

### Topics页面架构

#### 组件结构

- `TopicGraphPage`: 主页面容器
  - `TopicGraphHeader`: 头部控制区（返回首页、刷新图谱）
  - `TopicGraphCanvas`: 3D 拓扑图
  - `TopicAnalysisTabs`: 分析分类 Tabs
  - `TopicAnalysisPanel`: 分析内容展示
  - `TopicGraphSidebar`: 右侧详情栏
  - `ArticleTagList`: digest/article 标签的通用可视化组件

#### 状态管理

- 选中状态统一在 `TopicGraphPage` 管理
- `selectedCategory`: 当前选中的分类（event/person/keyword）
- `selectedTagInCategory`: 当前选中的标签 slug
- `highlightedNodeIds`: 需要高亮的节点 ID 列表

#### 数据流

1. 用户点击分类入口（热点分类按钮）后更新 `selectedCategory`
2. 页面基于 `selectedCategory` 计算 `highlightedNodeIds`
3. `highlightedNodeIds` 传给 `TopicGraphCanvas` 并驱动图谱高亮
4. 用户切换分析 Tabs 或选中标签后，底部分析面板按类型加载对应分析数据

#### 标签展示约定

- `ArticleContentView` 现在会直接展示标准 article tags
- digest 列表、digest 详情、topic graph timeline 使用 digest 的 `aggregated_tags`
- `/topics` 中当前选中的 topic slug 会在 digest/article 标签上做高亮

## 设计系统

当前前端已经切到 editorial / magazine 风格，而不是常见 SaaS 蓝紫模板。

- 主色：Ink Blue
- 强调色：Print Red
- 背景：Paper Warmth
- 阴影：偏纸张和印刷感
- 全局主题变量定义在 `front/app/assets/css/main.css`
- 分类和订阅源可选色定义在 `front/app/utils/constants.ts`

明确约束：

- 不用紫色 / 靛蓝色方案
- 不用纯平背景
- 不用默认 shadcn / Material 风格
- 不做对称、平均分栏的模板布局

## 运行与环境

- 前端开发端口：`http://localhost:3000`
- 后端 API：`http://localhost:5000/api`
- AI 总结 WebSocket：基于运行时 `apiBase` 推导，默认连 `ws://localhost:5000/ws`

## 相关文档

- `docs/architecture/frontend-components.md`
- `docs/architecture/data-flow.md`
- `docs/guides/frontend-features.md`
- `docs/guides/topic-graph.md`
- `docs/operations/development.md`
