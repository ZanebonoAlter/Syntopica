# align-lane-correction-workface — 聚焦 review（2026-11-24）

## 总评

后端四条红线（读端点零写副作用、auto 首义不虚构、首义列不被算法改写、质心刷新范围）经代码与用例双向核对**全部成立**；`ListCentroidWindowSections` 抽取是逐字搬迁（谓词/排序/LIMIT 未改，仅投影加宽），`ComputeTopicCentroid` 行为不变；手动建泳道的首义留痕落在建话题事务内，失败回滚与「只记参与聚合的节点」都有永久用例兜住。

问题集中在**前端接线与验收口径**：一处主故事断链（已生效首义标记的 prop 从未传入，真实界面永不渲染，且无任何用例覆盖）、一处错误态谎报来源、两处「留痕清单声称的用例不存在 / 门禁命令跑不到新增用例」。

## 问题清单

### R-1 2026-11-24【High】「已生效首义」标记在生产接线里根本没接上：`anchorIds` 从未传给画布，标记永不渲染
- **状态**：已修复（`BoardComposeOverlay.vue` 画布补传 `:anchor-ids`；新增宿主层回归用例，撤掉接线即红，见 `BoardComposeOverlay.correction.test.ts`）
- **证据**：
  - `front/app/features/tags/components/console/BoardComposeOverlay.vue:488-514` 的 `<LaneCorrectionCanvas>` 传了 17 个 prop，**没有 `:anchor-ids`**；同一文件 `:462`（LaneMemberList）与 `:473`（LaneAnchorScope）都传了 `:anchor-ids="anchorIdList"`。
  - `LaneCorrectionCanvas.vue:102`（`anchorIds?: string[]`）、`:107`（默认 `() => []`）、`:1017-1019`（`isAnchorEffective()` 读该 prop）、`:1204`（唯一消费点 `<circle v-if="isAnchorEffective(node.id)">`）。
  - 用例侧零覆盖：`grep -rn "anchorIds\|lcc-anchor-ring" front/app/features/tags/components/console/LaneCorrectionCanvas.test.ts` **无命中**；该文件 `:261-271` 只断言图例里存在 `.lcc-lg--anchor-effective` 色块，没有任何用例断言节点上出现 `lcc-anchor-ring--effective`。
- **判断**：prop 默认 `[]` → `isAnchorEffective()` 恒 false → 「已生效首义」实线环在真实工作面**永远不出现**。图例却宣称有这一项，用户看到的是「图例有、画布上找不到」——正是本 change 要修的图例语义错位（D10 / 5.4）原地复发，只是换了个方向。单测因为走 `mount()` 显式传 props 而无法暴露此缺口，属主故事断链。
- **建议**：`BoardComposeOverlay.vue` 的 `<LaneCorrectionCanvas>` 补 `:anchor-ids="anchorIdList"`；并在 `LaneCorrectionCanvas.test.ts` 加一条断言（`anchorIds: ['m1']` → `m1` 出现 `.lcc-anchor-ring--effective`、未勾选成员不出现），再在 `BoardComposeOverlay.correction.test.ts` 加一条装配级断言（画布收到非空 `anchor-ids`），把「传没传」也纳入回归。

