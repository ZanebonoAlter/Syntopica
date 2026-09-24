# Test Cases — night-window-alignment（复杂档白盒用例）

> 测试单元 = Requirement 的用户故事；本文档串完整故事，spec Scenario 是断言片段。
> 层约定：函数单测（配置解析/排序构造）｜repository 层用 testcontainer PG（禁 SQLite）｜scheduler job 层沿用 `internal/admin/scheduler` 既有测试模式。

## 一、主链路表（3 个故事）

### 故事 A：暂停态 firecrawl 照抓、下游不硬分析（analysis-pause-control MODIFIED ×2）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| A1 | 构造 analysispause.IsPaused()=true（用户暂停路径），触发 firecrawl 调度 tick | 暂停时 firecrawl 照常抓取 | firecrawl 正常执行抓取（非 skipped），JobResult 不带 `analysis paused` | job 层 | `internal/admin/scheduler/pause_test.go` 扩展 |
| A2 | 同 A1 但健康门路径（aihealthy 快照 NOT 健康） | 健康门硬执行-模型未就绪时 firecrawl 照常抓取 | firecrawl 照常执行；content_completion tick 仍 skipped（对照断言） | job 层 | `pause_test.go` 扩展 |
| A3 | 暂停态下 firecrawl 完成回调（feed tagging_enabled=true） | 暂停时 firecrawl 的下游不触发 LLM | tag_jobs 新增 pending 行；无 LLM 调用发起（可断言不进入 tagger 路径 / ai_call_logs 无新行） | service 层 | `internal/reader/.../firecrawl` 相关测试扩展 |
| A4 | `grep -n 'PauseAware' runtime.go` 找 firecrawl 注册行 | （结构断言） | firecrawl 注册处无 PauseAware 包裹；content_completion 仍有 | 静态 | tasks 2.1 验证命令 |

### 故事 B：tag 队列新任务优先（tag-queue-scheduling ADDED-1 + analysis-pause-control 恢复续跑 MODIFIED）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| B1 | PG 造 3 条 pending：昨日 T0、今晨 T1、刚才 T2（priority 同 0） | 新旧任务并存时新的先消费 | lease 顺序 T2→T1→T0 | repository | `tagmanagement/service/core/tag_queue_test.go`（testcontainer PG） |
| B2 | 旧任务 priority=10，新任务 priority=0 | 高优先级旧任务可插队 | 旧任务先 lease | repository | 同上 |
| B3 | 最新任务 available_at=未来（退避中） | 退避中的任务不被 lease | 该任务不被 lease，轮到下一顺位 | repository | 同上 |
| B4 | 暂停→堆积 3 条→恢复 | 恢复后消化堆积任务（MODIFIED） | 恢复后按新任务优先顺序消费 | job 层 | `pause_test.go` / tag worker 恢复用例调整 |

### 故事 D：日报队列感知生成（scheduler-accuracy MODIFIED）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| D1 | 20:40 双队列已空，墙钟 21:00 | 队列提前清空则在墙钟时刻准时生成 | 21:00 触发生成 | 函数/编排 | `job_daily_report_test.go`（时钟注入或 nextRun 计算） |
| D2 | 21:00 时 tag_jobs 有 pending；22:10 清空 | 墙钟时队列非空则等待清空 | 22:10 后下一复查点（60s 内）触发，不早于 21:00 | 编排 | 同上 |
| D3 | 队列到 23:30 仍有 pending | 队列持续非空则兜底强制生成 | 23:30 强制触发；当日不二次生成 | 编排 | 同上 |
| D4 | 当日报告已存在，队列空、未到兜底 | （幂等，既有语义） | 不重复生成 | 编排 | 同上 |
| D5 | 等待期内手动 TriggerNow | （409 重入，既有语义） | accepted=false + 409 语义 | handler/job | 同上 |
| D6 | 22:00 重启（墙钟过/兜底未到/未生成/队列非空） | 重启不丢失调度 | 重启后继续按队列感知等待，当日兜底前触发 | 编排 | 同上 |
| D7 | 23:50 启动（当日未生成） | 服务在目标时刻后启动 | 不立即补跑，顺延次日；缺档归补档机制 | 编排 | 同上 |
| D8 | deadline="20:00" < time="21:00" | 兜底早于墙钟的非法关系 | deadline 回退 23:30 + warn；time 不变 | 函数 | 同上 |
| D9 | 双 key 缺失 / "25:99" / "abc" | 默认时刻 / 非法配置值回退默认 | 21:00/23:30 回退 + warn | 函数 | 同上 |

