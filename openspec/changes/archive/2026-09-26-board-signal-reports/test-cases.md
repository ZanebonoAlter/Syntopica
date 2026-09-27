# test-cases: board-signal-reports

> 这是未执行的交付账本。纯解析/计算用无DB单测；SQL/repository/迁移用隔离PostgreSQL（禁止业务库DDL、禁止SQLite替代）；HTTP用stub handler；组件用Vitest maxWorkers=2。完整交互暂用「人工：静态:5100+浏览器网络断言」作为opencli替代，实施时留证据。原型合成演示不等于业务测试通过。
>
> **阶段4执行证据（2026-09-22，cwd=backend-go，全部 exit 0）**：S1=`go test ./internal/dataenrichment/handler/ -short -run "TestSignalDiscovery|TestSignalList"`（11绿）+ `go test ./internal/dataenrichment/repository/ -run "TestCreateSignalDiscoveryBatch|TestListSignalCandidatesByPeriod|TestSignalEvidenceRefHelpers"`（隔离PG 6绿）；S2=`go test ./internal/dataenrichment/handler/ -short -run "TestSignalResearch|TestSignalReports"`（9绿）；S3=`go test ./internal/dataenrichment/service/ -short -run "TestSignalResearch_"`（16绿）；S4=`go test ./internal/dataenrichment/service/ -short -run "TestSignalCalc_"`（9绿）；S5=`go test ./internal/dataenrichment/service/ -short -run "TestSignalCompose_"`（9绿）；PG约束=`go test ./internal/dataenrichment/repository/ -run "TestCreateSignalReportResult|TestSignalReportDBConstraints|TestSignalReportVersions|TestListSignalReportResults"`（隔离PG 4绿，DB直写非法形状被CHECK/FK拒绝）。人工验收 6.T4/T5 未做，留阶段6。

## 故事S1：发现后留下候选，等我决定

锚：周期材料与固定候选快照、信号发现与人工研究边界、候选持久化与展示来源、工作台界面、仅手动触发。

### 主链路

| 步 | 动作 | 来源Scenario | 期望 | 层/规划落点 |
| --- | --- | --- | --- | --- |
| 1 | 选period点发现 | 周期校验；仅手动触发 | 合法202，非法400无job | handler/signal_discovery_test.go |
| 2 | 查历史材料 | 历史材料隔离；周期筛选翻历史 | 非active历史材料可用，不混后期 | service/signal_material_test.go |
| 3 | detect得多条信号 | 达标只生成候选；依据白名单 | 事务保存批次/候选，无取数/计算/成文 | service/signal_detect_test.go + handler/signal_discovery_test.go |
| 4 | 等待、刷新、重启 | 刷新与重启保留候选；列表字段可追溯 | 候选保留、待研究，无后台续跑 | repository/signal_candidate_test.go + 人工：刷新并核对网络无research POST |
| 5 | 再次发现无结果/失败 | 重复发现不覆盖；零信号零打扰；检测故障可见 | 空不清旧，故障有错，不存半批 | handler/signal_discovery_test.go |
| 6 | 查来源与旧记录 | 证据链 tooltip 不跳转；兑现度复盘可见；契约为侦探墙铺路 | 原地依据/结构字段；旧命中字段仅API保留 | SignalCandidateList.test.ts + 旧handler兼容测试 |

## 故事S2：只研究我选的一条，完成直接阅读

锚：单候选人工触发与幂等、四十轮问题驱动研究、成功快照与无评审输出、失败恢复与周期查询。

