# test-cases：fix-spa-nav-loading-ux

> 用户故事（单元）：**切到没打开过的页面（如叙事工坊）时，用户全程有可见加载反馈而不是空白卡住；深色主题刷新首屏全程不闪白。**
> complexity: simple（无白盒附加节；状态机仅 2 态 + 延迟计时，不命中复杂档判据）。

## 主链路节拍表

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 深色主题硬刷新任意路由 | 首屏 pre-FCP 背景色：深色主题刷新无白闪 | FCP 前空窗 html 背景为 dark 主题色（#080c12），无白色闪现 | 人工（视觉时序）+ 文件断言 | `front/app/spa-loading-template.test.ts`；人工：agent-browser/浏览器复测留档 evidence/ |
| 2 | 浅色（或未设主题）打开首屏 | 首屏 pre-FCP 背景色：浅色主题背景一致 | pre-FCP 背景为 editorial 色（#faf7f2），与后续应用背景无缝衔接 | 文件断言 + 人工 | 同上 |
| 3 | 点击侧边栏「叙事工坊」（冷） | 路由切换加载反馈：慢 chunk 加载期间显示进度条 | 顶部进度条出现，路由完成后消失（既有行为保持） | 人工 | 人工：4.2 手测留档 |
| 4 | 同上导航超 250ms 未完成 | 路由切换加载反馈：慢路由切换出现加载态 | 内容区出现居中 spinner（accent 色、pointer-events:none、role=status） | composable 单测（fake timers）+ 组件测试 | `front/app/composables/useNavLoading.test.ts`、`front/app/components/common/NavLoadingOverlay.test.ts` |
| 5 | 导航在 250ms 内完成（暖切换） | 路由切换加载反馈：快路由切换不出现加载态 | 全程不出现加载态（计时器被清理，无闪烁） | composable 单测 | `front/app/composables/useNavLoading.test.ts` |
| 6 | 加载态显示中路由完成/失败/被新导航取代 | 路由切换加载反馈：导航完成或失败卸载反馈 | 反馈立即消失；连续导航重置计时不残留旧反馈 | composable 单测（afterEach/onError/二次 begin） | `front/app/composables/useNavLoading.test.ts` |
| 7 | 系统开启 prefers-reduced-motion 且触发加载态 | 路由切换加载反馈：减弱动效偏好 | 反馈仍显示但无旋转动画（motion-reduce 处理） | 组件测试 | `front/app/components/common/NavLoadingOverlay.test.ts` |
| 8 | tags 页点「话题总览」等四个非默认 tab（首次） | （实现层行为，无 spec Scenario；可用性变体-加载态落点） | 面板区域先显示居中 spinner 占位（<200ms 内就绪则不闪占位），随后渲染面板；默认「板块内容」tab 不受影响 | 组件测试 | `front/app/features/tags/components/TagsPage.test.ts`、`PanelAsyncPlaceholder.test.ts` |

## 变体走查（五组固定清单）

| 组 | 变体 | 答案 |
| --- | --- | --- |
| 输入 | 空串/空白/分隔符/大小写/特殊字符/超长 | 划除不适用：本 change 无文本输入面（导航触发与模板静态内容） |
| 前置 | 空集（无任何板块进 tags=pool 视图） | 导航反馈与页面内容无关（路由层）；懒 tab 面板在 selectedBoardId=null 分支不渲染——TagsPage 测试含无板块用例 |
| 前置 | 重复（同一路由重复点击） | 同路由同参数导航被 vue-router 忽略（beforeEach 不触发）→ 无反馈闪烁；不同路由连续快速切换 → 计时重置，见主链路步 6 |
| 前置 | 越界引用/部分满足 | 划除不适用：无外部引用解析面 |
| 时间窗口 | 250ms 边界两端 | 249ms 完成不显示 / 251ms 未完成显示——单测显式边界用例（fake timers） |
| 时间窗口 | 占位 delay=200 边界 | 懒面板 <200ms 就绪不出现占位；>200ms 出现——defineAsyncComponent delay 参数 + 组件测试 |
| 幂等 | 重复执行（end 后再 end / 已 visible 再 begin） | end 幂等无副作用；visible 态下 begin 视为新导航重置计时（先清旧计时器）——单测覆盖 |
| 幂等 | 并发 | 划除不适用：单浏览器单路由队列，router 钩子串行 |
| 可用性 | 加载态 | 本 change 主角（导航态 + 懒面板占位），见主链路步 4/8 |
| 可用性 | 错误态 | 导航失败（chunk error）→ onError 清理反馈 + 既有 error.vue 兜底链路不变——单测 onError 清理；兜底页行为有既有 spec 覆盖不动 |
| 可用性 | 空态/超长文本/误输入/重复提交 | 划除不适用（浮层无文本内容；重复提交=连续导航已在幂等组覆盖） |

## ⓪ 继承与调整（MODIFIED Requirement：路由切换加载反馈）

| 旧 Scenario | 处置 | 旧测试 | 动作 |
| --- | --- | --- | --- |
| 慢 chunk 加载期间显示进度条 | 保留（语义不变：顶条仍是第一层反馈；本 change 仅追加第二层 >250ms 加载态） | 无自动化映射（历史 change 人工验证；NuxtLoadingIndicator 为配置实现） | 本 change 验证节维持人工映射（4.2 手测），无旧资产需改写 |

## 效果核对

不适用：本 change 断言均为确定性 UI 行为/文件内容，不依赖数据覆盖率或 LLM 行为等外因。

## 验收命令

- `cd front && pnpm test:unit -- useNavLoading NavLoadingOverlay spa-loading-template TagsPage PanelAsyncPlaceholder`（vitest 按文件过滤跑新测试全绿）
- `cd front && pnpm lint && pnpm exec nuxi typecheck && pnpm build`（全量门禁）
- 人工：深色主题硬刷新无白窗；冷切叙事工坊 >250ms 出现反馈（agent-browser 采样或目视，留档 evidence/）
