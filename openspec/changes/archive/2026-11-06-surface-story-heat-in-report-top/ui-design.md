<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# UI 复用契约

## 入口与变更

- 既有版块态势矩阵：仅改 `ObservationMatrix.vue` 图例、`observationLanes.ts`/`ObservationMatrixGrid.vue` 的 title/aria，点名颜色对应事件线，不改格内数字/点击目的地。
- 既有日报阅读页：`DailyReportMasthead.vue` 今日重点标题旁显示「进展」，附「N 个来源」；标题文字按钮触发本 report 正文 thread 锚点滚动高亮。首条仍驱动报告标题摘要；最多三条，不新增重要度等级。
- 无新路由、弹层、分区或额外交互步骤。不做事件线详情页。无需 UI 原型。

## 受影响状态

| 状态 | 矩阵 | 今日重点 |
|---|---|---|
| 成功 | title/aria 同时点名，图例用明确措辞 | 来源数可验证；昨日同线标进展；定位首条正文 thread，高亮 2s |
| 缺身份/旧响应 | 保留格值颜色与未回填说明，提示最热线暂不可识别 | 旧重点仅标题理由，不伪造来源、进展或按钮 |
| 空态 | 无分区断档不可点，有分区零值可点 | 继续可见「本期暂无重点」，无新空态交互 |
| 加载/错误 | 保持现有骨架、错误条及 stale 提示 | 保持既有报告加载错误反馈；无正文目标时禁用定位并显示「本期正文暂无对应事件」 |
| 不完整证据 | 既有提示不变 | 来源快照热度 title/aria 带不完整说明；0 不等于缺失 |
| 长文/窄屏 | 长标题放提示/aria，格列宽不变 | 标题、来源/徽标换行，不裁断按钮可访问名称 |

## 复用组件与布局模式

- 矩阵保留 `workspace` 的 BoardConsoleShell/Stage，日报保留既有阅读台宽屏正文与窄屏布局，不调整栏宽或 shell 模式，不新增组件。
- `DailyReportReader.vue` 承接刊头定位事件；`DailyReportTopicSection.vue` 两条线程渲染分支增加 `report-thread-{id}`。复用既有展开、selection guard、轻提示与主题 token，禁止硬编码新色值。
- 按钮支持 Tab/Enter/Space；thread 仅程序聚焦（tabindex=-1），保留焦点轮廓；reduced-motion 不平滑滚动；report 切换清理 2s 计时器与高亮。
- 目标检查：1440×900、1920×1080，另检 390×844 换行不溢出。沿用既有布局，无新 dialog 尺寸档。

## 验收映射

| 验收点 | 落点 |
|---|---|
| 图例、title/aria 点名/空标题/未回填 | `front/app/features/tags/components/console/ObservationMatrix.test.ts`、`front/app/features/tags/components/console/ObservationMatrixGrid.test.ts`、`front/app/features/tags/components/console/observationLanes.test.ts` |
| 进展、来源数、旧重点、空态 | `front/app/features/tags/components/daily-report/DailyReportMasthead.test.ts` |
| 首条线程落点、两条渲染分支、展开/焦点/2s 清理 | `front/app/features/tags/components/DailyReportReader.test.ts`、`front/app/features/tags/components/daily-report/DailyReportTopicSection.test.ts` |
| 完整故事及双桌面视口/窄屏 | 人工：实施后用 agent-browser 访问静态 :5100，从矩阵提示→日报重点→正文定位，检查实际 scroll/focus/单高亮与缺目标降级，证据建账本 change `ui-verification/report.md`；本阶段未执行，不预写通过 |
