## MODIFIED Requirements

### Requirement: 参与者身份与去重

`participant_key` SHALL 标识「一个独立参与方」，用于回答「有多少不同来源在说这件事」。参与者身份 SHALL 按**内容身份优先**判定：同一篇内容（`articles.link` 经规范化后相同）的多个 article 副本 SHALL 只贡献一个参与者；内容身份不可判定（link 为空或规范化后为空）时 SHALL 退化为 feed 粒度（`feed:<feed_id>`）。

link 规范化口径 SHALL 固定为：去首尾空白 → 去掉 `#` 之后的 fragment → 去掉 `?` 之后的 query → scheme 与 host 转小写（path 保持原样，不做大小写折叠）。该口径 SHALL 只用于参与者身份与热度计数，SHALL NOT 改写 `articles.link`，SHALL NOT 变更入库判重键 `(feed_id, link)`。

热度计算 SHALL 对每个参与者只计一次：同一参与者在窗口内多次报道同一事件线 SHALL NOT 叠加热度。

同一篇内容被多个 feed 抓取时 SHALL 只记一个参与者；不同内容的独立报道 SHALL 各记一个参与者，与 feed 数量无关。原「同一篇 article 被多个 feed 转载时各记一个参与者（第一版不识别转载）」的已知简化条目 SHALL 废止。

参与者计数的所有消费面（`story_signals`、`participant_count`、`story_heat`、`lane_heat`、雷达「N 个来源」、矩阵颜色分档）SHALL 使用同一份内容身份判定，SHALL NOT 出现某处按 feed 计、某处按内容计的混用。

#### Scenario: 同一 feed 多条不叠加
- **WHEN** feed X 在 48 小时内发布 5 条关于同一事件线的文章，且 5 条 link 规范化后相同（互为副本）或均不可判定
- **THEN** 该事件线的热度 SHALL 只按 1 个参与者计算
- **AND** 若 5 条 link 规范化后互不相同，则 SHALL 按 5 个独立参与者计——参与者粒度是内容身份，不是 feed（见本 Requirement 正文「与 feed 数量无关」）

#### Scenario: 不同 feed 各计一次
- **WHEN** 5 个不同 feed 各发布 1 条关于同一事件线的文章，且 5 条 link 规范化后互不相同
- **THEN** 该事件线的参与者数 SHALL 为 5

#### Scenario: 排除不可见证据
- **WHEN** 某 article 已被撤回、或被人为标记为不参与统计
- **THEN** 其 signal SHALL NOT 计入热度
- **AND** 仅因轮转出 feed 活跃窗口而 `archived=true` 的文章 SHALL NOT 被排除（归档不是撤回）

#### Scenario: 同链接多 feed 副本只算一个参与者
- **WHEN** 同一 URL 的文章被 feed A 与 feed B 各抓取一份（两行 `articles`、同标题同 `pub_date`），并都被归属到同一事件线
- **THEN** 该事件线的参与者数 SHALL 为 1
- **AND** 热度 SHALL 只按 1 个参与者的 `observed_at` 计算（示例：24h 半衰期下 12.5 小时后 SHALL 为 0.70，SHALL NOT 为 1.40）

#### Scenario: 规范化链接覆盖 fragment 与 query 差异
- **WHEN** 同一篇文章的副本 link 分别为 `https://example.com/p/1?utm=x` 与 `https://EXAMPLE.com/p/1#comments`
- **THEN** 两副本 SHALL 判为同一内容，只计 1 个参与者

#### Scenario: 链接缺失退化按 feed
- **WHEN** 某事件线的两篇证据文章 link 均为空
- **THEN** 参与者身份 SHALL 退化为 `feed:<feed_id>` 粒度，SHALL NOT 因内容身份不可判定而合并不同 feed

#### Scenario: 归档不影响参与者计入
- **WHEN** 参与者的文章已归档（轮转出 feed 活跃窗口）
- **THEN** 该参与者 SHALL 照常计入热度，SHALL NOT 因 `archived` 被过滤，参与者数亦 SHALL NOT 因 `archived` 减少
