# ai-generated-article-detection Specification

## Purpose

AI 生成疑似度判定链路：对文章正文做一次轻量判定，给出 0-1 的置信分，并在超过阈值时把该文章标记为「疑似 AI 生成」。判定按订阅源独立开关控制，覆盖面由人逐源决定；判定结论可人工纠正且人工结论优先。

## Requirements

### Requirement: 独立的疑似 AI 判定作业队列

系统 SHALL 提供独立的疑似 AI 判定作业队列，与标签打标队列相互分离。判定作业 SHALL 以文章为单位，一篇判定作业对应一篇文章；同一篇文章存在未完成判定作业时 SHALL 复用该作业而非重复入队。判定作业 SHALL 有重试次数上限，超过上限后该作业 SHALL 终止为失败。

#### Scenario: 重复触发判定不产生重复作业

- **WHEN** 同一篇文章在已有未完成判定作业的情况下再次触发判定
- **THEN** 系统 SHALL 复用既有作业，SHALL NOT 为同一篇文章创建第二条未完成作业

#### Scenario: 数据库级唯一索引兜底并发入队

- **WHEN** 两个并发节点同时为同一篇文章入队判定作业（如抓取完成与入库即判定同秒到达）
- **THEN** 部分唯一索引 `uq_detection_jobs_article_active`（`(article_id) WHERE status IN ('pending','leased')`，迁移 20261120_0003）SHALL 拒绝第二条未完成作业
- **AND** 撞索引的入队 SHALL 按复用既有作业处理，SHALL NOT 报错中断调用方

#### Scenario: 判定失败不影响打标

- **WHEN** 一篇判定作业因能力未绑定模型而失败
- **THEN** 该文章的标签打标流程 SHALL NOT 受影响，SHALL NOT 因判定失败而中止或延迟

#### Scenario: 每篇文章一次独立判定

- **WHEN** 判定队列中有多篇文章等待判定
- **THEN** 每篇文章 SHALL 各自发起一次独立的判定调用，SHALL NOT 与其他文章合并为同一次调用

#### Scenario: 判定作业重试有上限

- **WHEN** 判定作业连续失败并达到重试次数上限
- **THEN** 该作业 SHALL 终止为失败，文章判定状态 SHALL 保持未判定

### Requirement: 订阅源级判定开关

每个订阅源 SHALL 具备独立的判定开关，默认关闭。开关关闭时该订阅源的文章 SHALL NOT 进入判定队列；开关的启停 SHALL 立即生效，无需重启服务。

#### Scenario: 新订阅源默认不判定

- **WHEN** 用户新建一个订阅源且未显式开启判定开关
- **THEN** 该订阅源的判定开关 SHALL 为关闭状态，其文章 SHALL NOT 进入判定队列

#### Scenario: 关闭后不再产生新判定

- **WHEN** 用户关闭某订阅源的判定开关
- **THEN** 该订阅源后续新入库的文章 SHALL NOT 进入判定队列

### Requirement: 判定输入的取用优先级

判定 SHALL 以文章正文为输入，取用优先级为：全文抓取正文 → 订阅源原始正文 → 订阅源摘要；取到的文本 SHALL 截断为定长样本（保留首段与尾段拼接），SHALL NOT 以整篇正文发起调用。

#### Scenario: 有全文时取全文

- **WHEN** 文章已完成全文抓取且抓取正文非空
- **THEN** 判定输入 SHALL 取自全文抓取正文

#### Scenario: 无全文时回退原始正文

- **WHEN** 文章未开启全文抓取，或抓取正文为空
- **THEN** 判定输入 SHALL 回退到订阅源原始正文；原始正文亦为空时 SHALL 再回退到订阅源摘要

#### Scenario: 长正文截断为样本

- **WHEN** 取到的正文长度超过判定样本长度上限
- **THEN** 判定输入 SHALL 截断为首段与尾段拼接的样本，SHALL NOT 发送整篇正文

### Requirement: 判定置信分与阈值

判定 SHALL 产出一个 0-1 的置信分。系统 SHALL 维护一个全局判定阈值，SHALL 可在线调整且调整后立即对后续判定生效，默认值为 0.7。置信分不低于阈值时该文章 SHALL 被标记为疑似 AI 生成；低于阈值时 SHALL NOT 标记。