### R-2 2026-11-24【Medium】首义读失败时左栏仍断言「当前首义来源：自动（创建话题时写入）」
- **状态**：已修复（`anchorSourceLabel` 在「读失败且来源未确证」时返回空串；`LaneAnchorScope` 该行 `v-if` 隐藏；写成功后重读，来源结论不被读失败抹掉）
- **证据**：`front/app/features/tags/composables/useLaneCorrection.ts:210`（`anchorSource` 初值 `'auto'`）、`:396-400`（`anchorSourceLabel` 仅按 auto/manual 两态出文案）、`:634-640`（读失败分支**只**写 `anchorError`，不改 `anchorSource`）；`BoardComposeOverlay.vue:469` 与 `:503` 都把 `anchorSourceLabel` 传给左栏与画布确认条。
- **判断**：读失败时同屏出现两句互相矛盾的话——「当前首义来源：自动（创建话题时写入）」＋「首义来源节点：读取失败 · 重试」。前者是一条**从未读到的断言**（用户的泳道可能是 manual，读失败 ≠ 自动），与 `lane-correction-surface`「来源不可考 SHALL 显式为不可考、不得以推测顶替」以及「无来源的字段不得出现」的取向冲突。`anchorOriginKind` 已经把 error 单列成一态，`anchorSourceLabel` 却没有。
- **建议**：让 `anchorSourceLabel` 在有 `anchorError` 时返回空/「读取失败」而不是 auto 文案（或给 `anchorSource` 补 `'unknown'` 三态），并把该行为纳入 `useLaneCorrection` 的用例。

### R-3 2026-11-24【Medium】tasks 8.5 / 5.5 声称的用例并不存在（`useLaneCorrection.test.ts` 根本没改，画布高度用例也没有）
- **状态**：已修复（补 `useLaneCorrection.test.ts` 9 例归一/失败隔离、`LaneCorrectionCanvas.test.ts` 2 例画布高度；反向撤改后 4 例变红证明非空转）
- **证据**：
  - `stat front/app/features/tags/composables/useLaneCorrection.test.ts` → `2026-11-16 22:31`（`useLaneCorrection.ts` 为 `2026-11-24 16:11`），`git status --porcelain` 中该文件**未修改**；`grep -rn "anchor_origin\|anchorOrigin\|centroidScope" front/app --include=*.test.ts` **零命中**——8.5 声称的「`anchor_origin` / `centroid` 归一与派生（含 degraded、读失败不阻断写入口）」没有任何用例。
  - `grep -n "空白带\|贴合\|富余" LaneCorrectionCanvas.test.ts` **零命中**——5.5 声称的「内容少时无空白带」「成员区位置不随富余下移」两例不存在。
- **判断**：`normalizeAnchorOrigin` / `normalizeCentroid` / `getTopicAnchor().catch(...)`（读失败不阻断写入口这条 spec 条款）是本 change 新增逻辑，目前只有 `LaneAnchorScope` 的纯 props 单测（测的是组件不是归一），真实归一与失败隔离路径无覆盖。任务被勾成 [x] 属假绿，后续回归没有防线。
- **建议**：补 `useLaneCorrection.test.ts` 三例（`anchor_origin` 三态归一、`centroid` 缺失/齐备与 degraded、`getTopicAnchor` reject 时 `load()` 仍成功且 `anchorError` 有值）；补 `LaneCorrectionCanvas.test.ts` 画布高度两例（容器高 > 内容高 → `svg height` 贴合容器；富余高度不改变 `membersBottom`）。或把 5.5 改成「既有行为复核，不新增用例」并删掉勾上的假证明。

### R-4 2026-11-24【Medium】门禁命令跑不到本次新增的编排画布用例（`BoardComposeOverlay.test.ts` 未改，用例实际落在 `LaneComposeCanvas.test.ts`）
- **状态**：已修复（tasks 8.6 路径改为 `LaneComposeCanvas.test.ts`；10.3 命令并入该文件）
- **证据**：`git status` 中 `BoardComposeOverlay.test.ts` 未修改（mtime `2026-11-23 14:56`），本次新增用例在 `front/app/features/tags/components/compose/LaneComposeCanvas.test.ts:1707-1819`（+114 行，4 条 7.x 用例）。而 tasks 7.1–7.4 的验证命令是 `pnpm test:unit BoardComposeOverlay`，10.3 的前端单测命令是 `pnpm test:unit LaneCorrectionCanvas LaneAnchorScope useLaneCorrection BoardComposeOverlay`；`ui-design.md`「验收映射」两行也写的是 `BoardComposeOverlay.test.ts`。
- **判断**：`pnpm test:unit <name>` 按文件名过滤，两条命令都不会跑 `LaneComposeCanvas.test.ts`。也就是说 7.1–7.4 与 10.3 是「跑过且绿」的，但**绿的不是新代码的用例**；这份 change 的高度贴合能力没有进入任何声明过的门禁。
- **建议**：把 7.1–7.4 与 10.3 的命令改为包含 `LaneComposeCanvas`，`ui-design.md` 验收映射两行改指 `LaneComposeCanvas.test.ts`（或给 `BoardComposeOverlay.test.ts` 补装配级高度用例）。