| 步 | 动作 | 来源Scenario | 期望 | 层/规划落点 |
| --- | --- | --- | --- | --- |
| 1 | 多候选中选一条 | 工作台呈现候选与报告；点击一条只研究一条；简报到调查由用户确认 | 只发一条research，其他不运行 | SignalCandidateList.test.ts + 人工：:5100网络计数 |
| 2 | 发现后很久才点 | 迟些点击仍用原快照；外部数据边界 | 读取冻结快照/cutoff，不换新闻 | service/signal_material_test.go |
| 3 | 并发重复点/再研究 | 重复点击与重新研究；跨板块拒绝 | 单job+409，已有报告200，显式重做202，异板块404 | handler/signal_research_test.go |
| 4 | 正常研究17轮/40轮 | 十几轮查询正常完成；达到四十轮收束；重复与非法动作有限结束 | 第17轮不截停，第41轮不执行，有stop_reason | service/signal_research_test.go |
| 5 | 成功阅读 | 报告完成直接可读；报告阅读无需评审 | 结果可读，judge/digest/review调用与写入0 | service/signal_compose_test.go + SignalReportView.test.ts |
| 6 | 查询历史版本 | 周期分页；旧数据与新版并存 | 稳定游标，周期过滤，追加不覆盖 | repository/signal_report_test.go |
| 7 | 失败/重启 | 工具失败和任务超时不同；研究失败可重试；重启恢复 | 单源缺口可成文，job终止有错，候选可手动重试 | handler/signal_research_test.go + 人工：job404后停止轮询 |

## 故事S3：从论据读到判断，数字可核查

锚：代码计算与公式证据、报告论证与条件判断、观测与计算引用附录。

| 步 | 动作 | 来源Scenario | 期望 | 层/规划落点 |
| --- | --- | --- | --- | --- |
| 1 | 原值进行计算 | 同单位差值与百分比；受限兼容流量；缺失与非法计算 | −1、−0.2381%、2800，混单位/脚本拒绝 | service/signal_calculation_test.go |
| 2 | 形成有限结论 | 不足以转多也能成文；结构与篇幅校验 | 可维持判断；缺字段/3000字符重试 | service/signal_compose_test.go + 人工：逐句对应证据 |
| 3 | 读图与附录 | 图表降级；精确引用与定位；图表口径不混淆；完整账本 | 无序列0图、null非0、计算输入可查、无50点裁切 | service/signal_compose_test.go + SignalReportView.test.ts |

## 故事S4：旧契约不被新研究悄悄改变

锚：显式历史窗口与原参数兼容、工具适配不改现有工具面、数据增强手动触发。

| 步 | 动作 | 来源Scenario | 期望 | 层/规划落点 |
| --- | --- | --- | --- | --- |
| 1 | 旧/新窗口请求 | EIA历史窗口；JODI原month兼容；JODI显式多年；回退只限缺省本年 | 缺省不扩大、month可用、years互斥、404不重复 | datasources/sources/eia_test.go、jodi_test.go |
| 2 | 新研究与旧agent | 研究显式授权；注入后现有工具面不变 | 只新research四源，discovery无取数 | datasources/wiring/tools_test.go + service/signal_research_test.go |
| 3 | 旧API/主数据 | 旧新入口语义隔离；只读不污染主数据 | 旧brief仍原kind，主表零写，旧review不删除 | 旧handler/enrich_board测试 + 新编排测试 |

## 故事S5：持久化不能接受错误归属

锚：候选持久化与展示来源、成功快照与无评审输出。

| 步 | 动作 | 来源Scenario | 期望 | 层/规划落点 |
| --- | --- | --- | --- | --- |
| 1 | 绕repo直接写入 | PG直接写入约束 | 非法owner/period/跨板块候选FK拒绝 | repository/signal_report_test.go，隔离PG |
| 2 | 候选事务中途失败 | 重复发现不覆盖 | 批次/候选全回滚，旧记录不动 | repository/signal_candidate_test.go，隔离PG |
| 3 | 旧库迁移再研究 | 旧数据与新版并存 | 新列旧行NULL，版本追加无覆盖 | repository/signal_report_test.go，隔离PG |

> 上表后端省略共同前缀`backend-go/internal/dataenrichment/`；datasources行前缀为`backend-go/internal/`；前端前缀为`front/app/features/tags/components/`。新落点尚未创建，实施后任务映射同步。

