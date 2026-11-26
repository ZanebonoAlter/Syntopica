# 新建泳道编排 — 产品实现用例账本（2026-10-31）

> 契约：[`lane-compose-implementation-contract.md`](lane-compose-implementation-contract.md)。delta spec：[`specs/workshop-lane-compose/spec.md`](specs/workshop-lane-compose/spec.md)。本文是**故事层账本**（spec Scenario = 断言片段），白盒分支表在 §3。
> 层次约定（见 `standard/shared/test-design.md`）：纯逻辑 → 函数单测；SQL/日期窗 → testcontainer PG（repository 禁 SQLite）；HTTP → handler；组件 → Vitest；完整交互故事 → agent-browser 端到端（前端交互 change 主链路至少一个 agent-browser 落点或人工豁免）。

## 1. 故事层：Scenario → 落点

| # | 故事（Scenario 组） | 落点文件 | 层 |
| --- | --- | --- | --- |
| S1 | 编排态入口与全屏画布 | `front/app/features/tags/components/console/BoardComposeOverlay.test.ts` | 组件 |
| S2 | 编排日期范围 | 同上 + `backend-go/internal/topicgraph/handler/daily_report_handler_test.go` | 组件 / handler |
| S3 | 候选话题侧栏 | `backend-go/internal/topicgraph/repository/compose_chain_test.go` + `front/app/features/tags/composables/useLaneCompose.test.ts` | 单测 / 组件 |
| S4 | 可读事件与人工补选 | `front/app/features/tags/components/console/BoardComposeOverlay.test.ts` | 组件 |
| S5 | 成员质心与关联推荐 | `front/app/features/tags/composables/laneComposeLogic.test.ts` | 单测 |
| S6 | 右栏职责与画布标题 | `front/app/features/tags/components/console/BoardComposeOverlay.test.ts` | 组件 |
| S7 | 保存与只读边界 | 同上 | 组件 |
| S8 | 窄屏禁用编排 | 同上 | 组件 |

## 2. 变异体走查（五组固定清单）

### 2.1 输入

| 变体 | 答案 |
| --- | --- |
| 空串 / 纯空白（名称） | 拒绝创建，保留成员与选中态（SHALL 报错，不执行创建） |
| 空串 / 非法日期（`from`/`to`） | 400 明确报错，**保留旧范围与草稿**，控件恢复原值 |
| 单 token 名称 | 允许（非空非纯空白即可） |
| 超长标题（>120 字） | 画布标题截断加省略号，悬停浮层给全文；候选卡标题截断 |
| 特殊字符（`&<>"'`） | 按纯文本转义渲染，不得破 DOM |
| 搜索关键词无匹配 | 空态文案，手动补选入口仍在 |
| 单边 `from`/`to` | 400 |

### 2.2 前置

| 变体 | 答案 |
| --- | --- |
| 空集（窗口内 0 section） | 候选栏空态、画布空态、推荐空态，保存按钮禁用（无成员） |
| 单元素（窗口内 1 section） | 该 section 无连线 → 不构成候选链；仍可从「按标题找」人工选入 |
| 重复（同一候选链成员重复提交） | Set 去重，成员数不变 |
| 越界引用（草稿成员不在当前窗口） | 切窗口先确认草稿；确认后清空重算，不把越界成员混进质心 |
| 部分满足（日期窗一半合法） | from>to / 超窗 → 400，不应用 |

### 2.3 时间窗口

| 变体 | 答案 |
| --- | --- |
| 边界两端 | 窗口**含首末报告日**（from 当天、to 当天都计入） |
| 空窗口 | 无已完成日报或窗口内无数据 → 空数组 + 空态 |
| 跨窗口 | 自选区间完全落在报告日之外 → 空数组 |
| 归一化 | `from`/`to` 按日历日闭区间解释，秒级时间戳忽略 |
| 末日锚点 | `days` 档以**最新已完成日报**为末日；`latest_completed_date` 在响应 `window` 里回显 |

### 2.4 幂等

| 变体 | 答案 |
| --- | --- |
| 重复执行（反复点候选标题） | 第二次 = 取消该组（不是幂等追加） |
| 部分失败重试（保存 5xx） | 保留成员与名称，错误可重试，不产生半状态 |
| 并发 | 保存走既有单事务 `CreateManualTopic`；本轮不改其事务边界 |