### R-5 2026-11-24【Low】`centroid.section_count` 出现在 API 文档示例与前端类型里，后端从不返回
- **状态**：已采纳（后端 `TopicAnchorCentroid` 补 `SectionCount`＝len(section_ids)，文档示例与前端类型据此对齐）
- **证据**：`docs/reference/api/daily-reports.md:344-355` 的响应示例含 `"section_count": 3`；`front/app/api/dailyReports.ts` 的 `TopicAnchorCentroid` 声明 `section_count?: number`；后端 `TopicAnchorCentroid`（`daily_report_anchor_read.go`）只有 `window_size`/`anchor_weight`/`section_ids`/`members`/`degraded`，`grep -rn "section_count" backend-go/internal/topicgraph` 在 anchor 段**零命中**。
- **判断**：文档示例无法复现（契约文档与实现对不上）；前端靠 `section_ids.length` 回退所以数值仍对，但「文档写着、接口没有」的字段会误导下一个消费方。
- **建议**：二选一——后端给 `TopicAnchorCentroid` 补 `SectionCount int`（= len(section_ids)，零成本、与文档/类型对齐），或删掉文档示例与 TS 类型里的该字段并保留 `section_ids.length`。

### R-6 2026-11-24【Low】按下落在只读参照 / 「更早」折叠带节点上时，既不起拖也不起平移（平移死区）
- **状态**：已修复（不可拖节点上按下退化为平移，点击仍只聚焦；补两例用例）
- **证据**：`LaneCorrectionCanvas.vue:280-292`：命中 `[data-node]` 后 `if (!isDraggable(id)) return`，直接 return，不再落到下方 `panStart = {...}` 分支（`git show HEAD:…LaneCorrectionCanvas.vue` 的旧 `onSurfacePointerDown` 无该早退，任何非交互目标都能起平移）。
- **判断**：不可拖节点的语义是「不启动拖拽」，实现顺带取消了该区域的平移手势——节点圆点 `lcc-hit` 半径约 12-13px，只读参照带与折叠带节点较密集，用户在此处按住拖动会「什么都没发生」。属旧能力（空白/节点处皆可平移）的局部收窄，未被任何制品声明为主动退役。
- **建议**：不可拖时不要 `return`，fall through 到平移分支（`isDraggable` 为假只表示「不接管拖拽」）。

### R-7 2026-11-24【Low】`resolveAnchorOrigin` 按留痕节点逐条查询（最多 N 次往返）
- **状态**：已修复（改一次 `WHERE s.id IN ?` 取回后按原序挑首个存活节点，`resolved=false` 语义不变；补「首节点已删→顺位」用例）
- **证据**：`backend-go/internal/topicgraph/repository/daily_report_anchor_read.go:168-190`，`for _, sectionID := range state.SectionIDs { r.db.Raw(<单条 section 查询>) }`，命中第一个仍存在的节点才 return。
- **判断**：读端点多出「最多等于留痕节点数」的查询；留痕节点被大量删除时最坏 O(N)。design 的「响应体积可接受、取数走单条 SQL、无 N+1」只覆盖 `centroid` 段，这条与那句话并列时会被读成矛盾。
- **建议**：改成一次 `WHERE s.id IN ?` 取回全部留痕节点，再在内存里按 `SectionIDs` 顺序挑第一个存在的；或在 design/代码注释里明确这是「有界的小 N，接受」。