## 二、变体走查（五组）

- **输入**：时间配置变体全走（D9 覆盖空串/非法/边界 HH:MM）。纯时序逻辑无自由文本输入，其余不适用，划除。
- **前置**：空队列（日报生成时队列本就空→D1）；单任务；重复任务（同 article 多 tag_job 并存→各自独立判定）；越界引用（article 已删除的任务行随 CASCADE 消失，不参与 lease）。
- **时间窗口**：墙钟/兜底边界（=21:00 触发、=23:30 强制）；跨窗口（pending 跨多个自然日）；归一化（HH:MM 带前导零/不带）。
- **幂等**：日报当日重复触发（D4）；firecrawl 完成→重试→再完成（enqueue 不重复，既有 dedupe 语义）。
- **可用性**：纯后端无 UI 输入，不适用，划除。

## 三、效果核对（真库量化）

- 触发原因：夜间窗口 FIFO 倒挂 + 日报死区（探索实测：21:02 生成时完成度 ~52%）。
- 方法：上线后首个完整日，对比 `tag_jobs` 消费顺序（updated_at 时序 vs created_at 新旧）与日报生成时刻/当日文章覆盖。
- 量化：①20:00-21:00 消费的 tag_jobs 中当日创建占比应从 ~0% 升至多数；②日报生成时刻 ≥ 队列清空时刻（或 =23:30 兜底）；③生成时当日 pending 剩余 =0（兜底日除外）。
- 结论回填本节（人工核对，留痕）。

## 四、继承与调整（契约 M：5 个 MODIFIED Requirement）

| 旧 Scenario | 处置 | 旧测试 | 动作 |
|---|---|---|---|
| 暂停时调度任务不 lease（content_completion 例） | 保留语义 | `pause_test.go`、`runtime_test.go` | 不动（仍须绿） |
| 暂停时 tag worker 不消费 | 保留语义 | `pause_test.go` | 不动 |
| 健康门硬执行-模型未就绪时调度任务不 lease（例举含 firecrawl） | 改写（去 firecrawl，例举 content_completion/daily_report） | `runtime_test.go` 中 firecrawl 受门断言（如有） | 改：断言 firecrawl 不再被门拦 |
| 恢复后消化堆积任务（created_at 顺序） | 改写（新任务优先） | 断言 FIFO 顺序的用例（`tag_queue_test.go`/`pause_test.go` 如有） | 改：按 B4 新顺序断言 |
| DailyReport-服务在目标时刻前/后启动、重启不丢失、默认时刻、非法回退 | 改写（队列感知语义） | scheduler-accuracy 无历史 change 映射；`job_daily_report*` 既有测试如有墙钟断言 | 按 D1-D9 改写/新增 |

## 五、白盒附加（分支表 + 边界值）

**分支表（等待循环状态机）**：`等待(墙钟到) → [复查] 队列双空? →生成→结束｜非空→ [=deadline?] 是→强制生成→结束｜否→sleep 60s→复查`；手动 TriggerNow 在任意等待点命中 isExecuting 409。需覆盖分支：双空首查（不等 60s）、deadline 恰在 sleep 中（醒后立即判）、当日已存在（进入即返回）。

**边界值**：priority 相等与不等（排序稳定性按 created_at DESC 决胜）；available_at=now（可 lease，`<=`）；墙钟=deadline（合法，等价单时刻）；HH:MM 解析（"9:00" 无前导零、"09:00"、"24:00" 非法、"23:59" 合法）。

**不适用划除**：并发 lease 竞争用例（既有条件 UPDATE 乐观锁测试已覆盖，本 change 不改并发语义，不重复）；embedding 队列排序（明确不改，划除留痕）。
