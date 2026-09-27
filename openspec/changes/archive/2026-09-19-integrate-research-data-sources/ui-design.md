<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## 复用契约（minor 档）

本 change 的 UI 面为**设置页一个新 section**（2026-09-19 迭代追加，初始为纯后端 none 档；key 管理按用户要求改为 bocha 同款界面配置后升 minor）。无新页面、无新布局模式、无新弹窗档位。

### 新增界面元素

`SettingsSectionDatasources.vue`（设置页「研究数据源」section，`?section=datasources`）：

1. **四源状态卡**（2 列网格，`grid grid-cols-1 md:grid-cols-2`）：源名+状态点（绿=enabled）、频度标签、覆盖描述、禁用原因（disabled 时 error 色）、滞后说明、「测试连通」probe 按钮（结果行内展示成功行数/耗时或失败 detail）。
2. **Comtrade key 管理卡**：标题+说明（免费档限额与获取入口）、AppToggle 开关（enabled）、API Key 密码框（眼睛切换可见性、脱敏 placeholder「已配置（末 4 位 ****），留空保持不变」）、保存按钮（accent 主色，loading 态）。

### 复用契约

- **组件模式**：完全复刻 `SettingsSectionBocha.vue` → `BochaConfigPanel.vue` 模式（薄壳 section + 面板组件 + composable `useDatasources.ts`）；本 change 面板直接内联在 section 组件里（无跨页复用需求，少一层）。
- **布局**：设置页 workspace 现有分栏与 sidebar 导航（`SettingsWorkspace.vue` sections 数组注册一项，icon `mdi:database-search`，排在「博查搜索」之后）；页面 shell 不变。
- **样式 token**：`var(--color-*)` 主题变量、`input`/`AppToggle` 现有类与组件，零新 CSS 变量。
- **交互约定**：保存成功/失败横幅（success/error bg 圆角条，同博查面板）；key 脱敏语义（空串=不改）与博查完全一致；probe 为显式按钮触发（不自动轮询）。
- **双视口**：2 列网格 `md:` 断点降单列；无 fixed 定位元素，320px 视口安全。

### 不属于本 change 的 UI

研究对话页/工作台数据源引用界面 → change ②（研究对话助手，major 档须原型审批）。
