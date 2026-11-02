# 新建泳道编排 — 产品实现契约（2026-10-31）

> 依据：原型 `ui-prototype/revision-2026-10-30-lane-compose-selection/standalone.html`（用户 2026-10-31 明确「原型通过了，按规范开始实现吧」）＋ 阶段合同 [`ui-design-lane-compose-draft.md`](ui-design-lane-compose-draft.md) ＋ delta spec [`specs/workshop-lane-compose/spec.md`](specs/workshop-lane-compose/spec.md)。
>
> **本文件是后端线程与前端线程共用的唯一口径来源**。两侧都按本文件的字段名/口径实现，不得各自发明。原型是**离线样本**（2026-10-30 冻结的 90 天真实数据切片），产品必须**真取数、真算质心**，不得把样本塞进代码或接口。

## 0. 批准与范围

- 批准范围**仅限新建泳道编排工作面**。总览/矩阵既有批准与其冻结基准不变；归档视图线（`ui-design-archive-rework.md`）仍未批准，不因本次批准扩批。
- 本轮**不改**：候选跨 3 天门槛（连接跨度 ≥3 天，非连续命中）、质心取当前全部有效已选成员均值、`match_threshold` 默认 0.3（读 `ai_settings`）、人工建话题落 `active/manual`、候选状态机（`planLifecycle` 只更新命中计数）、历史 section 快照不回填。
- 本轮**不做**：跨正式泳道移入、永久人工关系边、后台质心回填、候选/归档自动变更。

## 1. 后端 API 契约

### 1.1 `GET /api/semantic-boards/:id/persistent-topics/compose-candidates`

沿用现有端点（`handler/daily_report_handler.go:composeCandidates` → `repository.GetComposeCandidates`），**向后兼容扩展**。

**Query**

| 参数 | 类型 | 语义 |
| --- | --- | --- |
| `days` | int | 快捷档窗口天数（7/14/30/90）。**以最新已完成日报为末日、含首末报告日**（现有语义不变：末日 − (days−1)）。`days>90` 截到 90；`days<=0` 沿用既有「近似全历史」语义。**后端不传时默认 0（≈全历史）**，「默认近 30 天」由**前端快捷档显式传 30** 保证，后端保持向后兼容。 |
| `from` | `YYYY-MM-DD` | 自选起始日（闭区间）。给出时 `days` 被忽略。 |
| `to` | `YYYY-MM-DD` | 自选结束日（闭区间）。 |

**校验（失败一律 400 + `{"success":false,"error":"..."}`，不静默纠正）**

1. `from`/`to` 只给其一 → 400。
2. `from` > `to` → 400。
3. `to − from` > 89 天（超过 90 天支持窗）→ 400，**不得静默截成 90 天**。
4. `from` 早于「最新已完成日报 − 89 天」（取数窗外）→ 400。
5. `to` 晚于最新已完成日报 → **允许**（窗口内本来就没有更晚数据），不算错误。

**响应**（`{"success":true,"data":{...}}`，在现有 `sections` / `match_threshold` 之外**新增**三个字段）

```jsonc
{
  "sections": [ /* 不变：id, report_id, period_date, cluster_label, embedding,
                    persistent_topic_id?, topic_match_confidence?, persistent_topic? */ ],
  "relations": [
    { "from_id": 7345, "to_id": 7409, "distance": 0.12, "relation_type": "identity" }
  ],
  "candidate_chains": [
    {
      "key": "1984:7345",
      "topic_id": 1984,
      "topic_label": "以色列议会第26届议会选举投票，选出120名新议员",
      "topic_status": "candidate",
      "section_ids": [7345, 7409, 7514],
      "first_date": "2026-10-27",
      "last_date": "2026-10-30",
      "span_days": 3
    }
  ],
  "match_threshold": 0.3,
  "window": { "from": "2026-10-01", "to": "2026-10-30", "days": 30, "latest_completed_date": "2026-10-30" }
}
```

