# UI 导航地图（给 opencli 端到端验证用）

> 本文档**增量维护**：每验证过一个需要多步导航的界面链路，就把路径 + 选择器 + 断言锚点补进来。供 **opencli 按需交互验证**（见 `.agents/skills/ui-verify/`）复用，避免每次重新踩点。不沉淀固定回归脚本（脆弱）。
>
> 约定：URL 默认 `http://localhost:3000`；后端 `http://localhost:5100/api`（前端直连）。访问性以 opencli `open` 实测为准（见 skill references/network-and-navigation.md）。

## 通用前置

| 步骤 | 操作 | 说明 |
| ------ | ------ | ------ |
| 0 | `navigate http://localhost:3000/<page>` | Nuxt SSR 只给壳，内容异步 hydrate |
| 0.5 | 关闭「叙事工坊」引导弹窗 | 首屏大概率弹出 dialog 挡住业务区，找 `aria-label="Close"` 或 × 按钮点掉，否则后续 `evaluate` 查不到业务 DOM |
| 0.6 | 每步导航后 `wait_for` 1~3s 或 `await setTimeout` | hydration / 数据加载是异步的 |

---

## 链路 1：板块日报 → 话题 → section → thread（thread-fit 等验证走这条）

**入口页**：`/tags`

### 1.1 选板块（侧栏）

- 侧栏容器：`aside.tags-sidebar` → `.sb-list`
- 板块项：`.sb-item`，选中态加 `.sb-item--active`
- 点击切板块：`document.querySelectorAll('.sb-item').find(i => /<板块名>/.test(i.textContent)).click()`
- 标签文本在 `.sb-item-label`

### 1.2 切到「日报」tab

- 主区 tab 按钮组（`main button`）："板块内容" / **"日报"** / "文章"
- 点日报：`Array.from(document.querySelectorAll('main button')).find(b => b.textContent.trim() === '日报').click()`

### 1.3 选日期打开日报详情

- 日报列表项是日期按钮，文本形如 `6 月 26 日 · 周五completed<当天摘要…>`
- 多个日期时按摘要匹配避免误点：`find(b => /6 月 26 日/.test(b.textContent) && /<摘要关键词>/.test(b.textContent))`
- 点击后进入日报详情视图（按"质量分区"组织）

### 1.4 日报详情视图结构（断言锚点）

```
.drm-zone[data-zone=...]            质量分区（active/briefs）
  header.drm-zone__header           "N 个话题"
  .drm-zone__topics
    article.drm-topic               话题分组
      button.drm-topic__header      可点击 toggle（aria-expanded）
        .drm-topic__heading strong  ← 话题标题（断言用它定位话题）
      .drm-topic__body              仅展开时存在
        .drm-topic__sections
          article.drm-section-card  一个 section（一个叙事簇）
            .drm-section-card__head （tier/anchor 徽章 + 可选标题）
            .drm-section-card__threads
              article.drm-thread    一条 thread（事件）
                button.drm-thread__header   toggle 关联文章（aria-expanded）
                  strong            ← thread 标题
                  .drm-thread__meta  右侧：离群标记 + 文章数 + 折叠箭头
                .drm-articles       仅展开时存在：关联文章 + .drm-thread__fit-probe
              button.drm-thread__hint  仅有离群 thread 时出现："另有 N 条可能跑题的线索"
```

**注意**：

- **默认只展开第一个话题**（`expandedTopics` watch 自动展开首个 active 话题）。含目标数据的话题多半折叠，要主动点 `.drm-topic__header` 展开。判断某话题是否展开：`!!topic.querySelector('.drm-topic__body')` 或 header 的 `aria-expanded`。
- **定位含特定数据的话题**：先查 API（见 skill references/assertion-recipe.md §2）拿到 `{board, date, section}`，再用话题标题文本 `.drm-topic__heading strong` 匹配展开。

### 1.5 关键状态标记（断言用）

| 标记 | 选择器 | 含义 |
| ------ | -------- | ------ |
| thread 离群（软降级） | `.drm-thread--demoted` | fit_distance > 阈值（当前 0.28） |
| thread 内容已展开 | `.drm-thread` 内存在 `.drm-articles` | toggleThread 后出现 |
| 离群提示行 | `.drm-thread__hint` | section 有 ≥1 离群 thread 时渲染 |
| 贴合度探针 | `.drm-thread__fit-probe` | 展开后可见，含 fit_distance 数值 |

### 1.6 thread-fit 软降级验证要点

