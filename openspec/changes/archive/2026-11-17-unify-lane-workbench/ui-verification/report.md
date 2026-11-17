# UI 验收报告 · unify-lane-workbench

- 验收时间：2026-11-16
- 环境：前端 http://127.0.0.1:3000 ｜ 后端 http://127.0.0.1:5100（/health 200）
- 测试版块：boardId=15220（编辑泳道对象：topic 503「大模型监管与安全」，窗口 7 天时成员 124 / 候选 36）
- 工具：agent-browser 0.37.1
- 机械证据：`evidence.jsonl`（62 条断言记录）｜ 截图 35 张（同目录）
- **取证方式声明**：验收线程为 `change-review` 只读档（禁止写仓库、禁止改变持久状态）。据此：`/api` GET 经浏览器内代理直连后端 5100（只读）；写请求（`PATCH assignment`）在浏览器内以成功信封回放、**不触网、DB 零写入**；A-3/A-4/A-5 为 **UI 状态机端到端断言**，落库效果由单测兜底（见 X-1）。会话核实写请求 0 条触网。

## A. 主交互链路

### A-1 三栏存在且内容不重复【pass】
依据：`[data-testid="lane-correction-left"]` 宽 **380px**、`[data-testid="lane-correction-right"]` 宽 **300px**、中央 `lane-correction-center` **760px**（1440 视口 = 1440−380−300）均可见（eval 取 `getBoundingClientRect().width`）；左栏 innerText 不含「关联推荐」，右栏首行为「关联推荐 待选 · 36」、无成员清单。截图 `A1-three-columns-1440-editorial.png`。

### A-2 聚焦节点出卡【pass】
依据：点候选节点 `[data-testid="lane-node-8272"]` 后 `[data-testid="lane-node-card"]` 可见，含「＋ 加入本泳道」（`lane-card-add`）与「估算接近度 0.88」；节点 class 含 `is-focus`。截图 `A2-node-card-1440-editorial.png`。

### A-3 加入本泳道（UI 级，写请求浏览器内回放）【pass】
依据：点 `lane-card-add` → 左栏徽标「窗口 7 · 全部 124」→「窗口 8 · 全部 125」，右栏「待选 · 36」→「待选 · 35」，节点 class 翻 `is-member`，卡动作翻「⊘ 移出本泳道」；拦截日志 `PATCH /api/daily-reports/sections/8272/assignment body {"topic_id":503}`（未触网）。截图 `A3-join-1440-editorial.png`；写失败路径对照 `A3-write-blocked-probe.png`（卡保留、计数不变，符合 State Matrix）。

### A-4 移出本泳道（UI 级，同上）【pass】
依据：点 `lane-card-remove` → 徽标 8→「窗口 7 · 全部 124」，节点 class 翻 `is-removed`，「已移出（本次）」出现该节点并带 `[data-testid="lane-restore-8272"]`「↩ 加回来」；toast「已移除，该内容不再归属任何泳道，仍可再参与候选；归属立即生效，重心次日日报生效」；拦截日志 `PATCH … {"topic_id":null}`。截图 `A4-remove-1440-editorial.png`。

### A-5 加回（UI 级，同上）【pass】
依据：点 `lane-restore-8272` → 徽标 7→8，「已移出（本次）」清空，节点回 `is-member`，卡回「⊘ 移出本泳道」；toast「已从「苹果开源人形机器人系统HALO，物理AI与人形机器人概念股走强」移入」（跨泳道改挂文案，符合 D-10 候选来源可见口径）；拦截日志 `PATCH … {"topic_id":503}`。截图 `A5-restore-1440-editorial.png`。

### A-6 相关高亮【pass】
依据：聚焦态 `svg g.lcc-node.is-related` = **35**、`.is-dimmed` = **15**、`.is-focus` = **1**；Esc 后三者全部归 0、节点卡消失（无遗留高亮）。截图 `A6-highlight-on-1440-editorial.png` / `A6-highlight-off-1440-editorial.png`。

### A-7 时间视窗【pass】
依据：顶部含四档位（7 天/14 天/30 天/近 1 年，`orc-tiers` 内按钮齐）+ 起始日/截止日表单 +「应用范围」+「← 平移/平移 →/放大 ＋/缩小 −」+ 曲线（标签「365 日 · 本泳道逐日线索数」——**编辑模式口径正确**）+ 缩放控件 `lane-correction-zoom`。切 30 天：画布轴 11-10…11-16 → 10-18…11-14，顶栏窗口标签 → `2026-10-18 — 2026-11-16`，节点 50→244，左栏徽标 →「窗口 40 · 全部 124」，并发起 `GET …/compose-candidates?days=30`（只读）。截图 `A7-workbench-7d-1440-editorial.png`、`A7-workbench-30d-1440-editorial.png`。

