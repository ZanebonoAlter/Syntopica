# Test Cases: add-search-sort-modes

> 层选择：排序纯函数（日分组/多键比较/兜底）→ Go 单测；API 参数/响应/分页 → handler 测试（内存 sqlite）；前端状态机与页面交互 → Vitest 组件/composable 测试；真库效果 → 只读 API 抽查；视觉 → agent-browser 截图。

## 1. 主链路故事表（实时新闻进首屏）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 准备三篇命中：今天低分、昨天高分、前天高分 | 未传排序参数默认时间优先 | 默认顺序 = 今天 → 昨天 → 前天 | handler | handler_test.go |
| 2 | 同一天两篇：A 的 RRF 分高于 B | 同一天内按相关度排序 | A 在 B 前 | 单测 | sort_test.go |
| 3 | 两篇跨日 22 日 23:59 / 23 日 00:01（+08:00） | 日界按 Asia/Shanghai 划分 | 23 日在前 | 单测 | sort_test.go |
| 4 | 显式 `sort_by=relevance` | 相关度排序模式 | 恢复旧 RRF 顺序（昨天高分在前） | handler | handler_test.go |
| 5 | `sort_by=bogus` | 非法排序值回退默认 | 200 + `sortBy=date` | handler | handler_test.go |
| 6 | `per_page=1` 连续翻两页 | 分页顺序稳定 | 无重复、无遗漏 | handler | handler_test.go |
| 7 | 空 `pub_date` 的文章 | 空发布时间回退创建时间 | 用 created_at 参与日分组 | 单测 | sort_test.go |
| 8 | 前端首次搜索 | 默认最新优先 | 「最新优先」active，请求 `sortBy=date` | 组件 | search.test.ts / useSearch.test.ts |
| 9 | 有结果时点「相关度优先」 | 切换排序立即重查 | 同查询 + `sortBy=relevance` 重查，loadMore 沿用 | 组件 | search.test.ts / useSearch.test.ts |

## 2. 变体走查（五组）

- **输入**：`sort_by` 缺失 / 空串 / `date` / `relevance` / 非法值——有答案（默认 date、空串回退 date、非法回退 date）。
- **前置**：`pub_date` 为空；`created_at` 也为零值（防御路径排最后）；语义路 skipped（同日内语义分为 0，退化为 RRF+关键词）；expand 命中（扩展独占结果仍分区，分区内按 date 排）。
- **时间窗口**：日界前后 1 秒、跨月/跨年（2026-12-31 23:59 vs 2027-01-01 00:01）；无「时间范围筛选」语义（非目标），划除。
- **幂等**：同一查询相同 `sort_by` 重复请求顺序一致；切换排序后切回原值命中缓存/重查行为与现有过滤器一致；分页请求不因排序状态丢失而错序。
- **可用性（UI 必检前三）**：loading 时排序按钮 disabled；empty/error 下控件仍可见；窄屏过滤行折行不横向溢出。

## 3. 白盒附加（分支表）

| 分支 | 边界值 | 用例 |
| --- | --- | --- |
| `ParseSortMode` | `relevance` / `date` / `""` / `bogus` | sort_test.go |
| `sortDayKey` | 23:59:59.999 与次日 00:00:00（+08:00） | sort_test.go |
| `effectiveTime` | `PubDate != nil` / `nil` + `CreatedAt` / 双零值 | sort_test.go |
| `SortFused` date | 跨日、同日不同分、同日同分不同 ID | sort_test.go |
| `SortFused` relevance | 输入顺序 = 输出顺序（引用/内容不变） | sort_test.go |
| handler 排序装配 | 行投影池覆盖所有 fused ID（kw/sem/expand 三来源） | handler_test.go |
| 分页切片 | 空页、越界页、最后一页 | handler_test.go |
| 前端指纹 | q + archived + expand + sortBy 任一变化 → 重查 | useSearch.test.ts |
| 前端 loadMore | 切换排序后再加载更多 → 携带新排序 | useSearch.test.ts |

## 4. 效果核对（真库量化，只读）

| 指标 | 现状（2026-10-23） | 预期 | 核对 |
| --- | --- | --- | --- |
| `q=英伟达` 近 3 天命中 | 118 篇 | 默认首屏 50 条含 10-22/10-23 文章（≥1 条今日） | `GET /api/search?q=英伟达&per_page=50` 后统计 `pubDate` 前缀分布 |
| 默认首屏最旧日期 | 8 月 | 不早于当天分组的最后一页（首屏应集中在最近几天） | 同上 |
| `sort_by=relevance` 首屏 | 旧 RRF 顺序 | 与改动前一致（可用于回归对照） | 同查询加 `sort_by=relevance` |
| `total` | 1147 | 两种排序相同 | 两次响应 `total` 相等 |

## 5. 展示字段盘点

新用户可见字段仅「最新优先 / 相关度优先」两个按钮文案与 active 态，不新增数据字段。`sortBy` 为 API 回显字段（前端仅用于契约断言，不直接渲染），来源均为本 change 的 MODIFIED/ADDED Requirement，无隐式契约。
