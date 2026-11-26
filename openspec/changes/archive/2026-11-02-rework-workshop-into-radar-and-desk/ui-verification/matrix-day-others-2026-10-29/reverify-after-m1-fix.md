# 8C.7 M-1 收口复验（新产物 `2ba13652…` / `BBjtIkfj.js`）

> 线程：全新验收线程（implementer 档 + agent-browser 0.37.1），只读验收。**未改产品代码、未跑 pnpm/go、未碰数据库、未 commit、未勾 tasks。**写域仅本目录（`reverify-after-m1-fix.md` + `shots-reverify/`）。
> 任务：按 `matrix-day-others-8c7-reverify.md` 在**新产物 hash** 上复验独立评审 M-1 的关键断言（S11-A 主链 + M1/L1 + 1974 冒烟），不重跑全量 S11-A/B/C。
> 验收时间：2026-10-30 00:26–00:34 +08（2026-10-29 16:26–16:34Z）。
> 结论先行：**新产物复验通过**。11 条复验项全绿，零业务写（仅 2 次既有自动埋点 POST）。

## 1. 产物指纹（新 hash 一致）

| 项 | 实测值 |
| --- | --- |
| 前端静态根 | `backend-go/frontend/`（`:5100` 同源 API） |
| `backend-go/frontend/index.html` sha256 | `2ba136527001200c52ef7a35f5b6d07728c9cf140a7a718780c9af72ba80f251`（前 16 `2ba136527001200c`，7457 B，mtime 2026-10-30 00:24:03 +08） |
| `GET http://localhost:5100/` sha256 | 同上 `2ba13652…`（200，7457 B）→ 服务端确为新 index |
| index 入口引用 | `src="/_nuxt/BBjtIkfj.js"`（全页唯一 `src`），样式 `entry.DXW84SCK.css` |
| 入口 chunk `_nuxt/BBjtIkfj.js` | 盘上 sha256 `1923ef9667288df5f5ba8d5a484eae9496eda244528af7bbe5abac85cba55de8`（349388 B） |
| `GET /_nuxt/BBjtIkfj.js` sha256 | 同上（200，349388 B）→ 服务端与盘上一致 |
| 页面实载资源 | `performance` 资源条目含 `BBjtIkfj.js` 与 `UsywbxZ1.js`（后者含新窄屏文案 chunk） |
| 旧产物痕迹 | 构建产物（`_nuxt/*.js`）**零**命中 `console-no-lanes-fallback`、`最新日报唯一入口在标题区`、`暂无活跃泳道（0 正式泳道）` |
| 后端 | 监听 `:5100` 的 PID 16270，started 2026-10-29 22:05:08 +08（**未重启**）；本线程只发 GET（见 §4） |
| 浏览器 | agent-browser 0.37.1，Chromium headless；`no_proxy=localhost,127.0.0.1,::1` 生效 |

## 2. 逐条断言（期望 / 实际 / 结果）

### 2.1 S11-A 主链（246030 卫生健康，1440×900 editorial 为主）

