# Test Cases — add-searxng-health-indicator

测试单元=Requirement 用户故事，本文串成完整故事；spec Scenario 只是断言片段。complexity: simple（无白盒附加节；状态判定为简单枚举映射，非状态机）。

## ⓪ 契约核查：继承与调整

QA API 响应与 `annotation_qas` 均为**加法契约**（新增字段/列，无字段修改或删除），旧断言不破。apply 启动时先跑 `bash scripts/harness/test-assets.sh daily-report-margin-notes settings-workspace` 反查旧资产并逐行处置：

| 旧 Scenario / 旧测试 | 处置 | 动作 |
| --- | --- | --- |
| 「搜索失败静默降级」（回答照常返回、形态不变） | 保留不变 | 旧测试照跑绿；新标记断言为增量用例，不改旧断言 |
| 「联网补强回答」「编造 URL 白名单剔除」 | 保留不变 | 与 status 采集正交（status 描述搜索池，不描述引用） |
| `AskMarginNote` 现有 service/handler 测试 | 保留+扩展 | 搜索函数签名变更（返回 status）会改内部调用，断言增量补 `web_search_status` 落库与响应字段 |
| SearxngConfigPanel / MarginNoteCard 现有组件测试 | 保留+扩展 | 新增四态/标记用例，不改既有渲染断言 |

## 故事一：读者在联网失效时仍获得回答并看到降级标记

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 引擎全挂时提问（搜索 200 空 results） | 联网失效时回答带降级标记 | `web_search_status=empty` 落库+响应带出；卡片渲染「联网搜索无结果」标记 | service / handler / Vitest | `margin_notes_qa_test.go`、`margin_notes_handler_test.go`、`MarginNoteCard.test.ts` |
| 2 | searxng_config 未配置/enabled=false 时提问 | 未启用联网时不显示标记 | status=disabled 落库；卡片不渲染任何联网标记 | service / Vitest | 同上 + `margin_notes_websearch` 预检分支 |
| 3 | 搜索有结果进池（无论是否引用） | 联网正常时不显示标记 | status=ok；卡片不渲染标记 | service / Vitest | 同上 |
| 4 | 打开旧行（NULL）批注 | 历史问答不显示标记 | 不渲染标记、读取不报错 | Vitest / repository | `MarginNoteCard.test.ts`（null prop）、repository NULL 行读写 |

## 故事二：运维者在设置面板主动探测 SearXNG 健康

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 点击「测试连接」 | 点击测试连接获得四态结果 | loading 态按钮 disabled 防重复；结果渲染对应色点+耗时 | Vitest | `SearxngConfigPanel.test.ts` |
| 2 | 引擎全挂时探测 | 引擎全挂时探测显示 degraded | status=degraded + detail 摘要，非错误码 | handler | discovery handler 健康端点测试 |
| 3 | 未配置时探测 | 未配置时探测显示 disabled | 200 + disabled，不报错 | handler | 同上 |
| 4 | endpoint 拨不通/超时 | （四态边界） | status=unreachable；响应体异常归 unreachable（保守） | handler | 同上 |
| 5 | 正常时探测 | （四态边界） | status=ok + latency_ms | handler | 同上 |
| 6 | 保存配置成功 | 保存后自动探测 | 自动触发一次探测，探测异常不影响保存反馈 | Vitest | `SearxngConfigPanel.test.ts` |
| 7 | 真机完整链路（主交互） | 故事二步 1-3 连续 | opencli 落点：面板点击→状态渲染断言（静态托管 :5100） | opencli 端到端 | apply 期执行，或人工豁免留痕于此 |

### opencli 落点实录（apply 期执行，2026-10-04，agent-browser @ http://localhost:5100）

- **步 1+5（点击测试连接→ok 态渲染）**：设置→数据源与网络→SearXNG→点「测试连接」→状态行断言 `连接正常 · 4494ms`（含耗时）✓
- **步 4（unreachable）+步 6（保存后自动探测）**：endpoint 改死端口 `http://127.0.0.1:9`→保存→自动探测触发→红点 + `无法连接（endpoint 不可达或超时）` ✓；恢复 `http://localhost:8889`→保存→自动探测→`连接正常 · 2422ms` ✓（配置已还原无残留）
- **步 2/3（degraded/disabled）真机态**：当前 Clash 出口已恢复且配置启用，无法自然复现——handler 层 httptest 桩测试已覆盖四态判定（`searxng_health_handler_test.go`），真机留人工（tasks 8.3）

## 变体走查

- 输入：endpoint 空串/纯空白 → 预检不过 → disabled（复用既有 trim 逻辑）；探针 query 为固定常量无输入面；status 字符串仅枚举值，集外值前端按无标记处理
- 前置：未配置 / 已配置未启用 / 已配置已启用三态全覆盖（故事一步 2-3 + 故事二步 3-5）；annotation_qas 旧行 NULL、新行三态（故事一步 4）
- 时间窗口：不适用（无窗口逻辑）——划除
- 幂等：探测按钮重复点击（loading 防重复，故事二步 1）；保存后自动探测与手动探测可重复执行；同一批注多次追问每轮独立落 status（依赖步 1-3 落库断言覆盖）
- 可用性：loading（按钮 disabled+spinner）✓｜空态（disabled 灰点文案）✓｜错误态（unreachable 红点）✓｜长文本（detail 截断展示，组件测试覆盖超长 detail）✓｜重复提交防护 ✓

## 层选择与效果核对

- 层：状态判定纯逻辑=service 单测；迁移=testcontainer PG（禁 SQLite）；HTTP=handler；组件=Vitest；完整交互=opencli（故事二步 7）
- 效果核对：不适用数据覆盖率型核对（标记可见性由断言直接验证，不依赖 LLM 行为；真机 degraded→ok 对照在 tasks 8.3 手验节执行）

## 验收措辞对照

全部任务行验收=「命令+期望」（见 tasks.md §6-8）；无「单测覆盖」「PASS」裸词；simple 档无白盒附加节。
