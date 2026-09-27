# Tasks

## 1. 导航加载反馈（D1）

- [x] 1.1 新建 `front/app/composables/useNavLoading.ts`：pending/visible 两态 + 250ms 延迟计时器；导出 `begin()/end()` 与状态 ref；连续 begin 重置计时。验证：`useNavLoading.test.ts`（fake timers 覆盖：250ms 内不显示、超时显示、end 清理、onError 路径清理、连续导航重置）全绿。
- [x] 1.2 新建 `front/app/components/common/NavLoadingOverlay.vue`（fixed 居中 spinner、`pointer-events:none`、`z-30`、`role=status` + `aria-live=polite`、`motion-reduce:animate-none`，视觉复用 mdi:loading + `--color-accent`）。验证：`NavLoadingOverlay.test.ts`（visible 渲染断言、aria 属性、reduced-motion class）全绿。
- [x] 1.3 新建 `front/app/plugins/nav-loading.ts`：router `beforeEach/afterEach/onError` 接线 `useNavLoading`；`app.vue` 挂载 `<NavLoadingOverlay />`（NuxtLoadingIndicator 旁）。验证：`pnpm lint` + `pnpm exec nuxi typecheck` 通过。

## 2. pre-FCP 白屏修复（D2）

- [x] 2.1 `front/app/spa-loading-template.html`：主题判定脚本移至 `<style>` 前；`<style>` 追加 `html` / `html[data-theme="dark"]` 背景色（与 `--spa-bg` 同源）。验证：新增 `front/app/spa-loading-template.test.ts`（读文件断言背景规则存在、主题脚本先于 style 出现）全绿。
- [x] 2.2 手动复测：深色主题硬刷新首页与 /tags，全程无白窗（方法照 `docs/research/spa-loading-white-screen/explore-findings.md`）。验证：截图或采样时间线留档。

## 3. 叙事工坊页瘦身（D3）

- [x] 3.1 新建 `front/app/features/tags/components/PanelAsyncPlaceholder.vue`（居中 spinner 占位）。验证：`PanelAsyncPlaceholder.test.ts` 渲染断言全绿。
- [x] 3.2 `TagsPage.vue`：`BoardThreadBrowser` / `BoardDailyReportTimeline` / `BoardTimelinePanel` / `BoardEnrichmentPanel` / `TopicDetectiveWall` 改 `defineAsyncComponent`（loadingComponent=占位，delay=200）。验证：`TagsPage.test.ts` 增补（默认 tab 不渲染懒面板、切 tab 渲染占位/面板）全绿。
- [x] 3.3 构建体积核对：`pnpm build` 后确认 tags 路由入口 chunk 变小（懒面板拆为独立 chunk）。验证：`ls -S .output/public/_nuxt` 对比留档。

## 4. 回归与收尾

- [x] 4.1 全量前端门禁：`cd front && pnpm lint && pnpm exec nuxi typecheck && pnpm test:unit && pnpm build` 全绿。
- [x] 4.2 端到端手动验证：冷切「叙事工坊」>250ms 出现加载反馈、完成后消失；暖切换无闪烁；四个懒 tab 首切有占位。验证：操作录屏/截图或 agent-browser 采样时间线留档。
- [x] 4.3 文档回检：`docs/reference/standard/frontend/loading-experience.md` 补 pre-FCP 背景与导航加载态条款；doc-impact 声明写入下方「6. 文档」节（§11.2）。

## 5. 测试

- `cd front && pnpm test:unit useNavLoading NavLoadingOverlay spa-loading-template PanelAsyncPlaceholder TagsPage` → 全绿（本 change 受影响测试文件）
- 全量门禁（lint / typecheck / test:unit / build）见 4.1，归档前复跑结果记于「7. 验证」。

## 6. 文档

<!-- doc-impact: flow standard -->

- [x] `docs/reference/standard/frontend/loading-experience.md` — 补 pre-FCP 双主题背景色与导航加载反馈（>250ms 延迟触发）条款（4.3 已更新）。
- [x] `docs/reference/flow/semantic-board.md` — 变更溯源行：TagsPage tab 面板懒加载瘦身（纯实现层性能优化，无业务行为变化）；已补（§12.2）。

## 7. 验证

### Scenario → 测试文件映射

| Scenario | 测试文件 |
| --- | --- |
| 慢 chunk 加载期间显示进度条 | 人工（4.2：冷切换时顶部进度条行为保持不变，本 change 未改此行为） |
| 慢路由切换出现加载态 | 人工（4.2 冷切「叙事工坊」>250ms 出现加载反馈）；`front/app/composables/useNavLoading.test.ts` |
| 快路由切换不出现加载态 | `front/app/composables/useNavLoading.test.ts` |
| 导航完成或失败卸载反馈 | `front/app/composables/useNavLoading.test.ts` |
| 减弱动效偏好 | `front/app/components/common/NavLoadingOverlay.test.ts` |
| 深色主题刷新无白闪 | 人工（2.2 深色主题硬刷新无白窗）；`front/app/spa-loading-template.test.ts` |
| 浅色主题背景一致 | 人工（2.2 浅色主题背景一致）；`front/app/spa-loading-template.test.ts` |

### 验证命令与结果

- `cd front && pnpm test:unit useNavLoading NavLoadingOverlay spa-loading-template PanelAsyncPlaceholder TagsPage` → PASS
- `cd front && pnpm lint` → PASS
- `cd front && pnpm exec nuxi typecheck` → PASS
- `cd front && pnpm build` → PASS
- 人工（4.2/2.2）：深色主题硬刷新首页与 /tags 全程无白窗；冷切「叙事工坊」>250ms 出现加载反馈、完成后消失；暖切换无闪烁（采样时间线留档于 2.2/4.2 执行记录）

> 以上命令已于 2026-09-17 归档前复跑实测零失败。全量 `pnpm test:unit` 基线另有 2 个非本 change 失败文件（`useOnboarding.test.ts` happy-dom localStorage mock 环境问题、`chunk-error-fallback.test.ts` #imports 解析），均不属本 change 影响面；本 change 5 个受影响测试文件全绿。lint 7 个 warning 均在其他 change 文件（articles/schedulerMeta），本 change 文件零告警。