### R-8 2026-11-24【Low】tasks 10.4 记录的验证命令按原样执行会失败（缺 `openspec/changes/` 前缀）
- **状态**：已修复（tasks 10.4 命令补 `openspec/changes/` 前缀）
- **证据**：`bash scripts/harness/doc-impact.sh verify align-lane-correction-workface` → `rc=1`，stderr `FAIL: change 目录无效或无 tasks.md`；`bash scripts/harness/doc-impact.sh verify openspec/changes/align-lane-correction-workface` → `rc=0`（`通过（声明: api, database, flow 文件:5 个 启发式:ownership）`）。另一条 `check-tasks-tail.sh` 与 `openspec validate` 均 rc=0（validate 只有 >500 字符长度 WARNING）。
- **判断**：条目被勾成 [x] 且写「期望退出码 0」，但该命令按字面跑不出来；复检的人会得到红，误判为回归。
- **建议**：命令改成 `bash scripts/harness/doc-impact.sh verify openspec/changes/align-lane-correction-workface`。

### R-9 2026-11-24【Low】tasks 6.4 的验收口径与 delta spec 相反（「degraded 文案改为退回首义向量」vs 文案 SHALL NOT 断言其中一种）
- **状态**：已修复（tasks 6.4 验收口径改为中性退化口径，与 delta spec 一致）
- **证据**：`tasks.md` 6.4「`degraded` 为真时该组文案改为『质心当前退回首义向量』」；`specs/lane-manual-correction/spec.md`：「`degraded` SHALL 是单一布尔退化标记，前端文案 SHALL NOT 把它断言为『退回首义向量』」；实现 `LaneAnchorScope.vue:148` 输出「质心当前为退化口径（窗口向量不足或首义不可用）」并在 `LaneAnchorScope.test.ts` 断言 `not.toContain('退回首义向量')`。
- **判断**：实现与 spec 一致（正确），是 tasks 行的验收口径写反了。留着的风险是后续读者按任务行判定「未实现」而改坏实现。
- **建议**：改 tasks 6.4 的验收描述为「文案为中性退化口径，SHALL NOT 写死两种分支之一」。

### R-10 2026-11-24【Low】design D5 的落点符号漂移（写 `anchorSource`，实际由 `anchorOriginKind` 承接）
- **状态**：已修复（design D5 落点改为 `anchorOriginKind`，注明 label 是展示派生、不得回退成 auto 文案）
- **证据**：`design.md` D5「落点：… + `front/app/features/tags/composables/useLaneCorrection.ts:anchorSource`」；实现里 `anchorSource` 仍是 `'auto' | 'manual'` 两态（`:210`、`:396-400`），三态由 `anchorOriginKind`（`:404-408`）与 `LaneAnchorScope` 的 `originKind` 承接。
- **判断**：结论有承接（不是「有结论无承接」），但落点指错符号，按 design 去找会找到两态字段并误以为三态没做。
- **建议**：把 D5 落点改为 `useLaneCorrection.ts:anchorOriginKind`（+ `anchorOrigin`）。

## 核实记录

