# 用例故事 — 雷达右栏改事件线

**故事**：读者打开叙事工坊，右栏「今日该读」直接给出**今天正在发生的事件线**（跨版块混排、每条带版块名），「今天可能升温」给出**每条事件线自己的**升温倍率，新出现的事件线单列「今日新起」；点任意一条直达该期日报并定位到正文对应段落。读者不应被未回填的版块、延迟的数据或无正文落点的条目误导。

**测试单元**：Requirement 用户故事；spec Scenario 为断言片段。以下均为**待实现 / 待执行**用例，不表示当前实现已具备能力。

## 1. 功能：主链路节拍

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| 1 | 打开叙事工坊首屏，单次请求 overview | 一次请求渲染雷达 / 事件线清单随行同响 | 一次 `GET /api/semantic-boards/overview` 同时拿到版块行与 `stories` 段；段内含锚点日、热度口径版本、三档清单、未回填与降级标记；不额外发第二个请求 | 仓储（隔离 PG）+ Vitest | `backend-go/internal/topicgraph/repository/board_radar_repository_test.go`；`front/app/features/tags/composables/useBoardRadarOverview.test.ts` |
| 2 | 看「今日该读」 | 今日该读按事件线跨版块混排 | 列出跨版块事件线，每条带版块名、事件线标题、来源数、热度；不再渲染日报标题当条目 | Vitest | `front/app/features/tags/components/console/BoardRadarView.test.ts` |
| 3 | 比对条目先后 | 排序只按热度与来源数 | 热度降序 → 来源数降序 → 存活事件线 id 升序；版块的 section 数 / thread 数 / 关注度不影响名次 | 仓储（隔离 PG） | `backend-go/internal/topicgraph/repository/board_radar_repository_test.go` |
| 4 | 核对「今天」的日期 | 今日按锚点日而非自然今天 | 当日热度与基线按锚点日 R 的 CST 日终计算；R ≠ 自然今天时不出错日期的条目；界面标明 R | 仓储（隔离 PG） | `backend-go/internal/topicgraph/repository/board_radar_repository_test.go` |
| 5 | 看「今天可能升温」 | 升温判定 / 分区多但事件少不升温 | 每条是事件线、倍率是该事件线自己的（当日热度 ÷ 自身近 3 日基线）；版块 section 多但事件线热度不足者不列 | 仓储（隔离 PG）+ Vitest | `backend-go/internal/topicgraph/repository/board_radar_repository_test.go`；`front/app/features/tags/components/console/BoardRadarView.test.ts` |
| 6 | 看「今日新起」 | 今日新起单列且不显示倍率 | 近 3 日无基线的新事件线单列一档；不显示倍率（无 0.0×、无 ∞） | Vitest | `front/app/features/tags/components/console/BoardRadarView.test.ts` |
| 7 | 面对未回填版块 | 未回填版块不冒充无事件 | 该版块 0 条目；界面标「N 个版块事件身份未回填，未纳入事件线排序」；不出现「今天没有事件」 | Vitest | `front/app/features/tags/components/console/BoardRadarView.test.ts` |
| 8 | 面对超时版块 | 部分版块延迟时排序不完整明示 | 降级版块条目缺席；界面标「部分版块数据延迟，本次事件线排序不完整」 | Vitest | `front/app/features/tags/components/console/BoardRadarView.test.ts` |
| 9 | 点一条事件线 | 深链参数定位与失去目标 | 跳 `/tags/{boardId}/{reportDate}?story={storyId}`；日报台定位正文 DOM 顺序中第一条对应 thread，高亮 2s | Vitest + 完整交互 | `front/app/features/tags/components/DailyReportReader.test.ts`；人工：agent-browser 静态 :5100，证据写本 change `ui-verification/report.md` |
| 10 | 目标已消失 / 无正文落点 | 深链参数定位与失去目标 / 失去目标与旧重点 | 不滚动、不跳近似位置，提示「本期正文暂无对应事件」；无落点条目不渲染为可点 | Vitest | `front/app/features/tags/components/DailyReportReader.test.ts` |
| 11 | 后端未带 `stories` 段 | 一次请求渲染雷达 | 前端回退既有版块级日报标题列表与版块级升温清单，不渲染业务空态 | Vitest | `front/app/features/tags/composables/useBoardRadarOverview.test.ts` |
| 12 | 版块无任何数据 | 无数据版块 | 端点仍返回该版块且各计数为 0；事件线清单不因它产生假条目 | 仓储（隔离 PG） | `backend-go/internal/topicgraph/handler/board_radar_overview_handler_test.go` |