- `relations`：**窗口内两端都可见**的原关系全集（`identity` + `similarity` 两类都保留，`relation_type` 原样带出）。画布只画这批边，**不得新增/推断边**。
- `candidate_chains`：**服务端唯一实现**（口径见 §2.1），前端只渲染不再重算。
- `window.days`：**生效**窗口天数（闭区间，含首末）。`days` 档：`days>90` 回显 **90**（截断后的生效值，不回显请求的 365）、`days<=0` 回显 **0**（≈全历史）；`from`/`to` 档：回显 `to-from+1`。**回显生效值而非请求值**，与同对象 `from`/`to` 自洽（审核 M1 定稿口径）。
- `window.from` / `window.to`：**可能为 `null`**（不是缺省、不是空串），表示**开边界**：
  - 「无已完成日报」→ from/to 均为 null，`days` 回显请求值，**`sections`/`relations`/`candidate_chains` 均为 `[]`**（**不 404**）；
  - 「`days<=0` 全历史档」→ from/to 均为 null、`days=0`，**数据为全历史范围**（不限 90 天窗）。
  前端需按 `string | null` 归一化（审核 L1）。
- `window.latest_completed_date`：该版块**最新已完成日报**的日期；**该版块没有已完成日报时**为 `null`（全历史档但存在已完成日报时仍回显该日期）。

### 1.2 既有端点（不改语义）

- `POST /api/semantic-boards/:id/persistent-topics/manual`：人工建话题，已是 `source=manual` + `status=active` + 均值初始向量 + 单事务。**本轮不改**。
- `GET /api/semantic-boards/:id/persistent-topics`（管理名单）：**本轮不改**，管理可见门槛仍按累计命中 `can_activate`。
- 主题管理 PATCH（转 active/归档）：**本轮不改**，仍只能由用户显式操作触发。

## 2. 口径定义

### 2.1 候选链（服务端唯一实现）

输入：窗口内 sections（含 `persistent_topic_id` / `period_date`）+ 窗口内 relations。

> **前提**：候选链的输入 section 集合 = 窗口内**带可用 embedding** 的 section（端点既有过滤：`embedding IS NOT NULL` 且向量能解析）。无向量的 section 既不进链、也不进画布可选池（审核 L2）。

1. 只取 `persistent_topic.status == 'candidate'` 且 `persistent_topic_id != null` 的 section。
2. 按 `persistent_topic_id` 分组；**不同 topic_id 不得因相似边合并**成一个候选。
3. 组内沿**原 relations**（`identity` 或 `similarity` 皆可）取**无向连通分量**。
4. 分量内节点按 `(period_date, id)` 升序；`first_date`/`last_date` 取首末 section 的 `period_date`（UTC 日）。
5. **合格条件**：`len(members) >= 2` 且 `span_days = (last_date − first_date).UTC 日差 >= 3`。中间允许断档。
6. 一个 topic_id 的多个不连通分量 → **各自成为独立候选链**，不得混选。
7. `key` = `"{topic_id}:{分量内最小 section id}"`（稳定、可排序）。
8. 排序：按 `last_date` 倒序 → `span_days` 倒序 → `topic_id` 升序。

> 判定示例（真数据）：`1984` 的 `7345@10-27 / 7409@10-28 / 7514@10-30`（10-29 断档，跨度 3 天）合格；`1914` 的 `7107@10-23 / 7344@10-27`（跨度 4 天）合格；`1702` 的 `6343@10-09 / 6677@10-15`（跨度 6 天）合格。跨度 2 天或无连接的**不合格**。

### 2.2 质心与推荐（前端计算，沿用端点既有设计）

端点回传 embedding 就是为了让前端实时算（现有注释即此意），**不在服务端再实现一套**。

- `centroid = mean(所有有效已选成员的 embedding)`。有效 = 长度相同且全为有限数。**不是**首个成员、不是点击焦点、不是旧 candidate Embedding、不是后台滚动 Centroid。
- 对窗口内**未选**且有有效 embedding 的 section 算 `distance = 1 − cos(centroid, embedding)`。
- 保留 `distance <= match_threshold`（0.3，来自接口），按 `distance` 升序 → `section id` 升序稳定排序。
- 分两组：`unassigned`（`persistent_topic.status != 'active'`，含其他 candidate 与归档来源）为**优先组**；`active` 为**次级只读参照**（不可选入）。
- 无有效已选成员或向量不可比较 → **不伪造距离**，返回空推荐（不得因此取消已选成员）。
- 推荐**不生成关系边**，也**不是**置信度。

