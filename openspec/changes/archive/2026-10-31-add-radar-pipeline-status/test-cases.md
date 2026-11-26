# test-cases: add-radar-pipeline-status

> 用例即 openspec Scenario。本 change 声明 `complexity: complex`：含跨端协议（`board_ids` 新参数）、状态机（生成按钮多态）、派生算法（当日判定 / 进度计算），按 §2 复杂档义务附白盒分支表与边界值清单。
> 锚点 Requirement：`radar-pipeline-status`（新增 2 条）、`daily-report-system`（MODIFIED「日报生成 API — 异步触发」+ ADDED「日报调度窗口的生成时刻与兜底时刻可读」）。

## 故事 S1: 读者在雷达页看清日报为什么还没出（锚 Requirement: 雷达总览页右栏底部常驻管线状态块）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 打开 `/tags` | 状态块常驻右栏底部 | 右栏底部出现管线状态块；内容区独立滚动，状态块不随内容滚动 | 组件 | `front/app/features/tags/components/console/BoardRadarStatusBlock.test.ts` |
| 2 | 对账成功 + 总览成功 | 状态块展示四项事实 | 同时呈现：队列待打标数、今日完成进度、生成时刻、兜底时刻、最新落点、当日已出/未出 | 组件 | `BoardRadarStatusBlock.test.ts` |
| 3 | 首次对账未成功 | 首次对账未成功不呈现业务结论 | 呈现「读取中」；DOM 中不含「今日日报未生成」 | 组件 | `BoardRadarStatusBlock.test.ts` |
| 4 | 曾成功后对账失败 | 对账失败保留旧值 | 保留旧值 + 标注更新时刻；不清零、不转错误态 | 组件 | `BoardRadarStatusBlock.test.ts` |
| 5 | 对账成功、总览失败 | 总览取数失败不判未生成 | 队列行照常；日报行「读取失败」；不含「今日日报未生成」 | 组件 | `BoardRadarStatusBlock.test.ts` |
| 6 | 队列有积压 | 队列未清空时禁用生成 | 生成按钮 `disabled`；存在缺料说明文案 | 组件 | `BoardRadarStatusBlock.test.ts` |
| 7 | 队列清空 | 队列清空后生成可用 | 按钮可用；缺料说明消失 | 组件 | `BoardRadarStatusBlock.test.ts` |
| 8 | 对账成功但无任何板块有日报 | empty（成功空集） | 「最新日报：暂无」+「今日未生成」 | 组件 | `BoardRadarStatusBlock.test.ts` |
| 9 | 某板块 `report_date` == 本地今天 | success（今日已出） | 「今日日报已出（N/M 板块）」 | 组件 | `BoardRadarStatusBlock.test.ts` |

### 变体走查

| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| V1 | 队列 `pending=0, processing=0, completed_today=0` | 队列行「队列空闲」；生成可用 | 组件 | `BoardRadarStatusBlock.test.ts` |
| V2 | 队列 `pending=3, processing=1, completed_today=62` | 待打标 4；进度 62/66 | 组件 | `useRadarPipelineStatus.test.ts` |
| V3 | 队列 `failed=2`（失败态） | 红色「2 个失败」（对齐 `TagQueueProgressChip` 口径） | 组件 | `BoardRadarStatusBlock.test.ts` |
| V4 | `schedule_time` 缺 `schedule_deadline`（旧后端） | 兜底时刻按默认 23:30 降级显示，不留空 | 组件 | `useRadarPipelineStatus.test.ts` |
| V5 | 本地 23:59 vs UTC 日期跨天 | 「今天」按本地日期判定（不走 UTC） | 单元 | `front/app/utils/todayKey.test.ts` |
| V6 | 窄屏 375px | 状态块落到页面下方，单列清单，无横向溢出 | 人工 | `ui-verification/report.md` |
| V7 | 明暗双主题 | 两种主题下文字/边框/警示色均可读，无硬编码色 | 人工 | `ui-verification/report.md` |
| V8 | 标签队列 `pending=0`，Embedding 队列 `pending=166` | 两行并列；标签队列「队列空闲」、Embedding「待向量化 166」；**生成按钮仍禁用**（双队列判据），缺料说明指向泳道分桶 | 组件 | `BoardRadarStatusBlock.test.ts` |
| V9 | 对账响应不含 `embedding_queue` 段（旧后端） | Embedding 队列行「状态未知」（**不谎称已清空**）；生成按钮禁用 | 组件 | `BoardRadarStatusBlock.test.ts` |
| V10 | 两队列均有今日完成与活跃量 | 两条独立进度条；标签队列分母按**文章**、Embedding 队列分母按**标签**，文案分别为「今日完成 X / Y」与「今日完成 X / Y 个标签」 | 组件 | `BoardRadarStatusBlock.test.ts` |
| V11 | Embedding 队列 `failed=3`（失败态） | 红色「Embedding 队列 3 个失败」；**不阻断**生成（failed 是终态） | 组件 | `BoardRadarStatusBlock.test.ts` |
| V12 | 清单展开期间 Embedding 队列复堵 | 确认按钮禁用（与入口按钮同款判据，不给绕过口） | 组件 | `BoardRadarStatusBlock.test.ts` |

