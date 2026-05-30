## MODIFIED Requirements

### Requirement: Daily report excludes direction_mismatch tags

collectBoardTags 的主查询和 fallback 补算均排除 direction_mismatch=true 的标签。主查询添加 `AND NOT COALESCE(topic_tag_board_labels.direction_mismatch, false)` 条件。fallback 补算后过滤匹配结果时排除 direction_mismatch=true。

#### Scenario: main query excludes direction_mismatch
- **WHEN** tag has direction_mismatch=true for target board
- **THEN** tag excluded from daily report main query results

#### Scenario: fallback excludes direction_mismatch
- **WHEN** fallback recalculation produces a match with direction_mismatch=true for target board
- **THEN** that tag excluded from daily report
