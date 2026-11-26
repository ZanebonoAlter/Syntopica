# 8C.7 M1/L1 文案修复 — 执行报告

- 日期：2026-10-29
- 范围：8C 矩阵 day-others 收尾文案（`matrix-day-others-2026-10-29/report.md` §Medium M1 + §Low L1）
- 授权：用户已批准 UI（cdb0b70e 原型：0 正式场景为网格内联说明 + 默认展开其他组，**无横幅**）；只做 brief 声明的四处
- 边界：不改 `tasks.md`、不 commit、不部署、不跑 build/generate/typecheck/浏览器/Go/SQL；未改动既有 g4b / 8C.7 报告

## 1. 改动文件与尺寸（before → after）

| 文件 | 行数 | 字节 | SHA1（前 12） |
| --- | --- | --- | --- |
| `console/BoardConsoleStage.vue` | 335 → **299** | 12197 → **11237** | `4d52632e5f81` → `e07881cd504a` |
| `console/BoardConsoleShell.test.ts` | 161 → **164** | 7503 → **7825** | `707becf751b9` → `b96f1db4067b` |
| `console/BoardNarrowSummary.vue` | 385 → **385** | 12352 → **12328** | `61ac007230a1` → `65f351c2263c` |
| `console/BoardNarrowSummary.test.ts` | 324 → **324** | 14630 → **14622** | `867fa4c4917d` → `ea3641294e84` |

四个文件均 ≤500 行 / ≤15360B（最大 `BoardNarrowSummary.test.ts` 324 行 / 14622B）。

## 2. M1①：删除过时 0 正式横幅（BoardConsoleStage.vue）

删除理由：8C 后报日列头 + 「当日其他话题」组即真实入口，横幅「最新日报唯一入口在标题区」已失实且与网格内联说明重复；获批原型无横幅。

### 2.1 模板（`<template>` 内 `<template v-else>` 首块）

改前：

```vue
  <template v-else>
    <div
      v-if="emptyState === 'no-lanes' && latestReportDate"
      class="stage-fallback"
      data-testid="console-no-lanes-fallback"
    >
      <strong>这个版块还没有活跃泳道，但已有日报内容。</strong>
      <p>
        最新日报唯一入口在标题区「打开日报 · {{ shortDate(latestReportDate) }}」；
        也可切「文章归档」按日期核对本版块文章。不必先转正话题。
      </p>
    </div>

    <ObservationRangeControls
```

改后：

```vue
  <template v-else>
    <ObservationRangeControls
```

同文件其余分支（`report-chain-notice` / 池重试 / 网格 / 阅读槽）与 `:as-of="latestReportDate || anchorDate"` 均未动。

### 2.2 随之失效的局部函数（仅此块使用）

改前：

```ts
function shortDate(date: string): string {
  return date.slice(5).replace('-', '/')
}

function scrollToChronicle(): void {
```

改后：

```ts
function scrollToChronicle(): void {
```

### 2.3 随之失效的 scoped 样式（`.stage-fallback` 全仓仅此处使用）

改前（`<style scoped>` 中 `.stage-pool button:disabled` 与 `.stage-chronicle` 之间）：

```css
.stage-fallback {
  padding: 0.65rem 0.8rem;
  border: 1px solid var(--color-border-medium);
  border-left: 3px solid var(--color-accent);
  border-radius: 6px;
  background: var(--color-bg-elevated);
}

.stage-fallback strong {
  color: var(--color-text-primary);
  font-size: 0.82rem;
}

.stage-fallback p {
  margin: 0.2rem 0 0;
  color: var(--color-text-muted);
  font-size: 0.72rem;
  line-height: 1.6;
}

.stage-chronicle {
```

改后：

```css
.stage-chronicle {
```

## 3. M1②：断言改新契约（BoardConsoleShell.test.ts）

### 3.1 用例一（原 L42-53，0 active + 已有日报）

改前：

```ts
  it('0 active 泳道但已有日报：最新入口仍可见，且不被泳道最大日替代', async () => {
    getLaneDynamics.mockResolvedValue({ success: true, data: emptyResponse() })
    getOverview.mockResolvedValue(overview('2026-10-24'))
    const wrapper = mountShell()
    await flushPromises()
    expect(wrapper.find('[data-testid="console-today-report"]').text()).toContain('10/24')
    expect(wrapper.find('[data-testid="console-no-lanes-fallback"]').text()).toContain('打开日报 · 10/24')
    expect(wrapper.find('[data-testid="console-fallback-report"]').exists()).toBe(false)
```

