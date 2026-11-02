# Test Cases — board-health-triage-advice（v2 获批）

> 用户故事是交付单元，Scenario 只是节拍断言。原型42条仅证明模拟交互，不算本文件正式故事已交付。白盒枚举来自只读 reviewer run `38b5a43a-bb6a-49d9-a092-6074deb8c9d7`，主线程确认断言判据。实现接缝见 implementation-contract.md；精确新增文件由各 seam 交接校准。

## 主链路故事表

| 步 | 动作 / 来源Scenario | 期望 | 层 / 落点 |
| --- | --- | --- | --- |
| A1 | 体检装载全体挂载，计算磁铁/最近邻/词根；磁铁差成立、样本不足、缺向量 | 0.52/0.68 与40样本产生信号；不足不硬算；active top3排除自身 | metrics_test.go + boardhealth/repo_pg_test.go |
| A2 | 单次LLM裁决；产出评分/评语、短名不摘、多归属非充分摘除、证据不足 | 四类清单有真实证据、合法aux引用；短名不因低cosine误杀 | llm_test.go + run_test.go；真LLM另做效果核对 |
| A3 | 超时/坏JSON/非法分类 | score/comment/triage空，信号统计照常落库，兄弟版块继续 | run/llm桩单测 + PG |
| A4 | 查询最新/趋势；旧快照无triage，disabled/不存在 | 透传新字段；旧形态兼容；disabled空态、missing404；GET无写 | handler/board_health_handler_test.go |
| A5 | :5100 右轨→完整依据→索引→选问题→展开保留/不足/统计 | 不遍历四组大卡；所有结果可达，旧候选/构成/组合/来源/设置未丢 | 组件 + agent-browser（ui-verification/report.md） |
| A6 | 一轮体检前后核对匹配/升级建议 | 行数和内容不被体检写链改变，人工修复另行确认 | boardhealth/repo_pg_test.go（临时PG） |
| B1 | 读实时标签/context，编辑并确认共享影响 | 当前版本、直接引用版块、全活跃匹配范围先展示；取消零任务 | handler + 组件挂载 |
| B2 | 空白/标签名/501字符、disabled、非aux、越界job | 就地反馈/明确4xx；不改标签/匹配、不创建成功任务 | handler + auxrepair/service_pg_test.go + 组件 |
| B3 | 用户明确确认，同键重发/同标签异键并发 | 相同payload同任务，不重复embed；同键异payload409，同标签未结束互斥 | auxrepair PG + handler |
| B4 | 生成label:description storage向量（事务外）失败 | 旧描述/旧storage向量都不变，任务失败可重试 | auxrepair PG + 桩embedder |
| B5 | 生成后、保存前来源版本变化/事务失败 | 拒覆盖或整体回滚；草稿保留；label/aliases/merge向量不变 | auxrepair PG + 组件 |
| B6 | 原子提交描述/storage向量/任务提交阶段 | 两列同生死，cache失效；保存后故障恢复不重嵌/重存 | auxrepair PG 故障窗 |
| B7 | 固定全部active tag范围，包含无匹配者 | 不以现有匹配表代替全范围；范围持久可重启 | auxrepair PG（既有引擎桩/集成） |
| B8 | 部分匹配失败，再确认重试 | 仅补失败/未完成项；成功数不冒充total，embed调用不增加 | auxrepair PG + 组件 |
| B8b | 冻结范围内标签在处理前已删除/禁用/合并 | 该项持久记为 not_applicable：不算成功、不算失败、不缩 total、不调引擎；其余适用项照常执行 | auxrepair PG（service/auxrepair/matching_pg_test.go） |
| B8c | 匹配引擎合法零命中 | 计入 processed（成功执行），不伪造版块命中、不误报失败 | auxrepair PG（桩 + 真实引擎各一条） |
| B8d | 计数口径 | processed+failed+not_applicable+remaining=total；remaining=0且failed=0且各项为 succeeded/not_applicable 才 completed；旧任务 not_applicable=0，remaining 由服务端算；wire 缺 remaining 前端保持未知不伪造 0 | auxrepair PG + handler（DTO 继承 B2） |
| B9b | 用户明确 retry：失败/中断的任务重新排入同一任务 | 同一 job 重新排入（不新建、不改 expected_version/new_description/冻结范围）、清旧 error、已提交只续匹配；运行中/已完成是 202 no-op（连 UpdatedAt 都不变）；conflict/unavailable 409、越界 404、坏 body 400 | auxrepair PG + handler（retry 资源） |
| B9c | 后台自动边界 | 只自动领取 pending 与「失租未结束阶段」；明确失败（embedding_failed/save_failed/matching_failed）**不自动重跑**（不烧 AI），等用户 retry；多实例只一方领取 | auxrepair PG（并发领取/心跳/恢复） |
| B9 | 关闭/重进/服务实例重建；运行中重试 | 读持久阶段，无重复worker；能安全恢复或明确重试 | auxrepair PG + :5100 agent-browser |
| B10 | 全匹配成功→精简只读结果→下一项 | 三阶段/恢复/下一项首屏可见；无可编辑textarea；只说更新完成，效果待下轮，旧评分/日报不变 | 组件 + :5100 agent-browser + PG |
| C1 | 摘除确认→取消/失败重试→成功 | 既有DELETE；只移除本板构成、不删共享标签；历史匹配不自动回填，明确下一项 | BoardHealthPanel.test.ts + agent-browser网络/落态 |