- 审毕无发现（审查范围：`ListCentroidWindowSections` 抽取与原内联 SQL 的行集等价性、`ComputeTopicCentroid` 行为不变、首义读端点零写副作用与 `degraded` 口径、`resolveAnchorOrigin` 不虚构、手动建泳道留痕的事务性与「只记参与聚合节点」、首义列不被算法改写、质心刷新范围仍是「当日 touched ∪ pending」、候选桶相对归一只作用于 estimate 桶且成员尺寸不参与、成员/候选接近度不同名不同形、无 `.only`/`.skip`/探针残留、`openspec validate` / `check-tasks-tail` / `doc-impact verify`（带全路径）三门禁）。
- **等价性逐字比对**：`daily_report_topic_repository.go` 新旧 SQL 的 `FROM/JOIN/WHERE`（`persistent_topic_id = ?` + `embedding IS NOT NULL`）、`ORDER BY rpt.period_date DESC, s.id DESC`、`LIMIT ?` 完全一致，仅投影由 `s.embedding` 加宽为 `s.id/s.embedding/s.cluster_label/rpt.period_date`；`TestCentroidWindowExt_PredicateOrderAndLimit` 与 `…_ReadEndpointAndCentroidShareTheSameWindow` 逐 id 锁定。
- **零写副作用**：读路径全部为 `First/Find/Raw(SELECT)`；`LoadPersistentTopicConfig` 只 `Find` 不 seed（`:174-197`）；`TestAnchorReadExt_ReadHasNoWriteSideEffects` 用前后快照（embedding/centroid/pending 标记/section 归属快照/四张表行数）比对。
- **login 越界说明**：`files changed` 里混有他人会话的脏文件（KeyEventTimelineView、margin-notes、searxng 等），本报告只取本 change 路径（tasks 第 1–7 节所列）作为审查对象。

### 旧能力去向表（`git show HEAD:` 对照旧实现逐条判定）

| 旧能力 | 判定 | 证据 |
|---|---|---|
| 节点卡三按钮（＋加入 / ⊘移出 / ↩加回） | 仍可达 | `LaneCorrectionCanvas.vue:1345/1353/1361` 同 `data-testid`；`BoardComposeOverlay.vue:508-510` 同事件绑定 |
| 左栏成员勾选 / 取消勾选（直接移除） | 仍可达 | `LaneMemberList` 未改；`BoardComposeOverlay.vue:462-467` `@remove/@restore` 未变 |
| 右栏「关联推荐」＋加入 | 仍可达 | `LaneRecommendationList` 本 change 未动；`BoardComposeOverlay.vue:517+` 绑定未变 |
| 画布点节点 = 只改选中态（含 250ms 吞 click） | 仍可达 | `onNodeClick`（`:1005-1010`）保留 `lastPanAt`/`lastDragAt` 双吞判据 |
| 空白处拖拽平移 | 部分收窄（见 R-6） | `LaneCorrectionCanvas.vue:280-292`：命中不可拖节点即 `return`，该处无平移 |
| 滚轮缩放 + 「－ ＋ 重置」 | 仍可达 | `onWheel`/`applyZoom`/`changeZoom`/`resetZoom`（`:231-265`）与 `:1379-1382` 未改 |
| 「更早」折叠带节点勾选（首义范围） | 仍可达 | `lcc-fold-check` `@click.stop="emit('toggleAnchor', …)"`（`:1283-1305`） |
| 首义覆盖确认条（不叠弹窗） | 仍可达 | `:1036-1041` `lane-anchor-confirm` |
| 图例「虚线 = 首义」（单一标记） | **主动退役**（有承接） | 被「已生效首义 / 待提交勾选」两项取代；依据 delta spec「首义标记 SHALL 区分两种状态」；旧断言 `LaneCorrectionCanvas.test.ts` 已按新契约改写（属 spec 强制，非迎合实现）。但「已生效」这一项本身未接线，见 R-1 |
| 加载骨架 / 空态 / 错误 + 重试 | 仍可达 | `:1054-1076` 三支保留 |
| 节点卡跟随选中、缩放后不塌扁 | 仍可达 | `cardPosition`/`cardSize` 逻辑未改（`:977-1002`） |