### 2.3 视觉口径（前端）

- 已选：**实心 + 白色 ✓**；推荐/普通点：**实心度与大小表达接近程度**。
- 推荐点半径 **恒小于**已选点半径（原型实现：`recommendCap = cap*0.72`；已选 `radius = max(min(cap,10), cap*0.85)`）。
- 普通点半径下限 3px（只读参照 2.5px）；画布按**容器真实像素 1:1** 绘制（viewBox 宽 = 容器 clientWidth），窗口尺寸变化后重排重绘。
- **任何标题、编号、横条不得越出画布边界**；标题避让冲突时宁缺勿叠（原型 `placeLabels`）。
- 话题档为默认呈现：顶部「话题带」每候选话题一行写标题，成员以细线指向所属话题；未归属散料点不常显标题，靠悬停浮层读全文。话题多于可读行数时**明确写出**「另有 N 个候选话题在左侧候选栏」。
- **相关节点标题（2026-11-01 用户补充，同日二次澄清）**：**相关节点** = 与已选成员**有原关系**（1 跳，`identity` 与 `similarity` 都算）的其它节点 ∪ **质心较近**的**可补充**推荐节点（= 右栏「可补充的事件」里**同一批**，`centroidRecommendations` 的可补充组；**不含** active 只读参照那 100 多个——它们既不在右栏、也不该在画布上抢戏）。两条都排除已选成员自己。有了选中成员后，相关节点分两路呈现：
  - **可画的相关节点**（非只读，在主画布区）：在**节点下方**显示**标题缩略**（≤10 字，带省略号）；
  - **只读的相关节点**（属于已有 active 泳道）：**不逐点画标题**（只读参照区是 10px 密排的「梯子」，CJK 行高 13px 放不下逐点标题，硬放就是压字），改为在画布底部**提示行里点名**（标题缩略 + 「在已有泳道里（默认隐藏）」说明）。
  - 共同规则：**只认原关系**（不新增、不推断）、**1 跳不做传递闭包**、**已选成员自己不当自己的相关节点**、标题只放「下方」一个位置、撞上已放标题时**向下错行**（最多 2 行）仍撞上才**宁缺勿叠**（跳过）、越出画布不画。

#### 画布缩放（2026-11-01 用户当场补充）

- **滚轮缩放**：在画布区域滚轮即缩放，**普通滚轮与 Ctrl+滚轮同等可用**（不得只绑 Ctrl）；范围 **0.5× – 3×**，步进约 **1.1**（滚轮一格 = 一个步进）；**以光标位置为锚点**（缩放后光标下的内容尽量不动）。
- **画布上滚轮不得把整页滚走**：滚轮监听必须 `{ passive: false }` 并 `preventDefault()`（缩放后纵向滚动改用容器滚动条）。
- **缩放只影响呈现**：不碰选择、候选资格、质心计算、相关节点口径、候选链口径；不碰后端与 API。
- **不重排日期列**：缩放时**逻辑画布坐标不变**（`viewBox` 固定为 `0 0 容器宽 画布高`），**整体等比缩放**（节点 / 标题 / 连线一起缩）——靠 SVG 元素尺寸实现，日期列位置不得漂移。
- **1:1 契约**：`zoom = 100%` 时 `viewBox` 宽 == 容器 `clientWidth`（误差 <1px，机械断言口径）；缩放不改变 `viewBox`。
- **点的像素下限**：缩小时按 `max(逻辑半径, 下限 / 缩放)` 补偿，保证**渲染半径**仍 ≥3px（只读参照 2.5px）——不许把点缩到看不见。
- **节点卡必须跟随**：节点卡是 HTML 浮层，定位用**画布像素**，缩放时**必须把缩放比例算进去**（否则卡片与节点错位）。
- **节点卡宽度必须固定（不许 shrink-to-fit 反馈环）**：绝对定位卡片默认是 shrink-to-fit，宽度会被「容器宽 − left」夹住；而 left 若又用「量出来的宽度」算，就形成**反馈环**：量到 92px → 位置更靠右 → 可用宽更小 → 量到 60px → … → 卡片塌成一条竖条、标题一个字一行（2026-11-01 用户实机报障）。因此：宽度用**常量**（`CARD_WIDTH = 300`）参与定位，**量出来的只取高度**；样式上 `width` 固定、`max-height: calc(100% - 12px)` + 卡内滚动（内容超高不撑出超长竖条）。
- **读数与重置**：画布右下角显示**当前缩放读数**（如「100%」）与**重置到 100%** 入口；样式沿用 `--color-*` token，**不新开浮层档位、不手写 z-index**。

