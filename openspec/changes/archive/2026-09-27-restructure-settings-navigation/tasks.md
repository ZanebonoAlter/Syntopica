## 1. 用例先行

- [x] 1.1 创建 `test-cases.md`：主链路表（节拍/动作/来源 Scenario/期望/层/落点）覆盖 specs 三个 delta 的全部 Scenario；变体走查（非法 tab、空 tab 参数、旧键大小写）；「继承与调整」表逐行处置旧测试资产（`bash scripts/harness/test-assets.sh settings-workspace` / `user-onboarding` / `data-enrichment` 反查）；验证：文件存在且 Scenario→落点映射无空洞

## 2. 前端导航重组

- [x] 2.1 sections 数组收敛为单一来源（`{ key, group, label, description, icon, component }`），删除 `pages/settings.vue` 的 `sectionComponents` map 与 8 个纯转发薄壳组件（含 SettingsSectionReferenceRoles/SettingsSectionAnalysisMethods）；验证：`grep -r "sectionComponents" front/app` 无结果，`pnpm exec nuxi typecheck` 通过
- [x] 2.2 `SettingsSidebar` 渲染分组标题（组标题不可点击、抽屉模式同构）；验证：组件测试断言 4 个分组标题 + 7 个导航项存在（`pnpm test:unit SettingsSidebar --maxWorkers=2`）
- [x] 2.3 新建复合容器 ×2（datasources-network 含 6 子面板、runtime-status 含 3 子面板）+ `SettingsTabsNav` 子 tab 条（sticky、横向滚动、tab 参数入 URL）；验证：`pnpm test:unit <新容器测试文件> --maxWorkers=2` 断言默认子 tab 与切换后 URL 参数
- [x] 2.4 旧键重定向：9 个旧 section 键命中即 `router.replace` 到新键携带 `tab=<旧键>`；非法/空 tab 值回默认并清参数；验证：组件测试覆盖 9 旧键 + 1 非法 tab 用例全绿
- [x] 2.5 前端 lint + 类型 + 受影响测试收口：`pnpm lint && pnpm exec nuxi typecheck && pnpm test:unit <受影响文件列表> --maxWorkers=2` 全绿

## 3. 日报时刻修复

- [x] 3.1 定时任务面板读写端点改为 `/api/ai/settings`（并入运行状态复合容器时顺路）；验证：设置页定时任务子 tab 打开显示库中真实值（非默认 21:00 硬编码），改值保存出现「已保存」反馈，`docker logs` 后端无 404 记录

## 4. 后端删除：参考角色链

- [x] 4.1 摘 `/reference-roles` 路由注册与 `reference_role_handler.go`；service/orchestrator 中 `referenceRoleAppendix` prompt 拼装段摘除（循环 B interpret/analyze/agentLoop 三处注入点）；验证：`go build ./...` 通过且 `grep -rn "referenceRoleAppendix\|ReferenceRole" backend-go/internal --include='*.go' | grep -v _test` 仅剩 repository/models 待删项
- [x] 4.2 删 repository 的 ReferenceRole 仓储方法与 models 定义、相关测试文件；验证：`golangci-lint run ./internal/dataenrichment/... && go test ./internal/dataenrichment` 全绿

## 5. 后端删除：分析方法卡链

- [x] 5.1 整删 `analysis_method_handler.go`、`analysis_methods.go`、`method_sanitizer.go`，摘 `/analysis-methods` 路由；验证：`go build ./...` 通过
- [x] 5.2 摘调查链选卡与注入段：`board_investigation.go`（36 处）、`board_investigation_synthesis.go`（8 处）、`signal_research.go` 注入段；同步改造 10 个测试文件；验证：`go test ./internal/dataenrichment` 全绿，`grep -rn "methodCard\|MethodCard\|AnalysisMethod" backend-go/internal --include='*.go'` 零命中
- [x] 5.3 删 models 的 AnalysisMethod 与 repository 仓储方法（20+10 处）；验证：`golangci-lint run ./internal/dataenrichment/... && go test ./internal/dataenrichment && go vet ./...` 全绿

## 6. DB 迁移

- [x] 6.1 新增迁移：`DROP TABLE IF EXISTS reference_roles` + `DROP TABLE IF EXISTS analysis_methods`（幂等，注释保留两表结构快照供考古）；验证：迁移在临时容器库执行 `BEGIN; …; ROLLBACK;` 预检通过后真库执行，`\dt` 确认两表消失