### 效果核对

- 触发原因：状态块的核心价值是「解释日报为何未出」，其有效性依赖真实库里队列积压与日报窗口的实际组合（纯 fixture 绿灯不能证明读者真的能看懂）。
- 核对方法：跑一次真实库场景——人为制造标签队列积压（enqueue 若干 tag_jobs），在 `/tags` 观察状态块是否同时显示「待打标 N」与「今日未生成」，并确认缺料说明与队列数自洽。
- **双队列核对（2026-10-30 增量）**：真实库当日实测为「标签队列 0 积压 / Embedding 队列 166 积压」——只盯标签队列会把「定时任务其实不会出日报」这件事说反（定时路径要求双队列清空）。核对方法：`/tags` 实页确认两行并列且生成按钮因 Embedding 积压而禁用；`GET /api/poll` 的 `data.embedding_queue` 与 `data.tag_queue` 逐字段与页面一致。
- 量化结果：待填（归档前补）
- 结论：待填（达标交付 ｜ 瓶颈在<上游> ｜ 需调整预期）

### 白盒附加（复杂档）

**状态机：生成按钮**

| 状态 | 进入条件 | 按钮 | 说明 |
| --- | --- | --- | --- |
| `idle` | 无活跃量 + 未生成中 | 可用 | 队列清空后可生成 |
| `blocked` | 任一队列 `pending + processing > 0`（或 Embedding 计数未知） | 禁用 | 缺料说明常驻（按队列分列） |
| `picker-open` | 点击按钮（`blocked` 时不可达） | 展开清单 | 就地展开，不弹窗 |
| `picker-open`（队列复堵） | 清单展开期间 `pending + processing` 增长 | 确认按钮禁用 | 与入口按钮同款判据，不给绕过口 |
| `picker-open`（今日已出） | 至少一个板块 `report_date` == 本地今天 | 入口文案「重新生成今日日报」+ 清单内替换警示 + 确认按钮「确认替换」 | 同日替换是破坏性动作 |
| `submitting` | 已发请求未终态 | 进度态 + 禁用 | 防重复触发 |
| `done` | 收到 `daily_report_done`（job_id 匹配本次） | 恢复可用 + 结果内联 | 触发总览刷新 |
| `failed` | 请求被拒 / 错误 | 恢复可用 + 失败文案 | 409 `already_running` 显文案 |

**分支表：`board_ids` 请求**

| 输入 | 后端行为 | 期望 |
| --- | --- | --- |
| `{date}` | 全量 | 全部当日有内容板块；一个 `job_id` |
| `{date, board_id: 5}` | 单板 | 仅 #5（既有语义不变） |
| `{date, board_ids: [3,7,9]}` | 集合 | 仅 #3/#7/#9；集合外不生成 |
| `{date, board_ids: []}` | 全量 | 与缺省等价 |
| `{date, board_id: 5, board_ids: [3]}` | 拒绝 | HTTP 400，不启动管线、不取闸 |
| `{date, board_ids: [3,7,3]}` | 集合（重复折叠） | 仅 #3/#7 各生成一次；终态计数不重复计入 #3 |
| `{date, board_ids: {"id":3}}` | 拒绝（类型非法） | HTTP 400，不启动管线、不取闸、不广播不通知 |
| `{date, board_id: "5"}` | 拒绝（类型非法） | HTTP 400，同上 |
| 集合内 #7 失败 | 部分失败 | #3 照常完成；终态通知含成功+失败计数 |
| 集合内 #7 当日无内容（`report == nil`） | #7 计入 `empty_count`（既非成功也非失败） | 终态消息 `total_saved + failed_count + empty_count == total_boards` 恒成立；#7 不计入失败数（终态消息与终态通知同口径） |

**分支表：终态通知的计数口径（与 `daily_report_done` 终态消息同源）**

