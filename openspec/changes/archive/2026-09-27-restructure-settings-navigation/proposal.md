<!-- complexity: complex -->
<!-- ui-impact: major -->
<!-- constraint-domains: data-enrichment, discovery -->

## Why

设置工作区长年"加项不整理"：15 个平级导航项无分组、6 个外部服务配置各自占位、监控类与内容管理类与配置类混居；另有一条 2026-09-04 已摘除入口的"参考角色"僵尸链路（后端 API、prompt 注入、前端面板全部存活但无 UI 入口）与一个零引用死组件。导航重组 + 僵尸清理可以让设置页恢复可扫读性。

## What Changes

- **删除"参考角色"全链**（用户已拍板：中间产品，没用了）
  - 前端：`SettingsSectionReferenceRoles.vue`、`ReferenceRolePanel.vue`、`api/referenceRoles.ts` 及测试 mock 引用
  - 后端：`/reference-roles` CRUD handler/service/repository、`reference_roles` 表（含 seed migration）
  - 行为变化：数据增强循环 B 的 LLM prompt 不再注入参考角色画像（该功能 spec 从未立项，无契约影响）
- **删除"分析方法卡"全链**（用户拍板：事实上零使用，与参考角色同族面像 v2，一起入土）
  - 数据实证：库中仅 1 条 legacy 卡（从参考角色迁移、停用、从未启用），选卡注入从未发生过，零行为损失
  - 前端：`SettingsSectionAnalysisMethods.vue`、`AnalysisMethodPanel.vue`（573 行）、`api/analysisMethods.ts`、`pages/settings.vue` 引用
  - 后端：`analysis_method_handler.go`、`analysis_methods.go`、`method_sanitizer.go` 整删；`board_investigation.go` 选卡逻辑、`board_investigation_synthesis.go` 注入段、`signal_research.go` 注入段、repository/models 摘除；约 8 文件 + 10 测试文件适配
  - 数据库：drop `analysis_methods` 表 migration
  - spec：`data-enrichment` 中方法卡相关句（如 104 行「方法卡自动选择仅适用于 board_investigation」）delta 移除
- **删除死代码**：`features/ai/components/AIRouterSettingsPanel.vue`（零引用；`useAIRouterSettings.ts` 保留，仍被 AIProviderManagement 等使用）
- **设置导航重组**（信息架构变化）：
  - Sidebar 由 15 项平铺改为 **4 组分组导航**
  - 「数据源与网络」：6 个外部服务配置（Firecrawl / 博查 / SearXNG / RSSHub / 研究数据源 / 出站代理）合并为 1 个 section，内部子 tab 切换
  - 「运行状态」：AI 健康 / 队列 / 定时任务合并为 1 个 section，内部子 tab 切换
  - 「AI 配置」：AI 模型、能力路由保持独立（交互重、各自成页）
  - 「内容管理」：订阅源、兴趣画像、页边注保持独立（分析方法随功能删除消失；本次不动剩余内容，只归组）
  - 导航项 15 → 4 组 7 项
- **Bug 修复：日报生成时刻保存失败**：`SettingsSectionSchedulers.vue` 调用 `/api/settings`，但后端路由注册在 `/api/ai/settings`（admin/routes.go:22-23）——GET/POST 全部 404。症状：打开页面静默显示默认 21:00（非真实配置）、点保存报「保存失败」。修复：前端两处 URL 改为 `/api/ai/settings`（后端保存链路与调度器 60s 现读机制本身健全，改完即生效）
- **Onboarding 适配**：`useOnboarding` 的 settings tour 步骤绑定旧 section 键，随新导航键更新
- **薄壳层收编**：合并进新 section 的 8 个纯转发薄壳组件（8~18 行）随重组移除/内联，`sections` 数组与 `sectionComponents` map 的双重映射收敛为一处

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `settings-workspace`: 导航信息架构由 15 项平铺改为 4 分组 7 项；新增两个复合 section（数据源与网络、运行状态）及其内部子 tab 契约；section 路由键变化
- `user-onboarding`: Settings page guided tour 步骤锚点适配新 section 键与分组结构
- `data-enrichment`: 移除方法卡相关行为契约（调查链选卡/清洗注入/信号研究注入；库中零启用，无实际行为变化）

## Impact

- **前端**：`pages/settings.vue`、`features/settings/components/*`（Workspace/Sidebar 重组、6+3 个薄壳移除、两个新复合 section 容器）、`features/ai/components/AIRouterSettingsPanel.vue` 删除、`useOnboarding.ts` tour 步骤更新、`SettingsSectionSchedulers.vue` 端点修复
- **后端**：`internal/dataenrichment/`（reference_role + analysis_method 两套 handler/service/repository/models/seed migration 删除、调查链与信号研究中的选卡/清洗/注入段摘除）、`internal/app/router.go`（/reference-roles、/analysis-methods 路由摘除）
- **数据库**：drop `reference_roles` 与 `analysis_methods` 两表 migration（破坏性，需按 db-migration-safety 规范执行）
- **行为变化**：数据增强 LLM prompt 不再注入参考角色画像与方法卡（两者事实上从未生效注入，预期无可见差异）；设置页内容管理组剩订阅源/兴趣画像/页边注三项
- **不改**：feeds / preferences / margin-notes 三个内容管理 section 的内部功能与 `feed-settings-ui` 契约；AI 模型 / 能力路由的配置功能