## 变体走查（五组固定清单）

| 组 | 条目与答案 | 层/落点 |
| --- | --- | --- |
| 输入 | period空串/全角空白/tab/纯分隔符/单token/首尾多分隔符/大小写MONTH/特殊字符/超长串：严格400不发LLM；LLM标题空白或超预算：解析拒绝有界重试 | PC-1、SD-1 |
| 前置 | 空集no_signal；单元素可选；同批重复去重；跨板块/悬空候选404；增强未开启400；部分源不可用gap不伪造成功 | SD-2、API-3/4、LP-5 |
| 时间 | 起点包含/终点排除、cutoff前后一步；无材料空窗口；跨月/年不混；业务时区统一；显式month与years互斥 | PC-2/3、HR-3 |
| 幂等 | discovery重复追加批次；研究并发单任务；成功默认再点复用；部分失败需人工重试；regenerate才追加 | DB-2、API-5/6 |
| 可用性 | 非法输入保留选择并显错；空态不清旧；检测错误不同于无信号；loading有真实计数；长标题/问题折行可读；重复提交恢复原job；375px不依赖hover | FE-1至FE-12 |

不适用：无搜索语法/任意代码输入，故不设计“容错执行脚本”；所有脚本样输入直接拒绝。不新增自动重试队列、报告审批或预测命中率测试（新链明确不提供）。

## 继承与调整（已运行test-assets反查）

已运行`bash scripts/harness/test-assets.sh data-enrichment`和`research-data-sources`，两者返回0。历史映射有源路径省略sources/的旧痕迹，下表按当前存在路径定位；未验证旧文件每条断言已覆盖，实施时补精确case。

| 旧Scenario | 处置 | 旧测试文件/资产 | 动作 |
| --- | --- | --- | --- |
| 周期筛选翻历史 | 继承并扩展候选 | front/app/features/tags/components/BoardEnrichmentPanel.test.ts | 保留新闻历史断言，新候选按period断言 |
| 证据链 tooltip 不跳转 | 改为新引用；旧结构保留 | BoardEnrichmentPanel.test.ts（同上） | 移除新工作台legacy入口预期，新增引用展开定位 |
| 兑现度复盘可见 | 新UI退役，旧API保留 | backend-go/internal/dataenrichment/handler/board_enrichment_handler_test.go | 旧字段可读；新报告无兑现/审批徽标 |
| 契约为侦探墙铺路 | 继承结构性 | backend-go/internal/dataenrichment/repository/result_kind_repository_test.go | 保留旧形状测试，新增结构引用 |
| 简报到调查由用户确认 | 新界面两阶段，旧API不动 | BoardEnrichmentPanel.test.ts、旧board handler测试 | 新发现不得串发研究，旧调查不自动触发 |
| 仅手动触发 | 扩为两次点击 | backend-go/internal/dataenrichment/service/enrich_board_test.go | 旧API原断言照跑，新增人工门禁负向断言 |
| 只读不污染主数据 | 继承 | backend-go/internal/dataenrichment/handler/board_enrichment_handler_test.go | 新发现/研究也验证主表无写 |
| 注入后现有工具面不变 | 唯一新research例外 | backend-go/internal/datasources/wiring/tools_test.go | 旧集合逐一不变，新发现无取数 |
| 指定月份取数带元数据/最新期缺值不回退/本年404回退一次 | 继承 | backend-go/internal/datasources/sources/jodi_test.go | 原单月回归+years显式互斥/去重 |

## 白盒附加：分支与边界

