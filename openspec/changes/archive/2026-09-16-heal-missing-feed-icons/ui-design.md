<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## 入口与入口变更

无新入口。受影响的是既有 `FeedIcon.vue` 组件（feed 列表、文章卡片、侧边栏、设置页等所有用到它的位置）在**图片加载失败**这一分支上的渲染结果：从「渲染成空白」变成「渲染 `mdi:rss` 占位图标」；另外阅读页头部 feed 徽标（`ArticleContentToolbar.vue`）从直接用 `<Icon :icon="feed.icon">` 改为复用 `FeedIcon`（同一降级行为）。无新页面、无新导航、无布局变化。

## 受影响状态

| 状态 | 变更前 | 变更后 |
| --- | --- | --- |
| success（`/icons/feeds/N.png` 文件存在） | 正常显示 `<img>` 图片 | 不变（像素级一致） |
| error（图片 404 / 加载失败） | `<Icon icon="/icons/feeds/2.ico">` → **空白**（非法 iconify 名） | `<Icon icon="mdi:rss">` → 灰色 RSS 占位图标 |
| error 且 icon 本身是 `mdi:*`（fallback 态） | `<Icon icon="mdi:rss">` 正常 | 不变 |
| error 且 icon 是远程 http(s) URL（存量数据） | 空白（同上，路径当图标名） | `mdi:rss` 占位 |
| error 且 icon 非以上形态（`rss` 历史值、`data:` URL、无前导斜杠的 `icons/feeds/x.ico`） | 空白（被当图标名传给 Iconify） | `mdi:rss` 占位 |
| 阅读页头部 feed 徽标（`ArticleContentToolbar`）icon = `/icons/feeds/N.ext` 且文件缺失 | 空 `<svg>` → 空白 | `FeedIcon` 渲染 `mdi:rss` 占位（尺寸 16 不变） |
| loading | 无独立 loading 态（`<img>` 加载中即浏览器默认行为） | 不变 |
| empty（`icon` 为空） | `<Icon icon="mdi:rss">` | 不变 |

受影响状态仅 **error** 一条；其余为回归对照项。`color` / `size` prop 语义不变（仍透传给占位图标）。

## 复用组件与布局模式

- 复用 `@iconify/vue` 的 `<Icon>` 组件渲染占位，**不新增组件**、不新增资源（`mdi:rss` 已在本地子集 `app/assets/iconify-subset.json` 中，运行时零联网）。
- 布局模式不变：`FeedIcon` 是行内固定尺寸（默认 20px）原子组件，不涉及 page shell 四模式 / dialog 档位 / 双视口契约；无宽度、无换行、无溢出策略变更。阅读页徽标换成 `FeedIcon` 后尺寸仍为 16（显式传 `:size="16"`），颜色仍透传 `feed.color`。
- 文案零变更（图标无文案）。

## 验收映射

- 组件测试（Vitest + happy-dom）：`front/app/components/feed/FeedIcon.test.ts` 新增/修订用例——① 合法 iconify 名（`mdi:rss`、`simple-icons:nuxtdotjs`）渲染同名图标；② `/icons/feeds/2.ico` 触发 error 时占位图标名为 `mdi:rss`（**回归本 bug**，断言占位符拿到的不是路径）；③ 远程 URL 触发 error 时同样为 `mdi:rss`；④ `rss` / `icons/feeds/x.ico` / `data:` URL 等非图标名形态一律 `mdi:rss`（不留白）；⑤ 正常图片地址仍渲染 `<img>`；⑥ icon prop 变化时失败标记重置。
- opencli / 人工：真实部署上 feed 列表在图标文件缺失时显示统一 `mdi:rss` 占位（不再整片空白）；阅读页头部徽标同样不空白；数据修复后重抓成功的 feed 显示真实 favicon。两档视口（1440×900 / 1920×1080）无布局位移（行内固定尺寸，不触发布局契约）。