### 2.5 可用性（UI 必检前三）

| 变体 | 答案 |
| --- | --- |
| 误输入反馈 | 空名 / 非法日期 / 越界窗各有明确错误文案 |
| 空态 | 候选栏 / 画布 / 推荐 / 搜索各有空态 |
| 错误态 | 取数失败显示错误与重试入口，不静默空列表 |
| 加载态 | 取数中有骨架/加载态 |
| 超长文本 | 标题截断 + 悬停全文 |
| 重复提交 | 保存中按钮禁用，防重复创建 |

## 3. 白盒附加：分支表与边界值

> 断言判据由主线程定（2026-10-31）。实现线程按此写用例，**判据不得自行放宽**。

### 3.1 候选链算法（后端唯一实现）

输入：sections（`persistent_topic_id`、`status`、`period_date`）+ relations（`from_id`/`to_id`/`relation_type`）。

| 分支 | 输入 | 期望 |
| --- | --- | --- |
| C1 空输入 | sections=[] | 返回 `[]`，不 panic |
| C2 全无归属 | 所有 section `persistent_topic_id = null` | 返回 `[]` |
| C3 非 candidate 状态 | topic status=active/archived | **不进候选链** |
| C4 单节点分量 | 只有 1 个 section，无边 | 不合格（节点≥2） |
| C5 两节点跨度 3 天 | A@10-27 — B@10-30 有 identity 边 | **合格**，`span_days=3` |
| C6 两节点跨度 2 天 | A@10-27 — B@10-29 | **不合格** |
| C7 中间断档 | A@10-27 — B@10-30，10-28/29 无事件 | **合格**（断档允许） |
| C8 仅 similarity 边连通 | 同 topic_id，similarity 边连接 | **合格**（原 identity/similarity 边皆可） |
| C9 跨 topic_id 的相似边 | topic 1 的 section — topic 2 的 section 有相似边 | **不得合并**：各自按自己组内连通分量判定 |
| C10 同 topic 不连通 | topic 1 有两个互不连通的分量 | 输出**两条**候选链，不得混选 |
| C11 跨度按 UTC 日 | 首末跨月（09-30 → 10-02） | `span_days=2`（UTC 日差） |
| C12 key 稳定 | 同输入两次调用 | `key` 相同（`topic_id:最小 section id`） |
| C13 排序 | 多链 | `last_date` 倒序 → `span_days` 倒序 → `topic_id` 升序 |
| C14 关系缺端点 | relation 指向窗口外 section | 忽略该边，不 panic |
| C15 重复边 | 同对同 type 重复行 | 不影响连通性判定 |

### 3.2 日期窗校验（handler）

| 分支 | 输入 | 期望 |
| --- | --- | --- |
| D1 仅 `from` | `from=2026-10-01` | 400 |
| D2 仅 `to` | `to=2026-10-30` | 400 |
| D3 倒置 | from=10-30, to=10-01 | 400，**旧范围与草稿保留** |
| D4 超 90 天 | from=2026-01-01, to=2026-10-30 | 400，**不得静默截成 90 天** |
| D5 窗外起点 | from 早于「最新已完成日报 − 89 天」 | 400 |
| D6 `to` 晚于最新日报 | from=2026-10-20, to=2026-12-31（跨度 72 天，仍在 90 天窗内） | **允许**（窗口内无更晚数据） |
| D3b 规则优先级 | from=2026-10-01, to=2026-12-31（跨度 92 天，同时触发「超 90 天」与「to 晚于最新日报」） | **400**（跨度规则优先）；两条规则同触发时按「跨度 → 窗外 → 其它」顺序命中 |
| D7 格式非法 | `from=2026-13-01` | 400 |
| D8 正常 | from=2026-10-01, to=2026-10-30 | 200，`window.from/to/days` 回显正确 |
| D9 兼容 | 只给 `days=30` | 200，行为与现状一致（末日 = 最新已完成日报，含首末） |
| D10 days 上限 | `days=365` | 沿用现状：截到 90（**仅 days 档**；自选档不截断而是 400） |
| D11 无已完成日报 | 版块无 completed 日报 | 200 + 空数组（不 404） |

### 3.3 质心与推荐（前端纯函数）

