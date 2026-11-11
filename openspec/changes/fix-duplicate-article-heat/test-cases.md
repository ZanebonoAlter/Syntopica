# 用例故事 — 重复文章副本导致的来源数/热度虚高

**故事**：用户在雷达「今日续接」看到一条事件「2 个来源 · 热度 1.4」，点进日报却发现只有 1 篇博客；翻前几天的日报，又发现同一批文章在 10-31、11-01、11-02 反复出现。修复后：**同一篇内容无论被几个 feed 抓到，都只算 1 个来源**；**补档/重建生成的历史日报只含报告日当天的文章**——来源数与热度如实反映有几篇独立报道。

**测试单元**：Requirement 用户故事；spec Scenario 为断言片段。以下均为**待实现/待执行**用例。

## 1. 功能：主链路节拍

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| 1 | 当天按时生成日报 | 含关键字文章聚合为固定话题（回归） | 关键字物化 section 只含当日 `pub_date` 文章；`lane_tier=watch_keyword`、无持久话题 | Go 集成（隔离 PG） | `backend-go/internal/topicgraph/service/watch_materialize_integration_test.go` |
| 2 | 补档重建 3 天前日报 | 补档扫描绑定报告日而非执行时刻 | 物化 section 只含 D-3 的 2 篇文章；执行日（D）的 7 篇文章零出现 | Go 集成（隔离 PG） | `backend-go/internal/topicgraph/service/watch_materialize_integration_test.go` |
| 3 | 传零值日期 | 补档扫描绑定报告日而非执行时刻（边界） | 返回明确错误，SHALL NOT 静默扫「今天」 | Go 单测 | `backend-go/internal/topicgraph/service/watch_materialize_keyword_test.go` |
| 4 | 手动指定日期重建 | 手动按指定日期重建同口径 | 扫描窗口 = D-1 自然日，与当天按时生成结果一致 | Go 集成（隔离 PG） | `backend-go/internal/topicgraph/service/watch_materialize_integration_test.go` |
| 5 | 同一 URL 被两个 feed 抓取并归属同一事件线 | 同链接多 feed 副本只算一个参与者 | `participant_count = 1`；12.5h 后热度 = 0.70（非 1.40） | Go 仓测（隔离 PG） | `backend-go/internal/topicgraph/repository/story_heat_test.go`、`story_signals_test.go` |
| 6 | 5 个 feed 各发 1 条不同链接 | 不同内容各计一次（回归） | 参与者数 = 5，热度按 5 份累加 | Go 仓测（隔离 PG） | `backend-go/internal/topicgraph/repository/story_heat_test.go` |
| 7 | 副本 link 为 `?utm=x` 与 `#comments` 变体 | 规范化链接覆盖 fragment 与 query 差异 | 判同一内容，只计 1 个参与者 | Go 单测 + 仓测 | `backend-go/internal/platform/textutil/normalize_test.go`、`story_signals_test.go` |
| 8 | 事件线两篇证据 link 为空 | 链接缺失退化按 feed | 参与者身份退化为 `feed:<id>`，不同 feed 不合并 | Go 仓测（隔离 PG） | `backend-go/internal/topicgraph/repository/story_heat_test.go` |
| 9 | 同一 feed 48h 内 5 条同事件线 | 同一 feed 多条不叠加（回归） | 只按 1 个参与者计 | Go 仓测（隔离 PG） | `backend-go/internal/topicgraph/repository/story_heat_test.go` |
| 10 | 副本文章已归档 | 归档不影响参与者计入（回归） | 照常计入热度，不因 `archived` 过滤 | Go 仓测（隔离 PG） | `backend-go/internal/topicgraph/repository/story_heat_test.go` |
| 11 | 雷达 / 矩阵 / highlights / lane_heat 同读一条事件线 | 参与者身份与去重（消费面一致） | 四处「来源数」与热度同源同值；矩阵颜色分档按新参与者数 | Go 仓测（隔离 PG） | `backend-go/internal/topicgraph/repository/board_radar_stories_test.go`、`report_story_highlights_test.go`、`lane_snapshot_repository_test.go` |
| 12 | 真库回填后重跑一次 | 可重算（回归）+ 幂等 | `--rebuild-signals` 两次结果一致；5204 的 `participant_count` 由 2 → 1 | 运维 CLI + 只读核对 | `backend-go/cmd/backfill-stories/main_test.go` |
| 13 | 回填后核对报告正文 | 报告正文不被改写 | `daily_report_threads` / `daily_report_sections` 行数与 `related_article_ids` 校验和不变 | 只读 SQL | `tasks.md` §6.5–6.6 |

## 2. 继承与调整（⓪ 契约变更）

