# Test Cases: 泳道趋势概览（lane-trend-overview）

> 规划用例，未执行。Scenario 黑盒用例已在 specs/lane-trend-overview/spec.md 与 specs/board-lane-dynamics/spec.md（delta）定义，本文件为 complex 白盒枚举（分支/边界值），映射到目标测试文件。后端 SQLite service 测试注入 fake chat fn（仿 `lane_snapshot_test.go` 既有模式）；前端 vitest 组件测试。断言判据以 specs Scenario 为准，本表只做机械枚举展开。

## 继承与调整（⓪ 改契约反查：board-lane-dynamics 为 MODIFIED capability）

旧资产 = archive `2026-09-11-overview-lane-dynamics`（19 Scenario 映射，`bash scripts/harness/test-assets.sh board-lane-dynamics` 重建）。本 change 改了旧契约中的两个 Requirement（日报后滚动态势结算、泳道动态批量端点），受影响旧 Scenario 逐行处置：

| 旧 Scenario | 处置 | 旧测试 | 动作 |
| --- | --- | --- | --- |
| 日报完成后结算 | 扩展（产物单句→长短两版） | service/lane_snapshot_test.go, daily_report_lane_pipeline_test.go | 旧用例保留绿（触发链/异步红线不变）；新增 SN-1/2/3/9 覆盖两版契约 |
| 结算失败不阻塞 | 语义不变（失败口径新增解析降级一支） | service/lane_snapshot_test.go | 旧用例保留绿；新增 SN-4/5/6/8 覆盖降级/守卫分支 |
| 态势句与时间线同窗 | 语义不变（两版同窗） | service/lane_snapshot_test.go | 旧用例保留绿；SN-9 断言 prompt 同事实集要求 |
| 单请求聚合 | 扩展（snapshot 增 detail 字段） | repository/lane_snapshot_repository_test.go | 旧用例保留绿；新增 AG-1/2/4 覆盖两级缺失 |
| 无态势快照降级 | 语义不变（snapshot=null 不变） | LaneDynamicsCard.test.ts | 不动（卡片不改）；AG-3 在 repo 层断言 null 形状，FD-3 在趋势区断言占位 |

其余 14 个旧 Scenario（泳道卡片/候选/跳转/空态等）不涉本 change 契约，旧测试不动。

## 结算两版生成（backend topicgraph，目标：`service/lane_snapshot_test.go` 扩展）

| ID | 输入/分支 | 预期 |
| --- | --- | --- |
| SN-1 | LLM 返回合法 JSON `{"summary":"短版","detail":"长版"}` | upsert 两字段分别入库，as_of=anchor |
| SN-2 | summary 超 100 rune / detail 超 500 rune | 各自 rune 安全截断（中文不切半个字），两截断互不影响 |
| SN-3 | detail 恰 500 rune 边界 | 不截断，原样入库 |
| SN-4 | LLM 返回非 JSON 纯文本 | 降级：整段截 100 字作 summary、detail 空串、warn 日志；upsert 照常（下日报日自愈） |
| SN-5 | JSON 合法但 detail 缺失/空串 | summary 正常入库、detail 空；不算失败 |
| SN-6 | JSON 合法但 summary 缺失/空、detail 有值 | 维持既有空输出守卫：整次结算按失败处理（记日志跳过，快照保持旧值） |
| SN-7 | JSON 外裹 markdown code fence | 剥壳后按 SN-1 处理（容错与日报其它结构化输出一致） |
| SN-8 | LLM 调用报错 | 沿既有失败路径：不阻塞日报主流程、快照保持旧值、下日报日重试 |
| SN-9 | prompt 断言 | system prompt 含两版结构与字数约束、同事实集要求；请求 maxTokens=768 |
| SN-10 | 存量行 detail 为空 | 读侧不报错（Detail=nil 语义）；下个结算周期覆盖补齐 |

## 聚合响应扩展（backend，目标：`repository/lane_snapshot_repository_test.go` 扩展）

| ID | 输入/分支 | 预期 |
| --- | --- | --- |
| AG-1 | 快照含 detail 的泳道 | lane.snapshot.detail 为长版全文（非空字符串） |
| AG-2 | 快照 detail 空串（存量） | lane.snapshot.detail 为 nil（空串归一为缺失） |
| AG-3 | 泳道无快照 | lane.snapshot=null（既有语义不变） |
| AG-4 | detail=nil 与 snapshot=null 同时存在于不同泳道 | 两级缺失互不干扰，JSON 形状可区分 |

## 前端数据流（目标：`LaneTrendOverview.test.ts` + `DailyReportTopicSection.test.ts` 扩展）

| ID | 输入/分支 | 预期 |
| --- | --- | --- |
| FD-1 | 泳道展开、lane 含 detail | 默认 14 天档展示长版全文 + as_of；无截断类样式 |
| FD-2 | lane.snapshot.detail=nil、summary 有值 | 14 天档展示短版 + 「长版随下次日报结算生成」提示 |
| FD-3 | lane.snapshot=null | 「态势待结算」占位；三档切换仍可用 |
| FD-4 | lane=null（泳道不在聚合 lanes 中） | 同 FD-3 降级；逐日事件区显示无事件占位 |
| FD-5 | 切到月档，contexts 返回多条 period | 取 period 字典序最大一条渲染全文 + 该条 as_of_date |
| FD-6 | 切到月档，contexts 返回空数组 | 「该周期暂无归档摘要」占位，不渲染年档或其它粒度数据 |
| FD-7 | 切档请求 pending | 该档加载提示，不显示旧档内容 |
| FD-8 | 切档请求失败 | 内联错误条 + 重试按钮；重试成功后正常渲染 |
| FD-9 | 已拉取过的档位再次切入 | 命中缓存不再发请求 |
| FD-10 | 逐日事件默认收起 | 仅展开开关可见 |
| FD-11 | 展开逐日事件、某日事件 8 条 | 按日分组倒序渲染，单日显 5 条 + 「还有 3 条」就地展开 |
| FD-12 | 后端 folded_count>0 | 「另有 N 条未载入」如实标注（不可展开） |
| FD-13 | 非话题分组（topicId=null）或非 active zone | 不渲染趋势区；既有内容完整 |
| FD-14 | 收起泳道再展开 | 档位选中态与展开态重置为默认（14d、事件收起）——无跨展开残留 |
| FD-15 | 月/年 content 含换行 | pre-line 渲染保留段落结构，不解析 markdown |
| FD-16 | DailyReportTopicSection 挂载断言 | 趋势区渲染于节点图与当日明细之前；props 传递正确 |

## 宿主取数（目标：`BoardDailyReportTimeline` 相关测试补充/`useLaneTrendData` 组合式单测）

| ID | 输入/分支 | 预期 |
| --- | --- | --- |
| HD-1 | 首个泳道展开 | 触发一次板块级 lane-dynamics 请求；第二个泳道展开不重复请求 |
| HD-2 | 切换阅读的报告日期 | 不重拉 lane-dynamics（锚定最新报告期语义） |
| HD-3 | lane-dynamics 请求失败 | 展开体不受影响；趋势区错误条可重试 |
| HD-4 | contexts 缓存 | 同 topic 同粒度仅请求一次；跨泳道互不污染 |
