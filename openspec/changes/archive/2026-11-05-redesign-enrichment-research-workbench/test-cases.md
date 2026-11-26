# 精准故事用例与回归处置

> 仅规划，未实现/未运行产品测试。一个Requirement一个故事，关键节拍如下；每个Scenario的测试落点只维护在tasks.md，避免复制验收工程。优先扩展既有测试，相关边界用参数表合并，不要求一Scenario一个测试函数。

## 故事主链

### S1: 候选持久化与展示来源

| 步 / 动作 | 来源 Scenario | 期望 | 层 / 落点 |
| --- | --- | --- | --- |
| 1 进入场景 → 2 执行指定动作 → 3 返回/重进核对 | 刷新与重启保留候选；其余变体见tasks同Requirement映射 | 候选及其依据仍可查，无隐式研究启动，不留永久running状态 | backend-go/internal/dataenrichment/repository/signal_candidate_test.go<br>front/app/features/tags/components/BoardEnrichmentPanel.test.ts |

### S2: 单候选人工触发与幂等

| 步 / 动作 | 来源 Scenario | 期望 | 层 / 落点 |
| --- | --- | --- | --- |
| 1 进入场景 → 2 执行指定动作 → 3 返回/重进核对 | 点击一条只研究一条；其余变体见tasks同Requirement映射 | 仅该候选生成202研究job，其余不调用数据源或成文 | backend-go/internal/dataenrichment/handler/signal_research_test.go<br>front/app/features/tags/components/SignalCandidateList.test.ts |

### S3: 报告论证与条件判断

| 步 / 动作 | 来源 Scenario | 期望 | 层 / 落点 |
| --- | --- | --- | --- |
| 1 进入场景 → 2 执行指定动作 → 3 返回/重进核对 | 不足以转多也能成文；其余变体见tasks同Requirement映射 | 报告明确证据边界与需要的后续条件，可维持判断，不编造开工率/历史胜率凑结论 | backend-go/internal/dataenrichment/service/signal_compose_test.go<br>front/app/features/tags/components/SignalReportView.test.ts |

### S4: 报告数据时效声明

| 步 / 动作 | 来源 Scenario | 期望 | 层 / 落点 |
| --- | --- | --- | --- |
| 1 进入场景 → 2 执行指定动作 → 3 返回/重进核对 | 正文与阅读视图都标数据截至；其余变体见tasks同Requirement映射 | 旧facts原文不改；读取可见该最大期、研究边界及源时效，不把旧数据标为九月已验证 | backend-go/internal/dataenrichment/service/signal_compose_test.go<br>front/app/features/tags/components/SignalReportView.test.ts |

### S5: 观测与计算引用附录

| 步 / 动作 | 来源 Scenario | 期望 | 层 / 落点 |
| --- | --- | --- | --- |
| 1 进入场景 → 2 执行指定动作 → 3 返回/重进核对 | 精确引用与定位；其余变体见tasks同Requirement映射 | 显示来自同一观测/计算的阅读值和原单位；点击到原值/公式；原字符串不改，悬空或计算失败的新引用拒绝 | front/app/features/tags/components/reportPresentation.test.ts<br>front/app/features/tags/components/SignalReportView.test.ts |

### S6: 成功快照与无评审输出

| 步 / 动作 | 来源 Scenario | 期望 | 层 / 落点 |
| --- | --- | --- | --- |
| 1 进入场景 → 2 执行指定动作 → 3 返回/重进核对 | PG直接写入约束；其余变体见tasks同Requirement映射 | PostgreSQL拒绝，不仅依赖前端/SQLite校验 | backend-go/internal/dataenrichment/repository/signal_report_test.go<br>front/app/features/tags/components/SignalReportView.test.ts |

### S7: 失败恢复与周期查询

| 步 / 动作 | 来源 Scenario | 期望 | 层 / 落点 |
| --- | --- | --- | --- |
| 1 进入场景 → 2 执行指定动作 → 3 返回/重进核对 | 研究失败可重试；其余变体见tasks同Requirement映射 | job错误，无新result，候选仍在且可用户手动重试；不自动恢复付费研究；错误信息携带进展摘要（已完成轮次/已取得观测数），可查完整进展账本 | front/app/features/tags/composables/useSignalWorkbench.test.ts<br>front/app/features/tags/components/BoardEnrichmentPanel.bootstrap.test.ts |