改后（用例名诚实化并保留「报日入口不被泳道最大日替代」的意图）：

```ts
  it('0 active 泳道但已有日报：报日入口仍可见；旧横幅不再出现，网格内联与其他话题组在场，且不被泳道最大日替代', async () => {
    getLaneDynamics.mockResolvedValue({ success: true, data: emptyResponse() })
    getOverview.mockResolvedValue(overview('2026-10-24'))
    const wrapper = mountShell()
    await flushPromises()
    expect(wrapper.find('[data-testid="console-today-report"]').text()).toContain('10/24')
    expect(wrapper.find('[data-testid="console-no-lanes-fallback"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="matrix-empty-no-lanes"]').text()).toContain('0 正式泳道')
    expect(wrapper.find('[data-testid="other-topics-group"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="console-fallback-report"]').exists()).toBe(false)
```

新契约覆盖三层：① 旧横幅不存在；② 网格内联 `matrix-empty-no-lanes` 给出「0 正式泳道」说明（真实入口文案）；③ `other-topics-group` 在场。用例尾部 `触发 console-today-report → router.push('/tags/1974/2026-10-24')` 保持不动。

### 3.2 用例二（原 L85-98，R28 刷新后 0 active）

改前（仅末两行）：

```ts
    expect(wrapper.find('[data-testid="console-today-report"]').text()).toContain('10/25')
    expect(wrapper.find('[data-testid="console-no-lanes-fallback"]').text()).toContain('10/25')
```

改后：

```ts
    expect(wrapper.find('[data-testid="console-today-report"]').text()).toContain('10/25')
    expect(wrapper.find('[data-testid="console-no-lanes-fallback"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="matrix-empty-no-lanes"]').text()).toContain('0 正式泳道')
```

用例名 `R28：0 active 泳道也随候选新日报更新头部；不靠 active 最大日` 未含旧横幅语义，保持不变。

### 3.3 改名例清单

| 文件 | 旧用例名 | 新用例名 |
| --- | --- | --- |
| `BoardConsoleShell.test.ts` | `0 active 泳道但已有日报：最新入口仍可见，且不被泳道最大日替代` | `0 active 泳道但已有日报：报日入口仍可见；旧横幅不再出现，网格内联与其他话题组在场，且不被泳道最大日替代` |

改动性质：只改断言与 1 个用例名，**未增删 `it()`、未加 skip**（两文件 `it(` 计数：BoardConsoleShell.test.ts 10，BoardNarrowSummary.test.ts 12）。

## 4. L1：窄屏措辞（BoardNarrowSummary.vue）

`BoardNarrowSummary.vue` L196-197（`rows.length === 0 && emptyState === 'no-lanes'`，testid `narrow-no-lanes-note`）：

改前：

```vue
      <p v-if="rows.length === 0 && emptyState === 'no-lanes'" class="bns-inline" data-testid="narrow-no-lanes-note">
        暂无活跃泳道（0 正式泳道）——当日内容见下方「当日其他话题」。
      </p>
```

改后：

```vue
      <p v-if="rows.length === 0 && emptyState === 'no-lanes'" class="bns-inline" data-testid="narrow-no-lanes-note">
        0 正式泳道——当日内容见下方「当日其他话题」。
      </p>
```

同文件相邻的 `activeRows.length === 0` 分支文案（`0 正式泳道 · 当日内容见下方「当日其他话题」。`）与其它空态不动。

## 5. L1 断言（BoardNarrowSummary.test.ts）

| 位置 | 改前 | 改后 |
| --- | --- | --- |
| L153（无日报/无泳道/加载/错误合并例） | `expect(noLanes.text()).toContain('暂无活跃泳道')` | `expect(noLanes.text()).toContain('0 正式泳道')` |
| L313（8C 窄屏报日条 + 其他组同屏例） | `expect(wrapper.text()).toContain('暂无活跃泳道')` | `expect(wrapper.text()).toContain('0 正式泳道')` |

意图不变：窄屏 0 正式说明在场；两例同时仍断言报日条 / `narrow-other-group` / 分区行在场，故不构成阻断空态。

## 6. 验证

### 6.1 单测（7 文件，brief 给定命令，无 `--`）

