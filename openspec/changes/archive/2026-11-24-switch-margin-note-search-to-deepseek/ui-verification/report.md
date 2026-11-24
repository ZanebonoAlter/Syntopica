# UI 验收 — switch-margin-note-search-to-deepseek（ui-impact: minor）

- 验收方式：浏览器实走（agent-browser，本机 `http://localhost:5100`，静态产物 2026-11-24 10:53 部署）+ 组件测试
- 环境：后端已重启到新代码（`/health` 200）；`GET /api/settings/web-search/health` 返回 `ok`（DeepSeek 搜索可达）
- 证据截图：`settings-web-search.png`（本目录）

### A-1 设置 tab：网页搜索存在、SearXNG 消失【pass】

- **依据**：`agent-browser open http://localhost:5100/settings?section=datasources-network` 快照 tabs 为「Firecrawl / 博查 / 网页搜索 / RSSHub」，无「SearXNG」；分区描述文案为「Firecrawl、博查、网页搜索、RSSHub、研究数据源与出站代理配置」。


### A-2 旧深链 `tab=searxng` 重定向【pass】

- **依据**：打开 `?section=datasources-network&tab=searxng`，2s 后地址变为 `?section=datasources-network&tab=web-search`，「网页搜索」tab 为 selected，面板正常渲染「已关联供应商：DeepSeek」。


### A-3 更老旧键 `?section=searxng` 重定向【pass】

- **依据**：打开 `?section=searxng`，地址变为 `?section=datasources-network&tab=web-search`，「网页搜索」tab selected。


### A-4 负向：非本分区携带 `tab=searxng` 不触发重定向【pass】

- **依据**：打开 `?section=ai-providers&tab=searxng`，地址保持不变（未被改写为 web-search），说明 tab 级重定向只作用于「数据源与网络」分区。


### A-5 「测试连接」四态徽标 + 耗时【pass】

- **依据**：点击「测试连接」后按钮进入加载态，随后显示「连接正常 · 394ms」与次级说明「搜索正常，返回 10 条结果」。


### A-6 面板无可编辑字段（D6）【pass】

- **依据**：快照中面板只有 heading「网页搜索」、静态文案、「测试连接」按钮；组件测试另有守卫断言（`find('input')` / `find('textarea')` / `find('form')` / `AppToggle` 均不存在）。


### A-7 页边注卡三态标记（已联网 / 已联网 · 未采用 / 联网搜索无结果）【pass】

- **依据**：组件测试 `front/app/features/tags/components/daily-report/MarginNoteCard.test.ts` 覆盖三态与 `disabled`/NULL 不渲染（13 用例全绿）。
- **依据（20:03 补测，模型主机恢复后）**：`POST /api/annotations/26/questions` 产出 `web_search_status=ok` + 1 条网络来源（CRI 国际在线），打开 `/tags/2272/2026-11-06` 展开页边注栏，卡片渲染「已联网」标记与「网络来源」外链；截图 `margin-note-web-status.png`。

### 附：过程中发现的环境问题（不属本 change）

- `10.11.12.111:8080/8081`（ZeroTier 网段内的 LLM + embedding 主机）从本机 100% 丢包 → 所有 AI 能力（summary / topic_tagging / open_notebook / embedding）当前失败，`ai_call_logs` 记 `network_error`。用户已知悉并计划晚间处理。
- 2026-11-24 10:08 前端构建期间整机 reboot（构建未完成、未上线）。已把构建限载固化进 `scripts/dev/deploy-frontend.sh`（nice 19 + Node 堆上限 + 看门狗）并写入 `docs/reference/deployment.md`。
