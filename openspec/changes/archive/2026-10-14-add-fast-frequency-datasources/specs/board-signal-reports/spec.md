# board-signal-reports Delta

## MODIFIED Requirements

### Requirement: 四十轮问题驱动研究

新研究loop SHALL 总决策≤40、总执行≤40，每轮至多一个call_tool/calculate/finish；数据源与本地计算合计计执行，失败也占预算。非法/重复动作不执行但计轮次。仅核定数据源白名单（七源：eia_wpsr_table1/jodi_oil_primary/wb_wdi/un_comtrade_trade/yahoo_chart/fred_series/gdelt_doc）可取数，不含web_search/代码执行。SHALL 关thinking、完整工具结果不截断、重复调用拦截，每次动作带非空question。允许提前finish，不强迫跑满或在30轮再审批。研究job超时 SHALL 与轮数预算对齐（150分钟；发现保持30分钟），超时/取消/失败时 SHALL 将已完成的轮次成果（轮次计数+账本：调用/观测/计算/缺口）持久化到研究进展表并保留可查，不得随job终止丢失；成功落库报告后进展行 SHALL 标记superseded保留。进展表不回写候选快照、不替代不可变报告。同一数据源连续无覆盖或失败时，SHALL 向研究历史注入如实反馈引导提前收束，不诱导换参刷轮次探明能力，不改变40轮上限与计轮规则；进程重启后残留的 running 进展行 SHALL 在启动时收敛为可查终态，不得永久悬挂。

#### Scenario: 空手源提示早停

- **WHEN** 同一数据源连续3次返回无观测或源错误
- **THEN** 后续研究历史收到「该源无覆盖、勿再换参重试、白名单源皆无则直接 finish」的反馈（每源最多一次，全部源触发后再提示一次终局）；不改40轮上限、不强制终止、反馈不伪装成工具结果

#### Scenario: 进程重启收敛孤儿进展

- **WHEN** 研究进行中进程被终止，随后后端重启
- **THEN** 启动收敛把残留 running 行标为 abandoned 且 stop_reason=orphaned_by_restart，候选可查到该次研究已终止与当时轮次，UI 不永久显示研究中

#### Scenario: 十几轮查询正常完成

- **WHEN** 第17轮完成研究且未超预算
- **THEN** 正常进入成文，不因旧4次/8轮限制提前截停

#### Scenario: 达到四十轮收束

- **WHEN** 第40轮或第40次执行完成且还想继续
- **THEN** 不再执行第41次，stop_reason=budget_exhausted，以已有证据成文并披露影响结论的缺口

#### Scenario: 重复与非法动作有限结束

- **WHEN** 模型持续发重复参数或未授权动作
- **THEN** 不执行，但每次消耗轮次，最多40轮结束，不无限循环

#### Scenario: 工具失败和任务超时不同

- **WHEN** 单源失败或全部无匹配数据，或者整个job超时/取消
- **THEN** 前者记gap可继续/无数据成文；后者failed可人工重试，不伪装正常预算收束

#### Scenario: 超时后进展不白跑

- **WHEN** 研究进行到第N轮时job超时/取消/失败
- **THEN** 进展表保留前N轮的轮次计数与全量账本（调用/观测/计算/缺口），候选可查上次进展与失败原因，可人工重试；重启后进展仍可查

#### Scenario: 成功后进展归档

- **WHEN** 研究成功落库signal_report
- **THEN** 对应进展行标记superseded保留供追溯，不删除不覆盖报告账本

### Requirement: 报告数据时效声明

成文 SHALL 基于账本机械计算各源最新可得期并注入研究时点，正文 facts 段 SHALL 出现「数据截至」与最新数据期，缺失则回注重试（沿用成文重试上限）；各源最新可得期 SHALL 按源频度原样呈现——月度/年度源为 YYYY-MM/YYYY 形态，日度源（价格/事件热度）为 YYYY-MM-DD 形态，混合时「数据可用性」行各源期值并排、facts 段「数据截至」取其中时间最大者原样引用；期值比对 SHALL 在月度源间保持月粒度、含日度期值时按日粒度精确匹配。阅读视图 SHALL 由已入库 payload 机械渲染「数据截至 {最大期} · 研究时点 {cutoff}」，无观测期时如实显示无可用数据期。SHALL NOT 改变 payload schema 与 API 契约。

#### Scenario: 正文与阅读视图都标数据截至

- **WHEN** 账本最新观测期为 2026-07、研究时点为 2026-09-22
- **THEN** 正文 facts 段含「数据截至 2026-07」，阅读视图标题下渲染机械 as-of 行，读者能看出数据滞后

#### Scenario: 日度源参与时效声明

- **WHEN** 账本含 yahoo_chart 观测（最新 2026-10-01）与 jodi_oil_primary 观测（最新 2026-07）
- **THEN** 「数据可用性」行两源期值并排（yahoo 2026-10-01、jodi 2026-07），facts 段「数据截至 2026-10-01」（时间最大者），期值比对按日粒度通过

#### Scenario: 时效缺失回注重试

- **WHEN** 首稿正文未出现「数据截至」
- **THEN** 校验回注重试，成功或耗尽重试上限前不入库
