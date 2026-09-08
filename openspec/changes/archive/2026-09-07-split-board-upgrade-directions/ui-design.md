<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## UI Impact Summary

`UpgradeSuggestionPanel`（tags 页「升级建议」入口的自绘 overlay 面板，容器与触发按钮不动）内容区重组：旧内存探索上半区（候选列表/簇/建议 + mode radio）整体删除；持久化建议区上方新增「两步模式选择 + 版块单选」生成入口。无新页面、无布局模式变更、无 dialog 尺寸档变更。

## Entry & Entry Change

- 入口不变：`BoardListSidebar`「升级建议」按钮 → `UpgradeSuggestionPanel`（`visible` overlay）。
- 入口内部变更：
  - 顶部新「生成入口区」：方向两选（创建版块 / 版块扩充）→ 来源两选（单标签 / 组合标签）；选「版块扩充」时追加版块单选下拉（活跃版块，展示「版块名 + 描述摘要」，搜索可选）。
  - 时间窗口下拉（days）保留，挂生成按钮旁（仅创建方向生效，扩充方向禁用并提示）。
  - 旧上半区（`usp-mode-selector` radio、候选列表、簇列表、内存建议卡片、「获取 LLM 建议/重新分析」按钮）删除。

## Affected States

| 状态 | 行为 |
| --- | --- |
| generating（生成中） | 生成按钮 loading + 禁用；模式选择器与版块下拉同步禁用 |
| empty（无建议） | 区分两种：未生成过 → 引导文案（先选模式）+ 生成入口；已生成但无结果 → 「本轮无建议」（扩充方向附「版块可能已充分覆盖」提示） |
| error（生成失败） | API 参数错误（缺版块/无效版块）→ 入口区行内提示；LLM/服务端错误 → 沿用面板现有错误提示样式 |
| success（确认成功） | 沿用现状：建议行状态更新 + 匹配回填提示（backfillNotice）；扩充确认成功后目标版块构成变化不即时刷新侧栏（回填提示保留） |

## Component & Layout Reuse

- 容器：复用面板现有 overlay 结构与样式体系（backdrop、`usp-*` class 族），不引入 AppDialog 尺寸档（面板本就非 dialog 容器，保持现状）。
- 控件：模式选择器复用现有 `usp-mode-option` radio 样式（语义从 discover/expand 改为 create/expand 方向 + source）；版块下拉复用现有 merge 下拉的搜索列表样式（`usp-merge-dropdown--search` 族）改单选用；建议卡片、filter tabs、chips 全部沿用。
- 删除：`suggest` 事件链、candidates/clusters/suggestions/upgradeMode 等旧 props 与对应样式类。

## Acceptance Mapping

- 组件测试（`pnpm test:unit`）：模式选择器两步交互、扩充方向未选版块时生成按钮禁用、版块下拉渲染活跃版块、生成后列表刷新调用正确参数。
- opencli 主链路：tags 页 → 升级建议面板 → 选「版块扩充+单标签+某版块」→ 生成 → 建议列表出现且卡片展示锁定版块。
- 展示合理性（机械锚，2026-09-07 补修）：`.usp-overlay` 定位规则锚（fixed/inset/z-index）+ Teleport 挂载锚（overlay 在 body 不在组件原地），见 `UpgradeSuggestionPanel.gen.test.ts` 末尾 describe；浏览器层 computed style 断言 fixed/inset:0/z-index:100/挂 body（2026-09-07 实测过）。
- 人工视觉检查：双视口（1440×900 / 1920×1080）面板布局无破版 —— 截图实物：`ui-verification/tags-upgrade-panel-1440x900.png`、`ui-verification/tags-upgrade-panel-1920x1080.png`，视觉子代理目检 PASS（2026-09-07）。

## 差异与修复记录

- **2026-09-07 补修**：实现期重写 `<style scoped>` 误删 `.usp-overlay` 定位规则，弹窗退化为流内 div 坠到页面底部（验收四维度中「展示合理性」当时无机械锚、视觉检查空转未拦住）。修复：从旧版恢复 overlay 样式 + 补浮层展示锚单测 + 截图实物补齐。验收维度清单已回写《开发执行规范》§5.3「前端验收四维度」。