#### 画布拖拽平移（2026-11-01 用户当场补充）

- **按住左键拖画布即可平移**（内容跟着指针走，滚动位置与指针位移**反向**），与滚轮缩放配套：放大后不用去拖滚动条。
- **拖 ≠ 点**：位移未超过阈值（4px）算「点」，节点的点击（选中 / 节点卡 / 快捷选入）**必须照常生效**；判定用纯函数 `isDragBeyond`。
- **拖拽刚结束的那一下 click 不算「点节点」**：拖完画布顺手点到的节点不得被误选中（用「最近一次平移时刻」压掉 250ms 内的 click）。
- **指针捕获只在真的拖起来时做**：`pointerdown` **不得**立刻 `setPointerCapture`——那会把随后的 `pointerup` / `click` 改派到画布，节点自己的点击就收不到了；改到「位移超阈值、判定为拖」的那一刻再捕获（拖出画布也要继续跟）。
- **节点卡与缩放控件不参与平移**（卡上的按钮要照常点）。
- 平移**只影响呈现**：不碰选择、候选资格、质心、相关节点口径。

### 2.4 交互口径（前端）

- 组复选框与成员复选框用**原生 checkbox**，三态映射 `unchecked` / `indeterminate` / `checked`，与成员计数、图上 ✓、保存可用性**同源派生**。
- 点候选标题一次**补齐该片段全部成员**：追加去重、**不截 20 条**、**不自动补相似推荐**、**不吞组外人工补选**。
- 再点全选标题 / 取消组框 / 「取消整组」→ 撤销该组全部成员，保留组外选择。
- 自动预填名**跟随来源组**；该组被取消且名字未被人工改写时一并清空；**人工写过的名字不得被任何预填或历史「带入草稿」覆盖**。
- 节点卡：悬停出卡（完整标题/日期/距离/原关系条数/归档来源）；**选中后钉在该点旁并跟随**；卡内「＋选入/移出」真实改成员并当场翻字；**卡上 × 或再点同一节点 = 取消选中**（卡片收起、右栏线索清空）；已有 active 泳道的点**只读不给选入**。
- 右栏**只有三段**：当前事件线索（只叙述，不放选入按钮）、可补充的事件、tab 区（已选事件 / 按标题找）。不得出现关联事件清单、计算依据折叠、话题泳道参照。
- 已有 active 泳道**默认隐藏**，仅用户显式开启时显示且只读；取消聚焦/查看已有归属**不得偷改**该开关偏好。
- 日期范围是一等状态：候选/图/推荐共用同一实际窗口；**有草稿切换范围先确认**（确认才清草稿重算，取消/Esc 恢复原控件、成员与推荐）；日期非法保留旧范围与草稿并报错。
- 命名在最后；名称空白拒绝创建并保留成员；保存走 `POST .../persistent-topics/manual`。

## 3. 前端落点