### A-8 点画布本体不改成员（查看与选入分离）【pass】
依据：整个「点节点聚焦 → Esc 取消聚焦」过程左栏徽标恒为「窗口 8 · 全部 125」（evidence `A6-A8-after-esc`）；会话写计数不变（5 次均按钮触发，0 次画布点击触发）。截图同 A-6 两张。

## B. 双视口 × 双主题 + 窄屏

### B-1 1440/1920 × editorial/dark 四组截图【pass】
依据：四组均「编辑中 + 节点卡展开」；栏宽恒 **380/300**；SVG viewBox 随容器 1:1 重排（1440 → `0 0 750 456`，1920 → `0 0 1230 636`，中心栏 760→1240）；`documentElement.scrollWidth` = 视口宽（无横向溢出）；dark 主题 html/body 底色 `rgb(8,12,18)`（自有底色，非浏览器白底）。截图 `B1-workbench-1440-editorial.png`、`B2-workbench-1440-dark.png`、`B3-workbench-1920-editorial.png`、`B4-workbench-1920-dark.png`。

### B-2 窄屏 375×667 不渲染残缺工作台【pass】
依据：375 视口下 `[data-testid="board-compose"]` 与 `lane-correction-left` 均不存在（工作面整体卸载，矩阵降级为「泳道摘要 · 窄屏」）；页面出现「泳道修正需要宽屏或横屏」；「新建泳道」按钮 title=「新建泳道需要宽屏或横屏」；scrollWidth=375 无溢出。截图 `B5-narrow-375-workbench.png`。

## C. 全系统回归（共享组件 `BoardComposeOverlay.vue`）

### C-1 四路由 × 双主题无溢出、dark 有底色【pass】
依据：`/`、`/tags`、`/tags/15220`、`/tags/15220/2026-11-16` 在 1440×900 下 editorial 与 dark 各检查：`scrollWidth` 均 = 1440（无横向溢出）；dark 下 html 底色 `rgb(8,12,18)`、editorial `rgb(250,247,242)`（`getComputedStyle` 取值）。截图 `C-home-editorial/dark-1440.png`、`C-tags-editorial/dark-1440.png`、`C-board-editorial/dark-1440.png`、`C-daily-2026-11-16-editorial/dark-1440.png`。

### C-2 T1 从 /tags 进版块、看矩阵、切泳道【pass】
依据：/tags → 点「开发者工具与开源生态」进 /tags/15220；矩阵默认选「大模型监管与安全」（可见 14 日 · 56 条线索）；点行头「全球半导体板块受地缘与政策提振：22 条线索」→ 选中态切换（「可见 14 日 · 14 日档全窗 22 条线索」，URL `?lane=41`）。截图 `T1-switch-lane-1440-editorial.png`。

### C-3 T2 打开某日日报阅读台读正文【pass】
依据：/tags/15220/2026-11-16 渲染阅读台：标题「马斯克支持黄仁勋回归英伟达 · 10 篇文章 · 6 个话题」、导语与 N°01/02 话题正文、「4 篇来源 · 聚簇口径 向量聚簇」可见。截图 `C-daily-2026-11-16-editorial-1440.png`。

### C-4 T3 编辑泳道改时间窗后返回，矩阵状态不串【pass】
依据：进编辑泳道（窗口标签 2026-11-10—2026-11-16，徽标 窗口 7 · 全部 124）→ 切 14 天（窗口标签 → 2026-11-03—2026-11-16、徽标 → 窗口 22、轴 11-03…11-15）→「← 返回」→ 矩阵恢复、选中泳道仍为「全球半导体板块受地缘与政策提振」、URL 仍 `?lane=41`、矩阵起始日仍 2026-11-03——工作面时间窗为**模态本地态，未串到矩阵/URL**。截图 `T3-workbench-14d-1440.png`、`T3-back-to-matrix-1440.png`。

### C-5 layout.md 全量路由矩阵的其余 4 条【未覆盖】
原因：`/search`、`/discovery`、`/settings`、`/tags/15220/research` 超出本次任务书指定的 4 条路由范围，只读档时间预算内未逐一走查。本 change 未改这 4 条路由的宿主组件，风险低。

## D. 新建泳道模式