| 分支 | 输入 | 期望 |
| --- | --- | --- |
| M1 空已选 | chosen=∅ | 返回空推荐，`center=null`，**不伪造距离** |
| M2 单成员 | chosen={A} | center = A 的向量（不是别的来源） |
| M3 多成员均值 | chosen={A,B} | center = (A+B)/2，逐维校验（容差 1e-9） |
| M4 向量长度不齐 | A 长 8、B 长 4 | 只用与首个有效向量**同长**的成员参与平均 |
| M5 含非有限值 | A 含 NaN | A 视为无效，不参与平均，不污染结果 |
| M6 距离边界 | distance == threshold | **保留**（≤ 阈值） |
| M7 超过阈值 | distance > threshold | 过滤掉 |
| M8 稳定排序 | 两个 section 距离相同 | 按 section id 升序 |
| M9 分组 | 一个 active、一个 candidate 同距 | candidate 在 `unassigned` 优先组，active 在 `active` 次级组 |
| M10 焦点不影响 | 只点某节点不选它 | center 不变（焦点不作为质心来源） |
| M11 增删重算 | 加入一个推荐项 | center 用**新**成员集合重算，被加入项从待选移除 |
| M12 无向量 section | 某 section embedding 为 null | 不参与距离计算，不报错 |

### 3.4 选择状态与命名（前端）

| 分支 | 输入 | 期望 |
| --- | --- | --- |
| S1 空组 | ids=[] | unchecked（不是 indeterminate） |
| S2 全选 | count==total | checked |
| S3 半选 | 0<count<total | indeterminate |
| S4 整组补齐 | 组 3 成员 + 已有组外 2 成员 | 结果 5 成员，**去重**，不截 20 |
| S5 不补相似 | 整组补齐 | 只加组内成员，**不自动加相似推荐** |
| S6 取消组 | 取消整组 | 组内成员移除，**组外人工补选保留** |
| S7 自动名跟随 | 选组 A → 自动名=A | 取消组 A 且名字未被人工改写 → 自动名清空 |
| S8 人工名保护 | 人工改名后取消组 / 历史带入 | 人工名**保留** |
| S9 清空已选 | 清空全部成员 | 成员清空、**手写名保留** |
| S10 历史名单 | 打开历史管理候选 | 启用资格沿服务端 `can_activate`，本界面不改状态、不改门槛 |

### 3.5 画布与节点卡（前端组件）

| 分支 | 输入 | 期望 |
| --- | --- | --- |
| V1 1:1 比例 | 容器宽 W | `viewBox` 宽 == 容器 `clientWidth`（误差 <1px） |
| V2 重排 | 改窗口宽度 | 重新布局重绘，比例仍 1.0 |
| V3 不越界 | 任意视口 | 无元素越出画布右/下边界（标题、编号、横条） |
| V4 半径下限 | 普通点 | 半径 ≥3px（只读参照 ≥2.5px） |
| V5 已选恒大于推荐 | 有推荐点 | `selected.radius > recommend.radius`（严格大于） |
| V6 话题带 | 多个候选话题 | 每话题一行标题，各行不压字；点成员与话题带对应 |
| V7 话题过多 | 超过可读行数 | 显示「另有 N 个候选话题在左侧候选栏」 |
| V8 悬停浮层 | 悬停未归属散点 | 显示完整标题、日期、质心距离、原关系条数 |
| V9 卡片钉住 | 点选节点 | 卡片 pinned、跟随节点；移开鼠标不消失 |
| V10 快捷选入 | 卡内「＋选入」 | 成员真的加入、按钮翻成「移出」、图上✓出现 |
| V11 快捷移出 | 卡内「移出」 | 成员真的移除、按钮翻回 |
| V12 取消口子 | 卡上 × 或再点同一节点 | 卡片收起、焦点清空、右栏线索清空 |
| V13 active 只读 | 点 active 泳道的点 | 卡片**无**选入动作 |
| V14 已有泳道默认隐藏 | 初始 | active 点不画；显式开启才显示且只读 |
| V15 开关偏好不被偷改 | 取消聚焦 / 查看已有归属 | 不自动开/关已有泳道开关 |
| V16 窄屏 | 375 / 390px | 宽屏提示，无横向溢出 |
| V20 节点卡不塌扁 | 放大/平移后聚焦右缘节点 | 卡片宽度固定（≥240px 实测）、内容高时卡内滚动不撑竖条、仍贴在节点旁 |
| V19 画布拖拽平移 | 按住左键拖画布 | 内容跟着指针走（滚动位置与位移反向）、位移 <4px 仍算「点」（节点点击照常生效）、拖完那一下 click 不误选节点、节点卡与缩放控件不参与平移 |
| V18 画布滚轮缩放 | 画布上滚轮 / 缩放读数 | 读数随滚轮变（0.5×–3×）、100% 时 1:1 仍成立、等比缩放不重排日期列、缩放后点渲染半径 ≥3px、节点卡仍贴节点、重置回 100%、滚轮不触发页面滚动 |
| V17 相关节点标题 | 选中成员后 | 相关 = 1 跳原关系 ∪ 质心较近的**可补充**推荐（右栏同一批）；可画的相关节点在节点下显示标题缩略（≤10 字、带省略号）；只读（已有泳道）相关节点改为底部提示行点名、不逐点画标题；无选中时不画；已选成员自身不算；既无原关系又不达推荐阈值的不画；不越界、冲突先向下错行仍冲突才宁缺勿叠 |

