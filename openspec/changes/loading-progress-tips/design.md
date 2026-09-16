## Context

两层加载屏已存在且契约稳定（`docs/reference/standard/frontend/loading-experience.md`）：首帧模板（纯静态自包含）+ app.vue 初始化屏（Vue 响应式）。初始化只有两个并行请求（`Promise.all([fetchCategories, fetchFeeds])`），无法按阶段做真实分步进度——进度只能拟真。首帧模板在 bundle 加载前运行，**不能 import 任何模块**，因此两层的短句清单/进度逻辑天然是复制而非共享。

## Goals / Non-Goals

**Goals:**
- 两层加载屏获得一致的游戏感：拟真进度条 + 百分比 + 随机短句（长加载轮换）。
- 进度逻辑可测试：app.vue 侧抽纯函数/组合式，vitest 覆盖拟真曲线、100% 收尾、计时器清理。
- 零新依赖、零 API 变更、初始化时序不变。

**Non-Goals:**
- 不做真实请求级进度（两个并行请求无阶段可分，WebSocket/SSE 汇报超出简单档）。
- 不改 error 分支、路由切换 NuxtLoadingIndicator、字体策略。
- 不做短句后台可配置化（静态清单即可，后续要改再开 change）。

## Decisions

- **拟真进度曲线（快进缓动 + 90% 天花板）**：requestAnimationFrame/定时器驱动，ease-out 曲线爬升，渐近 90% 后停滞；`initialize()` resolve 后直接跳 100%。替代方案「按请求完成二分推进」（categories 完 →50%，feeds 完 →100%）被否：两请求并行且体量悬殊，二分与真实耗时不成比例，反而更假。
- **首帧模板内联复制短句与进度逻辑**：模板约束「零外部依赖」（spec 红线），不能与 Vue 侧共享模块；接受双份清单的维护成本，在两处注释互相指向（同步维护约定写入 loading-experience.md）。模板侧用 `setInterval` + CSS transition，无需 rAF（只在 bundle 下载期存活，几秒后被整体替换）。
- **短句清单规模**：每层约 12 条游戏风短句，与产品语境相关（语义向量、订阅源、时间线等玩梗），单文件数组字面量；随机用 `Math.random`（单用户产品，无需防重复抽样）。
- **Vue 侧进度逻辑抽组合式 `useFakeProgress`**（`front/app/composables/`）：参数化天花板/步进/完成回调，返回 `{ progress, tip, start, finish, dispose }`；app.vue 加载分支调用，卸载时 `dispose` 清理全部计时器。便于 vitest 用 fake timers 断言曲线与清理。
- **无障碍**：进度条 `role="progressbar"` + `aria-valuemin/max/now`；短句容器 `aria-live="polite"`（屏幕阅读器播报一次，轮换不轰炸）；`prefers-reduced-motion` 时去掉 transition，数值跳变直显（首帧模板已有同款 media query 惯例）。

## Risks / Trade-offs

- [两层短句清单漂移] → 清单各自内联是刻意取舍；在 loading-experience.md 写明「改短句两处同步」维护约定，模板头部注释指向 Vue 侧清单位置。
- [拟真进度在极快加载下闪跳]（本地 dev 接口 <300ms 返回）→ `finish()` 前保证最小展示时长（约 400ms）再卸载，避免进度条一闪而过的廉价感；首帧模板本身只在 bundle 下载期存活，无此问题。
- [定时器泄漏]（error 分支提前渲染时进度计时器仍在跑）→ `dispose()` 在 `finally` 与 `onUnmounted` 双路径调用，vitest 断言计时器清零。
- [aria-live 轮换频繁打扰读屏用户] → 轮换间隔 4s 且 polite 级别；若嫌吵，后续可将 `aria-live` 仅挂在首条短句（实现期微调，不影响契约）。

## Migration Plan

纯前端静态资源变更，无数据迁移；部署即生效，回滚即还原两个文件 + composables。用户侧无需任何手动操作。

## Open Questions

（无）
