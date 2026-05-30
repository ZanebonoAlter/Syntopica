## ADDED Requirements

### Requirement: Event tag keyword extraction
When `generateTagDescription` runs for an event category tag, the LLM SHALL return a `keywords` field (JSON array of short Chinese strings) alongside the `description`. Keywords SHALL represent the key entities, actors, locations, and actions involved in the event. The system SHALL store keywords in `topic_tags.metadata` JSONB under key `event_keywords`.

#### Scenario: LLM returns description and keywords
- **WHEN** `generateTagDescription` is called for event tag "美伊冲突" with article context about Iran missile attacks
- **THEN** the LLM response SHALL contain `{"description": "伊朗对以色列发动...", "keywords": ["美国", "伊朗", "袭击", "中东"]}` and the keywords SHALL be stored in `metadata.event_keywords`

#### Scenario: LLM returns empty keywords
- **WHEN** `generateTagDescription` is called and LLM returns `keywords: []`
- **THEN** the system SHALL store empty array in metadata and proceed with title embedding only

#### Scenario: Non-event tags skip keyword extraction
- **WHEN** `generateTagDescription` is called for a keyword or person category tag
- **THEN** the LLM prompt SHALL NOT include keywords request and metadata SHALL NOT contain `event_keywords`

### Requirement: Event tag multi-row embedding storage
For event category tags, the system SHALL store embedding rows in `topic_tag_embeddings` as:
- One title row with `embedding_type='semantic'`, embedding text = label + description
- N keyword rows with `embedding_type='event_keyword'`, one per keyword, embedding text = keyword string

Each row SHALL have a unique `text_hash` computed as `SHA256(embedding_type + "\n" + text)`. The unique index SHALL be `(topic_tag_id, embedding_type, text_hash)`.

#### Scenario: Event tag generates multiple embedding rows
- **WHEN** event tag "美伊冲突" has keywords ["美国","伊朗","袭击"]
- **THEN** `topic_tag_embeddings` SHALL contain 4 rows: 1 semantic + 3 event_keyword rows, each with distinct text_hash

#### Scenario: Keyword text hash prevents duplicate rows
- **WHEN** two event tags both have keyword "美国"
- **THEN** both rows SHALL have the same text_hash per tag but different topic_tag_id, no unique constraint violation

### Requirement: Event tag delayed embedding enrollment
When `findOrCreateTag` creates a new event category tag, the system SHALL NOT call `ensureTagEmbedding` (skip initial embedding queue enrollment). Instead, `generateTagDescription` SHALL trigger `qs.Enqueue(tagID)` after description and keywords are saved.

#### Scenario: Event tag creation skips embedding queue
- **WHEN** `findOrCreateTag` creates a new event tag
- **THEN** `ensureTagEmbedding` is NOT called; the tag has no embedding rows initially

#### Scenario: Event tag re-enqueued after description generation
- **WHEN** `generateTagDescription` completes successfully for an event tag with saved description and keywords
- **THEN** `qs.Enqueue(tagID)` is called, triggering embedding generation with full keyword set

#### Scenario: Non-event tags still enqueue on creation
- **WHEN** `findOrCreateTag` creates a new keyword or person tag
- **THEN** `ensureTagEmbedding` IS called as before; existing behavior unchanged

### Requirement: Weighted multi-keyword concept matching
`MatchTagToConcept` for event category tags SHALL retrieve all embedding rows with `embedding_type IN ('semantic', 'event_keyword')`. It SHALL compute cosine similarity for each row against concept embeddings, then calculate a weighted average: title rows (semantic) weight ×2, keyword rows (event_keyword) weight ×1. The weighted average SHALL be compared against the configured threshold.

#### Scenario: Event tag matches concept via weighted average
- **WHEN** event tag "美伊冲突" has title sim=0.8 and keyword sims=[0.6, 0.75, 0.5] against concept "中东冲突"
- **THEN** weighted average = (0.8×2 + 0.6 + 0.75 + 0.5) / 5 = 0.69, compared against threshold 0.7 → no match

#### Scenario: Title-only event tag (no keywords) uses single embedding
- **WHEN** event tag has only semantic embedding (no keywords)
- **THEN** `MatchTagToConcept` SHALL use the single semantic embedding row as before; weighted average is identical to direct similarity

#### Scenario: Non-event tags use single embedding unchanged
- **WHEN** `MatchTagToConcept` is called for keyword or person tags
- **THEN** matching SHALL use single semantic embedding row as before; no weighting applied
