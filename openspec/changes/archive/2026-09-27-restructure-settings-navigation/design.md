## Context

设置页现状：`pages/settings.vue` 持 `sectionComponents` map（15 键）→ `SettingsWorkspace`（sections 数组 + slot 分发）→ `SettingsSidebar`（平铺渲染）。8 个 8~18 行纯转发薄壳组件。参考角色与分析方法卡两套画像体系的完整链路存活但事实零使用（详见 explore-findings pin：`reference_roles` 表仅 seed 数据且 UI 入口 2026-09-04 已摘；`analysis_methods` 表仅 1 条 legacy 停用卡、注入零次发生）。日报时刻保存因前端调 `/api/settings`（正确为 `/api/ai/settings`）而 404。

约束：
- 布局契约：设置页保持既有全高双栏骨架（ui-design.md Layout Contract 已记录不引入 AppPageShell 的理由）
- data-enrichment 域红线（constraint-injection 注入）：分析编排 SHALL NOT 注入画像/方法卡——删除后该红线天然满足
- DB 迁移走 db-migration-safety（drop 表为破坏性操作，须可回滚）

## Goals / Non-Goals

**Goals:**
- 导航信息架构落地：4 分组 7 项 + 两个复合 section（子 tab + URL 承载 + 旧键重定向）
- 两套画像体系全链删除（前端/后端/DB/spec），调查链与信号研究的 prompt 装配段干净摘除且主流程不变
- 日报时刻保存修复 + onboarding tour 锚点适配

**Non-Goals:**
- 不重做任何保留 section 的内部 UI（feeds/preferences/margin-notes/ai-providers/capability-routes 原样嵌入）
- 不引入 AppPageShell / 不改设置页整体视觉（存量迁移豁免，另行 change）
- 不动 feed-settings-ui、ai-capability-routing 等相邻 capability 的行为
- 不清理 dialog/ 目录中 BochaConfigPanel 等的目录归属（复用原样，搬家另行处理）

## Decisions

### D1. 复合 section 容器：新轻组件 + 现有面板原样嵌入，不做懒加载抽象
「数据源与网络」「运行状态」各建一个复合容器组件（内部 `SettingsTabsNav` 子 tab 条 + `<component :is>` 分发现有实体面板）。备选：v-if 逐个条件渲染（等价但重复样板）、defineAsyncComponent 懒加载（6 个小面板多数 <150 行，懒加载收益低且引入 loading 态复杂度）——都否。默认子 tab：datasources-network→firecrawl，runtime-status→ai-health。

### D2. 旧键重定向在 `activeSection` computed 内做，不加中间路由
路由仍单页 `/settings?section=`，旧键映射表（9 旧键→2 新键+tab）放在 Workspace 的 computed 里，命中即 `router.replace` 到新键并保留 tab 参数。备选：navigate guard / 独立 redirect 路由——对本例过度设计。非法 tab 值回默认并清参数（spec 已定）。

### D3. sections 数组与 sectionComponents map 收敛为单一来源
sections 数组扩展 `{ key, group, label, description, icon, component }`，`pages/settings.vue` 的 map 删除，Workspace 直接从 sections 拿 component 渲染。薄壳组件全部移除（包括不合并 section 的 ai-providers/capability-routes 薄壳——实体组件直接挂进数组）。备选：保留 map 双写——正是现状债，否。

### D4. 画像删除顺序：先摘消费端再删供给端，一 change 内单任务组完成
删除顺序（任务依赖即按此排）：①前端入口（nav 键/组件/api client/测试）→ ②后端路由+handler → ③调查链/信号研究注入段摘除（board_investigation 选卡、synthesis 注入、signal_research 注入、method_sanitizer 删除）→ ④repository/models → ⑤DB migration drop 表。参考角色链同理（更简单：仅 prompt 拼装段 + CRUD）。两链共用任务模板，不同任务组。
备选：按层删（先删所有前端再删所有后端）——中间态编译不过，否。

### D5. DB migration：drop 表 + 前向清理，不做数据归档
`reference_roles`（仅 seed 演示数据）与 `analysis_methods`（1 条停用卡）均无用户数据价值，直接 drop。migration 记录保留表结构快照注释供考古（回滚 = 按快照重建空表，无数据恢复诉求）。seed migration 中 reference_role_seed_retire 相关历史迁移不改写（迁移链不可变，drop 是新迁移）。

### D6. 日报时刻修复随复合 section 顺路改端点
`SettingsSectionSchedulers` 的内容将并入运行状态复合容器，`/api/settings` → `/api/ai/settings` 两处 URL 修改与迁移同任务完成，不单独拆 PR 粒度。

### D7. onboarding tour 锚点最小改
`useOnboarding` settings tour 步骤表更新锚点：`settings-nav-schedulers` → `settings-nav-runtime-status`，其余锚点键不变（feeds/ai-providers/nav 本身）。不重写 tour 结构。

## Risks / Trade-offs

- [调查链摘除方法卡选卡时引入回归] → 摘除后跑 `go test ./internal/dataenrichment`（影响包）+ method 相关 10 个测试文件同步改，任一红即阻断
- [drop 表迁移在多环境执行差异] → 按 db-migration-safety：BEGIN/ROLLBACK 预检、迁移幂等（IF EXISTS）、部署前备份
- [旧键深链（外部书签/引导残留）] → D2 重定向兜底，spec 有对应 Scenario 验收
- [合并 section 后"找设置多一跳"] → 分组标题提供可扫读性补偿；组内子 tab 数 ≤6，横向可滚动（ui-design 已定）
- [tour 锚点漏改导致引导静默跳步] → spec 新增 Scenario「anchors resolve」+ useOnboarding 的 missing-element pre-filtering 会在日志留痕，验收时核对

## Migration Plan

1. 后端删除 + migration 先行（服务可独立发布：路由摘除后旧前端不再调 reference-roles/analysis-methods，settings nav 旧键仍可用）
2. 前端重组 + 修复随后发布（此时后端已无两套画像端点）
3. 回滚：前端回滚即可恢复 UI 导航（旧前端不调已删端点的功能仅 reference-roles 面板——其入口已不存在，无影响）；DB 不回滚（表内无价值数据，重建空表即可）

## Open Questions

（无——影响 specs/任务拆解的问题均已在本档与 ui-design.md 定案）
