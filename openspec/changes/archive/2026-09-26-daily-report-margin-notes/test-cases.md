# Test Cases: 日报页边注（daily-report-margin-notes）

> 规划用例，未执行。repository 层 testcontainer PG（禁 SQLite）；service 层 stub LLM；handler 层 HTTP；前端 vitest（maxWorkers=2）+ opencli/agent-browser 主链路。所有期望对齐 specs/daily-report-margin-notes/spec.md 与 ai-capability-routing delta，不以 mock 成功替代真实链路断言。

## 主链路故事（读者小李读「中国宏观」日报遇到逆回购）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 在 thread 摘要划选「以利率招标方式开展 3000 亿元 7 天期逆回购操作」 | 划选落锚 | 「问一问」气泡出现，thread 展开状态不变 | 前端 | SelectionAskBubble + guard vitest |
| 2 | 点击气泡 | 划选落锚 | 正文高亮 + 右栏新锚点卡 + 输入聚焦 | 前端 | MarginNoteCard vitest |
| 3 | 输入「逆回购是什么意思」提交 | 提问获得带引用回答 | loading 骨架→回答含通用概念+当天语境，引用 chips 可展开 3 篇 | 后端+前端 | QA service stub LLM / handler |
| 4 | 点引用 chip | 提问获得带引用回答 | 走 ensureArticles 打开文章预览（与 thread 溯源同链路） | 前端 | 手验/opencli |
| 5 | 追问「那到期不续做会怎样」 | 追问 | 新 QA 轮追加同卡，历史轮保留 | 后端+前端 | handler + MarginNoteCard vitest |
| 6 | 完成问答 | 问答后术语自动入库 | 「逆回购」「流动性对冲」入 term_notes，卡尾 chips 带入库角标 | 后端 | term upsert 单测 |
| 7 | 次日重开同份日报 | 批注跨会话持久 | 高亮与锚点卡（含问答）均显示 | 后端 | repository PG |
| 8 | 点锚点卡引用摘要 | 锚点双向跳转 | 滚动到高亮并闪现；反向点高亮点亮卡片 | 前端 | vitest |
| 9 | 删除批注并确认 | 删除批注连带问答 | 确认弹窗明示连带删除；确认后问答+高亮全清 | 前端+后端 | AppDialog + repository |
| 10 | 删光后看右栏 | 空态提示 | 灰色轻提示，无卡片容器边框 | 前端 | vitest |
| 11 | 视口缩到 <1100px | 窄屏抽屉形态 | 右栏收起，右下浮动入口含计数，抽屉内操作一致 | 前端 | vitest/手验 |
| 12 | 打开设置「页边注」分区 | 列表与筛选 | 跨报告全部批注按日期倒序；版块/关键词筛选即过滤 | 前端+后端 | SettingsSectionMarginNotes vitest |
| 13 | 管理页点「原日报」 | 跳转原日报定位 | 打开日报层对应报告，定位锚点高亮闪现 | 前端 | 手验/opencli |
| 14 | 管理页删除一条 | 管理页删除与日报内同源 | 行移除计数刷新；日报内锚点卡与高亮同步消失 | 前后端 | vitest + 手验 |

## 现有交互零回归（红线，来源 spec「现有收展与溯源交互零回归」）

| ID | 输入/分支 | 预期 |
| --- | --- | --- |
| RG-1 | 无选区纯点 thread 行头 | 相关文章列表展开/收起，与改动前一致 |
| RG-2 | thread 摘要内划选完成（mouseup 在行头内） | 气泡出现，toggle 不触发 |
| RG-3 | 划选后清选区，再点行头 | 正常展开 |
| RG-4 | 点正文高亮 mark | 跳右栏对应卡（窄屏开抽屉定位），thread 状态不变 |
| RG-5 | topic 行头点击收展、active zone ensureLifeline | 行为不变 |
| RG-6 | 无 related_article_ids 的 thread 行头 | 仍为 disabled 不可点 |

## 锚点卡内交互（引用展开/跳转/删除，来源 spec「划段批注锚定」「引用展开与双向跳转」）

| ID | 输入/分支 | 预期 |
| --- | --- | --- |
| MC-1 | 长划词默认展示 | 引用摘要两行截断，尾部「展开」可点 |
| MC-2 | 点引用摘要 | 全文展开↔收起切换，不触发跳转 |
| MC-3 | 点「↗ 跳回原文」 | 滚动到高亮并闪现；不触发展开/收起 |
| MC-4 | ✕ 删除按钮 | 常显（低调），点击弹确认；取消恢复 |

## 管理页（MG，来源 spec「全局批注管理」）

| ID | 输入/分支 | 预期 |
| --- | --- | --- |
| MG-1 | 全量列表 | 按日期倒序，含仅标记未提问行（显示「尚无提问」态） |
| MG-2 | 版块筛选/关键词搜索（命中划词/提问/术语） | 即时过滤；未知组合清空恢复 |
| MG-3 | 无匹配 | 空态提示，清筛选恢复 |
| MG-4 | 问答折叠展开 | details 展开显示全部轮次 |
| MG-5 | 跳原日报定位 | 打开对应报告并定位锚点闪现 |
| MG-6 | 管理页删除 | 与日报内同源（同一端点），确认后行移除+计数刷新+日报内同步消失 |
| MG-7 | 深链 `?section=margin-notes` | 直达分区，非法 section 回退默认（现有行为） |