## seam C2（task 3.2 / 3.3）组件故事落点

> 本批=**真实组件**：挂载 `BoardHealthWorkspace` + **真实 C1 composable**，只 mock API 层
> （体检快照 + 修复四资源）——不 mock 整个 composable 自证。
> 落点：`front/app/features/tags/components/console/board-health/`。

| # | 用例（来源 Scenario / 白盒） | 期望（可判定） | 落点 |
| --- | --- | --- | --- |
| C2-1 | 索引只列需核对项（分诊以问题处理为主线） | 顶部「本轮需核对 N 项」；索引只有 remove/rewrite；keep/insufficient 在「其余标签」折叠内且各有证据 | `BoardHealthWorkspace.test.ts` |
| C2-2 | 旧快照 / 空清单 / 降级轮次（历史快照缺分诊字段兼容） | 三种各自独立说明；**不展示问题清单、不反推诊断、不说全部健康**；原统计与评分趋势仍折叠可见 | 同上 |
| C2-3 | 读取失败 / stale（可用性变体：空态与错误态） | 错误态给「重试」；stale 明说"提交修复时仍须核对实时版本"；两者都不创建任务 | 同上 + `BoardHealthWorkspace.repair.test.ts` |
| C2-4 | 分诊有丢弃项（白盒：不得伪装完整） | 一行提示「N 条记录不可执行（已剔除），清单不完整」；不加信息卡 | `BoardHealthWorkspace.test.ts` |
| C2-5 | 选择 → 实时上下文（核对并确认共享影响） | 原描述、污染证据、直接引用版块、活跃标签覆盖数可见；建议预填为草稿 | 同上 |
| C2-6 | 三规则就地反馈（无效描述与禁用标签） | 空 / 超 500 码点 / 恰好重复标签名 → 就地报错且提交按钮 disabled；停用标签 → 编辑与提交禁用 | 同上 + `BoardHealthWorkspace.repair.test.ts` |
| C2-7 | 确认前零写入（取消也零写） | 打开/编辑/展开证据/取消确认全程只有 GET；`createDescriptionRepair` 零调用 | `BoardHealthWorkspace.test.ts` |
| C2-8 | 重复提交不重复执行（双击/重试） | 最终确认只发一次 POST；body = description + expected_version + idempotency_key | 同上 |
| C2-9 | 真实任务阶段与计数（生成→保存→匹配） | 三阶段随服务端 status 推进；计数 processed/failed/notApplicable/remaining 取服务端值；null 显示「未知」不补 0 | 同上 |
| C2-10 | 只重试匹配 / 不新建（匹配失败只重试匹配） | 失败态只给「重试」，只打 retry 端点（同 job），`createDescriptionRepair` 仍为 1 次 | `BoardHealthWorkspace.repair.test.ts` |
| C2-11 | 冲突保留草稿（来源版本已变化） | 409 展示**服务端原文**（不统一写成版本变化）；重新加载核对后草稿不丢 | 同上 |
| C2-12 | 未知结果 = 原意图未确认（白盒：不得换键重建/冒充他键任务） | 只读保留原确认内容；「重发原确认内容」严格同 key + 同 payload，草稿不丢；刷新不解冻 | 同上 + C1 `resendFrozen` 用例 |
| C2-13 | 重新进入恢复（离开或服务重启后再进入） | 从 `context.latest_task` 恢复已知任务与只读执行态（无 textarea） | `BoardHealthWorkspace.repair.test.ts` |
| C2-14 | 完成语义（结果不伪造健康改善） | 「更新完成，效果待下一轮体检验证」；主动作「处理下一项」；全部处理完 → 「返回问题清单」，**不说所有标签健康** | `BoardHealthWorkspace.test.ts` |
| C2-15 | 摘除契约（3.4 注入，真实 DELETE 未接） | 行内确认后发 `remove-composition`；未收到宿主成功反馈前**不标「已摘除」**；宿主回传后才显示 | `BoardHealthWorkspace.repair.test.ts` |
| C2-16 | 切标签迟到响应（竞态） | 旧标签的迟到上下文/建议预填不覆盖当前项 | 同上 |
| C2-17 | 纯派生（白盒：阶段/状态/计数推导） | 状态映射、三阶段序列、计数格式化、进展文案单测 | `boardHealthWorkspace.model.test.ts` |
| C2-18 | 旧能力→承接（不复制统计实现） | 评分/趋势复用既有 `BoardHealthSignal`；质量表/组合/候选仍由既有页签承载 | `seam-c2-workspace-handoff.md` §3 |
| C2-19 | 降级轮真 wire（LLM 裁决失败） | `llm_score`/`llm_comment` 均 NULL 且 `metrics.triage=null` → 显示「本轮未生成评分与分诊…不能推断健康」，**不**误标「历史快照」 | `BoardHealthWorkspace.test.ts` |
| C2-20 | 正常轮依据可达 | 正常轮也有默认折叠的「本轮体检依据」（评分/统计/趋势按需展开） | 同上 |
| C2-21 | remove 项也要看证据 | remove 项同样能展开「为什么建议处理 · 展开证据」（磁铁/最近邻/词根/失败挂载流向） | 同上 |
| C2-22 | 非 409 提交失败不吞 | 独立错误行透传服务端 message；行内确认**不静默关闭** | `BoardHealthWorkspace.repair.test.ts` |
| C2-23 | 范围未知禁确认 | repair-context 未加载/失败/字段缺失 → 确认按钮禁用并说明原因（不谎称 0 范围） | 同上 |
| C2-24 | 切项不闪现上一项草稿 | 切项先清草稿再取上下文（编辑器不出现上一项内容） | 同上 |
| C2-25 | stale 提示行 | 刷新失败保留上次快照并出现提示行（含「显示上次快照」「核对实时版本」） | 同上 |
| C2-26 | 404/403 不当 stale（复审新提 Low） | 版块已删/无权 → 明说「可能已经不存在」且**不展示可操作清单**；**不再**出现「刷新失败显示上次快照」 | `BoardHealthWorkspace.repair.test.ts` + `boardHealthWorkspace.model.test.ts` |
| C3-1 | 摘除经用户确认走既有接口（摘除确认→取消/失败重试） | 工作面发 `remove-composition` → 壳层转数字 → 既有 `removeFromComposition(boardId, auxId)`；取消零请求；失败只通知一次 | `BoardConsoleShellRepair.test.ts` + `BoardHealthWorkspace.repair.test.ts` |
| C3-2 | 「已摘除」只由真实成功驱动 | 成功才回填 `removedAuxIds`（工作面显示「已摘除」）；失败不回填、不谎称；历史匹配不自动回填 | `BoardConsoleShellRepair.test.ts` |
| C3-3 | 宿主接入不丢旧管理能力 | 体检页签渲染工作面；构成/组合/候选/来源/设置页签与右轨 brief 原样保留 | `BoardWideWorkface.test.ts` + `BoardContextRail.test.ts` |