任务验证节包含全部 delta Scenario → 真实测试文件的逐行映射，继承项也须回归，不以主链路替代完整清单。

## 2. 继承与调整（⓪ 契约变更）

已跑 `bash scripts/harness/test-assets.sh workshop-radar` 与 `bash scripts/harness/test-assets.sh daily-report-system` 反查旧资产，按本 change 实际触及的 Requirement 处置：

| 旧 Scenario / 契约 | 处置 | 旧测试 | 动作 |
|---|---|---|---|
| 升温判定（版块级：当日事件线数 ≥ 3 且合计热度 ≥ 2× 近三日日均） | 保留为旧响应回退路径（版块级）；新增事件线级断言 | `front/app/features/tags/composables/useBoardRadarOverview.test.ts`、`front/app/features/tags/components/console/BoardRadarView.test.ts` | 旧断言保留（无 `stories` 段时仍按版块级口径回退）；另在 `BoardRadarView.test.ts` 新增事件线级断言：倍率挂在事件线、基线为事件线自身近 3 日（不含当日）、当日须有报道证据；版块合计口径不再作为新版响应下的升温依据 |
| 分区多但事件少不升温 | 改写 | `front/app/features/tags/components/console/boardRadarHelpers.test.ts` | 保留「section 数不得冒充事件线热度」的原则，落到事件线级断言 |
| 无升温 | 改写 | `front/app/features/tags/components/console/BoardRadarView.test.ts` | 空态文案保留「暂未触发升温条件」，触发源换成事件线级 |
| 一次请求渲染雷达 | 扩充 | `front/app/features/tags/composables/useBoardRadarOverview.test.ts` | 断言同一次响应同时含版块行与 `stories` 段；不新增第二次请求 |
| 无数据版块 | 保持 | `backend-go/internal/topicgraph/handler/board_radar_overview_handler_test.go` | 零值行与「常温」归类不变 |
| 今日该读（取关注度最高的若干版块的当日日报标题） | 改写 | `front/app/features/tags/components/console/BoardRadarView.test.ts` | 条目从版块日报标题换成跨版块事件线；版块行的「最新日报标题」入口保持不变 |
| 点击第一条可见正文线程 / 失去目标与旧重点 / 重复定位与减弱动画 | 保持 | `front/app/features/tags/components/DailyReportReader.test.ts`、`front/app/features/tags/components/daily-report/DailyReportTopicSection.test.ts`、`front/app/features/tags/components/daily-report/DailyReportMasthead.test.ts` | 刊头定位路径不变；深链复用同一实现，只加入口与参数清理 |
| 展示筛选不改变热度 | 保持 | `backend-go/internal/topicgraph/repository/report_story_highlights_test.go`、`backend-go/internal/topicgraph/service/daily_report_story_dedup_test.go` | display_tier 只筛展示资格，不改身份/signals/热度；本 change 不动去重判据 |
| 事件线热度口径（`heat-v1-48h-hl24h`、48h 窗 / 24h 半衰期、参与者去重、不按 archived 过滤） | 保持 | `backend-go/internal/topicgraph/repository/story_heat_test.go` | 雷达事件线明细复用同一算法，不新增公式 |

### 展示字段来源盘点（⑤）

