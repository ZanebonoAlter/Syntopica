## MODIFIED Requirements

### Requirement: 关注标记 AI 命中判定

日报生成流程末尾（section 持久化与 persistent_topic 归属完成之后），系统 SHALL 对该 board 下所有 `status=active` 且 `type=label` 的关注标记执行 AI 命中判定：将该期日报的全部 section 与每个关注的 label 一并提交 AI，判定哪些 section 与该关注相关。`type=keyword` 的关注标记 SHALL 走纯文本匹配（零 AI），物化轨关注（`type=keyword_topic` / `type=sentence_topic`）SHALL 被跳过（既不走 AI 判定也不走文本匹配，SHALL NOT 产生命中提示记录）。

label 轨判定 SHALL 走 **AI 单信号**（SHALL NOT 使用 embedding 相似度，SHALL NOT 走 persistent_topic 的 embedding+LLM 双重确认 AND-gate），因为关注是用户意图声明而非聚类产物。

判定结果 SHALL 记录到 `topic_watch_hits` 表：`watch_id` / `section_id` / `report_id` / `period_date`，并带 AI 给出的命中理由一句话。

#### Scenario: 命中记录

- **WHEN** 关注「美伊会不会真打起来」对某期日报的 section #123 命中
- **THEN** 系统 SHALL 写入 `topic_watch_hits(watch_id, section_id=123, report_id, period_date, reason)`

#### Scenario: 不走双重确认

- **WHEN** 某 section 与关注的 embedding 相似度很低，但 AI 判定相关
- **THEN** 系统 SHALL 仍记录命中（AI 单信号即生效），SHALL NOT 因 embedding 距离否决

#### Scenario: 批量单次请求

- **WHEN** 同一期日报有 N 个 section、M 个 active label 关注
- **THEN** AI 命中判定 SHALL 以批量方式调用（单期日报的 section 与关注在一次或按关注分组的少量请求内完成），SHALL NOT 对每个 section 单独发起请求

#### Scenario: 物化轨关注不进入判定

- **GIVEN** board 下存在 `type=sentence_topic` 与 `type=keyword_topic` 的 active 关注
- **WHEN** 日报生成完成，AI 命中判定执行
- **THEN** 这两类关注 SHALL NOT 参与 AI 命中判定，SHALL NOT 产生任何 `topic_watch_hits` 记录