## 继承与调整（⓪：已跑 test-assets.sh board-health-checkup）

缩写：M=`service/boardhealth/metrics_test.go`，P=`service/boardhealth/repo_pg_test.go`，L=`service/boardhealth/llm_test.go`，R=`service/boardhealth/run_test.go`，N=`service/boardhealth/notify_test.go`，H=`handler/board_health_handler_test.go`，F=`front/app/features/tags/components/BoardHealthPanel.test.ts`；后端均在 backend-go/internal/tagmanagement/ 下。每行是独立旧Scenario，不用「现有测试绿」替代新契约。

| 旧Scenario | 处置 | 旧测试 | 动作 |
| --- | --- | --- | --- |
| 构成标签贴合度分布全覆盖 | 保留 | M | 加信号，不变原样本口径 |
| 样本不足显示数据不足 | 保留扩展 | M | 增magnet/neighbor不足，不按0硬算 |
| 低分线按板块内相对分位 | 保留 | M | 不变公式 |
| cosine 重算进入分布 | 保留 | M | 不依赖规则分数 |
| 规则分数不作为相似度统计输入 | 保留 | M | 负向回归 |
| 向量缺失跳过且计数 | 保留 | M | own-vector不足与tag-missing分开 |
| 高频未收编标签进入候选清单 | 保留 | P | 不因UI重构丢数据 |
| 已挂载构成标签不进候选 | 保留 | P/board expand test | 现有过滤保持 |
| 体检轮次不落扩充建议 | 保留 | R/P | 显式零写回归 |
| 未收编候选可一键收编 | 保留 | F | 保留宿主及动作去向 |
| LLM 产出评分与评语 | 修改 | R/L | 增完整合法triage，与score共同成功 |
| LLM 失败降级不丢统计 | 修改 | R/L | triage与score同降级，信号保留 |
| 相对垫底但绝对健康的标签不判分化 | 保留扩展 | L/R | 分诊也不得误摘 |
| 无组合挂载的板不因 reason_dist 判病 | 保留扩展 | L/R | 分诊提示词锚定 |
| 真分化仍被拦截 | 保留 | L/R | 防误读不放松真病征 |
| 每轮落一条快照 | 保留 | P | jsonb扩展兼容 |
| 历史快照不被覆盖 | 保留扩展 | P | 修复写链也不得改旧快照 |
| 查询趋势序列 | 保留 | P/H | 不以修复任务修改评分 |
| 异常且裁决值得则通知 | 保留 | N/R | notify产出不变 |
| 裁决不值得则静默 | 保留 | N/R | 无新增打扰 |
| 冷却期内不重复通知 | 保留 | N | 不变冷却策略 |
| 体检在日报后自动执行 | 保留 | R | 不自动执行修复 |
| 体检不改匹配数据 | 保留加强 | R/P | 人工写链与体检隔离 |
| 扩充建议仍需手动触发 | 保留 | R/P | 不偷跑扩充 |
| 查询最新快照 | 修改扩展 | H | 返回triage，GET只读 |
| 无快照返回空态 | 保留 | H/F | 不造问题/任务 |
| 版块不存在 404 | 保留 | H | disabled仍空态 |

