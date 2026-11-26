# 用例故事 — 事件线链（跨天同事件选材维度）

**故事**：用户新建泳道时，左侧除了既有的「话题链」，还多了一组「事件线」——每条是同一事件跨天的分区，一次勾齐即可把跨天材料整组带进本次泳道；两条链并列、互不干扰，事件线只是选材维度，不会顺手把任何候选话题转正。

**测试单元**：Requirement 用户故事；spec Scenario 为断言片段。以下均为**待实现/待执行**用例，不表示当前实现已具备能力。

## 1. 功能：主链路节拍

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
|---|---|---|---|---|---|
| 1 | 打开编排态看候选栏 | 两条链并列且互不干扰 | 话题链与事件线链两条维度并列；组标题/图标/文字标签可区分（非仅颜色） | Vitest | `front/app/features/tags/components/console/BoardComposeOverlay.test.ts` |
| 2 | 同一事件跨天成链 | 同一事件跨天直连成链 / 成员数或跨度不足不成链 | 同存活事件线的第 1 日与第 4 日分区入同一条链、跨度 3 天；差 1 天或只有 1 个分区不成链 | Go 纯函数 | `backend-go/internal/topicgraph/repository/compose_chain_test.go` |
| 3 | 事件线身份归一 | 链键按存活事件线归一 | A→B 合并后归入同一条以存活 id 为键的链；坏引用/两层链跳过并 warn | Go 纯函数 + 隔离 PG | `backend-go/internal/topicgraph/repository/compose_chain_test.go`；`backend-go/internal/topicgraph/repository/daily_report_manual_topic_test.go` |
| 4 | 一区多事件 | 一个分区可同属多条事件线链 | 同一分区同时出现在两条链的成员里，不拆副本、不合并两条事件线 | Go 纯函数 | `backend-go/internal/topicgraph/repository/compose_chain_test.go` |
| 5 | 取数落地 | 事件线链不写入关系表 | 响应只多出内存构造的事件线链与回填标记；`daily_report_section_relations` 零新增边、原 relations 边集不变、零 DDL | 隔离 PG | `backend-go/internal/topicgraph/repository/daily_report_manual_topic_test.go` |
| 6 | 整组勾齐跨天材料 | 事件线链只提供整组勾选 | 一次补齐该链全部成员（去重合并）；不附带勾其它链成员或相似推荐；取消只撤销本链 | Vitest | `front/app/features/tags/composables/useLaneCompose.test.ts`；`front/app/features/tags/components/console/BoardComposeOverlay.test.ts` |
| 7 | 两链同时用 | 两条链并列且互不干扰 | 勾一条不动另一条的分组/跨度/资格；取消一条不动另一条独有成员 | Vitest | `front/app/features/tags/composables/useLaneCompose.test.ts` |
| 8 | 保存不越权 | 事件线链整组带入不改候选状态机 / 事件线链不产生或激活话题 | 只按既有 active/manual 路径建本次泳道话题；候选话题命中计数/连续命中/status 不变；分区原话题归属不改 | 隔离 PG + Vitest | `backend-go/internal/topicgraph/repository/daily_report_manual_topic_test.go`；`front/app/features/tags/composables/useLaneCompose.test.ts` |
| 9 | 未回填板降级 | 未回填板诚实降级 | 事件线维度禁用 + 「事件身份未回填，本板暂无事件线链」；不冒充「暂无候选」；话题维度不受影响 | Vitest | `front/app/features/tags/components/console/BoardComposeOverlay.test.ts`；`front/app/features/tags/composables/useLaneCompose.test.ts` |
| 10 | 部分回填 | 部分回填标记不完整 | 顶部提示「部分事件未回填，事件线链不完整」；未回填线索不伪造成独立事件线 | 隔离 PG + Vitest | `backend-go/internal/topicgraph/repository/daily_report_manual_topic_test.go`；`front/app/features/tags/composables/useLaneCompose.test.ts` |
| 11 | 相似推荐不被污染 | 事件线不进入相似推荐召回 | 同事件线但超阈值距离的分区不进召回集；阈值/排序/质心不变；不因事件线被标「相近」或自动勾选 | Vitest | `front/app/features/tags/composables/laneComposeLogic.test.ts`；`front/app/features/tags/composables/useInlineCompose.test.ts` |
| 12 | 走完可见故事 | 两条链并列且互不干扰 + 事件线链只提供整组勾选 | 「新建泳道 → 切事件线维度 → 整组勾齐 → 保存」全程可用，事件线组与话题组视觉可分辨 | 完整交互 | 人工：agent-browser 静态 :5100 走完整链路并截图取证，证据写 `ui-verification/report.md` |