### S8: 研究工作台主从导航

| 步 / 动作 | 来源 Scenario | 期望 | 层 / 落点 |
| --- | --- | --- | --- |
| 1 进入场景 → 2 执行指定动作 → 3 返回/重进核对 | 索引切换与返回；其余变体见tasks同Requirement映射 | 保留对应阅读位置与列表选择，不重新研究；标题全文在详情可读，不硬裁证据 | front/app/features/tags/components/BoardEnrichmentPanel.test.ts<br>front/app/features/tags/components/BoardEnrichmentPanel.bootstrap.test.ts |

### S9: 候选忽略与恢复

| 步 / 动作 | 来源 Scenario | 期望 | 层 / 落点 |
| --- | --- | --- | --- |
| 1 进入场景 → 2 执行指定动作 → 3 返回/重进核对 | 忽略后重进与恢复；其余变体见tasks同Requirement映射 | 标记保留并可恢复，原内容/报告/依据不变，所有动作零LLM，其他新批次不受影响 | backend-go/internal/dataenrichment/repository/signal_disposition_test.go<br>backend-go/internal/dataenrichment/handler/signal_research_test.go<br>front/app/features/tags/components/SignalCandidateList.test.ts |

### S10: 新闻依据话题关联

| 步 / 动作 | 来源 Scenario | 期望 | 层 / 落点 |
| --- | --- | --- | --- |
| 1 进入场景 → 2 执行指定动作 → 3 返回/重进核对 | 多话题和历史话题关联；其余变体见tasks同Requirement映射 | 返回可验证关联和对应依据；可只读打开其背景，不强制建研究项目或启动聚焦分析 | backend-go/internal/dataenrichment/service/signal_material_test.go<br>front/app/features/tags/components/BoardThreadBrowser.focus.test.ts |

### S11: 工作台展示字段来源

| 步 / 动作 | 来源 Scenario | 期望 | 层 / 落点 |
| --- | --- | --- | --- |
| 1 进入场景 → 2 执行指定动作 → 3 返回/重进核对 | 展示不发明状态和质量；其余变体见tasks同Requirement映射 | UI以系统记录为准，未知计数显示未记录，不展示模型自报质量或评审徽标 | front/app/features/tags/components/BoardEnrichmentPanel.test.ts<br>front/app/features/tags/components/SignalReportView.test.ts |

### S12: 板块 tab 「认知工作台」界面

| 步 / 动作 | 来源 Scenario | 期望 | 层 / 落点 |
| --- | --- | --- | --- |
| 1 进入场景 → 2 执行指定动作 → 3 返回/重进核对 | 工作台呈现候选与报告；其余变体见tasks同Requirement映射 | 每条分别可深入分析，不自动研究；选一条不触发其余候选 | front/app/features/tags/components/BoardEnrichmentPanel.test.ts |

### S13: 报告追问交互

| 步 / 动作 | 来源 Scenario | 期望 | 层 / 落点 |
| --- | --- | --- | --- |
| 1 进入场景 → 2 执行指定动作 → 3 返回/重进核对 | 追问复用探索工具；其余变体见tasks同Requirement映射 | 原工具与引用契约保留；新工作台仅查看历史，不因阅读调用web_search或LLM | front/app/features/tags/components/QAPanel.test.ts<br>front/app/api/boardEnrichment.test.ts<br>front/app/features/tags/components/LegacyResearchHistory.test.ts |

### S14: 话题新闻背景独立上下文

| 步 / 动作 | 来源 Scenario | 期望 | 层 / 落点 |
| --- | --- | --- | --- |
| 1 进入场景 → 2 执行指定动作 → 3 返回/重进核对 | 从话题直接查看背景；其余变体见tasks同Requirement映射 | 显示该话题明确周期的新闻记忆；无须展开聚焦分析，不触发研究/生成；原话题浏览器筛选与关键事件行为不变 | front/app/features/tags/components/TopicNewsBackground.test.ts<br>front/app/features/tags/components/BoardThreadBrowser.focus.test.ts |

