## REMOVED Requirements

### Requirement: 14 天档内容与降级

**Reason**: 长短两版产物形态废除后，「长版优先、长版缺失回退短版+提示」的降级链整体不再存在，14 天档内容源收敛为单版态势句（见 ADDED「14 天档内容」）。

**Migration**: 前端不再读取 `rolling_detail`；存量长版在视图上不再渲染，也无「长版待生成」提示；`rolling_summary` 语义升级为演进叙事（展示面无破坏——仍为一段纯文本全文）。

## ADDED Requirements

### Requirement: 14 天档内容

「近 14 天」档 SHALL 展示该泳道滚动 14 天态势的**单版演进叙事态势句**（快照 `rolling_summary`）全文，SHALL NOT 消费 `rolling_detail`（含存量历史长版）；态势快照整体缺失时 SHALL 展示待结算占位且三档切换仍可用。

#### Scenario: 态势句就绪

- **WHEN** 该泳道快照含态势句
- **THEN** 14 天档 SHALL 展示态势句全文与汇总截止日

#### Scenario: 存量长版不消费

- **WHEN** 该泳道快照的 `rolling_detail` 含存量历史长版
- **THEN** 14 天档 SHALL 仅渲染 `rolling_summary`，SHALL NOT 渲染长版，也不展示任何「长版待生成」类提示

#### Scenario: 快照缺失

- **WHEN** 该泳道无态势快照
- **THEN** 14 天档 SHALL 展示待结算占位，用户仍可切换月/年档