任务验证节包含全部 29 个 delta Scenario→真实测试文件的逐行映射，继承项也须回归，不以主链路替代完整清单。

## 2. 继承与调整（⓪ 契约变更）

已运行 `bash scripts/harness/test-assets.sh workshop-lane-compose` 反查（8 Requirements / 36 Scenarios；历史映射来自 `openspec/changes/archive/2026-11-02-rework-workshop-into-radar-and-desk/tasks.md`）。本 change MODIFIED 三个 Requirement、ADDED 一个，按下表处置，其余不动。

| 旧 Scenario / 契约 | 处置 | 旧测试 | 动作 |
|---|---|---|---|
| 跨期连接而非连续出现、没连接或跨度不足 | 保持（话题链口径不变） | `backend-go/internal/topicgraph/repository/compose_chain_test.go` | 旧断言不改；新增事件线链的同门槛用例并列（新增另一组纯函数用例） |
| 采纳预填、取消整组与半选、人工名称不被覆盖、清空与取消本次 | 扩充（新增「另一条链」这一组参与者） | `front/app/features/tags/composables/useLaneCompose.test.ts` | 旧断言保留；补「两条链的组操作互不吞成员」「取消事件线链不动话题链成员」 |
| 已有泳道显式参考、历史启用资格 | 保持 | `front/app/features/tags/components/console/BoardComposeOverlay.test.ts` | 不动；事件线维度不得影响既有参照开关与启用资格 |
| 采纳后质量刷新、点击节点推荐关联、从推荐项加入、无关联、无距离数据、相近程度不等于已选 | 保持 + **负向新增** | `front/app/features/tags/composables/laneComposeLogic.test.ts` | 旧断言一字不改；新增「同事件线但超阈值不进召回、不改排序」负向用例 |
| 缺名称、保存成功 | 保持（事件线链不改变保存语义） | `front/app/features/tags/components/console/BoardComposeOverlay.test.ts` | 旧断言保留；补「按事件线链整组带入后保存仍只建 active/manual 话题」 |
| 编排态入口与全屏画布 / 编排日期范围 / 可读事件与人工补选 / 右栏职责与画布标题 / 窄屏禁用编排 | 保持 | `front/app/features/tags/components/console/BoardComposeOverlay.test.ts`、`front/app/features/tags/composables/laneComposeLogic.test.ts` | 本 change 不动这些 Requirement；事件线链沿用既有入口/窗口/右栏/窄屏口径（新增维度不新建页面、不改窗口、不占右栏） |

### 展示字段来源盘点（⑤）

| 字段 | 来源语义 | spec 锚点 |
|---|---|---|
| 事件线组标题 | `board_stories.title`（存活 id）；空标题降级「事件线 #ID」 | ADDED 事件线链 / MODIFIED 候选话题侧栏 |
| 「N 个分区 · 跨天 N 天 · 同一事件」 | 窗口内该事件线的成员分区数与首末 UTC 日差 | ADDED 事件线链 |
| 「事件身份未回填，本板暂无事件线链」 | 窗口内所有线索 `story_id` 为 NULL | ADDED 事件线链（未回填板诚实降级） |
| 「部分事件未回填，事件线链不完整」 | 窗口内部分线索已回填、部分仍 NULL | ADDED 事件线链（部分回填标记不完整） |
| 事件线链成员分区列表 | section→threads→`story_id` + `merged_into` 一跳存活解析 | ADDED 事件线链（链键按存活事件线归一） |
| 话题链组标题/元信息 | 既有 candidate 话题口径，不变 | MODIFIED 候选话题侧栏 |
| 相似推荐的距离/大小/深浅 | 已选成员质心 cosine（既有），不变 | MODIFIED 成员质心与关联推荐 |

## 3. 边界：五组变体走查

### 3.1 输入

| 变体 | 答案 |
|---|---|
| 空串 / 纯空白（全角/tab） | 事件线标题为空 → 降级「事件线 #ID」，不以空白标题冒充身份；group 标题用 `#ID` 形态 |
| 纯分隔符（单/连/首尾） | 标题不参与身份判定；链身份恒为存活事件线 id，不按标题分词造链 |
| 单 token | 事件线标题可短（如「关税」），不强凑字数，身份仍用存活 id |
| 大小写 | 标题大小写不影响链归属（按 id）；显示保留原标题 |
| 特殊字符/引号/HTML | Vue 文本转义，不用 v-html；后端参数化查询，不拼 SQL |
| 超长标题/理由 | 事件线标题过长在 UI 换行 + title/aria 承载完整标题，不撑宽列表；不按截断文本匹配 |
| 缺数字 vs 0 | `stories_backfilled=false` 是明确布尔，非缺省；缺字段（旧后端）按「服务端未升级」降级，不当 false 也不当 true |

