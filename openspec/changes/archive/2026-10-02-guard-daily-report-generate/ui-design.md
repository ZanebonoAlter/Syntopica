<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A Reason

本 change 为纯后端并发防护（`POST /api/daily-reports/generate` 防重入闸 + 调度作业共享闸），不新增任何界面结构或交互模式。被拒时的 409 提示走 `DailyReportGenerateDialog.vue` / `LaneDynamicsPanel.vue` 既有 `success:false` 错误 toast 通道，文案由后端 message 透出，前端零改动。与 proposal 的 `ui-impact: none` 声明一致。