## 7. Onboarding 适配

- [x] 7.1 `useOnboarding` settings tour 步骤锚点：`settings-nav-schedulers` → `settings-nav-runtime-status`，核对全部步骤锚点在新导航下可解析（missing-element pre-filtering 日志零跳步）；验证：`pnpm test:unit useOnboarding --maxWorkers=2` + 手动跑一次 settings tour（引导步骤全部定位到实际元素）

<!-- doc-impact: flow api database -->

## 8. 端到端验收与归档

- [x] 8.1 opencli 主链路断言：进入设置 → 分组渲染 4 组 7 项 → 数据源与网络 → 子 tab 切到出站代理 → URL 含 `tab=proxy` → 刷新 tab 保持 → 旧键 `?section=proxy` 重定向到新键；验证：opencli 断言脚本全过（或按 test-design 记人工豁免）
- [x] 8.2 双视口视觉检查：1440×900 / 1920×1080 / 375×667（抽屉）截图存档至 tasks 验证节，含与批准原型的差异说明（预期差异仅：真实数据替换示意数据）
- [x] 8.3 归档门禁：`bash scripts/harness/change-scope.sh` 影响包测试全绿、`doc-impact.sh verify` + `check-standards.sh` 通过、`openspec validate restructure-settings-navigation --strict` 通过；完工汇报含部署影响节（两表 drop、prompt 注入消失、旧深链重定向）

## 验证记录（8.1 / 8.2 证据存档）

### 8.1 opencli 主链路断言（agent-browser @ :5100 静态托管，2026-09-27）

| # | 断言 | 结果 |
| --- | --- | --- |
| 1 | 设置页 4 组 7 项（DOM: groups=4, items=7） | ✓ |
| 2 | 点「数据源与网络」→ 默认 Firecrawl 子 tab + 6 tab 渲染 | ✓ |
| 3 | 切「出站代理」→ URL `?section=datasources-network&tab=proxy` + 面板切换 | ✓ |
| 4 | 直开 URL（模拟刷新）→ tab 保持（出站代理） | ✓ |
| 5 | 旧键 `?section=proxy` → 重定向 `datasources-network&tab=proxy` + 出站代理面板 | ✓ |
| 6 | 非法 tab `?tab=bogus` → URL 清参 + 回默认 Firecrawl | ✓ |
| 7 | 定时任务子 tab：页面 GET 真实值 21:00 = 库值；改 21:05 保存 POST /api/ai/settings 200 → 库值 21:05 → 改回 21:00 | ✓ |
| 8 | settings tour 5 步全走完（锚点零跳步；末步高亮运行状态导航项） | ✓ |

### 顺手修复：研究数据源目录加载失败（pre-existing，复合容器验收时撞见）

根因：`GET /api/datasources` 后端返回裸结构 `{data_sources:[...]}`（spec 未定包装形状），
apiClient 提取 `data.data` 得 undefined，前端 `res.data?.data_sources` 判失败。旧平铺导航下同样坏
（前后端均非本 change 改动，低频面板未被撞见）。修复：`api/datasources.ts` 类型标注 extras 透传 +
`useDatasources.ts` 兼容取法（顶层优先）。验证：浏览器实测 4 源卡片 + Comtrade Key 区正常渲染、
无错误文本；typecheck/lint 过；已随 8.3 后二次部署。

### 8.2 双视口 + 窄屏机械断言与截图

截图存档：`screenshots/`（desktop-1440-datasources-proxy / desktop-1440-settings-home / desktop-1920-settings-home / mobile-375-settings / mobile-375-drawer-open）。

机械断言（优于视觉，test-design 原则）：
- 1440×900：4 组标题 + 7 导航项 + 6 子 tab + tab 条 `position:sticky` + 无横向溢出 ✓
- 375×667：桌面 sidebar 隐藏、汉堡入口可见、抽屉打开 4 组 7 项同构（宽 240px）、无横向溢出 ✓
- 与批准原型差异：仅真实数据替换示意数据（原型为静态 HTML，实现为真实面板嵌入）；原型说明文字中的项数笔误（8/9→7）已随本 change 修正。