- **测试夹具**：当前全库离群 thread 集中在 2026-06-26，分布于「生成式 AI 与大模型厂商」「中国国内新闻」「虚拟现实相关新闻」「美国新闻」四个板块（全球科技巨头板块**无**离群，别拿它验证）。
- **断言**：点 `.drm-thread__hint` 前后对比，翻转的 thread 必须全是 `.drm-thread--demoted`、正常 thread 的展开态不变。脚本见 skill references/assertion-recipe.md §1。（注：设计 A 后 `.drm-thread__hint` 已改为不可点 `<p>` 状态说明，此断言仅供历史参考；当前验证重点是「图标左对齐」「状态说明不可点」。）
- **交互语义 caveat**（2026-06-27 评审 + 设计 A 修订）：初版 hint 是可点 button 批量展开关联文章，但「展开文章对提示跑题无意义」。设计 A 改为：跑题 thread 行保持可见（灰显），alert 图标移至标题左侧，hint 行降级为 `<p>` 纯状态说明。见 `standard/frontend/interaction-conventions.md` §1§2。

---

## 链路 2：设置中心 /settings（订阅源 / AI 路由 / 队列 / 定时任务等）

**入口页**：`/settings`（工作台式 7-section 设置页，组件在 `features/settings/components/`）

### 2.1 通用前置

- 同 §通用前置（关闭引导弹窗、导航后 `wait_for` 1~3s）。
- `/settings` 首次访问会触发设置页 tour（`useOnboarding` 的 `startSettingsTour`，driver.js），可能弹引导，需先关闭。

### 2.2 section 切换（URL query 驱动）

- 当前 section 由 query `?section=<key>` 决定，key ∈ `feeds | ai-providers | capability-routes | queues | preferences | firecrawl | schedulers`，默认 `feeds`。
- 直接定位某 section：`navigate http://localhost:3000/settings?section=schedulers`。
- 侧栏切换：`SettingsSidebar` 内点击对应项 → `router.replace({ query: { section: key } })`，URL query 同步变化，可据此断言。

### 2.3 常用断言锚点

| section | 关键交互 | 提示 |
| ------ | ------ | ------ |
| `schedulers` | 定时任务状态卡片 + 手动触发按钮 | 复用 `SchedulerStatusPanel`；trigger 后状态异步刷新，`wait_for` 后再断言 |
| `queues` | tag-queue / embedding-queue / merge-reembedding-queue 的 retry | 复用 `EmbeddingQueuePanel` + `TagQueuePanel`；retry 后任务状态异步变化 |
| `ai-providers` / `capability-routes` | provider 凭据、能力路由绑定 | 复用 `features/ai/` 面板；表单提交走 `api/aiAdmin` |
| `feeds` | 订阅源主列表 + 详情编辑 | `FeedMasterList` + `FeedDetailEditor` |
| `feeds` | 管理工具条（增/导入/导出） | 顶部 `.feeds-toolbar__btn` × 4，文本依次为「添加订阅源 / 添加分类 / 导入 / 导出」（slim-header-feed-actions 起，首页顶栏不再有此入口）；前 3 个分别打开 `AddFeedDialog` / `AddCategoryDialog` / `ImportOpmlDialog`（`.app-dialog`），「导出」直接下载 `feeds-export-<date>.opml` 无对话框 |

---

## 链路 3：3D 侦探墙（detective-wall，Three.js）

**入口页**：`/tags`（需 WebGL 可用且屏幕宽 ≥768px，否则入口按钮不渲染，属预期降级）

### 3.1 进入路径

1. `/tags` → 侧栏选板块（同链路 1.1）。
2. 进入该板块日报区，打开 `BoardDailyReportTimeline`。
3. 切到「话题总览」视图（`BoardThreadBrowser`，头部按钮文本在「日报列表 / 话题总览」间切换）。
4. 点 `BoardThreadBrowser` 内 **「侦探墙」** 按钮（`title="进入 3D 侦探墙"`）→ 触发 `open-detective-wall` 事件 → `showDetectiveWall=true` → 全屏渲染 `TopicDetectiveWall.client.vue`。
   - 另一入口：日报全屏阅读层内 `DailyReportMiniLifeline` 的 `@open-detective`（按话题进入）。

### 3.2 墙内分段（顶部切换）

- `主墙`：回到当前 board timeline。
- `生命线`：保留当前 BFS 结果。
- `生命周期`：单话题完整演化视图（`getSectionLifecycle(sectionId)`）。

### 3.3 案件抽屉

- 案件详情为右侧抽屉（普通 Vue，非 CSS2D），含上一个/下一个案件导航；「下一个」存在分化时显示多个候选案件编号。

### 3.4 验证 caveat

- `.client.vue` 仅客户端渲染；SSR 阶段无 DOM，必须 `wait_for` 到 hydrate 后再断言。
- 无 WebGL / 窄屏时按钮缺失，属预期降级，不是 bug。
- 视觉对象 / 动画库分工等实现细节见 `frontend.md` §3D 侦探墙。

---

<!-- 新链路按上面格式增量追加 -->
