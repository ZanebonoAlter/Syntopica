# 用例故事 — 事件身份与热度证据

**故事**：用户打开版块矩阵，看到「今天这条泳道有 2 件事，最热的一件有 6 个来源在报」。这个数字必须来自事件身份与原始证据，不能来自 LLM 分桶的形状。

**测试单元**：一个 Requirement 的用户故事，由本文件串成完整故事。`spec.md` 的 Scenario 是断言片段，本文件负责节拍、变体与效果核对。

## 1. 主链路节拍

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| 1 | 文章入库，融合提取产出 event 标签 + subject/subject_key/action/object | `aggregate-tagging` 同一次调用产出事件指纹 | 3 个 event 标签各带四字段，调用次数仍为 1；Trump/川普 产出同一 `person:trump` | Go 单测（fake LLM） | `tagmanagement/service/core/extractor_section_test.go` |
| 2 | 指纹与 `event_keywords` 校验 | `aggregate-tagging` 指纹校验失败不阻断 | 不一致标不可信但标签照常入库 | Go 单测 | `extractor_section_test.go` |
| 3 | 日报生成，事件落库推导 `scope` | `daily-report-system` 事件落库时 scope 由事件标签数推导 | 3 个 event 标签 → `scope=composite`、`story_id=NULL` | Go 仓储测试（testcontainer PG） | `topicgraph/repository/daily_report_repository_test.go` |
| 4 | 事件线归属：subject 不同 → 新建 | `event-story` 主体不同一票否决 | 即使向量距离极近也新建，理由记「subject 不同」 | Go service 单测 | `topicgraph/service/story_assign_test.go` |
| 5 | 事件线归属：阶段推进 | `event-story` 阶段推进标记为进展 | 挂同一事件线，`is_development=true` | Go service 单测 | `story_assign_test.go` |
| 6 | signals 写入 | `story-heat-evidence` 信源时间优先 | `observed_at` = `pub_date`，重复写入收敛为一行 | Go 仓储测试 | `topicgraph/repository/story_signals_test.go` |
| 7 | 矩阵读取 | `story-heat-evidence` 矩阵数字为事件数 / 颜色表达参与者数 | 2 个事件线 → 格值 2；最热事件线 6 个 feed → 参与者 6 | Go 聚合测试 + 前端组件测试 | `lane_snapshot_test.go`、`ObservationMatrixGrid.test.ts` |
| 8 | 雷达读取 | `story-heat-evidence` 雷达排序与热度一致 | A(12.0) 排在 B(9.5) 之前，不因 A section 少而反转 | Go 聚合测试 + 前端 | `board_radar_repository_test.go`、`BoardRadarView.test.ts` |
| 9 | 日报去重：重述降级 | `daily-report-system` 重述降级为快讯 | 正文不含该事件，signals 与热度不变 | Go service 单测 | `topicgraph/service/daily_report_dedup_test.go`（新增） |
| 10 | 端到端：矩阵 → 事件数 → 图例 | `workshop-board-console` 图例可见 | 格值与图例同屏可见，title/aria 带具体数字 | agent-browser（静态 :5100） | `ui-verification/report.md` |

## 2. 继承与调整（契约变更，⓪ 三问）

本 change 修改 4 个已有 capability，旧测试可能仍断言旧契约——跑绿 ≠ 对。

