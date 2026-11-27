<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A Reason

本 change 只改后端「今日重点」的输出顺序，前端零改动：刊头（`DailyReportMasthead.vue`）与头条选取（`dailyReportMagazine.ts` 的 `selectLeadStory`）本就按接口返回的 `highlights` 数组顺序渲染，既有契约「highlights 按 API 顺序最多展示三项、头条优先取 `highlights[0]`」（`openspec/specs/daily-report-interface/spec.md`「日报时间线组件」）不变。依据 proposal 的 `ui-impact: none` 声明：不新增页面、面板、字段或交互，用户看到的只是同一组卡片换了顺序。