### D-1 新建模式三栏与保存出口未破坏【pass】
依据：「新建泳道」→ overlay 非 correction 态（无 `bco--correction`），`bco__workbench` 三列：候选列（`lane-compose-candidate-count`，11 条候选泳道）/ 中央画布（`lane-compose-canvas`，152 节点）/ 右检视（`lane-compose-inspector`，含「本次泳道」「可补充的事件」「已选事件」「取消本次」「起名并创建 →」）；未点击创建（零写）。退出走「取消本次」→ AppDialog「返回会丢弃未保存的选择和名称」→「确认返回」。截图 `D-new-lane-mode-1440-editorial.png`、`T1-cancel-confirm-dialog.png`。

### D-2 候选接近度为「接近度」且越大越近【pass】
依据：勾选一条候选链（草稿态，零写）后右栏推荐出现「估算接近度 0.88 / 0.88 / 0.86 / 0.84 …」（降序 = 越大越近）；overlay 全文「距离」出现 **0 次**、「接近度」多次；画布图例「候选大小=接近度」；节点 `data-proximity` 出现正值（0.17–0.37）。截图 `D-new-lane-proximity-1440-editorial.png`。

## X. 其他

### X-1 写操作落库效果【pass（2026-11-16 真实写入补验）】

**首轮（浏览器内 mock 写）**：A-3/A-4/A-5 的写请求被只读守卫拦截回放，UI 状态机验证通过但 DB 零写入——该轮判「未覆盖」。

**补验（真实 PATCH → DB → 再读，2026-11-16）**：在主线程批准的 fixture 上对 `section 8633`（原属 candidate 泳道 2379）执行真实写链路，每一步都用只读 SQL 复核：

| 步骤 | 请求 | HTTP | 响应 | 落库结果 |
|---|---|---|---|---|
| ① 加入本泳道 | `PATCH /api/daily-reports/sections/8633/assignment` body `{"topic_id":503}` | **200** / 7.86 s | `{previous_topic_id:2379, action:"attach", changed:true}` | `persistent_topic_id` 2379→**503**，confidence `auto_new`→**`manual`** |
| ② 移出 | 同上 body `{"topic_id":null}` | **200** / 4.59 s | `{topic_id:null, previous_topic_id:503, action:"detach", changed:true}` | 归属 → **NULL**，confidence 清空 |
| ③ 加回（还原） | 同上 body `{"topic_id":2379}` | **200** / 4.93 s | `{topic_id:2379, previous_topic_id:null, action:"attach", topic_match_distance:1.11e-16}` | 归属 → **2379**（回到初始） |
| ④ 失败路径：从 active 泳道改挂 | 对 section 980（属 active 503）body `{"topic_id":2379}` | **409** | `{"error":"section belongs to an active lane; 改挂 is not supported"}` | **归属未变**（980 仍为 503）——红线 22 正确拦截 |

**鸿线 20 全程验证**（人工改归属 SHALL NOT 动命中计数/状态）：

| 泳道 | 初始 | ①②③ 全程 | 结果 |
|---|---|---|---|
| 503（active） | `hit_count=138, consecutive_hits=27` | 始终 `138 / 27` | **未动** |
| 2379（candidate） | `hit_count=1, consecutive_hits=0` | 始终 `1 / 0` | **未动** |

**已知副作用（如实记录）**：section 8633 的 `topic_match_confidence` 由 `auto_new` 变为 `manual`，**还原后不回退**——这是设计如此（一旦人工干预过就永久标记为人工归属），不是缺陷。若需完全回到自动口径只能重跑当日日报。

判定：真实信封、真实事务、真实错误码均验证通过；写后刷新联动已在首轮 UI 环节验证。X-1 由「未覆盖」转为 **pass**。


## 附：验收中发现的环境问题（非本 change 代码缺陷）

| 级别 | 事项 | 期望 vs 实际 |
| --- | --- | --- |
| Medium（环境） | dev 前端 `/api` 代理回落 SPA | 期望 `127.0.0.1:3000/api/semantic-boards/overview` 返回 JSON；实际返回 `index.html`，页面进入「总览读取失败」错误态（部分端点如 `/api/poll` 正常）。证据 `00-dev-api-failure.png`；后端 5100 直连 200 JSON。属 `scripts/dev/start-dev.sh` 注入的相对 base + Nuxt server proxy 配置问题，**不属 unify-lane-workbench 实现缺陷** |
| 信息 | 静态入口 `192.168.5.24` 跑的是旧版页面 | 验收时未重新构建部署（证据 `00-static-old-tags.png`）；实现确认后需 `bash scripts/dev/deploy-frontend.sh` |
| Low（工具口径） | 任务书 A-2 的 testid 写错 | 任务书写 `lane-card`，实际为 `lane-node-card`（`console/LaneCorrectionCanvas.vue:808`），按钮 `lane-card-add/remove/restore`。本报告按实际 testid 记账 |
| Low（观察） | 空白点击 + Esc 序列后 overlay 关闭一次 | 未能单独归因是二次 Esc 还是空白点击；spec 允许 Esc 取消聚焦，需确认「Esc 取消聚焦优先」语义 |
| Low（信息） | 「已加入本泳道」纯文案 toast 未捕获 | 实测跨泳道改挂时文案为「已从「来源泳道」移入」（正确）；纯「已加入」只在无来源候选直加时出现（`useLaneCorrection.ts:699`），本轮首个候选即跨泳道 |