已运行 `bash scripts/harness/test-assets.sh story-heat-evidence` / `watch-materialized-topic` / `daily-report-system` 反查旧资产（6+8+Requirement 全部 Scenarios）。本 change MODIFIED 三个 Requirement，按下表处置：

| 旧 Scenario / 契约 | 处置 | 旧测试 | 动作 |
|---|---|---|---|
| 同一 feed 多条不叠加 | 保持（口径显式保留） | `story_heat_test.go` | 断言一字不改 |
| 不同 feed 各计一次 | **改写**（前置补「链接互不相同」） | `story_heat_test.go` | 旧用例两个 feed 用不同 link 继续保持；新增「同 link 两 feed → 1 参与者」并列用例 |
| 排除不可见证据 | 保持（本期不触碰 `archived` 口径，见红线） | `story_signals_test.go` | 不动 |
| 信源时间优先 / 重复写入收敛 / 归属变更时证据迁移 | 保持 + 扩充（新增 `content_key` 同源写入断言） | `story_signals_test.go` | 旧断言保留；补「两副本同 link 的 `content_key` 相等」 |
| 半衰期衰减 / 窗口外不计入 / 话题热度为事件线热度之和 | 保持（公式不变） | `story_heat_test.go` | 不动 |
| 历史重算一致 / 落后源标记后重算 | 保持 + 扩充（按内容键重算） | `story_heat_test.go`、`cmd/backfill-stories/main_test.go` | 保留；补「重算后同 link 多副本仍 1 参与者」 |
| 矩阵数字为事件线数 / 雷达排序与热度一致 | 保持 + 扩充（参与者数口径） | `story_heat_test.go`、`board_radar_stories_test.go`、`ObservationMatrix.test.ts` | 前端不动；后端断言改用内容键 |
| 含关键字文章聚合为固定话题 | 保持 + **负向新增**（补档日期） | `watch_materialize_keyword_test.go`、`watch_materialize_integration_test.go` | 旧断言保留；新增「补档日只取报告日文章」「零值日期报错」 |
| 漏网文章可被捞回 / 裁决批量单次请求 / 无命中不产空 section / 裁决关闭回退机械聚合 | 保持 | `watch_materialize_integration_test.go` | 不动 |
| 缺档日报自动补齐 / 队列未清空顺延 / 只补缺不重建已有 | 保持 + **负向新增**（内容口径） | `job_daily_report_test.go`、`job_daily_report_window_test.go` | 旧断言保留；新增「补档内容不含晚于报告日的文章」 |

### 展示字段来源盘点（⑤）

| 字段 | 来源语义 | spec 锚点 |
|---|---|---|
| 雷达「N 个来源」 | `participant_count`（内容身份去重后） | MODIFIED 参与者身份与去重 |
| 事件线热度小数 | `story_heat`（`COALESCE(content_key, participant_key)` 去重） | MODIFIED 参与者身份与去重 |
| 矩阵格颜色分档 | 当日最热事件线参与者数（同一去重键） | MODIFIED 参与者身份与去重 |
| lane_heat（板块关注度） | 同 keys 的 lane 汇总 | 热度消费口径一致（未改） |

## 3. 边界：五组变体走查

### 3.1 输入

| 变体 | 答案 |
|---|---|
| 空串 | `content_key = ''` → 退化按 `feed:<id>`，不合并不同 feed |
| 纯空白（全角/tab） | TrimSpace 后为空 → 同空串路径 |
| 纯 fragment / 纯 query（如 `https://a.com/p#x`） | 各自斩尾后得到 `https://a.com/p`；与无尾版本判同内容 |
| 单 token 非 URL（如 `abc`） | 规范化后原样（无 scheme/host 可小写）→ 仅同字面判同内容，不报错 |
| 大小写 | scheme/host 折叠小写；path 大小写敏感（`/P/1` ≠ `/p/1`，防止误合并） |
| 特殊字符（`#` 在 query 内、多个 `?`） | 按「第一个 `#` / 第一个 `?`」截断，剩余部分丢弃，不抛错 |
| 超长 link（>1000） | 规范化 O(n) 处理，不做截断（列宽由迁移决定 `text`） |

### 3.2 前置

| 变体 | 答案 |
|---|---|
| 无副本（每内容 1 篇） | 与修复前等价，参与者数不变 |
| 2+ 副本同 link | 合并为 1 参与者；heat 取该内容最新 `observed_at` |
| 部分副本 link 为空 | 有 link 的按内容合并，空 link 的单独按 feed 计（不吞并） |
| 越界引用（`story_signals.article_id` 指向已删文章） | 既有 FK `ON DELETE CASCADE` 语义不变，本期不改 |
| 部分满足（同一内容跨两个事件线） | 每条事件线各自判定参与者，不做跨事件线合并 |

### 3.3 时间窗口