辅助标签 description 原有三规则、alias不写向量、NULL回填只补空向量均保留；新修复是独立人工确认能力，不改这些旧入口的约束。

## seam C1（task 3.1）前端 API/归一/轮询用例落点

> 本批只做 API DTO/归一器 + 四资源 + 有界轮询 composable；**不含组件故事**（3.2/3.3）。
> 断言以“行为”表述，落点为真实新增文件。

| # | 用例（来源 Scenario / 白盒） | 期望（可判定） | 落点 |
| --- | --- | --- | --- |
| C1-1 | 历史快照缺 `metrics.triage`（历史快照缺分诊字段兼容） | `triage.available=false`、条目空；不从现有统计反推诊断 | `front/app/api/normalizers/boardHealthTriage.test.ts` |
| C1-2 | 新快照四类分诊（分诊以问题处理为主线 / 标签级分诊） | 视图 camelCase + 字符串 `auxId`；remove/rewrite `actionable=true`，keep/insufficient `actionable=false` 且 suggestion 清空 | 同上 |
| C1-3 | 非法分类 / 非法 aux 引用 / 重复记录 / 无证据（白盒：分诊不产生错误执行入口） | 该条丢弃且 `dropped` 计数；不透出为可执行建议；其余合法条目保留 | 同上 |
| C1-3b | 同 aux **矛盾**类别 / 矛盾证据（白盒：不得 first-wins 留可执行项） | 按 aux 分组：整组不可执行、全部丢弃、`dropped`=组内条目数；一致重复才去重留 1 | 同上 |
| C1-4 | 磁铁/最近邻/词根信号搬运（标签分诊信号计算） | 服务端数值原样搬运；`neighbors=null` 表示信号不足；前端不重算 gap/top-k | 同上 |
| C1-5 | 四资源 URL/方法/body（修复任务持久且幂等） | context=GET；创建=POST 带 description/expected_version/idempotency_key；任务=GET 带 job_id；retry=POST 空对象 | `front/app/api/auxiliaryLabels.test.ts` |
| C1-6 | 任务归一：typed 状态/阶段、opaque version、私有字段过滤（更新完成与效果验证分离） | 未知状态落 `unknown`；version 原样透传；claim_owner/phase_token/committed_* 不在视图；停止轮询集 / 互斥释放集 / 可重试集三集合分开 | `front/app/api/normalizers/auxiliaryRepair.test.ts` |
| C1-7 | `not_applicable` 单列、`remaining` 缺失为未知（计数口径） | NA 不并入 processed；缺 remaining → `null`（不补 0、不用差值猜）；total/processed/failed 缺失或非法 → `null`（未知，不捏造 0、不造完成进度）；旧 wire 缺 NA → 0 | 同上 |
| C1-8 | 描述三规则（无效描述与禁用标签） | 空/超 500 码点/恰好重复标签名 → 拒；合法 → trim 后值 | 同上 |
| C1-9 | 重新进入从 `context.latest_task` 恢复（离开或服务重启后再进入） | 已知任务取自服务端持久任务；终态不轮询、非终态进入轮询 | `front/app/features/tags/composables/useAuxiliaryDescriptionRepair.test.ts` |
| C1-10 | 双击与请求重试不重复执行（幂等变体） | 提交中再次提交被拒且只发一次 POST；一次确认=一个幂等键+一份冻结 payload | 同上 |
| C1-11 | 网络错/超时/5xx/408 = 结果未知（幂等变体） | 冻结保留；重发同键同体（逐字段相同）；改内容不得复用旧键且不发新请求；408 按状态码判为未知（不解析文案） | 同上 |
| C1-11b | 未知结果时**重载上下文**（latest 为空 / 他键任务）（幂等变体） | 查询为空或他键任务都不能证明未受理 → 不解冻、不换键、不把别的任务当本次结果；只有同键同 payload 的任务才采纳并解冻 | 同上 |
| C1-11c | 互斥边界（白盒：三失败态后端仍持锁） | 三失败态只能走 retryTask，不得铸新键新建；互斥已释放的终态才允许显式新确认 | 同上 |
| C1-12 | 409 冲突（来源版本已变化） | `conflict=true`、草稿保留、错误文案不被解析猜状态；404/传输错只向上层暴露一次 | 同上 |
| C1-13 | 轮询有界（白盒：轮询纪律） | 按 task_id 只发 GET；终态停；查询失败保留已知任务并标可恢复（不伪造后台失败/完成）；预算耗尽停并可手动恢复（只查同一 job） | 同上 |
| C1-14 | 切标签/卸载（迟到响应与卸载守卫） | 旧标签迟到响应不污染新标签（只维护该标签冻结）；卸载后不再发请求、不复活 refs | 同上 |
| C1-14b | 生命周期契约（白盒：dispose vs reset） | 组件/effect scope 自动 dispose；scope 外 dev 提示 + 显式 dispose；dispose 后不再写 refs/轮询；reset 只是清当前选择，随后仍可加载新标签；reset 后未知意图可恢复 | 同上 |
| C1-15 | 明确重试（失败等待明确重试） | 只打 retry 端点、不新建任务、不改 expected_version/new_description；retry 双击只发一次；retry 与 submit 互斥 | 同上 |