### 3.2 前置

| 变体 | 答案 |
|---|---|
| 空集 | 板内无任何带 story 的分区 → 事件线链空；若同时未回填走「未回填」文案，若已回填但无跨天事件走「本板暂无可串联的事件线」 |
| 单元素 | 只有 1 个分区的事件线不成链（≥2 门槛） |
| 重复 | 同一分区在同一链内只出现一次；同一分区可同时在多条链内；同 id 的重复边/重复线索不重复计数 |
| 越界引用 | 坏 `story_id`（指向不存在行）、两层合并链、环 → 跳过该线索 + warn；不影响其它链；不按原始 id 另开一条链 |
| 部分满足 | 部分线索已回填、部分未回填 → 链只含已回填部分，并标「部分事件未回填，事件线链不完整」 |
| 全 composite / 人工拆分 | composite 线索无 `story_id`，天然不入链；人工拆分（`story_manual=true` 且 story_id NULL）不入链、不被自动重挂 |
| active 泳道分区 | 不入事件线链（成员域 = 非 active），仍以画布只读参照呈现 |
| 已归档来源分区 | 可入链（成员域含已归档来源），按既有归档来源标注口径显示原来源 |

### 3.3 时间窗口

| 变体 | 答案 |
|---|---|
| 两端 | 事件线链与候选/画布/推荐**同一生效窗口**（含首末日）；窗口外分区不入链 |
| 空窗口 | 无已完成日报 → 窗口开放但无分区 → 两条维度都空，走既有空态 |
| 跨窗口/跨月跨年 | 跨度按 UTC 日差算（与话题链同口径）；跨月跨年不特殊处理 |
| 归一化 | 固定 UTC 日界（与既有 `utcDayDiff` 一致），不依赖宿主 TZ |
| 事件线跨度 vs 窗口跨度 | 资格用**窗口内成员极差**；`board_stories.first_seen_date/last_seen_date` 只作展示，不用于判资格、不拉窗口外分区 |
| 边界值 | UTC 日差 = 2 不成链、= 3 成链；成员数 = 1 不成链、= 2 成链 |

### 3.4 幂等

| 变体 | 答案 |
|---|---|
| 重复执行 | 同一窗口重复取数返回同一组链（链键 = 存活事件线 id，稳定；成员按 (period_date, id) 升序，顺序稳定） |
| 部分失败重试 | 一次取数失败 → 事件线维度不可用 + 既有错误条；不写库、不落半成品链 |
| 并发 | 编排态取数只读；不与日报生成/归属写路径争锁；`story_id` 在归属后可能变化 → 下次取数自然反映，不缓存快照 |
| UI 重复激活 | 反复整组勾选/取消幂等（按成员 ID 集合运算），不重复计数、不残留半选态 |

### 3.5 可用性

| 变体 | 答案 |
|---|---|
| 误输入反馈 | 无用户输入表单，不适用；事件线链由系统分组，用户只能选不能改 |
| 空态 | 未回填 → 明确降级文案 + 维度禁用；已回填无跨天事件 → 「本板暂无可串联的事件线」；两者不混淆 |
| 错误态 | 取数失败沿用既有错误条与重试；不静默显示空链 |
| 加载态 | 沿用既有骨架；加载中维度切换禁用 |
| 超长文本 | 事件线标题换行 + title/aria 完整承载，列表不撑宽 |
| 重复提交 | 无写按钮不适用；保存路径不变 |
| 划选与批注 | 事件线组标题可划选，不触发画布定位或焦点抢占 |
| reduced-motion / 焦点 | 维度切换可 Tab 到达、Enter/Space 激活；事件线边线型变化不影响焦点顺序与焦点轮廓可见 |

## 4. 效果核对（实施后执行，不伪填结果）

**触发原因**：事件线链的可信度完全取决于 `story_id` 的**实际回填覆盖率**——覆盖率低时功能形同虚设，而这无法由纯函数测试证明；同理「同一事件跨天成链」的真实命中率也依赖归属判定质量。

**方法**：在静态 :5100 配合**只读** API 抽样（禁止写库 SQL 探测、禁止重建超窗报告）：对若干版块（优先选已回填板与未回填板各若干），记录：
1. 各板窗口内线索的 `story_id` 非空比例（分母 = 窗口内线索总数）；
2. 各板事件线链条数分布、成员分区数分布、跨天跨度分布；
3. 事件线链横跨多个 candidate 话题的比例（正交性实测：一条链覆盖 ≥2 个话题的占比）；
4. 与话题链的成员交集比例（两条线覆盖同一批材料的比例）。