| 变体 | 答案 |
|---|---|
| 当天边界 00:00:00 与 23:59:59 | `localDayWindow` 半开区间 `[00:00, 次日00:00)`：00:00 计入、次日 00:00 不计入 |
| 空窗口（补档日无文章） | 物化不产 section（既有「无命中不产空 section」） |
| 跨窗口（48h 热度窗与 14d lane 窗） | 两窗口参数各自独立，均改用内容键去重 |
| 归一化（CST vs UTC） | 扫描窗口按北京时间自然日；`observed_at` 仍存 UTC |

### 3.4 幂等

| 变体 | 答案 |
|---|---|
| 重复执行回填 | `--rebuild-signals` 幂等：先删后建同事务，两次结果一致 |
| 部分失败重试 | 单事件线失败仅告警，下次重算自愈（沿用既有语义） |
| 并发（并发重建同日报告） | 沿用 `(board, date)` advisory lock，不新增并发面 |

### 3.5 可用性

| 变体 | 答案 |
|---|---|
| 误输入 / 空态 / 错误态 / 加载态 / 超长文本 / 重复提交 | **不适用**：本 change 的 ui-impact = `none`，前端零改动（见 `ui-design.md` N/A Reason）；仅数值语义变化 |

## 4. 效果核对

效果依赖断言外因素（真库数据分布 / 补档历史），已在真库量化：

| 核对项 | 方法 | 结果 | 结论 |
|---|---|---|---|
| 同 link 重复副本规模 | `SELECT link, count(*) FROM articles WHERE link<>'' GROUP BY 1 HAVING count(*)>1` | 505 组 / 1038 行 | 修复后参与者数应回落约一半 |
| 受污染报告 | `watch_keyword` 轨非当天文章统计 | 21 份报告 / 50 篇（2026-09-12 起） | 修复后新生成的报告为 0 |
| 受影响事件线 | 近 30 天 `participants > 独立内容数` | 3 条（5204/5259/5319） | 重算后全库 `stories_with_dup` 归零（实测 2026-11-10：3 条均回落为 1 个参与者） |
| 样本 5204 | `board_stories.participant_count` | 2 → 1 | 热度 1.394 → **0.697**（报日 23:59:59、48h/24h），与「1 篇报道」一致 |
| 全库计数自洽 | 存储 `participant_count` vs 按 `COALESCE(NULLIF(content_key,''), participant_key)` 重算 | 1262 条事件线 | `mismatched = 0`（2026-11-10 回填后实测） |
| 报告正文未动 | `daily_report_threads` / `daily_report_sections` 行数 + `related_article_ids` 指纹 | 13894 / 6437 / `57f911b3…` | 回填前后完全一致 |
| content_key 覆盖率 | `story_signals WHERE content_key IS NULL OR =''` | 1281 行全部回填 | 空值 0 行 |

## 5. 白盒附加（复杂档）

### 5.1 分支表

| 单元 | 分支 | 期望 |
|---|---|---|
| `NormalizeArticleLink` | link 含 `#` 且含 `?`（`?` 在 `#` 前） | 先斩 `#` 后段，再斩 `?` 后段（两步顺序无关，结果一致） |
| | link 仅 `#frag` | 得到 base，非空 |
| | link 为 `http://A.COM/x` / `HTTPS://a.com/x` | host 与 scheme 小写后相同；path 不同则不同 |
| | link 为空 / 纯空白 | 返回 `""` |
| `scanBoardDayArticles` | `date.IsZero()` | 返回 error，不取 `time.Now()` |
| `story_signals` 写入 | article 有 link | `content_key` = 规范化 link |
| | article link 空 | `content_key = ""`，`participant_key = feed:<id>` |
| 参与者计数 | 同 story 内 2 行同 `content_key` | 计 1 |
| | 两行 `content_key` 均空、feed 不同 | 计 2 |
| | 一行空、一行非空（同内容） | 计 2（空 link 不并入有 link 的副本组） |

### 5.2 边界值

| 值 | 期望 |
|---|---|
| link 长度 0 / 1 / 2048 | 不 panic、不截断、返回稳定结果 |
| 48h 窗口起止各差 1s | 沿用既有 `s.observed_at >= since AND <= until` 边界语义 |
| 两副本 `pub_date` 相差 12h | 内容键相同 → 只取较新 `observed_at` 计一次 |
| `content_key` 列存量 NULL | `COALESCE(NULLIF(content_key,''), participant_key)` → 退化按 feed（回填前行为安全） |

### 5.3 不适用划除留痕

- 并发安全：本 change 不改并发路径（沿用 advisory lock 与既有事务边界）→ 不新增并发用例。
- UI 状态矩阵 / 视觉验收：ui-impact = none，前端零改动 → 划除（见 `ui-design.md`）。
- LLM 行为断言：本 change 不新增/修改任何 prompt → 无人工实弹核对项。