## 变体走查（五组）

- **输入**：空串、ASCII/全角/tab纯空白→拒；纯分隔符（单/连/首尾）按当前三规则闭集为合法，不擅加黑名单；单token=label拒、其他合法token不一刀切；大小写/引号/HTML/反斜线正确传输并安全展示，不拼SQL/模板；499/500字符允许、501拒，Unicode rune计量。LLM空清单/未知分类/缺证据/非法auxID/重复aux必须校验，不能变成错误执行入口。
- **前置**：无版块/无匹配/无triage/无任务/空全活跃范围；单tag/单近邻；重复幂等键；越界job/不存在/disabled/非aux；三病部分信号不足；同标签已有未结束任务。0匹配范围也必须真实完成原子保存后才称更新结束。
- **时间窗口**：trend=1/90合法、0/91按既有处理；快照新旧不混。任务范围冻结前后新tag、范围内后来disabled/删除、lease失效边界与服务重启保留。快照日期由服务端生成，不新增客户端时区计算（该变体划除）；LLM单轮不声称并发安全（划除），修复并发仍必须测。
- **幂等**：同键同payload、同键不同payload、同标签异键并发；创建/生成后未保存/保存事务中/提交后未匹配/匹配中断各故障窗；运行中retry、终态retry、仅失败项续跑、已成功项不重复；网络超时不换key重提。
- **可用性**：输入错误、空态、错误态必须测；加载/stale/轮询断网、超长证据/描述/版块名、重复提交、返回/重新进入、窄屏375/390、双主题/桌面1440/1920。固定词根正则不从用户输入拼接，正则元字符注入变体不适用（文本传输本身仍测）。
- **执行与恢复变体（task 2.5 追加）**：租约在等标签行锁期间到期→拒绝保存、旧描述/旧向量不动；提交标记条件失败→整个事务回滚（含已写 label/向量）；同 owner 并发两次执行→第二次 busy 且不改第一次的任务状态、embed 调用数仍为 1；失败后明确重试→旧 error 清空、embedding 调用数按重试次数增加；过期后新 owner/token 接管→旧在途调用不得提交；不再适用项（删除/禁用/合并）→not_applicable 持久化且不计成功；后来新增的 tag 不入旧范围；真实引擎（既有 MatchTopicTag）至少一条持久匹配；重试只补失败/未完成项、累计不重复；**阻塞 embed 跨初始 TTL 但心跳续租后成功**；**被接管后心跳不复活过期租约、老 actor 拒写**；**保存后中断由新执行器恢复且 embedding 次数不增、committed_version/at 不变**；**明确失败空轮询不自动跑、显式 retry 后仅一新尝试**；**重复 Start/Stop 幂等无泄漏**；**HTTP 请求 ctx 取消不取消 app 任务**。
- **修复执行变体（task 2.3/2.4 追加）**：租约在等标签行锁期间到期→拒绝保存、旧描述/旧向量不动；提交标记条件失败→整个事务回滚（含已写 label/向量）；同 owner 并发两次执行→第二次 busy 且不改第一次的任务状态、embed 调用数仍为 1；失败后明确重试→旧 error 清空、embedding 调用数按重试次数增加；过期后新 owner/token 接管→旧在途调用不得提交；不再适用项（删除/禁用/合并）→not_applicable 持久化且不计成功；后来新增的 tag 不入旧范围；真实引擎（既有 MatchTopicTag）至少一条持久匹配；重试只补失败/未完成项、累计不重复。