| 管线路径 | WS 终态消息（三态） | 终态通知（唯一一条） | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 单板成功 | `total_saved=1, total_boards=1, failed_count=0, empty_count=0` | 完成通知「共 1 个版面，1 个版面已产出日报。」 | 后端 | `backend-go/internal/topicgraph/handler/daily_report_handler_test.go` |
| 单板失败 | `total_saved=0, total_boards=1, failed_count=1, empty_count=0` | 失败汇总「共 1 个版面，0 成功 1 失败。」 | 后端 | 同上 |
| 单板当日无内容 | `total_saved=0, total_boards=1, failed_count=0, empty_count=1` | 完成通知（非失败汇总）「共 1 个版面，0 个版面已产出日报，其中 1 个当日无内容。」 | 后端 | 同上 |
| 全量 / 集合 | 三态恒等式 `total_saved+failed_count+empty_count==total_boards` | 通知的「共 N 个版面」== `total_boards`；成功/失败/无内容三数与消息一一对应 | 后端 | 同上 |
| 1 成功 + 1 失败 + 1 无内容 | `total_saved=1, total_boards=3, failed_count=1, empty_count=1` | 失败汇总「共 3 个版面，1 成功 1 失败，其中 1 个当日无内容。」 | 后端 | 同上 |
| 收集阶段失败（早退） | `total_saved=0, total_boards=1, failed_count=1, empty_count=0` | 失败汇总「共 1 个版面，0 成功 1 失败。」 | 后端 | 同上 |
| 当日无可生成板块（`total_boards==0`） | `total_saved=0, total_boards=0, failed_count=0, empty_count=0` | 不发通知（只一次终态广播） | 后端 | 同上 |
| **定时调度路径** 全成功 | ——（该路径 WS 消息为 `daily_report_complete`，协议不变） | 完成通知「共 2 个版面，2 个版面已产出日报。」三态计数 `{TotalBoards:2, Saved:2, Failed:0, Empty:0}` | 后端 | `backend-go/internal/admin/scheduler/job_daily_report_test.go` |
| **定时调度路径** 1 成功 + 1 失败 + 1 无内容 | —— | 失败汇总「共 3 个版面，1 成功 1 失败，其中 1 个当日无内容。」三态计数 `{TotalBoards:3, Saved:1, Failed:1, Empty:1}`（与手动路径同口径） | 后端 | 同上 |
| **定时调度路径** 收集阶段失败（早退） | —— | 失败汇总「共 1 个版面，0 成功 1 失败。」三态计数 `{TotalBoards:1, Saved:0, Failed:1, Empty:0}` | 后端 | 同上 |
| 通知写入口（三态口径） | —— | `notification.DailyReportTerminal`：成功侧写「N 个版面已产出日报」（SHALL NOT 写「保存 N 条条目」），失败侧写「N 成功 M 失败」；两者互斥 | 后端 | `backend-go/internal/platform/notification/notification_service_test.go` + `backend-go/internal/topicgraph/handler/daily_report_notification_test.go` |

**边界值清单**

| 量 | 边界 | 期望 |
| --- | --- | --- |
| `pending + processing` | 0 | 按钮可用 |
| `pending + processing` | 1 | 按钮禁用 |
| `completed_today` | 0 且总量 0 | 进度 0%（不除零） |
| 勾选板块数 | 0 | 确认按钮禁用 |
| 勾选板块数 | 1 | 可提交 |
| 勾选板块数 | 全部 | 等价于全量 |
| `report_date` 集合 | 空 | 「最新日报：暂无」 |
| 本地日期 | 00:00 / 23:59 | 与 UTC 无关 |

## 故事 S2: 读者在明确知道后果的前提下手动补生成（锚 Requirement: 管线状态块的手动日报生成入口）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 点击「立即生成今日日报」 | 默认全选并可取消勾选 | 状态块内就地展开清单，全部勾选，展示「参与数/总数」 | 组件 | `BoardRadarStatusBlock.test.ts` |
| 2 | 取消勾选两个板块 | 生成以勾选集合为作用域 | 确认后请求体含剩余板块集合，被取消者不在其中 | 组件 + 后端 | `BoardRadarStatusBlock.test.ts` / `daily_report_handler_test.go` |
| 3 | 取消勾选全部 | 未勾选任何板块禁止提交 | 确认按钮禁用；不发请求 | 组件 | `BoardRadarStatusBlock.test.ts` |
| 4 | 确认生成 | 触发多板块集合生成 | 后端 202/200 + `job_id`；仅集合内板块生成 | 后端 | `backend-go/internal/topicgraph/handler/daily_report_handler_test.go` |
| 5 | 收到进度消息 | 生成期间显示进度 | 按钮进度态；不可重复触发 | 组件 | `BoardRadarStatusBlock.test.ts` |
| 6 | 收到终态消息 | 生成完成后内联显示结果并刷新 | 内联显示成功/失败数；总览刷新 | 组件 | `BoardRadarStatusBlock.test.ts` |
| 7 | 今日已出时展开清单 | 今日已出时重生成给出替换警示 | 入口文案「重新生成今日日报」；清单内给出替换警示（标题、章节、线索全部重写）；确认按钮文案「确认替换」 | 组件 | `BoardRadarStatusBlock.test.ts` |
| 8 | 清单展开期间队列出现新文章 | 清单展开期间队列复堵仍禁止提交 | 确认按钮禁用；不发请求 | 组件 | `BoardRadarStatusBlock.test.ts` |