## 变体走查

| 组 | ID | 输入/分支 | 预期 |
| --- | --- | --- | --- |
| 输入 | IN-1 | quoted_text 空串/纯空白/单字符 | 前端不弹气泡；直发 API 400 |
| 输入 | IN-2 | quoted_text 超长（>1000 runes） | 前端截断提示/后端 400（按实现定，spec 只要求拒绝） |
| 输入 | IN-3 | question 空白提交 | 前端拦截聚焦，不发请求 |
| 输入 | IN-4 | question 超长 | 后端 400，不调 LLM |
| 前置 | PR-1 | thread 无关联文章时提问 | 回答正常，标注「纯模型知识、无当天文章引用」，cited 为 `[]` |
| 前置 | PR-2 | annotation_id / report_id 不存在 | 404，无 LLM 调用 |
| 前置 | PR-3 | quoted_text 与 thread 摘要偏移失配（日报重跑后） | 模糊匹配兜底；全失配卡片显示「原文已变更」，不报错 |
| 前置 | PR-4 | lead 摘要批注（无 thread） | thread_id 为 NULL 正常落锚提问 |
| 时间窗口 | TW-1 | 对历史旧日报（>7 天）提问 | 不受重建窗口约束，AI 上下文取文章内容正常作答 |
| 幂等 | ID-1 | 同段文字重复落锚两次 | 两条独立批注（P1 允许，不合并） |
| 幂等 | ID-2 | 提问按钮 loading 中重复点击/双击 | 输入锁定，只发一次请求 |
| 幂等 | ID-3 | 删除确认后取消 | 卡片恢复，数据未动 |
| 可用性 | AV-1 | LLM 调用失败 | 行内错误+重试按钮，问题文本保留 |
| 可用性 | AV-2 | 超长 quoted_text 渲染 | 卡片引用摘要 2 行截断不破版 |
| 可用性 | AV-3 | 进入报告批注列表加载中 | 右栏骨架行，不闪空态 |
| 可用性 | AV-4 | open_notebook 无路由配置 | 报错路径与现有能力一致（airouter 降级链），行内可重试 |

## 白盒附加（复杂档）

| ID | 分支 | 预期 |
| --- | --- | --- |
| WB-1 | 选区 <2 字符 / 空选区 / 气泡自身点击 | guard 不吞、气泡不弹 |
| WB-2 | 跨节点选区 surroundContents 抛错 | 降级只落卡不高亮，不中断 |
| WB-3 | LLM cited 返回集外 id / 空 / 全合法 | 集外剔除、空合法（纯模型知识）、合法全保留 |
| WB-4 | LLM terms 解析失败 / >5 个 / 重复 | 截前 5、失败 Warn 不阻断 answer 返回 |
| WB-5 | 术语归一化：全角/半角、大小写、首尾空白 | 归一后同词命中既有条目 hit_count+1，不新建 |
| WB-6 | terms 含空串/纯分隔符 | 过滤不入库 |
| WB-7 | qa 写入 cited `null` 标量 | 拒绝（数组契约：空写 `[]`） |

## 联网搜索扩充（SearXNG，来源 spec「页边注问答与引用」增补条款）

| ID | 输入/分支 | 预期 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| WS-1 | 提问时 SearXNG 返回相关结果 | prompt 含联网参考块（≤5 条 title/url/content）；回答带 web_sources 白名单校验后落库 | 后端 | searxng httptest + QA service stub |
| WS-2 | SearXNG 超时/非 200/坏 JSON/未配置/禁用 | 静默降级：回答照常、无网络来源行、无错误提示（仅 Warn 日志） | 后端 | QA service stub 单测 |
| WS-3 | LLM 返回结果集外 URL / 合法 URL 混合 | 集外剔除、保序去重、≤5；全剔除=无网络来源行 | 后端 | QA service stub 单测 |
| WS-4 | 有网络来源无文章引用 / 两者皆空 | 前者不标纯模型知识；后者标注 | 前端 | MarginNoteCard vitest |
| WS-5 | cited_web_sources 写入 `[]` / `[{title,url}]` | 空写 `[]` 不写 null；保序去重 | 后端 | repository testcontainer PG |
| WS-6 | 设置页改 endpoint 后再提问 | 即时生效（现读，不重启）；enabled=false 静默降级 | 后端+前端 | config_store 单测 + 组件测试 |
| WS-7 | 点网络来源 chip | 新窗口打开原网页（target=_blank rel=noopener） | 前端 | vitest + 真机 |
| WS-8 | quoted_text 超长（>80 runes）/为空 | 搜索 query 截 80 runes / 回退问题文本 | 后端 | QA service stub 断言搜索入参 |
| WS-9 | migration 重复执行 | ADD COLUMN IF NOT EXISTS 幂等 | 后端 | 隔离 PG 库跑两遍 |

## 效果核对（依赖断言外因素）

| ID | 触发原因 | 方法 | 量化 | 结论 |
| --- | --- | --- | --- | --- |
| EF-1 | LLM 幻觉引用不可静态断言 | 真库跑 ≥10 轮真实提问，统计 cited 集外剔除次数 | 剔除率>0 且回答仍可用 | 白名单有效且不伤体验 |
| EF-2 | 术语归一化命中率未知 | 同上轮次统计 terms 归并 vs 新建比例 | 同词归并命中 | P1 精确归一化够用，P2 向量归并立项依据 |