```bash
cd front && pnpm test:unit \
  app/features/tags/components/console/BoardConsoleShell.test.ts \
  app/features/tags/components/console/BoardConsoleShellStates.test.ts \
  app/features/tags/components/console/BoardConsoleShellSelection.test.ts \
  app/features/tags/components/console/BoardConsoleShellNavigation.test.ts \
  app/features/tags/components/console/BoardConsoleShellRepair.test.ts \
  app/features/tags/components/console/BoardConsoleShell.integration.test.ts \
  app/features/tags/components/console/BoardNarrowSummary.test.ts --maxWorkers=2
```

结果：`Test Files 7 passed (7)` / `Tests 61 passed (61)`（两次运行一致）。

| 文件 | 例数 |
| --- | --- |
| `BoardConsoleShell.test.ts` | 10 |
| `BoardConsoleShellStates.test.ts` | 5 |
| `BoardConsoleShellSelection.test.ts` | 7 |
| `BoardConsoleShellNavigation.test.ts` | 16 |
| `BoardConsoleShellRepair.test.ts` | 2 |
| `BoardConsoleShell.integration.test.ts` | 9 |
| `BoardNarrowSummary.test.ts` | 12 |
| **合计** | **61** |

例数与改动性质一致（未增删用例 / 未新增 skip）；集成例 `0 正式泳道：真实报日列头可点、其他组默认展开、真实分区可读，网格不被旧空态挡` 仍绿，证明删横幅未影响真实组件链。

### 6.2 ESLint（仅改动 .ts，.vue 不在配置内）

```bash
cd front && pnpm exec eslint \
  app/features/tags/components/console/BoardConsoleShell.test.ts \
  app/features/tags/components/console/BoardNarrowSummary.test.ts
# exit 0（无输出）
```

未对 .vue 声称 lint 绿（不在 eslint 配置内）。

### 6.3 grep 零残留证据

```text
# 产品侧（.vue/.ts 源码）console-no-lanes-fallback 零残留
$ grep -rn "console-no-lanes-fallback" front/app
front/app/features/tags/components/console/BoardConsoleShell.test.ts:45:    expect(...).exists()).toBe(false)   ← 负向断言
front/app/features/tags/components/console/BoardConsoleShell.test.ts:99:    expect(...).exists()).toBe(false)   ← 负向断言

# 仅剩测试文件中的「横幅不存在」负向断言（brief 要求保留该 testid 作为反向契约）；
# 源码模板/样式/脚本零残留（含 .stage-fallback，全仓 grep 无命中）。

# console 壳/舞台/窄屏三个辖区文件「暂无活跃泳道」零残留
$ grep -rn "暂无活跃泳道" front/app/features/tags/components/console/BoardConsoleStage.vue \
    front/app/features/tags/components/console/BoardNarrowSummary.vue \
    front/app/features/tags/components/console/BoardConsoleShell*.test.ts \
    front/app/features/tags/components/console/BoardNarrowSummary.test.ts
(zero)

# 同目录 BoardMatrixView.vue 的同词属其他组件（brief 指定不动）
$ grep -rn "暂无活跃泳道" front/app | grep -v node_modules
front/app/features/tags/components/console/BoardMatrixView.vue:120   ← 不动（brief 指定）
front/app/features/tags/components/lane-dynamics/LaneDynamicsPanel.vue:184   ← 不动
front/app/features/tags/components/lane-dynamics/LaneDynamicsPanel.test.ts:240 ← 不动
```

## 7. 未做 / 遗留风险

- 未勾 `tasks.md`（父统一勾 8C.7）、未 commit、未部署、未跑 build/generate/typecheck/浏览器/Go/SQL、未派子线程。
- `console-no-lanes-fallback` 字符串仍存在于两个测试的负向断言中（设计要求：断言横幅不存在）；若后续再清理 testid，需同步这两行。
- `BoardMatrixView.vue`（同目录）与 `LaneDynamicsPanel.vue` 的「暂无活跃泳道」文案按 brief 保留不动；它们不在本次 S11-A 宽屏验收路径之外的口径内。
- 部署后用户可见变化：宽屏 0 正式场景不再出现重复横幅，仅网格内联「0 正式泳道 · 可展开当日其他话题」+ 报日列头；窄屏行内文案改为纯「0 正式泳道」前缀。无数据迁移、无用户手动操作。
