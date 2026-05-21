## ADDED Requirements

### Requirement: ExtractionInput carries article publication date
The `ExtractionInput` struct SHALL include a `PubDate` string field containing the article's publication date in `YYYY-MM-DD` format. The `buildExtractionUserPrompt` function SHALL include this date in the LLM prompt as `发布日期: <date>`.

#### Scenario: PubDate included in extraction prompt
- **WHEN** `buildExtractionUserPrompt` is called with an `ExtractionInput` that has `Title="特朗普访华"` and `PubDate="2025-05-10"`
- **THEN** the generated prompt SHALL contain the line `发布日期: 2025-05-10`

#### Scenario: Empty PubDate handled gracefully
- **WHEN** `buildExtractionUserPrompt` is called with an `ExtractionInput` that has `PubDate=""`
- **THEN** the generated prompt SHALL NOT contain a `发布日期` line (field omitted)

### Requirement: Article context includes publication date for description generation
When building `articleContext` for tag description generation (`generateTagDescription`), the system SHALL prepend the article's publication date in the format `[日期: YYYY-MM-DD]` before the title and summary content.

#### Scenario: Date prepended to article context
- **WHEN** building article context for an article with `PubDate=2025-05-10` and `Title="特朗普抵达北京"`
- **THEN** the context SHALL start with `[日期: 2025-05-10]` followed by the title and summary

#### Scenario: Date omitted when PubDate is zero
- **WHEN** building article context for an article with a zero/empty `PubDate`
- **THEN** the context SHALL NOT include a date prefix (only title and summary)

### Requirement: Event clustering candidates carry date ranges
When building the prompt for `ExtractAbstractTag` in the context of event keyword clustering, each candidate tag's context SHALL include the date range of its associated articles in the format `(最早文章: YYYY-MM-DD, 最新: YYYY-MM-DD)`.

#### Scenario: Date range in clustering candidate context
- **WHEN** a clustering candidate tag "特朗普抵达北京" has articles published between 2025-05-08 and 2025-05-12
- **THEN** the tag's context in the LLM prompt SHALL include `(最早文章: 2025-05-08, 最新: 2025-05-12)`

#### Scenario: Single-article tag shows single date
- **WHEN** a clustering candidate tag has only one associated article published on 2025-05-10
- **THEN** the tag's context SHALL show `(文章日期: 2025-05-10)`

#### Scenario: No articles found for tag
- **WHEN** a clustering candidate tag has zero associated articles (unlikely but defensive)
- **THEN** the tag's context SHALL NOT include a date range (empty string appended)
