# UI 验收记录：fix-daily-report-highlights-null

> minor 档（`ui-impact: minor`）。验收对象：刊头「今日重点」空态占位。
> 组件测试映射：`front/app/features/tags/components/daily-report/DailyReportMasthead.test.ts`（文件在盘）。

### A-1 空态占位在空 highlights 时可见且不渲染要闻条目【pass】
- **依据**：`cd front && pnpm test:unit app/features/tags/components/daily-report/DailyReportMasthead.test.ts --maxWorkers=2` → `3 tests passed`；用例 `renders empty state when highlights empty` 断言 `[data-testid="highlights-empty"]` 存在且文案含「本期暂无重点」、`.drm-highlight` 数量为 0。

### A-2 highlights 为 null 时渲染占位且不抛错【pass】
- **依据**：同上命令；用例 `renders empty state when highlights null` 断言占位存在、`.drm-highlight` 数量为 0，无未捕获异常。

### A-3 正常态渲染要闻且无空态占位（零视觉变化）【pass】
- **依据**：同上命令；用例 `renders highlights without empty state` 断言 3 个 `.drm-highlight`、`[data-testid="highlights-empty"]` 不存在、`#report-highlights` 唯一。
- **补充**：`git diff front/app/features/tags/components/daily-report/DailyReportMasthead.vue` 显示正常态 `<section id="report-highlights">` 模板与 `.drm-highlight*` 样式**逐字未改**，本次仅新增 `v-else` 空态分支与 `--empty` 修饰类。

### A-4 空态与正常态互斥（id 不重复）【pass】
- **依据**：同上命令；两条空态用例均断言 `wrapper.findAll('#report-highlights')` 长度为 1（`v-if` / `v-else` 互斥渲染，任一时刻只有一个分支存在）。

### A-5 浏览器视觉验收（空态占位实际呈现 / 与刊头其它内容不重叠 / 正常日报零变化）【pass】
- **核对方法**（2026-11-04 部署后实测，静态托管形态 `:5100` 同源，agent-browser 驱动真实浏览器）：
  - 坏日报 `http://127.0.0.1:5100/tags/1980/2026-11-03`（`highlights` 为历史 null 行）→ DOM 实测：`[data-testid="highlights-empty"]` **存在**，文案恰为「本期暂无重点」；`.drm-highlight` 数量 = **0**；`.drm-highlights` 区块数 = **1**（id 不重复）；刊头头条仍在（lead 标题正常渲染）。
  - 正常日报 `http://127.0.0.1:5100/tags/1980/2026-11-02` → DOM 实测：3 条要闻标题正常渲染（`OpenAI 合并 ChatGPT 与 Codex 额度` / `阿里云发布 AgentCore 平台` / `香港证监会公布首份 AI 调查`）；**无**空态占位；`.drm-highlights` 区块数 = 1。
- **截图**：`shot-empty-state.png`（坏日报空态占位）、`shot-normal-highlights.png`（正常日报 3 条要闻）。
- **结论**：空态「可见而非消失」达成；正常态零变化达成；id 互斥无重复达成。深色模式对比度已由 review R-11 处置（占位文字色改 `--color-text-secondary`，浅色 ≈6.9:1 / 深色 ≈7:1）。