#### Scenario: 置信分达阈值即标记

- **WHEN** 判定置信分为 0.86 且当前阈值为 0.7
- **THEN** 该文章 SHALL 被标记为疑似 AI 生成

#### Scenario: 置信分低于阈值不标记

- **WHEN** 判定置信分为 0.42 且当前阈值为 0.7
- **THEN** 该文章 SHALL NOT 被标记为疑似 AI 生成

#### Scenario: 阈值在线调整生效

- **WHEN** 用户把判定阈值从 0.7 调整为 0.9
- **THEN** 后续判定 SHALL 以 0.9 为准，SHALL NOT 需要改代码或重启服务

### Requirement: 判定结论的记录与呈现口径

每篇文章 SHALL 记录判定置信分、判定状态、判定来源与判定时间。判定状态 SHALL 至少区分未判定、疑似、已确认非 AI；判定来源 SHALL 区分自动判定与人工标记。判定状态为未判定的文章 SHALL NOT 对外呈现任何疑似标记，SHALL NOT 出现占位或「未检测」类文案。

#### Scenario: 未判定不呈现标记

- **WHEN** 文章判定状态为未判定
- **THEN** 文章列表、文章详情、版块归档视图与日报线索 SHALL 均不展示疑似 AI 标记，SHALL NOT 出现占位或「未检测」文案

#### Scenario: 自动判定来源可区分

- **WHEN** 判定结论由自动判定流程产生
- **THEN** 该文章的判定来源 SHALL 记为自动判定

### Requirement: 人工标记与人工结论优先

用户 SHALL 能在文章详情手动把文章标记为疑似 AI 生成或取消疑似 AI 标记。人工结论 SHALL 优先于自动结论：文章已被人工标记或人工取消后，后续自动判定 SHALL NOT 覆盖该人工结论，除非用户显式触发重新判定。

#### Scenario: 人工标记生效

- **WHEN** 用户在文章详情把文章标记为疑似 AI 生成
- **THEN** 该文章 SHALL 呈现疑似 AI 标记，判定来源 SHALL 记为人工标记

#### Scenario: 人工取消后不被自动结论覆盖

- **WHEN** 文章已被人工取消疑似标记，随后自动判定完成且置信分高于阈值
- **THEN** 该文章 SHALL 仍不呈现疑似 AI 标记，自动结论 SHALL NOT 覆盖人工结论

#### Scenario: 显式重判可覆盖人工结论

- **WHEN** 文章已被人工标记，用户显式触发重新判定且自动判定结论为非疑似
- **THEN** 该文章 SHALL 按自动判定结论呈现，判定来源 SHALL 更新为自动判定

### Requirement: 判定使用独立的 AI 能力

疑似 AI 判定 SHALL 通过专用的 AI 能力执行。该能力 SHALL 有独立的模型绑定与独立的并发配额，SHALL NOT 与标签打标共用模型绑定或并发配额。

#### Scenario: 判定与打标互不挤占

- **WHEN** 判定能力与标签打标能力分别绑定了不同模型，且判定能力并发配额已占满
- **THEN** 标签打标 SHALL NOT 因判定能力占满配额而被阻塞

### Requirement: 判定失败的静默降级

判定作业失败（能力未绑定可用模型、可用供应商全部降级失败）时，文章 SHALL 保持未判定状态，界面 SHALL NOT 呈现错误，失败原因 SHALL 进入 AI 调用审计。

#### Scenario: 能力未绑定模型时界面无错误

- **WHEN** 疑似 AI 判定能力未绑定任何可用模型
- **THEN** 判定作业 SHALL 失败并记入 AI 调用审计，该文章 SHALL 保持未判定，界面 SHALL NOT 出现错误提示

### Requirement: 存量文章不追溯判定

判定 SHALL 只作用于进入判定范围时的新文章；订阅源开启判定开关前已入库的存量文章 SHALL NOT 被自动追溯判定。

#### Scenario: 存量文章保持未判定

- **WHEN** 用户为某订阅源开启判定开关
- **THEN** 该订阅源开启前已入库的存量文章 SHALL 保持未判定状态
