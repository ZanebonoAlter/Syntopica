<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## 入口与入口变更

**无新增入口、无版式变更。** 受影响的是既有入口的数据刷新节奏与一张静态图标：

| 入口 | 现状 | 本 change 后 |
|---|---|---|
| 顶栏调度器状态 / 分析暂停指示（`AppHeaderView` → `useSchedulerStatus`） | 8s/15s/30s 自适应轮询 + 触发后即时刷新 | 合并进单一对账入口，节奏对用户不可见；热态（执行中）仍保持短间隔 |
| 标签队列进度芯片（`TagQueueProgressChip` → `useTagQueueProgress`） | 60s 独立轮询 + WS 事件驱动 | 合并轮询（计数来源不变），WS 驱动不变 |
| 通知未读角标（`useNotifications`） | 60s 独立轮询 + WS 事件 | 合并轮询，角标语义不变 |
| 标签页在后台（不可见） | 照常轮询 | 暂停轮询；恢复可见时立即刷新一次（角标/芯片可能由"滞后"变为"回来即准"） |
| 浏览器标签图标 + 顶栏 logo（`/favicon.png`） | 1254×1254 / 321.6 KB、`no-cache`、每次重下 | 64×64 / ≈4.5 KB、带缓存头；视觉形状按原图等比缩（不外框改版） |
| 阅读页空态插画（新 `/brand-mark.png`） | 复用同一张大图，随首屏一起下 322 KB | 720×720 / ≈29 KB，且只在“未选中文章”的空态渲染时才请求；插画视觉零变化（retina 反而更清） |

## 受影响状态

| 状态 | 变化 |
|---|---|
| loading | 不变（无新增加载态；合并请求失败时各消费点沿用既有兜底） |
| empty | demo 只读实例调度器状态由「500 错误」变为「200 + 空集合」→ 前端渲染既有空态而非错误态 |
| error | 合并端点失败时，三类数据 MUST 各自保有上次值（MUST NOT 因一次失败清空角标/芯片计数）；旧端点保留以免旧标签页 404 |
| success | 数值语义与刷新时效不变（热态间隔不变，空闲间隔不下调） |
| 后台标签页（hidden） | 轮询暂停（数据可能滞后）；恢复可见时立即对账一次 |

## 复用组件与布局模式

- 布局模式：**不变**（本 change 不触及页面壳与阅读页版式；顶栏/芯片/角标位置与尺寸零变化）。
- 复用组件：`TagQueueProgressChip`、通知角标、顶栏状态指示 —— 全部只换数据来源与节奏，props 与 DOM 结构不变。
- 静态资源：`favicon.png` 等比缩小（同图、同透明背景），不引入新图标体系；如需引入 apple-touch-icon 需在 tasks 中显式确认。
- 不新增组件、不新增 dialog。

## 验收映射

| 契约点 | 验收方式 |
|---|---|
| 文本响应压缩协商（静态 + API） | Go 中间件单测（`Accept-Encoding` 矩阵）+ `curl -H 'Accept-Encoding: gzip' -D -` 观测 `content-encoding: gzip` 与 `vary` |
| `/_nuxt/*` immutable / `index.html` no-cache | `curl -D -` 断言响应头；浏览器二次访问断言 200 from disk cache |
| favicon 体积与缓存（标签图标 + 空态插画） | `ls -l` 断言两个文件体积（≤ 64 KB / ≤ 64 KB）；`curl -D -` 断言带缓存语义；人工空态截图目视不模糊 |
| 轮询合并与预算 | unit test（fake timers）断言「同分钟至多 N 次请求」「hidden 时不请求」「可见恢复立即请求一次」 |
| demo 只读状态端点 200 | handler 单测 + `curl -s -o /dev/null -w '%{http_code}'` 断言 200 |
| 界面零回归 | 人工：顶栏状态、芯片、角标在合并后仍正确显示（opencli 断言 + 截图存 `evidence/`） |
