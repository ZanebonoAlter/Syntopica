## ADDED Requirements

### Requirement: 合并端点下发日报双队列计数

日报的自动生成要求两个消费队列**均**清空：`tag_jobs`（pending+leased）与 `embedding_queues`（pending+processing）——日报的泳道分桶依赖标签向量到质心的距离，embedding 未清空会把文章落进「未归位（unmatched）」桶。因此合并对账端点 SHALL 在标签队列计数之外**同请求**下发 Embedding 队列计数，客户端 SHALL NOT 为 Embedding 队列另建周期性请求。

两个队列的量纲不同（标签队列按**文章**、Embedding 队列按**标签**），响应 SHALL 以独立字段分别承载，SHALL NOT 合并为同一个计数。客户端在 Embedding 队列计数不可得时 SHALL 按「未知」处理，SHALL NOT 断言该队列已清空。

#### Scenario: Embedding 队列计数随合并端点下发

- **WHEN** 客户端请求合并对账端点
- **THEN** 响应 SHALL 在与标签队列计数并列的位置给出 Embedding 队列的待处理、处理中、完成、失败与今日完成计数

#### Scenario: 两个队列的计数互不混淆

- **WHEN** 客户端读取合并对账端点的响应
- **THEN** 标签队列计数与 Embedding 队列计数 SHALL 来自两个独立字段，SHALL NOT 被合并为同一个计数

#### Scenario: Embedding 队列计数缺失时不谎称已清空

- **WHEN** 合并对账端点的响应不含 Embedding 队列计数
- **THEN** 客户端 SHALL 把该队列呈现为未知，SHALL NOT 呈现为「已清空」