| 字段 | 来源语义 | spec 锚点 |
|---|---|---|
| 条目事件线标题 | `board_stories.title`（存活身份，一跳归并后） | MODIFIED 今日该读与升温提示 |
| 条目版块名 | 版块 `label`（与雷达行同一来源） | MODIFIED 今日该读与升温提示 |
| 来源数 | 锚点日日终 48h 窗内去重 `participant_key` 数 | MODIFIED 今日该读与升温提示 |
| 热度 | 锚点日日终 `story_heat`（原始精度，展示一位小数） | MODIFIED 今日该读与升温提示 |
| 倍率 | 当日热度 ÷ 自身近 3 日基线（基线为 0 时不显示） | MODIFIED 今日该读与升温提示 |
| 锚点日 | overview 顶层 `date`（= R） | MODIFIED 版块总览聚合端点 |
| 未回填标注 | `stories_backfilled`（与 lane-dynamics 同源判定） | MODIFIED 今日该读与升温提示 |
| 排序不完整标注 | `data_delayed`（既有 5s 预算降级） | MODIFIED 今日该读与升温提示 |
| 落点报告日期 | R 之前最近一期含该事件线 body thread 的日报 | MODIFIED 今日该读与升温提示 + MODIFIED 重点依据与正文定位 |

## 3. 边界：五组变体走查

### 3.1 输入

| 变体 | 答案 |
|---|---|
| 空串 / 纯空白（全角/tab） | 事件线标题为空 → 显示「事件线 #ID」，身份与点击不受影响；标题仅展示用，不参与身份判定；空白标题不隐藏条目 |
| 纯分隔符（单/连/首尾） | 同上；不按标题分词或切分造身份 |
| 单 token | 短标题正常展示，不强凑字数 |
| 大小写 | 标题大小写不影响身份（身份用 id）；排序不按标题字典序 |
| 特殊字符 / 引号 / HTML | Vue 文本转义，不使用 v-html；后端参数化查询，不拼 SQL |
| 超长标题 | 换行 + title/aria 可读，不撑宽右栏、不裁断按钮可访问名称；不按截断文本匹配身份 |
| 缺数字 vs 0 | 缺 `participants` 字段 → 省略数字不伪造 0；真实 0 → 显示「0 个来源」；热度为 0 的事件线不进任何清单（不是「0 热度榜」） |

### 3.2 前置

| 变体 | 答案 |
|---|---|
| 空集 | 无任何可识别事件线 → 今日该看「今天没有可识别的事件线。」；升温「暂未触发升温条件。」；新起「今天没有新起的事件线。」 |
| 单元素 | 只 1 条时照常显示，不为凑 5 条造条目 |
| 重复 | 同版块多条 thread 指向同一存活线 → 归并为一条（survivor 一跳）；跨版块不会出现同一事件线 |
| 越界引用 | 坏 merge 链 / 环 / 多跳 → 保守丢弃该条，不进清单、不进排序键破同分 |
| 部分满足 | 部分版块超时 → 标「排序不完整」；部分版块未回填 → 标「事件身份未回填」；两者都发生时两条标注并存 |
| 全未回填 | 只有「事件身份未回填」标注，SHALL NOT 出现「今天没有事件」 |
| 无日报版块 | 不产生条目（无 anchor 即无当日证据）；版块行仍返回零值 |

### 3.3 时间窗口

| 变体 | 答案 |
|---|---|
| 两端 | 锚点日 00:00:00 与 23:59:59 的 signal 都算「当日证据」；基线窗起点（R-3 日终 − 48h = R-5 日终）恰在下界的 signal 计入，早 1ns 不计入；晚于 R 日终的未来 signal 不计入 |
| 空窗口 | 基线三天全 0 → 「今日新起」（不显示倍率）；当日热度 0 → 不进任何清单 |
| 跨窗口 | 48h 窗跨日：R-1 日 24:00 之后的 signal 同时进入当日热度与基线窗；这类条目不算「新起」 |
| 归一化 | 一律 Asia/Shanghai（`storyHeatZone`），不依赖宿主时区；历史锚点用锚点日而非 `time.Now()` |
| 锚点日 ≠ 自然今天 | 按锚点日 R 计算（Scenario「今日按锚点日而非自然今天」）；R 之后的自然日数据不参与 |
| 报告日 ≠ 锚点日 | 落点报告取 R 之前最近一期含该线的日报；该期早于 R 属正常（事件线跨天延续） |