### R-11 2026-11-24【High】1440×900 下左栏各块被压扁裁切（本 change 新增块触发）
- **状态**：已修复（`LaneMemberList.vue` / `LaneAnchorScope.vue` 的 `.lml-panel` / `.las-panel` 加 `flex-shrink: 0`；块内长清单仍由各自 `max-height` 内部滚动，左栏整体 `overflow-y:auto` 承接超屏）
- **证据**：实机验收第一轮（`ui-verification/report.md` A-3 / A-8）：1440×900 下「已移出（本次）」行被压到 **15px**（行高需 28.5px，标题与「↩ 加回来」被硬切），「更早成员 · 109」面板头 27px（头部需 37px）。第二轮复验：列表体 **38.5px（client=scroll）**、行 28.5、按钮 20.5，`elementFromPoint` 命自身；「更早成员」面板 228.5（头 36.5 完整）；左栏 `client 512 / scroll 945`、`scrollTop` 可达 433，滚到底「首义范围」四个按钮全部 `hitSelf`。1920 无回归。
- **判断**：`.bco__left` 是 flex 列，子块默认 `flex-shrink: 1` → 内容总高超出容器时子块被压到低于自身内容高度并裁切；本 change 新增「首义来源 / 参与滚动计算的节点」两块把总高顶上去，1440 才必然触发。违反既有契约「左栏与右栏 SHALL 各自纵向可滚……该块的操作按钮 SHALL 始终可见」。
- **建议**：已完成；后续若在左栏再加块，MUST 保持 `flex-shrink: 0`，并在 1440×900 复验可见性。

## 第三轮增量自查（2026-11-25 用户实机反馈三处补丁）

> 范围：节点卡标题换行 / 话题带强调色横条移除 / 轻点空白取消聚焦。三处都在 `LaneCorrectionCanvas.vue` + 一处 `BoardComposeOverlay.vue` 接线。逐条按「有没有把旧能力弄丢」核。

| 旧能力 | 去向 | 依据 |
|---|---|---|
| 节点卡标题单行省略 | **主动退役** | 用户当场指令「换行也要能展示清楚」；卡体职责是让人认出是哪条，截断即失职。`white-space: normal` + `overflow-wrap: anywhere`，卡高沿用既有 `cardSize` 实测口径 |
| 节点卡卡体穿透点击（`pointer-events: none`） | **主动退役** | 取消聚焦上线后，穿透会让「点卡片」落到画布空白而被判成取消聚焦（把卡片变玻璃按钮）。改 `auto`，卡上按钮不再需要自己 `pointer-events: auto`（保留无害）。代价：卡遮住的画布区域点不到了——卡是随聚焦出现的临时浮层，位置在节点右侧且可再点空白关闭，不构成能力损失 |
| 话题带强调色横条 + 成员刻度线 | **主动退役** | 用户当场指令「多余可以删除」；不承载判断（成员归属由圆点 + 勾 + 分区标签表达），保留标签与中性基线与其它分区同形 |
| 顶部「取消聚焦」按钮 | **仍可达** | 未删，键盘与精确操作兜底 |
| 点节点 = 只改选中态；再点同一节点取消 | **仍可达** | 空白取消聚焦只认「起手不在 `[data-node]` 上」，不抢节点 click；`setFocus` 的切换语义未动 |
| 空白处拖拽平移 | **仍可达**（且平移松手不误判取消聚焦） | `panStart.onNode` 只标记起手位置，`wasPanning` 为真时跳过 emit（R-6 的退化为平移路径同样覆盖） |
| 折叠带勾选 / 缩放条 / 图例点击 | **仍可达** | `onSurfacePointerDown` 开头的 `closest('.lcc-card, .lcc-zoom, button, .lcc-fold-check')` 早退未改 |
| 归属写入口（拖拽 / 节点卡按钮 / 左栏勾选） | **仍可达且零改动** | 本轮只加 `clear-focus`（无写副作用），写路径与 `pendingIds` 防重入未动 |

**结论**：无 High/Medium 遗留；「卡体穿透」是唯一被顺带改掉的旧行为，已在上表登记理由与代价。实机三视口核验见 `ui-verification/report.md` 第三轮 B-1～B-3（pass 3 / fail 0）。