| ID | 分支/输入 | 明确预期 |
| --- | --- | --- |
| PC-1 | 合法/月13/week/空白/超长period | 合法贯穿，其余400无job |
| PC-2 | 时区边界/未来 | 半开区间；未来400 |
| PC-3 | 历史非active/混后期摘要 | 历史材料可选；后期排除/原切片降级 |
| PC-4 | 空材料/归属不可还原 | 空批次或gap，不捏造历史归属 |
| PC-5 | 候选生成后新新闻/工具期越界 | 冻结快照；工具过滤先于agent |
| SD-1 | 合法JSON/修复包装/非法字段 | 合法解析；非法最多2尝试后failed |
| SD-2 | []/空材料 | no_signal、无工具/compose、旧列表不变 |
| SD-3 | score5/6/11/6.5 | 5过滤、6入选，越界/非整数非法 |
| SD-4 | 悬空或跨板块依据 | 引用剔除，无依据信号丢弃 |
| SD-5 | 多条达标 | 保存多候选，无自动research |
| LP-1 | 第17轮finish | 正常成文，不再4/8截停 |
| LP-2 | 39/40/41轮/次 | 39可续；40后收束；41不执行 |
| LP-3 | 20取数+20计算 | 总40执行，无独立额外计算预算 |
| LP-4 | 重复/未知工具连续40轮 | 不执行，轮数仍增加，有限结束 |
| LP-5 | 部分/全源失败 | failed调用账本+gap，可无数据成文 |
| LP-6 | context取消/超时/容量不足 | failed，不静默截断或自动续跑 |
| LP-7 | finish早于上限 | 可提前结束，question必填，thinking关闭 |
| CA-1 | 419−420 | −1 MMbbl，有公式/输入 |
| CA-2 | (419−420)/420×100 | −0.2381%，half-up4位 |
| CA-3 | 同期6400−3600 | 2800 Mb/d，经批准进出口兼容 |
| CA-4 | mean明确同系列窗口 | 算术均值，缺期不称完整窗口 |
| CA-5 | 任一null/百分比base≤0 | null缺失不0；非正基数拒绝 |
| CA-6 | 混单位/库存减流量/跨源/脚本 | 拒绝，无任意执行 |
| CA-7 | forward/计算结果作输入/模型自带value | 拒绝；首版仅原观测输入 |
| HR-1 | EIA weeks未传/1/12/13 | 未传旧两期；范围内允许；13发网前拒绝 |
| HR-2 | JODI不传years，不传或显式month | 旧最新月或历史单月，不默认全年 |
| HR-3 | years1/5/6或与month同传 | 显式范围允许；6/同传拒绝 |
| HR-4 | 本年404、上一年已请求 | 回退一次并去重 |
| HR-5 | 显式month404/历史年404/null最新期 | 不回退；gap；不替旧值 |
| HR-6 | 缓存命中/漂移/HTML伪装 | 保留原元信息；漂移驱逐；错误不缓存 |
| HR-7 | 旧A/B/QA/调查/发现与新research | 旧集合不变，发现0取数，仅新研究四源 |
| SV-1 | 四段齐/缺段/未知kind | 合法通过；缺/未知拒绝 |
| SV-2 | 缺条件字段/direction非法 | 错误回注，不声称检测语义真伪 |
| SV-3 | 2999/3000字符 | 前者通过后者拒绝，附录/UI不计 |
| SV-4 | 原值/计算悬空、失败计算被引用 | 拒绝；合法引用数值代码渲染 |
| SV-5 | 无相关序列/有序列/混频图/null | 0图+gap/1～3图/拒绝混频/null断点 |
| SV-6 | 3次成文全失败 | attempts3/retries2，无result |
| SV-7 | 数据不足以做方向判断 | 允许conditional维持判断，真实内容人工核对 |
| DB-1 | 候选与批次owner/period不符 | 隔离PG直写拒绝 |
| DB-2 | 中途事务失败/重复发现 | 无半批；成功追加、不覆盖旧 |
| DB-3 | result非法kind/owner/周期/候选FK | 隔离PG CHECK/FK拒绝 |
| DB-4 | 旧库迁移重复执行 | 旧行新列NULL、原载荷不变 |
| DB-5 | 同候选再研究 | 新id快照，不覆盖旧版本 |
| API-1 | discovery合法/关闭/未来 | 202/400/400，无自动report |
| API-2 | signals分页/空新批 | 正确period游标，保留旧候选 |
| API-3 | 跨板块candidate/result | 404 |
| API-4 | 浏览器自带材料/cutoff | 拒绝未知字段或不采用，使用服务端快照 |
| API-5 | 并发/成功后默认重试 | 一个job+409/200已有，无额外LLM |
| API-6 | regenerate=true且无running | 202新任务，共享board锁 |
| JB-1 | discovery成功0/多条/失败 | no_signal/discovered/failed，无result_id |
| JB-2 | research成功/失败 | result_id仅成功存在，无nil解引用 |
| JB-3 | 内存job重建 | 404停止轮询，候选不永久running |
| JB-4 | 同board旧brief/investigation运行 | 新发现/研究均409，等待人工时不持锁 |
| NR-1 | 首篇/后续报告成功 | judge/digest/review写入和调用0 |
| NR-2 | 全链路前后主数据 | 报告不改新闻/lifeline；freshness独立写入保留 |
| FE-1 | 点击发现 | 只发discovery，候选显示后停止 |
| FE-2 | 候选多条选择一条 | 仅对应research |
| FE-3 | 字段/长标题/来源 | 可读可追溯，显示状态非LLM自报 |
| FE-4 | 无信号/发现错误 | 不清旧候选；错误区别空态 |
| FE-5 | 刷新候选 | 持久化后仍待研究 |
| FE-6 | 进度17/40/提前结束 | 真实取数/计算数，非必跑40 |
| FE-7 | data/calc点击或触摸 | 展开附录定位，公式输入可查 |
| FE-8 | 0图/派生图/null | 来源标记，缺口/断点正确 |
| FE-9 | 历史模式/返回列表 | 事后回顾标记与period保持 |
| FE-10 | 新报告UI | 无review操作/徽标 |
| FE-11 | 旧面板卸载 | 新闻背景保留、旧API可读 |
| FE-12 | job404/重试 | 停轮询、可人工重试，不自启研究 |
| OB-1 | discovery/research会话 | 独立session以candidate关联，无等待长驻会话 |
| OB-2 | 超50点/工具/计算日志 | 完整原响应、附录筛选全集、公式/计数可重建 |
| OB-3 | 真实三类板块样本 | 记录候选数/选择数/决策数/工具数/gap/成功率，逐条核对结论证据 |