### 3.6 相关节点标题（前端纯函数 + 组件，2026-11-01 用户补充）

| # | 分支 | 期望 |
| --- | --- | --- |
| R1 | 无已选成员 | 相关节点集合为空，不画任何相关标题、不出现提示行 |
| R2 | 已选成员的直接原关系邻居 | 进入相关集合（`identity` 与 `similarity` 都算） |
| R3 | 隔一层的节点（传递闭包） | **不**进入相关集合（只取 1 跳） |
| R4 | 已选成员自身 | **不**进入相关集合（用户要的是「其他」相关节点） |
| R5 | 畸形边（缺一端 id） | 不产出脏 id |
| R6 | 相关节点标题为长标题 | 缩略后 ≤10 字且带省略号（`truncateText`） |
| R7 | 标题落位 | 只放在节点下方（y > 节点 y）；越出画布上下左右边界则跳过 |
| T1 | 相邻日期列的两个相关节点 | 向下**错行**（行高 16px）而不是互相压字、也不是直接丢掉 |
| T2 | 错行用尽仍冲突 | 宁缺勿叠（跳过该条），不压字 |
| R8 | 相关节点属于已有 active 泳道（只读梯子） | **不**逐点画标题（梯子是 10px 密排，CJK 行高 13px，硬放就是压字）；改为底部提示行点名（标题缩略 + 「在已有泳道里」说明） |
| R9 | 质心较近但**未达推荐阈值**的节点 | 不算相关节点（相关只吃右栏「可补充的事件」同一批 + 1 跳原关系） |
| R10 | 达到推荐阈值但属于 active 只读参照的节点 | 不进画布相关集合（右栏也不列它），不进提示行 |
| R11 | 相关节点里**已选成员自己** | 排除（用户要的是「其他」相关节点） |

> 落点：`front/app/features/tags/composables/laneComposeLogic.ts`（`relatedNodeIds` / `placeRelatedTitles` 纯函数）、
> `front/app/features/tags/components/compose/LaneComposeCanvas.vue`（落位、渲染与提示行）、
> 用例：`laneComposeLogic.test.ts`（R1–R5、T1–T2）、`BoardComposeOverlay.test.ts`（R6–R8 与组件级断言）。

### 3.7 画布缩放（前端纯函数 + 组件，2026-11-01 用户补充）

| # | 分支 | 期望 |
| --- | --- | --- |
| Z1 | 滚轮放大/缩小 | 缩放读数随之变化，范围夹在 0.5×–3× |
| Z2 | 缩放档位 | `zoom = 100%` 时 `viewBox` 宽 == 容器 `clientWidth`（1:1 契约），缩放**不改 viewBox**、只改元素尺寸（日期列不重排） |
| Z3 | 缩小时 | 渲染半径仍 ≥3px（普通点）/ ≥2.5px（只读参照） |
| Z4 | 缩放后节点卡 | 仍贴在节点旁并跟随（水平距离 <200px） |
| Z5 | 缩放边界 | 放大后无元素越出画布边界 |
| Z6 | 重置 | 回到 100%，SVG 尺寸复原 |
| Z7 | 滚轮副作用 | 画布上滚轮 `defaultPrevented`（不把页面滚走） |
| Z8 | 缩放不改语义 | 选择、质心、候选、相关节点口径不随缩放变化 |

