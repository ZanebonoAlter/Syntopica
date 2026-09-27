<!-- ui-impact: major -->
<!-- ui-approval: approved -->
<!-- ui-prototype: ui-prototype/index.html -->

# UI Design — 设置工作区导航重组

## 1. User Journey

**入口**：主界面顶栏齿轮 / `pages/settings.vue` 直达（不变）。

**主任务**（改配置）：
1. 用户找"代理设置"：进入设置页 → sidebar「数据源与网络」组 → 点「数据源与网络」→ 子 tab「出站代理」→ 改地址保存。旧路径是平铺 15 项里找"出站代理"；新路径多一跳但可扫读性更高（组名先过滤了一半噪音）。
2. 用户查队列堵没堵：sidebar「运行状态」→ 点「运行状态」→ 子 tab「队列」。

**次任务**（内容管理）：订阅源 / 兴趣画像 / 分析方法 / 页边注四项功能与内部交互完全不变，只是住进「内容管理」组。

**深链**：`?section=<key>` 保持支持；旧 section 键（firecrawl/bocha/searxng/rsshub/datasources/proxy/queues/schedulers/ai-health）重定向到新复合键 + 子 tab（`?section=datasources-network&tab=proxy`），外部书签与 onboarding 不死链。

## 2. Information Architecture

```
设置页导航（15 平铺 → 4 组 7 项）
├─ 内容管理
│   ├─ 订阅源 feeds                （不变）
│   ├─ 兴趣画像 preferences        （不变，纯查看）
│   └─ 页边注 margin-notes         （不变）
│     （分析方法 analysis-methods 随功能删除消失）
├─ AI 配置
│   ├─ AI 模型 ai-providers        （不变）
│   └─ 能力路由 capability-routes  （不变）
├─ 数据源与网络
│   └─ 数据源与网络 datasources-network（新复合 section）
│       └─ 子 tab：Firecrawl｜博查｜SearXNG｜RSSHub｜研究数据源｜出站代理
└─ 运行状态
    └─ 运行状态 runtime-status（新复合 section）
        └─ 子 tab：AI 健康｜队列｜定时任务
```

删除项：「参考角色」（2026-09-04 已摘入口）与「分析方法卡」（事实上零使用）两套面像体系全链删除（见 proposal）。

**Section 键映射**（旧 → 新）：
| 旧键 | 新去向 |
| --- | --- |
| feeds / preferences / margin-notes | 键不变，归「内容管理」组 |
| analysis-methods | 功能全链删除，键不复用 |
| ai-providers / capability-routes | 键不变，归「AI 配置」组 |
| firecrawl / bocha / searxng / rsshub / datasources / proxy | `datasources-network` + `tab=<旧键>` |
| ai-health / queues / schedulers | `runtime-status` + `tab=<旧键>` |

## 3. Interaction Contract

**主导航**：sidebar 分组标题（不可点）+ 组内项（可点，active 态 accent）。移动端维持现有抽屉模式（767.98px 断点，抽屉宽 min(80vw, 320px)），组结构在抽屉内同样呈现。

**复合 section 子 tab**：
- 进入复合 section 默认落在第一个子 tab（datasources-network→firecrawl；runtime-status→ai-health）
- 子 tab 切换更新 URL `tab` 参数（可刷新/可分享）
- 子 tab 记忆：不跨 section 记忆（每次进入回默认）

**保存交互**：各子面板内部保存/校验行为不变（博查 key 校验、代理测试连通性等沿用现有面板逻辑），复合容器只做布局承载，不拦截不改造。

**危险操作**：无新增（各面板现有危险操作不变）。

**Onboarding tour**：settings tour 步骤改锚新键（`settings-nav-datasources-network` 等）；引导文案提及"数据源与网络"为六项合并。

## 4. State Matrix

| 状态 | 复合 section 容器 | 各子面板 |
| --- | --- | --- |
| loading | 容器即时渲染，子 tab 常驻（tab 数静态） | 各子面板沿用自身 loading（如代理 spinner） |
| empty | N/A（tab 静态） | 研究数据源空目录态沿用现有 |
| error | 子面板错误气泡在面板内展示，不顶掉 tab 条 | 沿用各面板现有错误呈现 |
| success | N/A | 保存成功气泡沿用（如代理 proxySuccess） |

**错误恢复**：某子面板加载失败不影响其他子 tab 切换（面板级隔离）；URL 非法 tab 值回默认 tab 并清参数。

## 5. Layout Contract

- **模式**：保持现有全高双栏骨架（100vh flex：sidebar + content 各自内滚），视作 `split` 模式既有存量变体——本次**不引入 AppPageShell**，避免导航重组混入无关视觉变化；未来设置页整体视觉收敛时另行 change。
- **栏宽**：sidebar 200px 固定（现状不变）；content 弹性，min-width 480px，窄于 768px 走抽屉单栏（现状不变）。
- **子 tab 条**：位于 section header 下方，横向排布，6 tab 放不下时横向滚动（`overflow-x: auto`，无换行），tab 条粘滞于内容滚动区顶部（sticky top-0，带 bg 遮罩防止内容透出）。
- **Dialog**：本 change 无新增/改动 dialog。
- **目标视口**：1440×900、1920×1080 双档验收；375×667 窄屏档（触及抽屉降级路径）。

## 6. Component Reuse

| 复用 | 用途 |
| --- | --- |
| `SettingsSidebar` | 加 `group` 字段渲染分组标题（结构微扩，无新组件） |
| `SettingsWorkspace` | sections 数组加 group；slot 分发机制不变 |
| 现有 6+3 个子面板实体组件 | 原样嵌入复合容器（BochaConfigPanel 等继续复用） |
| 新增 `SettingsTabsNav`（轻量子 tab 条） | 复合 section 内部导航；结构对齐现有 tabs 模式（若有 `components/ui` tabs 则映射之，实现时优先复用） |
| 删除 | 8~18 行纯转发薄壳 ×8（SettingsSectionBocha/Firecrawl/Searxng/Rsshub/ReferenceRoles/CapabilityRoutes→保留实体、AiProviders/AnalysisMethods 的薄壳层并入 map 直引）+ `AIRouterSettingsPanel.vue` |

## 7. Prototype

`ui-prototype/index.html` — 静态可丢弃原型（editorial 主题 token：stone 暖白 / ink #1a1a1a / 红 accent #d94a4a / 印刷阴影）。可点击：sidebar 7 项切换 + 两个复合 section 的子 tab 切换。展示三个代表性视图：数据源与网络（6 子 tab 表单示意）、运行状态（3 子 tab 监控示意）、内容管理归组（占位示意）。

## 8. Acceptance

实现后补：
- opencli 主链路断言：进入设置页 → sidebar 分组渲染 4 组 7 项 → 点「数据源与网络」→ 子 tab 切到「出站代理」→ URL 含 `tab=proxy` → 刷新后 tab 保持；旧键 `?section=proxy` 重定向到新键。
- 1440×900 / 1920×1080 双视口 + 375×667 窄屏（抽屉）视觉检查证据（tasks.md 验证节）。
- 与批准原型的差异说明。