| 落点 | 动作 |
| --- | --- |
| `front/app/api/persistentTopics.ts` | 扩展 `getComposeCandidates`：支持 `{days?, from?, to?}`，补 `relations` / `candidate_chains` / `window` 类型 |
| `front/app/features/tags/composables/laneComposeLogic.ts`（新） | 纯函数：`centroidOf` / `cosineFor` / `centroidRecommendations` / `laneSelectionState` / `candidateSelection` / `estimateTextWidth` / `truncateText` / `boxesOverlap` / `placeLabels` / `pointAppearance`（从原型 `compose-logic.js` 逐字移植，**不含** `candidateChains`——候选链口径在服务端） |
| `front/app/features/tags/composables/useLaneCompose.ts`（新） | 编排态状态机：窗口、候选链、选中集合、焦点、质心推荐、命名、草稿确认 |
| `front/app/features/tags/components/console/BoardComposeOverlay.vue`（重写） | 三栏工作面宿主 |
| `front/app/features/tags/components/compose/LaneComposeCandidates.vue`（新） | 左栏候选泳道（原生复选框组/成员） |
| `front/app/features/tags/components/compose/LaneComposeCanvas.vue`（新） | 中央 SVG 画布（1:1 像素、话题带、散点、悬停卡） |
| `front/app/features/tags/components/compose/LaneComposeInspector.vue`（新） | 右栏三段 + 两 tab |
| `front/app/features/tags/composables/useInlineCompose.ts` | **不改**（`BoardThreadBrowser.vue` 在用，改语义会波及别处） |

> **旧能力去向**：旧 `BoardComposeOverlay.vue` 的「散装事件列表 + 相似补选 + 20 条上限」编排面被本次三栏工作面**主动替换**（同一挂载点 `BoardConsoleShell.vue`）。旧 `useInlineCompose.adopt` 的「相似补选 + 20 上限」语义**保留在 `useInlineCompose` 给线程浏览器用**，编排态不再复用。

## 4. 测试落点（Scenario → 文件）

| Scenario 组 | 测试文件 | 层 |
| --- | --- | --- |
| 候选链：跨期连接合格 / 跨度不足不合格 / 不连通分量不混选 | `backend-go/internal/topicgraph/repository/compose_chain_test.go`（新） | 纯函数单测 |
| compose-candidates：日期窗校验（单边 / 倒置 / 超 90 天 / 窗外）、relations 与 chains 返回、window 字段 | `backend-go/internal/topicgraph/handler/daily_report_handler_test.go`（追加） | handler |
| 质心与推荐：均值来源 / 阈值筛选 / 稳定排序 / 无向量不伪造 / active 次级 | `front/app/features/tags/composables/laneComposeLogic.test.ts`（新） | Vitest |
| 选择状态：三态映射 / 整组补齐去重 / 取消组保组外 / 自动名跟随 / 人工名不被覆盖 | `front/app/features/tags/composables/useLaneCompose.test.ts`（新） | Vitest |
| 编排态组件：右栏三段两 tab、节点卡钉住/快捷选入/取消、日期共窗与草稿确认、已有泳道默认隐藏 | `front/app/features/tags/components/console/BoardComposeOverlay.test.ts`（改） | 组件 |
| 窄屏提示 | 同上 | 组件 |

## 5. 红线（实现期必守）

1. **不得自动改状态**：`planLifecycle` 只更新命中计数；candidate→active、任意→archived 只能用户显式操作。
2. **管理门槛不下调**：管理名单可见性仍按累计命中 `can_activate`；编排候选与历史管理名单是两套用途，不得为统一列表而放宽。
3. **`candidate_decay_window` 只影响 prompt 注入**，不得自动归档/删除/改状态。
4. **历史 section 快照不回填**：`topic_status_at_report` 等快照不随后续状态变化改写。
5. **人工建话题 = `active/manual`**，跳过 candidate 连续命中门禁，不受相似距离/已有连线/候选跨度阻挡。
6. **不造边**：推荐不是关系；人工共同入选只是本次草稿成员，不等于持久事件关系。
7. **归档来源只显示不复活**：标明原归档泳道名称，任何状态恢复需单独显式流程。
8. **后端 review 用 `zai-coding-cn/glm-5.3:max`**，其余子线程用 `minimax-cn/MiniMax-M3.1-Flash:max`（用户 2026-10-31 当场指定）。
9. **树莓派不做全量**：只跑影响包/影响文件；前端 `pnpm test:unit <文件> --maxWorkers=2`（参数不带 `--`）；不与 `pnpm build` 并行。
10. **不写真库副作用**：验证 DDL/DML 一律 `BEGIN; …; ROLLBACK;`。