> 落点：`front/app/features/tags/composables/laneComposeLogic.ts`（`clampZoom`/`zoomStep`/`zoomAnchorScroll`/`scaledPointRadius` 纯函数）、
> `front/app/features/tags/components/compose/LaneComposeCanvas.vue`（滚轮监听 + 读数/重置控件 + 卡片跟随）、
> 用例：`laneComposeLogic.test.ts` Z1–Z4、`LaneComposeCanvas.test.ts` Z1–Z7、真机 E2E E16.1–E16.7。

### 3.8 画布拖拽平移（前端纯函数 + 组件，2026-11-01 用户补充）

| # | 分支 | 期望 |
| --- | --- | --- |
| P1 | 拖动 | 滚动位置与指针位移**反向**（拖内容，滚动条跟着反向走） |
| P2 | 位移未超阈值（4px） | 算「点」不算「拖」→ 不改变滚动位置，节点点击照常 |
| P3 | 拖拽刚结束的那一下 click | 不选中节点（250ms 内压掉） |
| P4 | 指针捕获时机 | `pointerdown` **不**捕获；判定为拖的那一刻才捕获（否则节点点不中） |
| P5 | 节点卡 / 缩放控件上起手 | 不触发平移（卡上的按钮照常点） |
| P6 | 平移后的语义 | 选择、候选、质心、相关节点口径都不变 |

> 落点：`front/app/features/tags/composables/laneComposeLogic.ts`（`panScroll`/`isDragBeyond` 纯函数）、
> `front/app/features/tags/components/compose/LaneComposeCanvas.vue`（pointer 事件 + 抓取时机 + 光标）、
> 用例：`laneComposeLogic.test.ts` P1–P2、`LaneComposeCanvas.test.ts` P1–P3、真机 E2E E17.1–E17.3。

### 3.9 节点卡宽度固定（2026-11-01 用户报障的塌扁 bug）

| # | 分支 | 期望 |
| --- | --- | --- |
| C1 | 聚焦靠近画布右缘的节点 | 卡片宽度**固定**（300px），不被「容器宽 − left」夹扁 |
| C2 | 定位计算 | 用常量宽度，**不用**渲染后量出来的宽度（反馈环） |
| C3 | 内容超高 | 卡片高度封顶 + 卡内滚动，不撑出超长竖条 |
| C4 | 放大 / 平移后 | 卡片仍固定宽度、仍贴在节点旁（真机 E16.5 / E16.8） |

> 实机复现：1920 宽视口 + 126% 缩放 + 右缘节点 → 卡片被量成 92px 宽、296px 高，标题一个字一行（用户报「超长显示 bug」）。
> 修法：`CARD_WIDTH` 常量参与定位 + 样式固定宽度/高度封顶；回归锚见 `LaneComposeCanvas.test.ts` Z8（源码锚）与真机 E16.8（真实浏览器量宽）。

## 4. 验收命令（按影响范围，不做全量）

```bash
# 后端（影响包）
cd backend-go && golangci-lint run ./... && go vet ./... \
  && go test ./internal/topicgraph/... && go build ./...

# 前端（影响文件；参数不带 --）
cd front && pnpm lint && pnpm exec nuxi typecheck \
  && pnpm test:unit app/features/tags/composables/laneComposeLogic.test.ts \
                app/features/tags/composables/useLaneCompose.test.ts \
                app/features/tags/components/console/BoardComposeOverlay.test.ts \
                --maxWorkers=2
# pnpm build 单独跑，不与其它重活并行（树莓派 4 核 + SD 卡）
```

## 5. 人工豁免与未覆盖（须留痕，不得静默跳过）

| 项 | 处置 |
| --- | --- |
| 真实 POST 保存与事务回滚 | **人工**：在真后端以真实窗口数据创建一次 active/manual 话题，核对 `source=manual`/`status=active`/初始向量=均值；失败路径看事务是否整体回滚 |
| 旧 Centroid 健康与回填 | **人工**：核对既有后台滚动 Centroid 未被本轮改动；本轮质心是**前端草稿计算**，不落库 |
| 跨 active 移入 / 永久人工边 | **不在本轮范围**，保持关闭 |
| `test-assets.sh workshop-lane-compose` | 该 capability 目前只在本 change 的 ADDED delta 存在（主 specs 与 archive 均无），反查命令报「无匹配」属**新增能力的预期**，须在归档时以「capability 尚未归档」留痕，不记绿 |
