
## 日报时刻保存 404 根因

SettingsSectionSchedulers.vue（d96e1116 引入）GET/POST 调 `/api/settings`，后端实际注册在 `/api/ai/settings`（backend-go/internal/admin/routes.go:22-23，rg=/api + ai group=/ai）。实测 GET /api/settings → 404、GET /api/ai/settings → 200（当前库值 daily_report_time="21:00"）。症状：页面打开静默显示默认值非真实配置；保存必报「保存失败」。后端链路健全：SaveSettings→SaveDailyReportTimeConfig（HH:MM 校验 upsert ai_settings），调度器 dailyReportWindow 60s 现读配置即时生效。修复=前端 SettingsSectionSchedulers.vue 两处 URL 改 /api/ai/settings。

<!-- pinned 2026-09-26T15:02:03Z -->

## 方法卡零使用实证与删除范围

analysis_methods 表实测（2026-10 会话）：total=1、enabled=0、legacy=1，唯一一条 id=1 name=inside-america-v2「内部看美国·方法论画像（v2）」，summary 自述"从旧参考角色迁移；需补齐适用边界并人工启用"。用户从未建卡/启用，停用卡永不被调查链选中 → 方法卡注入自上线以来零次发生，删除无行为损失。删除范围：前端 SettingsSectionAnalysisMethods.vue + AnalysisMethodPanel.vue(573行) + api/analysisMethods.ts；后端 analysis_method_handler.go(整删) + analysis_methods.go(整删) + method_sanitizer.go(整删) + board_investigation.go(36处选卡逻辑) + board_investigation_synthesis.go(8处注入段) + signal_research.go(注入段) + repository.go/models.go(20+10处) + handler.go 路由(6处)；10 个测试文件适配；data-enrichment spec 104 行「方法卡自动选择仅适用于 board_investigation」句 delta 移除；DB drop analysis_methods 表。

<!-- pinned 2026-09-26T15:06:04Z -->