## 效果核对（不由桩测试替代）

触发原因：分类效果依赖真实LLM与真实语义分布。按 tasks 5.4 在保留快照/备份前提下经应用正常体检入口抽检；只读抽样不用副作用SQL验证。锚点：中国国内新闻的上海/中国媒体/中国大陆为rewrite，中国央行/全国人大与科技巨头短名不可因低cosine判remove；中东德黑兰低位次不是污染充分证据。量化记录抽样数、各类人眼一致性、短名remove误判数（必须0）与证据。未触发/不可用则保持该任务未完成并列阻塞，不宣称质量达标。真实描述修复必须另经用户逐项确认，不能为验收自动批量改真库。

## 白盒附加：分支与边界

| 分支 | 边界/断言 | 层 |
| --- | --- | --- |
| 磁铁 | gap=0.08±ε、失败样本4/5、任侧0/NULL；失败对手挂载不作可靠流向 | 纯函数/PG |
| 最近邻 | 3/2/0条、排除自身/disabled/非aux、缺自身向量 | PG |
| 词根 | 空/未命中/多个词根；固定正则 | 纯函数 |
| LLM | 60/61标签截断、四类/非法类别/坏JSON/超时、短名多归属保护 | 桩/提示词锚定 |
| 原子更新 | label/aliases/merge_embedding字节不变；版本变化或DB失败两列均不变 | PG |
| 持久任务 | 同标签互斥、同键payload冲突、job/label关联校验 | PG/handler |
| 计数口径 | P+F+NA+remaining=total；NA≠成功；remaining=0且F=0才 completed | PG |
| 执行权 | 同 owner 并发第二次 busy；过期后新 token 接管；失权不提交 | PG |
| 扫描/领取 | pending 与失租未结束阶段可领；明确失败不自动领；多实例只一方领取 | PG |
| 心跳 | 只有活 owner 且未过期才续租；不复活过期租约、不重置 phase_token | PG |
| retry | 同任务重排/no-op/409/404；不改版本与范围 | PG + handler |
| 故障窗W1/W2 | 创建响应丢失复用key；生成后未保存可重嵌，旧值仍在 | PG |
| 故障窗W3/W4 | 保存事务失败回滚；保存后恢复从matching续，embed次数恒1 | PG |
| 故障窗W5 | 固定范围逐项持久；失败/未完成续跑，不漏未归属者 | PG |
| 缓存 | 保存后/匹配重试前失效无TTL cache，不继续吃旧向量 | PG/集成 |
| 显示字段 | 诊断来自快照triage，实时版本/共享范围/任务阶段和计数来自repair API；旧评分不被本地标记覆写 | 组件/E2E |

层纪律：repository/SQL/迁移只用临时PG testcontainer；HTTP用handler；组件必须真实挂载的确认/disabled/草稿保留；完整故事至少一个 :5100 agent-browser 落点。Scenario→文件详表见 tasks.md 验证节，交接时校准真实新增路径。