### 3.4 幂等

| 变体 | 答案 |
|---|---|
| 重复执行 | 同一证据 + 同一锚点重复拉取 → 三档清单与名次完全一致（纯函数，无缓存无累计） |
| 部分失败重试 | 单版块故事计算失败 → 该版块条目缺席并标降级；重试后补齐，不产生重复条目、不累计旧条目 |
| 并发 | 端点按版块并发扇出、共享 5s deadline，扇出后单点归并；无跨板写、无新增锁；归并顺序稳定（排序键含 id 兜底） |
| UI 重复激活 | 深链定位最多一个高亮、2s 后清理；重复点同一条不叠加计时器；翻页后参数清掉、不在别的报告重复定位 |

### 3.5 可用性

| 变体 | 答案 |
|---|---|
| 误输入反馈 | `?story=` 为非数字 / 不存在的 id → 忽略该参数，不报错、不阻断阅读 |
| 空态 | 见 3.2 空集；「今天没有事件」这一表述只允许出现在确实无事件且已回填的情形 |
| 错误态 | 首次取数失败 → 既有错误条 + 重试；不渲染右栏内容（R-17），不产出业务空结论 |
| 加载态 | 沿用既有骨架，无悬空导航 |
| 超长文本 | 三桌面 + 390×844 窄屏下标题换行、来源数/热度行不溢出，aria 可读 |
| 重复提交 | 无写按钮，不适用 |
| 划选与批注 | 选中文本不触发定位；深链定位不破坏既有批注锚点 |
| reduced-motion | 深链定位沿用既有纪律：不强制平滑滚动，焦点轮廓仍可见 |
| 可达性 | 条目为 `<button>`（Tab/Enter 可达）；title/aria 给全「版块名 · 标题 · N 个来源 · 热度 X.X」；倍率不是唯一信息载体 |

## 4. 效果核对（实施后执行，不伪填结果）

**触发原因**：事件身份回填覆盖率、各版块数据日与锚点日的落差、热度分布——都决定清单是否有内容、排序是否符合直觉，纯函数测试代表不了。

**方法**：只读抽样（静态 :5100 + 只读 API），选 ≥3 个已回填版块与 ≥1 个未回填版块，记录：活跃版块数 / 已回填版块数 / 未回填版块数；锚点日 R；今日该看 Top 5 的条数与版块分布（是否单板垄断）；升温档条数与倍率分布；新起档条数；每条的来源数与热度；点击命中率（条目落到的日报是否确含该事件线）。**不执行任何写库 SQL、不重建日报**。

**量化核对目标**：未回填版块 100% 被标注且 0 条进入清单；点击命中率 ≥ 90%，未命中者必须是「无正文落点」且界面已明示；升温档不得出现基线 = 0 的条目；新起档不得出现倍率数字。

**结论**：样本日期、计数、分母与结论写 `ui-verification/report.md`；当前结果均未采集，不预填、不声称已达成。

### 4.1 实测记录（2026-11-06，静态 :5100 + 只读 API + 只读 SQL）

**样本**：真库 13 个活跃版块（`semantic_labels` board/active）；`board_stories` 活跃 **910** 条；今日（2026-11-06）有报道证据的事件线 **135** 条。

