# UI 验收报告：fix-daily-report-ke-row-layout（tasks.md 4.1）

- 执行日期：2026-10-16 23:29 – 2026-10-17 00:15
- 环境：静态托管 `http://localhost:5100/`（两轮 `deploy-frontend.sh` 部署：初版 + review R-2 处置后最终构建）；agent-browser 无头浏览器，session `dailyui-6d5795ec8cb0`；真实数据
- 数据：board 1974 / report 1135（dismissed 态线索 14170「美以就打击伊朗预案进行磋商」）；board 2272 / report 1141（真实 suggested 建议 ×2，thread 14237/14238；无标记行 ×2）
- 数据安全：**全程未点击任何标记动作按钮**——真实 suggested 建议属用户待处理项，确认/不采纳/仍标为关键均为不可原路还原的状态迁移（suggested→confirmed 后取消落 null，建议丢失），不做破坏性操作；「标记不误展开文章」交互语义由组件/宿主层回归测试兜底（2.2 跑绿），本报告仅验布局。
- 方法：`getBoundingClientRect` 机械量测（非目测）+ 截图存证。

## 逐场景结论

### A-1 建议行左右等宽 50/50【pass】
- 依据：board 2272 report 1141，2 条 suggested 行实测 `header 宽占比 48.7%`、`ke-detail 宽占比 48.7%`（两等宽列扣除 1rem column-gap 后各半），`ke-detail computed grid-column: 2`、顶部与标题对齐（Δy<2px）；理由块带 2px 左分隔线，长理由折行无横向溢出（`scrollWidth<=clientWidth`）。截图 `evidence/desktop-suggested-1280.png`。
- 对比修复前（同方法实测旧构建）：header 被挤至 min-content 窄条（用户截图 ≈18%），建议占右侧 auto 宽列——失调已消除。

### A-2 动作组行底右对齐（右下角）【pass】
- 依据：suggested 行 `.ke-actions` computed `grid-column: 1 / -1`、`justify-content: flex-end`、`flex-direction: row`，距行右缘 0px（右贴齐）、位于标题/理由内容下方 7px（row-gap）；徽标「AI 建议」+「确认为关键」+「不采纳」横排居右。截图同 A-1。
- dismissed / 无标记行同规则（`actCol 1/-1`、`flex-end`、右贴齐、间距 7px），跨状态动作位置一致。截图 `evidence/desktop-nomarker-1280.png`。

### A-3 标记单步交互不展开文章【未覆盖】
- 未覆盖原因：真实 suggested 建议属用户待处理项，确认/不采纳均不可原路还原，验收不做破坏性实点（见「数据安全」节）。交互语义由组件/宿主回归用例兜底（`pnpm test:unit KeyEventMarkerActions.test.ts DailyReportTopicSection.test.ts` 44/44 绿，含 busy 互斥、error 局部反馈、键盘激活断言）。本 change 未触碰任何交互脚本逻辑（review 已核实 `<script>` 零逻辑变更），布局变更不改变点击语义。

### A-4 窄屏 ≤720px 单列回退【pass】
- 依据：viewport 640×900，suggested 行 `grid-template-columns` 收敛为单列（574.4px），header/detail/actions 三块均全宽（fullW=true）、纵向堆叠（detail 在标题下方、动作组在 detail 下方）、动作组仍横排右对齐；截图 `evidence/narrow-suggested-640.png`。
- review R-2 处置后复测：无 detail 行窄屏 header→动作组间距 7px（修复前推演 14px 双重 row-gap），实测 `gapBelowHdr: 7`，无空轨道。

### A-5 状态覆盖与守卫【pass】
- 覆盖状态：suggested（真实 ×2）、dismissed（board 1974 thread 14170）、none（×4）；confirmed 无独立真实样本，但其渲染分支与 none 共用「无 detail + 动作组底行右下」路径（组件测试「hides the detail block when there is no detail content」断言 confirmed 无 detail、actions 存在，44/44 绿）。
- 源码守卫（防 scope 断链复发）：组件测试断言摆放规则在子组件 scoped 样式（`grid-column: 2`、`justify-content: flex-end`、`max-width: 720px` 断点字面量两文件同步）、宿主源码不含 `.drm-thread-row > .ke-detail/.ke-actions` 死选择器——守卫用例随 2.2 跑绿。

## 终态

- 两轮部署后线上构建均为本 change 产物；viewport 已还原 1280×800；无任何标记数据被本验收改动（board 1974 suggested×3/dismissed×1、board 2272 suggested×7 与开工时一致）。
- 溢出检查：1280/640 两档 `document.scrollWidth` 均等于视口宽，无整页横向溢出。