| 旧 Scenario | 处置 | 旧测试 | 动作 |
|---|---|---|---|
| `workshop-board-console` 矩阵格值是桶峰值 | **改写** → 格值是事件数 | `ObservationMatrixGrid.test.ts`、`ObservationMatrix.test.ts`、`observationLanes.test.ts` | 断言值从「桶峰值」改为「事件线数」；保留断档/零值/三态断言 |
| `workshop-board-console` 同日分桶不冒充分区热度 | **改写** → 同日分桶合并计数 | `ObservationMatrix.test.ts` | 原断言「7 与 10」改为「两桶各 1 事件线 → 2」 |
| `workshop-radar` 版块行构成与分档 | **改写** → 关注度 = lane_heat | `BoardRadarView.test.ts`、`useBoardRadarOverview.test.ts`、`boardRadarHelpers.test.ts` | 阈值常量换量纲，断言改为相对排序而非绝对数字 |
| `workshop-radar` 升温判定 | **改写** → story_heat 环比 | `BoardRadarView.test.ts`、`boardRadarHelpers.test.ts` | 从分区数 3 → 事件数 3；倍数按热度 |
| `aggregate-tagging` 逐片融合提取标签 | **扩充** | `internal/tagmanagement/service/core/*_test.go` | 新增三字段断言；既有「每片一次调用」「降级保留」断言保持 |
| `daily-report-system` 日报数据模型 | **扩充** | `daily_report_repository_test.go`、`daily_report_lane_integration_test.go` | 新增 `scope`/`story_id`/`is_development` 断言；既有 upsert/highlights 断言保持 |
| `daily-report-system` 线程存储在独立表 | **保持** | `daily_report_repository_test.go` | 不改，加 story_id 后仍通过 |

**反查命令**：`bash scripts/harness/test-assets.sh <capability>` 对四个 modified capability 各跑一次，结果已并入上表。

## 3. 变体走查

### 3.1 输入

| 变体 | 答案 |
|---|---|
| subject/action/object 空串 | 指纹不可信 → 走 LLM 或新建事件线；SHALL NOT 用空串做等值匹配 |
| 纯空白/全角空格 | 入库前 trim；trim 后为空按「空串」处理 |
| 超长 subject（>200 字） | 截断到列宽并标不可信，不因超长写入失败 |
| 特殊字符 / 引号 / 分号 | 走参数化查询；不参与字符串拼接 |
| 单 token（如 subject="AI"） | 允许，但同 subject 下 object 不同即分开；不因 subject 过于泛化而合并 |
| 大小写 / 全半角差异（"OpenAI" vs "openai"） | 归一化后比较（lower + NFKC） |
| 中英混写（"OpenAI 发布 GPT-5"） | 按原样存；比较时做别名展开 |
| subject 跨语言（Trump / 川普 / 特朗普） | `subject_key` 应同为 `person:trump`；若模型给出不同键，走 embedding（≥0.92）或字面层归并 |
| `subject_key` 缺失或空串 | 指纹不可信 → 走 LLM；SHALL NOT 用空键做等值匹配 |
| `subject_key` 两次不一致（`person:trump` vs `person:donald_trump`） | embedding ≥0.92 归并；否则按不同主体处理，不阻塞当日归属 |

### 3.2 前置

| 变体 | 答案 |
|---|---|
| 无候选事件线（新板块第一天） | 直接新建事件线，不调用 LLM |
| 单元素候选 | 正常判定 |
| 重复候选（同一事件线多次召回） | 去重后只判定一次 |
| `story_id` 指向已合并事件线 | 读取时经 `merged_into` 解析到存活者；写时不写已合并 id |
| 部分指纹缺失（有 subject 无 object） | 不可信 → LLM 或新建；SHALL NOT 用「缺字段等于相同」 |
| 事件无 embedding（watch 物化） | 只走文章集合重叠，不走向量召回 |

### 3.3 时间窗口

| 变体 | 答案 |
|---|---|
| 召回窗口边界（今天 − 14 天整） | 含当天（闭区间），验证时用 Asia/Shanghai 日历日 |
| 热度窗口边界（48h 整） | 含边界；49h 前的不计 |
| 空窗口（无任何 signal） | 热度 0，参与者 0；矩阵显示 0 而非空白 |
| 跨窗口（跨月/跨年） | 按 `observed_at` 绝对时间计算，不按日界切 |
| 时区归一 | 全部按 Asia/Shanghai 归一后比较；测试不得依赖运行机器时区 |
| `pub_date` 缺失 | 回退 `created_at` 并标 `observed_at_source=fallback` |

### 3.4 幂等

| 变体 | 答案 |
|---|---|
| 回填重复执行 | 跳过已有 `story_id`，事件线表不新增重复记录 |
| signal 重复写入 | `ON CONFLICT (story_id, article_id)` 收敛，`observed_at` 取最新 |
| 并发回填同一版块 | 版块级 advisory lock；后到者跳过已处理事件 |
| 部分失败重试 | 失败事件保持 `story_id=NULL`，下次回填重试 |
| 日报重新生成（同板同日） | 先删后插 section/事件；已建立的事件线与 signals 不删除，重新关联 |

