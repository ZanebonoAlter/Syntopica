## ADDED Requirements

### Requirement: 生成结束后清理空 board

`GenerateAndSaveForCategory` 和 `GenerateAndSave` 完成后，系统 SHALL 删除当天日期范围内无任何 `narrative_summaries` 关联的 `narrative_boards` 记录。

清理范围 MUST 限定为当天（`period_date >= startOfDay AND period_date < endOfDay`）。对于分类生成，额外限定 `scope_category_id`。

#### Scenario: 正常生成后无空 board 残留

- **WHEN** `GenerateAndSaveForCategory` 完成，所有 board 都有关联的 narrative
- **THEN** 无 board 被删除，日志记录清理数量为 0

#### Scenario: LLM 返回空数组导致空 board

- **WHEN** 某个 board 的 `GenerateNarrativesForBoard` 返回空数组，`SaveNarrativesForBoard` 不写入任何 summary
- **THEN** 生成结束后该 board 被删除，日志记录清理数量

#### Scenario: event tags 变 inactive 导致空 board

- **WHEN** 某个 board 创建后 `LoadBoardEventTags` 返回空（tags 变为 inactive），board 被跳过
- **THEN** 生成结束后该 board 被删除

#### Scenario: LLM 调用失败导致空 board

- **WHEN** 某个 board 的 `GenerateNarrativesForBoard` 返回 error，board 被跳过
- **THEN** 生成结束后该 board 被删除

### Requirement: 全局生成后清理空 board

`GenerateAndSave` 完成所有分类生成和全局合并后，系统 SHALL 清理当天无 narrative 关联的全局 scope boards。

#### Scenario: 全局合并产生空 board

- **WHEN** `MergeGlobalBoards` 创建了全局 board 但后续未生成 narrative
- **THEN** `GenerateAndSave` 结束后该全局 board 被删除
