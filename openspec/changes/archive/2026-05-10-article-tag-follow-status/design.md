## Context

系统的"关注标签"（watched tags）功能已存在完整的后端 API 和侧边栏 UI：
- `POST /api/topic-tags/:tag_id/watch` / `unwatch` — 关注/取消关注标签
- `GET /api/topic-tags/watched` — 列出所有已关注标签
- 侧边栏和话题页面已有爱心图标切换关注状态

但文章详情页的标签列表（`ArticleTagList`）仅展示标签信息，未显示关注状态，用户必须离开文章页才能操作关注。

当前 `GetArticleTags` 返回 `topictypes.TopicTag`，缺少 `IsWatched` 字段，也未返回 tag 的数据库 ID，前端无法判断某个标签是否已被关注，也无法调用 watch/unwatch API。

## Goals / Non-Goals

**Goals:**
- 文章详情页每个标签旁展示关注状态图标（实心/空心爱心）
- 点击图标可即时关注/取消关注，UI 即时反馈（乐观更新）
- 后端文章详情 API 返回每个标签的 `id` 和 `is_watched` 字段

**Non-Goals:**
- 不改变侧边栏或话题页面的关注逻辑
- 不改变 watch/unwatch API 的行为
- 不在文章列表卡片上添加关注操作（仅文章详情页）

## Decisions

### 1. 后端在 `topictypes.TopicTag` 中新增字段，而非新建类型

`topictypes.TopicTag` 已有 `ID uint` 字段（`json:"id,omitempty"`），只需在 `GetArticleTags` 中填充它，并新增 `IsWatched` 和 `ArticleCount` 字段。

**Alternatives considered:**
- 新建 `ArticleTagResponse` 类型：增加维护成本，且与现有 `TopicTag` 高度重叠
- 在 `Article` 模型的 `ToDict()` 中内联构建 tag 对象：破坏关注点分离

### 2. 前端在 `ArticleTagList` 通过 prop 控制是否展示关注按钮

新增 `showWatch?: boolean` prop，默认 `false`。仅在文章详情页传入 `true`。`ArticleTagList` 不直接调用 API，而是通过 emit 事件将 tag 信息传递给父组件处理。

**Alternatives considered:**
- 在 `ArticleTagList` 内部直接调用 `useWatchedTagsApi()`：组件变得有副作用，不利于复用
- 创建新的 `ArticleTagWithWatch` 组件：功能相近，增加碎片化

### 3. `ArticleContentView` 负责 watch/unwatch 的 API 调用

文章详情页组件已有类似的手动操作逻辑（手动抓取、手动总结、手动打标签），关注操作遵循相同模式：乐观更新本地 article 数据 + API 调用 + 失败回滚。

### 4. 前端乐观更新 + API 确认

点击爱心图标时立即翻转本地状态更新 UI，同时发出 API 请求。API 失败时回滚本地状态。

## Risks / Trade-offs

- **标签数据不一致**: 如果用户同时在侧边栏和文章页操作同一个标签的关注状态，可能存在短暂不一致。由于 watch/unwatch API 是幂等的（关注已关注的标签不会出错），后续刷新会自然同步。
- **`ArticleTag.id` 可选**: 列表接口不返回 tags 数组，因此只有文章详情页的 tags 有 `id`。`ArticleTag.id` 标记为 optional，组件需判空。