| # | 断言 | 期望 | 实际 | 结果 |
| --- | --- | --- | --- | --- |
| 1 | 产物指纹 | index `2ba13652…` / entry `BBjtIkfj.js` | 盘上=服务端=页面实载，sha256 逐字节一致（§1） | PASS |
| 2 | 默认 14 档；列头可点集合 = 现场报日索引 | 14 列头；可点 = 窗内真实报日 | 14 个 `matrix-dayhead-*`；BUTTON 仅 `2026-10-16/22/23/24/29`（5 个）= 现场 API `daily-reports?days=365` 在 `10-16…10-29` 窗内的报日 `[10-29,10-24,10-23,10-22,10-16]`；其余 9 日为 DIV；默认选中 `10-29`（最新报日） | PASS |
| 3 | **M1**：0 正式宽屏无旧横幅、网格内联说明唯一 | `console-no-lanes-fallback` DOM 不存在；内联「0 正式泳道」唯一 | `document.querySelector('[data-testid="console-no-lanes-fallback"]') === null`；`matrix-empty-no-lanes` 在场且 `innerText =「0 正式泳道 · 已展开当日其他话题」`；`document.body.innerText` 中「0 正式泳道」出现次数 = 1；旧横幅原文（`这个版块还没有活跃泳道` / `最新日报唯一入口在标题区`）全页零命中；`matrix-grid` / `other-topics-group` 同屏在场 | PASS |
| 4 | 0 正式组默认展开、行数 = 该日详情 sections − 泳道可读 | `aria-expanded=true`；10-29 行数 1−0=1 | `other-topics-toggle` 内 button `aria-expanded="true"`、摘要「当日其他话题 · 1 / 只标所选日」；DOM 1 行 `other-topic-row-7509`；API 详情 report 1290 sections=1（sid 7509），`lane-dynamics?days=365` lanes=[] → 1−0=1。另核 10-16：详情 sections=1（sid 6779，threads=2）、0 泳道 → 组仍 1 行（行数按 section 计、格计数=2） | PASS |
| 5 | 点首行 → 面板完整标题 + 全量线索；摘要逐字 | 面板标题=cluster_label；线索数=threads 数；摘要与 API 逐字相等且不裁切 | 点 `other-topic-row-7509`：面板 `h3` =「韩国财长表态将在市场波动加剧时立即应对」= API `cluster_label`（`===` 为 true，`title` 属性同文）；线索 1/1 = API `sections[7509].threads.length`；展开 `lane-chronicle-thread-15930`：摘要 `innerText` 与 API `summary` `===` true（160 字符），`scrollHeight(67) <= clientHeight(67)+1`（未被硬裁） | PASS |
| 6 | 相关文章 → 站内阅读 → 关闭焦点回落 | 相关文章按需加载；点开进站内阅读；关闭后面板仍在、焦点回触发元素 | 「相关文章 · 1 篇」→ 加载出 `lane-chronicle-article-158863`（= API `related_article_ids`）；点击后 `[role=dialog]` 打开、正文非空（「文章预览 / 华尔街见闻 … 韩国财长：若市场波动加剧，将立即采取应对措施。」）；**Escape 关闭**与**点 ✕ 按钮关闭**两条路径均：dialog 数 0、面板标题仍在、摘要仍展开、`document.activeElement.dataset.testid = lane-chronicle-article-158863` | PASS |
| 7 | 打开日报 → 返回矩阵仍停该报日 | 返回后 selectedDate 不丢 | 面板「打开 2026-10-29 日报 ↗」→ URL `/tags/246030/2026-10-29`；日报页「返回版块矩阵」（button，可见文案「矩阵」，`aria-label="返回版块矩阵"`）→ URL `/tags/246030?days=14`，`matrix-dayhead-2026-10-29[aria-pressed=true]` 唯一选中、其他组仍展开计数 1、面板标题仍为 7509 标题、`matrix-grid` 在场；URL 无 `date=` | PASS |
| 8 | **L1** 窄屏 375/390 文案与条 | 行内文案 =「0 正式泳道——当日内容见下方「当日其他话题」。」；无「暂无活跃泳道」；报日条同源、选中日滚入、无页面横溢 | 375×667 与 390×844（均 editorial）：`narrow-no-lanes-note.innerText` **逐字等于**「0 正式泳道——当日内容见下方「当日其他话题」。」（`===` true）；全页 `暂无活跃泳道` 零命中；`narrow-day-strip-track` 在场，5 个 `narrow-dayhead-*` = `[10-16,10-22,10-23,10-24,10-29]`（与宽屏/API 同源），默认选中 `10-29` 且完全落在条可视矩形内（375 `scrollLeft=323`、390 `308`）；375 点最早 `10-16` → `scrollLeft 323→0`、选中日仍在可视区；两次测量 `documentElement.scrollWidth = 375/390 = innerWidth`（无页面横溢） | PASS |
| 9 | dark 主题抽查（1440 + 390） | `data-theme=dark` 下断言同源 | 1440×900：`data-theme=dark`、body 背景 `rgb(8,12,18)`、14 列头、默认选中 10-29、无旧横幅、内联说明「0 正式泳道 · 已展开当日其他话题」、组展开 1 行、无横溢；390×844：窄屏文案逐字一致、无「暂无活跃泳道」、5 报日条 + 选中 10-29 滚入、其他组在场、无横溢 | PASS |

### 2.2 1974 冒烟（中东，1440×900 editorial）

| # | 断言 | 期望 | 实际 | 结果 |
| --- | --- | --- | --- | --- |
| 10 | 组默认收起 + 计数可见；正式行/休眠组独立在场 | 收起、真实计数；正式 5 行 + 休眠组独立 | 14 个 `matrix-dayhead-*` 全 BUTTON（14 天均有报日）；`other-topics-toggle` button `aria-expanded="false"`、计数「· 0」、note「当日分区都已在泳道内可读」、DOM 0 行；正式泳道行 `[matrix-lane-1204, -2, -1844, -9, -5]`（5 行）= 现场 `lane-dynamics` 名单；`dormant-toggle` 独立在场「休眠泳道 · 1 / 展开查看…」且可展开 | PASS |
| 11 | 展开组点一个多行报日：行标题=cluster_label、计数=threads.length（对照现场 API） | 逐行逐字 + 计数 | 展开组 → 点 `matrix-dayhead-2026-10-27`：组计数「· 4」，4 行 `other-topic-row-7344/7345/7346/7347`；以现场 `GET /api/daily-reports/1260` 逐行程序化比对（排除当日泳道可读 6 个 lane topicId，按 `cluster_index` 升序）：**4/4 行 label `===` API `cluster_label`**，格计数 `[2,1,2,1]` **=== API `threads.length`**（7344:2、7345:1、7346:2、7347:1）；组计数 4 = 行数 | PASS |

## 3. 证据截图（`shots-reverify/`，12 张）

