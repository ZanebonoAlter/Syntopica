## MODIFIED Requirements

### Requirement: 分层上下文驱动的数据增强编排

数据增强编排的入口 SHALL 是分层上下文（`topic_lifeline_context` + 14天窗口详情 + 历史 applied review），**不是单篇新闻，也不是单一 lifeline**。本条描述单泳道（topic 粒度）编排；版块简报与调查见 `board-level-analysis` capability。单泳道编排 SHALL 继续由三角色组成：

1. **解读员（结构化分析编辑）**：全层读分层上下文（按版块 `context_layers`，未生成的层跳过），提炼需补数据的研究方向，输出 JSON。SHALL NOT 硬编码特定金融方向。
2. **研究助理（agent loop）**：对研究方向使用 `web_search`、`fetch_page` 与内部导航工具搜集可核查材料；相同工具与参数的重复调用仍须拦截。
3. **分析员（结构化分析师）**：结合分层上下文与检索数据产出事实层、见解层和既有兼容的深度内容，显式给出反过度解读边界。

编排 SHALL 对单次 LLM 调用设 max_loops 上限（默认 6）。解读员 SHALL 读取历史 applied review。编排 SHALL NOT 产出已废弃的走向预测字段 `direction` / `confidence` / `horizon` / `trigger_up` / `trigger_down`。

从版块简报或调查下钻时，单泳道入口 MAY 接收可修改的预填研究问题/观察点；该输入只用于聚焦，不得作为不可推翻的既定命题。编排 SHALL NOT 注入作者画像或方法卡（方法卡体系已整体移除；单泳道不新增多假设 schema），并按「证据适配与反证纪律」SHALL NOT 使用固定证据类型配额。

#### Scenario: 消费分层上下文

- **WHEN** 触发某 topic 的数据增强
- **THEN** 解读员输入 SHALL 含配置的 context 层 + 14天窗口详情 + 历史 applied review，不得只含单篇 article

#### Scenario: 解读员领域自适应

- **WHEN** 解读员处理非金融结构话题
- **THEN** 研究方向按话题事实与用户问题生成，SHALL NOT 强制提炼 A 股 ETF 等固定方向

#### Scenario: 分析员产出深度层而非走向预测

- **WHEN** 单泳道分析员产出非 sparse 形态结果
- **THEN** 结果 SHALL 保持既有 `depth` 块兼容，且 SHALL NOT 含 `direction` / `trigger_up` / `trigger_down` 字段

#### Scenario: 死循环防御

- **WHEN** 研究助理尝试用相同参数重复调用同一工具
- **THEN** 系统 SHALL 拦截并返回已调用提示，不执行重复调用

#### Scenario: 下钻问题可修改且可推翻

- **WHEN** 用户从版块简报观察或调查证据发起单泳道分析
- **THEN** 解读员收到对应研究问题/观察点作为预填 lens，用户可修改，后续研究 MAY 得出与预填方向不同的结论

#### Scenario: 单泳道不继承作者画像

- **WHEN** 触发单泳道分析
- **THEN** 编排输入 SHALL NOT 含任何作者画像或方法卡内容