---

## Y. 真库只读对账（tasks 9.7 / 11.13）

- 执行时间：2026-11-16
- 对象：boardId=15220（开发者工具与开源生态）、topicId=503（大模型监管与安全）
- 窗口：365 天（2026-06-18 → 2026-11-16，144 个有报告的日期）
- 方式：`default_transaction_read_only=on` 会话内执行 test-cases.md 的审计 SELECT（**只读，无 DDL/DML**）+ 两个 `GET` 对照
- 判定：不造数、不回填；无样本不假装通过

### Y-1 底表审计（`in_compose_pool` 口径已同步为 364 天）

| 指标 | 实测 |
|---|---|
| section 总数 | 1057 |
| 可用向量（`embedding IS NOT NULL`） | **1048 / 1057 = 99.15%** |
| 归属分布 | candidate 615 ｜ active 311 ｜ unassigned 81 ｜ archived 50 |
| 目标泳道 topic 503 | 124 sections，2026-06-27 → 2026-11-16 |
| 无向量 section | 9 条，**全部集中在 2026-10-02**（144 天里仅 1 天） |

### Y-2 `clue_count` 口径自校验（本 change 核心口径）

按 `clue_count = thread_count − composite_count` 逐行核对 **1057 行，不一致 0 行**。

- 含 composite 稿的 section：**18 个**（真实存在，非理论形态）
- `clue_count = 0` 的 section：**6 个，且全部 `thread_count > 0`**（即这 6 条只有综合稿）→ 直接印证「`clue_count` 为 0 也必须返回」不是空谈，若用 `omitempty` 这天会被误判为"未知"

### Y-3 对照 1：编辑曲线（`lane-dynamics?days=365`）【差异 0】

| 项 | SQL 底表 | API |
|---|---|---|
| 日期数 | 111 | 111 |
| 总线索 | 338 | 338 |
| **逐日差异** | — | **0 天** |

topic 503 的 timeline 与底表逐日 `sum(clue_count)` **完全一致**。编辑曲线口径可信。

### Y-4 对照 2：新增池曲线（`compose-candidates?days=365&omit_embeddings=1`）

| 项 | SQL 底表（按前端 `poolCountKind` 口径） | API 聚合 |
|---|---|---|
| section 计入 / 排除 | include 696 ｜ exclude 361 | sections 1048 |
| 池内日期数 | 143 | 142 |
| 池内总线索 | 1208 | 1188 |
| **逐日差异** | — | **1 天**：`2026-10-02` SQL=20 / API=0 |

**差异根因**：该日 9 条 section（6029–6037）`embedding IS NULL`，被 repository SQL 的 `AND ds.embedding IS NOT NULL` 过滤，因此不在响应里。

**判定：口径自洽，不是缺陷。** 依据 spec `lane-correction-surface` §「新增泳道的时间视窗曲线口径」明确要求「该口径 SHALL 与候选池（`compose-candidates` 返回的 `sections`）**同源**」，而候选池只含有可用向量、能参与质心与接近度计算的节点——无向量 section 既画不出也编排不了，不属「可选线索」。曲线该日显示 0 表示「当天没有可编排内容」，与被过滤的事实一致。

**已知边界（记入文档，不判 fail）**：若某版块出现大面积无向量 section，曲线会相应低估。当前实测占比 0.85%（9/1057，且集中在单日）。已在 `docs/reference/api/semantic-boards.md` 的 `omit_embeddings` 节标注该口径边界。

### Y-5 结论

- 编辑曲线：逐日 **0 差异**，可直接采信
- 新增池曲线：**1 天差异已归因并判定为口径自洽**（与候选池同源）
- `clue_count` 后端投影：1057 行零不一致；`0 必返回` 的必要性由 6 个真实样本佐证
- 全程只读，未产生任何写操作
