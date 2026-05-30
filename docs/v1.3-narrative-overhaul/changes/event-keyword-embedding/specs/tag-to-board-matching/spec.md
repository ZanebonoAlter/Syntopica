## MODIFIED Requirements

### Requirement: Embedding-based tag-to-board matching
The system SHALL match tags to board concepts using cosine similarity of their embeddings against `board_concepts.embedding`. For event category tags, all embedding rows (semantic title + event_keyword rows) SHALL be used with weighted averaging: title rows weight ×2, keyword rows weight ×1. For keyword and person tags, the single semantic embedding SHALL be used as before.

#### Scenario: Event tag matches via weighted multi-keyword
- **WHEN** event tag "美伊冲突" has title sim=0.8 and keyword sims=[0.6, 0.75, 0.5] against concept "中东冲突"
- **THEN** weighted average SHALL be computed and compared against the configured threshold

#### Scenario: Tag falls below threshold for all concepts
- **WHEN** tag "某不知名标签" has max weighted similarity=0.35 against all concepts
- **THEN** the tag is placed in the "unclassified" bucket

#### Scenario: Non-event tag uses single embedding unchanged
- **WHEN** keyword tag "人工智能" has embedding matched against concepts
- **THEN** the single semantic embedding row SHALL be used for cosine similarity; no weighting applied