### S15: lane 引用与单泳道下钻

| 步 / 动作 | 来源 Scenario | 期望 | 层 / 落点 |
| --- | --- | --- | --- |
| 1 进入场景 → 2 执行指定动作 → 3 返回/重进核对 | 从简报观察下钻；其余变体见tasks同Requirement映射 | 进入相应话题只读资料/背景，不打开聚焦表单、不发起分析，原observation仍可读 | front/app/features/tags/components/LegacyResearchHistory.test.ts<br>front/app/features/tags/components/BoardThreadBrowser.focus.test.ts |

### S16: 旧论文式版块报告兼容

| 步 / 动作 | 来源 Scenario | 期望 | 层 / 落点 |
| --- | --- | --- | --- |
| 1 进入场景 → 2 执行指定动作 → 3 返回/重进核对 | 查看旧报告；其余变体见tasks同Requirement映射 | 以其原kind兼容展示并标明旧版/只读；缺失字段如实标注，不能把旧JSON送模型改写成新报告 | front/app/features/tags/components/LegacyResearchHistory.test.ts<br>backend-go/internal/dataenrichment/handler/handler_test.go |

### S17: 版块分析工作台信息架构

| 步 / 动作 | 来源 Scenario | 期望 | 层 / 落点 |
| --- | --- | --- | --- |
| 1 进入场景 → 2 执行指定动作 → 3 返回/重进核对 | 单一下拉选择泳道；其余变体见tasks同Requirement映射 | 工作台不存在旧聚焦泳道选择器；关联入口带明确topic，话题背景自己持有上下文，不存在两处共享旧selectedTopicId | front/app/features/tags/components/BoardEnrichmentPanel.test.ts<br>front/app/features/tags/components/BoardThreadBrowser.focus.test.ts |

### S18: 旧研究历史只读访问

| 步 / 动作 | 来源 Scenario | 期望 | 层 / 落点 |
| --- | --- | --- | --- |
| 1 进入场景 → 2 执行指定动作 → 3 返回/重进核对 | 旧结果与所有子产物可读；其余变体见tasks同Requirement映射 | 原kind内容与来源可读，没有旧生成/追问/沉淀/采纳按钮；不触发模型，signal_report不混入历史集合 | backend-go/internal/dataenrichment/repository/legacy_history_test.go<br>backend-go/internal/dataenrichment/handler/handler_test.go<br>front/app/features/tags/components/LegacyResearchHistory.test.ts |

## 五组变体：只走相关边界，不做组合穷举

| 组 | 明确处理 | 落点 |
| --- | --- | --- |
| 输入 | ID/枚举/ignored：空串、空白(全角/tab)、纯分隔符、大小写错误、特殊字符和超长拒绝；正确单token接受。背景特殊字符按纯文保存，空白按原API校验并留草稿。只读按钮无文本输入，字符串穷举不适用 | handler/API旧测试和背景挂载补参数表 |
| 前置 | 空集、单项、重复、跨域、部分数据各一个代表：空态可辨、单项可读、忽略幂等/重复chart拒绝、跨board拒绝、缺源保留正文与gap | ignore/compose/history/reader |
| 时间 | 原半开窗口两端/空窗口沿用旧material测试；新增只盯7/8/14日期间距、原频度、跨周期晚响应、时区归一、cutoff不混created_at | presentation/state；未改取数不新铺测试 |
| 幂等 | 忽略/恢复各重复一次，写失败再试；忽略先与研究先两种并发顺序。纯GET不做新锁压力测试，因为没有新增并发写承诺 | ignore+既有research handler |
| 可用性 | 误输入、空态、失败、加载明确区分；失败留草稿，长标题仅索引省略、详情全文可读，在途重复提交禁用。不把状态×颜色×设备做笛卡尔积 | 主宿主/背景/一次浏览器主链 |

## 白盒附加：只三个风险点