## 效果核对与样例

- 触发原因：依赖LLM发现质量、官方数据覆盖和因果论证，schema通过不足以保证效果。
- 方法：实现后在真实当前/历史/无源板块各选一个案例；只对用户明确选择的信号研究，记录真实数据时间/hash、调用数和未覆盖指标；无信号照实记录，不凑成功。
- 量化结果：**待实施**，不得用合成fixture填业务成功率。
- 结论：**待真实样本验收**。若只因数据能力不足，明确上游缺口，不声称加轮次就能补齐指标。
- `sample-report.md`为合成fixture内容验收样张，数值/图/公式须一致、正文<3000、无开工率/历史胜率伪数据；不是40轮实测或投资结论。
- 白盒附加 PC-1~PC-5/SD-1~5/LP-1~7/CA-1~7/SV-1~7/API-1~6/JB-1~4/NR-1~2 及 DB-1~5 的自动化用例已随上列 S1~S5/PG约束命令全部跑绿（分支覆盖明细见各测试文件）；HR-1~7 同批随 `go test -short ./internal/dataenrichment/...` 与 datasources 既有用例守护。OB-1/OB-2 由 LP/CA/SV 用例与完整账本断言覆盖；**OB-3（真实三类板块样本量化）待人工 6.T5**。FE-1~FE-12 自动化部分见 phase-3-report ④（76 用例全绿）；6.T4 全链路人工走查与 8.5 双视口验收待人工。
