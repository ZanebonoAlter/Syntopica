# UI 验收：总览报头「主页」入口归位（8F / FB-13）

- **变更**：`/tags` 未选版块时的雷达总览，页头「主页」返回入口从标题簇左侧（`.rd-heading` 内）挪到右上操作区（`.rd-issue-actions` 内），紧贴「全局治理」左侧，并与其共用 `rd-action-btn` 按钮档。
- **用户确认**：位置=紧贴「全局治理」左侧；样式=统一成与「全局治理」同档次要按钮（min-height 2rem / 0.7rem 字 / border-medium / hover 变 accent）。
- **环境**：静态托管 `http://localhost:5100`（`backend-go/frontend/` 产物，后端同端口），agent-browser 独立会话 `radar-home-8f`，真实库数据（2026-10-30，6 关注板块）。

## 判定条目

| ID | 场景 | 判定 | 依据 |
| --- | --- | --- | --- |
| A-8F-1 | 「主页」落在右上操作区内、紧贴「全局治理」左侧 | **pass** | 四档实测 `homeInActions=true`、`homeInHeading=false`；`.rd-issue-actions` 子元素顺序 = `div.rd-meta`(统计) → `a.rd-home`(主页) → `details.rd-more`(全局治理) → `button`(新建版块)；两者间距 14px = 操作区标准 gap，无额外留白 |
| A-8F-2 | 与「全局治理」同档（尺寸/字号/边框/悬停一致） | **pass** | 四档实测 computed 完全一致：fontSize 11.2px(=0.7rem)、minHeight 32px(=2rem)、padding 0px 8.8px(=0.55rem)、borderRadius 4px、borderColor 浅色 rgba(26,26,26,0.15) / 暗色 rgba(255,255,255,0.1)；作用域规则 `.rd-action-btn:hover/:focus-visible { border-color: accent; color: accent }` 同时命中两者 |
| A-8F-3 | 入口仍可点回主页 | **pass** | agent-browser 实点 `[data-testid="radar-home"]` → URL 变为 `http://localhost:5100/`（未跳错路由、未被遮挡） |
| A-8F-4 | 「全局治理」菜单未被改动破坏 | **pass** | 实点 summary → `details.open=true`、菜单 `display:flex`、浮层 x=1152 y=110 w=144（右下对齐未溢出）、五项齐全（扩充版块/匹配回填（全库）/匹配规则/标签合并/标签池） |
| A-8F-5 | 1440×900 无页面级横向溢出 | **pass** | `scrollWidth 1440 = clientWidth 1440`；主页 x=1157 w=61 h=32 y=73，治理 x=1232 w=64 h=32 y=73 |
| A-8F-6 | 1920×1080（major 双视口第二档）无横向溢出、同档一致 | **pass** | `scrollWidth 1920 = clientWidth 1920`；主页 x=1637 w=61 h=32 y=83，治理 x=1712 w=64 h=32 y=83，间距 14px，computed 同 A-8F-2 |
| A-8F-7 | 375×667 窄屏不破版、入口与治理同组 | **pass** | 主页 x=196 w=61 h=32 y=117、治理 x=265 w=64 h=32 y=117（同排），「新建版块」自然换行至 y=158；`scrollWidth 375 = clientWidth 375` |
| A-8F-8 | 390×844 窄屏不破版、入口与治理同组 | **pass** | 主页 x=196 w=61 h=32 y=99、治理 x=265 w=64 h=32 y=99（同排），「新建版块」换行至 y=140；`scrollWidth 390 = clientWidth 390` |

## 截图

- `overview-1440-light.png` — 1440×900 浅色：主页与全局治理同排同档，新建版块为主按钮。
- `overview-1440-dark.png` — 1440×900 暗色：两按钮边框/文字色同档。
- `overview-1920-light.png` — 1920×1080 浅色（major 双视口第二档）。
- `overview-375-dark.png` — 375×667 暗色：操作区换行，入口不溢出。
- `overview-390-light.png` — 390×844 浅色。

## 边界声明

- 本轮只验总览页头操作区归组；**未**重跑整 change 的 117 条历史 Scenario 欠账、8E 矩阵黄点链路、阅读台/编排态回归，也**未**据此宣称整 change 可归档。
- 组件级断言见 `front/app/features/tags/components/console/BoardRadarMasthead.test.ts`（6 例，含源码样式锚）与 `BoardRadarView.test.ts`（20 例）。
- 旧窄屏规则「隐藏『主页』文字只留图标」随入口移位删除：入口不再挤在标题旁，窄屏由操作区 flex-wrap 承载，与「全局治理」保持同档（与用户确认的「同档」一致）。
- 仓库 eslint 配置未覆盖 `**/*.vue`，`.vue` 的模板与 scoped CSS 对 lint 天然失明 → 档位口径由源码样式锚用例 + 本报告实页几何双重承保。