### 变体走查

| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| V8 | 一键全选 / 全不选 | 两种切换均生效；全不选时确认禁用 | 组件 | `BoardRadarStatusBlock.test.ts` |
| V9 | 生成请求被 409 拒绝 | 按钮恢复可用 + 显示「日报生成正在执行中，请稍后再试。」 | 组件 | `BoardRadarStatusBlock.test.ts` |
| V10 | 生成请求 400（超重建窗口） | 显示后端错误文案，不静默 | 组件 | `BoardRadarStatusBlock.test.ts` |
| V11 | 集合内单板失败 | 其余板块照常生成；终态含失败数 | 后端 | `daily_report_handler_test.go` |
| V14 | 清单展开期间队列复堵 | 确认按钮禁用；不发请求 | 组件 | `BoardRadarStatusBlock.test.ts` |
| V15 | 今日已出后再生成 | 入口文案「重新生成今日日报」+ 替换警示 + 确认按钮「确认替换」 | 组件 | `BoardRadarStatusBlock.test.ts` |
| V16 | 集合内重复 id（`[3,7,3]`） | #3 只生成一次；终态计数不重复计入 | 后端 | `daily_report_handler_test.go` |
| V17 | 字段类型非法（`board_ids` 传对象 / `board_id` 传字符串） | HTTP 400；不取闸、不启管线、不广播不通知 | 后端 | `daily_report_handler_test.go` |
| V18 | 并发管线的终态消息（job_id 不同） | 不被本次生成吸收；结果仍属本次 job | 单元 | `useRadarPipelineStatus.test.ts` |

### 效果核对

- 触发原因：手动生成的正确性依赖真实生成管线（LLM + DB 事务 + WS 广播），stub 级测试不能证明「勾选集合真的只生成了勾选的板块」。
- 核对方法：真库勾选 1 个板块触发生成，确认该板块日报落库、其余板块当日无新报告，且 WS 终态消息与通知各只一条。
- 量化结果：待填（归档前补）
- 结论：待填

### 继承与调整（涉及 MODIFIED Requirements）

| 旧 Scenario | 处置 | 旧测试文件 | 动作 |
| --- | --- | --- | --- |
| 触发单板日报生成 | 继承（`board_id` 语义不变） | `backend-go/internal/topicgraph/handler/daily_report_handler_test.go` | 保留原断言 |
| 触发全板日报生成 | 继承（缺省 `board_ids` 语义不变） | 同上 | 保留原断言 |
| 生成进行中重复手动触发 | 继承（互斥范围不变） | 同上 | 保留原断言 |
| 跨实例互斥 / 持闸崩溃 / 闸释放 / DB 不可达回落 | 继承（闸语义不变） | 同上 | 保留原断言 |
| 手动触发与定时调度互斥不产生重复通知 | 继承并扩展（集合生成同样只发一条） | 同上 | 新增集合场景断言 |

## 故事 S3: 前端能读到兜底时刻（锚 Requirement: 日报调度窗口的生成时刻与兜底时刻可读）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 请求 `GET /api/poll` | 状态载荷同时给出两个时刻 | `daily_report` 条目含 `schedule_time` 与 `schedule_deadline` | 后端 | `backend-go/internal/admin/handler/scheduler_handler_test.go` |
| 2 | `ai_settings` 无 `daily_report_deadline` 行 | 兜底时刻缺省回落默认值 | `schedule_deadline` == `23:30` | 后端 | 同上 |
| 3 | 兜底时刻早于生成时刻 | 兜底时刻早于生成时刻时回落 | `schedule_deadline` == `23:30`；不输出非法组合 | 后端 | 同上 |

### 变体走查

| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| V12 | `/api/schedulers/status` 同样端点 | 也含两个时刻 | 后端 | `scheduler_handler_test.go` |
| V13 | 前端收到旧后端（无 `schedule_deadline`） | 兜底时刻按 23:30 降级显示 | 组件 | `useRadarPipelineStatus.test.ts` |

### 效果核对

- 触发原因：兜底时刻的真实值来自 `ai_settings` 运行配置，fixture 常量不能证明与真实配置一致。
- 核对方法：改库中 `daily_report_deadline` 为真实非默认值（如 22:45），刷新 `/tags` 确认状态块显示 22:45；改回后确认回落。
- 量化结果：待填
- 结论：待填