### 3.5 可用性（UI 必检前三）

| 变体 | 答案 |
|---|---|
| 误输入反馈 | 无输入控件（只读矩阵），不适用 |
| 空态 | 版块无日报 → 既有空态；有日报但零事件 → 格值 0 且可点击 |
| 错误态 | 接口失败 → 既有错误条 + 重试；保留旧数据 + stale 提示 |
| 加载态 | 既有骨架行，不新增 |
| 超长文本 | 格内数字为整数，不涉及；title/aria 允许长文本但走既有截断策略 |
| 重复提交 | 无写操作，不适用 |
| 未回填降级 | 显示「事件身份未回填」，格值按事件上界，SHALL NOT 显示 0 冒充无事件 |

## 4. 效果核对（效果依赖断言外因素，须真库量化）

| 指标 | 方法 | 通过线 |
|---|---|---|
| 指纹可信率 | 全库 event 标签统计 `fingerprint_trusted` | 记录基线；低于 80% 需回看校验规则 |
| 事件线归组率 | `story_id IS NOT NULL` 的非综合稿事件占比 | 回填后 ≥ 95% |
| **误合并率（主指标）** | 人工抽检 50 条多事件事件线：是否真的是同一事件 | ≤ 5% |
| 误拆分率（参考） | 人工抽检 50 组「同 subject + 同 object 但被分到不同事件线」 | 记录，不设硬线 |
| 综合稿拦截 | `scope=composite` 且 `story_id IS NOT NULL` 的行数 | 必须为 0 |
| 热度 vs 旧口径 | 同一天同一版块的 lane_heat 排名与旧 attention 排名对比 | 记录差异；差异 > 30% 的版块人工看 5 个 |
| 雷达阈值标定 | 全量历史 lane_heat 的 P80/P20 | 写入 design 附录 |

核对结果记入 `review.md`，不写进自动化断言（LLM 行为与数据覆盖率不可控）。

## 5. 白盒附加（分支表 + 边界值）

### 5.1 归属判定分支

| # | subject_key | object | action | 指纹可信 | 期望分支 |
|---|---|---|---|---|---|
| 1 | 不同 | 任意 | 任意 | 任意 | 新建（一票否决） |
| 2 | 同 | 同 | 同 | 是 | 同一事件线 |
| 3 | 同 | 同 | 不同（非反向） | 是 | 同一事件线，`is_development=true` |
| 4 | 同 | 同 | 反向（发布 vs 召回/下架/否认） | 是 | 走 LLM |
| 5 | 同 | 不同 | 任意 | 任意 | 新建 |
| 6 | 任意 | 任意 | 任意 | 否 | 走 LLM；失败/低置信 → 新建 |
| 7 | — | — | — | — | `scope=composite` → 不建身份 |

**边界值**：召回窗口 14 天整 / 热度 48h 整 / 参与者 0 与 1 / `merged_into` 一层与两层（两层必须被解析到最终存活者）。

### 5.2 主体归一化分支

| # | 精确键 | embedding | 字面 | 期望 |
|---|---|---|---|---|
| 1 | 命中 | — | — | 同一主体（不调 embedding） |
| 2 | 未命中 | ≥ 0.92 | — | 同一主体（标记由 embedding 归并） |
| 3 | 未命中 | < 0.92 | 归一后相同 | 同一主体（字面命中） |
| 4 | 未命中 | < 0.92 | 不同 | 新建主体候选 |
| 5 | 空键 | — | — | 指纹不可信，`subject_key` 不参与比较 |

**边界值**：embedding 恰 0.92（含）/ 0.919（不含）；`subject_key` 大小写与下划线归一；`subject` 全半角 NFKC。

### 5.3 不适用划除

- 浮点精度：热度用 `numeric` 或 double 均可，比较用容差 1e-6；不做逐位比对。
- 加密/权限：单用户无鉴权，不适用。
- 国际化：仅中英混写场景，不做多语言矩阵。