- **忽略门禁**：普通/有报告可忽略，research/compose为409；已忽略需恢复才新研究；无running且有报告默认research仍200；owner错误404；两竞争顺序互斥。无需新分布式锁或压力平台。
- **数字/日期**：长小数、真0、极小正负、null/空串、非法值、超安全大数一组，断言阅读不假0且raw不变；不等距三日+显式null一组，空/单点降级沿用旧资产。不做随机大样本、像素逐点比对。
- **协议**：旧schema2适配、新schema3三类合法块，以及未知块/复制值/悬空ref/重复图/3000边界，补在既有compose参数表。三次失败不存。不修改的40轮/七源/公式/judge只继承原网，不新铺矩阵。

## 继承与调整

已执行test-assets.sh反查三个能力。以下每个旧Scenario有处置；这些大多是旧断言保留/修正，不代表新增同等数量测试。

| 旧 Scenario | 处置 | 旧测试文件 | 动作 |
| --- | --- | --- | --- |
| 刷新与重启保留候选 | 继承 | backend-go/internal/dataenrichment/repository/signal_candidate_test.go | 保留原断言，不重复新增；宿主变化只调定位 |
| 重复发现不覆盖 | 继承 | backend-go/internal/dataenrichment/repository/signal_candidate_test.go | 保留原断言，不重复新增；宿主变化只调定位 |
| 列表字段可追溯 | 改语义 | backend-go/internal/dataenrichment/repository/signal_candidate_test.go | 按delta就近调整旧断言，新增分支并入参数表 |
| 点击一条只研究一条 | 继承 | backend-go/internal/dataenrichment/handler/signal_research_test.go | 保留原断言，不重复新增；宿主变化只调定位 |
| 重复点击与重新研究 | 继承 | backend-go/internal/dataenrichment/handler/signal_research_test.go | 保留原断言，不重复新增；宿主变化只调定位 |
| 跨板块拒绝 | 继承 | backend-go/internal/dataenrichment/handler/signal_research_test.go | 保留原断言，不重复新增；宿主变化只调定位 |
| 不足以转多也能成文 | 继承 | backend-go/internal/dataenrichment/service/signal_compose_test.go | 保留原断言，不重复新增；宿主变化只调定位 |
| 结构与篇幅校验 | 改语义 | backend-go/internal/dataenrichment/service/signal_compose_test.go | 按delta就近调整旧断言，新增分支并入参数表 |
| 图表降级 | 继承 | backend-go/internal/dataenrichment/service/signal_compose_test.go | 保留原断言，不重复新增；宿主变化只调定位 |
| 正文与阅读视图都标数据截至 | 改语义 | backend-go/internal/dataenrichment/service/signal_compose_test.go | 按delta就近调整旧断言，新增分支并入参数表 |
| 日度源参与时效声明 | 改语义 | backend-go/internal/dataenrichment/service/signal_compose_test.go | 按delta就近调整旧断言，新增分支并入参数表 |
| 时效缺失回注重试 | 改语义 | backend-go/internal/dataenrichment/service/signal_compose_test.go | 按delta就近调整旧断言，新增分支并入参数表 |
| 精确引用与定位 | 改语义 | front/app/features/tags/components/SignalReportView.test.ts | 按delta就近调整旧断言，新增分支并入参数表 |
| 图表口径不混淆 | 改语义 | front/app/features/tags/components/SignalReportView.test.ts | 按delta就近调整旧断言，新增分支并入参数表 |
| 完整账本 | 继承 | front/app/features/tags/components/SignalReportView.test.ts | 保留原断言，不重复新增；宿主变化只调定位 |
| PG直接写入约束 | 继承 | backend-go/internal/dataenrichment/repository/signal_report_test.go | 保留原断言，不重复新增；宿主变化只调定位 |
| 报告完成直接可读 | 改语义 | backend-go/internal/dataenrichment/repository/signal_report_test.go | 按delta就近调整旧断言，新增分支并入参数表 |
| 旧数据与新版并存 | 改语义 | backend-go/internal/dataenrichment/repository/signal_report_test.go | 按delta就近调整旧断言，新增分支并入参数表 |
| 研究失败可重试 | 继承 | front/app/features/tags/composables/useSignalWorkbench.test.ts | 保留原断言，不重复新增；宿主变化只调定位 |
| 重启恢复 | 继承 | front/app/features/tags/composables/useSignalWorkbench.test.ts | 保留原断言，不重复新增；宿主变化只调定位 |
| 周期分页 | 继承 | front/app/features/tags/composables/useSignalWorkbench.test.ts | 保留原断言，不重复新增；宿主变化只调定位 |
| 工作台呈现候选与报告 | 继承 | front/app/features/tags/components/BoardEnrichmentPanel.test.ts | 保留原断言，不重复新增；宿主变化只调定位 |
| 周期筛选翻历史 | 改语义 | front/app/features/tags/components/BoardEnrichmentPanel.test.ts | 按delta就近调整旧断言，新增分支并入参数表 |
| 证据链 tooltip 不跳转 | 改语义 | front/app/features/tags/components/BoardEnrichmentPanel.test.ts | 按delta就近调整旧断言，新增分支并入参数表 |
| 兑现度复盘可见 | 改语义 | front/app/features/tags/components/BoardEnrichmentPanel.test.ts | 按delta就近调整旧断言，新增分支并入参数表 |
| 契约为侦探墙铺路 | 改语义 | front/app/features/tags/components/BoardEnrichmentPanel.test.ts | 按delta就近调整旧断言，新增分支并入参数表 |
| 简报到调查由用户确认 | 改语义 | front/app/features/tags/components/BoardEnrichmentPanel.test.ts | 按delta就近调整旧断言，新增分支并入参数表 |
| 报告阅读无需评审 | 改语义 | front/app/features/tags/components/BoardEnrichmentPanel.test.ts | 按delta就近调整旧断言，新增分支并入参数表 |
| 追问复用探索工具 | 改语义 | front/app/features/tags/components/QAPanel.test.ts | 按delta就近调整旧断言，新增分支并入参数表 |
| 多轮追问持久化 | 改语义 | front/app/features/tags/components/QAPanel.test.ts | 按delta就近调整旧断言，新增分支并入参数表 |
| 追问发现手动沉淀 | 改语义 | front/app/features/tags/components/QAPanel.test.ts | 按delta就近调整旧断言，新增分支并入参数表 |
| 从简报观察下钻 | 改语义 | front/app/features/tags/components/BoardEnrichmentPanel.test.ts | 按delta就近调整旧断言，新增分支并入参数表 |
| 从调查证据下钻 | 改语义 | front/app/features/tags/components/BoardEnrichmentPanel.test.ts | 按delta就近调整旧断言，新增分支并入参数表 |
| 查看旧报告 | 改语义 | backend-go/internal/dataenrichment/handler/handler_test.go | 按delta就近调整旧断言，新增分支并入参数表 |
| 单一下拉选择泳道 | 改语义 | front/app/features/tags/components/BoardEnrichmentPanel.test.ts | 按delta就近调整旧断言，新增分支并入参数表 |
| 新闻背景入口保留 | 改语义 | front/app/features/tags/components/BoardEnrichmentPanel.test.ts | 按delta就近调整旧断言，新增分支并入参数表 |

## 浏览器与效果核对

- 一条静态`:5100`隔离fixture主链：浏览→忽略→已忽略读报告→恢复→研究→跨周期阅读→返回任务→版本；顺手查看历史QA/辩论及背景保存/生成确认。不跑全站，不为每片段开浏览器。
- 同次走查取1440/1920与375/390、双主题代表证据，检查无整页溢出、弹层命中、长文可读；写ui-verification/report.md，原型不替代实现。
- 旧schema2与一种legacy各一只读样本；明确授权后一次混合证据成文样例，记录尝试数、块/引用合法性、raw保持和无HTML。其余fixture测，不为验证跑发现或40轮研究；无授权/无样本如实未完成。
- SQL/迁移在临时PG，浏览器写入对隔离fixture库；不在真库试验SQL或为截图清数据，不擅停其他会话服务。
