# UI 验收 — lane-coverage-entry-list（2026-11-27）

> minor 档：组件测试为验收主线（见 tasks §5 映射表）；本账本是加做的 agent-browser 实测，因为本次起因是「看着突兀」，只有视觉能证。
> 环境：树莓派本机，后端静态托管 `http://127.0.0.1:5100/tags/1974`（中东地缘政治与美伊关系），产物 `_nuxt/BU7iW8bj.js`（`deploy-frontend.sh --no-restart` 铺盘），agent-browser 0.37.1 无头 Chromium。
> 口径：delta spec「窗口内未进日报的活跃泳道可见」5 个 Scenario + ui-design Acceptance「折叠态 1440×900 / 1920×1080 各一张、展开态一张、⋯ 菜单一张、列表不产生页面级横向溢出」。

### A-1 折叠态 1440×900：矩阵底部只剩一条折叠条，没有整行「—」【pass】
依据：`[data-testid="coverage-toggle"]` 存在、`aria-expanded="false"`，文案「窗口内未进日报 · 2 协议落地引发能源市场剧烈震荡、中东冲突波及旅游业：Booking 发布谨慎指引 展开查看」；折叠条不在 `[data-testid="matrix-grid"]` 内（`grid.contains(bar) === false`）；`coverage-list` 不存在；日格总数 70（= 5 条有数据的活跃泳道 × 14 天，改动前为 7 行 × 14 = 98）。截图 `collapsed-1440.png`。

### A-2 展开态 1440×900：列表项 = 名字 + 「最近命中 MM-DD」+ 行尾 ⋯，没有日期格【pass】
依据：点折叠条后 `aria-expanded="true"`，`coverage-list` 共 2 项：`协议落地引发能源市场剧烈震荡 最近命中 11-10`、`中东冲突波及旅游业：Booking 发布谨慎指引 最近命中 06-26`；每项 `.omx-cell` 数为 0、带 `lane-menu-trigger-<id>`；列表左右缘 319 / 1134，与矩阵容器左右缘 290 / 1134 对齐（右缘重合，左缘缩进对齐折叠条标题）。日期与 `GET /api/semantic-boards/1974/topics` 的 `last_seen_date` 逐条一致（2026-11-10 / 2026-06-26）。截图 `expanded-1440.png`。

### A-3 列表项 ⋯ 菜单：重命名 / 编辑泳道 / 归档泳道三项，不改变选中泳道与选中日【pass】
依据：点 `lane-menu-trigger-9` 后菜单项 testid `lane-menu-trigger-9-rename / -edit / -archive` 文本为「重命名 / 编辑泳道 / 归档泳道」；菜单打开前后 `.omx-lane.is-selected` 仍是「美伊博弈升级：从经济威胁到全面孤立」、`.omx-cell.is-selected` 仍为 1（11-26 格）。截图 `menu-1440.png`。归档确认弹层带泳道名由 `BoardConsoleShell.integration.test.ts` 新用例锁住（review R-1），未在浏览器里重复走写操作。

### A-4 折叠态 1920×1080：折叠条与矩阵同宽，仍只有一条【pass】
依据：`aria-expanded="false"`，折叠条不在 `matrix-grid` 内，`coverage-list` 不存在；折叠条左右缘 290 / 1566 与网格左右缘 290 / 1566 重合。截图 `collapsed-1920.png`。

### A-5 列表不产生页面级横向溢出【pass】
依据：1440×900 折叠态、展开态与 1920×1080 折叠态下 `document.documentElement.scrollWidth <= clientWidth` 均成立（`pageOverflowX === false`）；列表是 `.omx-scroll` 之后的兄弟节点，不进横滚容器。

## 截图清单

- `collapsed-1440.png`
- `expanded-1440.png`
- `menu-1440.png`
- `collapsed-1920.png`
