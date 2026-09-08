## MODIFIED Requirements

### Requirement: 态势卡素材装配

版块级分析的素材注入 SHALL 以每泳道「态势卡」形态压缩（事实摘要 + 命中统计 + 质量信号），MUST NOT 将全部泳道的各粒度背景记忆全文拼接注入（防 token 爆炸）。态势卡事实摘要取材链 SHALL 为：**week 档 lifeline 摘要 → month 档 lifeline 摘要 → 近期 section 事实指纹 → 泳道描述 → 空**。month 档 SHALL 作为 week 缺失时的主要事实源（与循环 A 的实际覆盖形态对齐——month 由月度定时任务全量维护，week 依赖周任务逐周积累）；新鲜度门补齐的 month 档 SHALL 被本取材链消费（补齐成果不得白费）。section 指纹降级 MUST 携带 section 级实质事实内容（摘要/事实指纹），MUST NOT 输出仅含泳道名与篇数的同义反复。卡片事实来源（facts_source：lifeline_week | lifeline_month | section_fingerprint | description | none）SHALL 可追溯，供素材质量排查与效果核对。质量信号中的信息密度 SHALL 反映背景记忆丰满度（含 month 档可用性），MUST NOT 仅由文章篇数决定。

agent 需要某泳道论据细节时经泳道详情工具下钻获取（工具的素材可见范围见 `data-enrichment` 的「探索 agent 工具集」）。

#### Scenario: week 缺失时 month 兜底

- **WHEN** 泳道无 week 档 lifeline 但存在 month 档（生产库常态形态）
- **THEN** 态势卡事实摘要取自 month 档（卡片标注来源为 month），不降级为 section 指纹

#### Scenario: 指纹降级有实质内容

- **WHEN** 泳道无任何 lifeline 档、仅有 daily_report section 可用
- **THEN** 指纹携带 section 级事实内容（摘要/事实指纹），不是「泳道名 (N篇)」形态；确无可读内容时如实标注空，不编造

#### Scenario: 泳道多时素材仍可控

- **WHEN** 板块活跃泳道数量较多（如 ≥10 条）
- **THEN** 分析照常执行，注入素材总量受态势卡形态约束，不因泳道数量线性膨胀为全文拼接

#### Scenario: 无任何素材时卡片仍产出

- **WHEN** 泳道既无 lifeline 记录也无可用 section
- **THEN** 卡片仅含命中统计 + 质量信号并标注无事实源，装配不报错、该泳道不静默消失

#### Scenario: 低质量泳道不被静默删除

- **WHEN** 某泳道长期无命中或历史稀疏
- **THEN** 其卡片降权和缩短但仍可追溯，系统不得仅因质量低便从版块成员中删除

## ADDED Requirements
### Requirement: 版块分析工作台信息架构

版块数据增强工作台 SHALL 以版块级分析为主视图（最新报告 + 历史选择 + 触发入口）。泳道选择 SHALL 收敛于聚焦分析区的单一下拉——MUST NOT 在页面中存在两处控件绑定同一泳道选择状态（新旧堆叠）。聚焦分析区保留追问（QA）随展开可达。新闻背景（循环 A 记忆）SHALL 保留可及入口，MUST NOT 以仅含单一选项的 tab 栏形态呈现。

#### Scenario: 单一下拉选择泳道

- **WHEN** 用户在工作台选择聚焦分析的泳道
- **THEN** 页面仅存在一处泳道下拉（聚焦分析区），顶部不再有旧版话题选择条

#### Scenario: 新闻背景入口保留

- **WHEN** 用户需要查看该板块/泳道的新闻背景记忆（循环 A）
- **THEN** 工作台提供非单 tab 栏形态的可及入口，行为与改版前一致（周期筛选/翻历史/inline 编辑不回写分析）

## ADDED Requirements

### Requirement: 分析触发异步化

循环 B 分析（板块档与单泳道档）SHALL 以异步方式触发：触发接口立即返回（含已在跑的冲突码），分析在脱离 HTTP 请求生命周期的后台上下文中执行并照常落库。分析执行 MUST NOT 因客户端断开（离开页面/关标签页/网络抖动）而中止——同步 HTTP 下 request-context 取消会作废整次已跑的分析（含补全门备料）。同目标重复触发 SHALL 被拒绝；分析完成/失败状态 SHALL 可被前端轮询，完成后可拿到落库结果 id。未开启增强的板块 SHALL 在触发时同步拒绝（4xx 语义保留）。

#### Scenario: 触发立即返回且断连不中止

- **WHEN** 用户触发板块/单泳道分析后在完成前离开页面（或网络断开）
- **THEN** 触发接口立即返回「已开始」，后台分析照常跑完并落库；用户回界面轮询状态可拿到结果 id

#### Scenario: 同目标防重入

- **WHEN** 同一板块/泳道已有分析在后台执行时再次触发
- **THEN** 触发接口返回冲突（409），不启动第二个并行分析；跑完后可再次触发

#### Scenario: 分析状态可轮询

- **WHEN** 前端在触发后或进入面板时查询分析状态
- **THEN** 状态接口返回 running/finished/error/result_id；从未触发或进程重启后视为空闲（running=false），不拖垮后续触发
