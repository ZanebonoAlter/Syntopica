## Why

用户在阅读文章时看到感兴趣的标签，需要离开当前文章页面去侧边栏或话题页面才能关注该标签，操作路径过长。在文章详情页的标签上直接展示关注状态并支持一键关注/取消关注，能显著提升标签关注的使用效率和体验。

## What Changes

- 文章详情页的标签展示增加关注状态图标（实心/空心爱心），与侧边栏"关注标签"使用相同视觉语言
- 点击标签上的爱心图标可直接关注/取消关注该标签，无需离开文章页
- 后端文章详情接口返回的标签数据增加 `is_watched` 和 `tag_id` 字段，使前端能判断当前关注状态
- `ArticleTagList` 组件增加可选的关注操作能力（通过 prop 控制是否展示）

## Capabilities

### New Capabilities
- `article-tag-watch-toggle`: 在文章详情页的标签列表上展示关注状态并支持点击切换关注/取消关注

### Modified Capabilities
<!-- None - no existing specs have requirement-level changes -->

## Impact

- **前端**: `ArticleTagList` 组件新增关注状态展示和点击交互；`ArticleContentView` 传入关注相关 props 和回调
- **后端**: `GetArticle` handler 在返回文章标签时附带 `is_watched` 和 `id`（tag ID）字段
- **API**: `GET /api/articles/:article_id` 返回的 `tags` 数组中每个 tag 对象新增 `id` 和 `is_watched` 字段（非 breaking，纯新增字段）
- **依赖**: 复用已有的 `POST /api/topic-tags/:tag_id/watch` 和 `/unwatch` API，无需新增后端接口