| 文件 | 断言组 | 尺寸 / 主题机械核验 |
| --- | --- | --- |
| `reverify-1-s11a-default-1440-editorial.png` | #2/#3/#4 默认 0 正式 | 1440×900，右下角 `rgb(250,247,242)` |
| `reverify-2-s11a-reading-panel-1440-editorial.png` | #5 阅读面板 + 摘要逐字 | 1440×900，`rgb(250,247,242)` |
| `reverify-3-s11a-article-dialog-1440-editorial.png` | #6 站内阅读 dialog | 1440×900，角像素 `rgb(160,158,155)` = 遮挡层压暗的 editorial 底（非主题错误） |
| `reverify-4-s11a-return-date-retained-1440-editorial.png` | #7 返回仍停 10-29 | 1440×900，`rgb(250,247,242)` |
| `reverify-4b-s11a-otherday-1016-1440-editorial.png` | #4 补充：10-16 组 1 行 / 格计数 2 | 1440×900，`rgb(250,247,242)` |
| `reverify-5-s11a-narrow-375-editorial.png` | #8 375 默认 + L1 文案 | 375×667，`rgb(250,247,242)` |
| `reverify-5b-s11a-narrow-375-selected-scroll.png` | #8 375 选 10-16 条滚入 | 375×667，`rgb(250,247,242)` |
| `reverify-6-s11a-narrow-390-editorial.png` | #8 390 默认 + L1 文案 | 390×844，`rgb(250,247,242)` |
| `reverify-7-s11a-default-1440-dark.png` | #9 dark 1440 | 1440×900，`rgb(8,12,18)` |
| `reverify-8-s11a-narrow-390-dark.png` | #9 dark 390 | 390×844，`rgb(8,12,18)` |
| `reverify-9-s11b-default-1440-editorial.png` | #10 1974 默认收起 | 1440×900，`rgb(250,247,242)` |
| `reverify-10-s11b-expanded-multirow-1440-editorial.png` | #11 1974 10-27 四行 | 1440×900，`rgb(250,247,242)` |

## 4. 零写核对

| 检查 | 结果 |
| --- | --- |
| 网络方法统计（agent-browser 会话缓冲，全程） | 共 **1101** 请求：GET **1099** / POST **2** |
| 非 GET 明细 | 2 次均为 `POST /api/reading-behavior/track-batch → 200`（两次「相关文章 → 站内阅读」自动埋点，应用既有行为；brief 明示可忽略） |
| 业务写（生成/修复/确认/收藏/已读/归档写/合并/转正） | **0**（按 URL 过滤 `POST/PUT/PATCH/DELETE` 且排除 track-batch，命中 0） |
| 导航类动作 | 列头/条/行/节点/打开日报/返回/切视口/切主题/展开折叠，均为只读 GET 或纯前端状态 |
| URL 新增 `date=` | 0（返回态 `/tags/246030?days=14`；期间 `/tags/246030/2026-10-29` 为日报页路径，无 query 新增） |
| 数据库 / 产品代码 / tasks | 未触碰（本线程仅新增本报告与 `shots-reverify/`） |

## 5. 观察与边界（不构成失败）

1. **390 窄屏报日条首帧短暂 4 个、随后 5 个**：加载后约 <2s 内 `narrow-dayhead-*` 从 4 个（缺最早的 10-16）补齐到 5 个，再测/截图（settle 后）为 5 个，与宽屏/API 同源。属报日索引异步加载的中间态，不是本期改动引入，也未造成选中日/组行错误；建议后续如需首帧完整可在索引就绪前给条占位（非本 brief 范围）。
2. **站内阅读 dialog 的可访问名**：本轮 dialog 标题为「文章预览」，旧报告表述为「站内原文阅读」；同一 dialog（`role=dialog`、正文为站内原文、关闭焦点回落），非回归。
3. **线索展开态不跨「打开日报→返回」保留**：返回后日期/分区/线索列表恢复，已展开线索回到收起（与旧报告 L3 一致，非本期改动；不在本 brief 断言内）。
4. 未重跑 S11-A/B/C 全量，未做像素 diff、未触写类动作用例、未验 C4（缺身份观察带真库样本不足，旧报告已留账）。
5. 本线程为只读验收：未改产品代码、未跑 pnpm/go、未碰数据库、未 commit、未勾 `tasks.md`。浏览器会话已关闭（agent-browser `close`），临时请求转储已删除。

## 6. 结论

- **新产物复验通过**：M-1 修复涉及的三件事在新 hash 上均成立——①旧 0 正式横幅 DOM/产物字符串彻底消失；②新网格内联说明 + 默认展开其他组 + 报日列头链路完整；③窄屏文案改为纯「0 正式泳道——…」，且宽屏/窄屏均无「暂无活跃泳道」字样。
- S11-A 主链（列头=实测报日、0 正式组默认展开行数、阅读面板标题/全量线索/摘要逐字、相关文章→站内阅读→焦点回落、打开日报返回仍停该报日）与 L1/dark/1974 冒烟（默认收起+计数、多行报日 label/计数对现场 API）逐条 PASS。
- 零业务写；**无需回滚**，可据此关闭 M-1 评审项（tasks 由父勾）。