**判定**：覆盖率低于可用阈值时，如实报告真实降级比例并说明「需先跑 `cmd/backfill-stories`」，**不掩盖为「无事件线」**。输出样本板、样本日期、计数、分母与结论入 `ui-verification/report.md`。当前结果**均未采集**。

## 5. 白盒附加（complex）

### 5.1 分支表

| 分支条件 | 路径 / 断言 | 最便宜层 |
|---|---|---|
| 线索 `story_id` 非空且未合并 | 进该存活事件线的分组 | Go 纯函数 |
| 线索 `story_id` 指向已合并行 | 一跳解析到存活 id，进存活链（不按原始 id 开新链） | Go 纯函数 |
| 线索 `story_id` 指向坏行 / 两层链 / 环 | 跳过该线索 + warn；其它链不受影响 | Go 纯函数 |
| 线索 `story_id` 为 NULL（含 composite、人工拆分） | 不入任何链 | Go 纯函数 |
| 线索 `scope=composite` | 不入链（无身份） | Go 纯函数 |
| 事件线成员数 = 1 / 2 | 1 → 不成链；2 → 视跨度 | Go 纯函数 |
| 成员首末 UTC 日差 = 2 / 3 | 2 → 不成链；3 → 成链 | Go 纯函数 |
| 成员分区归属话题 = active | 不入链（成员域排除） | Go 纯函数 |
| 一个分区命中多条事件线 | 同时进多条链，成员集合取并集，不拆副本 | Go 纯函数 |
| 板内全部 `story_id` 为 NULL | `stories_backfilled=false`，链空 | Go 纯函数 + 隔离 PG |
| 部分 NULL 部分已回填 | `stories_partial=true`，链只含已回填部分 | Go 纯函数 + 隔离 PG |
| 已回填但无跨天事件 | 链空但 `stories_backfilled=true`（文案区别于未回填） | Go 纯函数 |
| 链键稳定性 | 同一事件线在窗口平移后键不变（`story:{survivorId}`） | Go 纯函数 |
| 链排序稳定性 | last_date desc → span_days desc → 存活 id asc | Go 纯函数 |
| 写入路径 | 组装链后 `daily_report_section_relations` 行数与 relation_type 分布不变 | 隔离 PG |
| 前端：勾事件线链 | 成员并入 `selectedIds`，按 ID 去重；话题链成员不动 | Vitest |
| 前端：取消事件线链 | 只移除本链成员；话题链成员与人工补选保留 | Vitest |
| 前端：相似推荐 | 同事件线但超阈值的分区不进召回；既有排序断言不变 | Vitest |

### 5.2 边界值清单

- 成员分区数 0 / 1 / 2 / 3；UTC 日差 0 / 1 / 2 / 3 / 10；成员分区落在窗口首日/末日（两端都算）；中间断档 1 天 / 2 天 / 5 天。
- 链成员数 1 / 2 / 20 / 50（上限不截断，但列表折叠展示）；成员排序按 (period_date, id) 升序，同日多分区按 id 升序。
- `story_id`：NULL / 0 / 不存在的 id / 已合并 id / 两层链 id；存活 id 重复出现多条线索（不得重复建链）。
- `stories_backfilled`：false / true；`stories_partial`：false / true；旧后端响应缺全部新字段（降级为「服务端未升级」，不炸）。
- 事件线标题：null / 空串 / 纯空白 / 超长（>200 字符）；标题含 HTML 片段（转义不渲染）。

### 5.3 不适用留痕

- **无新 DDL / 数据迁移**：事件线链只在查询期内存构造，不新增表列、不写关系表 → 相关迁移/回填用例不新增；既有「零 DDL」由任务 1.2 的 `git diff --stat -- backend-go/internal/platform/database` 为空证明。
- **无新用户输入表单**：事件线链由系统分组、用户只能选 → 输入类「误输入/校验」用例不新增；既有名称校验用例（缺名称）保留。
- **无新路由 / 弹层 / 页面**：沿用既有编排态 workspace 布局 → 窄屏用例（窄屏提示）保留不动，不新增窄屏编排用例。
- **无新并发写路径**：编排态取数只读 → 「并发线程安全」用例不新增；既有日报生成并发用例不涉及本 change。
- **无新的相似度阈值标定**：相似推荐阈值与排序一字不改 → 阈值标定类用例不新增，既有 M6–M9/M11/M12 保留。
- **无 LLM 参与**：事件线链组装是确定性纯函数 + 批量查询 → 不新增 LLM mock / 重试契约用例。
- 复杂逻辑的隔离 PG fixture 与一次性容器沿用既有仓储测试纪律，**不用 SQLite** 代替。