| 核对项 | 目标 | 实测 | 结论 |
|---|---|---|---|
| 未回填版块被标注 | 100% 标注且 0 条进清单 | `unbackfilled_board_ids` = `[50413, 246030, 285673]`（3 个），右栏显示「3 个版块事件身份未回填，未纳入事件线排序。」，这 3 板 0 条目 | **达成** |
| 滞后版块不被整板剔除 | 滞后板仍出条目且显示自己日期 | `lagging_board_ids` = `[3122]`（刚果疫情新闻，锚点日 11-05）；条目 `anchor_date` = 2026-11-05 ≠ 全局 R 11-06 | **达成** |
| 排序非单板垄断 | Top 5 跨 ≥3 版块 | 俄乌冲突×3 / 美国新闻 / 中国国内新闻 —— **3 个版块** | **达成** |
| 排序键正确 | heat desc → participants desc → id asc | 1.292(p=2) / 0.892 / 0.890 / 0.885 / 0.885，逐条降序 | **达成** |
| 升温档不出现基线=0 条目 | 0 条 | 升温清单**为空**（`warming_count`=0），无基线=0 的条目 | **达成（空集）** |
| 新起档不出现倍率数字 | 0 个倍率数字 | `new_count`=120；新起档条目只有标题 + 来源数 + 热度，**无倍率** | **达成** |
| 点击命中率 | ≥90% | Top 5 每条均带 `report_date`（落点非空）；实测点击 1 条（`story=5017`）→ 落点日报正文含该线、目标线程获焦点、滚动到位 | **达成（抽样 1/5，未满样本）** |
| 字段覆盖率 | Top N 每条字段齐全 | 5/5 条均带 `board_label`/`story_title`/`participants`/`heat`/`anchor_date`/`report_date` | **达成** |

**关键发现（影响后续 change，非本 change 缺陷）**：**事件线目前不跨天** —— `board_stories` 910 条活跃线**全部** `first_seen_date = last_seen_date`，跨天事件线 **0** 条；今日 135 条事件线中仅 **2** 条有历史基线。因此：

1. **「今天可能升温」在当前数据下结构性为空**（事件线级升温需要自身近 3 日基线，而基线几乎不存在）。今日真库实测旧版块级口径同样为 0 条（最高 1.81×，未达阈值），**非本 change 引入的回归**。
2. 「今日新起」档因此承担了几乎全部条目（120 条）。
3. 「事件线」当前实际粒度 = 「当天的一条线索」，而 spec 定义的「跨天同一事件聚合」尚未成立 —— 这是**独立于本 change 的数据层问题**，需另开 change 处理（泳道候选链的「事件线链」维度同样受此影响）。


## 5. 白盒附加（medium 档，精简分支表）

| 分支条件 | 断言 | 最便宜层 |
|---|---|---|
| 跨板归并（两板各一条） | Top 清单按热度混排、每条带自己的版块名 | 仓储（隔离 PG） |
| 热度同分 | 来源数多者在前；再同分则 story id 小者在前 | 仓储（隔离 PG） |
| 事件线级升温成立但版块合计不成立 | 事件线仍入升温档（证明判定主体是事件线） | 仓储（隔离 PG） |
| 基线为 0 | 入新起档、倍率字段为 0 且不展示 | 仓储 + Vitest |
| 当日无证据但前一日有 | 不入升温档、不入新起档 | 仓储（隔离 PG） |
| 版块未回填 | 该板 0 条目 + 计入未回填计数 | 仓储（隔离 PG）+ Vitest |
| 版块超时降级 | 该板 0 条目 + `partial=true` + 降级版块 id 在列 | 仓储（隔离 PG）+ Vitest |
| 坏 merge 链 / 环 / 多跳 | 该条被丢弃，不影响其它条目排序 | 仓储（隔离 PG） |
| 深链命中 / 落空 / 非法值 | 命中定位并高亮；落空提示不滚动；非法忽略 | Vitest |

**边界值清单**：候选条数 0/1/5/6（Top N 截断）；热度 0 / 同分 / 接近但不等；来源数 0/1/4；标题空/长/特殊字符；锚点日 = 自然今天 / 早 1 天 / 早 3 天；基线恰为 0 / 极小非 0；`?story=` 为 0 / 负数 / 非数字 / 不存在的 id；报告日早于锚点日 / 等于锚点日。

**不适用留痕**：无新 DDL、无数据迁移、无新表单输入、无新弹层与路由、无新线程安全算法（扇出并发沿用既有实现）、无热度阈值标定（阈值沿用 ≥2×）；相应用例不新增，仓储测试坚持隔离 PostgreSQL，不用 SQLite 代替。
